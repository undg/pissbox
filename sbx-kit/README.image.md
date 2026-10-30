# Local image

`sbx-kit/Dockerfile` extends `docker/sandbox-templates:shell-docker` with Node
26.9.0, Pi, mise 2026.10.21, pnpm 12.15.0, colgrep, the
`lightonai/LateOn-Code-edge` search model, fd,
ripgrep, Neovim, `jq`, GitHub CLI (`gh`), and the upstream `tomljson` CLI. `EDITOR` and `VISUAL`
select Neovim. The kit starts `pi --offline` as `agent`; Pi's global npm
installation stays writable for self-updates.

## Build and import

On the **host**, with Docker running and `docker`/`sbx` on PATH.
The wrapper supports macOS's bundled Bash 3.2 and newer Bash on Linux:

From the checkout root:

```bash
make build-image      # Host architecture: build, save, and import into SBX
make build-image-all  # amd64 + arm64: build and save only, no SBX import
```

These targets call `bin/build-image` and `bin/build-image --amd64 --arm64`.
The dual-architecture target builds sequentially; the foreign architecture uses
emulation and may be slow. Use the wrapper directly for the options below.

The wrapper resolves its checkout even through symlinks or from another working
directory. By default it detects this host's architecture from the Docker
daemon, builds `pissbox:local` for it, saves
`.cache/images/pissbox-local-<arch>.tar`, and runs `sbx template load`.
Failures stop the pipeline. Existing sandboxes are untouched. Host Docker and
SBX have separate image stores, so importing is required.

| Option | Effect |
| --- | --- |
| *(none)* | Detect the host architecture, build it, register it |
| `--amd64` | Build `linux/amd64` only; no registration |
| `--arm64` | Build `linux/arm64` only; no registration |
| `--register [ARCH]` | Load an existing archive into SBX; `ARCH` defaults to the host |
| `--help` | Show usage |

Builds are native by default: x86_64 hosts build amd64, Apple Silicon builds arm64.
Building the other architecture works but runs every `RUN` step under emulation, so
it is slow. Before such a build the wrapper registers the needed binfmt handlers
by running the privileged `tonistiigi/binfmt` image, which needs network access
the first time and pulls an image. Each architecture keeps its own archive, and
`.cache/images/pissbox-local.tar` is a generated symlink to the archive built or
registered last, so older notes and habits keep working. A real file at that path
is replaced.

Pass Docker build options after `--`; they reach every build in that run:

```bash
bin/build-image -- --build-arg PI_VERSION=latest --no-cache
```

| Build argument | Purpose |
| --- | --- |
| `BASE_IMAGE` | Base sandbox template |
| `PI_VERSION` | Pi npm version or tag; defaults to `latest` |
| `COLGREP_VERSION` | Colgrep release |
| `COLGREP_MODEL_REVISION` | Hugging Face model commit |

Pins/defaults live in the Dockerfile. These do not pin apt packages. The
image installs `libatomic1` because upstream Node binaries link
`libatomic.so.1` and the base template does not ship it.
mise 2026.10.21 is installed from checksum-verified upstream Linux archives for
amd64 and arm64 into `/usr/local/bin/mise`. Update its version and both checksums
together. `mise use -g node@26.9.0 pnpm@12.15.0` installs the default Node and
pnpm as `agent`, with downloads and verification handled by mise.
`MISE_DATA_DIR=/opt/pissbox/mise` keeps tool installations and shims outside
shared cache mounts, writable by `agent`; global configuration lives in
`~/.config/mise/config.toml` inside the sandbox. These are not shared across
sandboxes. `NPM_CONFIG_PREFIX=/opt/pissbox/npm` keeps global npm packages,
including Pi and its self-updates, writable by `agent` and independent of the
selected Node version. Pi runs with the selected Node; project Node versions
must satisfy Pi's runtime requirements.

mise shims precede the base image's tools on PATH, so project tool versions
from `mise.toml` work in Pi commands and non-interactive project setup without
shell activation. `node`, `npm`, `npx`, `pnpm`, and `pnpx` shims are also linked
into `/usr/local/bin`.
The image keeps mise's default trust and auto-install behavior; it adds no
blanket trusted paths. Plain tool-version configurations need no trust, and
shims can auto-install missing versions. Configurations that change the
environment or run code can require trust. For explicit setup with a reviewed
`mise.toml`, run:

```bash
mise trust
mise install
pnpm --version
```

To do this during sandbox creation, include `mise trust` and `mise install`
before dependency installation in the project's `.pissbox.toml` `setup` list.
Only trust configurations you have reviewed: mise configuration can execute code.
Use `mise exec -- COMMAND` when the entire command needs the project's mise
environment, or `eval "$(mise activate bash)"` for interactive shell activation.
Other tool backends may require additional network-policy approvals.

`tomljson` is pinned to v2.4.3 with architecture-specific SHA-256 checksums in
the Dockerfile; update the version and checksums together. It parses project
`.pissbox.toml` files without installing a Go runtime or a custom parser.

Registering without a rebuild, for example after copying an archive from
another host of the same architecture:

```bash
bin/build-image --register arm64
```

Preparing an archive for a different machine is the cross-build case: build
`--arm64` and `--amd64` on one host, copy both archives, and register the
matching one on each machine. Emulation is needed only for the build, not for
running the imported image.

## Optional public CA

If your network requires an additional CA, put its approved **public PEM
certificate** at the gitignored `sbx-kit/ca.crt`. The wrapper passes it as
BuildKit secret `custom_ca`; the Dockerfile installs it before npm/curl run.
Node uses the system trust store via `NODE_USE_SYSTEM_CA=1`.
Without the file, the wrapper logs an info message and uses base-image trust.
Never supply a private key or credentials; the public CA remains in the image.

Secret presence and contents do not invalidate BuildKit's cache. After adding,
changing, or removing the CA, use:

```bash
bin/build-image -- --no-cache
```

## Manual build, save, and load

From the checkout root, without an additional CA:

```bash
docker buildx build --platform linux/amd64 --load -t pissbox:local sbx-kit
mkdir -p .cache/images
docker image save pissbox:local -o .cache/images/pissbox-local-amd64.tar
sbx template load .cache/images/pissbox-local-amd64.tar
```

Adjust `linux/amd64` and the archive name for the host architecture.
For a manual build with a CA, add
`--secret id=custom_ca,src=sbx-kit/ca.crt` to `docker buildx build`.
The saved archive can be loaded on another architecture-compatible host; it
contains the image, not workspaces, Pi configuration, or shared caches.

## Use or update

The kit already references `pissbox:local`. After importing, create a sandbox
with a new name:

```bash
bin/pissbox --name project-dev /path/to/project
sbx exec project-dev pi --offline --version
sbx exec project-dev colgrep --version
```

Tool/image changes require another build/import and a new sandbox to use them.
Rebuilding a tag does not update existing sandboxes. No registry publishing or
nightly rebuild is configured here.

## Bundled search model

The image downloads pinned INT8 and FP32 search weights into
`/opt/pissbox/models/LateOn-Code-edge`, outside shared-cache mounts. Kit setup
seeds `$HOME/.local/share/colgrep/config.json` with this default only when no
file or symlink exists; it preserves existing settings and indexes.

For existing shared settings, select the bundled model inside an updated sandbox.
**This clears indexes; do not run automatically:**

```bash
colgrep set-model /opt/pissbox/models/LateOn-Code-edge
```

Use `colgrep --model /opt/pissbox/models/LateOn-Code-edge "query"` for a
per-command override. All sandboxes sharing the local-path default must contain
the bundled model. After changing `COLGREP_MODEL_REVISION`, rebuild indexes:
the stable local path prevents colgrep from detecting changed weights.

The bundled search model needs no runtime Hugging Face access. ONNX Runtime and
optional/custom models are not bundled and still require downloads. To install
the optional agent model (roughly 2.5 GB) after shared links are set up:

```bash
colgrep --install-agent
```
