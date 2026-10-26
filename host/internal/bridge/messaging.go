package bridge

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"
	"slices"
	"sort"
	"syscall"
	"time"
	"unicode/utf8"
)

const maxRecordBytes = 32 * 1024
const maxTextBytes = 16 * 1024
const presenceTTL = 10 * time.Second
const receiptTimeout = 10 * time.Second
const maxPending = 64

// Bound retained deduplication state rather than evicting IDs and permitting replay.
const maxIncarnationRecords = 4096

var messagingID = regexp.MustCompile(`^[0-9a-f]{32}$`)

type agentIdentity struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"`
	Name      string `json:"name"`
	AgentName string `json:"agentName"`
	Cwd       string `json:"cwd"`
	State     string `json:"state"`
}
type presenceRecord struct {
	Version int           `json:"version"`
	Agent   agentIdentity `json:"agent"`
}
type agentRequest struct {
	Version   int    `json:"version"`
	ID        string `json:"id"`
	Op        string `json:"op"`
	To        string `json:"to,omitempty"`
	Text      string `json:"text,omitempty"`
	InReplyTo string `json:"inReplyTo,omitempty"`
}
type peerMessage struct {
	Version   int           `json:"version"`
	ID        string        `json:"id"`
	To        string        `json:"to"`
	From      agentIdentity `json:"from"`
	Text      string        `json:"text"`
	InReplyTo string        `json:"inReplyTo,omitempty"`
	Expires   int64         `json:"expires"`
}
type agentResponse struct {
	Version   int             `json:"version"`
	ID        string          `json:"id"`
	Status    string          `json:"status"`
	Error     string          `json:"error,omitempty"`
	MessageID string          `json:"messageId,omitempty"`
	Self      *agentIdentity  `json:"self,omitempty"`
	Peers     []agentIdentity `json:"peers,omitempty"`
}
type inboxIndex struct {
	Version  int      `json:"version"`
	To       string   `json:"to"`
	Messages []string `json:"messages"`
}

type receiptRecord struct {
	Version int    `json:"version"`
	ID      string `json:"id"`
	To      string `json:"to"`
	Status  string `json:"status"`
}
type liveAgent struct {
	identity        agentIdentity
	registration    string
	root            *os.Root
	requests        map[string]*agentResponse // nil means awaiting receipt
	indexedMessages []string
	indexPublished  bool
}
type pendingMessage struct {
	message   peerMessage
	source    *liveAgent
	recipient *liveAgent
	requestID string
	deadline  time.Time
}
type messageRouter struct {
	agents  map[string]*liveAgent
	retired map[string]bool
	pending map[string]*pendingMessage
}

func randomID() string {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(token[:])
}

// Each path component is opened separately; neither directory nor record symlinks
// are accepted. os.Root additionally confines all operations to the mailbox.
func recordDirectory(root *os.Root, name string) (*os.Root, error) {
	info, err := root.Lstat(name)
	if err != nil || !info.IsDir() {
		return nil, errors.New("invalid record directory")
	}
	return root.OpenRoot(name)
}
func readRecord(root *os.Root, name string, value any) error {
	f, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxRecordBytes {
		return errors.New("invalid record file")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxRecordBytes+1))
	if err != nil || len(data) > maxRecordBytes || !utf8.Valid(data) {
		return errors.New("invalid record size or encoding")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(value); err != nil {
		return err
	}
	if dec.Decode(new(any)) != io.EOF {
		return errors.New("trailing record data")
	}
	return nil
}
func publishRecord(root *os.Root, name string, value any) error {
	data, err := json.Marshal(value)
	if err != nil || len(data) > maxRecordBytes {
		return errors.New("record too large")
	}
	temporary := "." + randomID() + ".pending"
	f, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(temporary)
	_, err = f.Write(data)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return root.Rename(temporary, name)
}
func recordNames(root *os.Root, limit int) ([]string, error) {
	f, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	entries, err := f.ReadDir(limit + 1)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if len(entries) > limit {
		return nil, errors.New("directory overflow")
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}
func validIdentity(a agentIdentity) bool {
	return messagingID.MatchString(a.ID) && a.SessionID != "" && len(a.SessionID) <= 512 && len(a.Name) <= 512 &&
		a.AgentName != "" && len(a.AgentName) <= 80 && len(a.Cwd) <= 4096 &&
		(a.State == "idle" || a.State == "working" || a.State == "blocked")
}
func (r *messageRouter) discover(d *daemon, now time.Time) {
	d.mu.Lock()
	registrations := make(map[string]string, len(d.regs))
	for id, reg := range d.regs {
		registrations[id] = reg.mailbox
	}
	d.mu.Unlock()
	seen := make(map[string]bool)
	for registration, mailbox := range registrations {
		root, err := os.OpenRoot(mailbox)
		if err != nil {
			continue
		}
		agents, err := recordDirectory(root, "agents")
		root.Close()
		if err != nil {
			continue
		}
		names, err := recordNames(agents, maxIncarnationRecords)
		if err != nil {
			agents.Close()
			continue
		}
		for _, id := range names {
			if !messagingID.MatchString(id) || r.retired[id] {
				continue
			}
			ar, err := recordDirectory(agents, id)
			if err != nil {
				continue
			}
			var p presenceRecord
			info, statErr := ar.Lstat("presence.json")
			err = readRecord(ar, "presence.json", &p)
			if err != nil || statErr != nil || now.Sub(info.ModTime()) > presenceTTL || info.ModTime().After(now.Add(time.Second)) || p.Version != 1 || p.Agent.ID != id || !validIdentity(p.Agent) {
				ar.Close()
				continue
			}
			a := r.agents[id]
			if a != nil && a.registration != registration {
				ar.Close()
				continue
			}
			if a == nil {
				if len(r.agents) >= 256 || len(r.retired) >= maxIncarnationRecords {
					ar.Close()
					continue
				}
				a = &liveAgent{registration: registration, root: ar, requests: make(map[string]*agentResponse)}
				r.agents[id] = a
			} else {
				ar.Close()
			}
			a.identity = p.Agent
			seen[id] = true
		}
		agents.Close()
	}
	for id, a := range r.agents {
		if !seen[id] {
			r.retire(a)
		}
	}
}
func (r *messageRouter) retire(a *liveAgent) {
	delete(r.agents, a.identity.ID)
	r.retired[a.identity.ID] = true
	for id, p := range r.pending {
		if p.source == a || p.recipient == a {
			if p.source != a {
				r.respond(p.source, agentResponse{Version: 1, ID: p.requestID, Status: "indeterminate", MessageID: id, Error: "recipient retired; queue insertion may have occurred; do not resend automatically"})
			}
			r.removeInbox(p)
			delete(r.pending, id)
		}
	}
	a.root.Close()
}
func (r *messageRouter) respond(a *liveAgent, response agentResponse) {
	_, known := a.requests[response.ID]
	cache := known || len(a.requests) < maxIncarnationRecords
	if cache {
		a.requests[response.ID] = &response
	}
	dir, err := recordDirectory(a.root, "responses")
	if err != nil {
		return
	}
	defer dir.Close()
	if err := publishRecord(dir, response.ID+".json", response); err != nil {
		// A large peer list must fail explicitly, never look like an absent daemon.
		fallback := agentResponse{Version: 1, ID: response.ID, Status: "failed", Error: "cannot publish response within record limit"}
		if response.MessageID != "" {
			fallback.Status = "indeterminate"
			fallback.MessageID = response.MessageID
			fallback.Error = "cannot publish delivery response; insertion may have occurred; do not resend automatically"
		}
		if cache {
			a.requests[response.ID] = &fallback
		}
		_ = publishRecord(dir, response.ID+".json", fallback)
	}
}
func (r *messageRouter) removeInbox(p *pendingMessage) {
	dir, err := recordDirectory(p.recipient.root, "inbox")
	if err == nil {
		dir.Remove(p.message.ID + ".json")
		dir.Close()
	}
}
func (r *messageRouter) requests(a *liveAgent) {
	dir, err := recordDirectory(a.root, "requests")
	if err != nil {
		return
	}
	defer dir.Close()
	names, err := recordNames(dir, 256)
	if err != nil {
		return
	}
	for _, name := range names {
		if len(name) != 37 || !messagingID.MatchString(name[:32]) || name[32:] != ".json" {
			continue
		}
		var req agentRequest
		err := readRecord(dir, name, &req)
		dir.Remove(name)
		id := name[:32]
		if cached, ok := a.requests[id]; ok {
			if cached != nil {
				r.respond(a, *cached)
			}
			continue
		}
		if len(a.requests) >= maxIncarnationRecords {
			r.respond(a, agentResponse{Version: 1, ID: id, Status: "failed", Error: "agent request limit reached; reload for a new address"})
			continue
		}
		response := agentResponse{Version: 1, ID: id, Status: "failed"}
		if err != nil || req.Version != 1 || req.ID != id {
			response.Error = "malformed request"
			r.respond(a, response)
			continue
		}
		switch req.Op {
		case "list":
			self := a.identity
			response.Self = &self
			response.Status = "ok"
			for peerID, peer := range r.agents {
				if peerID != a.identity.ID {
					response.Peers = append(response.Peers, peer.identity)
				}
			}
			sort.Slice(response.Peers, func(i, j int) bool { return response.Peers[i].ID < response.Peers[j].ID })
		case "send":
			recipient := r.agents[req.To]
			if req.Text == "" || len(req.Text) > maxTextBytes || !utf8.ValidString(req.Text) || (req.InReplyTo != "" && !messagingID.MatchString(req.InReplyTo)) {
				response.Error = "invalid message"
				break
			}
			if recipient == nil {
				response.Error = "unknown or expired recipient"
				break
			}
			outgoing, incoming := 0, 0
			for _, p := range r.pending {
				if p.source == a {
					outgoing++
				}
				if p.recipient == recipient {
					incoming++
				}
			}
			if outgoing >= maxPending || incoming >= maxPending {
				response.Error = "message queue full"
				break
			}
			inbox, err := recordDirectory(recipient.root, "inbox")
			if err != nil {
				response.Error = "recipient inbox unavailable"
				break
			}
			entries, err := recordNames(inbox, maxPending)
			if err != nil || len(entries) >= maxPending {
				inbox.Close()
				response.Error = "message queue full"
				break
			}
			deadline := time.Now().Add(receiptTimeout)
			message := peerMessage{Version: 1, ID: randomID(), To: req.To, From: a.identity, Text: req.Text, InReplyTo: req.InReplyTo, Expires: deadline.UnixMilli()}
			err = publishRecord(inbox, message.ID+".json", message)
			inbox.Close()
			if err != nil {
				response.Error = "cannot publish message"
				break
			}
			a.requests[id] = nil
			r.pending[message.ID] = &pendingMessage{message: message, source: a, recipient: recipient, requestID: id, deadline: deadline}
			continue
		default:
			response.Error = "unknown messaging operation"
		}
		r.respond(a, response)
	}
}
func (r *messageRouter) receipts(now time.Time) {
	for id, p := range r.pending {
		response := agentResponse{Version: 1, ID: p.requestID, MessageID: id}
		dir, err := recordDirectory(p.recipient.root, "receipts")
		acknowledged := false
		if err == nil {
			var ack receiptRecord
			err = readRecord(dir, id+".json", &ack)
			if err == nil && ack.Version == 1 && ack.ID == id && ack.To == p.recipient.identity.ID && ack.Status == "queued" {
				acknowledged = true
			}
			if err == nil {
				dir.Remove(id + ".json")
			}
			dir.Close()
		}
		if acknowledged {
			response.Status = "queued"
		} else if !now.Before(p.deadline) {
			response.Status = "indeterminate"
			response.Error = "receipt timeout; queue insertion may have occurred; do not resend automatically"
		} else {
			continue
		}
		r.respond(p.source, response)
		r.removeInbox(p)
		delete(r.pending, id)
	}
}
func (r *messageRouter) publishInboxIndexes() {
	for id, a := range r.agents {
		index := inboxIndex{Version: 1, To: id, Messages: make([]string, 0)}
		for messageID, pending := range r.pending {
			if pending.recipient == a {
				index.Messages = append(index.Messages, messageID)
			}
		}
		sort.Strings(index.Messages)
		if a.indexPublished && slices.Equal(a.indexedMessages, index.Messages) {
			continue
		}
		// A well-known file lets sandbox readers open message paths directly.
		// Host-created files can be absent from virtiofs directory listings.
		if publishRecord(a.root, "inbox-index.json", index) == nil {
			a.indexedMessages = index.Messages
			a.indexPublished = true
		}
	}
}

func (d *daemon) runMessaging(ctx context.Context) {
	r := messageRouter{agents: make(map[string]*liveAgent), retired: make(map[string]bool), pending: make(map[string]*pendingMessage)}
	defer func() {
		for _, a := range r.agents {
			a.root.Close()
		}
	}()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			r.discover(d, now)
			r.receipts(now)
			for _, a := range r.agents {
				r.requests(a)
			}
			r.publishInboxIndexes()
		}
	}
}
