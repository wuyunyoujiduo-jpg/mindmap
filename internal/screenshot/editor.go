package screenshot

import "github.com/egoist/mygo/ui"

// tool 是当前选中的标注工具，toolNone 时拖动的是选区本身。
type tool int

const (
	toolNone tool = iota
	toolRect
	toolArrow
	toolPen
	toolText
)

// mark 是一个标注：矩形和箭头用首尾两点，画笔用全部点，文字用首点作左上角。
type mark struct {
	tool tool
	pts  []pt
	text string
	col  ui.Color // the zero color is markColor
}

func (m mark) first() pt { return m.pts[0] }
func (m mark) last() pt  { return m.pts[len(m.pts)-1] }

// empty 报告标注是否小到可以丢掉（误点）。
func (m mark) empty() bool {
	switch m.tool {
	case toolText:
		return m.text == ""
	case toolPen:
		return len(m.pts) < 2
	default:
		r := rectFrom(m.first(), m.last())
		return r.W < 3 && r.H < 3
	}
}

type dragKind int

const (
	dragNone dragKind = iota
	dragCreate
	dragMove
	dragResize
	dragDraw
)

// minSelection 以下的拖动视为误点，不形成选区。
const minSelection = 4

// textLineHeight 是文字输入框的高度，点击处落在这一行的中间。
const textLineHeight = 26

// editor 是一块屏幕上的截图编辑状态，不涉及绘制，方便测试。
type editor struct {
	bounds ui.Rect
	color  ui.Color // of the marks drawn next; the zero color is markColor

	sel    ui.Rect
	hasSel bool
	tool   tool
	marks  []mark
	cur    *mark

	drag      dragKind
	dragStart pt
	dragOrig  ui.Rect
	dragEdge  edge

	// 正在输入的文字标注。
	editing  bool
	editAt   pt
	editText string
}

// result 告诉外面按下之后要做什么。
type result int

const (
	resultNone result = iota
	resultCopy        // 双击选区：复制并结束
)

func (e *editor) down(p pt, clicks int) result {
	e.commitText()
	if !e.hasSel {
		e.startCreate(p)
		return resultNone
	}
	if clicks >= 2 && e.tool != toolText && contains(e.sel, p) {
		e.cur, e.drag = nil, dragNone
		return resultCopy
	}
	if e.tool != toolNone {
		if !contains(e.sel, p) {
			return resultNone
		}
		if e.tool == toolText {
			e.editing, e.editAt, e.editText = true, pt{p.X, p.Y - textLineHeight/2}, ""
			return resultNone
		}
		e.cur = &mark{tool: e.tool, pts: []pt{p, p}, col: e.color}
		e.drag = dragDraw
		return resultNone
	}
	if h, ok := hitHandle(e.sel, p); ok {
		e.drag, e.dragStart, e.dragOrig, e.dragEdge = dragResize, p, e.sel, h
	} else if contains(e.sel, p) {
		e.drag, e.dragStart, e.dragOrig = dragMove, p, e.sel
	} else if len(e.marks) == 0 {
		e.startCreate(p) // 还没标注时，点选区外面重新框选
	}
	return resultNone
}

func (e *editor) startCreate(p pt) {
	e.hasSel, e.sel = false, ui.Rect{}
	e.drag, e.dragStart = dragCreate, clampPt(p, e.bounds)
}

func (e *editor) move(p pt) {
	switch e.drag {
	case dragCreate:
		e.sel = rectFrom(e.dragStart, clampPt(p, e.bounds))
	case dragMove:
		e.sel = moveRect(e.dragOrig, p.X-e.dragStart.X, p.Y-e.dragStart.Y, e.bounds)
	case dragResize:
		e.sel = resizeRect(e.dragOrig, e.dragEdge, p.X-e.dragStart.X, p.Y-e.dragStart.Y, e.bounds)
	case dragDraw:
		q := clampPt(p, e.sel)
		if e.cur.tool == toolPen {
			e.cur.pts = append(e.cur.pts, q)
		} else {
			e.cur.pts[1] = q
		}
	}
}

func (e *editor) up(p pt) {
	e.move(p)
	switch e.drag {
	case dragCreate, dragMove, dragResize:
		e.sel = snapRect(e.sel)
		e.hasSel = e.sel.W >= minSelection && e.sel.H >= minSelection
	case dragDraw:
		if !e.cur.empty() {
			e.marks = append(e.marks, *e.cur)
		}
		e.cur = nil
	}
	e.drag = dragNone
}

// dragging 报告是否正在拖动选区（拖动时隐藏工具条）。
func (e *editor) dragging() bool {
	return e.drag == dragCreate || e.drag == dragMove || e.drag == dragResize
}

func (e *editor) setTool(t tool) {
	e.commitText()
	if e.tool == t {
		t = toolNone // 再点一次取消选中
	}
	e.tool = t
}

func (e *editor) commitText() {
	if !e.editing {
		return
	}
	e.editing = false
	if e.editText != "" {
		e.marks = append(e.marks, mark{tool: toolText, pts: []pt{e.editAt}, text: e.editText, col: e.color})
	}
	e.editText = ""
}

func (e *editor) cancelText() { e.editing, e.editText = false, "" }

func (e *editor) undo() {
	if e.editing {
		e.cancelText()
		return
	}
	if len(e.marks) > 0 {
		e.marks = e.marks[:len(e.marks)-1]
	}
}

// visibleMarks 是要画出来的标注，包括正在画的那一个。
func (e *editor) visibleMarks() []mark {
	if e.cur == nil {
		return e.marks
	}
	return append(e.marks[:len(e.marks):len(e.marks)], *e.cur)
}

// cursor 返回指针在 p 时该显示的形状。
func (e *editor) cursor(p pt) ui.Cursor {
	if !e.hasSel || e.drag == dragCreate {
		return ui.CursorCrosshair
	}
	if e.tool != toolNone {
		if !contains(e.sel, p) {
			return ui.CursorDefault
		}
		if e.tool == toolText {
			return ui.CursorText
		}
		return ui.CursorCrosshair
	}
	if h, ok := hitHandle(e.sel, p); ok {
		switch {
		case h.X == 0:
			return ui.CursorResizeNS
		case h.Y == 0:
			return ui.CursorResizeEW
		case h.X == h.Y:
			return ui.CursorResizeNWSE
		default:
			return ui.CursorResizeNESW
		}
	}
	if contains(e.sel, p) {
		return ui.CursorMove
	}
	if len(e.marks) == 0 {
		return ui.CursorCrosshair
	}
	return ui.CursorDefault
}
