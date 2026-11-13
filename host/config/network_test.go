package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadGlobalNetwork(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	text := `[network]
allow = ['example.com', '*.example.org', 'api.example.com:8443']
block = ['blocked.example.com', '*.example.org']
[secrets.API]
host = 'example.com'
command = 'fetch-key'
`
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	settings, err := LoadSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	want := Network{Allow: []string{"example.com", "*.example.org", "api.example.com:8443"}, Block: []string{"blocked.example.com", "*.example.org"}}
	if !reflect.DeepEqual(settings.Network, want) || len(settings.Secrets) != 1 || settings.Secrets[0].Env != "API" {
		t.Fatalf("unexpected settings: %+v", settings)
	}
	if _, err := LoadVaults(path); err != nil {
		t.Fatalf("bridge must accept network configuration: %v", err)
	}
}

func TestGlobalNetworkValidation(t *testing.T) {
	for _, text := range []string{
		"", "[network]", "[network]\nallow = []\nblock = []",
		"[network]\nallow = ['example.com']", "[network]\nblock = ['example.com']",
	} {
		if _, err := parseSettings([]byte(text)); err != nil {
			t.Errorf("valid config %q: %v", text, err)
		}
	}
	for _, text := range []string{
		"network = []", "[network]\nallow = 'example.com'",
		"[network]\nblock = [1]", "[network]\nallow = ['']",
		"[network]\nblock = ['   ']", "[network]\nallow = ['example.com,other.com']",
		"[network]\nallow = ['https://example.com']", "[network]\ndeny = ['example.com']",
	} {
		if _, err := parseSettings([]byte(text)); err == nil {
			t.Errorf("invalid config accepted: %q", text)
		}
	}
}
