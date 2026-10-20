# Known issues

## Bundling / packaging

- [ ] **AMD64 / Apple Silicon image distribution:** Split image building/export
      from SBX import. Use Buildx to build `linux/amd64` and `linux/arm64` on one
      machine; first verify base-image and dependency support for both. Keep the
      same `pissbox:local` tag but export separate `pissbox-linux-amd64.tar` and
      `pissbox-linux-arm64.tar` archives, then import the matching archive on each
      host. Emulation may slow cross-builds, not runtime; matching images run natively.
- [ ] **Bundled plugins and portable distribution:** Explore baking plugins and
      their dependencies into the image to avoid runtime npm/Git downloads.
      Package the image, launcher, kit, and configuration for other machines
      requiring only Docker/SBX, without relying on Pi's npm/Git caches.
      Exclude credentials and sessions; implementation deferred.
## General

- [ ] **Model catalog refresh — very low priority:** `pi update --models`
      fails due to insufficient permissions.
- [x] **Fresh sandbox verification:** Custom-image availability in SBX,
      config links, Copilot login, and restart/replacement persistence need an
      end-to-end check. Stubbed CLI tests do not verify these.
- [x] **npm cache mapping:** Setup selects `$HOME/.cache/npm`, but
      `shared-directories.conf` lacks `npm|.cache/npm`. Three shared-directory
      regression tests fail; the cache is not shared across sandbox removal.

## Host terminal / Herd

- [ ] Herd's sidebar does not detect sandbox agents; bridge mechanism unknown.
- [x] **Selection-mode scrolling and copy/paste:** caused by full-screen mode,
      not the sandbox. Fix: set `"tuiMode": "regular"` in Pi settings.
