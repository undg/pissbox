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

## Optional Herd reporting

Build the host helper once with `bin/build-bridge` (Go 1.24+), or cross-compile
all macOS/Linux binaries with `bin/build-bridge --all`. Then launch pissbox
from a Herd pane. The launcher selects the matching prebuilt binary and reports
sandbox Pi as `pissbox-pi`, including working/idle state and blocking extension UI
prompts. Herd handles notifications according to its settings.

Outside Herd, no helper runs. Missing binaries or bridge failures warn without
blocking Pi. No network port, MCP server, new mount, or permanent daemon is
needed. The helper runs only while the launcher is attached; direct SBX launches
bypass it. See the [bridge guide](../go-bridge/README.md) for limitations,
shared-state security, testing, and future packaging.

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

## Existing sandboxes

List or delete sandboxes directly from the host:

```bash
pissbox -l              # or --ls
pissbox -d .            # or --delete .
pissbox --delete /path/to/project
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
all of them. No matches, missing/invalid directories, or EOF are errors. Declining
leaves everything untouched. Mounted host files remain; sandbox-local data is
lost. Deletion does not recreate sandboxes. List and delete cannot be combined
with each other, `--name`, or positional workspaces.

On an existing-sandbox/workspace conflict during a normal launch, the launcher offers:

| Choice | Action |
| --- | --- |
| Enter or `p` | Open Pi with existing mounts |
| `s` | Open an interactive Bash shell |
| `d` | Ask `Do you want to delete it? [y/N]` |
| `q` | Exit without changes |

Letters are case-insensitive. Deletion requires explicit consent and exits
without recreating. EOF aborts; invalid input retries.

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
