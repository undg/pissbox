package bridge

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func handoffMailbox(t *testing.T) (string, string) {
	t.Helper()
	mailbox, root := fixture(t)
	mailbox, err := filepath.Abs(mailbox)
	if err != nil {
		t.Fatal(err)
	}
	id := randomID()
	if err := publishRecord(root, "generation", id); err != nil {
		t.Fatal(err)
	}
	if err := markReady(root); err != nil {
		t.Fatal(err)
	}
	return mailbox, id
}

func readGeneration(t *testing.T, mailbox string) string {
	t.Helper()
	var generation string
	data, err := os.ReadFile(filepath.Join(mailbox, "generation"))
	if err != nil || json.Unmarshal(data, &generation) != nil {
		t.Fatalf("cannot read generation: %v", err)
	}
	return generation
}

func TestReloadPreservesRegistrationsHerdBindingsAndInbox(t *testing.T) {
	p := processFixture(t)
	p.cli("ensure")
	old := p.status()
	socket := filepath.Join(p.dir, "herd.sock")
	requests := herdFixture(t, socket)
	mailboxA, _ := fixture(t)
	mailboxA, _ = filepath.Abs(mailboxA)
	mailboxB, _ := fixture(t)
	mailboxB, _ = filepath.Abs(mailboxB)
	idA := registerProcess(t, p, mailboxA, socket, "pane-a", os.Getpid())
	idB := registerProcess(t, p, mailboxB, socket, "pane-b", os.Getpid())
	generationA := readGeneration(t, mailboxA)
	generationB := readGeneration(t, mailboxB)
	agent := newSimulatedAgent(t, mailboxA)
	message := peerMessage{Version: 1, ID: randomID(), To: agent.id, From: agentIdentity{ID: randomID(), SessionID: "sender", Name: "sender", AgentName: "Sender", Cwd: "/workspace", State: "idle"}, Text: "published before reload", Expires: time.Now().Add(time.Minute).UnixMilli()}
	if err := publishRecord(agent.root, "inbox/"+message.ID+".json", message); err != nil {
		t.Fatal(err)
	}

	p.cli("reload")
	after := p.status()
	if after.PID == old.PID || after.Registrations != 2 {
		t.Fatalf("reload did not replace the daemon cleanly: before=%+v after=%+v", old, after)
	}
	if readGeneration(t, mailboxA) != generationA || readGeneration(t, mailboxB) != generationB {
		t.Fatal("reload rotated a generation")
	}
	for _, mailbox := range []string{mailboxA, mailboxB} {
		if _, err := os.Stat(filepath.Join(mailbox, "ready")); err != nil {
			t.Fatalf("readiness was removed during reload: %v", err)
		}
	}
	for _, registration := range []struct{ mailbox, socket, pane, id string }{
		{mailboxA, socket, "pane-a", idA},
		{mailboxB, socket, "pane-b", idB},
	} {
		response, err := controlCall(p.dir, controlRequest{Op: "reconnect", Mailbox: registration.mailbox, Socket: registration.socket, Pane: registration.pane, Owner: os.Getpid()})
		if err != nil || response.ID != registration.id {
			t.Fatalf("registration changed across reload: %+v, %v", response, err)
		}
	}
	// No release on reload: the next Herd request must be the fresh report.
	queue(t, mailboxA, 1, `{"version":1,"kind":"state","state":"working"}`)
	if r := receive(t, requests); r.Method != "pane.report_agent" || r.Params["pane_id"] != "pane-a" {
		t.Fatalf("post-reload event was not forwarded first: %+v", r)
	}
	select {
	case r := <-requests:
		t.Fatalf("reload released a Herd agent: %+v", r)
	case <-time.After(150 * time.Millisecond):
	}
	waitFor(t, func() bool {
		var index inboxIndex
		return readRecord(agent.root, "inbox-index.json", &index) == nil && len(index.Messages) == 1 && index.Messages[0] == message.ID
	})
	if _, err := os.Stat(handoffPath(p.dir)); !os.IsNotExist(err) {
		t.Fatal("reload handoff survived the commit point")
	}
}

func TestAdoptionSkipsUnsafeOrDuplicateHandoffEntries(t *testing.T) {
	p := processFixture(t)
	validMailbox, validID := handoffMailbox(t)
	duplicateMailbox, duplicateID := handoffMailbox(t)
	deadMailbox, deadID := handoffMailbox(t)
	missingReadyMailbox, missingReadyID := handoffMailbox(t)
	os.Remove(filepath.Join(missingReadyMailbox, "ready"))
	mismatchMailbox, mismatchID := handoffMailbox(t)
	if err := publishRecordRoot(t, mismatchMailbox, "generation", randomID()); err != nil {
		t.Fatal(err)
	}
	retiredMailbox, retiredID := handoffMailbox(t)
	if err := publishRecordRoot(t, retiredMailbox, "retired", retiredRecord{Version: 1, ID: retiredID}); err != nil {
		t.Fatal(err)
	}
	records := []handoffRegistration{
		{ID: validID, Mailbox: validMailbox, Owner: os.Getpid()},
		{ID: validID, Mailbox: validMailbox, Owner: os.Getpid()},
		{ID: duplicateID, Mailbox: duplicateMailbox, Owner: os.Getpid()},
		{ID: duplicateID, Mailbox: validMailbox, Owner: os.Getpid()},
		{ID: deadID, Mailbox: deadMailbox, Owner: 999999},
		{ID: missingReadyID, Mailbox: missingReadyMailbox, Owner: os.Getpid()},
		{ID: mismatchID, Mailbox: mismatchMailbox, Owner: os.Getpid()},
		{ID: retiredID, Mailbox: retiredMailbox, Owner: os.Getpid()},
		{ID: "not-an-id", Mailbox: validMailbox, Owner: os.Getpid()},
		{ID: randomID(), Mailbox: filepath.Join(p.dir, "missing"), Owner: os.Getpid()},
	}
	if err := writeHandoff(p.dir, records); err != nil {
		t.Fatal(err)
	}
	p.cli("ensure")
	if status := p.status(); status.Registrations != 2 {
		t.Fatalf("adopted unsafe registrations: %+v", status)
	}
	if readGeneration(t, validMailbox) != validID || readGeneration(t, duplicateMailbox) != duplicateID {
		t.Fatal("adoption rewrote a generation")
	}
	for _, expected := range []struct{ mailbox, id string }{{validMailbox, validID}, {duplicateMailbox, duplicateID}} {
		response, err := controlCall(p.dir, controlRequest{Op: "reconnect", Mailbox: expected.mailbox, Owner: os.Getpid()})
		if err != nil || response.ID != expected.id {
			t.Fatalf("valid registration was not adopted: %+v, %v", response, err)
		}
	}
	if _, err := os.Stat(handoffPath(p.dir)); !os.IsNotExist(err) {
		t.Fatal("handoff was not consumed")
	}
}

func publishRecordRoot(t *testing.T, mailbox, name string, value any) error {
	t.Helper()
	root, err := os.OpenRoot(mailbox)
	if err != nil {
		return err
	}
	defer root.Close()
	return publishRecord(root, name, value)
}

func TestHandoffSurvivesAFailedReplacementBeforeCommit(t *testing.T) {
	p := processFixture(t)
	mailbox, id := handoffMailbox(t)
	if err := writeHandoff(p.dir, []handoffRegistration{{ID: id, Mailbox: mailbox, Owner: os.Getpid()}}); err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(p.dir, "control.sock")
	if err := os.WriteFile(socket, []byte("not a socket"), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := p.command("ensure").CombinedOutput(); err == nil {
		t.Fatalf("replacement started past a non-socket control path: %s", out)
	}
	if _, err := os.Stat(handoffPath(p.dir)); err != nil {
		t.Fatal("failed replacement consumed the handoff")
	}
	if err := os.Remove(socket); err != nil {
		t.Fatal(err)
	}
	p.cli("ensure")
	if status := p.status(); status.Registrations != 1 || readGeneration(t, mailbox) != id {
		t.Fatalf("next ensure did not adopt the handoff: %+v", status)
	}
}

func TestReloadLeavesHandoffForNextEnsureAndRetiredEntriesStayDead(t *testing.T) {
	p := processFixture(t)
	p.cli("ensure")
	mailboxA, _ := fixture(t)
	mailboxA, _ = filepath.Abs(mailboxA)
	mailboxB, _ := fixture(t)
	mailboxB, _ = filepath.Abs(mailboxB)
	registerProcess(t, p, mailboxA, "", "", os.Getpid())
	registerProcess(t, p, mailboxB, "", "", os.Getpid())
	generationB := readGeneration(t, mailboxB)
	old := p.status()
	if _, err := controlCall(p.dir, controlRequest{Op: "reload"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return lockReleased(p.dir) })
	if _, err := os.Stat(handoffPath(p.dir)); err != nil {
		t.Fatal("reload did not leave a handoff before a replacement started")
	}
	// Simulate a launcher-side retirement while the replacement is not running.
	if err := publishRecordRoot(t, mailboxA, "retired", retiredRecord{Version: 1, ID: readGeneration(t, mailboxA)}); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(mailboxA, "ready"))
	p.cli("ensure")
	if status := p.status(); status.PID == old.PID || status.Registrations != 1 {
		t.Fatalf("retired entry was resurrected: %+v", status)
	}
	if readGeneration(t, mailboxB) != generationB {
		t.Fatal("surviving registration was rotated")
	}
	if _, err := os.Stat(filepath.Join(mailboxA, "ready")); !os.IsNotExist(err) {
		t.Fatal("retired entry received readiness")
	}
}

func TestExplicitStopClearsAHandoff(t *testing.T) {
	p := processFixture(t)
	p.cli("ensure")
	mailbox, id := handoffMailbox(t)
	if err := writeHandoff(p.dir, []handoffRegistration{{ID: id, Mailbox: mailbox, Owner: os.Getpid()}}); err != nil {
		t.Fatal(err)
	}
	p.cli("stop")
	if _, err := os.Stat(handoffPath(p.dir)); !os.IsNotExist(err) {
		t.Fatal("explicit stop retained a reload snapshot")
	}
}

func TestReloadRejectsAnOldDaemonWithoutFallingBackToStop(t *testing.T) {
	p := processFixture(t)
	listener, err := net.Listen("unix", filepath.Join(p.dir, "control.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			var req controlRequest
			json.NewDecoder(conn).Decode(&req)
			json.NewEncoder(conn).Encode(controlResponse{Version: protocolVersion, PID: os.Getpid(), Error: "unknown control operation"})
			conn.Close()
		}
	}()
	out, err := p.command("reload").CombinedOutput()
	if err == nil || !strings.Contains(string(out), "--bridge-stop") {
		t.Fatalf("reload did not explain the cold upgrade: %v: %s", err, out)
	}
}

func TestUnregisterWritesARetirementMarkerAndRemovesReadiness(t *testing.T) {
	p := processFixture(t)
	p.cli("ensure")
	mailbox, _ := fixture(t)
	mailbox, _ = filepath.Abs(mailbox)
	id := registerProcess(t, p, mailbox, "", "", os.Getpid())
	p.cli("unregister", "--id", id)
	root, err := os.OpenRoot(mailbox)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	var marker retiredRecord
	if err := readRecord(root, "retired", &marker); err != nil || marker.Version != 1 || marker.ID != id {
		t.Fatalf("unregister did not retire the registration: %+v, %v", marker, err)
	}
	if _, err := root.Stat("ready"); !os.IsNotExist(err) {
		t.Fatal("unregister left readiness behind")
	}
}

func TestCrashAfterTheReloadCommitRotatesGenerationsNormally(t *testing.T) {
	p := processFixture(t)
	p.cli("ensure")
	mailbox, _ := fixture(t)
	mailbox, _ = filepath.Abs(mailbox)
	id := registerProcess(t, p, mailbox, "", "", os.Getpid())
	old := p.status().PID
	p.cli("reload")
	if _, err := os.Stat(handoffPath(p.dir)); !os.IsNotExist(err) {
		t.Fatal("reload did not commit")
	}
	pid := p.status().PID
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return lockReleased(p.dir) })
	p.cli("ensure")
	if status := p.status(); status.Registrations != 0 || status.PID == old {
		t.Fatalf("crashed replacement retained registrations: %+v", status)
	}
	if next := strings.TrimSpace(string(p.cli("reconnect", "--mailbox", mailbox, "--owner-pid", fmt.Sprint(os.Getpid())))); next == id || readGeneration(t, mailbox) == id {
		t.Fatal("post-commit crash reused a generation")
	}
}

func TestAdoptionReleasesAHerdPaneWhoseOwnerDied(t *testing.T) {
	p := processFixture(t)
	socket := filepath.Join(p.dir, "herd.sock")
	requests := herdFixture(t, socket)
	mailbox, id := handoffMailbox(t)
	if err := writeHandoff(p.dir, []handoffRegistration{{ID: id, Mailbox: mailbox, Owner: 999999, Socket: socket, Pane: "dead-pane"}}); err != nil {
		t.Fatal(err)
	}
	p.cli("ensure")
	if r := receive(t, requests); r.Method != "pane.release_agent" || r.Params["pane_id"] != "dead-pane" {
		t.Fatalf("dead owner was not released: %+v", r)
	}
	if status := p.status(); status.Registrations != 0 {
		t.Fatalf("dead owner was adopted: %+v", status)
	}
	if _, err := os.Stat(filepath.Join(mailbox, "ready")); !os.IsNotExist(err) {
		t.Fatal("dead owner retained readiness")
	}
}
