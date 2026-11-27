// @ts-check

import { isAbsolute, relative, resolve, sep } from "node:path";
import { rgbColor, truncateToWidth, visibleWidth } from "@earendil-works/pi-tui";

const orange = rgbColor(255, 165, 0);

/** @param {number} count */
function formatTokens(count) {
	if (count < 1000) return count.toString();
	if (count < 10000) return `${(count / 1000).toFixed(1)}k`;
	if (count < 1000000) return `${Math.round(count / 1000)}k`;
	if (count < 10000000) return `${(count / 1000000).toFixed(1)}M`;
	return `${Math.round(count / 1000000)}M`;
}

/** @param {string} text */
function sanitize(text) {
	return text.replace(/[\r\n\t]/g, " ").replace(/ +/g, " ").trim();
}

/** @param {string} cwd */
function displayCwd(cwd) {
	const home = process.env.HOME || process.env.USERPROFILE;
	if (!home) return cwd;
	const path = relative(resolve(home), resolve(cwd));
	if (isAbsolute(path) || path === ".." || path.startsWith(`..${sep}`)) return cwd;
	return path ? `~${sep}${path}` : "~";
}

/** @param {import("@earendil-works/pi-coding-agent").ExtensionContext} ctx */
function sessionStats(ctx) {
	const totals = { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, cost: 0 };
	/** @type {number | undefined} */
	let cacheHitRate;
	// Totals include abandoned branches and pre-compaction usage, like Pi's footer.
	for (const entry of ctx.sessionManager.getEntries()) {
		let usage;
		if (entry.type === "usage") usage = entry.usage;
		else if (entry.type === "message" && entry.message.role === "assistant") {
			usage = entry.message.usage;
			const prompt = usage.input + usage.cacheRead + usage.cacheWrite;
			cacheHitRate = prompt > 0 ? usage.cacheRead / prompt * 100 : undefined;
		} else if (entry.type === "message" && entry.message.role === "toolResult") usage = entry.message.usage;
		else if (entry.type === "compaction" || entry.type === "branch_summary") usage = entry.usage;
		if (!usage) continue;
		totals.input += usage.input;
		totals.output += usage.output;
		totals.cacheRead += usage.cacheRead;
		totals.cacheWrite += usage.cacheWrite;
		totals.cost += usage.cost.total;
	}
	return { totals, cacheHitRate, context: ctx.getContextUsage() };
}

/**
 * @param {import("@earendil-works/pi-coding-agent").ExtensionContext} ctx
 * @param {ReturnType<typeof sessionStats>} stats
 * @param {import("@earendil-works/pi-coding-agent").Theme} theme
 * @param {boolean} autoCompact
 */
function statsText(ctx, { totals, cacheHitRate, context }, theme, autoCompact) {
	const parts = [];
	if (totals.input) parts.push(`↑${formatTokens(totals.input)}`);
	if (totals.output) parts.push(`↓${formatTokens(totals.output)}`);
	if (totals.cacheRead) parts.push(`R${formatTokens(totals.cacheRead)}`);
	if (totals.cacheWrite) parts.push(`W${formatTokens(totals.cacheWrite)}`);
	if ((totals.cacheRead > 0 || totals.cacheWrite > 0) && cacheHitRate !== undefined) {
		parts.push(`CH${cacheHitRate.toFixed(1)}%`);
	}
	const subscription = ctx.model && (ctx.model.provider === "kimi-coding"
		|| (ctx.modelRegistry.isUsingOAuth(ctx.model)
			&& ctx.modelRegistry.getProvider(ctx.model.provider)?.auth?.oauth?.isSubscription === true));
	if (totals.cost || subscription) parts.push(`$${totals.cost.toFixed(3)}${subscription ? " (sub)" : ""}`);
	const tokens = context?.tokens;
	const window = context?.contextWindow ?? ctx.model?.contextWindow ?? 0;
	const auto = autoCompact ? " (auto)" : "";
	const usageText = `${tokens == null ? "?" : formatTokens(tokens)}/${formatTokens(window)}${auto}`;
	const usageColor = tokens == null ? "dim"
		: tokens >= 150_000 ? "error"
		: tokens >= 80_000 ? orange : "success";
	return parts.map((part) => theme.fg("dim", part)).concat(theme.style(usageText, { fg: usageColor })).join(" ");
}

/** @param {import("@earendil-works/pi-coding-agent").ExtensionAPI} pi */
export default function (pi) {
	/** @type {(() => void) | undefined} */
	let unsubscribe;
	pi.on("session_shutdown", () => {
		unsubscribe?.();
		unsubscribe = undefined;
	});
	pi.on("session_start", (_event, ctx) => {
		if (ctx.mode !== "tui") return;
		ctx.ui.setFooter((tui, theme, footerData) => {
			const release = footerData.onBranchChange(() => tui.requestRender());
			unsubscribe = release;
			/** @type {ReturnType<typeof sessionStats> | undefined} */
			let cached;
			/** @type {string | null | undefined} */
			let leaf;
			/** @type {typeof ctx.model} */
			let model;
			return {
				dispose: release,
				invalidate() { cached = undefined; },
				render(width) {
					// Streaming frames and the timing ticker must not rescan a long session.
					const currentLeaf = ctx.sessionManager.getLeafId();
					if (!cached || leaf !== currentLeaf || model !== ctx.model) {
						cached = sessionStats(ctx);
						leaf = currentLeaf;
						model = ctx.model;
					}
					const autoCompact = pi.getSettings().compaction?.enabled !== false;
					const left = truncateToWidth(statsText(ctx, cached, theme, autoCompact), width, "...");
					let right = ctx.model?.id || "no-model";
					if (ctx.model?.reasoning) {
						const thinking = pi.getThinkingLevel();
						right += ` • ${thinking === "off" ? "thinking off" : thinking}`;
					}
					if (footerData.getAvailableProviderCount() > 1 && ctx.model) {
						const withProvider = `(${ctx.model.provider}) ${right}`;
						if (visibleWidth(left) + 2 + visibleWidth(withProvider) <= width) right = withProvider;
					}
					const space = width - visibleWidth(left) - 2;
					right = space > 0 ? truncateToWidth(right, space, "") : "";
					const gap = " ".repeat(Math.max(0, width - visibleWidth(left) - visibleWidth(right)));
					let cwd = displayCwd(ctx.sessionManager.getCwd());
					const branch = footerData.getGitBranch();
					if (branch) cwd += ` (${branch})`;
					const name = ctx.sessionManager.getSessionName();
					const heading = theme.fg("dim", cwd)
						+ (name ? theme.fg("dim", " • ") + theme.fg("accent", name) : "");
					const lines = [truncateToWidth(heading, width), left + theme.fg("dim", gap + right)];
					const statuses = footerData.getExtensionStatuses();
					if (statuses.size > 0) {
						const text = [...statuses].sort(([a], [b]) => a.localeCompare(b)).map(([, text]) => sanitize(text)).join(" ");
						lines.push(truncateToWidth(text, width, theme.fg("dim", "...")));
					}
					return lines;
				},
			};
		});
	});
}
