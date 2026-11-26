# Opt-in SBX MCP gateway

**TL;DR** — Reuse SBX's host-managed MCP gateway. Pi stays disconnected by default and supports ad-hoc `/sandbox-mcp on|off|status`. New sandboxes use static mode with no servers unless explicitly selected.

## Scope and decisions

- Authorized: Pi extension, launcher server selection, tests, and operational documentation.
- Initial implementation excluded host browser registration, login, browser access, daemon changes, and sandbox recreation. Subsequent user-directed host setup and a tab-list connection check were performed; no trace or browser interaction was started.
- SBX injects `MCP_GATEWAY_URL` and `MCP_SENTINEL_TOKEN_NAME`; both are present in this sandbox. Upstream Docker agent kits use them for HTTP MCP and proxy-managed bearer authentication.
- Use session-local extension registration, not shared `mcp.json`. OFF resets on reload, resume, fork, new session, and exit.
- Launcher uses `--static-mcp none` by default; explicit server selections apply on creation only. This raises the host SBX prerequisite to v0.54.0, the user's version and the baseline for the documented gateway/static-mode behavior.
- Pi disconnect is not host-level revocation. Host `sbx mcp load/unload` remains the sandbox-scoped capability control. Browser/CDP stays on host loopback.

## Steps

- [x] Add the OFF-by-default gateway extension and behavior/lifecycle tests, including real Pi MCP transport coverage without host services. Focused JS suite: 8 tests, including overlapping consent and real Pi HTTP MCP coverage.
- [x] Add creation-only launcher `--mcp SERVER` selection, default static `none`, parser/forwarding tests, and aligned installer requirements.
- [x] Document browser registration, login/handover, local ON/OFF, host revocation, and security boundaries.
- [x] Run ordinary checks and scoped quality assessment; record verification limits and handoff checks.

## Verification

- Self-review reran `make check`: all Go race suites passed; 70 JS tests passed; Python 254/255 passed. One unchanged Python test (`test_configured_paths_are_literal_not_shell_expansions`) expects leading `$HOME` to stay literal, contrary to the launcher's current home-expansion contract. Built `HEAD`'s host module in an isolated fixture and reproduced the same failure with the pre-change launcher. Left unrelated behavior/test untouched.
- `make vet syntax`: passed. `git diff --check`: passed.
- `make crap-js FILES="pi-agent/extensions/sandbox-mcp.js"`: fresh full-suite coverage, 100% statements/branches/functions/lines for the extension; maximum CRAP 13, no functions over 30.
- `make crap-go FILES="host/launcher/mcp.go host/launcher/cli.go host/launcher/launcher.go"`: new helper functions have 100% statement coverage, CRAP 5 and 3. Existing `run` orchestration remains risky (CRAP 71.22, complexity 63, coverage 87.3%); no broad refactoring attempted.
- `make mutation-go FILES="host/launcher/mcp.go"`: 2 killed, 0 lived/uncovered/timed out/invalid/skipped. This is a small helper-only mutation scope, not a score for all launcher changes.
- `make mutation-js FILES="pi-agent/extensions/sandbox-mcp.js" DRY_RUN=1`: baseline passed, 109 mutants discovered. No JS mutants executed: the full-suite baseline takes ~14 seconds, so a full run would be substantially costlier (~25 minutes before overhead). No JS mutation score claimed.
- Checks used the approved Artifactory Go mirror with proxy-managed `ARTIFACTORY_AUTH` via a temporary GOAUTH helper, not extracted real credentials. Used the installed Pi package on PATH and ignored this checkout's mise config for test processes to avoid loading its optional `.env` or modifying shared trust state.
- Artifactory npm metadata and tarball for Chrome DevTools MCP 1.12.0 verified through the existing proxy placeholder. SHA-256 matched repository metadata. The user then installed 1.12.0 on the Mac using the existing Keychain helper, registered the direct Node executable, and loaded it into the existing sandbox. A live `list_pages` call succeeded; user subsequently reported it works as expected. This is not a real static-mode sandbox creation/revocation or authenticated performance-trace check.
- Self-review found no runtime correctness blocker. Updated the host setup guide to the verified install-once/direct-executable method after host `npx` failed authentication; added a test preventing overlapping ON confirmations from replacing a live connection. All four documentation Bash examples passed `bash -n` without execution.
- Static-mode creation/restart revocation and authenticated performance tracing remain user verification. Automated MCP tests use an isolated local fixture.

## Handoff checks and notes

### Notes

- Review starts at `pi-agent/extensions/sandbox-mcp.js`, `host/launcher/mcp.go`, and `doc/mcp.md`. Pi OFF is deliberately not advertised as security against arbitrary sandbox code; SBX grants are sandbox-wide.
- The host prerequisite is now SBX v0.54.0+, matching the user's host. Rebuild the native host launcher after updating; `/reload` loads the shared extension without an image rebuild.
- Existing sandboxes are never recreated or switched from dynamic to static automatically. Default static-none behavior applies only to new creation.
- Host MCP registration remains manual. The guide pins the inspected package and keeps company registry/browser port settings out of runtime code.

### Checks

- [ ] Review the diff, especially default static mode, session-only consent reset, and the distinction between Pi disconnect and host revocation. Confirm the new minimum SBX version is acceptable.
- [ ] On the Mac, rebuild the launcher and create a new uniquely named sandbox without `--mcp`; inspect it with SBX. Expect static mode, no loaded servers, and `/sandbox-mcp status` OFF. Dynamic mode means creation defaults were not picked up; do not assume old sandboxes changed.
- [ ] Follow `doc/mcp.md` to register the host server with approved npm authentication, log into the dedicated debugging Chrome, load the server into that sandbox, and confirm `/sandbox-mcp on`. Expect DevTools tools in `/mcp` and one authenticated performance trace. Connection failure needs gateway/host MCP/CDP diagnosis, not cookie export or public CDP exposure.
- [ ] Turn Pi OFF, then reload/resume/fork/new-session and verify it remains OFF. Unload the server on the host and restart the static-none sandbox; expect no browser attachment and no dynamic discovery. Other clients must not be treated as revoked by the Pi switch alone.
