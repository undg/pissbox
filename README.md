# pissbox

<img width="512" height="279" alt="image" src="https://github.com/user-attachments/assets/2153ea65-a15d-49c3-970e-e1a6b3bb937e" />



Personal [Pi coding-agent](https://www.npmjs.com/package/@earendil-works/pi-coding-agent)
environment for Docker Sandboxes (`sbx`): a local Linux image, disposable
sandboxes, and shared host-owned Pi configuration and tool state.

## Start

On the host, with Docker running and SBX **v0.39.0+** installed:

```bash
bin/build-image
bin/pissbox /path/to/project
```

The first command builds and imports `pissbox:local`; the second opens Pi.
Use Pi's `/login` inside the sandbox for GitHub Copilot authentication.
Workspaces are live host mounts: edits persist after sandbox removal.

## Real-life example: port skills between agents

Work on your sandbox Pi, OpenCode, and native Pi setups in one session
(with `pissbox` on your PATH):

```bash
pissbox ~/Code/pissbox ~/.config/opencode ~/.config/pi
```

The first directory is the primary workspace; the others are additional
read-write mounts. Pi can compare all three setups and port skills or tools
between them without copying files into the sandbox first.

For example, ask: “Adapt this OpenCode skill for both my native Pi and
sandbox Pi setups.”

## Guides

- [Usage, settings, and sandbox lifecycle](sbx-kit/README.md)
- [Image builds, optional CA, and bundled search model](sbx-kit/README.image.md)
- [Shared Linux tool state](sbx-kit/SHARED-DIRECTORIES.md)
- [Known issues](KNOWN_ISSUES.md)
- [Development rules and verification](AGENTS.md)
