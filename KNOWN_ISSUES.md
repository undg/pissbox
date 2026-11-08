# Known issues

Defects a user can hit with the current checkout or image. One entry each:
symptom, impact, workaround, next action. Open design work and long
investigations are not kept here.

## Bridge

**Reconnect cancellation can orphan a registration (P2).** Killing a launcher
mid-reconnect leaves the new registration active; a fresh mailbox on the same
Herd pane then fails with `mailbox or Herd pane already registered`, and
messaging plus Herd reporting stay unavailable for that attach. Introduced in
`a1d651d`; still unfixed.

Workaround: exit the launcher fully, let owner-liveness cleanup release the
registration, relaunch. No sandbox removal is needed.

**Bridge reload is not transactional for in-flight work.** A send or vault
mutation that crosses `bridge reload` can fail as indeterminate. Never retry
automatically. Reload also changes the daemon PID; the first upgrade from a
daemon without `reload` needs one explicit `--bridge-stop` and launch.

## Tools

**`pi update --models` fails.** Fails with insufficient permissions. Low
priority; no workaround.

## Where the details are

Reproduction steps, evidence and probe checklists for the bridge entries live in
the maintainer vault, `Projects/pissbox/known-issues.md` (KI-002, KI-003).
