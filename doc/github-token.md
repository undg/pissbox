# GitHub CLI token for agents

Use a dedicated **fine-grained personal access token** for the agent's `gh`
access. Select only the repositories the agent needs, set an expiration, and
use **read-only** permissions throughout. This profile supports reading code,
investigating CI, reviewing issues and PRs, and preparing local fixes for
security and dependency findings.

## Recommended repository permissions

Set each permission below to **Read-only** (Metadata is required by GitHub).
Leave all other repository, organization, and account permissions unselected.

| Permission | Purpose |
| --- | --- |
| Actions | Inspect workflows, runs, logs, and artifacts. |
| Code quality | Inspect code quality findings. |
| Code scanning alerts | Investigate static-analysis and security findings. |
| Commit statuses | Inspect CI results reported through commit statuses. |
| Contents | Read repository files, commits, branches, and releases. |
| Dependabot alerts | Investigate vulnerable dependencies. |
| Issues | Read issue context, comments, labels, and milestones. |
| License compliance alerts | Investigate dependency license findings. |
| Metadata | Read repository metadata; required. |
| Pull requests | Read PRs, diffs, reviews, and comments. |
| Repository license policy | Inspect license policy and package compliance. |
| Repository security advisories | Investigate accessible repository advisories. |
| Variables | Read Actions configuration stored as repository variables. |

Some features depend on the repository's GitHub plan and configuration. A
permission does not enable a feature or grant access beyond the token owner's
access.

Actions variables are treated as non-secret configuration here. Store secrets
in GitHub Actions secrets, not variables. Logs, artifacts, and private security
findings can still contain sensitive information; read-only does not make that
information safe to disclose.

## Human-only write boundary

The agent may read GitHub data and prepare local changes. **Git pushes and all
GitHub mutations are performed manually by the human**, not by the agent. This
includes posting comments, creating PRs, merging, changing settings, dismissing
alerts, and publishing releases. An available credential is not authorization
to cross this boundary.

This token is for `gh`, not permission to push with Git. Read-only permissions
restrict operations authenticated with this token; they do not restrict a
separately authenticated Git transport. Pissbox's proxy-managed Git HTTPS
authentication is a separate boundary and must be restricted independently for
technical enforcement. Neither this document nor a read-only token prevents
local edits or destructive shell commands.

## Permissions intentionally omitted

Copilot agent settings controls repository settings for GitHub's cloud coding
agent, not the models available to your Copilot account or Pi. It is not needed
for checking model availability or policy restrictions.

Secret scanning permissions are not part of this profile: alert access can
expose detected credentials. Artifact metadata, attestations, deployment and
Pages settings, discussions, custom properties, merge queues, and secret
scanning dismissal/bypass requests are also outside this profile.

Keep the token out of tracked files, prompts, logs, and screenshots. Do not
print it to diagnose authentication. Any token supplied directly to the sandbox
through its environment or readable credential storage is accessible to the
agent; prefer supported, configured proxy-managed authentication where possible.
