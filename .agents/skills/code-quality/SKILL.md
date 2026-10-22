---
name: code-quality
description: Check this repository's code quality after feature work or during review using Make targets, scoped Go/extension mutation testing, coverage, complexity, and CRAP risk metrics. Use when asked to verify a change, assess test strength, or inspect quality metrics.
---

# Check code quality

Read [the tooling guide](../../../tools/quality/README.md) before first use or
when changing tooling. Run commands from the checkout root. Start with `make help`.

## Workflow

1. Inspect `git status --short`. Preserve unrelated work. Select the feature's
   actual base ref; never assume a branch name or fetch without need.
2. Run `make check`. This covers Go race tests, both JS extension suites, Python
   regression tests, Go vet, Bash syntax, and whitespace. Fix relevant failures
   before interpreting metrics. `make check` never installs quality tools.
3. If dependencies are missing, use explicit `make tools` when installation is
   authorized. It needs network, Node 22+/npm, and Go 1.26.2+ or automatic Go
   toolchain downloads. The session-name tests need installed Pi on PATH.
4. Collect fresh risk metrics: `make metrics BASE=<feature-base>`, or narrow
   with `FILES`. `make metrics` without scope measures all supported Go and
   extension source. It does not run mutation testing.
5. For changed implementation, run scoped mutation testing:

   ```bash
   make mutation BASE=origin/main
   make mutation-go FILES="go-bridge/mailbox.go"
   make mutation-js FILES="pi-agent/extensions/pissbox-bridge.js"
   ```

   Replace the example base with the feature's real base. `FILES` overrides
   `BASE`; it is a quoted list of exact paths, not globs. Both language targets
   still run their full respective test suites. For test-only changes, name the
   implementation explicitly with `FILES`.
6. Preview a large selection with
   `python3 tools/quality/run.py mutation-go --base <ref> --list` (or
   `mutation-js`). `DRY_RUN=1` discovers mutants and checks baseline tests, but
   does not produce a trustworthy mutation score. Use `make mutation-all` only
   for an explicitly requested full audit. Never silently widen an empty scope.
7. Inspect survivors and high-risk functions. Add behavior assertions for real
   gaps; simplify branching where justified. Do not alter production behavior,
   suppress mutations, or add brittle assertions just to improve a score.
8. Rerun affected checks after changes. Run `make validate-kit` only when kit
   files are affected; report Docker sign-in failures as unverified validation.

## Interpret results honestly

- CRAP = complexity² × (1 − coverage_fraction)³ + complexity. Around 30 warrants
  investigation, not automatic failure. Fully covered complexity still matters.
- Coverage measures execution, not assertion strength. Use mutation survivors
  to find missing boundaries and behavior checks. Go/JS metric conventions
  differ; do not rank them as if their numbers were directly interchangeable.
- Separate missing tests, equivalent mutants, timeouts, compile failures, and
  tool limitations. Never count timeout/build failures as strong assertion kills.
- Go's subprocess coverage may omit execution. Gremlins skips uncovered mutants;
  do not call such paths tested just because the covered score is high.
- Gremlins copied workdirs need `PISSBOX_BRIDGE_TEST_FIXTURE_DIR` set by the
  wrapper, keeping socket fixtures under the actual checkout. If all mutants
  suddenly die, verify an **unmutated copy** passes before trusting the score.
- Do not run concurrent copies of the same mutation target. Never contact or
  stop the real bridge daemon while diagnosing test leftovers.
- Do not use `make fmt` or `make tidy` as read-only checks; they modify files.
  No commits, shared-state resets, credential access, or unrelated fixes are
  authorized by this skill.

## Report

State commands run, selected files/base, test failures, and metric limitations.
Quote mutation killed/survived/uncovered/timeout counts, not only percentages.
Identify relevant high-CRAP functions by path and line, with concrete findings.
Reports live under `.cache/coverage/{go,js}/`, `.cache/crap/{go,js}/`, and
`.cache/mutation/{go,js}/`. No-op/failed/dry runs may leave old reports: verify
freshness and scope. Never present a partial feature scan as a whole-project score.
