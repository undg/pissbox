# Project configuration (`.pissbox.toml`)

Put `.pissbox.toml` in the primary project: the first CLI directory, or the
current directory when none is given.

```toml
workspaces = ["..", "../shared-tools"]
isolated_directories = ["node_modules", "packages/app/node_modules"]
setup = ["pnpm install --frozen-lockfile", "colgrep init"]
```

All keys are optional arrays of strings; omitted keys and empty arrays do
nothing. Unknown keys, wrong types, and NUL bytes are rejected. Only the primary
config is read—no parent or additional-workspace search. Help/list/delete skip it.
The host requires `jq` and `tomljson` on PATH, even for an empty config;
see [installation](../README.md#start).

**Review unfamiliar configs:** mounts are read-write, and setup can modify
mounted host files, including shared Pi/tool state. Setup never runs on the host.

## Additional workspaces

`workspaces` adds directories after CLI mounts without changing the primary.
Duplicates are removed in order. Paths must be existing directories, absolute
or relative to the primary project. No `~`, variable, command, or glob expansion;
empty paths and line breaks are rejected.

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

Paths must be relative and normalized: no empty, `.` or `..` segments, colons,
control characters, duplicates, or overlaps. Existing files and symlink
components are refused. Primary/state mount paths cannot contain colons or
control characters either.

Isolation is **experimental**, using hidden SBX mounts tested on macOS with
SBX v0.49.1 for the original Bash launcher. Real-SBX verification of the Go port
remains on the [human checklist](../plan-launcher-migration.md#final-human-checklist--after-merge-to-main).
Use `pissbox`, not direct `sbx run`; `--name` is optional.

Setup runs only after isolated mounts are verified. Failed staging blocks Pi
and shell attaches; it never falls back to host dependencies or retries setup.
Recover with a new sandbox name or explicitly delete the failed sandbox.
Backing data under `PISSBOX_STATE_DIR/isolation.*` survives deletion and is not
cleaned automatically. Do not remove it while a sandbox uses it.

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

Secret commands are not supported in this file. Use the explicit launcher flag
`--secret ENV_NAME HOST COMMAND`; see [proxy-managed secrets](secrets.md).
Commands run on the host under SBX, while the sandbox receives a placeholder.

## Changing configuration

Use a **new sandbox name** for changed mounts, isolation, or kit files. Existing
sandboxes retain their mounts and setup state, even after keys are removed.
Reattaching still validates the current config.
