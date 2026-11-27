import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { createRequire } from "node:module";
import test from "node:test";
import { pathToFileURL } from "node:url";
import { resolvePiEntry } from "../../tests/pi-installation.mjs";

const piEntry = resolvePiEntry(execFileSync("sh", ["-c", "command -v pi"], { encoding: "utf8" }).trim());
const piRequire = createRequire(piEntry);
const { SessionManager, initTheme, Theme } = await import(pathToFileURL(piEntry));
const { rgbColor, visibleWidth, stripTerminalSequences } = piRequire("@earendil-works/pi-tui");
const { createJiti } = piRequire("jiti");
const jiti = createJiti(import.meta.url, {
	fsCache: false,
	alias: { "@earendil-works/pi-tui": piRequire.resolve("@earendil-works/pi-tui") },
});
const extension = await jiti.import("./context-footer.js", { default: true });
const timingExtension = await jiti.import("./turn-timing.js", { default: true });
const { getAgentCallsign } = await import("./lib/agent-identity.js");
initTheme("dark", false);
const theme = new Theme({ accent: "#00aaff", dim: "#888888", success: "#00ff00", error: "#ff0000", muted: "#888888", text: "#ffffff", thinkingXhigh: "#ff00ff" },
	{ selectedBg: "#000000" }, "truecolor");

function session(t, { tokens = 147400, window = 1100000, mode = "tui", timing = false } = {}) {
	const manager = SessionManager.inMemory("/workspace/demo");
	const handlers = new Map();
	const statuses = new Map();
	const branchListeners = new Set();
	let footer;
	let renders = 0;
	let scans = 0;
	const ctx = {
		mode, sessionManager: manager,
		model: { id: "test-model", provider: "test-provider", reasoning: true, contextWindow: window },
		getContextUsage: () => { scans++; return { tokens, contextWindow: window }; },
		modelRegistry: { isUsingOAuth: () => true, getProvider: () => ({ auth: { oauth: { isSubscription: true } } }) },
		ui: {
			setFooter(factory) {
				footer?.dispose();
				footer = factory({ requestRender: () => renders++ }, theme, {
					getGitBranch: () => "main",
					getExtensionStatuses: () => statuses,
					getAvailableProviderCount: () => 2,
					onBranchChange(listener) { branchListeners.add(listener); return () => branchListeners.delete(listener); },
				});
			},
			setStatus(key, value) { if (value === undefined) statuses.delete(key); else statuses.set(key, value); },
		},
	};
	const settings = { compaction: { enabled: true } };
	const pi = {
		on(event, handler) { const list = handlers.get(event) ?? []; list.push(handler); handlers.set(event, list); },
		getSettings: () => settings,
		getThinkingLevel: () => "medium",
		appendEntry: (type, data) => manager.appendCustomEntry(type, data),
	};
	extension(pi);
	if (timing) timingExtension(pi);
	const emit = (event, data = {}) => { for (const handler of handlers.get(event) ?? []) handler(data, ctx); };
	t.after(() => { emit("session_shutdown"); footer?.dispose(); assert.equal(branchListeners.size, 0); });
	emit("session_start");
	return {
		ctx, manager, settings, statuses, emit, branchListeners,
		get footer() { return footer; },
		get renders() { return renders; },
		get scans() { return scans; },
		lines: (width = 180) => footer.render(width),
		text: (width = 180) => footer.render(width).map(stripTerminalSequences),
		setUsage(count, size = window) { tokens = count; window = size; manager.appendCustomEntry("usage-changed", {}); },
	};
}

function usage(input, output, cacheRead, cacheWrite, cost) {
	return { input, output, cacheRead, cacheWrite, totalTokens: input + output + cacheRead + cacheWrite,
		cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: cost } };
}

function assistant(manager, stats) {
	return manager.appendMessage({ role: "assistant", content: [], usage: stats,
		provider: "test-provider", model: "test-model", api: "test-api", stopReason: "stop", timestamp: Date.now() });
}

test("shows context tokens, window, subscription and right-aligned model while retaining cwd and session title", (t) => {
	const s = session(t);
	s.manager.appendSessionInfo("Fix performance");
	const [cwd, stats] = s.text();
	assert.equal(cwd, "/workspace/demo (main) • Fix performance");
	assert.ok(s.lines()[0].includes(theme.fg("accent", "Fix performance")));
	assert.ok(s.lines()[0].startsWith(theme.fg("dim", "/workspace/demo (main)")));
	assert.match(stats, /^\$0\.000 \(sub\) 147k\/1\.1M \(auto\) +\(test-provider\) test-model • medium$/);
	assert.equal(visibleWidth(s.lines()[1]), 180);
	assert.ok(!stats.includes("%/"));
});

test("keeps renamed titles accented, truncates wide titles and removes the accent when the title is cleared", (t) => {
	const s = session(t);
	s.manager.appendSessionInfo("Original title");
	s.lines();
	s.manager.appendSessionInfo("New title 界界界");
	assert.ok(s.lines()[0].includes(theme.fg("accent", "New title 界界界")));
	for (const width of [0, 1, 25, 35, 50]) {
		assert.ok(visibleWidth(s.lines(width)[0]) <= width, `title overflows width ${width}`);
	}
	s.manager.appendSessionInfo("");
	assert.equal(s.lines()[0], theme.fg("dim", "/workspace/demo (main)"));
});

test("uses green below 80k, orange from 80k and red from 150k, independent of percentage or rounding", (t) => {
	const orange = rgbColor(255, 165, 0);
	const s = session(t);
	for (const [tokens, window, display, color] of [
		[0, 1100000, "0/1.1M", "success"],
		[79999, 90000, "80k/90k", "success"],
		[80000, 1100000, "80k/1.1M", orange],
		[80001, 1100000, "80k/1.1M", orange],
		[149999, 160000, "150k/160k", orange],
		[150000, 1100000, "150k/1.1M", "error"],
		[150001, 1100000, "150k/1.1M", "error"],
		[140000, 1100000, "140k/1.1M", orange],
		[70000, 1100000, "70k/1.1M", "success"],
	]) {
		s.setUsage(tokens, window);
		assert.ok(s.lines()[1].includes(theme.style(`${display} (auto)`, { fg: color })),
			`wrong context colour for ${tokens} tokens in a ${window}-token window`);
	}
});

test("handles unknown usage after compaction, zero tokens, missing model and live auto-compaction settings", (t) => {
	const s = session(t, { tokens: null });
	assert.match(s.text()[1], /\?\/1\.1M \(auto\)/);
	assert.ok(s.lines()[1].includes(theme.fg("dim", "?/1.1M (auto)")));
	s.setUsage(0);
	s.settings.compaction.enabled = false;
	assert.match(s.text()[1], /0\/1\.1M +\(test-provider\)/);
	assert.ok(!s.text()[1].includes("(auto)"));
	s.ctx.model = undefined;
	s.ctx.getContextUsage = () => undefined;
	assert.match(s.text()[1], /^\?\/0 +no-model$/);
});

test("totals include abandoned branches, compaction, summaries, nested tools and standalone usage", (t) => {
	const s = session(t);
	const first = assistant(s.manager, usage(1000, 1000, 1000, 1000, 1));
	assistant(s.manager, usage(1000, 1000, 1000, 1000, 1));
	s.manager.branch(first);
	s.manager.appendMessage({ role: "toolResult", toolCallId: "id", toolName: "test", content: [],
		isError: false, timestamp: Date.now(), usage: usage(1000, 1000, 1000, 1000, 1) });
	s.manager.appendCompaction("Summary", first, 100000, undefined, false, usage(1000, 1000, 1000, 1000, 1));
	s.manager.branchWithSummary(first, "Branch summary", undefined, false, usage(1000, 1000, 1000, 1000, 1));
	s.manager.appendUsage("test", "test-provider", "test-model", usage(1000, 1000, 1000, 1000, 1));
	assert.match(s.text()[1], /^↑6\.0k ↓6\.0k R6\.0k W6\.0k CH33\.3% \$6\.000 \(sub\) 147k\/1\.1M/);
});

test("uses the latest assistant's cache hit rate rather than aggregate session usage", (t) => {
	const s = session(t);
	assistant(s.manager, usage(1000, 50, 9000, 0, 1));
	assert.match(s.text()[1], /CH90\.0%/);
	assistant(s.manager, usage(0, 50, 0, 0, 1));
	assert.ok(!s.text()[1].includes("CH"), "zero-token prompt has no cache-hit rate");
	s.ctx.modelRegistry.isUsingOAuth = () => false;
	assert.ok(!s.text()[1].includes("(sub)"));
	s.ctx.model.provider = "kimi-coding";
	assert.ok(s.text()[1].includes("(sub)"), "Kimi subscription does not require OAuth");
});

test("composes with the real turn-timing extension, retaining callsign, padding and live timing", (t) => {
	t.mock.timers.enable({ apis: ["Date", "setInterval"], now: new Date(2026, 5, 12, 14, 30).getTime() });
	const columns = Object.getOwnPropertyDescriptor(process.stdout, "columns");
	Object.defineProperty(process.stdout, "columns", { value: 100, configurable: true });
	t.after(() => { if (columns) Object.defineProperty(process.stdout, "columns", columns); else delete process.stdout.columns; });
	const listeners = process.stdout.listenerCount("resize");
	const s = session(t, { timing: true });
	const original = s.text(100).slice(0, 2);
	s.emit("before_agent_start", { messageType: "user" });
	t.mock.timers.tick(61000);
	const lines = s.text(100);
	assert.equal(lines.length, 3);
	assert.deepEqual(lines.slice(0, 2), original);
	assert.match(lines[2].replaceAll("\u00a0", " "), /working 1m1s +sent 14:30 · done — · took —$/);
	assert.ok(lines[2].startsWith(`${getAgentCallsign(s.manager.getSessionId())} · `));
	assert.equal(visibleWidth(lines[2]), 100);
	s.emit("session_shutdown");
	assert.equal(process.stdout.listenerCount("resize"), listeners);
});

test("sanitizes and sorts other extension statuses without collapsing timing's non-breaking padding", (t) => {
	const s = session(t);
	s.statuses.set("z", "last\nstatus\tline");
	s.statuses.set("a", "first\u00a0\u00a0status");
	assert.equal(s.text()[2], "first\u00a0\u00a0status last status line");
	for (const width of [0, 1, 10, 30, 60, 100]) {
		for (const line of s.lines(width)) assert.ok(visibleWidth(line) <= width, `footer overflow at width ${width}`);
	}
});

test("caches unchanged sessions, refreshes on new entries, model changes and invalidation, and releases branch listeners on reload", (t) => {
	const s = session(t);
	s.lines(); s.lines(); s.lines(100);
	assert.equal(s.scans, 1);
	s.setUsage(160000); s.lines();
	assert.equal(s.scans, 2);
	s.ctx.model = { ...s.ctx.model, id: "other-model", reasoning: false }; s.lines();
	assert.equal(s.scans, 3);
	assert.match(s.text()[1], /other-model$/);
	s.footer.invalidate(); s.lines();
	assert.equal(s.scans, 4);
	for (const listener of s.branchListeners) listener();
	assert.equal(s.renders, 1);
	s.emit("session_start", { reason: "reload" });
	assert.equal(s.branchListeners.size, 1);
});

test("formats small and million-token contexts compactly without changing threshold colour", (t) => {
	const s = session(t, { tokens: 999 });
	assert.match(s.text()[1], /999\/1\.1M/);
	s.setUsage(1234);
	assert.match(s.text()[1], /1\.2k\/1\.1M/);
	s.setUsage(1500000);
	assert.ok(s.lines()[1].includes(theme.fg("error", "1.5M/1.1M (auto)")));
	s.setUsage(12000000, 20000000);
	assert.ok(s.lines()[1].includes(theme.fg("error", "12M/20M (auto)")));
});

test("abbreviates only cwd paths inside home and handles missing home variables", (t) => {
	const home = process.env.HOME;
	const userProfile = process.env.USERPROFILE;
	t.after(() => {
		if (home === undefined) delete process.env.HOME; else process.env.HOME = home;
		if (userProfile === undefined) delete process.env.USERPROFILE; else process.env.USERPROFILE = userProfile;
	});
	process.env.HOME = "/workspace";
	const s = session(t);
	assert.equal(s.text()[0], "~/demo (main)");
	s.ctx.sessionManager = SessionManager.inMemory("/workspace");
	assert.equal(s.text()[0], "~ (main)");
	s.ctx.sessionManager = SessionManager.inMemory("/workspace-other/demo");
	assert.equal(s.text()[0], "/workspace-other/demo (main)");
	delete process.env.HOME;
	process.env.USERPROFILE = "/workspace-other";
	assert.equal(s.text()[0], "~/demo (main)");
	delete process.env.USERPROFILE;
	assert.equal(s.text()[0], "/workspace-other/demo (main)");
});

test("does not replace the footer outside interactive mode", (t) => {
	for (const mode of ["rpc", "json", "print"]) {
		const s = session(t, { mode });
		assert.equal(s.footer, undefined);
		assert.equal(s.branchListeners.size, 0);
	}
});
