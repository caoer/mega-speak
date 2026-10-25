// Package i18n holds every line Mega Speak shows a person, in Chinese and
// English, and picks one language for the whole app: Chinese when the first
// macOS preferred language is Chinese, English otherwise.
package i18n

import (
	"fmt"
	"strings"
	"sync/atomic"
)

// Lang is a language the app speaks.
type Lang string

const (
	ZH Lang = "zh"
	EN Lang = "en"
)

// Pick is the app's language for the first macOS preferred language, a BCP 47
// tag such as "zh-Hans-CN" or "en-US": Chinese for any zh tag, English for
// every other tag and for none.
func Pick(tag string) Lang {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "zh" || strings.HasPrefix(tag, "zh-") || strings.HasPrefix(tag, "zh_") {
		return ZH
	}
	return EN
}

var current atomic.Value

func init() { current.Store(ZH) }

// Set makes l the language of every later T and F.
func Set(l Lang) { current.Store(l) }

// Current is the app's language.
func Current() Lang { return current.Load().(Lang) }

// text is one line in both languages.
type text struct{ zh, en string }

func (t text) in(l Lang) string {
	if l == EN {
		return t.en
	}
	return t.zh
}

// In is the line named key in l; a key with no line is returned as itself.
func In(l Lang, key string) string {
	if t, ok := table[key]; ok {
		return t.in(l)
	}
	return key
}

// T is the line named key in the current language.
func T(key string) string { return In(Current(), key) }

// F is T formatted with args.
func F(key string, args ...any) string { return fmt.Sprintf(T(key), args...) }

// Page is what an HTML page of the app needs in l: its lines, keyed without
// prefix, and the catalogue's own values (voice gender, age, category) shown
// in l, keyed by the Chinese value. Chinese values show as they are.
func Page(prefix string, l Lang) map[string]any {
	lines := map[string]string{}
	for k, t := range table {
		if rest, ok := strings.CutPrefix(k, prefix); ok {
			lines[rest] = t.in(l)
		}
	}
	values := map[string]string{}
	if l == EN {
		for zh, en := range catalogValues {
			values[zh] = en
		}
	}
	return map[string]any{"lang": string(l), "t": lines, "values": values}
}

// ReleaseNotes picks the feed's notes for l: notes_en in English when the
// feed carries it, else notes (Chinese, which 0.1.2 apps read).
func ReleaseNotes(l Lang, zh, en string) string {
	if l == EN && strings.TrimSpace(en) != "" {
		return en
	}
	return zh
}
