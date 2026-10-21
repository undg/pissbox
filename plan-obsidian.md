# Obsidian access bridge — deferred

## Agreement

Defer the Obsidian skill until an access bridge is designed. Record requirements
now; do not mount vaults, read notes, install integrations, or change other
harnesses. MCP is a possible mechanism, not a decision.

## Findings

Source inspected:
`/Users/bartek_laskowski/.config/opencode/skill/obsidian/SKILL.md`.

- The source assumes direct filesystem access to `~/Documents/Work` and
  `~/Documents/Personal`. These are source assumptions, not authorized paths
  for this session. Neither vault was inspected.
- Sandbox `$HOME` is `/home/agent`; those relative-to-home paths do not locate
  the user's host vaults.
- The current workspace list contains neither vault. No vault integration tool
  is exposed in this session.
- The source detects vaults by counting Markdown files, selects a sole active
  vault silently, and asks when multiple vaults are active.
- Its work-note conventions include YAML `id`, `aliases`, and `tags`, lowercase
  hyphenated tags, and ticket filenames of
  `{ticketNumber} ({type}) {Title}.md` under `tickets/`.
- Those conventions need confirmation before reuse. They are not evidence that
  a particular vault is currently accessible or should be selected silently.

## Missing pieces / future investigation

- [ ] Agree which vaults and operations are in scope: search/read versus
  create/update. Obtain explicit access authorization.
- [ ] Choose between explicit filesystem mounts and an Obsidian/API/MCP bridge;
  no host service, plugin, or endpoint is assumed available.
- [ ] If considering MCP, read installed Pi MCP documentation before designing
  integration. Determine host connectivity, authentication, and secret handling.
- [ ] Define explicit vault selection, path boundaries, and symlink handling so
  a request cannot escape the authorized vault.
- [ ] Verify search behavior, frontmatter preservation, ticket conventions,
  conflict detection, and concurrent edits before enabling writes.
- [ ] Adapt the skill to the selected access tools and authorized vault mapping,
  without hard-coded host-home assumptions or broad vault auto-discovery.
- [ ] Test an authorized read-only workflow first; test writes only on an
  explicitly approved fixture or note.

## Relationship to GitHub

See [plan-github.md](plan-github.md). Both need access integration, but there is
no evidence yet that they should share a server or custom bridge implementation.
