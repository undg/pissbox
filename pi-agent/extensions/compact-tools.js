// @ts-check
/**
 * One-line collapsed tool calls.
 *
 * Collapsed rows show only `toolName arguments`. Expanding with Pi's normal
 * tool-output toggle (Ctrl+O by default) uses each tool's native renderer.
 * Delete this file to remove the extension.
 */

import { Box, Text, TruncatedText } from "@earendil-works/pi-tui";

const PREFERRED_ARG_KEYS = [
	"command",
	"pattern",
	"file_path",
	"path",
	"query",
	"url",
	"prompt",
	"text",
	"code",
	"script",
	"description",
	"name",
	"id",
];
const MAX_SUMMARY = 200;
/** @type {import("@earendil-works/pi-tui").Component} */
const EMPTY = { render: () => [], invalidate() {} };

/** @param {unknown} value */
function oneLine(value) {
	return String(value)
		.replace(/[\u0000-\u001f\u007f-\u009f]+/g, " ")
		.replace(/\s+/g, " ")
		.trim()
		.slice(0, MAX_SUMMARY);
}

/**
 * Pick the most useful argument for a one-line tool row.
 * @param {unknown} args
 */
export function summarizeToolArgs(args) {
	if (args === null || args === undefined) return "";
	if (typeof args !== "object") return oneLine(args);

	const values = /** @type {Record<string, unknown>} */ (args);
	for (const key of PREFERRED_ARG_KEYS) {
		const value = values[key];
		if (typeof value === "string" && value.trim()) return oneLine(value);
	}
	for (const value of Object.values(values)) {
		if (typeof value === "string" && value.trim()) return oneLine(value);
	}
	try {
		const json = JSON.stringify(args);
		return json && json !== "{}" ? oneLine(json) : "";
	} catch {
		return "";
	}
}

/**
 * @param {string} toolName
 * @param {unknown} args
 * @param {import("@earendil-works/pi-coding-agent").Theme} theme
 */
function collapsedLine(toolName, args, theme) {
	const name = theme.bold(theme.fg("toolTitle", toolName));
	const summary = summarizeToolArgs(args);
	return summary ? `${name} ${theme.fg("accent", summary)}` : name;
}

/** @param {unknown} result */
function resultText(result) {
	const content = /** @type {{ content?: Array<{ type?: string, text?: string }> }} */ (result)?.content;
	if (!Array.isArray(content)) return "";
	return content
		.filter((block) => block?.type === "text" && block.text)
		.map((block) => block.text)
		.join("\n")
		.trim();
}

/**
 * Native renderers may keep type-specific state on their last component.
 * Never hand them this extension's one-line component.
 * @param {any} context
 */
function freshContext(context) {
	return { ...context, lastComponent: undefined };
}

/** @param {import("@earendil-works/pi-coding-agent").ExtensionAPI} pi */
export default function compactTools(pi) {
	pi.registerToolRenderer((toolName, next) => {
		const native = next();
		const shell = native?.renderShell === "self" ? "self" : "default";

		/** @param {number} outputPad */
		const padding = (outputPad) => shell === "self" ? outputPad : 0;

		/**
		 * Self-rendering avoids Pi's default vertical box padding. Keep native
		 * self-renderers untouched when expanded; frame default renderers here.
		 * @param {import("@earendil-works/pi-tui").Component} component
		 * @param {import("@earendil-works/pi-coding-agent").Theme} theme
		 * @param {any} context
		 */
		const frame = (component, theme, context) => {
			if (context.expanded && shell === "self") return component;
			const background = context.isPartial ? "toolPendingBg" : context.isError ? "toolErrorBg" : "toolSuccessBg";
			const box = new Box(context.outputPad, context.expanded ? 1 : 0, (text) => theme.bg(background, text));
			box.addChild(component);
			return box;
		};

		return {
			renderShell: "self",
			renderCall(args, theme, context) {
				if (context.expanded) {
					if (native?.renderCall) {
						return frame(native.renderCall(args, theme, freshContext(context)), theme, context);
					}
					const body = JSON.stringify(args ?? {}, null, 2);
					const text = `${theme.fg("toolTitle", theme.bold(toolName))}\n${theme.fg("toolOutput", body)}`;
					return frame(new Text(text, padding(context.outputPad), 0), theme, context);
				}
				return frame(new TruncatedText(collapsedLine(toolName, args, theme), 0, 0), theme, context);
			},
			renderResult(result, options, theme, context) {
				if (!context.expanded) return EMPTY;
				if (native?.renderResult) {
					return frame(native.renderResult(result, { ...options, expanded: true }, theme, freshContext(context)), theme, context);
				}
				const text = resultText(result);
				return text ? frame(new Text(theme.fg("toolOutput", text), padding(context.outputPad), 0), theme, context) : EMPTY;
			},
		};
	});
}
