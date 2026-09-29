package main

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/caoer/mega-speak/internal/herdr"
)

// panes is where the live agents are: herdr's panes and, from an optional
// fleet command, more session → pane pairs herdr's own list lacks.
type panes struct {
	bySession map[string]string // agent session id → pane id
	exists    map[string]bool   // every pane id herdr has
	tokens    map[string]map[string]string
	err       error // herdr did not answer: nothing can be focused
}

// livePanes asks herdr for its panes and, when agentsCmd is set, runs it for
// {"result":{"agents":[{"agent_id","pane_id","substrate"}]}} too; its
// failure only loses its pairs.
func livePanes(h herdr.Client, agentsCmd string) panes {
	p := panes{bySession: map[string]string{}, exists: map[string]bool{}, tokens: map[string]map[string]string{}}
	ps, err := h.Panes()
	if err != nil {
		p.err = err
		return p
	}
	for _, q := range ps {
		p.exists[q.ID] = true
		if q.Session != "" {
			p.bySession[q.Session] = q.ID
			p.tokens[q.Session] = q.Tokens
		}
	}
	if agentsCmd != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if out, err := exec.CommandContext(ctx, agentsCmd, "list").Output(); err == nil {
			p.addAgents(out)
		}
	}
	return p
}

// addAgents adds the fleet list's herdr pairs for sessions herdr did not name.
func (p panes) addAgents(out []byte) {
	var l struct {
		Result struct {
			Agents []struct {
				ID        string `json:"agent_id"`
				Pane      string `json:"pane_id"`
				Substrate string `json:"substrate"`
			} `json:"agents"`
		} `json:"result"`
	}
	if json.Unmarshal(out, &l) != nil {
		return
	}
	for _, a := range l.Result.Agents {
		if a.ID != "" && a.Substrate == "herdr" && p.exists[a.Pane] && p.bySession[a.ID] == "" {
			p.bySession[a.ID] = a.Pane
		}
	}
}

// where is a line's agent as the panel shows it.
type where struct {
	label  string // "role · session", "" when unknown
	id     string // the session's short id
	pane   string // the pane a click focuses; "" when none
	status string // the pane id, "agent gone", "on <host>", "" before herdr answered
}

// locate finds e's agent: the live pane of its session, else — for a caller
// on this host with no session — the pane it spoke from, if herdr still has
// it. A session id is unique across hosts; a pane id only within one herdr.
func locate(e entry, p panes, local string, cards string) where {
	w := where{id: short(e.Session)}
	w.label = cardLabel(cards, w.id)
	if w.label == "" {
		w.label = tokenLabel(p.tokens[e.Session])
	}
	if p.err != nil {
		return w
	}
	here := e.Host == "" || e.Host == local
	switch {
	case e.Session != "":
		w.pane = p.bySession[e.Session]
	case e.Pane != "" && here && p.exists[e.Pane]:
		w.pane = e.Pane
	}
	switch {
	case w.pane != "":
		w.status = w.pane
	case e.Session == "" && e.Pane == "":
		w.status = "no agent recorded"
	case !here && e.Host != "unknown":
		w.status = "on " + e.Host
	default:
		w.status = "agent gone"
	}
	return w
}

func short(session string) string {
	if len(session) > 8 {
		return session[:8]
	}
	return session
}

// cardLabel is "role · session" from an agent card's frontmatter at
// <cards>/<id>/<id>.md, or "".
func cardLabel(cards, id string) string {
	if cards == "" || id == "" {
		return ""
	}
	f, err := os.Open(filepath.Join(cards, id, id+".md"))
	if err != nil {
		return ""
	}
	defer f.Close()
	fm := map[string]string{}
	sc := bufio.NewScanner(f)
	for n := 0; sc.Scan(); n++ {
		line := sc.Text()
		if line == "---" {
			if n == 0 {
				continue
			}
			break
		}
		if n == 0 {
			return "" // no frontmatter
		}
		if k, v, ok := strings.Cut(line, ":"); ok {
			fm[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return join(fm["role"], fm["session"])
}

// tokenLabel is "role · session" from herdr's sidebar tokens, or "".
func tokenLabel(t map[string]string) string {
	var roles []string
	for k, v := range t {
		if strings.HasPrefix(k, "role_") && v != "" {
			roles = append(roles, v)
		}
	}
	slices.Sort(roles)
	return join(strings.Join(roles, " "), t["session"])
}

func join(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + " · " + b
}
