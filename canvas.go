package main

import (
	"fmt"
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

// canvasView draws the map: lines from each node to its children, and the
// nodes over them, in a canvas that scrolls both ways.
func (a *app) canvasView(c *ui.Context) {
	t := c.Theme()
	z := a.zoom
	r := layout.Compute(a.doc, a.measure(c), layout.DefaultOptions)
	a.boxes = r

	branch := map[string]int{}
	depths := map[string]int{}
	a.doc.Walk(func(n, p *model.Node, depth int) bool {
		depths[n.ID] = depth
		switch {
		case p == nil:
		case p == a.doc.Root:
			branch[n.ID] = slices.Index(p.Children, n)
		default:
			branch[n.ID] = branch[p.ID]
		}
		return true
	})

	scroll := ui.ScrollBoth(c.Key("canvas")).Grow(1).TrackScroll(&a.scroll).Focusable().FocusRing(false).Bind(&a.canvas)
	scroll.HandleInput(func(ev ui.InputEvent) bool { return a.canvasKey(ev) })
	if scroll.Pressed() && a.editing == "" {
		a.canvas.Focus()
	}
	scroll.Children(func() {
		view := a.canvas.Bounds(c) // the visible part, as of the last frame
		bg := ui.Box(c).Size(max(r.Width*z, view.W), max(r.Height*z, view.H))
		if dx, dy, ok := bg.Dragged(); ok {
			a.scroll.X -= dx
			a.scroll.Y -= dy
		}
		if bg.Clicked() {
			a.commitEditIfAny()
		}
		bg.Draw(func(p *ui.Painter, rect ui.Rect) {
			for _, e := range r.Edges {
				from, to := r.Boxes[e.From], r.Boxes[e.To]
				x0 := rect.X + (from.X+from.W)*z
				y0 := rect.Y + (from.Y+from.H/2)*z
				x1 := rect.X + to.X*z
				y1 := rect.Y + (to.Y+to.H/2)*z
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
		bg.Children(func() {
			a.doc.Walk(func(n, _ *model.Node, depth int) bool {
				box, ok := r.Boxes[n.ID]
				if !ok {
					return false // under a collapsed node
				}
				a.nodeView(c.Key(n.ID), n, depth, box, branchColor(branch[n.ID]))
				return true
			})
		})
	})
	_ = t
}

func (a *app) commitEditIfAny() {
	if a.editing != "" {
		a.commitEdit()
	}
}

// nodeView draws one node at its box.
func (a *app) nodeView(c *ui.Context, n *model.Node, depth int, box layout.Rect, accent ui.Color) {
	t := c.Theme()
	z := a.zoom
	p := a.parts(n)
	selected := n.ID == a.selected
	editing := n.ID == a.editing

	el := ui.Row(c).Absolute().Left(box.X*z).Top(box.Y*z).Size(box.W*z, box.H*z).
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
			a.moveNode(d.ID, n.ID)
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
	if selected && a.reveal {
		el.ScrollIntoView()
		a.reveal = false
	}

	// The button opening and closing the children, just right of the box.
	if len(n.Children) > 0 && (n.Collapsed || selected) {
		s := 18 * z
		btn := ui.Box(c.Key("fold")).Absolute().Left((box.X+box.W)*z+3*z).Top((box.Y+box.H/2)*z-s/2).
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
			a.toggleCollapse(n.ID)
		}
	}
}

// nodeMenu is a node's context menu.
func (a *app) nodeMenu(m *ui.Menu, n *model.Node, depth int) {
	a.selected = n.ID
	if m.Item("添加子节点").Shortcut(0, ui.KeyTab).Chosen() {
		a.addChild()
	}
	if m.Item("添加同级节点").Shortcut(0, ui.KeyEnter).Chosen() {
		a.addSibling()
	}
	if m.Item("编辑标题").Shortcut(0, ui.KeyF2).Chosen() {
		a.startEdit(n.ID)
	}
	m.Separator()
	if m.Item("作为任务").Checked(n.Task != nil).Chosen() {
		a.toggleTask(n)
	}
	m.Submenu("任务状态", func(m *ui.Menu) {
		for _, s := range model.Statuses {
			if m.Item(s.Label()).Checked(n.Task != nil && n.Task.Status == s).Chosen() {
				a.setStatus(n, s)
			}
		}
	})
	if len(n.Children) > 0 {
		label := "折叠"
		if n.Collapsed {
			label = "展开"
		}
		if m.Item(label).Chosen() {
			a.toggleCollapse(n.ID)
		}
	}
	m.Separator()
	if m.Item("删除").Shortcut(0, ui.KeyDelete).Disabled(depth == 0).Chosen() {
		a.deleteSelected()
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
