# Global network permissions

Add `[network]` to the host global configuration:
`$PISSBOX_CONFIG`, or `${XDG_CONFIG_HOME:-$HOME/.config}/pissbox/config.toml`.
It uses the same path validation and schema as [global secrets](secrets.md).

```toml
[network]
allow = [
  "repo.ihsmarkit.com",
  "repo.mi.ihsmarkit.com",
  "api.deepseek.com",
]
block = [
  "blocked.example.com",
  "*.untrusted.example.com",
]
```

1. Both lists are optional; omitted or empty lists add no rules.
2. `allow` adds to the base kit's permissions. It does not replace kit defaults.
3. `block` maps to SBX's `permissions.network.deny`. Deny takes precedence over
   allow, including rules from the base kit or other composed kits.
4. Use one SBX network pattern per string, without URL schemes, whitespace, or
   comma-separated values. Exact hosts, host-and-port patterns, and single-label
   wildcards such as `*.example.com` are supported. SBX validates pattern syntax;
   other pattern forms may parse without enforcement in some SBX releases.
5. The launcher creates a temporary schema-v2 mixin and supplies it with `--kit`
   to the initial `sbx run` or staged `sbx create`. It removes the temporary kit
   when the launch ends, without rewriting the base kit or global configuration.
6. Rules apply only to new sandboxes, including explicit `--recreate` creation.
   Attaching does not update an existing sandbox's network policy. Use a new
   sandbox name or explicitly recreate it to adopt changed rules.
7. Help, completion, list, delete, and bridge management do not apply network
   rules. The feature never changes SBX's host-wide policy. Host governance may
   impose additional restrictions; an allow entry is not a bypass.

The runtime kit uses SBX's schema-v2 mixin support (`--kit`), matching the base
kit's schema. It does not use cloud-only `--allow-network` flags.
