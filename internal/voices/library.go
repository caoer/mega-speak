// Package voices holds a frozen, public voice catalog and its local sample audio.
package voices

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed catalog.json picker.html media.zip
var bundle embed.FS

type Voice struct {
	Name        string              `json:"name"`
	ID          string              `json:"voice_id"`
	Description string              `json:"description"`
	Gender      string              `json:"gender"`
	Age         string              `json:"age"`
	Categories  []string            `json:"categories"`
	Languages   []map[string]string `json:"languages"`
	Labels      []string            `json:"labels"`
	Tags        []string            `json:"tags"`
	Emotions    []any               `json:"emotions"`
	Resource    string              `json:"resource_id"`
	Icon        string              `json:"icon"`
	Preview     string              `json:"preview"`
	IconHash    string              `json:"icon_sha256"`
	PreviewHash string              `json:"preview_sha256"`
	PreviewURL  string              `json:"preview_url"`
}

type Library struct {
	Captured string  `json:"captured"`
	Source   string  `json:"source"`
	Voices   []Voice `json:"voices"`
}

func Load() (Library, error) {
	var l Library
	b, err := bundle.ReadFile("catalog.json")
	if err == nil {
		err = json.Unmarshal(b, &l)
	}
	if err != nil {
		return l, err
	}
	seen := map[string]bool{}
	for _, v := range l.Voices {
		if v.ID == "" || seen[v.ID] {
			return l, fmt.Errorf("missing or duplicate voice ID: %s", v.ID)
		}
		seen[v.ID] = true
	}
	return l, nil
}

// Prepare verifies every packaged file and atomically installs one complete
// version of the library. A failed extraction never exposes a partial catalog.
func (l Library) Prepare(parent string) (string, error) {
	b, err := bundle.ReadFile("media.zip")
	if err != nil {
		return "", err
	}
	catalog, err := bundle.ReadFile("catalog.json")
	if err != nil {
		return "", err
	}
	page, err := bundle.ReadFile("picker.html")
	if err != nil {
		return "", err
	}
	hasher := sha256.New()
	hasher.Write(b)
	hasher.Write(catalog)
	hasher.Write(page)
	stamp := hex.EncodeToString(hasher.Sum(nil))
	dest := filepath.Join(parent, "voices-"+stamp[:16])
	if ready, err := os.ReadFile(filepath.Join(dest, ".ready")); err == nil && string(ready) == stamp {
		return dest, nil
	}
	if err = os.MkdirAll(parent, 0700); err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(parent, ".voices-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	reader, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return "", err
	}
	expected := map[string]string{}
	for _, v := range l.Voices {
		expected[v.Icon] = v.IconHash
		expected[v.Preview] = v.PreviewHash
	}
	seen := map[string]bool{}
	for _, f := range reader.File {
		want, ok := expected[f.Name]
		if !ok || seen[f.Name] || f.Name != filepath.ToSlash(filepath.Clean(f.Name)) || strings.HasPrefix(f.Name, "/") || strings.Contains(f.Name, "..") || f.Mode()&os.ModeSymlink != 0 || f.UncompressedSize64 > 16<<20 {
			return "", fmt.Errorf("invalid voice resource: %s", f.Name)
		}
		stream, err := f.Open()
		if err != nil {
			return "", err
		}
		content, err := io.ReadAll(io.LimitReader(stream, 16<<20+1))
		stream.Close()
		if err != nil {
			return "", err
		}
		digest := sha256.Sum256(content)
		if len(content) > 16<<20 || hex.EncodeToString(digest[:]) != want {
			return "", fmt.Errorf("checksum mismatch: %s", f.Name)
		}
		path := filepath.Join(stage, filepath.FromSlash(f.Name))
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return "", err
		}
		if err = os.WriteFile(path, content, 0600); err != nil {
			return "", err
		}
		seen[f.Name] = true
	}
	if len(seen) != len(expected) {
		return "", fmt.Errorf("incomplete voice resources: %d/%d", len(seen), len(expected))
	}
	data, err := json.Marshal(l)
	if err != nil {
		return "", err
	}
	page = bytes.ReplaceAll(page, []byte("@@CATALOG@@"), data)
	if err = os.WriteFile(filepath.Join(stage, "picker.html"), page, 0600); err != nil {
		return "", err
	}
	if err = os.WriteFile(filepath.Join(stage, ".ready"), []byte(stamp), 0600); err != nil {
		return "", err
	}
	if err = os.Rename(stage, dest); err != nil {
		if ready, readErr := os.ReadFile(filepath.Join(dest, ".ready")); readErr == nil && string(ready) == stamp {
			return dest, nil
		}
		return "", err
	}
	return dest, nil
}

type Favorites struct {
	Path string
	IDs  map[string]bool
}

func ReadFavorites(path string) (*Favorites, error) {
	f := &Favorites{Path: path, IDs: map[string]bool{}}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return f, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []string
	if err = json.Unmarshal(b, &ids); err != nil {
		return nil, fmt.Errorf("cannot read favorites: %w", err)
	}
	for _, id := range ids {
		f.IDs[id] = true
	}
	return f, nil
}
func (f *Favorites) List() []string {
	ids := make([]string, 0, len(f.IDs))
	for id, yes := range f.IDs {
		if yes {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}
func (f *Favorites) Set(id string, yes bool) error {
	before := f.IDs[id]
	f.IDs[id] = yes
	if !yes {
		delete(f.IDs, id)
	}
	b, _ := json.MarshalIndent(f.List(), "", "  ")
	err := atomicWrite(f.Path, append(b, '\n'))
	if err != nil {
		if before {
			f.IDs[id] = true
		} else {
			delete(f.IDs, id)
		}
	}
	return err
}
func (f *Favorites) Import(ids []string) error {
	before := f.IDs
	f.IDs = map[string]bool{}
	for id, v := range before {
		f.IDs[id] = v
	}
	for _, id := range ids {
		f.IDs[id] = true
	}
	b, _ := json.MarshalIndent(f.List(), "", "  ")
	if err := atomicWrite(f.Path, append(b, '\n')); err != nil {
		f.IDs = before
		return err
	}
	return nil
}
func atomicWrite(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".favorites-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
