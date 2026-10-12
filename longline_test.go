package main

import (
	"fmt"

	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// A line thousands of points long, as the bracket around a root with its
// branches all open, shows: MyGo leaves out such a path drawn at once.
func TestLongLines(t *testing.T) {
	red := ui.RGB(255, 0, 0)
	view := ui.Rect{W: 400, H: 300}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Size(400, 300).Background(ui.RGB(0, 0, 0)).Draw(func(p *ui.Painter, r ui.Rect) {
			// Through the view, from far above it to far below.
			p.StrokePath(new(ui.Path).MoveTo(100, -4000).LineTo(100, 4000), 4, red)
			strokeLong(p, view, polyline{{200, -4000}, {200, 4000}}, 4, red)
			l := polyline{{300, -4000}}
			l.cube(350, -4000, 250, 4000, 300, 4000)
			strokeLong(p, view, l, 4, red)
		})
	}, 400, 300)
	tt.Frame()
	img := tt.Image()
	at := func(x, y int) bool {
		r, g, _, _ := img.At(x, y).RGBA()
		return r > 0xc000 && g < 0x4000
	}
	if at(100, 150) {
		t.Log("MyGo now draws long paths at once; strokeLong may not be needed")
	}
	if !at(200, 150) || !at(200, 10) || !at(200, 290) {
		t.Error("a long straight line is not drawn")
	}
	found := false
	for x := 250; x < 350; x++ {
		found = found || at(x, 150)
	}
	if !found {
		t.Error("a long curve is not drawn")
	}
}

// The lines stay on the canvas: one running off its top doesn't cross the
// toolbar.
func TestLinesStayInCanvas(t *testing.T) {
	a, tt := newTestApp(t)
	root := a.doc.Root
	root.Children = nil
	for i := 0; i < 11; i++ {
		b := model.NewNode(fmt.Sprintf("分支 %d", i))
		for j := 0; j < 14; j++ {
			b.Children = append(b.Children, model.NewNode(fmt.Sprintf("子节点 %d-%d", i, j)))
		}
		root.Children = append(root.Children, b)
	}
	a.doc.Structure = model.StructureBracket
	tt.Frame()
	b := a.boxes.Boxes[root.ID]
	cam := a.cam()
	cam.X, cam.Y = 60-b.X, 350-b.Y
	tt.Frame()
	tt.Frame()
	img := tt.Image()
	col := lookOf(a.doc.Style).branch(0)
	near := func(x, y int) bool {
		px := img.RGBAAt(x, y)
		d := func(p, q uint8) bool { return max(p, q)-min(p, q) < 32 }
		return d(px.R, col.R) && d(px.G, col.G) && d(px.B, col.B)
	}
	vp := a.vp
	onCanvas, above := false, false
	for x := int(vp.X); x < int(vp.X+vp.W); x++ {
		onCanvas = onCanvas || near(x, int(vp.Y+vp.H/2))
		above = above || near(x, int(vp.Y)-6)
	}
	if !onCanvas {
		t.Fatal("the root's bracket is not drawn")
	}
	if above {
		t.Error("a line is drawn over the toolbar")
	}
}

// The levels button over the canvas folds the map to a level, or all of it.
func TestLevelsMenu(t *testing.T) {
	a, tt := newTestApp(t)
	shown := func() int {
		n := 0
		a.doc.WalkShown(func(_, _ *model.Node, _ int) { n++ })
		return n
	}
	all := 0
	a.doc.Walk(func(_, _ *model.Node, _ int) bool { all++; return true })
	choose := func(item string) {
		t.Helper()
		tt.Frame()
		tt.Click("层级")
		if err := tt.ChooseMenuItem(item); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
	}
	choose("全部折叠（只看中心主题）")
	if shown() != 1 || a.selected != a.doc.Root.ID {
		t.Fatalf("folded all: %d shown", shown())
	}
	choose("展开到第 1 层")
	if shown() != 1+len(a.doc.Root.Children) {
		t.Fatalf("level 1: %d shown", shown())
	}
	choose("全部展开")
	if shown() != all {
		t.Fatalf("all: %d of %d shown", shown(), all)
	}
}
