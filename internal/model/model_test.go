package model

import (
	"errors"
	"path/filepath"
	"slices"
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
	if err := d.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
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
	d, err := Parse([]byte(`{"version":1,"root":{"title":"r","children":[{"id":"x","title":"a"},{"id":"x","title":"b"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if d.Root.ID == "" || d.Root.Children[0].ID == d.Root.Children[1].ID {
		t.Fatal("IDs not repaired")
	}
	if _, err := Parse([]byte(`{"version":99,"root":{"title":"r"}}`)); err == nil {
		t.Fatal("newer version accepted")
	}
}

func TestHistory(t *testing.T) {
	d, m := sample(t)
	h := NewHistory(10)
	h.Record(d, m["a"].ID)
	d.Delete(m["a"].ID)
	d2, sel, ok := h.Undo(d, m["b"].ID)
	if !ok || sel != m["a"].ID || len(d2.Root.Children) != 3 {
		t.Fatalf("undo: ok %v sel %q children %d", ok, sel, len(d2.Root.Children))
	}
	d3, sel, ok := h.Redo(d2, sel)
	if !ok || sel != m["b"].ID || len(d3.Root.Children) != 2 {
		t.Fatalf("redo: ok %v sel %q children %d", ok, sel, len(d3.Root.Children))
	}
	// Snapshots are independent copies.
	d3.Root.Title = "changed"
	d4, _, _ := h.Undo(d3, "")
	if d4.Root.Title != "root" {
		t.Fatal("snapshot shares nodes with the document")
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
