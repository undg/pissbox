package bridge

import (
	"context"
	"time"

	"pissbox/host/bridge/services/github"
)

type githubJob struct {
	agent, id, op string
	params        github.Request
	deadline      time.Time
}

type githubResult struct {
	agent    string
	response agentResponse
}

// One bounded worker prevents API bursts and keeps host gh IO off the router.
// Deadlines include queue time; expired jobs never launch gh.
func (r *messageRouter) startGitHubWorker(ctx context.Context) <-chan struct{} {
	r.githubJobs = make(chan githubJob, 8)
	r.githubResults = make(chan githubResult, 8)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case job := <-r.githubJobs:
				operation, cancel := context.WithDeadline(ctx, job.deadline)
				result, err := (github.Service{}).Execute(operation, job.op, job.params)
				cancel()
				response := agentResponse{Version: 1, ID: job.id, Status: "ok", Result: result}
				if err != nil {
					response.Status, response.Error = "failed", err.Error()
				}
				select {
				case r.githubResults <- githubResult{job.agent, response}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return done
}

func (r *messageRouter) queueGitHub(a *liveAgent, req agentRequest, response *agentResponse) bool {
	if req.GitHubParams == nil || r.githubJobs == nil {
		response.Error = "GitHub service unavailable; rebuild and reload the host bridge"
		return false
	}
	select {
	case r.githubJobs <- githubJob{agent: a.identity.ID, id: req.ID, op: req.GitHubOp, params: *req.GitHubParams, deadline: time.Now().Add(8 * time.Second)}:
		a.requests[req.ID] = nil
		return true
	default:
		response.Error = "GitHub request queue full; no request performed"
		return false
	}
}
