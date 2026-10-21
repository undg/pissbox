---
name: skill-creator
description: Create or revise Pi skills with clear triggers, concise instructions, and correct global or project placement. Use when the user asks to create, improve, or move a skill.
---

# Create skills

Build the smallest skill that captures a reusable workflow. Do not turn a one-off request into a framework.

## Read the official documentation first

Before creating, revising, or moving a skill, always read the installed Pi `docs/skills.md` completely and follow relevant documentation cross-references. Do not skip this because the task seems simple or the destination is already known. If the documentation is unavailable, say so rather than silently relying on this skill alone.

This skill complements the official documentation with the user's workflow and placement preferences; it does not replace it. Use current Pi documentation for supported formats, discovery rules, and APIs.

## Choose the location

1. Confirm the requested scope from the conversation. Ask only when it is unclear.
2. For global Pi skills, use `~/.pi/agent/skills/<name>/SKILL.md`, alongside the user's existing global skills. If Pi uses a custom agent directory, verify it before writing.
3. For project skills, use the project's established skill location. If none exists, use `.agents/skills/<name>/SKILL.md` at the repository root. Do not put a global skill inside a repository.
4. Inspect the exact destination and check known loaded skill names before writing. Report name collisions across global and project skills: Pi keeps the first discovered skill. Do not delete conflicting skills or overwrite an existing skill without reading it. Do not search unrelated directories.
5. When moving a skill, preserve all supporting files, check for destination conflicts, and verify the source is gone and the destination is intact. Do not leave duplicate copies.

## Gather the workflow

1. Use the user's request, examples, and corrections as the source of requirements.
2. Identify when the skill should activate, what inputs it needs, what actions it permits, and what output it should produce.
3. Ask focused questions only for missing information that would change the workflow. Never invent project conventions or tool behavior.
4. Read relevant configuration documentation when the workflow involves configuration. Prefer current documentation over remembered paths or APIs.

## Write SKILL.md

Start with YAML frontmatter:

```markdown
---
name: skill-name
description: Describe what the skill does and when to use it.
---

# Skill title

Direct, actionable instructions.
```

1. Match the name to its directory. Use lowercase letters, numbers, and single hyphens; no leading or trailing hyphen. Keep the name within 64 characters.
2. Make the description specific enough for automatic discovery. Include both purpose and activation conditions; keep it within 1024 characters.
3. Write concise imperative instructions: prerequisites, workflow, boundaries, and expected output. Include examples only when they clarify a non-obvious rule.
4. Preserve meaningful user preferences. Remove generic advice, repeated instructions, and explanations the agent does not need.
5. Keep reusable instructions portable. Do not embed sandbox-specific absolute paths or unrelated project details in a global skill.
6. Add `references/`, `scripts/`, or `assets/` only when needed. Link supporting files from SKILL.md using relative paths. Do not add README files, scaffolding, or scripts just for ceremony.
7. Skill instructions do not grant permission to commit, publish, delete user work, access secrets, or modify unrelated configuration.

## Verify

1. Read the finished file. Check frontmatter, directory/name agreement, trigger clarity, and referenced files.
2. Check that the result follows the user's requested scope, that known name collisions have been addressed, and that no duplicate copies were created during this task.
3. When feasible, try a representative request and check whether the instructions produce the intended behavior. Do not claim runtime discovery or execution was tested unless it was.
4. Report the exact created or updated path. Tell the user to run `/reload` when skill discovery needs refreshing. Keep the response short.
