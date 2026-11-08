import assert from "node:assert/strict";
import test from "node:test";
import { registerVaultTools } from "./vault-tools.js";

test("vault adapter exposes domain operations, revisions and structured note data, never history commands", async () => {
	const tools = new Map(), calls = [];
	const result = { vault: "shared", path: "ideas.md", revision: "a".repeat(64) };
	registerVaultTools({ registerTool(tool) { tools.set(tool.name, tool); } }, async (...args) => {
		calls.push(args);
		return { structuredContent: { result } };
	});
	assert.equal(tools.size, 13);
	assert.ok(![...tools.keys()].some((name) => /git|commit|shell|delete/.test(name)));
	for (const name of ["save", "edit", "append", "tag"]) {
		assert.ok(tools.get(`vault_${name}`).parameters.required.includes("revision"));
	}
	const move = tools.get("vault_move");
	assert.deepEqual(move.parameters.required, ["oldPath", "newPath"]);
	assert.deepEqual(Object.keys(move.parameters.properties).sort(), ["newPath", "oldPath", "vault"]);
	assert.deepEqual(tools.get("vault_list").parameters.properties.kind.enum, ["notes", "folders", "all"]);
	const signal = new AbortController().signal;
	const params = { path: "ideas.md", revision: result.revision, oldText: "old", newText: "new" };
	const response = await tools.get("vault_edit").execute("call", params, signal);
	assert.deepEqual(calls, [["vault", { vaultOp: "edit", params }, signal]]);
	const moveParams = { oldPath: "old.md", newPath: "folder/new.md" };
	await tools.get("vault_move").execute("call", moveParams, signal);
	assert.deepEqual(calls[1], ["vault", { vaultOp: "move", params: moveParams }, signal]);
	assert.deepEqual(response.structuredContent, result);
	assert.deepEqual(response.details, result);
	assert.deepEqual(JSON.parse(response.content[0].text), result);
});

test("vault adapter rejects incompatible responses and propagates transport failures without retries", async () => {
	let call;
	registerVaultTools({ registerTool(tool) { call = tool.execute; } }, async () => ({ structuredContent: {} }));
	await assert.rejects(call("call", {}), /Invalid vault response/);
	let attempts = 0;
	registerVaultTools({ registerTool(tool) { call = tool.execute; } }, async () => {
		attempts++;
		throw new Error("Operation may have completed; do not resend automatically");
	});
	await assert.rejects(call("call", {}), /may have completed/);
	assert.equal(attempts, 1);
});
