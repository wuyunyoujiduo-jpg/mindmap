package screenshot

import (
	"image"

	"github.com/egoist/mygo/ui"
)

// Annotator marks up a screenshot already taken, in an ordinary window:
// macOS's own tool frames the region (screencapture -i), and this is
// OneShot's markup of it, with OneShot's tools, drawing and export, but
// none of its windows over the screen.
type Annotator struct {
	bmp     *ui.Bitmap
	pw, ph  int // the shot's size in pixels
	ed      *editor
	textKey int
	size    pt // the shot as shown, in points
	done    bool
	// Scale is the display's pixels per point: a shot of a Retina display
	// shows at half its pixels, as it was on the screen.
	Scale float32
	// Deliver gets the shot, marked up, in its own pixels, and what to do
	// with it; Cancel is told when it is let go. Both on the main thread.
	Deliver func(png []byte, act Action)
	Cancel  func()
}

// NewAnnotator makes the markup of img, rect the first tool, as the shot
// was framed to be marked up.
func NewAnnotator(img image.Image) *Annotator {
	b := img.Bounds()
	return &Annotator{bmp: ui.NewBitmap(img), pw: b.Dx(), ph: b.Dy(), ed: &editor{tool: toolRect}, Scale: 1}
}

// Size is a window's size that shows the shot whole, at most maxW×maxH,
// with room for the toolbar.
func (a *Annotator) Size(maxW, maxH float32) (float32, float32) {
	w, h := float32(a.pw)/a.scale(), float32(a.ph)/a.scale()
	k := min(1, (maxW-32)/w, (maxH-annotateBar-32)/h)
	return max(w*k+32, 520), max(h*k+annotateBar+32, 260)
}

const annotateBar = 52

func (a *Annotator) scale() float32 { return max(a.Scale, 1) }

// fit places the shot in the room left by the toolbar.
func (a *Annotator) fit(w, h, scale float32) ui.Rect {
	iw, ih := float32(a.pw)/scale, float32(a.ph)/scale
	k := min(1, (w-32)/iw, (h-32)/ih)
	iw, ih = iw*k, ih*k
	return ui.Rect{X: (w - iw) / 2, Y: (h - ih) / 2, W: iw, H: ih}
}

// View is the window's content: the toolbar, and the shot to draw on.
func (a *Annotator) View(c *ui.Context) {
	t := c.Theme()
	e := a.ed
	if c.Shortcut(0, ui.KeyEscape) {
		if e.editing {
			e.commitText()
		} else {
			a.cancel()
		}
	}
	if !e.editing {
		if c.Shortcut(ui.Cmd, ui.KeyZ) {
			e.undo()
		}
		if c.Shortcut(ui.Cmd, ui.KeyC) {
			a.finish(Copy)
		}
		if c.Shortcut(ui.Cmd, ui.KeyS) {
			a.finish(Save)
		}
		if c.Shortcut(0, ui.KeyEnter) {
			a.finish(Library)
		}
	} else if c.Shortcut(0, ui.KeyEnter) {
		e.commitText()
	}
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		a.toolbar(c)
		area := ui.Box(c.Key("shot-area")).Grow(1).FillWidth().Background(t.Text.Alpha(0.04))
		area.Children(func() {
			w, h := c.Size()
			h -= annotateBar
			if w <= 0 || h <= 0 {
				return
			}
			r := a.fit(w, h, a.scale())
			a.size = pt{r.W, r.H}
			e.bounds = ui.Rect{W: r.W, H: r.H}
			e.sel, e.hasSel = e.bounds, true
			cv := ui.Box(c.Key("shot-canvas")).Absolute().Left(r.X).Top(r.Y).Size(r.W, r.H).
				Shadow(0, 2, 12, 0, ui.RGBA(0, 0, 0, 0.25)).Cursor(a.cursor()).Label("截图画布")
			cv.HandleInput(func(ev ui.InputEvent) bool { return a.input(ev) })
			cv.Draw(func(p *ui.Painter, rr ui.Rect) {
				drawShot(p, a.bmp, pt{rr.W, rr.H}, e.visibleMarks(), pt{rr.X, rr.Y})
			})
			cv.Children(func() {
				if e.editing {
					in := ui.TextInputBase(c.Key(a.textKey), &e.editText).
						Absolute().Left(e.editAt.X).Top(e.editAt.Y).Width(max(80, r.W-e.editAt.X)).
						FontSize(textSize).FontWeight(textWeight).TextColor(colorOf(e.color)).
						SingleLine().AutoFocus().Label("标注文字")
					if in.Submitted() {
						e.commitText()
					}
				}
			})
		})
	})
}

func (a *Annotator) cursor() ui.Cursor {
	if a.ed.tool == toolText {
		return ui.CursorText
	}
	return ui.CursorCrosshair
}

// input draws the marks: the pointer pressed starts one, moved shapes it,
// let go keeps it. A click with the text tool writes there.
func (a *Annotator) input(ev ui.InputEvent) bool {
	e := a.ed
	p := pt{ev.X, ev.Y}
	switch ev.Kind {
	case ui.InputPointerDown:
		if ev.Button != 0 {
			return false
		}
		e.commitText()
		if !contains(e.sel, p) {
			return true
		}
		if e.tool == toolText {
			e.editing, e.editAt, e.editText = true, pt{p.X, p.Y - textLineHeight/2}, ""
			a.textKey++
			return true
		}
		e.cur = &mark{tool: e.tool, pts: []pt{p, p}}
		e.drag = dragDraw
		return true
	case ui.InputPointerMove:
		if e.drag == dragDraw {
			e.move(p)
		}
		return true
	case ui.InputPointerUp:
		if e.drag != dragDraw {
			return false
		}
		e.up(p)
		return true
	}
	return false
}

func (a *Annotator) toolbar(c *ui.Context) {
	t := c.Theme()
	e := a.ed
	// Clear of the window's buttons, when the bar is its title bar.
	left := max(14, c.TitleBar().Left+8)
	ui.Row(c.Key("shot-bar")).Height(annotateBar).Padding(0, 14, 0, left).Gap(4).AlignItems(ui.Center).FillWidth().
		BorderWidth(0, 0, 1, 0).BorderColor(t.Border).DragWindow().Children(func() {
		for _, tl := range []struct {
			icon *ui.SVG
			tip  string
			tool tool
		}{
			{iconRect, "矩形", toolRect},
			{iconArrow, "箭头", toolArrow},
			{iconPen, "画笔", toolPen},
			{iconText, "文字", toolText},
		} {
			btn := a.button(c, tl.icon, tl.tip, e.tool == tl.tool)
			if btn.Clicked() {
				e.commitText()
				e.tool = tl.tool // a tool stays chosen: there is no frame to move here
			}
		}
		ui.Box(c).Size(1, 20).MarginX(6).Background(t.Border)
		if a.button(c, iconUndo, "撤销 ⌘Z", false).Clicked() {
			e.undo()
		}
		ui.Spacer(c)
		if ui.Button(c, "取消").Label("取消截图").Clicked() {
			a.cancel()
		}
		if ui.Button(c, "保存…").Label("保存截图").Tooltip("存成 PNG 文件（⌘S）").Clicked() {
			a.finish(Save)
		}
		if ui.Button(c, "复制").Label("复制截图").Tooltip("复制到剪贴板（⌘C）").Clicked() {
			a.finish(Copy)
		}
		if ui.PrimaryButton(c, "收进资料库").Label("收进资料库").Tooltip("回车").Clicked() {
			a.finish(Library)
		}
	})
}

func (a *Annotator) button(c *ui.Context, icon *ui.SVG, tip string, on bool) ui.Element {
	t := c.Theme()
	btn := ui.ButtonBase(c.Key(tip)).Size(32, 30).Radius(6).Label(tip).Tooltip(tip)
	col := t.Text
	switch {
	case on:
		btn.Background(t.Accent.Alpha(0.14))
		col = t.Accent
	case btn.Hovered():
		btn.Background(t.Text.Alpha(0.06))
	}
	btn.TextColor(col).Children(func() { ui.Icon(c, icon).Size(18, 18) })
	return btn
}

// Export is the shot with its marks, in its own pixels.
func (a *Annotator) Export() ([]byte, error) {
	a.ed.commitText()
	scale := float32(a.pw) / a.size.X
	if a.size.X <= 0 {
		scale, a.size = 1, pt{float32(a.pw), float32(a.ph)}
	}
	sel := ui.Rect{W: a.size.X, H: a.size.Y}
	return encodePNG(renderSelection(a.bmp, a.size, sel, a.ed.marks, scale))
}

func (a *Annotator) finish(act Action) {
	if a.done {
		return
	}
	png, err := a.Export()
	if err != nil {
		Log("导出截图失败：%v", err)
		return
	}
	a.done = true
	if a.Deliver != nil {
		a.Deliver(png, act)
	}
}

func (a *Annotator) cancel() {
	if a.done {
		return
	}
	a.done = true
	if a.Cancel != nil {
		a.Cancel()
	}
}
