# Fresh-sandbox handoff

## Read this first

This is a handoff from the previous Pi session, including context inherited from
an earlier agent. It records decisions and observations, not proof that the new
sandbox workflow works. Read root `AGENTS.md`, `KNOWN_ISSUES.md`, and the current
kit files before making changes. User edits may have happened since this handoff.

**Immediate goal:** finish the necessary adjustments, then help the user create
and verify a fresh sandbox. Do not delete the old sandbox, overwrite shared
configuration, or migrate credentials without permission.

## User intent and decisions

- The project is **pissbox**: a personal, reproducible Linux toolset for Pi inside
  disposable Docker Sandboxes (`sbx`). The directory was still
  `/Users/bartek_laskowski/Code/pisbx` when this was written; the user plans to
  rename it. Calculate paths rather than hard-code this location.
- Start simple. An Arch-like environment is a future preference, not a selected
  base image. The current image uses Docker's shell-docker template.
- The user is considering taking full ownership of this derived kit, rather than
  keeping it close to upstream. This direction was discussed, not fully executed.
- Keep workspace mounting separate from shared Pi configuration.
- One host-owned `pi-agent/` directory is shared read-write across sandboxed Pi
  instances. Sharing credentials, sessions, extensions, and settings is deliberate.
- Use Pi's default `/home/agent/.pi/agent` through a symlink. Leave
  `PI_CODING_AGENT_DIR` unset; previous attempts with it had inconsistent discovery.
- Authenticate with GitHub Copilot using Pi's `/login` inside the sandbox.
- The user will copy important existing configuration into host `pi-agent/`.
  Do not assume that copying is complete. Files belong directly in `pi-agent/`,
  not a nested `agent/` directory.
- Keep `pi-agent/AGENTS.md` short and universal: it is loaded by all sessions.
  Repository-specific guidance belongs in root `AGENTS.md`.
- The existing `pi-agent/APPEND_SYSTEM.md` is the user's behavior/personality
  prompt. It was read, but not edited. Do not rewrite it during migration.
- Be concise. Do not add frameworks or abstractions without a concrete need.

## What was changed in this session

### `sbx-kit/Dockerfile`

Added required build argument `SHARED_PI_AGENT_DIR` and a build-time symlink:

```text
/home/agent/.pi/agent → <SHARED_PI_AGENT_DIR>
```

The command rejects empty and relative targets. `ln -sT` refuses to silently
nest the link inside an existing directory. No shared configuration or secrets
are copied into the image. The target is intentionally absent during the build
and must be mounted at sandbox creation.

This avoids hard-coding the host path in source, but the built image still
contains an absolute target. Moving the repository requires rebuilding the image
and creating a new sandbox. A runtime-discovery solution was not implemented.

The rest of the Dockerfile largely remains upstream code, including stale
comments about nightly CI and publishing infrastructure absent from this repo.

### `sbx-kit/spec.yaml`

- Image changed from upstream `docker.io/sbx/pi-image:latest` to
  `pissbox/pi-image:local`.
- Removed upstream Anthropic credentials declaration, including its startup
  `auth.json` materialization. That could overwrite shared Copilot credentials.
- Added common Copilot login/inference domains; kept npm access.
- Kept the creation-time npm proxy configuration.
- Kit name/entrypoint remain `pi`. Invoke it as `sbx run ... pi ...`.
- **Still missing:** `pi.dev` in the network allowlist.
- SBX schema acceptance and endpoint coverage have not been integration-tested.

### Documentation and instructions

- Rewrote `sbx-kit/README.md` around the local shared-state workflow.
- Updated `sbx-kit/README.image.md` for the custom image and build argument.
- Added root `AGENTS.md` with durable repository guidance.
- Replaced outdated permission-extension notes in `pi-agent/AGENTS.md` with four
  short general environment/safety rules. There is no verified active permission
  extension behind those notes.
- Added `KNOWN_ISSUES.md` for model-refresh failures and integration checks.
- `bin/` remains empty except `.gitkeep`; no launcher has been implemented.

### Tests

Added permanent regression tests in `sbx-kit/testdata/test_shared_agent.py`, using
Python's built-in `unittest`; no additional test dependencies.

They extract and execute the Dockerfile's actual symlink shell command in
isolated temporary directories. Three tests cover:

1. Absolute target with spaces, dangling link, and writes reaching the target
   once the directory exists.
2. Rejection of missing/relative targets.
3. Refusal to replace or nest inside an existing agent directory, preserving data.

**These tests were run in this session: all three passed.** `git diff --check`
also passed after edits. They are not disposable tests, but they are not a Docker
or SBX integration suite.

```bash
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover \
  -s sbx-kit/testdata -p 'test_*.py'
git diff --check
```

No full image build or fresh sandbox creation was performed. Docker responded to
`docker info`; the `sbx` CLI was not available inside this running sandbox.

## Current blockers and observed failures

### Model catalog refresh

The user asked the agent to run `pi update --models` because they did not have
shell access at that moment. Two attempts were made:

1. Failed with HTTP 403 for Anthropic and Copilot. A direct request confirmed
   `pi.dev:443` was blocked by default-deny network policy.
2. After the user adjusted network access, failed with `fetch failed` for both
   providers. Curl then reported an untrusted certificate chain:
   `unable to get local issuer certificate (20)`.

The tested endpoint was:

```text
https://pi.dev/api/models/providers/github-copilot?types=chat,image,classifier
```

The catalog was **not successfully refreshed**. Add `pi.dev` to the kit and
investigate CA trust for Node and curl. Do not disable TLS verification or assume
the precise certificate problem is already diagnosed. A current-sandbox policy
allowance alone does not update the source kit.

### Unexpected Anthropic configuration in the old sandbox

The running sandbox was observed to have:

```text
ANTHROPIC_API_KEY: equal to the upstream proxy-managed placeholder
SBX_CRED_ANTHROPIC_MODE: none
```

No secret value was printed. This is a stale upstream credential binding, not
proof the user has an Anthropic account/key. Pi sees the environment variable as
configured, explaining the unwanted Anthropic catalog-refresh attempts. It may
also explain startup authentication errors; the exact startup message was not
captured, so that link remains an inference.

The local spec no longer declares this binding. Existing sandboxes do not lose
bindings when source files change. A fresh sandbox should be checked for absence
of the unwanted variable. Do not blindly delete Anthropic entries from shared
`auth.json`; that file was not inspected for this diagnosis.

A temporary launch workaround was suggested, not executed:

```bash
env -u ANTHROPIC_API_KEY pi
```

## Migration checklist

### Before creating the new sandbox

- [ ] Confirm the final repository location/name before building the symlink.
- [ ] Confirm the user's important configuration/state is in host `pi-agent/`.
      Preserve private files; do not dump their contents into chat.
- [ ] Add `pi.dev` to the kit network allowlist.
- [ ] Investigate the certificate trust failure; check whether a fresh template
      fixes it before inventing certificate-copying machinery.
- [ ] Review the kit and docs together for necessary final adjustments.
- [ ] If taking ownership now, remove misleading upstream publishing comments,
      record source provenance, and check the upstream license before deciding
      which notices must be retained. Licensing/provenance review is not done.
- [ ] Review Git status. The user has staged/committed some files during this
      session; do not assume every change is ours or stage everything blindly.

Original upstream kit reference:

```text
git+https://github.com/docker/sbx-kits-contrib.git#dir=pi
```

The copied upstream revision was not recorded. Do not invent one.

### Build on the host

Run from the final pissbox repository root:

```bash
PISSBOX_ROOT="$(pwd -P)"
docker build \
  --build-arg SHARED_PI_AGENT_DIR="$PISSBOX_ROOT/pi-agent" \
  -t pissbox/pi-image:local \
  "$PISSBOX_ROOT/sbx-kit"
```

Pi defaults to npm's `latest` at build time. `--build-arg PI_VERSION=<version>`
can pin it. Local kit files do not eliminate base-image or package downloads.
Confirm that the host SBX installation can access the locally built image;
this has not been verified. Do not silently fall back to the upstream image,
which lacks the new symlink.

### Create with a new name

From the target project's worktree, in the same host shell:

```bash
sbx run --name <new-sandbox-name> \
  --kit "$PISSBOX_ROOT/sbx-kit" \
  pi . .. "$PISSBOX_ROOT/pi-agent"
```

`.` is primary and sets Pi's starting directory. `..` exposes shared Git metadata
for the user's bare-repo/sibling-worktree layout. Omit it when unnecessary.
The user accepts exposing siblings for simplicity, not modifying unrelated work.

For a sandbox working on pissbox itself, the repository can be the primary
workspace; exposing its parent is not automatically necessary. Ensure the shared
agent target is accessible and check SBX's handling of overlapping workspace
paths rather than claiming it has been tested.

### Verify in the fresh sandbox

- [ ] Read this file and root `AGENTS.md`; re-check actual files and Git state.
- [ ] Check `/home/agent/.pi/agent` is a symlink with the intended mounted target.
- [ ] Confirm `PI_CODING_AGENT_DIR` is unset.
- [ ] Check unwanted `ANTHROPIC_API_KEY` injection is absent, without printing
      credential values. If present, investigate actual runtime bindings.
- [ ] Confirm shared configuration is discovered; do not overwrite it to test.
- [ ] Use Pi's `/login` for GitHub Copilot if needed, then `/model`.
- [ ] Retry `pi update --models` and verify success; don't equate a changed error
      with resolution.
- [ ] Verify a real Copilot response.
- [ ] With permission, restart and verify credentials and sessions persist.
- [ ] With permission, verify a second/replacement sandbox can use shared state.
- [ ] Update `KNOWN_ISSUES.md` and root `AGENTS.md` with verified results.
- [ ] Keep the old sandbox until the user agrees it is safe to remove.

Host-side link check:

```bash
sbx exec <new-sandbox-name> -- sh -c \
  'readlink "$HOME/.pi/agent"; test -d "$HOME/.pi/agent"'
```

## Inherited SBX context

- Host home is `/Users/bartek_laskowski`; sandbox home is `/home/agent`.
- Normal workspaces are live mounts at preserved absolute paths, not copies.
  Host files survive sandbox deletion; sandbox-local state does not.
- Additional workspaces can use `:ro`, but standard syntax does not provide
  arbitrary Docker-style destination mapping. `sbx cp` is a real independent copy.
- New workspaces cannot be added to an existing sandbox. This exact failure was
  encountered previously: `already exists and can't be given new workspaces`.
  A new name creates a separate sandbox without deleting the old one.
- Sandbox instances and Pi conversation sessions are distinct. This handoff
  preserves context even if the current conversation is not migrated.
- The old sandbox reported direct workspace mode: edits reach the host directly.
  Check the fresh environment rather than assuming its mode or sandbox name.
- Pi has no dedicated browser/web-fetch tool by default. Earlier web access used
  curl, not browser rendering; GitHub returned HTTP 200 in an earlier session.
- Never confuse sandbox `localhost` with the host. Follow the environment's SBX
  instructions for network policy, host services, ports, and Git credentials.
