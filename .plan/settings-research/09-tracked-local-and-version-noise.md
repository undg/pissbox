# 09 — Tracked vs local profiles, and version noise

Read `.plan/README.md` (shared context) and `AGENTS.md` first.

Two coupled decisions: should profile files live in git or be machine-local,
and how to handle `lastChangelogVersion` plus Pi version divergence across
workstations. Build a matrix and recommend.

## Questions

1. `lastChangelogVersion`: can it be omitted from tracked settings? Does Pi
   re-add it? What depends on it, and is a stale value harmful?
2. Git scenario matrix for candidate layouts (single tracked file / tracked
   profiles / local profiles + tracked base / generated `settings.json`):
   - Pi version change on workstation A
   - Ctrl+S model save on A
   - `git pull` on B
   - both workstations change different keys
   Which produce conflicts or permanently dirty tracked files?
3. Should the image stay `docker.io/sbx/pi-image:latest` (`sbx-kit/spec.yaml`)
   or be pinned to keep Pi versions aligned? Repo rule: kit/image changes
   require a new sandbox. What is the simplest useful option?
4. Is `git update-index --skip-worktree` or `.gitattributes` worth it for
   runtime state? Verify and dismiss explicitly if not.
5. Sessions record model/thinking per session (`docs/sessions.md`) — does that
   reduce the importance of shared `settings.json` defaults?

## Constraints

- Design only; no edits to tracked files.

## Deliverable

`.plan/results/09-tracked-local-and-version-noise.md`: scenario matrix,
recommendation, explicitly dismissed options.
