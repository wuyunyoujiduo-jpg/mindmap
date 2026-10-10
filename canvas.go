package main

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/layout"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// nodeDrag is the value a node carries while it is dragged onto another.
type nodeDrag struct{ ID string }

// Sizes of a node's box at zoom 1.
const (
	nodePadX     = 14
	nodeGap      = 6
	nodeHeight   = 34
	rootHeight   = 46
	titleMax     = 260
	titleMin     = 24
	editMinWidth = 120
	markSize     = 11
	dotSize      = 8
)

func titleSize(depth int) float32 {
	switch depth {
	case 0:
		return 18
	case 1:
		return 15
	}
	return 14
}

// nodeParts are what a node shows besides its title.
type nodeParts struct {
	title string
	tags  []string // at most two shown
	more  int      // tags not shown
	due   string
	note  bool
}

func (a *app) parts(n *model.Node) nodeParts {
	p := nodeParts{title: n.Title, note: n.Note != ""}
	if a.editing == n.ID {
		p.title = a.editText
	}
	if p.title == "" {
		p.title = " "
	}
	p.tags = n.Tags
	if len(p.tags) > 2 {
		p.tags, p.more = p.tags[:2], len(p.tags)-2
	}
	if n.Task != nil && n.Task.Due != nil {
		p.due = n.Task.Due.Format("1/2")
	}
	return p
}

// measure sizes a node's box at zoom 1, as nodeView draws it.
func (a *app) measure(c *ui.Context) layout.Measure {
	return func(n *model.Node, depth int) (float32, float32) {
		p := a.parts(n)
		size := titleSize(depth)
		tw, _ := c.MeasureText(0, ui.Span{Text: p.title, Size: size, Weight: weight(depth)})
		tw = min(max(tw, titleMin), titleMax)
		if a.editing == n.ID {
			tw = max(tw+20, editMinWidth)
		}
		w := 2*nodePadX + tw
		if n.Task != nil {
			w += dotSize + nodeGap
		}
		for _, tag := range p.tags {
			mw, _ := c.MeasureText(0, ui.Span{Text: tag, Size: markSize})
			w += nodeGap + mw + markSize*1.1
		}
		if p.more > 0 {
			mw, _ := c.MeasureText(0, ui.Span{Text: fmt.Sprintf("+%d", p.more), Size: markSize})
			w += nodeGap + mw
		}
		if p.due != "" {
			mw, _ := c.MeasureText(0, ui.Span{Text: p.due, Size: markSize})
			w += nodeGap + mw
		}
		if p.note {
			mw, _ := c.MeasureText(0, ui.Span{Text: "✎", Size: markSize + 1})
			w += nodeGap + mw
		}
		h := float32(nodeHeight)
		if depth == 0 {
			h = rootHeight
		}
		return w, h
	}
}

func weight(depth int) int {
	if depth == 0 {
		return 700
	}
	if depth == 1 {
		return 600
	}
	return 400
}

func statusColor(s model.Status) ui.Color {
	switch s {
	case model.StatusIdea:
		return ui.Hex("#a78bfa")
	case model.StatusTodo:
		return ui.Hex("#3b82f6")
	case model.StatusDoing:
		return ui.Hex("#f59e0b")
	case model.StatusDone:
		return ui.Hex("#22c55e")
	}
	return ui.Hex("#9ca3af")
}

// branchColor colors the lines and accents of each top-level branch.
func branchColor(i int) ui.Color {
	return ui.Hex([]string{"#3b82f6", "#10b981", "#f59e0b", "#ec4899", "#8b5cf6", "#06b6d4", "#ef4444", "#84cc16"}[i%8])
}

// canvasView is the map on an endless canvas: dragging its empty part, the
// wheel or the touchpad move it, and Cmd with the wheel zooms. Below it are
// the tabs of the file's canvases.
func (a *app) canvasView(c *ui.Context) {
	ui.Column(c).Grow(1).Children(func() {
		a.canvasArea(c)
		ui.Divider(c)
		a.sheetTabs(c)
	})
}

func (a *app) canvasArea(c *ui.Context) {
	t := c.Theme()
	cam := a.cam()
	z := cam.Zoom
	r := layout.Compute(a.doc, a.measure(c), layout.DefaultOptions)
	a.boxes = r
	a.vp = a.canvas.Bounds(c) // the canvas's box, as of the last frame
	a.mods = c.Modifiers()    // held now; the view builds again as they change

	// Put the map at the left middle the first time, and keep the node
	// selected by keys in view.
	if a.vp.W > 0 {
		if root, ok := r.Boxes[a.doc.Root.ID]; ok && !cam.placed {
			cam.X = 60 - root.X*z
			cam.Y = a.vp.H/2 - (root.Y+root.H/2)*z
			cam.placed = true
		}
		if b, ok := r.Boxes[a.selected]; ok && a.reveal {
			a.reveal = false
			cam.X += keepIn(cam.X+b.X*z, b.W*z, a.vp.W)
			cam.Y += keepIn(cam.Y+b.Y*z, b.H*z, a.vp.H)
		}
	} else {
		c.Invalidate() // lay out once more, with the canvas's size
	}

	branch := map[string]int{}
	a.doc.Walk(func(n, p *model.Node, _ int) bool {
		switch {
		case p == nil:
		case p == a.doc.Root:
			branch[n.ID] = slices.Index(p.Children, n)
		default:
			branch[n.ID] = branch[p.ID]
		}
		return true
	})

	cv := ui.Box(c.Key("canvas")).Grow(1).Clip().Background(t.Background).
		Focusable().FocusRing(false).Bind(&a.canvas).Label("画布")
	cv.HandleInput(a.canvasInput)
	if cv.Pressed() && a.editing == "" {
		a.canvas.Focus()
	}
	if dx, dy, ok := cv.Dragged(); ok {
		cam.X += dx
		cam.Y += dy
	}
	a.panning = cv.Pressed()
	if a.panning {
		cv.Cursor(ui.CursorGrabbing)
	} else {
		cv.Cursor(ui.CursorGrab)
	}
	if cv.Clicked() {
		a.commitEditIfAny()
	}
	cv.Draw(func(p *ui.Painter, rect ui.Rect) {
		// A grid of dots, which moves with the map.
		step := 24 * z
		for step < 12 {
			step *= 2
		}
		dot := t.Border
		x0 := rect.X + mod(cam.X, step)
		y0 := rect.Y + mod(cam.Y, step)
		for y := y0; y < rect.Y+rect.H; y += step {
			for x := x0; x < rect.X+rect.W; x += step {
				p.Fill(ui.Rect{X: x - 1, Y: y - 1, W: 2, H: 2}, dot, 1)
			}
		}
		ox, oy := rect.X+cam.X, rect.Y+cam.Y
		for _, e := range r.Edges {
			from, to := r.Boxes[e.From], r.Boxes[e.To]
			x0 := ox + (from.X+from.W)*z
			y0 := oy + (from.Y+from.H/2)*z
			x1 := ox + to.X*z
			y1 := oy + (to.Y+to.H/2)*z
			var path ui.Path
			path.MoveTo(x0, y0)
			mid := (x1 - x0) / 2
			path.CubeTo(x0+mid, y0, x1-mid, y1, x1, y1)
			col := branchColor(branch[e.To])
			if a.filtering() {
				col = col.Alpha(0.3)
			}
			p.StrokePath(&path, max(1.5*z, 1), col)
		}
	})
	cv.Children(func() {
		a.doc.Walk(func(n, _ *model.Node, depth int) bool {
			box, ok := r.Boxes[n.ID]
			if !ok {
				return false // under a collapsed node
			}
			a.nodeView(c.Key(n.ID), n, depth, box, branchColor(branch[n.ID]))
			return true
		})
		// Zoom and recenter, over the canvas's corner.
		ui.Row(c).Absolute().Right(12).Bottom(12).Gap(2).Padding(3).Radius(8).
			Background(t.Surface).Border(1, t.Border).Children(func() {
			if ui.Button(c, "−").Label("缩小").Tooltip("缩小 ⌘-").Width(30).Clicked() {
				a.setZoom(cam.Zoom / 1.2)
			}
			if ui.Button(c, fmt.Sprintf("%d%%", int(cam.Zoom*100+0.5))).Label("缩放比例").Tooltip("实际大小 ⌘0").MinWidth(56).Clicked() {
				a.setZoom(1)
			}
			if ui.Button(c, "+").Label("放大").Tooltip("放大 ⌘=").Width(30).Clicked() {
				a.setZoom(cam.Zoom * 1.2)
			}
			if ui.Button(c, "◎").Label("回到中心").Tooltip("回到中心 ⌘⇧0").Width(30).Clicked() {
				a.recenter()
			}
		})
	})
}

// keepIn returns how far to move a span at pos, size long, to keep it in
// [margin, length-margin], preferring its start.
func keepIn(pos, size, length float32) float32 {
	const margin = 40
	switch {
	case pos < margin:
		return margin - pos
	case pos+size > length-margin:
		return max(length-margin-pos-size, margin-pos)
	}
	return 0
}

func mod(x, m float32) float32 {
	r := float32(math.Mod(float64(x), float64(m)))
	if r < 0 {
		r += m
	}
	return r
}

// canvasInput takes the wheel, the touchpad and the keys on the canvas.
func (a *app) canvasInput(ev ui.InputEvent) bool {
	cam := a.cam()
	switch ev.Kind {
	case ui.InputPointerMove:
		a.pointer = [2]float32{ev.X, ev.Y}
		return false
	case ui.InputScroll:
		mods := ev.Mods | a.mods
		if mods&(ui.Cmd|ui.Ctrl) != 0 {
			f := float32(math.Exp(float64(-ev.DY) * 0.01))
			if !ev.Precise {
				f = float32(math.Pow(1.1, float64(-ev.DY)/40))
			}
			a.setZoomAt(cam.Zoom*f, a.pointer[0], a.pointer[1])
			return true
		}
		dx, dy := ev.DX, ev.DY
		if mods&ui.Shift != 0 && dx == 0 {
			dx, dy = dy, 0
		}
		cam.X -= dx
		cam.Y -= dy
		return true
	}
	return a.canvasKey(ev)
}

// sheetTabs shows the canvases of the file as tabs: a click shows one, a
// double click renames it, and its context menu copies, moves or deletes it.
func (a *app) sheetTabs(c *ui.Context) {
	t := c.Theme()
	ui.ScrollHorizontal(c.Key("sheets")).Background(t.Surface).Children(func() {
		ui.Row(c).Padding(4, 8).Gap(4).AlignItems(ui.Center).Children(func() {
			for _, d := range a.file.Sheets {
				a.sheetTab(c.Key(d.ID), d)
			}
			if ui.Button(c, "+").Label("新建画布").Tooltip("新建画布").Width(28).Clicked() {
				a.addSheet()
			}
		})
	})
	if ui.AlertDialog(c, &a.confirmSheet, "删除画布「"+a.sheetName(a.renamingSheet)+"」？", "画布上的所有节点都会删除，可以用撤销恢复。", "取消", "删除") == 1 {
		a.deleteSheet(a.renamingSheet)
		a.renamingSheet = ""
	}
}

func (a *app) sheetName(id string) string {
	if d, _ := a.file.Sheet(id); d != nil {
		return d.Title
	}
	return ""
}

func (a *app) sheetTab(c *ui.Context, d *model.Document) {
	t := c.Theme()
	active := d == a.doc
	tab := ui.Row(c).Padding(4, 12).Radius(6).Cursor(ui.CursorPointer).Label(d.Title)
	if active {
		tab.Background(t.Background).Border(1, t.Border)
	} else if tab.Hovered() {
		tab.Background(t.SurfaceHover)
	}
	if a.renamingSheet == d.ID && !a.confirmSheet {
		tab.Children(func() {
			in := ui.TextInputBase(c.Key("rename"), &a.sheetTitle).Width(120).AutoFocus()
			switch {
			case in.Shortcut(0, ui.KeyEscape):
				a.renamingSheet = ""
			case in.Submitted():
				a.renameSheet(d.ID, a.sheetTitle)
				a.renamingSheet = ""
			case in.PressedOutside():
				a.renameSheet(d.ID, a.sheetTitle)
				a.renamingSheet = ""
			}
		})
		return
	}
	if tab.Pressed() && !active {
		a.showSheet(d.ID)
	}
	if tab.DoubleClicked() {
		a.renamingSheet, a.sheetTitle = d.ID, d.Title
	}
	_, i := a.file.Sheet(d.ID)
	id := d.ID
	tab.ContextMenu(func(m *ui.Menu) {
		if m.Item("重命名").Chosen() {
			a.renamingSheet, a.sheetTitle = id, a.sheetName(id)
		}
		if m.Item("复制画布").Chosen() {
			a.later(func() { a.duplicateSheet(id) })
		}
		m.Separator()
		if m.Item("左移").Disabled(i == 0).Chosen() {
			a.later(func() { a.moveSheet(id, -1) })
		}
		if m.Item("右移").Disabled(i == len(a.file.Sheets)-1).Chosen() {
			a.later(func() { a.moveSheet(id, 1) })
		}
		m.Separator()
		if m.Item("删除画布…").Disabled(len(a.file.Sheets) == 1).Chosen() {
			a.renamingSheet, a.confirmSheet = id, true
		}
	})
	tab.Children(func() {
		text := ui.Text(c, d.Title).FontSize(13).NoWrap()
		if !active {
			text.TextColor(t.TextMuted)
		}
	})
}

func (a *app) commitEditIfAny() {
	if a.editing != "" {
		a.commitEdit()
	}
}

// nodeView draws one node at its box.
func (a *app) nodeView(c *ui.Context, n *model.Node, depth int, box layout.Rect, accent ui.Color) {
	t := c.Theme()
	cam := a.cam()
	z := cam.Zoom
	p := a.parts(n)
	selected := n.ID == a.selected
	editing := n.ID == a.editing

	el := ui.Row(c).Absolute().Left(cam.X+box.X*z).Top(cam.Y+box.Y*z).Size(box.W*z, box.H*z).
		Padding(0, nodePadX*z).Gap(nodeGap * z).AlignItems(ui.Center).Radius(8 * z)
	switch depth {
	case 0:
		el.Background(t.Text).TextColor(t.Background)
	case 1:
		el.Background(accent.Alpha(0.14)).Border(1, accent.Alpha(0.5))
	default:
		el.Background(t.Surface).Border(1, t.Border)
	}
	if selected {
		el.Border(2*z, t.Accent)
	}
	if a.filtering() && !a.highlighted(n) {
		el.Opacity(0.35)
	} else if a.filtering() {
		el.Shadow(0, 0, 0, 3*z, ui.Hex("#facc15").Alpha(0.6))
	}
	if n.Task != nil && n.Task.Status == model.StatusDone {
		el.Opacity(0.7)
	}
	label := n.Title
	if n.Task != nil {
		label += "，" + n.Task.Status.Label()
	}
	el.Label(label)

	// Selecting, editing, dragging.
	if !editing {
		if el.Pressed() && !selected {
			a.commitEditIfAny()
			a.selected = n.ID
		}
		if el.DoubleClicked() {
			a.startEdit(n.ID)
		}
		if depth > 0 {
			el.Drag(nodeDrag{n.ID})
		}
		if d, over := ui.DragOver[nodeDrag](el); over && d.ID != n.ID {
			el.Border(2*z, t.Accent).BorderStyle(ui.BorderDashed)
		}
		if d, ok := ui.Drop[nodeDrag](el); ok {
			id := n.ID
			a.later(func() { a.moveNode(d.ID, id) })
		}
		el.ContextMenu(func(m *ui.Menu) { a.nodeMenu(m, n, depth) })
	}

	el.Children(func() {
		if n.Task != nil {
			ui.Box(c).Size(dotSize*z, dotSize*z).Radius(999).Background(statusColor(n.Task.Status)).Shrink(0)
		}
		size := titleSize(depth) * z
		tw, _ := c.MeasureText(0, ui.Span{Text: p.title, Size: titleSize(depth), Weight: weight(depth)})
		tw = min(max(tw, titleMin), titleMax)
		if editing {
			in := ui.TextInputBase(c.Key("edit"), &a.editText).FontSize(size).FontWeight(weight(depth)).
				Placeholder("新主题").Width(max(tw+20, editMinWidth) * z).AutoFocus()
			if in.Shortcut(0, ui.KeyEscape) {
				a.cancelEdit()
			} else if in.Submitted() {
				a.commitEdit()
			} else if in.Focused() {
				if !a.editFocus {
					in.SetTextSelection(0, utf8.RuneCountInString(a.editText)) // typing replaces the title
				}
				a.editFocus = true
			} else if a.editFocus {
				a.commitEdit() // the focus went elsewhere
			}
			return
		}
		title := ui.Text(c, n.Title).FontSize(size).FontWeight(weight(depth)).NoWrap().Ellipsis("…").Width(tw * z)
		if n.Task != nil && n.Task.Status == model.StatusDone {
			title.Strikethrough()
		}
		for _, tag := range p.tags {
			tagChip(c.Key("tag:"+tag), tag, tag, false, markSize*z).PassThrough()
		}
		if p.more > 0 {
			ui.Text(c, "+"+strconv.Itoa(p.more)).FontSize(markSize * z).TextColor(t.TextMuted).NoWrap()
		}
		if p.due != "" {
			col := t.TextMuted
			if n.Task.Overdue(a.now()) {
				col = t.Danger
			}
			ui.Text(c, p.due).FontSize(markSize * z).TextColor(col).NoWrap()
		}
		if p.note {
			ui.Text(c, "✎").FontSize((markSize + 1) * z).TextColor(t.TextMuted).NoWrap()
		}
	})

	// The button opening and closing the children, just right of the box.
	if len(n.Children) > 0 && (n.Collapsed || selected) {
		s := 18 * z
		btn := ui.Box(c.Key("fold")).Absolute().Left(cam.X+(box.X+box.W)*z+3*z).Top(cam.Y+(box.Y+box.H/2)*z-s/2).
			Size(s, s).Radius(999).Center().Background(t.Background).Border(1, accent).Cursor(ui.CursorPointer)
		text := "−"
		if n.Collapsed {
			text = strconv.Itoa(len(n.Children))
			btn.Label("展开")
		} else {
			btn.Label("折叠")
		}
		btn.Children(func() {
			ui.Text(c, text).FontSize(11 * z).TextColor(accent).Bold()
		})
		if btn.Clicked() {
			a.selected = n.ID
			a.later(func() { a.toggleCollapse(n.ID) })
		}
	}
}

// nodeMenu is a node's context menu. Its actions wait until the map is
// built, as they change the nodes being walked.
func (a *app) nodeMenu(m *ui.Menu, n *model.Node, depth int) {
	a.selected = n.ID
	if m.Item("添加子节点").Shortcut(0, ui.KeyTab).Chosen() {
		a.later(func() { a.addChild() })
	}
	if m.Item("添加同级节点").Shortcut(0, ui.KeyEnter).Chosen() {
		a.later(func() { a.addSibling() })
	}
	if m.Item("编辑标题").Shortcut(0, ui.KeyF2).Chosen() {
		a.later(func() { a.startEdit(n.ID) })
	}
	m.Separator()
	if m.Item("作为任务").Checked(n.Task != nil).Chosen() {
		a.later(func() { a.toggleTask(n) })
	}
	m.Submenu("任务状态", func(m *ui.Menu) {
		for _, s := range model.Statuses {
			if m.Item(s.Label()).Checked(n.Task != nil && n.Task.Status == s).Chosen() {
				a.later(func() { a.setStatus(n, s) })
			}
		}
	})
	if len(n.Children) > 0 {
		label := "折叠"
		if n.Collapsed {
			label = "展开"
		}
		if m.Item(label).Chosen() {
			a.later(func() { a.toggleCollapse(n.ID) })
		}
	}
	m.Separator()
	if m.Item("删除").Shortcut(0, ui.KeyDelete).Disabled(depth == 0).Chosen() {
		a.later(func() { a.deleteSelected() })
	}
}

// canvasKey handles the keys pressed while the canvas has the focus.
func (a *app) canvasKey(ev ui.InputEvent) bool {
	if a.editing != "" || a.doc == nil {
		return false
	}
	if ev.Kind == ui.InputCommand {
		switch ev.Text {
		case "undo":
			a.undo()
			return true
		case "redo":
			a.redo()
			return true
		case "delete":
			a.deleteSelected()
			return true
		}
		return false
	}
	if ev.Kind != ui.InputKeyDown {
		return false
	}
	switch {
	case ev.Mods == 0 && ev.Key == ui.KeyTab:
		a.addChild()
	case ev.Mods == 0 && ev.Key == ui.KeyEnter:
		a.addSibling()
	case ev.Mods == 0 && (ev.Key == ui.KeyDelete || ev.Key == ui.KeyBackspace):
		a.deleteSelected()
	case ev.Mods == 0 && (ev.Key == ui.KeyF2 || ev.Key == ui.KeySpace):
		a.startEdit(a.selected)
	case ev.Mods == 0 && ev.Key == ui.KeyLeft:
		a.navParent()
	case ev.Mods == 0 && ev.Key == ui.KeyRight:
		a.navChild()
	case ev.Mods == 0 && ev.Key == ui.KeyUp:
		a.navSibling(-1)
	case ev.Mods == 0 && ev.Key == ui.KeyDown:
		a.navSibling(1)
	case ev.Mods == ui.Cmd && ev.Key == ui.KeyUp:
		a.shiftSelected(-1)
	case ev.Mods == ui.Cmd && ev.Key == ui.KeyDown:
		a.shiftSelected(1)
	case ev.Mods == ui.Cmd && ev.Key == ui.KeySlash:
		a.toggleCollapse(a.selected)
	case ev.Mods == ui.Cmd && ev.Key == ui.KeyZ:
		a.undo()
	case ev.Mods == ui.Cmd|ui.Shift && ev.Key == ui.KeyZ:
		a.redo()
	default:
		return false
	}
	return true
}
