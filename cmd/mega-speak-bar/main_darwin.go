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
	flag.StringVar(&speakBin, "speak", "mega-speak", "the mega-speak CLI used to read and save the chosen voice")
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
	mac.Run(func() {
		a.board = mac.NewBoard("text.bubble", "Mega Speak", []mac.Column{
			{Width: 18},
			{Title: "Time", Width: 72, Mono: true},
			{Title: "Agent", Width: 196},
			{Title: "Text", Wrap: true},
			{Title: "Emotion", Width: 82},
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
				a.board.Flash("音色库未能载入，请检查 Mega Speak 日志。")
				return
			}
			a.voicePicker.window.Open()
			a.voicePicker.window.Eval("window.refreshVoiceState?.()")
			a.board.Close()
		}
		a.settingsWindow = newSettingsWindow(speakBin)
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
				a.board.Flash("归档失败：" + err.Error())
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
		if showSettings {
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
	note := fmt.Sprintf("Inbox · %d 条待处理 · 点击正文可返回 Agent", len(es))
	if a.history {
		note = fmt.Sprintf("History · %d 条播报 · 归档记录仍然保留", len(es))
	}
	if p.err != nil {
		note += " · Agent 跳转暂不可用"
	}
	empty := ""
	switch {
	case err != nil:
		empty = "Can't read " + a.log + ": " + err.Error()
	case len(es) == 0:
		empty = "Inbox 已清空。新的播报会显示在这里，归档内容可在 History 查看。"
		if a.history {
			empty = "还没有播报记录。"
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

func (a *app) archiveRow(row int) {
	if row < 0 || row >= len(a.visibleEntries) {
		return
	}
	e := a.visibleEntries[row]
	if err := a.inbox.set([]string{e.ID}, !a.inbox.Archived[e.ID]); err != nil {
		a.board.Flash("归档失败：" + err.Error())
		return
	}
	a.renderCurrent()
}

func (a *app) renderCurrent() { a.mu.Lock(); p := a.panes; a.mu.Unlock(); a.render(p) }
