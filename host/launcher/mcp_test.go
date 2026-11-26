package launcher

import (
	"slices"
	"testing"
)

func TestMcpSelectionParsing(t *testing.T) {
	for _, tc := range []struct {
		args  []string
		want  string
		paths []string
	}{
		{nil, "none", nil},
		{[]string{"--mcp", "NONE"}, "none", nil},
		{[]string{"--mcp", "chrome-devtools", ".", "--mcp=network,docs"}, "chrome-devtools,network,docs", []string{"."}},
		{[]string{"--mcp", "company.tools_1"}, "company.tools_1", nil},
		{[]string{"--", "--mcp", "browser"}, "none", []string{"--mcp", "browser"}},
	} {
		o, err := parse(tc.args)
		if err != nil || o.staticMcp() != tc.want || !slices.Equal(o.workspaces, tc.paths) {
			t.Fatalf("parse(%q): %q, paths=%q, err=%v", tc.args, o.staticMcp(), o.workspaces, err)
		}
	}
}

func TestInvalidMcpSelectionFailsBeforeLaunch(t *testing.T) {
	for _, args := range [][]string{
		{"--mcp"}, {"--mcp="}, {"--mcp", "--name", "dev"},
		{"--mcp", "none,browser"}, {"--mcp", "browser", "--mcp", "NoNe"},
		{"--mcp", "browser,"}, {"--mcp", "a b"}, {"--mcp", "http://browser"},
	} {
		f := newFixture(t)
		out, status, calls := f.launch("", nil, args...)
		if status == 0 || len(calls) != 0 {
			t.Fatalf("%q: status=%d calls=%v output=%s", args, status, calls, out)
		}
	}
}

func TestCreationMcpSelectionDoesNotChangeAttach(t *testing.T) {
	for _, staged := range []bool{false, true} {
		for _, servers := range []string{"", "chrome-devtools,docs"} {
			f := newFixture(t)
			if staged {
				f.config("isolated_directories = ['deps']\n")
			}
			args := []string{"-y"}
			if servers != "" {
				args = append(args, "--mcp", servers)
			}
			out, status, calls := f.launch("", nil, args...)
			if status != 0 {
				t.Fatalf("creation failed: %s", out)
			}
			want := servers
			if want == "" {
				want = "none"
			}
			if flagValue(calls[0], "--static-mcp") != want {
				t.Fatalf("creation MCP set: %v", calls)
			}
			out, status, calls = f.launch("", map[string]string{"CONFLICT": "1"}, args...)
			if status != 0 {
				t.Fatalf("attach failed: %s", out)
			}
			if flagValue(calls[0], "--static-mcp") != want || flagValue(calls[len(calls)-1], "--static-mcp") != "" {
				t.Fatalf("attach changed MCP mode: %v", calls)
			}
		}
	}
}

func TestRecreationRetainsExplicitMcpSelectionOnSingleRetry(t *testing.T) {
	f := newFixture(t)
	out, status, calls := f.launch("", map[string]string{"CONFLICT_UNTIL_REMOVED": "1"}, "--recreate", "-y", "--mcp", "browser")
	if status != 0 || kinds(calls) != "run,rm,run" {
		t.Fatalf("status=%d calls=%v: %s", status, calls, out)
	}
	for _, i := range []int{0, 2} {
		if flagValue(calls[i], "--static-mcp") != "browser" {
			t.Fatal(calls)
		}
	}
}
