// @ts-check

import { randomBytes } from "node:crypto";
import { renameSync, statSync, writeFileSync } from "node:fs";
import path from "node:path";

/** @typedef {"idle" | "working" | "blocked"} State */
/** @typedef {import("@earendil-works/pi-coding-agent").ExtensionContext} ExtensionContext */

/** @param {import("@earendil-works/pi-coding-agent").ExtensionAPI} pi */
export default function (pi) {
	const configured = process.env.PISSBOX_BRIDGE_DIR;
	if (!configured || !path.isAbsolute(configured)) return;
	const mailbox = configured;

	let active = false;
	let busy = false;
	let prompts = 0;
	/** @type {State | undefined} */
	let lastState;
	let warned = false;

	/**
	 * @param {{kind: "state", state: State} | {kind: "release"}} event
	 * @param {ExtensionContext} ctx
	 */
	function send(event, ctx) {
		try {
			if (Date.now() - statSync(path.join(mailbox, "ready")).mtimeMs > 5000) {
				throw new Error("Host helper is unavailable");
			}
			// Monotonic names preserve even fast working → idle transitions and reloads.
			const name = `${process.hrtime.bigint().toString().padStart(20, "0")}-${randomBytes(8).toString("hex")}`;
			const pending = path.join(mailbox, `.${name}.pending`);
			writeFileSync(pending, JSON.stringify({ version: 1, ...event }), { flag: "wx", mode: 0o600 });
			renameSync(pending, path.join(mailbox, `${name}.json`));
		} catch {
			if (!warned) {
				warned = true;
				ctx.ui.notify("Pissbox bridge unavailable; Pi will continue without Herd reporting.", "warning");
			}
		}
	}

	/** @param {ExtensionContext} ctx */
	function publish(ctx) {
		if (!active) return;
		const state = prompts > 0 ? "blocked" : busy ? "working" : "idle";
		if (state === lastState) return;
		lastState = state;
		send({ kind: "state", state }, ctx);
	}

	pi.on("session_start", (_event, ctx) => {
		if (ctx.mode !== "tui") return;
		// Old sandboxes may retain a previous launch's environment. Stay silent then.
		try {
			if (Date.now() - statSync(path.join(mailbox, "ready")).mtimeMs > 5000) return;
		} catch {
			return;
		}
		active = true;
		busy = !ctx.isIdle();
		prompts = 0;
		lastState = undefined;
		publish(ctx);
	});
	pi.on("agent_start", (_event, ctx) => {
		busy = true;
		publish(ctx);
	});
	pi.on("agent_settled", (_event, ctx) => {
		if (!ctx.isIdle()) return;
		busy = false;
		publish(ctx);
	});
	pi.on("ui_prompt_start", (_event, ctx) => {
		prompts += 1;
		publish(ctx);
	});
	pi.on("ui_prompt_end", (_event, ctx) => {
		prompts = Math.max(0, prompts - 1);
		publish(ctx);
	});
	pi.on("session_shutdown", (_event, ctx) => {
		if (!active) return;
		active = false;
		send({ kind: "release" }, ctx);
	});
}
