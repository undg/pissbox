# go-bridge

Optional host-side bridge between sandbox Pi and host integrations. First scope:
Herd sidebar state and Herd's existing completion/attention notifications.
One persistent daemon per host OS user, shared across checkouts and state roots.
No MCP endpoint, HTTP listener, service manager, or third-party Go dependencies.

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
Herd it never contacts the daemon. Missing binaries/extensions and startup failures warn
without preventing Pi from starting.

The selected Pi directory must contain `extensions/pissbox-bridge.js`. The
repository's default `pi-agent/` already does; alternate Pi directories need
the extension too. Pi discovers it automatically. The extension is plain JS
with JSDoc types so lifecycle tests need only Node, not a TS compiler.

```text
launchers → host control socket → persistent Go daemon
                                  ├─ mailbox A → Herd pane A
Pi lifecycle → atomic events ─────└─ mailbox B → Herd pane B
```

Herd sees the custom agent `pissbox-pi`, source `pissbox:pi`. The official Pi
identity depends on native host process detection, which cannot see Pi inside
SBX. The example Herd-managed extension is not used by this bridge.

The extension reports idle at startup, working at agent start, blocked during
blocking extension UI prompts, and idle only after `agent_settled` confirms no
automatic continuation remains. It releases on session shutdown. Headless Pi
sessions do not report. This needs a Pi version exposing `agent_settled` and
`ui_prompt_start`/`ui_prompt_end`; ordinary model/tool output is not inspected.

The launcher registers a fresh mailbox with its host-owned socket, pane, and
launcher PID. Normal exit unregisters that opaque registration ID, drains final
events, and best-effort releases only its own Herd agent. The daemon also polls
owner PID liveness for launcher crashes. Duplicate mailbox or socket/pane bindings
are refused, including while the old registration is draining. Workers are
independent: one failed Herd connection or mailbox cannot stall the other panes.

The daemon survives launcher exit and can remain idle indefinitely. It does not
supervise or stop sandbox Pi. Reattaching through the launcher's Pi choice gets
a new registration and mailbox; shell/quit/delete do not register another.
Help/list/delete commands never contact the daemon. Direct `sbx run` bypasses
this integration.

## Daemon control and restart

The runtime directory is Go's per-user cache directory plus `pissbox/bridge`:
`~/Library/Caches/pissbox/bridge` on macOS; `$XDG_CACHE_HOME/pissbox/bridge` on
Linux, or `~/.cache/pissbox/bridge` when XDG is unset. It is independent of the
checkout and `PISSBOX_STATE_DIR`. It must be owned by the current user with mode
0700. `control.sock` is host-only, mode 0600; `daemon.lock` holds an OS flock for
the daemon's lifetime. A versioned handshake checks readiness. PID/socket files
alone never establish ownership. Concurrent starters converge on the lock holder;
a stale socket is recovered only under the lock. Standard streams are detached
from the launcher's terminal and pipes.

Use the matching binary for your host (example: macOS arm64):

```bash
.cache/bin/go-bridge-darwin-arm64 ensure  # Start if absent; otherwise check compatibility
.cache/bin/go-bridge-darwin-arm64 status  # JSON: protocol version, PID, registration count
.cache/bin/go-bridge-darwin-arm64 stop    # Explicitly stop and release all registrations
```

`register --mailbox DIR --herdr-socket PATH --herdr-pane ID --owner-pid PID`
ensures the daemon, then prints an opaque ID after mailbox readiness is installed.
`unregister --id ID` drains/releases that registration only; it never starts or
stops a daemon. The launcher uses these operations automatically. Control messages
are capped at 16 KiB with 2-second IO deadlines; startup readiness has a 3-second
retry window. Socket paths are limited to 103 bytes for macOS compatibility.
`PISSBOX_BRIDGE_RUNTIME_DIR` overrides the private runtime directory for isolated
tests; setting different overrides intentionally creates separate daemons. Do not
mount the runtime directory into SBX.

Registrations exist only in memory. After daemon death, mailbox readiness becomes
stale within five seconds and Pi stops publishing as before. Starting another
launch ensures a new daemon but **does not restore old registrations**. Exit the
old Pi attachment and reattach through `bin/pissbox` (choose Pi), which registers
a fresh mailbox. No sandbox recreation is needed.

For a binary upgrade, close active launches, run `stop` with the old matching
binary, build/install the replacement, then launch again. An incompatible live
daemon is never silently replaced or killed. A protocol-compatible existing
daemon continues running its old executable until explicitly stopped. When moving
from the old per-launch model, first close launches that still own per-launch
helpers; the singleton cannot adopt them. There is no watchdog or automatic
restart for existing sessions.

## Mailbox and boundary

Each launch creates a mailbox under `PISSBOX_STATE_DIR/bridge-launch.*`, already
inside the shared-state mount. Only its path is passed into the sandbox as
`PISSBOX_BRIDGE_DIR`. The Herd socket path and pane ID stay on the host.

Events are small immutable JSON files, published by atomic rename and named
with a monotonic timestamp and random suffix. Reading them in filename order
preserves quick working → idle transitions between polls. Each mailbox worker polls at
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

Each worker refreshes its readiness marker every second. The extension stops
publishing to a missing/stale worker and warns once if an active bridge fails.
A stale environment from an earlier sandbox launch is silently ignored at
startup. A mailbox with more than 4096 directory entries disables that registration;
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
`main.go` owns mailbox polling, retries, readiness, and final drain;
`control.go` owns the host CLI, singleton lock/socket, registrations, and owner
liveness. The small adapter
interface separates transport from host integration. Add capabilities explicitly
with validation and tests, not a generic host RPC passthrough.

Review comments, session-log visibility, automatic session restore, and binary
bundle packaging are not implemented. Herd's notification delivery and active-tab
suppression settings still apply.

## Verify

From the repository root, `make check` runs Go/JS/Python tests, vet, Bash syntax,
and whitespace checks. Cross-build separately with `make build-all`. See
[quality tooling](../tools/quality/README.md) for scoped Go/extension mutation
testing and coverage/CRAP metrics.

```bash
(cd go-bridge && go test -race ./... && go vet ./...)
node --test go-bridge/pi-extension.test.mjs
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s tests -v
bin/build-bridge --all
```

Go tests use real CLI/daemon subprocesses and local Unix sockets with simulated
Herd responders. They cover concurrent startup, crash recovery, incompatible
protocols, independent routing, owner death, and stalled connections. Launcher
tests stub SBX and control operations, plus one real-daemon launcher-crash check;
JS tests drive unchanged extension lifecycle callbacks. All test daemons use
isolated directories under `.cache/test-fixtures/` and are stopped by cleanup.
These do not prove a real Herd/SBX connection or macOS runtime compatibility.

For the real smoke test, launch from two Herd panes and check `status` shows one
PID and two registrations. Check `herdr agent list` for `pissbox-pi`, run tasks,
and observe each pane's independent working → idle states. Switch away from a tab
to test completion notifications and exercise a blocking extension UI prompt for
attention. Close one launcher: only its agent should release; the other must keep
reporting under the same daemon PID. After both close, the daemon remains idle.
