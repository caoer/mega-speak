package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/caoer/mega-speak/internal/i18n"
)

// A friend's page carries their voice key in a link,
// megaspeak://set-key?key=<url-encoded key>, so it reaches Mega Speak with a
// tap and is never typed. The key is stored by the CLI's own --set-key.

// keyPattern is the CLI's rule for a key (bin/mega-speak save_key).
var keyPattern = regexp.MustCompile(`^[\x21-\x7e]{8,256}$`)

var errNotKeyLink = errors.New("not a megaspeak://set-key link with one well-formed key")

// keyFromLink is the key in a megaspeak://set-key?key=… link. The error never
// carries the link, which holds the key.
func keyFromLink(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "megaspeak") || !strings.EqualFold(u.Host, "set-key") ||
		(u.Path != "" && u.Path != "/") || u.User != nil || u.Port() != "" {
		return "", errNotKeyLink
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(q["key"]) != 1 {
		return "", errNotKeyLink
	}
	key := strings.TrimSpace(q["key"][0])
	if !keyPattern.MatchString(key) {
		return "", errNotKeyLink
	}
	return key, nil
}

// keyState is the CLI's --key-state answer: where the key comes from and the
// saved key's last four characters, never the key.
type keyState struct {
	Source  string `json:"source"`
	KeyFile string `json:"key_file"`
	Hint    string `json:"hint"`
}

// keyCLI runs the mega-speak CLI's key commands.
func keyCLI(cli string, stdin string, args ...string) (keyState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, cli, args...)
	cmd.Stdin = strings.NewReader(stdin)
	b, err := cmd.Output()
	var s keyState
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			return s, errors.New(strings.TrimSpace(string(e.Stderr)))
		}
		return s, err
	}
	return s, json.Unmarshal(b, &s)
}

// hintOf is how a key is shown: its last four characters.
func hintOf(key string) string { return "…" + key[len(key)-4:] }

// keyPrompt is how the import talks to the person: confirm asks a yes/no
// question, tell shows a result.
type keyPrompt struct {
	confirm func(title, body, yes, no string) bool
	tell    func(title, body string)
}

// installKeyLink stores the key from a megaspeak://set-key link the way
// `mega-speak --set-key` does, asking before it replaces a different saved
// key, and says what happened. A key from the environment, api_key_file or
// api_key_cmd stays in force: the link's key is then not stored. Neither the
// key nor the link is logged or shown, only last-four hints. It reports
// whether a key was stored.
func installKeyLink(raw, cli string, p keyPrompt) bool {
	key, err := keyFromLink(raw)
	if err != nil {
		log.Printf("key link: %v", err)
		p.tell(i18n.T("key.bad.title"), i18n.T("key.bad.body"))
		return false
	}
	st, err := keyCLI(cli, "", "--key-state")
	if err != nil {
		log.Printf("key link: key state: %v", err)
		p.tell(i18n.T("key.failed.title"), i18n.F("key.failed.body", err))
		return false
	}
	switch st.Source {
	case "none":
	case "saved":
		saved, err := os.ReadFile(st.KeyFile)
		old := strings.TrimSpace(string(saved))
		if err == nil && subtle.ConstantTimeCompare([]byte(old), []byte(key)) == 1 {
			log.Printf("key link: that key is already saved")
			p.tell(i18n.T("key.same.title"), i18n.F("key.same.body", st.Hint))
			return false
		}
		if !p.confirm(i18n.T("key.replace.title"), i18n.F("key.replace.body", st.Hint, hintOf(key)),
			i18n.T("key.replace.yes"), i18n.T("key.replace.no")) {
			log.Printf("key link: kept the saved key")
			p.tell(i18n.T("key.kept.title"), i18n.F("key.kept.body", st.Hint))
			return false
		}
	default:
		log.Printf("key link: the key comes from %s; not stored", st.Source)
		p.tell(i18n.T("key.configured.title"), i18n.F("key.configured.body", st.Source))
		return false
	}
	st, err = keyCLI(cli, key, "--set-key")
	if err != nil {
		log.Printf("key link: set key: %v", err)
		p.tell(i18n.T("key.failed.title"), i18n.F("key.failed.body", err))
		return false
	}
	log.Printf("key link: key stored, source %s", st.Source)
	p.tell(i18n.T("key.installed.title"), i18n.F("key.installed.body", st.Hint))
	return true
}
