package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	update "git.0xdao.app/caoer115/mega-update"
)

const (
	testSlug  = "mega-friends-test"
	testKey   = "release.megaspeak-mac"
	testToken = "app-token"
)

// feed is a fake ccc-pages page: POST /unlock, the raw feed record with ETag,
// /f/ 302 to a presigned blob.
type feed struct {
	srv      *httptest.Server
	mu       sync.Mutex
	rel      update.Release
	etag     int
	revoked  bool
	blob     []byte
	requests atomic.Int32
}

func newFeed(t *testing.T, ver string) *feed {
	f := &feed{blob: []byte("new Mega Speak.app")}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	f.publish(ver)
	return f
}

func (f *feed) publish(ver string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sum := sha256.Sum256(f.blob)
	f.rel = update.Release{Version: ver, URL: "/f/" + testSlug + "/dmg", SHA256: hex.EncodeToString(sum[:]), Notes: "修复了一个问题"}
	f.etag++
}

func (f *feed) serve(w http.ResponseWriter, r *http.Request) {
	f.requests.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	granted := func() bool { c, err := r.Cookie("ccc_grant"); return err == nil && c.Value == "ok" && !f.revoked }
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/unlock":
		var b struct{ Slug, Token string }
		json.NewDecoder(r.Body).Decode(&b)
		if f.revoked || b.Slug != testSlug || b.Token != testToken {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"error":"token_revoked","code":"token_revoked"}`)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "ccc_grant", Value: "ok", Path: "/", Secure: true, HttpOnly: true})
		fmt.Fprint(w, `{"ok":true}`)
	case r.URL.Path == "/d/"+testSlug+"/"+testKey && r.URL.Query().Get("raw") == "1" && granted():
		etag := fmt.Sprintf(`"%d"`, f.etag)
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		json.NewEncoder(w).Encode(f.rel)
	case r.URL.Path == "/f/"+testSlug+"/dmg" && granted():
		http.Redirect(w, r, "/blob", http.StatusFound)
	case r.URL.Path == "/blob":
		w.Write(f.blob)
	default:
		w.WriteHeader(http.StatusUnauthorized)
	}
}

// staging sets this build's ldflag values the way the staging build does.
func staging(t *testing.T, f *feed, ver string) {
	old := [4]string{channel, feedURL, pageToken, version}
	channel, feedURL, pageToken, version = "staging", f.srv.URL+"/d/"+testSlug+"/"+testKey+"?raw=1", testToken, ver
	t.Cleanup(func() { channel, feedURL, pageToken, version = old[0], old[1], old[2], old[3] })
}

// harness is an updater whose prompt answers from a script and whose
// installed app is a folder in a temp dir.
type harness struct {
	p         *updater
	asked     []string
	told      []string
	relaunchs int
}

func newHarness(t *testing.T, f *feed, answers ...updateChoice) *harness {
	cfg := updateConfig(t.TempDir())
	cfg.HTTPClient = f.srv.Client()
	cfg.OSVersion = "15.0"
	cfg.Extract = func(_ context.Context, archive, dir string) (string, error) {
		app := filepath.Join(dir, "Mega Speak.app")
		if err := os.MkdirAll(filepath.Join(app, "Contents"), 0o755); err != nil {
			return "", err
		}
		b, err := os.ReadFile(archive)
		if err != nil {
			return "", err
		}
		return app, os.WriteFile(filepath.Join(app, "Contents", "payload"), b, 0o644)
	}
	u, err := update.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(t.TempDir(), "Applications", "Mega Speak.app")
	if err := os.MkdirAll(filepath.Join(app, "Contents"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(app, "Contents", "payload"), []byte("old"), 0o644)
	h := &harness{}
	h.p = &updater{u: u, app: app,
		ask: func(rel *update.Release) updateChoice {
			h.asked = append(h.asked, rel.Version)
			if len(answers) == 0 {
				t.Fatalf("unexpected prompt for %s", rel.Version)
			}
			a := answers[0]
			answers = answers[1:]
			return a
		},
		notify:   func(s string) { h.told = append(h.told, s) },
		relaunch: func() { h.relaunchs++ },
	}
	return h
}

func (h *harness) installed(t *testing.T) string {
	b, err := os.ReadFile(filepath.Join(h.p.app, "Contents", "payload"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestNewerReleaseUpdatesAndRelaunches(t *testing.T) {
	f := newFeed(t, "v0.1.1")
	staging(t, f, "v0.1.0")
	h := newHarness(t, f, chooseUpdate)
	h.p.checkOnce(context.Background())
	if fmt.Sprint(h.asked) != "[v0.1.1]" || h.relaunchs != 1 || h.installed(t) != "new Mega Speak.app" {
		t.Fatalf("asked %v, relaunched %d, installed %q", h.asked, h.relaunchs, h.installed(t))
	}
	if _, err := os.Stat(update.BackupPath(h.p.app)); err != nil {
		t.Fatalf("no backup of the old app: %v", err)
	}
}

func TestSameVersionIsNotOffered(t *testing.T) {
	f := newFeed(t, "v0.1.0")
	staging(t, f, "v0.1.0")
	h := newHarness(t, f)
	h.p.checkOnce(context.Background())
	if len(h.asked) != 0 || h.installed(t) != "old" {
		t.Fatalf("asked %v, installed %q", h.asked, h.installed(t))
	}
}

func TestSkipSilencesThatVersionOnly(t *testing.T) {
	f := newFeed(t, "v0.1.1")
	staging(t, f, "v0.1.0")
	h := newHarness(t, f, chooseSkip, chooseLater)
	h.p.checkOnce(context.Background())
	h.p.checkOnce(context.Background()) // skipped: no prompt
	f.publish("v0.1.2")
	h.p.checkOnce(context.Background())
	if fmt.Sprint(h.asked) != "[v0.1.1 v0.1.2]" || h.installed(t) != "old" || h.relaunchs != 0 {
		t.Fatalf("asked %v, installed %q, relaunched %d", h.asked, h.installed(t), h.relaunchs)
	}
}

func TestLaterAsksAgainNextCheck(t *testing.T) {
	f := newFeed(t, "v0.1.1")
	staging(t, f, "v0.1.0")
	h := newHarness(t, f, chooseLater, chooseLater)
	h.p.checkOnce(context.Background())
	h.p.checkOnce(context.Background())
	if fmt.Sprint(h.asked) != "[v0.1.1 v0.1.1]" || h.installed(t) != "old" {
		t.Fatalf("asked %v, installed %q", h.asked, h.installed(t))
	}
}

func TestBadChecksumKeepsOldAppAndSays(t *testing.T) {
	f := newFeed(t, "v0.1.1")
	f.blob = []byte("tampered") // the feed's sha256 is of the original bytes
	staging(t, f, "v0.1.0")
	h := newHarness(t, f, chooseUpdate)
	h.p.checkOnce(context.Background())
	if h.installed(t) != "old" || h.relaunchs != 0 || fmt.Sprint(h.told) != "[更新校验失败，下次再试]" {
		t.Fatalf("installed %q, relaunched %d, told %v", h.installed(t), h.relaunchs, h.told)
	}
}

func TestRevokedTokenTellsOnceAndStopsAsking(t *testing.T) {
	f := newFeed(t, "v0.1.1")
	f.revoked = true
	staging(t, f, "v0.1.0")
	h := newHarness(t, f)
	h.p.checkOnce(context.Background())
	n := f.requests.Load()
	h.p.checkOnce(context.Background())
	if len(h.asked) != 0 || fmt.Sprint(h.told) != "[更新不可用，请从页面重新安装]" || f.requests.Load() != n {
		t.Fatalf("asked %v, told %v, requests %d then %d", h.asked, h.told, n, f.requests.Load())
	}
}

// ZT's own build: no ldflags, so no channel, no feed, no token — and no request.
func TestDevBuildNeverPolls(t *testing.T) {
	if channel != "" || feedURL != "" || pageToken != "" {
		t.Fatalf("the default build carries channel=%q feedURL=%q token set=%v", channel, feedURL, pageToken != "")
	}
	f := newFeed(t, "v9.9.9")
	cfg := updateConfig(t.TempDir())
	cfg.HTTPClient = f.srv.Client()
	u, err := update.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	p := &updater{u: u, app: "/Applications/Mega Speak.app",
		ask:    func(*update.Release) updateChoice { t.Fatal("prompted"); return chooseLater },
		notify: func(string) { t.Fatal("notified") }, relaunch: func() { t.Fatal("relaunched") }}
	done := make(chan struct{})
	go func() { p.run(context.Background(), time.Millisecond); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("run kept going with no channel")
	}
	if u.Enabled() || f.requests.Load() != 0 {
		t.Fatalf("enabled %v, requests %d", u.Enabled(), f.requests.Load())
	}
}

func TestBundleOf(t *testing.T) {
	for exe, want := range map[string]string{
		"/Applications/Mega Speak.app/Contents/MacOS/mega-speak-bar": "/Applications/Mega Speak.app",
		"/nix/store/x-mega-speak-app/bin/mega-speak-bar":             "",
		"/tmp/Contents/MacOS/mega-speak-bar":                         "",
	} {
		if got := bundleOf(exe); got != want {
			t.Errorf("bundleOf(%q) = %q, want %q", exe, got, want)
		}
	}
}
