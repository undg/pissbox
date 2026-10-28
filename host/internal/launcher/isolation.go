package launcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func safeMountPath(path string) bool {
	for _, r := range path {
		if r < 32 || (r >= 127 && r <= 159) || r == ':' {
			return false
		}
	}
	return true
}
func validateIsolation(primary, state string, paths []string) (string, error) {
	if !isDir(primary) {
		return "", fmt.Errorf("Isolation requires an existing primary workspace: %s", primary)
	}
	primary, err := physical(primary)
	if err != nil {
		return "", err
	}
	for _, path := range []string{primary, state} {
		if !safeMountPath(path) {
			return "", fmt.Errorf("Isolation mount paths must not contain colons or control characters.")
		}
	}
	ancestor := state
	for !isDir(ancestor) {
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", fmt.Errorf("Isolation state path has no accessible ancestor")
		}
		ancestor = parent
	}
	ancestor, err = physical(ancestor)
	if err != nil {
		return "", err
	}
	if !safeMountPath(ancestor) {
		return "", fmt.Errorf("Isolation state path resolves to a colon or control character.")
	}
	for _, path := range paths {
		target := primary
		for _, part := range strings.Split(path, "/") {
			target += "/" + part
			info, err := os.Lstat(target)
			if err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.IsDir()) {
				return "", fmt.Errorf("Isolated paths must contain only directories, never files or symlinks: %s", target)
			}
			if err != nil && !os.IsNotExist(err) {
				return "", err
			}
		}
	}
	return primary, nil
}

const mountProbe = `
			test ! -L "$1/.pissbox-isolation-mount"
			test "$(cat "$1/.pissbox-isolation-mount")" = "$2"
			probe=$(mktemp "$1/.pissbox-write.XXXXXXXX")
			rm "$probe"
		`
const readinessProbe = `
		if [ -n "${PISSBOX_ISOLATION_DIR:-}" ]; then
			if [ ! -f "$HOME/.pissbox/isolation-ready" ] ||
				[ "$(cat "$HOME/.pissbox/isolation-ready")" != "$PISSBOX_ISOLATION_DIR" ]; then
				echo "Isolation setup is incomplete; use a new sandbox or explicitly delete this one. Setup will not be replayed." >&2
				exit 1
			fi
		fi
	`

func (p *processes) finishCreation(primary, state, mailbox string, paths []string, secrets []secret) (string, int) {
	data, err := os.ReadFile(filepath.Join(mailbox, "name"))
	if err != nil {
		return "", fail("Isolation creation returned no identity; refusing to discover or guess a sandbox name.")
	}
	name := stringsTrimNewlines(string(data))
	if !validName.MatchString(name) {
		return "", fail("Isolation creation returned an invalid identity; refusing sandbox actions.")
	}
	fmt.Fprintf(os.Stderr, "Finalizing sandbox '%s'; failures require a new sandbox or explicit deletion, not setup replay.\n", name)
	primary, err = validateIsolation(primary, state, paths)
	if err != nil {
		return "", fail("%s", err)
	}
	for i, path := range paths {
		target := primary + "/" + path
		backing := filepath.Join(mailbox, strconv.Itoa(i+1))
		if err := os.MkdirAll(target, 0777); err != nil {
			return "", fail("%s", err)
		}
		if err := os.Mkdir(backing, 0777); err != nil {
			return "", fail("%s", err)
		}
		if err := os.WriteFile(filepath.Join(backing, ".pissbox-isolation-mount"), []byte(backing+"\n"), 0666); err != nil {
			return "", fail("%s", err)
		}
		if status := p.sbx("mount", name, backing+":"+target); status != 0 {
			return "", status
		}
		if status := p.sbx("exec", "--user", "1000", name, "sh", "-eu", "-c", mountProbe, "isolation-mount", target, backing); status != 0 {
			return "", status
		}
	}
	if status := p.registerSecrets(name, secrets); status != 0 {
		return "", status
	}
	status := p.sbx("exec", "--user", "1000", name, "bash", "-c", `exec bash "$HOME/.pissbox/scripts/project-setup.sh" --finalize-isolation`)
	return name, status
}
