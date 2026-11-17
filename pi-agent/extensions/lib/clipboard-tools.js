/** Register write-only host clipboard access over the session transport. */
export function registerClipboardTool(pi, request) {
	pi.registerTool({
		name: "clipboard_copy", label: "Copy to host clipboard",
		description: "Put text or one shell command on the user's host clipboard. Use whenever you give the user exactly one terminal command to run, even without an explicit copy request. Set shellCommand=true to prepare a Bash/zsh command that shows stdout/stderr and copies that output back using host pbcopy, wl-copy, xclip, or xsel. It only copies the prepared command; it never executes it. Otherwise text is copied exactly; empty text clears the clipboard. At most 16 KiB UTF-8 after preparation, subject to bridge record limits. Returns backend and copiedText. Never reads the clipboard or automatically retries uncertain writes.",
		promptGuidelines: [
			"Whenever you hand the user exactly one terminal command to run, call clipboard_copy automatically; an explicit copy request is not required. This does not apply to commands you execute yourself with bash, illustrative snippets, or multiple alternative commands. Never retry clipboard_copy automatically after an interrupted or indeterminate result.",
			"For a noninteractive shell command, use clipboard_copy with shellCommand=true and pass the original command without appending a clipboard pipeline yourself. Use shellCommand=false for ordinary text, commands requested verbatim, commands already capturing output, interactive programs, or commands such as cd/export that must change the user's shell state. Do not treat plain text as shell code.",
			"After clipboard_copy succeeds, show its returned copiedText verbatim in a fenced code block in your chat reply, not just a confirmation or the original unwrapped command. For an empty copiedText, explicitly say the clipboard was cleared. Never claim a failed or indeterminate copy succeeded.",
			"When using clipboard_copy, add one short, varied Grug-style quip casting the user as the meat-proxy who supplies hands and does the physical pasting. Use playful cave-speech and task-specific teasing, not a generic confirmation with meat-proxy tacked on. Keep jokes outside copiedText and its code block; never alter the returned payload.",
		],
		annotations: { readOnlyHint: false, destructiveHint: true, idempotentHint: true, openWorldHint: false },
		executionMode: "sequential",
		parameters: {
			type: "object", required: ["text"], additionalProperties: false,
			properties: {
				text: { type: "string", maxLength: 16384, description: "Text to copy exactly, or the original shell command when shellCommand=true. Empty text clears the clipboard only in plain-text mode." },
				shellCommand: { type: "boolean", description: "Prepare a noninteractive Bash/zsh command with visible stdout/stderr copied back to the host clipboard. Default false; plain text is never wrapped. Do not use for cd/export or interactive programs." },
			},
		},
		outputSchema: {
			type: "object", required: ["backend", "copiedText"], additionalProperties: false,
			properties: {
				backend: { type: "string", enum: ["pbcopy", "wl-copy", "xclip", "xsel"] },
				copiedText: { type: "string", description: "Exact clipboard payload, including any shell capture wrapper. Display this verbatim to the user." },
			},
		},
		async execute(_id, { text, shellCommand = false }, signal) {
			if (typeof text !== "string" || Buffer.byteLength(text) > 16384 || !text.isWellFormed()) {
				throw new Error("Clipboard text must be valid UTF-8 and at most 16384 bytes");
			}
			if (typeof shellCommand !== "boolean") throw new Error("shellCommand must be a boolean");
			if (shellCommand && (!text.trim() || text.includes("\0"))) throw new Error("Shell command must be nonempty and contain no NUL bytes");
			const params = { clipboardText: text };
			if (shellCommand) params.clipboardShellCommand = true;
			const response = await request("clipboard_copy", params, signal);
			const host = response.structuredContent?.result;
			const copiedText = shellCommand ? host?.copiedText : text;
			if (!host || !["pbcopy", "wl-copy", "xclip", "xsel"].includes(host.backend) ||
				typeof copiedText !== "string" || !copiedText.isWellFormed() || Buffer.byteLength(copiedText) > 16384 ||
				(shellCommand && !copiedText.trim())) {
				throw new Error("Invalid clipboard response; rebuild and reload the host bridge; do not retry automatically");
			}
			const result = { backend: host.backend, copiedText };
			return { content: [{ type: "text", text: JSON.stringify(result) }], details: result, structuredContent: result };
		},
	});
}
