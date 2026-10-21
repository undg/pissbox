---
name: commit-message
description: Write concise commit messages that capture change intent, rationale, and meaningful scope for later human and agent PR review. Use when drafting or revising commit messages.
---

# Commit messages

Code is the source of truth for implementation. Commit history records the steps taken, why they were taken, and briefly what changed. Future reviewers, including agents, use it to understand intent without reconstructing the whole conversation.

## Before writing

1. Read recent commit messages to match the repository's convention, including ticket scopes when established.
2. Inspect the relevant diff. For an existing commit, inspect that commit rather than assuming the working tree still contains its changes.
3. Use the conversation to recover intent and rationale. Never invent a reason or ticket number.

## Format

- Use a short, specific subject describing the intended change.
- Add a blank line and usually 1–3 short bullets only when they add useful context beyond the subject. A subject alone is fine.
- Prioritize why the change matters, meaningful behavior, deliberate scope, and non-obvious decisions.
- Mention reuse only when it explains a meaningful decision, not as an inventory of components.
- When asked for a message, return just the message in a code block. Do not commit or amend unless explicitly asked.

## Cut noise

- No test counts, passing-test claims, lint/type-check/formatting results, or routine verification notes. Those belong in PR checks, not the commit narrative.
- No obvious domain facts or descriptions of things that could not apply anyway.
- No implementation walkthroughs, file lists, component inventories, commit hashes, or tool/environment trivia unless essential to understanding the decision.
- No sections such as "Review context" or "Verification" for an ordinary small change.
- Do not restate the subject in the body or pad a small change into a long message.
- For each bullet ask: does this preserve useful intent, rationale, or scope that is not already obvious? If not, delete it.

## Example

```text
feat(592277): add table button to historic mnemonic

- Let users inspect historic values without leaving node metadata
- Keep previews available in read-only forms
```

Avoid bullets such as:

```text
- Add 6 tests; all 95 node-form tests and static checks pass
- Show historic values without scenario tabs
```

The first reports routine verification. The second states an obvious property of historic data. Neither helps a later reviewer understand why this change exists.
