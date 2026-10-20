# Local image

`sbx-kit/Dockerfile` extends `docker/sandbox-templates:shell-docker` with Pi,
pnpm, colgrep, the `lightonai/LateOn-Code-edge` search model, fd, ripgrep,
and Neovim. `EDITOR` and `VISUAL` select Neovim. The kit starts `pi --offline`
as `agent`; Pi's global npm installation stays writable for self-updates.

## Build and import

On the **host**, with Docker running and `docker`/`sbx` on PATH:

```bash
bin/build-image
```

The wrapper resolves its checkout even through symlinks or from another working
directory. It builds `pissbox:local`, saves
`.cache/images/pissbox-local.tar` (replacing the old archive), and runs
`sbx template load`. Failures stop the pipeline. Existing sandboxes are untouched.
Host Docker and SBX have separate image stores, so importing is required.

Pass Docker build options after `--`:

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
Use a builder compatible with the target SBX host's architecture.

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
docker build -t pissbox:local sbx-kit
mkdir -p .cache/images
docker image save pissbox:local -o .cache/images/pissbox-local.tar
sbx template load .cache/images/pissbox-local.tar
```

For a manual build with a CA, add
`--secret id=custom_ca,src=sbx-kit/ca.crt` to `docker build`.
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
