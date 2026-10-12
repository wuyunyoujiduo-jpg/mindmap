package main

import (
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
	ui.Row(c).Padding(3, 0, 3, float32(depth)*26).Gap(8).AlignItems(ui.Start).Children(func() {
		ui.Row(c).Height(rowH).Gap(8).AlignItems(ui.Center).Shrink(0).Children(func() {
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
			FocusBind(&a.outlineFocus, n.ID)
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
			if ev.Kind != ui.InputKeyDown || composing {
				return false
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
