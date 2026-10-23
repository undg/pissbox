# Make target audit

## Goal and authorization

Run meaningful Make targets sequentially inside pissbox, investigate failures,
fix verified issues, and commit completed changes before moving on. The user
approved execution, dependency installation inside the sandbox, and commits.
Do not push, rebuild images, recreate sandboxes, or touch shared credentials/state.

## Evidence and decisions

- Initial working tree clean; starting commit `78f9494`.
- Environment: Linux arm64, Go 1.26.8, Node 22.22.1; Pi, SBX and Docker on PATH.
- Host-reported issue: session-name tests assume Pi's entry point is next to the
  executable; Homebrew's layout violates that assumption. Investigate with an
  isolated fixture; do not claim a real macOS test from Linux.
- Run leaf targets once rather than rerun aggregate aliases unnecessarily.
- Full metrics audit is intentional. Start mutation checks with dry runs and
  choose bounded useful scopes after inspecting cost and findings. Do not run
  an unrestricted, potentially multi-day mutation-all just for target coverage.
- `fmt` and `tidy` modify tracked files: inspect their changes and retain only
  justified changes. Kit validation may be blocked by Docker authentication.
- Reports remain ignored under `.cache/`; record outcomes here, not generated data.

## Tasks

- [x] Ordinary checks: `test-go`, `test-js`, `test-python`, `vet`, `syntax`,
  whitespace check. Fix failures and the reproducible Homebrew discovery issue.
- [x] Build: `build`, `build-all`; inspect format/module hygiene with `fmt`, `tidy`.
- [ ] Kit: read affected kit configuration and attempt `validate-kit`; distinguish
  environment/authentication blockers from repository defects.
  **Blocked:** SBX reports `not signed in to Docker`; no login attempted.
- [!] Tools and metrics: `tools`, `coverage-go`, `coverage-js`, `crap-go`, `crap-js`;
  inspect fresh reports for concrete risks and fix verified problems.
- [ ] Mutation: preview explicit scopes; run bounded Go and JS dry runs and real
  mutations where practical; investigate meaningful survivors and baseline errors.
- [ ] Final ordinary aggregate `check`, review git status, record limitations and
  commits. No claim of real image build, host execution or sandbox launch.

## Results

- `make help`: passed; inspected all target recipes.
- `make test-go`: passed (race detector, no result cache).
- `make test-js`: passed, 17 tests on Linux.
- `make test-python`: initially 2 failures + 1 error, all caused by the missing
  npm shared-cache mapping. Added the declarative mapping and corrected docs;
  rerun passed, 113 tests, one real-Stryker test skipped until tool installation.
  No existing sandbox data was migrated or changed.
- Homebrew discovery: resolve the package under the formula's `libexec` when
  `pi` is a wrapper, and load dependencies relative to the package, not the
  wrapper. Isolated npm-symlink and Homebrew-layout fixtures pass; `make test-js`
  passes 18 tests. This is not a real macOS/Homebrew execution.
- `make vet`, `make syntax`, `git diff --check`: passed.
- `make build`, `make build-all`: passed. Built darwin/arm64, darwin/amd64,
  linux/arm64 and linux/amd64 binaries; cross-builds were not executed.
- `make fmt`, `make tidy`: passed, no tracked changes.
- `make validate-kit`: blocked by Docker sign-in, not a kit validation result.
