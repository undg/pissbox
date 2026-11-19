package github

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// The fake gh is a separate process, exercising actual exec, deadlines, buffers,
// and environment without accessing host credentials or the GitHub network.
func TestGitHubCLIProcess(t *testing.T) {
	if os.Getenv("GITHUB_TEST_PROCESS") != "1" {
		return
	}
	index := 0
	for index < len(os.Args) && os.Args[index] != "--" {
		index++
	}
	call := struct {
		Args          []string
		Prompt, Debug string
	}{os.Args[index+1:], os.Getenv("GH_PROMPT_DISABLED"), os.Getenv("GH_DEBUG")}
	data, _ := json.Marshal(call)
	f, err := os.OpenFile(os.Getenv("GITHUB_TEST_CALLS"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		os.Exit(90)
	}
	f.Write(append(data, '\n'))
	f.Close()
	if os.Getenv("GITHUB_TEST_SLEEP") == "1" {
		time.Sleep(20 * time.Second)
	}
	if os.Getenv("GITHUB_TEST_OVERFLOW") == "1" {
		os.Stdout.Write([]byte(strings.Repeat("x", maxOutputBytes+1)))
		os.Exit(0)
	}
	data, _ = os.ReadFile(os.Getenv("GITHUB_TEST_RESPONSE"))
	if os.Getenv("GITHUB_TEST_API_RESPONSES") == "1" {
		var responses map[string]json.RawMessage
		json.Unmarshal(data, &responses)
		data = responses[call.Args[len(call.Args)-1]]
	}
	os.Stdout.Write(data)
	fmt.Fprintln(os.Stderr, "private-stderr-marker")
	code, _ := strconv.Atoi(os.Getenv("GITHUB_TEST_EXIT"))
	os.Exit(code)
}

func fakeGH(t *testing.T, response string) (string, func() [][]string) {
	t.Helper()
	dir := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// Shell is only fixture plumbing; production never invokes a shell.
	script := "#!/bin/sh\nexec '" + strings.ReplaceAll(exe, "'", "'\\''") + "' -test.run=TestGitHubCLIProcess -- \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	responsePath := filepath.Join(dir, "response")
	if err := os.WriteFile(responsePath, []byte(response), 0600); err != nil {
		t.Fatal(err)
	}
	calls := filepath.Join(dir, "calls")
	t.Setenv("PATH", dir)
	t.Setenv("GITHUB_TEST_PROCESS", "1")
	t.Setenv("GITHUB_TEST_CALLS", calls)
	t.Setenv("GITHUB_TEST_RESPONSE", responsePath)
	t.Setenv("GH_DEBUG", "api")
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	return responsePath, func() [][]string {
		data, _ := os.ReadFile(calls)
		var result [][]string
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if line == "" {
				continue
			}
			var call struct {
				Args          []string
				Prompt, Debug string
			}
			if err := json.Unmarshal([]byte(line), &call); err != nil {
				t.Fatal(err)
			}
			if call.Prompt != "1" || call.Debug != "" {
				t.Fatalf("unsafe gh environment: %+v", call)
			}
			result = append(result, call.Args)
		}
		return result
	}
}

func execute(t *testing.T, op string, req Request) map[string]any {
	t.Helper()
	result, err := (Service{}).Execute(context.Background(), op, req)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result)
	if err != nil || len(data) > maxResultBytes {
		t.Fatal("result does not fit transport", len(data), err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestPRListUsesOneBoundedQueryAndSupportsHistory(t *testing.T) {
	_, calls := fakeGH(t, `{"data":{"repository":{"pullRequests":{"nodes":[{"number":7,"title":"Fix CI","author":{"login":"alice"},"state":"OPEN","isDraft":false,"reviewDecision":"APPROVED","updatedAt":"2026-01-02","headRefOid":"abc","commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"SUCCESS"}}}]}}],"pageInfo":{"hasNextPage":true,"endCursor":"next"}}}}}`)
	for _, state := range []string{"", "open", "closed", "merged", "all"} {
		result := execute(t, "pr_list", Request{Repo: "acme/project", State: state, Cursor: "cursor=literal", Limit: 5})
		if result["pageInfo"].(map[string]any)["endCursor"] != "next" || len(result["nodes"].([]any)) != 1 {
			t.Fatal(result)
		}
	}
	got := calls()
	if len(got) != 5 {
		t.Fatalf("unexpected extra API calls: %v", got)
	}
	for i, call := range got {
		joined := strings.Join(call, "\n")
		for _, expected := range []string{"api\n--hostname\ngithub.com\ngraphql", "owner=acme", "name=project", "limit=5", "cursor=cursor=literal", "statusCheckRollup { state }", "orderBy: {field: UPDATED_AT"} {
			if !strings.Contains(joined, expected) {
				t.Fatalf("missing %q in %v", expected, call)
			}
		}
		states := [][]string{{"OPEN"}, {"OPEN"}, {"CLOSED"}, {"MERGED"}, {"OPEN", "CLOSED", "MERGED"}}[i]
		var actual []string
		for _, arg := range call {
			if strings.HasPrefix(arg, "states[]=") {
				actual = append(actual, strings.TrimPrefix(arg, "states[]="))
			}
		}
		if !reflect.DeepEqual(actual, states) {
			t.Fatal(actual, states)
		}
	}
}

func TestPRViewDescriptionAndDiffHaveLosslessUTF8Pagination(t *testing.T) {
	text := strings.Repeat("界\t", 8000)
	bodyJSON, _ := json.Marshal(text)
	response, calls := fakeGH(t, `{"data":{"repository":{"pullRequest":{"number":2,"body":`+string(bodyJSON)+`,"headRefOid":"head","reviewRequests":{"totalCount":0,"nodes":[]}}}}}`)
	var body strings.Builder
	for offset := 0; ; {
		result := execute(t, "pr_view", Request{Repo: "acme/project", Number: 2, Offset: offset})
		part := result["body"].(string)
		if !utf8.ValidString(part) || len(part) > 12000 {
			t.Fatal("invalid chunk")
		}
		body.WriteString(part)
		if result["nextOffset"] == nil {
			break
		}
		offset = int(result["nextOffset"].(float64))
	}
	if body.String() != text {
		t.Fatal("description lost bytes")
	}
	if err := os.WriteFile(response, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	var patch strings.Builder
	for offset := 0; ; {
		result := execute(t, "pr_diff", Request{Repo: "acme/project", Number: 2, Offset: offset})
		patch.WriteString(result["diff"].(string))
		if result["totalBytes"] != float64(len(text)) {
			t.Fatal(result)
		}
		if result["nextOffset"] == nil {
			break
		}
		offset = int(result["nextOffset"].(float64))
	}
	if patch.String() != text {
		t.Fatal("diff lost bytes")
	}
	last := calls()[len(calls())-1]
	want := []string{"api", "--hostname", "github.com", "--method", "GET", "repos/acme/project/pulls/2", "--header", "Accept: application/vnd.github.diff"}
	if !reflect.DeepEqual(last, want) {
		t.Fatal(last)
	}
}

func TestCommentCategoriesUseFixedReadEndpointsAndPreserveAuthorsAndReplyContext(t *testing.T) {
	body, _ := json.Marshal(strings.Repeat("x", 7000))
	_, calls := fakeGH(t, `[{"id":9,"body":`+string(body)+`,"user":{"login":"reviewer"},"html_url":"https://github.com/acme/project/pull/2#discussion_r9","path":"main.go","line":8,"original_line":7,"in_reply_to_id":3,"pull_request_review_id":4,"state":"CHANGES_REQUESTED"}]`)
	for _, tc := range []struct{ kind, endpoint string }{
		{"", "issues/2/comments"}, {"conversation", "issues/2/comments"}, {"reviews", "pulls/2/reviews"}, {"inline", "pulls/2/comments"},
	} {
		result := execute(t, "pr_comments", Request{Repo: "acme/project", Number: 2, Kind: tc.kind, Page: 2, Limit: 1})
		item := result["items"].([]any)[0].(map[string]any)
		if result["nextPage"] != float64(3) || item["bodyTruncated"] != true || len(item["body"].(string)) != 6000 || item["user"].(map[string]any)["login"] != "reviewer" || item["in_reply_to_id"] != float64(3) || item["line"] != float64(8) {
			t.Fatal(result)
		}
		got := calls()
		want := []string{"api", "--hostname", "github.com", "--method", "GET", "repos/acme/project/" + tc.endpoint + "?per_page=1&page=2"}
		if !reflect.DeepEqual(got[len(got)-1], want) {
			t.Fatal(got)
		}
	}
}

func TestChecksReturnCurrentHeadAndBothCheckKindsWithoutTreatingFailureAsToolFailure(t *testing.T) {
	response, calls := fakeGH(t, `{"data":{"repository":{"pullRequest":{"headRefOid":"abc","commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"FAILURE","contexts":{"nodes":[{"__typename":"CheckRun","name":"test","status":"COMPLETED","conclusion":"FAILURE","detailsUrl":"https://github.com/check/1"},{"__typename":"StatusContext","context":"external","state":"PENDING","targetUrl":"https://ci.example/job"}],"pageInfo":{"hasNextPage":true,"endCursor":"next"}}}}}]}}}}}`)
	result := execute(t, "pr_checks", Request{Repo: "acme/project", Number: 7, Cursor: "previous", Limit: 2})
	if result["headRefOid"] != "abc" || result["state"] != "FAILURE" || len(result["nodes"].([]any)) != 2 || result["pageInfo"].(map[string]any)["endCursor"] != "next" {
		t.Fatal(result)
	}
	if len(calls()) != 1 || !strings.Contains(strings.Join(calls()[0], " "), "contexts(first: $limit, after: $cursor)") {
		t.Fatal(calls())
	}
	for _, commit := range []string{`[]`, `[{"commit":{"statusCheckRollup":null}}]`} {
		if err := os.WriteFile(response, []byte(`{"data":{"repository":{"pullRequest":{"headRefOid":"empty","commits":{"nodes":`+commit+`}}}}}`), 0600); err != nil {
			t.Fatal(err)
		}
		result = execute(t, "pr_checks", Request{Repo: "acme/project", Number: 7})
		if result["state"] != "NONE" || len(result["nodes"].([]any)) != 0 {
			t.Fatal(result)
		}
	}
}

func TestInvalidRequestsNeverExecuteGH(t *testing.T) {
	_, calls := fakeGH(t, "[]")
	for _, tc := range []struct {
		op      string
		request Request
	}{
		{"pr_list", Request{Repo: "https://github.com/acme/project"}},
		{"pr_list", Request{Repo: "../../etc/passwd"}},
		{"pr_list", Request{Repo: "acme/project?command=delete"}},
		{"pr_list", Request{Repo: "acme/.."}},
		{"pr_list", Request{Repo: "acme/project", State: "--web"}},
		{"pr_list", Request{Repo: "acme/project", Limit: 51}},
		{"pr_list", Request{Repo: "acme/project", Limit: -1}},
		{"pr_list", Request{Repo: "acme/project", Cursor: "bad\ncursor"}},
		{"pr_list", Request{Repo: "acme/project", Cursor: strings.Repeat("x", 1025)}},
		{"pr_list", Request{Repo: "acme/project", Number: 1}},
		{"pr_view", Request{Repo: "acme/project"}},
		{"pr_view", Request{Repo: "acme/project", Number: -1}},
		{"pr_view", Request{Repo: "acme/project", Number: 2147483648}},
		{"pr_view", Request{Repo: "acme/project", Number: 1, State: "open"}},
		{"pr_checks", Request{Repo: "acme/project", Number: 1, Page: 1}},
		{"pr_comments", Request{Repo: "acme/project", Number: 1, Kind: "delete"}},
		{"pr_comments", Request{Repo: "acme/project", Number: 1, Cursor: "other"}},
		{"pr_comments", Request{Repo: "acme/project", Number: 1, Page: 10001}},
		{"pr_diff", Request{Repo: "acme/project", Number: 1, Offset: -1}},
		{"pr_diff", Request{Repo: "acme/project", Number: 1, Offset: maxOutputBytes + 1}},
		{"pr_diff", Request{Repo: "acme/project", Number: 1, Limit: 1}},
		{"pr_merge", Request{Repo: "acme/project", Number: 1}},
	} {
		if _, err := (Service{}).Execute(context.Background(), tc.op, tc.request); err == nil {
			t.Fatalf("accepted %+v", tc)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Service{}).Execute(ctx, "pr_list", Request{Repo: "acme/project"}); err == nil {
		t.Fatal("accepted cancelled request")
	}
	if len(calls()) != 0 {
		t.Fatal("invalid requests reached gh", calls())
	}
}

func TestCommandFailureMissingGHDeadlineAndOutputCeiling(t *testing.T) {
	_, calls := fakeGH(t, "private-stdout-marker")
	t.Setenv("GITHUB_TEST_EXIT", "4")
	_, err := (Service{}).Execute(context.Background(), "pr_diff", Request{Repo: "acme/project", Number: 1})
	if err == nil || !strings.Contains(err.Error(), "gh exit 4") || strings.Contains(err.Error(), "marker") || len(calls()) != 1 {
		t.Fatal(err, calls())
	}
	t.Setenv("GITHUB_TEST_EXIT", "0")
	t.Setenv("GITHUB_TEST_SLEEP", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := (Service{}).Execute(ctx, "pr_diff", Request{Repo: "acme/project", Number: 1}); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatal(err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("deadline failed to terminate gh")
	}
	t.Setenv("GITHUB_TEST_SLEEP", "0")
	t.Setenv("GITHUB_TEST_OVERFLOW", "1")
	if _, err := (Service{}).Execute(context.Background(), "pr_diff", Request{Repo: "acme/project", Number: 1}); err == nil || !strings.Contains(err.Error(), "8 MiB") {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := (Service{}).Execute(context.Background(), "pr_diff", Request{Repo: "acme/project", Number: 1}); err == nil || !strings.Contains(err.Error(), "host PATH") {
		t.Fatal(err)
	}
}

func TestMalformedAndOversizedResponsesFailExplicitly(t *testing.T) {
	response, _ := fakeGH(t, "")
	for _, tc := range []struct{ op, data string }{
		{"pr_list", "not json"},
		{"pr_list", `{"errors":[{"message":"private-marker"}],"data":{"repository":{"pullRequests":{"nodes":[]}}}}`},
		{"pr_list", `{"data":{"repository":null}}`},
		{"pr_list", `{"data":{"repository":{"pullRequests":null}}}`},
		{"pr_view", `{"data":{"repository":{"pullRequest":null}}}`},
		{"pr_view", `{"data":{"repository":{"pullRequest":{}}}}`},
		{"pr_checks", `{"data":{"repository":{"pullRequest":{"commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"SUCCESS"}}}]}}}}}`},
		{"pr_comments", "{}"},
		{"pr_comments", "null"},
		{"pr_diff", string([]byte{0xff})},
		{"pr_comments", `[{"body":"` + strings.Repeat("x", 6000) + `"},{"body":"` + strings.Repeat("x", 6000) + `"},{"body":"` + strings.Repeat("x", 6000) + `"},{"body":"` + strings.Repeat("x", 6000) + `"},{"body":"` + strings.Repeat("x", 6000) + `"}]`},
	} {
		if err := os.WriteFile(response, []byte(tc.data), 0600); err != nil {
			t.Fatal(err)
		}
		number := 1
		if tc.op == "pr_list" {
			number = 0
		}
		if _, err := (Service{}).Execute(context.Background(), tc.op, Request{Repo: "acme/project", Number: number}); err == nil || strings.Contains(err.Error(), "private-marker") {
			t.Fatalf("%s: %v", tc.op, err)
		}
	}
}

func TestChunkBoundariesAndLineLimit(t *testing.T) {
	for _, offset := range []int{1, 4} {
		if _, _, err := chunk("界", offset, 12000); err == nil {
			t.Fatal("accepted invalid byte offset", offset)
		}
	}
	part, next, err := chunk(strings.Repeat("line\n", 1001), 0, 12000)
	if err != nil || strings.Count(part, "\n") != 1000 || next != len(part) {
		t.Fatal(len(part), next, err)
	}
	text := strings.Repeat("<\x00界", 10000)
	var rebuilt strings.Builder
	for offset := 0; ; {
		part, next, err := chunk(text, offset, 12000)
		encoded, _ := json.Marshal(part)
		if err != nil || len(encoded) > 16000 || !utf8.ValidString(part) || part == "" {
			t.Fatal("escaped chunk exceeded budget or stopped advancing", err)
		}
		rebuilt.WriteString(part)
		if next == 0 {
			break
		}
		offset = next
	}
	if rebuilt.String() != text {
		t.Fatal("escaped chunks lost bytes")
	}
	part, next, err = chunk("界", 3, 12000)
	if err != nil || part != "" || next != 0 {
		t.Fatal(part, next, err)
	}
}
