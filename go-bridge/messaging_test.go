package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

type simulatedAgent struct {
	id   string
	dir  string
	root *os.Root
}

func newSimulatedAgent(t *testing.T, mailbox string) simulatedAgent {
	t.Helper()
	id := randomID()
	dir := filepath.Join(mailbox, "agents", id)
	for _, sub := range []string{"requests", "responses", "inbox", "receipts"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0700); err != nil {
			t.Fatal(err)
		}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	a := simulatedAgent{id, dir, root}
	a.refresh(t)
	return a
}
func (a simulatedAgent) refresh(t *testing.T) {
	t.Helper()
	if err := publishRecord(a.root, "presence.json", presenceRecord{1, agentIdentity{ID: a.id, SessionID: "session-" + a.id, Name: "peer", Cwd: "/workspace", State: "idle"}}); err != nil {
		t.Fatal(err)
	}
}
func (a simulatedAgent) request(t *testing.T, req agentRequest) string {
	t.Helper()
	if req.ID == "" {
		req.ID = randomID()
	}
	req.Version = 1
	root, err := a.root.OpenRoot("requests")
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := publishRecord(root, req.ID+".json", req); err != nil {
		t.Fatal(err)
	}
	return req.ID
}
func (a simulatedAgent) response(t *testing.T, id string) agentResponse {
	t.Helper()
	var response agentResponse
	waitFor(t, func() bool { return readRecord(a.root, "responses/"+id+".json", &response) == nil })
	return response
}
func (a simulatedAgent) incoming(t *testing.T) peerMessage {
	t.Helper()
	var message peerMessage
	waitFor(t, func() bool {
		var index inboxIndex
		if readRecord(a.root, "inbox-index.json", &index) != nil || index.Version != 1 || index.To != a.id {
			return false
		}
		for _, id := range index.Messages {
			if readRecord(a.root, "inbox/"+id+".json", &message) == nil {
				return true
			}
		}
		return false
	})
	return message
}
func (a simulatedAgent) acknowledge(t *testing.T, m peerMessage) {
	t.Helper()
	root, err := a.root.OpenRoot("receipts")
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := publishRecord(root, m.ID+".json", receiptRecord{1, m.ID, a.id, "queued"}); err != nil {
		t.Fatal(err)
	}
}
func messagingMailbox(t *testing.T, p processBridge, socket, pane string) string {
	t.Helper()
	mailbox, _ := fixture(t)
	mailbox, err := filepath.Abs(mailbox)
	if err != nil {
		t.Fatal(err)
	}
	registerProcess(t, p, mailbox, socket, pane, os.Getpid())
	return mailbox
}
func awaitPeers(t *testing.T, a simulatedAgent, count int) {
	t.Helper()
	waitFor(t, func() bool {
		a.refresh(t)
		id := a.request(t, agentRequest{Op: "list"})
		response := a.response(t, id)
		return response.Self != nil && response.Self.ID == a.id && len(response.Peers) == count
	})
}
func TestMessagingRoutesAcrossRootsAndWithinOneMailboxWithoutHerd(t *testing.T) {
	p := processFixture(t)
	p.cli("ensure")
	mailbox := messagingMailbox(t, p, "", "")
	a := newSimulatedAgent(t, mailbox)
	b := newSimulatedAgent(t, mailbox)
	// A dead Herd adapter must not prevent messaging through that registration.
	deadHerd := messagingMailbox(t, p, filepath.Join(p.dir, "unavailable.sock"), "dead-herd")
	queue(t, deadHerd, 1, `{"version":1,"kind":"state","state":"working"}`)
	c := newSimulatedAgent(t, deadHerd)
	awaitPeers(t, a, 2)
	for _, recipient := range []simulatedAgent{b, c} {
		requestID := a.request(t, agentRequest{Op: "send", To: recipient.id, Text: "/not-a-command"})
		incoming := recipient.incoming(t)
		if incoming.From.ID != a.id || incoming.To != recipient.id || incoming.Text != "/not-a-command" {
			t.Fatalf("wrong message: %+v", incoming)
		}
		var response agentResponse
		time.Sleep(150 * time.Millisecond)
		if readRecord(a.root, "responses/"+requestID+".json", &response) == nil {
			t.Fatal("send succeeded before recipient acknowledgment")
		}
		recipient.acknowledge(t, incoming)
		response = a.response(t, requestID)
		if response.Status != "queued" || response.MessageID != incoming.ID {
			t.Fatalf("wrong receipt: %+v", response)
		}
		// Replaying a request must return the old result, not deliver again.
		a.request(t, agentRequest{ID: requestID, Op: "send", To: recipient.id, Text: "duplicate"})
		time.Sleep(200 * time.Millisecond)
		entries, _ := os.ReadDir(filepath.Join(recipient.dir, "inbox"))
		if len(entries) != 0 {
			t.Fatal("duplicate request delivered twice")
		}
		waitFor(t, func() bool {
			var index inboxIndex
			return readRecord(recipient.root, "inbox-index.json", &index) == nil && len(index.Messages) == 0
		})
		replyID := recipient.request(t, agentRequest{Op: "send", To: incoming.From.ID, Text: "reply", InReplyTo: incoming.ID})
		reply := a.incoming(t)
		if reply.InReplyTo != incoming.ID || reply.From.ID != recipient.id {
			t.Fatalf("wrong reply: %+v", reply)
		}
		a.acknowledge(t, reply)
		if recipient.response(t, replyID).Status != "queued" {
			t.Fatal("reply not acknowledged")
		}
	}
}
func TestMessagingExpiresAddressesAndRejectsUnsafeRecords(t *testing.T) {
	p := processFixture(t)
	p.cli("ensure")
	mailbox := messagingMailbox(t, p, "", "")
	a := newSimulatedAgent(t, mailbox)
	b := newSimulatedAgent(t, mailbox)
	awaitPeers(t, a, 1)
	for _, kind := range []string{"malformed", "symlink", "fifo", "oversized"} {
		id := randomID()
		name := filepath.Join(a.dir, "requests", id+".json")
		switch kind {
		case "malformed":
			os.WriteFile(name, []byte(`{"version":1,"sender":"forged"}`), 0600)
		case "symlink":
			os.Symlink(filepath.Join(b.dir, "presence.json"), name)
		case "fifo":
			if err := syscall.Mkfifo(name, 0600); err != nil {
				t.Fatal(err)
			}
		case "oversized":
			os.WriteFile(name, []byte(strings.Repeat("x", maxRecordBytes+1)), 0600)
		}
		if response := a.response(t, id); response.Status != "failed" {
			t.Fatalf("accepted %s: %+v", kind, response)
		}
	}
	id := a.request(t, agentRequest{Op: "send", To: b.id, Text: strings.Repeat("x", maxTextBytes+1)})
	if a.response(t, id).Status != "failed" {
		t.Fatal("accepted oversized text")
	}
	old := time.Now().Add(-presenceTTL - time.Second)
	os.Chtimes(filepath.Join(b.dir, "presence.json"), old, old)
	awaitPeers(t, a, 0)
	b.refresh(t)
	time.Sleep(200 * time.Millisecond)
	id = a.request(t, agentRequest{Op: "send", To: b.id, Text: "stale"})
	if response := a.response(t, id); response.Status != "failed" || !strings.Contains(response.Error, "expired") {
		t.Fatalf("retired address returned: %+v", response)
	}
}
func TestMessagingDuplicateAddressesAndLaunchRemovalDoNotAffectSurvivors(t *testing.T) {
	p := processFixture(t)
	p.cli("ensure")
	mailboxA := messagingMailbox(t, p, "", "")
	mailboxB, _ := fixture(t)
	mailboxB, _ = filepath.Abs(mailboxB)
	registrationB := registerProcess(t, p, mailboxB, "", "", os.Getpid())
	a := newSimulatedAgent(t, mailboxA)
	b := newSimulatedAgent(t, mailboxB)
	awaitPeers(t, a, 1)
	// Another registered mailbox cannot claim an already live address.
	cloneDir := filepath.Join(mailboxB, "agents", a.id)
	for _, sub := range []string{"requests", "responses", "inbox", "receipts"} {
		if err := os.MkdirAll(filepath.Join(cloneDir, sub), 0700); err != nil {
			t.Fatal(err)
		}
	}
	cloneRoot, err := os.OpenRoot(cloneDir)
	if err != nil {
		t.Fatal(err)
	}
	defer cloneRoot.Close()
	clone := simulatedAgent{a.id, cloneDir, cloneRoot}
	clone.refresh(t)
	awaitPeers(t, b, 1)
	requestID := b.request(t, agentRequest{Op: "send", To: a.id, Text: "original address"})
	incoming := a.incoming(t)
	a.acknowledge(t, incoming)
	if response := b.response(t, requestID); response.Status != "queued" {
		t.Fatalf("live address stolen: %+v", response)
	}
	entries, _ := os.ReadDir(filepath.Join(cloneDir, "inbox"))
	if len(entries) != 0 {
		t.Fatal("duplicate live ID received the message")
	}

	pending := a.request(t, agentRequest{Op: "send", To: b.id, Text: "disappearing recipient"})
	b.incoming(t)
	p.cli("unregister", "--id", registrationB)
	if response := a.response(t, pending); response.Status != "indeterminate" {
		t.Fatalf("removal concealed possible insertion: %+v", response)
	}
	awaitPeers(t, a, 0)
	c := newSimulatedAgent(t, mailboxA)
	awaitPeers(t, a, 1)
	survivor := a.request(t, agentRequest{Op: "send", To: c.id, Text: "survivor"})
	message := c.incoming(t)
	c.acknowledge(t, message)
	if a.response(t, survivor).Status != "queued" {
		t.Fatal("unregister broke surviving launch")
	}
}

func TestMessagingTimeoutAndOverflowAreHonest(t *testing.T) {
	p := processFixture(t)
	p.cli("ensure")
	mailbox := messagingMailbox(t, p, "", "")
	a := newSimulatedAgent(t, mailbox)
	b := newSimulatedAgent(t, mailbox)
	awaitPeers(t, a, 1)
	ids := make([]string, maxPending)
	for i := range ids {
		ids[i] = a.request(t, agentRequest{Op: "send", To: b.id, Text: "pending"})
	}
	waitFor(t, func() bool {
		entries, _ := os.ReadDir(filepath.Join(b.dir, "inbox"))
		return len(entries) == maxPending
	})
	overflow := a.request(t, agentRequest{Op: "send", To: b.id, Text: "overflow"})
	if response := a.response(t, overflow); response.Status != "failed" || !strings.Contains(response.Error, "full") {
		t.Fatalf("overflow accepted: %+v", response)
	}
	deadline := time.Now().Add(12 * time.Second)
	var response agentResponse
	for time.Now().Before(deadline) {
		a.refresh(t)
		b.refresh(t)
		if readRecord(a.root, "responses/"+ids[0]+".json", &response) == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if response.Status != "indeterminate" || response.MessageID == "" {
		t.Fatalf("timeout reported false certainty: %+v", response)
	}
}
