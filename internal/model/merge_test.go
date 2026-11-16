package model

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// mergeBase is a map of two canvases to change apart.
func mergeBase() *File {
	f := &File{Version: FormatVersion}
	d := &Document{ID: "s1", Title: "画布 1", Root: &Node{ID: "r", Title: "AI", Children: []*Node{
		{ID: "a", Title: "模型", Children: []*Node{{ID: "a1", Title: "Opus"}, {ID: "a2", Title: "Sonnet"}}},
		{ID: "b", Title: "工具", Tags: []string{"x"}},
		{ID: "c", Title: "论文"},
	}}}
	d2 := &Document{ID: "s2", Title: "画布 2", Root: &Node{ID: "r2", Title: "其他"}}
	f.Sheets = []*Document{d, d2}
	return f
}

func TestMergeApartChanges(t *testing.T) {
	base := mergeBase()
	ours, theirs := base.Clone(), base.Clone()

	// Ours: renames a topic, adds one, tags b.
	ours.Node("a1").Title = "Opus 5"
	o, _ := ours.Sheets[0].AddChild("r", "提示词")
	ours.Node("b").Tags = append(ours.Node("b").Tags, "y")
	// Theirs: a note, a task, a child, deletes c, tags b otherwise.
	theirs.Node("a1").Note = "最强"
	due := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	theirs.Node("a2").Task = &Task{Status: StatusDoing, Due: &due}
	tn, _ := theirs.Sheets[0].AddChild("a", "Haiku")
	if _, err := theirs.Sheets[0].Delete("c"); err != nil {
		t.Fatal(err)
	}
	theirs.Node("b").Tags = append(theirs.Node("b").Tags, "z")

	m, clashes := Merge(base, ours, theirs)
	if len(clashes) != 0 {
		t.Errorf("clashes = %+v, want none", clashes)
	}
	if n := m.Node("a1"); n.Title != "Opus 5" || n.Note != "最强" {
		t.Errorf("a1 = %q / %q, want both sides' changes", n.Title, n.Note)
	}
	if n := m.Node("a2"); n.Task == nil || n.Task.Status != StatusDoing {
		t.Errorf("a2's task lost: %+v", n.Task)
	}
	if m.Node(o.ID) == nil || m.Node(tn.ID) == nil {
		t.Error("a topic added on one side is lost")
	}
	if m.Node("c") != nil {
		t.Error("c, deleted by theirs and left by ours, came back")
	}
	if got := m.Node("b").Tags; !slices.Equal(got, []string{"x", "y", "z"}) {
		t.Errorf("tags = %v, want both sides' added", got)
	}
	if got := titles(m.Sheets[0].Root.Children); !slices.Equal(got, []string{"模型", "工具", "提示词"}) {
		t.Errorf("root's children = %v", got)
	}
	if got := titles(m.Node("a").Children); !slices.Equal(got, []string{"Opus 5", "Sonnet", "Haiku"}) {
		t.Errorf("a's children = %v", got)
	}
	if len(m.Sheets) != 2 {
		t.Errorf("%d canvases, want 2", len(m.Sheets))
	}
	if _, err := m.Marshal(); err != nil {
		t.Fatal(err)
	}
	b, _ := m.Marshal()
	if _, err := Parse(b); err != nil {
		t.Fatalf("the merge does not read back: %v", err)
	}
}

func TestMergeClash(t *testing.T) {
	base := mergeBase()
	ours, theirs := base.Clone(), base.Clone()
	ours.Node("a1").Title = "ours"
	theirs.Node("a1").Title = "theirs"
	due := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	ours.Node("a2").Task = &Task{Status: StatusDone, Due: &due}
	theirs.Node("a2").Task = &Task{Status: StatusDoing, Due: &due}
	ours.Sheets[0].Title, theirs.Sheets[0].Title = "甲", "乙"
	m, clashes := Merge(base, ours, theirs)
	if len(clashes) != 3 || m.Node("a1").Title != "ours" {
		t.Fatalf("clashes %+v, title %q: want 3, ours", clashes, m.Node("a1").Title)
	}
	for _, c := range clashes {
		if !m.TakeTheirs(c) {
			t.Fatalf("could not take %+v", c)
		}
	}
	if m.Node("a1").Title != "theirs" || m.Node("a2").Task.Status != StatusDoing || m.Node("a2").Task.Due == nil ||
		m.Sheets[0].Title != "乙" || len(m.Node("a").Children) != 2 {
		t.Errorf("theirs not taken: %q %+v %q", m.Node("a1").Title, m.Node("a2").Task, m.Sheets[0].Title)
	}
	var fields []string
	for _, c := range clashes {
		fields = append(fields, strings.Join(c.Field, "."))
	}
	slices.Sort(fields)
	if !slices.Equal(fields, []string{"task.status", "title", "title"}) {
		t.Errorf("fields %v", fields)
	}
}

func TestMergeDeletedButChanged(t *testing.T) {
	base := mergeBase()
	ours, theirs := base.Clone(), base.Clone()
	// Ours deletes a, theirs renames a child of it: a and the child stay.
	if _, err := ours.Sheets[0].Delete("a"); err != nil {
		t.Fatal(err)
	}
	theirs.Node("a2").Title = "Sonnet 5"
	m, _ := Merge(base, ours, theirs)
	if m.Node("a") == nil || m.Node("a2") == nil || m.Node("a2").Title != "Sonnet 5" {
		t.Error("a topic changed on one side, under one deleted on the other, is lost")
	}
	if m.Node("a1") != nil {
		t.Error("a1, unchanged under a deleted topic, came back")
	}
}

func TestMergeMovesAndLoops(t *testing.T) {
	base := mergeBase()
	ours, theirs := base.Clone(), base.Clone()
	// Ours puts b under c, theirs c under b: a loop, undone as ours.
	if err := ours.Sheets[0].Move("b", "c", 0); err != nil {
		t.Fatal(err)
	}
	if err := theirs.Sheets[0].Move("c", "b", 0); err != nil {
		t.Fatal(err)
	}
	m, _ := Merge(base, ours, theirs)
	if _, p := m.Sheets[0].Find("b"); p == nil || p.ID != "c" {
		t.Errorf("b should be under c, as ours has it")
	}
	if _, p := m.Sheets[0].Find("c"); p == nil || p.ID != "r" {
		t.Errorf("c should be under the root")
	}
	count := 0
	m.Walk(func(*Node) { count++ })
	if count != 7 {
		t.Errorf("%d topics, want 7", count)
	}
}

func TestMergeCanvases(t *testing.T) {
	base := mergeBase()
	ours, theirs := base.Clone(), base.Clone()
	ours.AddSheet(1).Title = "新画布"
	if err := theirs.DeleteSheet(1); err != nil {
		t.Fatal(err)
	}
	theirs.Sheets[0].Title = "主画布"
	m, _ := Merge(base, ours, theirs)
	var got []string
	for _, d := range m.Sheets {
		got = append(got, d.Title)
	}
	if !slices.Equal(got, []string{"主画布", "新画布"}) {
		t.Errorf("canvases = %v", got)
	}
}

func TestMergeWithoutBase(t *testing.T) {
	ours, theirs := mergeBase(), mergeBase()
	if _, err := ours.Sheets[0].Delete("c"); err != nil {
		t.Fatal(err)
	}
	n, _ := theirs.Sheets[0].AddChild("r", "新的")
	m, _ := Merge(nil, ours, theirs)
	if m.Node("c") == nil || m.Node(n.ID) == nil {
		t.Error("with no base, what is in either is kept")
	}
}

func TestMergeLog(t *testing.T) {
	base := mergeBase()
	e := base.AddLog(LogEntry{At: time.Unix(1, 0), Text: "旧"})
	ours, theirs := base.Clone(), base.Clone()
	ours.Log, theirs.Log = slices.Clone(base.Log), slices.Clone(base.Log)
	ours.AddLog(LogEntry{At: time.Unix(3, 0), Text: "ours"})
	theirs.AddLog(LogEntry{At: time.Unix(2, 0), Text: "theirs"})
	theirs.DropLog(e.ID)
	m, _ := Merge(base, ours, theirs)
	var got []string
	for _, l := range m.Log {
		got = append(got, l.Text)
	}
	if !slices.Equal(got, []string{"theirs", "ours"}) {
		t.Errorf("log = %v", got)
	}
}
