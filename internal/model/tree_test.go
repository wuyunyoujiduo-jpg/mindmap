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
