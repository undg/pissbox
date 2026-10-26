package launcher

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func fail(format string, args ...any) int { fmt.Fprintf(os.Stderr, format+"\n", args...); return 1 }
func isDir(path string) bool              { info, err := os.Stat(path); return err == nil && info.IsDir() }
func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
func physical(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(path)
}
func fallback(value, other string) string {
	if value != "" {
		return value
	}
	return other
}

// workspaceSet retains the first CLI spelling while comparing physical directories.
type workspaceSet struct{ paths, keys []string }

func (w *workspaceSet) append(path string) {
	key := path
	if isDir(path) {
		if resolved, err := physical(path); err == nil {
			key = resolved
		}
	}
	for _, existing := range w.keys {
		if key == existing {
			return
		}
	}
	w.paths = append(w.paths, path)
	w.keys = append(w.keys, key)
}

type project struct {
	Workspaces []string `json:"workspaces"`
	Isolated   []string `json:"isolated_directories"`
}

func (p *processes) readProject(root, primary string) (project, int) {
	var config project
	path := primary + "/.pissbox.toml"
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return config, 0
	}
	f, err := os.Open(path)
	if err != nil || !isFile(path) {
		if f != nil {
			f.Close()
		}
		return config, fail("Project configuration must be a readable TOML file: %s", path)
	}
	f.Close()
	for _, tool := range []string{"tomljson", "jq"} {
		if _, err := exec.LookPath(tool); err != nil {
			return config, fail("Reading .pissbox.toml requires %s on the host; install it before launching.", tool)
		}
	}
	data, status := p.capture("bash", []string{filepath.Join(root, "sbx-kit/files/home/.pissbox/scripts/project-setup.sh"), "--validate", path}, os.Stdin)
	if status != 0 {
		return config, status
	}
	if err := json.Unmarshal([]byte(data), &config); err != nil {
		return config, fail("Invalid project validation result: %s", err)
	}
	return config, 0
}
func settingsProfile(host string) string {
	host = strings.SplitN(host, ".", 2)[0]
	switch host {
	case "cm", "di-7415":
		return "settings-go.json"
	default:
		return "settings-co.json"
	}
}
func linkSettings(pi, profile string) error {
	source := filepath.Join(pi, profile)
	target := filepath.Join(pi, "settings.json")
	if !isFile(source) {
		return fmt.Errorf("Settings profile does not exist: %s", source)
	}
	info, err := os.Lstat(target)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		current, err := os.Readlink(target)
		if err != nil {
			return err
		}
		if current == profile {
			return nil
		}
		// Rename a fresh link, rather than leave the shared target missing on failure.
		temp, err := os.MkdirTemp(pi, ".settings-link-")
		if err != nil {
			return err
		}
		defer os.Remove(temp)
		next := filepath.Join(temp, "settings.json")
		if err := os.Symlink(profile, next); err != nil {
			return err
		}
		defer os.Remove(next)
		return os.Rename(next, target)
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		backup := target + ".replaced"
		if _, err := os.Lstat(backup); err == nil {
			backup += "." + time.Now().Format("20060102150405")
		}
		fmt.Fprintf(os.Stderr, "Backing up existing Pi settings file: %s -> %s\n", target, backup)
		if err := os.Rename(target, backup); err != nil {
			return err
		}
	}
	return os.Symlink(profile, target)
}
