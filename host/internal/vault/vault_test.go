package vault

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func service(t *testing.T, git bool) (*Service, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := New([]Definition{{Name: "personal", Path: dir, Git: git, Obsidian: "My vault & stuff"}})
	if err != nil {
		t.Fatal(err)
	}
	return s, dir
}
func execute(t *testing.T, s *Service, op string, r Request) any {
	t.Helper()
	result, err := s.Execute(context.Background(), op, r)
	if err != nil {
		t.Fatalf("%s: %v", op, err)
	}
	return result
}
func create(t *testing.T, s *Service, name, content string) Note {
	t.Helper()
	return execute(t, s, "new", Request{Path: name, Content: content}).(Note)
}
func put(t *testing.T, dir, name, text string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}
func reject(t *testing.T, s *Service, op string, r Request) {
	t.Helper()
	if _, err := s.Execute(context.Background(), op, r); err == nil {
		t.Fatalf("accepted %s %+v", op, r)
	}
}

func TestNoteLifecyclePreservesContentAndRejectsStaleEdits(t *testing.T) {
	s, _ := service(t, false)
	n := execute(t, s, "new", Request{Path: "topic/my-note.md", Content: "Findings\n", Tags: []string{"topic/sub"}, Aliases: []string{"Alternative"}}).(Note)
	read := execute(t, s, "read", Request{Path: n.Path}).(Note)
	if !strings.Contains(read.Content, "id: my-note\n") || !strings.Contains(read.Content, "Alternative") || read.Revision != n.Revision {
		t.Fatal(read)
	}
	reject(t, s, "new", Request{Path: n.Path, Content: "overwrite"})
	reject(t, s, "save", Request{Path: "missing.md", Content: "new"})
	reject(t, s, "save", Request{Path: n.Path, Content: "oops"})
	edited := execute(t, s, "edit", Request{Path: n.Path, Revision: n.Revision, OldText: "Findings", NewText: "Verified findings"}).(Note)
	reject(t, s, "save", Request{Path: n.Path, Revision: n.Revision, Content: "stale"})
	reject(t, s, "edit", Request{Path: n.Path, Revision: edited.Revision, OldText: "missing", NewText: "x"})
	appended := execute(t, s, "append", Request{Path: n.Path, Revision: edited.Revision, Content: "\nMore findings\n"}).(Note)
	final := execute(t, s, "read", Request{Path: n.Path}).(Note)
	if !strings.HasSuffix(final.Content, "Verified findings\n\nMore findings\n") || final.Revision != appended.Revision {
		t.Fatal(final)
	}
	saved := execute(t, s, "save", Request{Path: n.Path, Revision: final.Revision, Content: "---\nid: custom\ntags: []\n---\nDone\n"}).(Note)
	if got := execute(t, s, "read", Request{Path: n.Path}).(Note); got.Content != "---\nid: custom\ntags: []\n---\nDone\n" || got.Revision != saved.Revision {
		t.Fatal(got)
	}
}

func TestSharedRevisionAllowsOnlyOneConcurrentWriter(t *testing.T) {
	s, _ := service(t, false)
	n := create(t, s, "shared.md", "before")
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, text := range []string{"first", "second"} {
		wg.Add(1)
		go func(text string) {
			defer wg.Done()
			_, err := s.Execute(context.Background(), "save", Request{Path: n.Path, Revision: n.Revision, Content: text})
			results <- err
		}(text)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("successful writers: %d", success)
	}
}

func TestVaultBoundaryRejectsTraversalSymlinksSpecialFilesAndHiddenPaths(t *testing.T) {
	s, dir := service(t, false)
	outside := t.TempDir()
	put(t, outside, "secret.md", "do not read")
	for _, name := range []string{"../secret.md", "/tmp/secret.md", ".git/config.md", ".obsidian/note.md", "a/../secret.md", "a\\secret.md", "note.txt"} {
		reject(t, s, "read", Request{Path: name})
		reject(t, s, "new", Request{Path: name, Content: "bad"})
	}
	if err := os.Symlink(outside, filepath.Join(dir, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(dir, "link.md")); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(dir, "fifo.md"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"escape/secret.md", "link.md", "fifo.md"} {
		reject(t, s, "read", Request{Path: name})
		reject(t, s, "save", Request{Path: name, Content: "bad"})
		reject(t, s, "open", Request{Path: name})
	}
	reject(t, s, "search", Request{Folder: "escape", Query: "secret"})
	reject(t, s, "list", Request{Folder: "../"})
	reject(t, s, "new", Request{Path: "Not Kebab.md", Content: "bad"})
	put(t, dir, "Legacy Name.md", "legacy")
	legacy := execute(t, s, "read", Request{Path: "Legacy Name.md"}).(Note)
	execute(t, s, "save", Request{Path: legacy.Path, Revision: legacy.Revision, Content: "existing names stay valid"})
	listing := execute(t, s, "list", Request{}).(Listing)
	if len(listing.Notes) != 1 || listing.Notes[0].Path != "Legacy Name.md" {
		t.Fatal(listing)
	}
	data, _ := os.ReadFile(filepath.Join(outside, "secret.md"))
	if string(data) != "do not read" {
		t.Fatal("escaped vault")
	}
}

func TestPagedUTF8ReadsAndLargeTargetedEdit(t *testing.T) {
	s, dir := service(t, false)
	body := strings.Repeat("🐈", 5000) + "END"
	put(t, dir, "large.md", body)
	offset := 0
	all := ""
	rev := ""
	for {
		n := execute(t, s, "read", Request{Path: "large.md", Offset: offset, Limit: 999}).(Note)
		all += n.Content
		rev = n.Revision
		if n.NextOffset == nil {
			break
		}
		if *n.NextOffset <= offset {
			t.Fatal("no progress")
		}
		offset = *n.NextOffset
	}
	if all != body {
		t.Fatal("UTF-8 pagination changed content")
	}
	reject(t, s, "read", Request{Path: "large.md", Offset: 1})
	reject(t, s, "read", Request{Path: "large.md", Limit: 1})
	reject(t, s, "save", Request{Path: "large.md", Revision: rev, Content: body})
	execute(t, s, "edit", Request{Path: "large.md", Revision: rev, OldText: "END", NewText: "DONE"})
	data, _ := os.ReadFile(filepath.Join(dir, "large.md"))
	if string(data) != strings.TrimSuffix(body, "END")+"DONE" {
		t.Fatal("targeted edit changed other content")
	}
	put(t, dir, "huge.md", strings.Repeat("x", MaxNoteBytes+1))
	reject(t, s, "read", Request{Path: "huge.md"})
	put(t, dir, "invalid.md", string([]byte{0xff}))
	reject(t, s, "read", Request{Path: "invalid.md"})
}

func TestSearchRootNotesTagsFiltersBacklinksAndPagination(t *testing.T) {
	s, dir := service(t, false)
	put(t, dir, "ideas.md", "---\ntags: [topic/child]\n---\nRoot FINDING #inline\n")
	put(t, dir, "Daily/2026-01-02.md", "---\ntags:\n  - topic\n---\nAnother finding #inline\n")
	put(t, dir, "links.md", "See [[ideas|an alias]] and [[ideas#Heading]].\n")
	put(t, dir, "other.md", "Unrelated\n")
	result := execute(t, s, "search", Request{Query: "finding", Limit: 1}).(Listing)
	if len(result.Notes) != 1 || !result.Truncated || result.NextOffset == nil {
		t.Fatal(result)
	}
	next := execute(t, s, "search", Request{Query: "finding", Offset: *result.NextOffset, Limit: 1}).(Listing)
	if len(next.Notes) != 1 || next.Truncated || next.Notes[0].Path == result.Notes[0].Path {
		t.Fatal(next)
	}
	filtered := execute(t, s, "search", Request{Query: "finding", Folder: "Daily", Tag: "topic", DateFrom: "2026-01-02", DateTo: "2026-01-02"}).(Listing)
	if len(filtered.Notes) != 1 || filtered.Notes[0].Path != "Daily/2026-01-02.md" || filtered.Notes[0].Line != 5 {
		t.Fatal(filtered)
	}
	literal := execute(t, s, "search", Request{Query: "[ideas"}).(Listing)
	if len(literal.Notes) != 1 {
		t.Fatal(literal)
	}
	regex := execute(t, s, "search", Request{Query: "FIND.NG", Regex: true}).(Listing)
	if len(regex.Notes) != 1 || regex.Notes[0].Path != "ideas.md" {
		t.Fatal(regex)
	}
	enriched := execute(t, s, "search", Request{Query: "Root FINDING", Backlinks: true}).(Listing)
	if len(enriched.Notes) != 2 {
		t.Fatal(enriched)
	}
	backlinks := execute(t, s, "backlinks", Request{Path: "ideas.md"}).(Listing)
	if len(backlinks.Notes) != 1 || backlinks.Notes[0].Path != "links.md" {
		t.Fatal(backlinks)
	}
	for _, r := range []Request{{Query: "[", Regex: true}, {DateFrom: "2026-02-31"}, {Vault: "unknown"}, {Limit: 51}, {Offset: -1}} {
		reject(t, s, "search", r)
	}
}

func TestBrowseSkipsArchiveAndRootAgentsFile(t *testing.T) {
	s, dir := service(t, false)
	put(t, dir, "AGENTS.md", "conventions FINDING #agents")
	put(t, dir, "_index.md", "index FINDING")
	put(t, dir, "live.md", "live FINDING")
	put(t, dir, "links.md", "[[AGENTS]] [[_archive/old|old]] [[live]]\n")
	put(t, dir, "_archive/old.md", "archived FINDING #old")
	put(t, dir, "_archive/source.md", "[[live]]")
	put(t, dir, "folder/live.md", "folder live FINDING")
	put(t, dir, "folder/_archive/old.md", "nested archived FINDING #nested")

	list := execute(t, s, "list", Request{}).(Listing)
	assertPaths := func(label string, result Listing, want ...string) {
		t.Helper()
		if len(result.Notes) != len(want) {
			t.Fatalf("%s: %+v", label, result)
		}
		seen := map[string]bool{}
		for _, note := range result.Notes {
			seen[note.Path] = true
		}
		for _, path := range want {
			if !seen[path] {
				t.Fatalf("%s missing %s: %+v", label, path, result)
			}
		}
	}
	assertPaths("list", list, "_index.md", "folder/live.md", "links.md", "live.md")
	assertPaths("search", execute(t, s, "search", Request{Query: "FINDING"}).(Listing), "_index.md", "folder/live.md", "live.md")
	assertPaths("archive list", execute(t, s, "list", Request{Folder: "_archive"}).(Listing), "_archive/old.md", "_archive/source.md")
	assertPaths("nested archive list", execute(t, s, "list", Request{Folder: "folder/_archive"}).(Listing), "folder/_archive/old.md")
	assertPaths("archive search", execute(t, s, "search", Request{Folder: "_archive", Query: "FINDING"}).(Listing), "_archive/old.md")

	tags := execute(t, s, "tags", Request{}).(TagListing)
	if len(tags.Tags) != 0 {
		t.Fatal(tags)
	}
	archivedTags := execute(t, s, "tags", Request{Folder: "_archive"}).(TagListing)
	if !reflect.DeepEqual(archivedTags.Tags, []TagCount{{"old", 1}}) {
		t.Fatal(archivedTags)
	}

	assertPaths("backlinks to conventions", execute(t, s, "backlinks", Request{Path: "AGENTS.md"}).(Listing), "links.md")
	assertPaths("backlinks to archived", execute(t, s, "backlinks", Request{Path: "_archive/old.md"}).(Listing), "links.md")
	assertPaths("backlinks skip archived sources", execute(t, s, "backlinks", Request{Path: "live.md"}).(Listing), "links.md")

	agents := execute(t, s, "read", Request{Path: "AGENTS.md"}).(Note)
	execute(t, s, "save", Request{Path: "AGENTS.md", Revision: agents.Revision, Content: "updated"})
	archived := execute(t, s, "read", Request{Path: "_archive/old.md"}).(Note)
	execute(t, s, "save", Request{Path: "_archive/old.md", Revision: archived.Revision, Content: "updated archived"})
}

func TestFolderListingKindsFiltersAndPagination(t *testing.T) {
	s, dir := service(t, false)
	put(t, dir, "topic/note.md", "---\ntags: [only]\n---\nbody")
	put(t, dir, "topic/2026-01-02.md", "---\ntags: [only]\n---\ntagged")
	put(t, dir, "other/unrelated.md", "unrelated")
	put(t, dir, "links.md", "[[topic/note]]")
	put(t, dir, "_archive/old.md", "archived")
	for _, folder := range []string{"topic/empty", "other/child", "_archive/retired", ".hidden"} {
		if err := os.MkdirAll(filepath.Join(dir, filepath.FromSlash(folder)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(dir, "topic"), filepath.Join(dir, "alias")); err != nil {
		t.Fatal(err)
	}
	assertEntries := func(label string, result Listing, want ...string) {
		t.Helper()
		got := []string{}
		for _, entry := range result.Notes {
			got = append(got, entry.Kind+" "+entry.Path)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: got %v; want %v", label, got, want)
		}
	}
	assertEntries("default", execute(t, s, "list", Request{}).(Listing), "note links.md", "note other/unrelated.md", "note topic/2026-01-02.md", "note topic/note.md")
	assertEntries("folders", execute(t, s, "list", Request{Kind: "folders"}).(Listing), "folder other", "folder other/child", "folder topic", "folder topic/empty")
	assertEntries("all", execute(t, s, "list", Request{Kind: "all"}).(Listing), "note links.md", "folder other", "folder other/child", "note other/unrelated.md", "folder topic", "note topic/2026-01-02.md", "folder topic/empty", "note topic/note.md")
	assertEntries("nested root excluded", execute(t, s, "list", Request{Folder: "topic", Kind: "all"}).(Listing), "note topic/2026-01-02.md", "folder topic/empty", "note topic/note.md")
	assertEntries("archive folders", execute(t, s, "list", Request{Folder: "_archive", Kind: "folders"}).(Listing), "folder _archive/retired")
	assertEntries("archive all", execute(t, s, "list", Request{Folder: "_archive", Kind: "all"}).(Listing), "note _archive/old.md", "folder _archive/retired")
	assertEntries("pattern folders", execute(t, s, "list", Request{Kind: "folders", Pattern: "topic/*"}).(Listing), "folder topic/empty")
	assertEntries("tag notes only", execute(t, s, "list", Request{Kind: "all", Tag: "only"}).(Listing), "folder other", "folder other/child", "folder topic", "note topic/2026-01-02.md", "folder topic/empty", "note topic/note.md")
	assertEntries("date notes only", execute(t, s, "list", Request{Kind: "all", DateFrom: "2026-01-02", DateTo: "2026-01-02"}).(Listing), "folder other", "folder other/child", "folder topic", "note topic/2026-01-02.md", "folder topic/empty")

	first := execute(t, s, "list", Request{Kind: "folders", Limit: 2}).(Listing)
	assertEntries("pagination first", first, "folder other", "folder other/child")
	if !first.Truncated || first.NextOffset == nil {
		t.Fatal(first)
	}
	assertEntries("pagination second", execute(t, s, "list", Request{Kind: "folders", Offset: *first.NextOffset, Limit: 2}).(Listing), "folder topic", "folder topic/empty")

	search := execute(t, s, "search", Request{Query: "tagged"}).(Listing)
	assertEntries("search kind", search, "note topic/2026-01-02.md")
	assertEntries("backlink kind", execute(t, s, "backlinks", Request{Path: "topic/note.md"}).(Listing), "note links.md")
	reject(t, s, "list", Request{Kind: "files"})
}

func TestMovePreservesNoteAndCreatesParents(t *testing.T) {
	s, dir := service(t, false)
	source := "Legacy Source.md"
	put(t, dir, source, "---\nid: legacy\naliases: [Old]\ntags: [topic]\n---\nBody\n")
	if err := os.Chmod(filepath.Join(dir, source), 0640); err != nil {
		t.Fatal(err)
	}
	result := execute(t, s, "move", Request{OldPath: source, NewPath: "New Folder/Target Name.md"}).(Move)
	if result.Vault != "personal" || result.OldPath != source || result.NewPath != "New Folder/Target Name.md" || !result.BacklinksComplete || result.Warning != "" {
		t.Fatal(result)
	}
	if _, err := os.Stat(filepath.Join(dir, source)); !os.IsNotExist(err) {
		t.Fatal("source still exists", err)
	}
	destination := filepath.Join(dir, "New Folder", "Target Name.md")
	data, err := os.ReadFile(destination)
	if err != nil || string(data) != "---\nid: legacy\naliases: [Old]\ntags: [topic]\n---\nBody\n" {
		t.Fatal(string(data), err)
	}
	info, err := os.Stat(destination)
	if err != nil || info.Mode().Perm() != 0640 {
		t.Fatal(info, err)
	}
	read := execute(t, s, "read", Request{Path: result.NewPath}).(Note)
	if read.Revision != result.Revision || read.Content != string(data) {
		t.Fatal(read)
	}
	archived := execute(t, s, "move", Request{OldPath: result.NewPath, NewPath: "Archive/_archive/Target Name.md"}).(Move)
	if archived.NewPath != "Archive/_archive/Target Name.md" || archived.Warning != "" {
		t.Fatal(archived)
	}
	restored := execute(t, s, "move", Request{OldPath: archived.NewPath, NewPath: "archive-restored.md"}).(Move)
	if restored.NewPath != "archive-restored.md" || restored.Warning != "" {
		t.Fatal(restored)
	}
}

func TestMoveRefusesExistingMissingUnsafeAndUnboundedTargets(t *testing.T) {
	s, dir := service(t, false)
	outside := t.TempDir()
	put(t, outside, "secret.md", "outside")
	put(t, dir, "source.md", "source")
	put(t, dir, "exists.md", "destination")
	put(t, dir, "huge.md", strings.Repeat("x", MaxNoteBytes+1))
	put(t, dir, "invalid.md", string([]byte{0xff}))
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(dir, "symlink.md")); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(dir, "fifo.md"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, request := range []Request{
		{OldPath: "source.md", NewPath: "exists.md"},
		{OldPath: "missing.md", NewPath: "new.md"},
		{OldPath: "source.md", NewPath: "source.md"},
		{OldPath: "../source.md", NewPath: "new.md"},
		{OldPath: "source.md", NewPath: "../new.md"},
		{OldPath: "source.md", NewPath: ".hidden/new.md"},
		{OldPath: "source.md", NewPath: "new.txt"},
		{OldPath: "symlink.md", NewPath: "new.md"},
		{OldPath: "fifo.md", NewPath: "new.md"},
		{OldPath: "huge.md", NewPath: "new.md"},
		{OldPath: "invalid.md", NewPath: "new.md"},
	} {
		reject(t, s, "move", request)
	}
	for _, name := range []string{"source.md", "exists.md", "huge.md", "invalid.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatal("refused move changed", name, err)
		}
	}
	if data, _ := os.ReadFile(filepath.Join(outside, "secret.md")); string(data) != "outside" {
		t.Fatal("followed symlink")
	}
	if _, err := os.Stat(filepath.Join(dir, "new.md")); !os.IsNotExist(err) {
		t.Fatal("refused move published a note", err)
	}
}

func TestMoveReportsBacklinksWithoutRewritingThem(t *testing.T) {
	s, dir := service(t, false)
	put(t, dir, "Area/Old.md", "target body")
	put(t, dir, "links/path.md", "[[Area/Old]]\n")
	put(t, dir, "links/base.md", "[[Old|alias]]\n")
	put(t, dir, "_archive/retired.md", "[[Old]]\n")
	// A stale folder filter must not narrow the pre-move scan.
	result := execute(t, s, "move", Request{OldPath: "Area/Old.md", NewPath: "New Folder/Old.md", Folder: "Area"}).(Move)
	if !result.BacklinksComplete || !reflect.DeepEqual(result.Backlinks, []string{"links/base.md", "links/path.md"}) {
		t.Fatal(result)
	}
	for name, want := range map[string]string{"links/path.md": "[[Area/Old]]\n", "links/base.md": "[[Old|alias]]\n"} {
		if data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name))); err != nil || string(data) != want {
			t.Fatal(name, string(data), err)
		}
	}
	backlinks := execute(t, s, "backlinks", Request{Path: result.NewPath}).(Listing)
	if len(backlinks.Notes) != 1 || backlinks.Notes[0].Path != "links/base.md" || backlinks.Notes[0].Kind != "note" {
		t.Fatal(backlinks)
	}
}

func TestMoveBacklinkCapAndScanFailureAreExplicit(t *testing.T) {
	s, dir := service(t, false)
	put(t, dir, "target.md", "target")
	for i := range 21 {
		put(t, dir, fmt.Sprintf("links/%02d.md", i), "[[target]]")
	}
	capped := execute(t, s, "move", Request{OldPath: "target.md", NewPath: "moved.md"}).(Move)
	if len(capped.Backlinks) != 20 || capped.BacklinksComplete || capped.Warning != "Backlink list stopped at 20; it is partial." {
		t.Fatal(capped)
	}
	if capped.Revision == "" {
		t.Fatal("missing revision", capped)
	}

	s, dir = service(t, false)
	put(t, dir, "target.md", "target")
	put(t, dir, "bad.md", string([]byte{0xff}))
	failed := execute(t, s, "move", Request{OldPath: "target.md", NewPath: "moved.md"}).(Move)
	if failed.BacklinksComplete || failed.Warning != "Backlink scan failed; the list may be partial." || failed.Revision == "" {
		t.Fatal(failed)
	}
	if _, err := os.Stat(filepath.Join(dir, "moved.md")); err != nil {
		t.Fatal("scan failure blocked the move", err)
	}
}

// Execute refuses an already-dead context, so reach move directly to prove the
// publish step rechecks the job context after the scan.
func TestMoveRefusesWhenJobContextExpiresBeforePublish(t *testing.T) {
	s, dir := service(t, false)
	put(t, dir, "source.md", "body")
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	_, err = s.move(ctx, root, Definition{Name: "personal", Path: dir}, Request{OldPath: "source.md", NewPath: "moved.md"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("expected the job context to stop the move", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "source.md")); err != nil {
		t.Fatal("source lost", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "moved.md")); !os.IsNotExist(err) {
		t.Fatal("expired job context published a note", err)
	}
}

func TestMoveRefusesUninspectableAndSymlinkedDestinationParents(t *testing.T) {
	s, dir := service(t, false)
	put(t, dir, "source.md", "source")
	put(t, dir, "real/keep.md", "keep")
	if err := os.Symlink("real", filepath.Join(dir, "alias")); err != nil {
		t.Fatal(err)
	}
	for _, request := range []Request{
		{OldPath: "source.md", NewPath: "source.md/child.md"},
		{OldPath: "source.md", NewPath: "alias/new.md"},
	} {
		reject(t, s, "move", request)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "source.md")); err != nil || string(data) != "source" {
		t.Fatal(string(data), err)
	}
	if _, err := os.Stat(filepath.Join(dir, "real", "new.md")); !os.IsNotExist(err) {
		t.Fatal("refused move wrote through a symlink", err)
	}
}

func TestMoveWarnsWhenHistoryFailsAfterPublishing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s, dir := service(t, true)
	git(t, dir, "init")
	git(t, dir, "config", "user.name", "Vault Test")
	git(t, dir, "config", "user.email", "vault@example.invalid")
	create(t, s, "project/old-note.md", "tracked")
	lock := filepath.Join(dir, ".git", "index.lock")
	if err := os.WriteFile(lock, nil, 0600); err != nil {
		t.Fatal(err)
	}
	moved := execute(t, s, "move", Request{OldPath: "project/old-note.md", NewPath: "project/new-note.md"}).(Move)
	if moved.Revision == "" || !strings.Contains(moved.Warning, "automatic history recording failed") || !strings.Contains(moved.Warning, "Do not repeat the move") {
		t.Fatal(moved)
	}
	if _, err := os.Stat(filepath.Join(dir, "project", "new-note.md")); err != nil {
		t.Fatal("history failure undid the move", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "project", "old-note.md")); !os.IsNotExist(err) {
		t.Fatal("old note survived", err)
	}
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
}

func TestTagChangesPreserveOtherFrontmatterAndBody(t *testing.T) {
	for _, newline := range []string{"\n", "\r\n"} {
		s, dir := service(t, false)
		text := strings.ReplaceAll("---\nid: original # keep me\naliases: [First, Second]\ntags:\n  - existing\nstatus: 'unchanged'\n---\n# Title\nBody #existing\n", "\n", newline)
		put(t, dir, "tagged.md", text)
		n := execute(t, s, "read", Request{Path: "tagged.md"}).(Note)
		changed := execute(t, s, "tag", Request{Path: n.Path, Revision: n.Revision, Tag: "new/child", Action: "add"}).(Note)
		updated := execute(t, s, "read", Request{Path: n.Path}).(Note)
		expected := strings.Replace(text, "tags:"+newline+"  - existing", "tags: [existing, new/child]", 1)
		if updated.Content != expected {
			t.Fatalf("tag changed other bytes: %q", updated.Content)
		}
		removed := execute(t, s, "tag", Request{Path: n.Path, Revision: changed.Revision, Tag: "existing", Action: "remove"}).(Note)
		final := execute(t, s, "read", Request{Path: n.Path}).(Note)
		if !strings.Contains(final.Content, "tags: [new/child]") || !strings.Contains(final.Content, "Body #existing") {
			t.Fatal(final)
		}
		reject(t, s, "tag", Request{Path: n.Path, Revision: removed.Revision, Tag: "bad tag", Action: "add"})
	}
	for _, text := range []string{"---\ntags: [one, two]\nid: original\n---\nbody", "---\nid: original\n---\nbody", "body without frontmatter"} {
		changed, err := changeTag(text, "added", "add")
		if err != nil {
			t.Fatal(err)
		}
		tags, err := tagsIn(changed)
		if err != nil || !strings.Contains(strings.Join(tags, ","), "added") {
			t.Fatal(changed, err)
		}
		same, err := changeTag(changed, "added", "add")
		if err != nil || same != changed {
			t.Fatal("non-idempotent tag add")
		}
	}
	for _, text := range []string{"---\ntags: [one]\ntags: [two]\n---\nbody", "---\ntags: [12]\n---\nbody", "---\ntags: [one]\nbody", "---\n[one, two]\n---\nbody"} {
		if _, err := changeTag(text, "x", "add"); err == nil {
			t.Fatalf("accepted malformed tags: %q", text)
		}
	}
	tags, err := tagsIn("---\ntags: [one, one, topic/sub]\n---\n#one #two #one\n")
	if err != nil || !reflect.DeepEqual(tags, []string{"one", "topic/sub", "two"}) {
		t.Fatal(tags, err)
	}
}

func TestDailyAndHostObsidianOpening(t *testing.T) {
	s, _ := service(t, false)
	today := "Daily/" + time.Now().Format("2006-01-02") + ".md"
	n := execute(t, s, "daily", Request{}).(Note)
	if n.Path != today || !strings.Contains(n.Content, time.Now().Format("2006-01-02")) || !strings.Contains(n.Content, "aliases: []") {
		t.Fatal(n)
	}
	tags, err := tagsIn(n.Content)
	if err != nil || !reflect.DeepEqual(tags, []string{"daily", "journal"}) {
		t.Fatal(tags, err)
	}
	second := execute(t, s, "daily", Request{}).(Note)
	if second.Revision != n.Revision {
		t.Fatal("daily overwritten")
	}
	var uri string
	s.Open = func(_ context.Context, value string) error { uri = value; return nil }
	execute(t, s, "open", Request{Path: today})
	if !strings.HasPrefix(uri, "obsidian://open?") || !strings.Contains(uri, "vault=My+vault+%26+stuff") || !strings.Contains(uri, "file=Daily%2F") {
		t.Fatal(uri)
	}
	s.Open = func(context.Context, string) error { return errors.New("desktop unavailable") }
	reject(t, s, "open", Request{Path: today})
	reject(t, s, "shell", Request{Path: today})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Execute(ctx, "new", Request{Path: "cancelled.md", Content: "x"}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestMultipleVaultsRequireExplicitSelectionAndDoNotMixBacklinks(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	put(t, a, "target.md", "needle")
	put(t, b, "target.md", "unrelated")
	put(t, b, "links.md", "[[target]]")
	s, err := New([]Definition{{Name: "a", Path: a}, {Name: "b", Path: b}})
	if err != nil {
		t.Fatal(err)
	}
	reject(t, s, "read", Request{Path: "target.md"})
	result := execute(t, s, "search", Request{Query: "needle", Backlinks: true}).(Listing)
	if len(result.Notes) != 1 || result.Notes[0].Vault != "a" {
		t.Fatal(result)
	}
	execute(t, s, "read", Request{Vault: "b", Path: "target.md"})
	if _, err := New([]Definition{{Name: "a", Path: a}, {Name: "duplicate", Path: a}}); err == nil {
		t.Fatal("accepted duplicate physical directory")
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	data, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s %v", args, data, err)
	}
	return strings.TrimSpace(string(data))
}
func TestAutomaticHistoryCommitsOnlyChangedNoteAndPreservesUnrelatedStaging(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s, dir := service(t, true)
	git(t, dir, "init")
	git(t, dir, "config", "user.name", "Vault Test")
	git(t, dir, "config", "user.email", "vault@example.invalid")
	create(t, s, "existing.md", "initial")
	put(t, dir, "unrelated.md", "staged work")
	git(t, dir, "add", "unrelated.md")
	put(t, dir, "unstaged.md", "unstaged work")
	// Hooks must never execute across the sandbox-to-host boundary.
	marker := filepath.Join(dir, "hook-ran")
	hook := filepath.Join(dir, ".git/hooks/pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\ntouch '"+marker+"'\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	n := create(t, s, "new-note.md", "stored")
	if n.Warning != "" {
		t.Fatal(n.Warning)
	}
	if got := git(t, dir, "log", "-1", "--format=%s"); got != "vault: new-note.md" {
		t.Fatal(got)
	}
	if got := git(t, dir, "show", "--format=", "--name-only", "--no-renames", "HEAD"); got != "new-note.md" {
		t.Fatal(got)
	}
	if got := git(t, dir, "diff", "--cached", "--name-only"); got != "unrelated.md" {
		t.Fatal("lost unrelated staging", got)
	}
	if got := git(t, dir, "status", "--porcelain"); !strings.Contains(got, "?? unstaged.md") {
		t.Fatal(got)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("executed Git hook")
	}
	count := git(t, dir, "rev-list", "--count", "HEAD")
	execute(t, s, "save", Request{Path: n.Path, Revision: n.Revision, Content: execute(t, s, "read", Request{Path: n.Path}).(Note).Content})
	if got := git(t, dir, "rev-list", "--count", "HEAD"); got != count {
		t.Fatal("created empty commit")
	}
	git(t, dir, "config", "user.email", "")
	git(t, dir, "config", "user.name", "")
	result := execute(t, s, "append", Request{Path: n.Path, Revision: n.Revision, Content: " more"}).(Note)
	if result.Warning == "" || !strings.HasSuffix(execute(t, s, "read", Request{Path: n.Path}).(Note).Content, " more") {
		t.Fatal("save failure state hidden", result)
	}
}

func TestMoveHistoryCommitsBothPathsAndSkipsUntrackedSource(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s, dir := service(t, true)
	git(t, dir, "init")
	git(t, dir, "config", "user.name", "Vault Test")
	git(t, dir, "config", "user.email", "vault@example.invalid")
	create(t, s, "project/old-note.md", "tracked")
	put(t, dir, "unrelated.md", "staged work")
	git(t, dir, "add", "unrelated.md")
	moved := execute(t, s, "move", Request{OldPath: "project/old-note.md", NewPath: "Archive/New Name.md"}).(Move)
	if moved.Warning != "" || moved.Revision == "" {
		t.Fatal(moved)
	}
	if got := git(t, dir, "log", "-1", "--format=%s"); got != "vault: move project/old-note.md -> Archive/New Name.md" {
		t.Fatal(got)
	}
	committed := strings.Split(git(t, dir, "show", "--format=", "--name-only", "--no-renames", "HEAD"), "\n")
	if !reflect.DeepEqual(committed, []string{"Archive/New Name.md", "project/old-note.md"}) {
		t.Fatal(committed)
	}
	if got := git(t, dir, "diff", "--cached", "--name-only"); got != "unrelated.md" {
		t.Fatal("lost unrelated staging", got)
	}
	if got := git(t, dir, "ls-files", "project/old-note.md"); got != "" {
		t.Fatal("old tracked path remains", got)
	}

	put(t, dir, "untracked-old.md", "untracked")
	untracked := execute(t, s, "move", Request{OldPath: "untracked-old.md", NewPath: "moved-untracked.md"}).(Move)
	if untracked.Warning != "" {
		t.Fatal(untracked)
	}
	committed = strings.Split(git(t, dir, "show", "--format=", "--name-only", "--no-renames", "HEAD"), "\n")
	if !reflect.DeepEqual(committed, []string{"moved-untracked.md"}) {
		t.Fatal(committed)
	}
	if got := git(t, dir, "diff", "--cached", "--name-only"); got != "unrelated.md" {
		t.Fatal("lost unrelated staging", got)
	}
}

func TestTagCountsAndBrowseBoundaries(t *testing.T) {
	s, dir := service(t, false)
	put(t, dir, "root.md", "---\ntags: [shared, topic/child]\n---\n#shared #inline #inline\n")
	put(t, dir, "folder/second.md", "---\ntags: shared\n---\n#inline\n")
	put(t, dir, ".hidden/secret.md", "hidden #secret")
	first := execute(t, s, "tags", Request{Limit: 1}).(TagListing)
	if !first.Truncated || first.NextOffset == nil || !reflect.DeepEqual(first.Tags, []TagCount{{"inline", 2}}) {
		t.Fatal(first)
	}
	second := execute(t, s, "tags", Request{Offset: *first.NextOffset, Limit: 2}).(TagListing)
	if second.Truncated || !reflect.DeepEqual(second.Tags, []TagCount{{"shared", 2}, {"topic/child", 1}}) {
		t.Fatal(second)
	}
	filtered := execute(t, s, "tags", Request{Folder: "folder", Tag: "shared"}).(TagListing)
	if !reflect.DeepEqual(filtered.Tags, []TagCount{{"inline", 1}, {"shared", 1}}) {
		t.Fatal(filtered)
	}
	rootOnly := execute(t, s, "list", Request{Pattern: "*.md"}).(Listing)
	if len(rootOnly.Notes) != 1 || rootOnly.Notes[0].Path != "root.md" {
		t.Fatal(rootOnly)
	}
	all := execute(t, s, "list", Request{Pattern: "**/*.md"}).(Listing)
	if len(all.Notes) != 2 {
		t.Fatal(all)
	}
	question := execute(t, s, "list", Request{Pattern: "folder/secon?.md"}).(Listing)
	if len(question.Notes) != 1 || question.Notes[0].Path != "folder/second.md" {
		t.Fatal(question)
	}
	literal := execute(t, s, "list", Request{Pattern: "[root].md"}).(Listing)
	if len(literal.Notes) != 0 {
		t.Fatal("glob metacharacters became regular expressions", literal)
	}
	listed := execute(t, s, "list", Request{Tag: "topic"}).(Listing)
	if len(listed.Notes) != 1 || listed.Notes[0].Path != "root.md" || listed.Notes[0].Size == 0 {
		t.Fatal(listed)
	}
	reject(t, s, "search", Request{DateFrom: "2026-03-01", DateTo: "2026-01-01"})
	reject(t, s, "search", Request{Query: strings.Repeat("q", 1025)})
	reject(t, s, "list", Request{Folder: "missing"})
	if shortLine(strings.Repeat("🐈", 100)) != strings.Repeat("🐈", 64)+"…" {
		t.Fatal("invalid UTF-8 snippet boundary")
	}
	put(t, dir, "malformed.md", "---\ntags: [unclosed\n---\nbody")
	reject(t, s, "tags", Request{})
}

func TestDesktopOpenerUsesHostCommandWithEncodedURI(t *testing.T) {
	command := "xdg-open"
	if runtime.GOOS == "darwin" {
		command = "open"
	} else if runtime.GOOS != "linux" {
		t.Skip("unsupported desktop platform")
	}
	bin := t.TempDir()
	capture := filepath.Join(bin, "captured")
	t.Setenv("PATH", bin)
	t.Setenv("VAULT_TEST_OPEN_CAPTURE", capture)
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$VAULT_TEST_OPEN_CAPTURE\"\n"
	if err := os.WriteFile(filepath.Join(bin, command), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	s, _ := service(t, false)
	create(t, s, "note.md", "body")
	execute(t, s, "open", Request{Path: "note.md"})
	data, err := os.ReadFile(capture)
	if err != nil || string(data) != "obsidian://open?file=note.md&vault=My+vault+%26+stuff\n" {
		t.Fatal(string(data), err)
	}
	if err := os.Remove(filepath.Join(bin, command)); err != nil {
		t.Fatal(err)
	}
	reject(t, s, "open", Request{Path: "note.md"})
}

func TestDisabledHistoryAndParentRepositoriesAreNotTouched(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s, dir := service(t, false)
	git(t, dir, "init")
	if n := create(t, s, "no-history.md", "x"); n.Warning != "" {
		t.Fatal(n)
	}
	put(t, dir, "move-me.md", "x")
	if moved := execute(t, s, "move", Request{OldPath: "move-me.md", NewPath: "moved.md"}).(Move); moved.Warning != "" {
		t.Fatal(moved)
	}
	if got := git(t, dir, "ls-files"); got != "" {
		t.Fatal(got)
	}
	child := filepath.Join(dir, "child")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	nested, err := New([]Definition{{Name: "nested", Path: child, Git: true}})
	if err != nil {
		t.Fatal(err)
	}
	create(t, nested, "nested.md", "x")
	if got := git(t, dir, "ls-files"); got != "" {
		t.Fatal("used parent repository", got)
	}
}
