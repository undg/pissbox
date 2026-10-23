# Using pissbox

This local SBX kit runs `pi --offline` from the `pissbox:local` image.
Build and import it first with `bin/build-image`; see the
[image guide](README.image.md) for CA setup and build options.

## Launch

Run the launcher on the host, from any project directory:

```bash
/path/to/pissbox/bin/pissbox
/path/to/pissbox/bin/pissbox --name project-dev /path/to/project /path/to/extra
```

A symlink on PATH works. With no workspaces, the current directory is primary;
otherwise the first path is primary and the rest are additional read-write
mounts. SBX chooses the name unless `--name` is supplied.

Requires **SBX v0.39.0+** for `sbx run --env`. If `--env` is unknown, check
`sbx version` and upgrade SBX. For a Homebrew installation:

```bash
brew upgrade docker/tap/sbx
```

Run `bin/pissbox --help` for all options. Directory selection is:

| Directory | Flag | Environment variable | Default in this checkout |
| --- | --- | --- | --- |
| Pi configuration | `--pissbox-pi-dir DIR` | `PISSBOX_PI_DIR` | `pi-agent/` |
| Linux tool state | `--pissbox-state-dir DIR` | `PISSBOX_STATE_DIR` | `.cache/` |

Flags override environment variables. Relative paths use the caller's working
directory. Keep both directories separate from native/macOS tool state.

## Optional messaging and Herd reporting

Build the host helper once with `make build` (Go 1.25+), or cross-compile
all macOS/Linux binaries with `make build-all`. The launcher selects the matching
prebuilt binary and registers its mailbox with one persistent daemon per host
OS user. Agent discovery and addressed Pi-to-Pi messaging work without Herd.
Inside a Herd pane, the launcher also binds that pane and reports sandbox Pi as
`pissbox-pi`, including working/idle state and blocking extension UI prompts.
Herd handles notifications according to its settings.

Missing binaries/extensions or bridge failures warn without blocking Pi. No
network port, MCP server, or new mount is needed; the control socket stays on
the host. Launcher exit unregisters only its mailbox, not the daemon. Explicit
`stop` with the matching binary terminates the daemon. After daemon failure,
existing sessions need launcher reattachment to register fresh mailboxes; a new
launch does not restore old registrations. Direct SBX launches bypass this
integration. See the [bridge guide](../go-bridge/README.md) for control commands,
limitations, shared-state security, testing, and future packaging.

## Configuration and login

The launcher mounts `pi-agent/`; kit setup links it to `$HOME/.pi/agent`.
The directory holds profiles, credentials, sessions, extensions, and skills
shared across sandboxes. Use **Pi's `/login` inside the sandbox** for GitHub
Copilot. Authentication remains in the host-owned, gitignored `auth.json`.
Do not print or commit credentials.

This kit does not use upstream Anthropic credential injection: it can overwrite
Pi's shared auth store at startup.

Before every launch, the host-name map in `bin/pissbox` selects a tracked
`settings-*.json` profile and links `pi-agent/settings.json` to it. Hosts `cm`
and `di-7415` use `settings-go.json`; others fall back to `settings-co.json`.
Edit the map to add a host, or add another tracked profile. Do not edit the
generated link directly. Pi's settings changes write through to the active
profile. A pre-existing regular `settings.json` is backed up as
`settings.json.replaced*`.

## Session titles

New interactive sessions start generating a short title when the first text prompt
is submitted, without waiting for Pi to finish the task. The session-name extension
makes one background request to the current model, using only the first user
message (up to 4,000 characters). This consumes a small
amount of additional model usage. Failures leave the session unnamed, without
retries. Existing conversations and manually named sessions are left alone.
Titles appear in Pi's default footer and session picker.

To refresh the title from the current conversation, run:

```text
/skill:session-name
/skill:session-name focus on the shared-cache fix
```

The skill uses the conversation already in context and calls `set_session_name`
to replace the title. It can also accept an exact title. For a direct manual
rename without model generation, use `/name My title`.

Run `/reload` in an existing Pi session after adding or changing the extension
or skill. These live in the shared Pi directory; no image rebuild is needed.

## Existing sandboxes

List or delete sandboxes directly from the host:

```bash
pissbox -l              # or --ls
pissbox -d .            # or --delete .
pissbox --delete /path/to/project
pissbox --delete /path/to/project --yes  # Skip confirmation for automation
```

Both operations require `jq` on the host (`brew install jq` on macOS). They
read SBX's JSON listing without launching a sandbox or changing Pi settings
or shared state.

Listing shows two groups: **Current directory** first, then **Other sandboxes**.
Each entry includes its name, status, agent, and workspace paths. Empty groups
show `(none)`. Headers use bold cyan and muted paths on terminals; redirected
output, `TERM=dumb`, or a set `NO_COLOR` disables color.

Deletion requires an explicit existing directory. Relative paths use the caller's
working directory; symlinks in that argument resolve to their physical path.
A sandbox matches when any workspace equals that absolute path, including a
secondary workspace. Parent and child paths do not match automatically. This
also means deleting by a shared Pi or cache directory can match many sandboxes.

The launcher shows every matching sandbox and asks once `[y/N]` before removing
all of them. Pass `-y` or `--yes` with `--delete` to skip the prompt for automation.
Directory and metadata validation still apply. No matches, missing/invalid directories, or EOF at the prompt are errors.
Declining leaves everything untouched. Mounted host files remain; sandbox-local
data is lost. Deletion does not recreate sandboxes. List and delete cannot be
combined with each other, `--name`, or positional workspaces.

On an existing-sandbox/workspace conflict during a normal launch, the launcher offers:

| Choice | Action |
| --- | --- |
| Enter or `p` | Open Pi with existing mounts |
| `s` | Open an interactive Bash shell |
| `d` | Ask `Do you want to delete it? [y/N]` |
| `q` | Exit without changes |

Letters are case-insensitive. Deletion requires explicit consent and exits
without recreating. EOF aborts; invalid input retries.

Pass `-y` or `--yes` on a normal launch to choose Pi automatically if this conflict
occurs. It never selects deletion. Flags may appear before, between, or after
workspace arguments; the first workspace stays primary:

```bash
pissbox --yes . /path/to/other
pissbox . --yes /path/to/other
pissbox . /path/to/other -y
```

New launches forward all workspaces as usual. Attaching to an existing sandbox
keeps its existing mounts; `--yes` does not add or change them. After `--`, all
arguments are workspace paths, not flags.

Workspaces and copied kit files are fixed at creation. Use a **new sandbox
name** for changed mounts, kit setup, or images; attaching does not update them.
A host symlink does not expose its target unless that directory is mounted too.

Workspace edits, Pi sessions, and [shared tool state](SHARED-DIRECTORIES.md)
survive sandbox removal. Sandbox-local files do not. Stops/restarts retain
sandbox-local files. Never delete a sandbox just to update its environment
without checking for local data first.

## Checks

From the host, after creating a sandbox:

```bash
sbx exec <sandbox-name> pi --offline --version
sbx exec <sandbox-name> sh -c 'readlink "$HOME/.pi/agent"; test -d "$HOME/.pi/agent"'
readlink pi-agent/settings.json
```
