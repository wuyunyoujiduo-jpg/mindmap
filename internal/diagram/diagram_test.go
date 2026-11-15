package diagram

import (
	"slices"
	"strings"
	"testing"
)

// refund is the flow of the draft: a return of goods, in four lanes.
func refund() *Graph {
	return &Graph{
		Lanes: []string{"门店", "业务员 · 主管", "仓库 · 司机", "财务"},
		Nodes: []Node{
			{ID: "A", Kind: KindStart, Title: "申请退货"},
			{ID: "B", Kind: KindDecision, Title: "符合申请条件？"},
			{ID: "X", Kind: KindEnd, Title: "不受理"},
			{ID: "C", Kind: KindDecision, Title: "质量问题？", Lane: 1},
			{ID: "D", Title: "审核", Sub: "≤500 元业务员，以上主管", Lane: 1},
			{ID: "G", Kind: KindDecision, Title: "审核通过？", Lane: 1},
			{ID: "R", Kind: KindEnd, Title: "驳回", Lane: 1},
			{ID: "P", Title: "取货", Sub: "司机上门，或门店送回", Lane: 2},
			{ID: "V", Title: "验收", Lane: 2},
			{ID: "M", Kind: KindOK, Title: "退款", Sub: "现结原路退，账期冲应收", Lane: 3},
		},
		Edges: []Edge{
			{"A", "B", ""}, {"B", "X", "否"}, {"B", "C", "是"}, {"C", "D", "否"}, {"C", "P", "是，免审"},
			{"D", "G", ""}, {"G", "R", "否"}, {"G", "P", "是"}, {"P", "V", ""}, {"V", "M", ""},
		},
	}
}

func states() *Graph {
	return &Graph{
		Nodes: []Node{
			{ID: "o", Kind: KindStart},
			{ID: "s1", Title: "待审核"}, {ID: "s2", Title: "待取货"}, {ID: "s3", Title: "待验收"},
			{ID: "s4", Kind: KindOK, Title: "已退款"}, {ID: "s5", Kind: KindBad, Title: "已驳回"}, {ID: "s6", Kind: KindEnd, Title: "已关闭"},
		},
		Edges: []Edge{
			{"o", "s1", "提交申请"}, {"o", "s2", "质量问题，免审"}, {"s1", "s2", "审核通过"}, {"s2", "s3", "取到货"},
			{"s3", "s4", "验收通过"}, {"s1", "s5", "驳回"}, {"s5", "s1", "改后重提"}, {"s2", "s6", "门店撤销"}, {"s3", "s6", "货不对"},
		},
	}
}

// crossed tells which box a route runs through, other than those it joins.
func crossed(g *Graph, l Layout) string {
	for _, r := range l.Routes {
		e := g.Edges[r.Edge]
		for i := 0; i+1 < len(r.Points); i++ {
			a, b := r.Points[i], r.Points[i+1]
			x0, x1, y0, y1 := min(a.X, b.X), max(a.X, b.X), min(a.Y, b.Y), max(a.Y, b.Y)
			for id, box := range l.Boxes {
				if id == e.From || id == e.To {
					continue
				}
				if x1 > box.X+1 && x0 < box.X+box.W-1 && y1 > box.Y+1 && y0 < box.Y+box.H-1 {
					return e.From + "→" + e.To + " through " + id
				}
			}
		}
	}
	return ""
}

func TestFlowLayout(t *testing.T) {
	g := refund()
	l := g.LayoutFlow()
	row := func(id string) int { return int((l.Boxes[id].Y + l.Boxes[id].H/2 - l.Head) / flowRow) }
	col := func(id string) int { return int(l.Boxes[id].X / laneWidth) }
	// As drawn by hand: five rows, a hand-over staying on its row.
	want := map[string][2]int{"A": {0, 0}, "B": {0, 1}, "X": {0, 2}, "C": {1, 1}, "D": {1, 2}, "G": {1, 3}, "R": {1, 4}, "P": {2, 3}, "V": {2, 4}, "M": {3, 4}}
	for id, w := range want {
		if col(id) != w[0] || row(id) != w[1] {
			t.Errorf("%s at lane %d row %d, want %v", id, col(id), row(id), w)
		}
	}
	if l.Width != 4*laneWidth || len(l.Lanes) != 4 || len(l.Routes) != len(g.Edges) {
		t.Fatalf("layout %+v", l)
	}
	if c := crossed(g, l); c != "" {
		t.Fatal(c)
	}
	// 质量问题 → 取货 goes along its row and down, round the review.
	if r := l.Routes[4]; len(r.Points) != 3 || r.Points[0].Y != r.Points[1].Y || r.Points[1].X != r.Points[2].X {
		t.Fatalf("C→P %+v", r.Points)
	}
	if got := g.PathEdges([]string{"A", "B", "C", "P", "V", "M"}); len(got) != 5 || got[2] != 4 {
		t.Fatalf("path %v", got)
	}
}

func TestStateLayout(t *testing.T) {
	g := states()
	l := g.LayoutState()
	y := func(id string) float32 { return l.Boxes[id].Y + l.Boxes[id].H/2 }
	x := func(id string) float32 { return l.Boxes[id].X + l.Boxes[id].W/2 }
	// The long way along the first row; what leads nowhere under what it comes from.
	for _, id := range []string{"s1", "s2", "s3", "s4"} {
		if y(id) != stateTop {
			t.Errorf("%s not on the first row: %v", id, y(id))
		}
	}
	if !(x("s1") < x("s2") && x("s2") < x("s3") && x("s3") < x("s4")) {
		t.Fatal("the main way is not in order")
	}
	if x("s5") != x("s1") || y("s5") <= y("s1") || x("s6") != x("s2") || y("s6") <= y("s2") {
		t.Fatalf("side states: s5 %v,%v s6 %v,%v", x("s5"), y("s5"), x("s6"), y("s6"))
	}
	if x("o") != x("s1") || y("o") >= y("s1") {
		t.Fatalf("the dot %v,%v", x("o"), y("o"))
	}
	if c := crossed(g, l); c != "" {
		t.Fatal(c)
	}
	// 驳回 and 改后重提 run side by side, not on each other.
	down, up := l.Routes[5].Points, l.Routes[6].Points
	if len(down) != 2 || len(up) != 2 || down[0].X == up[0].X {
		t.Fatalf("pair %+v %+v", down, up)
	}
}

// A graph of any shape lays out, and no route runs through a node on the
// simple ways there are.
func TestLayoutAnyShape(t *testing.T) {
	g := &Graph{Lanes: []string{"甲", "乙"}, Nodes: []Node{
		{ID: "a", Title: "a"}, {ID: "b", Title: "b"}, {ID: "c", Title: "c"}, {ID: "d", Title: "d", Lane: 1}, {ID: "e", Title: "e", Lane: 1},
	}, Edges: []Edge{{"a", "b", ""}, {"b", "c", ""}, {"a", "c", "跳过"}, {"c", "a", "重来"}, {"b", "d", ""}, {"d", "e", ""}, {"e", "b", "退回"}}}
	l := g.LayoutFlow()
	if len(l.Boxes) != 5 || len(l.Routes) != 7 {
		t.Fatalf("layout %+v", l)
	}
	if c := crossed(g, l); c != "" {
		t.Fatal(c)
	}
	// One that loops on itself, with no beginning.
	ring := &Graph{Nodes: []Node{{ID: "o", Kind: KindStart}, {ID: "x", Title: "x"}, {ID: "y", Title: "y"}, {ID: "z", Title: "z"}},
		Edges: []Edge{{"o", "x", ""}, {"x", "y", ""}, {"y", "z", ""}, {"z", "x", ""}}}
	if l := ring.LayoutState(); len(l.Boxes) != 4 || crossed(ring, l) != "" {
		t.Fatalf("ring %+v %s", l, crossed(ring, l))
	}
}

// Whatever the answer draws, no route runs through a box.
func TestLayoutNeverCrosses(t *testing.T) {
	seed := uint32(7)
	rnd := func(n int) int { seed = seed*1664525 + 1013904223; return int(seed>>16) % n }
	for range 400 {
		n := 2 + rnd(maxFlowNodes-1)
		g := &Graph{Lanes: []string{"甲", "乙", "丙", "丁"}[:rnd(5)]} // none too: its columns are found
		for i := range n {
			n := Node{ID: string(rune('a' + i)), Title: "x", Lane: rnd(max(len(g.Lanes), 1))}
			switch rnd(4) { // of every size a box has
			case 0:
				n.Kind = KindDecision
			case 1:
				n.Sub = "y"
			}
			g.Nodes = append(g.Nodes, n)
		}
		for range n + rnd(n) {
			a, b := rnd(n), rnd(n)
			if a != b {
				g.Edges = append(g.Edges, Edge{From: g.Nodes[a].ID, To: g.Nodes[b].ID})
			}
		}
		if l := g.LayoutFlow(); l.Crossed(g) || len(l.Boxes) != n || len(l.Routes) != len(g.Edges) {
			t.Fatalf("flow %+v crossed", g)
		}
		s := &Graph{Nodes: append([]Node{{ID: "o", Kind: KindStart}}, g.Nodes[:min(n, maxStates)]...)}
		s.Edges = append(s.Edges, Edge{From: "o", To: s.Nodes[1].ID})
		for _, e := range g.Edges {
			if s.Node(e.From) != nil && s.Node(e.To) != nil {
				s.Edges = append(s.Edges, e)
			}
		}
		if l := s.LayoutState(); l.Crossed(s) || len(l.Boxes) != len(s.Nodes) || len(l.Routes) != len(s.Edges) {
			t.Fatalf("states %+v crossed", s)
		}
	}
}

// A flow with no lanes is not one long column: what turns off its
// longest way is beside it.
func TestFlowWithoutLanes(t *testing.T) {
	g := refund()
	g.Lanes = nil
	for i := range g.Nodes {
		g.Nodes[i].Lane = 0
	}
	l := g.LayoutFlow()
	x := func(id string) float32 { return l.Boxes[id].X }
	y := func(id string) float32 { return l.Boxes[id].Y + l.Boxes[id].H/2 }
	if l.Width <= laneWidth || len(l.Lanes) != 0 || l.Head != 0 {
		t.Fatalf("one column: %+v", l)
	}
	if x("X") <= x("B") || y("X") != y("B") || x("R") <= x("G") || y("R") != y("G") {
		t.Fatalf("the ends not beside their questions: X %v,%v B %v,%v", x("X"), y("X"), x("B"), y("B"))
	}
	if l.Height >= 10*flowRow || l.Crossed(g) {
		t.Fatalf("height %v crossed %v", l.Height, l.Crossed(g))
	}
}

func TestClean(t *testing.T) {
	s := &Set{
		Flow: &Graph{Lanes: []string{"门店", " "}, Nodes: []Node{
			{ID: "a", Title: " 申请  退货 ", Src: []string{"1", "9", "1"}}, {ID: "a", Title: "重复"}, {ID: "b", Title: "", Kind: "step"},
			{ID: "c", Title: "审核", Lane: 7, Kind: "what"}, {ID: "d", Title: "一个特别特别特别特别特别特别特别特别长的标题"},
		}, Edges: []Edge{{"a", "c", "是"}, {"a", "c", "又一条"}, {"a", "zz", ""}, {"c", "c", ""}, {"c", "d", ""}},
			Tell: []Step{{Nodes: []string{"a", "zz"}, Say: "先申请。"}, {Nodes: []string{"zz"}, Say: "没有的"}, {Nodes: []string{"c"}}}},
		State: &Graph{Nodes: []Node{{ID: "s1", Title: "待审核"}, {ID: "s2", Title: "已退款", Kind: KindDecision}}, Edges: []Edge{{"s1", "s2", "通过"}}},
		Table: &Table{Conds: []string{"金额"}, Results: []string{"谁审"}, Rows: []Row{
			{When: []string{"≤500", "多的"}, Then: nil, Src: []string{"2"}}, {When: []string{""}}, {When: []string{">500"}, Then: []string{"主管"}, Gap: true}}},
		Objects: []Object{{Name: "退货单", Aliases: []string{"退货单", "退单"}}, {Name: "退货单"}, {Name: ""}},
		Cases:   []Case{{Title: "320 元", Flow: []string{"a", "zz", "c"}, States: []string{"s1", "nope"}, Row: 9}, {Title: ""}},
		Gaps:    []Gap{{Say: "没写", Row: -3, Src: []string{"2"}}, {Say: ""}},
	}
	s.Clean(func(v string) (string, bool) {
		ids := map[string]string{"1": "n1", "2": "n2"}
		id, ok := ids[v]
		return id, ok
	})
	f := s.Flow
	if f == nil || len(f.Nodes) != 3 || f.Nodes[0].Title != "申请 退货" || len(f.Nodes[0].Src) != 1 || f.Nodes[0].Src[0] != "n1" ||
		f.Nodes[1].Lane != 0 || f.Nodes[1].Kind != KindStep || len([]rune(f.Nodes[2].Title)) != maxTitleRunes {
		t.Fatalf("flow nodes %+v", f)
	}
	if len(f.Lanes) != 1 || len(f.Edges) != 2 || len(f.Tell) != 1 || len(f.Tell[0].Nodes) != 1 {
		t.Fatalf("flow %+v", f)
	}
	// The states begin at a dot, given one when the answer had none.
	if st := s.State; st == nil || len(st.Nodes) != 3 || st.Nodes[0].Kind != KindStart || st.Nodes[2].Kind != KindStep ||
		len(st.Edges) != 2 || st.Edges[0].To != "s1" {
		t.Fatalf("state %+v", s.State)
	}
	if tb := s.Table; tb == nil || len(tb.Rows) != 2 || len(tb.Rows[0].When) != 1 || len(tb.Rows[0].Then) != 1 || tb.Rows[0].Src[0] != "n2" || !tb.Rows[1].Gap {
		t.Fatalf("table %+v", s.Table)
	}
	if len(s.Objects) != 1 || len(s.Objects[0].Aliases) != 1 || s.Objects[0].Aliases[0] != "退单" {
		t.Fatalf("objects %+v", s.Objects)
	}
	if len(s.Cases) != 1 || len(s.Cases[0].Flow) != 2 || len(s.Cases[0].States) != 1 || s.Cases[0].Row != 0 {
		t.Fatalf("cases %+v", s.Cases)
	}
	if len(s.Gaps) != 1 || s.Gaps[0].Row != 0 {
		t.Fatalf("gaps %+v", s.Gaps)
	}
	if k := s.Kinds(); len(k) != 4 {
		t.Fatalf("kinds %v", k)
	}
	if src := s.Sources(); len(src) != 2 {
		t.Fatalf("sources %v", src)
	}

	// Too little left is nothing drawn.
	thin := &Set{Flow: &Graph{Nodes: []Node{{ID: "a", Title: "一步"}}}, Table: &Table{Conds: []string{"x"}, Results: []string{"y"}, Rows: []Row{{When: []string{"1"}}}}}
	thin.Clean(func(string) (string, bool) { return "", false })
	if !thin.Empty() {
		t.Fatalf("kept %+v", thin)
	}
}

func TestParse(t *testing.T) {
	s, err := Parse("好的：\n```json\n" + `{"flow":{"lanes":["门店"],"nodes":[{"id":1,"title":"申请","lane":"0","src":[3,"4"]},{"id":"2","title":"审核","src":5}],
		"edges":[{"from":1,"to":"2","label":true}],"tell":[{"nodes":[1,2],"say":"先申请再审核。"}]},
		"table":{"conds":["金额"],"results":["谁审"],"rows":[{"when":[500],"then":["业务员"],"gap":"false"},{"when":["更多"],"then":[],"gap":"true"}]},
		"cases":[{"title":"一单","flow":[1,2],"row":"2"}]}` + "\n```")
	if err != nil {
		t.Fatal(err)
	}
	f := s.Flow
	if f == nil || f.Nodes[0].ID != "1" || f.Nodes[0].Lane != 0 || len(f.Nodes[0].Src) != 2 || f.Nodes[0].Src[0] != "3" ||
		f.Edges[0].From != "1" || f.Edges[0].Label != "true" || f.Tell[0].Nodes[1] != "2" {
		t.Fatalf("flow %+v", f)
	}
	if s.Table.Rows[0].When[0] != "500" || s.Table.Rows[0].Gap || !s.Table.Rows[1].Gap || s.Cases[0].Row != 2 || s.Cases[0].Flow[0] != "1" {
		t.Fatalf("table %+v cases %+v", s.Table, s.Cases)
	}
	if _, err := Parse("画不出来"); err == nil {
		t.Fatal("no error for no JSON")
	}
	// A kind written wrongly is not drawn; the others are.
	s, err = Parse(`{"flow": [], "state": "", "table": {"conds": ["a"], "results": ["b"], "rows": [{"when": ["1"]}, {"when": ["2"]}]}, "objects": {}, "gaps": [{"say": "没写"}]}`)
	if err != nil || s.Flow != nil || s.State != nil || s.Table == nil || len(s.Table.Rows) != 2 || len(s.Gaps) != 1 {
		t.Fatalf("mixed answer: %+v, %v", s, err)
	}
	// A set not cleaned, with two nodes of one ID, lays out and ends.
	twice := &Graph{Nodes: []Node{{ID: "a", Title: "x"}, {ID: "a", Title: "y"}, {ID: "c", Title: "z"}}, Edges: []Edge{{"a", "c", ""}}}
	if l := twice.LayoutFlow(); len(l.Routes) != 1 {
		t.Fatalf("layout %+v", l)
	}
}

// A flow told as steps: its main way in order, the other ways out of a
// step in words, every edge once.
func TestSteps(t *testing.T) {
	g := refund()
	steps, tangled := g.Steps()
	var ids []string
	for _, s := range steps {
		ids = append(ids, s.Node)
	}
	if strings.Join(ids, " ") != "A B C D G P V M" {
		t.Fatalf("main way %v", ids)
	}
	// 符合申请条件？: on by 是, out by 否 to 不受理, where it ends.
	b := steps[1]
	if b.Next != "是" || len(b.Exits) != 1 || b.Exits[0].Label != "否" || len(b.Exits[0].Pass) != 1 || b.Exits[0].Pass[0] != "X" || b.Exits[0].To != 0 {
		t.Fatalf("B %+v", b)
	}
	// 质量问题？: 是，免审 leaps on to 取货, the sixth step.
	if c := steps[2]; c.Next != "否" || len(c.Exits) != 1 || c.Exits[0].To != 6 || c.Exits[0].Back || len(c.Exits[0].Pass) != 0 {
		t.Fatalf("C %+v", c)
	}
	if tangled != 1 {
		t.Fatalf("tangled %d", tangled)
	}
	// A way back, and a question off the main way.
	g.Edges = append(g.Edges, Edge{"R", "A", "改后重提"}, Edge{"X", "A", "补齐资料"}, Edge{"X", "M", "特批"})
	steps, tangled = g.Steps()
	if x := steps[4].Exits[0]; x.Label != "否" || len(x.Pass) != 1 || x.Pass[0] != "R" || x.To != 1 || !x.Back {
		t.Fatalf("G's way back %+v", x)
	}
	if x := steps[1].Exits[0]; len(x.More) != 2 || x.More[0].To != 1 || x.More[1].To != 8 {
		t.Fatalf("B's question off the way %+v", x)
	}
	if tangled < 2 {
		t.Fatalf("tangled %d", tangled)
	}
	// Every edge is told once, of any graph; none is lost and none loops.
	seed := uint32(11)
	rnd := func(n int) int { seed = seed*1664525 + 1013904223; return int(seed>>16) % n }
	for range 300 {
		n := 2 + rnd(12)
		g := &Graph{}
		for i := range n {
			g.Nodes = append(g.Nodes, Node{ID: string(rune('a' + i)), Title: "x"})
		}
		for range n + rnd(n) {
			a, b := rnd(n), rnd(n)
			if a != b && !slices.ContainsFunc(g.Edges, func(e Edge) bool { return e.From == g.Nodes[a].ID && e.To == g.Nodes[b].ID }) {
				g.Edges = append(g.Edges, Edge{From: g.Nodes[a].ID, To: g.Nodes[b].ID})
			}
		}
		steps, _ := g.Steps()
		told := map[int]int{}
		nodes := map[string]bool{}
		var count func(xs []Exit)
		count = func(xs []Exit) {
			for _, x := range xs {
				told[x.Edge]++
				for _, k := range x.Chain {
					told[k]++
				}
				for _, id := range x.Pass {
					nodes[id] = true
				}
				count(x.More)
			}
		}
		for _, s := range steps {
			nodes[s.Node] = true
			count(s.Exits)
		}
		if len(nodes) != n {
			t.Fatalf("%d of %d nodes told: %+v", len(nodes), n, g)
		}
		for k, c := range told {
			if c > 1 {
				t.Fatalf("edge %d told %d times: %+v", k, c, g)
			}
		}
	}
}
