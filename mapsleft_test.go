package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// A task moved from the open map to a topic of another, as one dragged
// from 今天 onto a project, and the step undone: it was in both maps. And
// one moved into the open map, undone, was in neither. The move is
// followed as a branch's is.
func TestTaskMovedAcrossUndoneOnBothSides(t *testing.T) {
	a, tt := newTestApp(t)
	other, _, kid := newMapFile(t, a, "归档", "旧需求")
	n := find(t, a, "前端")
	id := n.ID
	in := func(path string) bool {
		t.Helper()
		f, err := model.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		return f.Node(id) != nil
	}
	here := func() bool { return a.file.Node(id) != nil }

	// Out of the open map.
	if err := a.moveNodeAcross(a.path, id, other, kid.ID); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if here() || !in(other) {
		t.Fatal("not moved")
	}
	a.undo()
	tt.Frame()
	if !here() || in(other) {
		t.Fatalf("undone: here %v, in the other map %v", here(), in(other))
	}
	a.redo()
	if here() || !in(other) {
		t.Fatalf("redone: here %v, in the other map %v", here(), in(other))
	}

	// Back into it, as the toast's 撤销 moves it: undone, it is in the
	// other map again, not in neither.
	if err := a.moveNodeAcross(other, id, a.path, a.doc.Root.ID); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !here() || in(other) {
		t.Fatal("not moved back")
	}
	a.undo()
	if here() || !in(other) {
		t.Fatalf("the move back undone: here %v, in the other map %v", here(), in(other))
	}
	a.redo()
	if !here() || in(other) {
		t.Fatalf("the move back redone: here %v, in the other map %v", here(), in(other))
	}
	// And further back, to before the first move: here alone.
	a.undo()
	a.undo()
	if !here() || in(other) {
		t.Fatalf("all undone: here %v, in the other map %v", here(), in(other))
	}
}

// A map whose file has a summary with no child to sum up, as one written
// before the app saw to them: it is mended as the map is opened, not in
// the first change made to it, whose 撤销 took the user's own change back
// with it. A map with none is not marked changed for being opened.
func TestSummaryOfAFileHealedAsItOpens(t *testing.T) {
	a, tt := newTestApp(t)
	f := model.NewFile("旧文件")
	d := f.Sheets[0]
	p, _ := d.AddChild(d.Root.ID, "只剩概要的主题")
	sum := model.NewNode("看不见的概要")
	p.Summary = &model.Mark{Topic: sum}
	other, _ := d.AddChild(d.Root.ID, "别的主题")
	path := filepath.Join(a.dir, model.FileName("旧文件"))
	if err := f.Save(path); err != nil {
		t.Fatal(err)
	}
	a.refreshFiles()
	first := a.path
	if a.dirty {
		t.Fatal("a map with nothing to mend is changed by being opened")
	}
	if err := a.open(path); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	hp, _ := a.doc.Find(p.ID)
	if hp.Summary != nil || len(hp.Children) != 1 || hp.Children[0].ID != sum.ID {
		t.Fatal("the summary with no child is not a child as the map opens")
	}
	if _, ok := a.boxes.Boxes[sum.ID]; !ok {
		t.Fatal("it is not on the canvas")
	}
	if !a.dirty || !strings.Contains(a.notice, "看不见的概要") || a.noticeUndo != nil {
		t.Fatalf("mended: changed %v, said %q", a.dirty, a.notice)
	}
	if a.hist.CanUndo() {
		t.Fatal("the mending is a step of the history")
	}
	// The first change made, and its 撤销: the user's change alone.
	o, _ := a.doc.Find(other.ID)
	a.mutate("", func() { o.Title = "改过的主题" })
	if a.noticeUndo != nil && strings.Contains(a.notice, "改成了") {
		t.Fatalf("the user's change carries the mending: %q", a.notice)
	}
	a.undo()
	hp, _ = a.doc.Find(p.ID)
	if o, _ = a.doc.Find(other.ID); o.Title != "别的主题" || hp.Summary != nil || len(hp.Children) != 1 {
		t.Fatal("undo of the user's change took the mending back")
	}
	// Saved so, and opened again with nothing to mend.
	a.flush()
	if err := a.open(first); err != nil {
		t.Fatal(err)
	}
	if err := a.open(path); err != nil {
		t.Fatal(err)
	}
	if a.dirty {
		t.Fatal("mended and saved, it is changed again by being opened")
	}
}
