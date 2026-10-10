package layout

import (
	"testing"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

func fixed(*model.Node, int) (float32, float32) { return 100, 30 }

func TestCompute(t *testing.T) {
	d := model.New("root")
	a, _ := d.AddChild(d.Root.ID, "a")
	b, _ := d.AddChild(d.Root.ID, "b")
	a1, _ := d.AddChild(a.ID, "a1")
	a2, _ := d.AddChild(a.ID, "a2")
	o := Options{Margin: 10, GapX: 20, GapY: 10}
	r := Compute(d, fixed, o)

	if len(r.Boxes) != 5 || len(r.Edges) != 4 {
		t.Fatalf("%d boxes, %d edges", len(r.Boxes), len(r.Edges))
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
