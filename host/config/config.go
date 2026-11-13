// Package config reads pissbox host configuration.
//
// It is a pure data package: callers decide when files are created or loaded,
// and use the returned values. It never runs commands, calls sbx, mounts
// directories, or manages sandbox lifecycles.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/BurntSushi/toml"
)

// Secret describes one proxy-managed secret. Command is opaque data: the
// launcher may probe it for availability but must pass it to SBX unchanged and
// never inspect its output.
type Secret struct {
	Env     string
	Hosts   []string
	Command string
}

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Validate checks one secret against the shared --secret and global rules.
// Existing entries are used to reject duplicate environment names.
func (s Secret) Validate(existing []Secret) error {
	if !envName.MatchString(s.Env) || strings.HasPrefix(s.Env, "PISSBOX_") || s.Env == "SANDBOX_NAME" {
		return fmt.Errorf("secret ENV_NAME must be a non-reserved environment variable name")
	}
	if len(s.Hosts) == 0 {
		return fmt.Errorf("secret HOST must contain at least one host")
	}
	for _, host := range s.Hosts {
		if host == "" || strings.HasPrefix(host, "-") || strings.ContainsAny(host, "/\\?#@") ||
			strings.ContainsFunc(host, unicode.IsSpace) || (strings.Contains(host, ":") && net.ParseIP(host) == nil) {
			return fmt.Errorf("secret HOST must be a host, IP, or wildcard pattern, not a URL or host:port")
		}
	}
	if strings.TrimSpace(s.Command) == "" || strings.HasPrefix(s.Command, "-") {
		return fmt.Errorf("secret COMMAND must be a non-empty quoted command")
	}
	for _, previous := range existing {
		if previous.Env == s.Env {
			return fmt.Errorf("secret ENV_NAME must not be repeated: %s", s.Env)
		}
	}
	return nil
}

// Path resolves the global configuration file. An explicit value is a file
// path relative to cwd; an empty value selects the user config directory.
func Path(explicit, cwd string) (string, error) {
	if explicit != "" {
		if !filepath.IsAbs(explicit) {
			explicit = filepath.Join(cwd, explicit)
		}
		return filepath.Clean(explicit), nil
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "pissbox", "config.toml"), nil
}

// Starter returns the generated starter file content. The schema directive
// points at the checkout that supplied the launcher executable.
func Starter(root string) string {
	schema := filepath.Join(root, "host", "config", "global-schema.json")
	if home, err := os.UserHomeDir(); err == nil {
		if prefix := home + string(filepath.Separator); strings.HasPrefix(schema, prefix) {
			schema = "~" + schema[len(home):]
		}
	}
	return "#:schema " + schema + "\n"
}

// CreateStarter creates a missing global configuration file. It never replaces
// an existing file: a concurrent creator wins and created is false.
func CreateStarter(path, root string) (created bool, err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return false, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, fs.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := file.WriteString(Starter(root)); err != nil {
		file.Close()
		return false, err
	}
	return true, file.Close()
}

// Load reads, validates, and returns global secrets sorted by environment name.
func Load(path string) ([]Secret, error) {
	settings, err := LoadSettings(path)
	if err != nil {
		return nil, err
	}
	return settings.Secrets, nil
}

// Settings contains validated host-wide launcher configuration.
type Settings struct {
	Secrets []Secret
	Network Network
}

// Network adds egress permissions to the base kit. Block rules take precedence.
type Network struct {
	Allow []string `toml:"allow" json:"allow,omitempty"`
	Block []string `toml:"block" json:"deny,omitempty"`
}

// LoadSettings reads and validates the global configuration without performing IO
// other than reading the file. It never executes configured commands.
func LoadSettings(path string) (*Settings, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("read global configuration %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("global configuration must be a regular file: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read global configuration %s: %w", path, err)
	}
	return parseSettings(data)
}

type document struct {
	Secrets map[string]entry `toml:"secrets"`
	Vaults  map[string]Vault `toml:"vault"`
	Network Network          `toml:"network"`
}

type entry struct {
	Host    hostList `toml:"host"`
	Command string   `toml:"command"`
}

// UnmarshalTOML normalizes a single host or an array to the same representation.
func (h *hostList) UnmarshalTOML(data any) error {
	switch value := data.(type) {
	case string:
		*h = []string{value}
	case []any:
		hosts := make([]string, 0, len(value))
		for _, item := range value {
			host, ok := item.(string)
			if !ok {
				return fmt.Errorf("secret host array must contain only strings")
			}
			hosts = append(hosts, host)
		}
		*h = hosts
	default:
		return fmt.Errorf("secret host must be a string or an array of strings")
	}
	return nil
}

type hostList []string

func parse(data []byte) ([]Secret, error) {
	settings, err := parseSettings(data)
	if err != nil {
		return nil, err
	}
	return settings.Secrets, nil
}

func parseSettings(data []byte) (*Settings, error) {
	var decoded map[string]any
	if _, err := toml.Decode(string(data), &decoded); err != nil {
		return nil, fmt.Errorf("invalid global configuration: %w", err)
	}
	if decoded == nil {
		decoded = map[string]any{}
	}
	if err := validateSchema(decoded); err != nil {
		return nil, fmt.Errorf("invalid global configuration: %w", err)
	}
	var doc document
	if _, err := toml.Decode(string(data), &doc); err != nil {
		return nil, fmt.Errorf("invalid global configuration: %w", err)
	}
	if _, err := vaultDefinitions(doc.Vaults); err != nil {
		return nil, err
	}
	secrets := make([]Secret, 0, len(doc.Secrets))
	for name, value := range doc.Secrets {
		secret := Secret{Env: name, Hosts: value.Host, Command: value.Command}
		if err := secret.Validate(nil); err != nil {
			return nil, fmt.Errorf("invalid global secret %s: %w", name, err)
		}
		secrets = append(secrets, secret)
	}
	sort.Slice(secrets, func(i, j int) bool { return secrets[i].Env < secrets[j].Env })
	return &Settings{Secrets: secrets, Network: doc.Network}, nil
}
