import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { createRequire } from "node:module";
import test from "node:test";
import { pathToFileURL } from "node:url";
import { resolvePiEntry } from "../../tests/pi-installation.mjs";

const piEntry = resolvePiEntry(execFileSync("sh", ["-c", "command -v pi"], { encoding: "utf8" }).trim());
const piRequire = createRequire(piEntry);
const { ToolExecutionComponent, initTheme } = await import(pathToFileURL(piEntry));
initTheme("dark", false);
const { stripTerminalSequences, Text, visibleWidth } = piRequire("@earendil-works/pi-tui");
const { createJiti } = piRequire("jiti");
const jiti = createJiti(import.meta.url, {
	fsCache: false,
	alias: { "@earendil-works/pi-tui": piRequire.resolve("@earendil-works/pi-tui") },
});
const extension = await jiti.import("./compact-tools.js", { default: true });

let resolveTool;
extension({
	registerToolRenderer(resolver) {
		resolveTool = resolver;
	},
});
assert.equal(typeof resolveTool, "function", "extension must register a tool renderer");

const native = {
	renderCall(args) {
		return new Text(`NATIVE ${args.command}`, 0, 0);
	},
	renderResult(result) {
		return new Text(`NATIVE RESULT ${result.content[0].text}`, 0, 0);
	},
};

function toolComponent(toolName, args, nativeRenderer = native) {
	const renderers = resolveTool(toolName, () => nativeRenderer);
	return new ToolExecutionComponent(
		toolName,
		"call-1",
		args,
		{ outputPad: 1 },
		renderers,
		{ requestRender() {} },
		process.cwd(),
	);
}

function result(text, isError = false) {
	return { content: [{ type: "text", text }], details: {}, isError };
}

function visibleLines(component, width = 80) {
	return component.render(width)
		.map((line) => stripTerminalSequences(line).trim())
		.filter(Boolean);
}

test("collapsed tools occupy two lines, keeping horizontal padding and hiding output", () => {
	const component = toolComponent("bash", { command: "make test" });
	const expected = ["", ` bash make test${" ".repeat(65)}`];
	assert.deepEqual(component.render(80).map(stripTerminalSequences), expected);

	component.updateResult(result("line one\nline two"));
	assert.deepEqual(component.render(80).map(stripTerminalSequences), expected);

	component.setExpanded(true);
	component.setExpanded(false);
	assert.deepEqual(component.render(80).map(stripTerminalSequences), expected);
});

test("collapsed tools keep pending, success and error background colors", () => {
	const component = toolComponent("bash", { command: "make test" });
	const pending = component.render(80)[1];
	component.updateResult(result("done"));
	const success = component.render(80)[1];
	component.updateResult(result("failed", true));
	const error = component.render(80)[1];
	assert.notEqual(pending, stripTerminalSequences(pending));
	assert.notEqual(pending, success);
	assert.notEqual(success, error);
	assert.notEqual(pending, error);
});

test("native self-renderers also collapse to two lines and keep their expanded framing", () => {
	const component = toolComponent("bash", { command: "make test" }, { ...native, renderShell: "self" });
	assert.equal(component.render(80).length, 2);
	assert.equal(stripTerminalSequences(component.render(80)[1]).indexOf("bash"), 1);
	component.updateResult(result("done"));
	component.setExpanded(true);
	assert.deepEqual(component.render(80).map((line) => stripTerminalSequences(line).trimEnd()), ["", "NATIVE make test", "NATIVE RESULT done"]);
});

test("expanded rows delegate to the native renderer", () => {
	const component = toolComponent("bash", { command: "make test" });
	component.updateResult(result("line one\nline two"));
	component.setExpanded(true);

	const lines = visibleLines(component);
	assert.ok(lines.includes("NATIVE make test"));
	assert.match(lines.join("\n"), /NATIVE RESULT line one\s+line two/);
});

test("collapsed argument summaries prefer useful strings and stay on one line", () => {
	for (const [toolName, args, expected] of [
		["read", { path: "src/app.js", offset: 10 }, "read src/app.js"],
		["grep", { pattern: "needle", path: "src" }, "grep needle"],
		["write", { path: "out.txt", content: "a\nb" }, "write out.txt"],
		["bash", { command: "echo one\necho two" }, "bash echo one echo two"],
	]) {
		const component = toolComponent(toolName, args);
		assert.deepEqual(visibleLines(component), [expected]);
	}
});

test("unknown tools also collapse to one line and expand generically", () => {
	const component = toolComponent("mcp__jira__issue", { query: "find bug" }, null);
	assert.deepEqual(visibleLines(component), ["mcp__jira__issue find bug"]);

	component.updateResult(result("issue found"));
	assert.deepEqual(visibleLines(component), ["mcp__jira__issue find bug"]);

	component.setExpanded(true);
	const lines = visibleLines(component).join("\n");
	assert.match(lines, /"query": "find bug"/);
	assert.match(lines, /issue found/);
});

test("long arguments stay within the requested width", () => {
	const component = toolComponent("bash", { command: "x".repeat(500) });
	const lines = component.render(40);
	assert.ok(lines.length > 0);
	for (const line of lines) {
		assert.ok(visibleWidth(line) <= 40, `line is ${visibleWidth(line)} columns: ${JSON.stringify(line)}`);
	}
});
