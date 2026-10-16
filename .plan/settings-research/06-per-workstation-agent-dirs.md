# 06 — Per-workstation agent dirs

Read `.plan/README.md` (shared context) and `AGENTS.md` first.

Idea 4: use the existing `--pissbox-pi-dir` per workstation (one full agent dir
per machine). Evaluate cost/benefit against Ideas 1–3. Read `bin/pissbox`,
`AGENTS.md`, `PLAN.md`.

## Questions

1. What gets separated per dir: `auth.json`, `sessions/`, `models-store.json`,
   extensions, skills, packages, `AGENTS.md`? Which sharing is intentional in
   this repo?
2. Can shared state stay shared via symlinks *inside* the mounted dir (e.g.
   `sessions -> ../../shared/sessions`)? Repo AGENTS.md warns that a host
   symlink does not expose its target to the sandbox. Determine exactly when
   symlinks inside a mounted dir resolve.
3. Would both dirs be git-tracked? Duplication cost?
4. Does `--pissbox-pi-dir` allow two dirs for the same workspaces (multiple
   sandbox names)? Any SBX conflict? (SBX picks names unless `--name`.)
5. Complexity score versus Ideas 1–3; explicit verdict.

## Constraints

- Design only; no edits to tracked files.

## Deliverable

`.plan/results/06-per-workstation-agent-dirs.md`: layout, sharing matrix,
mount/symlink constraints, verdict.
