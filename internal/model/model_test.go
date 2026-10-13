package model

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func titles(ns []*Node) []string {
	var out []string
	for _, n := range ns {
		out = append(out, n.Title)
	}
	return out
}

func sample(t *testing.T) (*Document, map[string]*Node) {
	t.Helper()
	d := New("root")
	m := map[string]*Node{"root": d.Root}
	add := func(parent, title string) {
		n, err := d.AddChild(m[parent].ID, title)
		if err != nil {
			t.Fatal(err)
		}
		m[title] = n
	}
	add("root", "a")
	add("root", "b")
	add("root", "c")
	add("a", "a1")
	add("a", "a2")
	return d, m
}

func TestAddSiblingAndDelete(t *testing.T) {
	d, m := sample(t)
	s, err := d.AddSibling(m["a"].ID, "a+")
	if err != nil {
		t.Fatal(err)
	}
	if got := titles(d.Root.Children); !slices.Equal(got, []string{"a", "a+", "b", "c"}) {
		t.Fatalf("children %v", got)
	}
	next, err := d.Delete(s.ID)
	if err != nil || next != m["b"].ID {
		t.Fatalf("next %q err %v, want b", next, err)
	}
	next, _ = d.Delete(m["c"].ID)
	if next != m["b"].ID {
		t.Fatalf("next after deleting last: %q", next)
	}
	next, _ = d.Delete(m["a2"].ID)
	next, _ = d.Delete(m["a1"].ID)
	if next != m["a"].ID {
		t.Fatalf("next after deleting only child: %q", next)
	}
	if _, err := d.Delete(d.Root.ID); !errors.Is(err, ErrRoot) {
		t.Fatalf("deleting root: %v", err)
	}
	// The root's sibling is a child.
	r, _ := d.AddSibling(d.Root.ID, "x")
	if _, p := d.Find(r.ID); p != d.Root {
		t.Fatal("sibling of root is not its child")
	}
}

func TestMove(t *testing.T) {
	d, m := sample(t)
	if err := d.Move(m["a"].ID, m["a1"].ID, -1); !errors.Is(err, ErrCycle) {
		t.Fatalf("move into own child: %v", err)
	}
	if err := d.Move(m["c"].ID, m["a"].ID, 0); err != nil {
		t.Fatal(err)
	}
	if got := titles(m["a"].Children); !slices.Equal(got, []string{"c", "a1", "a2"}) {
		t.Fatalf("a's children %v", got)
	}
	// Moving forward within the same parent.
	if err := d.Move(m["c"].ID, m["a"].ID, 2); err != nil {
		t.Fatal(err)
	}
	if got := titles(m["a"].Children); !slices.Equal(got, []string{"a1", "c", "a2"}) {
		t.Fatalf("a's children after reorder %v", got)
	}
	if err := d.Shift(m["c"].ID, 5); err != nil {
		t.Fatal(err)
	}
	if got := titles(m["a"].Children); !slices.Equal(got, []string{"a1", "a2", "c"}) {
		t.Fatalf("after shift %v", got)
	}
}

func TestIndentOutdent(t *testing.T) {
	d, m := sample(t)
	if err := d.Indent(m["b"].ID); err != nil {
		t.Fatal(err)
	}
	if got := titles(m["a"].Children); !slices.Equal(got, []string{"a1", "a2", "b"}) {
		t.Fatalf("after indent %v", got)
	}
	if err := d.Outdent(m["b"].ID); err != nil {
		t.Fatal(err)
	}
	if got := titles(d.Root.Children); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Fatalf("after outdent %v", got)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	d, m := sample(t)
	due := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	m["a1"].Note = "备注\n第二行"
	m["a1"].Tags = []string{"核心", "bug"}
	m["a1"].Task = &Task{Status: StatusDoing, Priority: PriorityHigh, Due: &due}
	path := filepath.Join(t.TempDir(), FileName("我的/项目"))
	if filepath.Base(path) != "我的-项目"+Ext {
		t.Fatalf("file name %q", filepath.Base(path))
	}
	f := &File{Sheets: []*Document{d, New("second")}, Project: "/code/p", GitHub: "me/p", Measures: []string{"工时"},
		Iterations: []Iteration{{ID: "s1", Name: "第一轮"}}, Baseline: &Baseline{Commit: "abc", Branch: "main", At: due}}
	if err := f.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Sheets) != 2 || loaded.Sheets[1].Root.Title != "second" || loaded.Sheets[1].Title != "画布 2" {
		t.Fatalf("sheets %+v", loaded.Sheets)
	}
	// What the file keeps besides its canvases.
	if loaded.Project != "/code/p" || loaded.GitHub != "me/p" || len(loaded.Measures) != 1 || len(loaded.Iterations) != 1 ||
		loaded.Baseline == nil || loaded.Baseline.Commit != "abc" {
		t.Fatalf("file %+v", loaded)
	}
	got := loaded.Sheets[0]
	n, _ := got.Find(m["a1"].ID)
	if n == nil || n.Note != "备注\n第二行" || !n.HasTag("bug") || n.Task.Status != StatusDoing || !n.Task.Due.Equal(due) {
		t.Fatalf("loaded %+v", n)
	}
	files, err := ListFiles(filepath.Dir(path))
	if err != nil || len(files) != 1 || files[0].Name != "我的-项目" {
		t.Fatalf("files %+v err %v", files, err)
	}
}

func TestParseRepairsIDs(t *testing.T) {
	f, err := Parse([]byte(`{"version":2,"sheets":[{"title":"a","root":{"title":"r","children":[{"id":"x","title":"a"},{"id":"x","title":"b"}]}},{"root":{"id":"x","title":"r2"}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	d := f.Sheets[0]
	if d.ID == "" || d.Root.ID == "" || d.Root.Children[0].ID == d.Root.Children[1].ID {
		t.Fatal("IDs not repaired")
	}
	if f.Sheets[1].Root.ID == "x" || f.Sheets[1].ID == d.ID || f.Sheets[1].Title != "画布 2" {
		t.Fatalf("second sheet %+v", f.Sheets[1])
	}
	if _, err := Parse([]byte(`{"version":99,"sheets":[]}`)); err == nil {
		t.Fatal("newer version accepted")
	}
	if _, err := Parse([]byte(`{"version":2,"sheets":[]}`)); err == nil {
		t.Fatal("file without sheets accepted")
	}
}

func TestParseVersion1(t *testing.T) {
	f, err := Parse([]byte(`{"version":1,"root":{"id":"r","title":"项目","children":[{"id":"a","title":"a"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Sheets) != 1 || f.Sheets[0].Title != "画布 1" || f.Sheets[0].Root.Title != "项目" {
		t.Fatalf("sheets %+v", f.Sheets)
	}
	b, _ := f.Marshal()
	if !strings.Contains(string(b), `"version": 2`) || !strings.Contains(string(b), `"sheets"`) {
		t.Fatalf("saved as %s", b)
	}
}

func TestSheets(t *testing.T) {
	f := NewFile("项目")
	if f.Sheets[0].Title != "画布 1" {
		t.Fatal(f.Sheets[0].Title)
	}
	b := f.AddSheet(0)
	if b.Title != "画布 2" || f.Sheets[1] != b {
		t.Fatal("add")
	}
	child, _ := b.AddChild(b.Root.ID, "x")
	child.Task = &Task{Status: StatusTodo}
	child.Tags = []string{"t"}
	c := f.DuplicateSheet(1)
	if c.Title != "画布 2 副本" || c.Root.ID == b.Root.ID || c.Root.Children[0].ID == child.ID {
		t.Fatal("duplicate shares IDs")
	}
	if got := f.SheetOf(child.ID); got != b {
		t.Fatal("SheetOf")
	}
	tasks := f.Tasks()
	if len(tasks) != 2 || tasks[0].Sheet != b || tasks[1].Sheet != c {
		t.Fatalf("tasks %+v", tasks)
	}
	if got := f.AllTags(); !slices.Equal(got, []string{"t"}) {
		t.Fatal(got)
	}
	f.MoveSheet(2, 0)
	if f.Sheets[0] != c {
		t.Fatal("move")
	}
	f.DeleteSheet(0)
	f.DeleteSheet(0)
	if err := f.DeleteSheet(0); !errors.Is(err, ErrLastSheet) || len(f.Sheets) != 1 {
		t.Fatal("deleted the last sheet")
	}
}

func TestHistory(t *testing.T) {
	d, m := sample(t)
	f := &File{Sheets: []*Document{d}}
	h := NewHistory(10)
	h.Record(State{File: f, Sheet: d.ID, Selected: m["a"].ID})
	d.Delete(m["a"].ID)
	s, ok := h.Undo(State{File: f, Sheet: d.ID, Selected: m["b"].ID})
	if !ok || s.Selected != m["a"].ID || len(s.File.Sheets[0].Root.Children) != 3 {
		t.Fatalf("undo: ok %v sel %q", ok, s.Selected)
	}
	s2, ok := h.Redo(s)
	if !ok || s2.Selected != m["b"].ID || len(s2.File.Sheets[0].Root.Children) != 2 {
		t.Fatalf("redo: ok %v sel %q", ok, s2.Selected)
	}
	// Snapshots are independent copies.
	s2.File.Sheets[0].Root.Title = "changed"
	s3, _ := h.Undo(s2)
	if s3.File.Sheets[0].Root.Title != "root" {
		t.Fatal("snapshot shares nodes with the file")
	}
}

func TestTasksAndTags(t *testing.T) {
	d, m := sample(t)
	m["a2"].Task = &Task{Status: StatusTodo}
	m["c"].Task = &Task{Status: StatusDone}
	m["a2"].Tags = []string{"z", "y"}
	m["b"].Tags = []string{"y"}
	tasks := d.Tasks()
	if len(tasks) != 2 || tasks[0].Node != m["a2"] || !slices.Equal(tasks[0].Parents, []string{"root", "a"}) {
		t.Fatalf("tasks %+v", tasks)
	}
	if got := d.AllTags(); !slices.Equal(got, []string{"y", "z"}) {
		t.Fatalf("tags %v", got)
	}
	yesterday := time.Now().AddDate(0, 0, -1)
	if !(&Task{Status: StatusTodo, Due: &yesterday}).Overdue(time.Now()) {
		t.Fatal("not overdue")
	}
	today := time.Now()
	if (&Task{Status: StatusTodo, Due: &today}).Overdue(time.Now()) {
		t.Fatal("due today is overdue")
	}
}

func TestScanFolder(t *testing.T) {
	dir := t.TempDir()
	mk := func(rel string) string {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := NewFile("x").Save(p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	top := mk("a" + Ext)
	inner := mk("工作/项目A/b" + Ext)
	mk(".hidden/c" + Ext)
	os.WriteFile(filepath.Join(dir, "notes.txt"), nil, 0o644)

	f, err := ScanFolder(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Files) != 1 || len(f.Folders) != 1 || f.Folders[0].Name != "工作" {
		t.Fatalf("scanned %+v", f)
	}
	if got := f.Folders[0].Folders[0].Files; len(got) != 1 || got[0].Path != inner {
		t.Fatalf("inner %+v", got)
	}
	if len(f.AllFiles()) != 2 || len(f.FolderPaths()) != 3 {
		t.Fatalf("%d files, %d folders", len(f.AllFiles()), len(f.FolderPaths()))
	}
	moved, err := MoveFile(top, filepath.Join(dir, "工作"))
	if err != nil || filepath.Dir(moved) != filepath.Join(dir, "工作") {
		t.Fatalf("moved to %q: %v", moved, err)
	}
	mk("a" + Ext)
	if _, err := MoveFile(filepath.Join(dir, "a"+Ext), filepath.Join(dir, "工作")); err == nil {
		t.Fatal("replaced a file of the same name")
	}
}
