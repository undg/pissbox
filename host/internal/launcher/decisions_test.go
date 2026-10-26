package launcher

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParserPreservesInterspersedOptionsAndLastValues(t *testing.T) {
	o, err := parse([]string{"one", "--name=old", "--pissbox-pi-dir=old", "two", "-c", "--name", "new", "--pissbox-pi-dir", "new", "--pissbox-state-dir=state", "-y", "--ignore-config-workspaces", "--", "-c", "--yes"})
	if err != nil {
		t.Fatal(err)
	}
	if o.name != "new" || o.piDir != "new" || o.stateDir != "state" || !o.yes || !o.ignoreConfig || !o.continued || fmt.Sprint(o.workspaces) != "[one two -c --yes]" {
		t.Fatalf("%+v", o)
	}
	for _, args := range [][]string{{"--name"}, {"--name="}, {"--name", "-y"}, {"-d"}, {"--delete="}, {"--pissbox-state-dir", ""}, {"--pissbox-pi-dir="}, {"--unknown"}, {"--bridge-stop", "-y"}} {
		if _, err := parse(args); err == nil {
			t.Fatalf("accepted %q", args)
		}
	}
	o, err = parse([]string{"--delete=missing", "-d", "last", "--help", "--unknown"})
	if err != nil || !o.help || o.deleteDir != "last" {
		t.Fatal(o, err)
	}
}
func TestSettingsProfilesAndRelinking(t *testing.T) {
	f := newFixture(t)
	for _, host := range []string{"cm", "di-7415", "other", "", "cm.example"} {
		profile := settingsProfile(host)
		expected := "settings-co.json"
		if host == "cm" || host == "di-7415" || host == "cm.example" {
			expected = "settings-go.json"
		}
		if profile != expected {
			t.Fatal(host, profile)
		}
		if err := linkSettings(f.pi, profile); err != nil {
			t.Fatal(err)
		}
		if actual, err := os.Readlink(filepath.Join(f.pi, "settings.json")); err != nil || actual != expected {
			t.Fatal(actual, err)
		}
	}
	target := filepath.Join(f.pi, "settings.json")
	os.Remove(target)
	f.write(target, "preserved settings")
	if err := linkSettings(f.pi, "settings-go.json"); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(target + ".replaced"); err != nil || string(data) != "preserved settings" {
		t.Fatal(string(data), err)
	}
	os.Remove(target)
	f.write(target, "second settings")
	if err := linkSettings(f.pi, "settings-co.json"); err != nil {
		t.Fatal(err)
	}
	backups, _ := filepath.Glob(target + ".replaced.*")
	if len(backups) != 1 {
		t.Fatal(backups)
	}
	if err := linkSettings(f.pi, "missing.json"); err == nil || !strings.Contains(err.Error(), "Settings profile does not exist") {
		t.Fatal(err)
	}
}
func TestMissingProfilePreventsStateAndSBX(t *testing.T) {
	f := newFixture(t)
	os.Remove(filepath.Join(f.pi, "settings-go.json"))
	out, status, calls := f.launch("", map[string]string{"LAUNCHER_HOST": "cm"})
	if status == 0 || len(calls) != 0 || !strings.Contains(out, "Settings profile does not exist") {
		t.Fatalf("%d %v %s", status, calls, out)
	}
	if _, err := os.Stat(f.state); !os.IsNotExist(err) {
		t.Fatal("created state")
	}
}
func TestWorkspaceKeysRetainFirstSpelling(t *testing.T) {
	f := newFixture(t)
	alias := filepath.Join(f.root, "alias")
	if err := os.Symlink(f.project, alias); err != nil {
		t.Fatal(err)
	}
	var w workspaceSet
	for _, path := range []string{alias, f.project, alias, "missing", "missing", "./missing"} {
		w.append(path)
	}
	if fmt.Sprint(w.paths) != fmt.Sprint([]string{alias, "missing", "./missing"}) {
		t.Fatal(w.paths)
	}
}
func TestIsolationMountPathValidation(t *testing.T) {
	for _, path := range []string{"a:b", "a\nb", "a\x7fb", "a\u0085b"} {
		if safeMountPath(path) {
			t.Fatal(path)
		}
	}
	if !safeMountPath("directory with spaces/日本語") {
		t.Fatal("rejected ordinary path")
	}
	f := newFixture(t)
	colon := filepath.Join(f.root, "colon:state")
	os.Mkdir(colon, 0700)
	alias := filepath.Join(f.root, "alias")
	os.Symlink(colon, alias)
	if _, err := validateIsolation(f.project, filepath.Join(alias, "new"), []string{"deps"}); err == nil || !strings.Contains(err.Error(), "state path resolves") {
		t.Fatal(err)
	}
	for _, kind := range []string{"file", "link", "broken"} {
		path := filepath.Join(f.project, kind)
		if kind == "file" {
			f.write(path, "")
		} else {
			os.Symlink("missing", path)
		}
		if _, err := validateIsolation(f.project, f.state, []string{kind + "/deps"}); err == nil {
			t.Fatal(kind)
		}
	}
}
func TestConflictClassificationIsNarrow(t *testing.T) {
	ordinary := "sandbox 'actual' already exists and can't be given new workspaces"
	created := `sandbox "actual" already exists for this workspace; pass --name to create another, or start the existing sandbox`
	named := "sandbox 'actual' already exists; connect to the existing sandbox"
	for _, tc := range []struct {
		text            string
		isolated        bool
		requested, want string
	}{
		{ordinary, false, "requested", "actual"}, {ordinary, true, "", ""}, {created, true, "", "actual"}, {created, false, "", ""}, {named, true, "actual", "actual"}, {named, true, "other", ""}, {named, true, "", ""}, {"daemon unavailable", false, "", ""},
	} {
		if got := conflictName(tc.text, tc.isolated, tc.requested); got != tc.want {
			t.Fatalf("%+v: %q", tc, got)
		}
	}
}
func TestSandboxListingValidatesWholeArray(t *testing.T) {
	valid := map[string]any{"name": "first", "agent": "pi", "status": "running", "workspaces": []string{"/exact"}}
	for _, tc := range []struct {
		field   string
		value   any
		message string
	}{
		{"name", nil, "name must be a string"}, {"name", "-unsafe", "name contains unsupported"}, {"agent", false, "agent must be a string"}, {"status", 42, "status must be a string"}, {"workspaces", false, "workspaces must be an array"}, {"workspaces", []any{42}, "workspaces must contain absolute"}, {"workspaces", []string{"relative"}, "workspaces must contain absolute"},
	} {
		invalid := map[string]any{"name": "broken", "agent": "pi", "status": "running", "workspaces": nil}
		invalid[tc.field] = tc.value
		data, _ := json.Marshal([]any{valid, invalid})
		list, err := sandboxListing(string(data), "/exact")
		if err == nil || list != nil || !strings.Contains(err.Error(), tc.message) {
			t.Fatalf("%s: %v %v", data, list, err)
		}
	}
	for _, data := range []string{"", "null", "{}", "[null]", `[{"name":"broken","agent":"pi","status":"running"}]`} {
		if _, err := sandboxListing(data, "/exact"); err == nil {
			t.Fatal(data)
		}
	}
	data, _ := json.Marshal([]any{valid, map[string]any{"name": "none", "agent": "pi", "status": "stopped", "workspaces": nil}})
	list, err := sandboxListing(string(data), "/exact")
	if err != nil || len(list) != 2 || !list[0].associated || list[1].associated {
		t.Fatal(list, err)
	}
	list, err = sandboxListing(string(data), "/exact/child")
	if err != nil || list[0].associated {
		t.Fatal(list, err)
	}
}
func TestManagementEarlyExitAndDeletionSafety(t *testing.T) {
	for _, tc := range []struct {
		args                 []string
		input, listing, want string
		status               int
	}{
		{[]string{"--help"}, "", "not-json", "", 0},
		{[]string{"-l"}, "", "[]", "ls", 0},
		{[]string{"-d", ".", "-y"}, "", `[{"name":"box","agent":"pi","status":"running","workspaces":["PRIMARY"]}]`, "ls,rm", 0},
		{[]string{"-d", "."}, "yes\n", `[{"name":"box","agent":"pi","status":"running","workspaces":["PRIMARY"]}]`, "ls,rm", 0},
		{[]string{"-d", "."}, "y", `[{"name":"box","agent":"pi","status":"running","workspaces":["PRIMARY"]}]`, "ls", 1},
		{[]string{"-d", ".", "-y"}, "", `[{"name":"box","agent":"pi","status":"running","workspaces":["PRIMARY"]},{"name":"bad","agent":null,"status":"running","workspaces":null}]`, "ls", 1},
		{[]string{"-d", ".", "-y"}, "", `[{"name":"parent","agent":"pi","status":"running","workspaces":["PRIMARY/child"]}]`, "ls", 1},
	} {
		t.Run(strings.Join(tc.args, " ")+tc.input+tc.want, func(t *testing.T) {
			f := newFixture(t)
			f.config("not valid toml")
			listing := strings.ReplaceAll(tc.listing, "PRIMARY", f.project)
			out, status, calls := f.launch(tc.input, map[string]string{"LISTING": listing}, tc.args...)
			if status != tc.status || kinds(calls) != tc.want {
				t.Fatalf("%d %v %s", status, calls, out)
			}
			if _, err := os.Lstat(filepath.Join(f.pi, "settings.json")); !os.IsNotExist(err) {
				t.Fatal("management touched settings")
			}
			if _, err := os.Stat(f.state); !os.IsNotExist(err) {
				t.Fatal("management created state")
			}
		})
	}
}
