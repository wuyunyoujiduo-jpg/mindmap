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
	node    *model.Node
	depth   int
	summary bool // the topic of its parent's summary
	branch  int  // of a main topic: which of them it is, for its color
}

// outlineRows lists the nodes below the root that the outline shows, in
// order: those under a folded node are left out. A summary's topic comes
// after the children it sums up, at their depth.
func outlineRows(d *model.Document) []outlineRow {
	var rows []outlineRow
	var walk func(n *model.Node, depth int)
	walk = func(n *model.Node, depth int) {
		for i, c := range n.Children {
			rows = append(rows, outlineRow{node: c, depth: depth, branch: i})
			if !c.Collapsed {
				walk(c, depth+1)
			}
		}
		if t := n.SummaryTopic(); t != nil && len(n.Children) > 0 {
			rows = append(rows, outlineRow{node: t, depth: depth, summary: true})
			if !t.Collapsed {
				walk(t, depth+1)
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
//
// It is a list (ui.List), which builds the lines in and near the view and
// no others: built every one at every frame, an outline of five thousand
// lines took an eighth of a second a frame. The heading is the list's
// first row. The line with the focus stays built when it is scrolled away
// (the list sees to that), and a line the focus is sent to that is not
// built is scrolled to, where the focus finds it.
func (a *app) outlineView(c *ui.Context) {
	rows := outlineRows(a.doc)
	if a.outlineBlock && (a.outlineIndex(rows, a.outlineAnchor) < 0 || a.outlineIndex(rows, a.outlineEnd) < 0) {
		a.outlineBlock = false // lines deleted, or folded away
	}
	// Where the lines were drawn: those built in the last frame, and no
	// others.
	was := a.outlineRowAt
	a.outlineRowAt = make(map[string]ui.Rect, len(was))
	st := a.outlineList(a.doc.ID)
	if f := a.outlineFocus; f != a.outlineFocusWas {
		a.outlineFocusWas = f
		if _, built := was[f]; !built && f != "" && f != outlineBlockFocus {
			if i := a.outlineIndex(rows, f); i >= 0 {
				st.ScrollIntoView(i + 1)
			} else if f == a.doc.Root.ID {
				st.ScrollIntoView(0)
			}
		}
	}
	root := a.doc.Root
	list := ui.List(c.Key("outline:"+a.doc.ID), st, len(rows)+1).Grow(1).Padding(32, 0).Gap(4).ObserveInput(a.stepEnds).
		ItemKey(func(i int) any {
			if i == 0 {
				return root.ID
			}
			return rows[i-1].node.ID
		}).
		FocusBind(&a.outlineFocus, outlineBlockFocus).Label("大纲")
	a.outlineBlockKeys(list, rows)
	// Where the pointer is, in the window, which a line dragged from
	// selects to: the list sees it move over the other lines.
	if _, y, ok := list.PointerPosition(); ok {
		a.outlineDragY = list.Bounds().Y + y
	}
	a.outlineDragSeen = false
	list.Rows(func(r ui.ListRow) {
		c := r.Context
		// The lines are a column in the page's middle, as wide as reads
		// well, each row its part of it.
		ui.Box(c).FillWidth().Children(func() {
			ui.Column(c).Padding(0, 40).Gap(4).MaxWidth(860).Margin(0, ui.Auto).FillWidth().Children(func() {
				if r.Index == 0 {
					a.outlineHead(c, len(rows) == 0)
					return
				}
				a.outlineLine(c.Key(rows[r.Index-1].node.ID), rows, r.Index-1)
			})
		})
	})
	a.outlineDrag(c, rows)
}

// outlineList is the list of a canvas's outline: where it is scrolled to,
// each canvas its own.
func (a *app) outlineList(id string) *ui.ListState {
	if a.outlineLists == nil || len(a.outlineLists) > 64 && a.outlineLists[id] == nil { // of canvases long left
		a.outlineLists = map[string]*ui.ListState{}
	}
	if a.outlineLists[id] == nil {
		a.outlineLists[id] = &ui.ListState{}
	}
	return a.outlineLists[id]
}

// outlineHead is the outline's first row: the root as a heading, and,
// with no line under it, how to write the first.
func (a *app) outlineHead(c *ui.Context, empty bool) {
	t := c.Theme()
	root := a.doc.Root
	title := root.Title
	head := ui.Box(c).Padding(0, 0, 4)
	head.Children(func() {
		in := ui.TextInputBase(c.Key("outline-root"), &title).FontSize(26).FontWeight(700).LetterSpacing(-0.7).FillWidth().
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
	if empty {
		ui.Text(c, "在标题上按回车，开始写第一行").TextColor(t.TextMuted).Margin(8, 14)
	}
}

// outlineDrag follows a drag from a line: onto others it selects the
// lines whole; back on the line, its text is selected again. It is seen to
// once the lines are built, by where they are.
//
// The line it began on is kept by its topic (outlineDragFrom), not by its
// row: scrolled out of view, by the wheel or as the pointer nears the
// list's edge, the row is built no more, and the drag ended there, the
// selection no longer growing. While it is not built, the press lasts
// till the pointer is let go (outlineLetGo).
func (a *app) outlineDrag(c *ui.Context, rows []outlineRow) {
	from := a.outlineDragFrom
	if from == "" {
		return
	}
	if !a.outlineDragSeen {
		a.outlinePressed = !a.outlineLetGo && a.outlineIndex(rows, from) >= 0
	}
	// Up to where it was let go: the pointer's place may come a frame
	// after the press it moves with.
	block := a.outlineBlock
	if over := a.outlineRowUnder(a.outlineDragY); over != "" && over != from {
		a.outlineSelect(rows, from, over)
	} else if over == from && a.outlineBlock {
		a.outlineBlock, a.multi, a.selected = false, nil, from
	}
	if !a.outlinePressed {
		a.outlineDragFrom, a.outlineLetGo = "", false
		if a.outlineBlock {
			a.outlineFocus = outlineBlockFocus // the drag ended: the lines take the keys
		}
	}
	if a.outlineBlock != block {
		c.Invalidate() // the lines are drawn again, as selected or not
	}
}

// outlineBuilt counts the lines built, for the tests.
var outlineBuilt int

func (a *app) outlineLine(c *ui.Context, rows []outlineRow, i int) {
	outlineBuilt++
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
			case rows[i].summary: // a brace, as the map draws it
				ui.Text(c, "}").FontSize(15).Bold().TextColor(t.Accent)
			case len(n.Children) > 0 && n.Collapsed:
				ui.Text(c, "▸").FontSize(13).TextColor(t.TextMuted)
			case len(n.Children) > 0:
				ui.Text(c, "▾").FontSize(13).TextColor(t.TextMuted)
			case depth == 0: // a main topic: a dot of its branch's color
				ui.Box(c).Size(7, 7).Radius(999).Background(lookOf(a.doc.Style).branch(rows[i].branch))
			default:
				ui.Box(c).Size(5, 5).Radius(999).Background(t.TextMuted.Alpha(0.6))
			}
		})
		if rows[i].summary {
			pill(c, "概要", t.Accent.Alpha(0.14), t.Accent)
		}
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
		size := float32(15)
		if depth == 0 {
			size = 16
		}
		// A long line wraps: the field grows to show it all.
		in := ui.TextAreaBase(c.Key("line"), &title).Lines(1, 30).FontSize(size).Grow(1).Shrink(1).MinWidth(40).
			Placeholder("输入内容").Label("大纲 "+n.Title).FocusBind(&a.outlineFocus, n.ID)
		if depth == 0 {
			in.FontWeight(700)
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
			a.titleTagOffer(c, in, "outline:"+n.ID, title, func(part, tag string) { a.takeFieldTag(n, part, tag) })
		}
		a.fieldTitleTags(in, "outline:"+n.ID, n, title, false)
		// Its tags after it, each taken off by a click, as on the canvas.
		for _, tag := range n.Tags {
			chip := tagChip(c.Key("tag:"+tag), tag, tag+" ✕", false, 12).KeepFocus().Shrink(0).
				Label("去掉「" + n.Title + "」的标签 " + tag).Tooltip("点一下去掉这个标签")
			if chip.Clicked() {
				tg := tag
				a.later(func() { a.toggleTag(n, tg) })
			}
		}
		// A drag from the line selects lines whole (outlineDrag): pressed
		// as it begins, held wherever the pointer goes after.
		switch button, held := in.PressedButton(); {
		case in.Pressed() && a.outlineDragFrom != n.ID:
			a.outlineDragFrom, a.outlineLetGo = n.ID, false
			a.outlineDragSeen, a.outlinePressed = true, true
		case n.ID == a.outlineDragFrom:
			a.outlineDragSeen, a.outlinePressed = true, held && button == 0
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
			case ev.Mods == 0 && ev.Key == ui.KeyTab && a.outlineTagTab(n):
				// A #word begun: Tab makes it a tag, not the line indented.
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

// outlineRowUnder is the line at a height of the window, of those built,
// or the first or last of them when it is above or below them all.
func (a *app) outlineRowUnder(y float32) string {
	var first, last string
	var top, bottom float32
	for id, b := range a.outlineRowAt {
		if b.W <= 0 || b.H <= 0 {
			continue // not laid out yet
		}
		if y >= b.Y && y < b.Y+b.H+4 { // the gap below a line is its
			return id
		}
		if first == "" || b.Y < top {
			first, top = id, b.Y
		}
		if last == "" || b.Y+b.H > bottom {
			last, bottom = id, b.Y+b.H
		}
	}
	switch {
	case first != "" && y < top:
		return first
	case last != "" && y >= bottom:
		return last
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
		// As the canvas pastes them: topics cut by their own IDs, once,
		// and what was drawn of them with them. (Here every paste made
		// copies, and a branch cut and pasted lost its 图解.)
		var carry func()
		nodes, carry = a.clipNodes()
		defer carry()
	} else {
		nodes = model.ParseOutline(text)
	}
	if len(nodes) == 0 {
		return
	}
	under := a.doc.IsSummaryTopic(n.ID) // a summary's topic has no siblings: its lines go under it
	a.mutate("", func() {
		if under {
			n.Children = append(n.Children, nodes...)
			n.Collapsed = false
			return
		}
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

// branchColor is the color of the branch a node is on, as the map draws it.
func (a *app) branchColor(id string) ui.Color {
	lk := lookOf(a.doc.Style)
	if p := a.doc.Path(id); len(p) > 1 {
		for i, ch := range p[0].Children {
			if ch == p[1] {
				return lk.branch(i)
			}
		}
	}
	return lk.branch(0)
}
