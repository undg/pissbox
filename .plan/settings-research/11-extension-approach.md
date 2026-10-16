# 11 — Extension approach (no fork)

Read `.plan/README.md` (shared context) and `AGENTS.md` first.

Idea 6: solve the machine-specific settings problem with a Pi extension instead
of file layering. Tracked settings keep a stable default; an extension reads a
per-workstation target and applies it. No fork, no host JSON tooling.

Preliminary findings to verify and build on:

- Settings load before extensions (extension paths come from settings), so an
  extension cannot rewrite `settings.json` before it is read on the same start.
- Extension API can read settings (`pi.getSettings()`) but not set or override
  them; no settings write hook. `model_select` and `thinking_level_select` are
  notification-only.
- Extensions can set the *session* model and thinking level (`pi.setModel()`,
  `pi.setThinkingLevel()`); `pi.setModel()` does not persist defaults.
- Virtual model registrations queued during extension load are flushed in
  `createAgentSessionServices` (`dist/core/agent-session-services.js`) **before**
  `main.js` resolves the startup default model. So a virtual model can be the
  startup default. Verify this with a real run.
- Docs: `docs/extensions.md`, `docs/virtual-models.md`,
  `examples/extensions/jev-router.ts`, `dist/core/extensions/types.d.ts`.

## Mission

Design and validate the extension approach end to end. Decide between:

- **A. Virtual model**: tracked settings contain `defaultProvider: "pissbox"`,
  `defaultModel: "auto"`, `enabledModels: ["pissbox/auto"]`. A tracked extension
  registers `pissbox/auto`; `route()` maps it to the workstation's physical
  provider/model from an untracked local file.
- **B. Session override**: tracked settings omit provider/model (or leave a
  harmless default); on `session_start` the extension reads the local file and
  calls `pi.setModel()` / `pi.setThinkingLevel()`.

## Questions

1. Verify at source level and, if possible, in a run: is a virtual model
   registered during extension load available when `main.js` resolves
   `defaultProvider`/`defaultModel`? What happens if it is not found?
2. For A: exact `route()` logic for `user` / `continuation` / `retry`
   (`request.previous`, `request.failed`) so prompt caches and retries behave.
   Thinking-level mapping and clamping. Error behavior when the target provider
   has no credentials.
3. Where does the per-workstation target live?
   - untracked `pi-agent/model-profile.json` read next to the extension via
     `import.meta.url` (extension lives in the mounted agent dir)
   - `PISSBOX_PROFILE`/`PISSBOX_MODEL` env passed by `bin/pissbox`
   - `$PI_CODING_AGENT_DIR` fallback
   Which is simplest and survives existing sandboxes?
4. For B: what does `session_start` (`reason: "startup" | "new" | "resume"`)
   do to a resumed session's recorded model? Should `resume` be skipped?
5. Interaction with `/model`, `enabledModels` scoping, `/scoped-models`,
   `defaultThinkingLevel`, and Ctrl+S: does saving the virtual `pissbox/auto`
   stay stable, and what happens if the user saves a physical model?
6. After A or B, does any tracked settings churn remain besides
   `lastChangelogVersion`? List every remaining write path.
7. Failure modes: extension load error (note: `main.js` exits on extension load
   errors, and prints the load-failure hint), invalid target file, unknown
   model, missing auth, `/reload`, model catalog refresh, registration conflicts.
8. Test plan without real credentials: fake provider via `registerProvider`
   with a stub `streamSimple`, throwaway `PI_CODING_AGENT_DIR`, or SDK test.
9. Cost/benefit versus `03` (symlink profiles) and `04` (base+local merge):
   lines of code, maintenance risk across Pi upgrades, launcher changes.
10. Twin extension risk: if Pi later adds native settings profiles, how much of
    this extension can be deleted?

## Constraints

- Do not fork or patch Pi internals. Extension API and documented hooks only.
- No code changes to tracked files; design and sketches only. Scratch in
  `~/.config/pi/tmp/`.
- Never read or print `pi-agent/auth.json` or other credentials.

## Deliverable

`.plan/results/11-extension-approach.md`: recommended variant (A or B), code
sketch, verified facts with `file:line` or observed commands, failure modes,
test plan, and verdict versus file layering.
