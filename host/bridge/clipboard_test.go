package bridge

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"pissbox/host/bridge/services/clipboard"
)

func clipboardDirectory(t *testing.T) string {
	t.Helper()
	dir, _ := fixture(t)
	dir, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func clipboardRequest(text string) agentRequest {
	return agentRequest{Op: "clipboard_copy", ClipboardText: &text}
}

func clipboardCommandRequest(text string) agentRequest {
	req := clipboardRequest(text)
	req.ClipboardShellCommand = true
	return req
}

func TestClipboardQueueValidationAndOverload(t *testing.T) {
	mailbox, root := fixture(t)
	if err := publishRecord(root, "generation", randomID()); err != nil {
		t.Fatal(err)
	}
	a := newSimulatedAgent(t, mailbox)
	live := &liveAgent{identity: agentIdentity{ID: a.id}, root: a.root, requests: make(map[string]*agentResponse)}
	r := messageRouter{}
	for _, tc := range []struct {
		req  agentRequest
		want string
	}{
		{agentRequest{Op: "clipboard_copy"}, "required"},
		{clipboardRequest(strings.Repeat("a", maxTextBytes+1)), "16384"},
		{clipboardRequest("text"), "unavailable"},
		{clipboardCommandRequest(strings.Repeat("<", 5300)), "cannot fit"},
	} {
		id := a.request(t, tc.req)
		r.requests(live)
		if got := a.response(t, id); got.Status != "failed" || !strings.Contains(got.Error, tc.want) {
			t.Fatal(got)
		}
	}
	// Sandbox requests cannot select host commands or desktop connections.
	malformed := randomID()
	raw := `{"version":1,"id":"` + malformed + `","op":"clipboard_copy","clipboardText":"text","desktop":{"display":":99"}}`
	if err := os.WriteFile(filepath.Join(a.dir, "requests", malformed+".json"), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	r.requests(live)
	if got := a.response(t, malformed); got.Status != "failed" || got.Error != "malformed request" {
		t.Fatal(got)
	}
	r.clipboardJobs = make(chan clipboardJob, 1)
	r.clipboardJobs <- clipboardJob{}
	id := a.request(t, clipboardRequest("text"))
	r.requests(live)
	if got := a.response(t, id); got.Status != "failed" || !strings.Contains(got.Error, "queue full") {
		t.Fatal(got)
	}
	<-r.clipboardJobs
	id = a.request(t, clipboardRequest(""))
	r.requests(live)
	if got := <-r.clipboardJobs; got.id != id || got.text != "" {
		t.Fatal(got)
	}
	if cached, ok := live.requests[id]; !ok || cached != nil {
		t.Fatal("queued request not marked pending")
	}
	id = a.request(t, clipboardCommandRequest("make check"))
	r.requests(live)
	if got := <-r.clipboardJobs; got.id != id || got.text != "make check" || !got.shellCommand {
		t.Fatal("command mode did not reach worker", got)
	}
}

func TestExpiredClipboardJobNeverExecutes(t *testing.T) {
	t.Setenv("PATH", clipboardDirectory(t))
	ctx, cancel := context.WithCancel(context.Background())
	r := messageRouter{}
	done := r.startClipboardWorker(ctx)
	t.Cleanup(func() { cancel(); <-done })
	r.clipboardJobs <- clipboardJob{agent: "agent", id: "expired", desktop: &clipboard.DesktopEnvironment{Display: ":1"}, deadline: time.Now().Add(-time.Second)}
	select {
	case result := <-r.clipboardResults:
		if result.agent != "agent" || result.response.ID != "expired" || result.response.Status != "failed" || !strings.Contains(result.response.Error, "expired") {
			t.Fatal(result)
		}
	case <-time.After(time.Second):
		t.Fatal("expired job not answered")
	}
}

func TestClipboardHostDesktopBindingSurvivesReloadAndDoesNotBlockOrReplay(t *testing.T) {
	p := processFixture(t)
	tools := filepath.Join(p.dir, "tools")
	if err := os.Mkdir(tools, 0700); err != nil {
		t.Fatal(err)
	}
	calls, started, release := filepath.Join(p.dir, "calls"), filepath.Join(p.dir, "started"), filepath.Join(p.dir, "release")
	script := `#!/bin/sh
printf '%s|%s|%s|%s\n' "$DISPLAY" "$WAYLAND_DISPLAY" "$XDG_RUNTIME_DIR" "$XAUTHORITY" >> "$CLIP_TEST_CALLS"
printf started > "$CLIP_TEST_STARTED"
while [ ! -f "$CLIP_TEST_RELEASE" ]; do /bin/sleep 0.02; done
/bin/cat > "$CLIP_TEST_OUT"
`
	for _, name := range []string{"pbcopy", "wl-copy", "xclip", "xsel"} {
		if err := os.WriteFile(filepath.Join(tools, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(p.dir, "out")
	p.env = append(p.env, "PATH="+tools, "DISPLAY=stale", "WAYLAND_DISPLAY=", "XDG_RUNTIME_DIR=", "XAUTHORITY=stale", "CLIP_TEST_CALLS="+calls, "CLIP_TEST_STARTED="+started, "CLIP_TEST_RELEASE="+release, "CLIP_TEST_OUT="+out)
	p.cli("ensure")
	// Register from a different host desktop than the daemon's startup environment.
	p.env = append(p.env, "DISPLAY=:42", "WAYLAND_DISPLAY=wayland-42", "XDG_RUNTIME_DIR=/run/test-42", "XAUTHORITY=")
	mailbox := clipboardDirectory(t)
	registration := strings.TrimSpace(string(p.cli("register", "--mailbox", mailbox, "--owner-pid", strconv.Itoa(os.Getpid()))))
	a := newSimulatedAgent(t, mailbox)
	text := "exact clipboard text 世界\n"
	id := a.request(t, clipboardRequest(text))
	waitFor(t, func() bool { _, err := os.Stat(started); return err == nil })
	if got := a.response(t, a.request(t, agentRequest{Op: "list"})); got.Status != "ok" {
		t.Fatal("clipboard stalled router", got)
	}
	if err := os.WriteFile(release, nil, 0600); err != nil {
		t.Fatal(err)
	}
	got := a.response(t, id)
	var result map[string]string
	data, _ := json.Marshal(got.Result)
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	wantBackend := "wl-copy"
	if runtime.GOOS == "darwin" {
		wantBackend = "pbcopy"
	}
	if got.Status != "ok" || result["backend"] != wantBackend {
		t.Fatal(got)
	}
	if data, err := os.ReadFile(out); err != nil || string(data) != text {
		t.Fatalf("copied %q: %v", data, err)
	}
	if err := a.root.Remove("responses/" + id + ".json"); err != nil {
		t.Fatal(err)
	}
	a.request(t, agentRequest{ID: id, Op: "clipboard_copy", ClipboardText: &text})
	if got := a.response(t, id); got.Status != "ok" {
		t.Fatal(got)
	}
	// A second registration must not overwrite the first launch's desktop.
	p.env = append(p.env, "DISPLAY=:43", "WAYLAND_DISPLAY=wayland-43", "XDG_RUNTIME_DIR=/run/test-43", "XAUTHORITY=auth-43")
	otherMailbox := clipboardDirectory(t)
	p.cli("register", "--mailbox", otherMailbox, "--owner-pid", strconv.Itoa(os.Getpid()))
	b := newSimulatedAgent(t, otherMailbox)
	if got := b.response(t, b.request(t, clipboardRequest("second desktop"))); got.Status != "ok" {
		t.Fatal(got)
	}
	// Reload starts from yet another environment; the handoff must preserve :42.
	p.env = append(p.env, "DISPLAY=wrong-reload", "WAYLAND_DISPLAY=wrong-reload", "XAUTHORITY=wrong-reload")
	p.cli("reload")
	a.refresh(t)
	if got := a.response(t, a.request(t, clipboardRequest(""))); got.Status != "ok" {
		t.Fatal(got)
	}
	data, err := os.ReadFile(calls)
	wantCalls := ":42|wayland-42|/run/test-42|\n:43|wayland-43|/run/test-43|auth-43\n:42|wayland-42|/run/test-42|\n"
	if err != nil || string(data) != wantCalls {
		t.Fatalf("clipboard replayed or lost desktop binding: %q, %v", data, err)
	}
	if data, err := os.ReadFile(out); err != nil || len(data) != 0 {
		t.Fatalf("empty text did not clear clipboard: %q, %v", data, err)
	}
	// The host-only reconnect path can refresh a registration's desktop binding.
	if got := strings.TrimSpace(string(p.cli("reconnect", "--mailbox", mailbox, "--owner-pid", strconv.Itoa(os.Getpid())))); got != registration {
		t.Fatalf("reconnect replaced registration: %q", got)
	}
	a.refresh(t)
	if got := a.response(t, a.request(t, clipboardRequest("after reconnect"))); got.Status != "ok" {
		t.Fatal(got)
	}
	data, err = os.ReadFile(calls)
	if err != nil || string(data) != wantCalls+"wrong-reload|wrong-reload|/run/test-43|wrong-reload\n" {
		t.Fatalf("reconnect did not refresh desktop: %q, %v", data, err)
	}
	got = a.response(t, a.request(t, clipboardCommandRequest("make check")))
	data, _ = json.Marshal(got.Result)
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if got.Status != "ok" || result["backend"] != wantBackend || !strings.Contains(result["copiedText"], "make check\n} 2>&1 | tee /dev/tty | "+wantBackend) {
		t.Fatal("command was not prepared", got)
	}
	if data, err := os.ReadFile(out); err != nil || string(data) != result["copiedText"] {
		t.Fatalf("prepared result differs from clipboard: %q, %v", data, err)
	}
}
