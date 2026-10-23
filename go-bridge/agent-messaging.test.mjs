import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, renameSync, symlinkSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";
import extension from "../pi-agent/extensions/pissbox-bridge.js";

const root = fileURLToPath(new URL("../", import.meta.url));
const fixtures = process.env.PISSBOX_BRIDGE_TEST_FIXTURE_DIR || path.join(root, ".cache/test-fixtures");
mkdirSync(fixtures, { recursive: true });
const delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
async function waitFor(check) {
	const deadline = Date.now() + 6000;
	while (Date.now() < deadline) {
		if (await check()) return;
		await delay(30);
	}
	assert.fail("Messaging condition did not become true");
}
function nativeSession(mailbox, mode = "rpc") {
	const handlers = new Map();
	const tools = new Map();
	const messages = [];
	let idle = true;
	let sessionID = "first-session";
	let sessionName = "First";
	const ctx = {
		mode, hasUI: false, cwd: "/workspace", isIdle: () => idle,
		sessionManager: { getSessionId: () => sessionID, getSessionName: () => sessionName },
		ui: { notify() {} },
	};
	const previous = process.env.PISSBOX_BRIDGE_DIR;
	process.env.PISSBOX_BRIDGE_DIR = mailbox;
	try {
		extension({
			on(name, handler) { handlers.set(name, [...(handlers.get(name) ?? []), handler]); },
			registerTool(tool) { tools.set(tool.name, tool); },
			sendMessage(message, options) { messages.push({ message, options, wasIdle: idle }); },
		});
	} finally {
		if (previous === undefined) delete process.env.PISSBOX_BRIDGE_DIR;
		else process.env.PISSBOX_BRIDGE_DIR = previous;
	}
	return {
		messages,
		emit(name) { for (const handler of handlers.get(name) ?? []) handler({}, ctx); },
		call(name, params = {}, signal) { return tools.get(name).execute("call", params, signal); },
		busy(value) { idle = !value; },
		rename(value) { sessionName = value; this.emit("session_info_changed"); },
		replace() { this.emit("session_shutdown"); sessionID = "replacement"; this.emit("session_start"); },
		identity() {
			for (const id of readdirSync(path.join(mailbox, "agents"))) {
				try {
					const presence = JSON.parse(readFileSync(path.join(mailbox, "agents", id, "presence.json"), "utf8"));
					if (presence.agent.sessionId === sessionID) return presence.agent;
				} catch { /* Retired address. */ }
			}
			assert.fail("No presence for this session");
		},
	};
}
function daemon(t) {
	const dir = mkdtempSync(path.join(fixtures, "am-"));
	const binary = path.join(dir, "bridge");
	execFileSync("go", ["build", "-o", binary, "."], { cwd: path.join(root, "go-bridge"), timeout: 60000 });
	const env = { ...process.env, PISSBOX_BRIDGE_RUNTIME_DIR: dir };
	const cli = (...args) => execFileSync(binary, args, { env, encoding: "utf8", timeout: 6000 });
	cli("ensure");
	t.after(async () => {
		cli("stop");
		await waitFor(() => !existsSync(path.join(dir, "control.sock")));
	});
	return () => {
		const mailbox = mkdtempSync(path.join(fixtures, "message-root-"));
		cli("register", "--mailbox", mailbox, "--owner-pid", String(process.pid));
		return mailbox;
	};
}
function inboxRecord(mailbox, identity, message) {
	const inbox = path.join(mailbox, "agents", identity.id, "inbox");
	const temporary = path.join(inbox, ".test.pending");
	writeFileSync(temporary, JSON.stringify(message));
	renameSync(temporary, path.join(inbox, `${message.id}.json`));
}

test("Pi tools exchange acknowledged messages, native queue options, replies and simultaneous sends", async (t) => {
	const mailbox = daemon(t);
	const a = nativeSession(mailbox(), "rpc");
	const b = nativeSession(mailbox(), "json");
	t.after(() => { a.emit("session_shutdown"); b.emit("session_shutdown"); });
	a.emit("session_start"); b.emit("session_start");
	let selfA, selfB;
	await waitFor(async () => {
		const result = (await a.call("agent_list")).details;
		selfA = result.self; selfB = result.peers?.[0]; return !!selfB;
	});
	const receipt = (await a.call("agent_send", { to: selfB.id, text: "/not-a-command" })).details;
	assert.equal(receipt.status, "queued");
	assert.equal(b.messages.length, 1);
	assert.equal(b.messages[0].wasIdle, true);
	assert.deepEqual(b.messages[0].options, { deliverAs: "followUp", triggerTurn: true });
	assert.equal(b.messages[0].message.customType, "agent-peer");
	assert.equal(b.messages[0].message.display, true);
	assert.equal(b.messages[0].message.details.from.id, selfA.id);
	assert.equal(b.messages[0].message.details.id, receipt.messageId);
	assert.match(b.messages[0].message.content, /\/not-a-command/);
	b.busy(true); b.emit("agent_start");
	const sends = await Promise.all([
		a.call("agent_send", { to: selfB.id, text: "busy follow-up" }),
		b.call("agent_send", { to: selfA.id, text: "explicit reply", inReplyTo: receipt.messageId }),
	]);
	assert.ok(sends.every((result) => result.details.status === "queued"));
	assert.equal(b.messages[1].wasIdle, false);
	assert.deepEqual(b.messages[1].options, { deliverAs: "followUp", triggerTurn: true });
	assert.equal(a.messages[0].message.details.inReplyTo, receipt.messageId);
	assert.equal(a.messages[0].message.details.from.id, selfB.id);
	b.rename("Renamed peer");
	const renamed = (await b.call("agent_list")).details.self;
	assert.equal(renamed.id, selfB.id);
	assert.equal(renamed.name, "Renamed peer");
});

test("replacement fences stale inboxes, cancels requests and retires the old address", async (t) => {
	const mailbox = daemon(t);
	const rootA = mailbox(), rootB = mailbox();
	const a = nativeSession(rootA), b = nativeSession(rootB);
	t.after(() => { a.emit("session_shutdown"); b.emit("session_shutdown"); });
	a.emit("session_start"); b.emit("session_start");
	let oldB;
	await waitFor(async () => { const peers = (await a.call("agent_list")).details.peers; oldB = peers?.[0]; return !!oldB; });
	const pending = b.call("agent_list");
	const cancelled = assert.rejects(pending, /retired/);
	b.replace();
	await cancelled;
	assert.notEqual(b.identity().id, oldB.id);
	const stale = { version: 1, id: "a".repeat(32), to: oldB.id, from: oldB, text: "stale", expires: Date.now() + 10000 };
	inboxRecord(rootB, oldB, stale);
	// Even if a stale file lands in the NEW inbox, its exact address must match.
	inboxRecord(rootB, b.identity(), stale);
	await delay(300);
	assert.equal(b.messages.length, 0);
	await assert.rejects(a.call("agent_send", { to: oldB.id, text: "old address" }), /expired/);
	const delivered = await a.call("agent_send", { to: b.identity().id, text: "new address" });
	assert.equal(delivered.details.status, "queued");
	assert.equal(b.messages.length, 1);
});

test("indexed inbox delivery opens known paths without directory enumeration", async (t) => {
	const mailbox = mkdtempSync(path.join(fixtures, "indexed-inbox-"));
	writeFileSync(path.join(mailbox, "ready"), "2\n");
	const s = nativeSession(mailbox);
	s.emit("session_start");
	const self = s.identity();
	const dir = path.join(mailbox, "agents", self.id);
	const inbox = path.join(dir, "inbox");
	t.after(() => { chmodSync(inbox, 0o700); s.emit("session_shutdown"); });
	const message = { version: 1, id: "f".repeat(32), to: self.id, from: self, text: "indexed delivery", expires: Date.now() + 10000 };
	inboxRecord(mailbox, self, message);
	writeFileSync(path.join(dir, "inbox-index.json"), JSON.stringify({ version: 1, to: self.id, messages: [message.id] }));
	// Named lookup works even when enumeration does not. Extra files also make
	// a fallback scan exceed its bound, including when tests run as root.
	for (let i = 0; i < 65; i++) writeFileSync(path.join(inbox, `.unrelated-${i}`), "");
	chmodSync(inbox, 0o300);
	await waitFor(() => s.messages.length === 1);
	assert.equal(s.messages[0].message.details.id, message.id);
	assert.deepEqual(JSON.parse(readFileSync(path.join(dir, "receipts", `${message.id}.json`), "utf8")), {
		version: 1, id: message.id, to: self.id, status: "queued",
	});
	assert.equal(existsSync(path.join(inbox, `${message.id}.json`)), false);
	writeFileSync(path.join(dir, "inbox-index.json"), JSON.stringify({ version: 1, to: self.id, messages: ["../outside"] }));
	await waitFor(() => !existsSync(path.join(dir, "presence.json")));
	assert.equal(s.messages.length, 1);
});

test("inbox deduplication, safe reads, cancellation and timer cleanup", async (t) => {
	const mailbox = mkdtempSync(path.join(fixtures, "native-inbox-"));
	writeFileSync(path.join(mailbox, "ready"), "1");
	const liveTimers = new Set();
	const originalSet = globalThis.setInterval, originalClear = globalThis.clearInterval;
	t.mock.method(globalThis, "setInterval", (...args) => { const timer = originalSet(...args); liveTimers.add(timer); return timer; });
	t.mock.method(globalThis, "clearInterval", (timer) => { liveTimers.delete(timer); originalClear(timer); });
	const s = nativeSession(mailbox);
	t.after(() => s.emit("session_shutdown"));
	// A fresh legacy worker cannot serve messaging, even though lifecycle works.
	s.emit("session_start");
	await assert.rejects(s.call("agent_list"), /unavailable/);
	assert.equal(liveTimers.size, 0);
	writeFileSync(path.join(mailbox, "ready"), "2\n");
	s.emit("session_start");
	const self = s.identity();
	const message = { version: 1, id: "b".repeat(32), to: self.id, from: self, text: "once", expires: Date.now() + 10000 };
	inboxRecord(mailbox, self, message);
	await waitFor(() => s.messages.length === 1);
	inboxRecord(mailbox, self, message);
	await delay(200);
	assert.equal(s.messages.length, 1);
	const inbox = path.join(mailbox, "agents", self.id, "inbox");
	symlinkSync(path.join(mailbox, "ready"), path.join(inbox, `${"c".repeat(32)}.json`));
	writeFileSync(path.join(inbox, `${"d".repeat(32)}.json`), "x".repeat(32769));
	inboxRecord(mailbox, self, { ...message, id: "e".repeat(32), expires: Date.now() - 1 });
	await delay(200);
	assert.equal(s.messages.length, 1);
	const controller = new AbortController();
	const cancelled = assert.rejects(s.call("agent_list", {}, controller.signal), /cancelled/);
	controller.abort(); await cancelled;
	assert.equal(liveTimers.size, 1);
	s.emit("session_shutdown"); s.emit("session_shutdown");
	assert.equal(liveTimers.size, 0);
	await assert.rejects(s.call("agent_list"), /unavailable/);
});
