package launcher

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"testing"
)

func TestVersionFromBuildInfo(t *testing.T) {
	for _, c := range []struct {
		name     string
		injected string
		info     *debug.BuildInfo
		want     string
	}{
		{
			name: "release takes precedence over dirty revision", injected: "v1.2.3",
			info: &debug.BuildInfo{Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "0123456789abcdef"},
				{Key: "vcs.modified", Value: "true"},
			}},
			want: "v1.2.3",
		},
		{name: "release without build info", injected: "v1.2.3", want: "v1.2.3"},
		{name: "no build info", injected: "dev", want: "dev"},
		{name: "no settings", injected: "dev", info: &debug.BuildInfo{}, want: "dev"},
		{
			name: "modified flag without revision", injected: "dev",
			info: &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.modified", Value: "true"}}},
			want: "dev",
		},
		{
			name: "unrelated build settings", injected: "dev",
			info: &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "GOARCH", Value: "arm64"}}},
			want: "dev",
		},
		{
			name: "short revision", injected: "dev",
			info: &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "abc123"}}},
			want: "abc123",
		},
		{
			name: "twelve character revision", injected: "dev",
			info: &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "0123456789ab"}}},
			want: "0123456789ab",
		},
		{
			name: "long clean revision", injected: "dev",
			info: &debug.BuildInfo{Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "0123456789abcdef"},
				{Key: "vcs.modified", Value: "false"},
			}},
			want: "0123456789ab",
		},
		{
			name: "dirty revision with settings in reverse order", injected: "dev",
			info: &debug.BuildInfo{Settings: []debug.BuildSetting{
				{Key: "vcs.modified", Value: "true"},
				{Key: "vcs.revision", Value: "0123456789abcdef"},
			}},
			want: "0123456789ab-dirty",
		},
		{
			name: "only literal true marks revision dirty", injected: "dev",
			info: &debug.BuildInfo{Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abc123"},
				{Key: "vcs.modified", Value: "1"},
			}},
			want: "abc123",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := versionFromBuildInfo(c.injected, c.info); got != c.want {
				t.Fatalf("versionFromBuildInfo(%q, %+v) = %q, want %q", c.injected, c.info, got, c.want)
			}
		})
	}
}

func TestResolveVersionUsesInjectedRelease(t *testing.T) {
	original := version
	t.Cleanup(func() { version = original })
	version = "v1.2.3"

	if got := resolveVersion(); got != "v1.2.3" {
		t.Fatalf("resolveVersion() = %q, want v1.2.3", got)
	}
}

func TestVersionFlagHasNoLaunchSideEffects(t *testing.T) {
	for _, args := range [][]string{{"--version"}, {"-v"}} {
		t.Run(args[0], func(t *testing.T) {
			f := newFixture(t)
			f.write(filepath.Join(f.project, ".pissbox.toml"), "not valid TOML = [")
			marker := filepath.Join(f.root, "secret-command-ran")
			f.write(f.userConfig, "[secrets.API]\nhost = 'example.com'\ncommand = 'touch "+marker+"'\n")

			out, status, calls := f.launch("", map[string]string{"PISSBOX_PI_DIR": "missing"}, args...)
			if status != 0 || len(calls) != 0 {
				t.Fatalf("status = %d, calls = %v, output = %s", status, calls, out)
			}
			if !regexp.MustCompile(`^pissbox \S+\n$`).MatchString(out) {
				t.Fatalf("unexpected version output: %q", out)
			}
			for _, path := range []string{marker, f.state, filepath.Join(f.pi, "settings.json")} {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatalf("unexpected side effect at %s: %v", path, err)
				}
			}
		})
	}
}
