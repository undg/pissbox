package bridge

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pissbox/host/bridge/services/github"
)

func TestGitHubQueueRefusesMissingParamsAndOverload(t *testing.T) {
	mailbox, root := fixture(t)
	if err := publishRecord(root, "generation", randomID()); err != nil {
		t.Fatal(err)
	}
	a := newSimulatedAgent(t, mailbox)
	live := &liveAgent{identity: agentIdentity{ID: a.id}, root: a.root, requests: make(map[string]*agentResponse)}
	r := messageRouter{githubJobs: make(chan githubJob, 1)}
	r.githubJobs <- githubJob{}
	id := a.request(t, agentRequest{Op: "github", GitHubOp: "pr_list", GitHubParams: &github.Request{Repo: "acme/project"}})
	r.requests(live)
	if got := a.response(t, id); got.Status != "failed" || !strings.Contains(got.Error, "queue full") || len(r.githubJobs) != 1 {
		t.Fatal(got)
	}
	id = a.request(t, agentRequest{Op: "github", GitHubOp: "pr_list"})
	r.requests(live)
	if got := a.response(t, id); got.Status != "failed" || !strings.Contains(got.Error, "unavailable") {
		t.Fatal(got)
	}
	r.githubJobs = nil
	id = a.request(t, agentRequest{Op: "github", GitHubOp: "pr_list", GitHubParams: &github.Request{Repo: "acme/project"}})
	r.requests(live)
	if got := a.response(t, id); got.Status != "failed" || !strings.Contains(got.Error, "unavailable") {
		t.Fatal(got)
	}
	malformed := randomID()
	raw := `{"version":1,"id":"` + malformed + `","op":"github","githubOp":"pr_list","githubParams":{"repo":"acme/project","command":"arbitrary"}}`
	if err := os.WriteFile(filepath.Join(a.dir, "requests", malformed+".json"), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	r.requests(live)
	if got := a.response(t, malformed); got.Status != "failed" || got.Error != "malformed request" {
		t.Fatal(got)
	}
}

func TestExpiredGitHubJobsNeverExecute(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	r := messageRouter{}
	done := r.startGitHubWorker(ctx)
	t.Cleanup(func() { cancel(); <-done })
	r.githubJobs <- githubJob{agent: "agent", id: "expired", op: "pr_list", params: github.Request{Repo: "acme/project"}, deadline: time.Now().Add(-time.Second)}
	select {
	case result := <-r.githubResults:
		if result.agent != "agent" || result.response.ID != "expired" || result.response.Status != "failed" || !strings.Contains(result.response.Error, "deadline") {
			t.Fatal(result)
		}
	case <-time.After(time.Second):
		t.Fatal("expired job not answered")
	}
}

func TestGitHubRequestsRunOffRouterAndDeduplicateWithoutReplayingOnReload(t *testing.T) {
	p := processFixture(t)
	tools := filepath.Join(p.dir, "tools")
	if err := os.Mkdir(tools, 0700); err != nil {
		t.Fatal(err)
	}
	calls := filepath.Join(p.dir, "gh-calls")
	started, release := filepath.Join(p.dir, "started"), filepath.Join(p.dir, "release")
	// Use absolute system tools: the daemon sees a closed PATH with only fake gh.
	// The blocked command proves router heartbeats and messaging remain responsive.
	script := `#!/bin/sh
printf 'call\n' >> "$GITHUB_TEST_CALLS"
printf started > "$GITHUB_TEST_STARTED"
while [ ! -f "$GITHUB_TEST_RELEASE" ]; do /bin/sleep 0.02; done
printf '%s' '{"data":{"repository":{"pullRequests":{"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":null}}}}}'
`
	if err := os.WriteFile(filepath.Join(tools, "gh"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	p.env = append(p.env, "PATH="+tools, "GITHUB_TEST_CALLS="+calls, "GITHUB_TEST_STARTED="+started, "GITHUB_TEST_RELEASE="+release)
	p.cli("ensure")
	a := newSimulatedAgent(t, messagingMailbox(t, p, "", ""))
	id := a.request(t, agentRequest{Op: "github", GitHubOp: "pr_list", GitHubParams: &github.Request{Repo: "acme/project", State: "all"}})
	waitFor(t, func() bool { _, err := os.Stat(started); return err == nil })
	list := a.request(t, agentRequest{Op: "list"})
	if got := a.response(t, list); got.Status != "ok" || got.Self == nil {
		t.Fatal("blocked gh stalled messaging", got)
	}
	if err := os.WriteFile(release, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if got := a.response(t, id); got.Status != "ok" || got.Result == nil {
		t.Fatal(got)
	}
	if err := a.root.Remove("responses/" + id + ".json"); err != nil {
		t.Fatal(err)
	}
	a.request(t, agentRequest{ID: id, Op: "github", GitHubOp: "pr_list", GitHubParams: &github.Request{Repo: "acme/other"}})
	if got := a.response(t, id); got.Status != "ok" {
		t.Fatal(got)
	}
	data, _ := os.ReadFile(calls)
	if string(data) != "call\n" {
		t.Fatal("duplicate ID executed gh again", string(data))
	}
	p.cli("reload")
	// Reload never reissues the already-consumed request. A new explicit read works.
	id = a.request(t, agentRequest{Op: "github", GitHubOp: "pr_list", GitHubParams: &github.Request{Repo: "acme/project"}})
	if got := a.response(t, id); got.Status != "ok" {
		t.Fatal(got)
	}
	data, _ = os.ReadFile(calls)
	if strings.Count(string(data), "call\n") != 2 {
		t.Fatal("reload replayed a read", string(data))
	}
	// The worker must return actionable failure without killing the bridge.
	if err := os.Remove(filepath.Join(tools, "gh")); err != nil {
		t.Fatal(err)
	}
	id = a.request(t, agentRequest{Op: "github", GitHubOp: "pr_list", GitHubParams: &github.Request{Repo: "acme/project"}})
	if got := a.response(t, id); got.Status != "failed" || !strings.Contains(got.Error, "host PATH") {
		t.Fatal(got)
	}
	list = a.request(t, agentRequest{Op: "list"})
	if got := a.response(t, list); got.Status != "ok" {
		t.Fatal(got)
	}
}
