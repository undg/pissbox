// Package vault manages host-only Markdown collections independently of Pi and
// the bridge. All requests share one mutation lock and revision checks.
package vault

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
)

// MaxNoteBytes bounds reads and the final size of a stored Markdown note.
const MaxNoteBytes = 2 * 1024 * 1024

// MaxContentBytes bounds each supplied body or edit fragment.
const MaxContentBytes = 16 * 1024

// Definition is trusted host configuration, never supplied by a tool call.
type Definition struct {
	Name, Path string
	Git        bool
}

// Request uses vault-relative paths. Revision is the token from a read or write.
type Request struct {
	Vault     string   `json:"vault,omitempty"`
	Path      string   `json:"path,omitempty"`
	OldPath   string   `json:"oldPath,omitempty"`
	NewPath   string   `json:"newPath,omitempty"`
	Kind      string   `json:"kind,omitempty"`
	Folder    string   `json:"folder,omitempty"`
	Query     string   `json:"query,omitempty"`
	Pattern   string   `json:"pattern,omitempty"`
	Regex     bool     `json:"regex,omitempty"`
	Tag       string   `json:"tag,omitempty"`
	DateFrom  string   `json:"dateFrom,omitempty"`
	DateTo    string   `json:"dateTo,omitempty"`
	Backlinks bool     `json:"backlinks,omitempty"`
	Content   string   `json:"content,omitempty"`
	OldText   string   `json:"oldText,omitempty"`
	NewText   string   `json:"newText,omitempty"`
	Revision  string   `json:"revision,omitempty"`
	Action    string   `json:"action,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Aliases   []string `json:"aliases,omitempty"`
	Offset    int      `json:"offset,omitempty"`
	Limit     int      `json:"limit,omitempty"`
}

// Note is also returned after writes so the caller can continue editing safely.
type Note struct {
	Vault      string `json:"vault"`
	Path       string `json:"path"`
	Revision   string `json:"revision"`
	Content    string `json:"content,omitempty"`
	NextOffset *int   `json:"nextOffset,omitempty"`
	Warning    string `json:"warning,omitempty"`
}

// Move is returned after a successful move. Backlinks is always a list and is
// complete only when the whole vault was scanned without hitting its cap.
type Move struct {
	Vault             string   `json:"vault"`
	OldPath           string   `json:"oldPath"`
	NewPath           string   `json:"newPath"`
	Revision          string   `json:"revision,omitempty"`
	Backlinks         []string `json:"backlinks"`
	BacklinksComplete bool     `json:"backlinksComplete"`
	Warning           string   `json:"warning,omitempty"`
}

// Service is shared across all agents; a host opener is injectable for tests.
type Service struct {
	mu          sync.Mutex
	definitions []Definition
	Open        func(context.Context, string) error
}

// New validates trusted definitions without creating directories or repositories.
func New(definitions []Definition) (*Service, error) {
	definitions = append([]Definition(nil), definitions...)
	seen := map[string]bool{}
	for i := range definitions {
		d := &definitions[i]
		if d.Name == "" || !filepath.IsAbs(d.Path) {
			return nil, errors.New("invalid vault definition")
		}
		canonical, err := filepath.EvalSymlinks(d.Path)
		if err != nil {
			return nil, fmt.Errorf("vault %s directory unavailable", d.Name)
		}
		info, err := os.Stat(canonical)
		if err != nil || !info.IsDir() || seen[canonical] {
			return nil, errors.New("vault directories must be distinct existing directories")
		}
		seen[canonical] = true
		d.Path = canonical
	}
	return &Service{definitions: definitions, Open: openDesktop}, nil
}

func (s *Service) selectVault(name string) (Definition, error) {
	if name == "" && len(s.definitions) == 1 {
		return s.definitions[0], nil
	}
	for _, d := range s.definitions {
		if d.Name == name {
			return d, nil
		}
	}
	return Definition{}, errors.New("select a configured vault by name (use vault_list)")
}

// Execute accepts only domain operations; there is no shell, arbitrary file, or
// history-management capability. The context bounds host subprocesses and scans.
func (s *Service) Execute(ctx context.Context, op string, req Request) (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch op {
	case "list", "search", "tags":
		return s.browse(ctx, op, req)
	}
	d, err := s.selectVault(req.Vault)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(d.Path)
	if err != nil {
		return nil, errors.New("vault unavailable")
	}
	defer root.Close()
	if op == "move" {
		return s.move(ctx, root, d, req)
	}
	if op == "daily" {
		req.Path = "Daily/" + time.Now().Format("2006-01-02") + ".md"
	}
	if err := notePath(req.Path); err != nil {
		return nil, err
	}
	if err := safeParents(root, req.Path, false); err != nil {
		return nil, err
	}
	switch op {
	case "read":
		return readNote(root, d, req)
	case "open":
		if _, err := readContent(root, req.Path); err != nil {
			return nil, err
		}
		return s.openNote(ctx, d, req.Path)
	case "backlinks":
		return s.backlinks(ctx, root, d, req)
	case "new", "save", "edit", "append", "tag", "daily":
		return s.mutate(ctx, root, d, op, req)
	default:
		return nil, errors.New("unknown vault operation")
	}
}

func notePath(name string) error {
	if !fs.ValidPath(name) || strings.ContainsAny(name, "\\:\x00") || !strings.EqualFold(path.Ext(name), ".md") || len(name) > 1024 {
		return errors.New("expected a vault-relative Markdown path")
	}
	for _, p := range strings.Split(name, "/") {
		if strings.HasPrefix(p, ".") {
			return errors.New("hidden vault paths are not accessible")
		}
	}
	return nil
}

// Root confines races as well as lexical traversal. Refuse symlinks and special
// files additionally; in particular, never block on a FIFO provided as a note.
func safeParents(root *os.Root, name string, create bool) error {
	parts := strings.Split(path.Dir(name), "/")
	if path.Dir(name) == "." {
		return nil
	}
	current := ""
	for _, part := range parts {
		current = path.Join(current, part)
		info, err := root.Lstat(current)
		if os.IsNotExist(err) {
			if !create {
				return nil
			}
			if err := root.Mkdir(current, 0700); err != nil && !os.IsExist(err) {
				return errors.New("cannot create note directory")
			}
			info, err = root.Lstat(current)
		}
		if err != nil || !info.IsDir() {
			return errors.New("note directory is not a real directory")
		}
	}
	return nil
}
func readContent(root *os.Root, name string) ([]byte, error) {
	if err := safeParents(root, name, false); err != nil {
		return nil, err
	}
	f, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fs.ErrNotExist
		}
		return nil, errors.New("cannot read note")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > MaxNoteBytes {
		return nil, errors.New("note must be a regular file of at most 2 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxNoteBytes+1))
	if err != nil || len(data) > MaxNoteBytes || !utf8.Valid(data) {
		return nil, errors.New("note must be bounded UTF-8 Markdown")
	}
	return data, nil
}
func revision(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func readNote(root *os.Root, d Definition, r Request) (Note, error) {
	data, err := readContent(root, r.Path)
	if err != nil {
		return Note{}, err
	}
	if r.Offset < 0 || r.Offset > len(data) || (r.Offset < len(data) && !utf8.RuneStart(data[r.Offset])) {
		return Note{}, errors.New("invalid byte offset")
	}
	limit := r.Limit
	if limit == 0 {
		limit = 12000
	}
	if limit < 1 || limit > 12000 {
		return Note{}, errors.New("read limit must be 1..12000 bytes")
	}
	end := min(len(data), r.Offset+limit)
	for end < len(data) && !utf8.RuneStart(data[end]) {
		end--
	}
	if end == r.Offset && end < len(data) {
		return Note{}, errors.New("read limit too small for UTF-8 character")
	}
	n := Note{Vault: d.Name, Path: r.Path, Revision: revision(data), Content: string(data[r.Offset:end])}
	if end < len(data) {
		n.NextOffset = &end
	}
	return n, nil
}

var kebab = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*\.md$`)

var errBacklinkLimit = errors.New("backlink limit reached")

// move publishes without overwriting: an exclusive hard link, then removal of
// the old name. Links are never rewritten, so the scan result is reported.
func (s *Service) move(ctx context.Context, root *os.Root, d Definition, r Request) (any, error) {
	if err := notePath(r.OldPath); err != nil {
		return nil, err
	}
	if err := notePath(r.NewPath); err != nil {
		return nil, err
	}
	if r.OldPath == r.NewPath {
		return nil, errors.New("oldPath and newPath must differ")
	}
	if _, err := readContent(root, r.OldPath); err != nil {
		return nil, err
	}
	if _, err := root.Lstat(r.NewPath); err == nil {
		return nil, errors.New("destination already exists; choose another path")
	} else if !os.IsNotExist(err) {
		return nil, errors.New("cannot inspect destination")
	}
	if err := safeParents(root, r.NewPath, true); err != nil {
		return nil, err
	}

	scanCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	backlinks, scanErr := scanBacklinks(scanCtx, root, r.OldPath)
	cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	warnings := []string{}
	if scanErr != nil {
		switch {
		case errors.Is(scanErr, errBacklinkLimit):
			warnings = append(warnings, "Backlink list stopped at 20; it is partial.")
		case errors.Is(scanErr, context.DeadlineExceeded):
			warnings = append(warnings, "Backlink scan timed out; the list may be partial.")
		default:
			warnings = append(warnings, "Backlink scan failed; the list may be partial.")
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := root.Link(r.OldPath, r.NewPath); err != nil {
		return nil, errors.New("cannot move note; destination may have been created concurrently")
	}
	if err := root.Remove(r.OldPath); err != nil {
		if rollback := root.Remove(r.NewPath); rollback != nil {
			return nil, errors.New("note move incomplete: the note now exists at both oldPath and newPath")
		}
		return nil, errors.New("cannot remove the old note; the note was not moved")
	}

	result := Move{Vault: d.Name, OldPath: r.OldPath, NewPath: r.NewPath, Backlinks: backlinks, BacklinksComplete: scanErr == nil}
	if data, err := readContent(root, r.NewPath); err != nil {
		warnings = append(warnings, "Note moved, but its new revision could not be read; reread before editing.")
	} else {
		result.Revision = revision(data)
	}
	if d.Git {
		if err := recordHistory(ctx, d, "vault: move "+r.OldPath+" -> "+r.NewPath, r.OldPath, r.NewPath); err != nil {
			warnings = append(warnings, "Note moved, but automatic history recording failed. Do not repeat the move; tell the user to check the host vault.")
		}
	}
	result.Warning = strings.Join(warnings, " ")
	return result, nil
}

func scanBacklinks(ctx context.Context, root *os.Root, target string) ([]string, error) {
	paths := []string{}
	err := walkVault(ctx, root, "", false, func(name string, _ fs.DirEntry) error {
		data, err := readContent(root, name)
		if err != nil {
			return err
		}
		if backlinkEntry("", name, string(data), target) == nil {
			return nil
		}
		if len(paths) == 20 {
			return errBacklinkLimit
		}
		paths = append(paths, name)
		return nil
	})
	return paths, err
}

func (s *Service) mutate(ctx context.Context, root *os.Root, d Definition, op string, r Request) (any, error) {
	before, err := readContent(root, r.Path)
	exists := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if op == "daily" && exists {
		return readNote(root, d, r)
	}
	if op == "new" && exists {
		return nil, errors.New("note already exists; read before editing")
	}
	if !exists && op != "new" && op != "daily" {
		return nil, errors.New("note not found; use vault_new to create")
	}
	if exists && r.Revision != revision(before) {
		return nil, errors.New("note changed or revision missing; reread and reconcile before saving")
	}
	if !exists && op != "daily" && !kebab.MatchString(path.Base(r.Path)) {
		return nil, errors.New("new filenames must be kebab-case.md")
	}
	if len(r.Content) > MaxContentBytes || len(r.OldText) > MaxContentBytes || len(r.NewText) > MaxContentBytes {
		return nil, errors.New("tool content exceeds 16 KiB; use targeted edits")
	}
	content, err := replacementContent(op, r, string(before))
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(content) > MaxNoteBytes || !utf8.ValidString(content) {
		return nil, errors.New("note exceeds 2 MiB or is not valid UTF-8")
	}
	if string(before) != content || !exists {
		if err := writeNote(root, r.Path, []byte(content), exists); err != nil {
			return nil, err
		}
	}
	n := Note{Vault: d.Name, Path: r.Path, Revision: revision([]byte(content))}
	if op == "daily" {
		n.Content = content
	}
	if d.Git {
		if err := recordHistory(ctx, d, "vault: "+r.Path, r.Path); err != nil {
			n.Warning = "Note saved, but automatic history recording failed. Do not repeat the write; tell the user to check the host vault."
		}
	}
	return n, nil
}

func replacementContent(op string, r Request, before string) (string, error) {
	switch op {
	case "new":
		return newFrontmatter(r.Path, r.Content, r.Tags, r.Aliases)
	case "daily":
		return newFrontmatter(r.Path, "# "+strings.TrimSuffix(path.Base(r.Path), ".md")+"\n", []string{"daily", "journal"}, nil)
	case "edit":
		if r.OldText == "" || strings.Count(before, r.OldText) != 1 {
			return "", errors.New("oldText must match exactly once")
		}
		return strings.Replace(before, r.OldText, r.NewText, 1), nil
	case "append":
		return before + r.Content, nil
	case "tag":
		return changeTag(before, r.Tag, r.Action)
	default:
		return r.Content, nil
	}
}

func writeNote(root *os.Root, name string, data []byte, exists bool) error {
	if err := safeParents(root, name, true); err != nil {
		return err
	}
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return err
	}
	temporary := path.Join(path.Dir(name), ".vault-"+hex.EncodeToString(token[:]))
	mode := fs.FileMode(0600)
	if exists {
		if info, err := root.Lstat(name); err == nil {
			mode = info.Mode().Perm()
		}
	}
	f, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return errors.New("cannot prepare note save")
	}
	defer root.Remove(temporary)
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return errors.New("cannot finish note save")
	}
	if exists {
		err = root.Rename(temporary, name)
	} else {
		err = root.Link(temporary, name)
	}
	if err != nil {
		return errors.New("cannot publish note (it may have been created concurrently)")
	}
	return nil
}
