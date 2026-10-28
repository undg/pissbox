package launcher

const helpText = `Usage: pissbox [OPTIONS] [WORKSPACE ...]
       pissbox --ls
       pissbox --delete DIR [--yes]

Start Pi in an SBX sandbox.

Options:
  -c, --continue       Continue the previous Pi session
  -d, --delete DIR     Delete matching sandboxes; loses sandbox-local data
  -y, --yes            Skip the existing-sandbox menu or delete confirmation
  -l, --ls             List sandboxes, current-directory matches first
  --name NAME          Use an explicit sandbox name
  --secret ENV_NAME HOST COMMAND
                       Register a proxy secret on creation only (repeatable)
  --ignore-config-workspaces
                       Skip configured mounts; keep CLI paths and setup
  --pissbox-pi-dir DIR  Override the shared sandbox Pi configuration directory
  --pissbox-state-dir DIR
                       Override the shared Linux tool-state directory
  --bridge-stop        Stop the compatible host bridge daemon
  -h, --help           Show this help
  --                   Treat remaining arguments as workspaces

Examples:
  pissbox
  pissbox --yes --continue
  pissbox --secret GH_TOKEN api.github.com '/usr/local/bin/get-token'

Documentation: sbx-kit/README.md (usage), doc/secrets.md (secrets).
`
