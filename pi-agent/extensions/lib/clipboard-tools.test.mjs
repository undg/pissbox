import assert from "node:assert/strict";
import test from "node:test";
import { registerClipboardTool } from "./clipboard-tools.js";

test("clipboard_copy exposes write-only text input and uses the session transport exactly once", async () => {
	let tool;
	const calls = [];
	registerClipboardTool({ registerTool(value) { tool = value; } }, async (...args) => {
		calls.push(args);
		return { structuredContent: { result: { backend: "pbcopy" } } };
	});
	assert.equal(tool.name, "clipboard_copy");
	assert.deepEqual(Object.keys(tool.parameters.properties), ["text"]);
	assert.deepEqual(tool.parameters.required, ["text"]);
	assert.equal(tool.parameters.additionalProperties, false);
	assert.equal(tool.annotations.readOnlyHint, false);
	assert.equal(tool.annotations.destructiveHint, true);
	assert.equal(tool.executionMode, "sequential");
	assert.match(tool.promptGuidelines[0], /only when the user asks/);
	assert.ok(tool.promptGuidelines.some((guideline) => guideline.includes("clipboard_copy succeeds") && guideline.includes("verbatim in a fenced code block")));
	assert.ok(tool.promptGuidelines.some((guideline) => guideline.includes("clipboard_copy") && guideline.includes("Grug-style") && guideline.includes("meat-proxy") && guideline.includes("Keep the clipboard text unchanged")));
	const signal = new AbortController().signal;
	for (const text of ["exact text 世界\n\t", "", "x".repeat(16384)]) {
		const result = await tool.execute("call", { text }, signal);
		assert.deepEqual(calls.at(-1), ["clipboard_copy", { clipboardText: text }, signal]);
		assert.deepEqual(result.structuredContent, { backend: "pbcopy" });
		assert.deepEqual(result.details, result.structuredContent);
		assert.deepEqual(JSON.parse(result.content[0].text), result.structuredContent);
	}
	assert.equal(calls.length, 3);
	for (const text of [undefined, null, 123, "x".repeat(16385), "世".repeat(6000), "\ud800"]) {
		await assert.rejects(tool.execute("call", { text }), /valid UTF-8/);
	}
	assert.equal(calls.length, 3, "invalid input must not reach host");
});

test("clipboard_copy validates host results and never retries uncertain writes", async () => {
	for (const result of [undefined, null, {}, { backend: "shell" }]) {
		let tool;
		registerClipboardTool({ registerTool(value) { tool = value; } }, async () => ({ structuredContent: { result } }));
		await assert.rejects(tool.execute("call", { text: "text" }), /Invalid clipboard response/);
	}
	for (const backend of ["pbcopy", "wl-copy", "xclip", "xsel"]) {
		let tool;
		registerClipboardTool({ registerTool(value) { tool = value; } }, async () => ({ structuredContent: { result: { backend } } }));
		assert.deepEqual((await tool.execute("call", { text: "text" })).structuredContent, { backend });
	}
	let tool, attempts = 0;
	registerClipboardTool({ registerTool(value) { tool = value; } }, async () => {
		attempts++;
		throw new Error("Bridge interrupted; operation may have completed");
	});
	await assert.rejects(tool.execute("call", { text: "text" }), /operation may have completed/);
	assert.equal(attempts, 1);
});
