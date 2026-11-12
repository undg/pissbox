package launcher

const helpTemplate = `Usage: {{.UseLine}}
       pissbox --ls
       pissbox --delete DIR [--yes]

{{.Short}}

Options:
{{.LocalFlags.FlagUsages}}      --                             Treat remaining arguments as workspaces

Examples:
{{.Example}}

Documentation: sbx-kit/README.md (usage), doc/secrets.md (secrets).
`
