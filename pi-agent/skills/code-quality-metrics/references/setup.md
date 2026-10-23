# Set up metrics tooling

Read this reference only when adding or changing tooling. Follow the main skill's
scope contract and read the relevant language reference before implementation.
Keep commands, configuration, pinned dependencies, and operational documentation
in the repository rather than creating a duplicate local skill.

## Commands and dependencies

1. Prefer the repository's build interface. For new npm scripts use
   `test:mutants`, `test:mutants:files`, `test:mutants:pr`, and `test:mutants:all`;
   use `crap`, `crap:files`, `crap:pr`, and `crap:all` for CRAP. In Make-based
   projects prefer `test-mutants` and `crap` with `FILES`/`BASE` and explicit
   `-all` targets. Preserve existing names rather than gratuitously renaming.
   Document exact quoting, precedence, and base arguments in help and repo docs.
2. Share the small scope resolver between mutants and CRAP where practical.
   Parse Git filenames using NUL-delimited output, not whitespace or lines. Pass
   paths as literal arguments; account for Git pathspec magic as well as tool
   globs. Resolve symlinks before enforcing repository boundaries. Fail closed
   on unresolved merge conflicts, missing merge bases, or Git command failures.
   Show eligible and excluded files with reasons; do not silently drop changes
   in unsupported languages. Do not build a universal multi-language framework.
3. Pin compatible development-tool versions and lockfiles. Check current tool
   documentation, runtime requirements, and flags before implementing. Keep
   installation explicit and separate from ordinary tests/checks. Do not add
   runtime dependencies or global installs merely to run development metrics.

## Execution and isolation

1. Collect fresh coverage before calculating CRAP. Restrict CRAP output to the
   selected implementation files even when coverage requires the full suite.
   Preserve function paths/lines, complexity, coverage, and CRAP in reports.
2. Run baseline tests before mutants. Use the native test-runner integration
   where compatible. Bound workers and timeouts conservatively; enable safe
   incremental/caching features only with documented invalidation. A dry run
   checks baseline/discovery, not mutation effectiveness. Do not launch full
   mutation audits as setup smoke tests or ordinary check dependencies.
3. Keep sandbox copies minimal: exclude credentials, sessions, private settings,
   and unrelated workspaces. Avoid bulk-copying dependency trees; make required
   dependencies available using the tool's supported resolution/install mechanism.
   Copies must reflect the selected current working-tree contents, including
   authorized dirty and untracked source, not just HEAD. Preserve required tests,
   fixtures, and configuration. Verify the unmutated sandbox baseline. Isolate test
   processes/state; never contact real user daemons or mutate the original tree
   with a homegrown runner. Avoid concurrent writers to shared reports.

## Reports and Git ignores

1. Save reports under the repository root using this layout:
   - `.reports/crap/yyyy-mm-dd_hh-mm_commitsha.*`
   - `.reports/mutation/yyyy-mm-dd_hh-mm_commitsha.*`
   - `.reports/coverage/yyyy-mm-dd_hh-mm_commitsha.*` for standalone coverage.
2. Use one run stem across JSON, HTML, and metadata; use the actual run time and
   short HEAD SHA. For example: `.reports/crap/2026-10-22_21-13_a1b2c3d.json`.
   HTML assets may live in a sibling directory with that stem. Append a language
   or collision suffix after the SHA when necessary; never overwrite a different
   run silently. Document the timezone.
3. Add `/.reports/` to the repository-root `.gitignore` unless an existing rule
   already ignores it. If repository instructions prohibit ignore-rule changes,
   ask rather than silently omitting this step. Keep tool databases, sandbox
   copies, and temporary coverage inputs separate from final reports.
4. Record scope, base, HEAD, dirty-tree status, tool versions, and run outcome
   alongside reports; a commit SHA alone does not identify uncommitted input.
   Never present old artifacts after a skipped, failed, interrupted, or dry run
   as fresh results. Preserve established layouts during assessment; apply this
   layout during new setup or an explicitly requested migration.

## Verify and document

1. Test scope resolution: one/multiple files, spaces, file precedence, branch
   commits plus dirty/untracked changes, renames/deletions, test-only changes,
   empty selection, invalid refs/files, literal glob/pathspec characters, escaping
   symlinks, Git failures, exclusions, and explicit full mode. Use
   isolated fixture repositories and stubs for expensive tool invocations. Then
   run one small real integration per supported tool when available. Distinguish
   stub verification from real tool execution.
2. Document install commands, supported source, scoped/full examples, report
   locations, limitations, and expected cost. Do not introduce hard score gates
   without agreement. Setup does not authorize commits, publication, deleting
   old skills, or changes to unrelated configuration.
