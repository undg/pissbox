# Host Go code

The `pissbox/host` module keeps the bridge executable in `cmd/pissbox-bridge`
and its implementation and colocated tests together in `internal/bridge`.
The default Go launcher lives in `cmd/pissbox` and `internal/launcher`.
`bin/pissbox` only resolves symlinks, selects the native binary, and execs it.
The explicit `bin/pissbox-legacy` reference remains until manual acceptance.

Optional host-side bridge providing addressed Pi-to-Pi messaging plus Herd sidebar
state and Herd's existing completion/attention notifications.
One persistent daemon per host OS user, shared across checkouts and state roots.
No MCP endpoint, HTTP listener, service manager, or third-party Go dependencies.

## Build

Requires **Go 1.25+ on the build machine only** (for rooted atomic rename):

```bash
make build-bridge      # Current host OS and CPU
make build-bridge-all  # darwin/linux × arm64/amd64, from either OS
```

Binaries go to the ignored `.cache/bin/go-bridge-<os>-<arch>` paths.
All builds use `CGO_ENABLED=0`; Linux can cross-compile macOS binaries without
an Apple SDK. Packaged hosts need only their matching binary, not Go. Building
for another target does not test its runtime behavior.

The launcher selects the binary from this checkout, independently of a custom
`PISSBOX_STATE_DIR`. It never compiles or installs tools automatically.

## Launcher verification

```bash
make build-launcher      # Explicit native build; also included in make build/install
make build-launcher-all  # darwin/linux × arm64/amd64
PISSBOX_TEST_LAUNCHER=bash make test-python
PISSBOX_TEST_LAUNCHER=go make test-python
```

Launcher binaries use `.cache/bin/pissbox-<os>-<arch>`. The wrapper never builds
or falls back; a missing/non-executable binary reports `bin/build-launcher`.
Both host build scripts select the native host despite inherited GOOS/GOARCH.
Artifact export/import includes all four launcher and unchanged bridge filenames.
Python acceptance tests default to Go, build one fresh isolated candidate per run,
and copy it with the production wrapper into fixture checkouts; they never depend
on installed artifacts. Explicit Bash selection uses `bin/pissbox-legacy`.
Both implementations use public fixture resources, not the developer checkout's
project config or Pi credentials.
The Go launcher locates resources relative to its resolved executable under
`.cache/bin/`, preserves the caller's directory, and invokes the bridge as a
separate CLI process. No production dependencies were added; host Bash, jq and
tomljson remain necessary for the shared project-schema validator.

Go subprocess tests re-execute the instrumented test binary, including signal
forwarding and bridge cleanup. Linux PTY tests check single-key mode, terminal
restoration, and descriptor identity; Python PTY cases also run against the real
candidate. See the migration plan for remaining platform/manual verification.

## Use

Build, then launch `bin/pissbox` from any terminal. Agent discovery and messaging
work without Herd. Inside Herd the launcher additionally binds `HERDR_SOCKET_PATH`
and `HERDR_PANE_ID` when `HERDR_ENV=1`. An incomplete Herd binding warns but leaves
messaging enabled. Missing binaries/extensions and startup failures warn without
preventing Pi from starting.

The selected Pi directory must contain `extensions/pissbox-bridge.js`,
`extensions/lib/agent-messaging.js`, `extensions/lib/agent-identity.js`, and
`extensions/agent-names.txt`. The repository's default `pi-agent/` already does;
alternate Pi directories need all four. The footer and bridge share the identity
helper and callsign list. Pi discovers the root extension automatically. These
files are plain JS/text; tests use Node and Go, not a TS compiler.

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

The launcher registers a fresh mailbox with its launcher PID and an optional,
complete host-owned Herd socket/pane binding. Normal exit unregisters that opaque
registration ID, drains final events, and best-effort releases only its own Herd agent. The daemon also polls
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

`register --mailbox DIR --owner-pid PID [--herdr-socket PATH --herdr-pane ID]`
ensures the daemon, then prints an opaque ID after mailbox readiness is installed.
The Herd arguments must either both be present or both be absent. Host control
protocol **3** rejects older live daemons for normal operations. Run
`pissbox --bridge-stop` to explicitly stop a protocol-2 or protocol-3 daemon;
this command tries the current protocol and its immediate predecessor. This
version bump is required because the strict presence schema now carries a
separate `agentName`. Lifecycle envelopes remain version 1. The readiness
marker contains `3`; messaging refuses older markers too, including after an
extension reload inside an old still-running launch.
`unregister --id ID` drains/releases that registration only; it never starts or
stops a daemon. The launcher uses these operations automatically. Control messages
are capped at 16 KiB with 2-second IO deadlines; startup readiness has a 3-second
retry window. Socket paths are limited to 103 bytes for macOS compatibility.
`PISSBOX_BRIDGE_RUNTIME_DIR` overrides the private runtime directory for isolated
tests; setting different overrides intentionally creates separate daemons. Do not
mount the runtime directory into SBX. `pissbox --bridge-stop` stops the compatible
host daemon without touching sandboxes; use it before rebuilding the bridge, then
reattach Pi sessions through the launcher to register fresh mailboxes.

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
Herd requests remain queued for retry with a bounded socket timeout. Lifecycle events
contain no prompts, source contents, credentials, or session logs. Peer message
payloads exist only in the filesystem transport and Pi conversation; the daemon
never logs them.

The version-1 envelope accepts only:

```json
{"version":1,"kind":"state","state":"working"}
{"version":1,"kind":"release"}
```

Lifecycle state may be `idle`, `working`, or `blocked`, with an optional message of at most
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
state events or peer identities/messages/receipts. Per-launch directories prevent
accidental mixing, not a malicious shared-state writer. Host adapter operations
remain limited to reporting/releasing the bound Herd agent.

## Agent messaging

`agent_list` returns `self` and every live `peer` across all registered
mailboxes and state roots; it does not filter. Each identity keeps `name` as the
Pi session title and has a separate `agentName` callsign, shown in the footer.
The editable 50-name pool is
`pi-agent/extensions/agent-names.txt`. Each callsign combines two different
entries from the 50-name pool. Pair order matters, giving 50 × 49 = 2,450
possible callsigns; collisions are possible. The bridge keeps the task title in
the identity name and the project directory in `cwd`, so peers with the same
callsign can be distinguished in `agent_list` using its session title and
`cwd`. Use `agent_send({ to: agentName, text, inReplyTo? })` to address one live
peer by callsign. Exact agent IDs remain supported and are required if a
callsign is ambiguous. A peer can reply by sending to the incoming `from.id` and
optionally referencing its `id` as `inReplyTo`. There is no broadcast, automatic reply, or
automatic resend.

The recipient inserts a displayed `agent-peer` custom message through
`pi.sendMessage(..., { deliverAs: "followUp", triggerTurn: true })`. Idle Pi wakes;
busy Pi receives a follow-up after current work. Text is never submitted as user
input or expanded as a slash command. The tool succeeds only after an explicit
`queued` receipt following native queue insertion, **not** after reasoning or an
answer. A timeout or disappearing recipient after publication is `indeterminate`:
insertion may already have happened. Do not retry automatically. Unknown/expired
addresses and full queues fail before publication. Bridge loss or local
cancellation after publishing a request is likewise potentially indeterminate.

Each session startup (including new/resume/fork/reload) creates a random 128-bit
live ID; shutdown removes presence, cancels local requests, and clears the poll
timer. The callsign stays stable for the Pi session and does not change when its
task title changes. Messaging runs in TUI,
RPC, JSON, and print modes while Pi is alive; Herd reporting stays TUI-only.
Presence refreshes every second and expires after 10 seconds. An expired or
retired ID never revives within this daemon lifetime. After a long process pause,
reload/reattach for a new address. Removing a launch registration retires its
agents on the next messaging poll without affecting other launches.

### File layout and version-1 envelopes

All IDs below are 32 lowercase hex characters. The sandbox knows only its mounted
launch mailbox, never the host control socket. Each agent has independent queues:

```text
bridge-launch.*/
  ready                          # Host writes 3; refreshes every second
  <lifecycle-event>.json          # Existing lifecycle transport
  agents/<live-agent-id>/
    presence.json                # Pi publishes identity atomically
    inbox-index.json             # Host publishes pending message IDs atomically
    requests/<request-id>.json    # Pi → daemon: list or send
    responses/<request-id>.json   # Daemon → originating Pi
    inbox/<message-id>.json       # Daemon → recipient Pi
    receipts/<message-id>.json    # Recipient Pi → daemon
```

Presence (file mtime is the lease; names/cwd are metadata, not routing paths):

```json
{"version":1,"agent":{"id":"<agent-id>","sessionId":"<Pi-session-id>","name":"Fix login redirect","agentName":"Aiden Jasper","cwd":"/workspace","state":"idle"}}
```

Requests contain no caller-selected sender identity. The daemon resolves the
sender from the registered source agent. Unknown fields are rejected:

```json
{"version":1,"id":"<request-id>","op":"list"}
{"version":1,"id":"<request-id>","op":"send","to":"<recipient-id>","text":"Hello","inReplyTo":"<optional-message-id>"}
```

List responses contain `self` and, when nonempty, `peers` identity objects. Send
responses carry a bridge-generated `messageId`. Failures use `error`:

```json
{"version":1,"id":"<request-id>","status":"ok","self":{"id":"<self-id>","sessionId":"<session-id>","name":"Fix login redirect","agentName":"Aiden Jasper","cwd":"/workspace","state":"idle"}}
{"version":1,"id":"<request-id>","status":"queued","messageId":"<message-id>"}
{"version":1,"id":"<request-id>","status":"failed","error":"unknown or expired recipient"}
{"version":1,"id":"<request-id>","status":"indeterminate","messageId":"<message-id>","error":"receipt timeout; queue insertion may have occurred; do not resend automatically"}
```

The bridge atomically publishes a bounded inbox index when its pending set
changes, including an empty index when no messages are pending:

```json
{"version":1,"to":"<recipient-id>","messages":["<message-id>"]}
```

The recipient opens this well-known file, then reads each message by its exact
validated filename. Do not rely on directory enumeration for host-to-sandbox
notification: observed virtiofs listings omitted host-created files even while
those files were readable by exact path. The index holds at most 64 unique live
message IDs; malformed indexes, traversal IDs, and mismatched recipients are
rejected. If the index is absent, the extension falls back to directory
scanning on filesystems where directory scanning works. Updating binaries alone
does not change an already-running compatible daemon:
explicitly stop/start it and reattach affected launches to activate the fix.

Inbox records include the exact recipient, sender identity, and deadline as Unix
milliseconds. Optional `inReplyTo` is copied from the send request:

```json
{"version":1,"id":"<message-id>","to":"<recipient-id>","from":{"id":"<sender-id>","sessionId":"<session-id>","name":"Fix login redirect","agentName":"Aiden Jasper","cwd":"/workspace","state":"working"},"text":"Hello","expires":1780000000000}
{"version":1,"id":"<message-id>","to":"<recipient-id>","status":"queued"}
```

The second envelope belongs in `receipts/`, only after insertion. The recipient
processes inboxes serially, fences mismatched/expired addresses, and deduplicates
message IDs for its live incarnation. Replayed send request IDs return the cached
result without a second insertion; pending duplicates do not enqueue again.

Limits: 16 KiB UTF-8 text, 32 KiB serialized record (including JSON escaping), 64
unacknowledged incoming messages and 64 outgoing sends per agent. The receipt
deadline is 10 seconds after bridge publication; the extension waits up to 12
seconds for the bridge response. Requests, inboxes, and responses use exclusive
temporary `.pending` files plus atomic rename. Reads are bounded regular-file
reads; symlinks, FIFOs, malformed records, traversal IDs, and queue overflow are
rejected. Go operations are rooted in each mailbox; Node uses only validated IDs
under its own agent directory and rejects non-directory/symlink queue paths.

Additional safety ceilings: 256 live agents, 4096 agent directories per mailbox,
256 request-directory entries, 4096 processed request IDs and received message
IDs per incarnation, and 4096 retained retired IDs before admitting new agents
is disabled. Reload for per-incarnation exhaustion; explicitly restart the daemon
for retired-ID exhaustion. A peer list exceeding 32 KiB fails explicitly rather
than silently truncating peers. No deduplication IDs are evicted to permit replay.
Consumed records are removed; abandoned agent directories and incomplete pending
files are retained, never silently migrated or deleted. Shared writable state is
a trusted local transport, not an authorization boundary.

## Extend

Under `internal/bridge`, `mailbox.go` owns the envelope and safe file reads;
`herdr.go` owns Herd requests; `main.go` owns mailbox polling, retries, readiness,
and final drain; `control.go` owns the host CLI, singleton lock/socket,
registrations, and owner liveness. `messaging.go` owns the separate
presence/routing/receipt worker, so a
stalled Herd adapter cannot delay peer messages. The small adapter
interface separates transport from host integration. Add capabilities explicitly
with validation and tests, not a generic host RPC passthrough.

Review comments, session-log visibility, automatic session restore, and binary
bundle packaging are not implemented. Herd's notification delivery and active-tab
suppression settings still apply.

## Verify

From the repository root, `make check` runs Go/JS/Python tests, vet, Bash syntax,
and whitespace checks. Cross-build separately with `make build-bridge-all`. See
[quality tooling](../tools/quality/README.md) for scoped Go/extension mutation
testing and coverage/CRAP metrics.

```bash
make test-go vet
make test-js
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s tests -v
make build-bridge-all
```

Go tests use real CLI/daemon subprocesses and local Unix sockets with simulated
Herd responders. They cover concurrent startup, crash recovery, incompatible
protocols, independent routing, owner death, and stalled connections. Launcher
tests stub SBX and control operations, plus one real-daemon launcher-crash check;
JS suites live beside their extensions and libraries under `pi-agent/extensions/`.
They drive lifecycle callbacks and connect real daemon subprocesses to the
extension tools, checking native queue call options, idle/busy delivery contracts,
replies, simultaneous sends, address replacement, deduplication, and timer cleanup.
Indexed delivery is tested with inbox enumeration unavailable, plus malformed
index/path-traversal rejection. Go simulated receivers consume the actual indexes
and check they clear after acknowledgements.
All test daemons use isolated directories under `.cache/test-fixtures/` and are stopped by cleanup.
These do not prove a real Herd/SBX connection or macOS runtime compatibility.

For the real smoke test, launch from two Herd panes and check `status` shows one
PID and two registrations. Check `herdr agent list` for `pissbox-pi`, run tasks,
and observe each pane's independent working → idle states. Switch away from a tab
to test completion notifications and exercise a blocking extension UI prompt for
attention. Close one launcher: only its agent should release; the other must keep
reporting under the same daemon PID. After both close, the daemon remains idle.
