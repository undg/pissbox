# 05 — Minimal: track no machine defaults

Read `.plan/README.md` (shared context) and `AGENTS.md` first.

Idea 3: remove `defaultProvider`, `defaultModel`, `enabledModels`,
`defaultThinkingLevel`, `lastChangelogVersion` from tracked settings; rely on
Pi auto-selection from authenticated providers. Evaluate viability and cost
from docs/source/experiments.

## Questions

1. With `defaultProvider`/`defaultModel` unset, how does Pi pick the startup
   model? Deterministic order? Does it prefer the last used in session only?
2. On a workstation with only GitHub Copilot auth, does it pick a Copilot
   model with no user action? With two authenticated providers, which wins?
3. Is shared `defaultThinkingLevel: "high"` safe on both providers? What if a
   model does not support `high`?
4. Ctrl+S still writes `defaultModel` into `settings.json` → pollution. Compare:
   gitignoring `settings.json` and tracking a `settings.default.json` seed, or
   accepting a dirty tracked file.
5. What is actually lost: per-workstation default model, `enabledModels` cycle.
   Can `/scoped-models` cover cycling per session?
6. Does dropping `enabledModels` change `/model` picker behavior/cost?

## Constraints

- Throwaway agent dirs for experiments; no real auth.
- Design only; no edits to tracked files.

## Deliverable

`.plan/results/05-minimal-machine-defaults.md`: findings, viability verdict,
what is lost, recommended minimal layout if viable.
