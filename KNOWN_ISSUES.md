# Known issues

## Model catalog refresh fails

**Status:** Open. After the user adjusted network access, a retry of
`pi update --models` failed with `fetch failed` for GitHub Copilot and Anthropic.
A direct curl request to the catalog endpoint now fails certificate validation:

```text
curl: (60) SSL certificate OpenSSL verify result: unable to get local issuer certificate (20)
```

Investigate the sandbox/proxy certificate trust chain and configure the correct
CA trust for both Node (Pi) and curl. Do not disable TLS verification.

Originally, Pi's catalog host `pi.dev` was blocked with HTTP 403:

```text
Blocked by network policy: domain pi.dev:443
  detail: no matching allow rule — blocked by default deny policy
```

### To resolve

- Add `pi.dev` to `permissions.network.allow` in `sbx-kit/spec.yaml` for future
  sandboxes.
- If a sandbox still reports the original policy block, allow the domain from
  the host:

  ```bash
  sbx policy allow network pi.dev
  ```

- Resolve certificate trust, then retry inside the sandbox:

  ```bash
  pi update --models
  ```

- Confirm the refresh succeeds and the models are available in Pi's `/model`
  picker.

## Custom image and sandbox integration not yet verified

**Status:** Unverified, not a confirmed defect.

The symlink shell tests pass, but the complete workflow has not been tested:

- Build `pissbox/pi-image:local` with `SHARED_PI_AGENT_DIR`.
- Confirm host SBX can use that local image.
- Create a sandbox with the shared `pi-agent/` workspace mounted.
- Verify `/home/agent/.pi/agent` resolves to the mounted directory.
- Authenticate with GitHub Copilot through `/login`.
- Confirm credentials and sessions remain available after a sandbox restart
  and in a replacement sandbox using the same shared directory.

See `sbx-kit/README.md` for build, launch, and test commands.
