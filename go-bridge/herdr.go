package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

// Adapters receive validated lifecycle events, never arbitrary host API requests.
type adapter interface {
	Send(context.Context, event) error
}

type herdr struct {
	socket string
	pane   string
	seq    int64
}

func (h *herdr) Send(ctx context.Context, e event) error {
	seq := time.Now().UnixMicro()
	if seq <= h.seq {
		seq = h.seq + 1
	}
	h.seq = seq
	method := "pane.report_agent"
	if e.Kind == "release" {
		method = "pane.release_agent"
	}
	params := struct {
		Pane    string `json:"pane_id"`
		Source  string `json:"source"`
		Agent   string `json:"agent"`
		Seq     int64  `json:"seq"`
		State   string `json:"state,omitempty"`
		Message string `json:"message,omitempty"`
	}{h.pane, "pissbox:pi", "pissbox-pi", seq, e.State, e.Message}
	id := fmt.Sprintf("pissbox:%d", seq)
	request := struct {
		ID     string `json:"id"`
		Method string `json:"method"`
		Params any    `json:"params"`
	}{id, method, params}

	ctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", h.socket)
	if err != nil {
		return errors.New("cannot connect to Herd")
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return errors.New("cannot send report to Herd")
	}
	line, err := bufio.NewReader(io.LimitReader(conn, 64*1024)).ReadBytes('\n')
	if err != nil {
		return errors.New("missing or oversized Herd response")
	}
	var response struct {
		ID     string `json:"id"`
		Result *struct {
			Type string `json:"type"`
		} `json:"result"`
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(line, &response) != nil || response.ID != id || response.Result == nil ||
		response.Result.Type != "ok" || (len(response.Error) > 0 && string(response.Error) != "null") {
		return errors.New("Herd rejected the report")
	}
	return nil
}
