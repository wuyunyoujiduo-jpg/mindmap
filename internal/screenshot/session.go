package screenshot

import (
	"log"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// screenShot 是一块屏幕：冻结的截图、它的选区窗口和编辑状态。
type screenShot struct {
	index   int
	display mygo.Display
	bmp     *ui.Bitmap
	scale   float32 // 截图像素 / DIP
	ed      *editor
	win     *mygo.Window
	hover   pt
	drawn   atomic.Bool // its window drew a frame: it shows
	textKey int         // 每次新开文字输入换一个 key，输入框状态不沿用
}

// session 是一次截图：每块屏幕一个全屏窗口，最先框选的那块屏幕独占编辑。
type session struct {
	shots []*screenShot
	owner int // 正在编辑的屏幕，-1 表示还没人框选
	ended bool
	onEnd func()
	// deliver 收下导出的图片和要做的事。截图窗口关掉后、在后台调用。
	deliver func(png []byte, act Action)
}

// captureScreens 截下所有屏幕。任意 goroutine 调用。
func captureScreens() ([]*screenShot, error) {
	displays := mygo.Screen.Displays()
	shots := make([]*screenShot, len(displays))
	errs := make([]error, len(displays))
	var wg sync.WaitGroup
	for i, d := range displays {
		wg.Go(func() {
			img, err := captureDisplay(d)
			if err != nil {
				errs[i] = err
				return
			}
			shots[i] = &screenShot{
				index:   i,
				display: d,
				bmp:     ui.NewBitmap(img),
				scale:   float32(img.Bounds().Dx()) / float32(d.Bounds.Width),
				ed:      &editor{},
			}
		})
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return shots, nil
}

// openSession 给每块屏幕铺一个选区窗口，并把焦点给鼠标所在的屏幕。任意 goroutine 调用。
func openSession(shots []*screenShot, deliver func([]byte, Action), onEnd func()) *session {
	s := &session{shots: shots, owner: -1, onEnd: onEnd, deliver: deliver}
	cursor := mygo.Screen.CursorScreenPoint()
	focus := shots[0]
	for _, sh := range shots {
		b := sh.display.Bounds
		sh.win = mygo.NewWindow(mygo.WindowOptions{
			Title:             "截图",
			X:                 b.X,
			Y:                 b.Y,
			Width:             b.Width,
			Height:            b.Height,
			Hidden:            true,
			Frameless:         true,
			DisableResize:     true,
			DisableMinimize:   true,
			DisableMaximize:   true,
			DisableFullScreen: true,
			DisableShadow:     true,
			AlwaysOnTop:       true,
			SkipTaskbar:       true,
			BackgroundColor:   "#000",
			Content:           ui.View(s.view(sh)),
		})
		if cursor.X >= b.X && cursor.X < b.X+b.Width && cursor.Y >= b.Y && cursor.Y < b.Y+b.Height {
			focus = sh
		}
	}
	for _, sh := range shots {
		w, b := sh.win, sh.display.Bounds
		w.Update(func() {
			raiseOverlay(w)
			w.SetBounds(b)
			w.ShowInactive()
		})
	}
	focus.win.Update(func() {
		focus.win.Focus()
		Log("截图窗口已显示在屏幕 %d", focus.index)
	})
	return s
}

func (s *session) view(sh *screenShot) func(c *ui.Context) {
	return func(c *ui.Context) {
		sh.drawn.Store(true)
		e := sh.ed
		w, h := c.Size()
		e.bounds = ui.Rect{W: w, H: h}
		s.shortcuts(c, sh)

		var avoid []ui.Rect // 落在控件上的按下交给控件
		showBar := e.hasSel && !e.dragging() && (s.owner == sh.index)
		var barAt pt
		if showBar {
			barAt = toolbarOrigin(e.sel, toolbarW, toolbarH, e.bounds)
			avoid = append(avoid, ui.Rect{X: barAt.X, Y: barAt.Y, W: toolbarW, H: toolbarH})
		}
		if e.editing {
			avoid = append(avoid, ui.Rect{X: e.editAt.X, Y: e.editAt.Y, W: textInputWidth(e), H: textLineHeight})
		}

		c.Root().Background(ui.RGB(0, 0, 0))
		root := ui.Box(c).Fill().Cursor(e.cursor(sh.hover))
		root.HandleInput(func(ev ui.InputEvent) bool { return s.input(sh, ev, avoid) })
		root.Draw(func(p *ui.Painter, r ui.Rect) {
			drawShot(p, sh.bmp, pt{r.W, r.H}, e.visibleMarks(), pt{})
			drawChrome(p, e, sh.scale)
		})
		root.Children(func() {
			if e.editing {
				s.textInput(c, sh)
			}
			if showBar {
				s.toolbar(c, sh, barAt)
			}
		})
	}
}

func (s *session) shortcuts(c *ui.Context, sh *screenShot) {
	e := sh.ed
	if c.Shortcut(0, ui.KeyEscape) {
		if e.editing {
			e.commitText()
		} else {
			s.end()
		}
	}
	if c.Shortcut(0, ui.KeyEnter) {
		if e.editing {
			e.commitText()
		} else if e.hasSel {
			s.keep(sh) // 回车收进资料库
		}
	}
	if e.editing {
		return // 输入文字时 ⌘Z、⌘C 等留给输入框
	}
	if c.Shortcut(ui.Cmd, ui.KeyZ) {
		e.undo()
	}
	if c.Shortcut(ui.Cmd, ui.KeyC) && e.hasSel {
		s.copy(sh)
	}
	if c.Shortcut(ui.Cmd, ui.KeyS) && e.hasSel {
		s.save(sh)
	}
}

func (s *session) input(sh *screenShot, ev ui.InputEvent, avoid []ui.Rect) bool {
	e := sh.ed
	p := pt{ev.X, ev.Y}
	switch ev.Kind {
	case ui.InputPointerMove:
		sh.hover = p
		e.move(p)
		return true
	case ui.InputPointerDown:
		for _, r := range avoid {
			if contains(r, p) {
				return false
			}
		}
		if ev.Button == 1 { // 右键取消
			s.end()
			return true
		}
		if ev.Button != 0 || (s.owner != -1 && s.owner != sh.index) {
			return true
		}
		if e.down(p, ev.Clicks) == resultCopy { // 双击选区：收进资料库
			s.keep(sh)
		}
		if e.editing {
			sh.textKey++
		}
		return true
	case ui.InputPointerUp:
		if e.drag == dragNone {
			return false
		}
		e.up(p)
		switch {
		case e.hasSel:
			s.owner = sh.index
		case s.owner == sh.index:
			s.owner = -1
		}
		return true
	}
	return false
}

func textInputWidth(e *editor) float32 {
	return max(60, e.sel.X+e.sel.W-e.editAt.X)
}

func (s *session) textInput(c *ui.Context, sh *screenShot) {
	e := sh.ed
	in := ui.TextInputBase(c.Key(sh.textKey), &e.editText).
		Absolute().Left(e.editAt.X).Top(e.editAt.Y).Width(textInputWidth(e)).
		FontSize(textSize).FontWeight(textWeight).TextColor(colorOf(e.color)).
		SingleLine().AutoFocus()
	if in.Submitted() {
		e.commitText()
	}
}

const (
	toolbarBtn = float32(30)
	toolbarH   = float32(36)
	toolbarPad = float32(4)
	toolbarSep = float32(9) // 分隔线 1 + 左右各 4
	toolbarDot = float32(22)
	// 4 个工具、分隔线、4 种颜色、分隔线、撤销和 4 个操作，子元素间隔 2。
	toolbarW = 9*toolbarBtn + 4*toolbarDot + 2*toolbarSep + 16*2 + 2*toolbarPad
)

var (
	toolbarBg   = ui.RGB(0xFA, 0xFA, 0xFA)
	iconGray    = ui.RGB(0x44, 0x44, 0x44)
	iconActive  = ui.RGB(0x1E, 0x6F, 0xFF)
	iconCancel  = ui.RGB(0xE5, 0x48, 0x4D)
	iconConfirm = ui.RGB(0x22, 0xA8, 0x4A)
)

func (s *session) toolbar(c *ui.Context, sh *screenShot, at pt) {
	e := sh.ed
	ui.Row(c).Absolute().Left(at.X).Top(at.Y).Size(toolbarW, toolbarH).Padding(toolbarPad).Gap(2).
		AlignItems(ui.Center).Background(toolbarBg).Radius(8).
		Shadow(0, 2, 12, 0, ui.RGBA(0, 0, 0, 0.3)).Cursor(ui.CursorDefault).
		Children(func() {
			for _, t := range []struct {
				icon *ui.SVG
				tip  string
				tool tool
			}{
				{iconRect, "矩形", toolRect},
				{iconArrow, "箭头", toolArrow},
				{iconPen, "画笔", toolPen},
				{iconText, "文字", toolText},
			} {
				color := iconGray
				btn := toolButton(c, t.icon, t.tip)
				if e.tool == t.tool {
					color = iconActive
					btn.Background(ui.RGBA(0x1E, 0x6F, 0xFF, 0.12))
				}
				btn.TextColor(color)
				if btn.Clicked() {
					e.setTool(t.tool)
				}
			}
			ui.Box(c).Size(1, 20).MarginX(4).Background(ui.RGBA(0, 0, 0, 0.15))
			for i, col := range markColors {
				if colorDot(c, i, col, colorOf(e.color) == col).Clicked() {
					e.color = col
					if e.tool == toolNone {
						e.setTool(toolRect) // 选了颜色就是要画
					}
				}
			}
			ui.Box(c).Size(1, 20).MarginX(4).Background(ui.RGBA(0, 0, 0, 0.15))
			if toolButton(c, iconUndo, "撤销 "+cmdLabel("Z")).TextColor(iconGray).Clicked() {
				e.undo()
			}
			if toolButton(c, iconClose, "取消 Esc").TextColor(iconCancel).Clicked() {
				s.end()
			}
			if toolButton(c, iconSave, "保存 "+cmdLabel("S")).TextColor(iconGray).Clicked() {
				s.save(sh)
			}
			if toolButton(c, iconCopy, "复制 "+cmdLabel("C")).TextColor(iconGray).Clicked() {
				s.copy(sh)
			}
			if toolButton(c, iconCheck, "收进资料库 ↩").TextColor(iconConfirm).Clicked() {
				s.keep(sh)
			}
		})
}

// colorDot is a color to mark in, ringed when chosen.
func colorDot(c *ui.Context, i int, col ui.Color, on bool) ui.Element {
	names := []string{"红色", "黄色", "绿色", "蓝色"}
	b := ui.ButtonBase(c.Key(names[i])).Size(toolbarDot, 28).Label(names[i]).Tooltip(names[i])
	b.Draw(func(p *ui.Painter, r ui.Rect) {
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		if on {
			p.StrokePath(new(ui.Path).Circle(cx, cy, 9), 2, col)
		}
		p.FillPath(new(ui.Path).Circle(cx, cy, 6), col)
	})
	return b
}

func toolButton(c *ui.Context, icon *ui.SVG, tip string) ui.Element {
	btn := ui.ButtonBase(c).Size(toolbarBtn, 28).Radius(4).Label(tip).Tooltip(tip)
	if btn.Hovered() {
		btn.Background(ui.RGBA(0, 0, 0, 0.06))
	}
	btn.Children(func() { ui.Icon(c, icon).Size(18, 18) })
	return btn
}

func cmdLabel(key string) string {
	if runtime.GOOS == "darwin" {
		return "⌘" + key
	}
	return "Ctrl+" + key
}

// copy 复制选区到剪贴板并结束截图。
func (s *session) copy(sh *screenShot) {
	sh.ed.commitText()
	s.finish(sh, Copy)
}

// save 把选区存成 PNG 并结束截图。
func (s *session) save(sh *screenShot) {
	sh.ed.commitText()
	s.finish(sh, Save)
}

// keep 把选区收进资料库并结束截图。
func (s *session) keep(sh *screenShot) {
	sh.ed.commitText()
	s.finish(sh, Library)
}

// finish 先关掉窗口，再回到主线程渲染导出（不在窗口的帧里做重活）。
func (s *session) finish(sh *screenShot, act Action) {
	if s.ended {
		return
	}
	e := sh.ed
	size, sel, marks := pt{e.bounds.W, e.bounds.H}, e.sel, append([]mark(nil), e.marks...)
	s.endThen(func() {
		var png []byte
		var err error
		mygo.RunOnMain(func() { png, err = encodePNG(renderSelection(sh.bmp, size, sel, marks, sh.scale)) })
		if err == nil && act == Copy {
			err = mygo.Clipboard.WriteImage(png)
		}
		if err != nil {
			log.Println("截图:", err)
			mygo.Dialog.Error("截图失败", err.Error())
			return
		}
		if s.deliver != nil {
			s.deliver(png, act)
		}
	})
}

// isEnded tells, from any goroutine, whether the shot is over.
func (s *session) isEnded() bool {
	var ended bool
	mygo.RunOnMain(func() { ended = s.ended })
	return ended
}

// end 关闭所有选区窗口。
func (s *session) end() { s.endThen(nil) }

// endThen 隐藏窗口，在后台销毁它们后执行 then。
func (s *session) endThen(then func()) {
	if s.ended {
		return
	}
	s.ended = true
	for _, sh := range s.shots {
		sh.win.Hide()
	}
	go func() {
		for _, sh := range s.shots {
			sh.win.Destroy()
		}
		if then != nil {
			then()
		}
		s.onEnd()
	}()
}
