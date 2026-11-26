package launcher

import (
	"fmt"
	"regexp"
	"strings"
)

var mcpServerName = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func validateMcpServers(names []string) error {
	for _, name := range names {
		if !mcpServerName.MatchString(name) {
			return fmt.Errorf("--mcp requires MCP server names containing only letters, numbers, dots, hyphens, or underscores")
		}
		if strings.EqualFold(name, "none") && len(names) != 1 {
			return fmt.Errorf("--mcp none cannot be combined with server names")
		}
	}
	return nil
}

func (o options) staticMcp() string {
	if len(o.mcpServers) == 0 || strings.EqualFold(o.mcpServers[0], "none") {
		return "none"
	}
	return strings.Join(o.mcpServers, ",")
}
