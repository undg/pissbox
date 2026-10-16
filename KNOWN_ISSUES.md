# Known issues

Deferred investigations; migration details are in `PLAN.md`.

- [ ] **Model catalog refresh:** `pi update --models` first failed because
      `pi.dev` was blocked, then with `fetch failed`; curl reported an untrusted
      certificate chain. Add `pi.dev` to the kit allowlist and investigate CA trust
      without disabling TLS verification.
- [ ] **Fresh sandbox verification:** Full custom-image build, local image
      availability in SBX, shared-config symlink, Copilot login, and persistence
      across restart/replacement remain unverified. Shell regression tests passed.
- [ ] **Decompose pi-agent/setting.json** This file have shared settings that I like to have committed, but also settings that are specific to machine that I'm working on (providers and models). Decomposition is necessary.

## Herd integration

- [ ] **Sandbox agent detection:** Herd's sidebar does not recognize agents
      running inside sandboxes. Investigate a host–sandbox bridge so Herd can detect
      and display those agents; the mechanism is not yet determined.
- [ ] **Scrolling in terminal selection mode:** Scrolling up while the host
      terminal is in selection mode does not scroll Pi's conversation pane. Investigate
      terminal/Pi mouse and scroll-event handling; cause unknown.
- [ ] **Host clipboard integration:** Mouse selection/copy-paste in the host
      terminal does not reliably put selected Pi text on the host clipboard. The
      clipboard command (`xclip`, `pbcopy`, or equivalent) reportedly runs inside
      the sandbox instead of on the host. Investigate shared clipboard support,
      terminal plugins, or a host–sandbox clipboard bridge; exact mechanism unverified.
