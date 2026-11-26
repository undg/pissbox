# Host MCP and authenticated browser handover

Requires **SBX v0.54.0+**, Docker sign-in, and Pi with native MCP extension
registration. This uses SBX's local MCP gateway, not Docker Desktop MCP Toolkit
and not the pissbox mailbox bridge.

After updating, rebuild the launcher on the host with `make build-launcher` and
use `/reload` in Pi to load the extension. No image rebuild is needed. Create a
new sandbox name to adopt the static-mode default; existing modes are not changed.

## OFF by default; connect ad hoc

The launcher creates sandboxes in **static mode**, with no MCP servers unless
`--mcp SERVER` selects registered names. Static mode disables agent-driven server
discovery/loading. The shared Pi extension does not connect at load or startup.

| Pi command | Effect |
| --- | --- |
| `/sandbox-mcp on` | Confirm exposing all servers loaded into this sandbox, then register the gateway with Pi. |
| `/sandbox-mcp off` | Unregister the gateway and block queued calls to its tool namespace. |
| `/sandbox-mcp status` (or `/sandbox-mcp`) | Show the session-local ON/OFF switch, not transport health. |
| `/mcp` | Inspect the actual connection, errors, tools, and exposure after ON. |

ON does not need a restart. OFF resets after `/reload`, `/new`, resume, fork, and
exit; consent is not written to sessions or shared configuration. Headless runs
cannot enable it. Other independently configured Pi MCP servers are unaffected.

The extension reads `MCP_GATEWAY_URL` and `MCP_SENTINEL_TOKEN_NAME`, injected by
SBX, and uses a proxy-managed bearer header. It registers `pissbox-sbx` with
**deferred** tool exposure: Pi's native tool search loads tools as needed. Do not
create an `mcp.json` entry named `pissbox-sbx`; file entries override extension
registrations. Keep native MCP and tool search enabled in Pi.

## Host server selection

Register trusted servers on the **host** with `sbx mcp add`. Registration does
not attach them to a sandbox. To select already-registered servers at creation:

```bash
pissbox --name browser-perf --mcp chrome-devtools
pissbox --name tools --mcp chrome-devtools,docs --mcp metrics
```

Omitting `--mcp`, or using `--mcp none`, passes `--static-mcp none`. `none` cannot
be combined with other names. Repeated selections append names. This applies to
ordinary creation, deferred setup, shell creation, and explicit recreation.

**Existing sandboxes keep their saved mode and attachments.** Passing `--mcp`
on attach does not replace the set or change its mode. The launcher never
registers, loads, unloads, or removes host servers automatically.

For an ad-hoc grant to an existing static sandbox, on the host:

```bash
sbx mcp load chrome-devtools --sandbox browser-perf
sbx mcp unload chrome-devtools --sandbox browser-perf
```

Load before `/sandbox-mcp on`, or while connected; SBX sends tool-list updates.
Unload removes the sandbox attachment and its persisted restoration on restart.
Registration remains available for a later explicit grant. Removing a registration
with `sbx mcp rm` is **not** a substitute for unloading existing attachments.

A sandbox created with `--static-mcp none` retains disabled dynamic discovery
across restarts. Docker documents different restart behavior after unloading the
last server from a named static set. Prefer creating with no servers and loading
ad hoc when persistent discovery suppression matters. Older dynamic sandboxes
can allow agents to reload unloaded servers; use a new static sandbox rather
than assuming the launcher changes existing policy.

## Chrome DevTools MCP on your host

Use a dedicated Chrome profile with a remote-debugging port bound to loopback.
Do not use your everyday browsing profile. Chrome requires a non-default user
data directory for the remote-debugging-port launch method. Keep the browser
and MCP process on the host:

```text
Sandbox Pi → SBX MCP gateway → host Chrome DevTools MCP → loopback CDP → Chrome
```

No Chrome installation in the sandbox, browser-profile mount, cookie export,
or exposed host CDP listener is needed.

The example pins **chrome-devtools-mcp 1.12.0**. Its package declares Node
`^20.19.0 || ^22.12.0 || >=23`; use a supported Node LTS release on the host.
The installer does not check or install Node/npm, this server, or Chrome.
Review the server and its dependencies: SBX runs explicit stdio commands with
host permissions, outside sandbox isolation.

Set `NPM_REGISTRY` to your approved npm registry. Never put a real token in
chat, project configuration, or the registration command. An Artifactory UI
package link is not an npm registry URL. The company mirror verified for this
package is:

```text
https://repo.ihsmarkit.com/artifactory/api/npm/registry.npmjs.org/
```

### Install once with host authentication

A sandbox's `ARTIFACTORY_AUTH` placeholder cannot authenticate a host `npx`
process. Host registration can succeed while loading later fails with npm E401,
before the MCP handshake. Do not use public `npm login` to fix a private registry.

Install the package once on the host, then register its executable directly.
Keep the installation outside writable sandbox mounts (workspaces, Pi, and
shared state). The default below uses a dedicated host-only application directory.
For macOS, this reuses the Keychain entry supplying the global Artifactory secret.
Set `NPM_REGISTRY` first; run the following on the **host**, not in the sandbox:

```bash
(
  set -euo pipefail
  install_dir="$HOME/.local/share/pissbox/chrome-devtools-mcp"
  temporary_dir="$(mktemp -d)"
  trap 'rm -rf "$temporary_dir"' EXIT
  auth_scope="${NPM_REGISTRY#https:}"
  printf '%s\n' "registry=$NPM_REGISTRY" "${auth_scope%/}/:_authToken=\${ARTIFACTORY_AUTH}" > "$temporary_dir/npmrc"
  token="$(security find-generic-password -s ARTIFACTORY_AUTH -w)"
  ARTIFACTORY_AUTH="$token" NPM_CONFIG_USERCONFIG="$temporary_dir/npmrc" npm install \
    --prefix "$install_dir" --registry "$NPM_REGISTRY" --save-exact --ignore-scripts \
    chrome-devtools-mcp@1.12.0
)
```

The temporary npm config contains an environment-variable reference, not the
real token. Only the installation process receives the token in its environment;
no permanent npm authentication configuration or sandbox token handover is needed.
For another platform, replace the Keychain lookup with your existing host secret
helper. `--ignore-scripts` avoids dependency install scripts; this package's bundled
1.12.0 distribution was installed and loaded successfully with that option.

### Register the installed server

If replacing a failed registration, remove it first with `sbx mcp rm chrome-devtools`
and confirm when prompted. If it is loaded elsewhere, unload it from those sandboxes
first; removing the registration alone does not disconnect existing attachments.
Then register on the host:

```bash
install_dir="$HOME/.local/share/pissbox/chrome-devtools-mcp"
node_binary="$(node -p 'process.execPath')"
DEBUG_PORT="${CHROME_DEBUG_PORT:-9222}"
sbx mcp add chrome-devtools --command "$node_binary" \
  --args "$install_dir/node_modules/chrome-devtools-mcp/build/src/bin/chrome-devtools-mcp.js,--browser-url=http://127.0.0.1:${DEBUG_PORT},--no-usage-statistics,--no-performance-crux,--redact-network-headers,--no-file-navigations"
sbx mcp inspect chrome-devtools
```

This resolves the real Node executable instead of depending on the daemon's PATH,
a version-manager shim, or an npm download on every connection. Node/package updates
can require re-registration; the launcher never maintains host installations.

SBX's `--args` is comma-separated. This resolves `DEBUG_PORT` at registration
time; a later shell variable change does not update the saved registration.
The example disables usage statistics and CrUX trace-URL submission, redacts
some sensitive network headers, and disallows tool-driven `file:` navigation.
These options are present in 1.12.0. They do not sanitize all tool results or
confine the browser's host filesystem/network access. Review existing global
DevTools configuration as well; it can enable extra capabilities.

## Login and performance workflow

1. Create a new sandbox without selecting servers: `pissbox --name browser-perf`.
2. Open your dedicated host debugging browser and log into the application
   manually, including MFA. Keep only intended application tabs open.
3. Load `chrome-devtools` into that sandbox from the host, then enter
   `/sandbox-mcp on` in Pi and confirm. Use `/mcp` to check the connection.
4. Ask the agent to reproduce the slow interaction, record a trace, inspect
   performance insights/network/console data, and correlate findings with source.
   Avoid competing navigation or profiling from nvim/DevTools during recording.
5. Use `/sandbox-mcp off` to disconnect this session. Unload the server on the
   host to revoke the sandbox attachment. Stop the dedicated debugging browser
   when the handover is finished.

## Security boundaries

- Browser access can read authenticated responses and act as you, even though
  you never give the agent your password. Prefer a development/test account.
- Tool results, screenshots, and traces can enter model requests and saved Pi
  sessions. Header redaction is not comprehensive secret removal.
- The host browser uses **host network access**, not the sandbox's network
  restrictions. A host MCP process also has host filesystem permissions.
- Pi OFF is a convenience switch, **not a security boundary against sandbox
  code**. Code can use injected gateway placeholders directly; other MCP clients
  in that sandbox remain connected. SBX load/unload is sandbox-scoped, not
  exclusive to one Pi agent. Revocation cannot undo completed or in-flight work.
- URL filters and disabled input/evaluation tools can reduce exposure, but do
  not turn an authenticated browser into a read-only sandbox. Start with one
  controller per browser; page IDs do not isolate tracing or session state.

Sources: [SBX MCP gateway](https://docs.docker.com/ai/sandboxes/mcp-gateway/),
[Docker agent kit gateway wiring](https://github.com/docker/sbx-kits-contrib/blob/main/claude/spec.yaml),
[Chrome DevTools MCP](https://github.com/ChromeDevTools/chrome-devtools-mcp).
