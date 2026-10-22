# go-bridge

Optional host-side bridge between sandbox Pi and host integrations. First scope:
Herd sidebar state and Herd's existing completion/attention notifications.
No MCP endpoint, HTTP listener, permanent daemon, or third-party Go dependencies.

## Build

Requires **Go 1.24+ on the build machine only**:

```bash
bin/build-bridge        # Current host OS and CPU
bin/build-bridge --all  # darwin/linux × arm64/amd64, from either OS
```

Binaries go to the ignored `.cache/bin/go-bridge-<os>-<arch>` paths.
All builds use `CGO_ENABLED=0`; Linux can cross-compile macOS binaries without
an Apple SDK. Packaged hosts need only their matching binary, not Go. Building
for another target does not test its runtime behavior.

The launcher selects the binary from this checkout, independently of a custom
`PISSBOX_STATE_DIR`. It never compiles or installs tools automatically.

## Use

Build, then launch `bin/pissbox` from a normal Herd terminal pane. The launcher
uses `HERDR_ENV=1`, `HERDR_SOCKET_PATH`, and `HERDR_PANE_ID` on the host. Outside
Herd it starts no helper. Missing binaries/extensions and startup failures warn
without preventing Pi from starting.

The selected Pi directory must contain `extensions/pissbox-bridge.js`. The
repository's default `pi-agent/` already does; alternate Pi directories need
the extension too. Pi discovers it automatically. The extension is plain JS
with JSDoc types so lifecycle tests need only Node, not a TS compiler.

```text
Pi lifecycle → atomic mailbox events → Go helper → Herd local socket
```

Herd sees the custom agent `pissbox-pi`, source `pissbox:pi`. The official Pi
identity depends on native host process detection, which cannot see Pi inside
SBX. The example Herd-managed extension is not used by this bridge.

The extension reports idle at startup, working at agent start, blocked during
blocking extension UI prompts, and idle only after `agent_settled` confirms no
automatic continuation remains. It releases on session shutdown. Headless Pi
sessions do not report. This needs a Pi version exposing `agent_settled` and
`ui_prompt_start`/`ui_prompt_end`; ordinary model/tool output is not inspected.

The helper stops and releases when the launcher exits/disconnects; it also
checks for parent-process exit. It does not supervise or stop sandbox Pi.
Reattaching through the launcher's Pi choice gets a new helper and mailbox;
choosing shell does not. Direct `sbx run` bypasses this integration.

## Mailbox and boundary

Each launch creates a mailbox under `PISSBOX_STATE_DIR/bridge-launch.*`, already
inside the shared-state mount. Only its path is passed into the sandbox as
`PISSBOX_BRIDGE_DIR`. The Herd socket path and pane ID stay on the host.

Events are small immutable JSON files, published by atomic rename and named
with a monotonic timestamp and random suffix. Reading them in filename order
preserves quick working → idle transitions between polls. The helper polls at
100 ms, validates and forwards events, and removes consumed event files. Failed
Herd requests remain queued for retry with a bounded socket timeout. Messages
contain no prompts, source contents, credentials, or session logs.

The version-1 envelope accepts only:

```json
{"version":1,"kind":"state","state":"working"}
{"version":1,"kind":"release"}
```

State may be `idle`, `working`, or `blocked`, with an optional message of at most
512 bytes without control characters. Unknown fields, other event kinds,
oversized files, symlinks, and non-regular files are rejected. Reads are rooted in the mailbox. The host fixes
the adapter, pane, source, and agent; sandbox events cannot request arbitrary
Herd methods, host commands, or resume commands.

The helper refreshes a readiness marker every second. The extension stops
publishing to a missing/stale helper and warns once if an active bridge fails.
A stale environment from an earlier sandbox launch is silently ignored at
startup. A mailbox with more than 4096 directory entries disables the helper;
after its marker expires the extension stops publishing. Empty mailboxes and
incomplete pending files are retained; no background cleanup removes shared
state.

**This is not an isolation boundary between sandboxes sharing writable state.**
Any process that can write another launch's mailbox can forge that launch's
state events. Per-launch directories prevent accidental mixing, not a malicious
shared-state writer. Host operations remain limited to reporting/releasing the
bound Herd agent.

## Extend

`mailbox.go` owns the envelope and safe file reads; `herdr.go` owns Herd requests;
`main.go` owns polling, retries, readiness, and shutdown. The small adapter
interface separates transport from host integration. Add capabilities explicitly
with validation and tests, not a generic host RPC passthrough.

Review comments, session-log visibility, automatic session restore, and binary
bundle packaging are not implemented. Herd's notification delivery and active-tab
suppression settings still apply.

## Verify

From the repository root:

```bash
(cd go-bridge && go test -race ./... && go vet ./...)
node --test go-bridge/pi-extension.test.mjs
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s tests -v
bin/build-bridge --all
```

Go tests use a real local Unix socket with a simulated Herd responder. Launcher
tests stub SBX and the helper; JS tests drive extension lifecycle callbacks.
These do not prove a real Herd/SBX connection or macOS runtime compatibility.

For the real smoke test, launch from Herd, check `herdr agent list` for
`pissbox-pi`, run a task, and observe working → idle. Switch away from its tab
to test completion notifications, exercise a blocking extension UI prompt for
attention, then disconnect and confirm the custom agent is released.
