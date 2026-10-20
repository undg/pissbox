# pissbox development

Personal Pi environment for Docker Sandboxes (`sbx`), using a locally built
Linux image and host-owned Pi/tool state. Do not hard-code the checkout's path
or directory name. User guides start at [README.md](README.md).

## Layout

- `bin/pissbox`: host launcher; `bin/build-image`: build, save, and SBX import.
- `sbx-kit/`: Dockerfile, kit spec, and operational guides.
- `sbx-kit/files/home/.pissbox/`: small setup scripts and shared-directory mappings.
- `pi-agent/`: shared sandbox-only configuration, credentials, and sessions.
  Keep its `AGENTS.md` brief and general; repository rules belong here.
- `pi-agent/APPEND_SYSTEM.md`: user-authored personality prompt; do not rewrite
  during environment work. Use the `bash-cli-style` skill for Bash changes.
- `.cache/`: gitignored, host-owned Linux tool state and test fixtures.
- `tests/`: Python-standard-library regression tests.

## Image and kit constraints

- `spec.yaml` uses `pissbox:local`. `bin/build-image` builds the host
  architecture (or `--amd64` / `--arm64`), saves
  `.cache/images/pissbox-local-<arch>.tar`, points the `pissbox-local.tar`
  symlink at it, and imports into SBX's separate image store unless flags say
  otherwise. `--register [arch]` imports an existing archive. Builds are native
  per host; cross-architecture builds stay manual.
  It resolves symlinks and forwards Docker options after `--`. The launcher
  does not build images. Existing sandboxes are never updated automatically.
- The optional public PEM CA at `sbx-kit/ca.crt` is passed by the build wrapper
  as BuildKit secret `custom_ca` and installed before npm/curl. It remains in
  the image. Secret changes do not invalidate build cache: use `-- --no-cache`
  after adding, changing, or removing it. Never use private keys or credentials.
- Do not bake machine-specific config paths or symlinks into the image.
  Kit setup links `$HOME/.pi/agent` as user `1000` before Pi starts.
  Keep `entrypoint: [pi, --offline]` and leave `PI_CODING_AGENT_DIR` unset.
- Install tool binaries in the image, not `files/`: bundling the large colgrep
  binary there previously made sandbox creation much slower. SBX only copies
  static kit files under `files/home/` and `files/workspace/`.
- Colgrep uses the upstream unmanaged installer, pinned by `COLGREP_VERSION`.
  Keep its `/usr/local/bin` symlink usable in minimal-PATH contexts and avoid
  installer-created profiles/config that conflict with shared links.
- Search weights are pinned by `COLGREP_MODEL_REVISION` and kept at
  `/opt/pissbox/models/LateOn-Code-edge`, outside shared mounts. Setup seeds
  colgrep's local-model default only if no config file or symlink exists.
  Never reset settings or indexes during setup. `colgrep set-model` clears
  indexes; changing weights at the stable path also requires index rebuilds.
  Sandboxes sharing this default must contain the model. ONNX Runtime and
  optional/custom models still need network downloads.

## Launcher and lifecycle contracts

- Resolve launcher symlinks before locating resources; preserve the caller's
  working directory. First workspace is primary; with none, use the current
  directory. Extra workspaces retain host absolute paths, not arbitrary Docker
  source/destination mappings. Mount symlink targets explicitly.
- Keep the `case` parser: options may appear among workspaces; `--` ends parsing.
  Repeated settings use the last value. Directory precedence is CLI flag >
  `PISSBOX_PI_DIR` / `PISSBOX_STATE_DIR` > checkout defaults. Canonicalize before
  mounting; relative overrides use the caller's directory. Do not discover
  native Pi configuration or use `PI_HOST_CONFIG_DIR`.
- Pass the local kit, workspaces, Pi directory, and state root to `sbx run`, with
  `--env PISSBOX_PI_DIR=...` and `--env PISSBOX_STATE_DIR=...`. SBX selects the
  sandbox unless `--name` is supplied; do not add custom discovery/bootstrap.
- On the known existing-sandbox conflict, use the name reported by SBX. Enter
  or `p` runs `sbx run --name NAME`; `s` runs `sbx exec -it NAME bash`; `q` exits.
  Choices are case-insensitive. `d` warns about local data loss, requires a
  separate explicit yes, then runs `sbx rm --force NAME` without recreating.
  Invalid input retries; EOF aborts. Other errors preserve output and status.
- Workspaces are live host mounts. New mounts, mappings, kit files, or images
  require a new sandbox name. Never silently remove/recreate a sandbox.
  Sandbox-local data survives restart, not removal; mounted state survives both.

## Shared configuration and state

- Mount the sandbox-only `pi-agent/` directly, not a nested `agent/`. Sharing
  credentials and sessions read-write across sandboxes is intentional. Keep it
  separate from native Pi. Do not read/print credentials to debug mounts.
- GitHub Copilot login uses Pi's `/login` inside the sandbox. Do not restore
  upstream Anthropic credential injection: it can overwrite the shared auth
  store at startup. Never print/commit credentials or reset shared state outside
  the task; review generated/private files before staging.
- `pi-agent/settings.json` is an ignored generated link. The host-name map in
  `settings_profile_for_host` selects `settings-go.json` for `cm`/`di-7415`,
  otherwise `settings-co.json`, before every launch. Add hosts there and track
  new `settings-*.json` profiles rather than editing the generated link.
  Regular settings files are backed up as `settings.json.replaced*`. Pi writes
  through the link into the tracked profile, including runtime defaults.
- Shared Linux state defaults to `.cache/`. Keep it separate from native/macOS
  caches. Add tool mappings in
  `sbx-kit/files/home/.pissbox/shared-directories.conf`, not per-tool launcher
  code. Rows are literal `state-relative|home-relative` paths without dot
  segments or expansion. Setup accepts matching links and refuses existing
  data/different links; never delete or migrate data silently.
- Custom tool/XDG paths may bypass shared links. Avoid concurrent colgrep index
  writers until upstream locking is verified. Preserve old state-path links
  while existing sandboxes depend on them. See
  [shared directories](sbx-kit/SHARED-DIRECTORIES.md) for current mappings and
  [known issues](KNOWN_ISSUES.md) for the missing npm cache mapping.

## Verification

Read affected kit files before editing. Run:

```bash
bash -n bin/pissbox bin/build-image sbx-kit/files/home/.pissbox/scripts/*.sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s tests -v
git diff --check
sbx kit validate ./sbx-kit
```

The launcher requires SBX v0.39.0+ for `--env`; upgrade old hosts rather than
removing required variables. SBX v0.48.0 requires Docker sign-in even for kit
validation; installing the CLI or reading help is not validation.

Tests stub Docker/SBX and use isolated homes for link scripts. Fixtures stay
under ignored `.cache/test-fixtures/`. They cover argument forwarding, symlink
launches, menu/deletion/EOF behavior, terminal handling, settings profiles,
shared links, and refusal to replace data. Do not claim a real image build,
sandbox launch, authentication, or persistence check without running it.
Keep operational details in repository docs, not `pi-agent/AGENTS.md`.
