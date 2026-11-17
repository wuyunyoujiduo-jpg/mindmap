package model

import (
	"bytes"
	"encoding/json"
	"slices"
	"sort"
	"strings"
)

// Merge joins two versions of a map changed apart from a version both
// came from, as two computers syncing: topic by topic, by their IDs, and
// field by field of each. What one side changed and the other did not is
// taken; a topic, a canvas or a line deleted on one side is deleted unless
// the other changed it, and a topic kept under one deleted keeps it too. A
// field changed on both sides, unless both changed it alike, is ours,
// and a clash for the user to choose (File.TakeTheirs); the tags and
// other lists of words take what each added and dropped. A topic moved on both sides goes where ours put
// it; a child order changed on both is ours, with theirs' new children
// after the sibling they followed there.
//
// A picture, a place, a rule of repeating and a baseline are each one
// value (atoms): taken field by field, two pictures made one of a file
// and another's size, asked about three times. The markers are one of a
// group, so they are merged group by group. A summary's topic is a topic
// as any other, found by its ID, not a part of the one it sums up: a
// topic moved under it on one side and renamed on the other was in the
// map twice.
//
// base nil merges two versions with no known common one: what is in
// either is kept, and nothing is deleted.
func Merge(base, ours, theirs *File) (merged *File, clashes []Clash) {
	m := &merger{noBase: base == nil}
	if base == nil {
		base = &File{}
	}
	merged = &File{}
	m.at = Clash{}
	m.into(merged, m.object(nil, fileFields(base), fileFields(ours), fileFields(theirs)))
	merged.Log = m.log(base.Log, ours.Log, theirs.Log)

	b, o, t := indexTree(base), indexTree(ours), indexTree(theirs)

	// The canvases, as they are without their topics.
	sheets := map[string]*Document{}
	keptSheet := map[string]bool{}
	for _, id := range unionKeys(b.sheets, o.sheets, t.sheets) {
		bs, os, ts := b.sheets[id], o.sheets[id], t.sheets[id]
		switch {
		case os != nil && ts != nil:
		case os != nil && (bs == nil || !sameJSON(os, bs)),
			ts != nil && (bs == nil || !sameJSON(ts, bs)):
		default:
			continue // deleted, and not changed on the other side
		}
		keptSheet[id] = true
		if os == nil || ts == nil {
			bs = nil // kept as the side that has it
		}
		d := &Document{}
		m.at = Clash{Sheet: id}
		m.into(d, m.object(nil, sheetFields(bs), sheetFields(os), sheetFields(ts)))
		d.ID = id
		sheets[id] = d
	}

	// The topics: what each is, and where.
	content := map[string]*Node{}
	at := map[string]string{}
	for _, id := range unionKeys(b.nodes, o.nodes, t.nodes) {
		bn, on, tn := b.nodes[id], o.nodes[id], t.nodes[id]
		switch {
		case on != nil && tn != nil:
			m.at = Clash{Node: id}
			content[id] = m.node(bn, on, tn)
			at[id] = on.at
			if bn != nil && on.at == bn.at {
				at[id] = tn.at
			}
		case on != nil:
			if bn != nil && on.at == bn.at && sameJSON(nodeFields(on.n), nodeFields(bn.n)) {
				continue // theirs deleted it, ours left it
			}
			content[id], at[id] = on.n, on.at
		case tn != nil:
			if bn != nil && tn.at == bn.at && sameJSON(nodeFields(tn.n), nodeFields(bn.n)) {
				continue
			}
			content[id], at[id] = tn.n, tn.at
		}
	}
	// A topic kept under one deleted keeps it: the parents come back.
	for changed := true; changed; {
		changed = false
		for id := range at {
			p := ownerOf(at[id])
			if _, ok := content[p]; ok || isSheetKey(p) {
				continue
			}
			for _, x := range []*treeIndex{o, t, b} {
				if e := x.nodes[p]; e != nil {
					content[p], at[p] = e.n, e.at
					changed = true
					break
				}
			}
			if _, ok := content[p]; !ok { // of none: cannot be
				at[id] = ""
			}
		}
	}
	// Two moves apart can make a loop, one topic under the other and that
	// under the first: the topics in it go where ours has them.
	for range len(at) + 1 {
		loop := findLoop(at)
		if loop == nil {
			break
		}
		moved := false
		for _, id := range loop {
			if e := o.nodes[id]; e != nil && at[id] != e.at {
				at[id], moved = e.at, true
			}
		}
		if !moved {
			at[loop[0]] = "" // to the first canvas's root, below
		}
	}
	// The canvases a kept topic is on are kept.
	for id, p := range at {
		if p == "" {
			continue
		}
		s := sheetOfKey(p)
		if s == "" || keptSheet[s] {
			continue
		}
		for _, x := range []*treeIndex{o, t, b} {
			if d := x.sheets[s]; d != nil {
				nd := &Document{}
				m.into(nd, m.object(nil, nil, sheetFields(d), nil))
				nd.ID = s
				sheets[s], keptSheet[s] = nd, true
				break
			}
		}
		if !keptSheet[s] {
			at[id] = ""
		}
	}

	// The children of each parent, in order.
	under := map[string][]string{}
	for id, p := range at {
		under[p] = append(under[p], id)
	}
	children := func(key string) []string {
		ids := under[key]
		if len(ids) == 0 {
			return nil
		}
		return order(b.kids[key], o.kids[key], t.kids[key], ids)
	}
	var build func(id string) *Node
	build = func(id string) *Node {
		n := content[id].clone()
		n.Children = nil
		if n.Summary != nil {
			n.Summary.Topic = nil
		}
		for _, c := range children(id) {
			n.Children = append(n.Children, build(c))
		}
		// Its summary's topic: the first there, and one more (each side
		// gave it one) a child.
		for i, c := range children(sumKey(id)) {
			if i > 0 {
				n.Children = append(n.Children, build(c))
				continue
			}
			if n.Summary == nil { // taken off on one side, its topic changed on the other: kept
				n.Summary = &Mark{}
			}
			n.Summary.Topic = build(c)
		}
		if n.Summary != nil && n.Summary.Topic == nil && n.Summary.Label == "" {
			n.Summary = nil // a summary is its topic, which is gone
		}
		return n
	}
	for _, id := range order(b.sheetOrder, o.sheetOrder, t.sheetOrder, keys(keptSheet)) {
		d := sheets[id]
		for i, r := range children(rootKey(id)) {
			if i == 0 {
				d.Root = build(r)
			} else { // a second root, of no version: kept, floating
				d.Floating = append(d.Floating, build(r))
			}
		}
		for _, f := range children(floatKey(id)) {
			d.Floating = append(d.Floating, build(f))
		}
		if d.Root == nil {
			d.Root = NewNode(d.Title)
		}
		merged.Sheets = append(merged.Sheets, d)
	}
	if len(merged.Sheets) == 0 {
		merged.Sheets = []*Document{New("画布 1")}
	}
	// Topics of nowhere (a loop not undone, a canvas of none): under the
	// first canvas's root.
	if ids := under[""]; len(ids) > 0 {
		sort.Strings(ids)
		root := merged.Sheets[0].Root
		for _, id := range ids {
			root.Children = append(root.Children, build(id))
		}
	}

	// The lines between topics, on the canvas each is of.
	present := map[string]bool{}
	for _, d := range merged.Sheets {
		d.Walk(func(n, _ *Node, _ int) bool { present[n.ID] = true; return true })
	}
	for _, d := range merged.Sheets {
		bs, os, ts := b.sheets[d.ID], o.sheets[d.ID], t.sheets[d.ID]
		rel := func(x *Document) []Relation {
			if x == nil {
				return nil
			}
			return x.Relations
		}
		m.at = Clash{quiet: true}
		for _, r := range m.relations(rel(bs), rel(os), rel(ts)) {
			if present[r.From] && present[r.To] {
				d.Relations = append(d.Relations, r)
			}
		}
	}
	merged.Version = FormatVersion
	return merged, m.clashes
}

// Clash is a field both sides changed, each its own way: the merge has
// ours, and theirs can be taken instead.
type Clash struct {
	Node  string // the topic's ID; "" for a canvas's or the map's own field
	Sheet string // the canvas's ID, for a canvas's own field
	// Field is where the field is in the JSON of the topic, the canvas or
	// the map: ["title"], ["task", "status"].
	Field              []string
	Base, Ours, Theirs json.RawMessage // nil for none; Base nil too when there is no base
	NoBase             bool            // merged with no version both came from
	quiet              bool            // of a line between topics: not asked
}

type merger struct {
	noBase  bool
	clashes []Clash
	at      Clash // what is being merged
}

// value merges one JSON value three ways; nil is none.
func (m *merger) value(path []string, b, o, t json.RawMessage) json.RawMessage {
	b, o, t = orNil(b), orNil(o), orNil(t)
	switch {
	case bytes.Equal(o, t):
		return o
	case bytes.Equal(o, b):
		return t
	case bytes.Equal(t, b):
		return o
	}
	if isObject(o) && isObject(t) && (b == nil || isObject(b)) && !atom(path) {
		return m.object(path, b, o, t)
	}
	if len(path) > 0 && path[len(path)-1] == "markers" {
		if v, ok := m.markers(path, b, o, t); ok {
			return v
		}
	}
	if os, ok := words(o); ok {
		if ts, ok := words(t); ok {
			if bs, ok := words(b); ok || b == nil {
				return wordsMerge(bs, os, ts)
			}
		}
	}
	if !m.at.quiet {
		c := m.at
		c.Field, c.Base, c.Ours, c.Theirs, c.NoBase = slices.Clone(path), b, o, t, m.noBase
		m.clashes = append(m.clashes, c)
	}
	return o
}

// atom tells whether the value at path is one thing, though written as
// several fields: a picture (its file and size), a place, a line's bend,
// how a task repeats, the code's baseline.
func atom(path []string) bool {
	if len(path) == 0 {
		return false
	}
	switch path[len(path)-1] {
	case "image", "pos", "bend", "repeat", "baseline":
		return true
	}
	return false
}

// markers merges a topic's markers, "group:value" and one of a group:
// group by group, each side's change of a group taken, and a group both
// changed, each its own way, ours and a clash, whose other choice is the
// markers with theirs of those groups. ok false when they are not lists.
func (m *merger) markers(path []string, b, o, t json.RawMessage) (json.RawMessage, bool) {
	os, ok1 := words(o)
	ts, ok2 := words(t)
	bs, ok3 := words(b)
	if !(ok1 || o == nil) || !(ok2 || t == nil) || !(ok3 || b == nil) {
		return nil, false
	}
	group := func(w string) string {
		g, _, _ := strings.Cut(w, ":")
		return g
	}
	of := func(ws []string) map[string]string {
		out := map[string]string{}
		for _, w := range ws {
			out[group(w)] = w
		}
		return out
	}
	bm, om, tm := of(bs), of(os), of(ts)
	var order []string // ours' groups as they stand, then theirs' new ones
	for _, ws := range [][]string{os, ts, bs} {
		for _, w := range ws {
			if g := group(w); !slices.Contains(order, g) {
				order = append(order, g)
			}
		}
	}
	var ours, theirs []string
	clash := false
	for _, g := range order {
		bw, ow, tw := bm[g], om[g], tm[g]
		switch {
		case ow == tw, tw == bw:
			if ow != "" {
				ours, theirs = append(ours, ow), append(theirs, ow)
			}
		case ow == bw:
			if tw != "" {
				ours, theirs = append(ours, tw), append(theirs, tw)
			}
		default: // both changed it, each its own way
			clash = true
			if ow != "" {
				ours = append(ours, ow)
			}
			if tw != "" {
				theirs = append(theirs, tw)
			}
		}
	}
	list := func(ws []string) json.RawMessage {
		if len(ws) == 0 {
			return nil
		}
		data, _ := json.Marshal(ws)
		return data
	}
	if clash && !m.at.quiet {
		c := m.at
		c.Field, c.Base, c.Ours, c.Theirs, c.NoBase = slices.Clone(path), b, list(ours), list(theirs), m.noBase
		m.clashes = append(m.clashes, c)
	}
	return list(ours), true
}

// object merges JSON objects key by key.
func (m *merger) object(path []string, b, o, t json.RawMessage) json.RawMessage {
	var bm, om, tm map[string]json.RawMessage
	_ = json.Unmarshal(orNil(b), &bm)
	_ = json.Unmarshal(orNil(o), &om)
	_ = json.Unmarshal(orNil(t), &tm)
	out := map[string]json.RawMessage{}
	for _, k := range unionKeys(bm, om, tm) {
		if v := m.value(append(slices.Clip(path), k), bm[k], om[k], tm[k]); v != nil {
			out[k] = v
		}
	}
	data, _ := json.Marshal(out)
	return data
}

// into decodes what was merged into v.
func (m *merger) into(v any, data json.RawMessage) { _ = json.Unmarshal(data, v) }

// node merges a topic's own fields, its children aside.
func (m *merger) node(b, o, t *treeEntry) *Node {
	var bf json.RawMessage
	if b != nil {
		bf = nodeFields(b.n)
	}
	n := &Node{}
	m.into(n, m.object(nil, bf, nodeFields(o.n), nodeFields(t.n)))
	return n
}

// log merges the agents' logs: what either added, less what either
// took out.
func (m *merger) log(b, o, t []LogEntry) []LogEntry {
	inB := map[string]bool{}
	for _, e := range b {
		inB[e.ID] = true
	}
	inO, inT := map[string]bool{}, map[string]bool{}
	for _, e := range o {
		inO[e.ID] = true
	}
	for _, e := range t {
		inT[e.ID] = true
	}
	var out []LogEntry
	for _, e := range o {
		if !inB[e.ID] || inT[e.ID] {
			out = append(out, e)
		}
	}
	for _, e := range t {
		if !inO[e.ID] && !inB[e.ID] {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	if n := len(out) - maxLog; n > 0 {
		out = out[n:]
	}
	return out
}

// relations merges the lines of a canvas by their IDs, as topics are.
func (m *merger) relations(b, o, t []Relation) []Relation {
	byID := func(rs []Relation) map[string]Relation {
		out := map[string]Relation{}
		for _, r := range rs {
			out[r.ID] = r
		}
		return out
	}
	bm, om, tm := byID(b), byID(o), byID(t)
	var out []Relation
	add := func(id string) {
		br, inB := bm[id]
		or, inO := om[id]
		tr, inT := tm[id]
		switch {
		case inO && inT:
			var bj json.RawMessage
			if inB {
				bj, _ = json.Marshal(br)
			}
			oj, _ := json.Marshal(or)
			tj, _ := json.Marshal(tr)
			var r Relation
			m.into(&r, m.object(nil, bj, oj, tj))
			out = append(out, r)
		case inO && !(inB && sameJSON(or, br)):
			out = append(out, or)
		case inT && !(inB && sameJSON(tr, br)):
			out = append(out, tr)
		}
	}
	for _, r := range o {
		add(r.ID)
	}
	for _, r := range t {
		if _, ok := om[r.ID]; !ok {
			add(r.ID)
		}
	}
	return out
}

// treeIndex is a version's canvases and topics, each topic with where it
// is: its parent's ID, or the key of its canvas's root or floating topics.
type treeIndex struct {
	sheets     map[string]*Document
	sheetOrder []string
	nodes      map[string]*treeEntry
	kids       map[string][]string // by parent key, in order
}

type treeEntry struct {
	n  *Node
	at string
}

func rootKey(sheet string) string  { return "\x00root\x00" + sheet }
func floatKey(sheet string) string { return "\x00float\x00" + sheet }

// sumKey is where the topic of a topic's summary is: beside its children,
// not among them.
func sumKey(node string) string { return sumPrefix + node }

const sumPrefix = "\x00sum\x00"

// ownerOf is the topic a place is of: the topic itself, or the one whose
// summary it is; a canvas's key as it is.
func ownerOf(k string) string {
	if strings.HasPrefix(k, sumPrefix) {
		return k[len(sumPrefix):]
	}
	return k
}

func isSheetKey(k string) bool { return len(k) > 0 && k[0] == 0 }

// sheetOfKey is the canvas of a root's or floating topics' key; "" for a
// topic's ID.
func sheetOfKey(k string) string {
	for _, p := range []string{"\x00root\x00", "\x00float\x00"} {
		if len(k) > len(p) && k[:len(p)] == p {
			return k[len(p):]
		}
	}
	return ""
}

func indexTree(f *File) *treeIndex {
	x := &treeIndex{sheets: map[string]*Document{}, nodes: map[string]*treeEntry{}, kids: map[string][]string{}}
	var walk func(n *Node, at string)
	walk = func(n *Node, at string) {
		if n == nil {
			return
		}
		x.nodes[n.ID] = &treeEntry{n, at}
		x.kids[at] = append(x.kids[at], n.ID)
		for _, c := range n.Children {
			walk(c, n.ID)
		}
		walk(n.SummaryTopic(), sumKey(n.ID))
	}
	for _, d := range f.Sheets {
		x.sheets[d.ID] = d
		x.sheetOrder = append(x.sheetOrder, d.ID)
		walk(d.Root, rootKey(d.ID))
		for _, fl := range d.Floating {
			walk(fl, floatKey(d.ID))
		}
	}
	return x
}

// findLoop is the topics of a loop of parents, or nil.
func findLoop(at map[string]string) []string {
	done := map[string]bool{}
	for _, start := range keys(at) {
		seen := map[string]int{}
		var path []string
		for id := start; ; id = ownerOf(at[id]) {
			if done[id] || isSheetKey(id) || id == "" {
				break
			}
			if i, ok := seen[id]; ok {
				return path[i:]
			}
			if _, ok := at[id]; !ok {
				break
			}
			seen[id] = len(path)
			path = append(path, id)
		}
		for _, id := range path {
			done[id] = true
		}
	}
	return nil
}

// order puts ids, the children of one parent, in order: as in the side
// that reordered them (ours when both did), with what is only in the
// other after the sibling it followed there, and the rest at the end.
func order(b, o, t, ids []string) []string {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	first, second := o, t
	if slices.Equal(o, b) {
		first, second = t, o
	}
	var out []string
	placed := map[string]bool{}
	for _, id := range first {
		if want[id] && !placed[id] {
			out = append(out, id)
			placed[id] = true
		}
	}
	for i, id := range second {
		if !want[id] || placed[id] {
			continue
		}
		pos := 0
		for j := i - 1; j >= 0; j-- {
			if k := slices.Index(out, second[j]); k >= 0 {
				pos = k + 1
				break
			}
		}
		out = slices.Insert(out, pos, id)
		placed[id] = true
	}
	var rest []string
	for _, id := range ids {
		if !placed[id] {
			rest = append(rest, id)
		}
	}
	sort.Slice(rest, func(i, j int) bool {
		bi, bj := slices.Index(b, rest[i]), slices.Index(b, rest[j])
		if bi != bj {
			return bi < bj
		}
		return rest[i] < rest[j]
	})
	return append(out, rest...)
}

func fileFields(f *File) json.RawMessage {
	if f == nil {
		return nil
	}
	c := *f
	c.Sheets, c.Log, c.Version = nil, nil, 0
	data, _ := json.Marshal(c)
	return data
}

func sheetFields(d *Document) json.RawMessage {
	if d == nil {
		return nil
	}
	c := *d
	c.Root, c.Floating, c.Relations = nil, nil, nil
	data, _ := json.Marshal(c)
	return data
}

// nodeFields is a topic's own fields: without its children, nor its
// summary's topic, which is a topic of its own.
func nodeFields(n *Node) json.RawMessage {
	c := *n
	c.Children = nil
	if c.Summary != nil && c.Summary.Topic != nil {
		s := *c.Summary
		s.Topic = nil
		c.Summary = &s
	}
	data, _ := json.Marshal(c)
	return data
}

func orNil(v json.RawMessage) json.RawMessage {
	v = bytes.TrimSpace(v)
	if len(v) == 0 || bytes.Equal(v, []byte("null")) {
		return nil
	}
	return v
}

func isObject(v json.RawMessage) bool { return len(v) > 0 && v[0] == '{' }

// words reads a list of strings, as tags, links and markers are.
func words(v json.RawMessage) ([]string, bool) {
	if len(v) == 0 || v[0] != '[' {
		return nil, false
	}
	var out []string
	return out, json.Unmarshal(v, &out) == nil
}

// wordsMerge is ours, with what theirs added and less what it dropped.
func wordsMerge(b, o, t []string) json.RawMessage {
	var out []string
	for _, w := range o {
		if !slices.Contains(b, w) || slices.Contains(t, w) {
			out = append(out, w)
		}
	}
	for _, w := range t {
		if !slices.Contains(b, w) && !slices.Contains(out, w) {
			out = append(out, w)
		}
	}
	if len(out) == 0 {
		return nil
	}
	data, _ := json.Marshal(out)
	return data
}

func unionKeys[V any](ms ...map[string]V) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range ms {
		for k := range m {
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	sort.Strings(out)
	return out
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TakeTheirs puts theirs in place of ours at a clash of a merge; false
// when what it is of is no more.
func (f *File) TakeTheirs(c Clash) bool {
	switch {
	case c.Node != "":
		n := f.Node(c.Node)
		if n == nil {
			return false
		}
		var nn Node
		if json.Unmarshal(setField(nodeFields(n), c.Field, c.Theirs), &nn) != nil {
			return false
		}
		nn.ID, nn.Children = n.ID, n.Children
		if t := n.SummaryTopic(); t != nil { // its summary's topic is not of its fields
			if nn.Summary == nil {
				nn.Summary = &Mark{}
			}
			nn.Summary.Topic = t
		}
		*n = nn
	case c.Sheet != "":
		d, _ := f.Sheet(c.Sheet)
		if d == nil {
			return false
		}
		var nd Document
		if json.Unmarshal(setField(sheetFields(d), c.Field, c.Theirs), &nd) != nil {
			return false
		}
		nd.ID, nd.Root, nd.Floating, nd.Relations = d.ID, d.Root, d.Floating, d.Relations
		*d = nd
	default:
		var nf File
		if json.Unmarshal(setField(fileFields(f), c.Field, c.Theirs), &nf) != nil {
			return false
		}
		nf.Sheets, nf.Log, nf.Version = f.Sheets, f.Log, f.Version
		*f = nf
	}
	return true
}

// setField sets the value at path in a JSON object; nil takes it out.
func setField(obj json.RawMessage, path []string, v json.RawMessage) json.RawMessage {
	if len(path) == 0 {
		return v
	}
	var m map[string]json.RawMessage
	_ = json.Unmarshal(orNil(obj), &m)
	if m == nil {
		m = map[string]json.RawMessage{}
	}
	if w := setField(m[path[0]], path[1:], v); orNil(w) == nil {
		delete(m, path[0])
	} else {
		m[path[0]] = w
	}
	data, _ := json.Marshal(m)
	return data
}
