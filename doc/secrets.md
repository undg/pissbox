# Proxy-managed secrets

```text
pissbox --secret ENV_NAME HOST COMMAND
```

Repeat `--secret` for multiple credentials. Requires SBX's experimental
`secret set-custom --command` support (CLI verified with v0.50.0).

```bash
pissbox --secret GH_TOKEN api.github.com '/usr/local/bin/get_token "gh-token"'
```

For github.com, use `GH_TOKEN`, even with an Enterprise subscription.
`GH_ENTERPRISE_TOKEN` is for a separate Enterprise Server host.
`api.github.com` covers REST/GraphQL calls, not Git operations or every upload/download.

## Arguments

1. **ENV_NAME** receives a placeholder, not the credential. The proxy substitutes
   the real value in request headers sent to `HOST`, not URLs or request bodies.
   Duplicate names, `PISSBOX_*`, and `SANDBOX_NAME` are rejected.
2. **HOST** is a host/IP or quoted wildcard, without scheme, port, or path.
   `example.com` does not match `api.example.com`. `'*.example.com'` matches
   one subdomain level; `'**.example.com'` covers deeper levels.
3. **COMMAND** is one quoted host command; SBX uses its stdout as the credential.
   Do not use `"$(get_token)"`: that exposes the resolved token in arguments.
   Helpers must be executable by SBX, not just interactive shell functions.

## Register once per sandbox

Registration is sandbox-scoped, during creation, before project setup and Pi.
Afterward, run `pissbox` normally. On attach, secret flags do not add or update
registrations. Manage existing registrations with `sbx secret set-custom
--sandbox NAME ...`.

SBX handles verification and refresh (`on-demand` by default), so the command
may run again later. Registration/setup failure blocks Pi and later Pi/shell
attaches; use a new sandbox name or explicitly delete the failed sandbox.
Completed work and registrations are not rolled back; setup is never replayed.

## Host command safety

Keep helpers, dependencies, configuration, and the host temporary directory
outside writable sandbox mounts. SBX runs commands from a fresh host temporary
directory; prefer absolute helper paths. Commands do not belong in `.pissbox.toml`.
A sandbox can use the credential through the proxy even without reading it;
use narrow host patterns and scoped credentials.

Launcher tests use stubbed SBX with real project setup. Real placeholder
injection and proxy substitution still need a host SBX smoke test.
