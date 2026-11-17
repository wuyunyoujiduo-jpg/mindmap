package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/ghsync"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// A map renamed on the other computer alone, open here: the sync removes
// its file and writes the new one, and the app follows it there. An edit
// made here while the sync ran is not saved under the old name meanwhile,
// and is merged with the map as it came, its steps kept.
func TestSyncRenamedThereFollowed(t *testing.T) {
	a, tt := newTestApp(t)
	a.cfg.SyncRepo, a.cfg.SyncToken = "me/notes", "github_pat_x"
	from := filepath.Join(a.dir, "计划.mindmap.json")
	to := filepath.Join(a.dir, "规划.mindmap.json")
	f := model.NewFile("计划")
	if err := f.Save(from); err != nil {
		t.Fatal(err)
	}
	a.refreshFiles()
	if err := a.open(from); err != nil {
		t.Fatal(err)
	}
	var work func() (any, error)
	var done func(any, error)
	a.async = func(w func() (any, error), d func(any, error)) { work, done = w, d }
	a.syncRun = func(_ context.Context, dir string, cfg ghsync.Config, _ func(string)) (ghsync.Result, error) {
		g := f.Clone()
		g.Sheets[0].Root.Title = "规划"
		data, _ := g.Marshal()
		os.WriteFile(filepath.Join(dir, "规划.mindmap.json"), data, 0o644)
		os.Remove(filepath.Join(dir, "计划.mindmap.json"))
		return ghsync.Result{Pulled: 1, Deleted: 1, Written: []string{"计划.mindmap.json", "规划.mindmap.json"},
			Renamed: map[string]string{"计划.mindmap.json": "规划.mindmap.json"}}, nil
	}
	a.syncNow()
	a.mutate("", func() { a.doc.Root.Note = "这边改的" })
	v, err := work()
	// The autosave comes while the sync is under way.
	a.now = func() time.Time { return testNow.Add(time.Minute) }
	tt.Frame()
	if _, err := os.Stat(from); err == nil {
		t.Fatal("saved under the old name while the sync ran")
	}
	done(v, err)
	a.checked = time.Time{}
	tt.Frame()
	a.now = func() time.Time { return testNow.Add(2 * time.Minute) }
	tt.Frame()
	if a.path != to || a.doc.Root.Title != "规划" || a.doc.Root.Note != "这边改的" {
		t.Fatalf("not followed: %q, %q, %q", a.path, a.doc.Root.Title, a.doc.Root.Note)
	}
	a.save()
	if _, err := os.Stat(from); err == nil {
		t.Fatal("the old name came back")
	}
	if g, _ := model.Load(to); g.Sheets[0].Root.Note != "这边改的" || g.Sheets[0].Root.Title != "规划" {
		t.Fatal("not saved under the new name")
	}
	// The steps are kept: the merge, and then the edit, taken back.
	a.undo()
	a.undo()
	if a.doc.Root.Note != "" {
		t.Fatalf("the edit's step was lost: %q", a.doc.Root.Note)
	}
}

// A map deleted on the other computer, open here: closed, and said, when
// it was not changed here; kept under its name when it was.
func TestSyncDeletedThereOpen(t *testing.T) {
	for _, changed := range []bool{false, true} {
		a, tt := newTestApp(t)
		a.cfg.SyncRepo, a.cfg.SyncToken = "me/notes", "github_pat_x"
		p := filepath.Join(a.dir, "计划.mindmap.json")
		if err := model.NewFile("计划").Save(p); err != nil {
			t.Fatal(err)
		}
		a.refreshFiles()
		if err := a.open(p); err != nil {
			t.Fatal(err)
		}
		var work func() (any, error)
		var done func(any, error)
		a.async = func(w func() (any, error), d func(any, error)) { work, done = w, d }
		a.syncRun = func(_ context.Context, dir string, _ ghsync.Config, _ func(string)) (ghsync.Result, error) {
			os.Remove(filepath.Join(dir, "计划.mindmap.json"))
			return ghsync.Result{Deleted: 1, Written: []string{"计划.mindmap.json"}}, nil
		}
		a.syncNow()
		if changed {
			a.mutate("", func() { a.doc.Root.Note = "这边改的" })
		}
		done(work())
		tt.Frame()
		if !changed {
			if a.path == p || slices.ContainsFunc(a.files, func(f model.FileInfo) bool { return f.Path == p }) {
				t.Fatalf("still open: %q", a.path)
			}
			if _, err := os.Stat(p); err == nil {
				t.Fatal("written again")
			}
			if !strings.Contains(a.notice, "在另一台电脑上删了") {
				t.Fatalf("not said: %q", a.notice)
			}
			continue
		}
		if g, err := model.Load(p); err != nil || g.Sheets[0].Root.Note != "这边改的" || a.path != p || a.dirty {
			t.Fatalf("the edit here was not kept: %v", err)
		}
		if !strings.Contains(a.notice, "已留下") {
			t.Fatalf("not said: %q", a.notice)
		}
	}
}
