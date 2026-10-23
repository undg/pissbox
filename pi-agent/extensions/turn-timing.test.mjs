import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { createRequire } from "node:module";
import test from "node:test";
import { pathToFileURL } from "node:url";
import { resolvePiEntry } from "../../tests/pi-installation.mjs";

const piEntry = resolvePiEntry(execFileSync("sh", ["-c", "command -v pi"], { encoding: "utf8" }).trim());
const piRequire = createRequire(piEntry);
const { FooterComponent, SessionManager, initTheme } = await import(pathToFileURL(piEntry));
initTheme("dark", false);
const { visibleWidth, stripTerminalSequences } = piRequire("@earendil-works/pi-tui");
const { createJiti } = piRequire("jiti");
const jiti = createJiti(import.meta.url, {
	fsCache: false,
	alias: { "@earendil-works/pi-tui": piRequire.resolve("@earendil-works/pi-tui") },
});
const extension = await jiti.import("./turn-timing.js", { default: true });
const sent = new Date(2026, 5, 12, 14, 30).getTime();

function session(t, { mode = "tui", width = 100, manager = SessionManager.inMemory(process.cwd()) } = {}) {
	t.mock.timers.enable({ apis: ["Date", "setInterval"], now: sent });
	const columns = Object.getOwnPropertyDescriptor(process.stdout, "columns");
	Object.defineProperty(process.stdout, "columns", { value: width, configurable: true, writable: true });
	const listeners = process.stdout.listenerCount("resize");
	const handlers = new Map();
	const statuses = new Map();
	const writes = [];
	const ctx = {
		mode, sessionManager: manager,
		ui: { setStatus(key, text) {
			writes.push(text);
			if (text === undefined) statuses.delete(key);
			else statuses.set(key, text);
		} },
	};
	extension({
		on: (event, handler) => handlers.set(event, handler),
		appendEntry: (type, data) => manager.appendCustomEntry(type, data),
	});
	const emit = (event, data = {}) => handlers.get(event)?.(data, ctx);
	t.after(() => {
		emit("session_shutdown");
		assert.equal(process.stdout.listenerCount("resize"), listeners, "release the resize listener");
		if (columns) Object.defineProperty(process.stdout, "columns", columns);
		else delete process.stdout.columns;
	});
	const footerData = {
		getExtensionStatuses: () => statuses,
		getGitBranch: () => "main",
		getAvailableProviderCount: () => 1,
	};
	const footer = new FooterComponent({ sessionManager: manager, state: {}, getContextUsage: () => undefined }, footerData);
	return {
		emit, manager, writes,
		line: () => stripTerminalSequences(footer.render(process.stdout.columns).at(-1)).replaceAll("\u00a0", " "),
		lines: () => footer.render(process.stdout.columns),
		prompt(timestamp = Date.now()) {
			emit("before_agent_start", { messageType: "user", prompt: "Fix this" });
			const message = { role: "user", content: "Fix this", timestamp };
			manager.appendMessage(message);
			emit("message_start", { message });
		},
	};
}

test("adds a right-aligned third line without changing Pi's two built-in lines", (t) => {
	const s = session(t);
	const original = s.lines();
	s.emit("session_start");
	assert.deepEqual(s.lines().slice(0, 2), original);
	assert.equal(s.lines().length, 3);
	assert.match(s.line(), /^idle — +sent — · done — · took —$/);
	assert.equal(visibleWidth(s.lines()[2]), 100, "Pi's real status sanitizer must preserve the padding");
});

test("counts working time through turns and retries, then counts idle time while keeping completed timing fixed", (t) => {
	const s = session(t);
	s.emit("session_start");
	s.prompt();
	t.mock.timers.tick(61000);
	assert.match(s.line(), /^working 1m1s +sent 14:30 · done — · took —$/);
	s.emit("agent_end");
	s.emit("before_agent_start", { messageType: "custom" });
	t.mock.timers.tick(73000);
	s.emit("agent_settled", { aborted: false });
	assert.match(s.line(), /^idle 0s +sent 14:30 · done 14:32 · took 2m14s$/);
	t.mock.timers.tick(60000);
	assert.match(s.line(), /^idle 1m0s +sent 14:30 · done 14:32 · took 2m14s$/);
	assert.equal(s.manager.getBranch().filter((e) => e.type === "custom").length, 1,
		"timer ticks must not write session metadata");
	s.prompt();
	assert.match(s.line(), /^working 0s +sent 14:33 · done — · took —$/);
});

test("uses submission time rather than the delayed message timestamp after preflight compaction", (t) => {
	const s = session(t);
	s.emit("session_start");
	s.emit("input", { text: "Fix this", source: "interactive" });
	t.mock.timers.tick(65000);
	s.prompt();
	assert.match(s.line(), /^working 1m5s +sent 14:30 · done — · took —$/);
	t.mock.timers.tick(5000);
	s.emit("agent_settled", { aborted: false });
	assert.match(s.line(), /sent 14:30 · done 14:31 · took 1m10s$/);
});

test("restores saved timing on reload and clears it when navigating to a different branch", (t) => {
	const s = session(t);
	s.emit("session_start");
	s.prompt();
	const userId = s.manager.getLeafId();
	t.mock.timers.tick(9000);
	s.emit("agent_settled", { aborted: false });
	const completed = s.line();
	s.emit("session_start", { reason: "reload" });
	assert.equal(s.line(), completed);
	t.mock.timers.tick(10000);
	assert.match(s.line(), /^idle 10s +sent 14:30 · done 14:30 · took 9s$/);
	s.emit("session_start", { reason: "resume" });
	assert.match(s.line(), /^idle 10s +sent 14:30 · done 14:30 · took 9s$/);
	s.manager.branch(userId);
	s.emit("session_tree");
	assert.match(s.line(), /sent 14:30 · done — · took —$/);
	assert.equal(s.manager.getBranch().filter((e) => e.type === "message").length, 1,
		"timing metadata must not add conversation messages");
});

test("counts idle time after interrupted runs and uses the newest queued user timestamp", (t) => {
	const s = session(t);
	s.emit("session_start");
	s.prompt();
	t.mock.timers.tick(60000);
	s.emit("message_start", { message: { role: "user", timestamp: Date.now() } });
	t.mock.timers.tick(5000);
	s.emit("agent_settled", { aborted: true });
	assert.match(s.line(), /^idle 0s +sent 14:31 · done 14:31 · took 1m5s$/);
	t.mock.timers.tick(3000);
	assert.match(s.line(), /^idle 3s +sent 14:31 · done 14:31 · took 1m5s$/);
});

test("keeps timing right-aligned on resize and truncates without overflowing narrow terminals", (t) => {
	const s = session(t);
	s.emit("session_start");
	s.prompt();
	assert.equal(visibleWidth(s.lines()[2]), 100);
	assert.match(s.line(), /^working 0s +sent 14:30 · done — · took —$/);
	process.stdout.columns = 60;
	process.stdout.emit("resize");
	assert.equal(visibleWidth(s.lines()[2]), 60);
	assert.match(s.line(), /^working 0s +sent 14:30 · done — · took —$/);
	process.stdout.columns = 20;
	process.stdout.emit("resize");
	assert.ok(visibleWidth(s.lines()[2]) <= 20);
});

test("handles historical timestamps and long runs, and ignores corrupt saved timing", (t) => {
	const s = session(t);
	s.manager.appendMessage({ role: "user", content: "Old prompt", timestamp: new Date(2026, 5, 11, 23, 50).getTime() });
	s.manager.appendCustomEntry("turn-timing", { sent: "invalid" });
	s.emit("session_start");
	assert.match(s.line(), /sent 6\/11 23:50 · done — · took —$/);
	s.prompt();
	t.mock.timers.tick(3661000);
	s.emit("agent_settled", { aborted: false });
	assert.match(s.line(), /done 15:31 · took 1h1m$/);
});

test("shutdown stops active timers and repeated startup never accumulates listeners", (t) => {
	const s = session(t);
	s.emit("session_start");
	s.emit("session_start");
	const beforeTick = s.writes.length;
	t.mock.timers.tick(1000);
	assert.equal(s.writes.length, beforeTick + 1, "startup must leave only one timer");
	s.prompt();
	s.emit("session_shutdown");
	const writes = s.writes.length;
	t.mock.timers.tick(5000);
	process.stdout.emit("resize");
	assert.equal(s.writes.length, writes);
	assert.equal(s.lines().length, 2);
});

for (const mode of ["rpc", "json", "print"]) {
	test(`does not change UI or session metadata in ${mode} mode`, (t) => {
		const s = session(t, { mode });
		s.emit("session_start");
		s.emit("input", { text: "Fix this", source: "interactive" });
		s.prompt();
		t.mock.timers.tick(2000);
		s.emit("agent_settled", { aborted: false });
		assert.deepEqual(s.writes, []);
		assert.equal(s.manager.getBranch().filter((e) => e.type === "custom").length, 0);
	});
}
