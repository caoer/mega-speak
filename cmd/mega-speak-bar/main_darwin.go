//go:build darwin

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/caoer/mega-speak/internal/herdr"
	"github.com/caoer/mega-speak/internal/mac"
)

const usage = `mega-speak-bar — Mega Speak in the menu bar: what was said aloud

usage: mega-speak-bar [flags]

The icon opens a panel with the log's lines, newest first: time, agent, text,
emotion. A click on a line focuses the herdr pane of the agent that said it
and brings the terminal that shows herdr forward.

flags:
`

// version is set at build time.
var version = "dev"

// app is the running panel: the log it reads and where it finds agents.
type app struct {
	log, cards, agents, terminal, local string
	herdr                               herdr.Client
	board                               *mac.Board
	wake                                chan struct{}

	mu    sync.Mutex
	open  bool
	panes panes // the last answer from herdr

	lines []line // main thread only: the rows on screen
}

func main() {
	a := &app{wake: make(chan struct{}, 1)}
	var herdrBin string
	var show bool
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage); flag.PrintDefaults() }
	home, _ := os.UserHomeDir()
	flag.StringVar(&a.log, "log", filepath.Join(home, ".local/share/mega-speak/said.jsonl"), "the log mega-speak writes")
	flag.StringVar(&a.cards, "cards", "", "folder of agent cards, <id>/<id>.md with role and session in frontmatter, for agent names")
	flag.StringVar(&herdrBin, "herdr", "herdr", "the herdr CLI")
	flag.StringVar(&a.agents, "agents", "", "a command whose `list` prints {result:{agents:[{agent_id,pane_id,substrate}]}}, for sessions herdr does not name")
	flag.StringVar(&a.terminal, "terminal", "", "bundle id of the terminal that shows herdr, brought forward on a click")
	flag.BoolVar(&show, "show", false, "open the panel at start and log its window number")
	flag.BoolFunc("version", "print the version", func(string) error { fmt.Println("mega-speak-bar", version); os.Exit(0); return nil })
	flag.Parse()
	if flag.NArg() > 0 {
		flag.Usage()
		os.Exit(2)
	}
	a.herdr = herdr.Client{Bin: herdrBin}
	host, _ := os.Hostname()
	a.local, _, _ = strings.Cut(host, ".")
	mac.Run(func() {
		a.board = mac.NewBoard("text.bubble", "Mega Speak", []mac.Column{
			{Width: 18},
			{Title: "Time", Width: 72, Mono: true},
			{Title: "Agent", Width: 196},
			{Title: "Text", Wrap: true},
			{Title: "Emotion", Width: 82},
		}, 800, 540)
		a.board.OnOpen, a.board.OnClose, a.board.OnClick = a.opened, a.closed, a.clicked
		log.Printf("mega-speak-bar %s: %s, host %s", version, a.log, a.local)
		go a.loop()
		if show {
			time.Sleep(time.Second) // the new icon takes its place in the menu bar first
			a.board.Open()
			time.Sleep(time.Second)
			log.Printf("panel window %d", a.board.WindowNumber())
		}
	})
}

// opened renders the log at once with the last known panes, then asks herdr.
func (a *app) opened() {
	a.mu.Lock()
	a.open = true
	p := a.panes
	a.mu.Unlock()
	a.render(p)
	a.poke()
}

func (a *app) closed() {
	a.mu.Lock()
	a.open = false
	a.mu.Unlock()
}

func (a *app) poke() {
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

// loop asks herdr where the agents are at every open, and every 2 s while
// the panel is open, rendering after each answer.
func (a *app) loop() {
	for {
		a.mu.Lock()
		open := a.open
		a.mu.Unlock()
		if open {
			select {
			case <-a.wake:
			case <-time.After(2 * time.Second):
			}
		} else {
			<-a.wake
		}
		p := livePanes(a.herdr, a.agents)
		a.mu.Lock()
		a.panes = p
		a.mu.Unlock()
		a.render(p)
	}
}

// render reads the log and puts it on the board with where each agent is.
func (a *app) render(p panes) {
	es, err := readLog(a.log, 300)
	now := time.Now()
	ls := make([]line, len(es))
	rows := make([][]mac.Cell, len(es))
	for i, e := range es {
		ls[i] = lineOf(e, locate(e, p, a.local, a.cards), now)
		rows[i] = make([]mac.Cell, nCols)
		for j, c := range ls[i].cells {
			rows[i][j] = mac.Cell{Text: c.text, Sub: c.sub, Tip: c.tip, Dim: c.dim, Icon: c.icon, Tint: c.tint, SubTint: c.subTint}
		}
	}
	note := "Click a line to go to its agent"
	if p.err != nil {
		note = "herdr is not answering: " + p.err.Error()
	}
	empty := ""
	switch {
	case err != nil:
		empty = "Can't read " + a.log + ": " + err.Error()
	case len(es) == 0:
		empty = "Nothing said yet. What agents say aloud shows up here, newest first."
	}
	mac.DoSync(func() {
		a.lines = ls
		a.board.Set(rows, note, empty)
	})
}

// clicked focuses the row's pane and brings its terminal forward, or says
// why it cannot.
func (a *app) clicked(row int) {
	if row >= len(a.lines) {
		return
	}
	l := a.lines[row]
	if l.pane == "" {
		a.board.Flash(l.why)
		return
	}
	go func() {
		if err := a.herdr.Focus(l.pane); err != nil {
			log.Printf("focus %s: %v", l.pane, err)
			a.board.Flash("herdr could not focus " + l.pane + ": " + err.Error())
			return
		}
		log.Printf("focused %s", l.pane)
		mac.Do(func() {
			if a.terminal != "" && !mac.ActivateBundle(a.terminal) {
				log.Printf("%s is not running", a.terminal)
			}
			a.board.Close()
		})
	}()
}
