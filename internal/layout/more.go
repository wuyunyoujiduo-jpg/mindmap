package layout

import "github.com/wuyunyoujiduo-jpg/mindmap/internal/model"

// Modes of a layout besides the horizontal tree of Compute.
const (
	ModeTree     = ""         // the root in the middle or on the left, children beside their parent
	ModeDown     = "down"     // an org chart: children in a row below their parent
	ModeTimeline = "timeline" // the main branches along a line, above and below it by turns
	ModeFishbone = "fishbone" // a timeline from the head on the right, the bones leaning back
)

// computeDown lays the map out as an org chart: each node centered over
// the row of its children.
func computeDown(d *model.Document, measure Measure, o Options) Result {
	r := Result{Boxes: map[string]Rect{}, Left: map[string]bool{}, Up: map[string]bool{}}
	type sized struct{ w, h, span float32 }
	sizes := map[string]sized{}
	gapX, gapY := o.GapY*2, o.GapX
	var size func(n *model.Node, depth int)
	size = func(n *model.Node, depth int) {
		w, h := measure(n, depth)
		s := sized{w: w, h: h, span: w}
		if !n.Collapsed && len(n.Children) > 0 {
			var row float32
			for i, c := range n.Children {
				size(c, depth+1)
				if i > 0 {
					row += gapX
				}
				row += sizes[c.ID].span
			}
			s.span = max(w, row)
		}
		sizes[n.ID] = s
	}
	size(d.Root, 0)
	var place func(n *model.Node, left, top float32)
	place = func(n *model.Node, left, top float32) {
		s := sizes[n.ID]
		r.Boxes[n.ID] = Rect{X: left + (s.span-s.w)/2, Y: top, W: s.w, H: s.h}
		if n.Collapsed || len(n.Children) == 0 {
			return
		}
		var row float32
		for i, c := range n.Children {
			if i > 0 {
				row += gapX
			}
			row += sizes[c.ID].span
		}
		x := left + (s.span-row)/2
		for _, c := range n.Children { // a row under the parent, by their tops
			place(c, x, top+s.h+gapY)
			r.Edges = append(r.Edges, Edge{From: n.ID, To: c.ID})
			x += sizes[c.ID].span + gapX
		}
	}
	place(d.Root, 0, 0)
	return r.finish(o)
}

// computeLine lays the map out along a line: the root at its start, the
// main branches one after another along it, above and below it by turns,
// each with what is below it in an indented column going away from the
// line. A fishbone runs the line leftward from the root, its head.
func computeLine(d *model.Document, measure Measure, o Options, fishbone bool) Result {
	r := Result{Boxes: map[string]Rect{}, Left: map[string]bool{}, Up: map[string]bool{}}
	const indent = 24
	gap := o.GapY
	// block measures an indented column: its width and height.
	type blockSize struct{ w, h float32 }
	var block func(n *model.Node, depth int) blockSize
	block = func(n *model.Node, depth int) blockSize {
		w, h := measure(n, depth)
		b := blockSize{w, h}
		if !n.Collapsed {
			for _, c := range n.Children {
				cb := block(c, depth+1)
				b.w = max(b.w, indent+cb.w)
				b.h += gap + cb.h
			}
		}
		return b
	}
	// column places an indented column from the top left, as a list does.
	var column func(n *model.Node, x, y float32, depth int) float32
	column = func(n *model.Node, x, y float32, depth int) float32 {
		w, h := measure(n, depth)
		r.Boxes[n.ID] = Rect{X: x, Y: y, W: w, H: h}
		y += h
		if n.Collapsed {
			return y
		}
		for _, c := range n.Children {
			y += gap
			r.Edges = append(r.Edges, Edge{From: n.ID, To: c.ID})
			y = column(c, x+indent, y, depth+1)
		}
		return y
	}
	rw, rh := measure(d.Root, 0)
	root := Rect{X: 0, Y: -rh / 2, W: rw, H: rh}
	r.Boxes[d.Root.ID] = root
	along := o.GapX // from the root's edge
	lean := float32(0)
	if fishbone {
		lean = 40 // a bone's lean, back from where it meets the spine
	}
	if !d.Root.Collapsed {
		for i, b := range d.Root.Children {
			size := block(b, 1)
			up := i%2 == 0
			x := rw + along + lean
			if fishbone {
				x = -along - lean - size.w
			}
			var top float32
			if up {
				// The column above the line, its branch nearest to it:
				// laid out downward, then flipped about its block.
				top = -o.GapX - size.h
			} else {
				top = o.GapX
			}
			column(b, x, top, 1)
			if up {
				flip(&r, b, top, top+size.h)
			}
			r.Up[b.ID] = up
			r.Left[b.ID] = fishbone
			r.Edges = append(r.Edges, Edge{From: d.Root.ID, To: b.ID, Left: fishbone})
			// The next branch starts beside this one, overlapping the
			// other side's by half, as on a timeline.
			if i%2 == 1 || i == len(d.Root.Children)-1 {
				along += size.w + o.GapX
			} else {
				along += size.w/2 + o.GapX/2
			}
		}
	}
	return r.finish(o)
}

// flip mirrors the boxes of a branch's column within top..bottom, so that
// the branch is nearest the line and what is below it goes up.
func flip(r *Result, n *model.Node, top, bottom float32) {
	b := r.Boxes[n.ID]
	b.Y = top + bottom - b.Y - b.H
	r.Boxes[n.ID] = b
	if n.Collapsed {
		return
	}
	for _, c := range n.Children {
		flip(r, c, top, bottom)
	}
}

// finish moves the map to start at the margin and sizes it.
func (r Result) finish(o Options) Result {
	first := true
	var minX, minY, maxX, maxY float32
	for _, b := range r.Boxes {
		if first {
			minX, minY, maxX, maxY = b.X, b.Y, b.X+b.W, b.Y+b.H
			first = false
			continue
		}
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
