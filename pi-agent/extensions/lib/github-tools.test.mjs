import assert from "node:assert/strict";
import test from "node:test";
import { registerGitHubTools } from "./github-tools.js";

test("GitHub tools expose only the five read operations and share session-bound transport", async () => {
	const tools = new Map(), calls = [];
	const result = { nodes: [], pageInfo: { hasNextPage: false, endCursor: null } };
	registerGitHubTools({ registerTool(tool) { tools.set(tool.name, tool); } }, async (...args) => {
		calls.push(args);
		return { structuredContent: { result } };
	});
	assert.deepEqual([...tools.keys()], ["github_pr_list", "github_pr_view", "github_pr_diff", "github_pr_comments", "github_pr_checks"]);
	const signal = new AbortController().signal;
	for (const [name, tool] of tools) {
		const params = { repo: "acme/project", ...(name === "github_pr_list" ? { state: "all" } : { number: 7 }) };
		const response = await tool.execute("call", params, signal);
		assert.deepEqual(calls.at(-1), ["github", { githubOp: name.slice("github_".length), githubParams: params }, signal]);
		assert.deepEqual(response.structuredContent, result);
		assert.deepEqual(response.details, result);
		assert.deepEqual(JSON.parse(response.content[0].text), result);
		assert.equal(tool.parameters.additionalProperties, false);
		assert.equal(tool.annotations.readOnlyHint, true);
		assert.equal(tool.annotations.destructiveHint, false);
		assert.equal(tool.namespace.name, "github");
		assert.ok(tool.promptGuidelines[0].includes(name));
		assert.ok(!["command", "args", "query", "endpoint", "headers"].some((key) => key in tool.parameters.properties));
	}
	assert.deepEqual(tools.get("github_pr_list").parameters.properties.state.enum, ["open", "closed", "merged", "all"]);
	assert.deepEqual(tools.get("github_pr_comments").parameters.properties.kind.enum, ["conversation", "reviews", "inline"]);
	assert.deepEqual(tools.get("github_pr_list").parameters.required, ["repo"]);
	assert.deepEqual(tools.get("github_pr_view").parameters.required, ["repo", "number"]);
	assert.ok(tools.get("github_pr_checks").parameters.properties.cursor);
	assert.ok(tools.get("github_pr_diff").parameters.properties.offset);
});

test("GitHub adapter rejects incompatible results and never retries a failed request", async () => {
	for (const result of [undefined, null, "text", [], 4]) {
		let call;
		registerGitHubTools({ registerTool(tool) { call = tool.execute; } }, async () => ({ structuredContent: { result } }));
		await assert.rejects(call("call", {}), /Invalid GitHub response/);
	}
	let call, attempts = 0;
	registerGitHubTools({ registerTool(tool) { call = tool.execute; } }, async () => {
		attempts++;
		throw new Error("host gh unavailable");
	});
	await assert.rejects(call("call", {}), /host gh unavailable/);
	assert.equal(attempts, 1);
});
