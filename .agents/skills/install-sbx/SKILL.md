---
name: install-sbx
description: Install sbx in the Debian/Ubuntu development sandbox when missing.
---

# Install Docker Sandboxes CLI

Check for an existing installation first:

```bash
command -v sbx
sbx version
```

If `sbx` is not found, retry in a fresh login shell before installing:

```bash
bash -l -c 'command -v sbx && sbx version'
```

If still missing, use the user-approved Debian/Ubuntu installation:

```bash
curl -fsSL https://get.docker.com | sudo REPO_ONLY=1 sh
sudo DEBIAN_FRONTEND=noninteractive apt-get install -y docker-sbx
```

`REPO_ONLY=1` configures Docker's package repository without installing Docker Engine. Do not drop it or substitute a full Docker installation. This workflow is for the development sandbox, not macOS or an unrelated host.

Verify with `sbx version` and inspect the relevant subcommand's `--help`. The version command is not `sbx --version`.

If downloads are blocked, report the network-policy response instead of bypassing it. Do not sign in, modify credentials, start services, or create/remove sandboxes merely to verify installation. CLI availability and help output do not prove daemon access, kit validation, or a working sandbox launch.
