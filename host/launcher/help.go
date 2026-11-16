package launcher

import (
	"fmt"
	"path/filepath"
)

const helpTemplate = `Usage: {{.UseLine}}
       pissbox --ls
       pissbox --delete DIR [--yes]
       pissbox --name pi-sbx-and-cfg ~/Code/pissbox ~/.config/pissbox

{{.Short}}

Options:
{{.LocalFlags.FlagUsages}}

Documentation: %s
`

// helpText points at the checkout that supplied the executable. The root README
// is the documentation entry point; swap it for a published URL when there is one.
func helpText(root string) string {
	return fmt.Sprintf(helpTemplate, filepath.Join(root, "README.md"))
}
