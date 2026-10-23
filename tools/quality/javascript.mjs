import { spawnSync } from "node:child_process";
import { mkdirSync } from "node:fs";
import { createRequire } from "node:module";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const root = fileURLToPath(new URL("../../", import.meta.url));
const tools = path.join(root, ".cache/quality-tools/js");
const requireTool = createRequire(path.join(tools, "package.json"));
const tests = ["go-bridge/pi-extension.test.mjs", "tests/session-name.test.mjs"];
const [command, ...args] = process.argv.slice(2);
const dryRun = args.includes("--dry-run");
const files = args.filter((arg) => arg !== "--dry-run");
process.chdir(root);

function loadTool(name) {
	return import(pathToFileURL(requireTool.resolve(name)).href);
}

function coverage() {
	const output = path.join(root, ".cache/coverage/js");
	const result = spawnSync(process.execPath, [
		path.join(tools, "node_modules/c8/bin/c8.js"),
		"--all", "--include=pi-agent/extensions/**/*.js",
		"--reporter=json", "--reporter=lcov", "--reporter=text", "--reporter=html",
		`--reports-dir=${output}`, `--temp-directory=${output}/tmp`,
		process.execPath, "--test", ...tests,
	], { stdio: "inherit" });
	if (result.error) throw result.error;
	if (result.status !== 0) throw new Error(`JS coverage tests failed (${result.signal ?? result.status})`);
	return output;
}

async function mutation() {
	if (!files.length) throw new Error("Refusing mutation testing without an explicit file selection");
	const { Stryker } = await loadTool("@stryker-mutator/core");
	const output = ".cache/mutation/js";
	// Stryker normalizes backslashes to separators. Bracket-escape literals
	// instead, including any metacharacters in the checkout's own path.
	const { escape } = await loadTool("minimatch");
	const patterns = files.map((name) => escape(path.resolve(name), { windowsPathsNoEscape: true }));
	// Messaging integration tests spawn an isolated bridge. Keep its socket
	// paths outside Stryker's deeply nested workdirs (Unix has a short limit).
	process.env.PISSBOX_BRIDGE_TEST_FIXTURE_DIR = path.join(root, ".cache/test-fixtures");
	const stryker = new Stryker({
		mutate: patterns,
		// Whitelist source and test dependencies, including the Go messaging
		// fixture. Never copy shared auth, sessions, caches or host binaries.
		ignorePatterns: [
			"**", "!pi-agent/extensions/**/*.js", "!tests/pi-installation.mjs",
			"!go-bridge/agent-messaging.test.mjs", "!go-bridge/*.go",
			"!go-bridge/go.mod", "!go-bridge/go.sum",
			...tests.map((name) => `!${name}`),
		],
		testRunner: "command",
		commandRunner: { command: `node --test ${tests.join(" ")}` },
		coverageAnalysis: "off",
		concurrency: 1,
		symlinkNodeModules: false,
		dryRunOnly: dryRun,
		cleanTempDir: false,
		tempDirName: `${output}/sandboxes`,
		thresholds: { high: 80, low: 60, break: 0 },
		reporters: ["clear-text", "html", "json"],
		htmlReporter: { fileName: `${output}/index.html` },
		jsonReporter: { fileName: `${output}/report.json` },
	});
	await stryker.runMutationTest();
}

async function crap() {
	if (!files.length) throw new Error("CRAP analysis needs selected source files");
	const covered = coverage();
	const output = path.join(root, ".cache/crap/js");
	mkdirSync(output, { recursive: true });
	const cli = path.join(tools, "node_modules/@danibram/crap4ts/dist/cli.js");
	const common = [cli, ...files, "--coverage", path.join(covered, "lcov.info"), "--coverage-format=lcov"];
	for (const options of [["--reporter=json", "--output", path.join(output, "report.json")], ["--reporter=table"]]) {
		const result = spawnSync(process.execPath, [...common, ...options], { stdio: "inherit" });
		if (result.error) throw result.error;
		if (result.status !== 0) throw new Error(`JS CRAP analysis failed (${result.signal ?? result.status})`);
	}
}

try {
	if (command === "coverage") coverage();
	else if (command === "mutation") await mutation();
	else if (command === "crap") await crap();
	else throw new Error(`Unknown quality command: ${command}`);
} catch (error) {
	console.error(error.code === "MODULE_NOT_FOUND" ? "Missing quality dependencies; run make tools first" : error);
	process.exitCode = 1;
}
