package model

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// What a topic says is its title, note, task, tags and links: a change of
// one is told by its print and stamped; where it stands and how it looks
// are not.
func TestStampTellsWhatChanged(t *testing.T) {
	f := mergeBase()
	now := time.Date(2026, 11, 17, 10, 0, 0, 500, time.UTC)
	p := PrintsOf(f)

	f.Node("a1").Title = "Opus 5"
	f.Node("a2").Note = "快"
	f.Node("b").Tags = append(f.Node("b").Tags, "y")
	f.Node("c").Links = []string{"clip:1"}
	f.Node("a").Task = &Task{Status: StatusTodo}
	f.Node("r").Collapsed = true                  // how it is seen
	f.Node("r2").Style = &NodeStyle{Fill: "#fff"} // how it looks
	made, _ := f.Sheets[0].AddChild("r", "新的")
	if err := f.Sheets[0].Move("c", "a", 0); err != nil { // where it stands
		t.Fatal(err)
	}
	pasted := &Node{ID: "p", Title: "贴来的", Changed: now.AddDate(0, -4, 0)}
	f.Node("b").Children = append(f.Node("b").Children, pasted)

	got := map[string]bool{}
	for _, n := range p.Stamp(f, now) {
		got[n.ID] = true
	}
	for _, id := range []string{"a1", "a2", "b", "c", "a", made.ID} {
		if !got[id] || !f.Node(id).Changed.Equal(now.Truncate(time.Second)) {
			t.Errorf("%s was changed and is not said so: %v", id, f.Node(id).Changed)
		}
	}
	for _, id := range []string{"r", "r2"} {
		if got[id] || !f.Node(id).Changed.IsZero() {
			t.Errorf("%s said nothing else and is said changed", id)
		}
	}
	if got["p"] || !pasted.Changed.Equal(now.AddDate(0, -4, 0)) {
		t.Errorf("a topic that came with its own time lost it: %v", pasted.Changed)
	}
	// The prints are the file's now: nothing more to say.
	if again := p.Stamp(f, now.Add(time.Hour)); len(again) != 0 {
		t.Errorf("%d topics stamped twice", len(again))
	}
	// A task's state and day are of what it says.
	due := now.AddDate(0, 0, 3)
	f.Node("a").Task.Due = &due
	if again := p.Stamp(f, now.Add(time.Hour)); len(again) != 1 || again[0].ID != "a" {
		t.Errorf("a task's day changed: %v", again)
	}
}

// The times are kept through the file, a copy and the history's clone; a
// file that says none for its topics takes its own time when read, and
// keeps it once written.
func TestChangedKept(t *testing.T) {
	f := mergeBase()
	at := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	f.Node("a1").Changed = at
	path := filepath.Join(t.TempDir(), "m"+Ext)
	if err := f.Save(path); err != nil {
		t.Fatal(err)
	}
	old := time.Date(2026, 3, 2, 8, 0, 0, 0, time.UTC)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	g, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !g.Node("a1").Changed.Equal(at) || !g.Clone().Node("a1").Changed.Equal(at) {
		t.Fatalf("a topic's time was not kept: %v", g.Node("a1").Changed)
	}
	if !g.Dated.Equal(old) || !g.Clone().Dated.Equal(old) {
		t.Fatalf("the file's topics are of %v, not of its time %v", g.Dated, old)
	}
	if got := g.ChangedAt(g.Node("a2")); !got.Equal(old) {
		t.Fatalf("a topic with no time is of %v, not the file's", got)
	}
	if got := g.ChangedAt(g.Node("a1")); !got.Equal(at) {
		t.Fatalf("a topic's own time gives way to the file's: %v", got)
	}
	// Written, the file is of now, and its topics still of then.
	if err := g.Save(path); err != nil {
		t.Fatal(err)
	}
	if h, err := Load(path); err != nil || !h.Dated.Equal(old) {
		t.Fatalf("the time was lost at a save: %v %v", h.Dated, err)
	}
	// When a topic changed is not what the map says.
	h := g.Clone()
	h.Node("b").Changed, h.Dated = at, at
	if !SameContent(g, h) {
		t.Fatal("two maps that say the same differ by a time")
	}
}

// Merged, a topic both changed is of the later time, with nothing asked;
// a side that has no times (an app before them wrote it) takes none away.
func TestMergeChanged(t *testing.T) {
	base := mergeBase()
	t0 := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	base.Dated = t0
	base.Node("a1").Changed = t0
	ours, theirs := base.Clone(), base.Clone()
	t1, t2 := t0.AddDate(0, 0, 3), t0.AddDate(0, 0, 5)
	ours.Node("a1").Title, ours.Node("a1").Changed = "Opus 5", t1
	theirs.Node("a1").Note, theirs.Node("a1").Changed = "最强", t2
	ours.Node("b").Note, ours.Node("b").Changed = "我的", t2
	theirs.Node("b").Tags, theirs.Node("b").Changed = []string{"x", "z"}, t1
	ours.Node("c").Title, ours.Node("c").Changed = "论文集", t1
	theirs.Dated = t0.AddDate(0, 0, -9)

	m, clashes := Merge(base, ours, theirs)
	if len(clashes) != 0 {
		t.Fatalf("the times are asked about: %+v", clashes)
	}
	for id, want := range map[string]time.Time{"a1": t2, "b": t2, "c": t1} {
		if got := m.Node(id).Changed; !got.Equal(want) {
			t.Errorf("%s is of %v, the later is %v", id, got, want)
		}
	}
	if m.Node("a1").Title != "Opus 5" || m.Node("a1").Note != "最强" {
		t.Errorf("the merge itself: %+v", m.Node("a1"))
	}
	if !m.Dated.Equal(theirs.Dated) {
		t.Errorf("the file's time is %v, the earlier is %v", m.Dated, theirs.Dated)
	}

	// The other side written by an app that knows no times: all of them
	// gone there, and none lost here.
	bare := base.Clone()
	bare.Dated = time.Time{}
	bare.Walk(func(n *Node) { n.Changed = time.Time{} })
	bare.Node("a2").Note = "快"
	m, clashes = Merge(base, ours, bare)
	if len(clashes) != 0 {
		t.Fatalf("asked: %+v", clashes)
	}
	ours.Sheets[0].Delete("a2")
	if m, _ := Merge(base, ours, bare.Clone()); m.Node("a2") == nil {
		t.Error("a topic the other side changed was deleted")
	}
	gone := base.Clone()
	gone.Node("c").Changed = t0
	if _, err := ours.Sheets[0].Delete("c"); err != nil {
		t.Fatal(err)
	}
	theirsBare := gone.Clone()
	theirsBare.Walk(func(n *Node) { n.Changed = time.Time{} })
	if m, _ := Merge(gone, ours, theirsBare); m.Node("c") != nil {
		t.Error("a topic deleted here came back for the time the other side has not")
	}
	if !m.Node("a1").Changed.Equal(t1) || !m.Dated.Equal(t0) || m.Node("a2").Note != "快" {
		t.Errorf("times lost to a side with none: %v %v", m.Node("a1").Changed, m.Dated)
	}
}
