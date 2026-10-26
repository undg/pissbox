# Known issues

## Bundling / packaging

- [x] **AMD64 / Apple Silicon image distribution:** `bin/build-image` now builds
      and registers the host architecture (`--amd64` / `--arm64` / `--register`)
      and keeps one archive per architecture. Still open: one host emitting both
      architectures (Buildx cross-build or emulated build), verifying image and
      dependency support for both, and verifying that an archive copied to
      another host of the same architecture loads and runs. Nothing tests real
      SBX imports on either architecture yet.
- [x] **Portable runtime artifacts:** `bin/artifacts --export FILE` and
      `--import FILE` transfer allowlisted image archives, host bridge binaries,
      and installed Pi npm packages as `.tar.gz`; Git supplies the launcher, kit,
      and configuration. Credentials, sessions, and general caches are excluded.
      Import refuses existing artifacts; SBX image registration remains explicit.
      Isolated fixture tests cover transfer and rejection behavior. Real
      cross-machine/architecture transfers and offline startup remain unverified;
      plugins are not baked into the image. See [transfer guide](README.md#transfer-built-artifacts).

## Shared-state security

- [ ] **Read-only and disposable state:** Sandboxes currently share writable Pi
      configuration, credentials, sessions, and tool state. Separate state that
      should be read-only to pissbox from writable data, and make regenerable
      caches and npm/Git tool state easy to erase and recreate frequently.
      Do not treat credentials, sessions, or all of `.cache/` as disposable.
      Design and implementation deferred; no cleanup is authorized by this item.

## General

- [ ] **Model catalog refresh — very low priority:** `pi update --models`
      fails due to insufficient permissions.
- [x] **Fresh sandbox verification:** Custom-image availability in SBX,
      config links, Copilot login, and restart/replacement persistence need an
      end-to-end check. Stubbed CLI tests do not verify these.
- [x] **npm cache mapping:** Added `npm|.cache/npm` to the shared-directory
      configuration. Isolated-home regression tests verify persistence and
      preservation of the image's `.npm` directory. Existing sandboxes need
      a new sandbox name to receive the mapping; real SBX persistence remains
      unverified.

## Host ↔ pissbox bridge / Herd

Agent discovery, review feedback, session visibility, and completion notifications
are related parts of the host/sandbox bridge gap. The initial
[Go mailbox bridge](host/README.md) implements custom-agent lifecycle
reporting; real Herd/SBX behavior and notifications still need a host smoke test.
Upstream's official Pi integration requires host Pi process detection, so merely
forwarding its socket messages is insufficient for sandbox Pi. Review delivery
and native session-log integration remain unresolved.

- [x] **Sandbox agent visibility:** custom `pissbox-pi` lifecycle reporting is
      implemented; real Herd sidebar visibility still needs verification.
- [x] **Finished-job notifications:** the initial bridge reports final Pi
      settlement through Herd's semantic state API, bound to the launching pane.
      Real host notifications still need verification, including active-tab
      suppression. Native session visibility remains outside this first slice.
- [ ] **herdr-reviewr → sandbox Pi comments:**
      [herdr-reviewr](https://github.com/persiyanov/herdr-reviewr) collects
      line/range comments and sends them together to the workspace agent's input.
      The user reports this host-to-pissbox workflow is blocked. Shared source
      files alone do not make sandbox Pi a registered host Herd agent; bridge
      discovery and delivery to the correct sandbox session need investigation.
      This is local review feedback, not GitHub PR comment posting.
- [ ] **herdr-nvim → sandbox Pi annotations and references:**
      [herdr-nvim](https://github.com/ChmaraX/herdr-nvim) can paste annotated
      file/line selections, submit them, or insert inline references into an
      agent's input. The user reports the same host/sandbox integration gap.
      Bridge work must preserve target-session selection, file/range context,
      and the distinction between paste-only and auto-submit. Host-side editing
      through live mounts does not establish an agent messaging channel.
- [ ] **Host review tools need sandbox session/turn visibility:** reviewr's
      last-turn scope depends on agent turn tracking; herdr-nvim's recent-file
      picker reads agent session logs, with pane-output scraping as a fallback.
      Investigate whether these work for sandbox Pi alongside agent discovery
      and comment delivery. Upstream READMEs establish these dependencies; these
      additional symptoms and the exact failure mechanism have not been tested.
## Host terminal

- [x] **Selection-mode scrolling and copy/paste:** caused by full-screen mode,
      not the sandbox. Fix: set `"tuiMode": "regular"` in Pi settings.
