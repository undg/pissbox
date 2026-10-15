# pissbox

Personal Pi coding-agent environment for Docker Sandboxes (`sbx`). The goal is
disposable sandboxes with a reproducible, customizable Linux toolset and shared
host-owned Pi state. Start simple; an Arch-like environment is a longer-term
idea, not the current base image.

The repository is named **pissbox**; its directory may still be named `pisbx`.
Do not hard-code either directory location.

## Layout

- `sbx-kit/`: local SBX kit, Dockerfile, setup, network policy, and documentation.
- `sbx-kit/files/home/.pissbox/scripts/`: setup scripts bundled with the kit;
  SBX copies them to `$HOME/.pissbox/scripts/` before running `setup.install`.
- `pi-agent/`: sandbox-only Pi agent directory, shared across SBX instances,
  not with native/non-sandboxed Pi. Contains configuration and private runtime
  state. Its `AGENTS.md` applies to sandbox Pi sessions: keep it brief and
  general. Put repository-specific guidance here instead.
- `pi-agent/skills/`: sandbox-wide skills, including `bash-cli-style` for the
  user's Bash command-line scripting conventions.
- `pi-agent/APPEND_SYSTEM.md`: existing user-authored behavior/personality prompt.
  Do not rewrite it as part of environment setup.
- `bin/`: host-side Pi sandbox launcher; run `bin/pissbox --help`.
- `tests/`: launcher regression tests using a stub SBX CLI (Python standard library).

## Image and kit

`sbx-kit/spec.yaml` references the published `docker.io/sbx/pi-image:latest`.
`sbx-kit/Dockerfile` describes that upstream image but is **not** built by our
normal launch flow. Do not add a machine-specific host path or config symlink
to the image: the host home directory varies by machine. The launcher mounts
the sandbox-only `pi-agent/` and passes its absolute path with
`--env PISSBOX_PI_DIR=...`.
The kit's `setup.install` creates the config symlink as user `1000` before Pi
first launches. Keep `entrypoint: [pi]`; no image startup helper is needed.
Leave `PI_CODING_AGENT_DIR` unset; Pi reads the symlink at its default path.
No configuration or credentials are copied into image layers.

## Workspaces and lifecycle

From a project's worktree, run `/path/to/pissbox/bin/pissbox` (or a symlink
to it on PATH) with optional workspace directories. Resolve launcher symlinks
before locating the checkout's kit and default Pi directory; preserve the caller's
working directory. The first workspace is primary; the rest are additional
mounts. With none, the current directory is primary. SBX picks the sandbox
name unless `--name NAME` (or `--name=NAME`) is provided; the launcher forwards
it to `sbx run` to allow multiple named sandboxes for the same workspaces.
Repeated name options use the last value. Pi directory selection is `--pissbox-pi-dir DIR` first, then the
sandbox-specific `PISSBOX_PI_DIR` environment variable, then this checkout's
`pi-agent/`. Relative overrides resolve from the caller's current directory;
the selected path is canonicalized before mounting. The launcher uses a
`case` loop, accepts options among workspaces, and supports `--` to end option
parsing. It does not use `PI_HOST_CONFIG_DIR` or automatically discover native
Pi configuration. Keep overrides sandbox-only. It calls
`sbx run --env PISSBOX_PI_DIR=...` with the local
sandbox kit as the first positional argument, followed by workspaces and the
Pi directory. Sandbox selection is left to SBX; no custom discovery or exec
bootstrap is needed. The kit links `$HOME/.pi/agent` at creation and refuses
to replace existing data or a different symlink. Run `bin/pissbox --help`
for usage. On SBX's existing-sandbox/workspace conflict, the launcher uses the
name reported by SBX and asks `Get into it? [Y/n]`. Accepting runs
`sbx run --name NAME` without new mounts. Declining offers deletion with
`Do you want to delete it? [y/N]`, after warning about sandbox-local data loss.
Only an explicit yes runs `sbx rm --force NAME`; deletion exits without recreating.
Invalid answers retry, and EOF aborts without attaching or deleting. Other SBX
errors retain their output and exit status. Do not silently recreate or remove
an existing sandbox.

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

The mounted sandbox-only `pi-agent/` must contain `settings.json`, `auth.json`,
`sessions/`, etc. directly, not in a nested `agent/`. Keep it separate from
native/non-sandboxed Pi configuration; those agents have different restrictions.
Sharing the whole directory read-write across sandboxes, including credentials
and sessions, is intentional. Do not read or print credentials to debug the mount.

The user uses GitHub Copilot and authenticates through Pi's `/login` inside the
sandbox. The kit intentionally removes upstream Anthropic credential injection:
that integration can materialize `auth.json` at startup and overwrite entries
for other providers. Do not restore it over the shared auth store.

Never print or commit credentials. `.gitignore` excludes `pi-agent/auth.json`
and `pi-agent/sessions`; review other generated/private files before staging.
Do not reset shared state or change shared configuration outside the task.
Avoid mixing macOS and Linux package installations into the same shared tree.

## Development and verification

The launcher requires SBX v0.39.0 or newer: that release added `--env` and
`--env-file` to `sbx run` and `sbx create`. If a host reports `unknown flag:
--env`, check `sbx version` and upgrade SBX; do not remove the variable needed
by the config-link setup script. Official release binaries downloaded for
local CLI checks belong in ignored `.tools/`, not in commits. CLI help can be
checked without a daemon, but v0.48.0 requires Docker sign-in even for
`sbx kit validate`; installing the CLI alone does not verify kit validity.

Keep changes simple and reproducible. Read kit files before editing. Validate
kit and launcher changes with `sbx kit validate ./sbx-kit`,
`bash -n bin/pissbox`, and `git diff --check`. Do not claim a real sandbox run,
authentication, or restart persistence is verified without testing it.
Run launcher regression tests with
`PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s tests -v`.
These cover symlink launches from another directory, caller-relative overrides,
confirmation defaults, deletion consent, EOF, argument forwarding, exit statuses,
and terminal stdin/stdout preservation; they do not run real SBX.

After a real sandbox is created, check the link from the host:

```bash
sbx exec <sandbox-name> sh -c \
  'readlink "$HOME/.pi/agent"; test -d "$HOME/.pi/agent"'
```

Update this file as the repository evolves. Keep operational detail here rather
than expanding the sandbox-shared `pi-agent/AGENTS.md`.
