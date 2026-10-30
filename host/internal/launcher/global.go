package launcher

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"pissbox/host/config"
)

// loadGlobalSecrets resolves, validates, and loads the user-level config. It
// only creates the starter for the default path; an explicit override must
// already exist. A starter creation failure warns and continues.
func loadGlobalSecrets(root, cwd string, mounts []string) ([]secret, int) {
	explicit := os.Getenv("PISSBOX_CONFIG")
	path, err := config.Path(explicit, cwd)
	if err != nil {
		return nil, fail("%s", err)
	}
	candidate, err := physicalCandidate(path)
	if err != nil {
		return nil, fail("%s", err)
	}
	for _, mount := range mounts {
		target, err := physicalCandidate(mount)
		if err != nil {
			return nil, fail("%s", err)
		}
		if within(candidate, target) {
			return nil, fail("Global configuration must not be inside a mounted workspace, Pi, or state directory: %s", path)
		}
	}
	if explicit != "" {
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				return nil, fail("Global configuration file does not exist: %s", path)
			}
			return nil, fail("read global configuration %s: %v", path, err)
		}
	} else if _, err := os.Stat(path); missing(err) {
		created, err := config.CreateStarter(path, root)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not create global configuration %s: %v\n", path, err)
			return nil, 0
		}
		if created {
			return nil, 0
		}
	}
	secrets, err := config.Load(path)
	if err != nil {
		return nil, fail("%s", err)
	}
	return secrets, 0
}

// mergeSecrets applies CLI entries over global entries by environment name.
func mergeSecrets(global, cli []secret) []secret {
	merged := append([]secret(nil), global...)
	for _, candidate := range cli {
		replaced := false
		for i := range merged {
			if merged[i].Env == candidate.Env {
				merged[i] = candidate
				replaced = true
				break
			}
		}
		if !replaced {
			merged = append(merged, candidate)
		}
	}
	return merged
}

// physicalCandidate resolves symlinks in the existing part of path and keeps
// the missing suffix. It lets config safety checks work before state creation.
func physicalCandidate(path string) (string, error) {
	current, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return resolved, nil
		}
		if !missing(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

func missing(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)
}

func within(path, dir string) bool {
	relative, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}
