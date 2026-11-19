// Package github provides a small read-only GitHub interface through host gh auth.
// It has no dependency on Pi, mailbox transport, or launcher configuration.
package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

const maxResultBytes = 24 * 1024

var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,38}/[A-Za-z0-9_.-]{1,100}$`)

// Request selects a fixed operation's inputs, never command arguments or API paths.
type Request struct {
	Repo   string `json:"repo"`
	RunID  int64  `json:"runId,omitempty"`
	JobID  int64  `json:"jobId,omitempty"`
	Number int    `json:"number,omitempty"`
	State  string `json:"state,omitempty"`
	Kind   string `json:"kind,omitempty"`
	Cursor string `json:"cursor,omitempty"`
	Page   int    `json:"page,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Offset int    `json:"offset,omitempty"`
}

// Service uses the host's gh executable and authentication without probing or login.
type Service struct{}

// Execute performs one explicit read. No operation polls, retries, or follows pages.
func (Service) Execute(ctx context.Context, op string, req Request) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validate(op, req); err != nil {
		return nil, err
	}
	var result any
	var err error
	switch op {
	case "pr_list", "pr_view", "pr_checks":
		result, err = graphql(ctx, op, req)
	case "pr_comments":
		result, err = comments(ctx, req)
	case "pr_diff":
		result, err = diff(ctx, req)
	case "run_view":
		result, err = runView(ctx, req)
	case "job_logs":
		result, err = jobLogs(ctx, req)
	}
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(result)
	if err != nil || len(data) > maxResultBytes {
		return nil, errors.New("GitHub result exceeds 24 KiB; request a smaller limit")
	}
	return result, nil
}

func validate(op string, r Request) error {
	if !repositoryPattern.MatchString(r.Repo) || strings.HasSuffix(r.Repo, "/.") || strings.HasSuffix(r.Repo, "/..") {
		return errors.New("repo must be a github.com owner/repo name, not a URL or host path")
	}
	if r.Number < 0 || r.Number > 2147483647 || (strings.HasPrefix(op, "pr_") && op != "pr_list" && r.Number == 0) {
		return errors.New("a positive PR number is required")
	}
	if r.Limit < 0 || r.Limit > 50 || r.Page < 0 || r.Page > 10000 || r.Offset < 0 || r.Offset > maxOutputBytes {
		return errors.New("invalid GitHub pagination: limit 1..50, page 1..10000, offset 0..8388608")
	}
	if len(r.Cursor) > 1024 || strings.ContainsAny(r.Cursor, "\x00\r\n") {
		return errors.New("invalid GitHub cursor")
	}
	if op == "run_view" || op == "job_logs" {
		return validateActions(op, r)
	}
	if r.RunID != 0 || r.JobID != 0 {
		return errors.New("unsupported workflow IDs for PR operation")
	}
	switch op {
	case "pr_list":
		if r.Number != 0 || r.Kind != "" || r.Page != 0 || r.Offset != 0 {
			return errors.New("unsupported PR list parameters")
		}
		switch r.State {
		case "", "open", "closed", "merged", "all":
		default:
			return errors.New("state must be open, closed, merged, or all")
		}
	case "pr_view":
		if r.State != "" || r.Kind != "" || r.Cursor != "" || r.Page != 0 || r.Limit != 0 {
			return errors.New("unsupported PR view parameters")
		}
	case "pr_checks":
		if r.State != "" || r.Kind != "" || r.Page != 0 || r.Offset != 0 {
			return errors.New("unsupported PR checks parameters")
		}
	case "pr_comments":
		if r.State != "" || r.Cursor != "" || r.Offset != 0 {
			return errors.New("unsupported PR comments parameters")
		}
		switch r.Kind {
		case "", "conversation", "reviews", "inline":
		default:
			return errors.New("kind must be conversation, reviews, or inline")
		}
	case "pr_diff":
		if r.State != "" || r.Kind != "" || r.Cursor != "" || r.Page != 0 || r.Limit != 0 {
			return errors.New("unsupported PR diff parameters")
		}
	default:
		return errors.New("unknown GitHub operation")
	}
	return nil
}

func pageSize(limit int) int {
	if limit == 0 {
		return 20
	}
	return limit
}

// chunk preserves UTF-8 boundaries and caps both bytes and lines. Offsets refer
// to the original text so even a large diff can be read without silent loss.
func chunk(text string, offset, maxBytes int) (string, int, error) {
	if offset > len(text) || (offset < len(text) && !utf8.RuneStart(text[offset])) {
		return "", 0, errors.New("offset is outside the text or splits a UTF-8 character")
	}
	end := min(len(text), offset+maxBytes)
	for end < len(text) && !utf8.RuneStart(text[end]) {
		end--
	}
	lines := 0
	for i := offset; i < end; i++ {
		if text[i] == '\n' {
			lines++
			if lines == 1000 {
				end = i + 1
				break
			}
		}
	}
	// JSON escaping can expand ordinary diff text (for example HTML or control
	// characters). Leave room for metadata rather than making it unpageable.
	for {
		encoded, _ := json.Marshal(text[offset:end])
		if len(encoded) <= 16000 {
			break
		}
		end = offset + (end-offset)/2
		for end < len(text) && !utf8.RuneStart(text[end]) {
			end--
		}
	}
	next := 0
	if end < len(text) {
		next = end
	}
	return text[offset:end], next, nil
}

func diff(ctx context.Context, r Request) (any, error) {
	endpoint := fmt.Sprintf("repos/%s/pulls/%d", r.Repo, r.Number)
	data, err := run(ctx, "api", "--hostname", "github.com", "--method", "GET", endpoint, "--header", "Accept: application/vnd.github.diff")
	if err != nil {
		return nil, err
	}
	text, next, err := chunk(string(data), r.Offset, 12000)
	if err != nil {
		return nil, err
	}
	return struct {
		Diff       string `json:"diff"`
		NextOffset int    `json:"nextOffset,omitempty"`
		TotalBytes int    `json:"totalBytes"`
	}{text, next, len(data)}, nil
}

type comment struct {
	ID            int64  `json:"id"`
	User          actor  `json:"user"`
	Body          string `json:"body"`
	URL           string `json:"html_url"`
	Created       string `json:"created_at,omitempty"`
	Updated       string `json:"updated_at,omitempty"`
	Submitted     string `json:"submitted_at,omitempty"`
	State         string `json:"state,omitempty"`
	Path          string `json:"path,omitempty"`
	Line          *int   `json:"line,omitempty"`
	OriginalLine  *int   `json:"original_line,omitempty"`
	Side          string `json:"side,omitempty"`
	ReplyTo       int64  `json:"in_reply_to_id,omitempty"`
	ReviewID      int64  `json:"pull_request_review_id,omitempty"`
	BodyTruncated bool   `json:"bodyTruncated,omitempty"`
}

type actor struct {
	Login string `json:"login"`
}

func comments(ctx context.Context, r Request) (any, error) {
	kind := r.Kind
	if kind == "" {
		kind = "conversation"
	}
	var endpoint string
	switch kind {
	case "conversation":
		endpoint = fmt.Sprintf("repos/%s/issues/%d/comments", r.Repo, r.Number)
	case "reviews":
		endpoint = fmt.Sprintf("repos/%s/pulls/%d/reviews", r.Repo, r.Number)
	case "inline":
		endpoint = fmt.Sprintf("repos/%s/pulls/%d/comments", r.Repo, r.Number)
	}
	page := max(1, r.Page)
	limit := pageSize(r.Limit)
	endpoint += "?per_page=" + strconv.Itoa(limit) + "&page=" + strconv.Itoa(page)
	data, err := run(ctx, "api", "--hostname", "github.com", "--method", "GET", endpoint)
	if err != nil {
		return nil, err
	}
	var items []comment
	if err := json.Unmarshal(data, &items); err != nil || items == nil {
		return nil, errors.New("invalid GitHub comments response")
	}
	for i := range items {
		body, next, _ := chunk(items[i].Body, 0, 6000)
		items[i].Body = body
		items[i].BodyTruncated = next != 0
	}
	next := 0
	if len(items) == limit {
		next = page + 1
	}
	return struct {
		Kind     string    `json:"kind"`
		Items    []comment `json:"items"`
		NextPage int       `json:"nextPage,omitempty"`
	}{kind, items, next}, nil
}
