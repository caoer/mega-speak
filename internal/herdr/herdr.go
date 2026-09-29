// Package herdr is the little of herdr's CLI and API socket the menu bar app
// uses: the pane list and focusing a pane.
package herdr

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"
)

// Client drives the herdr CLI at Bin.
type Client struct{ Bin string }

// wait bounds each call: a run of the CLI, or a request on the socket.
const wait = 3 * time.Second

// Pane is one herdr pane: its id, the session id its agent reported, and
// its sidebar tokens.
type Pane struct {
	ID      string
	Session string
	Tokens  map[string]string
}

func (c Client) run(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Bin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("herdr %s: %v %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// Panes lists herdr's panes.
func (c Client) Panes() ([]Pane, error) {
	out, err := c.run("pane", "list")
	if err != nil {
		return nil, err
	}
	return parsePanes(out)
}

func parsePanes(out []byte) ([]Pane, error) {
	var pl struct {
		Result struct {
			Panes []struct {
				PaneID       string `json:"pane_id"`
				AgentSession *struct {
					Value string `json:"value"`
				} `json:"agent_session"`
				Tokens map[string]any `json:"tokens"` // display metadata: a value of any type never fails the list
			} `json:"panes"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &pl); err != nil {
		return nil, fmt.Errorf("herdr pane list: %w", err)
	}
	panes := make([]Pane, 0, len(pl.Result.Panes))
	for _, p := range pl.Result.Panes {
		q := Pane{ID: p.PaneID}
		if p.AgentSession != nil {
			q.Session = p.AgentSession.Value
		}
		for k, v := range p.Tokens {
			if v, ok := v.(string); ok {
				if q.Tokens == nil {
					q.Tokens = map[string]string{}
				}
				q.Tokens[k] = v
			}
		}
		panes = append(panes, q)
	}
	return panes, nil
}

// Focus makes pane herdr's focused pane, switching to its workspace and tab,
// through the API's pane.focus.
func (c Client) Focus(pane string) error {
	out, err := c.run("status", "server")
	if err != nil {
		return err
	}
	var sock string
	for line := range strings.Lines(string(out)) {
		if p, ok := strings.CutPrefix(line, "socket:"); ok {
			sock = strings.TrimSpace(p)
		}
	}
	if sock == "" {
		return errors.New("herdr status server: no socket line")
	}
	return request(sock, "pane.focus", map[string]string{"pane_id": pane})
}

// request makes one request on herdr's API socket.
func request(sock, method string, params any) error {
	conn, err := net.DialTimeout("unix", sock, wait)
	if err != nil {
		return fmt.Errorf("herdr socket: %w", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(wait))
	req, err := json.Marshal(map[string]any{"id": "mega-speak", "method": method, "params": params})
	if err != nil {
		return err
	}
	if _, err := conn.Write(append(req, '\n')); err != nil {
		return fmt.Errorf("herdr %s: %w", method, err)
	}
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return fmt.Errorf("herdr %s: %w", method, err)
	}
	var resp struct {
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(line, &resp); err != nil {
		return fmt.Errorf("herdr %s: %w", method, err)
	}
	if resp.Error != nil {
		return fmt.Errorf("herdr %s: %s %s", method, resp.Error.Code, resp.Error.Message)
	}
	return nil
}
