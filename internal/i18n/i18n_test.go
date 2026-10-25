package i18n

import (
	"go/scanner"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"
)

func TestPick(t *testing.T) {
	for tag, want := range map[string]Lang{
		"zh-Hans-CN": ZH, "zh-Hant-TW": ZH, "zh-HK": ZH, "zh": ZH, "ZH-hans": ZH, "zh_CN": ZH,
		"en-US": EN, "en-GB": EN, "en": EN, "ja-JP": EN, "fr-FR": EN, "": EN, "zu-ZA": EN,
	} {
		if got := Pick(tag); got != want {
			t.Errorf("Pick(%q) = %s, want %s", tag, got, want)
		}
	}
}

var verb = regexp.MustCompile(`%[sdvq]`)

func hasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func TestEveryLineHasBothLanguages(t *testing.T) {
	for k, l := range table {
		if strings.TrimSpace(l.zh) == "" || strings.TrimSpace(l.en) == "" {
			t.Errorf("%s: a language is empty: %q / %q", k, l.zh, l.en)
		}
		if hasHan(l.en) {
			t.Errorf("%s: the English line has Chinese in it: %q", k, l.en)
		}
		if !slices.Equal(verb.FindAllString(l.zh, -1), verb.FindAllString(l.en, -1)) {
			t.Errorf("%s: the two lines take different arguments: %q / %q", k, l.zh, l.en)
		}
	}
	for zh, en := range catalogValues {
		if !hasHan(zh) || en == "" || hasHan(en) {
			t.Errorf("catalog value %q -> %q", zh, en)
		}
	}
}

func TestLanguageFollowsSet(t *testing.T) {
	defer Set(Current())
	Set(EN)
	if T("update.update") != "Update" || F("update.title", "0.1.3") != "A new version of Mega Speak is available: 0.1.3" {
		t.Fatal(T("update.update"), F("update.title", "0.1.3"))
	}
	if p := Page("settings.html.", EN); p["lang"] != "en" || p["t"].(map[string]string)["save_key"] != "Save Key" || p["values"].(map[string]string)["男"] != "Male" {
		t.Fatalf("English page: %v", p["lang"])
	}
	Set(ZH)
	if T("update.update") != "更新" || T("update.skip") != "跳过此版本" || T("update.later") != "稍后" {
		t.Fatal(T("update.update"))
	}
	if p := Page("settings.html.", ZH); p["lang"] != "zh" || len(p["values"].(map[string]string)) != 0 {
		t.Fatalf("Chinese page: %v", p)
	}
}

func TestReleaseNotes(t *testing.T) {
	if ReleaseNotes(EN, "修复", "Fixes") != "Fixes" || ReleaseNotes(EN, "修复", " ") != "修复" || ReleaseNotes(ZH, "修复", "Fixes") != "修复" {
		t.Fatal("release notes pick")
	}
}

// sources walks the app's Go and HTML files outside this package and vendor/.
func sources(t *testing.T, fn func(path string, b []byte)) {
	t.Helper()
	root := "../.."
	for _, dir := range []string{"cmd", "internal"} {
		filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				t.Fatal(err)
			}
			if d.IsDir() && d.Name() == "i18n" {
				return filepath.SkipDir
			}
			if d.IsDir() || strings.HasSuffix(path, "_test.go") || !(strings.HasSuffix(path, ".go") || strings.HasSuffix(path, ".html")) {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			fn(path, b)
			return nil
		})
	}
}

// No Chinese line lives outside the table: every Go string literal and every
// HTML page of the app is free of it, so each line someone reads goes through
// T, F or a page's t().
func TestNoLineOutsideTheTable(t *testing.T) {
	n := 0
	sources(t, func(path string, b []byte) {
		n++
		if strings.HasSuffix(path, ".html") {
			if hasHan(string(b)) {
				t.Errorf("%s holds Chinese text; move it to the table", path)
			}
			return
		}
		var s scanner.Scanner
		fset := token.NewFileSet()
		s.Init(fset.AddFile(path, -1, len(b)), b, nil, 0)
		for {
			pos, tok, lit := s.Scan()
			if tok == token.EOF {
				break
			}
			if (tok == token.STRING || tok == token.CHAR) && hasHan(lit) {
				t.Errorf("%s: %s", fset.Position(pos), lit)
			}
		}
	})
	if n < 15 {
		t.Fatalf("scanned only %d files", n)
	}
}

var (
	goKey   = regexp.MustCompile(`\b(?:i18n\.)?(?:T|F)\("([a-z_.\-]+)"`)
	htmlKey = regexp.MustCompile(`data-(?:t|tp|ta|preset)="([a-z_]+)"|\bt\('([a-z_]+)'|\bt\([^;]*?\?'([a-z_]+)':'([a-z_]+)'`)
)

// Every key the app asks for has a line, and every line is asked for.
func TestKeysMatchTheTable(t *testing.T) {
	used := map[string]bool{}
	sources(t, func(path string, b []byte) {
		prefix := ""
		switch filepath.Base(path) {
		case "settings.html":
			prefix = "settings.html."
		case "picker.html":
			prefix = "picker."
		}
		if prefix == "" {
			for _, m := range goKey.FindAllStringSubmatch(string(b), -1) {
				used[m[1]] = true
				if _, ok := table[m[1]]; !ok {
					t.Errorf("%s asks for %q, which the table lacks", path, m[1])
				}
			}
			return
		}
		for _, m := range htmlKey.FindAllStringSubmatch(string(b), -1) {
			for _, k := range m[1:] {
				if k == "" {
					continue
				}
				used[prefix+k] = true
				if _, ok := table[prefix+k]; !ok {
					t.Errorf("%s asks for %q, which the table lacks", path, prefix+k)
				}
			}
		}
	})
	for k := range table {
		if !used[k] && !strings.HasPrefix(k, "picker.lang.") {
			t.Errorf("%s is in the table but nothing shows it", k)
		}
	}
}
