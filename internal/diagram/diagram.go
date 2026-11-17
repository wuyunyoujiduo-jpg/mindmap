// Package diagram holds what the AI draws of a topic's branch, to be
// understood by: a flow in lanes, the states of a thing, a table of what
// is decided when, the nouns of the matter, options side by side, and
// cases to walk them with.
// It lays the flow and the states out on a grid. It knows nothing about
// the UI, and the map stays the one source: a set is drawn again, never
// edited.
package diagram

import (
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Kinds of a node of a flow or of the states.
const (
	KindStep     = ""         // a step, or a state
	KindStart    = "start"    // where a flow begins; among states, the dot before the first
	KindDecision = "decision" // a question whose answers are its edges
	KindEnd      = "end"      // an end that is neither good nor bad
	KindOK       = "ok"       // the end hoped for
	KindBad      = "bad"      // an end refused
)

// Set is what was drawn of one topic's branch.
type Set struct {
	Node  string    `json:"node"`            // the topic's ID
	Title string    `json:"title,omitempty"` // its title when drawn
	Drawn time.Time `json:"drawn"`
	// Seen is a hash of each topic of the branch as it was when drawn, by
	// its ID: by it the set tells what the map says otherwise now.
	Seen map[string]string `json:"seen,omitempty"`
	// Told is what the user asked of the drawing, kept for the next.
	Told string `json:"told,omitempty"`
	// Partial tells that the branch was too long to be given whole: what
	// was drawn is of its beginning.
	Partial bool `json:"partial,omitempty"`
	// Present has the set's diagrams among the slides of its canvas, after
	// its topic's.
	Present bool     `json:"present,omitempty"`
	Flow    *Graph   `json:"flow,omitempty"`
	State   *Graph   `json:"state,omitempty"`
	Table   *Table   `json:"table,omitempty"`
	Objects []Object `json:"objects,omitempty"`
	Cases   []Case   `json:"cases,omitempty"`
	Gaps    []Gap    `json:"gaps,omitempty"`
	Quiz    *Quiz    `json:"quiz,omitempty"`
	// Compare sets options side by side. A version before it reads a set
	// without it: the other kinds show, and a set of it alone not at all.
	Compare *Compare `json:"compare,omitempty"`
	// Lean is the option of the comparison the user leans to, by its name:
	// the one thing of a set written by hand, and so beside what is drawn,
	// kept when the set is drawn again while the option is still there.
	Lean string `json:"lean,omitempty"`
	// CompareQuiz is how a column of the comparison was last recalled.
	CompareQuiz *Quiz `json:"compare_quiz,omitempty"`
}

// Compare sets options side by side: a column an option, a row what they
// are compared by. It says what the map says of each and no more: which
// is the better is the user's to say (Set.Lean).
type Compare struct {
	Options []string   `json:"options"`
	Rows    []Compared `json:"rows"`
}

// Compared is a row of a comparison: what the options are compared by,
// and a cell an option. Same tells that every option says the same there.
type Compared struct {
	Dim   string `json:"dim"`
	Cells []Cell `json:"cells"`
	Same  bool   `json:"same,omitempty"`
}

// Cell is what the map says of an option in a row, and the topics it says
// so in. Gap marks one the map does not speak of, which has no text: it is
// left so, and not filled in.
type Cell struct {
	Text string   `json:"text,omitempty"`
	Src  []string `json:"src,omitempty"`
	Gap  bool     `json:"gap,omitempty"`
}

// UnmarshalJSON reads a cell, taken too as models write one for short: a
// text alone, or nothing for one unsaid.
func (c *Cell) UnmarshalJSON(data []byte) error {
	var text string
	if json.Unmarshal(data, &text) == nil {
		*c = Cell{Text: text}
		return nil
	}
	type plain Cell
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*c = Cell(p)
	return nil
}

// Gaps counts the cells the map does not speak of.
func (t *Compare) Gaps() int {
	n := 0
	for _, r := range t.Rows {
		for _, c := range r.Cells {
			if c.Gap {
				n++
			}
		}
	}
	return n
}

// Sames counts the rows in which every option says the same.
func (t *Compare) Sames() int {
	n := 0
	for _, r := range t.Rows {
		if r.Same {
			n++
		}
	}
	return n
}

// Option is the column of the option named, from 0, or -1.
func (t *Compare) Option(name string) int {
	if t == nil || name == "" {
		return -1
	}
	return slices.Index(t.Options, name)
}

// Graph is a flow (with lanes, and how it is told step by step) or the
// states of a thing.
type Graph struct {
	Lanes []string `json:"lanes,omitempty"`
	Nodes []Node   `json:"nodes"`
	Edges []Edge   `json:"edges"`
	Tell  []Step   `json:"tell,omitempty"`
}

// Node is a box of a graph. Src are the topics of the map it was drawn
// from.
type Node struct {
	ID    string   `json:"id"`
	Kind  string   `json:"kind,omitempty"`
	Title string   `json:"title"`
	Sub   string   `json:"sub,omitempty"`
	Lane  int      `json:"lane,omitempty"`
	Src   []string `json:"src,omitempty"`
}

// Edge is an arrow of a graph.
type Edge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label,omitempty"`
}

// Step is a sentence of a flow told step by step, and the nodes it is on.
type Step struct {
	Nodes []string `json:"nodes"`
	Say   string   `json:"say"`
}

// Table says what is decided in which case: a row's When answers the
// conditions, its Then the results.
type Table struct {
	Conds   []string `json:"conds"`
	Results []string `json:"results"`
	Rows    []Row    `json:"rows"`
	Notes   []Note   `json:"notes,omitempty"`
}

// Row is a case of a table. Gap marks one the map does not decide, and
// Why says what is unsaid.
type Row struct {
	When []string `json:"when"`
	Then []string `json:"then"`
	Src  []string `json:"src,omitempty"`
	Gap  bool     `json:"gap,omitempty"`
	Why  string   `json:"why,omitempty"`
}

// Note is a rule beside a table, which its columns do not hold.
type Note struct {
	Text string   `json:"text"`
	Src  []string `json:"src,omitempty"`
}

// Object is a noun of the matter: who acts, what is handled, a state.
// Where the map speaks of it is looked up as it is shown, by its name and
// its other names.
type Object struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind,omitempty"`
	What    string   `json:"what,omitempty"`
	Aliases []string `json:"aliases,omitempty"`
}

// Case is one concrete instance to walk the diagrams with: the nodes of
// the flow it passes, the states it takes, the row of the table that
// decides it (from 1; 0 is none).
type Case struct {
	Title  string   `json:"title"`
	Facts  []string `json:"facts,omitempty"`
	Flow   []string `json:"flow,omitempty"`
	States []string `json:"states,omitempty"`
	Row    int      `json:"row,omitempty"`
	Result string   `json:"result,omitempty"`
	Gap    bool     `json:"gap,omitempty"`
}

// Gap is something the map leaves unsaid, or says twice otherwise.
type Gap struct {
	Say string   `json:"say"`
	Src []string `json:"src,omitempty"`
	Row int      `json:"row,omitempty"` // of the table, from 1; 0 is none
}

// Quiz is how the table was last recalled: the rows missed (from 1), when,
// and when to ask again. Of a comparison, Col is the column that was
// covered (from 1).
type Quiz struct {
	Col    int       `json:"col,omitempty"`
	Missed []int     `json:"missed,omitempty"`
	At     time.Time `json:"at"`
	Next   time.Time `json:"next"`
}

// What a set may hold at most: more is not understood at a glance, and an
// answer gone wrong does not grow without end.
const (
	maxFlowNodes  = 16
	maxStates     = 10
	maxLanes      = 5
	maxEdges      = 30
	maxTell       = 16
	maxConds      = 6
	maxResults    = 3
	maxRows       = 12
	maxNotes      = 4
	maxObjects    = 14
	maxAliases    = 4
	maxCases      = 6
	maxFacts      = 6
	maxGaps       = 5
	maxTitleRunes = 18
	maxSubRunes   = 24
	maxCellRunes  = 20
	maxSayRunes   = 120
	maxOptions    = 6
	maxDims       = 12
	maxOptRunes   = 14
	maxCmpRunes   = 30
)

// Kinds are the kinds of diagram a set has, in the order shown.
func (s *Set) Kinds() []string {
	var k []string
	if s.Flow != nil {
		k = append(k, "flow")
	}
	if s.State != nil {
		k = append(k, "state")
	}
	if s.Table != nil {
		k = append(k, "table")
	}
	if len(s.Objects) > 0 {
		k = append(k, "objects")
	}
	if s.Compare != nil {
		k = append(k, "compare")
	}
	return k
}

// Empty tells whether nothing was drawn.
func (s *Set) Empty() bool { return len(s.Kinds()) == 0 }

// Clean makes an answer of the AI safe to draw: what names nothing is
// dropped, what is too much is cut, and a diagram with too little left is
// not kept. src gives the topic an answer's source stands for, or false.
func (s *Set) Clean(src func(string) (string, bool)) {
	srcs := func(in []string) []string {
		var out []string
		for _, v := range in {
			if id, ok := src(strings.TrimSpace(v)); ok && !slices.Contains(out, id) {
				out = append(out, id)
			}
		}
		return out
	}
	s.Flow = cleanGraph(s.Flow, false, srcs)
	s.State = cleanGraph(s.State, true, srcs)
	s.Table = cleanTable(s.Table, srcs)
	s.Compare = cleanCompare(s.Compare, srcs)
	if s.Compare.Option(s.Lean) < 0 { // the option leant to is gone
		s.Lean = ""
	}
	if q := s.CompareQuiz; q != nil && (s.Compare == nil || q.Col < 1 || q.Col > len(s.Compare.Options)) {
		s.CompareQuiz = nil
	}

	var objs []Object
	for _, o := range s.Objects {
		o.Name, o.Kind, o.What = cut(o.Name, maxTitleRunes), cut(o.Kind, 8), cut(o.What, maxSayRunes)
		if o.Name == "" || slices.ContainsFunc(objs, func(p Object) bool { return p.Name == o.Name }) {
			continue
		}
		var al []string
		for _, a := range o.Aliases {
			if a = cut(a, maxTitleRunes); a != "" && a != o.Name && !slices.Contains(al, a) && len(al) < maxAliases {
				al = append(al, a)
			}
		}
		o.Aliases = al
		if objs = append(objs, o); len(objs) == maxObjects {
			break
		}
	}
	s.Objects = objs

	rows := 0
	if s.Table != nil {
		rows = len(s.Table.Rows)
	}
	var cases []Case
	for _, c := range s.Cases {
		if c.Title = cut(c.Title, maxTitleRunes); c.Title == "" {
			continue
		}
		c.Facts = cutAll(c.Facts, maxCellRunes, maxFacts)
		c.Flow, c.States = known(s.Flow, c.Flow), known(s.State, c.States)
		if c.Row < 0 || c.Row > rows {
			c.Row = 0
		}
		c.Result = cut(c.Result, maxSayRunes)
		if cases = append(cases, c); len(cases) == maxCases {
			break
		}
	}
	s.Cases = cases

	var gaps []Gap
	for _, g := range s.Gaps {
		if g.Say = cut(g.Say, maxSayRunes); g.Say == "" {
			continue
		}
		g.Src = srcs(g.Src)
		if g.Row < 0 || g.Row > rows {
			g.Row = 0
		}
		if gaps = append(gaps, g); len(gaps) == maxGaps {
			break
		}
	}
	s.Gaps = gaps
}

func cleanGraph(g *Graph, states bool, srcs func([]string) []string) *Graph {
	if g == nil {
		return nil
	}
	limit := maxFlowNodes
	if states {
		limit = maxStates + 1 // and the dot they begin at
		g.Lanes, g.Tell = nil, nil
	}
	g.Lanes = cutAll(g.Lanes, 10, maxLanes)
	var nodes []Node
	start := ""
	rename := map[string]string{} // a second dot is the first
	for _, n := range g.Nodes {
		n.ID, n.Title, n.Sub = strings.TrimSpace(n.ID), cut(n.Title, maxTitleRunes), cut(n.Sub, maxSubRunes)
		switch n.Kind {
		case KindStart, KindDecision, KindEnd, KindOK, KindBad:
		default:
			n.Kind = KindStep
		}
		if states && n.Kind == KindDecision {
			n.Kind = KindStep
		}
		if n.ID == "" || slices.ContainsFunc(nodes, func(m Node) bool { return m.ID == n.ID }) {
			continue
		}
		if states && n.Kind == KindStart {
			if start != "" {
				rename[n.ID] = start
				continue
			}
			start, n.Title, n.Sub = n.ID, "", ""
		} else if n.Title == "" {
			continue
		}
		if n.Lane < 0 || n.Lane >= max(len(g.Lanes), 1) {
			n.Lane = 0
		}
		n.Src = srcs(n.Src)
		if len(nodes) < limit {
			nodes = append(nodes, n)
		}
	}
	has := func(id string) bool { return slices.ContainsFunc(nodes, func(n Node) bool { return n.ID == id }) }
	if states && start == "" && len(nodes) > 0 { // they begin somewhere: at the first
		start = "_start"
		nodes = append([]Node{{ID: start, Kind: KindStart}}, nodes...)
		g.Edges = append([]Edge{{From: start, To: nodes[1].ID}}, g.Edges...)
	}
	var edges []Edge
	for _, e := range g.Edges {
		if to, ok := rename[e.From]; ok {
			e.From = to
		}
		e.Label = cut(e.Label, 8)
		if e.From == e.To || !has(e.From) || !has(e.To) || states && e.To == start ||
			slices.ContainsFunc(edges, func(f Edge) bool { return f.From == e.From && f.To == e.To }) {
			continue
		}
		if edges = append(edges, e); len(edges) == maxEdges {
			break
		}
	}
	real := len(nodes)
	if states {
		real--
	}
	if real < 2 || len(edges) == 0 {
		return nil
	}
	var tell []Step
	for _, st := range g.Tell {
		st.Say = cut(st.Say, maxSayRunes)
		var ids []string
		for _, id := range st.Nodes {
			if has(id) && !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
		if st.Nodes = ids; st.Say == "" || len(ids) == 0 {
			continue
		}
		if tell = append(tell, st); len(tell) == maxTell {
			break
		}
	}
	g.Nodes, g.Edges, g.Tell = nodes, edges, tell
	return g
}

func cleanTable(t *Table, srcs func([]string) []string) *Table {
	if t == nil {
		return nil
	}
	t.Conds, t.Results = cutAll(t.Conds, 10, maxConds), cutAll(t.Results, 10, maxResults)
	if len(t.Conds) == 0 || len(t.Results) == 0 {
		return nil
	}
	fit := func(in []string, n int) []string {
		out := make([]string, n)
		for i := range out {
			if i < len(in) {
				out[i] = cut(in[i], maxCellRunes)
			}
		}
		return out
	}
	var rows []Row
	for _, r := range t.Rows {
		r.When, r.Then = fit(r.When, len(t.Conds)), fit(r.Then, len(t.Results))
		if strings.Join(r.When, "") == "" {
			continue
		}
		r.Src, r.Why = srcs(r.Src), cut(r.Why, maxSayRunes)
		if rows = append(rows, r); len(rows) == maxRows {
			break
		}
	}
	if len(rows) < 2 {
		return nil
	}
	var notes []Note
	for _, n := range t.Notes {
		if n.Text = cut(n.Text, maxSayRunes); n.Text == "" {
			continue
		}
		n.Src = srcs(n.Src)
		if notes = append(notes, n); len(notes) == maxNotes {
			break
		}
	}
	t.Rows, t.Notes = rows, notes
	return t
}

// verdicts are the words of a row that judges between the options: the
// better, a rank, a score, what to choose. A comparison says what the map
// says of each; a row of these is the AI's own and is not kept.
var verdicts = []string{"结论", "推荐", "建议选", "选哪", "哪个好", "哪个更", "谁更", "更好", "最好", "最佳", "首选", "优选", "胜出", "优胜",
	"排名", "名次", "评分", "打分", "得分", "总分", "总评", "综合评", "综合来看", "总结", "优劣", "倾向", "赢家"}

// Verdict tells whether a row's name is one that judges between the
// options.
func Verdict(dim string) bool {
	return slices.ContainsFunc(verdicts, func(w string) bool { return strings.Contains(dim, w) })
}

// unsaid are the words models write in a cell for nothing said.
var unsaid = []string{"", "-", "—", "–", "?", "？", "/", "未提及", "未说明", "未提到", "没说", "没有说", "没提", "不详", "未知",
	"导图里没说", "导图没说", "n/a", "N/A", "null"}

func cleanCompare(t *Compare, srcs func([]string) []string) *Compare {
	if t == nil {
		return nil
	}
	// The options kept, and where each was among those given: a cell is of
	// its option by its place.
	var opts []string
	var at []int
	for i, o := range t.Options {
		if o = cut(o, maxOptRunes); o != "" && !slices.Contains(opts, o) && len(opts) < maxOptions {
			opts, at = append(opts, o), append(at, i)
		}
	}
	if len(opts) < 2 {
		return nil
	}
	var rows []Compared
	for _, r := range t.Rows {
		if r.Dim = cut(r.Dim, maxOptRunes); r.Dim == "" || Verdict(r.Dim) ||
			slices.ContainsFunc(rows, func(p Compared) bool { return p.Dim == r.Dim }) {
			continue
		}
		cells := make([]Cell, len(opts))
		said, alike := 0, true
		for i, from := range at {
			var c Cell
			if from < len(r.Cells) {
				c = r.Cells[from]
			}
			c.Text = cut(c.Text, maxCmpRunes)
			if c.Gap || slices.Contains(unsaid, c.Text) {
				c = Cell{Gap: true} // unsaid: no words, and nothing it came from
			} else {
				c.Src = srcs(c.Src)
				said++
			}
			cells[i] = c
			alike = alike && c.Text == cells[0].Text
		}
		if said == 0 { // a row of nothing said compares nothing
			continue
		}
		// The same in every option: by their words, or as the answer says
		// of words that differ; never with one unsaid.
		r.Cells, r.Same = cells, said == len(opts) && (alike || r.Same)
		if rows = append(rows, r); len(rows) == maxDims {
			break
		}
	}
	if len(rows) == 0 {
		return nil
	}
	t.Options, t.Rows = opts, rows
	return t
}

// known keeps the IDs that name a node of the graph.
func known(g *Graph, ids []string) []string {
	if g == nil {
		return nil
	}
	var out []string
	for _, id := range ids {
		if g.Node(id) != nil {
			out = append(out, id)
		}
	}
	return out
}

// cut trims a text and cuts it at n runes.
func cut(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func cutAll(in []string, runes, count int) []string {
	var out []string
	for _, s := range in {
		if s = cut(s, runes); s != "" && len(out) < count {
			out = append(out, s)
		}
	}
	return out
}

// Node returns the node with the ID, or nil.
func (g *Graph) Node(id string) *Node {
	if g == nil {
		return nil
	}
	for i := range g.Nodes {
		if g.Nodes[i].ID == id {
			return &g.Nodes[i]
		}
	}
	return nil
}

// PathEdges are the edges a walk through the nodes takes, by their index:
// from each to the next, where there is one.
func (g *Graph) PathEdges(ids []string) []int {
	var out []int
	for i := 0; i+1 < len(ids); i++ {
		for k, e := range g.Edges {
			if e.From == ids[i] && e.To == ids[i+1] {
				out = append(out, k)
				break
			}
		}
	}
	return out
}

// Parse reads the set of an answer of the AI: the JSON object in it,
// around which the model may have put words or a fence, taken with what
// models get wrong of a form: a number where a text was asked, a text
// where a number was.
func Parse(text string) (*Set, error) {
	i, j := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if i < 0 || j < i {
		return nil, errors.New("no JSON object")
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(text[i:j+1]), &raw); err != nil {
		return nil, err
	}
	// A kind at a time: one written wrongly ("flow": [] for none, a list
	// where a thing was asked) is not drawn, and the others are.
	var s Set
	for key, into := range map[string]any{"flow": &s.Flow, "state": &s.State, "table": &s.Table,
		"objects": &s.Objects, "cases": &s.Cases, "gaps": &s.Gaps, "compare": &s.Compare} {
		v, ok := raw[key]
		if !ok || v == nil {
			continue
		}
		switch v.(type) {
		case map[string]any:
			if key == "objects" || key == "cases" || key == "gaps" {
				continue
			}
		case []any:
			if key == "flow" || key == "state" || key == "table" || key == "compare" {
				continue
			}
		default:
			continue
		}
		if b, err := json.Marshal(tidy("", v)); err == nil {
			_ = json.Unmarshal(b, into)
		}
	}
	return &s, nil
}

// The keys of an answer by what they hold.
var (
	textKeys  = []string{"id", "from", "to", "kind", "title", "sub", "label", "say", "name", "what", "text", "why", "result", "dim"}
	textsKeys = []string{"src", "lanes", "conds", "results", "when", "then", "facts", "flow", "states", "aliases", "nodes", "options"}
	intKeys   = []string{"lane", "row"}
	boolKeys  = []string{"gap", "same"}
)

// tidy gives each value of an answer the type its key is read as.
func tidy(key string, v any) any {
	scalar := func(v any) (string, bool) {
		switch x := v.(type) {
		case string:
			return x, true
		case float64:
			return strconv.FormatFloat(x, 'f', -1, 64), true
		case bool:
			return strconv.FormatBool(x), true
		}
		return "", false
	}
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			x[k] = tidy(k, val)
		}
		return x
	case []any:
		for i, item := range x {
			if s, ok := scalar(item); ok && slices.Contains(textsKeys, key) {
				x[i] = s
			} else {
				x[i] = tidy("", item)
			}
		}
		return x
	}
	s, ok := scalar(v)
	switch {
	case !ok:
		return v
	case slices.Contains(textKeys, key):
		return s
	case slices.Contains(textsKeys, key): // one where several were asked
		return []any{s}
	case slices.Contains(intKeys, key):
		n, _ := strconv.Atoi(strings.TrimSpace(s))
		return n
	case slices.Contains(boolKeys, key):
		return s == "true" || s == "1"
	}
	return v
}

// Renamed gives the set with its topics renamed: ids gives the new ID of
// each that has one, as when its branch was cut and pasted elsewhere.
func (s Set) Renamed(ids map[string]string) Set {
	re := func(in []string) []string {
		out := make([]string, len(in))
		for i, id := range in {
			out[i] = id
			if to, ok := ids[id]; ok {
				out[i] = to
			}
		}
		return out
	}
	graph := func(g *Graph) *Graph {
		if g == nil {
			return nil
		}
		c := *g
		c.Nodes = slices.Clone(g.Nodes)
		for i := range c.Nodes {
			c.Nodes[i].Src = re(c.Nodes[i].Src)
		}
		return &c
	}
	if to, ok := ids[s.Node]; ok {
		s.Node = to
	}
	seen := map[string]string{}
	for id, h := range s.Seen {
		if to, ok := ids[id]; ok {
			id = to
		}
		// What is under what is in it, by the parent's ID.
		// "parent/index/hash": read from its end, an ID from elsewhere
		// having what it will in it.
		if i := strings.LastIndex(h, "/"); i > 0 {
			if j := strings.LastIndex(h[:i], "/"); j >= 0 {
				if to, renamed := ids[h[:j]]; renamed {
					h = to + h[j:]
				}
			}
		}
		seen[id] = h
	}
	s.Seen = seen
	s.Flow, s.State = graph(s.Flow), graph(s.State)
	if s.Table != nil {
		t := *s.Table
		t.Rows, t.Notes = slices.Clone(t.Rows), slices.Clone(t.Notes)
		for i := range t.Rows {
			t.Rows[i].Src = re(t.Rows[i].Src)
		}
		for i := range t.Notes {
			t.Notes[i].Src = re(t.Notes[i].Src)
		}
		s.Table = &t
	}
	if s.Compare != nil {
		t := *s.Compare
		t.Rows = slices.Clone(t.Rows)
		for i := range t.Rows {
			t.Rows[i].Cells = slices.Clone(t.Rows[i].Cells)
			for k := range t.Rows[i].Cells {
				if c := &t.Rows[i].Cells[k]; len(c.Src) > 0 {
					c.Src = re(c.Src)
				}
			}
		}
		s.Compare = &t
	}
	s.Gaps = slices.Clone(s.Gaps)
	for i := range s.Gaps {
		s.Gaps[i].Src = re(s.Gaps[i].Src)
	}
	return s
}

// Markdown writes the set out to be kept elsewhere: the flow and the
// states as Mermaid, which most places that take Markdown draw, the table
// as a table, the nouns and what is unsaid as lists.
func (s *Set) Markdown() string {
	var b strings.Builder
	esc := func(t string) string { return strings.NewReplacer(`"`, "'", "\n", " ", "|", "/").Replace(t) }
	if s.Title != "" {
		b.WriteString("# " + s.Title + "\n\n")
	}
	if g := s.Flow; g != nil {
		b.WriteString("## 流程\n\n```mermaid\nflowchart TD\n")
		name := func(n Node) string {
			t := esc(n.Title)
			if n.Sub != "" {
				t += "<br/>" + esc(n.Sub)
			}
			return t
		}
		draw := func(n Node) {
			switch n.Kind {
			case KindDecision:
				b.WriteString("    " + mid(n.ID) + "{\"" + name(n) + "\"}\n")
			case KindStart, KindEnd, KindOK, KindBad:
				b.WriteString("    " + mid(n.ID) + "([\"" + name(n) + "\"])\n")
			default:
				b.WriteString("    " + mid(n.ID) + "[\"" + name(n) + "\"]\n")
			}
		}
		if len(g.Lanes) > 1 {
			for i, lane := range g.Lanes {
				b.WriteString("  subgraph L" + strconv.Itoa(i) + "[\"" + esc(lane) + "\"]\n")
				for _, n := range g.Nodes {
					if n.Lane == i {
						draw(n)
					}
				}
				b.WriteString("  end\n")
			}
		} else {
			for _, n := range g.Nodes {
				draw(n)
			}
		}
		for _, e := range g.Edges {
			if e.Label != "" {
				b.WriteString("    " + mid(e.From) + " -->|\"" + esc(e.Label) + "\"| " + mid(e.To) + "\n")
			} else {
				b.WriteString("    " + mid(e.From) + " --> " + mid(e.To) + "\n")
			}
		}
		b.WriteString("```\n\n")
	}
	if g := s.State; g != nil {
		b.WriteString("## 状态\n\n```mermaid\nstateDiagram-v2\n")
		at := func(id string) string {
			if n := g.Node(id); n != nil && n.Kind == KindStart {
				return "[*]"
			}
			return mid(id)
		}
		for _, n := range g.Nodes {
			if n.Kind != KindStart {
				b.WriteString("    " + mid(n.ID) + ": " + esc(n.Title) + "\n")
			}
		}
		for _, e := range g.Edges {
			b.WriteString("    " + at(e.From) + " --> " + at(e.To))
			if e.Label != "" {
				b.WriteString(": " + esc(e.Label))
			}
			b.WriteString("\n")
		}
		b.WriteString("```\n\n")
	}
	if t := s.Table; t != nil {
		head := append(slices.Clone(t.Conds), t.Results...)
		for i, h := range head {
			head[i] = esc(h)
		}
		b.WriteString("## 判定表\n\n| " + strings.Join(head, " | ") + " |\n|" + strings.Repeat("---|", len(head)) + "\n")
		for _, r := range t.Rows {
			cells := append(slices.Clone(r.When), r.Then...)
			for i, c := range cells {
				cells[i] = esc(c)
				if c == "" {
					cells[i] = "—"
				}
			}
			if r.Gap {
				cells[len(r.When)] = "没说清：" + esc(r.Why)
			}
			b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
		}
		for _, n := range t.Notes {
			b.WriteString("\n- " + n.Text)
		}
		b.WriteString("\n\n")
	}
	if len(s.Objects) > 0 {
		b.WriteString("## 名词\n\n")
		for _, o := range s.Objects {
			b.WriteString("- **" + o.Name + "**")
			if o.Kind != "" {
				b.WriteString("（" + o.Kind + "）")
			}
			if o.What != "" {
				b.WriteString("：" + o.What)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if t := s.Compare; t != nil {
		b.WriteString("## 对比\n\n| |")
		for _, o := range t.Options {
			b.WriteString(" " + esc(o) + " |")
		}
		b.WriteString("\n|---|" + strings.Repeat("---|", len(t.Options)) + "\n")
		for _, r := range t.Rows {
			b.WriteString("| " + esc(r.Dim) + " |")
			for _, c := range r.Cells {
				if c.Gap {
					b.WriteString(" 导图里没说 |")
				} else {
					b.WriteString(" " + esc(c.Text) + " |")
				}
			}
			b.WriteString("\n")
		}
		if s.Lean != "" {
			b.WriteString("\n我的倾向：" + esc(s.Lean) + "\n")
		}
		b.WriteString("\n")
	}
	if len(s.Gaps) > 0 {
		b.WriteString("## 没说清的地方\n\n")
		for _, g := range s.Gaps {
			b.WriteString("- " + g.Say + "\n")
		}
	}
	return strings.TrimSpace(b.String()) + "\n"
}

// mid is a node's ID as Mermaid takes one: letters, digits and _.
func mid(id string) string {
	var b strings.Builder
	b.WriteString("n")
	for _, r := range id {
		if r < 128 && (r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			b.WriteRune(r)
		} else {
			b.WriteString("_" + strconv.Itoa(int(r)) + "_") // closed, so that no two IDs make one
		}
	}
	return b.String()
}
