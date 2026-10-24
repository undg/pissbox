# Using pissbox

This local SBX kit runs `pi --offline` from the `pissbox:local` image.
Build and import it first with `make build-image` from the checkout root; see the
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

## Project configuration and additional workspaces

Put `.pissbox.toml` in the primary project. The host reads it before launch,
so `jq` and `tomljson` must be on PATH even for an empty config. `make install`
checks, but does not install them. Install `jq` with your package manager and:

```bash
go install github.com/pelletier/go-toml/v2/cmd/tomljson@v2.4.3
```

Add `GOBIN` (normally `~/go/bin`) to PATH. The image supplies its own parser.

```toml
workspaces = [
  "..",
  "../shared-tools",
]

setup = [
  "pnpm install --frozen-lockfile",
  "colgrep init",
]
```

- `workspaces` **adds** read-write mounts after CLI paths; it never changes the
  primary. Duplicates are removed in order, including shared Pi/state mounts.
- Relative entries use the primary project, not a symlinked config's location.
  Absolute paths work. Entries must be existing directories; no `~`, variable,
  command, or glob expansion. Empty paths, line breaks, and NUL bytes are errors.
- Only the primary config is read; no parent/additional-directory search.
  Missing files/keys and empty arrays are no-ops. Help/list/delete ignore config.

With `workspaces = [".."]`, these all mount the current directory and its parent:

```bash
pissbox
pissbox -y
pissbox .
pissbox . -y
```

For `pissbox /path/to/project /path/to/extra`, both CLI paths stay mounted and
`/path/to/project/.pissbox.toml` supplies additional directories.

Skip configured mounts for one launch:

```bash
pissbox --ignore-config-workspaces
pissbox . /path/to/extra -y --ignore-config-workspaces
```

CLI and shared Pi/state mounts stay; configured directory checks are skipped.
The file/schema is still validated and `setup` still runs on creation. Existing
sandbox mounts cannot be changed by this flag.

### Symlinked configuration

The host can read a symlink before choosing mounts, but the sandbox also needs
access to its target. For a config in the parent directory, use
`workspaces = [".."]`; targets elsewhere need their containing directory listed.
Targets are not mounted automatically. With `--ignore-config-workspaces`,
supply target mounts on the CLI. Broken/unreadable config links fail before SBX creation.

Use a **new sandbox name** for changed mounts or kit files; no image rebuild
is needed if the image has the parser. Reattaching with `-y` keeps existing
mounts, although the host still validates the current config.

## Optional dependency isolation

Keep source and lockfiles live on the host, but give Linux its own dependency
contents at the same project paths:

```toml
isolated_directories = ["node_modules", "packages/app/node_modules"]
setup = ["pnpm install --frozen-lockfile"]
```

Omit the key or use `[]` for the original launch behavior. List each needed
workspace dependency directory; there is no monorepo discovery. Entries must be
normalized relative directories: no absolute paths, empty/dot/traversal segments,
colons, control characters, duplicates, or overlapping directories. Existing
files and symlink components are refused, including symlinks staying inside the
project. Primary/state mount paths cannot contain colons or control characters.
Validation happens before settings, state creation, or SBX actions.

On a **new** sandbox, the launcher creates with project setup deferred. Its kit
hook reports SBX's assigned identity through a unique launch mailbox, so plain
`pissbox` works without `--name`. The launcher creates missing host mountpoint
folders as empty directories, preserving existing directories and contents, then
mounts fresh Linux backing directories over them. It verifies each unique mount
marker before testing UID-1000 writability. Only then does project setup run as
UID 1000 and mark the sandbox ready for Pi. Source and lockfile edits still reach
the host; host dependencies stay separate. Each sandbox/worktree gets fresh
backing directories; installing separately per platform is expected.

The adapter lives in `bin/lib/isolated-directories.sh`, uses SBX's hidden
`sbx mount NAME SOURCE:TARGET` command, and is
**experimental**. Hidden mounts and install-time `SANDBOX_NAME` were tested on
macOS with SBX v0.49.1, not promised by upstream documentation. Nameless and
explicit-name duplicate creation conflicts were also tested on that version.
An explicit-name conflict must report the exact requested name. Unrecognized
creation errors are preserved rather than matched broadly or used to mutate an
existing sandbox. The ordinary
installer still checks SBX v0.39.0+; it does not certify this optional feature.
No new tools, image build, or entrypoint changes are needed. New kit files require
a new sandbox. Native-addon compatibility is not covered by the host experiment.

Attachments never remount directories or replay setup, and current config cannot
retrofit an old sandbox. Use a new name when enabling/changing isolation. A small
readiness check before existing Pi **and shell** attaches blocks unfinished
isolated sandboxes, including after the config is removed. It only reads readiness
state, but `sbx exec` starts stopped sandboxes and can run their existing startup
hooks before opening Pi or shell. Old sandboxes without the persisted isolation
environment pass. Quit and explicit deletion still work.
Failed or interrupted staging leaves evidence and backing data in
`PISSBOX_STATE_DIR/isolation.*`; it does not fall back to host dependencies,
resume setup, roll back, or delete anything. Recover with a new sandbox name or
explicitly delete the failed sandbox. Backing data survives sandbox deletion and
is never removed automatically. Do not remove or move it while a sandbox uses it.

Disabling the key only changes future creations; existing mounts remain. Once
SBX has native isolation, replace the adapter while retaining this intent key.
Direct `sbx run` kit launches are **refused before project setup** when the config
requests isolation without the persisted staging environment. Use `pissbox` for
projects opting in; otherwise setup could overwrite host dependencies.

## First-time project setup

`setup` runs **once during creation, before Pi**, after built-in kit setup.
Commands run as user `1000` in one Bash process, initially in the primary project;
`cd`, variables, and exports carry between commands, but not into Pi.
Bash uses `errexit`, `nounset`, and `pipefail`: unhandled failures prevent the
initial Pi launch. Completed work is not rolled back; keep commands repeat-safe
for failed-creation retries. Restarts and attachments never rerun setup.

All three keys (`setup`, `workspaces`, `isolated_directories`) must be arrays of strings. The entire config
is validated before launch; unknown keys, wrong types, and NUL bytes fail.
TOML literal (`'...'`) and multiline strings work for shell quoting. The old
executable `.pissbox` format is not read.

**Review unfamiliar configs:** mounts expose host directories read-write and
setup commands can modify them, including shared Pi/tool state. Commands run
only inside the sandbox, never on the host.

For direct SBX kit launches, pass all workspaces explicitly and add
`--env PISSBOX_WORKSPACE_DIR=/absolute/primary/workspace`.

## Optional messaging and Herd reporting

Build the host helper once with `make build-bridge` (Go 1.25+), or cross-compile
all macOS/Linux binaries with `make build-bridge-all`. The launcher selects the matching
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
| `c` | Continue the previous Pi session with existing mounts |
| `s` | Open an interactive Bash shell |
| `d` | Ask `Do you want to delete it? [y/N]` |
| `q` | Exit without changes |

Letters are case-insensitive and act immediately, without Enter. Enter still
selects the default Pi action. Deletion requires a separate explicit yes-and-Enter
confirmation and exits without recreating. EOF aborts; invalid input retries.

Pass `-y` or `--yes` on a normal launch to choose Pi automatically if this conflict
occurs. It never selects deletion. Flags may appear before, between, or after
workspace arguments; the first workspace stays primary:

```bash
pissbox --yes . /path/to/other
pissbox . --yes /path/to/other
pissbox . /path/to/other -y
```

The menu's `c` choice and the launcher's `-c` / `--continue` flags forward Pi's
`--continue` to resume the previous session for the workspace, including
dependency-isolated sandboxes. Flags apply on new launches and existing Pi
attaches, not shell attaches. Use `pissbox -y -c` to continue without the menu;
`--yes` alone still selects the ordinary Pi action.

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
