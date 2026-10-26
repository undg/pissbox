# Quality checks

Run `make` or `make help` from the checkout root. The Makefile wraps the existing
build scripts; it does not build images, install tools, mutate source, or contact
SBX as part of `make check`.

## Setup and ordinary checks

```bash
make tools       # Explicit network install; never runs as a test dependency
make check       # Go -race, all JS suites, Python tests, vet, Bash syntax, diff check
make metrics     # Fresh coverage plus complexity/CRAP reports for Go and extensions
```

Ordinary bridge development requires Go 1.25+. Building the pinned quality
tools requires **Go 1.26.2+** (or Go's enabled automatic toolchain download).
JavaScript quality tooling requires **Node 22+ and npm**. The session-name suite
also requires Pi installed on PATH with its loader dependencies; it exercises Pi's
actual session metadata using the installed package. No Pi login is needed.

JavaScript suites are colocated with their source as `*.test.mjs` under
`pi-agent/extensions/`; shared helpers and their tests stay under `tests/`.
Ordinary tests, coverage, and mutation baselines use Node's recursive
`pi-agent/extensions/**/*.test.mjs` and `tests/**/*.test.mjs` discovery, not
filename lists or suite-to-suite imports. Pi only auto-loads `.js` and `.ts`
extensions, so the `.test.mjs` suites do not run when Pi starts.

`make tools` installs Gremlins v0.6.0 and go-crap v0.5.1 into
`.cache/quality-tools/bin/<os>-<arch>/`, and npm tools into
`.cache/quality-tools/js/`. npm versions are locked in the adjacent package files:
StrykerJS 10.0.0, c8 10.1.3, and `@danibram/crap4ts` 0.6.0. The `qs` override pins
a patched transitive dependency of Stryker's dashboard client. Install scripts
are disabled. These are development tools, not bridge runtime dependencies.
No global npm installation, image changes, or ignore-rule changes are needed.

## Mutation testing a feature

```bash
make mutation BASE=origin/main
make mutation-go BASE=origin/main
make mutation-js FILES="pi-agent/extensions/pissbox-bridge.js"
make mutation-go FILES="host/internal/bridge/mailbox.go host/internal/bridge/herdr.go"
make mutation-go FILES="host/internal/bridge/mailbox.go" DRY_RUN=1
make mutation-all  # Deliberate, potentially expensive full scan
```

`FILES` is a shell-quoted list of exact checkout-relative filenames, **not globs**.
It overrides `BASE`. To preserve a space in a filename, quote it inside the value:
`FILES="'host/internal/bridge/my file.go' host/internal/bridge/mailbox.go"`. The wrapper parses this as
data, without `eval`. Direct invocation accepts individual path arguments:

```bash
python3 tools/quality/run.py mutation-go -- 'host/internal/bridge/my file.go'
python3 tools/quality/run.py mutation-js --base origin/main --list
```

With `BASE`, selection compares the working tree with `git merge-base HEAD BASE`.
This includes feature commits, staged and unstaged changes, and untracked,
nonignored files. It does not fetch refs. Added files and rename destinations
are included. Deleted files, tests, generated-code-marked files, vendor,
node_modules, and Go testdata are excluded. Supported implementation scope is
`host/**/*.go` (including nested `cmd/` and `internal/` packages) and
`pi-agent/extensions/**/*.js`.

A bare mutation target refuses to run without a scope. Invalid refs, missing
explicit files, or unsupported explicit files fail. A valid scope with no
matching files prints a no-op message and succeeds, **never widening to a full
scan**. Test-only changes need explicit implementation `FILES` to audit the
assertions they improve. `--list` previews selection without requiring tools.

Selection restricts **mutated files**, not tests. Gremlins runs the complete Go
suite (`--integration`); Stryker's built-in command runner runs all extension
suites with coverage analysis off. Each tool uses one worker. Do not run two
copies of the same language's target concurrently: report paths are shared.

`DRY_RUN=1` runs baseline checks and discovers mutants without executing mutation
tests. Stryker executes the instrumented baseline in its sandbox. Gremlins
collects coverage in the original module and lists candidates; this alone does
**not** prove that copied-workdir tests pass. Dry runs are not mutation scores.

### Go workdir and subprocess caveats

Gremlins uses copies of the module. Test fixtures must stay rooted at the real
checkout's `.cache/test-fixtures/`, not beneath those deep copies: Unix socket
paths otherwise exceed their length limit, causing **every mutation to appear
killed by unrelated test failures**. The wrapper sets
`PISSBOX_BRIDGE_TEST_FIXTURE_DIR` for this reason, preserving an explicit override.
For deeply nested worktrees, set it to a shorter absolute directory beneath a
checkout's `.cache/test-fixtures/`; Go/JS tests, real-daemon launcher tests, and
mutation adapters honor it.
Tests still create unique fixture directories, and their daemons use isolated
runtime directories. Launcher process tests also need the shared project-schema
validator outside the copied module. The wrapper sets
`PISSBOX_LAUNCHER_TEST_RESOURCE_DIR` to the real checkout, preserving an explicit
override; this variable is consumed only by tests, not the production launcher.
The wrapper disables the Go test-result cache with `GOFLAGS=-count=1` (preserving
other GOFLAGS).

Coverage and mutation eligibility depend on what Go's coverage instrumentation
records, including subprocess execution. `NOT COVERED` is not proof that a daemon
path is unused. A suspicious score or a new test layout needs an unmutated-copy
check before trusting the results. Investigate timeouts and broken builds rather
than treating them as useful kills. Interrupted mutant runs can leave isolated
test daemons; never stop the user's real singleton to clean those up.

## Coverage and CRAP

```bash
make coverage             # Coverage only
make crap-go BASE=origin/main
make crap-js FILES="pi-agent/extensions/session-name.js"
make metrics              # Both languages, all supported source files by default
```

CRAP targets accept the same optional `BASE`/`FILES` selection. They always collect
fresh coverage using the full corresponding test suite, then report only selected
source files. Go uses go-crap with its existing-profile option. JS uses c8's LCOV
and crap4ts's TypeScript AST analysis, including anonymous exports and callbacks.
Their complexity and coverage conventions differ; compare functions within a
language rather than treating cross-language scores as interchangeable.

The standard Change Risk Anti-Patterns formula is:

```text
CRAP = complexity² × (1 − coverage_fraction)³ + complexity
```

Use 30 as an investigation signal, not a gate. High scores suggest missing tests,
complex branching, or both. At 100% coverage, CRAP still equals complexity.
Coverage only shows execution, not assertion quality; inspect surviving mutants.
No mutation-score or CRAP threshold fails builds initially. Tool, baseline-test,
and ordinary test failures do fail commands.

## Reports

| Report | Location |
| --- | --- |
| Go coverage and HTML | `.cache/coverage/go/coverage.out`, `index.html` |
| JS coverage, LCOV and HTML | `.cache/coverage/js/coverage-final.json`, `lcov.info`, `index.html` |
| Go CRAP JSON | `.cache/crap/go/report.json` |
| JS CRAP JSON | `.cache/crap/js/report.json` |
| Go mutations | `.cache/mutation/go/report.json` |
| JS mutations and HTML | `.cache/mutation/js/report.json`, `index.html` |

Reports describe the latest successful run's scope, not necessarily the whole
project. No-op, failed, and JS dry runs can leave previous reports untouched;
check command output and timestamps before quoting results. Stryker keeps
sandbox copies under `.cache/mutation/js/sandboxes/`, with extension source,
the lifecycle suites and their helpers, and Go bridge source/module files
whitelisted. The messaging suite builds an isolated daemon in each sandbox;
its runtime fixtures use the real checkout's `.cache/test-fixtures/` to avoid
Unix socket path limits. Shared auth, sessions, settings, host binaries, and
caches are never copied. Generated reports, fixtures, tools, and sandboxes stay ignored.

`make validate-kit` is separate because SBX validation requires Docker sign-in.
`make fmt` and `make tidy` are explicit source/modfile-changing operations.
