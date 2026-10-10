package model

// History keeps snapshots of a document for undo and redo.
type History struct {
	undo  []snapshot
	redo  []snapshot
	limit int
}

type snapshot struct {
	doc      *Document
	selected string
}

// NewHistory returns a history keeping up to limit steps.
func NewHistory(limit int) *History {
	return &History{limit: limit}
}

// Record saves the document as it is before a change, with the node
// selected then, and forgets what could be redone.
func (h *History) Record(d *Document, selected string) {
	h.undo = append(h.undo, snapshot{d.Clone(), selected})
	if len(h.undo) > h.limit {
		h.undo = h.undo[len(h.undo)-h.limit:]
	}
	h.redo = nil
}

// Undo returns the document before the last change, given the current
// one, which redo brings back. ok is false when there is nothing to undo.
func (h *History) Undo(cur *Document, selected string) (d *Document, sel string, ok bool) {
	if len(h.undo) == 0 {
		return cur, selected, false
	}
	s := h.undo[len(h.undo)-1]
	h.undo = h.undo[:len(h.undo)-1]
	h.redo = append(h.redo, snapshot{cur.Clone(), selected})
	return s.doc.Clone(), s.selected, true
}

// Redo reverses the last Undo.
func (h *History) Redo(cur *Document, selected string) (d *Document, sel string, ok bool) {
	if len(h.redo) == 0 {
		return cur, selected, false
	}
	s := h.redo[len(h.redo)-1]
	h.redo = h.redo[:len(h.redo)-1]
	h.undo = append(h.undo, snapshot{cur.Clone(), selected})
	return s.doc.Clone(), s.selected, true
}

// CanUndo reports whether there is a change to undo.
func (h *History) CanUndo() bool { return len(h.undo) > 0 }

// CanRedo reports whether there is an undone change to redo.
func (h *History) CanRedo() bool { return len(h.redo) > 0 }

// Clear forgets every step, as when another file opens.
func (h *History) Clear() { h.undo, h.redo = nil, nil }
