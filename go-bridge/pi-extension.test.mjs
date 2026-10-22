import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, readFileSync, readdirSync, utimesSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";
import extension from "../pi-agent/extensions/pissbox-bridge.js";
import "./agent-messaging.test.mjs";

const fixtures = fileURLToPath(new URL("../.cache/test-fixtures/", import.meta.url));
mkdirSync(fixtures, { recursive: true });

function session(mode = "tui", ready = true) {
	const mailbox = mkdtempSync(path.join(fixtures, "extension-"));
	if (ready) writeFileSync(path.join(mailbox, "ready"), "2\n");
	const previous = process.env.PISSBOX_BRIDGE_DIR;
	process.env.PISSBOX_BRIDGE_DIR = mailbox;
	const handlers = new Map();
	const warnings = [];
	let idle = true;
	try {
		extension({
			on: (name, handler) => handlers.set(name, [...(handlers.get(name) ?? []), handler]),
			registerTool() {},
		});
	} finally {
		if (previous === undefined) delete process.env.PISSBOX_BRIDGE_DIR;
		else process.env.PISSBOX_BRIDGE_DIR = previous;
	}
	const ctx = {
		mode, cwd: "/workspace", isIdle: () => idle,
		sessionManager: { getSessionId: () => "test", getSessionName: () => undefined },
		ui: { notify: (message) => warnings.push(message) },
	};
	return {
		mailbox,
		warnings,
		setIdle(value) { idle = value; },
		emit(name) { for (const handler of handlers.get(name) ?? []) handler({}, ctx); },
		events() {
			return readdirSync(mailbox).filter((name) => name.endsWith(".json")).sort()
				.map((name) => JSON.parse(readFileSync(path.join(mailbox, name), "utf8")));
		},
	};
}

test("preserves fast state transitions, nested prompts, final settlement and release", () => {
	const s = session();
	s.emit("session_start");
	s.setIdle(false);
	s.emit("agent_start");
	s.emit("agent_start");
	s.emit("ui_prompt_start");
	s.emit("ui_prompt_start");
	s.emit("ui_prompt_end");
	s.emit("ui_prompt_end");
	// agent_end may be followed by retries or queued work; only settled finishes.
	s.emit("agent_end");
	s.emit("agent_settled");
	s.setIdle(true);
	s.emit("agent_settled");
	s.emit("session_shutdown");
	s.emit("session_shutdown");
	assert.deepEqual(s.events(), [
		{ version: 1, kind: "state", state: "idle" },
		{ version: 1, kind: "state", state: "working" },
		{ version: 1, kind: "state", state: "blocked" },
		{ version: 1, kind: "state", state: "working" },
		{ version: 1, kind: "state", state: "idle" },
		{ version: 1, kind: "release" },
	]);
	assert.deepEqual(s.warnings, []);
});

test("does not report Herd state for headless sessions or a stale mailbox", () => {
	for (const s of [session("rpc"), session("json"), session("tui", false)]) {
		s.emit("session_start");
		s.emit("agent_start");
		s.emit("ui_prompt_start");
		s.emit("session_shutdown");
		assert.deepEqual(s.events(), []);
		assert.deepEqual(s.warnings, []);
	}
	const stale = session();
	utimesSync(path.join(stale.mailbox, "ready"), new Date(0), new Date(0));
	stale.emit("session_start");
	assert.deepEqual(stale.events(), []);
});

test("reload startup reflects a busy run and helper failure warns only once", () => {
	const s = session();
	s.setIdle(false);
	s.emit("session_start");
	assert.deepEqual(s.events(), [{ version: 1, kind: "state", state: "working" }]);
	utimesSync(path.join(s.mailbox, "ready"), new Date(0), new Date(0));
	s.emit("ui_prompt_start");
	s.emit("ui_prompt_end");
	s.emit("session_shutdown");
	assert.equal(s.warnings.length, 1);
	assert.equal(s.events().length, 1);
});
