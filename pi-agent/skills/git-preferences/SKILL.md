---
name: git-preferences
description: Apply the user's Git history preferences when pulling changes, updating branches, or choosing between rebase and merge.
---

# Git preferences

1. Prefer linear history. Use `git pull --rebase` when pulling changes.
2. On a solo, isolated branch, prefer rebasing onto the branch it was forked from. The common case is rebasing a feature branch onto the latest `main`; verify the actual base and remote rather than assuming them.
3. Preserve commit order and boundaries. Do not reorder commits; squash only when explicitly requested. Avoid elaborate history rewrites.
4. Prefer a merge when collaboration or numerous difficult conflicts make rebasing risky or needlessly painful. Do not rewrite shared history without agreement.
5. If a rebase becomes conflict-heavy, stop and discuss merging instead. Do not silently switch strategies or discard work.
6. Do not push changes to the upstream repository unless explicitly requested. Check remote URLs before pushing; remote names alone do not identify a fork or upstream.
7. These preferences do not authorize commits, pushes, or destructive actions. If publishing rewritten history requires a force push, ask first and use `--force-with-lease`, never plain `--force`.
