# Global secret configuration

## Goal and status

Planning only. Implementation is not authorized by this file.

Replace repeated `--secret ENV_NAME HOST COMMAND` flags with one user-level
TOML file that defines proxy-managed secrets by environment variable name.
The file stores host commands, not credential values, so it is intended to be
commit-friendly. This is not a guarantee: a command can still embed a secret by
mistake. Keep `--secret` for one-off additions and CLI overrides.

The sandbox still receives only a placeholder. Pi settings reference the
placeholder environment variable, for example `"apiKey": "${GH_TOKEN}"`.

## Agreed decisions

- Resolve the file from `PISSBOX_CONFIG` when set; otherwise use
  `${XDG_CONFIG_HOME:-$HOME/.config}/pissbox/config.toml`.
- `PISSBOX_CONFIG` is a file path. A relative value uses the caller's
  directory; an empty value means unset. Reject directories and non-regular
  resolved files.
- A missing default file is created during a normal launch. Create parent
  directories and write a starter file, mode `0600`, whose first line is:

```toml
#:schema ~/Code/pissbox/host/config/global-schema.json
```

  Generate this path from the resolved checkout root and compact a `$HOME`
  prefix to `~`; do not hard-code the checkout path into the binary. On the
  current machine the result is the requested line. This schema location is a
  temporary first version; packaging/install ownership can replace it later.
- Create the starter with exclusive-create semantics. If another process wins
  the race, load the existing file instead of overwriting it.
- A missing explicit `PISSBOX_CONFIG` target is an error. Do not silently
  create a file at a mistyped override path.
- If creating the default starter file fails, warn on stderr and continue with
  no global entries. Do not block launch for an optional config. A file that
  exists but cannot be read or validated is still an error.
- Never search the project, current directory, parent directories, or
  additional workspaces for this config.
- Use TOML keyed by sandbox environment name:

```toml
[secrets.GH_TOKEN]
host = "api.github.com"
command = "security find-generic-password -s gh-token -w"

[secrets.OPENAI_API_KEY]
host = "api.openai.com"
command = "op read op://Private/OpenAI/api-key"
```

- Accept only `host` and `command` per entry. Reject unknown keys and any
  literal value field. There is no leading `!` shorthand: the field is always
  a command. This prevents a separate credential field, but does not prove the
  command text contains no credential; commands must fetch values, not embed
  them.
- Reuse the existing host, command, environment-name, and reserved-name rules
  from `--secret`.
- Global config is read-only after starter-file creation. Never write CLI
  flags, resolved values, or commands into it. Build the in-memory secret map
  from global entries first, then apply CLI `--secret` entries by environment
  name so a CLI entry overrides the global entry. Duplicate CLI entries remain
  parser errors.
- Do not add `--ignore-config-secrets`. Global means every new sandbox gets the
  configured secrets. Use `PISSBOX_CONFIG` with an empty or smaller file when a
  one-off launch needs a different set.
- Keep the existing execution boundary: the launcher validates and passes the
  command to `sbx secret set-custom --command`; it never runs the command.
  Registration stays creation-only, after the reported sandbox name is known
  and before project setup finalization and Pi.
- `--help`, `--ls`, `--delete`, and `--bridge-stop` ignore global secrets and
  must not create, read, or validate the global file.
- Existing sandboxes keep their registrations. New secrets require a new
  sandbox name. Attaches never add registrations.
- Every global secret applies to every new sandbox. Per-project opt-in is out
  of scope until the wider scope becomes a demonstrated problem.
- The physical global-config path must not be inside any mounted workspace,
  Pi directory, or state directory. The file contains executable host commands;
  a writable sandbox mount must not be able to alter it for the next launch.
- All config-specific implementation belongs in `host/config/`. The launcher
  package only resolves inputs, calls this package, and wires the resulting
  secrets into the existing creation flow.
- Validate with an embedded JSON Schema using `github.com/BurntSushi/toml`
  v1.6.0 plus `github.com/santhosh-tekuri/jsonschema/v6` v6.0.3. Do not extend
  `sbx-kit/files/home/.pissbox/scripts/project-setup.sh` for this feature. That
  host-side reuse of sandbox scripts is a known design problem and will be fixed
  separately.

## Schema and validation split

- Keep the schema at `host/config/global-schema.json` and embed it into the Go binary.
- Parse TOML with BurntSushi/toml. Load and compile the embedded schema once
  behind `sync.OnceValues`, then validate the decoded document in memory. No
  network, temp files, WASI, or shelling out.
- TOML syntax errors carry line/column positions. JSON Schema validation errors
  expose instance key paths, not line/column. For the first version, report key
  paths; add a key scanner only if real diagnostics are insufficient.
- Schema responsibilities: top-level object shape, unknown-key rejection,
  `secrets` table, entry object shape, `host`/`command` string types, required
  fields, and environment-key pattern/reserved-name restrictions where
  expressible. Give every schema property a clear `description`, including
  top-level and per-secret keys.
- Go semantic checks remain necessary for host grammar, URL/host:port
  rejection, command whitespace/leading-dash rules, and duplicate environment
  names within CLI input. Global-versus-CLI overlap is resolved by CLI
  precedence, not rejected by this layer.
- An empty or comment-only TOML document is treated as an empty object before
  schema validation, so the generated schema-only starter file is valid without
  requiring a `secrets = {}` line.
- Decode into a deterministic ordered list, sorted by environment name, so SBX
  registration order and tests are stable.
- Error messages must not echo full command values.

## Config module boundary

`host/config` is a pure configuration library called by the launcher. It owns:

- config models, embedded JSON Schemas, `jsonschema/v6` validation, semantic
  checks, path resolution, starter-file creation, and deterministic ordering;
- reading the TOML file as data and returning typed global secret entries.

It must not own orchestration. `host/config` never calls `sbx`, executes a
command, captures stdout, mounts a directory, creates a sandbox, or writes
resolved values back to disk. In particular, `command` strings are opaque data
from this module's perspective. The launcher decides when to create/load the
file, then passes the returned command strings unchanged to
`sbx secret set-custom --command`.

The launcher remains the only SBX caller and lifecycle orchestrator: workspace
and mount resolution, staging, secret registration, setup finalization, Pi
attach, and management commands stay in `host/internal/launcher`.

The package must leave room for the later project-config schema. Shared schema
loading/validation helpers stay common; global and future project models get
separate schema files and loaders under `host/config/` without adding
project-orchestration logic to the package or duplicating the schema engine.

## Launcher sequence

1. Parse CLI. Handle help and management commands without touching global config.
2. Resolve Pi directory, state directory, workspaces, and project mounts.
3. Resolve the global config path. Check that path and its nearest existing
   parent are outside the mounted workspace/Pi/state paths.
4. If the default path is missing, create the starter file. On creation failure,
   warn and continue with no global entries. A missing explicit override fails.
5. Load and validate the global config before any state creation, mailbox, or
   SBX call.
6. Put global entries into an in-memory map keyed by environment name, then put
   CLI entries into the same map. CLI wins on overlap. Never write the file.
7. If the final secret list is non-empty, run the existing SBX capability check.
8. Compute `staged` from the final list and continue the existing creation and
   registration flow.

## Evidence and starting points

- `host/internal/launcher/cli.go` parses repeatable `--secret`.
- `host/internal/launcher/secrets.go` validates names/hosts/commands, checks
  SBX `secret set-custom` capabilities, and registers entries.
- `host/internal/launcher/launcher.go` forces deferred creation when secrets
  exist and calls `finishCreation`.
- `host/internal/launcher/isolation.go` reads the reported sandbox name,
  mounts isolated directories, registers secrets, then finalizes setup.
- `host/internal/launcher/project.go` contains a TODO explicitly calling for a
  JSON Schema validation module, rather than invoking sandbox scripts from the
  host. This feature should establish the pattern in `host/config/` without
  migrating project configuration yet.
- Validation stack verified by agent Felix Willa: `BurntSushi/toml` plus
  `santhosh-tekuri/jsonschema/v6` build on darwin/linux amd64/arm64 with no cgo,
  embedded schemas, in-memory validation, and roughly +2.3 MB stripped binary
  size. Sources:
  <https://pkg.go.dev/github.com/santhosh-tekuri/jsonschema/v6>,
  <https://pkg.go.dev/github.com/BurntSushi/toml>.
- `host/go.mod` is module `pissbox/host`; a new `host/config` package can be
  imported as `pissbox/host/config`.
- Current docs already forbid secret commands in `.pissbox.toml`; keep that
  rule. See `doc/secrets.md`, `doc/project-configuration.md`, and `README.md`.
- Existing coverage lives in `host/internal/launcher/secrets_test.go`,
  `tests/test_secrets.py`, and the launcher fixture in
  `host/internal/launcher/launcher_test.go`.
- Real SBX placeholder injection and proxy substitution still need a host smoke
  test. Stubbed launcher tests do not prove that behavior.

## Tasks

- [ ] Add and pin `github.com/BurntSushi/toml` v1.6.0 and
  `github.com/santhosh-tekuri/jsonschema/v6` v6.0.3, review `bin/install` and
  README dependency statements, then run `make tidy`. Both are Go-only; no new
  host CLI prerequisite is expected.

- [ ] Create `host/config/` with the config model, default/explicit path
  resolution, starter-file creation, schema embedding, schema validation, and
  semantic secret checks. Provide creation and loading entry points that the
  launcher can call without duplicating config logic. Keep the package free of
  SBX calls, command execution, mount logic, and lifecycle decisions, and shape
  it so a future project-config schema can be added beside the global one.

- [ ] Add `host/config/global-schema.json`. Reject unknown top-level keys and unknown
  entry keys, require `host` and `command` per secret, constrain environment
  names, accept the generated schema-only file as empty valid config, and
  provide a `description` for every key. Add a test that walks nested schema
  objects, including the `secrets` entry schema, and fails when any property
  lacks a description.

- [ ] Wire the launcher using the sequence above: resolve mounts, check config
  path safety, create the default config when absent, load and validate it,
  apply global entries then CLI overrides, run the SBX capability check, and
  compute `staged` before creation. Never rewrite the config. Keep all
  management commands and help on the existing no-config path.

- [ ] Keep CLI parsing in the launcher, but share one secret type/validation
  rule set with `host/config` rather than maintaining separate definitions.
  The launcher must never execute configured commands or capture their stdout.
  It passes each entry's `command` string directly as SBX arguments, exactly as
  today:
  `sbx secret set-custom --sandbox NAME --env ENV --host HOST --command COMMAND`.
  SBX resolves and refreshes the value; the launcher never sees it.

- [ ] Add `host/config` tests for: path resolution and relative/empty override
  semantics, directory/non-regular rejection, starter-file creation and
  first-line schema directive, exclusive create/no overwrite, creation failure
  returning an error, comment-only normalization, missing explicit override,
  malformed TOML, unknown keys, literal value fields, invalid environment names,
  invalid hosts/commands, deterministic ordering, every nested schema property
  having a description, and empty config.

- [ ] Add launcher tests for: global secrets registered on creation before setup
  and Pi, later attaches not re-registering, CLI entries overriding global
  entries by environment name, duplicate CLI entries still failing, config left
  unchanged by CLI flags, starter-creation error producing a warning and a
  continued launch, management/help bypass, path-inside-mount refusal, and no
  host command execution. Extend the fixture as needed to set an isolated
  `PISSBOX_CONFIG` and config directory.

- [ ] Audit both Go and Python launcher test harnesses so ambient user config
  cannot leak. Set `PISSBOX_CONFIG` explicitly to a fixture path for every
  launch, including tests that do not exercise secrets. Unset falls back to the
  real home directory.

- [ ] Update `--help`, `doc/secrets.md`, `doc/project-configuration.md`,
  `README.md`, `host/README.md`, and the host lifecycle/config contract in
  `AGENTS.md`. Document the path, generated starter line, schema location,
  schema, read-only rule, no-literal-values rule, escape hatch, scope widening,
  commit intent and limits. Remove any claim that global secret commands require
  `tomljson`/`jq` or the sandbox project-setup script.

- [ ] Run `make check` and the focused Go suites. Record any real-SBX
  verification gap rather than claiming proxy behavior was tested.

## Acceptance criteria

- With no global file, a normal launch creates the starter file with the
  schema directive and continues as if no global secrets were configured.
- With no global file, help/list/delete/bridge-stop do not create the file.
- The generated file validates against the embedded schema, and a
  comment-only config is accepted as an empty object.
- Every nested schema property has a `description` usable by editors and
  documentation.
- A valid global file registers every configured secret for a new sandbox in
  the existing creation-only order; the launcher never executes the commands.
- `--secret` still works independently. A CLI entry overrides the same
  environment name from global config; duplicate CLI entries still fail; CLI
  flags never alter the global config file.
- A malformed global file fails before any sandbox or state side effect. A
  failure to create the optional starter file warns and continues.
- No separate credential value field can be placed in the supported config
  schema; command text is not proof that a credential was not embedded.
- `host/config` contains no SBX invocation, shell execution, mounting, or
  sandbox lifecycle code; it only reads, validates, creates, and returns data.
- A config path inside a writable mount is refused.
- Global config validation uses the embedded JSON Schema, `BurntSushi/toml`,
  and `jsonschema/v6`, not `project-setup.sh`, `tomljson`, or `jq`.
- Documentation no longer tells users that all secrets require repeated CLI
  flags.
- Go and Python launcher tests never read or create the real user config.
