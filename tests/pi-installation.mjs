import { realpathSync } from "node:fs";
import { createRequire } from "node:module";
import { pathToFileURL } from "node:url";

// npm links directly to dist/cli.js; Homebrew can link to a bin wrapper whose
// package lives under the formula's libexec instead.
export function resolvePiEntry(executable) {
	const cli = pathToFileURL(realpathSync(executable));
	const localRequire = createRequire(cli);
	try {
		return localRequire.resolve("./index.js");
	} catch (error) {
		if (error.code !== "MODULE_NOT_FOUND") throw error;
	}
	for (const anchor of ["../libexec/lib/pi.cjs", "../libexec/pi.cjs"]) {
		const require = createRequire(new URL(anchor, cli));
		try {
			return require.resolve("@earendil-works/pi-coding-agent");
		} catch (error) {
			if (error.code !== "MODULE_NOT_FOUND") throw error;
		}
	}
	throw new Error(`Cannot locate Pi's JavaScript package from ${cli.pathname}`);
}
