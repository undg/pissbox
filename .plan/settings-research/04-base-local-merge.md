# 04 — Base + local overlay merge

Read `.plan/README.md` (shared context) and `AGENTS.md` first.

Idea 2: `settings.base.json` tracked (stable prefs), `settings.local.json`
machine-local untracked (provider/model/enabledModels/runtime), `settings.json`
generated and gitignored. Design the merge. Read Pi's own `deepMergeSettings`
in `dist/core/settings-manager.js` so the generated file behaves like Pi
expects.

## Questions

1. Merge semantics: deep object merge, array replace vs concat, null handling,
   `defaultTools` special case? Match Pi's `deepMergeSettings`.
2. Where can the merge run?
   - host launcher (`bin/pissbox`): which JSON tool is guaranteed on the user's
     macOS/Linux hosts (jq, node, python3)? Behavior if missing?
   - kit setup script (`sbx-kit/files/home/.pissbox/scripts/`): node is
     guaranteed in the Pi image, but it runs only at sandbox creation.
   - `sbx exec` before attach: works for existing sandboxes but adds a failure
     path/latency.
   Pick one and justify.
3. Pi's own writes into generated `settings.json` (e.g. Ctrl+S `defaultModel`)
   are lost on the next regeneration. Is that acceptable, or should generated
   state fold back into `settings.local.json`? Propose exact rule.
4. Fresh clone with no `settings.local.json`: seed from what? Fail loud?
5. Invalid JSON / missing local file: fail, warn, or write base only?
6. Any `.gitignore` negation pitfalls for a generated `pi-agent/settings.json`?
7. Can merge be avoided (e.g. JSON include/concatenation)? Verify and reject.

## Constraints

- Design only; no edits to tracked files.
- Prefer the least machinery that actually works on a fresh host.

## Deliverable

`.plan/results/04-base-local-merge.md`: merge location + semantics, text
sequence, failure modes, diff sketch, trade-offs.
