package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
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
