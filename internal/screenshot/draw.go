package screenshot

import (
	"bytes"
	"image"
	"image/png"
	"math"
	"strconv"

	"github.com/egoist/mygo/ui"
)

var (
	markColor = ui.RGB(0xF5, 0x3B, 0x3B)
	// markColors are the colors to mark in: red, yellow, green, blue.
	markColors  = []ui.Color{markColor, ui.RGB(0xFF, 0xC1, 0x07), ui.RGB(0x22, 0xC5, 0x5E), ui.RGB(0x1E, 0x6F, 0xFF)}
	selColor    = ui.RGB(0x2E, 0xC4, 0x6A)
	maskColor   = ui.RGBA(0, 0, 0, 0.4)
	labelBg     = ui.RGBA(0, 0, 0, 0.75)
	markWidth   = float32(3)
	textSize    = float32(18)
	textWeight  = 700
	arrowHead   = float32(16)
	arrowSpread = math.Pi / 7
)

// drawShot 画截图和标注，off 是整体平移量（导出时把选区移到原点）。
func drawShot(p *ui.Painter, bmp *ui.Bitmap, size pt, marks []mark, off pt) {
	p.Image(bmp, ui.Rect{X: off.X, Y: off.Y, W: size.X, H: size.Y}, ui.FillBox)
	for _, m := range marks {
		drawMark(p, m, off)
	}
}

// colorOf is a mark's color.
func colorOf(c ui.Color) ui.Color {
	if c == (ui.Color{}) {
		return markColor
	}
	return c
}

func drawMark(p *ui.Painter, m mark, off pt) {
	at := func(q pt) pt { return pt{q.X + off.X, q.Y + off.Y} }
	col := colorOf(m.col)
	switch m.tool {
	case toolRect:
		r := rectFrom(at(m.first()), at(m.last()))
		p.Stroke(r, col, 0, markWidth)
	case toolArrow:
		drawArrow(p, at(m.first()), at(m.last()), col)
	case toolPen:
		var path ui.Path
		for i, q := range m.pts {
			q = at(q)
			if i == 0 {
				path.MoveTo(q.X, q.Y)
			} else {
				path.LineTo(q.X, q.Y)
			}
		}
		p.StrokePath(&path, markWidth, col)
	case toolText:
		q := at(m.first())
		p.RichText(q.X, q.Y, 0, textSpan(m.text, col))
	}
}

func textSpan(s string, col ui.Color) ui.Span {
	return ui.Span{Text: s, Size: textSize, Weight: textWeight, Color: col}
}

func drawArrow(p *ui.Painter, a, b pt, col ui.Color) {
	angle := math.Atan2(float64(b.Y-a.Y), float64(b.X-a.X))
	back := func(length float32, da float64) pt {
		return pt{b.X - length*float32(math.Cos(angle+da)), b.Y - length*float32(math.Sin(angle+da))}
	}
	var line ui.Path
	base := back(arrowHead*0.8, 0)
	line.MoveTo(a.X, a.Y).LineTo(base.X, base.Y)
	p.StrokePath(&line, markWidth, col)
	l, r := back(arrowHead, -arrowSpread), back(arrowHead, arrowSpread)
	var head ui.Path
	head.MoveTo(b.X, b.Y).LineTo(l.X, l.Y).LineTo(r.X, r.Y).Close()
	p.FillPath(&head, col)
}

// drawChrome 画选区外的遮罩、选区边框、控制点和尺寸。
func drawChrome(p *ui.Painter, e *editor, scale float32) {
	b := e.bounds
	if !e.hasSel && e.drag != dragCreate {
		p.Fill(b, maskColor, 0)
		return
	}
	s := e.sel
	p.Fill(ui.Rect{X: b.X, Y: b.Y, W: b.W, H: s.Y - b.Y}, maskColor, 0)
	p.Fill(ui.Rect{X: b.X, Y: s.Y + s.H, W: b.W, H: b.Y + b.H - s.Y - s.H}, maskColor, 0)
	p.Fill(ui.Rect{X: b.X, Y: s.Y, W: s.X - b.X, H: s.H}, maskColor, 0)
	p.Fill(ui.Rect{X: s.X + s.W, Y: s.Y, W: b.X + b.W - s.X - s.W, H: s.H}, maskColor, 0)
	p.Stroke(ui.Rect{X: s.X - 1, Y: s.Y - 1, W: s.W + 2, H: s.H + 2}, selColor, 0, 1)
	if e.tool == toolNone && e.drag != dragCreate {
		for _, h := range handles {
			c := handlePoint(s, h)
			p.Fill(ui.Rect{X: c.X - 3, Y: c.Y - 3, W: 6, H: 6}, selColor, 3)
		}
	}
	drawSizeLabel(p, s, scale, b)
}

func drawSizeLabel(p *ui.Painter, s ui.Rect, scale float32, b ui.Rect) {
	label := ui.Span{Text: sizeText(s, scale), Size: 12, Weight: 500, Color: ui.RGB(255, 255, 255)}
	w, h := p.MeasureText(0, label)
	box := ui.Rect{X: s.X, Y: s.Y - h - 10, W: w + 12, H: h + 6}
	if box.Y < b.Y {
		box.X, box.Y = s.X+4, s.Y+4
	}
	p.Fill(box, labelBg, 4)
	p.RichText(box.X+6, box.Y+3, 0, label)
}

// sizeText 是选区的像素尺寸，如 "1487 x 831"。
func sizeText(s ui.Rect, scale float32) string {
	w := int(math.Round(float64(s.W * scale)))
	h := int(math.Round(float64(s.H * scale)))
	return strconv.Itoa(w) + " x " + strconv.Itoa(h)
}

// renderSelection 把选区连同标注按屏幕原始像素渲染成图片，所见即所得。
func renderSelection(bmp *ui.Bitmap, size pt, sel ui.Rect, marks []mark, scale float32) *image.RGBA {
	view := func(c *ui.Context) {
		ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
			drawShot(p, bmp, size, marks, pt{-sel.X, -sel.Y})
		})
	}
	return ui.Render(view, int(sel.W), int(sel.H), scale)
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
