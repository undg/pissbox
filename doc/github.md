# Read-only GitHub tools

The host bridge exposes read-only PR and Actions tools through the existing Pi
extension. GitHub CLI (`gh`) must be available on the **host daemon's PATH**,
with existing GitHub.com authentication. Normal host `gh` token/environment/keychain behavior
applies. The bridge never logs in, prints tokens, mounts credentials into the
sandbox, or configures Git authentication. The installer does not check or
install this optional dependency.

Repository access relies on host `gh` authentication. There is no separate
repository allowlist; restrict the host token if needed. These tools can read
any repository that authentication permits, independently of mounted workspaces.
They do not grant writes even if the token permits them.

## Tools

Every call requires an explicit `repo` such as `acme/project`, not a URL, hostname,
or local directory. PR tools other than listing require an integer `number`;
Actions tools use `runId` or `jobId` instead. Only GitHub.com is supported.

| Tool | Inputs beyond repo/number | Result |
| --- | --- | --- |
| `github_pr_list` | `state`, `limit`, `cursor`; no number | PR summaries, authors, branches, review decision, CI rollup, latest activity |
| `github_pr_view` | `offset` | Description, author, branches, review/merge status, CI summary, requested reviewers |
| `github_pr_diff` | `offset` | Unified diff chunk, `totalBytes`, optional `nextOffset` |
| `github_pr_comments` | `kind`, `page`, `limit` | One discussion category with authors, links, timestamps and available line/reply IDs |
| `github_pr_checks` | `limit`, `cursor` | Current `headRefOid`, aggregate CI state, checks and legacy commit statuses with links |
| `github_run_view` | `runId`, `page`, `limit`; no number | Run metadata and one page of jobs with step status/conclusions |
| `github_job_logs` | `jobId`, `offset`; no number | Plain-text job log chunk, `totalBytes`, optional `nextOffset` |

Listing defaults to open PRs, ordered by most recently updated. `state` also
accepts `closed` (closed without merging), `merged`, or `all` for historical
research. List summaries deliberately do not fetch commenter identities or
individual check details for each PR. CI summaries appear under
`commits.nodes[0].commit.statusCheckRollup.state`; a null rollup means no reported
checks, not a successful build.

Comments default to `kind: "conversation"` (issue-style PR comments).
`"reviews"` reads submitted review summaries, including approval/change-request
states; `"inline"` reads code review comments and replies. Each category has
independent pagination. REST comment fields retain GitHub names such as
`html_url`, `in_reply_to_id`, and `pull_request_review_id`; the author is
`user.login`. These results do not report thread resolution. Fetch categories
only when needed, rather than issuing all three reads for every listed PR.

Checks are attached to the PR's **current head**, including when inspecting a
closed PR. They are not workflow history, all historical commits, or workflow
logs. `CheckRun` nodes include check/workflow links when available;
`StatusContext` nodes represent legacy/external commit statuses. Aggregate
`FAILURE`, pending checks, and an empty check list are successful *reads*, not
tool errors. `state: "NONE"` means no reported checks.

### Diagnosing an Actions failure

Give the agent an Actions URL, for example
`https://github.com/acme/project/actions/runs/6000000000/job/7000000000`,
and ask it to diagnose the failure. The agent can extract `repo: "acme/project"`,
`runId: 6000000000`, and `jobId: 7000000000`; URLs are not passed as tool inputs.
PR check links can provide the same IDs. A check-run ID is not a job ID.

Use `github_run_view` to inspect the run's commit, attempt, status/conclusion,
and jobs with step names and conclusions. Then call `github_job_logs` with the
failed job's `id`. A job URL also permits reading its logs directly. Historical
runs are supported; there is no run-listing or workflow-rerun operation. Run IDs
are not the sequential run numbers shown in workflow titles. IDs must be positive
JavaScript-safe integers (at most 9,007,199,254,740,991).

Run view pins its jobs read to the observed `run.run_attempt`. Compare that value
between pages and restart pagination if it changes. Logs are the full selected
job's plain text, not just failed steps. There is no workflow ZIP download,
archive extraction, or persistent host log cache. Unavailable (including not-yet
available), expired, or inaccessible logs produce an explicit request failure,
not a successful empty log. GitHub must permit Actions reads for the repository; PR access alone may not
be sufficient. Logs can contain sensitive data not masked by GitHub; request only
jobs needed for the diagnosis.

## Pagination and bounds

1. List/check pages use `limit` (default 20, maximum 50) and GraphQL cursors.
   Continue with `pageInfo.endCursor` only when `pageInfo.hasNextPage` is true.
2. Comments use REST `page` (default 1, maximum 10,000) and `limit` (default 20,
   maximum 50). Keep the limit unchanged between pages. `nextPage` means a full
   page was returned and more may exist; a final empty page is possible.
   Each body is capped at 6,000 UTF-8 bytes or 1,000 lines. `bodyTruncated: true`
   explicitly marks omitted content; use the comment's `html_url` to inspect it
   outside these tools. Reduce `limit` if a comment collection exceeds the result
   ceiling. Changing the page size means restarting pagination from page 1.
3. Description/diff/job-log chunks contain at most 12,000 UTF-8 bytes or 1,000 lines.
   Pass the returned `nextOffset` verbatim; no `nextOffset` means the text is
   complete. Requested reviewers are capped at 50; compare their `nodes` length
   with `totalCount` to detect omission.
4. Each result must fit 24 KiB of serialized JSON inside the existing 32 KiB
   mailbox envelope. Oversized results fail explicitly, never silently drop
   records. Host command stdout is bounded at 8 MiB.
5. Each call makes one `gh api` invocation except run view, which makes two:
   run metadata, then one page of that attempt's jobs. PR list/view/checks use
   fixed GraphQL **queries**; other tools use fixed REST **GETs**. Run job pages
   use REST `page` (default 1, maximum 10,000), `limit` (default 20, maximum 50),
   `totalCount`, and optional `nextPage`. Keep the limit unchanged between pages.
   No startup auth probe, automatic pagination, retries, or polling is added.
   Repeated description, diff, and job-log chunks reread the source; do not
   refetch chunks without need. Job-log downloads above 8 MiB fail explicitly,
   even when requesting a small chunk.
6. Reads are live, not snapshots. A PR can change between pages. If the checks'
   `headRefOid` changes, restart their pagination. Avoid combining diff chunks
   across a known PR update.

One bounded worker serializes GitHub reads across sessions, independently of
messaging, readiness, Herd, and vault IO. At most eight jobs wait in its queue;
each has an eight-second deadline including queue time. Expired jobs do not
launch `gh`. Queue overload, unavailable `gh`, authentication/permission/rate-limit
failures, malformed responses, and oversized output return explicit errors.
Raw host stderr and remote error bodies are not forwarded, since credential
helpers and host debug settings can disclose secrets. Diagnose detailed failures
with `gh` on the host. Cancelling a Pi call stops the local wait; already-queued
host work may continue until its deadline. Requests are never automatically
replayed after reconnect or daemon reload.

## Boundary and deployment

PR descriptions, titles, comments, diffs, workflow/job/step names, and logs are
**untrusted repository data**, not instructions. Tools accept no command, flags,
query text, API path, headers, token, or shell fragment. There are no checkout, comment, approval,
merge, workflow-rerun, or other write operations. The GraphQL transport uses POST
for fixed read queries; it accepts no mutations.

The host implementation is `host/bridge/services/github/`, with thin worker glue in
`host/bridge/github.go`. Pi schemas and result conversion live in
`pi-agent/extensions/lib/github-tools.js`, registered by the existing bridge
extension's session transport. No discovery/registration framework was added.

After updating, build the host binary with `make build-bridge` and explicitly run
`pissbox --bridge-reload`. Building alone does not replace the live daemon. Use
Pi's `/reload` to register the new tools. No image rebuild or sandbox recreation
is needed when using the shared updated Pi directory and a compatible bridge.
A bridge predating graceful reload support requires an explicit stop and launch
instead; see the [bridge lifecycle guide](../host/README.md).
