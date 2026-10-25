//go:build darwin

package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/caoer/mega-speak/internal/herdr"
	"github.com/caoer/mega-speak/internal/i18n"
	"github.com/caoer/mega-speak/internal/mac"
	"github.com/caoer/mega-speak/internal/voices"
)

const usage = `mega-speak-bar — Mega Speak in the menu bar: voice controls and an archivable speech inbox

usage: mega-speak-bar [flags]

The icon opens a menu: Inbox, History, voice selection and speech settings.
Inbox entries can be archived; History retains the original playback log.
A click on a line focuses the herdr pane of the agent that said it.

flags:
`

// version is set at build time.
var version = "dev"

// app is the running panel: the log it reads and where it finds agents.
type app struct {
	log, cards, agents, terminal, local string
	herdr                               herdr.Client
	board                               *mac.Board
	voicePicker                         *voicePicker
	settingsWindow                      *mac.VoiceWindow
	inbox                               *inboxState
	history                             bool
	visibleEntries                      []entry
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
	var showVoices, showSettings bool
	var instanceDir string
	var speakBin, settings, importFavorites string
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage); flag.PrintDefaults() }
	home, _ := os.UserHomeDir()
	flag.StringVar(&speakBin, "speak", defaultSpeak(), "the mega-speak CLI used to read and save the chosen voice")
	flag.StringVar(&settings, "settings-dir", filepath.Join(home, "Library/Application Support/MegaSpeak"), "persistent app preferences and cached voice media")
	flag.StringVar(&importFavorites, "import-favorites", "", "merge a JSON array of voice IDs into app favorites, then exit")
	flag.BoolVar(&showSettings, "show-settings", false, "open speech settings at start")
	flag.StringVar(&instanceDir, "instance-dir", filepath.Join(home, ".cache/mega-speak-app"), "instance coordination directory; separate only for isolated UI testing")
	flag.BoolVar(&showVoices, "show-voices", false, "open the voice selector at start")
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
	if importFavorites != "" {
		b, err := os.ReadFile(importFavorites)
		var ids []string
		if err == nil {
			err = json.Unmarshal(b, &ids)
		}
		if err == nil {
			library, loadErr := voices.Load()
			err = loadErr
			valid := map[string]bool{}
			for _, v := range library.Voices {
				valid[v.ID] = true
			}
			for _, id := range ids {
				if !valid[id] {
					err = fmt.Errorf("unknown favorite voice: %s", id)
					break
				}
			}
		}
		if err == nil {
			var f *voices.Favorites
			f, err = voices.ReadFavorites(filepath.Join(settings, "favorites.json"))
			if err == nil {
				err = f.Import(ids)
			}
		}
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Imported %d favorite voices\n", len(ids))
		return
	}
	command := "inbox"
	if showVoices {
		command = "voices"
	}
	if showSettings {
		command = "settings"
	}
	lock, err := acquireInstance(instanceDir, command)
	if err != nil {
		log.Fatal(err)
	}
	if lock == nil {
		return
	}
	defer lock.Close()
	a.inbox, err = loadInbox(filepath.Join(settings, "inbox.json"))
	if err != nil {
		log.Fatal(err)
	}
	a.herdr = herdr.Client{Bin: herdrBin}
	host, _ := os.Hostname()
	a.local, _, _ = strings.Cut(host, ".")
	i18n.Set(i18n.Pick(mac.PreferredLanguage()))
	up := make(chan struct{}) // closed once the windows exist
	mac.OnURL = func(link string) { go a.keyLink(link, speakBin, up) }
	mac.Run(func() {
		a.board = mac.NewBoard("text.bubble", i18n.T("app.name"), []mac.Column{
			{Width: 18},
			{Title: i18n.T("col.time"), Width: 72, Mono: true},
			{Title: i18n.T("col.agent"), Width: 196},
			{Title: i18n.T("col.text"), Wrap: true},
			{Title: i18n.T("col.emotion"), Width: 82},
			{Title: "", Width: 64},
		}, 900, 540)
		a.board.OnOpen, a.board.OnClose, a.board.OnClick = a.opened, a.closed, a.clicked
		var err error
		a.voicePicker, err = newVoicePicker(speakBin, settings)
		if err != nil {
			log.Printf("voice picker: %v", err)
		}
		a.board.OnVoices = func() {
			if a.voicePicker == nil {
				a.board.Flash(i18n.T("board.voices_failed"))
				return
			}
			a.voicePicker.window.Open()
			a.voicePicker.window.Eval("window.refreshVoiceState?.()")
			a.board.Close()
		}
		a.settingsWindow = newSettingsWindow(speakBin)
		close(up)
		a.board.OnInbox = func() { a.history = false; a.board.SetHistory(false) }
		a.board.OnHistory = func() { a.history = true; a.board.SetHistory(true) }
		a.board.OnSettings = func() { a.board.Close(); a.settingsWindow.Open(); a.settingsWindow.Eval("window.refreshSettings?.()") }
		a.board.OnArchive = a.archiveRow
		a.board.OnArchiveAll = func() {
			ids := []string{}
			for _, e := range a.visibleEntries {
				ids = append(ids, e.ID)
			}
			if err := a.inbox.set(ids, true); err != nil {
				a.board.Flash(i18n.F("board.archive_failed", err))
				return
			}
			a.renderCurrent()
		}
		a.board.OnQuit = func() {
			go func() {
				target := "gui/" + strconv.Itoa(os.Getuid()) + "/app.0xdao.megaspeak"
				output, err := exec.Command("/bin/launchctl", "print", target).Output()
				if err == nil {
					for _, line := range strings.Split(string(output), "\n") {
						if strings.TrimSpace(line) == "pid = "+strconv.Itoa(os.Getpid()) {
							if err := exec.Command("/bin/launchctl", "bootout", target).Run(); err != nil {
								log.Printf("quit: %v", err)
								return
							}
							break
						}
					}
				}
				os.Exit(0)
			}()
		}

		sock := instanceSocket(instanceDir)
		os.Remove(sock)
		listener, listenErr := net.Listen("unix", sock)
		if listenErr != nil {
			log.Fatal(listenErr)
		}
		os.Chmod(sock, 0600)
		go func() {
			for {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				conn.SetReadDeadline(time.Now().Add(time.Second))
				line, _ := bufio.NewReader(conn).ReadString('\n')
				conn.Close()
				mac.Do(func() {
					switch strings.TrimSpace(line) {
					case "voices":
						a.board.OnVoices()
					case "settings":
						a.board.OnSettings()
					case "history":
						a.board.OnHistory()
						a.board.Open()
					default:
						a.board.OnInbox()
						a.board.Open()
					}
				})
			}
		}()
		log.Printf("mega-speak-bar %s: %s, host %s", version, a.log, a.local)
		go a.loop()
		startUpdates(filepath.Join(settings, "update"))
		// First run on a Mac with no key: open settings at the key field.
		if showSettings || needsKey(speakBin) {
			a.board.OnSettings()
		}
		if showVoices && a.voicePicker != nil {
			a.voicePicker.window.Open()
		}
		if show {
			time.Sleep(time.Second) // the new icon takes its place in the menu bar first
			a.board.Open()
			time.Sleep(time.Second)
			log.Printf("panel window %d", a.board.WindowNumber())
		}
	})
}

// keyLink installs the voice key from a megaspeak://set-key link once the
// app's windows are up, and refreshes settings to show it.
func (a *app) keyLink(link, cli string, up <-chan struct{}) {
	<-up
	stored := installKeyLink(link, cli, keyPrompt{
		confirm: func(title, body, yes, no string) bool {
			done := make(chan int, 1)
			mac.Later(func() { done <- mac.Choose(title, body, yes, no) })
			return <-done == 0
		},
		tell: func(title, body string) { mac.Later(func() { mac.Alert(title, body) }) },
	})
	if stored {
		a.settingsWindow.Eval("window.refreshSettings?.()")
	}
}

// defaultSpeak is the CLI beside this executable in Mega Speak.app, else
// mega-speak on PATH.
func defaultSpeak() string {
	if exe := executable(); exe != "" {
		sibling := filepath.Join(filepath.Dir(exe), "mega-speak")
		if fi, err := os.Stat(sibling); err == nil && !fi.IsDir() {
			return sibling
		}
	}
	return "mega-speak"
}

// needsKey reports whether the CLI has no voice key from any source.
func needsKey(cli string) bool {
	out, err := exec.Command(cli, "--key-state").Output()
	if err != nil {
		log.Printf("key state: %v", err)
		return false
	}
	var s struct{ Source string }
	return json.Unmarshal(out, &s) == nil && s.Source == "none"
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
		mac.Do(func() { a.render(p) })
	}
}

// render reads the log and puts it on the board with where each agent is.
func (a *app) render(p panes) {
	all, err := readInboxLog(a.log)
	es := make([]entry, 0, len(all))
	for _, e := range all {
		if a.history || !a.inbox.Archived[e.ID] {
			es = append(es, e)
		}
	}
	a.visibleEntries = es
	now := time.Now()
	ls := make([]line, len(es))
	rows := make([][]mac.Cell, len(es))
	for i, e := range es {
		ls[i] = lineOf(e, locate(e, p, a.local, a.cards), now)
		rows[i] = make([]mac.Cell, nCols+1)
		for j, c := range ls[i].cells {
			rows[i][j] = mac.Cell{Text: c.text, Sub: c.sub, Tip: c.tip, Dim: c.dim, Icon: c.icon, Tint: c.tint, SubTint: c.subTint}
		}
		action := "archive-action"
		if a.inbox.Archived[e.ID] {
			action = "restore-action"
		}
		rows[i][nCols] = mac.Cell{Icon: action}
	}
	note := i18n.F("board.inbox_note", len(es))
	if a.history {
		note = i18n.F("board.history_note", len(es))
	}
	if p.err != nil {
		note += i18n.T("board.no_jump")
	}
	empty := ""
	switch {
	case err != nil:
		empty = i18n.F("board.cant_read", a.log, err)
	case len(es) == 0:
		empty = i18n.T("board.inbox_empty")
		if a.history {
			empty = i18n.T("board.history_empty")
		}
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
			a.board.Flash(i18n.F("board.focus_failed", l.pane, err))
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

func (a *app) archiveRow(row int) {
	if row < 0 || row >= len(a.visibleEntries) {
		return
	}
	e := a.visibleEntries[row]
	if err := a.inbox.set([]string{e.ID}, !a.inbox.Archived[e.ID]); err != nil {
		a.board.Flash(i18n.F("board.archive_failed", err))
		return
	}
	a.renderCurrent()
}

func (a *app) renderCurrent() { a.mu.Lock(); p := a.panes; a.mu.Unlock(); a.render(p) }
