# 01 — Verify Pi settings write behavior

Read `.plan/README.md` (shared context) and `AGENTS.md` first.

We are choosing a fix for the tracked `pi-agent/settings.json` problem, so get
the exact behavior, not guesses. Prefer reading Pi source; run experiments only
with a throwaway agent dir.

## Questions

1. Which user actions write which keys? (`Ctrl+S` in `/model`, `/thinking`,
   `/settings`, version change, model catalog update, `/reload`, …)
2. When are writes global (agent dir) vs project (`.pi/settings.json`)? How do
   `markModified` / `markProjectModified` route them?
3. Confirm partial persistence: does `persistScopedSettings` preserve unmodified
   and unknown keys? What are JSON formatting, trailing newline, file mode?
4. Does a symlinked `settings.json` get its target rewritten (not replaced)?
5. What happens at startup with no `defaultProvider`/`defaultModel`? Selection
   order and provider availability checks via `auth.json`.
6. Is `settings.json` created if missing? What happens on invalid JSON?
7. Does `lastChangelogVersion` get added on first run / version change, and does
   anything besides the changelog display depend on it?

## Constraints

- Throwaway dirs only, e.g. `PI_CODING_AGENT_DIR=$(mktemp -d)`; no real auth.
- No code changes in the repo.

## Deliverable

`.plan/results/01-verify-pi-settings-write-behavior.md`: fact table
(action → written keys → scope), each claim with `file:line` or the observed
command/output, plus open unknowns.
