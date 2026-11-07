package vault

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

type Entry struct {
	Vault    string `json:"vault"`
	Path     string `json:"path"`
	Line     int    `json:"line,omitempty"`
	Match    string `json:"match,omitempty"`
	Size     int64  `json:"size,omitempty"`
	Modified string `json:"modified,omitempty"`
}
type Listing struct {
	Vaults     []string `json:"vaults,omitempty"`
	Notes      []Entry  `json:"notes"`
	Truncated  bool     `json:"truncated"`
	NextOffset *int     `json:"nextOffset,omitempty"`
}
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}
type TagListing struct {
	Tags       []TagCount `json:"tags"`
	Truncated  bool       `json:"truncated"`
	NextOffset *int       `json:"nextOffset,omitempty"`
}

func folderValid(folder string) bool {
	if folder == "" {
		return true
	}
	if !fs.ValidPath(folder) || strings.ContainsAny(folder, "\\:\x00") {
		return false
	}
	for _, part := range strings.Split(folder, "/") {
		if strings.HasPrefix(part, ".") {
			return false
		}
	}
	return true
}
func walkNotes(ctx context.Context, root *os.Root, folder string, visit func(string, fs.DirEntry) error) error {
	if !folderValid(folder) {
		return errors.New("invalid vault-relative folder")
	}
	if folder != "" {
		if err := safeParents(root, path.Join(folder, "probe.md"), false); err != nil {
			return err
		}
	}
	start := folder
	if start == "" {
		start = "."
	}
	count := 0
	return fs.WalkDir(root.FS(), start, func(name string, entry fs.DirEntry, err error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err != nil {
			return errors.New("cannot scan vault folder")
		}
		count++
		if count > 10000 {
			return errors.New("scan limit reached; narrow the folder")
		}
		if name != "." && strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		// Retired notes and the vault's convention file are not note content.
		// An explicitly named archive folder is still browsable.
		if entry.IsDir() && entry.Name() == "_archive" && name != start {
			return fs.SkipDir
		}
		if !entry.IsDir() && name == "AGENTS.md" {
			return nil
		}
		if !entry.IsDir() && strings.EqualFold(path.Ext(name), ".md") {
			if err := notePath(name); err != nil {
				return nil
			}
			if !entry.Type().IsRegular() {
				return nil
			}
			return visit(name, entry)
		}
		return nil
	})
}
func dateAllowed(name string, r Request) bool {
	if r.DateFrom == "" && r.DateTo == "" {
		return true
	}
	date := datePattern.FindString(name)
	return date != "" && (r.DateFrom == "" || date >= r.DateFrom) && (r.DateTo == "" || date <= r.DateTo)
}

var datePattern = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)

func hasTag(content, tag string) (bool, error) {
	if tag == "" {
		return true, nil
	}
	tags, err := tagsIn(content)
	if err != nil {
		return false, err
	}
	for _, t := range tags {
		if t == tag || strings.HasPrefix(t, tag+"/") {
			return true, nil
		}
	}
	return false, nil
}
func shortLine(text string) string {
	if len(text) <= 256 {
		return text
	}
	end := 256
	for !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end] + "…"
}
func validateBrowse(r Request) error {
	if !folderValid(r.Folder) || r.Offset < 0 || r.Limit < 0 || r.Limit > 50 {
		return errors.New("invalid folder, offset, or limit (maximum 50)")
	}
	for _, date := range []string{r.DateFrom, r.DateTo} {
		if date != "" {
			if _, err := time.Parse("2006-01-02", date); err != nil {
				return errors.New("expected ISO date YYYY-MM-DD")
			}
		}
	}
	if r.DateFrom != "" && r.DateTo != "" && r.DateFrom > r.DateTo {
		return errors.New("dateFrom must not exceed dateTo")
	}
	if len(r.Query) > 1024 || len(r.Pattern) > 1024 {
		return errors.New("query or pattern too long")
	}
	return nil
}

// entryPage retains only the requested page while tracking whether more exists.
type entryPage struct {
	notes                []Entry
	total, offset, limit int
}

func newPage(r Request) *entryPage {
	limit := r.Limit
	if limit == 0 {
		limit = 30
	}
	return &entryPage{notes: []Entry{}, offset: r.Offset, limit: limit}
}
func (p *entryPage) add(e Entry) {
	if p.total >= p.offset && len(p.notes) < p.limit {
		p.notes = append(p.notes, e)
	}
	p.total++
}
func (p *entryPage) listing() Listing {
	next := nextOffset(p.offset+len(p.notes), p.total)
	return Listing{Notes: p.notes, NextOffset: next, Truncated: next != nil}
}
func nextOffset(end, total int) *int {
	if end < total {
		return &end
	}
	return nil
}
func pageTags(counts map[string]int, r Request) TagListing {
	tags := []TagCount{}
	for tag, count := range counts {
		tags = append(tags, TagCount{tag, count})
	}
	sort.Slice(tags, func(i, j int) bool { return tags[i].Tag < tags[j].Tag })
	limit := r.Limit
	if limit == 0 {
		limit = 30
	}
	start := min(r.Offset, len(tags))
	end := min(len(tags), start+limit)
	return TagListing{Tags: tags[start:end], NextOffset: nextOffset(end, len(tags)), Truncated: end < len(tags)}
}

// A scanner owns only one vault's temporary search state. Pagination and tag
// counts are shared across scanners; link targets never cross vault boundaries.
type scanner struct {
	root       *os.Root
	definition Definition
	request    Request
	op         string
	pattern    *regexp.Regexp
	page       *entryPage
	counts     map[string]int
	matches    map[string]bool
}

func (s *Service) browse(ctx context.Context, op string, r Request) (any, error) {
	if err := validateBrowse(r); err != nil {
		return nil, err
	}
	definitions := s.definitions
	if r.Vault != "" {
		d, err := s.selectVault(r.Vault)
		if err != nil {
			return nil, err
		}
		definitions = []Definition{d}
	}
	var pattern *regexp.Regexp
	if op == "search" && r.Regex {
		var err error
		pattern, err = regexp.Compile(r.Query)
		if err != nil {
			return nil, errors.New("invalid search regular expression")
		}
	}
	if op == "list" && r.Pattern != "" {
		pattern = compileGlob(r.Pattern)
	}
	page := newPage(r)
	counts := map[string]int{}
	for _, d := range definitions {
		root, err := os.OpenRoot(d.Path)
		if err != nil {
			return nil, errors.New("vault unavailable")
		}
		scan := scanner{root: root, definition: d, request: r, op: op, pattern: pattern, page: page, counts: counts, matches: map[string]bool{}}
		err = scan.run(ctx)
		root.Close()
		if err != nil {
			return nil, err
		}
	}
	if op == "tags" {
		return pageTags(counts, r), nil
	}
	result := page.listing()
	for _, d := range s.definitions {
		result.Vaults = append(result.Vaults, d.Name)
	}
	return result, nil
}
func (s *scanner) run(ctx context.Context) error {
	if err := walkNotes(ctx, s.root, s.request.Folder, s.visit); err != nil {
		return err
	}
	if s.op == "search" && s.request.Backlinks && len(s.matches) > 0 {
		return walkNotes(ctx, s.root, s.request.Folder, func(name string, _ fs.DirEntry) error { return s.includeBacklink(name) })
	}
	return nil
}
func (s *scanner) visit(name string, entry fs.DirEntry) error {
	if s.op == "list" && s.pattern != nil && !s.pattern.MatchString(name) {
		return nil
	}
	if !dateAllowed(name, s.request) {
		return nil
	}
	if s.op == "list" && s.request.Tag == "" {
		return s.list(name, entry)
	}
	data, err := readContent(s.root, name)
	if err != nil {
		return errors.New("cannot scan note in vault " + s.definition.Name + ": " + name)
	}
	text := string(data)
	allowed, err := hasTag(text, s.request.Tag)
	if err != nil || !allowed {
		return err
	}
	switch s.op {
	case "list":
		return s.list(name, entry)
	case "tags":
		tags, err := tagsIn(text)
		if err != nil {
			return err
		}
		for _, tag := range tags {
			s.counts[tag]++
		}
	case "search":
		s.search(name, text)
	}
	return nil
}
func (s *scanner) list(name string, entry fs.DirEntry) error {
	info, err := entry.Info()
	if err != nil {
		return errors.New("cannot inspect note")
	}
	s.page.add(Entry{Vault: s.definition.Name, Path: name, Size: info.Size(), Modified: info.ModTime().UTC().Format(time.RFC3339)})
	return nil
}
func (s *scanner) search(name, text string) {
	for i, line := range strings.Split(text, "\n") {
		found := strings.Contains(strings.ToLower(line), strings.ToLower(s.request.Query))
		if s.pattern != nil {
			found = s.pattern.MatchString(line)
		}
		if found {
			s.page.add(Entry{Vault: s.definition.Name, Path: name, Line: i + 1, Match: shortLine(line)})
			s.matches[name] = true
		}
	}
}
func (s *scanner) includeBacklink(name string) error {
	if s.matches[name] || !dateAllowed(name, s.request) {
		return nil
	}
	data, err := readContent(s.root, name)
	if err != nil {
		return err
	}
	allowed, err := hasTag(string(data), s.request.Tag)
	if err != nil || !allowed {
		return err
	}
	for i, line := range strings.Split(string(data), "\n") {
		for target := range s.matches {
			if linksTo(line, target) {
				s.page.add(Entry{Vault: s.definition.Name, Path: name, Line: i + 1, Match: shortLine(line)})
				return nil
			}
		}
	}
	return nil
}

func compileGlob(pattern string) *regexp.Regexp {
	quoted := regexp.QuoteMeta(pattern)
	quoted = strings.ReplaceAll(quoted, `\*\*/`, `(?:.*/)?`)
	quoted = strings.ReplaceAll(quoted, `\*\*`, `.*`)
	quoted = strings.ReplaceAll(quoted, `\*`, `[^/]*`)
	quoted = strings.ReplaceAll(quoted, `\?`, `[^/]`)
	return regexp.MustCompile("^" + quoted + "$")
}

var wikiLink = regexp.MustCompile(`\[\[([^\]|#]+)(?:[^\]]*)\]\]`)

func linksTo(line, target string) bool {
	target = strings.TrimSuffix(target, ".md")
	for _, match := range wikiLink.FindAllStringSubmatch(line, -1) {
		link := strings.TrimSuffix(strings.TrimSpace(match[1]), ".md")
		if link == target || (!strings.Contains(link, "/") && link == path.Base(target)) {
			return true
		}
	}
	return false
}
func backlinkEntry(vault, name, text, target string) *Entry {
	for i, line := range strings.Split(text, "\n") {
		if linksTo(line, target) {
			return &Entry{Vault: vault, Path: name, Line: i + 1, Match: shortLine(line)}
		}
	}
	return nil
}
func (s *Service) backlinks(ctx context.Context, root *os.Root, d Definition, r Request) (any, error) {
	if _, err := readContent(root, r.Path); err != nil {
		return nil, err
	}
	if err := validateBrowse(r); err != nil {
		return nil, err
	}
	page := newPage(r)
	err := walkNotes(ctx, root, "", func(name string, _ fs.DirEntry) error {
		data, err := readContent(root, name)
		if err != nil {
			return err
		}
		if e := backlinkEntry(d.Name, name, string(data), r.Path); e != nil {
			page.add(*e)
		}
		return nil
	})
	return page.listing(), err
}
