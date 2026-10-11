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
	if err := os.Rename(from, to); err != nil {
		return false
	}
	followPaths(c, from, to)
	return true
}

// followPaths takes along what the app keeps by the maps' paths when the
// maps folder moved from from to to: their versions, their views, the map
// open last, the favorites and the recent ones. The config is saved.
func followPaths(c *config, from, to string) {
	moved := func(p string) string {
		if rel, err := filepath.Rel(from, p); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			return filepath.Join(to, rel)
		}
		if filepath.Clean(p) == filepath.Clean(from) {
			return to
		}
		return p
	}
	store := versions.Store{Dir: versionsDir()}
	_ = filepath.WalkDir(to, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, model.Ext) {
			if rel, err := filepath.Rel(to, p); err == nil {
				_ = store.Moved(filepath.Join(from, rel), p)
			}
		}
		return nil
	})
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
}

// followDir finds the maps folder again when it was renamed, as in Finder,
// by its identity among its parent's folders, and goes on in it under its
// new name. It reports whether it did.
func (a *app) followDir() bool {
	if a.dir == "" || a.dirID == (fileID{}) {
		return false
	}
	if _, err := os.Stat(a.dir); err == nil {
		return false
	}
	parent := filepath.Dir(a.dir)
	entries, err := os.ReadDir(parent)
	if err != nil {
		return false
	}
	for _, e := range entries {
		p := filepath.Join(parent, e.Name())
		if id, ok := idOf(p); e.IsDir() && ok && id == a.dirID {
			from, open := a.dir, ""
			if rel, err := filepath.Rel(from, a.path); a.path != "" && err == nil && !strings.HasPrefix(rel, "..") {
				open = filepath.Join(p, rel)
				a.path = open // saved where it is now, not where it was
			}
			a.flush()
			followPaths(&a.cfg, from, p)
			a.views = loadViews()
			a.openDir(p)
			if open != "" {
				_ = a.open(open)
			}
			a.tell("导图文件夹已改名为「" + e.Name() + "」")
			return true
		}
	}
	return false
}
