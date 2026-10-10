package model

// History keeps snapshots of a file for undo and redo, with the canvas
// shown and the node selected at the time.
type History struct {
	undo  []State
	redo  []State
	limit int
}

// State is what a step of the history restores.
type State struct {
	File     *File
	Sheet    string // the ID of the canvas shown
	Selected string // the ID of the node selected
}

// NewHistory returns a history keeping up to limit steps.
func NewHistory(limit int) *History {
	return &History{limit: limit}
}

func (s State) clone() State {
	s.File = s.File.Clone()
	return s
}

// Record saves the state before a change and forgets what could be redone.
func (h *History) Record(s State) {
	h.undo = append(h.undo, s.clone())
	if len(h.undo) > h.limit {
		h.undo = h.undo[len(h.undo)-h.limit:]
	}
	h.redo = nil
}

// Undo returns the state before the last change, given the current one,
// which Redo brings back. ok is false when there is nothing to undo.
func (h *History) Undo(cur State) (State, bool) {
	if len(h.undo) == 0 {
		return cur, false
	}
	s := h.undo[len(h.undo)-1]
	h.undo = h.undo[:len(h.undo)-1]
	h.redo = append(h.redo, cur.clone())
	return s.clone(), true
}

// Redo reverses the last Undo.
func (h *History) Redo(cur State) (State, bool) {
	if len(h.redo) == 0 {
		return cur, false
	}
	s := h.redo[len(h.redo)-1]
	h.redo = h.redo[:len(h.redo)-1]
	h.undo = append(h.undo, cur.clone())
	return s.clone(), true
}

// CanUndo reports whether there is a change to undo.
func (h *History) CanUndo() bool { return len(h.undo) > 0 }

// CanRedo reports whether there is an undone change to redo.
func (h *History) CanRedo() bool { return len(h.redo) > 0 }

// Clear forgets every step, as when another file opens.
func (h *History) Clear() { h.undo, h.redo = nil, nil }
