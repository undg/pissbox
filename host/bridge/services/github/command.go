package github

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
	"unicode/utf8"
)

const maxOutputBytes = 8 * 1024 * 1024

// A hard stdout ceiling bounds memory even when gh or a remote response misbehaves.
type boundedOutput struct {
	buffer   bytes.Buffer
	overflow bool
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	if len(p) > maxOutputBytes-b.buffer.Len() {
		b.overflow = true
		return 0, errors.New("GitHub output limit exceeded")
	}
	return b.buffer.Write(p)
}

func run(ctx context.Context, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "GH_DEBUG=", "GH_PAGER=cat", "NO_COLOR=1")
	cmd.WaitDelay = time.Second
	// Never forward stderr: host debug settings and helpers can disclose secrets.
	cmd.Stderr = io.Discard
	var output boundedOutput
	cmd.Stdout = &output
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil, errors.New("GitHub request cancelled or timed out")
	}
	if output.overflow {
		return nil, errors.New("GitHub response exceeds the 8 MiB host read limit")
	}
	if errors.Is(err, exec.ErrNotFound) {
		return nil, errors.New("GitHub tools require gh on the host PATH; install GitHub CLI on the host")
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return nil, fmt.Errorf("GitHub request failed (gh exit %d); check host gh auth, repository access, and API rate limits", exit.ExitCode())
		}
		return nil, errors.New("could not execute host gh")
	}
	if !utf8.Valid(output.buffer.Bytes()) {
		return nil, errors.New("GitHub returned invalid UTF-8")
	}
	return output.buffer.Bytes(), nil
}
