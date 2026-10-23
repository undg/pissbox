// @ts-check

import { truncateToWidth, visibleWidth } from "@earendil-works/pi-tui";

const key = "turn-timing";

/** @param {number | undefined} timestamp */
function clock(timestamp) {
	if (timestamp === undefined) return "—";
	const date = new Date(timestamp);
	const time = date.toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit" });
	return date.toDateString() === new Date().toDateString()
		? time : `${date.getMonth() + 1}/${date.getDate()} ${time}`;
}

/** @param {number} milliseconds */
function duration(milliseconds) {
	const seconds = Math.max(0, Math.floor(milliseconds / 1000));
	if (seconds < 60) return `${seconds}s`;
	const minutes = Math.floor(seconds / 60);
	if (minutes < 60) return `${minutes}m${seconds % 60}s`;
	return `${Math.floor(minutes / 60)}h${minutes % 60}m`;
}

/** @typedef {{ sent?: number, started?: number, done?: number }} Timing */

/** @param {unknown} value @returns {value is Timing} */
function isTiming(value) {
	if (!value || typeof value !== "object") return false;
	return ["sent", "started", "done"].every((field) => {
		const timestamp = /** @type {Record<string, unknown>} */ (value)[field];
		return timestamp === undefined || (typeof timestamp === "number" && Number.isFinite(timestamp) && timestamp >= 0);
	});
}

/** @param {import("@earendil-works/pi-coding-agent").ExtensionAPI} pi */
export default function (pi) {
	/** @type {Timing} */
	let timing = {};
	let running = false;
	/** @type {number | undefined} */
	let submitted;
	/** @type {ReturnType<typeof setInterval> | undefined} */
	let timer;
	/** @type {(() => void) | undefined} */
	let resize;

	function stopTimer() {
		clearInterval(timer);
		timer = undefined;
	}

	function cleanup() {
		stopTimer();
		if (resize) process.stdout.off("resize", resize);
		resize = undefined;
	}

	/** @param {import("@earendil-works/pi-coding-agent").ExtensionContext} ctx */
	function update(ctx) {
		if (ctx.mode !== "tui") return;
		const width = process.stdout.columns || 80;
		const now = Date.now();
		const elapsed = timing.started === undefined ? "—" : duration((timing.done ?? now) - timing.started);
		const right = `sent ${clock(timing.sent)} · done ${clock(timing.done)} · took ${running ? "—" : elapsed}`;
		const activity = running ? `working ${elapsed}`
			: `idle ${timing.done === undefined ? "—" : duration(now - timing.done)}`;
		const left = truncateToWidth(activity, Math.max(0, width - visibleWidth(right) - 2), "");
		const gap = Math.max(0, width - visibleWidth(left) - visibleWidth(right));
		// Pi collapses ordinary spaces in status text. Non-breaking spaces preserve alignment.
		const text = visibleWidth(right) >= width ? truncateToWidth(right, width, "")
			: left + "\u00a0".repeat(gap) + right;
		ctx.ui.setStatus(key, text);
	}

	/** @param {import("@earendil-works/pi-coding-agent").ExtensionContext} ctx */
	function restore(ctx) {
		cleanup();
		timing = {};
		running = false;
		submitted = undefined;
		if (ctx.mode !== "tui") return;
		for (const entry of ctx.sessionManager.getBranch()) {
			if (entry.type === "message" && entry.message.role === "user") {
				timing = { sent: entry.message.timestamp };
			} else if (entry.type === "custom" && entry.customType === key && isTiming(entry.data)) {
				timing = { ...entry.data };
			}
		}
		resize = () => update(ctx);
		process.stdout.on("resize", resize);
		timer = setInterval(() => update(ctx), 1000);
		timer.unref();
		update(ctx);
	}

	pi.on("session_start", (_event, ctx) => restore(ctx));
	pi.on("session_tree", (_event, ctx) => restore(ctx));
	pi.on("session_shutdown", (_event, ctx) => {
		cleanup();
		if (ctx.mode === "tui") ctx.ui.setStatus(key, undefined);
	});

	pi.on("input", (_event, ctx) => {
		if (ctx.mode !== "tui") return;
		submitted = Date.now();
		timing.sent = submitted;
		update(ctx);
	});
	pi.on("before_agent_start", (event, ctx) => {
		if (ctx.mode !== "tui") return;
		if (!running) {
			const start = event.messageType === "user" ? submitted ?? Date.now() : Date.now();
			timing = { sent: event.messageType === "user" ? start : timing.sent, started: start };
			running = true;
		}
		update(ctx);
	});
	pi.on("message_start", (event, ctx) => {
		if (ctx.mode !== "tui" || event.message.role !== "user") return;
		timing.sent = submitted ?? event.message.timestamp;
		submitted = undefined;
		update(ctx);
	});
	pi.on("agent_settled", (_event, ctx) => {
		if (ctx.mode !== "tui" || !running) return;
		timing.done = Date.now();
		running = false;
		// Display-only session metadata; never added to the model's conversation.
		pi.appendEntry(key, { ...timing });
		update(ctx);
	});
}
