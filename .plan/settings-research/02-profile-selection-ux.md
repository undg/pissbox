# 02 — Profile selection UX

Read `.plan/README.md` (shared context) and `AGENTS.md` first.

Goal: when multiple `settings.<profile>.json` files exist, a workstation must
pick one. Design the selection mechanism for `bin/pissbox` before any
implementation. Read `bin/pissbox`, `tests/`, and the `bash-cli-style` skill
first.

## Questions

1. Precedence: `--profile NAME` > `PISSBOX_PROFILE` env > hostname map > default?
2. Hostname map: explicit `case "$(hostname -s)"` in the launcher, or a tracked
   data file (e.g. `pi-agent/profiles.conf`)? Which is simpler and testable?
3. Missing profile file: hard error, fallback to `settings.json`, or no-op?
   Compare with existing `--pissbox-pi-dir` validation behavior.
4. How does profile selection interact with `--pissbox-pi-dir`? Is the profile
   resolved inside the selected Pi dir?
5. Exact `--help` wording and where the flag fits the existing `case` loop.
6. Which `tests/` additions are needed (stub SBX CLI)? List cases: explicit
   flag, env, hostname fallback, missing profile, option after a workspace, `--`.
7. Must switching profiles work on an already-created sandbox without recreate?
   (Host-side symlink yes, sandbox env var maybe not.)

## Constraints

- Design only; no edits to tracked files.
- Keep it simple — two workstations, two profiles.

## Deliverable

`.plan/results/02-profile-selection-ux.md`: CLI contract, precedence, error
semantics, diff sketch, test list.
