package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadLogNewestFirstFromTheTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spoken.jsonl")
	var b strings.Builder
	b.WriteString(strings.Repeat("x", tailBytes) + "\n") // cut by the tail read: never parsed
	b.WriteString(`{"ts":"2020-03-01T08:00:00Z","text":"alpha"}` + "\n")
	b.WriteString("not json\n")
	b.WriteString(`{"ts":"2020-03-01T08:01:00Z","text":"  "}` + "\n")
	b.WriteString(`{"ts":"2020-03-01T08:02:00Z","text":"beta","dry_run":true}` + "\n")
	b.WriteString(`{"ts":"2020-03-01T08:03:00Z","text":"gamma","session":"s-3"}` + "\n")
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	es, err := readLog(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 2 || es[0].Text != "gamma" || es[0].Session != "s-3" || es[1].Text != "beta" || !es[1].DryRun {
		t.Fatalf("got %+v", es)
	}
	if es, err := readLog(filepath.Join(t.TempDir(), "none"), 5); err != nil || es != nil {
		t.Fatalf("missing log: %v %v", es, err)
	}
}

func TestLocate(t *testing.T) {
	cards := t.TempDir()
	os.MkdirAll(filepath.Join(cards, "feed0001"), 0o755)
	os.WriteFile(filepath.Join(cards, "feed0001", "feed0001.md"), []byte("---\nrole: scout\nsession: \"harbor-lights\"\n---\nbody\n"), 0o644)
	p := panes{
		bySession: map[string]string{"feed0001-live": "w1:p2", "cafe0002-live": "w3:p4"},
		exists:    map[string]bool{"w1:p2": true, "w3:p4": true, "w5:p6": true},
		tokens:    map[string]map[string]string{"cafe0002-live": {"role_pilot": "pilot", "session": "kite-field", "n": "x"}},
	}
	for _, c := range []struct {
		name string
		e    entry
		p    panes
		want where
	}{
		{"live session, card label", entry{Session: "feed0001-live", Host: "mac"}, p,
			where{label: "scout · harbor-lights", id: "feed0001", pane: "w1:p2", status: "w1:p2"}},
		{"live session on another host, token label", entry{Session: "cafe0002-live", Host: "box"}, p,
			where{label: "pilot · kite-field", id: "cafe0002", pane: "w3:p4", status: "w3:p4"}},
		{"gone session keeps its card label", entry{Session: "feed0001-gone", Host: "mac", Pane: "w5:p6"}, p,
			where{label: "scout · harbor-lights", id: "feed0001", status: "agent gone"}},
		{"session on another host", entry{Session: "beef0003-x", Host: "box"}, p,
			where{id: "beef0003", status: "on box"}},
		{"no session: this host's pane", entry{Host: "mac", Pane: "w5:p6"}, p,
			where{pane: "w5:p6", status: "w5:p6"}},
		{"no session: another host's pane id is not ours", entry{Host: "box", Pane: "w5:p6"}, p,
			where{status: "on box"}},
		{"no session: closed pane", entry{Host: "mac", Pane: "w9:p9"}, p,
			where{status: "agent gone"}},
		{"nothing recorded", entry{Host: "unknown"}, p,
			where{status: "no agent recorded"}},
		{"herdr down", entry{Session: "feed0001-live", Host: "mac"}, panes{err: errors.New("no socket")},
			where{label: "scout · harbor-lights", id: "feed0001"}},
	} {
		if got := locate(c.e, c.p, "mac", cards); got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestAddAgentsFillsOnlyWhatHerdrLacks(t *testing.T) {
	p := panes{bySession: map[string]string{"s1": "w1:p1"}, exists: map[string]bool{"w1:p1": true, "w2:p2": true}}
	p.addAgents([]byte(`{"result":{"agents":[
		{"agent_id":"s1","pane_id":"w2:p2","substrate":"herdr"},
		{"agent_id":"s2","pane_id":"w2:p2","substrate":"herdr"},
		{"agent_id":"s3","pane_id":"%3","substrate":"tmux"},
		{"agent_id":"s4","pane_id":"w8:p8","substrate":"herdr"}]}}`))
	if p.bySession["s1"] != "w1:p1" || p.bySession["s2"] != "w2:p2" || p.bySession["s3"] != "" || p.bySession["s4"] != "" {
		t.Fatalf("got %v", p.bySession)
	}
}

func TestAge(t *testing.T) {
	now := time.Date(2020, 3, 5, 10, 0, 0, 0, time.Local)
	for _, c := range []struct {
		ago  time.Duration
		want string
	}{
		{20 * time.Second, "just now"}, {5 * time.Minute, "5 min ago"}, {3 * time.Hour, "3 h ago"},
		{11 * time.Hour, "yesterday"}, {40 * time.Hour, "Mar 3"},
	} {
		if got := age(now.Add(-c.ago), now); got != c.want {
			t.Errorf("%v ago: got %q, want %q", c.ago, got, c.want)
		}
	}
}
