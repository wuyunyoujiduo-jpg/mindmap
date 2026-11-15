package diagram

import (
	"fmt"
	"math/rand"
	"testing"
)

func TestZZStepsFuzz(t *testing.T) {
	kinds := []string{KindStart, KindStep, KindDecision, KindEnd, KindOK, KindBad}
	fails := map[string]int{}
	examples := map[string]string{}
	for it := 0; it < 20000; it++ {
		r := rand.New(rand.NewSource(int64(it)))
		n := 1 + r.Intn(9)
		g := &Graph{}
		for l := 0; l < r.Intn(5); l++ {
			g.Lanes = append(g.Lanes, fmt.Sprint("L", l))
		}
		for i := 0; i < n; i++ {
			g.Nodes = append(g.Nodes, Node{ID: fmt.Sprint("n", i), Title: "t", Kind: kinds[r.Intn(len(kinds))], Lane: r.Intn(5)})
		}
		for e := 0; e < r.Intn(3*n+1); e++ {
			g.Edges = append(g.Edges, Edge{From: fmt.Sprint("n", r.Intn(n)), To: fmt.Sprint("n", r.Intn(n))})
		}
		s := Set{Flow: g}
		s.Clean(func(v string) (string, bool) { return v, true })
		g = s.Flow
		if g == nil || len(g.Nodes) == 0 {
			continue
		}
		desc := fmt.Sprintf("%+v", *g)
		var steps []StepLine
		func() {
			defer func() {
				if p := recover(); p != nil {
					fails["panic"]++
					examples["panic"] = desc
				}
			}()
			steps, _ = g.Steps()
		}()
		count := map[string]int{}
		edges := map[int]int{}
		var walk func(xs []Exit)
		walk = func(xs []Exit) {
			for _, x := range xs {
				edges[x.Edge]++
				for _, c := range x.Chain {
					edges[c]++
				}
				for _, p := range x.Pass {
					count[p]++
				}
				if x.To < 0 || x.To > len(steps) {
					fails["To out of range"]++
					examples["To out of range"] = desc
				}
				walk(x.More)
			}
		}
		for i, st := range steps {
			count[st.Node]++
			if st.Next != "" && i+1 < len(steps) {
				for k, e := range g.Edges {
					if e.From == st.Node && e.To == steps[i+1].Node {
						edges[k]++
					}
				}
			}
			walk(st.Exits)
		}
		for _, nd := range g.Nodes {
			if count[nd.ID] != 1 {
				k := fmt.Sprint("node told ", min(count[nd.ID], 2), " times")
				fails[k]++
				if len(examples[k]) == 0 || len(desc) < len(examples[k]) {
					examples[k] = desc
				}
			}
		}
		for k := range g.Edges {
			if edges[k] != 1 {
				appended := false
				for _, st := range steps {
					if st.Node == g.Edges[k].From && len(st.Exits) == 0 && st.Next == "" {
						appended = true
					}
				}
				kk := fmt.Sprint("edge told ", min(edges[k], 2), " times appendedFrom=", appended)
				fails[kk]++
				if len(examples[kk]) == 0 || len(desc) < len(examples[kk]) {
					examples[kk] = desc
				}
			}
		}
	}
	for k, v := range fails {
		t.Logf("%s: %d\n  e.g. %s", k, v, examples[k])
	}
}

func TestZZMainWay(t *testing.T) {
	g := &Graph{Nodes: []Node{{ID: "a", Kind: KindStart, Title: "a"}, {ID: "b", Title: "b"}, {ID: "c", Title: "c"}, {ID: "d", Kind: KindEnd, Title: "d"}},
		Edges: []Edge{{From: "a", To: "c"}, {From: "a", To: "b"}, {From: "b", To: "c"}, {From: "c", To: "d"}}}
	steps, tg := g.Steps()
	for _, s := range steps {
		t.Logf("%s next=%q exits=%+v", s.Node, s.Next, s.Exits)
	}
	t.Log(tg, sideLanes(g, forward(g)))
	g2 := &Graph{Nodes: []Node{{ID: "s1", Kind: KindStart, Title: "a"}, {ID: "s2", Kind: KindStart, Title: "b"}, {ID: "x", Title: "x"}, {ID: "y", Title: "y"}},
		Edges: []Edge{{From: "s1", To: "x"}, {From: "s2", To: "y"}, {From: "y", To: "x"}}}
	steps, _ = g2.Steps()
	for _, s := range steps {
		t.Logf("%s next=%q exits=%+v", s.Node, s.Next, s.Exits)
	}
}
