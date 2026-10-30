package launcher

const helpText = `Usage: pissbox [OPTIONS] [WORKSPACE ...]
       pissbox -d DIR [-y]
       pissbox -l

Start Pi in an SBX sandbox with sandbox-only configuration.

Options:
  -c, --continue       Continue the previous Pi session for the workspace
  -d, --delete DIR     Delete all sandboxes with DIR as a workspace
  -y, --yes            Open existing Pi without a menu; skip --delete confirmation
  -l, --ls             List sandboxes, current-directory matches first
  --bridge-stop        Stop this checkout's compatible host bridge daemon
  --name NAME          Pass an explicit sandbox name to SBX
  --secret ENV_NAME HOST COMMAND
                       Register a proxy-managed secret on creation (repeatable)
  --ignore-config-workspaces
                       Skip configured mounts; keep CLI paths and setup
  --pissbox-pi-dir DIR  Override PISSBOX_PI_DIR (default: this checkout's pi-agent/)
  --pissbox-state-dir DIR
                       Override PISSBOX_STATE_DIR (default: this checkout's .cache/)
  -h, --help           Show this help
  --                   Treat remaining arguments as workspaces

With no workspaces, use the current directory.
The first workspace is primary; additional ones are mounted read-write.
Primary-workspace .pissbox.toml workspaces add mounts, relative to that project.
CLI workspaces stay first; duplicate directories are removed.
Reading .pissbox.toml requires tomljson and jq on the host.
Its setup commands run inside the sandbox once during creation, before Pi.
Opt-in isolated_directories keeps Linux dependencies separate from host directories.
Shared Linux caches live in PISSBOX_STATE_DIR, created if missing.
Directory flags override environment variables; relative paths use the caller's directory.
Settings profile follows the host name; unknown hosts use settings-co.json.
SBX chooses the sandbox name unless --name is provided.
Secrets require SBX secret set-custom --command/--placeholder support (verified with v0.50.0).
ENV_NAME receives a placeholder; HOST is a host/pattern, not a URL.
Quote COMMAND: SBX runs it on the host for verification and later refresh.
Keep helpers and dependencies outside writable sandbox mounts.
Secret registration precedes project setup/Pi and never runs on attach.
Global secrets live in PISSBOX_CONFIG or ~/.config/pissbox/config.toml.
The launcher creates a schema-only starter at the default path when missing.
Global entries apply to every new sandbox; CLI --secret overrides the same ENV_NAME.
The global file is read-only to the launcher; store commands, never literal values.
Help/list/delete/bridge-stop never read or create the global file.
See doc/secrets.md; secret flags cannot be combined with list/delete/bridge-stop.
If it already exists, choose Pi (default), continue, shell, delete, or quit [P/c/s/d/q].
With --yes, open Pi in the existing sandbox without prompting; existing mounts stay.
Deletion requires a separate confirmation [y/N], unless --delete uses --yes.
An optional prebuilt host bridge enables agent messaging and Herd lifecycle state.
Build it with bin/build-bridge; bridge failures never prevent launching Pi.
Deletion removes sandbox-local data and exits; mounted host files remain.
Delete requires an existing directory and errors if no sandboxes match.
Matching uses exact absolute workspace paths, not parent or child directories.
Delete and list require jq and cannot be combined with each other, --name, or workspaces.
`
