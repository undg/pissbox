package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// Vault describes a shared host-only Markdown collection. Git defaults to true.
type Vault struct {
	Name string `toml:"-"`
	Path string `toml:"path"`
	Git  *bool  `toml:"git"`
}

// LoadVaults reads the same validated global document as Load. It does not
// access vault directories or execute commands. Missing files are caller policy.
func LoadVaults(path string) ([]Vault, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("global configuration must be a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if _, err := parse(data); err != nil {
		return nil, err
	}
	var doc document
	if _, err := toml.Decode(string(data), &doc); err != nil {
		return nil, err
	}
	return vaultDefinitions(doc.Vaults)
}

func vaultDefinitions(entries map[string]Vault) ([]Vault, error) {
	result := make([]Vault, 0, len(entries))
	paths := make(map[string]bool)
	for name, v := range entries {
		if strings.TrimSpace(name) == "" || len(name) > 80 || strings.ContainsAny(name, "\x00\r\n") {
			return nil, fmt.Errorf("invalid vault name")
		}
		v.Name = name
		if strings.HasPrefix(v.Path, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, err
			}
			v.Path = filepath.Join(home, v.Path[2:])
		}
		if !filepath.IsAbs(v.Path) || strings.ContainsRune(v.Path, 0) {
			return nil, fmt.Errorf("vault %s path must be absolute or start with ~/", name)
		}
		v.Path = filepath.Clean(v.Path)
		if paths[v.Path] {
			return nil, fmt.Errorf("duplicate vault path")
		}
		paths[v.Path] = true
		result = append(result, v)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}
