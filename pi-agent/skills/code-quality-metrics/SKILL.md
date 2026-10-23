---
name: code-quality-metrics
description: Set up or use coverage, mutation testing (test mutants), complexity, and CRAP metrics in a repository. Use when adding metrics tooling, checking feature changes or specific files, investigating surviving mutants, or assessing test strength and risky functions.
---

# Code quality metrics

Keep the workflow global; keep executable tooling, pinned dependencies, commands,
and repository-specific documentation in the repository. Do not create another
local skill just to repeat this workflow.

## Choose the mode

1. Inspect repository instructions, working-tree changes, build commands, test
   runners, existing metrics configuration, and tooling documentation. Preserve
   unrelated work. Use existing tools rather than installing a competing stack.
2. Distinguish **setup** from **assessment**. Setup may add scripts, dependencies,
   configuration, documentation, and regression tests when requested. Assessment
   uses existing commands; it does not silently install or reconfigure tooling.
   If setup is missing, explain what is needed before changing it.
3. For setup or tooling changes, read [setup instructions](references/setup.md)
   and the relevant language reference. For tool-specific troubleshooting, read
   only the relevant language reference; ordinary assessment needs neither:
   - [Go](references/go.md): Gremlins, go-crap, native coverage.
   - [JavaScript/TypeScript](references/javascript-typescript.md): StrykerJS,
     crap4ts, runner-native coverage or c8.
   - [Python](references/python.md): mutmut or Cosmic Ray, coverage.py, Radon.
   For other languages, inspect supported tools and explain the choice first;
   do not invent an equivalent or force these tools onto unsupported source.

## Scope contract — both mutants and CRAP

1. Support **one exact source file**, **multiple exact source files**, and
   **feature changes against an explicit base ref** for both kinds of metrics.
   Add an explicit full-project mode; do not make an unscoped mutant run full.
   New CRAP interfaces should also require a scope or explicit all mode.
2. Explicit files override branch selection. Treat paths as data, not shell
   fragments. Support spaces without `eval`; reject missing, unsupported, or
   out-of-repository explicit paths. Do not interpret exact paths as globs.
3. Branch selection uses `git merge-base HEAD <base>` and the current working
   tree, including feature commits, staged/unstaged changes, and nonignored
   untracked files. Include added files and rename destinations. Exclude deleted
   files, tests, generated source, vendored dependencies, and build artifacts.
   Do not guess a base branch or fetch without need; ask if the base is unknown.
4. Fail for an invalid base or invalid explicit selection. A valid branch scope
   with no eligible implementation files prints a clear skip and succeeds.
   **Never broaden an empty selection to the whole repository.** Test-only
   changes need explicitly selected implementation files; do not guess them.
5. Show the resolved source selection before expensive work. Provide a list-only
   preview that needs no mutation tools. Source scoping restricts mutated files
   or reported CRAP functions, not automatically the test suite. Run enough tests
   to obtain valid coverage and detect affected behavior; document any reduction.

## Assess a change

1. Read the repository's tooling guide/help. Run relevant ordinary checks first.
   Choose explicit feature files or the actual feature base, then preview scope.
2. Collect fresh scoped coverage/CRAP. Run mutants on the smallest useful source
   selection. Explain that one file can still be expensive when its tests are
   slow; get agreement before expanding to a substantially costlier audit.
3. Inspect survivors and high-risk functions. Assessment alone is read-only:
   report concrete test gaps and simplification opportunities. Change tests or
   production code only when the user's task authorizes fixes. Never change
   intended behavior, blanket-exclude mutants, or add brittle score-only assertions.
4. After authorized fixes, rerun affected checks. Verify report freshness, scope,
   and outcome. Skipped, failed, interrupted, and dry runs may leave old artifacts.
   A dry run is not a mutation score. Do not treat unrelated failures as kills.
5. Avoid concurrent report writers. Keep test processes/state isolated; never
   contact real user daemons. Preserve existing report layouts during assessment.
   This skill does not authorize commits, publication, deleting old skills, or
   changes to unrelated configuration.

## Interpret and report

- CRAP = complexity² × (1 − coverage_fraction)³ + complexity. Around 30 is an
  investigation signal, not a universal gate. Fully covered code still has its
  complexity score. Note whether a tool uses line, statement, or branch coverage;
  do not compare language/tool conventions as interchangeable measurements.
- Coverage measures execution, not assertion quality. Report mutant killed,
  survived, uncovered, timeout, invalid/build-failed, and skipped counts as
  available. Separate tool-reported scores from your interpretation; timeouts
  and unrelated failures are not evidence of good assertions. Investigate
  equivalent mutants without pretending equivalence can always be proven.
- Report commands, selected files/base, test results, report paths, limitations,
  and concrete risky functions/survivors with source paths and lines. Mark
  partial runs as partial and unavailable measurements as unavailable. Never
  claim an unexecuted install, baseline, mutant run, or score was verified.
