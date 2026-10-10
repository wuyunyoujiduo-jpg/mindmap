// Package layout places the nodes of a mind map: the root in the middle,
// its branches shared between its right and its left, and each node's
// children in a column beside it, on the side away from the root, centered
// on it.
package layout

import "github.com/wuyunyoujiduo-jpg/mindmap/internal/model"

// Rect is a box in map coordinates, before zooming.
type Rect struct{ X, Y, W, H float32 }

// Edge joins a parent's box to a child's. Left edges go from the parent's
// left side to the child's right side.
type Edge struct {
	From, To string
	Left     bool
}

// Options are the spacing of a layout.
type Options struct {
	Margin float32 // around the whole map
	GapX   float32 // between a node and its children's column
	GapY   float32 // between sibling subtrees
	// OneSided puts every branch on the root's right.
	OneSided bool
	// Mode lays the map out otherwise: ModeDown, ModeTimeline or
	// ModeFishbone.
	Mode string
}

// DefaultOptions are the spacing the app uses.
var DefaultOptions = Options{Margin: 40, GapX: 48, GapY: 14}

// Result is where every visible node goes.
type Result struct {
	Boxes  map[string]Rect
	Edges  []Edge
	Left   map[string]bool // the nodes on the root's left
	Up     map[string]bool // the main branches above the line of a timeline or fishbone
	Width  float32
	Height float32
}

// Measure returns the size of a node's box.
type Measure func(n *model.Node, depth int) (w, h float32)

// Compute lays out the visible nodes of the document: those under a
// collapsed node are left out.
func Compute(d *model.Document, measure Measure, o Options) Result {
	r := Result{Boxes: map[string]Rect{}, Left: map[string]bool{}, Up: map[string]bool{}}
	if d.Root == nil {
		return r
	}
	switch o.Mode {
	case ModeDown:
		return computeDown(d, measure, o)
	case ModeTimeline:
		return computeLine(d, measure, o, false)
	case ModeFishbone:
		return computeLine(d, measure, o, true)
	}
	type sized struct {
		w, h    float32 // the node's box
		subtree float32 // the height of the node and everything shown below it
	}
	sizes := map[string]sized{}
	column := func(nodes []*model.Node) float32 {
		var h float32
		for i, c := range nodes {
			if i > 0 {
				h += o.GapY
			}
			h += sizes[c.ID].subtree
		}
		return h
	}
	var measureTree func(n *model.Node, depth int)
	measureTree = func(n *model.Node, depth int) {
		w, h := measure(n, depth)
		s := sized{w: w, h: h, subtree: h}
		if !n.Collapsed {
			for _, c := range n.Children {
				measureTree(c, depth+1)
			}
			s.subtree = max(h, column(n.Children))
		}
		sizes[n.ID] = s
	}
	measureTree(d.Root, 0)

	// place puts n with its box's near side at x (its left on the right,
	// its right on the left) and its subtree from top, then its children
	// further out.
	var place func(n *model.Node, x, top float32, left bool)
	placeColumn := func(parent *model.Node, nodes []*model.Node, x, mid float32, left bool) {
		y := mid - column(nodes)/2
		for _, c := range nodes {
			place(c, x, y, left)
			r.Edges = append(r.Edges, Edge{From: parent.ID, To: c.ID, Left: left})
			y += sizes[c.ID].subtree + o.GapY
		}
	}
	place = func(n *model.Node, x, top float32, left bool) {
		s := sizes[n.ID]
		box := Rect{X: x, Y: top + (s.subtree-s.h)/2, W: s.w, H: s.h}
		if left {
			box.X = x - s.w
			r.Left[n.ID] = true
		}
		r.Boxes[n.ID] = box
		if n.Collapsed || len(n.Children) == 0 {
			return
		}
		mid := box.Y + box.H/2
		if left {
			placeColumn(n, n.Children, box.X-o.GapX, mid, true)
		} else {
			placeColumn(n, n.Children, box.X+box.W+o.GapX, mid, false)
		}
	}

	root := d.Root
	rs := sizes[root.ID]
	rootBox := Rect{X: 0, Y: -rs.h / 2, W: rs.w, H: rs.h}
	r.Boxes[root.ID] = rootBox
	if !root.Collapsed {
		right, left := split(root.Children, func(n *model.Node) float32 { return sizes[n.ID].subtree }, o)
		placeColumn(root, right, rootBox.W+o.GapX, 0, false)
		placeColumn(root, left, -o.GapX, 0, true)
	}

	// Move everything so the map starts at the margin.
	minX, minY := float32(0), rootBox.Y
	maxX, maxY := rootBox.W, rootBox.Y+rootBox.H
	for _, b := range r.Boxes {
		minX, minY = min(minX, b.X), min(minY, b.Y)
		maxX, maxY = max(maxX, b.X+b.W), max(maxY, b.Y+b.H)
	}
	for id, b := range r.Boxes {
		b.X += o.Margin - minX
		b.Y += o.Margin - minY
		r.Boxes[id] = b
	}
	r.Width = maxX - minX + 2*o.Margin
	r.Height = maxY - minY + 2*o.Margin
	return r
}

// split shares the root's branches between its right and its left: the
// first ones go right until that side holds about half the height, the
// rest left, in order.
func split(nodes []*model.Node, height func(*model.Node) float32, o Options) (right, left []*model.Node) {
	if o.OneSided || len(nodes) < 2 {
		return nodes, nil
	}
	var total float32
	for _, n := range nodes {
		total += height(n)
	}
	var h float32
	for i, n := range nodes {
		if i > 0 && h+height(n)/2 > total/2 {
			return nodes[:i], nodes[i:]
		}
		h += height(n)
	}
	return nodes[:len(nodes)-1], nodes[len(nodes)-1:]
}

// Hit returns the ID of the box containing the point, or "".
func (r Result) Hit(x, y float32) string {
	for id, b := range r.Boxes {
		if x >= b.X && x < b.X+b.W && y >= b.Y && y < b.Y+b.H {
			return id
		}
	}
	return ""
}
