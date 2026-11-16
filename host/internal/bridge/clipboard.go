package bridge

import (
	"context"
	"time"

	"pissbox/host/internal/clipboard"
)

type clipboardJob struct {
	agent, id, text string
	desktop         *clipboard.DesktopEnvironment
	deadline        time.Time
}

type clipboardResult struct {
	agent    string
	response agentResponse
}

// Serialize writes without blocking messaging or readiness on desktop IO.
func (r *messageRouter) startClipboardWorker(ctx context.Context) <-chan struct{} {
	r.clipboardJobs = make(chan clipboardJob, 8)
	r.clipboardResults = make(chan clipboardResult, 8)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case job := <-r.clipboardJobs:
				operation, cancel := context.WithDeadline(ctx, job.deadline)
				backend, err := clipboard.Copy(operation, job.desktop, job.text)
				cancel()
				response := agentResponse{Version: 1, ID: job.id, Status: "ok", Result: map[string]string{"backend": backend}}
				if err != nil {
					response.Status, response.Error, response.Result = "failed", err.Error(), nil
				}
				select {
				case r.clipboardResults <- clipboardResult{job.agent, response}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return done
}

func (r *messageRouter) queueClipboard(a *liveAgent, req agentRequest, response *agentResponse) bool {
	if req.ClipboardText == nil {
		response.Error = "clipboard text is required"
		return false
	}
	if err := clipboard.ValidateText(*req.ClipboardText); err != nil {
		response.Error = err.Error()
		return false
	}
	if r.clipboardJobs == nil {
		response.Error = "clipboard service unavailable; rebuild and reload the host bridge"
		return false
	}
	select {
	case r.clipboardJobs <- clipboardJob{agent: a.identity.ID, id: req.ID, text: *req.ClipboardText, desktop: a.desktop, deadline: time.Now().Add(8 * time.Second)}:
		a.requests[req.ID] = nil
		return true
	default:
		response.Error = "clipboard request queue full; no operation performed"
		return false
	}
}
