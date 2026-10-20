# Known issues

## Bundling / packaging

- [ ] **AMD64 / Apple Silicon image distribution:** `bin/build-image` now builds
      and registers the host architecture (`--amd64` / `--arm64` / `--register`)
      and keeps one archive per architecture. Still open: one host emitting both
      architectures (Buildx cross-build or emulated build), verifying image and
      dependency support for both, and verifying that an archive copied to
      another host of the same architecture loads and runs. Nothing tests real
      SBX imports on either architecture yet.
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
