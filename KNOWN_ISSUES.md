# Known issues

Deferred investigations; migration details are in `PLAN.md`.

- [ ] **Host clipboard integration:** Mouse selection/copy-paste in the host
  terminal does not reliably put selected Pi text on the host clipboard. The
  clipboard command (`xclip`, `pbcopy`, or equivalent) reportedly runs inside
  the sandbox instead of on the host. Investigate shared clipboard support,
  terminal plugins, or a host–sandbox clipboard bridge; exact mechanism unverified.
- [ ] **Scrolling in terminal selection mode:** Scrolling up while the host
  terminal is in selection mode does not scroll Pi's conversation pane. Investigate
  terminal/Pi mouse and scroll-event handling; cause unknown.
- [ ] **Model catalog refresh:** `pi update --models` first failed because
  `pi.dev` was blocked, then with `fetch failed`; curl reported an untrusted
  certificate chain. Add `pi.dev` to the kit allowlist and investigate CA trust
  without disabling TLS verification.
- [ ] **Fresh sandbox verification:** Full custom-image build, local image
  availability in SBX, shared-config symlink, Copilot login, and persistence
  across restart/replacement remain unverified. Shell regression tests passed.
