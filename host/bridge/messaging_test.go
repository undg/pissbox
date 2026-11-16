package bridge

import (
	"encoding/json"
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
	var generation string
	data, err := os.ReadFile(filepath.Join(a.dir, "..", "..", "generation"))
	if err != nil || json.Unmarshal(data, &generation) != nil {
		t.Fatalf("cannot read mailbox generation: %v", err)
	}
	if err := publishRecord(a.root, "presence.json", presenceRecord{Version: 1, Agent: agentIdentity{ID: a.id, SessionID: "session-" + a.id, Name: "peer task", AgentName: "Peer Name", Cwd: "/workspace", State: "idle"}, Generation: generation}); err != nil {
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
	listID := a.request(t, agentRequest{Op: "list"})
	listed := a.response(t, listID)
	if listed.Self.Name != "peer task" || listed.Self.AgentName != "Peer Name" ||
		len(listed.Peers) != 2 || listed.Peers[0].AgentName != "Peer Name" {
		t.Fatalf("session name and agentName were not kept separate: %+v", listed)
	}
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
func TestMessagingDeliveryModesAndRecipientCapabilities(t *testing.T) {
	p := processFixture(t)
	p.cli("ensure")
	mailbox := messagingMailbox(t, p, "", "")
	a := newSimulatedAgent(t, mailbox)
	b := newSimulatedAgent(t, mailbox)
	awaitPeers(t, a, 1)
	id := a.request(t, agentRequest{Op: "send", To: b.id, Text: "urgent", Delivery: "steer"})
	if response := a.response(t, id); response.Status != "failed" || !strings.Contains(response.Error, "does not support steering") {
		t.Fatalf("silently downgraded unsupported steering: %+v", response)
	}
	if err := publishRecord(b.root, "capabilities.json", agentCapabilities{Version: 1, Steering: true}); err != nil {
		t.Fatal(err)
	}
	for _, delivery := range []string{"", "followUp", "steer"} {
		id := a.request(t, agentRequest{Op: "send", To: b.id, Text: "coordination", Delivery: delivery})
		message := b.incoming(t)
		if message.Delivery != delivery || message.Text != "coordination" {
			t.Fatalf("delivery changed: %+v; want %q", message, delivery)
		}
		b.acknowledge(t, message)
		if response := a.response(t, id); response.Status != "queued" || response.MessageID != message.ID {
			t.Fatal(response)
		}
		// The receipt response is published before inbox/index cleanup. Wait
		// for that cleanup so the next receive cannot read this message again.
		waitFor(t, func() bool {
			var index inboxIndex
			return readRecord(b.root, "inbox-index.json", &index) == nil && len(index.Messages) == 0
		})
	}
	id = a.request(t, agentRequest{Op: "send", To: b.id, Text: "invalid", Delivery: "nextTurn"})
	if response := a.response(t, id); response.Status != "failed" || response.Error != "invalid message" {
		t.Fatalf("accepted invalid delivery: %+v", response)
	}
	entries, err := os.ReadDir(filepath.Join(b.dir, "inbox"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed sends published inbox records: %v, %v", entries, err)
	}
}

func TestMessagingKeepsUnresponsiveAgentsAndRejectsUnsafeRecords(t *testing.T) {
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
	waitFor(t, func() bool {
		response := a.response(t, a.request(t, agentRequest{Op: "list"}))
		return len(response.Peers) == 1 && response.Peers[0].ID == b.id && response.Peers[0].Availability == "unresponsive" && response.Peers[0].LastSeen == old.UnixMilli()
	})
	id = a.request(t, agentRequest{Op: "send", To: b.id, Text: "not queued"})
	if response := a.response(t, id); response.Status != "failed" || !strings.Contains(response.Error, "unresponsive") {
		t.Fatalf("sent to unresponsive agent: %+v", response)
	}
	b.refresh(t)
	waitFor(t, func() bool {
		response := a.response(t, a.request(t, agentRequest{Op: "list"}))
		return len(response.Peers) == 1 && response.Peers[0].ID == b.id && response.Peers[0].Availability == "responsive"
	})
	id = a.request(t, agentRequest{Op: "send", To: b.id, Text: "recovered"})
	incoming := b.incoming(t)
	b.acknowledge(t, incoming)
	if response := a.response(t, id); response.Status != "queued" {
		t.Fatalf("recovered address unusable: %+v", response)
	}
	// Missing or malformed presence is uncertainty, not a shutdown signal.
	os.Remove(filepath.Join(b.dir, "presence.json"))
	awaitPeers(t, a, 1)
	os.WriteFile(filepath.Join(b.dir, "presence.json"), []byte("broken"), 0600)
	awaitPeers(t, a, 1)
	if err := publishRecord(b.root, "closed.json", closedRecord{Version: 1, ID: b.id}); err != nil {
		t.Fatal(err)
	}
	awaitPeers(t, a, 0)
	b.refresh(t)
	awaitPeers(t, a, 0)
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

func TestInboxIndexSeedsExistingRecordsAndPrunesConsumedOnes(t *testing.T) {
	mailbox, mailboxRoot := fixture(t)
	id := randomID()
	if err := publishRecord(mailboxRoot, "generation", id); err != nil {
		t.Fatal(err)
	}
	a := newSimulatedAgent(t, mailbox)
	message := peerMessage{Version: 1, ID: randomID(), To: a.id, From: agentIdentity{ID: randomID(), SessionID: "sender", Name: "sender", AgentName: "Sender", Cwd: "/workspace", State: "idle"}, Text: "before reload", Expires: time.Now().Add(time.Minute).UnixMilli()}
	if err := publishRecord(a.root, "inbox/"+message.ID+".json", message); err != nil {
		t.Fatal(err)
	}
	d := daemon{regs: map[string]*registration{id: {mailbox: mailbox, generation: id}}}
	r := messageRouter{agents: make(map[string]*liveAgent), retired: make(map[string]bool), pending: make(map[string]*pendingMessage)}
	now := time.Now()
	r.discover(&d, now)
	t.Cleanup(func() {
		for _, agent := range r.agents {
			agent.root.Close()
		}
	})
	r.publishInboxIndexes(now)
	var index inboxIndex
	if err := readRecord(a.root, "inbox-index.json", &index); err != nil || len(index.Messages) != 1 || index.Messages[0] != message.ID {
		t.Fatalf("existing inbox was not seeded: %+v, %v", index, err)
	}
	if err := a.root.Remove("inbox/" + message.ID + ".json"); err != nil {
		t.Fatal(err)
	}
	r.publishInboxIndexes(now.Add(2 * time.Second))
	if err := readRecord(a.root, "inbox-index.json", &index); err != nil || len(index.Messages) != 0 {
		t.Fatalf("consumed inbox record was not pruned: %+v, %v", index, err)
	}
}
