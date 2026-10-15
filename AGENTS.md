# pissbox

Personal Pi coding-agent environment for Docker Sandboxes (`sbx`). The goal is
disposable sandboxes with a reproducible, customizable Linux toolset and shared
host-owned Pi state. Start simple; an Arch-like environment is a longer-term
idea, not the current base image.

The repository is named **pissbox**; its directory may still be named `pisbx`.
Do not hard-code either directory location.

## Layout

- `sbx-kit/`: local SBX kit, Dockerfile, setup, network policy, and documentation.
- `pi-agent/`: shared Pi agent directory. Contains user configuration and private
  runtime state. Its `AGENTS.md` applies to every Pi session: keep it brief and
  general. Put repository-specific guidance here instead.
- `pi-agent/APPEND_SYSTEM.md`: existing user-authored behavior/personality prompt.
  Do not rewrite it as part of environment setup.
- `bin/`: reserved for host-side launch helpers; currently empty.

## Image and kit

`sbx-kit/spec.yaml` references `pissbox/pi-image:local`. Merely editing the
Dockerfile does not change the image: build it before creating a new sandbox.
The current base is `docker/sandbox-templates:shell-docker`, not Arch Linux.
The image installs Pi from npm and fd; the base supplies Node, ripgrep, and Docker.
Pi defaults to npm's latest release at build time; `PI_VERSION` can pin it.

From this repository's root, on the host:

```bash
PISSBOX_ROOT="$(pwd -P)"
docker build \
  --build-arg SHARED_PI_AGENT_DIR="$PISSBOX_ROOT/pi-agent" \
  -t pissbox/pi-image:local \
  "$PISSBOX_ROOT/sbx-kit"
```

The required build argument creates this symlink in the image:

```text
/home/agent/.pi/agent → <absolute host repository path>/pi-agent
```

Leave `PI_CODING_AGENT_DIR` unset. No configuration or credentials should be
copied into image layers. The link is dangling until its target is mounted.
Moving this repository requires rebuilding with the new path and creating a
new sandbox.

## Workspaces and lifecycle

From a project's worktree, in the same host shell:

```bash
sbx run --name <new-sandbox-name> \
  --kit "$PISSBOX_ROOT/sbx-kit" \
  pi . .. "$PISSBOX_ROOT/pi-agent"
```

- `.` is the primary workspace and Pi's starting directory.
- `..` exposes sibling worktrees and shared Git metadata for bare-repository
  layouts. Omit it when unnecessary. The user accepts this broader exposure.
- The final argument separately mounts shared Pi state read-write.

SBX preserves absolute host workspace paths. Additional workspaces are not
Docker-style arbitrary source/destination mappings. A host symlink alone does
not expose its target; mount the actual target directory.

Normal workspaces are live mounts, not copies. Writes immediately affect host
files. `sbx cp` makes an independent copy, not a synchronized mount. Workspaces
are selected at creation and cannot be added to an existing sandbox. Use a new
name for changed mounts; do not delete existing sandboxes without permission.
Kit/image changes generally require a new sandbox too.

Sandbox-local state survives stops/restarts but not sandbox removal. Mounted
host state survives removal. A Pi conversation session and an SBX instance are
separate things; shared sessions can outlive individual sandboxes.

## Shared configuration and authentication

Put `settings.json`, `auth.json`, `sessions/`, extensions, etc. directly inside
`pi-agent/`, not a nested `agent/`. Sharing the whole directory read-write across
sandboxed agents is intentional for now, including credentials and sessions.
More granular permissions can come later.

The user uses GitHub Copilot and authenticates through Pi's `/login` inside the
sandbox. The kit intentionally removes upstream Anthropic credential injection:
that integration can materialize `auth.json` at startup and overwrite entries
for other providers. Do not restore it over the shared auth store.

Never print or commit credentials. `.gitignore` excludes `pi-agent/auth.json`
and `pi-agent/sessions`; review other generated/private files before staging.
Do not reset shared state or change shared configuration outside the task.
Avoid mixing macOS and Linux package installations into the same shared tree.

## Development and verification

Keep changes simple and reproducible. Read the relevant kit files before
editing; keep `sbx-kit/README.md` and image documentation aligned with behavior.

```bash
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover \
  -s sbx-kit/testdata -p 'test_*.py'
git diff --check
```

The Python tests execute the Dockerfile's symlink shell command in temporary
directories. They cover absolute-path validation, spaces, a dangling target,
write-through behavior, and refusing to replace an existing directory.
They do not build the image or validate SBX schema, image availability, network
policy, or Copilot login.

At the time this guidance was written, the shell tests passed, but a full custom
image build, SBX startup, and authentication persistence across restarts had not
been verified. Do not report those integration steps as proven without testing.

After a real sandbox is created, check the link from the host:

```bash
sbx exec <sandbox-name> -- sh -c \
  'readlink "$HOME/.pi/agent"; test -d "$HOME/.pi/agent"'
```

Update this file as the repository evolves. Keep operational detail here rather
than expanding the globally shared `pi-agent/AGENTS.md`.
