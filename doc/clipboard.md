# Host clipboard

The bridge tool `clipboard_copy({ text, shellCommand? })` puts text or a prepared
shell command on your host's normal clipboard. It never reads the clipboard or
targets X11's primary selection. Its prompt guidelines instruct agents to use it
automatically whenever they hand you exactly one command to run in your terminal,
not only when you explicitly ask to copy something.

With `shellCommand` omitted or false, text is copied exactly. An empty string
clears the clipboard; whitespace and trailing newlines are preserved. The tool
returns the backend and exact payload for display in chat, for example:

```json
{"backend":"pbcopy","copiedText":"hello"}
```

## Shell commands and output capture

For one noninteractive command, the agent passes the original command with
`shellCommand: true`. The host prepares a Bash/zsh wrapper using the backend that
successfully copies it. For example, `make check` on macOS becomes:

```bash
(set -o pipefail; {
make check
} 2>&1 | tee /dev/tty | pbcopy)
```

**Copying never executes the command.** After you paste it into your terminal,
stdout and stderr remain visible and also replace your clipboard so you can paste
the output back into chat. Failures are not hidden by the capture pipeline, and
`pipefail` is scoped to the subshell rather than changing your interactive shell.
On Linux the suffix instead uses `wl-copy`, `xclip -selection clipboard`, or
`xsel --clipboard --input`, including their fixed arguments.

The copied command requires Bash or zsh, `tee`, an interactive terminal providing
`/dev/tty`, and the selected host clipboard helper. These optional capabilities
are neither checked nor installed by the installer. Clipboard availability can
change between preparation and execution.

The source command is preserved inside the wrapper, including compound commands,
pipelines, trailing comments, and here-documents. It runs in a subshell with piped
output: use plain-text mode for `cd`/`export` commands that must modify your current
shell, interactive/full-screen programs, commands already capturing output, or
commands requested verbatim. Ordinary prose and example snippets are never
implicitly interpreted as shell commands by the tool. Mode selection is explicit;
the tool does not guess whether text is code.

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

1. Clipboard text is at most 16 KiB of UTF-8, including the wrapper in shell-command
   mode. The shared 32 KiB JSON record limit also applies; command mode reserves
   response space before writing, so heavily escaped input may hit a limit sooner.
   Oversized prepared commands are rejected, never truncated.
2. Clipboard helper commands and arguments are fixed by the host. Payloads go
   through stdin, never shell interpolation or command-line arguments. Even in
   command mode the bridge copies source text without evaluating it. Helper
   stdout/stderr are discarded and the bridge does not log clipboard text.
   Tool arguments and the returned payload remain in the ordinary Pi conversation
   and transient mailbox transport.
3. A bounded, serialized worker performs writes without blocking peer messaging,
   readiness, or the host control socket. Queue time counts toward the eight-second
   request deadline. Expired queued writes are not executed.
4. A consumed request ID is not executed again within the live router. Reloads,
   transport failures, and cancellation never automatically replay writes. If a
   call is interrupted, the clipboard may already have changed; do not retry
   automatically. A later explicit copy is a new write, not a transaction rollback.
