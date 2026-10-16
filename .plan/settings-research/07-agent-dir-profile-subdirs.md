# 07 — Agent-dir profile subdirs via PI_CODING_AGENT_DIR

Read `.plan/README.md` (shared context) and `AGENTS.md` first.

Idea 5: profile dirs inside the mounted `pi-agent/` (e.g.
`pi-agent/profiles/opencode-go/`) containing symlinks to shared content plus
their own `settings.json`; the launcher sets `PI_CODING_AGENT_DIR` and/or the
kit links `~/.pi/agent` to the profile dir. Flesh out and compare with Idea 1
(`03`). Read `link-pi-agent.sh`, `AGENTS.md`.

## Questions

1. Exact entry list a profile dir must expose: settings, auth, sessions,
   models-store, AGENTS.md, APPEND_SYSTEM.md, extensions, skills, prompts,
   themes, npm, git, keybindings.json, mcp.json, models.json. Which exist today?
2. Do relative symlinks (`../auth.json`) inside the mount resolve in the
   sandbox? Does `~/.pi/agent` pointing at the profile dir break
   `link-pi-agent.sh`'s refusal checks?
3. Maintenance: adding a shared file later means symlinking it into every
   profile. Any way to avoid the farm (overlay mounts, mount shared dir too)?
4. Setting `PI_CODING_AGENT_DIR` contradicts repo AGENTS.md ("Leave
   PI_CODING_AGENT_DIR unset"). What breaks if it stays unset and only the
   `~/.pi/agent` symlink points at the profile dir?
5. Where do `auth.json` / `sessions/` live so profiles share them on one
   workstation? Is sharing auth across profiles desirable?
6. Compare with Idea 1 on: moving parts, failure modes, fresh clone, existing
   sandboxes, git churn.

## Constraints

- Design only; no edits to tracked files.

## Deliverable

`.plan/results/07-agent-dir-profile-subdirs.md`: layout tree, symlink list,
verdict versus Idea 1.
