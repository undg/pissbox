# Shared Linux tool state

The launcher mounts a host-owned Linux state directory, defaulting to this
checkout's gitignored `.cache/`. It survives sandbox removal. Override it with
`--pissbox-state-dir DIR` or `PISSBOX_STATE_DIR`; the flag wins and relative
paths use the caller's directory. Keep it separate from native/macOS caches
and Pi configuration.

## Mappings

Edit `files/home/.pissbox/shared-directories.conf` to add or change a mapping:

```text
# state-relative-path|sandbox-home-relative-path
another-tool/cache|.cache/another-tool
```

Paths are literal and relative: no shell expansion, dot segments, or absolute
paths. Spaces are supported. No launcher, Dockerfile, or per-tool script edits
are needed. Kit setup creates source directories on the mounted root and links
tool locations as user `1000`.

| State directory | Sandbox home location | Contents |
| --- | --- | --- |
| `huggingface/hub` | `.cache/huggingface/hub` | Models/tokenizers, not HF tokens |
| `colgrep/data` | `.local/share/colgrep` | Indexes, data, current upstream config |
| `colgrep/config` | `.config/colgrep` | Config for versions using this location |
| `colgrep/cache` | `.cache/colgrep` | Runtime cache |
| `onnxruntime` | `.cache/onnxruntime` | Linux runtime libraries |
| `npm` | `.cache/npm` | npm package cache |
| `pnpm/store` | `.local/share/pnpm/store` | Content-addressable packages |
| `pnpm/cache` | `.cache/pnpm` | Package metadata |

Project `node_modules` stays in the project. Kit setup points npm at the shared
`$HOME/.cache/npm` directory. The image's `$HOME/.npm` is left untouched.
Existing sandboxes need a new sandbox name to receive the mapping; setup does
not migrate an existing local cache.

## Boundaries

- Setup accepts matching symlinks but refuses existing files, directories, or
  different links. It never deletes or migrates existing data.
- New mappings or a different state root require a new sandbox name. Attaching
  does not change mounts or rerun setup. Keep any old state-path symlinks while
  existing sandboxes still depend on them.
- Custom `HF_HOME`, `XDG_*`, npm cache, or pnpm store/cache settings may bypass
  these locations. Sharing directories does not install tools or grant network access.
- Avoid simultaneous colgrep writes to the same project's index until upstream
  locking is verified. Use dedicated state roots and manage ignore rules for
  locations outside this checkout's `.cache/`.

## Verify from the host

```bash
sbx exec <sandbox-name> sh -c 'readlink "$HOME/.cache/huggingface/hub"; readlink "$HOME/.local/share/colgrep"; readlink "$HOME/.local/share/pnpm/store"'
sbx exec <sandbox-name> pnpm store path
```

The [regression suite](../AGENTS.md#verification) checks real links in isolated
fixture homes; it does not run real SBX, colgrep, or pnpm.
