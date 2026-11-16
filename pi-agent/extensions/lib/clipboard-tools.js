/** Register write-only host clipboard access over the session transport. */
export function registerClipboardTool(pi, request) {
	pi.registerTool({
		name: "clipboard_copy", label: "Copy to host clipboard",
		description: "Copy text exactly to the user's host clipboard using pbcopy, wl-copy, xclip, or xsel. Write-only; never reads the clipboard. At most 16 KiB UTF-8, subject to the bridge record limit. Empty text clears it. Returns the backend used. An interrupted request may have completed; never retry automatically.",
		promptGuidelines: [
			"Use clipboard_copy only when the user asks to copy text to their clipboard. Never retry clipboard_copy automatically after an interrupted or indeterminate result.",
			"After clipboard_copy succeeds, show the complete copied text verbatim in a fenced code block in your chat reply, not just a confirmation or summary. For empty text, explicitly say the clipboard was cleared. Never claim a failed or indeterminate copy succeeded.",
			"When using clipboard_copy, add one short, varied Grug-style quip casting the user as the meat-proxy who supplies hands and does the physical pasting. Use playful cave-speech and task-specific teasing, not a generic confirmation with meat-proxy tacked on. Keep the clipboard text unchanged; all jokes belong outside the copied text and its code block.",
		],
		annotations: { readOnlyHint: false, destructiveHint: true, idempotentHint: true, openWorldHint: false },
		executionMode: "sequential",
		parameters: {
			type: "object", required: ["text"], additionalProperties: false,
			properties: { text: { type: "string", maxLength: 16384, description: "Exact text to copy, including whitespace; empty string clears the clipboard." } },
		},
		outputSchema: {
			type: "object", required: ["backend"], additionalProperties: false,
			properties: { backend: { type: "string", enum: ["pbcopy", "wl-copy", "xclip", "xsel"] } },
		},
		async execute(_id, { text }, signal) {
			if (typeof text !== "string" || Buffer.byteLength(text) > 16384 || !text.isWellFormed()) {
				throw new Error("Clipboard text must be valid UTF-8 and at most 16384 bytes");
			}
			const response = await request("clipboard_copy", { clipboardText: text }, signal);
			const result = response.structuredContent?.result;
			if (!result || !["pbcopy", "wl-copy", "xclip", "xsel"].includes(result.backend)) {
				throw new Error("Invalid clipboard response; rebuild and reload the host bridge");
			}
			return { content: [{ type: "text", text: JSON.stringify(result) }], details: result, structuredContent: result };
		},
	});
}
