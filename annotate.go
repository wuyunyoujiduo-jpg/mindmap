package main

import (
	"fmt"
	"math"
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/layout"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// Beyond the tree: floating topics placed anywhere, relation lines between
// any two nodes, summaries beside a node's children and boundaries around
// a branch.

// layoutFloating lays out the floating topics, each as a little map of its
// own at its place, and adds them to the canvas's layout.
func (a *app) layoutFloating(r layout.Result, measure layout.Measure, o layout.Options) layout.Result {
	root, ok := r.Boxes[a.doc.Root.ID]
	if !ok || len(a.doc.Floating) == 0 {
		return r
	}
	o.Mode, o.OneSided = layout.ModeTree, true
	sub := func(n *model.Node, depth int) (float32, float32) { return measure(n, depth+1) }
	for _, f := range a.doc.Floating {
		if f.Pos == nil {
			f.Pos = &model.Point{X: root.W + 200, Y: -120}
		}
		fr := layout.Compute(&model.Document{Root: f}, sub, o)
		fb := fr.Boxes[f.ID]
		dx, dy := root.X+f.Pos.X-fb.X, root.Y+f.Pos.Y-fb.Y
		for id, b := range fr.Boxes {
			b.X += dx
			b.Y += dy
			r.Boxes[id] = b
			r.Width = max(r.Width, b.X+b.W+o.Margin)
			r.Height = max(r.Height, b.Y+b.H+o.Margin)
		}
		r.Edges = append(r.Edges, fr.Edges...)
	}
	return r
}

// floatingTree tells, of every node of a floating topic, the topic.
func (a *app) floatingTree() map[string]*model.Node {
	out := map[string]*model.Node{}
	for _, f := range a.doc.Floating {
		var walk func(n *model.Node)
		walk = func(n *model.Node) {
			out[n.ID] = f
			for _, c := range n.Children {
				walk(c)
			}
		}
		walk(f)
	}
	return out
}

// addFloatingAt adds a floating topic where the canvas was double-clicked,
// (x, y) on the canvas, and edits its title.
func (a *app) addFloatingAt(x, y float32) {
	root, ok := a.boxes.Boxes[a.doc.Root.ID]
	if !ok {
		return
	}
	cam := a.cam()
	at := model.Point{X: (x-cam.X)/cam.Zoom - root.X, Y: (y-cam.Y)/cam.Zoom - root.Y}
	var n *model.Node
	a.mutate("", func() { n = a.doc.AddFloating("", at) })
	a.selected, a.multi = n.ID, nil
	a.startEdit(n.ID)
	a.justFloated = true
}

// floatNode takes a node out of the tree, as a floating topic beside where
// it was.
func (a *app) floatNode(n *model.Node) {
	root := a.boxes.Boxes[a.doc.Root.ID]
	b, ok := a.boxes.Boxes[n.ID]
	at := model.Point{X: b.X - root.X + 40, Y: b.Y - root.Y + 60}
	if !ok {
		at = model.Point{X: root.W + 200, Y: -120}
	}
	a.mutate("", func() { _ = a.doc.Float(n.ID, at) })
}

// Relations: a node's menu starts one, and a click on another ends it.

func (a *app) startRelation(n *model.Node) {
	a.relFrom = n.ID
	a.tell("点击另一个节点，连上关联线（Esc 取消）")
}

func (a *app) endRelation(to string) {
	from := a.relFrom
	a.relFrom = ""
	if from == "" || from == to {
		return
	}
	a.mutate("", func() {
		a.doc.Relations = append(a.doc.Relations, model.Relation{ID: model.NewID(), From: from, To: to})
	})
	a.tell("已添加关联线，可在右侧面板写标签")
}

func (a *app) removeRelation(id string) {
	a.mutate("", func() {
		a.doc.Relations = slices.DeleteFunc(slices.Clone(a.doc.Relations), func(r model.Relation) bool { return r.ID == id })
		if len(a.doc.Relations) == 0 {
			a.doc.Relations = nil
		}
	})
}

// drawRelations draws the relation lines between the nodes shown: dashed
// curves bowing away from the straight line, an arrow at the end, and the
// label at the middle.
func (a *app) drawRelations(p *ui.Painter, r layout.Result, ox, oy, z float32, col ui.Color) {
	for _, rel := range a.doc.Relations {
		fb, ok1 := r.Boxes[rel.From]
		tb, ok2 := r.Boxes[rel.To]
		if !ok1 || !ok2 {
			continue
		}
		fx, fy := ox+(fb.X+fb.W/2)*z, oy+(fb.Y+fb.H/2)*z
		tx, ty := ox+(tb.X+tb.W/2)*z, oy+(tb.Y+tb.H/2)*z
		// From and to the boxes' edges, toward each other.
		fx, fy = edgePoint(ox+fb.X*z, oy+fb.Y*z, fb.W*z, fb.H*z, tx, ty)
		tx, ty = edgePoint(ox+tb.X*z, oy+tb.Y*z, tb.W*z, tb.H*z, fx, fy)
		dx, dy := tx-fx, ty-fy
		length := float32(math.Hypot(float64(dx), float64(dy)))
		if length < 1 {
			continue
		}
		// The bow: a quarter of the length, to the side.
		nx, ny := -dy/length, dx/length
		bow := min(length/6, 50*z)
		c1x, c1y := fx+dx/3+nx*bow, fy+dy/3+ny*bow
		c2x, c2y := fx+2*dx/3+nx*bow, fy+2*dy/3+ny*bow
		at := func(t float32) (float32, float32) {
			u := 1 - t
			return u*u*u*fx + 3*u*u*t*c1x + 3*u*t*t*c2x + t*t*t*tx,
				u*u*u*fy + 3*u*u*t*c1y + 3*u*t*t*c2y + t*t*t*ty
		}
		// Dashes along the curve.
		const steps = 48
		for i := 0; i < steps; i += 2 {
			x0, y0 := at(float32(i) / steps)
			x1, y1 := at(float32(i+1) / steps)
			p.Line(x0, y0, x1, y1, max(1.5*z, 1), col)
		}
		// The arrow, along the curve's end.
		ex, ey := at(0.94)
		ang := math.Atan2(float64(ty-ey), float64(tx-ex))
		s := float64(8 * z)
		head := new(ui.Path).MoveTo(tx, ty).
			LineTo(tx-float32(s*math.Cos(ang-0.45)), ty-float32(s*math.Sin(ang-0.45))).
			LineTo(tx-float32(s*math.Cos(ang+0.45)), ty-float32(s*math.Sin(ang+0.45))).Close()
		p.FillPath(head, col)
		if rel.Label != "" {
			mx, my := at(0.5)
			size := 12 * z
			w := float32(len([]rune(rel.Label)))*size + 10*z
			p.Fill(ui.Rect{X: mx - w/2, Y: my - size*0.9, W: w, H: size * 1.8}, p.Theme().Background, 4*z)
			p.Text(mx-w/2+5*z, my-size*0.65, rel.Label, size, col)
		}
	}
}

// edgePoint is where the line from a box's center to (x, y) leaves it.
func edgePoint(bx, by, bw, bh, x, y float32) (float32, float32) {
	cx, cy := bx+bw/2, by+bh/2
	dx, dy := x-cx, y-cy
	if dx == 0 && dy == 0 {
		return cx, cy
	}
	sx, sy := float32(math.Inf(1)), float32(math.Inf(1))
	if dx != 0 {
		sx = (bw / 2) / float32(math.Abs(float64(dx)))
	}
	if dy != 0 {
		sy = (bh / 2) / float32(math.Abs(float64(dy)))
	}
	s := min(sx, sy)
	return cx + dx*s, cy + dy*s
}

// subtreeBox is the box around a node and every node shown below it.
func subtreeBox(r layout.Result, n *model.Node) (layout.Rect, bool) {
	b, ok := r.Boxes[n.ID]
	if !ok {
		return b, false
	}
	minX, minY, maxX, maxY := b.X, b.Y, b.X+b.W, b.Y+b.H
	var walk func(m *model.Node)
	walk = func(m *model.Node) {
		if m.Collapsed {
			return
		}
		for _, c := range m.Children {
			if cb, ok := r.Boxes[c.ID]; ok {
				minX, minY = min(minX, cb.X), min(minY, cb.Y)
				maxX, maxY = max(maxX, cb.X+cb.W), max(maxY, cb.Y+cb.H)
				walk(c)
			}
		}
	}
	walk(n)
	return layout.Rect{X: minX, Y: minY, W: maxX - minX, H: maxY - minY}, true
}

// drawBoundaries draws the frames around branches, behind the nodes.
func (a *app) drawBoundaries(p *ui.Painter, r layout.Result, shown *model.Document, ox, oy, z float32, colorOf func(string) ui.Color) {
	shown.Walk(func(n, _ *model.Node, _ int) bool {
		if n.Boundary == nil {
			return true
		}
		b, ok := subtreeBox(r, n)
		if !ok {
			return true
		}
		pad := 10 * z
		rect := ui.Rect{X: ox + b.X*z - pad, Y: oy + b.Y*z - pad, W: b.W*z + 2*pad, H: b.H*z + 2*pad}
		col := colorOf(n.ID)
		p.Fill(rect, col.Alpha(0.06), 12*z)
		p.StrokeDashed(rect, col.Alpha(0.7), 12*z, max(1.2*z, 1))
		if n.Boundary.Label != "" {
			size := 12 * z
			w := float32(len([]rune(n.Boundary.Label)))*size + 12*z
			label := ui.Rect{X: rect.X + 12*z, Y: rect.Y - size*0.9, W: w, H: size * 1.7}
			p.Fill(label, col, 4*z)
			p.Text(label.X+6*z, label.Y+size*0.25, n.Boundary.Label, size, textOn(col))
		}
		return true
	})
}

// drawSummaries draws, beside the children of nodes with a summary, a
// brace around them and the summary's label beyond it.
func (a *app) drawSummaries(p *ui.Painter, r layout.Result, shown *model.Document, ox, oy, z float32, colorOf func(string) ui.Color) {
	shown.Walk(func(n, _ *model.Node, _ int) bool {
		if n.Summary == nil || n.Collapsed || len(n.Children) == 0 {
			return true
		}
		own, ok := r.Boxes[n.ID]
		if !ok {
			return true
		}
		var kids layout.Rect
		first := true
		for _, c := range n.Children {
			b, ok := subtreeBox(r, c)
			if !ok {
				continue
			}
			if first {
				kids, first = b, false
				continue
			}
			x1, y1 := max(kids.X+kids.W, b.X+b.W), max(kids.Y+kids.H, b.Y+b.H)
			kids.X, kids.Y = min(kids.X, b.X), min(kids.Y, b.Y)
			kids.W, kids.H = x1-kids.X, y1-kids.Y
		}
		if first {
			return true
		}
		left := r.Left[n.ID] || own.X > kids.X+kids.W // the children are left of it
		col := colorOf(n.ID)
		top, bottom := oy+kids.Y*z, oy+(kids.Y+kids.H)*z
		bx := ox + (kids.X+kids.W)*z + 14*z
		dir := float32(1)
		if left {
			bx, dir = ox+kids.X*z-14*z, -1
		}
		rad := min(8*z, (bottom-top)/4)
		mid := (top + bottom) / 2
		brace := new(ui.Path).MoveTo(bx-dir*rad, top).QuadTo(bx, top, bx, top+rad).LineTo(bx, mid-rad).
			QuadTo(bx, mid, bx+dir*rad, mid).QuadTo(bx, mid, bx, mid+rad).LineTo(bx, bottom-rad).
			QuadTo(bx, bottom, bx-dir*rad, bottom)
		p.StrokePath(brace, max(1.5*z, 1), col)
		if label := n.Summary.Label; label != "" {
			size := 14 * z
			w := float32(len([]rune(label)))*size + 16*z
			x := bx + dir*(rad+8*z)
			if left {
				x -= w
			}
			box := ui.Rect{X: x, Y: mid - size, W: w, H: size * 2}
			p.Fill(box, col.Alpha(0.15), 6*z)
			p.Stroke(box, col, 6*z, max(z, 1))
			p.Text(box.X+8*z, box.Y+size*0.45, label, size, p.Theme().Text)
		}
		return true
	})
}

// annotationsSection is the panel's part for a node's summary, boundary,
// relations, and whether it floats.
func (a *app) annotationsSection(c *ui.Context, n *model.Node) {
	t := c.Theme()
	rels := slices.DeleteFunc(slices.Clone(a.doc.Relations), func(r model.Relation) bool { return r.From != n.ID && r.To != n.ID })
	if n.Summary == nil && n.Boundary == nil && len(rels) == 0 {
		return
	}
	ui.Field(c, "概要、外框与关联线", func() {
		ui.Column(c).Gap(8).Children(func() {
			mark := func(key, name string, m **model.Mark) {
				if *m == nil {
					return
				}
				ui.Row(c.Key(key)).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, name).FontSize(12).TextColor(t.TextMuted).Width(40)
					label := (*m).Label
					if ui.TextInput(c, &label).Placeholder("标签").FontSize(12).Grow(1).Label(name + "标签").Changed() {
						a.mutate(key+":"+n.ID, func() { (*m).Label = label })
					}
					if iconButton(c, iconClose, "删除"+name, "删除"+name, 22, true).Clicked() {
						a.later(func() { a.mutate("", func() { *m = nil }) })
					}
				})
			}
			mark("summary", "概要", &n.Summary)
			mark("boundary", "外框", &n.Boundary)
			for i := range rels {
				rel := rels[i]
				other, arrow := rel.To, "→"
				if rel.To == n.ID {
					other, arrow = rel.From, "←"
				}
				o, _ := a.doc.Find(other)
				if o == nil {
					continue
				}
				ui.Row(c.Key("rel:" + rel.ID)).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, fmt.Sprintf("%s %s", arrow, o.Title)).FontSize(12).Width(110).MaxLines(1)
					label := rel.Label
					if ui.TextInput(c, &label).Placeholder("关联线标签").FontSize(12).Grow(1).Label("关联线标签").Changed() {
						a.mutate("rel:"+rel.ID, func() {
							for j := range a.doc.Relations {
								if a.doc.Relations[j].ID == rel.ID {
									a.doc.Relations[j].Label = label
								}
							}
						})
					}
					if iconButton(c, iconClose, "删除关联线", "删除关联线", 22, true).Clicked() {
						id := rel.ID
						a.later(func() { a.removeRelation(id) })
					}
				})
			}
		})
	})
}

// annotationMenu is the part of a node's menu that adds these.
func (a *app) annotationMenu(m *ui.Menu, n *model.Node, depth int) {
	m.Submenu("插入", func(m *ui.Menu) {
		if m.Item("关联线到…").Chosen() {
			a.later(func() { a.startRelation(n) })
		}
		if m.Item("概要").Disabled(len(n.Children) == 0 || n.Summary != nil).Chosen() {
			a.later(func() { a.mutate("", func() { n.Summary = &model.Mark{Label: "概要"} }) })
		}
		if m.Item("外框").Disabled(n.Boundary != nil).Chosen() {
			a.later(func() { a.mutate("", func() { n.Boundary = &model.Mark{} }) })
		}
		m.Separator()
		if depth > 0 && !a.doc.IsFloating(n.ID) && m.Item("设为自由主题").Chosen() {
			a.later(func() { a.floatNode(n) })
		}
		if a.doc.IsFloating(n.ID) && m.Item("挂到节点下…").Chosen() {
			a.later(func() { a.openPalette(paletteMove) })
		}
	})
}

// overNode reports whether the pointer is over a node of the canvas.
func (a *app) overNode() bool {
	cam := a.cam()
	x, y := (a.pointer[0]-cam.X)/cam.Zoom, (a.pointer[1]-cam.Y)/cam.Zoom
	for _, b := range a.boxes.Boxes {
		if x >= b.X && x <= b.X+b.W && y >= b.Y && y <= b.Y+b.H {
			return true
		}
	}
	return false
}
