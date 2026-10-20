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
- `sbx-kit/files/home/.pissbox/shared-directories.conf`: shared-directory mappings;
  add or modify tool state here, not in launcher branches or per-tool scripts.
- `.cache/`: gitignored, host-owned Linux model/index/package state shared
  across sandboxes. See `sbx-kit/SHARED-DIRECTORIES.md`.
- `pi-agent/`: sandbox-only Pi agent directory, shared across SBX instances,
  not with native/non-sandboxed Pi. Contains configuration and private runtime
  state. Its `AGENTS.md` applies to sandbox Pi sessions: keep it brief and
  general. Put repository-specific guidance here instead.
- `pi-agent/skills/`: sandbox-wide skills, including `bash-cli-style` for the
  user's Bash command-line scripting conventions.
- `pi-agent/APPEND_SYSTEM.md`: existing user-authored behavior/personality prompt.
  Do not rewrite it as part of environment setup.
- `bin/`: host-side Pi sandbox launcher and colgrep release updater; run
  `bin/pissbox --help` or `bin/update-colgrep --help`.
- `tests/`: launcher regression tests using a stub SBX CLI, shared-directory
  setup tests, and colgrep updater tests with local release fixtures (Python
  standard library).

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

## Bundled colgrep

Run `bin/update-colgrep` on the macOS or Arch Linux host before creating a
sandbox; rerun it to update. It fetches the latest GitHub release archive and
matching SHA-256 checksum, verifies the archive, and extracts only the binary
without executing downloaded code.
The target follows the host architecture with `uname -m`: `arm64`/`aarch64`
selects `aarch64-unknown-linux-gnu`, `x86_64`/`amd64` selects
`x86_64-unknown-linux-gnu`. The image is multi-arch, so the sandbox
architecture matches the host. Other architectures fail before downloading;
there is no architecture option. The gitignored `sbx-kit/files/home/.local/bin/colgrep` lands at
`/home/agent/.local/bin/colgrep` (already on PATH) as a static file, and the
kit's `setup.install` step marks it executable. SBX only copies static files
under `files/home/` and `files/workspace/`; a `files/usr/...` path is ignored.
No root install, remote installer, Dockerfile install, launcher download, or
image rebuild is needed.
Archives and checksums remain in ignored `.cache/downloads/colgrep/`.
The updater uses the host's curl, tar with XZ support, and sha256sum (preferred)
or shasum -a 256; no Python or sandbox-side tools are needed. On Arch Linux,
the packages are curl, tar, xz, and coreutils. A fresh checkout needs the download
step. Existing sandboxes are unchanged by updates. Model downloads, including optional
`colgrep --install-agent`, belong inside the sandbox after shared-cache links
exist and need their hosts allowed by network policy.

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
sandbox kit as the first positional argument, followed by workspaces, the
Pi directory, and the shared state directory. It also passes
`--env PISSBOX_STATE_DIR=...`. Sandbox selection is left to SBX; no custom
discovery or exec bootstrap is needed. The kit links `$HOME/.pi/agent` at creation and refuses
to replace existing data or a different symlink. Run `bin/pissbox --help`
for usage. On SBX's existing-sandbox/workspace conflict, the launcher uses the
name reported by SBX and offers `Choose [P/s/d/q]:`. Enter or `p` opens Pi with
`sbx run --name NAME` without new mounts; `s` opens an interactive Bash shell with
`sbx exec -it NAME bash`; `q` exits without changes. Menu letters are case-insensitive.
Choosing `d` warns about sandbox-local data loss and separately asks
`Do you want to delete it? [y/N]`. Only an explicit yes runs
`sbx rm --force NAME`; deletion exits without recreating, and declining exits
without changes. Invalid answers retry, and EOF aborts without opening Pi,
opening a shell, or deleting. Other SBX
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

The mounted sandbox-only `pi-agent/` must contain its profile files, `auth.json`,
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

## Settings profiles

`pi-agent/settings.json` is an ignored, generated symlink to one tracked
profile. `bin/pissbox` chooses the profile from the host name before every
`sbx run`, so existing sandboxes pick up switches too. The host map lives at the
top of `bin/pissbox` in `settings_profile_for_host`; `cm` and `di-7415` use
`settings-go.json`, and unknown or unavailable host names fall back to
`settings-co.json` (GitHub Copilot).

Add a workstation by editing that map. Add a new profile by tracking another
`pi-agent/settings-*.json` file. Do not track or edit `settings.json` directly.
If a stale regular `settings.json` exists, the launcher moves it to an ignored
`settings.json.replaced*` backup before creating the link. Pi writes through the
link, so runtime changes such as `lastChangelogVersion` and model defaults land
in the active tracked profile.

## Shared tool state

The shared state root defaults to this checkout's gitignored `.cache/`.
Selection is `--pissbox-state-dir DIR` first, then `PISSBOX_STATE_DIR`, then the
checkout default. Relative overrides use the caller's directory. The launcher
creates and canonicalizes the root before mounting it. Keep this state Linux
sandbox-only, separate from native/macOS caches and Pi configuration. The old
`shared-state` and `sandbox-state` names may remain as ignored compatibility
symlinks to `.cache` for existing sandbox paths. Do not remove them while those
paths are in use; the launcher does not migrate existing sandboxes.

Mappings live in `sbx-kit/files/home/.pissbox/shared-directories.conf` as literal
`state-relative-path|home-relative-path` rows. Use relative paths without dot
segments or shell expansion. The kit's `link-shared-directories.sh` creates
sources on the mounted root and links default tool locations as user `1000`.
It accepts matching links but refuses existing data or different links. No
per-tool launcher code or Dockerfile changes are needed to add another mapping.
The default mappings include the HF hub (not tokens), colgrep data/config/cache,
ONNX Runtime, npm's package cache, and both pnpm's content-addressable store and
metadata cache. Kit setup configures npm's user-level cache as `$HOME/.cache/npm`,
linked to `npm` under the state root, without replacing the image's `$HOME/.npm`.
Custom tool/XDG paths can bypass these links. This setup does
not install tools or change network policy. New mappings/mounts require a new
sandbox; attaching never updates them. Avoid concurrent colgrep index writers
until upstream locking is verified. See `sbx-kit/SHARED-DIRECTORIES.md`.

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
`bash -n bin/pissbox bin/update-colgrep sbx-kit/files/home/.pissbox/scripts/link-shared-directories.sh`,
and `git diff --check`. Do not claim a real sandbox run,
authentication, or restart persistence is verified without testing it.
Run launcher regression tests with
`PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s tests -v`.
These cover symlink launches from another directory, caller-relative overrides,
menu choices/defaults, deletion consent, EOF, argument forwarding, exit statuses,
terminal stdin/stdout preservation, and host-to-settings-profile selection with
a stub `hostname`. Shared-directory tests check real links,
retained data across isolated homes, refusal to replace data, and adding a
mapping without script edits. Updater tests use stub curl and real
SHA-256/tar commands to check the detected Linux target, updates, executable
modes, architecture selection, and preserving the binary on failure. Isolated
PATH tests cover sha256sum, shasum fallback, and a missing checksum tool. A
test-only Python XZ decompressor lets GNU tar run in the Linux test sandbox
without xz; production uses macOS tar's built-in XZ support or Arch's xz
package. A stub uname supplies the host architecture. Generated fixtures
remain under ignored
`.cache/test-fixtures/`. Tests do not run real SBX or colgrep/pnpm.

After a real sandbox is created, check the links from the host:

```bash
sbx exec <sandbox-name> sh -c \
  'readlink "$HOME/.pi/agent"; test -d "$HOME/.pi/agent"'
readlink pi-agent/settings.json
```

Update this file as the repository evolves. Keep operational detail here rather
than expanding the sandbox-shared `pi-agent/AGENTS.md`.
