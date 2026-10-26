# pissbox development

Personal Pi environment for Docker Sandboxes (`sbx`), using a locally built
Linux image and host-owned Pi/tool state. Do not hard-code the checkout's path
or directory name. User guides start at [README.md](README.md).

## Layout

- `bin/pissbox`: native Go executable-selection wrapper; `bin/pissbox-legacy`:
  explicit Bash compatibility reference until manual acceptance.
  `bin/build-image`: build, save, and SBX import.
- `bin/install`: check host dependencies, build, and link `~/.local/bin/pissbox`.
  Repoint existing symlinks, but never overwrite regular files or directories.
- `sbx-kit/`: Dockerfile, kit spec, and operational guides.
- `sbx-kit/files/home/.pissbox/`: small setup scripts and shared-directory mappings.
  `scripts/project-setup.sh` contains validation and sandbox setup. The host
  invokes `--validate FILE` for JSON data only; keep validation in this one file.
- `pi-agent/`: shared sandbox-only configuration, credentials, and sessions.
  Keep its `AGENTS.md` brief and general; repository rules belong here.
- `pi-agent/APPEND_SYSTEM.md`: user-authored personality prompt; do not rewrite
  during environment work. Use the `bash-cli-style` skill for Bash changes.
- `.cache/`: gitignored, host-owned Linux tool state, bridge binaries, and test fixtures.
- `host/`: host Go module; bridge entrypoint in `cmd/pissbox-bridge`, implementation
  and tests in `internal/bridge`. Default launcher entrypoint in `cmd/pissbox`,
  implementation/tests in `internal/launcher`. Requires Go 1.25+ to build.
  Use `bin/build-bridge` / `bin/build-launcher`
  for native binaries, or `--all` for all four supported targets.
- `tests/`: Python-standard-library regression tests, shared JS helpers/tests, and fixtures.
  Keep JS suites beside their extension/library as `*.test.mjs`; Make, coverage,
  and mutation runners discover them recursively. Go tests stay beside Go source.

## Dependency changes

When adding, removing, or changing a dependency or its minimum version, review
`bin/install` in the same change. Keep host prerequisite checks, installer help,
`tests/test_install.py`, and README requirements aligned. Sandbox-only tools
belong in `sbx-kit/Dockerfile`, not in host prerequisite checks; update image
pins/checksums and affected tests/docs as needed. The installer checks host
dependencies but never installs them automatically.

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

- Read only the primary workspace's `.pissbox.toml` on the host, before settings,
  shared state, bridge registration, or SBX launch. Host `tomljson` and `jq` are
  required when the config exists; keep `bin/install` checks and docs aligned.
  Parse as data, never execute `setup` on the host. `workspaces` adds directories
  after CLI workspaces, without changing the primary; paths are literal and
  relative to the primary project, not the config symlink target. Validate
  configured directories and deduplicate physical directory references in order.
  No path arguments (including with `-y`) means current directory plus configured
  extras. `--ignore-config-workspaces` skips configured mounts and directory
  checks, not TOML/schema validation, CLI/shared mounts, or setup execution.
  Help/list/delete bypass config. Existing mounts never change on attach.
  A symlinked config must declare mounts exposing its target inside the sandbox;
  do not automatically mount its target or search parent/additional configs.
- Resolve launcher symlinks before locating resources; preserve the caller's
  working directory. First workspace is primary; with none, use the current
  directory. Extra workspaces retain host absolute paths, not arbitrary Docker
  source/destination mappings. Mount symlink targets explicitly.
- Keep linear option parsing (the legacy Bash reference uses `case`): options may
  appear among workspaces; `--` ends parsing.
  Repeated settings use the last value. Directory precedence is CLI flag >
  `PISSBOX_PI_DIR` / `PISSBOX_STATE_DIR` > checkout defaults. Canonicalize before
  mounting; relative overrides use the caller's directory. Do not discover
  native Pi configuration or use `PI_HOST_CONFIG_DIR`.
- Pass the local kit, workspaces, Pi directory, and state root to `sbx run`, with
  `--env PISSBOX_PI_DIR=...`, `--env PISSBOX_STATE_DIR=...`, and
  `--env PISSBOX_WORKSPACE_DIR=...` for the primary project. SBX selects the
  sandbox unless `--name` is supplied; do not add custom discovery/bootstrap
  except the narrow opted-in isolation handshake below.
- `isolated_directories` is opt-in intent, relative to the primary workspace.
  Omitted/empty keeps the ordinary `sbx run` creation path. Keep the replaceable
  hidden-mount mechanism in `host/internal/launcher/isolation.go` (legacy reference
  in `bin/lib/isolated-directories.sh`): create with project
  setup deferred, receive runtime `SANDBOX_NAME` through a unique mounted launch
  mailbox, mount fresh per-instance backing directories, verify markers before
  writable probes, then finalize the same project setup as UID 1000 before Pi.
  Never guess names, parse creation summaries, discover unrelated sandboxes,
  remount existing instances, or automatically replay failed setup. The mailbox
  contract and hidden mounts are version-tested behavior, not upstream guarantees.
  Preserve nameless creation; `--name` remains optional.
  Existing Pi/shell attaches run a small inline readiness probe using persisted
  isolation environment plus a sandbox-local ready marker, even if current config
  removed isolation. The probe's `sbx exec` can start a stopped sandbox and run
  its existing startup hooks. Old kits without that environment pass. Quit/delete
  never require readiness; failures need a new sandbox or explicit user deletion.
- On the known existing-sandbox conflict, use the name reported by SBX. Enter
  or `p` runs `sbx run --name NAME`; `c` adds `-- --continue` to resume Pi;
  `s` runs `sbx exec -it NAME bash`; `q` exits.
  Choices are case-insensitive single keypresses; Enter selects Pi. `d` warns about local data loss, requires a
  separate explicit yes, then runs `sbx rm --force NAME` without recreating.
  Invalid input retries; EOF aborts. Other errors preserve output and status.
- The optional host bridge provides agent messaging without Herd and lifecycle
  reporting with a complete optional Herd binding. It is one persistent host daemon
  per OS user, independent of checkout/state root. Launchers own opaque mailbox
  registrations, never the daemon PID. Cleanup unregisters; only explicit bridge `stop` terminates it.
  Keep the control socket host-only, retain the filesystem mailbox boundary, and
  never silently replace an incompatible live daemon. After daemon failure,
  existing Pi sessions need launcher reattach; registrations are memory-only.
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
- If suspicious untracked or unstaged files could expose credentials, sessions,
  or other private data, alert the user before staging. Report paths, never
  secret contents. Do not delete files or change ignore rules without asking.
- `pi-agent/settings.json` is an ignored generated link. The host-name map
  `settingsProfile` in `host/internal/launcher/project.go` selects `settings-go.json`
  for `cm`/`di-7415`, otherwise `settings-co.json`, before every launch. Add hosts there and track
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

Use the global `code-quality-metrics` skill for the assessment workflow and
[quality tooling](tools/quality/README.md) for repository-specific details.
Run from the checkout root. **Prefer the existing Make targets and repository
wrappers over bespoke commands or direct Gremlins/Stryker/coverage invocations.**
They preserve pinned tools, source selection, test isolation, and report paths.
Use a lower-level command only when a target cannot express a needed focused
check or when diagnosing the wrapper itself; explain the exception.

Read affected kit files before editing. Start with `make help`; use:

| Command | Purpose |
| --- | --- |
| `make check` | Ordinary tests, Go vet, Bash syntax, and whitespace checks |
| `make test` | All suites wired into the Makefile |
| `make test-go` | Go tests with race detection and no result cache |
| `make test-js` | Extension lifecycle tests using installed Pi |
| `make test-python` | Launcher, setup, and quality-wrapper regression tests |
| `make vet syntax` | Go vet and Bash syntax only |
| `make validate-kit` | SBX kit validation; requires Docker sign-in |
| `make install` | Check host prerequisites, build, and install the launcher symlink |
| `make build` | Build the host launcher/bridge and build/save/import the host image |
| `make build-image` / `make build-image-all` | Host image build/save/import / both architectures without import |
| `make build-bridge` / `make build-bridge-all` | Host bridge build / all supported cross-builds |
| `make build-launcher` / `make build-launcher-all` | Go launcher build / all supported cross-builds |
| `make tools` | Explicit network installation of pinned quality tools |
| `make coverage` | Fresh Go and JS coverage reports |
| `make coverage-go` / `make coverage-js` | Coverage for one language |
| `make metrics BASE=origin/main` | Fresh coverage and scoped CRAP/complexity reports |
| `make crap-go FILES="host/internal/bridge/mailbox.go"` | CRAP report for one exact Go source file |
| `make crap-js FILES="pi-agent/extensions/session-name.js"` | CRAP report for one exact extension |
| `make mutation BASE=origin/main` | Mutants for feature changes in both languages |
| `make mutation-go FILES="host/internal/bridge/mailbox.go host/internal/bridge/herdr.go"` | Mutants for exact Go files |
| `make mutation-js FILES="pi-agent/extensions/pissbox-bridge.js"` | Mutants for one exact extension |

Replace `origin/main` with the actual feature base; do not assume or fetch it.
`FILES` overrides `BASE` and accepts exact paths, not globs. Mutation targets
require a scope; never widen an empty selection. Scope limits mutated files or
reported CRAP functions, not the corresponding test suite. Add `DRY_RUN=1` to a
mutation command for discovery/baseline checks, not a mutation score.

Preview selection without running tools through the existing wrapper:

```bash
python3 tools/quality/run.py mutation-go --base origin/main --list
python3 tools/quality/run.py mutation-js --base origin/main --list
```

Use unscoped `make metrics` / `make crap` or `make mutation-all` only for a
deliberate full audit. Reports live under `.cache/coverage/`, `.cache/crap/`, and
`.cache/mutation/`; verify freshness and scope before quoting results. Do not run
concurrent writers for the same language/report. `make check` never installs
quality tools or runs mutants. `make fmt` and `make tidy` modify files; they are
not read-only checks.

The launcher requires SBX v0.39.0+ for `--env`; upgrade old hosts rather than
removing required variables. SBX v0.48.0 requires Docker sign-in even for kit
validation; installing the CLI or reading help is not validation.

During migration, `PISSBOX_TEST_LAUNCHER=bash make test-python` runs the explicit
`bin/pissbox-legacy` reference; `make test-python` defaults to Go, building one
fresh isolated candidate and exercising the production wrapper.
Both use fixture checkouts without ambient project configuration or credentials.
Go launcher process tests re-execute their instrumented test binary. Copied-module
tests need `PISSBOX_LAUNCHER_TEST_RESOURCE_DIR` pointing at this checkout for the
shared schema validator; the mutation wrapper sets it explicitly.

Tests stub Docker/SBX and use isolated homes for link scripts. Fixtures stay
under ignored `.cache/test-fixtures/`. Bridge process tests must set an isolated
`PISSBOX_BRIDGE_RUNTIME_DIR` there and stop their daemons in cleanup; never
contact or stop the user's actual bridge. They cover argument forwarding, symlink
launches, menu/deletion/EOF behavior, terminal handling, settings profiles,
shared links, and refusal to replace data. Do not claim a real image build,
sandbox launch, authentication, or persistence check without running it.
Keep operational details in repository docs, not `pi-agent/AGENTS.md`.
