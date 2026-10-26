package bridge

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLifecycleMessageAndFileSizeBoundaries(t *testing.T) {
	dir, root := fixture(t)
	for _, size := range []int{511, 512, 513} {
		t.Run(fmt.Sprintf("message-%d", size), func(t *testing.T) {
			body := fmt.Sprintf(`{"version":1,"kind":"state","state":"blocked","message":%q}`, strings.Repeat("a", size))
			name := queue(t, dir, 1, body)
			_, err := readEvent(root, name)
			if (err == nil) != (size <= 512) {
				t.Fatalf("message size %d: %v", size, err)
			}
		})
	}
	body := `{"version":1,"kind":"state","state":"idle"}`
	for _, size := range []int{maxEventBytes - 1, maxEventBytes, maxEventBytes + 1} {
		t.Run(fmt.Sprintf("file-%d", size), func(t *testing.T) {
			// Put the closing JSON token at the size limit: a short read must fail.
			name := queue(t, dir, 1, strings.Repeat(" ", size-len(body))+body)
			_, err := readEvent(root, name)
			if (err == nil) != (size <= maxEventBytes) {
				t.Fatalf("file size %d: %v", size, err)
			}
		})
	}
}

func TestReleaseEventMustNotCarryAMessage(t *testing.T) {
	dir, root := fixture(t)
	valid := queue(t, dir, 1, `{"version":1,"kind":"release"}`)
	if _, err := readEvent(root, valid); err != nil {
		t.Fatalf("rejected empty release: %v", err)
	}
	invalid := queue(t, dir, 2, `{"version":1,"kind":"release","message":"unexpected"}`)
	if _, err := readEvent(root, invalid); err == nil {
		t.Fatal("accepted a release carrying a message")
	}
}

func TestMailboxDirectoryEntryLimit(t *testing.T) {
	dir, root := fixture(t)
	if names, err := eventNames(root); err != nil || len(names) != 0 {
		t.Fatalf("empty mailbox: %v, %v", names, err)
	}
	for i := 1; i <= 4096; i++ {
		queue(t, dir, i, `{"version":1,"kind":"release"}`)
	}
	if names, err := eventNames(root); err != nil || len(names) != 4096 {
		t.Fatalf("mailbox at limit: %d entries, %v", len(names), err)
	}
	queue(t, dir, 4097, `{"version":1,"kind":"release"}`)
	if _, err := eventNames(root); err == nil {
		t.Fatal("accepted mailbox beyond entry limit")
	}
}

func TestReadyMarkerRefreshesCapabilityAndRejectsUnsafePaths(t *testing.T) {
	dir, root := fixture(t)
	if err := markReady(root); err != nil {
		t.Fatal(err)
	}
	// A heartbeat must publish the current capability even over an old marker.
	if err := os.WriteFile(filepath.Join(dir, "ready"), []byte("1"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := markReady(root); err != nil {
		t.Fatal(err)
	}
	if data, err := root.ReadFile("ready"); err != nil || string(data) != fmt.Sprint(protocolVersion) {
		t.Fatalf("ready capability not refreshed: %q, %v", data, err)
	}
	if err := os.Remove(filepath.Join(dir, "ready")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(dir, "ready")); err != nil {
		t.Fatal(err)
	}
	if err := markReady(root); err == nil {
		t.Fatal("replaced a symlink ready marker")
	}
	if err := root.Close(); err != nil {
		t.Fatal(err)
	}
	if err := markReady(root); err == nil {
		t.Fatal("ignored failure to inspect ready marker")
	}
}
