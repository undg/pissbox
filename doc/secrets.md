# Proxy-managed secrets

Requires SBX's experimental `secret set-custom --command` support (CLI verified
with v0.50.0). The sandbox receives a placeholder environment variable; the
proxy substitutes the real value in request headers sent to `HOST`.

## Global configuration

Put recurring secrets in one user-level TOML file:

```toml
[secrets.GH_TOKEN]
host = "api.github.com"
command = "security find-generic-password -s gh-token -w"

[secrets.OPENAI_API_KEY]
host = "api.openai.com"
command = "op read op://Private/OpenAI/api-key"
```

The path is `PISSBOX_CONFIG` when set, otherwise
`${XDG_CONFIG_HOME:-$HOME/.config}/pissbox/config.toml`. A relative
`PISSBOX_CONFIG` uses the caller's directory; empty means unset. The file must
not be inside a workspace, Pi directory, or state directory mounted into the
sandbox: it contains host commands, so sandbox-writable storage must not be able
to change the next launch.

On a normal launch, a missing default file is created with parent directories,
mode `0600`, and a first line like:

```toml
#:schema ~/Code/pissbox/host/config/global-schema.json
```

The path comes from the checkout that supplied the launcher executable and
compacts a `$HOME` prefix to `~`. The schema is embedded in the launcher at
`host/config/global-schema.json`; the directive is for editor completion and
documentation. If starter creation fails, the launcher warns and continues with
no global entries. A missing explicit `PISSBOX_CONFIG` target is an error;
possibly mistyped paths are never created silently.

Entries accept only `host` and `command`. Unknown keys, literal value fields,
invalid environment names, URLs, ports, and empty or dash-prefixed commands are
rejected. The schema is the single source for shape and key validation; the
launcher adds host/command grammar checks. An empty or comment-only file is
valid and means no global secrets.

**Review before committing:** this format stores host commands, not credential
values, so it is usually commit-friendly. It is not a guarantee. A command can
still embed a secret by mistake, and command text is not proof that it did not.

## Registration and precedence

Global entries are registered for every **new** sandbox, sorted by environment
name, before project setup and Pi. Existing sandboxes keep their registrations;
attaches never add or update them. Use a new sandbox name to change the set.

`--secret ENV_NAME HOST COMMAND` remains available for one-off additions.
A CLI entry overrides the same environment name from global config. Duplicate
CLI environment names are still errors. CLI flags never write into the global
file, and the launcher never executes a configured command or captures its
stdout: it passes the command string unchanged to
`sbx secret set-custom --command`. SBX resolves and refreshes the value.

To use a different or smaller set for one launch, point `PISSBOX_CONFIG` at
another file outside writable mounts, including an empty TOML file. To disable
global secrets for one launch, do the same with an empty file; do not rely on a
flag. Help, list, delete, and bridge-stop never create, read, or validate the
global file.

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

For github.com, use `GH_TOKEN`, even with an Enterprise subscription.
`GH_ENTERPRISE_TOKEN` is for a separate Enterprise Server host.
`api.github.com` covers REST/GraphQL calls, not Git operations or every upload/download.

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

Global config validation uses the embedded JSON Schema with Go libraries, not
the sandbox project-setup script, `tomljson`, or `jq`. Project configuration
validation still needs `tomljson` and `jq`.

Launcher tests use stubbed SBX with real project setup. Real placeholder
injection and proxy substitution still need a host SBX smoke test.
