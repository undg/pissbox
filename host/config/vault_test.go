package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVaultConfigurationCoexistsWithSecretsAndExpandsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(t.TempDir(), "config.toml")
	write(t, path, `[vault.work]
path = "~/notes/work"
git = false
[vault.personal]
path = "/host/notes/personal"
[secrets.API]
host = "example.com"
command = "fetch-token"
`)
	values, err := LoadVaults(path)
	if err != nil || len(values) != 2 {
		t.Fatal(values, err)
	}
	if values[0].Name != "personal" || values[0].Git != nil || values[1].Name != "work" || values[1].Path != filepath.Join(home, "notes/work") || values[1].Git == nil || *values[1].Git {
		t.Fatal(values)
	}
	if secrets, err := Load(path); err != nil || len(secrets) != 1 {
		t.Fatal(secrets, err)
	}
	if _, err := os.Stat(filepath.Join(home, "notes")); !os.IsNotExist(err) {
		t.Fatal("configuration accessed/created vault directories")
	}
}
func TestInvalidVaultConfigurationFailsBothLoaders(t *testing.T) {
	for _, text := range []string{
		"[vault.a]\npath = 'relative/notes'\n",
		"[vault.a]\npath = '/notes'\ncommand = 'shell'\n",
		"[vault.a]\npath = '/notes'\ngit = 'yes'\n",
		"[vault.a]\npath = '/notes'\nobsidian = 'Notes'\n",
		"[vault.a]\ngit = true\n",
		"[vault.a]\npath = '/notes'\n[vault.b]\npath = '/notes/.'\n",
		"[vault.' ']\npath = '/notes'\n",
	} {
		path := filepath.Join(t.TempDir(), "config.toml")
		write(t, path, text)
		if _, err := Load(path); err == nil {
			t.Fatalf("Load accepted %q", text)
		}
		if _, err := LoadVaults(path); err == nil {
			t.Fatalf("LoadVaults accepted %q", text)
		}
	}
	if _, err := LoadVaults(t.TempDir()); err == nil {
		t.Fatal("accepted directory")
	}
	if _, err := LoadVaults(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("accepted missing file")
	}
}
