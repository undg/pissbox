package launcher

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type registration struct {
	binary, id, mailbox string
	env                 []string
	processes           *processes
	cancel              context.CancelFunc
	done                chan struct{}
}

func bridgeBinary(root string) string {
	return filepath.Join(root, ".cache/bin/go-bridge-"+runtime.GOOS+"-"+runtime.GOARCH)
}
func executable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0
}
func (r *registration) stop() {
	if r.cancel != nil {
		r.cancel()
		<-r.done
		r.cancel = nil
	}
	if r.id != "" {
		retireMailbox(r.mailbox, r.id)
		r.processes.command(r.binary, []string{"unregister", "--id", r.id}, os.Stdin, os.Stdout, os.Stderr)
		r.id = ""
	}
	r.env = nil
}

// retireMailbox leaves a durable tombstone that handoff adoption checks. It
// also covers a daemon that is down or dies before processing unregister.
func retireMailbox(mailbox, id string) {
	if mailbox == "" || id == "" {
		return
	}
	data, err := json.Marshal(struct {
		Version int    `json:"version"`
		ID      string `json:"id"`
	}{1, id})
	if err != nil {
		return
	}
	temporary, err := os.CreateTemp(mailbox, ".retired-")
	if err != nil {
		return
	}
	name := temporary.Name()
	defer os.Remove(name)
	_, err = temporary.Write(data)
	closeErr := temporary.Close()
	if err != nil || closeErr != nil {
		return
	}
	os.Rename(name, filepath.Join(mailbox, "retired"))
}

// Reconnect only to a running compatible daemon. Explicit stop stays stopped;
// another launch or an explicit ensure brings the daemon back.
func (r *registration) maintain(ctx context.Context, args []string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			attempt, cancel := context.WithTimeout(ctx, 3*time.Second)
			output, err := exec.CommandContext(attempt, r.binary, args...).Output()
			cancel()
			if err == nil {
				if id := strings.TrimSpace(string(output)); id != "" {
					r.id = id
				}
			}
		}
	}
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
	for _, resource := range []string{"extensions/pissbox-bridge.js", "extensions/lib/agent-messaging.js", "extensions/lib/agent-lifetime.js", "extensions/lib/vault-tools.js", "extensions/lib/agent-identity.js", "extensions/agent-names.txt"} {
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
		r.mailbox = mailbox
		r.env = []string{"--env", "PISSBOX_BRIDGE_DIR=" + mailbox}
		ctx, cancel := context.WithCancel(context.Background())
		r.cancel, r.done = cancel, make(chan struct{})
		args[0] = "reconnect"
		go func() {
			defer close(r.done)
			r.maintain(ctx, args, 2*time.Second)
		}()
		return
	}
	r.id = ""
	fmt.Fprintln(os.Stderr, "Warning: Host bridge failed to start. Continuing without it.")
}
