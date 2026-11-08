package bridge

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"syscall"
)

const handoffVersion = 1
const maxHandoffBytes = 256 * 1024
const maxRegistrations = 256

type handoffRegistration struct {
	ID      string `json:"id"`
	Mailbox string `json:"mailbox"`
	Owner   int    `json:"owner"`
	Socket  string `json:"socket,omitempty"`
	Pane    string `json:"pane,omitempty"`
}

type handoffFile struct {
	Version       int                   `json:"version"`
	Registrations []handoffRegistration `json:"registrations"`
}

type retiredRecord struct {
	Version int    `json:"version"`
	ID      string `json:"id"`
}

func handoffPath(dir string) string {
	return filepath.Join(dir, "handoff.json")
}

// writeHandoff replaces the snapshot atomically. It only contains host-side
// registration binding, never mailbox contents.
func writeHandoff(dir string, registrations []handoffRegistration) error {
	records := append([]handoffRegistration(nil), registrations...)
	sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
	data, err := json.Marshal(handoffFile{Version: handoffVersion, Registrations: records})
	if err != nil || len(data) > maxHandoffBytes {
		return errors.New("bridge reload handoff is too large")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	temporary := "." + randomID() + ".pending"
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(temporary)
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return root.Rename(temporary, "handoff.json")
}

func readHandoff(dir string) (*handoffFile, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file, err := root.OpenFile("handoff.json", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxHandoffBytes {
		return nil, errors.New("invalid bridge reload handoff")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxHandoffBytes+1))
	if err != nil || len(data) > maxHandoffBytes {
		return nil, errors.New("invalid bridge reload handoff")
	}
	var value handoffFile
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil || decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("invalid bridge reload handoff")
	}
	if value.Version != handoffVersion || len(value.Registrations) > maxRegistrations {
		return nil, errors.New("unsupported bridge reload handoff")
	}
	return &value, nil
}

func removeHandoff(dir string) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	err = root.Remove("handoff.json")
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func writeRetired(root *os.Root, id string) error {
	return publishRecord(root, "retired", retiredRecord{Version: 1, ID: id})
}

func retired(root *os.Root, id string) bool {
	var marker retiredRecord
	return readRecord(root, "retired", &marker) == nil && marker.Version == 1 && marker.ID == id
}
