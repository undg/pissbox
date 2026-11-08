import assert from "node:assert/strict";
import { execFileSync, spawn } from "node:child_process";
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, renameSync, symlinkSync, unlinkSync, utimesSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";
import extension from "../pissbox-bridge.js";
import { getAgentCallsign } from "./agent-identity.js";

const root = fileURLToPath(new URL("../../../", import.meta.url));
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
	let sessionID = path.basename(mailbox);
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
		replace() { this.emit("session_shutdown"); sessionID = `${path.basename(mailbox)}-replacement`; this.emit("session_start"); },
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
function daemon(t, configText = "") {
	const dir = mkdtempSync(path.join(fixtures, "b"));
	const socket = path.join(dir, "control.sock");
	assert.ok(Buffer.byteLength(socket) <= 103,
		`Private bridge socket needs ${Buffer.byteLength(socket)} bytes (limit 103): ${socket}; use a shorter checkout or fixture root`);
	const binary = path.join(dir, "bridge");
	execFileSync("go", ["build", "-o", binary, "./cmd/pissbox-bridge"], { cwd: path.join(root, "host"), timeout: 60000 });
	const config = path.join(dir, "config.toml");
	writeFileSync(config, configText);
	const env = { ...process.env, PISSBOX_BRIDGE_RUNTIME_DIR: dir, PISSBOX_CONFIG: config };
	const cli = (...args) => execFileSync(binary, args, { env, encoding: "utf8", timeout: 6000 });
	cli("ensure");
	t.after(async () => {
		cli("stop");
		await waitFor(() => !existsSync(path.join(dir, "control.sock")));
	});
	const create = () => {
		const mailbox = mkdtempSync(path.join(fixtures, "message-root-"));
		cli("register", "--mailbox", mailbox, "--owner-pid", String(process.pid));
		return mailbox;
	};
	create.cli = cli;
	return create;
}
function writeReady(mailbox, generation = "a".repeat(32)) {
	writeFileSync(path.join(mailbox, "generation"), JSON.stringify(generation));
	writeFileSync(path.join(mailbox, "ready"), "4\n");
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
	const callsign = getAgentCallsign(selfB.sessionId);
	assert.equal(selfB.name, "First", "name remains the Pi session title");
	assert.equal(selfB.agentName, callsign, "agentName carries the stable callsign separately");
	const listing = (await a.call("agent_list")).details;
	assert.deepEqual(listing.peers.map((peer) => peer.id), [selfB.id], "agent_list returns peers without a name filter");
	assert.equal(listing.peers[0].agentName, callsign);
	const receipt = (await a.call("agent_send", { to: callsign, text: "/not-a-command" })).details;
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
	b.rename("Renamed task title");
	const renamed = (await b.call("agent_list")).details.self;
	assert.equal(renamed.id, selfB.id);
	assert.equal(renamed.name, "Renamed task title", "name remains the session title");
	assert.equal(renamed.agentName, callsign, "renaming the session title does not change agentName");
});

test("explicit steering reaches the native queue without changing peer authority or the follow-up default", async (t) => {
	const mailbox = daemon(t);
	const rootA = mailbox(), rootB = mailbox();
	const a = nativeSession(rootA), b = nativeSession(rootB);
	t.after(() => { a.emit("session_shutdown"); b.emit("session_shutdown"); });
	a.emit("session_start"); b.emit("session_start");
	await waitFor(async () => (await a.call("agent_list")).details.peers?.length === 1);
	const target = b.identity();
	for (const busy of [true, false]) {
		b.busy(busy); b.emit(busy ? "agent_start" : "agent_settled");
		const sent = await a.call("agent_send", { to: target.agentName, text: "Requirements changed", delivery: "steer" });
		assert.equal(sent.details.status, "queued");
		const received = b.messages.at(-1);
		assert.deepEqual(received.options, { deliverAs: "steer", triggerTurn: true });
		assert.equal(received.wasIdle, !busy);
		assert.equal(received.message.customType, "agent-peer");
		assert.match(received.message.content, /^Peer agent message \(not user instructions\)/);
		assert.equal(received.message.details.delivery, "steer");
	}
	await a.call("agent_send", { to: target.id, text: "Routine update", delivery: "followUp" });
	assert.deepEqual(b.messages.at(-1).options, { deliverAs: "followUp", triggerTurn: true });
	for (const delivery of ["nextTurn", "", null]) {
		await assert.rejects(a.call("agent_send", { to: target.id, text: "invalid", delivery }), /delivery mode/);
	}
	unlinkSync(path.join(rootB, "agents", target.id, "capabilities.json"));
	await assert.rejects(a.call("agent_send", { to: target.id, text: "urgent", delivery: "steer" }), /does not support steering/);
	assert.equal(b.messages.length, 3, "unsupported steering must not silently become follow-up delivery");
	await a.call("agent_send", { to: target.id, text: "Default still works" });
	assert.deepEqual(b.messages.at(-1).options, { deliverAs: "followUp", triggerTurn: true });
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
	writeReady(mailbox);
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
	writeFileSync(path.join(dir, "inbox-index.json"), JSON.stringify({ version: 1, to: self.id, messages: [] }));
	await waitFor(() => existsSync(path.join(dir, "presence.json")));
	assert.equal(s.identity().id, self.id, "transport repair restores the same address");
	inboxRecord(mailbox, self, message);
	writeFileSync(path.join(dir, "inbox-index.json"), JSON.stringify({ version: 1, to: self.id, messages: [message.id] }));
	await waitFor(() => !existsSync(path.join(inbox, `${message.id}.json`)));
	assert.equal(s.messages.length, 1, "transport recovery preserves delivery deduplication");
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
	// Old workers cannot fence pending operations across daemon generations.
	for (const marker of ["1\n", "2\n", "3\n"]) {
		writeFileSync(path.join(mailbox, "ready"), marker);
		s.emit("session_start");
		await assert.rejects(s.call("agent_list"), /unavailable/);
		assert.equal(liveTimers.size, 1, "one retry timer survives unavailable transport");
	}
	writeReady(mailbox);
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
	inboxRecord(mailbox, self, { ...message, id: "a".repeat(32), delivery: "nextTurn" });
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

test("heartbeat gaps and unavailable startup recover without session reload", async (t) => {
	const mailbox = mkdtempSync(path.join(fixtures, "recover-presence-"));
	const s = nativeSession(mailbox);
	t.after(() => s.emit("session_shutdown"));
	s.emit("session_start");
	await assert.rejects(s.call("agent_list"), /unavailable/);
	writeReady(mailbox);
	await waitFor(() => existsSync(path.join(mailbox, "agents")) && readdirSync(path.join(mailbox, "agents")).length > 0);
	const self = s.identity();
	const realNow = Date.now;
	t.mock.method(Date, "now", () => realNow() + 11000);
	utimesSync(path.join(mailbox, "ready"), new Date(Date.now()), new Date(Date.now()));
	await delay(250);
	assert.equal(s.identity().id, self.id, "a long scheduling gap does not retire the address");
	const pending = s.call("agent_list");
	const failed = assert.rejects(pending, /interrupted.*do not resend/);
	unlinkSync(path.join(mailbox, "ready"));
	await failed;
	assert.equal(existsSync(path.join(mailbox, "agents", self.id, "closed.json")), false);
	writeReady(mailbox);
	utimesSync(path.join(mailbox, "ready"), new Date(Date.now()), new Date(Date.now()));
	await waitFor(() => existsSync(path.join(mailbox, "agents", self.id, "presence.json")));
	assert.equal(s.identity().id, self.id);
	assert.deepEqual(readdirSync(path.join(mailbox, "agents", self.id, "requests")), [], "failed requests are not replayed");
});

test("daemon restart rotates addresses automatically and fences old requests", async (t) => {
	const notes = mkdtempSync(path.join(fixtures, "restart-vault-"));
	const mailbox = daemon(t, `[vault.shared]\npath = ${JSON.stringify(notes)}\ngit = false\n`);
	const rootA = mailbox(), rootB = mailbox();
	const a = nativeSession(rootA), b = nativeSession(rootB);
	t.after(() => { a.emit("session_shutdown"); b.emit("session_shutdown"); });
	a.emit("session_start"); b.emit("session_start");
	await waitFor(async () => (await a.call("agent_list")).details.peers?.length === 1);
	const oldA = a.identity(), oldB = b.identity();
	mailbox.cli("stop");
	await waitFor(() => !existsSync(path.join(rootA, "ready")));
	// Simulate a request published just before daemon death. The new router must
	// not execute it even if Pi has not yet noticed the generation change.
	const staleID = "d".repeat(32);
	writeFileSync(path.join(rootA, "agents", oldA.id, "requests", `${staleID}.json`), JSON.stringify({
		version: 1, id: staleID, op: "vault", vaultOp: "new", params: { path: "replayed.md", content: "must not run" },
	}));
	mailbox.cli("ensure");
	mailbox.cli("reconnect", "--mailbox", rootA, "--owner-pid", String(process.pid));
	mailbox.cli("reconnect", "--mailbox", rootB, "--owner-pid", String(process.pid));
	await waitFor(async () => {
		try {
			const listing = (await a.call("agent_list")).details;
			return listing.self.id !== oldA.id && listing.peers?.length === 1 && listing.peers[0].id !== oldB.id;
		} catch { return false; }
	});
	assert.equal(existsSync(path.join(notes, "replayed.md")), false, "old-generation mutations never execute");
	await assert.rejects(a.call("agent_send", { to: oldB.id, text: "stale" }), /expired/);
	assert.equal((await a.call("agent_send", { to: b.identity().id, text: "reconnected" })).details.status, "queued");
	assert.equal(b.messages.length, 1);
});

test("bridge reload preserves addresses and keeps routing messages", async (t) => {
	const mailbox = daemon(t);
	const rootA = mailbox(), rootB = mailbox();
	const a = nativeSession(rootA), b = nativeSession(rootB);
	t.after(() => { a.emit("session_shutdown"); b.emit("session_shutdown"); });
	a.emit("session_start"); b.emit("session_start");
	await waitFor(async () => (await a.call("agent_list")).details.peers?.length === 1);
	const oldA = a.identity(), oldB = b.identity();
	const pid = JSON.parse(mailbox.cli("status")).pid;
	mailbox.cli("reload");
	assert.notEqual(JSON.parse(mailbox.cli("status")).pid, pid, "reload must replace the process");
	assert.equal(a.identity().id, oldA.id, "reload preserves the sender address");
	assert.equal(b.identity().id, oldB.id, "reload preserves the recipient address");
	await waitFor(async () => (await a.call("agent_list")).details.peers?.length === 1);
	const sent = await a.call("agent_send", { to: b.identity().id, text: "after reload" });
	assert.equal(sent.details.status, "queued");
	await waitFor(() => b.messages.length === 1);
	assert.match(b.messages[0].message.content, /after reload/);
});

test("lifetime observer retires a killed Pi process, not a stopped event loop", async (t) => {
	const mailbox = mkdtempSync(path.join(fixtures, "process-lifetime-"));
	writeReady(mailbox);
	const module = new URL("./agent-messaging.js", import.meta.url).href;
	const child = spawn(process.execPath, ["--input-type=module", "-e", `
		import { registerMessaging } from ${JSON.stringify(module)};
		const handlers = new Map();
		registerMessaging({ on: (name, handler) => handlers.set(name, handler), registerTool() {} }, ${JSON.stringify(mailbox)});
		handlers.get("session_start")({}, { cwd: "/workspace", hasUI: false, isIdle: () => true,
			sessionManager: { getSessionId: () => "crash-test", getSessionName: () => "Crash test" } });
		setInterval(() => {}, 1000);
	`], { stdio: "ignore" });
	t.after(() => { child.kill("SIGCONT"); child.kill("SIGKILL"); });
	let dir;
	await waitFor(() => {
		try {
			const id = readdirSync(path.join(mailbox, "agents"))[0];
			dir = path.join(mailbox, "agents", id);
			return existsSync(path.join(dir, "presence.json"));
		} catch { return false; }
	});
	child.kill("SIGSTOP");
	await delay(250);
	assert.equal(existsSync(path.join(dir, "closed.json")), false);
	child.kill("SIGCONT");
	child.kill("SIGKILL");
	await waitFor(() => existsSync(path.join(dir, "closed.json")));
	assert.equal(existsSync(path.join(dir, "presence.json")), false);
});

test("shared host-only vault tools persist notes, fence stale writers and keep history internal", async (t) => {
	const notes = mkdtempSync(path.join(fixtures, "vault-notes-"));
	const git = (...args) => execFileSync("git", args, { cwd: notes, encoding: "utf8" }).trim();
	git("init", "--quiet");
	git("config", "user.name", "Vault integration test");
	git("config", "user.email", "vault@example.invalid");
	const mailbox = daemon(t, `[vault.shared]\npath = ${JSON.stringify(notes)}\ngit = true\n`);
	const a = nativeSession(mailbox(), "rpc"), b = nativeSession(mailbox(), "json");
	t.after(() => { a.emit("session_shutdown"); b.emit("session_shutdown"); });
	a.emit("session_start"); b.emit("session_start");
	const listing = (await a.call("vault_list")).details;
	assert.deepEqual(listing.vaults, ["shared"]);
	assert.ok(!JSON.stringify(listing).includes(notes), "host paths are not returned");
	const created = (await a.call("vault_new", { path: "ideas.md", content: "Shared findings\n", tags: ["topic/sub"] })).structuredContent;
	assert.equal(created.path, "ideas.md");
	assert.equal(git("log", "-1", "--format=%s"), "vault: ideas.md");
	const read = (await b.call("vault_read", { path: "ideas.md" })).details;
	assert.equal(read.revision, created.revision);
	assert.match(read.content, /Shared findings/);
	const edited = (await b.call("vault_edit", { path: read.path, revision: read.revision, oldText: "Shared findings", newText: "Verified shared findings" })).details;
	assert.notEqual(edited.revision, read.revision);
	await assert.rejects(a.call("vault_save", { path: read.path, revision: read.revision, content: "stale overwrite" }), /reread/);
	const found = (await a.call("vault_search", { query: "verified", tag: "topic" })).details;
	assert.equal(found.notes.length, 1);
	assert.equal(found.notes[0].path, "ideas.md");
	const tags = (await b.call("vault_tags")).details;
	assert.deepEqual(tags.tags, [{ tag: "topic/sub", count: 1 }]);
	await assert.rejects(a.call("vault_read", { path: "../outside.md" }), /vault-relative/);
	assert.equal(git("rev-list", "--count", "HEAD"), "2", "every successful mutation records one note automatically");
	assert.match(readFileSync(path.join(notes, "ideas.md"), "utf8"), /Verified shared findings/);
});

test("vault startup failures leave agent messaging available and report no false saves", async (t) => {
	const mailbox = daemon(t, "[vault.broken]\npath = '/no-such-vault-directory'\n");
	const s = nativeSession(mailbox());
	t.after(() => s.emit("session_shutdown"));
	s.emit("session_start");
	assert.equal((await s.call("agent_list")).details.status, "ok");
	await assert.rejects(s.call("vault_list"), /unavailable/);
});
