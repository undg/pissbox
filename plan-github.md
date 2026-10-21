# GitHub tool bridge — deferred

## Agreement

A GitHub bridge is needed before porting the existing GitHub skill. MCP is a
candidate, not a selected architecture. Store findings now; investigate and
implement later. Do not change settings, install packages, configure credentials,
or contact GitHub as part of this planning task.

## Findings

Source inspected:
`/Users/bartek_laskowski/.config/opencode/skill/github/SKILL.md`.

The source is primarily a routing policy: prefer dedicated `github_pr_*` tools
and fall back to `gh`, using `gh api` only as a last resort. It names:

| Tool | Expected capability |
| --- | --- |
| `github_pr_get_number` | Resolve the current branch's PR |
| `github_pr_comments` | Read inline and review-level PR comments |
| `github_pr_diff` | Read the PR diff, optionally for one file |
| `github_pr_ci` | Read CI status and failing job logs |

Those tools are not exposed in this session. Their implementations and precise
schemas have not been inspected. Do not assume they are an MCP server or that
an existing GitHub MCP server provides identical behavior.

Installed Pi settings documentation lists a built-in MCP extension; this is a
possible integration point, not evidence of a configured or working GitHub
connection. Repository guidance says the sandbox proxy handles HTTPS GitHub Git
authentication; that does not establish API/MCP authentication compatibility.
Do not inspect or print credential stores to investigate this.

## Missing pieces / future investigation

- [ ] Locate the relevant source tool implementations using explicitly
  authorized paths; document schemas, pagination, comment coverage, and CI logs.
- [ ] Establish what `gh` can already provide and what the bridge must add.
- [ ] Read installed Pi MCP documentation and relevant implementation examples
  before selecting a transport or package.
- [ ] Compare an existing MCP integration with a small direct tool adapter.
- [ ] Establish sandbox network reachability and safe authentication without
  copying host credentials or altering shared auth state implicitly.
- [ ] Separate read-only tools from writes; define explicit authorization for
  creating comments, submitting reviews, or changing PR state.
- [ ] Test branch-to-PR resolution, inline/review comments, paginated results,
  file-filtered diffs, failed CI logs, and useful error reporting.
- [ ] Adapt the skill to tools actually exposed, then verify discovery and a
  real read-only workflow before claiming integration works.

## Related work

[Parallel review](plan-review.md) may consume these tools. The
[Obsidian access plan](plan-obsidian.md) is a separate integration problem;
there is no agreed shared bridge architecture.
