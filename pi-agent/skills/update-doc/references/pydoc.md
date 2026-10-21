# Python docstrings

- Place a function's docstring inside its body as the first statement.
- Follow the existing style; prefer Google style only when the project has no convention.
- Explain behavior, side effects, exceptions, and non-obvious defaults.
- Document arguments and returns when their semantics are not already clear.
- Do not repeat type hints or add long narratives to trivial functions.

```python
def find_user(users: dict[str, str], user_id: str) -> str | None:
    """Return the user's name, or None if the ID is unknown."""
    return users.get(user_id)
```
