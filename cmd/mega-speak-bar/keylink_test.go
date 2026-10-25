package main

import (
	"bytes"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/caoer/mega-speak/internal/i18n"
)

const (
	linkKey  = "sk-vp-Link0123456789+/=%&wxyz" // URL-special characters survive encoding
	otherKey = "sk-vp-Other9876543210abcd"
)

func keyLinkOf(key string) string { return "megaspeak://set-key?key=" + url.QueryEscape(key) }

func TestKeyFromLink(t *testing.T) {
	if k, err := keyFromLink(keyLinkOf(linkKey)); err != nil || k != linkKey {
		t.Fatalf("well-formed link: %q, %v", k, err)
	}
	if k, err := keyFromLink("MegaSpeak://Set-Key/?key=" + url.QueryEscape(otherKey)); err != nil || k != otherKey {
		t.Fatalf("case and trailing slash: %q, %v", k, err)
	}
	for _, bad := range []string{
		"megaspeak://set-key",
		"megaspeak://set-key?key=",
		"megaspeak://set-key?key=short",
		"megaspeak://set-key?key=" + url.QueryEscape("has space in it"),
		"megaspeak://set-key?key=" + url.QueryEscape("sk-vp-钥匙钥匙钥匙"),
		"megaspeak://set-key?key=" + strings.Repeat("k", 257),
		"megaspeak://set-key?key=" + linkKey[:10] + "&key=" + otherKey,
		"megaspeak://other?key=" + url.QueryEscape(linkKey),
		"megaspeak://set-key/more?key=" + url.QueryEscape(linkKey),
		"megaspeak://u@set-key?key=" + url.QueryEscape(linkKey),
		"https://set-key?key=" + url.QueryEscape(linkKey),
		"megaspeak://set-key?key=%zz",
	} {
		k, err := keyFromLink(bad)
		if err == nil || k != "" {
			t.Errorf("%q: accepted as %q", bad, k)
		} else if strings.Contains(err.Error(), "sk-vp") {
			t.Errorf("%q: the error carries the key: %v", bad, err)
		}
	}
}

// cliHarness runs the repository's CLI (bin/mega-speak) the way the app does,
// with a key file of its own and no system or user config.
type cliHarness struct {
	cli, keyFile string
	asked        []string // confirm questions, title + body
	answer       bool     // what confirm answers
	told         []string // tell titles and bodies
	logs         bytes.Buffer
}

func newCLI(t *testing.T) *cliHarness {
	t.Helper()
	py := ""
	for _, name := range []string{"python3.14", "python3.13", "python3.12", "python3.11", "python3"} {
		if p, err := exec.LookPath(name); err == nil && exec.Command(p, "-c", "import tomllib").Run() == nil {
			py = p
			break
		}
	}
	if py == "" {
		t.Skip("no python with tomllib (3.11+) on PATH to run bin/mega-speak")
	}
	dir := t.TempDir()
	src, err := os.ReadFile("../../bin/mega-speak")
	if err != nil {
		t.Fatal(err)
	}
	// A Mac's /etc/mega-speak/config.toml (zmax-1 has one) must not reach the test.
	src = bytes.ReplaceAll(src, []byte(`"/etc/mega-speak/config.toml"`), []byte(`"`+filepath.Join(dir, "no-etc.toml")+`"`))
	os.WriteFile(filepath.Join(dir, "mega-speak.py"), src, 0o644)
	wrapper := filepath.Join(dir, "mega-speak")
	os.WriteFile(wrapper, []byte("#!/bin/sh\nexec '"+py+"' '"+filepath.Join(dir, "mega-speak.py")+"' \"$@\"\n"), 0o755)
	home := filepath.Join(dir, "home")
	os.MkdirAll(home, 0o700)
	t.Setenv("HOME", home)
	t.Setenv("MEGA_SPEAK_CONFIG", "")
	t.Setenv("MEGA_SPEAK_API_KEY", "")
	os.Unsetenv("MEGA_SPEAK_API_KEY")
	h := &cliHarness{cli: wrapper, keyFile: filepath.Join(dir, "cfg", "api-key")}
	t.Setenv("MEGA_SPEAK_KEY_PATH", h.keyFile)
	log.SetOutput(&h.logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return h
}

func (h *cliHarness) prompt() keyPrompt {
	return keyPrompt{
		confirm: func(title, body, yes, no string) bool {
			h.asked = append(h.asked, title+" | "+body+" | "+yes+" | "+no)
			return h.answer
		},
		tell: func(title, body string) { h.told = append(h.told, title+" | "+body) },
	}
}

// noKeyShown fails when any key reached a prompt, an alert or the log.
func (h *cliHarness) noKeyShown(t *testing.T) {
	t.Helper()
	all := strings.Join(h.asked, "\n") + strings.Join(h.told, "\n") + h.logs.String()
	for _, k := range []string{linkKey, otherKey, url.QueryEscape(linkKey), url.QueryEscape(otherKey)} {
		if strings.Contains(all, k) {
			t.Fatalf("a key was shown or logged:\n%s", all)
		}
	}
}

func TestKeyLinkStoresWhatSetKeyStores(t *testing.T) {
	// --set-key typed into the CLI by hand: the reference bytes and mode.
	h := newCLI(t)
	if _, err := keyCLI(h.cli, linkKey, "--set-key"); err != nil {
		t.Fatal(err)
	}
	want, _ := os.ReadFile(h.keyFile)
	os.Remove(h.keyFile)

	if !installKeyLink(keyLinkOf(linkKey), h.cli, h.prompt()) {
		t.Fatalf("not stored; told %q, log %s", h.told, h.logs.String())
	}
	got, err := os.ReadFile(h.keyFile)
	fi, _ := os.Stat(h.keyFile)
	if err != nil || !bytes.Equal(got, want) || fi.Mode().Perm() != 0o600 {
		t.Fatalf("key file %q %v (%v), want --set-key's %q 0600", got, fi.Mode(), err, want)
	}
	if len(h.asked) != 0 || len(h.told) != 1 || h.told[0] != i18n.T("key.installed.title")+" | "+i18n.F("key.installed.body", "…wxyz") {
		t.Fatalf("asked %q, told %q", h.asked, h.told)
	}
	st, err := keyCLI(h.cli, "", "--key-state")
	if err != nil || st.Source != "saved" || st.Hint != "…wxyz" {
		t.Fatalf("--key-state after the link: %+v %v", st, err)
	}
	h.noKeyShown(t)
}

func TestKeyLinkAsksBeforeReplacingADifferentKey(t *testing.T) {
	h := newCLI(t)
	keyCLI(h.cli, otherKey, "--set-key")

	h.answer = false
	if installKeyLink(keyLinkOf(linkKey), h.cli, h.prompt()) {
		t.Fatal("stored though the person kept the current key")
	}
	if b, _ := os.ReadFile(h.keyFile); string(b) != otherKey+"\n" {
		t.Fatalf("kept key changed: %q", b)
	}
	if len(h.asked) != 1 || !strings.Contains(h.asked[0], "…abcd") || !strings.Contains(h.asked[0], "…wxyz") {
		t.Fatalf("the question shows both last fours: %q", h.asked)
	}
	if h.told[0] != i18n.T("key.kept.title")+" | "+i18n.F("key.kept.body", "…abcd") {
		t.Fatalf("told %q", h.told)
	}

	h.answer = true
	if !installKeyLink(keyLinkOf(linkKey), h.cli, h.prompt()) {
		t.Fatal("not stored after Replace")
	}
	if b, _ := os.ReadFile(h.keyFile); string(b) != linkKey+"\n" {
		t.Fatalf("replaced key: %q", b)
	}

	// The same key again: nothing to ask, nothing to change.
	h.asked, h.told = nil, nil
	if installKeyLink(keyLinkOf(linkKey), h.cli, h.prompt()) || len(h.asked) != 0 ||
		h.told[0] != i18n.T("key.same.title")+" | "+i18n.F("key.same.body", "…wxyz") {
		t.Fatalf("same key: asked %q, told %q", h.asked, h.told)
	}
	h.noKeyShown(t)
}

func TestKeyLinkRejectsAMalformedLink(t *testing.T) {
	h := newCLI(t)
	keyCLI(h.cli, otherKey, "--set-key")
	for _, bad := range []string{"megaspeak://set-key?key=" + url.QueryEscape("sk-vp has spaces"), "megaspeak://set-key"} {
		h.told = nil
		if installKeyLink(bad, h.cli, h.prompt()) || len(h.told) != 1 || h.told[0] != i18n.T("key.bad.title")+" | "+i18n.T("key.bad.body") {
			t.Fatalf("%q: told %q", bad, h.told)
		}
	}
	if b, _ := os.ReadFile(h.keyFile); string(b) != otherKey+"\n" || len(h.asked) != 0 {
		t.Fatalf("a bad link changed the key: %q, asked %q", b, h.asked)
	}
	h.noKeyShown(t)
}

func TestKeyLinkLeavesAConfiguredKeyInForce(t *testing.T) {
	h := newCLI(t)
	t.Setenv("MEGA_SPEAK_API_KEY", otherKey) // as an env, api_key_file or api_key_cmd setup provides it
	if installKeyLink(keyLinkOf(linkKey), h.cli, h.prompt()) {
		t.Fatal("stored over a configured key")
	}
	if _, err := os.Stat(h.keyFile); !os.IsNotExist(err) {
		t.Fatalf("key file written: %v", err)
	}
	if h.told[0] != i18n.T("key.configured.title")+" | "+i18n.F("key.configured.body", "env") {
		t.Fatalf("told %q", h.told)
	}
	h.noKeyShown(t)
}

func TestKeyLinkSpeaksTheAppLanguage(t *testing.T) {
	defer i18n.Set(i18n.Current())
	h := newCLI(t)
	i18n.Set(i18n.EN)
	installKeyLink(keyLinkOf(linkKey), h.cli, h.prompt())
	if h.told[0] != "Voice key installed | The key ends in …wxyz. Mega Speak can speak now." {
		t.Fatalf("English: %q", h.told)
	}
	i18n.Set(i18n.ZH)
	h.told = nil
	installKeyLink("megaspeak://set-key", h.cli, h.prompt())
	if h.told[0] != "链接里的语音钥匙不对 | 钥匙没有装。请重新打开你收到的页面，点里面的 Mega Speak 按钮。" {
		t.Fatalf("Chinese: %q", h.told)
	}
}
