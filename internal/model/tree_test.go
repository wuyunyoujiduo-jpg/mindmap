package model

import "testing"

func TestTreeHelpers(t *testing.T) {
	d := New("根")
	a, _ := d.AddChild(d.Root.ID, "A")
	b, _ := d.AddChild(a.ID, "B")
	b.Task = &Task{Status: StatusDone}
	c, _ := d.AddChild(a.ID, "C")
	c.Task = &Task{Status: StatusTodo}
	e, _ := d.AddChild(c.ID, "E")
	e.Task = &Task{Status: StatusDone}
	if done, total := a.Progress(); done != 2 || total != 3 {
		t.Fatalf("progress %d/%d", done, total)
	}
	cp := a.Copy()
	if cp.ID == a.ID || cp.Children[0].ID == b.ID || cp.Children[0].Title != "B" {
		t.Fatal("copy")
	}
	cp.Children[0].Title = "X"
	if b.Title != "B" {
		t.Fatal("copy shares nodes")
	}
	d.ExpandTo(1)
	if !a.Collapsed || !c.Collapsed || b.Collapsed { // leaves stay open
		t.Fatalf("expand to 1: %v %v", a.Collapsed, c.Collapsed)
	}
	d.ExpandTo(0)
	if a.Collapsed {
		t.Fatal("expand all")
	}
	if !d.Ancestors(a.ID, e.ID) || d.Ancestors(b.ID, e.ID) {
		t.Fatal("ancestors")
	}
}

func TestParseOutline(t *testing.T) {
	nodes := ParseOutline("# 标题\n- 一\n  - [x] 一点一\n  - [ ] 一点二\n- 二\n1. 三\n\n纯文本行")
	if len(nodes) != 1 || nodes[0].Title != "标题" {
		t.Fatalf("roots %v", len(nodes))
	}
	r := nodes[0]
	var titles []string
	for _, c := range r.Children {
		titles = append(titles, c.Title)
	}
	if len(titles) != 4 || titles[0] != "一" || titles[2] != "三" || titles[3] != "纯文本行" {
		t.Fatalf("children %q", titles)
	}
	one := r.Children[0]
	if len(one.Children) != 2 || one.Children[0].Task.Status != StatusDone || one.Children[1].Task.Status != StatusTodo {
		t.Fatalf("one %+v", one.Children)
	}
	plain := ParseOutline("甲\n乙\n  丙")
	if len(plain) != 2 || len(plain[1].Children) != 1 {
		t.Fatalf("plain %d", len(plain))
	}
}

func TestFloating(t *testing.T) {
	d := New("根")
	a, _ := d.AddChild(d.Root.ID, "A")
	b, _ := d.AddChild(a.ID, "B")
	f := d.AddFloating("浮", Point{X: 10, Y: 20})
	c, err := d.AddChild(f.ID, "浮下")
	if err != nil {
		t.Fatal(err)
	}
	if n, p := d.Find(c.ID); n != c || p != f {
		t.Fatal("find in a floating topic")
	}
	if path := d.Path(c.ID); len(path) != 2 || path[0] != f {
		t.Fatalf("path %v", path)
	}
	d.Relations = []Relation{{ID: "r", From: b.ID, To: c.ID}}
	cl := d.Clone()
	if len(cl.Floating) != 1 || cl.Floating[0] == f || cl.Floating[0].Pos.X != 10 || len(cl.Relations) != 1 {
		t.Fatal("clone")
	}
	// Into the tree and out again.
	if err := d.Move(f.ID, a.ID, -1); err != nil || len(d.Floating) != 0 || f.Pos != nil || a.Children[1] != f {
		t.Fatalf("move in: %v", err)
	}
	if err := d.Float(b.ID, Point{X: 5}); err != nil || len(d.Floating) != 1 || d.Floating[0] != b || b.Pos == nil {
		t.Fatalf("float: %v", err)
	}
	// Deleting a floating topic drops its relations.
	if _, err := d.Delete(b.ID); err != nil || len(d.Floating) != 0 || len(d.Relations) != 0 {
		t.Fatalf("delete: %v %v", err, d.Relations)
	}
	// WalkShown skips what is folded.
	a.Collapsed = true
	var seen []string
	d.WalkShown(func(n, _ *Node, _ int) { seen = append(seen, n.Title) })
	if len(seen) != 2 {
		t.Fatalf("shown %v", seen)
	}
}

// Links at a line's end become the node's links.
func TestParseOutlineLinks(t *testing.T) {
	ns := ParseOutline("- 解析器 <clip:ab12> <mindmap:项目/ergora#n1>\n- 用 List<String> 存\n- 文档 <https://go.dev>")
	if len(ns) != 3 || ns[0].Title != "解析器" || len(ns[0].Links) != 2 || ns[0].Links[1] != "mindmap:项目/ergora#n1" {
		t.Fatalf("%+v", ns[0])
	}
	if ns[1].Title != "用 List<String> 存" || ns[1].Links != nil {
		t.Fatalf("%+v", ns[1])
	}
	if ns[2].Title != "文档" || ns[2].Links[0] != "https://go.dev" {
		t.Fatalf("%+v", ns[2])
	}
}
