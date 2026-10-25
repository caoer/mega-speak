package voices

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/caoer/mega-speak/internal/i18n"
)

func TestCompleteCatalogAndLocalMedia(t *testing.T) {
	l, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Voices) != 445 {
		t.Fatalf("snapshot has %d voices, want 445", len(l.Voices))
	}
	root, err := l.Prepare(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range l.Voices {
		for _, name := range []string{v.Icon, v.Preview} {
			info, err := os.Stat(filepath.Join(root, name))
			if err != nil || info.Size() == 0 {
				t.Fatalf("%s: %v", name, err)
			}
		}
	}
	for _, lang := range []i18n.Lang{i18n.ZH, i18n.EN} {
		page, err := os.ReadFile(filepath.Join(root, PageName(lang)))
		if err != nil || len(page) == 0 || bytes.Contains(page, []byte("@@")) || !bytes.Contains(page, []byte(`"lang":"`+string(lang)+`"`)) {
			t.Fatalf("picker %s: %v", lang, err)
		}
	}
}
func TestFavoritesSurviveReopenAndImportPreservesExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "favorites.json")
	f, err := ReadFavorites(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Set("first", true); err != nil {
		t.Fatal(err)
	}
	if err = f.Import([]string{"second", "first"}); err != nil {
		t.Fatal(err)
	}
	f, err = ReadFavorites(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.List()) != 2 || !f.IDs["first"] || !f.IDs["second"] {
		t.Fatal(f.List())
	}
	if err = f.Set("first", false); err != nil {
		t.Fatal(err)
	}
	var ids []string
	b, _ := os.ReadFile(path)
	json.Unmarshal(b, &ids)
	if len(ids) != 1 || ids[0] != "second" {
		t.Fatal(ids)
	}
}
func TestFailedFavoritesWriteRestoresMemory(t *testing.T) {
	root := t.TempDir()
	blocked := filepath.Join(root, "not-a-directory")
	os.WriteFile(blocked, []byte("preserved"), 0600)
	f := &Favorites{Path: filepath.Join(blocked, "favorites.json"), IDs: map[string]bool{"kept": true}}
	if err := f.Set("new", true); err == nil {
		t.Fatal("expected write failure")
	}
	if f.IDs["new"] || !f.IDs["kept"] {
		t.Fatal(f.IDs)
	}
	b, _ := os.ReadFile(blocked)
	if string(b) != "preserved" {
		t.Fatal("foreign bytes changed")
	}
}
