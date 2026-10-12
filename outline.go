package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// outlineText is the text of an outline line, and the title it was last
// seen to be.
type outlineText struct{ text, seen string }

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
// empty line deletes it, and the arrows move between lines.
func (a *app) outlineView(c *ui.Context) {
	t := c.Theme()
	rows := outlineRows(a.doc)
	a.keepOutlineFocus(rows)
	ui.Scroll(c.Key("outline:" + a.doc.ID)).Grow(1).Children(func() {
		ui.Column(c).Padding(32, 40).Gap(4).MaxWidth(860).Margin(0, ui.Auto).FillWidth().Children(func() {
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
			lo, hi := a.outlineSelRange(rows)
			// While lines are selected the keys, and the Edit menu's
			// commands, go here rather than to a line's text, which would
			// copy its own.
			ui.Box(c.Key("outline-sel")).Size(0, 0).Focusable().Bind(&a.outlineSelKeys).Label("选中的行").
				HandleInput(func(ev ui.InputEvent) bool { return a.outlineSelKey(rows, ev) })
			for i, r := range rows {
				a.outlineLine(c.Key(r.node.ID), rows, i, i >= lo && i <= hi)
			}
			if len(rows) == 0 {
				ui.Text(c, "在标题上按回车，开始写第一行").TextColor(t.TextMuted).Margin(8, 14)
			}
			// Below the lines: a click there writes on in the last one,
			// rather than leave no line to type in.
			end := ui.Box(c.Key("outline-end")).FillWidth().Height(240).Cursor(ui.CursorText)
			if end.Pressed() {
				a.outlineFocus = root.ID
				if len(rows) > 0 {
					a.outlineFocus = rows[len(rows)-1].node.ID
				}
			}
		})
	})
}

// keepOutlineFocus moves the focus off a line that folding its parent hid,
// to the nearest line still shown: without it no line would take what is
// typed, and input methods would compose in the corner of the screen.
func (a *app) keepOutlineFocus(rows []outlineRow) {
	if a.outlineFocus == "" || a.outlineFocus == a.doc.Root.ID {
		return
	}
	path := a.doc.Path(a.outlineFocus)
	if len(path) == 0 {
		return
	}
	shown := make(map[string]bool, len(rows))
	for _, r := range rows {
		shown[r.node.ID] = true
	}
	for i := len(path) - 1; i >= 0; i-- {
		if id := path[i].ID; id == a.doc.Root.ID || shown[id] {
			if id != a.outlineFocus {
				a.outlineFocus = id
			}
			return
		}
	}
}

func (a *app) outlineLine(c *ui.Context, rows []outlineRow, i int, selectedLine bool) {
	t := c.Theme()
	n, depth := rows[i].node, rows[i].depth
	// The line's text stays from frame to frame: the area writes what is
	// typed into it as it comes, after the frame that built it.
	line := a.outlineText[n.ID]
	if line == nil {
		line = &outlineText{text: n.Title, seen: n.Title}
		if a.outlineText == nil {
			a.outlineText = map[string]*outlineText{}
		}
		a.outlineText[n.ID] = line
	}
	switch {
	case n.Title != line.seen: // changed elsewhere, as by undo
		line.text, line.seen = n.Title, n.Title
	case line.text != line.seen: // typed
		if strings.ContainsAny(line.text, "\r\n\t") { // pasted lines join: a title has no line breaks
			line.text = strings.Join(strings.Fields(strings.ReplaceAll(line.text, "\r", "\n")), " ")
		}
		line.seen = line.text
		a.mutate("title:"+n.ID, func() { n.Title = line.text })
	}
	size := float32(16)
	if depth == 0 {
		size = 18
	}
	// The marks before the title sit by its first row, as high as a row.
	_, rowH := a.measureText(c, 0, ui.Span{Text: "国", Size: size})
	row := ui.Row(c).Padding(3, 0, 3, float32(depth)*26).Gap(8).AlignItems(ui.Start).Radius(6).Bind(a.outlineRowHandle(n.ID))
	if selectedLine {
		row.Background(t.Accent.Alpha(0.14))
	}
	row.Children(func() {
		ui.Row(c).Height(rowH).Gap(8).AlignItems(ui.Center).Shrink(0).Children(func() {
			// The bullet: a dot, or, for a line with lines under it, an arrow
			// that folds them.
			bullet := ui.Box(c).Size(18, 18).Center().Shrink(0)
			if len(n.Children) > 0 {
				bullet.Cursor(ui.CursorPointer).Label("折叠")
				if bullet.Clicked() {
					a.later(func() { a.toggleCollapse(n.ID) })
					a.outlineFocus = n.ID // the click takes the focus: give it to the line
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
				markerView(c, m, 16)
			}
			if n.Image != nil {
				if ui.Text(c, "🖼").FontSize(14).Cursor(ui.CursorPointer).Tooltip("查看图片").Clicked() {
					a.lightbox = n.ID
				}
			}
		})
		// A text area, wrapping a long title onto more rows, rather than a
		// line that hides what does not fit.
		in := ui.TextAreaBase(c.Key("line"), &line.text).FontSize(size).Grow(1).Lines(1, 50).Placeholder("输入内容").
			FocusBind(&a.outlineFocus, n.ID).Bind(a.outlineRowHandle(n.ID + "/text"))
		if a.outlineFocus == n.ID {
			in.TrackLines(&a.outlineLines)
		}
		if depth == 0 {
			in.FontWeight(600)
		}
		if n.Task != nil && n.Task.Status == model.StatusDone {
			in.TextColor(t.TextMuted)
		}
		if in.Focused() {
			a.selected = n.ID
		}
		prev, next := "", ""
		if i > 0 {
			prev = rows[i-1].node.ID
		} else {
			prev = a.doc.Root.ID
		}
		if i+1 < len(rows) {
			next = rows[i+1].node.ID
		}
		// The area takes its keys before shortcuts: its key handler, which
		// comes first, does the outline's. Enter adds a line, Backspace on
		// an empty one deletes it, Tab indents, and Up and Down go to the
		// line before or after from the first or last row of a wrapped one.
		composing := in.Composing()
		caret, _ := in.TextSelection()
		wrapped := a.outlineLines.Rows(0)
		first := a.outlineFocus != n.ID || len(wrapped) < 2 || caret < wrapped[1]
		last := a.outlineFocus != n.ID || len(wrapped) < 2 || caret >= wrapped[len(wrapped)-1]
		in.HandleInput(func(ev ui.InputEvent) bool {
			switch {
			case ev.Kind == ui.InputPointerMove && ev.Button == 0:
				// A drag from this line into others selects them whole;
				// back in it, the text it selects in the line.
				at := a.outlineRowAt(c, rows, a.outlineRowHandle(n.ID+"/text").Bounds(c).Y+ev.Y)
				switch {
				case at != n.ID:
					a.outlineSel = outlineSel{anchor: n.ID, head: at}
				case a.outlineSel.anchor == n.ID:
					a.outlineSel = outlineSel{}
				}
				return false
			case ev.Kind == ui.InputPointerDown:
				a.outlineSel = outlineSel{} // a press in a line lets lines go
				return false
			case ev.Kind == ui.InputPointerUp && a.outlineSel.anchor != "":
				a.selectOutlineLines()
				return false
			}
			if ev.Kind != ui.InputKeyDown || composing {
				return false
			}
			// Shift with Up or Down past the line's first or last row
			// selects whole lines.
			switch {
			case ev.Mods == ui.Shift && ev.Key == ui.KeyUp && first,
				ev.Mods == ui.Shift && ev.Key == ui.KeyDown && last:
				a.extendOutlineSel(rows, i, 0)
				a.selectOutlineLines()
				return true
			}
			switch {
			case ev.Mods == 0 && ev.Key == ui.KeyBackspace && n.Title == "" && len(n.Children) == 0:
				a.mutate("", func() { _, _ = a.doc.Delete(n.ID) })
				a.outlineFocus = prev
			case ev.Mods&^ui.Shift == 0 && ev.Key == ui.KeyEnter:
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
			case ev.Mods == 0 && ev.Key == ui.KeyUp && first:
				a.outlineFocus = prev
			case ev.Mods == 0 && ev.Key == ui.KeyDown && last && next != "":
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

// outlineSel is the lines selected whole: from the anchor, where it
// started, to the head, which Shift with the arrows moves.
type outlineSel struct{ anchor, head string }

// outlineSelRange is the indexes of the first and last rows selected, or
// -1 and -2 when none is.
func (a *app) outlineSelRange(rows []outlineRow) (lo, hi int) {
	if a.outlineSel.anchor == "" {
		return -1, -2
	}
	ia, ih := -1, -1
	for i, r := range rows {
		if r.node.ID == a.outlineSel.anchor {
			ia = i
		}
		if r.node.ID == a.outlineSel.head {
			ih = i
		}
	}
	if ia < 0 || ih < 0 { // gone, or hidden by a fold
		a.outlineSel = outlineSel{}
		return -1, -2
	}
	return min(ia, ih), max(ia, ih)
}

// extendOutlineSel moves the head of the selection a row up (dir -1) or
// down (1); without one, it selects the line at row i.
func (a *app) extendOutlineSel(rows []outlineRow, i, dir int) {
	if a.outlineSel.anchor == "" || dir == 0 {
		id := rows[i].node.ID
		a.outlineSel = outlineSel{anchor: id, head: id}
		return
	}
	for j, r := range rows {
		if r.node.ID == a.outlineSel.head {
			a.outlineSel.head = rows[min(max(j+dir, 0), len(rows)-1)].node.ID
			return
		}
	}
}

// selectOutlineLines gives the keys to the lines selected.
func (a *app) selectOutlineLines() {
	a.outlineFocus = ""
	a.outlineSelKeys.Focus()
}

// endOutlineSel lets the lines selected go, and goes on in the line at
// the head of the selection.
func (a *app) endOutlineSel() {
	head := a.outlineSel.head
	a.outlineSel = outlineSel{}
	if head != "" {
		a.outlineFocus = head
	}
}

// outlineSelKey takes the keys while lines are selected: Shift with Up or
// Down moves the end of the selection, Cmd+C copies them, Cmd+X cuts them,
// Backspace deletes them, and Escape, the arrows or a key typed let them
// go, back in a line.
func (a *app) outlineSelKey(rows []outlineRow, ev ui.InputEvent) bool {
	if a.outlineSel.anchor == "" {
		return false
	}
	if ev.Kind == ui.InputCommand { // the Edit menu's
		switch ev.Text {
		case "copy":
			a.copyOutlineSel(rows)
		case "cut":
			a.copyOutlineSel(rows)
			a.later(func() { a.deleteOutlineSel(rows) })
		case "delete":
			a.later(func() { a.deleteOutlineSel(rows) })
		default:
			return false
		}
		return true
	}
	if ev.Kind == ui.InputText {
		a.endOutlineSel()
		return true
	}
	if ev.Kind != ui.InputKeyDown {
		return false
	}
	switch {
	case ev.Mods == ui.Shift && ev.Key == ui.KeyUp:
		a.extendOutlineSel(rows, 0, -1)
	case ev.Mods == ui.Shift && ev.Key == ui.KeyDown:
		a.extendOutlineSel(rows, 0, 1)
	case ev.Mods == ui.Cmd && ev.Key == ui.KeyC:
		a.copyOutlineSel(rows)
	case ev.Mods == ui.Cmd && ev.Key == ui.KeyX:
		a.copyOutlineSel(rows)
		a.later(func() { a.deleteOutlineSel(rows) })
	case ev.Mods == 0 && (ev.Key == ui.KeyBackspace || ev.Key == ui.KeyDelete):
		a.later(func() { a.deleteOutlineSel(rows) })
	case ev.Key == ui.KeyUnknown:
		return false
	default:
		a.endOutlineSel()
	}
	return true
}

// outlineRowHandle is the handle of a line's row.
func (a *app) outlineRowHandle(id string) *ui.Handle {
	if a.outlineRowH == nil {
		a.outlineRowH = map[string]*ui.Handle{}
	}
	h := a.outlineRowH[id]
	if h == nil {
		h = new(ui.Handle)
		a.outlineRowH[id] = h
	}
	return h
}

// outlineRowAt is the line whose row is at the window's height y: the
// first above them all, the last below.
func (a *app) outlineRowAt(c *ui.Context, rows []outlineRow, y float32) string {
	if len(rows) == 0 {
		return ""
	}
	for _, r := range rows {
		if b := a.outlineRowHandle(r.node.ID).Bounds(c); y < b.Y+b.H {
			return r.node.ID
		}
	}
	return rows[len(rows)-1].node.ID
}

// outlineSelText is the lines selected as text, indented by two spaces a
// level from the least deep, with what a folded line holds: what the
// outline and the map take back as lines under lines when pasted.
func outlineSelText(rows []outlineRow) string {
	if len(rows) == 0 {
		return ""
	}
	top := rows[0].depth
	for _, r := range rows {
		top = min(top, r.depth)
	}
	var b strings.Builder
	var write func(n *model.Node, level int, folded bool)
	write = func(n *model.Node, level int, folded bool) {
		b.WriteString(strings.Repeat("  ", level) + n.Title + "\n")
		if folded {
			for _, ch := range n.Children {
				write(ch, level+1, true)
			}
		}
	}
	for _, r := range rows {
		write(r.node, r.depth-top, r.node.Collapsed)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func (a *app) copyOutlineSel(rows []outlineRow) {
	lo, hi := a.outlineSelRange(rows)
	if lo < 0 {
		return
	}
	a.plat.CopyText(outlineSelText(rows[lo : hi+1]))
	a.tell(fmt.Sprintf("已复制 %d 行", hi-lo+1))
}

// deleteOutlineSel deletes the lines selected, with the lines under them,
// as one step, and goes on in the line before them.
func (a *app) deleteOutlineSel(rows []outlineRow) {
	lo, hi := a.outlineSelRange(rows)
	if lo < 0 {
		return
	}
	a.mutate("", func() {
		for _, r := range rows[lo : hi+1] {
			_, _ = a.doc.Delete(r.node.ID) // gone already with its parent: nothing
		}
	})
	a.outlineSel = outlineSel{}
	a.outlineFocus = a.doc.Root.ID
	if lo > 0 {
		a.outlineFocus = rows[lo-1].node.ID
	}
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
