package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	update "git.0xdao.app/caoer115/mega-update"
)

// Set by the friends' staging build (scripts/release-mac.sh): -X main.channel=staging,
// -X main.feedURL=https://<host>/d/<slug>/release.megaspeak-mac?raw=1, -X main.pageToken=….
// ZT's own build sets none of them, and an empty channel never makes a request.
// version is set at build time (-X main.version=…).
var (
	version   = "dev"
	channel   string
	feedURL   string
	pageToken string
)

type updateChoice int

const (
	chooseLater  updateChoice = iota // 稍后: the next daily check asks again
	chooseUpdate                     // 更新
	chooseSkip                       // 跳过此版本
)

// updateConfig is this build's updater configuration; state lives in stateDir.
func updateConfig(stateDir string) update.Config {
	return update.Config{Channel: channel, FeedURL: feedURL, Token: pageToken, Current: version, StateDir: stateDir}
}

// updater offers a staging release at launch and every update.Interval.
type updater struct {
	u        *update.Updater
	app      string                             // the running Mega Speak.app; "" when not run from a bundle
	ask      func(*update.Release) updateChoice // 更新 / 跳过此版本 / 稍后, blocking until answered
	notify   func(string)                       // a line the user reads
	relaunch func()                             // after a swap: start the new bundle, end this process
	told     sync.Map                           // messages already shown once
}

// bundleOf is the .app holding exe (…/X.app/Contents/MacOS/exe), or "".
func bundleOf(exe string) string {
	macos := filepath.Dir(exe)
	contents := filepath.Dir(macos)
	app := filepath.Dir(contents)
	if filepath.Base(macos) != "MacOS" || filepath.Base(contents) != "Contents" || !strings.HasSuffix(app, ".app") {
		return ""
	}
	return app
}

// checkOnce asks the feed once and acts on the answer.
func (p *updater) checkOnce(ctx context.Context) {
	rel, err := p.u.Check(ctx)
	if err != nil {
		log.Printf("update check: %v", err)
		p.tell(update.UserMessage(err))
	}
	if rel == nil {
		return
	}
	if p.app == "" {
		log.Printf("update %s offered, but this copy is not in an app bundle", rel.Version)
		return
	}
	switch p.ask(rel) {
	case chooseUpdate:
		if err := p.u.Apply(ctx, rel, p.app); err != nil {
			log.Printf("update %s: %v", rel.Version, err)
			if msg := update.UserMessage(err); msg != "" {
				p.notify(msg)
			}
			return
		}
		log.Printf("updated to %s: %s", rel.Version, p.app)
		p.relaunch()
	case chooseSkip:
		if err := p.u.Skip(rel.Version); err != nil {
			log.Printf("skip %s: %v", rel.Version, err)
		}
	}
}

// tell shows msg once per run; a revoked token answers the same on every check.
func (p *updater) tell(msg string) {
	if msg == "" {
		return
	}
	if _, seen := p.told.LoadOrStore(msg, true); !seen {
		p.notify(msg)
	}
}

// run checks at once and then every interval until ctx ends; with no channel it
// returns at once.
func (p *updater) run(ctx context.Context, interval time.Duration) {
	if !p.u.Enabled() {
		return
	}
	p.checkOnce(ctx)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.checkOnce(ctx)
		}
	}
}

// executable is this process's path with symlinks resolved.
func executable() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		return r
	}
	return exe
}
