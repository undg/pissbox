# pissbox

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

## Guides

- [Usage, settings, and sandbox lifecycle](sbx-kit/README.md)
- [Image builds, optional CA, and bundled search model](sbx-kit/README.image.md)
- [Shared Linux tool state](sbx-kit/SHARED-DIRECTORIES.md)
- [Known issues](KNOWN_ISSUES.md)
- [Development rules and verification](AGENTS.md)
