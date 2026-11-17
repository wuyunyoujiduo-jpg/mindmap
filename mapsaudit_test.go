package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// Several topics chosen, of which one holds the others: Delete takes that
// one, not the child chosen last; and the root chosen with others does
// not stand for them.
func TestDeleteSelectionOfABranch(t *testing.T) {
	a, tt := newTestApp(t)
	root := a.doc.Root
	p, _ := a.doc.AddChild(root.ID, "要删的分支")
	c1, _ := a.doc.AddChild(p.ID, "甲")
	c2, _ := a.doc.AddChild(p.ID, "乙")
	a.selected = c2.ID
	a.multi = map[string]bool{p.ID: true, c1.ID: true}
	a.deleteSelection()
	tt.Frame()
	if n, _ := a.doc.Find(p.ID); n != nil {
		t.Fatal("the branch chosen stayed, and its last child went alone")
	}
	other, _ := a.doc.AddChild(root.ID, "另一个")
	a.selected = other.ID
	a.multi = map[string]bool{root.ID: true}
	if got := a.topSelection(); len(got) != 1 || got[0] != other {
		t.Fatalf("with the root chosen too: %d topics", len(got))
	}
}

// A branch is moved under a topic of another canvas of the same map.
func TestMoveToAnotherCanvas(t *testing.T) {
	a, tt := newTestApp(t)
	n, _ := a.doc.AddChild(a.doc.Root.ID, "要搬的")
	first := a.doc
	second := a.file.AddSheet(0)
	a.selected, a.multi = n.ID, nil
	a.moveToMap(a.path, second.ID, second.Root.ID)
	tt.Frame()
	if got, _ := first.Find(n.ID); got != nil {
		t.Fatal("still on the first canvas")
	}
	if got, p := second.Find(n.ID); got == nil || p != second.Root {
		t.Fatal("not under the other canvas's topic")
	}
}

// A template is of no project, whatever map it was saved from; a view
// chosen keeps the title being written.
func TestTemplateIsOfNoProject(t *testing.T) {
	a, tt := newTestApp(t)
	a.file.Project, a.file.Repo = "/code/ergora", "github.com/o/ergora"
	a.file.Baseline = &model.Baseline{Commit: "abcdef1"}
	a.file.Log = []model.LogEntry{{}}
	a.saveAsTemplate()
	files, _ := filepath.Glob(filepath.Join(templatesDir(), "*.mindmap.json"))
	if len(files) != 1 {
		t.Fatalf("templates %v", files)
	}
	f, err := model.Load(files[0])
	if err != nil || f.Project != "" || f.Repo != "" || f.Baseline != nil || len(f.Log) != 0 {
		t.Fatalf("the template is a project's: %+v (%v)", f, err)
	}
	if a.file.Project == "" {
		t.Fatal("the map itself lost its project")
	}
	_ = os.Remove(files[0])

	n, _ := a.doc.AddChild(a.doc.Root.ID, "")
	a.selected = n.ID
	a.startEdit(n.ID)
	a.editText = "写到一半的标题"
	a.showView(viewOutline)
	tt.Frame()
	if a.editing != "" || n.Title != "写到一半的标题" {
		t.Fatalf("the title being written: editing %q, title %q", a.editing, n.Title)
	}
}

// A map left, changed by another program, and opened again: ⌘Z does not
// put it back to before that change.
func TestUndoDoesNotCrossAnOutsideChange(t *testing.T) {
	a, tt := newTestApp(t)
	first := a.path
	a.mutate("", func() { a.doc.AddChild(a.doc.Root.ID, "我加的") })
	a.flush()
	a.newMap()
	tt.Frame()
	if a.path == first {
		t.Fatal("no other map opened")
	}
	f, err := model.Load(first)
	if err != nil {
		t.Fatal(err)
	}
	f.Sheets[0].AddChild(f.Sheets[0].Root.ID, "外面加的")
	if err := f.Save(first); err != nil {
		t.Fatal(err)
	}
	later := testNow.Add(time.Hour)
	os.Chtimes(first, later, later)
	if err := a.open(first); err != nil {
		t.Fatal(err)
	}
	a.undo()
	a.flush()
	f, _ = model.Load(first)
	found := false
	f.Sheets[0].Walk(func(n, _ *model.Node, _ int) bool {
		found = found || n.Title == "外面加的"
		return true
	})
	if !found {
		t.Fatal("undo took away what was added outside")
	}
}
