package launcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
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
	Setup      []string `toml:"setup"`
	Workspaces []string `toml:"workspaces"`
	Isolated   []string `toml:"isolated_directories"`
}

func (p *processes) readProject(primary string) (project, int) {
	path := primary + "/.pissbox.toml"
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return project{}, 0
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return project{}, fail("Project configuration must be a readable TOML file: %s", path)
	}
	config, err := parseProject(data)
	if err != nil {
		return project{}, fail("Invalid project configuration %s: %s", path, err)
	}
	return config, 0
}

// parseProject keeps host-side validation in Go. Project setup still validates
// independently before running commands inside the sandbox.
func parseProject(data []byte) (project, error) {
	var config project
	metadata, err := toml.Decode(string(data), &config)
	if err != nil {
		return project{}, err
	}
	if unknown := metadata.Undecoded(); len(unknown) != 0 {
		return project{}, fmt.Errorf("unknown key %q; supported keys: setup, workspaces, isolated_directories", unknown[0])
	}
	for _, command := range config.Setup {
		if strings.ContainsRune(command, 0) {
			return project{}, fmt.Errorf("setup commands must not contain NUL bytes")
		}
	}
	for _, workspace := range config.Workspaces {
		if workspace == "" || strings.ContainsAny(workspace, "\r\n\x00") {
			return project{}, fmt.Errorf("workspaces paths must not be empty or contain NUL bytes or line breaks")
		}
	}
	if err := validateIsolatedDirectories(config.Isolated); err != nil {
		return project{}, err
	}
	return config, nil
}

func validateIsolatedDirectories(paths []string) error {
	for _, path := range paths {
		normalized := path != "" && !strings.HasPrefix(path, "/") && safeMountPath(path)
		for _, part := range strings.Split(path, "/") {
			if part == "" || part == "." || part == ".." {
				normalized = false
			}
		}
		if !normalized {
			return fmt.Errorf("isolated_directories paths must be normalized relative directories without colons or control characters")
		}
	}
	for i, path := range paths {
		for _, other := range paths[i+1:] {
			if path == other || strings.HasPrefix(path, other+"/") || strings.HasPrefix(other, path+"/") {
				return fmt.Errorf("isolated_directories paths must not duplicate or overlap")
			}
		}
	}
	return nil
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
