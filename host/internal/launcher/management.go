package launcher

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

var validName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

type sandbox struct {
	Name, Agent, Status string
	Workspaces          []string
	associated          bool
}

func sandboxListing(data, directory string) ([]sandbox, error) {
	var listing any
	if err := json.Unmarshal([]byte(data), &listing); err != nil {
		return nil, err
	}
	entries, ok := listing.([]any)
	if !ok {
		return nil, fmt.Errorf("Expected a sandbox array")
	}
	sandboxes := make([]sandbox, 0, len(entries))
	for _, entry := range entries {
		fields, ok := entry.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("Expected a sandbox object; refusing to continue")
		}
		invalid := func(field string) error {
			name, _ := json.Marshal(fields["name"])
			return fmt.Errorf("Invalid sandbox metadata for %s: %s; refusing to continue", name, field)
		}
		name, ok := fields["name"].(string)
		if !ok {
			return nil, invalid("name must be a string")
		}
		if !validName.MatchString(name) {
			return nil, invalid("name contains unsupported characters")
		}
		agent, ok := fields["agent"].(string)
		if !ok {
			return nil, invalid("agent must be a string")
		}
		status, ok := fields["status"].(string)
		if !ok {
			return nil, invalid("status must be a string")
		}
		workspaces, exists := fields["workspaces"]
		if exists && workspaces == nil {
			workspaces = []any{}
		}
		paths, ok := workspaces.([]any)
		if !ok {
			return nil, invalid("workspaces must be an array or explicit null")
		}
		s := sandbox{Name: name, Agent: agent, Status: status}
		for _, path := range paths {
			value, ok := path.(string)
			if !ok || !strings.HasPrefix(value, "/") {
				return nil, invalid("workspaces must contain absolute path strings")
			}
			s.Workspaces = append(s.Workspaces, value)
			if value == directory {
				s.associated = true
			}
		}
		sandboxes = append(sandboxes, s)
	}
	return sandboxes, nil
}
func printGroup(w io.Writer, list []sandbox, associated bool) {
	count := 0
	for _, s := range list {
		if s.associated == associated {
			fmt.Fprintf(w, "  %s  [%s]  (%s)\n    %s\n", s.Name, s.Status, s.Agent, strings.Join(s.Workspaces, ", "))
			count++
		}
	}
	if count == 0 {
		fmt.Fprintln(w, "  (none)")
	}
}
func printHeader(title, directory string) {
	heading, muted, reset := "", "", ""
	_, noColor := os.LookupEnv("NO_COLOR")
	if isTerminal(os.Stdout) && os.Getenv("TERM") != "dumb" && !noColor {
		heading = "\033[1;36m"
		muted = "\033[2m"
		reset = "\033[0m"
	}
	fmt.Fprintf(os.Stdout, "%s%s%s\n%s────────────────────────────────────────%s\n", heading, title, reset, muted, reset)
	if directory != "" {
		fmt.Fprintf(os.Stdout, "  %s%s%s\n", muted, directory, reset)
	}
}
func (p *processes) manage(o options) int {
	if (o.deleteDir != "" && o.list) || o.name != "" || len(o.workspaces) != 0 {
		return fail("Delete and list cannot be combined with each other, --name, or workspaces.")
	}
	directory := o.deleteDir
	if directory == "" {
		directory = "."
	}
	if !isDir(directory) {
		return fail("Directory does not exist: %s", directory)
	}
	directory, err := physical(directory)
	if err != nil {
		return fail("%s", err)
	}
	if _, err := exec.LookPath("jq"); err != nil {
		return fail("Listing and deleting sandboxes requires jq; install jq first.")
	}
	data, status := p.capture("sbx", []string{"ls", "--json"}, os.Stdin)
	if status != 0 {
		return status
	}
	list, err := sandboxListing(data, directory)
	if err != nil {
		return fail("%s", err)
	}
	if o.list {
		printHeader("Current directory", directory)
		printGroup(os.Stdout, list, true)
		fmt.Fprintln(os.Stdout)
		printHeader("Other sandboxes", "")
		printGroup(os.Stdout, list, false)
		return 0
	}
	names := []string{}
	for _, s := range list {
		if s.associated {
			names = append(names, s.Name)
		}
	}
	if len(names) == 0 {
		return fail("No sandboxes associated with directory: %s", directory)
	}
	fmt.Fprintf(os.Stderr, "Sandboxes associated with %s:\n", directory)
	printGroup(os.Stderr, list, true)
	fmt.Fprintln(os.Stderr, "Deletion loses sandbox-local data. Mounted host files remain.")
	if !o.yes {
		yes, status := p.confirm(fmt.Sprintf("Delete all %d matching sandboxes? [y/N]", len(names)))
		if status != 0 {
			return status
		}
		if !yes {
			return 0
		}
	}
	return p.sbx(append([]string{"rm", "--force"}, names...)...)
}
