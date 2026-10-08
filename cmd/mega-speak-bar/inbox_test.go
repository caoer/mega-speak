package main

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestArchivePreservesLogAndIndividualLegacyEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "said.jsonl")
	raw := []byte("{\"ts\":\"2026-10-08T10:00:00Z\",\"text\":\"same\"}\n{\"ts\":\"2026-10-08T10:00:00Z\",\"text\":\"same\"}\n")
	os.WriteFile(path, raw, 0600)
	entries, err := readInboxLog(path)
	if err != nil || len(entries) != 2 {
		t.Fatal(entries, err)
	}
	if entries[0].ID == entries[1].ID {
		t.Fatal("identical legacy entries need distinct identity")
	}
	state, err := loadInbox(filepath.Join(dir, "inbox.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = state.set([]string{entries[0].ID}, true); err != nil {
		t.Fatal(err)
	}
	state, err = loadInbox(state.path)
	if err != nil || !state.Archived[entries[0].ID] || state.Archived[entries[1].ID] {
		t.Fatal(state, err)
	}
	bytes, _ := os.ReadFile(path)
	if string(bytes) != string(raw) {
		t.Fatal("archive rewrote source log")
	}
	if err = state.set([]string{entries[0].ID}, false); err != nil || len(state.Archived) != 0 {
		t.Fatal(state, err)
	}
	os.WriteFile(state.path, []byte("broken"), 0600)
	if _, err = loadInbox(state.path); err == nil {
		t.Fatal("corrupt state must be reported")
	}
}
func TestSingleInstanceRedirectsViewAndReleasesLock(t *testing.T) {
	// Nix's build HOME is intentionally unwritable. A short private home
	// also keeps the Unix socket under Darwin's 104-byte path limit.
	home, err := os.MkdirTemp(".", ".instance-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(home)
	t.Setenv("HOME", home)
	dir := t.TempDir()
	lock, err := acquireInstance(dir, "inbox")
	if err != nil || lock == nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", instanceSocket(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	received := make(chan string, 1)
	go func() {
		c, e := listener.Accept()
		if e != nil {
			return
		}
		defer c.Close()
		c.SetReadDeadline(time.Now().Add(time.Second))
		b := make([]byte, 100)
		n, _ := c.Read(b)
		received <- string(b[:n])
	}()
	second, err := acquireInstance(dir, "settings")
	if err != nil || second != nil {
		t.Fatal(second, err)
	}
	if got := <-received; got != "settings\n" {
		t.Fatal(got)
	}
	lock.Close()
	third, err := acquireInstance(dir, "inbox")
	if err != nil || third == nil {
		t.Fatal(third, err)
	}
	third.Close()
}
