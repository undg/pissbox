# Project configuration (`.pissbox.toml`)

Put `.pissbox.toml` in the primary project: the first CLI directory, or the
current directory when none is given.

```toml
workspaces = ["..", "../shared-tools", "~/.dot"]
isolated_directories = ["node_modules", "packages/app/node_modules"]
setup = ["pnpm install --frozen-lockfile", "colgrep init"]
```

All keys are optional arrays of strings; omitted keys and empty arrays do
nothing. Unknown keys, wrong types, and NUL bytes are rejected. Only the primary
config is read—no parent or additional-workspace search. Help/list/delete skip it.
The launcher parses and validates this file in Go before settings, state, or
SBX effects. The sandbox validates it again before running `setup`, where
`tomljson` and `jq` are available in the image.

**Review unfamiliar configs:** mounts are read-write, and setup can modify
mounted host files, including shared Pi/tool state. Setup never runs on the host.

## Additional workspaces

`workspaces` adds directories after CLI mounts without changing the primary.
Duplicates are removed in order. Paths must be existing directories, absolute
or relative to the primary project. A leading `~`, `$HOME`, or `${HOME}`
(as the whole path or followed by `/`) resolves to the launcher host's home
directory. No other variable expansion, command substitution, or globbing is
performed. Empty paths and line breaks are rejected.

`--ignore-config-workspaces` skips configured mounts and their directory checks,
not schema validation, CLI/shared mounts, or setup.

### Symlinked configuration

Relative paths still use the primary project, not the config target's directory.
Mount the target's containing directory through `workspaces` or CLI arguments;
it is not mounted automatically. Broken/unreadable config links fail.

## Optional dependency isolation

`isolated_directories` gives each sandbox separate Linux dependencies at the
listed project paths. Source and lockfiles remain shared with the host.
List every directory explicitly; there is no monorepo discovery.

Paths are literal, relative to the primary project, and not home-expanded.
They must be normalized: no empty, `.` or `..` segments, colons, control
characters, duplicates, or overlaps. Existing files and symlink
components are refused. Primary/state mount paths cannot contain colons or
control characters either.

Isolation uses SBX runtime bind mounts through staged creation. Direct tests on
macOS/SBX v0.54.0 verified ordinary and overlapping mounts and stop/start
restoration; they did not verify the complete Go launcher flow.
Use `pissbox`, not direct `sbx run`; `--name` is optional.

During creation, setup runs only after isolated mounts are verified. Failed
staging blocks Pi and shell attaches rather than falling back to host
dependencies or retrying setup.
Recover with a new sandbox name or explicitly delete the failed sandbox.
Backing data under `PISSBOX_STATE_DIR/isolation.*` survives deletion and is not
cleaned automatically. Do not remove it while a sandbox uses it.

**Known limitation (macOS/SBX v0.54.0):** replacing a host isolated directory
(rename, then recreate at the same path) can expose host dependencies inside the
running sandbox, despite its saved mount registration. Attachment checks setup
completion, not live mounts. **Stop/start the affected sandbox**, not just Pi:
both reproduction tests reattached the existing Linux data without recreation
or reinstalling dependencies. Stopping terminates running processes; avoid
writes through the affected path until isolation is restored.

## First-time project setup

`setup` runs once during creation, after built-in setup and before Pi, as UID
1000. Commands share one Bash process starting in the primary project; `cd`,
variables, and exports carry between commands, but not into Pi.

Bash uses `errexit`, `nounset`, and `pipefail`. Failures block the initial Pi
launch without rolling back completed work. Keep commands repeat-safe.
Restarts and attaches never rerun setup. TOML literal and multiline strings
work for shell quoting.

For direct SBX launches without isolation, pass mounts explicitly and set
`--env PISSBOX_WORKSPACE_DIR=/absolute/primary/workspace`.

## Secrets

Secret commands are not supported in this project file. Use the user-level
global config for recurring secrets, or the explicit launcher flag for one-off
additions; see [proxy-managed secrets](secrets.md). Commands run on the host
under SBX, while the sandbox receives a placeholder. The global file is not
read or created by help/list/delete/bridge-stop.

## Changing configuration

Use a **new sandbox name** for changed mounts, isolation, or kit files. Existing
sandboxes retain their mounts and setup state, even after keys are removed.
Reattaching still validates the current config.
