package bridge

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const protocolVersion = 4
const controlTimeout = 2 * time.Second

// Control is host-only. The sandbox only receives its mailbox directory.
type controlRequest struct {
	Version int    `json:"version"`
	Op      string `json:"op"`
	Mailbox string `json:"mailbox,omitempty"`
	Socket  string `json:"socket,omitempty"`
	Pane    string `json:"pane,omitempty"`
	Owner   int    `json:"owner,omitempty"`
	ID      string `json:"id,omitempty"`
}

type controlResponse struct {
	Version       int    `json:"version"`
	PID           int    `json:"pid"`
	Registrations int    `json:"registrations"`
	ID            string `json:"id,omitempty"`
	Error         string `json:"error,omitempty"`
}

func runtimeDirectory() (string, error) {
	dir := os.Getenv("PISSBOX_BRIDGE_RUNTIME_DIR")
	if dir == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(cache, "pissbox", "bridge")
	}
	if !filepath.IsAbs(dir) {
		return "", errors.New("bridge runtime directory must be absolute")
	}
	// macOS sockaddr_un has 104 bytes, including the terminating NUL.
	if len(filepath.Join(dir, "control.sock")) > 103 {
		return "", errors.New("bridge control socket path exceeds 103 bytes")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return "", errors.New("bridge runtime directory must be a private directory (0700)")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Getuid()) {
		return "", errors.New("bridge runtime directory has a different owner")
	}
	return dir, nil
}

var errUnavailable = errors.New("bridge daemon unavailable")
var errHandshake = errors.New("invalid bridge handshake")
var errIncompatibleDaemon = errors.New("incompatible bridge daemon; stop it explicitly before restarting")
var errRetrying = errors.New("bridge is restarting; retry shortly")
var errNoReload = errors.New("running bridge daemon does not support reload; stop it once with --bridge-stop, then launch again")

func previousOwnerDead(lock *os.File) bool {
	data, err := io.ReadAll(io.LimitReader(lock, 32))
	pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
	return err == nil && parseErr == nil && pid > 1 && !ownerAlive(pid)
}

func staleLock(dir string) bool {
	lock, err := os.OpenFile(filepath.Join(dir, "daemon.lock"), os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return false
	}
	defer lock.Close()
	if syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return false
	}
	return previousOwnerDead(lock)
}

func controlCall(dir string, req controlRequest) (controlResponse, error) {
	return controlCallVersion(dir, req, protocolVersion)
}

func controlCallVersion(dir string, req controlRequest, version int) (controlResponse, error) {
	var response controlResponse
	conn, err := net.DialTimeout("unix", filepath.Join(dir, "control.sock"), controlTimeout)
	if err != nil {
		// Only a missing/refused socket permits startup. Permission and resource
		// errors must not cause an attempt to replace an unknown live daemon.
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ECONNREFUSED) {
			return response, errUnavailable
		}
		return response, err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(controlTimeout))
	req.Version = version
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return response, fmt.Errorf("%w: %v", errHandshake, err)
	}
	if err := json.NewDecoder(io.LimitReader(conn, 16384)).Decode(&response); err != nil {
		return response, fmt.Errorf("%w: %v", errHandshake, err)
	}
	if response.Version != version {
		return response, errIncompatibleDaemon
	}
	if response.Error != "" {
		if response.Error == errRetrying.Error() {
			return response, errRetrying
		}
		return response, errors.New(response.Error)
	}
	return response, nil
}

func ensureDaemon(ctx context.Context, dir string) error {
	if _, err := controlCall(dir, controlRequest{Op: "status"}); err != errUnavailable {
		if !errors.Is(err, errHandshake) || !staleLock(dir) {
			return err
		}
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer null.Close()
	cmd := exec.Command(executable, "serve")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = null, null, null
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	// Reap the startup child while this CLI lives; the daemon is not owned by it.
	go cmd.Wait()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := controlCall(dir, controlRequest{Op: "status"}); err != errUnavailable {
			if !errors.Is(err, errHandshake) {
				return err
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("bridge daemon did not become ready")
		case <-ticker.C:
		}
	}
}

func controlCLI(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("expected ensure, register, reconnect, unregister, status, reload, or stop")
	}
	if args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "Usage: go-bridge {ensure|status|stop|reload|register|reconnect|unregister}\nregister/reconnect --mailbox DIR --owner-pid PID [--herdr-socket PATH --herdr-pane ID]\nunregister --id ID\nRuntime: per-user cache/pissbox/bridge; override PISSBOX_BRIDGE_RUNTIME_DIR.")
		return nil
	}
	op := args[0]
	flags := flag.NewFlagSet(op, flag.ContinueOnError)
	flags.SetOutput(stderr)
	req := controlRequest{Op: op}
	switch op {
	case "register", "reconnect":
		flags.StringVar(&req.Mailbox, "mailbox", "", "existing mailbox directory")
		flags.StringVar(&req.Socket, "herdr-socket", "", "host Herd socket")
		flags.StringVar(&req.Pane, "herdr-pane", "", "host Herd pane")
		flags.IntVar(&req.Owner, "owner-pid", 0, "launcher PID")
	case "unregister":
		flags.StringVar(&req.ID, "id", "", "opaque registration ID")
	case "ensure", "status", "stop", "reload", "serve":
	default:
		return errors.New("unknown bridge operation")
	}
	if err := flags.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if (op == "register" || op == "reconnect") && (req.Mailbox == "" || req.Owner <= 1 || (req.Socket == "") != (req.Pane == "")) {
		return errors.New("register requires --mailbox and --owner-pid; Herd socket and pane must be supplied together")
	}
	if op == "unregister" && req.ID == "" {
		return errors.New("unregister requires --id")
	}
	if (op == "register" || op == "reconnect") && req.Socket != "" {
		// Relative host socket paths belong to the caller, not the first launcher
		// that happened to start the shared daemon.
		var err error
		req.Socket, err = filepath.Abs(req.Socket)
		if err != nil {
			return err
		}
	}
	dir, err := runtimeDirectory()
	if err != nil {
		return err
	}
	if op == "serve" {
		return serve(ctx, dir)
	}
	if op == "reload" {
		return reloadDaemon(ctx, dir)
	}
	if op == "ensure" || op == "register" {
		if op == "register" {
			response, err := registerWithRetry(ctx, dir, req)
			if err != nil {
				return err
			}
			fmt.Fprintln(stdout, response.ID)
			return nil
		}
		return ensureDaemon(ctx, dir)
	}
	response, err := controlCall(dir, req)
	if op == "stop" && errors.Is(err, errIncompatibleDaemon) {
		// Stopping is always explicit. Try the immediately previous protocol so
		// an upgraded CLI can gracefully stop its old live daemon.
		response, err = controlCallVersion(dir, req, protocolVersion-1)
	}
	if op == "stop" {
		// Explicit stop also clears a handoff left by a failed or interrupted
		// reload, including when no daemon is running to receive the request.
		if removeErr := removeHandoff(dir); err == nil && removeErr != nil {
			return removeErr
		}
	}
	if err != nil {
		return err
	}
	if op == "reconnect" {
		fmt.Fprintln(stdout, response.ID)
	} else if op == "status" {
		return json.NewEncoder(stdout).Encode(response)
	}
	return nil
}

func registerWithRetry(ctx context.Context, dir string, req controlRequest) (controlResponse, error) {
	deadline := time.Now().Add(3 * time.Second)
	var response controlResponse
	for {
		if err := ensureDaemon(ctx, dir); err != nil && !errors.Is(err, errRetrying) {
			return response, err
		}
		var err error
		response, err = controlCall(dir, req)
		if !errors.Is(err, errRetrying) && !errors.Is(err, errUnavailable) {
			return response, err
		}
		if time.Now().After(deadline) {
			return response, err
		}
		select {
		case <-ctx.Done():
			return response, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func lockReleased(dir string) bool {
	lock, err := os.OpenFile(filepath.Join(dir, "daemon.lock"), os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if os.IsNotExist(err) {
		return true
	}
	if err != nil {
		return false
	}
	defer lock.Close()
	if syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return false
	}
	syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	return true
}

// reloadDaemon waits for the old socket and OS lock, not just PID death. A new
// daemon adopts the handoff before binding its listening socket.
func reloadDaemon(ctx context.Context, dir string) error {
	response, err := controlCall(dir, controlRequest{Op: "reload"})
	switch {
	case errors.Is(err, errUnavailable):
		// No live daemon: ensure adopts a leftover handoff or starts cleanly.
	case errors.Is(err, errIncompatibleDaemon):
		return errNoReload
	case err != nil:
		if strings.Contains(err.Error(), "unknown control operation") {
			return errNoReload
		}
		return err
	default:
		deadline := time.NewTimer(5 * time.Second)
		defer deadline.Stop()
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for {
			current, callErr := controlCall(dir, controlRequest{Op: "status"})
			if callErr == nil {
				if current.PID != response.PID {
					break
				}
			} else if errors.Is(callErr, errUnavailable) && lockReleased(dir) {
				break
			} else if !errors.Is(callErr, errUnavailable) && !errors.Is(callErr, errHandshake) {
				return callErr
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-deadline.C:
				return errors.New("bridge reload did not release the old daemon")
			case <-ticker.C:
			}
		}
	}
	return ensureDaemon(ctx, dir)
}

func ownerAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

type registration struct {
	mailbox    string
	owner      int
	generation string
	socket     string
	pane       string
	root       *os.Root
	cancel     context.CancelFunc
	done       chan struct{}
}

type daemon struct {
	mu        sync.Mutex
	regs      map[string]*registration
	ctx       context.Context
	dir       string
	reloading bool
	stopping  bool
	preserve  bool
}

func (d *daemon) preserving() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.preserve
}

// canonicalRegistration accepts only host paths and a live owner. A missing
// Herd socket is not fatal; its worker reports the failure independently.
func canonicalRegistration(req controlRequest) (controlRequest, error) {
	if !filepath.IsAbs(req.Mailbox) || (req.Socket == "") != (req.Pane == "") ||
		(req.Socket != "" && !filepath.IsAbs(req.Socket)) || req.Owner <= 1 || !ownerAlive(req.Owner) {
		return req, errors.New("invalid host registration or dead owner")
	}
	mailbox, err := filepath.EvalSymlinks(req.Mailbox)
	if err != nil || mailbox != filepath.Clean(req.Mailbox) {
		return req, errors.New("mailbox must be an existing canonical directory")
	}
	req.Mailbox = mailbox
	if req.Socket != "" {
		if socket, err := filepath.EvalSymlinks(req.Socket); err == nil {
			req.Socket = socket
		} else if parent, err := filepath.EvalSymlinks(filepath.Dir(req.Socket)); err == nil {
			req.Socket = filepath.Join(parent, filepath.Base(req.Socket))
		}
	}
	return req, nil
}

func (d *daemon) duplicate(req controlRequest) bool {
	for _, r := range d.regs {
		if r.mailbox == req.Mailbox || (req.Socket != "" && r.socket == req.Socket && r.pane == req.Pane) {
			return true
		}
	}
	return false
}

func (d *daemon) launch(r *registration) {
	ctx, cancel := context.WithCancel(d.ctx)
	r.cancel = cancel
	go func() {
		defer cancel()
		b := bridge{root: r.root, warn: func(string) {}, preserve: d.preserving}
		if r.socket != "" {
			b.target = &herdr{socket: r.socket, pane: r.pane}
		}
		// An independent worker bounds the effect of a failed mailbox or Herd socket.
		_ = b.run(ctx, r.owner)
		if !d.preserving() {
			r.root.Remove("ready")
		}
		r.root.Close()
		d.mu.Lock()
		if d.regs[r.generation] == r {
			delete(d.regs, r.generation)
		}
		close(r.done)
		d.mu.Unlock()
	}()
}

func (d *daemon) adopt(records []handoffRegistration) []handoffRegistration {
	d.mu.Lock()
	defer d.mu.Unlock()
	var dead []handoffRegistration
	for _, record := range records {
		if len(d.regs) >= maxRegistrations {
			return dead
		}
		if !messagingID.MatchString(record.ID) {
			continue
		}
		if _, exists := d.regs[record.ID]; exists {
			continue
		}
		if !ownerAlive(record.Owner) {
			dead = append(dead, record)
			continue
		}
		req, err := canonicalRegistration(controlRequest{Mailbox: record.Mailbox, Owner: record.Owner, Socket: record.Socket, Pane: record.Pane})
		if err != nil || d.duplicate(req) {
			continue
		}
		root, err := os.OpenRoot(req.Mailbox)
		if err != nil {
			continue
		}
		info, err := root.Lstat("ready")
		if err != nil || !info.Mode().IsRegular() {
			root.Close()
			continue
		}
		var generation string
		if readRecord(root, "generation", &generation) != nil || generation != record.ID || retired(root, record.ID) {
			root.Close()
			continue
		}
		d.regs[record.ID] = &registration{mailbox: req.Mailbox, owner: req.Owner, generation: record.ID, socket: req.Socket, pane: req.Pane, root: root, done: make(chan struct{})}
	}
	return dead
}

// A launcher that died during the reload gap cannot be adopted. Release its
// Herd binding once and remove readiness so nothing later looks live.
func dropAdoption(record handoffRegistration) {
	if filepath.IsAbs(record.Mailbox) {
		if root, err := os.OpenRoot(record.Mailbox); err == nil {
			root.Remove("ready")
			root.Close()
		}
	}
	if record.Socket != "" && record.Pane != "" && filepath.IsAbs(record.Socket) {
		ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
		defer cancel()
		target := herdr{socket: record.Socket, pane: record.Pane}
		_ = target.Send(ctx, event{Version: 1, Kind: "release"})
	}
}

func (d *daemon) startAdopted() {
	d.mu.Lock()
	pending := make([]*registration, 0, len(d.regs))
	for _, r := range d.regs {
		if r.cancel == nil {
			pending = append(pending, r)
		}
	}
	d.mu.Unlock()
	for _, r := range pending {
		d.launch(r)
	}
}

func (d *daemon) closeAdopted() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for id, r := range d.regs {
		if r.cancel == nil {
			r.root.Close()
			delete(d.regs, id)
		}
	}
}

func (d *daemon) reload() error {
	d.mu.Lock()
	if d.reloading || d.stopping {
		d.mu.Unlock()
		return errors.New("bridge is already shutting down")
	}
	d.reloading = true
	records := make([]handoffRegistration, 0, len(d.regs))
	for id, r := range d.regs {
		records = append(records, handoffRegistration{ID: id, Mailbox: r.mailbox, Owner: r.owner, Socket: r.socket, Pane: r.pane})
	}
	d.mu.Unlock()
	if err := writeHandoff(d.dir, records); err != nil {
		d.mu.Lock()
		d.reloading = false
		d.mu.Unlock()
		return errors.New("cannot write bridge reload handoff")
	}
	d.mu.Lock()
	d.preserve = true
	d.mu.Unlock()
	return nil
}

func (d *daemon) register(req controlRequest) (string, error) {
	req, err := canonicalRegistration(req)
	if err != nil {
		return "", err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.reloading || d.stopping {
		return "", errRetrying
	}
	for id, r := range d.regs {
		if r.mailbox == req.Mailbox || (req.Socket != "" && r.socket == req.Socket && r.pane == req.Pane) {
			if req.Op == "reconnect" && r.mailbox == req.Mailbox && r.owner == req.Owner && r.socket == req.Socket && r.pane == req.Pane {
				return id, nil
			}
			return "", errors.New("mailbox or Herd pane already registered")
		}
	}
	if len(d.regs) >= maxRegistrations {
		return "", errors.New("too many bridge registrations")
	}
	root, err := os.OpenRoot(req.Mailbox)
	if err != nil {
		return "", errors.New("cannot open mailbox")
	}
	if err := markReady(root); err != nil {
		root.Close()
		return "", errors.New("cannot mark mailbox ready")
	}
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		root.Remove("ready")
		root.Close()
		return "", err
	}
	id := hex.EncodeToString(token)
	if err := publishRecord(root, "generation", id); err != nil {
		root.Remove("ready")
		root.Close()
		return "", errors.New("cannot publish mailbox generation")
	}
	r := &registration{mailbox: req.Mailbox, owner: req.Owner, generation: id, socket: req.Socket, pane: req.Pane, root: root, done: make(chan struct{})}
	d.regs[id] = r
	d.launch(r)
	return id, nil
}

func (d *daemon) handle(req controlRequest) controlResponse {
	response := controlResponse{Version: protocolVersion, PID: os.Getpid()}
	if req.Version != protocolVersion {
		response.Error = "incompatible control version"
		return response
	}
	switch req.Op {
	case "register", "reconnect":
		id, err := d.register(req)
		if err != nil {
			response.Error = err.Error()
		}
		response.ID = id
	case "unregister":
		d.mu.Lock()
		r := d.regs[req.ID]
		d.mu.Unlock()
		if r != nil {
			if root, err := os.OpenRoot(r.mailbox); err == nil {
				writeRetired(root, req.ID)
				root.Close()
			}
			r.cancel()
			<-r.done
			// Unregistration removes readiness even if reload preservation was active.
			if root, err := os.OpenRoot(r.mailbox); err == nil {
				root.Remove("ready")
				root.Close()
			}
		}
	case "reload":
		if err := d.reload(); err != nil {
			response.Error = err.Error()
		}
	case "status":
	case "stop":
		d.mu.Lock()
		if d.reloading || d.stopping {
			response.Error = "bridge is already shutting down"
		} else {
			d.stopping = true
		}
		d.mu.Unlock()
		if response.Error == "" {
			// An explicit stop must never replay an old reload snapshot later.
			removeHandoff(d.dir)
		}
	default:
		response.Error = "unknown control operation"
	}
	d.mu.Lock()
	response.Registrations = len(d.regs)
	d.mu.Unlock()
	return response
}

func serve(ctx context.Context, dir string) error {
	lock, err := os.OpenFile(filepath.Join(dir, "daemon.lock"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return err
	}
	// Some mounted filesystems leave a forwarding socket that resets rather than
	// refuses after a crash. Recovery still requires the OS lock and a dead owner.
	deadOwner := previousOwnerDead(lock)
	if _, err := controlCall(dir, controlRequest{Op: "status"}); err != errUnavailable && !(errors.Is(err, errHandshake) && deadOwner) {
		if err == nil {
			return errors.New("a live bridge already owns the socket")
		}
		return err
	}
	snapshot, err := readHandoff(dir)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "control.sock")
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return errors.New("refusing to remove a non-socket control path")
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := lock.Truncate(0); err != nil {
		return err
	}
	if _, err := lock.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	d := &daemon{ctx: ctx, regs: make(map[string]*registration), dir: dir}
	if snapshot != nil {
		for _, record := range d.adopt(snapshot.Registrations) {
			dropAdoption(record)
		}
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		d.closeAdopted()
		return err
	}
	defer listener.Close()
	if err := os.Chmod(path, 0600); err != nil {
		d.closeAdopted()
		return err
	}
	d.startAdopted()
	messagingDone := make(chan struct{})
	go func() {
		defer close(messagingDone)
		d.runMessaging(ctx)
	}()
	if snapshot != nil {
		// The handoff is the commit point: only delete it after adopted workers
		// and the replacement listener are running.
		if err := removeHandoff(dir); err != nil {
			d.mu.Lock()
			d.preserve = true
			d.mu.Unlock()
			return fmt.Errorf("cannot consume bridge reload handoff: %w", err)
		}
	}
	var handlers sync.WaitGroup
	go func() {
		<-ctx.Done()
		listener.Close()
	}()
	// Bound concurrent control clients, including clients sending no data.
	slots := make(chan struct{}, 32)
	for {
		conn, err := listener.AcceptUnix()
		if err != nil {
			break
		}
		select {
		case slots <- struct{}{}:
		default:
			conn.Close()
			continue
		}
		handlers.Add(1)
		go func() {
			defer handlers.Done()
			defer func() { <-slots }()
			defer conn.Close()
			conn.SetDeadline(time.Now().Add(controlTimeout))
			var req controlRequest
			decoder := json.NewDecoder(io.LimitReader(conn, 16384))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&req) != nil {
				return
			}
			response := d.handle(req)
			json.NewEncoder(conn).Encode(response)
			if response.Error == "" && req.Version == protocolVersion && (req.Op == "stop" || req.Op == "reload") {
				cancel()
			}
		}()
	}
	cancel()
	<-messagingDone
	handlers.Wait()
	d.mu.Lock()
	pending := make([]*registration, 0, len(d.regs))
	for _, r := range d.regs {
		pending = append(pending, r)
	}
	d.mu.Unlock()
	for _, r := range pending {
		<-r.done
	}
	return nil
}
