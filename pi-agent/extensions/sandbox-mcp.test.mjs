import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { createRequire } from "node:module";
import { createServer } from "node:http";
import { mkdir, mkdtemp, rm } from "node:fs/promises";
import { resolve } from "node:path";
import { pathToFileURL } from "node:url";
import test from "node:test";
import { resolvePiEntry } from "../../tests/pi-installation.mjs";

const piEntry = resolvePiEntry(execFileSync("sh", ["-c", "command -v pi"], { encoding: "utf8" }).trim());
const require = createRequire(piEntry);
const { createJiti } = require("jiti");
const jiti = createJiti(import.meta.url, { fsCache: false });
const extension = await jiti.import("./sandbox-mcp.js", { default: true });

function gatewayEnvironment(t, env = {}) {
	for (const [key, value] of Object.entries({
		MCP_GATEWAY_URL: "http://gateway.invalid/mcp",
		MCP_SENTINEL_TOKEN_NAME: "fixture-sentinel",
		...env,
	})) {
		const previous = process.env[key];
		t.after(() => {
			if (previous === undefined) delete process.env[key];
			else process.env[key] = previous;
		});
		if (value === undefined) delete process.env[key];
		else process.env[key] = value;
	}
}

function session({ confirm = async () => true, hasUI = true, registerError, nativeMcp = true } = {}) {
	const handlers = new Map();
	const commands = new Map();
	const servers = new Map();
	const registrations = [];
	const removals = [];
	const notices = [];
	const statuses = [];
	const ctx = { hasUI, ui: {
		confirm,
		notify: (text, level) => notices.push({ text, level }),
		setStatus: (key, text) => statuses.push({ key, text }),
	} };
	extension({
		on: (event, handler) => handlers.set(event, handler),
		registerCommand: (name, command) => commands.set(name, command),
		registerMcpServer: nativeMcp ? (name, config) => {
			if (registerError) throw registerError;
			servers.set(name, config);
			registrations.push({ name, config });
		} : undefined,
		unregisterMcpServer(name) { removals.push(name); servers.delete(name); },
	});
	return { servers, registrations, removals, notices, statuses, commands,
		emit: (event, data = {}) => handlers.get(event)?.(data, ctx),
		command: (args = "") => commands.get("sandbox-mcp").handler(args, ctx),
	};
}

test("gateway stays OFF at load, startup, and status without touching servers", async (t) => {
	gatewayEnvironment(t);
	const s = session();
	s.emit("session_start");
	await s.command();
	await s.command("status");
	assert.deepEqual(s.registrations, []);
	assert.deepEqual(s.removals, []);
	assert.ok(s.notices.every(({ text }) => text === "Sandbox MCP OFF."));
	assert.equal(s.commands.size, 1);
});

test("explicit confirmed ON registers the injected gateway once and OFF removes it", async (t) => {
	gatewayEnvironment(t);
	let confirmations = 0;
	const s = session({ confirm: async (_title, message) => {
		assert.match(message, /authenticated data and act as you/);
		confirmations++;
		return true;
	} });
	await s.command(" ON ");
	await s.command("on");
	await s.command("status");
	assert.equal(s.notices.at(-1).text, "Sandbox MCP ON (registered). Use /mcp for connection status.");
	assert.equal(confirmations, 1);
	assert.deepEqual(s.registrations, [{ name: "pissbox-sbx", config: {
		url: "http://gateway.invalid/mcp",
		headers: { Authorization: "Bearer fixture-sentinel" },
		exposure: "deferred",
		description: "Host-managed tools explicitly loaded into this sandbox's SBX MCP gateway, including Chrome DevTools when granted.",
	} }]);
	assert.equal(s.emit("tool_call", { toolName: "mcp__pissbox_sbx__navigate" }), undefined);
	await s.command("off");
	await s.command("off");
	assert.equal(s.servers.size, 0);
	assert.deepEqual(s.removals, ["pissbox-sbx"]);
	assert.match(s.notices.at(-1).text, /attachments are unchanged/);
	assert.equal(s.statuses.at(-1).text, undefined);
	assert.equal(s.emit("tool_call", { toolName: "mcp__pissbox_sbx__navigate" }).block, true);
	assert.equal(s.emit("tool_call", { toolName: "mcp__other__navigate" }), undefined);
	assert.equal(s.emit("tool_call", { toolName: "bash" }), undefined);
	assert.ok(!JSON.stringify(s.notices).includes("fixture-sentinel"));
});

test("cancelled, headless, invalid, and missing-environment requests never connect", async (t) => {
	gatewayEnvironment(t);
	for (const options of [{ confirm: async () => false }, { hasUI: false }]) {
		const s = session(options);
		await s.command("on");
		assert.equal(s.servers.size, 0);
	}
	const s = session();
	await s.command("toggle");
	assert.match(s.notices.at(-1).text, /Usage:/);
	for (const key of ["MCP_GATEWAY_URL", "MCP_SENTINEL_TOKEN_NAME"]) {
		const value = process.env[key];
		delete process.env[key];
		await s.command("on");
		process.env[key] = value;
		assert.match(s.notices.at(-1).text, /unavailable/);
	}
	assert.deepEqual(s.registrations, []);
	const unsupported = session({ nativeMcp: false });
	unsupported.emit("session_start");
	await unsupported.command("on");
	assert.match(unsupported.notices.at(-1).text, /native MCP support/);
	assert.deepEqual(unsupported.registrations, []);
});

test("shutdown and every replacement reason reset ON instead of restoring consent", async (t) => {
	gatewayEnvironment(t);
	for (const reason of ["startup", "reload", "new", "resume", "fork"]) {
		const s = session();
		await s.command("on");
		s.emit("session_start", { reason });
		await s.command("status");
		assert.equal(s.servers.size, 0);
		assert.equal(s.notices.at(-1).text, "Sandbox MCP OFF.");
	}
	const s = session();
	await s.command("on");
	s.emit("session_shutdown");
	s.emit("session_shutdown");
	assert.deepEqual(s.removals, ["pissbox-sbx"]);
});

test("OFF or session shutdown during confirmation cancels the pending grant", async (t) => {
	gatewayEnvironment(t);
	for (const action of ["off", "session_shutdown"]) {
		const approval = Promise.withResolvers();
		const s = session({ confirm: () => approval.promise });
		const pending = s.command("on");
		if (action === "off") await s.command("off");
		else s.emit(action);
		approval.resolve(true);
		await pending;
		assert.deepEqual(s.registrations, []);
	}
});

test("overlapping ON confirmations register once instead of replacing a live connection", async (t) => {
	gatewayEnvironment(t);
	const approvals = [Promise.withResolvers(), Promise.withResolvers()];
	let next = 0;
	const s = session({ confirm: () => approvals[next++].promise });
	const first = s.command("on");
	const second = s.command("on");
	approvals[0].resolve(true);
	await first;
	approvals[1].resolve(true);
	await second;
	assert.equal(s.registrations.length, 1);
});

test("registration failure leaves OFF and completions only offer supported actions", async (t) => {
	gatewayEnvironment(t);
	const s = session({ registerError: new Error("registration failed") });
	await assert.rejects(s.command("on"), /registration failed/);
	await s.command("status");
	assert.equal(s.notices.at(-1).text, "Sandbox MCP OFF.");
	const complete = s.commands.get("sandbox-mcp").getArgumentCompletions;
	assert.deepEqual(complete("o"), [{ value: "on", label: "on" }, { value: "off", label: "off" }]);
	assert.deepEqual(complete("status"), [{ value: "status", label: "status" }]);
	assert.equal(complete("bad"), null);
});

test("real Pi discovers, invokes, and withdraws HTTP MCP tools through the ad-hoc switch", { timeout: 15000 }, async (t) => {
	const { createAgentSession, createMcpExtension, createToolSearchExtension,
		DefaultResourceLoader, ModelRuntime, SessionManager, SettingsManager } = await import(pathToFileURL(piEntry));
	const fixtures = resolve(".cache/test-fixtures");
	await mkdir(fixtures, { recursive: true });
	const directory = await mkdtemp(fixtures + "/sandbox-mcp-");
	t.after(() => rm(directory, { recursive: true, force: true }));
	const requests = [];
	const server = createServer(async (req, res) => {
		if (req.method !== "POST") { res.writeHead(405).end(); return; }
		const chunks = [];
		for await (const chunk of req) chunks.push(chunk);
		const message = JSON.parse(Buffer.concat(chunks).toString());
		requests.push({ method: message.method, authorization: req.headers.authorization });
		if (message.id === undefined) { res.writeHead(202).end(); return; }
		const results = {
			initialize: { protocolVersion: "2025-03-26", capabilities: { tools: {} }, serverInfo: { name: "fixture", version: "1" } },
			"tools/list": { tools: [{ name: "probe", description: "Inspect fixture browser performance", inputSchema: { type: "object", properties: {} } }] },
			"tools/call": { content: [{ type: "text", text: "fixture performance result" }] },
		};
		res.writeHead(200, { "Content-Type": "application/json" });
		res.end(JSON.stringify({ jsonrpc: "2.0", id: message.id, result: results[message.method] ?? {} }));
	});
	await new Promise((done) => server.listen(0, "127.0.0.1", done));
	t.after(async () => {
		server.closeAllConnections();
		await new Promise((done) => server.close(done));
	});
	gatewayEnvironment(t, { MCP_GATEWAY_URL: `http://127.0.0.1:${server.address().port}/mcp` });
	const settingsManager = SettingsManager.inMemory({ defaultTools: ["+tool_search"] });
	const resourceLoader = new DefaultResourceLoader({
		cwd: directory, agentDir: directory, settingsManager,
		noSkills: true, noPromptTemplates: true, noThemes: true, noContextFiles: true,
		extensionFactories: [extension, createToolSearchExtension(), createMcpExtension({ logPath: directory + "/mcp.log" })],
	});
	await resourceLoader.reload();
	const modelRuntime = await ModelRuntime.create({ authPath: directory + "/auth.json", modelsPath: directory + "/models.json" });
	const manager = SessionManager.inMemory(directory);
	// Nested tool calls require an issuing assistant message, not a model request.
	manager.appendMessage({ role: "assistant", content: [{ type: "toolCall", id: "fixture-parent", name: "fixture", arguments: {} }],
		api: "openai-completions", provider: "fixture", model: "fixture", stopReason: "toolUse", timestamp: Date.now(),
		usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, totalTokens: 0, cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 } },
	});
	const { session: piSession } = await createAgentSession({
		cwd: directory, agentDir: directory, resourceLoader, modelRuntime, settingsManager,
		sessionManager: manager,
	});
	const errors = [];
	try {
		await piSession.bindExtensions({ mode: "tui", uiContext: {
			confirm: async () => true, notify() {}, setStatus() {},
		}, onError: (error) => errors.push(error) });
		assert.ok(piSession.getAllTools().some(({ name }) => name === "tool_search"), JSON.stringify(piSession.getAllTools().map(({ name }) => name)));
		piSession.setActiveToolsByName([...piSession.getActiveToolNames(), "tool_search"]);
		await piSession.prompt("/sandbox-mcp status");
		assert.deepEqual(requests, [], "OFF must not initialize a transport");
		for (let cycle = 0; cycle < 2; cycle++) {
			await piSession.prompt("/sandbox-mcp on");
			const context = piSession.extensionRunner.createToolContext("fixture-parent", undefined);
			// Pi's native MCP hook waits for discovery when a search reaches this server.
			await piSession.extensionRunner.emitToolCall({ type: "tool_call", toolName: "tool_search",
				toolCallId: "fixture-parent", input: { query: "fixture browser performance" } });
			assert.ok(piSession.getCallableToolNames().includes("mcp__pissbox_sbx__probe"));
			const result = await context.executeTool("mcp__pissbox_sbx__probe", {});
			assert.equal(result.isError, false, JSON.stringify(result));
			assert.match(JSON.stringify(result.result.content), /fixture performance result/);
			assert.ok(requests.every(({ authorization }) => authorization === "Bearer fixture-sentinel"));
			await piSession.prompt("/sandbox-mcp off");
			const calls = requests.filter(({ method }) => method === "tools/call").length;
			const blocked = await context.executeTool("mcp__pissbox_sbx__probe", {});
			assert.equal(blocked.isError, true);
			assert.equal(requests.filter(({ method }) => method === "tools/call").length, calls);
		}
		assert.deepEqual(errors, []);
	} finally {
		await piSession.extensionRunner.emit({ type: "session_shutdown", reason: "quit" });
		piSession.dispose();
	}
});
