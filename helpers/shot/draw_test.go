package main

import (
	"image"
	"image/color"
	"testing"

	"github.com/egoist/mygo/ui"
)

// checker 是一张 2 倍屏截图：每个像素的颜色由坐标决定，便于核对裁剪位置。
func checker(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 0x80, 0xFF})
		}
	}
	return img
}

func TestRenderSelectionCropsAtNativePixels(t *testing.T) {
	shot := checker(200, 160) // 100x80 DIP，2 倍
	bmp := ui.NewBitmap(shot)
	sel := ui.Rect{X: 10, Y: 20, W: 30, H: 15}
	img := renderSelection(bmp, pt{100, 80}, sel, nil, 2)
	if img.Bounds().Dx() != 60 || img.Bounds().Dy() != 30 {
		t.Fatalf("尺寸 %v，期望 60x30", img.Bounds())
	}
	for _, q := range []image.Point{{0, 0}, {59, 0}, {0, 29}, {33, 17}} {
		got := img.RGBAAt(q.X, q.Y)
		want := shot.RGBAAt(20+q.X, 40+q.Y)
		if diff(got.R, want.R) > 2 || diff(got.G, want.G) > 2 || diff(got.B, want.B) > 2 {
			t.Errorf("像素 %v = %v，期望 %v", q, got, want)
		}
	}
}

func TestRenderSelectionIncludesMarks(t *testing.T) {
	shot := checker(200, 160)
	sel := ui.Rect{X: 10, Y: 20, W: 60, H: 40}
	marks := []mark{{tool: toolRect, pts: []pt{{20, 30}, {50, 50}}}}
	img := renderSelection(ui.NewBitmap(shot), pt{100, 80}, sel, marks, 2)
	// 矩形左边框在 DIP x=20，即选区内 x=10，2 倍后约第 20 列
	got := img.RGBAAt(20, 30)
	if !isMarkColor(got) {
		t.Errorf("矩形边框处 %v，期望红色", got)
	}
	if isMarkColor(img.RGBAAt(60, 40)) {
		t.Error("矩形内部不应被填充")
	}
}

func TestRenderSelectionText(t *testing.T) {
	shot := image.NewRGBA(image.Rect(0, 0, 200, 100))
	for i := range shot.Pix {
		shot.Pix[i] = 0xFF
	}
	sel := ui.Rect{W: 200, H: 100}
	marks := []mark{{tool: toolText, pts: []pt{{10, 10}}, text: "Hello 你好"}}
	img := renderSelection(ui.NewBitmap(shot), pt{200, 100}, sel, marks, 1)
	red := 0
	for y := range 100 {
		for x := range 200 {
			if isMarkColor(img.RGBAAt(x, y)) {
				red++
			}
		}
	}
	if red < 30 {
		t.Errorf("文字几乎没画出来：%d 个红色像素", red)
	}
}

func isMarkColor(c color.RGBA) bool { return c.R > 200 && c.G < 120 && c.B < 120 }

func diff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

func TestSizeText(t *testing.T) {
	if got := sizeText(ui.Rect{W: 743.5, H: 415.5}, 2); got != "1487 x 831" {
		t.Errorf("%q", got)
	}
}
