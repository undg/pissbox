# Host clipboard

The optional bridge tool `clipboard_copy({ text })` copies text from an agent to
your host's normal clipboard. It never reads the clipboard or targets X11's
primary selection. Use it when you explicitly ask the agent to copy something.
An empty string clears the clipboard; whitespace and trailing newlines are
preserved. The result names the successful backend, for example:

```json
{"backend":"pbcopy"}
```

## Host requirements

No sandbox package or desktop socket mount is needed. Install an applicable
clipboard helper on the **host**, on the bridge daemon's `PATH`:

| Host session | Commands, in preference order |
| --- | --- |
| macOS | `pbcopy` |
| Linux, Wayland available | `wl-copy --type text/plain;charset=utf-8` |
| Linux, X11 available | `xclip -selection clipboard`, then `xsel --clipboard --input` |

When both Wayland and X11 are available, Wayland is tried first, then the X11
commands. Missing or failing commands fall through to the next applicable
backend. Each attempt has a two-second timeout. A headless Linux launch without
`WAYLAND_DISPLAY` or `DISPLAY` fails explicitly, even if helpers are installed.
There is no OSC 52, Windows, arbitrary command, or clipboard-read fallback.

These helpers are optional: the installer neither checks nor installs them.
The rest of the bridge still works when clipboard support is unavailable.

## Setup and lifecycle

```bash
make build-bridge
bin/pissbox --bridge-reload
```

Reload Pi extensions with `/reload` in existing sessions. Alternate Pi directories
also need `extensions/lib/clipboard-tools.js` alongside the other bridge helpers.
Launchers already running an older bridge client need a new launcher attach to
supply per-launch desktop settings.

Each host registration captures `DISPLAY`, `WAYLAND_DISPLAY`, `XDG_RUNTIME_DIR`,
and `XAUTHORITY` from its host bridge client, not the sandbox. These values stay
on the host and survive daemon reloads. Different launches can use different
graphical sessions without inheriting the first launch's desktop. Reattach from
the correct host graphical session if these values change. Helpers still inherit
the daemon's host `PATH`; reload the bridge from the correct host environment if
that path changes.

## Safety and limits

1. The input is text only, at most 16 KiB of UTF-8. The shared 32 KiB JSON record
   limit also applies, so heavily escaped input may hit that limit sooner.
2. Commands and arguments are fixed by the host. Text goes through stdin, never
   through shell interpolation or command-line arguments. Helper stdout/stderr
   are discarded and the bridge does not log clipboard text. Tool-call arguments
   remain part of the ordinary Pi conversation and transient mailbox transport.
3. A bounded, serialized worker performs writes without blocking peer messaging,
   readiness, or the host control socket. Queue time counts toward the eight-second
   request deadline. Expired queued writes are not executed.
4. A consumed request ID is not executed again within the live router. Reloads,
   transport failures, and cancellation never automatically replay writes. If a
   call is interrupted, the clipboard may already have changed; do not retry
   automatically. A later explicit copy is a new write, not a transaction rollback.
