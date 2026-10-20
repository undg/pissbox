# Plan: architecture-aware `bin/build-image` and `--register`

Status: proposal only. No implementation yet.

Owner tags: **(grug)** = agent writes/verifies by running the stubs. **(you)** =
needs a real host, real docker, or a real SBX install.

## Scope

Native build by default: x86_64 Linux builds `linux/amd64`, Apple Silicon builds
`linux/arm64`. Cross-arch builds are supported because the archive is prepared
on one machine for another (planned offline laptop): the flags select the
platform and BuildKit runs `RUN` steps under emulation, which needs binfmt
handlers in the Docker VM and is slow. The wrapper does not install or probe
emulation; it warns with the fix before starting a cross build.

## Decisions already made

- [x] Native build by default, cross-arch builds allowed under emulation
- [x] Cross builds register binfmt automatically via the privileged
      `tonistiigi/binfmt` image (needs network the first time)
- [x] `--amd64`, `--arm64`, `--register [amd64|arm64]` as the only new flags
- [x] Explicit arch flags override detection; without flags the host arch is used
- [x] Arch flags alone build only; registering is always explicit except in the no-flag case
- [x] Archives named `pissbox-local-<arch>.tar`, image tag stays `pissbox:local`
- [x] `pissbox-local.tar` becomes a generated symlink, `settings.json` style
- [x] Bundling packages/caches is out of scope
- [x] Fixed pipeline order: detect → build → register

## Goal

One script that can

1. build `pissbox:local` for the selected architecture, amd64 or arm64;
2. keep one archive per architecture in `.cache/images/`;
3. import one of them into SBX so a sandbox can use it.

Today the script builds for the host architecture only, saves
`.cache/images/pissbox-local.tar`, and always runs `sbx template load`.

## Flags

| Flag | Meaning |
| --- | --- |
| `--amd64` | Build `linux/amd64`, do not register |
| `--arm64` | Build `linux/arm64`, do not register |
| `--register [amd64\|arm64]` | Import an existing archive into SBX; arch defaults to the detected host arch |

Naming: `--register` is kept because it is distinct from building and matches
"make this image usable by SBX". Rejected alternatives: `--load` (already means
`docker buildx build --load`), `--import` (Docker import is a different
operation), `--sbx-load` (verbose).

### Combination matrix

| Invocation | Detect arch | Build arch(es) | Register arch |
| --- | --- | --- | --- |
| *(none)* | yes | host | host |
| `--amd64` | no (given) | amd64 | none |
| `--arm64` | no (given) | arm64 | none |
| `--amd64 --arm64` | no (given) | amd64 + arm64 | none |
| `--register` | yes | none | host |
| `--register amd64` | no (given) | none | amd64 |
| `--register arm64` | no (given) | none | arm64 |
| `--arm64 --register` | yes (for register) | arm64 | host |
| `--arm64 --amd64 --register` | yes (for register) | amd64 + arm64 | host |

Rules:

- Explicit arch flags win over detection. They exist for the mismatch cases:
  an arm Mac with a Rosetta shell, or a docker context pointing at another
  machine.
- `--register` without an argument needs detection.
- Arch flags alone build only. Explicit build request, no side effect beyond
  the archive.
- No arch flag at all, with or without `--register`, also builds the host
  architecture. That keeps `bin/build-image` doing what it does today.
- `--register ARCH` never builds. Missing archive is an error naming the flag
  that would create it.
- Two arch flags build both, in a stable order (`amd64` then `arm64`).
- `--` still passes the rest unchanged to `docker buildx build`; docker options
  apply to every platform in that run.

### Parsing work

- [x] **(grug)** Accept `--amd64` / `--arm64` as booleans feeding a build list
- [x] **(grug)** Accept `--register` with an optional following `amd64|arm64`
      value without eating other flags or the `--` separator
- [x] **(grug)** Reject unknown `--register` values before any tool runs
- [x] **(grug)** Keep the existing `case` parser shape and last-value-wins
      behavior for repeated flags
- [x] **(grug)** Help text lists all three flags and the no-flag default

## Architecture detection

Ask the Docker daemon, not the shell:

```bash
docker info --format '{{.Architecture}}'
```

`x86_64|amd64` → `amd64`, `aarch64|arm64` → `arm64`, anything else → error.
The daemon is what runs the build and the sandbox VM, so its answer is the one
that matters; `uname -m` inside a Rosetta shell lies on Apple Silicon.

- [x] **(grug)** Detect only when a flag does not already name the arch
- [x] **(grug)** Translate daemon output and fail with a precise error on
      unknown values
- [ ] **(you)** Confirm on the Mac that the daemon reports the arch the sandbox
      actually runs, not Rosetta's answer

## Pipeline

Three independent modules in a fixed order, each with one job:

1. `detect_arch` — set `HOST_ARCH` and `PLATFORM` (`linux/<arch>`), only when
   detection is actually needed.
2. `build_arch ARCH` — `docker buildx build --platform "linux/$arch" --load`
   with the CA secret and forwarded options, then
   `docker image save -o "$ARCHIVE_DIR/pissbox-local-$arch.tar"`.
3. `register_arch ARCH` — `sbx template load` of that archive.

Execution section reads the configured state and calls them in order: detect,
then build for each requested arch, then register if requested.

- [x] **(grug)** `detect_arch`, `build_arch`, `register_arch` as separate
      functions, no shared state beyond the parsed variables
- [x] **(grug)** Build loop runs requested arches in fixed order
- [x] **(grug)** Register step refuses to run without an existing archive
- [x] **(grug)** Keep the existing buildx preflight and CA handling unchanged
- [x] **(grug)** Register binfmt via `tonistiigi/binfmt` when the target arch
      differs from the daemon arch, and fail clearly if that step fails

## Naming inside and outside the archive

- Archive: `.cache/images/pissbox-local-amd64.tar`,
  `.cache/images/pissbox-local-arm64.tar`.
- Compatibility link: `.cache/images/pissbox-local.tar` is a generated symlink
  to the archive built or registered last, the same way
  `pi-agent/settings.json` is a generated link to the selected profile. Created
  on every successful run; a real file at that path is replaced, not followed.
  Nothing reads it as a source of truth.
- Image tag inside the archive: `pissbox:local`, because `sbx-kit/spec.yaml`
  names that tag and image/kit changes need a new sandbox anyway. Native builds
  mean the docker store only ever holds the matching architecture.

- [x] **(grug)** Symlink refreshed on every successful build or register
- [x] **(grug)** Existing regular file at the link path is replaced, not followed
- [ ] **(you)** Confirm no other script, doc, or habit depends on
      `pissbox-local.tar` being a regular file

## Out of scope

Bundling Pi packages, extensions, or cache contents into the image is not part of
this plan. Those paths are mounted or linked from the host at runtime
(`$HOME/.pi/agent`, shared-directory links into `.cache`), so baked content would
be shadowed. Bundling, if ever done, needs content in `/opt/pissbox` plus
absence-only seeding in setup, and belongs to the "Bundled plugins and portable
distribution" item in `KNOWN_ISSUES.md`.

## Tests (stubbed, no real docker)

Extend `tests/test_build_image.py`. The docker stub needs to answer
`docker info --format ...` with a configurable architecture; it already records
every call.

- [x] **(grug)** `docker info` stub answering with a settable architecture
- [x] **(grug)** No flags: detect, build host arch, save host archive, register
      host archive
- [x] **(grug)** `--amd64` / `--arm64`: that build and save only, no detection,
      no `sbx` call
- [x] **(grug)** Both arch flags: two builds in fixed order, two saves, no `sbx`
      call
- [x] **(grug)** `--register` alone: no build, `sbx template load` host archive
- [x] **(grug)** `--register arm64`: no detection, register arm64 archive
- [x] **(grug)** `--register` with missing archive: nonzero exit, message names
      the missing file and the build flag that creates it
- [x] **(grug)** Unknown arch value (`--register riscv64`): rejected before any
      tool runs
- [x] **(grug)** `--` options forwarded to every build call
- [x] **(grug)** Detection mapping: `x86_64`, `aarch64`, `arm64`, unknown value
- [x] **(grug)** Cross-arch build installs emulation first, then builds
- [x] **(grug)** Failed emulation installation stops before any build
- [x] **(grug)** Compatibility symlink points at the last archive and replaces a
      real file
- [x] **(grug)** Existing tests keep passing: CA forwarding, symlink launch,
      help, failures preserving exit status, missing buildx message
- [x] **(grug)** `bash -n`, `python3 -m unittest discover -s tests`,
      `git diff --check`

## Docs and follow-up

- [x] **(grug)** `sbx-kit/README.image.md`: flag table, per-arch archive names,
      native-per-host note, symlink note
- [x] **(grug)** `KNOWN_ISSUES.md`: "AMD64 / Apple Silicon image distribution"
      stays open, wording adjusted to say what now exists (build + register per
      host) and what does not (one host emitting both, verified transfer)

## Real-host verification

- [ ] **(you)** x86_64 Linux: `bin/build-image` with no flags, then create a
      sandbox and check `pi --offline --version` and `colgrep --version`
- [ ] **(you)** x86_64 Linux: `bin/build-image --arm64` after installing binfmt,
      then copy the arm64 archive to the MacBook and register it there
- [ ] **(you)** Apple Silicon: same two checks, after `--arm64`
- [ ] **(you)** `bin/build-image --register` alone imports the archive without a
      rebuild and a new sandbox starts
- [ ] **(you)** `sbx template load` on an archive whose tag already exists
      behaves as expected (replaces, does not stack)

## Grug critique, before anyone writes code

1. Three flags, one pipeline. Resist adding `--all`, `--push`, or a config file.
2. `--register ARCH` is a second way to do something SBX already does
   (`sbx template load`). Accept it because it saves path juggling, reject any
   further wrapper around it.
3. The one exception to "no magic": cross builds register binfmt through
   `tonistiigi/binfmt`, because the flag is worthless without it. Keep it to
   that single privileged container, no probing, no retries, no builder
   creation.
4. Keep `--` as the single door to docker options. No per-platform option
   forwarding anyone has not asked for.
5. Order is fixed: detect, build, register. Nothing registers an archive that
   was not built or already on disk.
6. Archival convenience usually loses. The symlink is one line of code and
   keeps old paths working; anything more than that gets rejected.
