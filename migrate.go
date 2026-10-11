package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/versions"
)

// The app was MindMap before 0.25.13, and its maps folder, unless the user
// chose another, ~/Documents/MindMap. It becomes ~/Documents/MyMind, with
// what the app keeps by the maps' paths: their versions, their views, the
// map open last, the favorites and the recent ones.

// oldDefaultDir is the maps folder of before.
func oldDefaultDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Documents", "MindMap")
}

// moveOldDir moves the maps folder of before to the default one, when the
// app uses it and nothing is in the way, and reports whether it did.
func moveOldDir(c *config) bool {
	from, to := oldDefaultDir(), defaultDir()
	if from == "" || filepath.Clean(c.Dir) != from {
		return false
	}
	if info, err := os.Stat(from); err != nil || !info.IsDir() {
		return false
	}
	if _, err := os.Stat(to); err == nil {
		return false // a folder of the new name is there already
	}
	var maps []string
	_ = filepath.WalkDir(from, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, model.Ext) {
			maps = append(maps, p)
		}
		return nil
	})
	if err := os.Rename(from, to); err != nil {
		return false
	}
	moved := func(p string) string {
		if rel, err := filepath.Rel(from, p); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.Join(to, rel)
		}
		return p
	}
	store := versions.Store{Dir: versionsDir()}
	for _, p := range maps {
		_ = store.Moved(p, moved(p))
	}
	c.Dir, c.File = to, moved(c.File)
	for i := range c.Favorites {
		c.Favorites[i] = moved(c.Favorites[i])
	}
	for i := range c.Recent {
		c.Recent[i] = moved(c.Recent[i])
	}
	c.save()
	if p := viewsPath(); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			var views map[string]json.RawMessage
			if json.Unmarshal(b, &views) == nil {
				out := map[string]json.RawMessage{}
				for k, v := range views {
					out[moved(k)] = v
				}
				if b, err := json.Marshal(out); err == nil {
					_ = os.WriteFile(p, b, 0o644)
				}
			}
		}
	}
	return true
}
