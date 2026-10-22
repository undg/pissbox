package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type bridge struct {
	root      *os.Root
	target    adapter
	warn      func(string)
	attempted bool
	warned    bool
}

func (b *bridge) warning(message string) {
	if !b.warned {
		b.warn(message)
		b.warned = true
	}
}

func (b *bridge) poll(ctx context.Context) error {
	names, err := eventNames(b.root)
	if err != nil {
		return err
	}
	for i, name := range names {
		if i == 64 || ctx.Err() != nil {
			break
		}
		e, err := readEvent(b.root, name)
		if err != nil {
			// Do not log sandbox-provided contents, filenames, or error strings.
			b.warning("discarding an invalid mailbox event")
		} else {
			b.attempted = true
			if err := b.target.Send(ctx, e); err != nil {
				b.warning("Herd reporting unavailable; queued events will retry")
				return nil
			}
			b.attempted = e.Kind != "release"
			b.warned = false
		}
		if err := b.root.Remove(name); err != nil {
			return fmt.Errorf("cannot consume mailbox event")
		}
	}
	return nil
}

func (b *bridge) run(ctx context.Context, parentPID int) error {
	if err := markReady(b.root); err != nil {
		return fmt.Errorf("cannot create mailbox readiness marker")
	}
	defer b.root.Remove("ready")
	defer func() {
		// Pick up the final idle/release event even if Pi exits between polls.
		finish, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
		defer cancel()
		if err := b.poll(finish); err != nil {
			b.warning("cannot drain the mailbox during shutdown")
		}
		if b.attempted {
			if err := b.target.Send(context.Background(), event{Version: 1, Kind: "release"}); err != nil {
				b.warning("could not release Herd agent; idle-shell detection remains the fallback")
			}
		}
	}()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	lastHeartbeat := time.Now()
	for {
		if parentPID != 0 && os.Getppid() != parentPID {
			return nil
		}
		if err := b.poll(ctx); err != nil {
			return err
		}
		if time.Since(lastHeartbeat) >= time.Second {
			if err := markReady(b.root); err != nil {
				return fmt.Errorf("cannot refresh mailbox readiness marker")
			}
			lastHeartbeat = time.Now()
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func run(args []string, stderr io.Writer) error {
	flags := flag.NewFlagSet("go-bridge", flag.ContinueOnError)
	flags.SetOutput(stderr)
	mailbox := flags.String("mailbox", "", "dedicated shared mailbox directory")
	socket := flags.String("herdr-socket", "", "host Herd Unix socket")
	pane := flags.String("herdr-pane", "", "launching Herd pane ID (host-owned)")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if *mailbox == "" || *socket == "" || *pane == "" || flags.NArg() != 0 {
		return fmt.Errorf("--mailbox, --herdr-socket and --herdr-pane are required; no positional arguments")
	}
	root, err := os.OpenRoot(*mailbox)
	if err != nil {
		return fmt.Errorf("cannot open mailbox directory")
	}
	defer root.Close()
	logger := log.New(stderr, "pissbox bridge: ", 0)
	b := bridge{
		root:   root,
		target: &herdr{socket: *socket, pane: *pane},
		warn:   func(message string) { logger.Println(message) },
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return b.run(ctx, os.Getppid())
}

func main() {
	if err := run(os.Args[1:], os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "pissbox bridge:", err)
		os.Exit(1)
	}
}
