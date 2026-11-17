import assert from "node:assert/strict";
import test from "node:test";
import { registerClipboardTool } from "./clipboard-tools.js";

test("clipboard_copy automatically offers one terminal command while preserving plain text", async () => {
	let tool;
	const calls = [];
	registerClipboardTool({ registerTool(value) { tool = value; } }, async (...args) => {
		calls.push(args);
		return { structuredContent: { result: { backend: "pbcopy" } } };
	});
	assert.equal(tool.name, "clipboard_copy");
	assert.deepEqual(Object.keys(tool.parameters.properties), ["text", "shellCommand"]);
	assert.deepEqual(tool.parameters.required, ["text"]);
	assert.equal(tool.parameters.additionalProperties, false);
	assert.equal(tool.parameters.properties.shellCommand.type, "boolean");
	assert.equal(tool.annotations.readOnlyHint, false);
	assert.equal(tool.annotations.destructiveHint, true);
	assert.equal(tool.executionMode, "sequential");
	assert.match(tool.description, /exactly one terminal command/);
	assert.ok(tool.promptGuidelines.some((rule) => rule.includes("clipboard_copy automatically") && rule.includes("explicit copy request is not required")));
	assert.ok(tool.promptGuidelines.some((rule) => rule.includes("shellCommand=true") && rule.includes("without appending a clipboard pipeline yourself")));
	assert.ok(tool.promptGuidelines.some((rule) => rule.includes("clipboard_copy succeeds") && rule.includes("copiedText verbatim")));
	assert.ok(tool.promptGuidelines.some((rule) => rule.includes("clipboard_copy") && rule.includes("Grug-style") && rule.includes("meat-proxy")));
	const signal = new AbortController().signal;
	for (const text of ["exact text 世界\n\t", "", "x".repeat(16384), "printf shell-looking-text"]) {
		const result = await tool.execute("call", { text }, signal);
		assert.deepEqual(calls.at(-1), ["clipboard_copy", { clipboardText: text }, signal]);
		assert.deepEqual(result.structuredContent, { backend: "pbcopy", copiedText: text });
		assert.deepEqual(result.details, result.structuredContent);
		assert.deepEqual(JSON.parse(result.content[0].text), result.structuredContent);
	}
	await tool.execute("call", { text: "verbatim", shellCommand: false }, signal);
	assert.deepEqual(calls.at(-1), ["clipboard_copy", { clipboardText: "verbatim" }, signal]);
	for (const text of [undefined, null, 123, "x".repeat(16385), "世".repeat(6000), "\ud800"]) {
		await assert.rejects(tool.execute("call", { text }), /valid UTF-8/);
	}
	assert.equal(calls.length, 5, "invalid input must not reach host");
});

test("shell commands request host preparation and display the actual wrapped payload", async () => {
	let tool;
	const calls = [];
	const original = "make check";
	const prepared = "(set -o pipefail; {\nmake check\n} 2>&1 | tee /dev/tty | pbcopy)";
	registerClipboardTool({ registerTool(value) { tool = value; } }, async (...args) => {
		calls.push(args);
		return { structuredContent: { result: { backend: "pbcopy", copiedText: prepared } } };
	});
	const signal = new AbortController().signal;
	const result = await tool.execute("call", { text: original, shellCommand: true }, signal);
	assert.deepEqual(calls, [["clipboard_copy", { clipboardText: original, clipboardShellCommand: true }, signal]]);
	assert.deepEqual(result.structuredContent, { backend: "pbcopy", copiedText: prepared });
	assert.deepEqual(result.details, result.structuredContent);
	assert.deepEqual(JSON.parse(result.content[0].text), result.structuredContent);
	assert.notEqual(result.structuredContent.copiedText, original);
	for (const text of ["", " \n\t", "printf\0bad"]) {
		await assert.rejects(tool.execute("call", { text, shellCommand: true }), /nonempty/);
	}
	for (const shellCommand of [null, "true", 1]) {
		await assert.rejects(tool.execute("call", { text: original, shellCommand }), /boolean/);
	}
	assert.equal(calls.length, 1);
});

test("clipboard_copy rejects incompatible command results instead of silently copying raw commands", async () => {
	for (const copiedText of [undefined, null, "", " \n", "\ud800", "x".repeat(16385)]) {
		let tool;
		registerClipboardTool({ registerTool(value) { tool = value; } }, async () => ({ structuredContent: { result: { backend: "pbcopy", copiedText } } }));
		await assert.rejects(tool.execute("call", { text: "make check", shellCommand: true }), /rebuild and reload/);
	}
});

test("clipboard_copy validates host backends and never retries uncertain writes", async () => {
	for (const result of [undefined, null, {}, { backend: "shell" }]) {
		let tool;
		registerClipboardTool({ registerTool(value) { tool = value; } }, async () => ({ structuredContent: { result } }));
		await assert.rejects(tool.execute("call", { text: "text" }), /Invalid clipboard response/);
	}
	for (const backend of ["pbcopy", "wl-copy", "xclip", "xsel"]) {
		let tool;
		registerClipboardTool({ registerTool(value) { tool = value; } }, async () => ({ structuredContent: { result: { backend } } }));
		assert.deepEqual((await tool.execute("call", { text: "text" })).structuredContent, { backend, copiedText: "text" });
	}
	let tool, attempts = 0;
	registerClipboardTool({ registerTool(value) { tool = value; } }, async () => {
		attempts++;
		throw new Error("Bridge interrupted; operation may have completed");
	});
	await assert.rejects(tool.execute("call", { text: "make check", shellCommand: true }), /operation may have completed/);
	assert.equal(attempts, 1);
});
