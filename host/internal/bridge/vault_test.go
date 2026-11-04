package bridge

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"pissbox/host/internal/vault"
)

func TestVaultRequestsAreDeduplicatedAcrossLostResponses(t *testing.T) {
	p := processFixture(t)
	notes, _ := fixture(t)
	notes, err := filepath.Abs(notes)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(p.dir, "config.toml")
	if err := os.WriteFile(configPath, []byte("[vault.shared]\npath = "+strconv.Quote(notes)+"\ngit = false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	p.cli("ensure")
	a := newSimulatedAgent(t, messagingMailbox(t, p, "", ""))
	id := a.request(t, agentRequest{Op: "vault", VaultOp: "new", Params: &vault.Request{Path: "shared.md", Content: "original"}})
	first := a.response(t, id)
	if first.Status != "ok" {
		t.Fatal(first)
	}
	data, err := os.ReadFile(filepath.Join(notes, "shared.md"))
	if err != nil || !strings.HasSuffix(string(data), "original") {
		t.Fatal("successful request did not persist the note", string(data), err)
	}
	// The same ID with different arguments must not execute a second write.
	a.request(t, agentRequest{ID: id, Op: "vault", VaultOp: "new", Params: &vault.Request{Path: "duplicate.md", Content: "should not exist"}})
	time.Sleep(250 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(notes, "duplicate.md")); !os.IsNotExist(err) {
		t.Fatal("replayed mutation")
	}
	if got := a.response(t, id); got.Status != "ok" {
		t.Fatal(got)
	}
	bad := a.request(t, agentRequest{Op: "vault", VaultOp: "new"})
	if got := a.response(t, bad); got.Status != "failed" {
		t.Fatal("accepted missing params", got)
	}
	malformed := randomID()
	raw := `{"version":1,"id":"` + malformed + `","op":"vault","vaultOp":"read","params":{"path":"shared.md","command":"touch forbidden"}}`
	if err := os.WriteFile(filepath.Join(a.dir, "requests", malformed+".json"), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if got := a.response(t, malformed); got.Status != "failed" {
		t.Fatal("accepted unknown nested capability", got)
	}
}

func TestHostVaultHistoryConfigurationDefaultTrueAndFalse(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, tc := range []struct {
		setting  string
		recorded bool
	}{{"", true}, {"git = true\n", true}, {"git = false\n", false}} {
		t.Run(tc.setting, func(t *testing.T) {
			dir := t.TempDir()
			git := func(args ...string) string {
				cmd := exec.Command("git", args...)
				cmd.Dir = dir
				data, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("git %v: %s %v", args, data, err)
				}
				return strings.TrimSpace(string(data))
			}
			git("init")
			git("config", "user.name", "Vault config test")
			git("config", "user.email", "vault@example.invalid")
			configPath := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(configPath, []byte("[vault.shared]\npath = "+strconv.Quote(dir)+"\n"+tc.setting), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PISSBOX_CONFIG", configPath)
			s, err := hostVaultService()
			if err != nil {
				t.Fatal(err)
			}
			result, err := s.Execute(context.Background(), "new", vault.Request{Path: "recorded.md", Content: "note"})
			if err != nil {
				t.Fatal(err)
			}
			if n := result.(vault.Note); n.Warning != "" {
				t.Fatal(n.Warning)
			}
			recorded := git("ls-files") == "recorded.md"
			if recorded != tc.recorded {
				t.Fatalf("history recorded=%v; want %v", recorded, tc.recorded)
			}
		})
	}
}

func TestVaultQueueFullAndUnavailableReturnExplicitFailures(t *testing.T) {
	mailbox, _ := fixture(t)
	a := newSimulatedAgent(t, mailbox)
	live := &liveAgent{identity: agentIdentity{ID: a.id}, root: a.root, requests: make(map[string]*agentResponse)}
	r := messageRouter{vaultJobs: make(chan vaultJob, 1)}
	r.vaultJobs <- vaultJob{}
	id := a.request(t, agentRequest{Op: "vault", VaultOp: "new", Params: &vault.Request{Path: "note.md", Content: "body"}})
	r.requests(live)
	if got := a.response(t, id); got.Status != "failed" || !strings.Contains(got.Error, "queue full") {
		t.Fatal(got)
	}
	if len(r.vaultJobs) != 1 {
		t.Fatal("full queue accepted another mutation")
	}
	r.vaultJobs = nil
	id = a.request(t, agentRequest{Op: "vault", VaultOp: "list", Params: &vault.Request{}})
	r.requests(live)
	if got := a.response(t, id); got.Status != "failed" || !strings.Contains(got.Error, "unavailable") {
		t.Fatal(got)
	}
}

func TestExpiredQueuedVaultRequestsNeverWrite(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configPath, []byte("[vault.shared]\npath = "+strconv.Quote(dir)+"\ngit = false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PISSBOX_CONFIG", configPath)
	ctx, cancel := context.WithCancel(context.Background())
	r := messageRouter{}
	done := r.startVaultWorker(ctx)
	t.Cleanup(func() { cancel(); <-done })
	r.vaultJobs <- vaultJob{agent: randomID(), id: randomID(), op: "new", params: vault.Request{Path: "expired.md", Content: "late write"}, deadline: time.Now().Add(-time.Second)}
	select {
	case result := <-r.vaultResults:
		if result.response.Status != "failed" || !strings.Contains(result.response.Error, "deadline") {
			t.Fatal(result)
		}
	case <-time.After(time.Second):
		t.Fatal("expired request was not answered")
	}
	if _, err := os.Stat(filepath.Join(dir, "expired.md")); !os.IsNotExist(err) {
		t.Fatal("expired request wrote a note")
	}
}

func TestHostVaultConfigurationMissingDefaultAndInvalidExplicit(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("PISSBOX_CONFIG", "")
	if s, err := hostVaultService(); err != nil || s == nil {
		t.Fatal(s, err)
	}
	t.Setenv("PISSBOX_CONFIG", filepath.Join(t.TempDir(), "missing.toml"))
	if _, err := hostVaultService(); err == nil {
		t.Fatal("accepted missing explicit config")
	}
	path := filepath.Join(t.TempDir(), "invalid.toml")
	if err := os.WriteFile(path, []byte("secret-marker-not-for-output = true"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PISSBOX_CONFIG", path)
	if _, err := hostVaultService(); err == nil || strings.Contains(err.Error(), "secret-marker") {
		t.Fatal(err)
	}
}
