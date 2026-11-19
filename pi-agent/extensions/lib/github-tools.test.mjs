import assert from "node:assert/strict";
import test from "node:test";
import { registerGitHubTools } from "./github-tools.js";

test("GitHub tools expose only fixed read operations and share session-bound transport", async () => {
	const tools = new Map(), calls = [];
	const result = { nodes: [], pageInfo: { hasNextPage: false, endCursor: null } };
	registerGitHubTools({ registerTool(tool) { tools.set(tool.name, tool); } }, async (...args) => {
		calls.push(args);
		return { structuredContent: { result } };
	});
	assert.deepEqual([...tools.keys()], ["github_pr_list", "github_pr_view", "github_pr_diff", "github_pr_comments", "github_pr_checks", "github_run_view", "github_job_logs"]);
	const signal = new AbortController().signal;
	for (const [name, tool] of tools) {
		const inputs = { github_pr_list: { state: "all" }, github_run_view: { runId: 6000000000, page: 2, limit: 1 }, github_job_logs: { jobId: 7000000000, offset: 12000 } };
		const params = { repo: "acme/project", ...(inputs[name] ?? { number: 7 }) };
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
	assert.deepEqual(tools.get("github_run_view").parameters.required, ["repo", "runId"]);
	assert.deepEqual(tools.get("github_job_logs").parameters.required, ["repo", "jobId"]);
	assert.equal(tools.get("github_run_view").parameters.properties.runId.maximum, Number.MAX_SAFE_INTEGER);
	assert.equal(tools.get("github_job_logs").parameters.properties.jobId.maximum, Number.MAX_SAFE_INTEGER);
	assert.ok(tools.get("github_run_view").parameters.properties.page);
	assert.ok(tools.get("github_job_logs").parameters.properties.offset);
	assert.ok(!("number" in tools.get("github_run_view").parameters.properties));
	assert.ok(!("runId" in tools.get("github_job_logs").parameters.properties));
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
