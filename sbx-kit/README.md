# pissbox — local Pi sandbox kit

Personal Pi environment based on `docker/sandbox-templates:shell-docker`.
The image installs Pi and fd, and links Pi's default agent directory to the
host-owned `pi-agent/` directory. Leave `PI_CODING_AGENT_DIR` unset.

## Build on the host

From the root of this repository:

```bash
PISSBOX_ROOT="$(pwd -P)"
docker build \
  --build-arg SHARED_PI_AGENT_DIR="$PISSBOX_ROOT/pi-agent" \
  -t pissbox/pi-image:local \
  "$PISSBOX_ROOT/sbx-kit"
```

The resulting image contains only a symlink:

```text
/home/agent/.pi/agent → <resolved repository path>/pi-agent
```

No configuration or credentials are copied into the image. Put `settings.json`,
`auth.json`, `extensions/`, `sessions/`, etc. directly in host `pi-agent/`, not
in another nested `agent/` directory. Keep credentials and private state out of Git.

A symlink cannot discover where a repository is mounted. The build argument
supplies that target without embedding your username or repository location in
the Dockerfile. The link is intentionally dangling during the build; the target
becomes available when SBX mounts it. If you move the repository, rebuild the
image with the new path and create a new sandbox.

## Create a sandbox on the host

In the same shell, change into your project's worktree, then run:

```bash
sbx run --name my-pi-sandbox \
  --kit "$PISSBOX_ROOT/sbx-kit" \
  pi . .. "$PISSBOX_ROOT/pi-agent"
```

- `.` is the primary workspace and Pi's starting directory.
- `..` exposes the repository parent, including sibling worktrees and shared
  Git metadata. Omit it for repositories that do not need it.
- The last workspace mounts shared configuration read-write at the same absolute
  path used by the symlink. Mounting the target is mandatory: mounting a symlink
  alone does not expose its target.

Use a new sandbox name when changing workspace mounts or kit setup. Existing
sandboxes do not automatically pick up image or kit changes.

The kit references `pissbox/pi-image:local`, not the upstream image. Building and
image availability in your host SBX installation must be verified there; this
repository's shell tests do not exercise SBX image loading.

## Authentication and shared state

Run `/login` in Pi and choose GitHub Copilot. Pi writes to the shared `auth.json`.
The upstream Anthropic credential integration has been removed so it cannot
materialize a replacement authentication file at sandbox startup.

The kit allows the common GitHub Copilot login and inference domains plus npm.
Enterprise endpoints or extensions may require additional network allowances;
central organization restrictions still apply.

The entire agent directory is shared read-write, including credentials,
settings, extensions, and conversation sessions. All attached agents can read
and modify it. Host files survive sandbox removal; sandbox-local files do not.
Avoid running host/macOS and sandbox/Linux package installs into the same shared
agent directory, since dependencies may be platform-specific.

## Checks

```bash
python3 -m unittest discover -s sbx-kit/testdata -p 'test_*.py'
```

These checks exercise the Dockerfile's symlink command without building an
image. After creating a real sandbox, verify from the host:

```bash
sbx exec my-pi-sandbox -- sh -c 'readlink "$HOME/.pi/agent"; test -d "$HOME/.pi/agent"'
```

Building the full image and confirming `/login` persistence across sandbox
restarts are separate integration checks.
