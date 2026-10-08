// Command mega-speak-bar is Mega Speak's menu bar app: mega-speak appends one
// JSON line per utterance to a log, and a click on the menu bar icon opens a
// panel with the newest first — kind, time, agent, text, emotion. A click on
// a line focuses the herdr pane of the agent that said it.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"
	"time"
)

// entry is what the panel reads from one line of the log.
type entry struct {
	ID          string    `json:"id"`
	TS          time.Time `json:"ts"`
	Text        string    `json:"text"`
	Emotion     string    `json:"emotion"`
	Instruction string    `json:"instruction"` // a custom voice instruction; Emotion is "custom"
	Host        string    `json:"host"`        // the caller's host
	Session     string    `json:"session"`     // the caller's agent session id
	Pane        string    `json:"pane"`        // the caller's herdr pane id
	DryRun      bool      `json:"dry_run"`     // synthesized, not played
}

// tailBytes bounds how much of the log a read looks at: about a thousand lines.
const tailBytes = 512 << 10

// readLog returns the log's last n entries, newest first. A missing log is
// no entries; a line that does not parse is skipped.
func readLog(path string, n int) ([]entry, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	off := max(st.Size()-tailBytes, 0)
	b, err := io.ReadAll(io.NewSectionReader(f, off, st.Size()-off))
	if err != nil {
		return nil, err
	}
	if off > 0 { // the first line is cut
		if i := bytes.IndexByte(b, '\n'); i >= 0 {
			b = b[i+1:]
		}
	}
	return parseLog(b, n), nil
}

func parseLog(b []byte, n int) []entry {
	var all []entry
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 64<<10), tailBytes)
	for sc.Scan() {
		var e entry
		if json.Unmarshal(sc.Bytes(), &e) != nil || strings.TrimSpace(e.Text) == "" {
			continue
		}
		all = append(all, e)
	}
	out := make([]entry, 0, min(len(all), n))
	for i := len(all) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, all[i])
	}
	return out
}
