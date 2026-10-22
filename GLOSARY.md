# Glossary

- **Bridge daemon** — one persistent host process providing capabilities to sandboxes.
- **Capability** — a named function the bridge provides, such as lifecycle reporting or agent messaging.
- **Adapter** — translates between a bridge capability and an external system, such as Herd.

## Bridge messaging system

- **Agent messaging** — the bridge capability for discovering live Pi agents and
  exchanging addressed text messages. Exposed through `agent_list` and
  `agent_send`; transported through filesystem mailboxes, not sandbox networking.
- **Live agent address** — a random ID identifying one running agent incarnation.
  Session titles and workspace paths are metadata, not routing addresses. Session
  replacement or extension reload retires the old address.
- **Peer message** — a displayed custom Pi message from another agent, distinct
  from human input. It wakes an idle agent or queues a follow-up for a busy one.
- **Queue receipt** — acknowledgement that the recipient inserted a message into
  Pi's queue, not that it finished reasoning or sent an answer.
- **Indeterminate delivery** — insertion may have happened, but no receipt was
  confirmed. Never automatically resend: that could deliver the message twice.

### Why peer messages are not commands

Agent messaging never feeds peer text into Pi's user-input/command path. Text
such as `/help` or `/new` remains conversation content, rather than opening help
or resetting another agent's session. Only actual user command input should
trigger those actions. This prevents accidental command execution; shared
writable mailboxes remain a trusted local transport, not an isolation boundary.

See [bridge usage and protocol](go-bridge/README.md#agent-messaging).
