# Parallel review — missing pieces

## Agreement

The user wants the Claude-style reviewer with real parallel minions. A sequential
multi-pass review is not an acceptable substitute. Parallelism is intended to
reduce review latency; no speedup has been measured yet.

Planning and evidence storage only. Do not install a runner, launch agents,
change settings, or implement a bridge in this task.

## Findings

- No subagent tool or tmux runner is available in the current session.
- The supplied source mount
  `/Users/bartek_laskowski/.claude/skills/review-pr` is readable, but contains
  only an empty `.claude/.cc-writes/` directory tree. No skill instructions,
  prompts, scripts, or symlinks were found.
- The actual Claude reviewer workflow has therefore not been inspected. Its
  reviewer roles, orchestration, dependencies, and output format remain unknown.
- A skill can describe review behavior, but cannot by itself provide a working
  parallel agent execution mechanism.
- GitHub access also needs a bridge investigation. See [plan-github.md](plan-github.md).
  Local diff review and GitHub PR integration should be distinguished; whether
  the original workflow requires GitHub is not yet known.

## Missing pieces / future investigation

- [ ] Obtain the actual Claude skill and every referenced reviewer prompt/script
  through explicitly authorized, accessible paths.
- [ ] Inventory its reviewer roles and required tools; distinguish essential
  behavior from Claude-specific plumbing.
- [ ] Investigate a real parallel agent runner compatible with sandbox Pi.
  Candidate mechanisms remain undecided; tmux is not assumed mandatory.
- [ ] Determine child-agent model/auth access, context handoff, read-only review
  boundaries, concurrency limits, cancellation, timeouts, and failure reporting.
- [ ] Define a shared immutable review target so all minions inspect the same
  diff, with independent contexts and no concurrent working-tree edits.
- [ ] Define how findings are collected, verified, deduplicated, and attributed;
  do not treat agreement between agents as proof of correctness.
- [ ] Resolve GitHub bridge requirements if the workflow needs PR metadata,
  comments, CI, or posting review results.
- [ ] Agree on a small implementation and test parallel execution, failed-child
  handling, output quality, and actual wall-clock time before claiming speedup.

## Decisions still open

Runner/package, number of minions, model selection, reviewer roles, UI, result
storage, permissions, and GitHub posting policy. No technical approach selected.
