# Shared Markdown vaults

Vaults are shared across every pissbox agent and session. The host bridge owns
access; this feature never mounts vault directories into sandboxes. Markdown is
the storage format. Obsidian is optional and used only to open an existing note
on the host desktop. There is no MCP server, network listener, or dependency on
Obsidian plugins.

## Host configuration

Add named vault tables to the same host global configuration used for secrets:
`${XDG_CONFIG_HOME:-$HOME/.config}/pissbox/config.toml`, or `PISSBOX_CONFIG`.
Do not put vault paths in `.pissbox.toml` workspaces.

```toml
[vault.personal]
path = "~/Documents/Notes"
# git = true                    # Default; records changes if this is a Git repo.

[vault.work]
path = "/absolute/host/path/to/work-notes"
git = false
```

Notes open in Obsidian using their absolute host path; no separate Obsidian
vault name is needed. The former `obsidian` configuration key is no longer
accepted; remove it from existing vault tables.

Directories must already exist and refer to distinct physical directories.
Paths must be absolute or start with `~/`. The launcher validates configuration
as data; it does not access the notes or run their commands. The bridge reads its
configuration once on startup, using its host environment. Because it is a
per-user daemon, the first launcher that starts it selects `PISSBOX_CONFIG`;
subsequent sessions do not switch vault configurations. Prefer the normal
per-user configuration for notes shared across all projects.

Rebuild the host launcher and bridge after upgrading, then **explicitly restart
the bridge yourself** and reattach through the launcher. Reload Pi to discover
new tools and the skill. No image rebuild or new vault mount is needed. Changing
vault configuration also requires an explicit bridge restart. A missing default
configuration leaves vaults unconfigured; invalid configuration or unavailable
vault directories disable vault operations without disabling agent messaging.
The old `~/.config/pi/vault.json` is neither read nor changed.

## Usage

The shared `vault-manager` skill teaches the agent the note workflow:

- “Find my notes about database migrations.”
- “Save these findings into the work vault.”
- “Append this thought to today's daily note.”
- “Commit those changes” / “This note is done.”
- “Open that note in Obsidian.”

Use `/skill:vault-manager` to explicitly load the workflow. Before the first
note operation, the skill reads the vault's root `AGENTS.md` through `vault_read`
and follows its folder, filename, tag, todo, plan and archive conventions. A
missing file is normal and needs no action. Only that root file is treated as
conventions; it never overrides the user's request, project instructions, or
safety rules.

Available tools:

| Tool | Purpose |
| --- | --- |
| `vault_list` | Discover vault names; list notes (`kind: notes`, default), folders (`folders`), or both (`all`); optional `*`, `**`, `?` path glob |
| `vault_search` | Literal or RE2 text search, folder/tag/date filters, optional backlinks |
| `vault_read` | Read Markdown and its revision, with byte-based pagination |
| `vault_new` | Exclusively create a kebab-case note with standard frontmatter |
| `vault_save` | Replace an existing note's complete Markdown, revision required |
| `vault_edit` | Replace exactly one matching text fragment, revision required |
| `vault_append` | Append exact text, revision required |
| `vault_move` | Move or rename one note in one vault; creates parents, refuses overwrite, reports pre-move backlinks |
| `vault_tag` | Change only frontmatter tags, revision required |
| `vault_tags` | List frontmatter/inline tags and per-note usage counts |
| `vault_backlinks` | Find incoming wikilinks, including heading/alias forms |
| `vault_daily` | Read/create `Daily/YYYY-MM-DD.md` using the host's local date, tagged `daily` and `journal` |
| `vault_open` | Open an existing note using the host's Obsidian URI handler |

Search/list/tags cover all configured vaults by default. Browse skips directories
named `_archive` and the root `AGENTS.md`; explicit paths still read and write
both, and naming a folder inside `_archive` browses it. `vault_list` returns
notes by default; `kind: folders` lists directories and `kind: all` lists both.
The scan root is never listed, so an empty folder shows its top-level
directories. Folders carry only vault, path and kind; the JSON key stays
`notes`. `folder` and `pattern` narrow both kinds; tag and filename-date filters
apply to notes only, and pagination counts both kinds. Single-note operations
require a vault name if multiple vaults exist; they never guess the first vault.
Filename date filters are inclusive ISO dates, not file modification dates.
Tag filters match nested tags. Tags count each tag once per note across the
selected vaults. Backlinks are wikilinks, not a full Obsidian link resolver;
bare basenames can be ambiguous.

`vault_move` moves or renames a note within one vault; cross-vault and folder
moves are not supported, and there is still no delete tool. Destination parents
are created; an existing destination is refused. Publication uses an exclusive
hard link before removing the old name, so a filesystem without hard-link
support fails before the source is removed and there is no copy fallback. A
crash between link and removal can leave the note under both names: a visible
duplicate, not data loss. Links and frontmatter are never rewritten. Before the
move, the host scans the entire vault (excluding `_archive/` sources) for
wikilinks to the old path and returns up to 20 source paths. A
`backlinksComplete: false` result carries a warning and means the list may be
partial; a missing link is meaningful only when it is true. A rename breaks
bare `[[name]]` wikilinks; moving between folders keeps them valid, while
path-form links break in both cases. The result includes the destination
revision unless it could not be re-read; reread before editing after such a
warning. Moves require no revision. Destination names need not be kebab-case,
so existing legacy notes can be renamed.

No delete, shell, arbitrary-path, or commit tool is exposed.

## Persistence and history

Every successful mutation is persisted immediately, not held until session end
or a separate “commit.” Writes are atomically published; creating an existing
file fails. A shared service serializes all sessions' operations. Editing an
existing note requires its SHA-256 revision so one agent cannot silently replace
another agent's newer version. On conflict, reread and reconcile. An external
editor does not participate in this lock: revision checks detect changes since
the last read, but cannot provide a filesystem transaction with Obsidian or any
other external writer. Avoid simultaneous edits to the same note in Obsidian.

History is an internal host detail. With `git = true` (default), an existing Git
repository rooted exactly at the vault directory records each changed note with
`vault: <relative/path.md>` and each move with
`vault: move <oldPath> -> <newPath>`. A move commits the new path and, when Git
tracked it, the old path as a deletion; an untracked old path is not staged.
New repositories are not initialized automatically; parent repositories are
never used. Unrelated staged and unstaged files are not committed. Hooks and
commit signing are disabled for these automatic commits; there are no pushes or
other remote operations. Git author identity must already be configured on the
host. A failed history operation returns the successful result (saved or moved
note) and a warning: the note **has been saved or moved**. Do not retry the
operation. Failures may leave paths staged; fix the host repository explicitly.
A later save of the same content can record outstanding history without
changing it.

A bridge request ID is deduplicated for the lifetime of that agent address.
Cancellation/timeout does not prove that a write or move was canceled; it may
already have completed. Read or list to establish the outcome before another
mutation. Never retry automatically. Bridge restart loses in-memory
deduplication state.

## Boundaries and limits

Only configured vaults and non-hidden `.md` files are accessible. Absolute paths,
parent traversal, symlinks, special files, and hidden directories such as `.git`
and `.obsidian` are refused or skipped; moves apply the same rules to both
paths. Go's `os.Root` also confines filesystem
operations against symlink races. Tools return logical vault names and relative
paths, not host directory locations. This boundary cannot hide a vault that the
user separately mounts as a workspace; do not mount it if host-only access is
required. Notes are reference data, not agent instructions, except the root
`AGENTS.md`, which supplies vault conventions only.

A note may be up to 2 MiB of UTF-8. Reads return up to 12,000 bytes per page;
follow `nextOffset`. Creation/full replacement/individual edit fragments are
limited to 16 KiB, so use targeted edits for larger existing notes. Browse results
are paged (default 30, maximum 50); scans are bounded to 10,000 directory entries
per vault/folder and eight seconds including queue time. Narrow the folder on
scan limits. The existing bridge record limit is 32 KiB including JSON encoding;
unusually long paths or heavily escaped text can require smaller pages/fragments.
Limits and IO errors are explicit, not disguised as empty search results.

The host needs Git only for enabled history in existing repositories. Obsidian
opening needs the registered `obsidian://` handler plus macOS `open` or Linux
`xdg-open`. The installer does not install these optional applications. No new
sandbox executable or host CLI parser is required; YAML parsing is a pinned Go
library dependency.

## Implementation

`host/internal/vault` is an independent domain package with no Pi, SBX, mailbox,
or configuration-file dependencies. It handles boundaries, note operations,
frontmatter, scanning, history, and the injectable desktop opener.
`host/config` only parses/validates trusted configuration.
`host/internal/bridge/vault.go` adapts configuration and dispatches operations
through one bounded worker, leaving messaging and heartbeat processing responsive.
`pi-agent/extensions/lib/vault-tools.js` declares tools over the existing
session-bound mailbox transport. The host control socket is never exposed.
