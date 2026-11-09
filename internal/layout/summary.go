package layout

import "github.com/wuyunyoujiduo-jpg/mindmap/internal/model"

// Summaries: a brace by a node's children, and beyond it the summary's
// topic with what is below it, laid out as the structure lays out a
// branch: beside the children in a tree, under them in an org chart, and
// as an indented column beside them on a timeline.

// Brace is where a summary's brace goes: beside the children (from top to
// bottom at x, opening toward Dir, 1 right or -1 left), or, Horizontal,
// under them (from left to right at y, opening down).
type Brace struct {
	Horizontal bool
	At         float32 // the x of a brace beside, the y of one under
	From, To   float32 // where it spans
	Dir        float32
	Topic      string // the summary's topic
}

const (
	// braceGap is between the children and their brace, SummaryGap between
	// the brace and the topic.
	braceGap   = 14
	SummaryGap = 18
)

// How summaries are laid out.
const (
	summaryBeside = iota // a tree: beside the children
	summaryBelow         // an org chart: under them
	summaryColumn        // a timeline: an indented column beside them
)

// placeSummaries lays out the summaries of the nodes below root shown,
// the deepest first, so that a summary goes past those within its
// children. A root with branches on both sides, or along a line, has none.
func placeSummaries(r *Result, root *model.Node, measure Measure, o Options, kind int, rootToo bool) {
	var visit func(n *model.Node, depth int)
	visit = func(n *model.Node, depth int) {
		if n.Collapsed {
			return
		}
		for _, c := range n.Children {
			visit(c, depth+1)
		}
		t := n.SummaryTopic()
		if t == nil || len(n.Children) == 0 || n == root && !rootToo {
			return
		}
		kids, ok := extent(r, n.Children)
		if !ok {
			return
		}
		sub := func(m *model.Node, d int) (float32, float32) { return measure(m, depth+1+d) }
		so := Options{GapX: o.GapX, GapY: o.GapY, OneSided: true}
		switch kind {
		case summaryBelow:
			sr := computeDown(&model.Document{Root: t}, sub, so)
			tb := sr.Boxes[t.ID]
			y := kids.Y + kids.H + braceGap
			r.merge(sr, kids.X+kids.W/2-(tb.X+tb.W/2), y+SummaryGap-tb.Y, false, 0)
			r.Braces[n.ID] = Brace{Horizontal: true, At: y, From: kids.X, To: kids.X + kids.W, Dir: 1, Topic: t.ID}
		case summaryColumn:
			sr := computeColumn(&model.Document{Root: t}, sub, so)
			tb := sr.Boxes[t.ID]
			x := kids.X + kids.W + braceGap
			r.merge(sr, x+SummaryGap-tb.X, kids.Y+kids.H/2-(tb.Y+tb.H/2), false, 0) // level with the brace's tip
			r.Braces[n.ID] = Brace{At: x, From: kids.Y, To: kids.Y + kids.H, Dir: 1, Topic: t.ID}
		default:
			own := r.Boxes[n.ID]
			left := r.Left[n.ID] || own.X > kids.X+kids.W // the children are left of it
			sr := Compute(&model.Document{Root: t}, sub, so)
			tb := sr.Boxes[t.ID]
			mid := kids.Y + kids.H/2
			x, dir := kids.X+kids.W+braceGap, float32(1)
			dx := x + SummaryGap - tb.X
			if left { // mirrored about the topic, which stays where it is
				x, dir = kids.X-braceGap, -1
				dx = x - SummaryGap - (tb.X + tb.W)
			}
			r.merge(sr, dx, mid-(tb.Y+tb.H/2), left, tb.X+tb.W/2)
			r.Braces[n.ID] = Brace{At: x, From: kids.Y, To: kids.Y + kids.H, Dir: dir, Topic: t.ID}
		}
	}
	visit(root, 0)
}

// extent is the box around nodes and all shown below them, summaries
// included.
func extent(r *Result, nodes []*model.Node) (Rect, bool) {
	var box Rect
	found := false
	var walk func(n *model.Node)
	walk = func(n *model.Node) {
		if b, ok := r.Boxes[n.ID]; ok {
			if !found {
				box, found = b, true
			} else {
				x1, y1 := max(box.X+box.W, b.X+b.W), max(box.Y+box.H, b.Y+b.H)
				box.X, box.Y = min(box.X, b.X), min(box.Y, b.Y)
				box.W, box.H = x1-box.X, y1-box.Y
			}
		}
		if n.Collapsed {
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
		if t := n.SummaryTopic(); t != nil && len(n.Children) > 0 {
			walk(t)
		}
	}
	for _, n := range nodes {
		walk(n)
	}
	return box, found
}

// merge adds a summary's layout, moved by (dx, dy) and, on the left,
// first mirrored about x = axis.
func (r *Result) merge(sr Result, dx, dy float32, left bool, axis float32) {
	for id, b := range sr.Boxes {
		if left {
			b.X = 2*axis - (b.X + b.W)
		}
		b.X += dx
		b.Y += dy
		r.Boxes[id] = b
		r.Left[id] = left || sr.Left[id]
	}
	for _, e := range sr.Edges {
		e.Left = e.Left || left
		r.Edges = append(r.Edges, e)
	}
	for id, br := range sr.Braces {
		if br.Horizontal {
			br.At += dy
			br.From, br.To = br.From+dx, br.To+dx
		} else {
			if left {
				br.At, br.Dir = 2*axis-br.At, -br.Dir
			}
			br.At += dx
			br.From, br.To = br.From+dy, br.To+dy
		}
		r.Braces[id] = br
	}
}

// Shift moves the whole layout, its braces too.
func (r *Result) Shift(dx, dy float32) {
	for id, b := range r.Boxes {
		b.X += dx
		b.Y += dy
		r.Boxes[id] = b
	}
	for id, br := range r.Braces {
		if br.Horizontal {
			br.At += dy
			br.From, br.To = br.From+dx, br.To+dx
		} else {
			br.At += dx
			br.From, br.To = br.From+dy, br.To+dy
		}
		r.Braces[id] = br
	}
}

// computeColumn lays a node out as an indented column, as the branches of
// a timeline are: what is below it under it, a step further in.
func computeColumn(d *model.Document, measure Measure, o Options) Result {
	r := newResult()
	var column func(n *model.Node, x, y float32, depth int) float32
	column = func(n *model.Node, x, y float32, depth int) float32 {
		w, h := measure(n, depth)
		r.Boxes[n.ID] = Rect{X: x, Y: y, W: w, H: h}
		y += h
		if n.Collapsed {
			return y
		}
		for _, c := range n.Children {
			y += o.GapY
			r.Edges = append(r.Edges, Edge{From: n.ID, To: c.ID})
			y = column(c, x+columnIndent, y, depth+1)
		}
		return y
	}
	column(d.Root, 0, 0, 0)
	placeSummaries(&r, d.Root, measure, o, summaryColumn, true)
	return r.finish(o)
}

// columnIndent is how far in a column's children are.
const columnIndent = 24

func newResult() Result {
	return Result{Boxes: map[string]Rect{}, Left: map[string]bool{}, Up: map[string]bool{}, Braces: map[string]Brace{}}
}
