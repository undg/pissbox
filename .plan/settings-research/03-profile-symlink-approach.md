# 03 — Profile symlink approach (full tracked profiles)

Read `.plan/README.md` (shared context) and `AGENTS.md` first.

Idea 1: track `pi-agent/settings.opencode-go.json` and
`pi-agent/settings.copilot.json` (full files, duplication accepted);
`pi-agent/settings.json` becomes a gitignored symlink to the active profile;
the launcher relinks it before `sbx run`. Flesh this out into an implementable
design and compare with the copy variant. Read `bin/pissbox`,
`sbx-kit/files/home/.pissbox/scripts/link-pi-agent.sh`, `.gitignore` first.

## Questions

1. Symlink or copy? Pi writes through a symlink into the tracked profile. How
   much churn (`lastChangelogVersion`, Ctrl+S) and is it acceptable? Or should
   profiles be machine-local untracked instead (see `09`)?
2. Who creates the link: host `bin/pissbox` before `sbx run`, or the sandbox
   setup script? Host approach works for existing sandboxes; setup runs only at
   creation. Compare.
3. Fresh clone: `settings.json` does not exist. What happens on first run?
4. Does the existing-sandbox attach path in `bin/pissbox` apply the symlink
   change? Can the profile switch while the sandbox is stopped/running?
5. `.gitignore`: add `pi-agent/settings.json`; anything else?
6. Duplicate stable prefs in both profiles: sync strategy, or accept drift and
   use `04` later?
7. Failure modes: dangling symlink, profile deleted, launcher run from another
   checkout, `ln -sfn` behavior on macOS vs Linux.
8. Does `link-pi-agent.sh` interact badly with a symlinked `settings.json`
   inside the mount (`[ -e ]` checks)?

## Constraints

- Design only; no edits to tracked files.

## Deliverable

`.plan/results/03-profile-symlink-approach.md`: layout, sequence, failure
modes, minimal diff sketch, verdict.
