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

## Guides

- [Usage, settings, and sandbox lifecycle](sbx-kit/README.md)
- [Image builds, optional CA, and bundled search model](sbx-kit/README.image.md)
- [Shared Linux tool state](sbx-kit/SHARED-DIRECTORIES.md)
- [Agent messaging, optional Herd lifecycle bridge, and Go cross-builds](go-bridge/README.md)
- [Known issues](KNOWN_ISSUES.md)
- [Development rules and verification](AGENTS.md)
- [Make targets, scoped mutation testing, coverage, and CRAP metrics](tools/quality/README.md)
