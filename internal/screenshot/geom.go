package screenshot

import (
	"math"

	"github.com/egoist/mygo/ui"
)

// 坐标都是窗口内的 DIP，原点在左上角。

type pt struct{ X, Y float32 }

func rectFrom(a, b pt) ui.Rect {
	x0, x1 := min(a.X, b.X), max(a.X, b.X)
	y0, y1 := min(a.Y, b.Y), max(a.Y, b.Y)
	return ui.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

func contains(r ui.Rect, p pt) bool {
	return p.X >= r.X && p.X <= r.X+r.W && p.Y >= r.Y && p.Y <= r.Y+r.H
}

func clampPt(p pt, r ui.Rect) pt {
	return pt{min(max(p.X, r.X), r.X+r.W), min(max(p.Y, r.Y), r.Y+r.H)}
}

// snapRect 把选区对齐到整数 DIP，导出时按整数尺寸渲染。
func snapRect(r ui.Rect) ui.Rect {
	x0, y0 := float32(math.Round(float64(r.X))), float32(math.Round(float64(r.Y)))
	x1, y1 := float32(math.Round(float64(r.X+r.W))), float32(math.Round(float64(r.Y+r.H)))
	return ui.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

// edge 是选区的一个控制点：X、Y 各取 -1（左/上）、0（中）、1（右/下）。
type edge struct{ X, Y int }

var handles = []edge{{-1, -1}, {0, -1}, {1, -1}, {-1, 0}, {1, 0}, {-1, 1}, {0, 1}, {1, 1}}

func handlePoint(r ui.Rect, e edge) pt {
	return pt{r.X + r.W*float32(e.X+1)/2, r.Y + r.H*float32(e.Y+1)/2}
}

const handleHit = 6

// hitHandle 返回 p 落在哪个控制点上。
func hitHandle(r ui.Rect, p pt) (edge, bool) {
	for _, e := range handles {
		h := handlePoint(r, e)
		if abs(h.X-p.X) <= handleHit && abs(h.Y-p.Y) <= handleHit {
			return e, true
		}
	}
	return edge{}, false
}

// resizeRect 拖动控制点 e 移动 (dx, dy)，拖过对边时自动翻转，结果限制在 bounds 内。
func resizeRect(orig ui.Rect, e edge, dx, dy float32, bounds ui.Rect) ui.Rect {
	x0, y0, x1, y1 := orig.X, orig.Y, orig.X+orig.W, orig.Y+orig.H
	switch e.X {
	case -1:
		x0 += dx
	case 1:
		x1 += dx
	}
	switch e.Y {
	case -1:
		y0 += dy
	case 1:
		y1 += dy
	}
	return rectFrom(clampPt(pt{x0, y0}, bounds), clampPt(pt{x1, y1}, bounds))
}

// moveRect 平移选区，不超出 bounds。
func moveRect(orig ui.Rect, dx, dy float32, bounds ui.Rect) ui.Rect {
	r := orig
	r.X = min(max(orig.X+dx, bounds.X), bounds.X+bounds.W-orig.W)
	r.Y = min(max(orig.Y+dy, bounds.Y), bounds.Y+bounds.H-orig.H)
	return r
}

const toolbarGap = 8

// toolbarOrigin 把工具条放在选区右下方；下面放不下放上面，都放不下放选区里面。
func toolbarOrigin(sel ui.Rect, w, h float32, bounds ui.Rect) pt {
	x := sel.X + sel.W - w
	y := sel.Y + sel.H + toolbarGap
	if y+h > bounds.Y+bounds.H {
		y = sel.Y - toolbarGap - h
		if y < bounds.Y {
			y = sel.Y + sel.H - toolbarGap - h
		}
	}
	x = min(max(x, bounds.X+4), bounds.X+bounds.W-w-4)
	return pt{x, y}
}

func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
