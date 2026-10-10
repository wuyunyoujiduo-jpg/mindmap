package layout

import (
	"testing"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

func fixed(*model.Node, int) (float32, float32) { return 100, 30 }

func TestOneSided(t *testing.T) {
	d := model.New("root")
	a, _ := d.AddChild(d.Root.ID, "a")
	b, _ := d.AddChild(d.Root.ID, "b")
	a1, _ := d.AddChild(a.ID, "a1")
	a2, _ := d.AddChild(a.ID, "a2")
	o := Options{Margin: 10, GapX: 20, GapY: 10, OneSided: true}
	r := Compute(d, fixed, o)

	if len(r.Boxes) != 5 || len(r.Edges) != 4 || len(r.Left) != 0 {
		t.Fatalf("%d boxes, %d edges, %d left", len(r.Boxes), len(r.Edges), len(r.Left))
	}
	// Columns by depth.
	if r.Boxes[d.Root.ID].X != 10 || r.Boxes[a.ID].X != 130 || r.Boxes[a1.ID].X != 250 {
		t.Fatalf("x: %v %v %v", r.Boxes[d.Root.ID].X, r.Boxes[a.ID].X, r.Boxes[a1.ID].X)
	}
	// a's subtree is 70 high (two children and a gap), b's 30; total 110.
	if r.Height != 130 || r.Width != 360 {
		t.Fatalf("size %vx%v", r.Width, r.Height)
	}
	// Parents are centered on their children; the root on everything.
	ca := r.Boxes[a.ID].Y + 15
	if mid := (r.Boxes[a1.ID].Y + r.Boxes[a2.ID].Y + 30) / 2; ca != mid {
		t.Fatalf("a centered at %v, children at %v", ca, mid)
	}
	if r.Boxes[d.Root.ID].Y != 10+40 {
		t.Fatalf("root y %v", r.Boxes[d.Root.ID].Y)
	}
	if r.Boxes[b.ID].Y != 10+80 {
		t.Fatalf("b y %v", r.Boxes[b.ID].Y)
	}
	if got := r.Hit(r.Boxes[a2.ID].X+5, r.Boxes[a2.ID].Y+5); got != a2.ID {
		t.Fatalf("hit %q", got)
	}

	// Collapsing hides the children.
	a.Collapsed = true
	r = Compute(d, fixed, o)
	if _, ok := r.Boxes[a1.ID]; ok || len(r.Boxes) != 3 {
		t.Fatal("collapsed children laid out")
	}
}

func TestBalanced(t *testing.T) {
	d := model.New("root")
	var top []*model.Node
	for _, s := range []string{"a", "b", "c", "d"} {
		n, _ := d.AddChild(d.Root.ID, s)
		top = append(top, n)
	}
	l1, _ := d.AddChild(top[3].ID, "d1")
	o := Options{Margin: 10, GapX: 20, GapY: 10}
	r := Compute(d, fixed, o)

	root := r.Boxes[d.Root.ID]
	// a and b on the right, c and d on the left.
	for i, n := range top {
		left := i >= 2
		b := r.Boxes[n.ID]
		if r.Left[n.ID] != left {
			t.Fatalf("%s left %v", n.Title, r.Left[n.ID])
		}
		if left && b.X+b.W != root.X-20 || !left && b.X != root.X+root.W+20 {
			t.Fatalf("%s at x %v, root at %v", n.Title, b.X, root.X)
		}
	}
	// Further out on the left.
	if b := r.Boxes[l1.ID]; !r.Left[l1.ID] || b.X+b.W != r.Boxes[top[3].ID].X-20 {
		t.Fatalf("d1 at %v", b.X)
	}
	for _, e := range r.Edges {
		if e.Left != r.Left[e.To] {
			t.Fatalf("edge to %s left %v", e.To, e.Left)
		}
	}
	// Everything inside the margin, the root in the middle of its sides.
	for id, b := range r.Boxes {
		if b.X < 10 || b.Y < 10 || b.X+b.W > r.Width-10 || b.Y+b.H > r.Height-10 {
			t.Fatalf("%s outside: %+v in %vx%v", id, b, r.Width, r.Height)
		}
	}
	if root.X != 10+100+20+100+20 {
		t.Fatalf("root x %v", root.X)
	}
}

func TestSplit(t *testing.T) {
	var ns []*model.Node
	for range 5 {
		ns = append(ns, model.NewNode(""))
	}
	h := func(*model.Node) float32 { return 10 }
	right, left := split(ns, h, Options{})
	if len(right) != 3 || len(left) != 2 {
		t.Fatalf("%d right, %d left", len(right), len(left))
	}
	right, left = split(ns[:1], h, Options{})
	if len(right) != 1 || len(left) != 0 {
		t.Fatal("one branch not on the right")
	}
	right, left = split(ns[:2], h, Options{})
	if len(right) != 1 || len(left) != 1 {
		t.Fatalf("two branches: %d right", len(right))
	}
}
