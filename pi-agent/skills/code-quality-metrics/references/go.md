# Go metrics

## Preferred tools

1. Use native `go test` coverage and `go tool cover` for coverage artifacts.
2. Prefer [Gremlins](https://github.com/go-gremlins/gremlins) for mutation testing.
   Pin a released version; verify its Go toolchain requirements separately from
   the application's minimum Go version. Do not silently upgrade the module's
   Go version to install development tooling.
3. Prefer [go-crap](https://github.com/padiazg/go-crap) for function complexity and
   CRAP using an existing fresh Go coverage profile. Verify the pinned version's
   profile-input, selection, and JSON-output flags before writing commands.
   If native file scoping is insufficient, filter its structured function output
   by canonical selected paths rather than relying on substring matching.

Known reference versions used together in a Go/JS repository: Gremlins v0.6.0
and go-crap v0.5.1. These are starting points, not a claim of latest releases or
compatibility with every repository. Those tools required Go 1.26.2+ in that
setup; inspect current module/tool requirements before pinning or downloading.

## Integration

1. Identify module/workspace roots and build tags. For multiple modules, partition
   selected source files by module; do not assume `go test ./...` from the Git
   root covers them all. Exclude `_test.go`, generated source, vendor, testdata,
   and unsupported files explicitly.
2. Run the appropriate unmutated suite. Collect fresh coverage with a command
   shaped like `go test -count=1 -coverprofile=<report> ./...` from the module root;
   preserve project-specific tags, environment, and integration-test requirements.
   Use `go tool cover` for summaries/HTML. Add `-race` according to project checks
   and runtime budget, not as an unexplained mutation multiplier.
3. Configure Gremlins to mutate only selected implementation files while retaining
   enough tests. Verify the pinned version's integration mode for cross-package
   tests. Disable Go test-result caching during mutant runs, preserving existing
   `GOFLAGS`. Do not confuse Go build caching with test-result caching.
4. Gremlins can skip uncovered mutations. Report uncovered/skipped code separately;
   a high covered-mutant score does not establish whole-feature test strength.
5. CRAP runs may cover the entire relevant module and then filter function reports
   to selected files. State this cost explicitly. Normalize absolute/module-relative
   paths before filtering. Do not assign zero coverage silently on path mismatch.

## Copied workdirs and subprocesses

- Check how tests find fixture roots, binaries, and subprocess configuration in
  Gremlins' copied workdirs. Keep sockets and process fixtures in short, isolated,
  repository-owned test directories, not deeply nested copies or real user state.
- A broken copy can make every mutant appear killed. Verify an **unmutated copy**
  passes when introducing integration tests or diagnosing suspicious scores.
  Baseline success only in the original tree is insufficient for that failure.
- Native test coverage may miss helper binaries and subprocess execution. Inspect
  actual profiles; use supported Go integration-coverage mechanisms when needed.
  Do not assume uncovered daemon paths were never exercised.
- Bound worker count, clean up only test-owned processes, and document platform,
  socket, or build-tag limitations. No implicit full mutation run to verify setup.

## Suggested Make interface

These are interface examples to implement, not assumed existing commands:

```bash
make help
make tools
make test-mutants FILES="internal/parser.go"
make test-mutants FILES="internal/parser.go internal/options.go"
make test-mutants BASE=<actual-feature-base>
make crap FILES="internal/parser.go internal/options.go"
make crap BASE=<actual-feature-base>
make test-mutants-all  # Only when explicitly requested
make crap-all
```

Provide list-only scope preview. Test both commands against the main skill's
scope contract before relying on them for feature review.
