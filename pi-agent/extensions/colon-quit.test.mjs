import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { createRequire } from "node:module";
import test from "node:test";
import { resolvePiEntry } from "../../tests/pi-installation.mjs";

const piEntry = resolvePiEntry(execFileSync("sh", ["-c", "command -v pi"], { encoding: "utf8" }).trim());
const require = createRequire(piEntry);
const { createJiti } = require("jiti");
const { getKeybindings } = require("@earendil-works/pi-tui");
const jiti = createJiti(import.meta.url, {
	fsCache: false,
	alias: { "@earendil-works/pi-coding-agent": piEntry },
});
const extension = await jiti.import("./colon-quit.js", { default: true });

function session() {
	let editor;
	let quits = 0;
	const submitted = [];
	extension({ on: (_event, start) => start({}, {
		mode: "tui",
		shutdown: () => { quits++; },
		ui: {
			getEditorComponent: () => undefined,
			setEditorComponent: (factory) => {
				editor = factory({ requestRender() {}, terminal: { rows: 24 } }, {}, getKeybindings());
				editor.onSubmit = (text) => submitted.push(text);
			},
		},
	}) });
	return { editor, submitted, quits: () => quits };
}

test("quits only after submitting the exact :q buffer, without sending a prompt", () => {
	const { editor, submitted, quits } = session();
	editor.handleInput(":");
	editor.handleInput("q");
	assert.equal(quits(), 0, "typing :q does not quit before submission");
	editor.handleInput("\r");
	assert.equal(quits(), 1);
	assert.deepEqual(submitted, []);
});

test("submits near matches normally, even when Pi trims them to :q", () => {
	const { editor, submitted, quits } = session();
	const inputs = [":q!", ":Q", "say :q", " :q", ":q ", ":q\n"];
	for (const text of inputs) {
		editor.setText(text);
		editor.handleInput("\r");
	}
	assert.equal(quits(), 0);
	assert.deepEqual(submitted, [":q!", ":Q", "say :q", ":q", ":q", ":q"]);
});
