package screenshot

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

var screen = ui.Rect{W: 1000, H: 800}

func TestRectFromAnyDirection(t *testing.T) {
	want := ui.Rect{X: 10, Y: 20, W: 90, H: 60}
	for _, pair := range [][2]pt{{{10, 20}, {100, 80}}, {{100, 80}, {10, 20}}, {{10, 80}, {100, 20}}} {
		if got := rectFrom(pair[0], pair[1]); got != want {
			t.Errorf("%v → %v", pair, got)
		}
	}
}

func TestHitHandle(t *testing.T) {
	r := ui.Rect{X: 100, Y: 100, W: 200, H: 100}
	if e, ok := hitHandle(r, pt{103, 98}); !ok || e != (edge{-1, -1}) {
		t.Errorf("左上角 %v %v", e, ok)
	}
	if e, ok := hitHandle(r, pt{200, 200}); !ok || e != (edge{0, 1}) {
		t.Errorf("下边中点 %v %v", e, ok)
	}
	if _, ok := hitHandle(r, pt{150, 150}); ok {
		t.Error("选区中间不是控制点")
	}
}

func TestResizeRect(t *testing.T) {
	r := ui.Rect{X: 100, Y: 100, W: 200, H: 100}
	if got := resizeRect(r, edge{1, 1}, 50, 20, screen); got != (ui.Rect{X: 100, Y: 100, W: 250, H: 120}) {
		t.Errorf("右下角 %v", got)
	}
	if got := resizeRect(r, edge{0, -1}, 30, -40, screen); got != (ui.Rect{X: 100, Y: 60, W: 200, H: 140}) {
		t.Errorf("上边只改高度 %v", got)
	}
	// 左边拖过右边：翻转
	if got := resizeRect(r, edge{-1, 0}, 300, 0, screen); got != (ui.Rect{X: 300, Y: 100, W: 100, H: 100}) {
		t.Errorf("翻转 %v", got)
	}
	// 不超出屏幕
	if got := resizeRect(r, edge{1, 1}, 5000, 5000, screen); got != (ui.Rect{X: 100, Y: 100, W: 900, H: 700}) {
		t.Errorf("越界 %v", got)
	}
}

func TestMoveRectStaysOnScreen(t *testing.T) {
	r := ui.Rect{X: 100, Y: 100, W: 200, H: 100}
	if got := moveRect(r, -500, 10, screen); got != (ui.Rect{X: 0, Y: 110, W: 200, H: 100}) {
		t.Errorf("左越界 %v", got)
	}
	if got := moveRect(r, 900, 900, screen); got != (ui.Rect{X: 800, Y: 700, W: 200, H: 100}) {
		t.Errorf("右下越界 %v", got)
	}
}

func TestToolbarOrigin(t *testing.T) {
	// 下方放得下：右对齐放在下面
	if got := toolbarOrigin(ui.Rect{X: 300, Y: 100, W: 400, H: 200}, 300, 36, screen); got != (pt{400, 308}) {
		t.Errorf("下方 %v", got)
	}
	// 贴底：放上面
	if got := toolbarOrigin(ui.Rect{X: 300, Y: 500, W: 400, H: 290}, 300, 36, screen); got != (pt{400, 456}) {
		t.Errorf("上方 %v", got)
	}
	// 全屏选区：放里面
	if got := toolbarOrigin(screen, 300, 36, screen); got != (pt{696, 756}) {
		t.Errorf("里面 %v", got)
	}
	// 选区太靠左：不出左边界
	if got := toolbarOrigin(ui.Rect{X: 0, Y: 0, W: 50, H: 50}, 300, 36, screen); got.X != 4 {
		t.Errorf("左边界 %v", got)
	}
}

func TestSnapRect(t *testing.T) {
	if got := snapRect(ui.Rect{X: 10.4, Y: 20.6, W: 99.3, H: 50.2}); got != (ui.Rect{X: 10, Y: 21, W: 100, H: 50}) {
		t.Errorf("%v", got)
	}
}
