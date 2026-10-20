# Known issues

- [ ] **Model catalog refresh:** `pi update --models` previously failed with
      network/CA errors. `pi.dev` is now allowed in the kit; verify refresh and
      certificate trust without disabling TLS verification.
- [x] **Fresh sandbox verification:** Custom-image availability in SBX,
      config links, Copilot login, and restart/replacement persistence need an
      end-to-end check. Stubbed CLI tests do not verify these.
- [x] **npm cache mapping:** Setup selects `$HOME/.cache/npm`, but
      `shared-directories.conf` lacks `npm|.cache/npm`. Three shared-directory
      regression tests fail; the cache is not shared across sandbox removal.

## Host terminal / Herd

- [ ] Herd's sidebar does not detect sandbox agents; bridge mechanism unknown.
- [x] Host terminal selection-mode scrolling does not scroll Pi's conversation.
- [x] Selection/copy-paste does not reliably reach the host clipboard; investigate
      terminal support or a host–sandbox bridge.
