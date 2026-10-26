package launcher

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
)

type registration struct {
	binary, id string
	env        []string
	processes  *processes
}

func bridgeBinary(root string) string {
	return filepath.Join(root, ".cache/bin/go-bridge-"+runtime.GOOS+"-"+runtime.GOARCH)
}
func executable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0
}
func (r *registration) stop() {
	if r.id != "" {
		r.processes.command(r.binary, []string{"unregister", "--id", r.id}, os.Stdin, os.Stdout, os.Stderr)
		r.id = ""
	}
	r.env = nil
}
func (r *registration) start(root, pi, state string) {
	r.env = nil
	herd := []string{}
	if os.Getenv("HERDR_ENV") == "1" {
		socket, pane := os.Getenv("HERDR_SOCKET_PATH"), os.Getenv("HERDR_PANE_ID")
		if socket != "" && pane != "" {
			herd = []string{"--herdr-socket", socket, "--herdr-pane", pane}
		} else {
			fmt.Fprintln(os.Stderr, "Warning: Herd reporting disabled; missing socket or pane environment. Agent messaging remains available.")
		}
	}
	binary := bridgeBinary(root)
	if !executable(binary) {
		fmt.Fprintln(os.Stderr, "Warning: Host bridge binary missing; run bin/build-bridge. Continuing without it.")
		return
	}
	for _, resource := range []string{"extensions/pissbox-bridge.js", "extensions/lib/agent-messaging.js", "extensions/lib/agent-identity.js", "extensions/agent-names.txt"} {
		if !isFile(filepath.Join(pi, resource)) {
			fmt.Fprintln(os.Stderr, "Warning: Host bridge extension missing from selected Pi directory. Continuing without it.")
			return
		}
	}
	mailbox, err := os.MkdirTemp(state, "bridge-launch.")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Warning: cannot create host bridge mailbox. Continuing without it.")
		return
	}
	r.binary = binary
	args := append([]string{"register", "--mailbox", mailbox}, herd...)
	args = append(args, "--owner-pid", strconv.Itoa(os.Getpid()))
	id, status := r.processes.capture(binary, args, nil)
	if status == 0 && id != "" {
		r.id = id
		r.env = []string{"--env", "PISSBOX_BRIDGE_DIR=" + mailbox}
		return
	}
	r.id = ""
	fmt.Fprintln(os.Stderr, "Warning: Host bridge failed to start. Continuing without it.")
}
