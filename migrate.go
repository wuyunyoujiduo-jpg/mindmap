package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/versions"
)

// The maps folder renamed, as in Finder, is found again and gone on in
// (followDir), with what the app keeps by the maps' paths.

// followPaths takes along what the app keeps by the maps' paths when the
// maps folder moved from from to to: their versions, their views, the map
// open last, the favorites, the recent ones, the folders shown open, the
// folders of their code, and what the sync remembers of the folder. The
// config is saved.
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
	for i := range c.OpenFolders {
		c.OpenFolders[i] = moved(c.OpenFolders[i])
	}
	if len(c.Projects) > 0 {
		projects := map[string]string{}
		for p, dir := range c.Projects {
			projects[moved(p)] = dir
		}
		c.Projects = projects
	}
	// The sync's memory of the folder, and the questions of its merges
	// that wait, are by the folder's path too.
	if old, now := syncStatePath(from), syncStatePath(to); old != "" {
		if _, err := os.Stat(now); err != nil {
			_ = os.Rename(old, now)
			_ = os.Rename(mergesPath(from), mergesPath(to))
		}
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
					_ = writeFile(p, b) // whole or not at all
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
