package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// IDs must round-trip exactly through JavaScript and JSON tool arguments.
const maxActionsID = 9007199254740991

func validateActions(op string, r Request) error {
	if r.Number != 0 || r.State != "" || r.Kind != "" || r.Cursor != "" {
		return errors.New("unsupported workflow parameters")
	}
	if op == "run_view" {
		if r.RunID < 1 || r.RunID > maxActionsID || r.JobID != 0 || r.Offset != 0 {
			return errors.New("run_view requires a positive runId and accepts only page and limit")
		}
	} else if r.JobID < 1 || r.JobID > maxActionsID || r.RunID != 0 || r.Page != 0 || r.Limit != 0 {
		return errors.New("job_logs requires a positive jobId and accepts only offset")
	}
	return nil
}

type workflowRun struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Title      string `json:"display_title"`
	URL        string `json:"html_url"`
	Event      string `json:"event"`
	HeadBranch string `json:"head_branch"`
	HeadSHA    string `json:"head_sha"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	Attempt    int    `json:"run_attempt"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

type workflowStep struct {
	Number     int    `json:"number"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

type workflowJob struct {
	ID          int64          `json:"id"`
	RunID       int64          `json:"run_id"`
	Name        string         `json:"name"`
	URL         string         `json:"html_url"`
	Status      string         `json:"status"`
	Conclusion  string         `json:"conclusion"`
	StartedAt   string         `json:"started_at"`
	CompletedAt string         `json:"completed_at"`
	Steps       []workflowStep `json:"steps"`
}

func runView(ctx context.Context, r Request) (any, error) {
	endpoint := fmt.Sprintf("repos/%s/actions/runs/%d", r.Repo, r.RunID)
	data, err := run(ctx, "api", "--hostname", "github.com", "--method", "GET", endpoint)
	if err != nil {
		return nil, err
	}
	var info workflowRun
	if err := json.Unmarshal(data, &info); err != nil || info.ID != r.RunID || info.Attempt < 1 {
		return nil, errors.New("invalid GitHub workflow run response")
	}
	page, limit := max(1, r.Page), pageSize(r.Limit)
	// Pin jobs to the observed attempt: a concurrent rerun must not mix attempts.
	endpoint += fmt.Sprintf("/attempts/%d/jobs?per_page=%d&page=%d", info.Attempt, limit, page)
	data, err = run(ctx, "api", "--hostname", "github.com", "--method", "GET", endpoint)
	if err != nil {
		return nil, err
	}
	var jobs struct {
		TotalCount *int          `json:"total_count"`
		Jobs       []workflowJob `json:"jobs"`
	}
	if err := json.Unmarshal(data, &jobs); err != nil || jobs.TotalCount == nil || *jobs.TotalCount < 0 || jobs.Jobs == nil {
		return nil, errors.New("invalid GitHub workflow jobs response")
	}
	for _, job := range jobs.Jobs {
		if job.ID < 1 || job.RunID != r.RunID {
			return nil, errors.New("invalid GitHub workflow job identity")
		}
	}
	next := 0
	if page*limit < *jobs.TotalCount {
		next = page + 1
	}
	return struct {
		Run        workflowRun   `json:"run"`
		Jobs       []workflowJob `json:"jobs"`
		TotalCount int           `json:"totalCount"`
		NextPage   int           `json:"nextPage,omitempty"`
	}{info, jobs.Jobs, *jobs.TotalCount, next}, nil
}

func jobLogs(ctx context.Context, r Request) (any, error) {
	endpoint := fmt.Sprintf("repos/%s/actions/jobs/%d/logs", r.Repo, r.JobID)
	// The job endpoint returns plain text after gh follows GitHub's redirect.
	// Avoid workflow ZIP archives, gh's persistent log cache, and shell commands.
	data, err := run(ctx, "api", "--hostname", "github.com", "--method", "GET", endpoint)
	if err != nil {
		return nil, err
	}
	text, next, err := chunk(string(data), r.Offset, 12000)
	if err != nil {
		return nil, err
	}
	return struct {
		JobID      int64  `json:"jobId"`
		Log        string `json:"log"`
		TotalBytes int    `json:"totalBytes"`
		NextOffset int    `json:"nextOffset,omitempty"`
	}{r.JobID, text, len(data), next}, nil
}
