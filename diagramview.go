package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/diagram"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// The page of a topic's 图解 (diagram.go), in the canvas's place: its own
// head (the way back, the kinds, 重画), the diagram, and under it the
// three ways to read it. What is chosen in it says, in the right column,
// the topics of the map it was drawn from.

// diaKinds names the kinds of a set as its tabs do.
var diaKinds = map[string]string{"flow": "流程", "state": "状态", "table": "判定表", "objects": "名词"}

// diaLit is what of a diagram is lit while it is told or walked: nil
// lights all.
type diaLit struct {
	nodes map[string]bool // those lit
	cur   map[string]bool // those the step told is on
	edges map[int]bool
	hot   map[int]bool // the edges just taken
	miss  string       // the node a case cannot go on from
	rows  map[int]bool // of the table, from 1
}

// diaLight works out what is lit of the kind shown.
func (a *app) diaLight(f *diaFile) *diaLit {
	set := func(ids []string) map[string]bool {
		m := map[string]bool{}
		for _, id := range ids {
			m[id] = true
		}
		return m
	}
	ints := func(ids []int) map[int]bool {
		m := map[int]bool{}
		for _, id := range ids {
			m[id] = true
		}
		return m
	}
	switch {
	case a.dia.way == diaRead && a.dia.noun >= 1 && a.dia.noun <= len(f.Objects) && a.dia.kind != "objects":
		return a.nounLight(f, f.Objects[a.dia.noun-1], a.dia.kind)
	case a.dia.way == diaTell && a.dia.kind == "flow" && f.Flow != nil && len(f.Flow.Tell) > 0:
		step := min(a.dia.step, len(f.Flow.Tell)-1)
		l := &diaLit{nodes: map[string]bool{}, cur: set(f.Flow.Tell[step].Nodes), edges: map[int]bool{}, hot: map[int]bool{}}
		for _, st := range f.Flow.Tell[:step+1] {
			for _, id := range st.Nodes {
				l.nodes[id] = true
			}
		}
		for i, e := range f.Flow.Edges {
			if l.nodes[e.From] && l.nodes[e.To] {
				l.edges[i] = true
				l.hot[i] = l.cur[e.To]
			}
		}
		return l
	case a.dia.way == diaCase && a.dia.cas < len(f.Cases):
		cs := f.Cases[a.dia.cas]
		switch {
		case a.dia.kind == "flow" && f.Flow != nil:
			edges := ints(f.Flow.PathEdges(cs.Flow))
			l := &diaLit{nodes: set(cs.Flow), edges: edges, hot: edges}
			if cs.Gap && len(cs.Flow) > 0 {
				l.miss = cs.Flow[len(cs.Flow)-1]
			}
			return l
		case a.dia.kind == "state" && f.State != nil:
			path := cs.States
			if len(path) > 0 { // from the dot they begin at, where it leads to the first
				for _, n := range f.State.Nodes {
					if n.Kind == diagram.KindStart && len(f.State.PathEdges([]string{n.ID, path[0]})) > 0 {
						path = append([]string{n.ID}, path...)
					}
				}
			}
			edges := ints(f.State.PathEdges(path))
			return &diaLit{nodes: set(path), edges: edges, hot: edges}
		case a.dia.kind == "table":
			l := &diaLit{rows: map[int]bool{}}
			if cs.Row > 0 {
				l.rows[cs.Row] = true
			}
			return l
		}
	}
	return nil
}

// diagramView is the page of the set shown.
func (a *app) diagramView(c *ui.Context) {
	f := a.diaOf(a.dia.node)
	if f == nil || a.file.SheetOf(f.Node) != a.doc { // gone, or another canvas was turned to
		a.dia, a.mode = diaState{}, viewMap
		a.canvasView(c)
		return
	}
	if !slices.Contains(f.Kinds(), a.dia.kind) { // drawn again without it
		a.dia.kind, a.dia.pick, a.dia.way = f.Kinds()[0], "", diaRead
	}
	stage := ui.Column(c.Key("diagram")).Grow(1).Basis(0).MinHeight(0).Focusable().FocusRing(false).Bind(&a.diaStage).Label("图解")
	stage.HandleInput(a.diaKey)
	if a.diaFocus || stage.Pressed() {
		a.diaFocus = false
		a.diaStage.Focus()
	}
	stage.Children(func() {
		a.diaHead(c, f)
		a.diaStaleBar(c, f)
		if !a.panelShown(c) && a.dia.pick != "" {
			a.diaSourceStrip(c, f)
		}
		if a.diaBusy != f.Node {
			a.nounChip(c, f, a.diaLight(f))
		}
		switch {
		case a.diaBusy == f.Node:
			ui.Column(c.Key("dia-busy")).Grow(1).Basis(0).Center().Children(func() {
				ui.Box(c).Width(300).Children(func() { a.diaStages(c, f.Node) })
			})
		case a.dia.kind == "flow":
			a.diaGraph(c, f, f.Flow, true)
		case a.dia.kind == "state":
			a.diaGraph(c, f, f.State, false)
		case a.dia.kind == "table":
			a.diaTable(c, f)
		default:
			a.diaObjects(c, f)
		}
		if a.diaBusy != f.Node {
			a.diaWays(c, f)
		}
		a.sheetTabs(c)
	})
}

// diaKey is the keys of the page: Escape leaves a way of reading, then
// what is chosen, then the page; the arrows step through the telling.
func (a *app) diaKey(ev ui.InputEvent) bool {
	if ev.Kind != ui.InputKeyDown || ev.Mods != 0 {
		return false
	}
	f := a.diaOf(a.dia.node)
	if f == nil {
		return false
	}
	switch ev.Key {
	case ui.KeyEscape:
		switch {
		case a.dia.way != diaRead:
			a.dia.way = diaRead
		case a.dia.pick != "":
			a.dia.pick = ""
		case a.dia.noun != 0:
			a.dia.noun = 0
		default:
			a.later(a.closeDia)
		}
	case ui.KeyRight, ui.KeyDown, ui.KeyLeft, ui.KeyUp:
		by := 1
		if ev.Key == ui.KeyLeft || ev.Key == ui.KeyUp {
			by = -1
		}
		switch a.dia.way {
		case diaTell:
			a.tellStep(f, by)
		case diaRead:
			a.diaMove(f, ev.Key)
		default:
			return false
		}
	case ui.KeySpace, ui.KeyEnter:
		if a.dia.way != diaTell {
			return false
		}
		a.tellStep(f, 1)
	default:
		return false
	}
	return true
}

// diaMove takes what is chosen to the next by an arrow: on a graph, the
// box nearest that way; in the table and the nouns, the row above or
// below.
func (a *app) diaMove(f *diaFile, key ui.Key) {
	step := func(prefix string, n int) {
		at, _ := strconv.Atoi(strings.TrimPrefix(a.dia.pick, prefix))
		if !strings.HasPrefix(a.dia.pick, prefix) {
			at = 0
		}
		switch key {
		case ui.KeyDown:
			at++
		case ui.KeyUp:
			at--
		default:
			return
		}
		if at >= 1 && at <= n {
			a.dia.pick = prefix + strconv.Itoa(at)
		}
	}
	g := a.diaGraphOf(f)
	switch {
	case a.dia.kind == "table" && f.Table != nil:
		step("row:", len(f.Table.Rows))
		return
	case a.dia.kind == "objects":
		step("obj:", len(f.Objects))
		return
	case g == nil:
		return
	}
	l := diaLayout(g, a.dia.kind == "flow")
	cur, ok := l.Boxes[a.dia.pick]
	if !ok || g.Node(a.dia.pick) == nil { // none chosen: where it begins
		for _, n := range g.Nodes {
			if n.Kind != diagram.KindStart || a.dia.kind == "flow" {
				a.dia.pick, a.diaReveal = n.ID, n.ID
				return
			}
		}
		return
	}
	dx, dy := float32(0), float32(0)
	switch key {
	case ui.KeyRight:
		dx = 1
	case ui.KeyLeft:
		dx = -1
	case ui.KeyDown:
		dy = 1
	default:
		dy = -1
	}
	best, bestAt := "", float32(0)
	for _, n := range g.Nodes {
		b := l.Boxes[n.ID]
		if n.ID == a.dia.pick || n.Kind == diagram.KindStart && a.dia.kind == "state" {
			continue
		}
		ox, oy := b.X+b.W/2-(cur.X+cur.W/2), b.Y+b.H/2-(cur.Y+cur.H/2)
		along, across := ox*dx+oy*dy, abs32(ox*dy)+abs32(oy*dx)
		if along <= 1 {
			continue
		}
		if d := along + 2*across; best == "" || d < bestAt { // that way first, then near
			best, bestAt = n.ID, d
		}
	}
	if best != "" {
		a.dia.pick, a.diaReveal = best, best
	}
}

// tellStep goes a step on or back in the telling; past its end, it is
// over.
func (a *app) tellStep(f *diaFile, by int) {
	if f.Flow == nil {
		return
	}
	switch n := a.dia.step + by; {
	case n >= len(f.Flow.Tell):
		a.dia.way = diaRead
	case n >= 0:
		a.dia.step = n
		a.diaReveal = f.Flow.Tell[n].Nodes[0]
	}
}

// diaHead is the page's own bar: the way back to the map, whose topic it
// is of, its kinds, and what draws it again or forgets it.
func (a *app) diaHead(c *ui.Context, f *diaFile) {
	t := c.Theme()
	title := f.Title
	if n := a.file.Node(f.Node); n != nil {
		title = n.Title
	}
	ui.Row(c.Key("dia-head")).Height(44).Padding(0, 12, 6, 16).Gap(10).AlignItems(ui.Center).Shrink(0).Children(func() {
		back := key(ui.Row(c.Key("dia-back")).Height(30).Padding(0, 12, 0, 8).Gap(4).Radius(radiusPill).AlignItems(ui.Center).
			Cursor(ui.CursorPointer).Label("回到导图").Tooltip("回到导图 Esc").Shrink(0), t)
		if back.Clicked() {
			a.later(a.closeDia)
		}
		back.Children(func() {
			glyph(c, iconBack, 12, t.TextMuted)
			ui.Text(c, "导图").FontSize(textSmall).PassThrough()
		})
		ui.Text(c, title).FontSize(textBody + 1).FontWeight(700).NoWrap().MaxWidth(180).Ellipsis("…")
		ui.Text(c, "的图解").FontSize(textCaption).TextColor(t.TextMuted).NoWrap().Margin(0, 0, 0, -4)
		ui.Spacer(c)
		kinds := f.Kinds()
		names := make([]string, len(kinds))
		for i, k := range kinds {
			names[i] = diaKinds[k]
		}
		ui.Row(c.Key("dia-kinds-room")).Width(float32(len(kinds)) * 68).Shrink(0).Children(func() {
			at := slices.Index(kinds, a.dia.kind)
			if now := segmented(c, "dia-kinds", "图解", names, at); now != at {
				a.dia.kind, a.dia.pick = kinds[now], ""
				if a.dia.way == diaTell || a.dia.way == diaQuiz || kinds[now] == "objects" {
					a.dia.way = diaRead // told on the flow, asked on the table
				}
			}
		})
		ui.Spacer(c)
		busy := a.diaBusy != ""
		again := busyKey(c, "重画", "照现在的导图再画一遍", busy, true, false)
		if busyClicked(again, busy, true) {
			a.later(func() { a.drawDia(f.Node, f.Told) })
		}
		iconButton(c, iconMore, "图解的更多操作", "更多", 30, true).Menu(func(m *ui.Menu) {
			if m.Item("回到导图").Chosen() {
				a.later(a.closeDia)
			}
			if m.Item("复制成 Markdown").Chosen() {
				a.plat.CopyText(f.Markdown())
				a.tell("已复制：流程和状态是 Mermaid，贴到能画它的地方就是图")
			}
			m.Separator()
			if m.Item("删掉这组图解").Chosen() {
				a.later(func() { a.deleteDia(f.Node) })
			}
		})
	})
}

// diaStaleBar says that the map changed since the set was drawn, and
// what did not get drawn when it was tried.
func (a *app) diaStaleBar(c *ui.Context, f *diaFile) {
	t := c.Theme()
	line := func(k, text, act string, do func()) {
		ui.Row(c.Key(k)).Margin(0, 16, 8).Padding(8, 12).Gap(10).Radius(radiusPill).AlignItems(ui.Center).Shrink(0).
			Background(blockFill(t)).Children(func() {
			ui.Box(c).Size(8, 8).Radius(4).Background(t.Warning).Shrink(0)
			ui.Text(c, text).FontSize(textSmall).TextColor(ink2(t)).Grow(1).Shrink(1).MinWidth(0).SingleLine()
			if textKey(c, act, k+act, "").Clicked() {
				a.later(do)
			}
		})
	}
	if a.diaErr != "" && a.diaErrFor == f.Node && a.diaBusy == "" {
		line("dia-err", a.diaErr, "再试一次", func() { a.drawDia(f.Node, f.Told) })
	}
	if ch := a.diaChanges(f); ch.n > 0 && a.diaBusy != f.Node {
		text := fmt.Sprintf("画完之后导图改过 %d 处", ch.n)
		if ch.first != "" {
			text += "：「" + ch.first + "」"
			if ch.n > 1 {
				text += "等"
			}
		}
		line("dia-stale", text+"。图可能旧了。", "重画", func() { a.drawDia(f.Node, f.Told) })
	}
}

// diaStages says what the drawing is doing, which cannot be counted.
func (a *app) diaStages(c *ui.Context, node string) {
	n := a.file.Node(node)
	if n == nil {
		return
	}
	read := fmt.Sprintf("读「%s」和下面 %d 个主题", n.Title, countTopics(n)-1)
	if a.now().Sub(a.diaSince).Seconds() < 2 {
		stageLines(c, "diagram", nil, read)
		return
	}
	stageLines(c, "diagram", []string{read}, "AI 正在找出先后、条件和状态，排成图…")
}

// diaBlue is the color of a diagram's steps and states: one calm blue on
// every map. In its topic's own color a step read, on a map whose color
// is near red or green, as an end refused or an end reached.
var diaBlue = ui.Hex("#3B8FE0")

// diaLayout places the graph shown.
func diaLayout(g *diagram.Graph, flow bool) diagram.Layout {
	if flow {
		return g.LayoutFlow()
	}
	return g.LayoutState()
}

// diaGraph draws a flow or the states: the lanes and the arrows painted,
// each node a box to choose. It fits the page's width, and is made
// larger or smaller by the bar at its corner.
func (a *app) diaGraph(c *ui.Context, f *diaFile, g *diagram.Graph, flow bool) {
	t := c.Theme()
	l := diaLayout(g, flow)
	lit := a.diaLight(f)
	changed := a.diaChanges(f).changed
	const pad = 20
	fit := float32(1)
	if room := a.diaBody.Bounds(c).W - 2*pad; room > 0 {
		fit = max(min(room/l.Width, 1), 0.72) // no smaller than reads
	} else {
		c.Invalidate() // once more, with the page's width
	}
	z := fit
	if a.diaZoom > 0 {
		z = a.diaZoom
	}
	w, h := l.Width*z, l.Height*z
	left := max((a.diaBody.Bounds(c).W-w)/2, pad)
	dim := func(on bool) float32 {
		if on {
			return 1
		}
		return 0.2
	}
	nodeLit := func(id string) bool { return lit == nil || lit.nodes == nil || lit.nodes[id] }
	edgeLit := func(i int) bool { return lit == nil || lit.edges == nil || lit.edges[i] }
	// The steps of the case walked, numbered as it takes them.
	order := map[string]int{}
	if a.dia.way == diaCase && flow && a.dia.cas < len(f.Cases) {
		for _, id := range f.Cases[a.dia.cas].Flow {
			if _, seen := order[id]; !seen {
				order[id] = len(order) + 1
			}
		}
	}
	hover := a.diaHover
	a.diaHover = ""
	ui.Column(c.Key("dia-graph-room")).Grow(1).Basis(0).MinHeight(0).Children(func() {
		ui.ScrollBoth(c.Key("dia-graph:" + a.dia.kind)).Grow(1).Basis(0).MinHeight(0).Bind(&a.diaBody).Children(func() {
			cv := ui.Box(c.Key("dia-canvas")).Size(w, h).Margin(6, pad, pad, left).Shrink(0).Label("图")
			if cv.Clicked() {
				a.dia.pick = ""
			}
			cv.Draw(func(p *ui.Painter, r ui.Rect) {
				x := func(v float32) float32 { return r.X + v*z }
				y := func(v float32) float32 { return r.Y + v*z }
				for i, lane := range l.Lanes {
					if i > 0 {
						p.Line(x(lane.X), y(6), x(lane.X), y(l.Height-6), 1, hairline(t))
					}
				}
				if l.Head > 0 {
					p.Line(x(0), y(l.Head), x(l.Width), y(l.Head), 1, hairline(t))
				}
				for _, rt := range l.Routes {
					ink := faint(t)
					e := g.Edges[rt.Edge]
					if lit != nil && lit.hot[rt.Edge] || lit == nil && a.dia.pick != "" && (e.From == a.dia.pick || e.To == a.dia.pick) {
						ink = t.Accent // taken by what is told or walked, or of the box chosen: followed among many
					}
					ink = ink.Alpha(dim(edgeLit(rt.Edge)))
					pts := make([][2]float32, len(rt.Points))
					for i, q := range rt.Points {
						pts[i] = [2]float32{x(q.X), y(q.Y)}
					}
					p.StrokePolyline(pts, max(1.4*z, 1), ink)
					// Its arrow, along its last run.
					n := len(pts)
					ex, ey := pts[n-1][0], pts[n-1][1]
					dx, dy := ex-pts[n-2][0], ey-pts[n-2][1]
					if d := abs32(dx) + abs32(dy); d > 0 {
						dx, dy = dx/d, dy/d
						s := 7 * z
						p.FillPath(new(ui.Path).MoveTo(ex, ey).LineTo(ex-dx*s-dy*s/2, ey-dy*s+dx*s/2).LineTo(ex-dx*s+dy*s/2, ey-dy*s-dx*s/2).Close(), ink)
					}
				}
				// A question is a diamond, which no box is.
				for _, n := range g.Nodes {
					if n.Kind != diagram.KindDecision {
						continue
					}
					b := l.Boxes[n.ID]
					dia := new(ui.Path).MoveTo(x(b.X), y(b.Y+b.H/2)).LineTo(x(b.X+b.W/2), y(b.Y)).LineTo(x(b.X+b.W), y(b.Y+b.H/2)).LineTo(x(b.X+b.W/2), y(b.Y+b.H)).Close()
					o := dim(nodeLit(n.ID))
					fill := t.Background
					if hover == n.ID {
						fill = diaBlue.Alpha(0.09)
					}
					p.FillPath(dia, fill)
					line, lw := diaBlue.Alpha(0.6*o), max(1.5*z, 1)
					switch {
					case lit != nil && lit.miss == n.ID:
						line, lw = t.Warning, 2
					case a.dia.pick == n.ID || lit != nil && lit.cur[n.ID]:
						line, lw = t.Accent, 2
					}
					p.StrokePath(dia, lw, line)
				}
			})
			cv.Children(func() {
				for i, name := range g.Lanes {
					ui.Box(c.Key(fmt.Sprint("lane", i))).Absolute().Left(l.Lanes[i].X*z).Top(0).Size(l.Lanes[i].W*z, l.Head*z).Center().PassThrough().Children(func() {
						ui.Text(c, name).FontSize(12 * z).FontWeight(700).LetterSpacing(0.4).TextColor(t.TextMuted).NoWrap().PassThrough()
					})
				}
				for _, rt := range l.Routes {
					if e := g.Edges[rt.Edge]; e.Label != "" {
						a.diaEdgeLabel(c, rt, e.Label, z, dim(edgeLit(rt.Edge)),
							lit != nil && lit.hot[rt.Edge] || lit == nil && a.dia.pick != "" && (e.From == a.dia.pick || e.To == a.dia.pick))
					}
				}
				for _, n := range g.Nodes {
					b := l.Boxes[n.ID]
					if !flow && n.Kind == diagram.KindStart { // the dot the states begin at
						ui.Box(c.Key(n.ID)).Absolute().Left(b.X*z).Top(b.Y*z).Size(b.W*z, b.H*z).Radius(b.W * z).Background(t.Text).
							Opacity(dim(nodeLit(n.ID))).Transition(easeLook).PassThrough()
						continue
					}
					a.diaNode(c, n, b, z, flow, lit, dim(nodeLit(n.ID)), slices.ContainsFunc(n.Src, func(id string) bool { return changed[id] }), order[n.ID])
				}
			})
		})
		// Larger and smaller, over the diagram's corner as over the canvas's.
		lifted(ui.Row(c.Key("dia-zoom")).Absolute().Right(14).Bottom(8).Gap(0).Padding(3, 5).Radius(radiusPill).AlignItems(ui.Center).
			Background(cardColor(t)), t).Children(func() {
			if iconButton(c, iconMinus, "缩小图解", "缩小", 28, z > 0.5).Clicked() {
				a.diaZoom = max(float32(int(z*10+0.5)-1)/10, 0.5)
			}
			fits := ui.Box(c.Key("dia-fit")).Height(28).MinWidth(48).Padding(0, 8).Radius(6).Center().Cursor(ui.CursorPointer).
				Label("图解的大小").Tooltip("适合页面的宽度")
			if fits.Hovered() {
				fits.Background(t.SurfaceHover)
			}
			if fits.Clicked() {
				a.diaZoom = 0
			}
			fits.Children(func() {
				ui.Text(c, fmt.Sprintf("%d%%", int(z*100+0.5))).FontSize(11).TextColor(t.TextMuted).PassThrough()
			})
			if iconButton(c, iconPlus, "放大图解", "放大", 28, z < 1.6).Clicked() {
				a.diaZoom = min(float32(int(z*10+0.5)+1)/10, 1.6)
			}
		})
	})
}

// nounChip says, over a diagram, the noun it is lit for, with the way out
// of it.
func (a *app) nounChip(c *ui.Context, f *diaFile, lit *diaLit) {
	if a.dia.way != diaRead || a.dia.noun < 1 || a.dia.noun > len(f.Objects) || a.dia.kind == "objects" {
		return
	}
	t := c.Theme()
	text := "只亮提到「" + f.Objects[a.dia.noun-1].Name + "」的"
	if lit != nil && len(lit.nodes) == 0 && len(lit.rows) == 0 {
		text = "这张图上没有提到「" + f.Objects[a.dia.noun-1].Name + "」"
	}
	chip := ui.Row(c.Key("noun-chip")).Margin(0, 16, 6).Height(28).Padding(0, 8, 0, 10).Gap(6).Radius(radiusPill).AlignItems(ui.Center).
		AlignSelf(ui.Start).Shrink(0).Background(t.Accent.Alpha(0.12)).Cursor(ui.CursorPointer).Label("不只亮这个名词了").Tooltip("亮回全部")
	if chip.Clicked() {
		a.dia.noun = 0
	}
	chip.Children(func() {
		ui.Text(c, text).FontSize(textCaption).FontWeight(700).TextColor(t.Accent).NoWrap().PassThrough()
		glyph(c, iconClose, 10, t.Accent)
	})
}

// diaNode is a box of a graph: a step, an end, a state; of a question,
// what is written in its diamond. ord, from 1, is its place in the case
// walked.
func (a *app) diaNode(c *ui.Context, n diagram.Node, b diagram.Rect, z float32, flow bool, lit *diaLit, opacity float32, stale bool, ord int) {
	t := c.Theme()
	el := ui.Column(c.Key(n.ID)).Absolute().Left(b.X*z).Top(b.Y*z).Size(b.W*z, b.H*z).Center().Gap(1 * z).Radius(radiusPill * z).
		Cursor(ui.CursorPointer).Label(n.Title).Opacity(opacity).Transition(easeLook)
	if a.diaReveal == n.ID { // told or walked to, and maybe off the page
		a.diaReveal = ""
		el.ScrollIntoView()
	}
	hovered := el.Hovered()
	if hovered {
		a.diaHover = n.ID
	}
	tint := float32(0.14)
	if t.Dark {
		tint = 0.24
	}
	if hovered {
		tint += 0.09
	}
	ink, sub := t.Text, ink2(t)
	bold := false
	switch n.Kind {
	case diagram.KindDecision: // its diamond is painted (diaGraph)
	case diagram.KindStart:
		el.Background(t.Text)
		ink, sub, bold = t.Background, t.Background.Alpha(0.7), true
	case diagram.KindEnd:
		el.Background(t.Text.Alpha(0.045 + (tint-0.14)/2))
		ink = ink2(t)
	case diagram.KindOK:
		el.Background(t.Success.Alpha(tint + 0.04))
	case diagram.KindBad:
		el.Background(t.Danger.Alpha(tint - 0.01))
	default:
		el.Background(diaBlue.Alpha(tint))
	}
	if n.Kind != diagram.KindDecision {
		switch {
		case lit != nil && lit.miss == n.ID:
			el.Border(2, t.Warning)
		case a.dia.pick == n.ID || lit != nil && lit.cur[n.ID]:
			el.Border(2, t.Accent)
		default:
			el.Border(2, t.Text.Alpha(0))
		}
	}
	// What a line of it holds; a longer title takes two.
	line := diagram.TitleRunes
	switch {
	case n.Kind == diagram.KindDecision:
		line = diagram.DiamondRunes
	case !flow:
		line = 6
	}
	long := len([]rune(n.Title)) > line
	if len([]rune(n.Title)) > 2*line || len([]rune(n.Sub)) > 12 {
		el.Tooltip(strings.TrimSpace(n.Title + "\n" + n.Sub))
	}
	if el.Clicked() {
		switch {
		case a.dia.way == diaTell && flow: // to the step that tells it
			if f := a.diaOf(a.dia.node); f != nil && f.Flow != nil {
				for i, st := range f.Flow.Tell {
					if slices.Contains(st.Nodes, n.ID) {
						a.dia.step = i
						break
					}
				}
			}
		case a.dia.pick == n.ID:
			a.dia.pick = ""
		default:
			a.dia.pick = n.ID
		}
	}
	el.Children(func() {
		wide := (b.W - 14) * z
		if n.Kind == diagram.KindDecision {
			wide = (b.W - 44) * z // within the diamond
		}
		size := 14 * z
		if !flow && long {
			size = 13 * z
		}
		title := ui.Text(c, n.Title).FontSize(size).TextColor(ink).MaxWidth(wide).PassThrough()
		if long {
			title.Wrap().MaxLines(2).LineHeight(1.2).TextAlign(ui.Center)
		} else {
			title.NoWrap().Ellipsis("…")
		}
		if bold {
			title.FontWeight(700)
		}
		if n.Sub != "" && n.Kind != diagram.KindDecision {
			ui.Text(c, n.Sub).FontSize(11.5 * z).TextColor(sub).NoWrap().MaxWidth(wide).Ellipsis("…").PassThrough()
		}
		if stale { // what it was drawn from has changed
			ui.Box(c.Key("stale")).Absolute().Right(-3).Top(-3).Size(10, 10).Radius(5).Background(t.Warning).Border(2, t.Background).PassThrough()
		}
		if ord > 0 { // which step of the case it is
			ui.Box(c.Key("ord")).Absolute().Left(-7).Top(-7).Size(18, 18).Radius(9).Background(t.Accent).Border(2, t.Background).Center().PassThrough().Children(func() {
				ui.Text(c, strconv.Itoa(ord)).FontSize(10).FontWeight(700).TextColor(t.AccentText).FontFeatures("tnum").PassThrough()
			})
		}
	})
}

// diaEdgeLabel writes an arrow's label by it, on the leaf's white so that
// a line under it does not cross its words.
func (a *app) diaEdgeLabel(c *ui.Context, rt diagram.Route, text string, z, opacity float32, hot bool) {
	t := c.Theme()
	const wide = 160
	x, y := rt.LabelAt.X*z, rt.LabelAt.Y*z-13*z
	box := ui.Row(c.Key(fmt.Sprint("label", rt.Edge))).Absolute().Top(y).Width(wide).PassThrough().Opacity(opacity).Transition(easeLook)
	switch rt.Anchor {
	case diagram.AnchorStart:
		box.Left(x).Justify(ui.Start)
	case diagram.AnchorEnd:
		box.Left(x - wide).Justify(ui.End)
	default:
		box.Left(x - wide/2).Justify(ui.Center)
	}
	ink := ink2(t)
	if hot {
		ink = t.Accent
	}
	box.Children(func() {
		ui.Text(c, text).FontSize(12*z).TextColor(ink).NoWrap().Background(t.Background).Padding(0, 3).Radius(3).PassThrough()
	})
}

// diaTable is the table of what is decided when: a row a case, to choose;
// asked back, its results are covered until a row is opened.
func (a *app) diaTable(c *ui.Context, f *diaFile) {
	t := c.Theme()
	tb := f.Table
	lit := a.diaLight(f)
	changed := a.diaChanges(f).changed
	quiz := a.dia.way == diaQuiz
	cell := func(k any, text string, head bool) ui.Element {
		b := ui.Box(c.Key(k)).Grow(1).Basis(0).MinWidth(0).Padding(0, 8)
		b.Children(func() {
			txt := ui.Text(c, text).FontSize(textSmall + 0.5).Wrap().MaxLines(2).LineHeight(1.35).PassThrough()
			switch {
			case head:
				txt.FontSize(textCaption).TextColor(t.TextMuted)
			case text == "不限" || text == "—" || text == "":
				txt.TextColor(faint(t))
			}
		})
		return b
	}
	ui.Scroll(c.Key("dia-table")).Grow(1).Basis(0).MinHeight(0).Bind(&a.diaBody).Children(func() {
		ui.Column(c.Key("table")).MaxWidth(860).FillWidth().Padding(4, 20, 20).AlignSelf(ui.Center).Children(func() {
			ui.Row(c.Key("groups")).Padding(4, 0, 0, 22).FillWidth().Children(func() {
				ui.Box(c).Grow(float32(len(tb.Conds))).Basis(0).Padding(0, 8).Children(func() {
					ui.Text(c, "什么情况").FontSize(textCaption).FontWeight(700).LetterSpacing(0.4).TextColor(t.TextMuted)
				})
				ui.Box(c).Grow(float32(len(tb.Results))*1.3).Basis(0).Padding(0, 8, 0, 14).Children(func() {
					ui.Text(c, "怎么办").FontSize(textCaption).FontWeight(700).LetterSpacing(0.4).TextColor(t.TextMuted)
				})
			})
			ui.Row(c.Key("heads")).Padding(6, 0, 8, 22).FillWidth().BorderWidth(0, 0, 1, 0).BorderColor(hairline(t)).Children(func() {
				for i, s := range tb.Conds {
					cell(fmt.Sprint("c", i), s, true)
				}
				for i, s := range tb.Results {
					cell(fmt.Sprint("r", i), s, true).Grow(1.3).Padding(0, 8, 0, b2f(i == 0)*6+8)
				}
			})
			for i, r := range tb.Rows {
				no := i + 1
				pick := "row:" + strconv.Itoa(no)
				hit := lit != nil && lit.rows[no]
				row := ui.Row(c.Key(pick)).MinHeight(44).Padding(5, 0).FillWidth().AlignItems(ui.Center).Cursor(ui.CursorPointer).
					BorderWidth(0, 0, 1, 0).BorderColor(hairline(t)).Background(t.Text.Alpha(0)).Transition(rowMotion).
					Label(fmt.Sprintf("第 %d 条", no))
				switch {
				case a.dia.pick == pick || hit:
					row.Background(t.Accent.Alpha(0.1))
				case r.Gap:
					row.Background(t.Warning.Alpha(0.07))
				case row.Hovered():
					row.Background(t.Text.Alpha(0.035))
				}
				if lit != nil && lit.rows != nil && !hit {
					row.Opacity(0.25)
				}
				covered := quiz && !r.Gap && !a.dia.open[no]
				if row.Clicked() {
					switch {
					case quiz && r.Gap:
					case quiz:
						if a.dia.open == nil {
							a.dia.open, a.dia.got = map[int]bool{}, map[int]bool{}
						}
						a.dia.open[no], a.dia.cur = true, no
					case a.dia.pick == pick:
						a.dia.pick = ""
					default:
						a.dia.pick = pick
					}
				}
				row.Children(func() {
					ui.Text(c, strconv.Itoa(no)).FontSize(textCaption).TextColor(faint(t)).FontFeatures("tnum").Width(22).Shrink(0).TextAlign(ui.Center).PassThrough()
					for k, s := range r.When {
						b := cell(fmt.Sprint("w", k), s, false)
						if k == 0 && slices.ContainsFunc(r.Src, func(id string) bool { return changed[id] }) {
							b.Children(func() {
								ui.Box(c).Absolute().Left(-2).Top(5).Size(7, 7).Radius(4).Background(t.Warning)
							})
						}
					}
					for k, s := range r.Then {
						b := ui.Row(c.Key(fmt.Sprint("t", k))).Grow(1.3).Basis(0).MinWidth(0).MinHeight(44).AlignSelf(ui.Stretch).
							Padding(0, 8, 0, b2f(k == 0)*6+8).Gap(7).AlignItems(ui.Center).PassThrough()
						if k == 0 {
							b.BorderWidth(0, 0, 0, 1).BorderColor(hairline(t))
						}
						b.Children(func() {
							switch {
							case covered:
								ui.Box(c).Size(64, 16).Radius(5).Background(t.Text.Alpha(0.09))
							case r.Gap && k == 0:
								ui.Box(c).Size(8, 8).Radius(4).Background(t.Warning).Shrink(0)
								ui.Text(c, "导图里没写").FontSize(textSmall + 0.5).TextColor(t.Warning).SingleLine()
							case r.Gap:
								ui.Text(c, "？").FontSize(textSmall + 0.5).TextColor(faint(t))
							default:
								txt := ui.Text(c, cmpOr(s, "—")).FontSize(textSmall + 0.5).Wrap().MaxLines(2).LineHeight(1.35).Shrink(1).MinWidth(0)
								if s == "" || s == "—" {
									txt.TextColor(faint(t))
								}
							}
						})
					}
				})
			}
			if len(tb.Notes) > 0 {
				ui.Text(c, "另外").FontSize(textSmall).FontWeight(700).Margin(20, 8, 2)
				for i, n := range tb.Notes {
					pick := "note:" + strconv.Itoa(i+1)
					row := ui.Row(c.Key(pick)).FillWidth().Padding(8).Radius(radiusRow).Cursor(ui.CursorPointer).Label(n.Text).
						Background(t.Text.Alpha(0)).Transition(easeLook)
					switch {
					case a.dia.pick == pick:
						row.Background(t.Accent.Alpha(0.1))
					case row.Hovered():
						row.Background(t.Text.Alpha(0.04))
					}
					if lit != nil && lit.rows != nil {
						row.Opacity(0.25)
					}
					if row.Clicked() && !quiz {
						a.dia.pick = pick
					}
					row.Children(func() {
						ui.Text(c, n.Text).FontSize(textSmall).TextColor(ink2(t)).Wrap().LineHeight(1.5).PassThrough()
					})
				}
			}
		})
	})
}

// diaObjects lists the nouns of the matter, by their kind: each with a
// line on what it is, and in how many places the map speaks of it.
func (a *app) diaObjects(c *ui.Context, f *diaFile) {
	t := c.Theme()
	var kinds []string
	for _, o := range f.Objects {
		if k := cmpOr(o.Kind, "其他"); !slices.Contains(kinds, k) {
			kinds = append(kinds, k)
		}
	}
	ui.Scroll(c.Key("dia-objects")).Grow(1).Basis(0).MinHeight(0).Bind(&a.diaBody).Children(func() {
		ui.Column(c.Key("objects")).MaxWidth(700).FillWidth().Padding(0, 20, 20).AlignSelf(ui.Center).Children(func() {
			for _, k := range kinds {
				n := 0
				for _, o := range f.Objects {
					if cmpOr(o.Kind, "其他") == k {
						n++
					}
				}
				trayHead(c, "kind:"+k, k, strconv.Itoa(n), nil).Margin(10, 0, 0)
				for i, o := range f.Objects {
					if cmpOr(o.Kind, "其他") != k {
						continue
					}
					pick := "obj:" + strconv.Itoa(i+1)
					row := listCardRow(c, pick).Cursor(ui.CursorPointer).Label("名词 " + o.Name)
					if a.dia.pick == pick {
						row.Background(t.Accent.Alpha(0.1))
					}
					if row.Clicked() {
						if a.dia.pick == pick {
							a.dia.pick = ""
						} else {
							a.dia.pick = pick
						}
					}
					row.Children(func() {
						ui.Text(c, o.Name).FontSize(textBody + 0.5).FontWeight(700).NoWrap().Shrink(0).MinWidth(96).PassThrough()
						ui.Text(c, o.What).FontSize(textSmall).TextColor(ink2(t)).Grow(1).Shrink(1).MinWidth(0).Wrap().MaxLines(2).LineHeight(1.45).PassThrough()
						if m := len(a.mentions(o)); m > 0 {
							ui.Text(c, fmt.Sprintf("%d 处", m)).FontSize(textCaption).TextColor(t.TextMuted).FontFeatures("tnum").NoWrap().PassThrough()
						}
						glyph(c, iconForward, 10, faint(t))
					})
				}
			}
		})
	})
}

// diaWays is the row under a diagram: the three ways to read it, and,
// taken one, what steps through it.
func (a *app) diaWays(c *ui.Context, f *diaFile) {
	t := c.Theme()
	canTell := f.Flow != nil && len(f.Flow.Tell) > 0
	canCase := len(f.Cases) > 0
	canQuiz := len(quizRows(f.Table)) > 0
	if !canTell && !canCase && !canQuiz {
		return
	}
	off := func() {
		if iconButton(c, iconClose, "不这样看了", "不这样看了 Esc", 30, true).Clicked() {
			a.dia.way = diaRead
		}
	}
	chip := func(k, text string, on bool) ui.Element {
		b := ui.Box(c.Key(k)).Height(30).Padding(0, 12).Radius(radiusPill).Center().Shrink(0).Cursor(ui.CursorPointer).Label(text).Transition(easeLook)
		ink := ink2(t)
		switch {
		case on:
			b.Background(t.Accent.Alpha(0.12))
			ink = t.Accent
		case b.Hovered():
			b.Background(t.Text.Alpha(0.09))
		default:
			b.Background(t.Text.Alpha(0.05))
		}
		b.Children(func() {
			txt := ui.Text(c, text).FontSize(textSmall).TextColor(ink).NoWrap().PassThrough()
			if on {
				txt.FontWeight(700)
			}
		})
		return press(b, 0.97)
	}
	row := ui.Row(c.Key(fmt.Sprint("dia-ways", a.dia.way))).MinHeight(48).Margin(0, 16, 6).Padding(4, 12).Gap(8).Radius(radiusCard).AlignItems(ui.Center).Justify(ui.Center).Shrink(0)
	if a.dia.way != diaRead {
		row.Background(blockFill(t))
	}
	row.Children(func() {
		switch a.dia.way {
		case diaTell:
			if !canTell {
				a.dia.way = diaRead
				return
			}
			step := min(a.dia.step, len(f.Flow.Tell)-1)
			if iconButton(c, iconBack, "上一步", "上一步 ←", 30, step > 0).Clicked() && step > 0 {
				a.tellStep(f, -1)
			}
			ui.Text(c, fmt.Sprintf("%d / %d", step+1, len(f.Flow.Tell))).FontSize(textCaption).TextColor(t.TextMuted).FontFeatures("tnum").NoWrap()
			ui.Text(c, f.Flow.Tell[step].Say).FontSize(textSmall + 0.5).Shrink(1).MinWidth(0).Wrap().MaxLines(2).LineHeight(1.4)
			next := "下一步"
			if step == len(f.Flow.Tell)-1 {
				next = "讲完了"
			}
			if accentButton(c, next, "→").Clicked() {
				a.tellStep(f, 1)
			}
			off()
		case diaCase:
			if !canCase {
				a.dia.way = diaRead
				return
			}
			for i, cs := range f.Cases {
				if chip(fmt.Sprint("case", i), cs.Title, i == a.dia.cas).Clicked() {
					a.dia.cas, a.dia.pick = i, ""
					a.revealCase(f)
				}
			}
			off()
		case diaQuiz:
			rows := quizRows(f.Table)
			right := 0
			for _, r := range rows {
				if a.dia.got[r] {
					right++
				}
			}
			_, answered := a.dia.got[a.dia.cur]
			switch {
			case a.dia.cur > 0 && !answered:
				ui.Text(c, "想对了吗？").FontSize(textSmall + 0.5)
				if chip("quiz-right", "想对了", false).Clicked() {
					a.answerQuiz(true)
				}
				if chip("quiz-missed", "没想起来", false).Clicked() {
					a.answerQuiz(false)
				}
			case len(a.dia.got) >= len(rows):
				say := fmt.Sprintf("%d 条里想起 %d 条。", len(rows), right)
				if right < len(rows) {
					say += fmt.Sprintf("没想起来的，%d 天后「今天」会提醒你再考。", quizAgainMissed)
				} else {
					say += fmt.Sprintf("%d 天后再考一次。", quizAgainRight)
				}
				ui.Text(c, say).FontSize(textSmall + 0.5)
			default:
				ui.Text(c, "先自己想这一行的结果，再点开它。").FontSize(textSmall + 0.5)
				ui.Text(c, fmt.Sprintf("%d / %d", len(a.dia.got), len(rows))).FontSize(textCaption).TextColor(t.TextMuted).FontFeatures("tnum").NoWrap()
			}
			off()
		default:
			way := func(text, tip string, ic icon, w int, mark bool) {
				b := key(ui.Row(c.Key("way:"+text)).Height(30).Padding(0, 12).Gap(6).Radius(radiusPill).AlignItems(ui.Center).
					Cursor(ui.CursorPointer).Label(text).Tooltip(tip).Shrink(0), t)
				if b.Clicked() {
					a.setDiaWay(w)
				}
				b.Children(func() {
					if ic >= 0 {
						glyph(c, ic, 11, ink2(t))
					}
					ui.Text(c, text).FontSize(textSmall).TextColor(ink2(t)).NoWrap().PassThrough()
					if mark {
						ui.Box(c).Size(7, 7).Radius(4).Background(t.Warning).PassThrough()
					}
				})
			}
			if canTell {
				way("讲一遍", "一步一步讲这个流程，每步一句话", iconPlay, diaTell, false)
			}
			if canCase {
				way("拿例子走一遍", "挑一个具体的例子，看它在每张图上怎么走", -1, diaCase, false)
			}
			if canQuiz {
				way("考我", "盖住判定表的结果，先自己想，再点开对一对", -1, diaQuiz, a.quizDue(f))
			}
		}
	})
}

// diaPanel is the right column of the page: where what is chosen comes
// from in the map; told or walked, the step or the case; nothing chosen,
// the set itself.
func (a *app) diaPanel(c *ui.Context) {
	f := a.diaOf(a.dia.node)
	if f == nil {
		return
	}
	t := c.Theme()
	changed := a.diaChanges(f).changed
	head := func(over, title string) {
		if over != "" {
			ui.Text(c, over).FontSize(textCaption).TextColor(t.TextMuted).Margin(0, 0, -8)
		}
		ui.Text(c, title).FontSize(textHeadline + 2).FontWeight(700).LetterSpacing(-0.5).Wrap().LineHeight(1.25)
	}
	say := func(text string) {
		if text != "" {
			ui.Text(c, text).FontSize(textSmall+0.5).TextColor(ink2(t)).Wrap().LineHeight(1.55).Margin(-6, 0, 0)
		}
	}
	sources := func(title string, ids []string) {
		if len(ids) == 0 {
			return
		}
		ui.Column(c.Key("sources")).FillWidth().Children(func() {
			trayHead(c, "sources-head", title, fmt.Sprintf("%d 个主题", len(ids)), nil)
			for _, id := range ids {
				a.diaSourceRow(c, id, changed[id])
			}
		})
	}
	mend := func(ids []string) {
		to := f.Node
		if len(ids) > 0 && a.file.Node(ids[0]) != nil {
			to = ids[0]
		}
		ui.Row(c.Key("mend")).Children(func() { // a key its own width, not the column's
			if accentButton(c, "去导图补上", "回到导图里这条规则的地方").Clicked() {
				a.later(func() { a.diaSource(to) })
			}
		})
	}
	num := func(prefix string) int {
		n, _ := strconv.Atoi(strings.TrimPrefix(a.dia.pick, prefix))
		return n
	}
	ui.Column(c.Key(fmt.Sprint("dia-panel", a.dia.kind, a.dia.pick, a.dia.way, a.dia.step, a.dia.cas))).Padding(4, 16, 20).Gap(14).Children(func() {
		switch {
		case a.diaBusy == f.Node:
			head("", cmpOr(f.Title, "图解"))
			say("正在重画…")
		case a.dia.way == diaTell && f.Flow != nil && len(f.Flow.Tell) > 0:
			step := min(a.dia.step, len(f.Flow.Tell)-1)
			st := f.Flow.Tell[step]
			var ids []string
			name := ""
			for _, id := range st.Nodes {
				n := f.Flow.Node(id)
				if n == nil {
					continue
				}
				if name == "" {
					name = n.Title
				}
				for _, s := range n.Src {
					if !slices.Contains(ids, s) {
						ids = append(ids, s)
					}
				}
			}
			head(fmt.Sprintf("讲一遍 · 第 %d 步", step+1), name)
			say(st.Say)
			sources("出自导图", ids)
		case a.dia.way == diaCase && a.dia.cas < len(f.Cases) && a.dia.pick == "":
			cs := f.Cases[a.dia.cas]
			head("拿例子走一遍", cs.Title)
			if len(cs.Facts) > 0 {
				ui.Column(c.Key("facts")).FillWidth().Children(func() {
					trayHead(c, "facts-head", "这个例子", "", nil)
					for i, fact := range cs.Facts {
						ui.Row(c.Key(i)).FillWidth().Padding(9, 4).BorderWidth(0, 0, 1, 0).BorderColor(hairline(t)).Children(func() {
							ui.Text(c, fact).FontSize(textBody).Wrap()
						})
					}
				})
			}
			ui.Column(c.Key("result")).FillWidth().Gap(4).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(0, 4).Children(func() {
					if cs.Gap {
						ui.Box(c).Size(8, 8).Radius(4).Background(t.Warning)
					}
					ui.Text(c, "结果").FontSize(textSmall).FontWeight(700).TextColor(t.Text.MixOklab(t.TextMuted, 0.45))
				})
				ui.Text(c, cs.Result).FontSize(textSmall+0.5).TextColor(ink2(t)).Wrap().LineHeight(1.55).Padding(0, 4)
			})
			if cs.Gap {
				var ids []string
				if cs.Row > 0 && f.Table != nil && cs.Row <= len(f.Table.Rows) {
					ids = f.Table.Rows[cs.Row-1].Src
				}
				mend(ids)
			}
			ui.Text(c, "例子是 AI 照规则挑的。每张图上走的是同一个例子。").FontSize(textCaption).TextColor(t.TextMuted).Wrap().LineHeight(1.5)
		case a.dia.way == diaQuiz:
			head("考我", "判定表")
			say("结果盖住了。先自己想，再点开一行对一对。")
			ui.Text(c, fmt.Sprintf("自己回想一遍，比再看一遍记得久。有没想起来的，%d 天后「今天」会提醒你再考。", quizAgainMissed)).
				FontSize(textCaption).TextColor(t.TextMuted).Wrap().LineHeight(1.5)
		case strings.HasPrefix(a.dia.pick, "row:") && f.Table != nil && num("row:") >= 1 && num("row:") <= len(f.Table.Rows):
			no := num("row:")
			r := f.Table.Rows[no-1]
			if r.Gap {
				head("AI 看出来的", "这种情况没说清")
				say(r.Why)
			} else {
				head("表里的这一条", fmt.Sprintf("第 %d 条", no))
			}
			ui.Column(c.Key("cells")).FillWidth().Children(func() {
				line := func(k any, name, v string) {
					ui.Row(c.Key(k)).FillWidth().Padding(8, 4).Gap(10).BorderWidth(0, 0, 1, 0).BorderColor(hairline(t)).Children(func() {
						ui.Text(c, name).FontSize(textSmall).TextColor(t.TextMuted).Width(84).Shrink(0).SingleLine()
						ui.Text(c, cmpOr(v, "—")).FontSize(textBody).Grow(1).Shrink(1).MinWidth(0).Wrap()
					})
				}
				for i, name := range f.Table.Conds {
					line(fmt.Sprint("w", i), name, r.When[i])
				}
				if !r.Gap {
					for i, name := range f.Table.Results {
						line(fmt.Sprint("t", i), name, r.Then[i])
					}
				}
			})
			if r.Gap {
				sources("碰到一起的规则", r.Src)
				mend(r.Src)
			} else {
				sources("出自导图", r.Src)
			}
		case strings.HasPrefix(a.dia.pick, "note:") && f.Table != nil && num("note:") >= 1 && num("note:") <= len(f.Table.Notes):
			n := f.Table.Notes[num("note:")-1]
			head("表外的一条规则", "另外")
			say(n.Text)
			sources("出自导图", n.Src)
		case strings.HasPrefix(a.dia.pick, "gap:") && num("gap:") >= 1 && num("gap:") <= len(f.Gaps):
			g := f.Gaps[num("gap:")-1]
			head("AI 看出来的", "这里没说清")
			say(g.Say)
			sources("和它有关的主题", g.Src)
			mend(g.Src)
		case strings.HasPrefix(a.dia.pick, "obj:") && num("obj:") >= 1 && num("obj:") <= len(f.Objects):
			a.diaObjectPanel(c, f.Objects[num("obj:")-1], head, say)
		case a.dia.pick != "" && a.diaGraphOf(f) != nil && a.diaGraphOf(f).Node(a.dia.pick) != nil:
			n := a.diaGraphOf(f).Node(a.dia.pick)
			over := "图上的这一步"
			if a.dia.kind == "state" {
				over = "图上的这个状态"
			}
			head(over, n.Title)
			say(n.Sub)
			if len(n.Src) > 0 {
				sources("出自导图", n.Src)
				ui.Text(c, "点一个主题，回到导图里看它。").FontSize(textCaption).TextColor(t.TextMuted)
			} else {
				ui.Text(c, "AI 没说它出自哪个主题：这一步可能是它补上的，对一对导图。").FontSize(textCaption).TextColor(t.TextMuted).Wrap().LineHeight(1.5)
			}
		default:
			a.diaSetPanel(c, f, head)
		}
	})
}

// diaGraphOf is the graph of the kind shown, or nil.
func (a *app) diaGraphOf(f *diaFile) *diagram.Graph {
	switch a.dia.kind {
	case "flow":
		return f.Flow
	case "state":
		return f.State
	}
	return nil
}

// diaSetPanel says of the set itself: what it was drawn from and when,
// its kinds, what the AI found unsaid, and the line to have it drawn
// otherwise.
func (a *app) diaSetPanel(c *ui.Context, f *diaFile, head func(over, title string)) {
	t := c.Theme()
	title := f.Title
	n := a.file.Node(f.Node)
	if n != nil {
		title = n.Title
	}
	head("", title)
	under := len(f.Seen) - 1
	if _, rel := f.Seen["~relations"]; rel {
		under--
	}
	from := "照这个主题画的"
	if under > 0 {
		from = fmt.Sprintf("照这个主题和下面 %d 个主题画的", under)
	}
	ui.Text(c, from+" · "+whenLabel(f.Drawn, a.now())).FontSize(textCaption).TextColor(t.TextMuted).Wrap().Margin(-8, 0, 0)

	if f.Partial {
		ui.Row(c.Key("partial")).FillWidth().Padding(8, 10).Gap(8).Radius(radiusPill).Background(blockFill(t)).Children(func() {
			ui.Box(c).Size(8, 8).Radius(4).Background(t.Warning).Shrink(0).Margin(5, 0, 0)
			ui.Text(c, "这一支太长，AI 只读了前面一部分。选它下面小一些的分支来画，会更全、更准。").FontSize(textSmall).TextColor(ink2(t)).Wrap().LineHeight(1.5).Grow(1).Shrink(1).MinWidth(0)
		})
	}
	if len(f.Gaps) > 0 {
		ui.Column(c.Key("gaps")).FillWidth().Children(func() {
			trayHead(c, "gaps-head", "AI 看出来的", fmt.Sprintf("%d 处没说清", len(f.Gaps)), nil)
			for i, g := range f.Gaps {
				row := ui.Row(c.Key(i)).FillWidth().Padding(9, 4).Gap(10).Cursor(ui.CursorPointer).Label(g.Say).
					BorderWidth(0, 0, 1, 0).BorderColor(hairline(t)).Background(t.Text.Alpha(0)).Transition(rowMotion)
				if row.Hovered() {
					row.Background(t.Text.Alpha(0.035))
				}
				if row.Clicked() {
					if g.Row > 0 && f.Table != nil {
						a.dia.kind, a.dia.pick, a.dia.way = "table", "row:"+strconv.Itoa(g.Row), diaRead
					} else {
						a.dia.pick = "gap:" + strconv.Itoa(i+1)
					}
				}
				row.Children(func() {
					ui.Box(c).Size(8, 8).Radius(4).Background(t.Warning).Shrink(0).Margin(6, 0, 0).PassThrough()
					ui.Text(c, g.Say).FontSize(textSmall + 0.5).Grow(1).Shrink(1).MinWidth(0).Wrap().LineHeight(1.5).PassThrough()
				})
			}
		})
	}
	if len(f.Cases) > 0 {
		ui.Column(c.Key("cases")).FillWidth().Children(func() {
			trayHead(c, "cases-head", "拿例子走一遍", "", nil)
			for i, cs := range f.Cases {
				sub := ""
				if cs.Gap {
					sub = "走不通"
				}
				if diaRow(c, fmt.Sprint("case:", i), cs.Title, sub, false).Label("例子 " + cs.Title).Clicked() {
					a.setDiaWay(diaCase)
					a.dia.cas = i
					a.revealCase(f)
				}
			}
		})
	}
	if q := f.Quiz; q != nil && f.Table != nil {
		rows := len(quizRows(f.Table))
		text := fmt.Sprintf("上次考：%d 条里想起 %d 条", rows, rows-len(q.Missed))
		if a.quizDue(f) {
			text += " · 该再考了"
		} else {
			text += " · " + dayLabel(q.Next, a.now()) + "再考"
		}
		ui.Text(c, text).FontSize(textCaption).TextColor(t.TextMuted).Wrap()
	}
	ui.Column(c.Key("tell-ai")).FillWidth().Gap(8).Children(func() {
		trayHead(c, "tell-head", "让 AI 改图", "", nil)
		box := captureBox(c, "dia-told").MinHeight(38).Padding(2, 8, 2, 10)
		var in ui.Element
		box.Children(func() {
			in = ui.TextInputBase(c.Key("told"), &a.dia.told).FontSize(textSmall + 0.5).Placeholder("比如：把验收拆成两步").Label("告诉 AI 怎么改这张图").
				Grow(1).Basis(0).MinWidth(0)
			if in.Submitted() && strings.TrimSpace(a.dia.told) != "" {
				told := a.dia.told
				if a.diaBusy == "" { // kept while another is being drawn, which is said
					a.dia.told = ""
				}
				a.later(func() { a.drawDia(f.Node, told) })
			}
		})
		captureFocus(c, box, in)
		ui.Text(c, "写一句，记下后 AI 照着重画。图是照导图画的：规则变了就改导图，再重画。").FontSize(textCaption).TextColor(t.TextMuted).Wrap().LineHeight(1.5)
	})
}

// diaRow is a row of a short list in the right column: a name, a word on
// it, and that it goes somewhere.
func diaRow(c *ui.Context, k, name, sub string, on bool) ui.Element {
	t := c.Theme()
	row := ui.Row(c.Key(k)).MinHeight(42).FillWidth().Padding(0, 4).Gap(10).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(name).
		BorderWidth(0, 0, 1, 0).BorderColor(hairline(t)).Background(t.Text.Alpha(0)).Transition(rowMotion)
	if row.Hovered() {
		row.Background(t.Text.Alpha(0.035))
	}
	row.Children(func() {
		txt := ui.Text(c, name).FontSize(textBody).NoWrap().PassThrough()
		if on {
			txt.FontWeight(700)
		}
		ui.Spacer(c)
		if sub != "" {
			ui.Text(c, sub).FontSize(textCaption).TextColor(t.TextMuted).NoWrap().PassThrough()
		}
		glyph(c, iconForward, 10, faint(t))
	})
	return row
}

// diaSourceRow is a topic something was drawn from: where it is, what it
// says, its note; a click goes to it in the map.
func (a *app) diaSourceRow(c *ui.Context, id string, changed bool) {
	t := c.Theme()
	n := a.file.Node(id)
	if n == nil {
		ui.Row(c.Key("src:"+id)).FillWidth().Padding(10, 4).BorderWidth(0, 0, 1, 0).BorderColor(hairline(t)).Children(func() {
			ui.Text(c, "这个主题已经不在导图里了").FontSize(textSmall).TextColor(t.Warning)
		})
		return
	}
	d := a.file.SheetOf(id)
	var trail []string
	if path := d.Path(id); len(path) > 1 {
		for _, p := range path[max(len(path)-3, 0) : len(path)-1] {
			trail = append(trail, p.Title)
		}
	}
	row := ui.Column(c.Key("src:"+id)).FillWidth().Padding(10, 4).Gap(3).Cursor(ui.CursorPointer).Label("主题 "+n.Title).
		Tooltip("回到导图里看这个主题").BorderWidth(0, 0, 1, 0).BorderColor(hairline(t)).Background(t.Text.Alpha(0)).Transition(rowMotion)
	if row.Hovered() {
		row.Background(t.Text.Alpha(0.035))
	}
	if row.Clicked() {
		a.later(func() { a.diaSource(id) })
	}
	row.Children(func() {
		if len(trail) > 0 {
			ui.Text(c, strings.Join(trail, " › ")).FontSize(textCaption).TextColor(t.TextMuted).SingleLine().PassThrough()
		}
		ui.Text(c, n.Title).FontSize(textBody).Wrap().PassThrough()
		if changed {
			ui.Text(c, "画完之后改过，图上还是旧的").FontSize(textCaption).TextColor(t.Warning).PassThrough()
		}
		if note := strings.Join(strings.Fields(linkToken.ReplaceAllString(n.Note, "")), " "); note != "" {
			if r := []rune(note); len(r) > 90 {
				note = string(r[:90]) + "…"
			}
			ui.Box(c).FillWidth().Padding(6, 9).Margin(4, 0, 0).Radius(radiusSmall).Background(blockFill(t)).PassThrough().Children(func() {
				ui.Text(c, "备注："+library.Mask(note)).FontSize(textCaption + 0.5).TextColor(ink2(t)).Wrap().LineHeight(1.5).PassThrough()
			})
		}
	})
}

// diaObjectPanel says of a noun: what it is, its other names, and every
// place the map speaks of it, by canvas.
func (a *app) diaObjectPanel(c *ui.Context, o diagram.Object, head func(over, title string), say func(string)) {
	t := c.Theme()
	head(cmpOr(o.Kind, "名词"), o.Name)
	say(o.What)
	if len(o.Aliases) > 0 {
		ui.Text(c, "也叫："+strings.Join(o.Aliases, "、")).FontSize(textCaption).TextColor(t.TextMuted).Wrap()
	}
	if f := a.diaOf(a.dia.node); f != nil {
		idx := slices.IndexFunc(f.Objects, func(p diagram.Object) bool { return p.Name == o.Name }) + 1
		type place struct {
			kind string
			n    int
		}
		var places []place
		for _, k := range f.Kinds() {
			if l := a.nounLight(f, o, k); k != "objects" && len(l.nodes)+len(l.rows) > 0 {
				places = append(places, place{k, len(l.nodes) + len(l.rows)})
			}
		}
		if len(places) > 0 && idx > 0 {
			ui.Column(c.Key("on-diagrams")).FillWidth().Children(func() {
				trayHead(c, "on-diagrams-head", "在图上", "", nil)
				for _, p := range places {
					unit := "处"
					if p.kind == "table" {
						unit = "行"
					}
					if diaRow(c, "on:"+p.kind, "在"+diaKinds[p.kind]+"里看", fmt.Sprintf("%d %s", p.n, unit), false).Clicked() {
						a.dia.kind, a.dia.pick, a.dia.noun = p.kind, "", idx
					}
				}
			})
		}
	}
	ms := a.mentions(o)
	if len(ms) == 0 {
		ui.Text(c, "导图里没有找到写着这个词的主题。").FontSize(textCaption).TextColor(t.TextMuted).Wrap()
		return
	}
	ui.Column(c.Key("mentions")).FillWidth().Children(func() {
		trayHead(c, "mentions-head", "导图里提到它的地方", fmt.Sprintf("%d 处", len(ms)), nil)
		var sheet *model.Document
		for i, m := range ms {
			if i == 40 {
				ui.Text(c, fmt.Sprintf("还有 %d 处", len(ms)-i)).FontSize(textCaption).TextColor(t.TextMuted).Padding(8, 4)
				break
			}
			if m.sheet != sheet && len(a.file.Sheets) > 1 { // by canvas, where the map has several
				sheet = m.sheet
				ui.Text(c, "画布 · "+sheet.Title).FontSize(textCaption).FontWeight(700).TextColor(t.TextMuted).Padding(10, 4, 2)
			}
			row := ui.Column(c.Key(m.node.ID)).FillWidth().Padding(9, 4).Gap(2).Cursor(ui.CursorPointer).Label("提到 "+m.node.Title).
				BorderWidth(0, 0, 1, 0).BorderColor(hairline(t)).Background(t.Text.Alpha(0)).Transition(rowMotion)
			if row.Hovered() {
				row.Background(t.Text.Alpha(0.035))
			}
			if row.Clicked() {
				id := m.node.ID
				a.later(func() { a.diaSource(id) })
			}
			row.Children(func() {
				if m.path != "" {
					ui.Text(c, m.path).FontSize(textCaption).TextColor(t.TextMuted).SingleLine().PassThrough()
				}
				ui.Text(c, m.node.Title).FontSize(textBody).Wrap().PassThrough()
			})
		}
	})
}

// diaSection is, on a topic's page, what was drawn of it: its kinds, each
// opened by a click; that the map changed since; that its table is to be
// asked back.
func (a *app) diaSection(c *ui.Context, n *model.Node) {
	f := a.diaOf(n.ID)
	busy := a.diaBusy == n.ID
	if f == nil && !busy && (a.diaErr == "" || a.diaErrFor != n.ID) {
		return
	}
	t := c.Theme()
	sub := ""
	if f != nil {
		sub = whenLabel(f.Drawn, a.now()) + " 画的"
	}
	ui.Column(c.Key("dia-section")).FillWidth().Children(func() {
		trayHead(c, "dia-section-head", "图解", sub, nil)
		switch {
		case busy:
			a.diaStages(c, n.ID)
			return
		case f == nil:
			ui.Row(c.Key("dia-failed")).Gap(8).AlignItems(ui.Center).FillWidth().Padding(0, 4).Children(func() {
				ui.Text(c, a.diaErr).FontSize(textSmall).TextColor(t.Danger).Grow(1).Shrink(1).MinWidth(0).Wrap()
				if textKey(c, "再试一次", "再画一次图解", "").Clicked() {
					a.later(func() { a.drawDia(n.ID, "") })
				}
			})
			return
		}
		for _, k := range f.Kinds() {
			if diaRow(c, "open:"+k, diaKinds[k], "", false).Label("图解 " + diaKinds[k]).Clicked() {
				a.later(func() { a.openDia(n.ID, k) })
			}
		}
		note := func(k, text, act string, do func()) {
			ui.Row(c.Key(k)).FillWidth().Margin(8, 0, 0).Padding(6, 6, 6, 10).Gap(8).Radius(radiusPill).AlignItems(ui.Center).Background(blockFill(t)).Children(func() {
				ui.Box(c).Size(8, 8).Radius(4).Background(t.Warning).Shrink(0)
				ui.Text(c, text).FontSize(textSmall).TextColor(ink2(t)).Grow(1).Shrink(1).MinWidth(0).SingleLine()
				if textKey(c, act, k+act, "").Clicked() {
					a.later(do)
				}
			})
		}
		if a.diaChanges(f).n > 0 {
			note("dia-stale-note", "导图改过，图可能旧了", "重画", func() { a.drawDia(n.ID, f.Told) })
		}
		if a.quizDue(f) {
			note("dia-due-note", "判定表该再考了", "考我", func() { a.askBack(f) })
		}
	})
}

// mapDiasSection lists, on the canvas's page, what was drawn of the map's
// topics, on every canvas: a set is found again from here.
func (a *app) mapDiasSection(c *ui.Context) {
	sets := a.mapDias()
	if len(sets) == 0 {
		return
	}
	ui.Column(c.Key("map-dias")).FillWidth().Children(func() {
		trayHead(c, "map-dias-head", "图解", strconv.Itoa(len(sets)), nil)
		for _, f := range sets {
			n := a.file.Node(f.Node)
			sub := ""
			if d := a.file.SheetOf(f.Node); d != nil && len(a.file.Sheets) > 1 {
				sub = d.Title
			}
			if diaRow(c, "set:"+f.Node, n.Title, sub, false).Label("图解 " + n.Title).Clicked() {
				id := f.Node
				a.later(func() { a.openDia(id, "") })
			}
		}
	})
}

// diaLine says, on 今天, that tables are to be asked back: a click opens
// the first, its results covered.
func (a *app) diaLine(c *ui.Context) {
	due := a.diasDue()
	if len(due) == 0 {
		return
	}
	t := c.Theme()
	row := ui.Row(c.Key("dia-line")).MinHeight(36).Padding(6, 10).Gap(8).Radius(radiusRow).AlignItems(ui.Center).FillWidth().
		Cursor(ui.CursorPointer).Label("该再考的判定表").Background(t.Accent.Alpha(0.07)).Transition(rowMotion)
	if row.Hovered() {
		row.Background(t.Accent.Alpha(0.11))
	}
	if row.Clicked() {
		a.later(func() { a.askBack(due[0]) })
	}
	text := fmt.Sprintf("「%s」的判定表该再考了", cmpOr(due[0].Title, "图解"))
	if len(due) > 1 {
		text = fmt.Sprintf("有 %d 张判定表该再考了，先考「%s」", len(due), cmpOr(due[0].Title, "图解"))
	}
	row.Children(func() {
		glyph(c, iconSparkle, 14, t.Accent).PassThrough()
		ui.Text(c, text).FontSize(textBody).Grow(1).Shrink(1).MinWidth(0).SingleLine().PassThrough()
		ui.Text(c, "考我 ›").FontSize(textSmall).TextColor(t.Accent).NoWrap().PassThrough()
	})
}

// diaMark is, at the corner of a topic on the canvas, that it has a set:
// a click opens it. It is over the corner, taking no room of the topic's
// box, so that drawing a set or forgetting one moves nothing.
func (a *app) diaMark(c *ui.Context, f *diaFile, z float32) {
	t := c.Theme()
	col, tip := t.Accent, "有图解：点开看"
	if a.quizDue(f) {
		col, tip = t.Warning, "有图解，判定表该再考了：点开看"
	}
	s := 15 * z
	mark := ui.Box(c.Key("dia-mark")).Absolute().Right(-s*0.4).Top(-s*0.4).Size(s, s).Radius(s/2).Background(col).Border(1.5, t.Background).
		Cursor(ui.CursorPointer).Label("图解").Tooltip(tip)
	if mark.Clicked() {
		id := f.Node
		a.later(func() { a.openDia(id, "") })
	}
	mark.Draw(func(p *ui.Painter, r ui.Rect) {
		// Two boxes joined: a step and the next.
		u := r.W / 15
		ink := t.AccentText
		p.Fill(ui.Rect{X: r.X + 3.5*u, Y: r.Y + 4*u, W: 4 * u, H: 2.6 * u}, ink, 0.8*u)
		p.Fill(ui.Rect{X: r.X + 7.5*u, Y: r.Y + 8.4*u, W: 4 * u, H: 2.6 * u}, ink, 0.8*u)
		p.StrokePolyline([][2]float32{{r.X + 5.5*u, r.Y + 6.6*u}, {r.X + 5.5*u, r.Y + 9.7*u}, {r.X + 7.5*u, r.Y + 9.7*u}}, max(u, 0.8), ink)
	})
}

// nounLight is what of a kind speaks of a noun: the boxes and the rows
// that name it, or were drawn from a topic that does.
func (a *app) nounLight(f *diaFile, o diagram.Object, kind string) *diaLit {
	var names []string
	for _, s := range append([]string{o.Name}, o.Aliases...) {
		if len([]rune(s)) >= minNoun {
			names = append(names, strings.ToLower(s))
		}
	}
	from := map[string]bool{}
	for _, m := range a.mentions(o) {
		from[m.node.ID] = true
	}
	speaks := func(src []string, texts ...string) bool {
		text := strings.ToLower(strings.Join(texts, "\n"))
		return slices.ContainsFunc(names, func(s string) bool { return strings.Contains(text, s) }) ||
			slices.ContainsFunc(src, func(id string) bool { return from[id] })
	}
	l := &diaLit{nodes: map[string]bool{}, edges: map[int]bool{}, hot: map[int]bool{}, rows: map[int]bool{}}
	graph := func(g *diagram.Graph) {
		for _, n := range g.Nodes {
			if n.Kind != diagram.KindStart || kind == "flow" {
				l.nodes[n.ID] = speaks(n.Src, n.Title, n.Sub)
			}
			if !l.nodes[n.ID] {
				delete(l.nodes, n.ID)
			}
		}
		for i, e := range g.Edges {
			if l.nodes[e.From] && l.nodes[e.To] {
				l.edges[i] = true
			}
		}
	}
	switch {
	case kind == "flow" && f.Flow != nil:
		graph(f.Flow)
	case kind == "state" && f.State != nil:
		graph(f.State)
	case kind == "table" && f.Table != nil:
		for i, r := range f.Table.Rows {
			if speaks(r.Src, append(slices.Clone(r.When), r.Then...)...) {
				l.rows[i+1] = true
			}
		}
	}
	return l
}

// revealCase brings into view where the case walked begins.
func (a *app) revealCase(f *diaFile) {
	if a.dia.cas < len(f.Cases) && a.dia.kind == "flow" && len(f.Cases[a.dia.cas].Flow) > 0 {
		a.diaReveal = f.Cases[a.dia.cas].Flow[0]
	}
}

// diaSourceStrip says where what is chosen comes from in a window too
// narrow for the right column: the first topic, and how many more.
func (a *app) diaSourceStrip(c *ui.Context, f *diaFile) {
	var src []string
	switch g := a.diaGraphOf(f); {
	case g != nil && g.Node(a.dia.pick) != nil:
		src = g.Node(a.dia.pick).Src
	case strings.HasPrefix(a.dia.pick, "row:") && f.Table != nil:
		if n, _ := strconv.Atoi(strings.TrimPrefix(a.dia.pick, "row:")); n >= 1 && n <= len(f.Table.Rows) {
			src = f.Table.Rows[n-1].Src
		}
	}
	var first *model.Node
	for _, id := range src {
		if first = a.file.Node(id); first != nil {
			break
		}
	}
	if first == nil {
		return
	}
	t := c.Theme()
	text := "出自「" + first.Title + "」"
	if len(src) > 1 {
		text += fmt.Sprintf("等 %d 个主题", len(src))
	}
	row := ui.Row(c.Key("dia-source-strip")).Margin(0, 16, 8).Padding(8, 12).Gap(8).Radius(radiusPill).AlignItems(ui.Center).Shrink(0).
		Background(t.Accent.Alpha(0.08)).Cursor(ui.CursorPointer).Label("回到导图里的出处")
	if row.Clicked() {
		id := first.ID
		a.later(func() { a.diaSource(id) })
	}
	row.Children(func() {
		ui.Text(c, text).FontSize(textSmall).Grow(1).Shrink(1).MinWidth(0).SingleLine().PassThrough()
		ui.Text(c, "在导图里看 ›").FontSize(textSmall).TextColor(t.Accent).NoWrap().PassThrough()
	})
}
