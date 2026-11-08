const text = (description, extra = {}) => ({ type: "string", description, ...extra });
const vault = text("Configured vault name. Required for single-note operations when several vaults exist; search/list/tags otherwise cover all vaults.", { maxLength: 80 });
const notePath = text("Markdown path relative to the vault root; never a host path.", { maxLength: 1024 });
const revision = text("Opaque revision returned by the latest read or write. On conflict, reread and reconcile; never overwrite blindly.", { pattern: "^[0-9a-f]{64}$" });
const content = text("Markdown text (up to 16 KiB UTF-8).", { maxLength: 16384 });
const page = {
	offset: { type: "integer", minimum: 0, description: "Result offset for pagination (default 0)." },
	limit: { type: "integer", minimum: 1, maximum: 50, description: "Maximum results (default 30)." },
};
const filter = {
	vault,
	folder: text("Restrict to a vault-relative folder."),
	tag: text("Tag filter; includes nested tags such as topic/subtopic."),
	dateFrom: text("Inclusive YYYY-MM-DD filename date."),
	dateTo: text("Inclusive YYYY-MM-DD filename date."),
	...page,
};
const definitions = [
	["list", "List shared vault names, Markdown notes, folders, or both. The scan root is not listed, so an empty folder lists top-level directories. Skips `_archive/` and root `AGENTS.md`; name a folder inside `_archive` to inspect archived notes. No host paths are exposed.", {
		...filter,
		kind: { type: "string", enum: ["notes", "folders", "all"], description: "Entry kinds to list (default notes). Tag and date filters apply to notes only." },
		pattern: text("Optional vault-relative path glob using *, ** and ? (for example **/*.md). Matches notes and folders alike.", { maxLength: 1024 }),
	}, []],
	["search", "Find notes by case-insensitive literal text or an explicit regular expression. Returns paths, line numbers and snippets. Follow nextOffset or narrow the folder when truncated. Skips `_archive/` and root `AGENTS.md`; name a folder inside `_archive` to inspect archived notes.", {
		...filter, query: text("Text to find, or a Go/RE2 pattern when regex is true.", { maxLength: 1024 }),
		regex: { type: "boolean", description: "Use a case-sensitive RE2 regular expression instead of literal text." },
		backlinks: { type: "boolean", description: "Also include notes with wikilinks to matching notes within the same vault and filters." },
	}, ["query"]],
	["read", "Read a shared Markdown note and its revision. Large notes are paged by UTF-8 byte offset; follow nextOffset until absent before replacing the whole note.", {
		vault, path: notePath,
		offset: { type: "integer", minimum: 0, description: "UTF-8 byte offset returned as nextOffset; default 0." },
		limit: { type: "integer", minimum: 1, maximum: 12000, description: "Maximum UTF-8 bytes (default 12000)." },
	}, ["path"]],
	["new", "Create a new shared note with id, aliases and tags frontmatter. New filenames must be kebab-case.md. Fails if the note exists.", {
		vault, path: notePath, content,
		tags: { type: "array", items: text("Frontmatter tag, without #."), maxItems: 100 },
		aliases: { type: "array", items: text("Note alias."), maxItems: 100 },
	}, ["path", "content"]],
	["save", "Persist the complete Markdown of an existing shared note, including frontmatter. Requires its current revision. For notes larger than 16 KiB use targeted vault_edit instead.", { vault, path: notePath, revision, content }, ["path", "revision", "content"]],
	["edit", "Persist a targeted edit to a shared note. oldText must occur exactly once; every other byte is preserved. Requires the current revision.", { vault, path: notePath, revision, oldText: content, newText: content }, ["path", "revision", "oldText", "newText"]],
	["append", "Persist text appended exactly to a shared note. Include any desired separating newlines. Requires its current revision.", { vault, path: notePath, revision, content }, ["path", "revision", "content"]],
	["tag", "Add or remove one frontmatter tag, preserving other fields and the body. Does not change inline #tags. Requires the current revision.", { vault, path: notePath, revision, tag: text("Tag without #."), action: { type: "string", enum: ["add", "remove"] } }, ["path", "revision", "tag", "action"]],
	["tags", "List frontmatter and inline tags with counts of notes containing each tag. Skips `_archive/` and root `AGENTS.md`; name a folder inside `_archive` to include archived notes. Supports pagination and the same scope filters as search.", filter, []],
	["backlinks", "Find notes in this vault that wikilink to the target note (including aliases and headings). Bare-name wikilinks may be ambiguous when filenames repeat. Archived notes and root `AGENTS.md` are not scanned as sources.", { vault, path: notePath, ...page }, ["path"]],
	["daily", "Read or create today's Daily/YYYY-MM-DD.md with standard frontmatter, using the host's local date. Returns its revision; large existing daily notes may require vault_read pagination.", { vault }, []],
	["open", "Open an existing shared note in Obsidian on the user's host desktop. Only when the user asks to open/show a note.", { vault, path: notePath }, ["path"]],
];

/** Thin domain adapter; request is the existing bridge's session-bound transport. */
export function registerVaultTools(pi, request) {
	for (const [op, description, properties, required] of definitions) {
		pi.registerTool({
			name: `vault_${op}`, label: `Vault ${op}`, description,
			namespace: { name: "vault", aliases: ["obsidian"], description: "Shared host-only Markdown notes. Read the vault-manager skill for note conventions." },
			parameters: { type: "object", properties, required, additionalProperties: false },
			outputSchema: { type: "object", additionalProperties: true },
			async execute(_id, params, signal) {
				const response = await request("vault", { vaultOp: op, params }, signal);
				const result = response.structuredContent.result;
				if (!result || typeof result !== "object") throw new Error("Invalid vault response; host bridge may need rebuilding and restarting");
				return { content: [{ type: "text", text: JSON.stringify(result) }], details: result, structuredContent: result };
			},
		});
	}
}
