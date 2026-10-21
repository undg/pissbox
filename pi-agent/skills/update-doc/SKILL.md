---
name: update-doc
description: Create or update Go, JavaScript, TypeScript, Python, and Lua API documentation. Use for godoc, JSDoc, TSDoc, Python docstrings, or EmmyLua annotations.
---

# Update documentation

## Workflow

1. Read the affected code and surrounding documentation conventions.
2. Read the matching bundled reference below, relative to this skill directory.
3. Change only the documentation needed for the task. If code and docs disagree, establish the intended behavior before changing either.
4. Review the result against the implementation and run relevant, available project checks.

## Rules

- Follow the project's conventions first.
- Document intent, behavior, errors, side effects, and constraints callers need.
- Do not repeat types unless the language needs annotations for typing.
- Skip trivial comments when the signature already explains the behavior.
- Do not invent guarantees, exceptions, fallback behavior, or examples the implementation does not support.
- Keep changes small. Do not install documentation tools or hand off to unavailable review agents.

## References

| Language | Reference | Typical checks, when configured |
| --- | --- | --- |
| Go | `references/godoc.md` | `go doc`, `go vet`, project tests |
| JavaScript | `references/jsdoc.md` | Project linter, type checker, or docs generator |
| TypeScript | `references/tsdoc.md` | Project linter, type checker, or docs generator |
| Python | `references/pydoc.md` | Project docstring linter and tests |
| Lua | `references/luadoc.md` | Project EmmyLua/LuaLS checks |

Report checks actually run and any verification gaps. A successful formatter does not prove documentation is accurate.
