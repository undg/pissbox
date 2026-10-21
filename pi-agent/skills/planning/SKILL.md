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
4. Use the user's requested path or existing repository convention. If neither exists, use `plan.md` in the project root. Do not overwrite an unrelated plan; ask for a path if it conflicts.
5. Save the plan and stop when the user asked only for planning. Do not treat a task list as permission to execute it.

Keep the plan proportionate to the work. Simple one-step requests do not need a new plan file.

## Execute an approved plan

1. Read the current plan and confirm which work is authorized. If the plan conflicts with the user's request or repository rules, resolve that before proceeding.
2. Mark one ready task `[!]` before starting. Preserve the user's existing format when it differs from this skill.
3. Work within that task's scope. Stop and record blockers or decisions requiring user input rather than guessing.
4. Run meaningful checks. Mark `[x]` only when the task's acceptance criteria are met; record checks actually run and verification gaps.
5. Update the plan when evidence changes scope, dependencies, or decisions. Do not silently drop unfinished tasks.
6. Continue through approved tasks. Commit, push, install dependencies, or perform destructive operations only when separately authorized by the user or applicable repository instructions—not merely because the plan lists them.

## Task markers

- `[ ]` — pending
- `[!]` — in progress; at most one at a time
- `[x]` — complete

For blocked or cancelled work, add an explicit note with the reason rather than marking it complete.

## Minimal shape

```markdown
# Feature plan

## Goal and scope

Agreed outcome and boundaries. State whether this is planning-only or approved execution.

## Evidence and decisions

Known facts, proposals, agreements, and open questions.

## Tasks

- [ ] Investigate the affected behavior and record acceptance criteria.
- [ ] Implement the approved change.
- [ ] Run relevant checks and record results.
```

Use only sections that help the actual task. For research plans, replace implementation tasks with evidence-gathering tasks.
