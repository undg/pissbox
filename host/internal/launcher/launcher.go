package launcher

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Main returns an exit status after releasing this launch's bridge registration.
// Resources are always relative to the resolved executable in .cache/bin.
func Main(args []string) int {
	executable, err := os.Executable()
	if err != nil {
		return fail("%s", err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return fail("%s", err)
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(executable)))
	host, _ := os.Hostname()
	return run(root, host, args)
}

func run(root, host string, args []string) int {
	o, err := parse(args)
	if err != nil {
		return fail("%s", err)
	}
	if o.help {
		fmt.Fprint(os.Stdout, helpText)
		return 0
	}
	if len(o.secrets) > 0 && (o.deleteDir != "" || o.list || o.bridgeStop) {
		return fail("--secret cannot be combined with list, delete, or bridge-stop")
	}
	p := newProcesses()
	defer p.close()
	if o.deleteDir != "" || o.list {
		return p.manage(o)
	}
	if o.bridgeStop {
		binary := bridgeBinary(root)
		if !executable(binary) {
			return fail("Matching host bridge binary is missing or unsupported.")
		}
		return p.command(binary, []string{"stop"}, os.Stdin, os.Stdout, os.Stderr)
	}
	pi := fallback(o.piDir, fallback(os.Getenv("PISSBOX_PI_DIR"), filepath.Join(root, "pi-agent")))
	state := fallback(o.stateDir, fallback(os.Getenv("PISSBOX_STATE_DIR"), filepath.Join(root, ".cache")))
	cwd, err := os.Getwd()
	if err != nil {
		return fail("%s", err)
	}
	if len(o.workspaces) == 0 {
		o.workspaces = []string{cwd}
	}
	primary := o.workspaces[0]
	if !filepath.IsAbs(primary) {
		primary = cwd + "/" + primary
	}
	var workspaces workspaceSet
	for _, workspace := range o.workspaces {
		workspaces.append(workspace)
	}
	project, status := p.readProject(root, primary)
	if status != 0 {
		return status
	}
	if !o.ignoreConfig {
		for _, workspace := range project.Workspaces {
			if !filepath.IsAbs(workspace) {
				workspace = primary + "/" + workspace
			}
			if !isDir(workspace) {
				return fail("Configured workspace directory does not exist: %s", workspace)
			}
			workspace, err = physical(workspace)
			if err != nil {
				return fail("%s", err)
			}
			workspaces.append(workspace)
		}
	}
	if !isDir(pi) {
		return fail("Pi directory does not exist: %s", pi)
	}
	pi, err = physical(pi)
	if err != nil {
		return fail("%s", err)
	}
	global, status := loadGlobalSecrets(root, cwd, append(append([]string{}, workspaces.paths...), pi, state))
	if status != 0 {
		return status
	}
	secrets := mergeSecrets(global, o.secrets)
	staged := len(project.Isolated) > 0 || len(secrets) > 0
	if len(secrets) > 0 {
		if status := p.checkSecrets(); status != 0 {
			return status
		}
	}
	if staged {
		primary, err = validateIsolation(primary, state, project.Isolated)
		if err != nil {
			return fail("%s", err)
		}
	}
	if err := linkSettings(pi, settingsProfile(host)); err != nil {
		return fail("%s", err)
	}
	if err := os.MkdirAll(state, 0777); err != nil {
		return fail("%s", err)
	}
	state, err = physical(state)
	if err != nil {
		return fail("%s", err)
	}
	workspaces.append(pi)
	workspaces.append(state)
	sbxArgs := []string{"run", "--env", "PISSBOX_PI_DIR=" + pi, "--env", "PISSBOX_STATE_DIR=" + state, "--env", "PISSBOX_WORKSPACE_DIR=" + primary}
	if o.name != "" {
		sbxArgs = append(sbxArgs, "--name", o.name)
	}
	bridge := registration{processes: p}
	defer bridge.stop()
	isolation := ""
	if staged {
		// Reuse the deferred-setup mailbox even when no isolated mounts are needed.
		isolation, err = os.MkdirTemp(state, "isolation.")
		if err != nil {
			return fail("%s", err)
		}
		sbxArgs[0] = "create"
		sbxArgs = append(sbxArgs, "--env", "PISSBOX_ISOLATION_DIR="+isolation)
	} else {
		bridge.start(root, pi, state)
	}
	sbxArgs = append(sbxArgs, bridge.env...)
	// Retain the legacy resource spelling passed to SBX; its ordering is public.
	sbxArgs = append(sbxArgs, root+"/bin/../sbx-kit")
	sbxArgs = append(sbxArgs, workspaces.paths...)
	if isolation == "" {
		sbxArgs = append(sbxArgs, o.piArgs()...)
	}
	diagnostic, status := p.initial(sbxArgs)
	if status == 0 {
		if isolation == "" {
			return 0
		}
		name, status := p.finishCreation(primary, state, isolation, project.Isolated, secrets)
		if status != 0 {
			return status
		}
		bridge.start(root, pi, state)
		attach := append([]string{"run", "--name", name}, bridge.env...)
		return p.sbx(append(attach, o.piArgs()...)...)
	}
	bridge.stop()
	name := conflictName(diagnostic, isolation != "", o.name)
	if name == "" {
		return status
	}
	return p.existing(root, pi, state, name, o, &bridge)
}

var ordinaryConflict = regexp.MustCompile(`sandbox '([^']+)' already exists and can't be given new workspaces`)
var createConflict = regexp.MustCompile(`sandbox "([A-Za-z0-9][A-Za-z0-9_.-]*)" already exists for this workspace; pass --name to create another, or start the existing sandbox`)
var namedCreateConflict = regexp.MustCompile(`sandbox '([A-Za-z0-9][A-Za-z0-9_.-]*)' already exists; connect to the existing sandbox`)

func conflictName(diagnostic string, isolated bool, requested string) string {
	pattern := ordinaryConflict
	if isolated {
		if requested != "" {
			pattern = namedCreateConflict
		} else {
			pattern = createConflict
		}
	}
	match := pattern.FindStringSubmatch(diagnostic)
	if len(match) == 0 {
		return ""
	}
	if isolated && requested != "" && match[1] != requested {
		return ""
	}
	return match[1]
}
func (p *processes) existing(root, pi, state, name string, o options, bridge *registration) int {
	if !o.yes {
		fmt.Fprintf(os.Stderr, "Sandbox '%s' already exists.\n\n  p) Open Pi [default]\n  c) Continue previous Pi session\n  s) Open shell\n  d) Delete sandbox\n  q) Quit\n\n", name)
	}
	for {
		choice := "p"
		if !o.yes {
			fmt.Fprint(os.Stderr, "Choose [P/c/s/d/q]: ")
			var status int
			choice, status = p.readInput(true)
			fmt.Fprintln(os.Stderr)
			if status != 0 {
				return status
			}
		}
		switch strings.ToLower(choice) {
		case "", "p", "c", "s":
			if status := p.sbx("exec", "--user", "1000", name, "sh", "-eu", "-c", readinessProbe, "pissbox-readiness"); status != 0 {
				return status
			}
			if strings.EqualFold(choice, "s") {
				return p.sbx("exec", "-it", name, "bash")
			}
			if strings.EqualFold(choice, "c") {
				o.continued = true
			}
			bridge.start(root, pi, state)
			attach := append([]string{"run", "--name", name}, bridge.env...)
			return p.sbx(append(attach, o.piArgs()...)...)
		case "d":
			fmt.Fprintln(os.Stderr, "Deletion loses sandbox-local data. Mounted host files remain.")
			yes, status := p.confirm("Do you want to delete it? [y/N]")
			if status != 0 {
				return status
			}
			if yes {
				return p.sbx("rm", "--force", name)
			}
			return 0
		case "q":
			return 0
		default:
			fmt.Fprintln(os.Stderr, "Please choose p, c, s, d, or q.")
		}
	}
}
