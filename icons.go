package main

import (
	"math"

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
	iconClose
	iconStar
	iconStarFilled
	iconMore
	iconSparkle
	iconShare
	iconFolder
	iconLibrary
	iconInbox
	iconMap
	iconSearch
	iconSend     // a paper plane
	iconText     // a clip of text: lines on a page
	iconLink     // two links of a chain
	iconImage    // a picture: a frame, a hill and the sun
	iconFile     // a page with its corner folded
	iconShot     // a screenshot: the corners of a frame
	iconCloud    // the sync with GitHub
	iconClip     // a paperclip: attach files
	iconCategory // a category: four squares
	iconTag      // a tag: a label with a hole
	iconLevels   // levels shown: an outline, each row further in
	iconSidebar  // the list beside: a frame, its left part marked off
	iconPanel    // the panel beside: a frame, its right part marked off
	iconBack     // back: a chevron to the left
	iconForward  // on: a chevron to the right
	iconGear     // settings: a wheel with teeth
	iconFilter   // what shows is narrowed: three lines, each shorter
)

// glyph is an icon drawn in a box of the given side, as a piece of text.
func glyph(c *ui.Context, ic icon, size float32, col ui.Color) ui.Element {
	return ui.Box(c).Size(size, size).Shrink(0).PassThrough().
		Draw(func(p *ui.Painter, r ui.Rect) { drawIcon(p, r, size, ic, col) })
}

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
	case iconGear:
		p.StrokePath(new(ui.Path).Circle(cx, cy, h*0.48), w, col)
		p.StrokePath(new(ui.Path).Circle(cx, cy, h*0.17), w, col)
		for i := range 6 { // the teeth, short and apart
			a := float64(i)*math.Pi/3 + math.Pi/6
			dx, dy := float32(math.Cos(a)), float32(math.Sin(a))
			p.Line(cx+dx*h*0.48, cy+dy*h*0.48, cx+dx*h*0.78, cy+dy*h*0.78, w*1.2, col)
		}
	case iconBack, iconForward:
		k := float32(1)
		if ic == iconForward {
			k = -1
		}
		p.StrokePath(new(ui.Path).MoveTo(cx+k*h*0.25, cy-h*0.55).LineTo(cx-k*h*0.3, cy).LineTo(cx+k*h*0.25, cy+h*0.55), w*1.1, col)
	case iconSidebar, iconPanel:
		p.StrokePath(roundedRect(cx-h*0.9, cy-h*0.7, h*1.8, h*1.4, h*0.28), w, col)
		x := cx - h*0.3
		if ic == iconPanel {
			x = cx + h*0.3
		}
		p.Line(x, cy-h*0.7, x, cy+h*0.7, w, col)
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
	case iconClose:
		p.StrokePath(new(ui.Path).MoveTo(cx-h*0.55, cy-h*0.55).LineTo(cx+h*0.55, cy+h*0.55), w, col)
		p.StrokePath(new(ui.Path).MoveTo(cx-h*0.55, cy+h*0.55).LineTo(cx+h*0.55, cy-h*0.55), w, col)
	case iconStar, iconStarFilled:
		star := new(ui.Path)
		for i := range 10 {
			rr := h * 0.95
			if i%2 == 1 {
				rr = h * 0.42
			}
			a := float64(i)*math.Pi/5 - math.Pi/2
			x, y := cx+rr*float32(math.Cos(a)), cy+h*0.08+rr*float32(math.Sin(a))
			if i == 0 {
				star.MoveTo(x, y)
			} else {
				star.LineTo(x, y)
			}
		}
		star.Close()
		if ic == iconStarFilled {
			p.FillPath(star, col)
		} else {
			p.StrokePath(star, w*0.9, col)
		}
	case iconMore:
		for _, dx := range []float32{-0.6, 0, 0.6} {
			p.FillPath(new(ui.Path).Circle(cx+dx*h, cy, max(w*0.85, 1.4)), col)
		}
	case iconSparkle:
		// A four-pointed star with curved sides, and a small one.
		spark := func(x, y, r float32) *ui.Path {
			k := r * 0.18
			return new(ui.Path).MoveTo(x, y-r).QuadTo(x+k, y-k, x+r, y).QuadTo(x+k, y+k, x, y+r).
				QuadTo(x-k, y+k, x-r, y).QuadTo(x-k, y-k, x, y-r).Close()
		}
		p.FillPath(spark(cx-h*0.12, cy+h*0.1, h*0.82), col)
		p.FillPath(spark(cx+h*0.62, cy-h*0.6, h*0.32), col)
	case iconShare:
		// A card with an arrow going up out of it.
		p.Line(cx, cy-h*0.85, cx, cy+h*0.25, w, col)
		p.StrokePath(new(ui.Path).MoveTo(cx-h*0.38, cy-h*0.45).LineTo(cx, cy-h*0.85).LineTo(cx+h*0.38, cy-h*0.45), w, col)
		p.StrokePath(new(ui.Path).MoveTo(cx-h*0.3, cy-h*0.1).LineTo(cx-h*0.75, cy-h*0.1).LineTo(cx-h*0.75, cy+h*0.85).
			LineTo(cx+h*0.75, cy+h*0.85).LineTo(cx+h*0.75, cy-h*0.1).LineTo(cx+h*0.3, cy-h*0.1), w, col)
	case iconFolder: // a folder, as the Finder's: a tab, round corners
		x0, x1, tab, top, bot, r := cx-h*0.9, cx+h*0.9, cy-h*0.7, cy-h*0.42, cy+h*0.68, h*0.2
		f := new(ui.Path).MoveTo(x0, bot-r).LineTo(x0, tab+r).QuadTo(x0, tab, x0+r, tab).
			LineTo(cx-h*0.28, tab).QuadTo(cx-h*0.16, tab, cx-h*0.08, top).
			LineTo(x1-r, top).QuadTo(x1, top, x1, top+r).LineTo(x1, bot-r).QuadTo(x1, bot, x1-r, bot).
			LineTo(x0+r, bot).QuadTo(x0, bot, x0, bot-r).Close()
		p.StrokePath(f, w, col)
		p.Line(x0+w/2, top+h*0.16, x1-w/2, top+h*0.16, w*0.8, col)
	case iconLibrary: // books on a shelf
		p.StrokePath(roundedRect(cx-h*0.8, cy-h*0.7, h*0.42, h*1.4, h*0.08), w, col)
		p.StrokePath(roundedRect(cx-h*0.25, cy-h*0.7, h*0.42, h*1.4, h*0.08), w, col)
		p.StrokePath(new(ui.Path).MoveTo(cx+h*0.35, cy-h*0.55).LineTo(cx+h*0.68, cy-h*0.68).LineTo(cx+h*0.95, cy+h*0.62).LineTo(cx+h*0.62, cy+h*0.75).Close(), w, col)
	case iconInbox:
		p.StrokePath(roundedRect(cx-h*0.85, cy-h*0.65, h*1.7, h*1.3, h*0.2), w, col)
		p.StrokePath(new(ui.Path).MoveTo(cx-h*0.85, cy+h*0.1).LineTo(cx-h*0.35, cy+h*0.1).LineTo(cx-h*0.2, cy+h*0.35).
			LineTo(cx+h*0.2, cy+h*0.35).LineTo(cx+h*0.35, cy+h*0.1).LineTo(cx+h*0.85, cy+h*0.1), w, col)
	case iconFilter: // three lines, each shorter, about one middle
		for i, dy := range []float32{-0.6, 0, 0.6} {
			half := h * (0.85 - 0.3*float32(i))
			p.Line(cx-half, cy+dy*h, cx+half, cy+dy*h, w, col)
		}
	case iconLevels: // an outline, each row further in
		for i, dy := range []float32{-0.6, 0, 0.6} {
			x := cx - h*0.8 + float32(i)*h*0.4
			p.FillPath(new(ui.Path).Circle(x, cy+dy*h, h*0.13), col)
			p.Line(x+h*0.3, cy+dy*h, cx+h*0.85, cy+dy*h, w, col)
		}
	case iconMap: // a map's page: a rounded square, a small map in it
		p.StrokePath(roundedRect(cx-h*0.85, cy-h*0.85, h*1.7, h*1.7, h*0.38), w, col)
		p.FillPath(new(ui.Path).Circle(cx-h*0.33, cy, h*0.17), col)
		for _, dy := range []float32{-0.36, 0.36} {
			y := cy + dy*h
			p.StrokePath(new(ui.Path).MoveTo(cx-h*0.2, cy).CubeTo(cx, cy, cx-h*0.02, y, cx+h*0.18, y).
				LineTo(cx+h*0.42, y), w*0.85, col)
		}
	case iconText:
		p.StrokePath(roundedRect(cx-h*0.7, cy-h*0.85, h*1.4, h*1.7, h*0.22), w, col)
		for i, f := range []float32{-0.35, 0, 0.35} {
			end := cx + h*0.35
			if i == 2 {
				end = cx + h*0.05
			}
			p.StrokePath(new(ui.Path).MoveTo(cx-h*0.35, cy+f*h).LineTo(end, cy+f*h), w*0.9, col)
		}
	case iconLink:
		p.StrokePath(new(ui.Path).MoveTo(cx-h*0.05, cy-h*0.25).LineTo(cx-h*0.45, cy+h*0.15).
			CubeTo(cx-h*0.75, cy+h*0.45, cx-h*0.45, cy+h*0.95, cx-h*0.12, cy+h*0.62).LineTo(cx+h*0.05, cy+h*0.45), w, col)
		p.StrokePath(new(ui.Path).MoveTo(cx+h*0.05, cy+h*0.25).LineTo(cx+h*0.45, cy-h*0.15).
			CubeTo(cx+h*0.75, cy-h*0.45, cx+h*0.45, cy-h*0.95, cx+h*0.12, cy-h*0.62).LineTo(cx-h*0.05, cy-h*0.45), w, col)
		p.StrokePath(new(ui.Path).MoveTo(cx-h*0.22, cy+h*0.22).LineTo(cx+h*0.22, cy-h*0.22), w, col)
	case iconImage:
		p.StrokePath(roundedRect(cx-h*0.85, cy-h*0.7, h*1.7, h*1.4, h*0.22), w, col)
		p.FillPath(new(ui.Path).Circle(cx+h*0.35, cy-h*0.25, h*0.15), col)
		p.StrokePath(new(ui.Path).MoveTo(cx-h*0.85, cy+h*0.45).LineTo(cx-h*0.3, cy-h*0.05).LineTo(cx+h*0.15, cy+h*0.35).
			LineTo(cx+h*0.4, cy+h*0.15).LineTo(cx+h*0.85, cy+h*0.5), w, col)
	case iconFile:
		p.StrokePath(new(ui.Path).MoveTo(cx-h*0.65, cy-h*0.85).LineTo(cx+h*0.2, cy-h*0.85).LineTo(cx+h*0.65, cy-h*0.4).
			LineTo(cx+h*0.65, cy+h*0.85).LineTo(cx-h*0.65, cy+h*0.85).Close(), w, col)
		p.StrokePath(new(ui.Path).MoveTo(cx+h*0.2, cy-h*0.85).LineTo(cx+h*0.2, cy-h*0.4).LineTo(cx+h*0.65, cy-h*0.4), w*0.9, col)
	case iconTag:
		p.StrokePath(new(ui.Path).MoveTo(cx-h*0.75, cy-h*0.75).LineTo(cx+h*0.05, cy-h*0.75).LineTo(cx+h*0.8, cy).
			LineTo(cx+h*0.05, cy+h*0.75).LineTo(cx-h*0.75, cy+h*0.75).Close(), w, col)
		p.FillPath(new(ui.Path).Circle(cx-h*0.35, cy-h*0.35, h*0.14), col)
	case iconCategory:
		k := h * 0.62
		for _, d := range [][2]float32{{-1, -1}, {1, -1}, {-1, 1}, {1, 1}} {
			x, y := cx+d[0]*h*0.42, cy+d[1]*h*0.42
			p.StrokePath(roundedRect(x-k/2, y-k/2, k, k, k*0.25), w*0.9, col)
		}
	case iconClip:
		// Drawn upright and turned 40° to the right.
		sin, cos := float32(math.Sin(40*math.Pi/180)), float32(math.Cos(40*math.Pi/180))
		pt := func(x, y float32) (float32, float32) { return cx + h*(x*cos-y*sin), cy + h*(x*sin+y*cos) }
		path := new(ui.Path)
		path.MoveTo(pt(0.32, -0.25))
		path.LineTo(pt(0.32, 0.5))
		x1, y1 := pt(0.32, 0.95)
		x2, y2 := pt(-0.32, 0.95)
		x3, y3 := pt(-0.32, 0.5)
		path.CubeTo(x1, y1, x2, y2, x3, y3)
		path.LineTo(pt(-0.32, -0.55))
		x1, y1 = pt(-0.32, -0.95)
		x2, y2 = pt(0.12, -0.95)
		x3, y3 = pt(0.12, -0.55)
		path.CubeTo(x1, y1, x2, y2, x3, y3)
		path.LineTo(pt(0.12, 0.4))
		p.StrokePath(path, w, col)
	case iconCloud:
		by := cy + h*0.5
		p.StrokePath(new(ui.Path).MoveTo(cx-h*0.45, by).
			CubeTo(cx-h*1.0, by, cx-h*1.0, cy-h*0.2, cx-h*0.5, cy-h*0.12).
			CubeTo(cx-h*0.42, cy-h*0.8, cx+h*0.38, cy-h*0.8, cx+h*0.42, cy-h*0.18).
			CubeTo(cx+h*1.0, cy-h*0.18, cx+h*1.0, by, cx+h*0.45, by).Close(), w, col)
	case iconShot:
		k := h * 0.4
		for _, c := range [][4]float32{{-1, -1, 1, 1}, {1, -1, -1, 1}, {-1, 1, 1, -1}, {1, 1, -1, -1}} {
			x, y := cx+c[0]*h*0.8, cy+c[1]*h*0.8
			p.StrokePath(new(ui.Path).MoveTo(x+c[2]*k, y).LineTo(x, y).LineTo(x, y+c[3]*k), w, col)
		}
		p.FillPath(new(ui.Path).Circle(cx, cy, h*0.18), col)
	case iconSend:
		p.FillPath(new(ui.Path).MoveTo(cx-h*0.8, cy-h*0.7).LineTo(cx+h*0.85, cy).LineTo(cx-h*0.8, cy+h*0.7).
			LineTo(cx-h*0.5, cy+h*0.08).LineTo(cx+h*0.15, cy).LineTo(cx-h*0.5, cy-h*0.08).Close(), col)
	case iconSearch:
		p.StrokePath(new(ui.Path).Circle(cx-h*0.15, cy-h*0.15, h*0.55), w, col)
		p.StrokePath(new(ui.Path).MoveTo(cx+h*0.25, cy+h*0.25).LineTo(cx+h*0.8, cy+h*0.8), w*1.1, col)
	case iconPencil:
		body := new(ui.Path).MoveTo(cx-h*0.7, cy+h*0.7).LineTo(cx-h*0.6, cy+h*0.3).LineTo(cx+h*0.35, cy-h*0.65).
			LineTo(cx+h*0.7, cy-h*0.3).LineTo(cx-h*0.25, cy+h*0.65).Close()
		p.StrokePath(body, w*0.9, col)
		p.StrokePath(new(ui.Path).MoveTo(cx+h*0.15, cy-h*0.45).LineTo(cx+h*0.5, cy-h*0.1), w*0.9, col)
	}
}

// iconButton is a button showing an icon, centered, with a tooltip; a
// button not enabled is dimmed, and its clicks are to be ignored.
func iconButton(c *ui.Context, ic icon, label, tip string, size float32, enabled bool) ui.Element {
	t := c.Theme()
	// Round, as the keys of words are pills; it lights up under the
	// pointer and gives when pressed.
	b := ui.Box(c.Key(label)).Size(size, size).Radius(radiusPill).Label(label).Tooltip(tip).Shrink(0).
		Background(t.Text.Alpha(0)).Transition(easeLook)
	col := t.Text
	switch {
	case !enabled:
		col = t.Text.Alpha(0.3)
	case b.Pressed():
		b.Background(t.Text.Alpha(0.12))
	case b.Hovered():
		b.Background(t.Text.Alpha(0.07))
	}
	if enabled {
		b.Cursor(ui.CursorPointer)
		press(b, 0.92)
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
	b := ui.Box(c.Key("task-check")).Size(size, size).Shrink(0).Cursor(ui.CursorPointer).Label(label)
	if a.mode != viewMap || a.libOpen {
		// On the canvas, the tip's dark box would sit on the topic's, the
		// central one's as dark: the panel says the state there.
		b.Tooltip(tip)
	}
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
	b.Draw(func(p *ui.Painter, r ui.Rect) {
		// The status's progress marker, as the markers draw it.
		p.Fill(r, t.Background, r.W/2)
		drawMarker(p, r, statusMarker(task.Status))
		if hover && task.Status != model.StatusDone { // a hint of the check a click makes
			drawIcon(p, r, size*0.7, iconCheck, ui.Hex("#4FA374").Alpha(0.6))
		}
	})
	return b
}

// statusMarker is the progress marker a status shows as: the empty circle
// for an idea, the start's for one to do, a sixth for one under way, the
// check for one done.
func statusMarker(s model.Status) string {
	switch s {
	case model.StatusIdea:
		return "task:idea"
	case model.StatusDoing:
		return "task:1"
	case model.StatusDone:
		return "task:6"
	}
	return "task:0"
}

// statusIcon shows a status as its marker.
func statusIcon(c *ui.Context, s model.Status, size float32) ui.Element {
	return ui.Box(c).Size(size, size).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) { drawMarker(p, r, statusMarker(s)) })
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
	// A few lines, lightly: it says there is a note, no more.
	mark.Draw(func(p *ui.Painter, r ui.Rect) {
		ink, w := col.Alpha(0.7), max(size/11, 1)
		for i, k := range []float32{0.62, 0.62, 0.36} {
			y := r.Y + r.H*(0.3+0.2*float32(i))
			p.Line(r.X+r.W*0.19, y, r.X+r.W*(0.19+k), y, w, ink)
		}
	})
	if mark.Clicked() {
		a.noteEditing = n.ID
	}
	peek := hover && a.noteEditing != n.ID
	if peek {
		ui.PopoverBase(c.Key("note-peek"), mark, &peek, func(panel ui.Element) {
			lifted(panel.Padding(14, 16).Radius(14).Background(cardColor(t)), t).
				Width(320).PassThrough().Children(func() {
				ui.Text(c, "备注 · 点击编辑").FontSize(11).TextColor(t.TextMuted)
				note := notePictureMark.ReplaceAllString(n.Note, "🖼")
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
		lifted(panel.Padding(14).Radius(14).Background(cardColor(t)), t).
			Width(380).Gap(8).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "备注 · "+n.Title).FontSize(12).Bold().SingleLine().Grow(1).Shrink(1).MinWidth(0)
				ui.Text(c, "# 标题  - 列表  **粗体**").FontSize(11).TextColor(t.TextMuted).Margin(0, 6, 0, 0)
				a.notePictureButton(c, "popover", n)
			})
			in := a.noteEditor(c, "popover", n, 14, "实现要点、决策、注意事项…").AutoFocus().MinHeight(90).MaxHeight(360)
			if in.Shortcut(0, ui.KeyEscape) || in.Shortcut(ui.Cmd, ui.KeyEnter) {
				open = false
			}
			ui.Row(c).Children(func() {
				ui.Text(c, "⌘↩ 或 Esc 完成").FontSize(11).TextColor(t.TextMuted).Grow(1)
				if inkButton(c, "完成", true).Clicked() {
					open = false
				}
			})
		})
	})
	if !open {
		a.noteEditing = ""
	}
}

// iconTextButton is a button of an icon and a word, as the AI's.
func iconTextButton(c *ui.Context, ic icon, text, label, tip string, active bool) ui.Element {
	t := c.Theme()
	b := ui.Row(c.Key(label)).Height(keyHeight).Padding(0, 14, 0, 12).Gap(6).Radius(radiusPill).AlignItems(ui.Center).Cursor(ui.CursorPointer).
		Label(label).Tooltip(tip).Shrink(0)
	col, mark := t.Text, t.Text
	if ic == iconSparkle {
		mark = t.Accent // the AI's spark is the accent's wherever it is
	}
	if active {
		press(b.Background(t.Accent).Transition(easeLook), 0.96)
		col, mark = t.AccentText, t.AccentText
	} else {
		key(b, t)
	}
	b.Children(func() {
		ui.Box(c).Size(14, 14).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) { drawIcon(p, r, 13, ic, mark) })
		if text != "" {
			ui.Text(c, text).FontSize(13).TextColor(col).PassThrough()
		}
	})
	return b
}

// The rail's icons: one set drawn alike, in lines of one weight with
// round ends on a 24 grid, each with one part filled, so that they read
// as a family and stand at a small size.
const (
	railDesk = iota
	railMap
	railTasks
	railCollect
	railSearch
	railGear
)

// drawRailIcon draws an icon of the rail in the square of side s centered
// in r, in col; its filled part in col too, lightly.
func drawRailIcon(p *ui.Painter, r ui.Rect, s float32, kind int, col ui.Color) {
	k := s / 24
	ox, oy := r.X+(r.W-s)/2, r.Y+(r.H-s)/2
	x := func(v float32) float32 { return ox + v*k }
	y := func(v float32) float32 { return oy + v*k }
	w := 1.9 * k
	soft := col.Alpha(0.22)
	line := func(path *ui.Path) { p.StrokePath(path, w, col) }
	box := func(bx, by, bw, bh, rad float32, fill bool) {
		path := roundedRect(x(bx), y(by), bw*k, bh*k, rad*k)
		if fill {
			p.FillPath(path, soft)
		}
		line(path)
	}
	switch kind {
	case railDesk: // the desk: four tiles, two tall, the first filled
		box(3.5, 3.5, 7.5, 9.5, 2.2, true)
		box(13, 3.5, 7.5, 5.5, 2.2, false)
		box(13, 11, 7.5, 9.5, 2.2, false)
		box(3.5, 15, 7.5, 5.5, 2.2, false)
	case railMap: // a map: a topic, and two branches ending in topics
		p.FillPath(new(ui.Path).Circle(x(6.6), y(12), 3.6*k), soft)
		line(new(ui.Path).Circle(x(6.6), y(12), 3.6*k))
		line(new(ui.Path).MoveTo(x(10.2), y(12)).CubeTo(x(13.5), y(12), x(12.5), y(6), x(16.2), y(6)))
		line(new(ui.Path).MoveTo(x(10.2), y(12)).CubeTo(x(13.5), y(12), x(12.5), y(18), x(16.2), y(18)))
		line(new(ui.Path).Circle(x(18.6), y(6), 2.4*k))
		line(new(ui.Path).Circle(x(18.6), y(18), 2.4*k))
	case railTasks: // a task done: a disc, ticked
		p.FillPath(new(ui.Path).Circle(x(12), y(12), 8.5*k), soft)
		line(new(ui.Path).Circle(x(12), y(12), 8.5*k))
		line(new(ui.Path).MoveTo(x(8.2), y(12.3)).LineTo(x(10.9), y(15)).LineTo(x(15.9), y(9.4)))
	case railCollect: // what is kept: a box with its lid, a line on it
		box(3.5, 4, 17, 5, 1.8, true)
		line(new(ui.Path).MoveTo(x(5), y(9)).LineTo(x(5), y(18)).QuadTo(x(5), y(20), x(7), y(20)).
			LineTo(x(17), y(20)).QuadTo(x(19), y(20), x(19), y(18)).LineTo(x(19), y(9)))
		line(new(ui.Path).MoveTo(x(9.5), y(13.5)).LineTo(x(14.5), y(13.5)))
	case railSearch:
		line(new(ui.Path).Circle(x(11), y(11), 6.5*k))
		line(new(ui.Path).MoveTo(x(16), y(16)).LineTo(x(20.5), y(20.5)))
	case railGear: // the settings: three sliders
		for i, knob := range []float32{15, 9, 13.5} {
			cy := 6 + float32(i)*6
			line(new(ui.Path).MoveTo(x(4), y(cy)).LineTo(x(knob-2.4), y(cy)))
			line(new(ui.Path).MoveTo(x(knob+2.4), y(cy)).LineTo(x(20), y(cy)))
			line(new(ui.Path).Circle(x(knob), y(cy), 2.4*k))
		}
	}
}
