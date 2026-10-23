# pissbox

<img width="512" height="279" alt="image" src="https://github.com/user-attachments/assets/2153ea65-a15d-49c3-970e-e1a6b3bb937e" />

Personal [Pi coding-agent](https://www.npmjs.com/package/@earendil-works/pi-coding-agent)
environment for Docker Sandboxes (`sbx`): a local Linux image, disposable
sandboxes, and shared host-owned Pi configuration and tool state.

## Start

From the checkout root on the host:

```bash
make install
```

The installer checks for Make, Go **1.25+**, Docker with buildx, SBX **v0.39.0+**,
and `jq`, checks Docker connectivity, and validates the kit (including the
Docker sign-in required by SBX). It does not install missing dependencies or
perform login automatically. It then runs `make build` to build the host bridge
and build/import the sandbox image, and links `~/.local/bin/pissbox` to this
checkout's launcher.

Existing symlinks are repointed to this checkout, including broken links;
matching links are unchanged. Regular files and directories are never replaced.
Failed checks or builds leave the existing launcher link unchanged. Keep the
checkout in place. If `~/.local/bin` is missing from `PATH`, the installer warns
without modifying shell configuration. `bin/install --help` documents the flow.

After installation, run `pissbox` from your project directory. No arguments
needed: it opens Pi in the current directory. Authenticate using Pi's `/login`
inside the sandbox on first use. Workspaces are live host mounts: edits persist
after sandbox removal.

For build-only use without creating a launcher link, run `make build`.
Use `make build-image` instead if you do not need the optional bridge or Go.

For API keys, prefer SBX proxy-managed secrets when supported and configured:
keys stay on the host while the proxy authenticates requests. The agent can
read Pi's `auth.json` and environment variables passed directly into the sandbox.

## Work across directories

Add directories when you need them (with multiple define current one with `.`):

```bash
pissbox . ~/.config/opencode ~/.config/pi
```

`.` is your workspace; the other directories are also mounted read-write.
Ask Pi: “Adapt this OpenCode skill for my native and sandbox Pi setups.”

## Optional agent tools

`make build` already includes the host bridge. To rebuild only the bridge:

```bash
make build-bridge  # Current host; requires Go 1.25+
bin/pissbox
```

The bridge adds `agent_list` and `agent_send` for discovery and addressed
Pi-to-Pi messaging across active launches. Herd is not required; when launched
inside Herd, the bridge also reports working, idle, and blocked states.
`make build-bridge-all` cross-compiles for macOS/Linux on arm64/amd64.
See the [bridge guide](go-bridge/README.md) for setup and daemon lifecycle.

New interactive sessions also generate a title from the first prompt. Use
`/skill:session-name` to refresh it from the conversation, or `/name My title`
for a manual rename. Automatic naming makes a small additional model request.

A third footer line shows a live activity timer on the left: `working 2m14s`
during a run, then `idle 12m0s` for time since it finished (including interrupted
runs). The right side shows `sent 14:30 · done 14:32 · took 2m14s`; `done` and
`took` stay unknown during a run. Timing covers tools and automatic continuations
until the agent settles; `sent` tracks the latest user message, not terminal focus.
Idle time is unknown until a completed run has been recorded.
Completed timing is saved as display-only session metadata and restored on
resume or reload. Run `/reload` in an existing Pi instance to load the extension.
This uses Pi's shared extension-status line; other status extensions can consume
its space.

## Development

Run from the checkout root. Tests require Go 1.25+, Node.js, Python 3, and Pi
installed on PATH; no Pi login is needed. JavaScript suites live beside their
extensions as `*.test.mjs`; ordinary tests, coverage, and mutation baselines
all discover them recursively. Shared helpers and fixtures remain in `tests/`.

```bash
make help        # All targets and scope options; also the default for make
make check       # Go/JS/Python tests, Go vet, Bash syntax, whitespace checks
make tools       # Explicit network install of pinned quality tools into .cache/
make coverage    # Fresh Go and JavaScript coverage reports
make metrics     # Fresh coverage plus complexity and CRAP risk reports
make mutation-js FILES="pi-agent/extensions/pissbox-bridge.js"
make mutation-go FILES="go-bridge/mailbox.go"
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

- [Usage, settings, and sandbox lifecycle](sbx-kit/README.md)
- [Image builds, optional CA, and bundled search model](sbx-kit/README.image.md)
- [Shared Linux tool state](sbx-kit/SHARED-DIRECTORIES.md)
- [Agent messaging, optional Herd lifecycle bridge, and Go cross-builds](go-bridge/README.md)
- [Known issues](KNOWN_ISSUES.md)
- [Development rules and verification](AGENTS.md)
- [Make targets, scoped mutation testing, coverage, and CRAP metrics](tools/quality/README.md)
