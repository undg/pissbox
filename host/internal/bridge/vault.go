package bridge

import (
	"context"
	"errors"
	"os"
	"time"

	"pissbox/host/config"
	"pissbox/host/internal/vault"
)

type vaultJob struct {
	agent, id, op string
	params        vault.Request
	deadline      time.Time
}
type vaultResult struct {
	agent    string
	response agentResponse
}

func hostVaultService() (*vault.Service, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	explicit := os.Getenv("PISSBOX_CONFIG")
	path, err := config.Path(explicit, cwd)
	if err != nil {
		return nil, err
	}
	definitions, err := config.LoadVaults(path)
	if os.IsNotExist(err) && explicit == "" {
		return vault.New(nil)
	}
	if err != nil {
		return nil, errors.New("vault configuration unavailable; check the host global configuration and restart the bridge")
	}
	values := make([]vault.Definition, 0, len(definitions))
	for _, d := range definitions {
		values = append(values, vault.Definition{Name: d.Name, Path: d.Path, Git: d.Git == nil || *d.Git})
	}
	return vault.New(values)
}

// One bounded worker serializes all sessions' vault operations without blocking
// messaging, readiness heartbeats, or the host control socket on disk/desktop IO.
func (r *messageRouter) startVaultWorker(ctx context.Context) <-chan struct{} {
	r.vaultJobs = make(chan vaultJob, 16)
	r.vaultResults = make(chan vaultResult, 16)
	done := make(chan struct{})
	service, loadErr := hostVaultService()
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case job := <-r.vaultJobs:
				response := agentResponse{Version: 1, ID: job.id, Status: "failed"}
				err := loadErr
				if err == nil {
					operation, cancel := context.WithDeadline(ctx, job.deadline)
					response.Result, err = service.Execute(operation, job.op, job.params)
					cancel()
				}
				if err != nil {
					response.Error = err.Error()
				} else {
					response.Status = "ok"
				}
				select {
				case r.vaultResults <- vaultResult{job.agent, response}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return done
}
