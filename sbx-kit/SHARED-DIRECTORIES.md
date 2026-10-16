# Shared sandbox directories

The launcher mounts one host-owned, **sandbox-only Linux** state directory into
all new sandboxes. The default is `shared-state/` beside `pi-agent/` in this
checkout. It is created on demand and ignored by Git. Removing a sandbox does
not remove this mounted state.

The previous `sandbox-state` name may remain as an ignored compatibility
symlink to `shared-state` in existing checkouts. Keep it while any sandbox
still uses the old paths. The launcher does not migrate existing sandboxes.

Use `--pissbox-state-dir DIR` to choose another location, or set
`PISSBOX_STATE_DIR`. Precedence is flag > environment > checkout default.
Relative paths resolve from the caller's working directory. The launcher
canonicalizes the path before passing it as a workspace and through
`--env PISSBOX_STATE_DIR=...`.

## Add or change a shared directory

Edit `files/home/.pissbox/shared-directories.conf`. Each non-comment row is:

```text
path-under-shared-state|path-under-sandbox-home
```

Paths are literal, relative directory paths; no shell expansion, absolute
paths, or `..` segments. Spaces are supported; `|` is the separator. For example:

```text
another-tool/cache|.cache/another-tool
```

No launcher, Dockerfile, or per-tool setup changes are needed. The kit copies
this file and `scripts/link-shared-directories.sh` into `$HOME/.pissbox/`.
`setup.install` runs the script as the agent user, creating the source directories
on the mounted state tree and symlinks at the default Linux locations.

| Shared-state directory | Sandbox home directory | Purpose |
| --- | --- | --- |
| `huggingface/hub` | `.cache/huggingface/hub` | Downloaded model weights/tokenizers; not the HF token file |
| `colgrep/data` | `.local/share/colgrep` | Indexes and data; current upstream also stores config here |
| `colgrep/config` | `.config/colgrep` | Configuration for versions using this separate location |
| `colgrep/cache` | `.cache/colgrep` | Colgrep runtime cache |
| `onnxruntime` | `.cache/onnxruntime` | Downloaded Linux ONNX Runtime libraries |
| `pnpm/store` | `.local/share/pnpm/store` | pnpm's content-addressable package store |
| `pnpm/cache` | `.cache/pnpm` | pnpm metadata cache |

pnpm's store and metadata cache are both shared; project `node_modules` stays
in the project. Model files are shared across all projects, while colgrep keeps
per-project indexes inside its shared data directory.

## Boundaries

- This only persists directories. It does not install colgrep or pnpm, download
  models in advance, or grant network access to download hosts.
- The links use tools' default Linux locations. Custom `HF_HOME`, `XDG_*`, or
  pnpm `store-dir`/`cache-dir` settings may bypass them.
- Do not point this at macOS/native caches. Runtime libraries and some generated
  caches are platform-specific. Keep Linux sandbox state separate.
- Setup accepts an existing matching symlink. It refuses to replace existing
  files, directories (even empty ones), or different symlinks. Move existing
  data deliberately; setup never deletes or silently migrates it.
- An existing sandbox keeps its original mounts and copied kit files. Use a new
  sandbox name to pick up changed definitions or a different state root. The
  launcher's attach path does not add mounts or rerun this setup.
- Sharing data does not make simultaneous colgrep writes to the same project's
  index safe. Avoid concurrent index updates until upstream locking is verified.
- State-root overrides should be dedicated directories. Git ignores this
  checkout's default `shared-state/` and the legacy `sandbox-state` path;
  manage ignore rules for other locations.

## Verification

Run the local regression suite:

```console
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s tests -v
```

It checks launcher arguments using a stub SBX CLI, and runs the link script with
isolated fixture homes/state directories. Generated fixtures are retained under
ignored `shared-state/test-fixtures/`. These tests do not launch real SBX or run
colgrep/pnpm.

After creating a real sandbox, check from the host:

```console
sbx exec <sandbox-name> sh -c 'readlink "$HOME/.cache/huggingface/hub"; readlink "$HOME/.local/share/colgrep"; readlink "$HOME/.local/share/pnpm/store"'
sbx exec <sandbox-name> pnpm store path
```
