package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestPathResolutionAndEmptyOverride(t *testing.T) {
	cwd := t.TempDir()
	explicit, err := Path("nested/config.toml", cwd)
	if err != nil || explicit != filepath.Join(cwd, "nested/config.toml") {
		t.Fatal(explicit, err)
	}
	absolute := filepath.Join(cwd, "absolute.toml")
	if got, err := Path(absolute, cwd); err != nil || got != absolute {
		t.Fatal(got, err)
	}
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	if got, err := Path("", cwd); err != nil || got != filepath.Join(configHome, "pissbox/config.toml") {
		t.Fatal(got, err)
	}
}

func TestStarterUsesCheckoutSchemaDirective(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, "Code/pissbox")
	want := "#:schema ~/Code/pissbox/host/config/global-schema.json\n"
	if got := Starter(root); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	created, err := CreateStarter(path, root)
	if err != nil || !created {
		t.Fatal(created, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != want {
		t.Fatal(string(data), err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal(info, err)
	}
	if err := os.WriteFile(path, []byte("custom = true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	created, err = CreateStarter(path, root)
	if err != nil || created {
		t.Fatal(created, err)
	}
	if data, _ := os.ReadFile(path); string(data) != "custom = true\n" {
		t.Fatal("overwrote existing config")
	}
}

func TestStarterCreationFailureIsReturned(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "not-a-directory")
	write(t, parent, "x")
	if created, err := CreateStarter(filepath.Join(parent, "config.toml"), t.TempDir()); err == nil || created {
		t.Fatal(created, err)
	}
}

func TestLoadEmptyCommentOnlyAndStarter(t *testing.T) {
	for _, text := range []string{"", "# comment\n", "\n\t# another\n"} {
		path := filepath.Join(t.TempDir(), "config.toml")
		write(t, path, text)
		secrets, err := Load(path)
		if err != nil || len(secrets) != 0 {
			t.Fatalf("%q: %v %v", text, secrets, err)
		}
	}
	root := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.toml")
	if _, err := CreateStarter(path, root); err != nil {
		t.Fatal(err)
	}
	if secrets, err := Load(path); err != nil || len(secrets) != 0 {
		t.Fatal(secrets, err)
	}
}

func TestLoadRejectsMissingDirectoriesAndMalformedTOML(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.toml")); err == nil {
		t.Fatal("loaded missing file")
	}
	if _, err := Load(t.TempDir()); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	write(t, path, "[secrets.API\nhost = \"example.com\"\n")
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "invalid global configuration") {
		t.Fatal(err)
	}
}

func TestLoadRejectsUnknownAndLiteralValueFields(t *testing.T) {
	for _, text := range []string{
		"unknown = true\n",
		"[secrets.API]\nhost = \"example.com\"\ncommand = \"get_token\"\nvalue = \"literal\"\n",
		"[secrets.API]\nhost = \"example.com\"\ncommand = \"get_token\"\nextra = true\n",
		"[secrets.API]\nhost = 5\ncommand = \"get_token\"\n",
		"[secrets.API]\nhost = \"example.com\"\n",
	} {
		path := filepath.Join(t.TempDir(), "config.toml")
		write(t, path, text)
		if _, err := Load(path); err == nil {
			t.Fatalf("accepted %q", text)
		}
	}
}

func TestLoadRejectsInvalidEnvironmentNames(t *testing.T) {
	for _, name := range []string{"1API", "API-KEY", "PISSBOX_PI_DIR", "SANDBOX_NAME", "API.KEY"} {
		path := filepath.Join(t.TempDir(), "config.toml")
		write(t, path, "[secrets."+name+"]\nhost = \"example.com\"\ncommand = \"get_token\"\n")
		if _, err := Load(path); err == nil {
			t.Fatalf("accepted %s", name)
		}
	}
}

func TestLoadRejectsInvalidHostsAndCommands(t *testing.T) {
	for _, entry := range []string{
		"host = \"https://example.com\"\ncommand = \"get_token\"\n",
		"host = \"example.com:443\"\ncommand = \"get_token\"\n",
		"host = \"example.com/path\"\ncommand = \"get_token\"\n",
		"host = \"example.com\"\ncommand = \"  \"\n",
		"host = \"example.com\"\ncommand = \"--yes\"\n",
	} {
		path := filepath.Join(t.TempDir(), "config.toml")
		write(t, path, "[secrets.API]\n"+entry)
		if _, err := Load(path); err == nil {
			t.Fatalf("accepted %q", entry)
		}
	}
}

func TestLoadSortsSecretsAndPreservesCommands(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	write(t, path, `[secrets.ZED]
host = "z.example.com"
command = "get_zed --account 'a b'"
[secrets.ALPHA]
host = "*.example.com"
command = "/host/bin/get-alpha"
[secrets.MID]
host = "127.0.0.1"
command = "get_mid"
`)
	secrets, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []Secret{
		{Env: "ALPHA", Host: "*.example.com", Command: "/host/bin/get-alpha"},
		{Env: "MID", Host: "127.0.0.1", Command: "get_mid"},
		{Env: "ZED", Host: "z.example.com", Command: "get_zed --account 'a b'"},
	}
	if !reflect.DeepEqual(secrets, want) {
		t.Fatalf("%+v", secrets)
	}
}

func TestLoadDoesNotEchoCommandValues(t *testing.T) {
	marker := "do-not-print-this-command-value"
	path := filepath.Join(t.TempDir(), "config.toml")
	write(t, path, "[secrets.API]\nhost = \"https://example.com\"\ncommand = \""+marker+"\"\n")
	_, err := Load(path)
	if err == nil || strings.Contains(err.Error(), marker) {
		t.Fatal(err)
	}
}

func TestSchemaDescribesEveryProperty(t *testing.T) {
	var schema any
	if err := json.Unmarshal(globalSchemaJSON, &schema); err != nil {
		t.Fatal(err)
	}
	root, ok := schema.(map[string]any)
	if !ok {
		t.Fatal("schema root is not an object")
	}
	if description, _ := root["description"].(string); strings.TrimSpace(description) == "" {
		t.Error("schema root has no description")
	}
	var walk func(any, string)
	walk = func(value any, path string) {
		object, ok := value.(map[string]any)
		if !ok {
			return
		}
		if properties, ok := object["properties"].(map[string]any); ok {
			for name, property := range properties {
				child, ok := property.(map[string]any)
				if !ok {
					t.Fatalf("%s/%s is not an object", path, name)
				}
				if description, _ := child["description"].(string); strings.TrimSpace(description) == "" {
					t.Errorf("schema property %s/%s has no description", path, name)
				}
				walk(child, path+"/"+name)
			}
		}
		for key, child := range object {
			if key != "properties" {
				walk(child, path+"/"+key)
			}
		}
	}
	walk(schema, "")
}
