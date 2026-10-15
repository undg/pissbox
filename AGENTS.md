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
- `bin/`: host-side Pi sandbox launcher; run `bin/pissbox-start --help`.

## Image and kit

`sbx-kit/spec.yaml` references the published `docker.io/sbx/pi-image:latest`.
`sbx-kit/Dockerfile` describes that upstream image but is **not** built by our
normal launch flow. Do not add a machine-specific host path or config symlink
to the image: the host home directory varies by machine. The launcher mounts
host configuration and makes the symlink inside the sandbox at creation time.
Leave `PI_CODING_AGENT_DIR` unset; Pi reads the symlink at its default path.
No configuration or credentials are copied into image layers.

## Workspaces and lifecycle

From a project's worktree, run `/path/to/pissbox/bin/pissbox-start` with
optional workspace directories. The first is primary; the rest are additional
mounts. With none, the current directory is primary. SBX picks the sandbox
name. The launcher defaults to the host's `$HOME/.config/pi`;
`PI_HOST_CONFIG_DIR` overrides that path. It mounts the workspaces and Pi
directory, refuses to overwrite an existing sandbox Pi directory, links
`$HOME/.pi/agent` in the sandbox, then attaches with `sbx run`. Run
`bin/pissbox-start --help` for usage. Do not silently recreate or remove an
existing sandbox.

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

The mounted host Pi directory must contain `settings.json`, `auth.json`,
`sessions/`, etc. directly, not in a nested `agent/`. The launcher's default is
`$HOME/.config/pi`; `PI_HOST_CONFIG_DIR` can point to `pi-agent/` instead.
Sharing the whole directory read-write, including credentials and sessions, is
intentional. Do not read or print credentials to debug the mount.

The user uses GitHub Copilot and authenticates through Pi's `/login` inside the
sandbox. The kit intentionally removes upstream Anthropic credential injection:
that integration can materialize `auth.json` at startup and overwrite entries
for other providers. Do not restore it over the shared auth store.

Never print or commit credentials. `.gitignore` excludes `pi-agent/auth.json`
and `pi-agent/sessions`; review other generated/private files before staging.
Do not reset shared state or change shared configuration outside the task.
Avoid mixing macOS and Linux package installations into the same shared tree.

## Development and verification

Keep changes simple and reproducible. Read kit files before editing. Validate
kit and launcher changes with `sbx kit validate ./sbx-kit`,
`bash -n bin/pissbox-start`, and `git diff --check`. Do not claim a real sandbox run,
authentication, or restart persistence is verified without testing it.

After a real sandbox is created, check the link from the host:

```bash
sbx exec <sandbox-name> sh -c \
  'readlink "$HOME/.pi/agent"; test -d "$HOME/.pi/agent"'
```

Update this file as the repository evolves. Keep operational detail here rather
than expanding the globally shared `pi-agent/AGENTS.md`.
