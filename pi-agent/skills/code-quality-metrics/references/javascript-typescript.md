# JavaScript and TypeScript metrics

## Preferred tools

1. Prefer [StrykerJS](https://stryker-mutator.io/docs/stryker-js/introduction/)
   for mutants. Use `@stryker-mutator/core` and a compatible official test-runner
   plugin for the repository's runner when supported. Pin compatible versions
   and check runtime/peer requirements. Tool branding is Stryker; user-facing
   commands should say `test:mutants` in new npm interfaces.
2. Reuse the runner's coverage support (for example Vitest/Jest) when available.
   Use [c8](https://github.com/bcoe/c8) for Node's test runner or compatible
   command-based suites. Emit LCOV plus structured coverage/HTML as supported.
3. Prefer the maintained `@danibram/crap4ts` package from
   [crap4ts](https://github.com/sebassdc/crap4ts) for JS/TS complexity and CRAP.
   Verify the pinned package's source/LCOV inputs and supported syntax. Confirm
   it includes callbacks and anonymous exports used by the project. Do not
   replace a working existing tool just to match this preference.

Known reference versions used together with Node 22+: StrykerJS 10.0.0,
c8 10.1.3, and `@danibram/crap4ts` 0.6.0. Treat these as reference versions, not
latest-release claims. Verify compatibility with the actual runner, Node version,
TypeScript configuration, and package manager before pinning. Keep development
packages and lockfiles repository-owned; avoid global npm installations.

## Integration

1. Inspect package scripts, package manager, workspace packages, runner config,
   aliases, transforms, and source maps. Scope source files across workspace
   packages deliberately; do not assume one root config exercises all packages.
2. Install the runner plugin explicitly. A configuration section alone does not
   install a plugin. Treat unknown-option warnings as a setup problem, not proof
   that an integration is active. Use the command runner only when a compatible
   native plugin is unavailable or unsuitable, and document the performance cost.
3. Pass only resolved source files to Stryker's `mutate` selection. Escape literal
   paths if the tool interprets them as patterns; do not turn an exact-file API
   into accidental glob matching. Keep required tests/config/source dependencies
   available even when only a single source file is mutated.
4. Use per-test coverage analysis where the runner supports it. For command
   runners, verify supported coverage-analysis settings; do not promise test
   selection that the integration cannot provide. Bound concurrency and timeouts.
   Static mutants can rerun large suites; inspect and explain their cost. Do not
   silently ignore them or uncovered mutants just to improve speed or scores.
5. Collect fresh coverage for CRAP, including selected source with no executed
   tests where supported. Retain source maps and normalize LCOV paths to original
   JS/TS sources. Missing coverage mappings are errors/limitations, not silent
   zeroes or permission to drop risky functions. Filter the final function report
   to selected source paths, regardless of how many tests coverage collection runs.
6. Whitelist files required in Stryker sandboxes. Watch for sockets, symlinks,
   large caches, private config, credentials, and sessions in broad copy roots.
   Keep reports and sandbox directories ignored and avoid recursive copying of
   earlier artifacts. Preserve unrelated source and shared user configuration.
7. A Stryker dry run checks instrumentation/baseline, not assertion strength.
   Confirm one small real scoped run before claiming mutation tooling works.
   Expose incremental reports only with clear freshness/cache-invalidation rules.

## Suggested npm interface

These are interface examples to implement, not assumed existing scripts. Use the
repository's actual package manager and document its argument forwarding:

```bash
npm run test:mutants:files -- src/parser.ts
npm run test:mutants:files -- src/parser.ts src/options.ts
npm run test:mutants:pr -- --base <actual-feature-base>
npm run crap:files -- src/parser.ts src/options.ts
npm run crap:pr -- --base <actual-feature-base>
npm run test:mutants:all  # Only when explicitly requested
npm run crap:all
```

An unscoped `test:mutants` or `crap` should explain required scope rather than
launch a full audit. Do not embed an assumed base branch in `:pr` scripts.
Expose a list-only preview. Document report directories, tool versions, exclusions,
and the distinction between source selection and tests actually executed.
