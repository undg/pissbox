---
name: project-setup
description: Create or adjust .pissbox.toml sandbox configuration and .wk.toml worktree setup for a project. Use when preparing a repository for new sandbox agents or wk worktrees, choosing dependency isolation, mounts, or first-time setup commands.
---

# Project setup

Use `.pissbox.toml` and `.wk.toml`, not executable `.pissbox` files or the `.wk/` worktree container. Configure only the requested files; preserve existing settings and unrelated work.

## Inspect first

Read project instructions, existing configs, dependency manifests/lockfiles, README development instructions, and build targets (`make help` when available). Identify the actual install, test, formatting, and optional quality-tool commands. Do not copy frontend setup into other stacks.

Consult available pissbox/wk documentation for unfamiliar behavior. Ask only about choices that cannot be inferred safely.

## .pissbox.toml — Linux sandbox

Place it in the primary workspace. Supported keys are arrays of strings: `workspaces`, `isolated_directories`, and `setup`. Omit unnecessary keys.

1. Add only required extra read-write mounts. Do not mount a broad ancestor by default. Linked Git worktrees need access to their owning Git metadata; symlinks need their targets mounted. Relative workspace paths resolve from the primary workspace, even for symlinked configs. No `~` or environment expansion.
2. Isolate project-local platform-specific dependencies/build outputs that could overwrite host artifacts: explicit `node_modules` directories, `.venv`, or `bin` as appropriate. Inspect the project first. Go caches normally live outside the checkout and need no project isolation; Linux tool state must remain separate from native host state.
3. Isolation paths must be normalized project-relative directories, with no symlink components, duplicates, or overlaps. Never isolate source, Git metadata, lockfiles, or `.wk/`. Keep reports shared unless requested otherwise.
4. Setup runs once on creation, as UID 1000, in one strict Bash process starting in the primary workspace. Use repeat-safe dependency installation with the project's lockfile policy. Check required compiler/toolchain versions. Include existing quality-tool installation targets when agents are expected to run those checks; downloads need network access. Do not run formatting, tidy, tests, or application builds automatically merely to prepare an agent.
5. Setup exports do not persist into Pi. Existing sandbox attachments neither rerun setup nor change mounts. Tell the user to use a new sandbox name for changed isolation/mounts; never delete a sandbox automatically.

Example for a Go project with a `bin` build directory and an existing `make tools` target:

```toml
isolated_directories = ["bin"]
setup = ["go mod download", "make tools"]
```

## .wk.toml — worktree lifecycle

Place it at the owning project root, not merely a linked worktree's tracked copy. Git metadata determines ownership.

1. Preserve or infer `name`; do not guess a remote/base branch. `base_ref` selects the starting ref for new branches in bare repositories.
2. `setup` runs in each target worktree after creation. Add only worktree-specific dependencies or necessary links to untracked shared files. In the standard `<project>/.wk/<name>` layout, project-root shared files are reached through `../../`; verify the actual layout before using that path. Never replace tracked files with links.
3. `teardown` runs before removal. Omit it unless real cleanup is needed; do not invent destructive cleanup commands.
4. Keep sandbox-only tool installation and Linux isolation in pissbox configuration. Do not duplicate expensive setup across both lifecycles without a reason. Do not run `wk init` over existing configuration just to generate a template.

## Connect wk worktrees to pissbox

Link the **`.pissbox.toml` config**, not the `pissbox` executable. The launcher stays on the host PATH. Pissbox reads only the primary workspace's config; it does not search the owning repository or parent directories.

For the standard wk layout:

```text
project/
  .pissbox.toml
  .wk.toml
  .wk/task/
    .pissbox.toml -> ../../.pissbox.toml
```

If the config is an untracked shared root file, add this to the owning project's `.wk.toml` setup, preserving other commands:

```toml
setup = [
  '''
  if [ ! -e .pissbox.toml ] && [ ! -L .pissbox.toml ]; then
    ln -s ../../.pissbox.toml .pissbox.toml
  fi
  ''',
]
```

Keep an existing tracked config. Inspect existing files/links rather than forcing replacement; the guard deliberately preserves broken or different links for review. Changing `.wk.toml` does not repair existing worktrees automatically.

A host-readable config link is not enough: its target must also be mounted inside the sandbox. For a worktree at `<project>/.wk/task`, this entry in the shared `.pissbox.toml` exposes the owning root, including the config target and usual linked-worktree Git metadata:

```toml
workspaces = ["../.."]
```

`../..` resolves from **the primary worktree**, not from the shared config's directory. The same entry used when launching from the owning root would mount that root's grandparent instead. Check every intended launch location before choosing a shared relative entry; do not add it blindly or assume it always means "owning repository." Use an explicit owning-root path or separate configs when the launch locations need different mounts. Mount only the directories actually needed and disclose that mounts are read-write.

Verify the config link and the worktree's `.git` metadata paths are reachable through the selected mounts. Do not auto-mount symlink targets. With `--ignore-config-workspaces`, required target/metadata mounts must be supplied on the CLI. If host validation succeeds but sandbox setup says the TOML file is unreadable, check for an unmounted symlink target before changing permissions. Changed mounts require a new sandbox name; failed isolation setup is not replayed on attach.

## Finish

Review changes for valid TOML, repeat-safe commands, minimal mount exposure, and host artifact protection. Use an existing TOML validator if available. Do not launch sandboxes, create worktrees, install tools, or run setup without authorization. Report changed paths and any required new-sandbox launch; do not claim setup was executed when only configuration changed.
