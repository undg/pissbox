---
name: planning
description: Create, revise, or execute persistent Markdown plans for multi-step work. Use when asked for a plan or to work through an existing plan; planning alone does not authorize implementation.
---

# Planning and execution

Use one Markdown file as the source of truth. Do not maintain a duplicate tool-based todo list.

## Create or revise a plan

1. Read the request, relevant repository instructions, and existing plan before proposing changes.
2. Record the goal, agreed scope, constraints, evidence, decisions, and unresolved questions. Separate proposals from agreements.
3. Break approved work into concrete tasks ordered by dependencies. Include success criteria and relevant verification; avoid ceremonial tasks and speculative features.
4. Use the user's requested path or the destination's convention. A repository or managed note store may define the path; if none exists, use `plan.md` in the project root. Do not overwrite an unrelated plan; ask for a path if it conflicts.
5. Save with the destination's write mechanism. Stop when the user asked only for planning. Do not treat a task list as permission to execute it.
6. Keep content and storage separate. This skill owns the plan shape; a destination skill or convention owns its path, frontmatter, naming, and commits. Do not duplicate those rules here.

Keep the plan proportionate to the work. Simple one-step requests do not need a new plan file.

## Execute an approved plan

1. Read the current plan and confirm which work is authorized. If the plan conflicts with the user's request or repository rules, resolve that before proceeding.
2. Mark one ready task `[!]` before starting. Preserve the user's existing format when it differs from this skill.
3. Work within that task's scope. Stop and record blockers or decisions requiring user input rather than guessing.
4. Run meaningful checks. Mark `[x]` only when the task's acceptance criteria are met; record checks actually run and verification gaps.
5. Update the plan when evidence changes scope, dependencies, or decisions. Do not silently drop unfinished tasks.
6. Continue through approved tasks. Commit, push, install dependencies, or perform destructive operations only when separately authorized by the user or applicable repository instructions—not merely because the plan lists them.
7. Once Steps and Verification are complete, add `Handoff checks and notes` from the actual diff: a few review notes, then ordered checks. A plan is not ready for handoff until that section exists. Do not mark user checks complete on the user's behalf.

## Task markers

- `[ ]` — pending
- `[!]` — in progress; at most one at a time
- `[x]` — complete

User handoff checks stay `[ ]` at handoff; only the user marks them complete.

For blocked or cancelled work, add an explicit note with the reason rather than marking it complete.

## Plan document

A plan is read by a human before execution and again after handoff. Put the outcome first and use only sections that help.

1. **TL;DR** — outcome, rough size, main risk or open decision.
2. **Why** — problem and evidence. Keep facts and guesses apart.
3. **Goal and scope** — what is in and what is deliberately out.
4. **Decisions** — agreed, recommended, unresolved.
5. **Steps** — ordered, concrete, checkboxes. At most one `[!]`.
6. **Verification** — checks the agent can run. Distinguish run from planned.
7. **Files** — the short set of paths involved.
8. **Risks and out of scope** — what could bite, what this refuses to do.
9. **Handoff checks and notes** — added after execution; review notes plus user checks.

Small plans may use only TL;DR, goal/scope, steps and verification. Do not pad.

### Handoff checks and notes

Add this after execution, before handoff. Do not write it during planning: only the actual change shows what needs review and manual proof.

#### Notes

A few short notes for the reviewer, only when the diff does not make them obvious:

- Where to start review.
- Non-obvious decisions or tradeoffs.
- Caveats, risks, or parts you are unsure about.
- Anything the user should know before accepting the change.

Do not repeat the Decisions or Risks sections, and do not summarize the diff.

#### Checks

The user's post-handoff checklist: code and design review first, then manual acceptance. It is not limited to things the agent cannot run. The user may repeat an automated check when judgment or the real environment matters.

- Order by importance: review first, then highest-risk or highest-impact manual checks.
- Point to what to review: the diff, key decisions, and risky files.
- For runtime checks, prefer a few broad end-to-end scenarios over many isolated ones. Manual checks are expensive; each must cover a distinct risk. Split one only when setup or failure diagnosis differs.
- Give exact steps, expected result, failure meaning, and where to run it.
- Do not duplicate routine automated coverage. List a check when human judgment, a real environment, or user acceptance matters, even if an automated test also covers it.
- Leave the boxes unchecked. Only the user marks them complete.
- If review or a check fails, the work is not done; record the finding and reopen the relevant task.

```markdown
# Feature plan

**TL;DR** — outcome, rough size, main risk.

## Why
Problem and evidence.

## Goal and scope
In scope and deliberately out.

## Decisions
Agreed, recommended, unresolved.

## Steps
- [ ] Ordered, concrete task.

## Verification
- [ ] Agent check.

## Files
- `path/to/file`

## Risks and out of scope
What could bite; what this plan refuses to do.
```

Use only sections that help. After execution the plan gains the `Handoff checks and notes` section above.
