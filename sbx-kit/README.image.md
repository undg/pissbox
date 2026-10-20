# Local pissbox image

Build your own image from `sbx-kit/Dockerfile` and reuse it with
[Docker Sandboxes](https://docs.docker.com/ai/sandboxes/). No registry publishing
is required. SBX uses the finished image; it does not execute this Dockerfile
when you start a sandbox.

## Contents

Built on `docker/sandbox-templates:shell-docker`: the standard sandbox tool
chain plus a Docker engine, requesting Docker-in-Docker via
`com.docker.sandboxes.start-docker=true`. On top of that:

- [`@earendil-works/pi-coding-agent`](https://www.npmjs.com/package/@earendil-works/pi-coding-agent),
  installed globally at build time (`PI_VERSION=latest` by default),
  plus a `/usr/local/bin/pi` symlink
- `pnpm`, installed globally through npm as `agent`
- `colgrep`, installed as `agent` using the upstream installer, pinned by
  `COLGREP_VERSION` (currently `1.8.5`), plus a `/usr/local/bin/colgrep` symlink
- the `lightonai/LateOn-Code-edge` search model, pinned by
  `COLGREP_MODEL_REVISION`, with INT8 and FP32 ONNX weights under
  `/opt/pissbox/models/LateOn-Code-edge` (about 85 MiB of model files)
- `fd-find` (apt), which backs pi's `find` tool, plus a `/usr/local/bin/fd`
  symlink for the canonical name — without it pi downloads a binary from
  GitHub releases on first use, which a locked-down sandbox blocks
- `ripgrep` and `neovim` (apt), installed explicitly; `EDITOR=nvim` and
  `VISUAL=nvim` make Neovim the default external editor
- no Node layer — the template already ships Node 22.22.1, and pi requires
  `>= 22.19.0`

The npm global prefix stays owned by `agent`, so `pi install` and
`pi update --self` work inside the sandbox.

Runs as the non-root `agent` user. The local kit launches `pi --offline`.
Colgrep's optional agent model is not included in the image.

## Build on your host

To build, save, and load the image into SBX in one command, run on your host:

```bash
bin/build-image
```

The script locates this checkout even through a symlink or from another working
directory. It builds `pissbox:local`, saves it to
`.cache/images/pissbox-local.tar` (replacing the previous archive), and runs
`sbx template load`. It stops on failure and does not update or remove existing
sandboxes. Docker must be running and both `docker` and `sbx` must be on PATH.
Forward Docker build options after `--`, for example:

```bash
bin/build-image -- --build-arg PI_VERSION=latest --no-cache
```

The individual commands are documented below.

Run these commands on the **macOS or Linux host**, from this repository's root,
not inside an existing sandbox. Docker must be running and SBX must be installed.
The image architecture follows the Docker builder; build for the architecture
of the machine where SBX will run it.

The current Dockerfile requires an approved public CA certificate in PEM format
at `sbx-kit/ca.crt`. It installs that certificate into the image's system trust
store and enables `NODE_USE_SYSTEM_CA=1` for Node. The certificate becomes part
of the image; do not supply a private key or credentials. If no additional CA is
needed, remove the certificate `COPY` and its `RUN update-ca-certificates` step
before building.

```bash
docker build -t pissbox:local sbx-kit
```

The first build downloads the base image and installs tools. Later builds reuse
unchanged Docker layers. To choose a precise Pi release, add
`--build-arg PI_VERSION=<version>`; colgrep accepts
`--build-arg COLGREP_VERSION=<version>`. These arguments do not pin the base image,
apt packages, or pnpm. The search model is pinned separately to Hugging Face
commit `4bcdf5ed93f791259eb130b577a240f753d68dd8`; override it with
`--build-arg COLGREP_MODEL_REVISION=<commit-hash>`.

## Save the image and load it into SBX

Host Docker and SBX use separate image stores. A successful `docker build` alone
does not make the image available to SBX. Export it and import it once:

```bash
mkdir -p .cache/images
docker image save pissbox:local -o .cache/images/pissbox-local.tar
sbx template load .cache/images/pissbox-local.tar
```

The archive stays on your disk under the repository's gitignored `.cache/`.
It contains the built image, not a snapshot of a running sandbox or its mounted
workspaces, Pi configuration, or shared caches.

## Use the image

Set the following field in `sbx-kit/spec.yaml`, keeping the other `sandbox`
settings, including `entrypoint`:

```yaml
sandbox:
  image: pissbox:local
```

Create a sandbox with a new, unused name:

```bash
bin/pissbox --name pi-local-image
```

Verify the installed tools:

```bash
sbx exec pi-local-image pi --offline --version
sbx exec pi-local-image colgrep --version
```

## Reuse or update

- **Open the same sandbox:** run `bin/pissbox --name pi-local-image` again and
  choose Pi at the existing-sandbox menu. No rebuild or image import is needed.
- **Create another sandbox:** use a different name, such as
  `bin/pissbox --name pi-local-image-2`. It reuses the imported image.
- **Restore the image or move it to another compatible machine:** load the saved
  archive with `sbx template load` again, then use the kit with the same image
  reference. The destination must support the image's architecture. The archive
  does not include the kit, workspaces, Pi configuration, or shared state.
- **Change installed tools:** edit the Dockerfile, then repeat the build, save,
  and load commands. Create a sandbox with a new name to use the updated image.
  Rebuilding or reloading a tag does not update existing sandboxes. Do not delete
  a sandbox just to pick up an image change; it may contain sandbox-local data.

No registry push is needed for this workflow. SBX may still check a registry at
creation before falling back to its locally cached image.

## Bundled colgrep search model

The build downloads the pinned search model with TLS-verified `curl`, using the
image's system trust store. This avoids Colgrep's runtime Hugging Face TLS
failure behind an intercepting proxy. The model files are OS-independent;
Colgrep and ONNX Runtime binaries are not.

The files live outside `$HOME` so shared-cache links do not hide them. The image
also includes `/opt/pissbox/colgrep-config.json`, selecting this local model.
After linking shared state, kit setup copies that default into
`$HOME/.local/share/colgrep/config.json` only if no config file or symlink exists.
New shared state therefore uses the bundled model automatically, without a
runtime Hugging Face download. Existing settings and indexes are not changed.

For existing shared state still using the remote model, select the bundled model
once with Colgrep's own command. **This clears existing indexes**; run it only
when ready to rebuild them:

```bash
sbx exec pi-local-image colgrep set-model /opt/pissbox/models/LateOn-Code-edge
```

The choice persists in shared state. For a single search without changing the
saved default, use `colgrep --model /opt/pissbox/models/LateOn-Code-edge "query"`.

Using the local path avoids runtime Hugging Face requests for this model.
ONNX Runtime is **not** bundled and may still require a first-run download.
The shared Hugging Face cache remains available for other models.

Model updates require rebuilding, saving, and importing the image, then creating
a new sandbox. Existing sandboxes do not gain the bundled files.
The default setting is shared: after selecting the local model, older sandboxes
using the same state root will also see that path, but lack its files. Use the
updated image for sandboxes sharing that default.
If you change `COLGREP_MODEL_REVISION`, rebuild the affected indexes too: the
local model path stays the same, so Colgrep cannot detect changed weights.

## Optional colgrep agent model

Download the model inside a created sandbox, after the shared-cache links exist:

```bash
sbx exec pi-local-image colgrep --install-agent
```

This downloads roughly 2.5 GB and requires network access to the download hosts.
This optional agent model is separate from the bundled search model. Its model
and runtime caches persist in shared state; do not copy them through the kit.

## References

- [SBX: load a locally built template](https://docs.docker.com/ai/sandboxes/usage/#load-a-template)
- [Shared sandbox directories](SHARED-DIRECTORIES.md)
