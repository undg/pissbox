# Machine-specific Pi settings — investigation plan

Parallel research prompts for `pissbox`. Each numbered file is a self-contained
prompt for one fresh agent session. Agents write findings to
`.plan/results/<same-name>.md`.

## Problem

`pi-agent/settings.json` is git-tracked and mixes three kinds of state:

- shared preferences (`doubleEscapeAction`, `autocompleteMaxVisible`, extensions, …)
- machine-specific defaults (`defaultProvider`, `defaultModel`, `enabledModels`)
- runtime state (`lastChangelogVersion`)

Workstations differ: one uses GitHub Copilot, another OpenCode Go, and Pi
versions can differ. Git sync then fights over the same file.

## Ground truth already established

- Pi writes into its own `settings.json`: `lastChangelogVersion` on version
  change; model/thinking defaults on Ctrl+S in `/model` and `/thinking`; `/settings`.
- Pi writes only modified fields and preserves the rest
  (`dist/core/settings-manager.js`, `persistScopedSettings`).
- Writes follow symlinks.
- Pi has no settings profile/include support. Knobs: `PI_CODING_AGENT_DIR`
  (whole agent dir), project `.pi/settings.json` (per project, trust-gated).
  No `--settings` flag.
- Entrypoint is bare `pi`; `sbx run` positional args are workspaces, so the
  launcher cannot pass Pi CLI flags.
- `auth.json` and `models-store.json` are already local/untracked per workstation.
- Extensions load *after* settings (extension paths come from settings), can read
  settings (`pi.getSettings()`) but cannot set/override them, and there is no
  settings write hook. `model_select` / `thinking_level_select` are notifications.
- Extensions can set the session model/thinking (`pi.setModel()`,
  `pi.setThinkingLevel()`); this does not persist defaults.
- Virtual models registered during extension load are flushed in
  `createAgentSessionServices` before the startup default is resolved, so a
  virtual model can be the tracked default and route per workstation.

## Reference

- Repo rules: `AGENTS.md`.
- Pi 1.3.0 docs:
  `/usr/local/share/npm-global/lib/node_modules/@earendil-works/pi-coding-agent/docs/`
  (`settings.md`, `configuration.md`, `models.md`, `sessions.md`).
- Pi source: same package, `dist/core/settings-manager.js`.
- Launcher: `bin/pissbox`. Kit: `sbx-kit/` (setup scripts in
  `sbx-kit/files/home/.pissbox/scripts/`).

## Prompt files

| File | Topic |
|---|---|
| `01-verify-pi-settings-write-behavior.md` | Verify exact write/persistence behavior |
| `02-profile-selection-ux.md` | `--profile` / env / hostname selection design |
| `03-profile-symlink-approach.md` | Idea 1: tracked profiles + symlink |
| `04-base-local-merge.md` | Idea 2: base + local overlay merge |
| `05-minimal-machine-defaults.md` | Idea 3: track no machine defaults |
| `06-per-workstation-agent-dirs.md` | Idea 4: separate agent dirs |
| `07-agent-dir-profile-subdirs.md` | Idea 5: `PI_CODING_AGENT_DIR` profile subdirs |
| `08-shareable-vs-machine-keys.md` | Classify every settings key |
| `09-tracked-local-and-version-noise.md` | Tracked vs local; `lastChangelogVersion` |
| `10-synthesize.md` | Read all results, recommend final design |
| `11-extension-approach.md` | Idea 6: extension instead of file layering |

## Rules for every agent

- Read `AGENTS.md` and this README first.
- Research/design only; do not change tracked code or config.
- Do not edit other agents' files; write only your own result file.
- Never read, print, or copy `pi-agent/auth.json` or any credential.
- Scratch files: `~/.config/pi/tmp/`.
- All agents share one working tree — no code edits, no commits.
