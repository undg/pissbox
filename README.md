# pissbox

<img width="512" height="279" alt="image" src="https://github.com/user-attachments/assets/2153ea65-a15d-49c3-970e-e1a6b3bb937e" />

Personal [Pi coding-agent](https://www.npmjs.com/package/@earendil-works/pi-coding-agent)
environment for Docker Sandboxes (`sbx`): a local Linux image, disposable
sandboxes, and shared host-owned Pi configuration and tool state.

## Start

On the host, with Docker running and SBX **v0.39.0+** installed:

```bash
bin/build-image
bin/pissbox
```

Build once, then run `pissbox` from your project directory.
No arguments needed: it opens Pi in the current directory.
Workspaces are live host mounts: edits persist after sandbox removal.

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

Build the host bridge, then launch normally:

```bash
make build       # Current host; requires Go 1.25+
bin/pissbox
```

The bridge adds `agent_list` and `agent_send` for discovery and addressed
Pi-to-Pi messaging across active launches. Herd is not required; when launched
inside Herd, the bridge also reports working, idle, and blocked states.
`make build-all` cross-compiles for macOS/Linux on arm64/amd64.
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
installed on PATH; no Pi login is needed.

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

Image builds remain `bin/build-image`. `make validate-kit` separately validates
the kit with SBX and requires Docker sign-in.

## Guides

- [Usage, settings, and sandbox lifecycle](sbx-kit/README.md)
- [Image builds, optional CA, and bundled search model](sbx-kit/README.image.md)
- [Shared Linux tool state](sbx-kit/SHARED-DIRECTORIES.md)
- [Agent messaging, optional Herd lifecycle bridge, and Go cross-builds](go-bridge/README.md)
- [Known issues](KNOWN_ISSUES.md)
- [Development rules and verification](AGENTS.md)
- [Make targets, scoped mutation testing, coverage, and CRAP metrics](tools/quality/README.md)
