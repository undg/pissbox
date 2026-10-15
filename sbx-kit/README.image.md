# pissbox/pi-image:local

Base image for the pi agent kit for
[Docker Sandboxes](https://docs.docker.com/ai/sandboxes/).

## Contents

Built on `docker/sandbox-templates:shell-docker`: the standard sandbox tool
chain plus a Docker engine, requesting Docker-in-Docker via
`com.docker.sandboxes.start-docker=true`. On top of that:

- [`@earendil-works/pi-coding-agent`](https://www.npmjs.com/package/@earendil-works/pi-coding-agent),
  installed globally at the latest upstream release at build time,
  plus a `/usr/local/bin/pi` symlink
- `fd-find` (apt), which backs pi's `find` tool, plus a `/usr/local/bin/fd`
  symlink for the canonical name — without it pi downloads a binary from
  GitHub releases on first use, which a locked-down sandbox blocks
- no ripgrep layer — pi probes the system for `rg` just as it does for `fd`,
  and the template already ships it
- no Node layer — the template already ships Node 22.22.1, and pi requires
  `>= 22.19.0`

The npm global prefix stays owned by `agent`, so `pi install` and
`pi update --self` work inside the sandbox.

Runs as the non-root `agent` user, with `CMD ["pi"]`.

The required `SHARED_PI_AGENT_DIR` build argument supplies an absolute host path.
`/home/agent/.pi/agent` is a symlink to that path, which must also be mounted as
an SBX workspace. No shared configuration or credentials are baked into the image.

See [README.md](README.md) for local build and sandbox creation commands.
