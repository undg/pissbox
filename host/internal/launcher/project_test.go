package launcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSymlinkedConfigPathsRemainRelativeToPrimary(t *testing.T) {
	f := newFixture(t)
	f.config("workspaces = []")
	extra := filepath.Join(f.root, "extra")
	os.Mkdir(extra, 0700)
	rules := filepath.Join(f.root, "rules/nested/project.toml")
	f.write(rules, "workspaces = [\"../extra\"]\nsetup = [\"touch forbidden-host-marker\"]\n")
	// Invalid secondary config must never be read.
	f.write(filepath.Join(extra, ".pissbox.toml"), "invalid TOML")
	config := filepath.Join(f.project, ".pissbox.toml")
	os.Remove(config)
	os.Symlink(rules, config)
	out, status, calls := f.launch("", nil, ".", extra)
	if status != 0 || kinds(calls) != "run" {
		t.Fatalf("%d %v %s", status, calls, out)
	}
	mounts := strings.Join(calls[0], "\n")
	if strings.Contains(mounts, "rules/nested") || strings.Count(mounts, extra) != 1 {
		t.Fatal(calls)
	}
	if _, err := os.Stat(filepath.Join(f.project, "forbidden-host-marker")); !os.IsNotExist(err) {
		t.Fatal("ran setup on host")
	}
}
func TestSettingsFailurePreservesExistingLinkAndPreventsLaunch(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	f := newFixture(t)
	target := filepath.Join(f.pi, "settings.json")
	if err := os.Symlink("settings-go.json", target); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(f.pi, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(f.pi, 0700) })
	out, status, calls := f.launch("", nil)
	if status == 0 || len(calls) != 0 || !strings.Contains(out, "permission denied") {
		t.Fatalf("%d %v %s", status, calls, out)
	}
	if link, err := os.Readlink(target); err != nil || link != "settings-go.json" {
		t.Fatal(link, err)
	}
	if _, err := os.Stat(f.state); !os.IsNotExist(err) {
		t.Fatal("created state despite failed settings")
	}
}
func TestRelativeDirectoryOverridesAreCallerRelativeAndPhysical(t *testing.T) {
	f := newFixture(t)
	alias := filepath.Join(f.root, "pi-alias")
	os.Symlink(f.pi, alias)
	out, status, calls := f.launch("", map[string]string{"PISSBOX_PI_DIR": "missing", "PISSBOX_STATE_DIR": "missing"}, "--pissbox-pi-dir=../pi-alias", "--pissbox-state-dir=../new-state")
	if status != 0 || kinds(calls) != "run" {
		t.Fatalf("%d %v %s", status, calls, out)
	}
	args := strings.Join(calls[0], "\n")
	if !strings.Contains(args, "PISSBOX_PI_DIR="+f.pi) || !strings.Contains(args, "PISSBOX_STATE_DIR="+filepath.Join(f.root, "new-state")) || strings.Contains(args, "pi-alias") {
		t.Fatal(calls)
	}
}
func TestBrokenConfigLinkPreventsEffects(t *testing.T) {
	f := newFixture(t)
	os.Symlink("missing", filepath.Join(f.project, ".pissbox.toml"))
	out, status, calls := f.launch("", nil)
	if status == 0 || len(calls) != 0 || !strings.Contains(out, "readable TOML file") {
		t.Fatalf("%d %v %s", status, calls, out)
	}
	if _, err := os.Stat(f.state); !os.IsNotExist(err) {
		t.Fatal("created state")
	}
}

func TestParseProjectConfigAcceptsDocumentedValues(t *testing.T) {
	for name, test := range map[string]struct {
		text string
		want project
	}{
		"empty": {"", project{}},
		"all keys": {
			"setup = [\"pnpm install --frozen-lockfile\"]\nworkspaces = [\"..\", \"../shared-tools\"]\nisolated_directories = [\"node_modules\"]\n",
			project{
				Setup:      []string{"pnpm install --frozen-lockfile"},
				Workspaces: []string{"..", "../shared-tools"},
				Isolated:   []string{"node_modules"},
			},
		},
		"nested isolated directories": {
			"isolated_directories = [\"node_modules\", \"packages/app/node_modules\"]",
			project{Isolated: []string{"node_modules", "packages/app/node_modules"}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := parseProject([]byte(test.text))
			if err != nil {
				t.Fatal(err)
			}
			if fmt.Sprint(got) != fmt.Sprint(test.want) {
				t.Fatalf("got %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestParseProjectConfigRejectsInvalidDocuments(t *testing.T) {
	for _, text := range []string{
		"not toml",
		"[]",
		"\"top-level string\"",
		"setup = [\"a\"]\nsetup = []",
		"unknown = []",
		"[setup]\ncommands = []",
		"workspaces = \"..\"",
		"setup = 5",
		"setup = [7]",
		"setup = [\"a\\u0000b\"]",
		"workspaces = [\"\"]",
		"workspaces = [\"a\\nb\"]",
		"workspaces = [\"a\\u0000b\"]",
		"isolated_directories = [\"\"]",
		"isolated_directories = [\"/node_modules\"]",
		"isolated_directories = [\".\"]",
		"isolated_directories = [\"..\"]",
		"isolated_directories = [\"a/../node_modules\"]",
		"isolated_directories = [\"a/./node_modules\"]",
		"isolated_directories = [\"a//node_modules\"]",
		"isolated_directories = [\"node_modules/\"]",
		"isolated_directories = [\"a:b\"]",
		"isolated_directories = [\"a\\nb\"]",
		"isolated_directories = [\"a\\u007fb\"]",
		"isolated_directories = [\"a\\u0085b\"]",
		"isolated_directories = [\"a\", \"a\"]",
		"isolated_directories = [\"a\", \"a/b\"]",
		"isolated_directories = [\"a/b\", \"a\"]",
	} {
		if _, err := parseProject([]byte(text)); err == nil {
			t.Errorf("accepted %q", text)
		}
	}
}
