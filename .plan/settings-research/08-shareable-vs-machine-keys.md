# 08 — Shareable vs machine-specific settings keys

Read `.plan/README.md` (shared context) and `AGENTS.md` first.

Before splitting `pi-agent/settings.json`, classify every key: shared /
machine-specific / runtime state. The uncertain one is `enabledModels` — it
looks shareable, but references provider-specific model IDs.

## Questions

1. For each key in the current `pi-agent/settings.json`: which class? Evidence
   from `docs/settings.md` and Pi behavior.
2. `enabledModels`: if it lists `github-copilot/*` while only OpenCode is
   authenticated, does Pi ignore unavailable entries or misbehave? Test with a
   throwaway agent dir. Can a superset of both providers be the shared list?
3. `modelThinkingLevels` (keyed by `provider/modelId`): harmless superset?
4. `defaultThinkingLevel`: what happens on a model without `high`?
5. `npmCommand`, `extensions`, `packages`, terminal keys: any machine dependency
   (mise/node availability in the image)?
6. List everything else Pi writes into the agent dir that is machine state:
   `lastChangelogVersion`, `models-store.json`, anything else.

## Constraints

- Throwaway agent dirs for experiments; no real auth.
- Design only; no edits to tracked files.

## Deliverable

`.plan/results/08-shareable-vs-machine-keys.md`: table
`key | class | evidence | notes`, plus the minimal machine-specific key set.
