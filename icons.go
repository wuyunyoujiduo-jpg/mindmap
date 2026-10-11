package main

import (
	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// Icons drawn as lines rather than taken from a font, whose glyphs sit
// off center and differ from machine to machine: centered in their box,
// in the stroke of the text around them.

type icon int

const (
	iconMinus icon = iota
	iconPlus
	iconTarget   // back to the center
	iconDownload // export
	iconPlay
	iconUndo
	iconRedo
	iconCheck
	iconPencil
)

// drawIcon draws an icon in the square of side s centered in r.
func drawIcon(p *ui.Painter, r ui.Rect, s float32, ic icon, col ui.Color) {
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	h := s / 2
	w := max(s/9, 1.3) // the stroke
	switch ic {
	case iconMinus:
		p.Line(cx-h*0.7, cy, cx+h*0.7, cy, w, col)
	case iconPlus:
		p.Line(cx-h*0.7, cy, cx+h*0.7, cy, w, col)
		p.Line(cx, cy-h*0.7, cx, cy+h*0.7, w, col)
	case iconTarget:
		p.StrokePath(new(ui.Path).Circle(cx, cy, h*0.62), w, col)
		p.FillPath(new(ui.Path).Circle(cx, cy, h*0.2), col)
		for _, d := range [][2]float32{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			p.Line(cx+d[0]*h*0.62, cy+d[1]*h*0.62, cx+d[0]*h*0.95, cy+d[1]*h*0.95, w, col)
		}
	case iconDownload:
		p.Line(cx, cy-h*0.8, cx, cy+h*0.25, w, col)
		arrow := new(ui.Path).MoveTo(cx-h*0.42, cy-h*0.12).LineTo(cx, cy+h*0.3).LineTo(cx+h*0.42, cy-h*0.12)
		p.StrokePath(arrow, w, col)
		tray := new(ui.Path).MoveTo(cx-h*0.8, cy+h*0.35).LineTo(cx-h*0.8, cy+h*0.78).LineTo(cx+h*0.8, cy+h*0.78).LineTo(cx+h*0.8, cy+h*0.35)
		p.StrokePath(tray, w, col)
	case iconPlay:
		tri := new(ui.Path).MoveTo(cx-h*0.45, cy-h*0.7).LineTo(cx+h*0.7, cy).LineTo(cx-h*0.45, cy+h*0.7).Close()
		p.FillPath(tri, col)
	case iconUndo, iconRedo:
		k := float32(1)
		if ic == iconRedo {
			k = -1
		}
		// A hook: back along the top, then round and down.
		arc := new(ui.Path).MoveTo(cx-k*h*0.6, cy-h*0.25).LineTo(cx+k*h*0.2, cy-h*0.25).
			CubeTo(cx+k*h*0.95, cy-h*0.25, cx+k*h*0.95, cy+h*0.7, cx+k*h*0.2, cy+h*0.7).LineTo(cx-k*h*0.2, cy+h*0.7)
		p.StrokePath(arc, w, col)
		head := new(ui.Path).MoveTo(cx-k*h*0.25, cy-h*0.62).LineTo(cx-k*h*0.65, cy-h*0.25).LineTo(cx-k*h*0.25, cy+h*0.12)
		p.StrokePath(head, w, col)
	case iconCheck:
		check := new(ui.Path).MoveTo(cx-h*0.55, cy+h*0.02).LineTo(cx-h*0.15, cy+h*0.42).LineTo(cx+h*0.6, cy-h*0.4)
		p.StrokePath(check, w*1.15, col)
	case iconPencil:
		body := new(ui.Path).MoveTo(cx-h*0.7, cy+h*0.7).LineTo(cx-h*0.6, cy+h*0.3).LineTo(cx+h*0.35, cy-h*0.65).
			LineTo(cx+h*0.7, cy-h*0.3).LineTo(cx-h*0.25, cy+h*0.65).Close()
		p.StrokePath(body, w*0.9, col)
		p.Line(cx+h*0.15, cy-h*0.45, cx+h*0.5, cy-h*0.1, w*0.9, col)
	}
}

// iconButton is a button showing an icon, centered, with a tooltip; a
// button not enabled is dimmed, and its clicks are to be ignored.
func iconButton(c *ui.Context, ic icon, label, tip string, size float32, enabled bool) ui.Element {
	t := c.Theme()
	b := ui.Box(c.Key(label)).Size(size, size).Radius(6).Label(label).Tooltip(tip).Shrink(0)
	col := t.Text
	switch {
	case !enabled:
		col = t.Text.Alpha(0.3)
	case b.Pressed():
		b.Background(t.Text.Alpha(0.12))
	case b.Hovered():
		b.Background(t.SurfaceHover)
	}
	if enabled {
		b.Cursor(ui.CursorPointer)
	}
	b.Draw(func(p *ui.Painter, r ui.Rect) { drawIcon(p, r, size*0.5, ic, col) })
	return b
}

// taskCheck is a task's check box: empty in the color of its status, half
// filled while in progress, dashed while an idea, checked when done. A
// click marks it done, or not done again.
func (a *app) taskCheck(c *ui.Context, n *model.Node, size float32) ui.Element {
	t := c.Theme()
	task := n.Task
	done := task.Status == model.StatusDone
	tip := a.taskSummary(task) + "\n点击标为完成"
	label := "完成任务 " + n.Title
	if done {
		tip, label = a.taskSummary(task)+"\n点击标为未完成", "取消完成 "+n.Title
	}
	b := ui.Box(c.Key("task-check")).Size(size, size).Shrink(0).Cursor(ui.CursorPointer).Label(label).Tooltip(tip)
	hover := b.Hovered()
	if b.Clicked() {
		id := n.ID
		a.later(func() {
			if node := a.file.Node(id); node != nil && node.Task != nil {
				if node.Task.Status == model.StatusDone {
					a.setStatus(node, model.StatusTodo)
				} else {
					a.setStatus(node, model.StatusDone)
				}
			}
		})
	}
	col := statusColor(task.Status)
	b.Draw(func(p *ui.Painter, r ui.Rect) {
		rad := size * 0.26
		w := max(size/9, 1.2)
		box := roundedRect(r.X+w/2, r.Y+w/2, r.W-w, r.H-w, rad)
		switch task.Status {
		case model.StatusDone:
			p.FillPath(box, col)
			drawIcon(p, r, size*0.78, iconCheck, ui.Hex("#ffffff"))
			return
		case model.StatusDoing:
			p.FillPath(box, t.Background)
			half := roundedRect(r.X+w/2, r.Y+w/2, (r.W-w)/2, r.H-w, rad)
			p.FillPath(half, col.Alpha(0.85))
			p.StrokePath(box, w, col)
		case model.StatusIdea:
			p.FillPath(box, t.Background)
			p.StrokePath(box, w, col.Alpha(0.55))
		default:
			p.FillPath(box, t.Background)
			p.StrokePath(box, w, col)
		}
		if hover { // a hint of the check a click makes
			drawIcon(p, r, size*0.78, iconCheck, col.Alpha(0.5))
		}
	})
	return b
}

// roundedRect is the path of a rectangle with rounded corners.
func roundedRect(x, y, w, h, r float32) *ui.Path {
	r = min(r, w/2, h/2)
	return new(ui.Path).MoveTo(x+r, y).LineTo(x+w-r, y).QuadTo(x+w, y, x+w, y+r).
		LineTo(x+w, y+h-r).QuadTo(x+w, y+h, x+w-r, y+h).LineTo(x+r, y+h).QuadTo(x, y+h, x, y+h-r).
		LineTo(x, y+r).QuadTo(x, y, x+r, y).Close()
}

// noteMark is the mark of a node with a note: hovering shows the note,
// and a click opens it to edit, in a popover by the node.
func (a *app) noteMark(c *ui.Context, n *model.Node, size float32, col ui.Color) {
	t := c.Theme()
	mark := ui.Box(c.Key("note-mark")).Size(size, size).Shrink(0).Radius(4).Cursor(ui.CursorPointer).Label("编辑备注")
	hover := mark.Hovered()
	if hover {
		mark.Background(col.Alpha(0.15))
	}
	mark.Draw(func(p *ui.Painter, r ui.Rect) { drawIcon(p, r, size*0.8, iconPencil, col) })
	if mark.Clicked() {
		a.noteEditing = n.ID
	}
	peek := hover && a.noteEditing != n.ID
	if peek {
		ui.PopoverBase(c.Key("note-peek"), mark, &peek, func(panel ui.Element) {
			panel.Padding(12, 14).Radius(10).Background(t.Surface).Border(1, t.Border).Shadow(0, 6, 20, 0, ui.RGBA(0, 0, 0, 0.18)).
				Width(320).PassThrough().Children(func() {
				ui.Text(c, "备注 · 点击编辑").FontSize(11).TextColor(t.TextMuted)
				note := n.Note
				if r := []rune(note); len(r) > 600 {
					note = string(r[:600]) + "…"
				}
				markdownView(c.Key("peek"), note, 13)
			})
		})
	}
}

// notePopover edits a node's note by the node, each change a step of the
// history, as in the panel.
func (a *app) notePopover(c *ui.Context, anchor ui.Element, n *model.Node) {
	t := c.Theme()
	open := true
	ui.PopoverBase(c.Key("note-edit"), anchor, &open, func(panel ui.Element) {
		panel.Padding(12).Radius(10).Background(t.Surface).Border(1, t.Border).Shadow(0, 8, 28, 0, ui.RGBA(0, 0, 0, 0.22)).
			Width(380).Gap(8).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "备注 · "+n.Title).FontSize(12).Bold().NoWrap().Ellipsis("…").Grow(1).Shrink(1).MinWidth(0)
				ui.Text(c, "支持 Markdown").FontSize(11).TextColor(t.TextMuted)
			})
			note := n.Note
			in := ui.TextArea(c.Key("note-text"), &note).Lines(4, 14).AutoFocus().Label("备注内容").
				Placeholder("实现要点、决策、注意事项…")
			if in.Changed() {
				a.mutate("note:"+n.ID, func() { n.Note = note })
			}
			if in.Shortcut(0, ui.KeyEscape) || in.Shortcut(ui.Cmd, ui.KeyEnter) {
				open = false
			}
			ui.Row(c).Children(func() {
				ui.Text(c, "⌘↩ 或 Esc 完成").FontSize(11).TextColor(t.TextMuted).Grow(1)
				if ui.PrimaryButton(c, "完成").Clicked() {
					open = false
				}
			})
		})
	})
	if !open {
		a.noteEditing = ""
	}
}
