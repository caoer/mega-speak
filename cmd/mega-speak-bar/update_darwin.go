//go:build darwin

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"syscall"

	update "git.0xdao.app/caoer115/mega-update"

	"github.com/caoer/mega-speak/internal/mac"
)

// startUpdates runs the staging updater in the background; a build with no
// channel returns without a request.
func startUpdates(stateDir string) {
	u, err := update.New(updateConfig(stateDir))
	if err != nil {
		log.Printf("update: %v", err)
		return
	}
	if !u.Enabled() {
		return
	}
	app := bundleOf(executable())
	p := &updater{
		u:   u,
		app: app,
		ask: func(rel *update.Release) updateChoice {
			info := fmt.Sprintf("当前版本 %s。", version)
			if rel.Notes != "" {
				info += "\n\n" + rel.Notes
			}
			done := make(chan int, 1)
			mac.Later(func() {
				done <- mac.Choose("Mega Speak 有新版本 "+rel.Version, info, "更新", "跳过此版本", "稍后")
			})
			switch <-done {
			case 0:
				return chooseUpdate
			case 1:
				return chooseSkip
			}
			return chooseLater
		},
		notify: func(msg string) {
			mac.Later(func() { mac.Alert("Mega Speak", msg) })
		},
		relaunch: func() { relaunch(app) },
	}
	log.Printf("update: channel %s, %s", channel, version)
	go p.run(context.Background(), update.Interval)
}

// relaunch opens app once this process has exited, then exits: the new copy
// takes the single-instance lock this one holds.
func relaunch(app string) {
	cmd := exec.Command("/bin/sh", "-c", `while kill -0 "$1" 2>/dev/null; do sleep 0.2; done; exec /usr/bin/open "$2"`,
		"relaunch", strconv.Itoa(os.Getpid()), app)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		log.Printf("relaunch %s: %v", app, err)
		mac.Later(func() { mac.Alert("Mega Speak", "已更新，请重新打开 Mega Speak。") })
		return
	}
	log.Printf("relaunching %s", app)
	os.Exit(0)
}
