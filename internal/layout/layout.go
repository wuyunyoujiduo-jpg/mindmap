// Package layout places the nodes of a mind map: the root on the left and
// each node's children in a column to its right, centered on it.
package layout

import "github.com/wuyunyoujiduo-jpg/mindmap/internal/model"

// Rect is a box in map coordinates, before zooming.
type Rect struct{ X, Y, W, H float32 }

// Edge joins a parent's box to a child's.
type Edge struct{ From, To string }

// Options are the spacing of a layout.
type Options struct {
	Margin float32 // around the whole map
	GapX   float32 // between a node and its children's column
	GapY   float32 // between sibling subtrees
}

// DefaultOptions are the spacing the app uses.
var DefaultOptions = Options{Margin: 40, GapX: 48, GapY: 14}

// Result is where every visible node goes.
type Result struct {
	Boxes  map[string]Rect
	Edges  []Edge
	Width  float32
	Height float32
}

// Measure returns the size of a node's box.
type Measure func(n *model.Node, depth int) (w, h float32)

// Compute lays out the visible nodes of the document: those under a
// collapsed node are left out.
func Compute(d *model.Document, measure Measure, o Options) Result {
	r := Result{Boxes: map[string]Rect{}}
	if d.Root == nil {
		return r
	}
	type sized struct {
		w, h    float32 // the node's box
		subtree float32 // the height of the node and everything shown below it
	}
	sizes := map[string]sized{}
	var measureTree func(n *model.Node, depth int) float32
	measureTree = func(n *model.Node, depth int) float32 {
		w, h := measure(n, depth)
		var children float32
		if !n.Collapsed && len(n.Children) > 0 {
			for i, c := range n.Children {
				if i > 0 {
					children += o.GapY
				}
				children += measureTree(c, depth+1)
			}
		}
		s := sized{w: w, h: h, subtree: max(h, children)}
		sizes[n.ID] = s
		return s.subtree
	}
	total := measureTree(d.Root, 0)

	var place func(n *model.Node, x, top float32)
	place = func(n *model.Node, x, top float32) {
		s := sizes[n.ID]
		box := Rect{X: x, Y: top + (s.subtree-s.h)/2, W: s.w, H: s.h}
		r.Boxes[n.ID] = box
		r.Width = max(r.Width, box.X+box.W)
		if n.Collapsed || len(n.Children) == 0 {
			return
		}
		var children float32
		for i, c := range n.Children {
			if i > 0 {
				children += o.GapY
			}
			children += sizes[c.ID].subtree
		}
		y := top + (s.subtree-children)/2
		cx := x + s.w + o.GapX
		for _, c := range n.Children {
			place(c, cx, y)
			r.Edges = append(r.Edges, Edge{From: n.ID, To: c.ID})
			y += sizes[c.ID].subtree + o.GapY
		}
	}
	place(d.Root, o.Margin, o.Margin)
	r.Width += o.Margin
	r.Height = total + 2*o.Margin
	return r
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
