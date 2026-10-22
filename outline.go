package main

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// outlineBlockFocus is the focus of the outline while lines are selected
// whole: the column of lines, which takes their keys.
const outlineBlockFocus = "outline-block"

// outlineRow is a node shown in the outline, with its depth.
type outlineRow struct {
	node  *model.Node
	depth int
}

// outlineRows lists the nodes below the root that the outline shows, in
// order: those under a folded node are left out.
func outlineRows(d *model.Document) []outlineRow {
	var rows []outlineRow
	var walk func(n *model.Node, depth int)
	walk = func(n *model.Node, depth int) {
		for _, c := range n.Children {
			rows = append(rows, outlineRow{c, depth})
			if !c.Collapsed {
				walk(c, depth+1)
			}
		}
	}
	walk(d.Root, 0)
	return rows
}

// outlineView shows the canvas as text: the root as a heading and every
// node as a line under it, indented by its depth. Its keys edit the tree:
// Enter adds a line, Tab and Shift+Tab indent and outdent, Backspace on an
// empty line deletes it, and the arrows move between lines. Dragging across
// lines, or Shift and the arrows, selects lines whole, which copy as an
// outline, nested as they are.
func (a *app) outlineView(c *ui.Context) {
	t := c.Theme()
	rows := outlineRows(a.doc)
	if a.outlineBlock && (a.outlineIndex(rows, a.outlineAnchor) < 0 || a.outlineIndex(rows, a.outlineEnd) < 0) {
		a.outlineBlock = false // lines deleted, or folded away
	}
	if a.outlineRowAt == nil {
		a.outlineRowAt = map[string]ui.Rect{}
	}
	ui.Scroll(c.Key("outline:" + a.doc.ID)).Grow(1).Children(func() {
		col := ui.Column(c.Key("outline-lines")).Padding(32, 40).Gap(4).MaxWidth(860).Margin(0, ui.Auto).FillWidth().
			FocusBind(&a.outlineFocus, outlineBlockFocus).Label("大纲")
		a.outlineBlockKeys(col, rows)
		// Where the pointer is, in the window, which a line dragged from
		// selects to: the column sees it move over the other lines.
		if _, y, ok := col.PointerPosition(); ok {
			a.outlineDragY = col.Bounds().Y + y
		}
		col.Children(func() {
			root := a.doc.Root
			title := root.Title
			head := ui.Box(c).Padding(10, 14).Radius(8).Background(t.Accent.Alpha(0.1))
			head.Children(func() {
				in := ui.TextInputBase(c.Key("outline-root"), &title).FontSize(26).Bold().FillWidth().
					Placeholder("中心主题").FocusBind(&a.outlineFocus, root.ID)
				if in.Changed() {
					a.mutate("title:"+root.ID, func() { root.Title = title })
				}
				if in.Focused() {
					a.selected = root.ID
				}
				if in.Submitted() || in.Shortcut(0, ui.KeyDown) {
					if len(root.Children) == 0 {
						a.outlineAdd(root.ID, true)
					} else {
						a.outlineFocus = root.Children[0].ID
					}
				}
			})
			ui.Box(c).Height(8)
			for i, r := range rows {
				a.outlineLine(c.Key(r.node.ID), rows, i)
			}
			if len(rows) == 0 {
				ui.Text(c, "在标题上按回车，开始写第一行").TextColor(t.TextMuted).Margin(8, 14)
			}
		})
	})
}

func (a *app) outlineLine(c *ui.Context, rows []outlineRow, i int) {
	t := c.Theme()
	n, depth := rows[i].node, rows[i].depth
	title := n.Title
	line := ui.Row(c).Padding(3, 0, 3, float32(depth)*26).Gap(8).AlignItems(ui.Start).Radius(6)
	a.outlineRowAt[n.ID] = line.Bounds()
	if a.outlineBlock && a.isSelected(n.ID) {
		line.Background(t.Accent.Alpha(0.18))
	}
	line.Children(func() {
		// The bullet: a dot, or, for a line with lines under it, an arrow
		// that folds them.
		bullet := ui.Box(c).Size(18, 18).Center().Shrink(0)
		if len(n.Children) > 0 {
			bullet.Cursor(ui.CursorPointer).Label("折叠")
			if bullet.Clicked() {
				a.later(func() { a.toggleCollapse(n.ID) })
			}
		}
		bullet.Children(func() {
			switch {
			case len(n.Children) > 0 && n.Collapsed:
				ui.Text(c, "▸").FontSize(13).TextColor(t.TextMuted)
			case len(n.Children) > 0:
				ui.Text(c, "▾").FontSize(13).TextColor(t.TextMuted)
			default:
				ui.Box(c).Size(6, 6).Radius(999).Background(t.Text)
			}
		})
		if n.Task != nil {
			a.taskCheck(c, n, 15)
		}
		for _, m := range n.Markers {
			if n.Task != nil && strings.HasPrefix(m, "task:") {
				continue // the task's box above shows its status
			}
			markerView(c, m, 16)
		}
		if n.Image != nil {
			if ui.Text(c, "🖼").FontSize(14).Cursor(ui.CursorPointer).Tooltip("查看图片").Clicked() {
				a.lightbox = n.ID
			}
		}
		size := float32(16)
		if depth == 0 {
			size = 18
		}
		// A long line wraps: the field grows to show it all.
		in := ui.TextAreaBase(c.Key("line"), &title).Lines(1, 30).FontSize(size).Grow(1).Shrink(1).MinWidth(40).
			Placeholder("输入内容").Label("大纲 "+n.Title).FocusBind(&a.outlineFocus, n.ID)
		if depth == 0 {
			in.FontWeight(600)
		}
		if n.Task != nil && n.Task.Status == model.StatusDone {
			in.TextColor(t.TextMuted)
		}
		if in.Changed() {
			title = strings.ReplaceAll(strings.ReplaceAll(title, "\r", ""), "\n", " ") // a title is one line
			a.mutate("title:"+n.ID, func() { n.Title = title })
		}
		if in.Focused() && !a.outlineBlock {
			a.selected, a.multi = n.ID, nil
		}
		// Dragging from the line onto others selects the lines whole; back
		// on it, its text is selected again.
		if pressed := in.Pressed(); pressed || a.outlineDragFrom == n.ID {
			// Up to where it was let go: the pointer's place may come a
			// frame after the press it moves with.
			if over := a.outlineRowUnder(rows, a.outlineDragY); over != "" && over != n.ID {
				a.outlineSelect(rows, n.ID, over)
			} else if over == n.ID && a.outlineBlock {
				a.outlineBlock, a.multi, a.selected = false, nil, n.ID
			}
			a.outlineDragFrom = n.ID
			if !pressed {
				a.outlineDragFrom = ""
				if a.outlineBlock {
					a.outlineFocus = outlineBlockFocus // the drag ended: the lines take the keys
				}
			}
		}
		// Lines pasted, as copied from here or another outliner, come in
		// as lines after this one, nested as they were.
		in.OnPaste(func(text string) bool {
			if !strings.Contains(strings.TrimSpace(text), "\n") {
				return false
			}
			a.later(func() { a.outlinePaste(n.ID) })
			return true
		})
		prev, next := "", ""
		if i > 0 {
			prev = rows[i-1].node.ID
		} else {
			prev = a.doc.Root.ID
		}
		if i+1 < len(rows) {
			next = rows[i+1].node.ID
		}
		// The field takes keys before shortcuts: its key handler, which
		// comes first, makes the outline's keys. Return adds a line, Tab
		// indents, the arrows go to the line above or below, Backspace on
		// an empty line deletes it.
		in.HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind != ui.InputKeyDown {
				return false
			}
			switch {
			case ev.Mods == ui.Shift && ev.Key == ui.KeyUp && i > 0:
				a.outlineSelect(rows, n.ID, prev)
			case ev.Mods == ui.Shift && ev.Key == ui.KeyDown && next != "":
				a.outlineSelect(rows, n.ID, next)
			case ev.Mods == 0 && ev.Key == ui.KeyBackspace && n.Title == "" && len(n.Children) == 0:
				a.mutate("", func() { _, _ = a.doc.Delete(n.ID) })
				a.outlineFocus = prev
			case ev.Mods == 0 && ev.Key == ui.KeyEnter:
				a.later(func() { a.outlineAdd(n.ID, false) })
			case ev.Mods == 0 && ev.Key == ui.KeyTab:
				a.later(func() {
					a.mutate("", func() { _ = a.doc.Indent(n.ID) })
					a.outlineFocus = n.ID
				})
			case ev.Mods == ui.Shift && ev.Key == ui.KeyTab:
				a.later(func() {
					a.mutate("", func() { _ = a.doc.Outdent(n.ID) })
					a.outlineFocus = n.ID
				})
			case ev.Mods == 0 && ev.Key == ui.KeyUp:
				a.outlineFocus = prev
			case ev.Mods == 0 && ev.Key == ui.KeyDown && next != "":
				a.outlineFocus = next
			case ev.Mods == ui.Cmd && ev.Key == ui.KeyUp:
				a.later(func() { a.mutate("", func() { _ = a.doc.Shift(n.ID, -1) }); a.outlineFocus = n.ID })
			case ev.Mods == ui.Cmd && ev.Key == ui.KeyDown:
				a.later(func() { a.mutate("", func() { _ = a.doc.Shift(n.ID, 1) }); a.outlineFocus = n.ID })
			default:
				return false
			}
			return true
		})
	})
}

// outlineAdd adds an empty line after the node with the ID, or as its
// first child, and moves the focus to it. A line with lines open under it
// gets its new line first among them, as in other outliners.
func (a *app) outlineAdd(id string, child bool) {
	n, _ := a.doc.Find(id)
	if n == nil {
		return
	}
	if !child && len(n.Children) > 0 && !n.Collapsed {
		child = true
	}
	var added *model.Node
	a.mutate("", func() {
		if child {
			added = model.NewNode("")
			n.Children = append([]*model.Node{added}, n.Children...)
			n.Collapsed = false
		} else {
			added, _ = a.doc.AddSibling(id, "")
		}
	})
	if added != nil {
		a.outlineFocus = added.ID
		a.selected = added.ID
	}
}

func (a *app) outlineIndex(rows []outlineRow, id string) int {
	return slices.IndexFunc(rows, func(r outlineRow) bool { return r.node.ID == id })
}

// outlineRowUnder is the line at a height of the window, or the first or
// last when it is above or below them all.
func (a *app) outlineRowUnder(rows []outlineRow, y float32) string {
	if len(rows) == 0 {
		return ""
	}
	for _, r := range rows {
		b, ok := a.outlineRowAt[r.node.ID]
		if ok && y >= b.Y && y < b.Y+b.H+4 { // the gap below a line is its
			return r.node.ID
		}
	}
	if b := a.outlineRowAt[rows[0].node.ID]; y < b.Y {
		return rows[0].node.ID
	}
	if b := a.outlineRowAt[rows[len(rows)-1].node.ID]; y >= b.Y+b.H {
		return rows[len(rows)-1].node.ID
	}
	return ""
}

// outlineSelect selects the lines from one to another, whole: they are
// the nodes selected, which copy, cut and delete together.
func (a *app) outlineSelect(rows []outlineRow, from, to string) {
	i, j := a.outlineIndex(rows, from), a.outlineIndex(rows, to)
	if i < 0 || j < 0 {
		return
	}
	if !a.outlineBlock {
		a.outlineFocus = outlineBlockFocus
	}
	a.outlineBlock, a.outlineAnchor, a.outlineEnd = true, from, to
	a.selected, a.multi = to, map[string]bool{}
	for k := min(i, j); k <= max(i, j); k++ {
		if id := rows[k].node.ID; id != to {
			a.multi[id] = true
		}
	}
}

// outlineBlockKeys are the keys of the lines selected whole: Shift and the
// arrows grow or shrink the selection, ⌘C and ⌘X copy and cut the lines,
// Delete deletes them, Escape, the arrows and Enter go back to editing.
func (a *app) outlineBlockKeys(col ui.Element, rows []outlineRow) {
	if !a.outlineBlock {
		return
	}
	// ⌘C, ⌘X and ⌘A come as the Edit menu's commands, which take the keys
	// first, as Delete may.
	col.HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind != ui.InputCommand || !a.outlineBlock {
			return false
		}
		switch ev.Text {
		case "copy":
			a.copyNodes(false)
		case "cut":
			a.copyNodes(true)
			a.outlineBlock, a.outlineFocus = false, a.selected
		case "delete":
			a.deleteSelection()
			a.outlineBlock, a.outlineFocus = false, a.selected
		case "selectAll":
			if len(rows) > 0 {
				a.outlineSelect(rows, rows[0].node.ID, rows[len(rows)-1].node.ID)
			}
		default:
			return false
		}
		return true
	})
	end := a.outlineIndex(rows, a.outlineEnd)
	edit := func(id string) {
		a.outlineBlock, a.multi, a.selected = false, nil, id
		a.outlineFocus = id
	}
	switch {
	case col.Shortcut(ui.Shift, ui.KeyUp) && end > 0:
		a.outlineSelect(rows, a.outlineAnchor, rows[end-1].node.ID)
	case col.Shortcut(ui.Shift, ui.KeyDown) && end+1 < len(rows):
		a.outlineSelect(rows, a.outlineAnchor, rows[end+1].node.ID)
	case col.Shortcut(ui.Cmd, ui.KeyC):
		a.copyNodes(false)
	case col.Shortcut(ui.Cmd, ui.KeyX):
		a.copyNodes(true)
		a.outlineBlock = false
		a.outlineFocus = a.selected
	case col.Shortcut(0, ui.KeyBackspace), col.Shortcut(0, ui.KeyDelete):
		a.deleteSelection()
		a.outlineBlock = false
		a.outlineFocus = a.selected
	case col.Shortcut(ui.Cmd, ui.KeyA):
		if len(rows) > 0 {
			a.outlineSelect(rows, rows[0].node.ID, rows[len(rows)-1].node.ID)
		}
	case col.Shortcut(0, ui.KeyEscape), col.Shortcut(0, ui.KeyEnter):
		edit(a.outlineEnd)
	case col.Shortcut(0, ui.KeyUp):
		edit(rows[max(end-1, 0)].node.ID)
	case col.Shortcut(0, ui.KeyDown):
		edit(rows[min(end+1, len(rows)-1)].node.ID)
	}
}

// outlinePaste puts what is on the clipboard after a line, as lines at its
// depth: nodes copied here, or text, a line a node, nested by indentation.
func (a *app) outlinePaste(id string) {
	n, parent := a.doc.Find(id)
	if n == nil || parent == nil {
		return
	}
	text := a.plat.ClipboardText()
	var nodes []*model.Node
	if a.clip != nil && text == a.clipText {
		for _, c := range a.clip {
			cp := c.Copy()
			copyAssets(cp, a.clipAssets, model.AssetsDir(a.path))
			nodes = append(nodes, cp)
		}
	} else {
		nodes = model.ParseOutline(text)
	}
	if len(nodes) == 0 {
		return
	}
	a.mutate("", func() {
		at := slices.Index(parent.Children, n) + 1
		if n.Title == "" && len(n.Children) == 0 { // an empty line takes the first
			at--
			parent.Children = slices.Delete(parent.Children, at, at+1)
		}
		parent.Children = slices.Insert(parent.Children, at, nodes...)
	})
	last := nodes[len(nodes)-1]
	a.outlineFocus, a.selected, a.multi = last.ID, last.ID, nil
	a.tell("已粘贴 " + itoa(len(nodes)) + " 行")
}
