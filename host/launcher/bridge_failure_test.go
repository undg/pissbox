package launcher

import (
	"os"
	"strings"
	"testing"
)

func TestEmptyBridgeRegistrationDoesNotExposeMailbox(t *testing.T) {
	f := newFixture(t)
	f.bridge()
	out, status, calls := f.launch("", map[string]string{"BRIDGE_EMPTY": "1", "RUN_STATUS": "31"})
	if status != 31 || kinds(calls) != "bridge-register,run" || !strings.Contains(out, "failed to start") {
		t.Fatalf("%d %v %s", status, calls, out)
	}
	if strings.Contains(strings.Join(calls[1], " "), "PISSBOX_BRIDGE_DIR=") {
		t.Fatal(calls)
	}
}
func TestBridgeMailboxFilesystemFailureIsOptional(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	f := newFixture(t)
	f.bridge()
	if err := os.Mkdir(f.state, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(f.state, 0700) })
	out, status, calls := f.launch("", map[string]string{"RUN_STATUS": "32"})
	if status != 32 || kinds(calls) != "run" || !strings.Contains(out, "cannot create host bridge mailbox") {
		t.Fatalf("%d %v %s", status, calls, out)
	}
}
