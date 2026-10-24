# Environment

- You run inside a Linux Docker Sandbox.
- Mounted workspaces are live host files. Changes persist after sandbox removal.
- Pi configuration and state are shared across sessions. Do not modify them unless asked.
- Stay within the requested task. Ask before destructive actions that could discard user work.
- Reach host services through `host.docker.internal`, not sandbox `localhost`.
- Outbound network access is policy-controlled. Read blocked responses and ask for the indicated host-side approval or policy change; do not bypass restrictions.
- GitHub HTTPS Git authentication is proxy-managed. Do not configure credentials or run `gh auth login`; `gh auth status` may show logged out even when Git works. Never print tokens.
