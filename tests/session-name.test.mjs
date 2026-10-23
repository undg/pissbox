import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdirSync, mkdtempSync, symlinkSync, writeFileSync } from "node:fs";
import path from "node:path";
import { createRequire } from "node:module";
import { setImmediate } from "node:timers/promises";
import test from "node:test";
import { fileURLToPath, pathToFileURL } from "node:url";
import { resolvePiEntry } from "./pi-installation.mjs";

// Use the installed Pi dependencies and the same alias its extension loader uses.
const piEntry = resolvePiEntry(execFileSync("sh", ["-c", "command -v pi"], { encoding: "utf8" }).trim());
const piRequire = createRequire(piEntry);
const { SessionManager } = await import(pathToFileURL(piEntry));
const { createJiti } = piRequire("jiti");
const { Check } = piRequire("typebox/value");
const jiti = createJiti(import.meta.url, {
	fsCache: false,
	alias: { typebox: piRequire.resolve("typebox") },
});
const extension = await jiti.import("../pi-agent/extensions/session-name.js", { default: true });

test("resolves Pi's API through npm symlinks and Homebrew wrappers", () => {
	const fixtures = fileURLToPath(new URL("../.cache/test-fixtures/", import.meta.url));
	mkdirSync(fixtures, { recursive: true });
	const root = mkdtempSync(path.join(fixtures, "pi-installation-"));
	const npmDist = path.join(root, "npm/node_modules/@earendil-works/pi-coding-agent/dist");
	mkdirSync(npmDist, { recursive: true });
	writeFileSync(path.join(npmDist, "cli.js"), "");
	writeFileSync(path.join(npmDist, "index.js"), "");
	const npmBin = path.join(root, "npm/bin");
	mkdirSync(npmBin);
	symlinkSync(path.join(npmDist, "cli.js"), path.join(npmBin, "pi"));
	assert.equal(resolvePiEntry(path.join(npmBin, "pi")), path.join(npmDist, "index.js"));

	for (const location of ["libexec/lib", "libexec"]) {
		const formula = path.join(root, location.replaceAll("/", "-"));
		const packageRoot = path.join(formula, location, "node_modules/@earendil-works/pi-coding-agent");
		mkdirSync(path.join(packageRoot, "dist"), { recursive: true });
		writeFileSync(path.join(packageRoot, "package.json"), JSON.stringify({ main: "dist/index.js" }));
		writeFileSync(path.join(packageRoot, "dist/index.js"), "");
		mkdirSync(path.join(formula, "bin"));
		const wrapper = path.join(formula, "bin/pi");
		writeFileSync(wrapper, "#!/bin/sh\n# Homebrew executable wrapper\n");
		const link = path.join(root, location.replaceAll("/", "-") + "-pi");
		symlinkSync(wrapper, link);
		assert.equal(resolvePiEntry(link), path.join(packageRoot, "dist/index.js"));
	}
	const missing = path.join(root, "unsupported-pi");
	writeFileSync(missing, "");
	assert.throws(() => resolvePiEntry(missing), /Cannot locate Pi's JavaScript package/);
});

function session({ mode = "tui", name, entries = [
	{ type: "model_change", provider: "current-provider", modelId: "current-model" },
	{ type: "thinking_level_change", thinkingLevel: "medium" },
] } = {}) {
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
		ctx, entries, requests, saved, emit, tools,
		rename: (name) => tools.get("set_session_name").execute("rename", { name }),
		prompt(text = "Fix login redirect loop") {
			const result = emit("before_agent_start", { messageType: "user", prompt: text });
			entries.push({ type: "message", message: { role: "user", content: text } });
			return result;
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

test("names a new session on its first prompt without waiting for the task to finish", async () => {
	const s = session();
	s.emit("session_start");
	assert.equal(s.requests.length, 0);
	assert.equal(s.prompt("Fix login redirect loop " + "x".repeat(5000)), undefined,
		"prompt submission must not wait for the title request");
	assert.equal(s.requests.length, 1, "start naming before any assistant response or settlement");
	s.prompt("Second message must not enter the naming request");
	assert.equal(s.requests.length, 1);
	assert.equal(s.requests[0].model, s.ctx.model);
	assert.equal(s.requests[0].context.messages.length, 1);
	assert.equal(s.requests[0].context.messages[0].content.length, 4000);
	assert.equal(s.requests[0].context.messages[0].role, "user");
	assert.ok(s.requests[0].context.systemPrompt.trim(), "title generation needs its own system instructions");
	assert.match(s.requests[0].context.messages[0].content, /^Fix login redirect loop/);
	assert.equal(s.requests[0].options.maxRetries, 0);
	assert.equal(s.requests[0].options.reasoning, "minimal");
	assert.equal(s.requests[0].options.cacheRetention, "none");
	s.requests[0].resolve(title('"Fix login redirect loop"'));
	await setImmediate();
	assert.deepEqual(s.saved, ["Fix login redirect loop"]);
	s.settle();
	assert.equal(s.requests.length, 1);
});

test("auto-names a session initialized with Pi's real startup metadata", async () => {
	const manager = SessionManager.inMemory(process.cwd());
	manager.appendModelChange("current-provider", "current-model");
	manager.appendThinkingLevelChange("medium");
	const s = session({ entries: manager.getBranch() });
	s.emit("session_start");
	s.prompt();
	assert.equal(s.requests.length, 1, "startup metadata must not count as an existing conversation");
	s.requests[0].resolve(title("Fix login redirect loop"));
	await setImmediate();
	assert.deepEqual(s.saved, ["Fix login redirect loop"]);
});

test("leaves existing, resumed, forked, reloaded and headless conversations alone", () => {
	for (const s of [
		session({ name: "Manual title" }),
		session({ entries: [{ type: "message", message: { role: "user", content: "Old conversation" } }] }),
		...["compaction", "branch_summary", "custom_message"].map((type) => session({ entries: [{ type }] })),
		...["rpc", "json", "print"].map((mode) => session({ mode })),
	]) {
		s.emit("session_start");
		s.prompt();
		s.settle();
		assert.equal(s.requests.length, 0);
		assert.deepEqual(s.saved, []);
	}
});

test("a session metadata change before the first prompt disables automatic naming", () => {
	const s = session();
	s.emit("session_start");
	s.emit("session_info_changed", { name: undefined });
	s.prompt();
	assert.equal(s.requests.length, 0);
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
	// The old request settles while the replacement is still pending. Its
	// cleanup must not clear the new controller.
	s.requests[0].resolve(title("Stale title"));
	await setImmediate();
	assert.deepEqual(s.saved, []);
	assert.equal(s.requests[1].options.signal.aborted, false);
	s.requests[1].resolve(title("New task title"));
	await setImmediate();
	assert.deepEqual(s.saved, ["New task title"]);
	assert.equal(s.requests[1].context.messages[0].content, "New task");
});

test("waits for a user text prompt rather than custom messages or image-only prompts", async () => {
	const s = session();
	s.emit("session_start");
	s.emit("before_agent_start", { messageType: "custom" });
	s.prompt("");
	s.prompt(" \t\n ");
	assert.equal(s.requests.length, 0);
	s.prompt();
	assert.equal(s.requests.length, 1);
	s.requests[0].resolve(title("Fix login redirect loop"));
	await setImmediate();
	assert.deepEqual(s.saved, ["Fix login redirect loop"]);
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
		s.prompt("Another task must not retry a failed title request");
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
	assert.deepEqual(result, {
		content: [{ type: "text", text: "Session named: Refresh session naming workflow" }],
		details: { name: "Refresh session naming workflow" },
	});
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

test("title normalization removes controls and surrounding quotes without losing task text", async () => {
	for (const [input, expected] of [
		["  ``  Fix\u0000\u0085   login's redirect  ''  ", "Fix login's redirect"],
		["x".repeat(81), "x".repeat(80)],
	]) {
		const automatic = session();
		automatic.emit("session_start");
		automatic.prompt();
		automatic.requests[0].resolve(title(input));
		await setImmediate();
		assert.deepEqual(automatic.saved, [expected]);
		const manual = session({ name: "Old title" });
		const result = await manual.rename(input);
		assert.deepEqual(manual.saved, [expected]);
		assert.equal(result.details.name, expected);
	}
});

test("only text response parts contribute to a generated title", async () => {
	const s = session();
	s.emit("session_start");
	s.prompt();
	s.requests[0].resolve({ stopReason: "stop", content: [
		{ type: "thinking", text: "Not part of the title" },
		{ type: "text", text: "Fix" },
		{ type: "text", text: "login redirect" },
	] });
	await setImmediate();
	assert.deepEqual(s.saved, ["Fix login redirect"]);
});

test("waits for session startup and an available model, but ignores system-only history", async () => {
	const s = session({ entries: [{ type: "message", message: { role: "system", content: "System metadata" } }] });
	s.emit("before_agent_start", { messageType: "user", prompt: "Before session startup" });
	assert.equal(s.requests.length, 0);
	s.emit("session_start");
	const model = s.ctx.model;
	s.ctx.model = undefined;
	s.prompt();
	assert.equal(s.requests.length, 0);
	s.ctx.model = model;
	s.prompt();
	assert.equal(s.requests.length, 1);
	s.requests[0].resolve(title("Fix login redirect"));
	await setImmediate();
	assert.deepEqual(s.saved, ["Fix login redirect"]);
});

test("a provider result arriving after the naming deadline is ignored", async (t) => {
	const deadline = new AbortController();
	t.mock.method(AbortSignal, "timeout", (milliseconds) => {
		assert.equal(milliseconds, 15000);
		return deadline.signal;
	});
	const s = session();
	s.emit("session_start");
	s.prompt();
	deadline.abort();
	s.requests[0].resolve(title("Late provider result"));
	await setImmediate();
	assert.equal(s.requests[0].options.signal.aborted, true);
	assert.deepEqual(s.saved, []);
});

test("rename tool schema requires a title between one and eighty characters", () => {
	const schema = session().tools.get("set_session_name").parameters;
	assert.equal(Check(schema, { name: "Title" }), true);
	assert.equal(Check(schema, { name: "x".repeat(80) }), true);
	for (const input of [{}, { name: "" }, { name: "x".repeat(81) }, { name: 12 }]) {
		assert.equal(Check(schema, input), false, JSON.stringify(input));
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
