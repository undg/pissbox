package github

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRunViewPinsJobsToAttemptAndPreservesFailureDetails(t *testing.T) {
	_, calls := fakeGH(t, `{
		"repos/acme/project/actions/runs/6000000000": {"id":6000000000,"name":"CI","display_title":"Fix build","status":"completed","conclusion":"failure","head_sha":"abc","run_attempt":2,"html_url":"https://github.com/acme/project/actions/runs/6000000000"},
		"repos/acme/project/actions/runs/6000000000/attempts/2/jobs?per_page=1&page=2": {"total_count":3,"jobs":[{"id":7000000000,"run_id":6000000000,"name":"test","status":"completed","conclusion":"failure","steps":[{"number":4,"name":"Run tests","status":"completed","conclusion":"failure"}]}]}
	}`)
	t.Setenv("GITHUB_TEST_API_RESPONSES", "1")
	result := execute(t, "run_view", Request{Repo: "acme/project", RunID: 6000000000, Page: 2, Limit: 1})
	info := result["run"].(map[string]any)
	job := result["jobs"].([]any)[0].(map[string]any)
	step := job["steps"].([]any)[0].(map[string]any)
	if info["conclusion"] != "failure" || info["head_sha"] != "abc" || info["run_attempt"] != float64(2) || job["id"] != float64(7000000000) || step["name"] != "Run tests" || step["conclusion"] != "failure" || result["totalCount"] != float64(3) || result["nextPage"] != float64(3) {
		t.Fatal(result)
	}
	want := [][]string{
		{"api", "--hostname", "github.com", "--method", "GET", "repos/acme/project/actions/runs/6000000000"},
		{"api", "--hostname", "github.com", "--method", "GET", "repos/acme/project/actions/runs/6000000000/attempts/2/jobs?per_page=1&page=2"},
	}
	if !reflect.DeepEqual(calls(), want) {
		t.Fatal(calls())
	}
}

func TestRunViewDefaultsAndEmptyJobs(t *testing.T) {
	_, calls := fakeGH(t, `{
		"repos/acme/project/actions/runs/1": {"id":1,"run_attempt":1,"status":"queued","conclusion":null},
		"repos/acme/project/actions/runs/1/attempts/1/jobs?per_page=20&page=1": {"total_count":0,"jobs":[]}
	}`)
	t.Setenv("GITHUB_TEST_API_RESPONSES", "1")
	result := execute(t, "run_view", Request{Repo: "acme/project", RunID: 1})
	if len(result["jobs"].([]any)) != 0 || result["nextPage"] != nil || len(calls()) != 2 {
		t.Fatal(result, calls())
	}
}

func TestRunViewRejectsMalformedResponsesWithoutFollowingRemotePaths(t *testing.T) {
	for _, tc := range []struct{ name, info, jobs string }{
		{"invalid run JSON", `[]`, `{}`},
		{"missing run", `{}`, `{}`},
		{"wrong run", `{"id":2,"run_attempt":1}`, `{}`},
		{"missing attempt", `{"id":1}`, `{}`},
		{"invalid jobs JSON", `{"id":1,"run_attempt":1}`, `[]`},
		{"missing total", `{"id":1,"run_attempt":1}`, `{"jobs":[]}`},
		{"negative total", `{"id":1,"run_attempt":1}`, `{"total_count":-1,"jobs":[]}`},
		{"missing jobs", `{"id":1,"run_attempt":1}`, `{"total_count":0}`},
		{"wrong job run", `{"id":1,"run_attempt":1}`, `{"total_count":1,"jobs":[{"id":3,"run_id":2}]}`},
		{"missing job ID", `{"id":1,"run_attempt":1}`, `{"total_count":1,"jobs":[{"run_id":1}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			responses, _ := json.Marshal(map[string]json.RawMessage{
				"repos/acme/project/actions/runs/1":                                    json.RawMessage(tc.info),
				"repos/acme/project/actions/runs/1/attempts/1/jobs?per_page=20&page=1": json.RawMessage(tc.jobs),
			})
			_, calls := fakeGH(t, string(responses))
			t.Setenv("GITHUB_TEST_API_RESPONSES", "1")
			if _, err := (Service{}).Execute(context.Background(), "run_view", Request{Repo: "acme/project", RunID: 1}); err == nil {
				t.Fatal("accepted malformed response")
			}
			if len(calls()) > 2 {
				t.Fatal("unexpected API reads", calls())
			}
		})
	}
}

func TestJobLogsAreLosslesslyPagedWithFixedReadEndpoint(t *testing.T) {
	text := strings.Repeat("2026-01-02T00:00:00Z\t<failed 界>\n", 2000)
	_, calls := fakeGH(t, text)
	var rebuilt strings.Builder
	for offset := 0; ; {
		result := execute(t, "job_logs", Request{Repo: "acme/project", JobID: 7000000000, Offset: offset})
		part := result["log"].(string)
		if !utf8.ValidString(part) || len(part) > 12000 || strings.Count(part, "\n") > 1000 || result["jobId"] != float64(7000000000) || result["totalBytes"] != float64(len(text)) {
			t.Fatal(result)
		}
		rebuilt.WriteString(part)
		if result["nextOffset"] == nil {
			break
		}
		offset = int(result["nextOffset"].(float64))
	}
	if rebuilt.String() != text {
		t.Fatal("job logs lost bytes")
	}
	for _, call := range calls() {
		want := []string{"api", "--hostname", "github.com", "--method", "GET", "repos/acme/project/actions/jobs/7000000000/logs"}
		if !reflect.DeepEqual(call, want) {
			t.Fatal(call)
		}
	}
}

func TestJobLogsEmptyInvalidOffsetsAndUnavailableLogs(t *testing.T) {
	response, calls := fakeGH(t, "")
	result := execute(t, "job_logs", Request{Repo: "acme/project", JobID: 1})
	if result["log"] != "" || result["totalBytes"] != float64(0) || result["nextOffset"] != nil {
		t.Fatal(result)
	}
	if err := os.WriteFile(response, []byte("界"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, offset := range []int{1, 4} {
		if _, err := (Service{}).Execute(context.Background(), "job_logs", Request{Repo: "acme/project", JobID: 1, Offset: offset}); err == nil {
			t.Fatal("accepted invalid log offset", offset)
		}
	}
	t.Setenv("GITHUB_TEST_EXIT", "1")
	before := len(calls())
	if _, err := (Service{}).Execute(context.Background(), "job_logs", Request{Repo: "acme/project", JobID: 1}); err == nil || !strings.Contains(err.Error(), "gh exit 1") {
		t.Fatal(err)
	}
	if len(calls()) != before+1 {
		t.Fatal("unavailable logs retried", calls())
	}
}

func TestInvalidActionsRequestsNeverExecuteGH(t *testing.T) {
	_, calls := fakeGH(t, "")
	for _, tc := range []struct {
		op string
		r  Request
	}{
		{"run_view", Request{Repo: "acme/project"}},
		{"run_view", Request{Repo: "acme/project", RunID: -1}},
		{"run_view", Request{Repo: "acme/project", RunID: maxActionsID + 1}},
		{"run_view", Request{Repo: "acme/project", RunID: 1, JobID: 2}},
		{"run_view", Request{Repo: "acme/project", RunID: 1, Offset: 1}},
		{"run_view", Request{Repo: "acme/project", RunID: 1, Number: 1}},
		{"run_view", Request{Repo: "acme/project", RunID: 1, State: "failure"}},
		{"run_view", Request{Repo: "acme/project", RunID: 1, Kind: "failed"}},
		{"run_view", Request{Repo: "acme/project", RunID: 1, Cursor: "next"}},
		{"run_view", Request{Repo: "acme/project", RunID: 1, Page: 10001}},
		{"job_logs", Request{Repo: "acme/project"}},
		{"job_logs", Request{Repo: "acme/project", JobID: -1}},
		{"job_logs", Request{Repo: "acme/project", JobID: maxActionsID + 1}},
		{"job_logs", Request{Repo: "acme/project", JobID: 1, RunID: 2}},
		{"job_logs", Request{Repo: "acme/project", JobID: 1, Page: 1}},
		{"job_logs", Request{Repo: "acme/project", JobID: 1, Limit: 1}},
		{"job_logs", Request{Repo: "acme/project", JobID: 1, Offset: -1}},
		{"job_logs", Request{Repo: "acme/project", JobID: 1, Offset: maxOutputBytes + 1}},
		{"job_logs", Request{Repo: "https://github.com/acme/project", JobID: 1}},
		{"pr_list", Request{Repo: "acme/project", RunID: 1}},
		{"pr_checks", Request{Repo: "acme/project", Number: 1, JobID: 1}},
		{"run_rerun", Request{Repo: "acme/project", RunID: 1}},
	} {
		if _, err := (Service{}).Execute(context.Background(), tc.op, tc.r); err == nil {
			t.Fatalf("accepted %+v", tc)
		}
	}
	if len(calls()) != 0 {
		t.Fatal("invalid requests reached gh", calls())
	}
}
