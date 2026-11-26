const SERVER_NAME = "pissbox-sbx";
const TOOL_PREFIX = "mcp__pissbox_sbx__";

export default function sandboxMcp(pi) {
	let enabled = false;
	let generation = 0;

	function disconnect() {
		generation++;
		const registered = enabled;
		enabled = false;
		if (registered) pi.unregisterMcpServer(SERVER_NAME);
	}

	pi.on("session_start", (_event, ctx) => {
		disconnect();
		ctx.ui.setStatus("sandbox-mcp", undefined);
	});
	pi.on("session_shutdown", disconnect);
	// Already queued calls must not regain access after OFF.
	pi.on("tool_call", (event) => {
		if (!enabled && event.toolName.startsWith(TOOL_PREFIX)) {
			return { block: true, reason: "Sandbox MCP is OFF. The user must enable /sandbox-mcp on." };
		}
	});

	pi.registerCommand("sandbox-mcp", {
		description: "Connect to the SBX MCP gateway for this session: on | off | status (default OFF)",
		getArgumentCompletions(prefix) {
			const items = ["on", "off", "status"]
				.filter((value) => value.startsWith(prefix))
				.map((value) => ({ value, label: value }));
			return items.length ? items : null;
		},
		async handler(args, ctx) {
			const action = args.trim().toLowerCase() || "status";
			if (action === "off") {
				disconnect();
				ctx.ui.setStatus("sandbox-mcp", undefined);
				ctx.ui.notify("Sandbox MCP OFF. SBX server attachments are unchanged.", "info");
				return;
			}
			if (action === "status") {
				ctx.ui.notify(enabled ? "Sandbox MCP ON (registered). Use /mcp for connection status." : "Sandbox MCP OFF.", "info");
				return;
			}
			if (action !== "on") {
				ctx.ui.notify("Usage: /sandbox-mcp on|off|status", "warning");
				return;
			}
			if (enabled) {
				ctx.ui.notify("Sandbox MCP already ON. Use /mcp for connection status.", "info");
				return;
			}
			const url = process.env.MCP_GATEWAY_URL;
			const token = process.env.MCP_SENTINEL_TOKEN_NAME;
			if (!url || !token || typeof pi.registerMcpServer !== "function") {
				ctx.ui.notify("SBX MCP gateway unavailable. Requires SBX gateway environment and Pi native MCP support.", "warning");
				return;
			}
			const currentGeneration = generation;
			if (!ctx.hasUI || !(await ctx.ui.confirm("Enable sandbox MCP?",
				"Expose all servers loaded into this sandbox to this Pi session? Browser tools can read authenticated data and act as you. OFF disconnects Pi, not other clients or SBX attachments."))) return;
			if (currentGeneration !== generation) return;
			pi.registerMcpServer(SERVER_NAME, {
				url,
				headers: { Authorization: `Bearer ${token}` },
				exposure: "deferred",
				description: "Host-managed tools explicitly loaded into this sandbox's SBX MCP gateway, including Chrome DevTools when granted.",
			});
			generation++;
			enabled = true;
			ctx.ui.setStatus("sandbox-mcp", "SBX MCP ON");
			ctx.ui.notify("Sandbox MCP ON (registered). Use /mcp to inspect connection and tools.", "info");
		},
	});
}
