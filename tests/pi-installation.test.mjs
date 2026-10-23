import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, symlinkSync, writeFileSync } from "node:fs";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { resolvePiEntry } from "./pi-installation.mjs";

test("resolves Pi's API through npm symlinks and Homebrew wrappers", () => {
	const fixtures = fileURLToPath(new URL("../.cache/test-fixtures/", import.meta.url));
	mkdirSync(fixtures, { recursive: true });
	const root = mkdtempSync(path.join(fixtures, "pi-installation-"));
	const npmDist = path.join(root, "npm/node_modules/@earendil-works/pi-coding-agent/dist");
	mkdirSync(npmDist, { recursive: true });
	writeFileSync(path.join(npmDist, "cli.js"), "");
	writeFileSync(path.join(npmDist, "index.js"), "");
	const npmBin = path.join(root, "npm/bin");
	mkdirSync(npmBin);
	symlinkSync(path.join(npmDist, "cli.js"), path.join(npmBin, "pi"));
	assert.equal(resolvePiEntry(path.join(npmBin, "pi")), path.join(npmDist, "index.js"));

	for (const location of ["libexec/lib", "libexec"]) {
		const formula = path.join(root, location.replaceAll("/", "-"));
		const packageRoot = path.join(formula, location, "node_modules/@earendil-works/pi-coding-agent");
		mkdirSync(path.join(packageRoot, "dist"), { recursive: true });
		writeFileSync(path.join(packageRoot, "package.json"), JSON.stringify({ main: "dist/index.js" }));
		writeFileSync(path.join(packageRoot, "dist/index.js"), "");
		mkdirSync(path.join(formula, "bin"));
		const wrapper = path.join(formula, "bin/pi");
		writeFileSync(wrapper, "#!/bin/sh\n# Homebrew executable wrapper\n");
		const link = path.join(root, location.replaceAll("/", "-") + "-pi");
		symlinkSync(wrapper, link);
		assert.equal(resolvePiEntry(link), path.join(packageRoot, "dist/index.js"));
	}
	const missing = path.join(root, "unsupported-pi");
	writeFileSync(missing, "");
	assert.throws(() => resolvePiEntry(missing), /Cannot locate Pi's JavaScript package/);
});
