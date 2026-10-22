package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"unicode"
	"unicode/utf8"
)

const maxEventBytes = 2048

var eventName = regexp.MustCompile(`^[0-9]{20}-[0-9a-f]{16}\.json$`)

type event struct {
	Version int    `json:"version"`
	Kind    string `json:"kind"`
	State   string `json:"state,omitempty"`
	Message string `json:"message,omitempty"`
}

func (e event) validate() error {
	if e.Version != 1 || len(e.Message) > 512 || !utf8.ValidString(e.Message) ||
		strings.IndexFunc(e.Message, unicode.IsControl) >= 0 {
		return errors.New("invalid event version or message")
	}
	switch e.Kind {
	case "state":
		if e.State == "idle" || e.State == "working" || e.State == "blocked" {
			return nil
		}
	case "release":
		if e.State == "" && e.Message == "" {
			return nil
		}
	}
	return errors.New("invalid lifecycle event")
}

func eventNames(root *os.Root) ([]string, error) {
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(4097)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if len(entries) > 4096 {
		return nil, errors.New("mailbox has too many entries")
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if eventName.MatchString(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func readEvent(root *os.Root, name string) (event, error) {
	var e event
	// Refuse symlinks and avoid blocking on a FIFO supplied by the sandbox.
	file, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return e, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxEventBytes {
		return e, errors.New("event is not a bounded regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxEventBytes+1))
	if err != nil || len(data) > maxEventBytes {
		return e, errors.New("cannot read bounded event")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&e); err != nil {
		return e, errors.New("invalid event JSON")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return e, errors.New("trailing event data")
	}
	return e, e.validate()
}

func markReady(root *os.Root) error {
	file, err := root.OpenFile("ready", os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("ready marker is not a regular file")
	}
	_, err = fmt.Fprintln(file, "1")
	return err
}
