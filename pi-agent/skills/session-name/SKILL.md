---
name: session-name
description: Update the current Pi session title from the conversation or a user-supplied title. Use when the user explicitly asks to name, rename, or refresh the session title.
disable-model-invocation: true
---

# Update session name

1. If the user supplies an exact title, use it. Otherwise choose a specific 3–7 word title reflecting the conversation's current goal and meaningful progress, not merely the first message. Honor any supplied emphasis.
2. Keep the title on one line and within 80 characters. Omit quotes, Markdown, secrets, and personal identifiers.
3. Call `set_session_name` with the title. This explicitly replaces any existing session name.
4. Confirm the saved title briefly. Do not just suggest a title or edit session files. If the tool is unavailable, say so and give the user `/name <title>` to run instead; never claim the rename succeeded.
