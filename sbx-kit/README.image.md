# Local image

`sbx-kit/Dockerfile` extends `docker/sandbox-templates:shell-docker` with Node
26.9.0, Pi, pnpm, colgrep, the `lightonai/LateOn-Code-edge` search model, fd,
ripgrep, Neovim, `jq`, and the upstream `tomljson` CLI. `EDITOR` and `VISUAL`
select Neovim. The kit starts `pi --offline` as `agent`; Pi's global npm
installation stays writable for self-updates.

## Build and import

On the **host**, with Docker running and `docker`/`sbx` on PATH.
The wrapper supports macOS's bundled Bash 3.2 and newer Bash on Linux:

```bash
bin/build-image
```

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

Pins/defaults live in the Dockerfile. These do not pin apt packages or pnpm.
Node 26.9.0 is installed from checksum-verified official Linux archives for
amd64 and arm64. Its directory takes precedence over the base image's Node,
with `node`, `npm`, and `npx` also linked into `/usr/local/bin`. The installation
is owned by `agent` so global npm installs and Pi self-updates remain writable.
Update the Node version and architecture-specific checksums together.

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
