import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { realpathSync } from "node:fs";
import { createRequire } from "node:module";
import { setImmediate } from "node:timers/promises";
import test from "node:test";

// Use the installed Pi dependencies and the same alias its extension loader uses.
const piRequire = createRequire(realpathSync(execFileSync("sh", ["-c", "command -v pi"], { encoding: "utf8" }).trim()));
const { createJiti } = piRequire("jiti");
const jiti = createJiti(import.meta.url, {
	fsCache: false,
	alias: { typebox: piRequire.resolve("typebox") },
});
const extension = await jiti.import("../pi-agent/extensions/session-name.js", { default: true });

function session({ mode = "tui", name, entries = [] } = {}) {
	const handlers = new Map();
	const tools = new Map();
	const requests = [];
	const saved = [];
	const ctx = {
		mode,
		model: { id: "current-model", provider: "current-provider" },
		sessionManager: { getBranch: () => entries },
		modelRegistry: {
			streamSimple(model, context, options) {
				const { promise, resolve, reject } = Promise.withResolvers();
				requests.push({ model, context, options, resolve, reject });
				return { result: () => promise };
			},
		},
	};
	extension({
		on: (event, handler) => handlers.set(event, handler),
		registerTool: (tool) => tools.set(tool.name, tool),
		getSessionName: () => name,
		setSessionName(value) {
			name = value;
			saved.push(value);
			handlers.get("session_info_changed")?.({ name }, ctx);
		},
	});
	const emit = (event, data = {}) => handlers.get(event)?.(
		event === "session_start" ? { reason: "startup", ...data } : data, ctx,
	);
	return {
		ctx, entries, requests, saved, emit,
		rename: (name) => tools.get("set_session_name").execute("rename", { name }),
		prompt(text = "Fix login redirect loop") {
			emit("before_agent_start", { messageType: "user", prompt: text });
			entries.push({ type: "message", message: { role: "user", content: text } });
		},
		settle(stopReason = "stop") {
			entries.push({ type: "message", message: { role: "assistant", stopReason } });
			return emit("agent_settled", { aborted: false });
		},
	};
}

function title(text, stopReason = "stop") {
	return { stopReason, content: [{ type: "text", text }] };
}

test("names a new session once, in the background, using only the bounded first prompt", async () => {
	const s = session();
	s.emit("session_start");
	s.prompt("Fix login redirect loop " + "x".repeat(5000));
	s.prompt("Second message must not enter the naming request");
	assert.equal(s.requests.length, 0);
	assert.equal(s.settle(), undefined, "settlement must not wait for the title request");
	assert.equal(s.requests.length, 1);
	assert.equal(s.requests[0].model, s.ctx.model);
	assert.equal(s.requests[0].context.messages.length, 1);
	assert.equal(s.requests[0].context.messages[0].content.length, 4000);
	assert.match(s.requests[0].context.messages[0].content, /^Fix login redirect loop/);
	assert.equal(s.requests[0].options.maxRetries, 0);
	s.requests[0].resolve(title('"Fix login redirect loop"'));
	await setImmediate();
	assert.deepEqual(s.saved, ["Fix login redirect loop"]);
	s.settle();
	assert.equal(s.requests.length, 1);
});

test("leaves existing, resumed, forked, reloaded and headless conversations alone", () => {
	for (const s of [
		session({ name: "Manual title" }),
		session({ entries: [{ type: "message", message: { role: "user", content: "Old conversation" } }] }),
		...["rpc", "json", "print"].map((mode) => session({ mode })),
	]) {
		s.emit("session_start");
		s.prompt();
		s.settle();
		assert.equal(s.requests.length, 0);
		assert.deepEqual(s.saved, []);
	}
});

test("skips even empty resumed, forked or reloaded sessions", () => {
	for (const reason of ["resume", "fork", "reload"]) {
		const s = session();
		s.emit("session_start", { reason });
		s.prompt();
		s.settle();
		assert.equal(s.requests.length, 0);
	}
});

test("a new session gets its own request and cannot receive the previous session's title", async () => {
	const s = session();
	s.emit("session_start");
	s.prompt("Old task");
	s.settle();
	s.entries.length = 0;
	s.emit("session_start", { reason: "new" });
	s.prompt("New task");
	s.settle();
	s.requests[1].resolve(title("New task title"));
	s.requests[0].resolve(title("Stale title"));
	await setImmediate();
	assert.deepEqual(s.saved, ["New task title"]);
	assert.equal(s.requests[1].context.messages[0].content, "New task");
});

test("does not generate a title for an aborted or failed exchange", () => {
	const s = session();
	s.emit("session_start");
	s.prompt();
	s.emit("agent_settled", { aborted: true });
	s.settle("error");
	assert.equal(s.requests.length, 0);
});

test("generation failures and empty output are silent and never retried", async () => {
	for (const result of [new Error("Provider unavailable"), title(""), title("partial", "error"), title("cut off", "length")]) {
		const s = session();
		s.emit("session_start");
		s.prompt();
		s.settle();
		if (result instanceof Error) s.requests[0].reject(result);
		else s.requests[0].resolve(result);
		await setImmediate();
		assert.deepEqual(s.saved, []);
		s.settle();
		assert.equal(s.requests.length, 1);
	}
});

test("manual rename wins over an in-flight automatic title", async () => {
	const s = session();
	s.emit("session_start");
	s.prompt();
	s.settle();
	const result = await s.rename("Refresh session naming workflow");
	assert.equal(result.details.name, "Refresh session naming workflow");
	assert.equal(s.requests[0].options.signal.aborted, true);
	s.requests[0].resolve(title("Stale automatic title"));
	await setImmediate();
	assert.deepEqual(s.saved, ["Refresh session naming workflow"]);
	s.settle();
	assert.equal(s.requests.length, 1);
});

test("/name changes, session switches and shutdown cancel pending generation", async () => {
	for (const event of ["session_info_changed", "session_start", "session_shutdown"]) {
		const s = session();
		s.emit("session_start");
		s.prompt();
		s.settle();
		s.emit(event);
		assert.equal(s.requests[0].options.signal.aborted, true);
		s.requests[0].resolve(title("Wrong session title"));
		await setImmediate();
		assert.deepEqual(s.saved, []);
	}
});

test("on-demand tool replaces existing names and rejects blank titles", async () => {
	const s = session({ name: "Old title" });
	s.emit("session_start");
	await s.rename("  Current\nconversation goal  ");
	assert.deepEqual(s.saved, ["Current conversation goal"]);
	await assert.rejects(s.rename(" \n "), /must not be empty/);
	assert.deepEqual(s.saved, ["Current conversation goal"]);
});
