package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Archive decisions are separate from the append-only playback log.
type inboxState struct {
	path     string
	Archived map[string]bool `json:"archived"`
}

func loadInbox(path string) (*inboxState, error) {
	s := &inboxState{path: path, Archived: map[string]bool{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(b, s); err != nil {
		return nil, fmt.Errorf("cannot read archive state: %w", err)
	}
	if s.Archived == nil {
		s.Archived = map[string]bool{}
	}
	return s, nil
}
func (s *inboxState) set(ids []string, archived bool) error {
	next := map[string]bool{}
	for k, v := range s.Archived {
		next[k] = v
	}
	for _, id := range ids {
		if archived {
			next[id] = true
		} else {
			delete(next, id)
		}
	}
	b, err := json.Marshal(struct {
		Archived map[string]bool `json:"archived"`
	}{next})
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".inbox-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(f.Name(), s.path); err != nil {
		return err
	}
	s.Archived = next
	return nil
}
func readInboxLog(path string) ([]entry, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), tailBytes)
	var all []entry
	var offset int64
	for sc.Scan() {
		b := sc.Bytes()
		var e entry
		if json.Unmarshal(b, &e) == nil && e.Text != "" {
			if e.ID == "" {
				h := sha256.Sum256(b)
				e.ID = fmt.Sprintf("legacy-%d-%s", offset, hex.EncodeToString(h[:12]))
			}
			all = append(all, e)
		}
		offset += int64(len(b) + 1)
	}
	if err = sc.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	return all, nil
}
