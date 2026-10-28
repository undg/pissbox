# pissbox

<img width="512" height="279" alt="image" src="https://github.com/user-attachments/assets/2153ea65-a15d-49c3-970e-e1a6b3bb937e" />

Personal [Pi coding-agent](https://www.npmjs.com/package/@earendil-works/pi-coding-agent)
environment for Docker Sandboxes (`sbx`): a local Linux image, disposable
sandboxes, and shared host-owned Pi configuration and tool state.

## Start

Host prerequisites: Make, Go **1.25+**, running Docker with buildx, SBX
**v0.39.0+** with Docker sign-in, and `jq`. Put Go's binary directory (`GOBIN`,
normally `~/go/bin`) on PATH, then from this checkout:

```bash
go install github.com/pelletier/go-toml/v2/cmd/tomljson@v2.4.3
make install
```

`make install` checks prerequisites, validates the kit, builds the Go launcher
and host bridge, and builds/imports the image. It then links
`~/.local/bin/pissbox` to this checkout's `bin/pissbox` wrapper.
It updates existing symlinks but refuses regular files/directories; failed
checks/builds preserve the old link. Keep the checkout in place. Missing
dependencies, login, and PATH changes remain manual (`bin/install --help`).

Run `pissbox` from your project directory; use Pi's `/login` inside the sandbox
on first use. Use `pissbox -c` / `pissbox --continue`, or press `c` in the
existing-sandbox menu, to resume the previous Pi session for that workspace.
Menu letters act without Enter; `pissbox -y -c` skips the menu.
Workspace edits persist on the host after sandbox removal.
`make build` builds without installing the launcher link; `make build-image`
builds only the image and needs no host Go or bridge.

For API keys, prefer SBX proxy-managed secrets when supported and configured:
keys stay on the host while the proxy authenticates requests. The agent can
read Pi's `auth.json` and environment variables passed directly into the sandbox.
Use repeatable `--secret ENV_NAME HOST COMMAND` flags for sandbox-scoped custom
credentials; see [proxy-managed secrets](doc/secrets.md) for examples, host
matching, command safety, and creation-only registration. This optional feature
requires SBX `secret set-custom --command` support (verified with v0.50.0).

## Go launcher

`bin/pissbox` is a small Bash wrapper that selects the native Go launcher in
`.cache/bin/pissbox-<os>-<arch>` and executes it without changing arguments or
the caller's directory. Build explicitly after source changes; the wrapper never
rebuilds or falls back. A missing binary reports `bin/build-launcher`.

```bash
make build-launcher      # Explicit native build
make build-launcher-all  # All four supported OS/architecture targets
make test-python         # Fresh Go executable with the production wrapper
```

Acceptance tests build a fresh candidate in isolated fixture storage and exercise
the production wrapper, rather than trusting a previously built executable.
`make check` also runs instrumented Go launcher tests. The former Bash launcher,
its host-side isolation helper, and dual-implementation test selection have been
removed; sandbox setup and shared schema validation still use Bash. See
[host development](host/README.md#launcher-verification) and the
[post-merge human checklist](plan-launcher-migration.md#final-human-checklist--after-merge-to-main)
for pending final verification.

## Transfer built artifacts

Git carries the launcher, kit, and configuration. `bin/artifacts` transfers only
untracked runtime artifacts between checkouts, without rebuilding or downloading
the installed Pi package. It requires **Python 3** on the exporting and importing
machines; normal installation and launching do not require Python.

```bash
# Source checkout; output paths are relative to the caller's working directory.
make export-artifacts ARCHIVE=runtime.tar.gz
# Transfer runtime.tar.gz to the destination machine, then in its checkout:
make import-artifacts ARCHIVE=runtime.tar.gz
bin/build-image --register  # Import this host architecture's restored image into SBX
```

Both Make targets require `ARCHIVE=...` and appear in `make help`.
Direct script usage supports `bin/artifacts --export FILE` / `--import FILE`,
`--export=FILE` / `--import=FILE`, and short flags `-e FILE` / `-i FILE`. Use matching Git revisions on both machines. Stop Pi and
builds during transfers. Only import packages from trusted sources.

The single allowlist in `bin/artifacts` defines archive paths and their placement:

- `.cache/images/pissbox-local-{amd64,arm64}.tar` and `pissbox-local.tar` symlink.
- `.cache/bin/go-bridge-{darwin,linux}-{amd64,arm64}` bridge binaries.
- `.cache/bin/pissbox-{darwin,linux}-{amd64,arm64}` launcher binaries.
- `pi-agent/npm/package.json`, `package-lock.json`, and `node_modules/`.

Missing source artifacts are skipped; an empty export fails. The gzip archive
preserves executable permissions and relative symlinks. Credentials, sessions,
tracked configuration, generated settings links, general caches, and indexes
are not included. The search model is already in the image. Packages can still
contain private/proprietary dependencies: review before sharing publicly.

Export refuses an existing output file. Import validates archive paths, links,
and destination conflicts before writing, and refuses existing artifacts rather
than merging or overwriting them. It restores into the checkout containing the
script, regardless of the caller's directory. An interrupted or failed write can
leave partial artifacts; inspect these before retrying. Nothing loads an image,
recreates a sandbox, installs dependencies, or changes the launcher link
automatically. The destination still needs Docker/SBX and the launcher's `jq`
and `tomljson` dependencies. Supply the image and bridge binaries matching its
architecture, including the launcher binary; installed sandbox npm dependencies
must also support that architecture. A packaged host does not need Go to launch.

## Work across directories

The first directory is primary; additional directories are mounted read-write:

```bash
pissbox . ~/.config/opencode ~/.config/pi
```

See [project configuration (`.pissbox.toml`)](doc/project-configuration.md)
for recurring mounts, first-time setup, and dependency isolation.

## Optional agent tools

`make build` already includes the host bridge. To rebuild only the bridge:

```bash
make build-bridge  # Current host; requires Go 1.25+
bin/pissbox
```

The bridge adds `agent_list` and `agent_send` for discovery and addressed
Pi-to-Pi messaging across active launches. Each session gets a stable callsign
from two different entries in the editable 50-name list at
`pi-agent/extensions/agent-names.txt`. That gives 2,450 ordered pairs. The
bridge keeps `name` as the session title and adds a separate `agentName`
callsign; `agent_list` always returns all peers. Use `agent_send({ to })` with
the callsign or exact agent ID. Herd is not
required; when launched inside Herd, the bridge also reports working, idle, and
blocked states.
`bin/pissbox --bridge-stop` explicitly stops the host bridge without removing
sandboxes; use it when upgrading or recovering from a protocol mismatch.
`make build-bridge-all` cross-compiles for macOS/Linux on arm64/amd64. See the
[bridge guide](host/README.md) for setup and daemon lifecycle.

New interactive sessions also generate a title from the first prompt. Use
`/skill:session-name` to refresh it from the conversation, or `/name My title`
for a manual rename. Automatic naming makes a small additional model request.

A third footer line starts with the agent callsign, then shows a live activity
timer: `Aiden Jasper · working 2m14s` during a run, then
`Aiden Jasper · idle 12m0s` for time since it finished (including interrupted
runs). The right side shows `sent 14:30 · done 14:32 · took 2m14s`; `done` and
`took` stay unknown during a run. Timing covers tools and automatic continuations
until the agent settles; `sent` tracks the latest user message, not terminal focus.
Idle time is unknown until a completed run has been recorded.
Completed timing is saved as display-only session metadata and restored on
resume or reload. Run `/reload` in an existing Pi instance to load the extension.
This uses Pi's shared extension-status line; other status extensions can consume
its space.

## Development

Run from the checkout root. Tests require Go 1.25+, Node.js, Python 3, Pi,
`tomljson`, and `jq` installed on PATH; no Pi login is needed. JavaScript suites
live beside their extensions as `*.test.mjs`; ordinary tests, coverage, and mutation baselines
all discover them recursively. Shared helpers and fixtures remain in `tests/`.

```bash
make help        # All targets and scope options; also the default for make
make check       # Go/JS/Python tests, Go vet, Bash syntax, whitespace checks
make tools       # Explicit network install of pinned quality tools into .cache/
make coverage    # Fresh Go and JavaScript coverage reports
make metrics     # Fresh coverage plus complexity and CRAP risk reports
make mutation-js FILES="pi-agent/extensions/pissbox-bridge.js"
make mutation-go FILES="host/internal/bridge/mailbox.go"
```

`make check` does not install quality tools or run mutation testing. `make tools`
installs Gremlins and go-crap for Go, plus StrykerJS, c8, and crap4ts for
JavaScript. Building these tools requires Go 1.26.2+ (or automatic toolchain
download); JavaScript quality tooling requires Node 22+ and npm.

Mutation targets require exact `FILES` or a `BASE` Git ref; `FILES` overrides
`BASE`. Scope limits mutated source files, not the test suite. Reports live under
`.cache/coverage/`, `.cache/crap/`, and `.cache/mutation/`.
See the [quality guide](tools/quality/README.md) for selection previews, dry runs,
report interpretation, and tool versions.

`make build-image` builds, saves, and imports the host-architecture sandbox image.
`make build-image-all` builds and saves both amd64 and arm64 archives without
importing them; the foreign architecture uses emulation. Use `bin/build-image`
directly for custom flags or archive registration. `make validate-kit` separately
validates the kit with SBX and requires Docker sign-in.

## Guides

- [Recommended read-only GitHub CLI token permissions](doc/github-token.md)
- [Usage, settings, and sandbox lifecycle](sbx-kit/README.md)
- [Image builds, optional CA, and bundled search model](sbx-kit/README.image.md)
- [Shared Linux tool state](sbx-kit/SHARED-DIRECTORIES.md)
- [Agent messaging, optional Herd lifecycle bridge, and Go cross-builds](host/README.md)
- [Known issues](KNOWN_ISSUES.md)
- [Development rules and verification](AGENTS.md)
- [Make targets, scoped mutation testing, coverage, and CRAP metrics](tools/quality/README.md)
