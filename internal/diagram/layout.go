package diagram

import "slices"

// Rect is a box, in the diagram's own units.
type Rect struct{ X, Y, W, H float32 }

// Point is a place of a diagram.
type Point struct{ X, Y float32 }

// Where a label is written from its place.
const (
	AnchorStart  = 1  // it begins there
	AnchorMiddle = 0  // it is centered there
	AnchorEnd    = -1 // it ends there
)

// Route is an edge as drawn: a line of straight runs, with an arrow at
// its end, and where its label goes.
type Route struct {
	Edge    int // its index among the graph's edges
	Points  []Point
	LabelAt Point // the label's baseline's place, by Anchor
	Anchor  int
}

// Layout is where everything of a graph goes.
type Layout struct {
	Boxes  map[string]Rect
	Routes []Route
	Lanes  []Rect // a flow's lanes, each a column the height of the diagram
	Head   float32
	Width  float32
	Height float32
}

// The sizes of a flow and of the states.
const (
	laneWidth   = 175
	flowHead    = 36 // the lanes' names
	flowRow     = 92
	flowBoxW    = 152
	flowBoxH    = 44
	flowTallH   = 56 // a box with a second line
	flowDiaH    = 60
	stateColMin = 166
	stateColMax = 230
	stateRow    = 140
	stateBoxW   = 100
	stateBoxH   = 44
	stateLeft   = 60
	stateDotY   = 44 // the row of the dot the states begin at
	stateTop    = 140
	dotSize     = 12
	pairShift   = 9  // between two edges that would run on each other
	gutterSide  = 10 // of a detour between states, from a column's edge
)

type cell struct{ c, r int }

// grid is nodes on cells, which routes are found on.
type grid struct {
	at   map[cell]string
	pos  map[string]cell
	size map[string][2]float32
	colX func(c int) float32 // a column's middle
	rowY func(r int) float32 // a row's middle
	colL func(c int) float32 // a column's left edge
	rowT func(r int) float32 // a row's top edge
	gut  float32             // a detour's distance from a column's edge
	used map[[2]int]int      // the detours taken by a column's edge, each a little further in
}

// beside is where a detour runs by a column: by its right edge, or its
// left, each one after the first a little further from the edge.
func (g *grid) beside(c int, right bool) float32 {
	if g.used == nil {
		g.used = map[[2]int]int{}
	}
	k := [2]int{c, 0}
	if right {
		k[1] = 1
	}
	// In a flow the boxes leave little by the lanes' edges: on the edge
	// itself, then a little to either side of it.
	off := []float32{0, 4, -4, 8}[g.used[k]%4]
	if g.gut > 0 {
		off = g.gut + 6*float32(g.used[k]%4)
	}
	g.used[k]++
	if right {
		return g.colL(c+1) - off
	}
	return g.colL(c) + off
}

func (g *grid) put(id string, c cell, w, h float32) {
	g.at[c], g.pos[id], g.size[id] = id, c, [2]float32{w, h}
}

func (g *grid) free(c, r int) bool { _, taken := g.at[cell{c, r}]; return !taken }

// clear tells whether the cells strictly between two of a row or a column
// are free.
func (g *grid) clear(a, b cell) bool {
	switch {
	case a.r == b.r:
		for c := min(a.c, b.c) + 1; c < max(a.c, b.c); c++ {
			if !g.free(c, a.r) {
				return false
			}
		}
	case a.c == b.c:
		for r := min(a.r, b.r) + 1; r < max(a.r, b.r); r++ {
			if !g.free(a.c, r) {
				return false
			}
		}
	}
	return true
}

func (g *grid) box(id string) Rect {
	p, s := g.pos[id], g.size[id]
	return Rect{g.colX(p.c) - s[0]/2, g.rowY(p.r) - s[1]/2, s[0], s[1]}
}

// port is the middle of a side of a node's box: 'n', 'e', 's' or 'w'.
func (g *grid) port(id string, side byte) Point {
	b := g.box(id)
	switch side {
	case 'n':
		return Point{b.X + b.W/2, b.Y}
	case 's':
		return Point{b.X + b.W/2, b.Y + b.H}
	case 'w':
		return Point{b.X, b.Y + b.H/2}
	}
	return Point{b.X + b.W, b.Y + b.H/2}
}

// forward are the edges that are not back edges: those which, followed
// from the first nodes, do not return to a node being followed.
func forward(g *Graph) []bool {
	fwd := make([]bool, len(g.Edges))
	state := map[string]int{} // 1 being followed, 2 done
	var visit func(id string)
	visit = func(id string) {
		state[id] = 1
		for i, e := range g.Edges {
			if e.From != id {
				continue
			}
			switch state[e.To] {
			case 0:
				fwd[i] = true
				visit(e.To)
			case 2:
				fwd[i] = true
			}
		}
		state[id] = 2
	}
	for _, n := range g.Nodes {
		if state[n.ID] == 0 {
			visit(n.ID)
		}
	}
	return fwd
}

// topo is the nodes with each after those its forward edges come from, in
// their own order otherwise.
func topo(g *Graph, fwd []bool) []string {
	in := map[string]int{}
	for i, e := range g.Edges {
		if fwd[i] {
			in[e.To]++
		}
	}
	var order []string
	done := map[string]bool{}
	for len(order) < len(g.Nodes) {
		progressed := false
		for _, n := range g.Nodes {
			if done[n.ID] || in[n.ID] > 0 {
				continue
			}
			done[n.ID], progressed = true, true
			order = append(order, n.ID)
			for i, e := range g.Edges {
				if fwd[i] && e.From == n.ID {
					in[e.To]--
				}
			}
			break // from the first again: the order written is kept where it may be
		}
		if !progressed { // cannot be, forward edges having no cycle
			before := len(order)
			for _, n := range g.Nodes {
				if !done[n.ID] {
					done[n.ID] = true
					order = append(order, n.ID)
				}
			}
			if len(order) == before { // two nodes of one ID, in a set not cleaned
				break
			}
		}
	}
	return order
}

// TitleRunes is how many characters of a title a box of a flow holds on
// a line; DiamondRunes, a question's diamond. A longer one takes two.
const (
	TitleRunes   = 9
	DiamondRunes = 7
)

// FlowBoxHeight is how tall a node of a flow is drawn: taller with a
// second line under its title, or a title of two lines.
func FlowBoxHeight(n *Node) float32 {
	long := len([]rune(n.Title))
	if n.Kind == KindDecision {
		if long > DiamondRunes {
			return flowDiaH + 14
		}
		return flowDiaH
	}
	h := float32(flowBoxH)
	if n.Sub != "" {
		h = flowTallH
	}
	if long > TitleRunes {
		h += 14
	}
	return h
}

// LayoutFlow places a flow: a column a lane, and down the rows as it
// goes. A step in the lane of the one before it is on the next row; one
// in another lane stays on its row while the way there is free, so that a
// flow that changes hands does not grow a row at each.
//
// Where that leaves a route through a node, every step has a row of its
// own, on which a way is always found.
func (g *Graph) LayoutFlow() Layout {
	if l := g.layoutFlow(false); !l.Crossed(g) {
		return l
	}
	return g.layoutFlow(true)
}

func (g *Graph) layoutFlow(apart bool) Layout {
	fwd := forward(g)
	lanes := max(len(g.Lanes), 1)
	lane := func(n *Node) int { return n.Lane }
	if len(g.Lanes) == 0 {
		// With no lanes named it would be one long column: its longest
		// way is the first column, and what turns off it goes beside.
		auto := sideLanes(g, fwd)
		lane = func(n *Node) int { return auto[n.ID] }
		for _, c := range auto {
			lanes = max(lanes, c+1)
		}
	}
	head := float32(flowHead)
	if len(g.Lanes) == 0 {
		head = 0
	}
	gr := &grid{at: map[cell]string{}, pos: map[string]cell{}, size: map[string][2]float32{},
		colX: func(c int) float32 { return float32(c)*laneWidth + laneWidth/2 },
		rowY: func(r int) float32 { return head + flowRow/2 + 2 + float32(r)*flowRow },
		colL: func(c int) float32 { return float32(c) * laneWidth },
		rowT: func(r int) float32 { return head + 2 + float32(r)*flowRow },
	}
	rows := 0
	for _, id := range topo(g, fwd) {
		n := g.Node(id)
		col := lane(n)
		r := 0
		var beside []cell // those it comes from on other lanes
		for i, e := range g.Edges {
			if !fwd[i] || e.To != id {
				continue
			}
			p, ok := gr.pos[e.From]
			if !ok {
				continue
			}
			if p.c == col {
				r = max(r, p.r+1)
			} else {
				r = max(r, p.r)
				beside = append(beside, p)
			}
		}
		if apart {
			r = rows
		}
		for ; ; r++ {
			if !gr.free(col, r) {
				continue
			}
			if !slices.ContainsFunc(beside, func(p cell) bool { return p.r == r && !gr.clear(p, cell{col, r}) }) {
				break
			}
		}
		h := FlowBoxHeight(n)
		gr.put(id, cell{col, r}, flowBoxW, h)
		rows = max(rows, r+1)
	}
	l := Layout{Boxes: map[string]Rect{}, Head: head, Width: float32(lanes) * laneWidth, Height: head + float32(rows)*flowRow + 8}
	for _, n := range g.Nodes {
		l.Boxes[n.ID] = gr.box(n.ID)
	}
	for i := range g.Lanes {
		l.Lanes = append(l.Lanes, Rect{float32(i) * laneWidth, 0, laneWidth, l.Height})
	}
	l.Routes = routes(g, gr)
	return l
}

// sideLanes gives the nodes of a flow with no lanes their columns: the
// longest way from its beginning the first, and a node off it one column
// further than what leads to it, three columns at most.
func sideLanes(g *Graph, fwd []bool) map[string]int {
	next := func(id string) []string {
		var out []string
		for i, e := range g.Edges {
			if fwd[i] && e.From == id {
				out = append(out, e.To)
			}
		}
		return out
	}
	depth := map[string]int{}
	var deep func(id string) int
	deep = func(id string) int {
		if d, ok := depth[id]; ok {
			return d
		}
		depth[id] = 0
		d := 0
		for _, to := range next(id) {
			d = max(d, deep(to)+1)
		}
		depth[id] = d
		return d
	}
	lanes := map[string]int{}
	order := topo(g, fwd)
	at := ""
	for _, n := range g.Nodes {
		if n.Kind == KindStart {
			at = n.ID
			break
		}
	}
	if at == "" && len(order) > 0 {
		at = order[0]
	}
	main := map[string]bool{}
	for at != "" {
		main[at] = true
		best := ""
		for _, to := range next(at) {
			if !main[to] && (best == "" || deep(to) > deep(best)) {
				best = to
			}
		}
		at = best
	}
	for _, id := range order {
		if main[id] {
			lanes[id] = 0
			continue
		}
		c := -1
		for i, e := range g.Edges {
			if p, ok := lanes[e.From]; ok && fwd[i] && e.To == id && (c < 0 || p+1 < c) {
				c = p + 1
			}
		}
		lanes[id] = min(max(c, 1), 2)
	}
	return lanes
}

// LayoutState places the states of a thing: the longest way from the
// first along a row, a column a step; a state nothing leads on from under
// the one it comes from, and the others in the column of their step. The
// dot they begin at is over the first.
//
// Where that leaves a route through a node, they are one column, in
// their order.
func (g *Graph) LayoutState() Layout {
	if l := g.layoutState(false); !l.Crossed(g) {
		return l
	}
	return g.layoutState(true)
}

func (g *Graph) layoutState(apart bool) Layout {
	// A column is as wide as the longest label of an arrow asks, which is
	// written between two states.
	stateCol := float32(stateColMin)
	for _, e := range g.Edges {
		stateCol = max(stateCol, min(stateBoxW+float32(len([]rune(e.Label)))*12.5+26, stateColMax))
	}
	gr := &grid{gut: gutterSide + 4, at: map[cell]string{}, pos: map[string]cell{}, size: map[string][2]float32{},
		colX: func(c int) float32 { return stateLeft + stateBoxW/2 + float32(c)*stateCol },
		rowY: func(r int) float32 {
			if r < 0 {
				return stateDotY
			}
			return stateTop + float32(r)*stateRow
		},
		colL: func(c int) float32 { return stateLeft + stateBoxW/2 + float32(c)*stateCol - stateCol/2 },
		rowT: func(r int) float32 {
			if r <= 0 {
				return (stateDotY + stateTop) / 2
			}
			return stateTop + float32(r)*stateRow - stateRow/2
		},
	}
	fwd := forward(g)
	start := ""
	for _, n := range g.Nodes {
		if n.Kind == KindStart {
			start = n.ID
			break
		}
	}
	next := func(id string) []string { // where its forward edges go
		var out []string
		for i, e := range g.Edges {
			if fwd[i] && e.From == id {
				out = append(out, e.To)
			}
		}
		return out
	}
	depth := map[string]int{} // the longest way on from a node
	var deep func(id string) int
	deep = func(id string) int {
		if d, ok := depth[id]; ok {
			return d
		}
		depth[id] = 0
		d := 0
		for _, to := range next(id) {
			d = max(d, deep(to)+1)
		}
		depth[id] = d
		return d
	}
	// The main way: from the first state, each time on to the one with the
	// longest way after it.
	cols, rows := 0, 1
	at := ""
	if first := next(start); start != "" && len(first) > 0 {
		at = first[0]
	} else {
		for _, n := range g.Nodes {
			if n.ID != start {
				at = n.ID
				break
			}
		}
	}
	for c := 0; at != "" && !apart; c++ {
		gr.put(at, cell{c, 0}, stateBoxW, stateBoxH)
		cols = c + 1
		best := ""
		for _, to := range next(at) {
			if _, placed := gr.pos[to]; !placed && (best == "" || deep(to) > deep(best)) {
				best = to
			}
		}
		at = best
	}
	for _, id := range topo(g, fwd) {
		if _, placed := gr.pos[id]; placed || id == start {
			continue
		}
		c, from := 0, -1
		for i, e := range g.Edges {
			if p, ok := gr.pos[e.From]; ok && fwd[i] && e.To == id {
				if from < 0 {
					from = p.c
				}
				c = max(c, p.c+1)
			}
		}
		if len(next(id)) == 0 && from >= 0 {
			c = from // nothing leads on from it: under the one it comes from
		}
		r := 1
		if apart {
			c, r = 0, 0
		}
		for !gr.free(c, r) {
			r++
		}
		gr.put(id, cell{c, r}, stateBoxW, stateBoxH)
		cols, rows = max(cols, c+1), max(rows, r+1)
	}
	if start != "" {
		c := 0
		if first := next(start); len(first) > 0 && !apart {
			c = gr.pos[first[0]].c
		}
		gr.put(start, cell{c, -1}, dotSize, dotSize)
	}
	l := Layout{Boxes: map[string]Rect{},
		Width:  stateLeft*2 + stateBoxW + float32(cols-1)*stateCol,
		Height: stateTop + float32(rows-1)*stateRow + stateBoxH/2 + 28}
	for _, n := range g.Nodes {
		l.Boxes[n.ID] = gr.box(n.ID)
	}
	l.Routes = routes(g, gr)
	return l
}

// routes finds each edge its way on the grid: straight where the two are
// in a row or a column with nothing between; else round one corner, the
// one whose two runs cross no node; and by the gutter beside the column
// or over the row when a node is in the way.
func routes(g *Graph, gr *grid) []Route {
	var out []Route
	for i, e := range g.Edges {
		a, b := gr.pos[e.From], gr.pos[e.To]
		var pts []Point
		seg := 0 // the run its label is on
		switch {
		case a.c == b.c && gr.clear(a, b):
			if b.r > a.r {
				pts = []Point{gr.port(e.From, 's'), gr.port(e.To, 'n')}
			} else {
				pts = []Point{gr.port(e.From, 'n'), gr.port(e.To, 's')}
			}
		case a.r == b.r && gr.clear(a, b):
			if b.c > a.c {
				pts = []Point{gr.port(e.From, 'e'), gr.port(e.To, 'w')}
			} else {
				pts = []Point{gr.port(e.From, 'w'), gr.port(e.To, 'e')}
			}
		case a.c == b.c: // beside the column: on its right going down, on its left going back up
			side := byte('e')
			if b.r < a.r {
				side = 'w'
			}
			x := gr.beside(a.c, side == 'e')
			p, q := gr.port(e.From, side), gr.port(e.To, side)
			pts, seg = []Point{p, {x, p.Y}, {x, q.Y}, q}, 1
		case a.r == b.r: // over the row
			p, q := gr.port(e.From, 'n'), gr.port(e.To, 'n')
			y := gr.rowT(a.r)
			pts, seg = []Point{p, {p.X, y}, {q.X, y}, q}, 1
		default:
			corner1, corner2 := cell{b.c, a.r}, cell{a.c, b.r}
			across := gr.free(corner1.c, corner1.r) && gr.clear(a, corner1) && gr.clear(corner1, b)
			down := gr.free(corner2.c, corner2.r) && gr.clear(a, corner2) && gr.clear(corner2, b)
			if k, ok := between(gr, a, b); !across && !down && ok {
				// Both corners taken: along its row to between two columns,
				// there up or down, and along the other's row to it.
				from, to := byte('e'), byte('w')
				if b.c < a.c {
					from, to = 'w', 'e'
				}
				p, q := gr.port(e.From, from), gr.port(e.To, to)
				x := gr.colL(k)
				pts = []Point{p, {x, p.Y}, {x, q.Y}, q}
			} else if across || !down { // along its row, then to the other
				from, to := byte('e'), byte('n')
				if b.c < a.c {
					from = 'w'
				}
				if b.r < a.r {
					to = 's'
				}
				p, q := gr.port(e.From, from), gr.port(e.To, to)
				pts = []Point{p, {q.X, p.Y}, q}
			} else { // along its column, then to the other
				from, to := byte('s'), byte('w')
				if b.r < a.r {
					from = 'n'
				}
				if b.c < a.c {
					to = 'e'
				}
				p, q := gr.port(e.From, from), gr.port(e.To, to)
				pts = []Point{p, {p.X, q.Y}, q}
			}
			if length(pts[0], pts[1]) < 22 {
				seg = 1
			}
		}
		out = append(out, Route{Edge: i, Points: pts, Anchor: seg})
	}
	// Two edges that go each other's way run side by side.
	shifted := map[int]float32{}
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if sameWay(out[i].Points, out[j].Points) {
				shift(out[i].Points, pairShift)
				shift(out[j].Points, -pairShift)
				shifted[i], shifted[j] = pairShift, -pairShift
			}
		}
	}
	for i := range out {
		seg := out[i].Anchor
		out[i].LabelAt, out[i].Anchor = labelPlace(out[i].Points, seg, shifted[i] < 0, len([]rune(g.Edges[out[i].Edge].Label)))
	}
	return out
}

// between finds the edge between two columns that a route may turn at,
// going along the row of each with no node in its way: the edge's column
// (the edge is its left one), and whether there is one.
func between(gr *grid, a, b cell) (int, bool) {
	lo, hi := min(a.c, b.c), max(a.c, b.c)
	for k := lo + 1; k <= hi; k++ {
		ok := true
		for c := lo + 1; c < hi; c++ { // a's row up to the edge, b's row from it
			onA := c < k == (a.c < b.c)
			if onA && !gr.free(c, a.r) || !onA && !gr.free(c, b.r) {
				ok = false
			}
		}
		if ok {
			return k, true
		}
	}
	return 0, false
}

// Crossed tells whether a route runs through a box other than the two it
// joins: a layout that failed.
func (l Layout) Crossed(g *Graph) bool {
	for _, r := range l.Routes {
		e := g.Edges[r.Edge]
		for i := 0; i+1 < len(r.Points); i++ {
			a, b := r.Points[i], r.Points[i+1]
			x0, x1, y0, y1 := min(a.X, b.X), max(a.X, b.X), min(a.Y, b.Y), max(a.Y, b.Y)
			for id, box := range l.Boxes {
				if id != e.From && id != e.To && x1 > box.X+1 && x0 < box.X+box.W-1 && y1 > box.Y+1 && y0 < box.Y+box.H-1 {
					return true
				}
			}
		}
	}
	return false
}

func length(a, b Point) float32 { return abs(a.X-b.X) + abs(a.Y-b.Y) }

func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// sameWay tells whether one line is the other backwards.
func sameWay(a, b []Point) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if length(a[i], b[len(b)-1-i]) > 0.5 {
			return false
		}
	}
	return true
}

// shift moves a line of straight runs aside by d: its upright runs along
// x, its level runs along y.
func shift(pts []Point, d float32) {
	upright := make([]bool, len(pts))
	level := make([]bool, len(pts))
	for i := 0; i+1 < len(pts); i++ {
		if abs(pts[i].X-pts[i+1].X) < 0.5 {
			upright[i], upright[i+1] = true, true
		} else {
			level[i], level[i+1] = true, true
		}
	}
	for i := range pts {
		if upright[i] {
			pts[i].X += d
		}
		if level[i] {
			pts[i].Y += d
		}
	}
}

// labelPlace is where a label of so many characters goes by a run of a
// line: over a level run, beside an upright one (on its left when left
// is set), at its middle when it is short and near its beginning when
// long.
func labelPlace(pts []Point, seg int, left bool, runes int) (Point, int) {
	if seg+1 >= len(pts) {
		seg = 0
	}
	a, b := pts[seg], pts[seg+1]
	long := length(a, b) > 110
	if abs(a.Y-b.Y) < 0.5 { // level
		dir := float32(1)
		if b.X < a.X {
			dir = -1
		}
		if length(a, b) < 64 && runes > 1 {
			// Too short to hold it: over where it begins, clear of the two
			// it joins (under, for the one of two that runs lower).
			y := a.Y - 30
			if left {
				y = a.Y + 40
			}
			if dir > 0 {
				return Point{a.X - 6, y}, AnchorStart
			}
			return Point{a.X + 6, y}, AnchorEnd
		}
		if left { // of two side by side, the one under
			return Point{(a.X + b.X) / 2, a.Y + 18}, AnchorMiddle
		}
		if long {
			if dir > 0 {
				return Point{a.X + 12, a.Y - 8}, AnchorStart
			}
			return Point{a.X - 12, a.Y - 8}, AnchorEnd
		}
		return Point{(a.X + b.X) / 2, a.Y - 8}, AnchorMiddle
	}
	y := (a.Y + b.Y) / 2
	if long {
		y = a.Y + 30
		if b.Y < a.Y {
			y = a.Y - 30
		}
	}
	if left {
		return Point{a.X - 7, y + 4}, AnchorEnd
	}
	return Point{a.X + 7, y + 4}, AnchorStart
}

// A flow as steps: its longest way from its beginning, a step a line in
// the order taken, and at each step the other ways out of it, each said
// in words: what it passes and where it ends, or which step it comes back
// to. A flow drawn with boxes and arrows says every "then" with a line,
// and it is the few that are not "then", the ways back and the leaps on,
// that cross the others: as steps the order says "then" for nothing, and
// a way back is a number to go to, not a line to follow.

// StepLine is a step of a flow's main way, and the other ways out of it.
type StepLine struct {
	Node  string
	Next  string // the label of the way on to the next step, when it has one
	Exits []Exit
}

// Exit is a way out of a step other than on to the next: by which edge,
// the nodes it passes off the main way, and where it comes to.
type Exit struct {
	Edge  int    // the edge taken out of the step
	Label string // what is asked of it, as the edge says
	Chain []int  // the edges passed after it, off the main way
	Pass  []string
	// To is the step of the main way it comes to (from 1), 0 when it ends
	// off it; Back tells that the step is this one or an earlier.
	To   int
	Back bool
	// Join is a node off the main way that another way out already told,
	// which this one comes to: told once, and named here.
	Join string
	// More are the ways out of the last node passed, when it has several:
	// a question off the main way.
	More []Exit
}

// Steps tells a flow as steps, and how tangled it is as a drawing: how
// many of its ways go back or leap over a step, which are those whose
// lines cross the others.
func (g *Graph) Steps() (steps []StepLine, tangled int) {
	fwd := forward(g)
	lanes := sideLanes(g, fwd)
	at := map[string]int{} // a node's step, from 1
	order := topo(g, fwd)
	// The main way again, in its order: the nodes sideLanes put first.
	start := ""
	for _, n := range g.Nodes {
		if n.Kind == KindStart {
			start = n.ID
			break
		}
	}
	if start == "" && len(order) > 0 {
		start = order[0]
	}
	onMain := func(id string) bool { return lanes[id] == 0 }
	for id := start; id != ""; {
		steps = append(steps, StepLine{Node: id})
		at[id] = len(steps)
		next := ""
		for i, e := range g.Edges {
			if fwd[i] && e.From == id && onMain(e.To) && at[e.To] == 0 && next == "" {
				next = e.To
			}
		}
		id = next
	}
	used := map[int]bool{}
	seen := map[string]bool{} // the nodes off the main way told already
	var exit func(i int) Exit
	exit = func(i int) Exit {
		used[i] = true
		x := Exit{Edge: i, Label: g.Edges[i].Label}
		for to := g.Edges[i].To; ; {
			if n := at[to]; n > 0 {
				x.To = n
				return x
			}
			if seen[to] { // told by another way out, or round and round
				x.Join = to
				return x
			}
			seen[to] = true
			x.Pass = append(x.Pass, to)
			var out []int
			for k, e := range g.Edges {
				if e.From == to {
					out = append(out, k)
				}
			}
			switch len(out) {
			case 0:
				return x
			case 1:
				used[out[0]] = true
				x.Chain = append(x.Chain, out[0])
				to = g.Edges[out[0]].To
			default:
				for _, k := range out {
					x.More = append(x.More, exit(k))
				}
				return x
			}
		}
	}
	for s := range steps {
		id := steps[s].Node
		for i, e := range g.Edges {
			if e.From != id || used[i] {
				continue
			}
			if s+1 < len(steps) && e.To == steps[s+1].Node && steps[s].Next == "" && !used[i] {
				used[i] = true
				steps[s].Next = e.Label
				if e.Label == "" {
					steps[s].Next = " " // it has one, with nothing said on it
				}
				continue
			}
			x := exit(i)
			x.Back = x.To > 0 && x.To <= s+1
			if x.To > 0 && (x.Back || x.To > s+2) {
				tangled++
			}
			steps[s].Exits = append(steps[s].Exits, x)
		}
	}
	// What no way from the main way reaches is told after it, each a step.
	for _, n := range g.Nodes {
		if at[n.ID] > 0 {
			continue
		}
		reached := false
		for _, st := range steps {
			var has func(xs []Exit) bool
			has = func(xs []Exit) bool {
				for _, x := range xs {
					if slices.Contains(x.Pass, n.ID) || has(x.More) {
						return true
					}
				}
				return false
			}
			reached = reached || has(st.Exits)
		}
		if !reached {
			steps = append(steps, StepLine{Node: n.ID})
			at[n.ID] = len(steps)
		}
	}
	return steps, tangled
}
