package model

// History keeps snapshots of a file for undo and redo, with the canvas
// shown and the node selected at the time.
type History struct {
	undo  []State
	redo  []State
	limit int
	// bytes, when above 0, bounds what the steps hold together: the
	// oldest go first, but for the last keep, which stay whatever they
	// weigh.
	bytes int64
	keep  int
	steps uint64 // how many steps were recorded, the number of the last
}

// State is what a step of the history restores.
type State struct {
	File     *File
	Sheet    string // the ID of the canvas shown
	Selected string // the ID of the node selected
	weight   int64  // about the bytes its file holds, once in a history
	step     uint64 // the number of the step it is the state before, or after
}

// NewHistory returns a history keeping up to limit steps.
func NewHistory(limit int) *History {
	return &History{limit: limit}
}

// Budget bounds the history by what it holds as well as by its steps: a
// step is a copy of the whole file, and two hundred of a map of five
// thousand topics were 245 MB. Steps beyond bytes are forgotten, the
// oldest first, the last keep never.
func (h *History) Budget(bytes int64, keep int) *History {
	h.bytes, h.keep = bytes, keep
	return h
}

func (s State) clone() State {
	s.File = s.File.Clone()
	s.weight = s.File.weight()
	return s
}

// Node, task and style are about what a copy of each takes: the structs
// with their slices' headers. The words of a title or a note are not
// counted, a copy sharing them with the file it was made of.
const (
	nodeWeight  = 256
	taskWeight  = 192
	styleWeight = 128
	wordWeight  = 16 // a string of a slice
)

// weight is about the bytes a copy of the file holds of its own.
func (f *File) weight() int64 {
	var w int64
	var walk func(n *Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		w += nodeWeight + wordWeight*int64(len(n.Tags)+len(n.Markers)+len(n.Links)+2*len(n.Files))
		if n.Task != nil {
			w += taskWeight
		}
		if n.Style != nil {
			w += styleWeight
		}
		if n.Summary != nil {
			walk(n.Summary.Topic)
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, d := range f.Sheets {
		walk(d.Root)
		for _, n := range d.Floating {
			walk(n)
		}
		w += nodeWeight * int64(len(d.Relations)) / 2
	}
	return w + nodeWeight*int64(len(f.Log))
}

// Size is about the bytes the history's steps hold.
func (h *History) Size() int64 {
	var w int64
	for _, s := range h.undo {
		w += s.weight
	}
	for _, s := range h.redo {
		w += s.weight
	}
	return w
}

// Steps is how many changes can be undone.
func (h *History) Steps() int { return len(h.undo) }

// trim forgets the oldest steps over the limit or the budget.
func (h *History) trim() {
	drop := max(len(h.undo)-h.limit, 0)
	if h.bytes > 0 {
		size := h.Size()
		for _, s := range h.undo[:drop] {
			size -= s.weight
		}
		for ; size > h.bytes && len(h.undo)-drop > h.keep; drop++ {
			size -= h.undo[drop].weight
		}
	}
	if drop > 0 {
		// Copied, not cut: the array would hold the files dropped.
		h.undo = append([]State(nil), h.undo[drop:]...)
	}
}

// Record saves the state before a change and forgets what could be redone.
func (h *History) Record(s State) {
	s = s.clone()
	h.steps++
	s.step = h.steps
	h.undo = append(h.undo, s)
	h.redo = nil
	h.trim()
}

// LastStep is the number of the step recorded last: what a change that
// must follow its own undo and redo keeps (Step).
func (h *History) LastStep() uint64 { return h.steps }

// Step says where the step of a number is: 1 when it is made and can be
// undone, -1 when it is undone and can be made again, 0 when it is
// neither, forgotten for its age or for a step recorded after its undo.
func (h *History) Step(step uint64) int {
	for _, s := range h.undo {
		if s.step == step {
			return 1
		}
	}
	for _, s := range h.redo {
		if s.step == step {
			return -1
		}
	}
	return 0
}

// Undo returns the state before the last change, given the current one,
// which Redo brings back. ok is false when there is nothing to undo.
func (h *History) Undo(cur State) (State, bool) {
	if len(h.undo) == 0 {
		return cur, false
	}
	s := h.undo[len(h.undo)-1]
	h.undo[len(h.undo)-1] = State{}
	h.undo = h.undo[:len(h.undo)-1]
	cur = cur.clone()
	cur.step = s.step // the same step, from its other side
	h.redo = append(h.redo, cur)
	return s.clone(), true
}

// Redo reverses the last Undo.
func (h *History) Redo(cur State) (State, bool) {
	if len(h.redo) == 0 {
		return cur, false
	}
	s := h.redo[len(h.redo)-1]
	h.redo[len(h.redo)-1] = State{}
	h.redo = h.redo[:len(h.redo)-1]
	cur = cur.clone()
	cur.step = s.step
	h.undo = append(h.undo, cur)
	h.trim()
	return s.clone(), true
}

// CanUndo reports whether there is a change to undo.
func (h *History) CanUndo() bool { return len(h.undo) > 0 }

// CanRedo reports whether there is an undone change to redo.
func (h *History) CanRedo() bool { return len(h.redo) > 0 }

// Clear forgets every step, as when another file opens.
func (h *History) Clear() { h.undo, h.redo = nil, nil }
