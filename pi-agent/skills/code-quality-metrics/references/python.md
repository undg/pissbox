# Python metrics

## Preferred tools

1. Use [coverage.py](https://coverage.readthedocs.io/) for fresh coverage, JSON,
   and HTML. Reuse pytest-cov if the project already uses pytest; do not replace
   unittest merely to collect coverage.
2. Prefer [mutmut](https://mutmut.readthedocs.io/) for pytest-based projects when
   its current Python/platform requirements and mutation scope fit the repository.
   Verify the pinned version's exact-file selection, test discovery, subprocess
   behavior, and report/export support; interfaces differ across major versions.
3. Consider [Cosmic Ray](https://cosmic-ray.readthedocs.io/) when a configurable
   test command is needed, including unittest-based suites. Validate it with the
   repository's actual test command before choosing it. Do not assume mutmut
   supports arbitrary unittest commands or force a test-runner migration.
4. Use [Radon](https://radon.readthedocs.io/) for cyclomatic complexity. Combine
   function-level complexity with coverage.py's executed/missing executable lines
   in a small repository-owned CRAP adapter. Do not label Radon's maintainability
   index or complexity grade as CRAP. Python has no preferred turnkey CRAP tool
   in this skill; assess an existing repository tool before adding an adapter.

Check current upstream documentation before setup. Pin compatible development
versions using the repository's existing dependency manager/lockfile. Do not
invent version pins, install globally, or add these tools to runtime dependencies.

## Coverage and CRAP

1. Inspect package roots, import paths, supported Python versions, and test runner.
   Exclude tests, generated modules, virtual environments, vendored code, and build
   outputs from implementation selection. Apply the main skill's same single-file,
   multiple-file, and explicit-base branch scope contract to both metric types.
2. Run the ordinary baseline, then collect coverage using the actual runner.
   For unittest, a typical shape is
   `python -m coverage run --source=<package> -m unittest discover -s tests`;
   for pytest, reuse its configured pytest-cov integration. Confirm selected but
   unimported modules are included as unexecuted source rather than disappearing.
3. Produce coverage JSON/HTML with coverage.py. Normalize paths before joining
   coverage with Radon's function records. Calculate coverage from executable
   lines within each function, not covered physical lines divided by its source
   span. This is line-based coverage, not AST statement or branch coverage; label
   it accordingly. Respect exclusions and ignore comments/blank lines.
4. Define function boundaries explicitly: decorators, nested functions, methods,
   closures, and async functions. Use AST structure and compatible Radon records;
   do not double-count nested function bodies in their enclosing function.
   Unsupported or ambiguous mappings must be reported, not silently dropped.
   Multiple callables on one line cannot be distinguished by line coverage alone;
   do not claim separate execution evidence for each.
5. Compute `complexity² × (1 − coverage_fraction)³ + complexity` per function.
   Record complexity, executable/covered counts, coverage basis, path, line, and
   CRAP. A function with no measurable executable statements is unavailable,
   not automatically 100% covered. Report only selected implementation files.
6. Test the adapter against fully covered, partially covered, and uncovered
   functions plus nested/async functions, decorators, exclusions, and missing
   mappings. Verify a known calculation independently of the adapter. Do not
   claim exact comparability with Go or JS/TS tools using other conventions.

## Mutants

1. Mutate only resolved implementation files; retain the required tests and source
   dependencies. Verify literal paths are not accidentally interpreted as globs.
   For tools lacking direct exact-file selection, implement supported configuration
   or an isolated snapshot with a selected mutation scope. Retain required source
   dependencies/tests and current dirty/untracked content; a HEAD-only Git worktree
   is insufficient for dirty-tree assessment. Never mutate unrelated files and
   merely filter the final report. If safe scoping is unavailable, report the blocker.
2. Run unmutated tests in the tool's execution environment. Python imports can pick
   up an installed original package instead of mutated source: verify import roots,
   editable installs, subprocess environments, and working-directory behavior.
3. Bound workers/timeouts. Test selection or stack-depth optimizations can miss
   indirect callers; document and verify their effect before enabling them.
   Treat timeouts, import failures, and invalid mutants separately from assertion
   kills. Check a real scoped run, not only mutant discovery.
4. Preserve native tool state needed to inspect/resume mutants separately from
   final artifacts. Export structured outcomes when supported; if an adapter is
   necessary, document its schema and limitations rather than inventing counts.
   Save final JSON/HTML and run metadata using [the setup guide](setup.md)'s
   `.reports/mutation/` layout; save CRAP under `.reports/crap/`. Keep these
   outputs ignored.

## Interface

Use the project's existing Make/task/package interface, with help and list-only
preview. A Make interface can expose `test-mutants FILES="src/a.py src/b.py"`,
`test-mutants BASE=<actual-feature-base>`, and matching `crap` commands. Require
explicit full-run targets. Do not run a complete Python mutation campaign merely
to verify installation.
