package model

import (
	"fmt"
	"slices"
	"time"
)

// File is what one mind map file holds: one or more canvases, each a map
// of its own, shown as tabs.
type File struct {
	Version int         `json:"version"`
	Sheets  []*Document `json:"sheets"`
	// Project is the folder of the project's code, which relative links
	// of nodes are in.
	Project string `json:"project,omitempty"`
	// GitHub is the repository ("owner/repo") whose issues sync as tasks.
	GitHub string `json:"github,omitempty"`
	// Iterations are the periods tasks are planned in, as sprints.
	Iterations []Iteration `json:"iterations,omitempty"`
	// Measures are the numbers nodes are measured by, as hours or cost,
	// in the order they show (Node.Values).
	Measures []string `json:"measures,omitempty"`
	// Analysis is what the AI made of the map, for the search.
	Analysis *Analysis `json:"analysis,omitempty"`
}

// Iteration is a period of work, as a sprint or a milestone.
type Iteration struct {
	ID    string     `json:"id"`
	Name  string     `json:"name"`
	Start *time.Time `json:"start,omitempty"`
	End   *time.Time `json:"end,omitempty"`
}

// Iteration returns the iteration with the ID, or nil.
func (f *File) Iteration(id string) *Iteration {
	for i := range f.Iterations {
		if f.Iterations[i].ID == id {
			return &f.Iterations[i]
		}
	}
	return nil
}

// Node returns the node with the ID, of any canvas, or nil.
func (f *File) Node(id string) *Node {
	if d := f.SheetOf(id); d != nil {
		n, _ := d.Find(id)
		return n
	}
	return nil
}

// Blockers are the tasks a task waits on that are not done yet.
func (f *File) Blockers(n *Node) []*Node {
	if n.Task == nil {
		return nil
	}
	var out []*Node
	for _, id := range n.Task.After {
		if b := f.Node(id); b != nil && b.Task != nil && b.Task.Status != StatusDone {
			out = append(out, b)
		}
	}
	return out
}

// NewFile returns a file with one canvas whose root has the given title.
func NewFile(title string) *File {
	d := New(title)
	d.Title = "画布 1"
	return &File{Version: FormatVersion, Sheets: []*Document{d}}
}

// Sheet returns the canvas with the ID and its index, or nil and -1.
func (f *File) Sheet(id string) (*Document, int) {
	for i, d := range f.Sheets {
		if d.ID == id {
			return d, i
		}
	}
	return nil, -1
}

// SheetOf returns the canvas holding the node with the ID, or nil.
func (f *File) SheetOf(nodeID string) *Document {
	for _, d := range f.Sheets {
		if n, _ := d.Find(nodeID); n != nil {
			return d
		}
	}
	return nil
}

// NextSheetTitle returns "画布 N" for the first N no canvas is titled.
func (f *File) NextSheetTitle() string {
	for i := len(f.Sheets) + 1; ; i++ {
		t := fmt.Sprintf("画布 %d", i)
		if !slices.ContainsFunc(f.Sheets, func(d *Document) bool { return d.Title == t }) {
			return t
		}
	}
}

// AddSheet inserts a new canvas after index (at the end for -1) and
// returns it.
func (f *File) AddSheet(after int) *Document {
	d := New("中心主题")
	d.Title = f.NextSheetTitle()
	if after < 0 || after >= len(f.Sheets) {
		f.Sheets = append(f.Sheets, d)
	} else {
		f.Sheets = slices.Insert(f.Sheets, after+1, d)
	}
	return d
}

// DuplicateSheet inserts a copy of the canvas at index after it, with new
// IDs, and returns it.
func (f *File) DuplicateSheet(index int) *Document {
	d := f.Sheets[index].Clone()
	d.ID = NewID()
	d.Root.renew()
	d.Title += " 副本"
	f.Sheets = slices.Insert(f.Sheets, index+1, d)
	return d
}

// DeleteSheet removes the canvas at index. The last canvas stays.
func (f *File) DeleteSheet(index int) error {
	if len(f.Sheets) <= 1 {
		return ErrLastSheet
	}
	f.Sheets = slices.Delete(f.Sheets, index, index+1)
	return nil
}

// MoveSheet moves the canvas at index from to index to.
func (f *File) MoveSheet(from, to int) {
	to = min(max(to, 0), len(f.Sheets)-1)
	if from == to {
		return
	}
	d := f.Sheets[from]
	f.Sheets = slices.Delete(f.Sheets, from, from+1)
	f.Sheets = slices.Insert(f.Sheets, to, d)
}

// Clone returns a deep copy of the file.
func (f *File) Clone() *File {
	c := &File{Version: f.Version, Project: f.Project, GitHub: f.GitHub, Sheets: make([]*Document, len(f.Sheets)),
		Measures: slices.Clone(f.Measures), Analysis: f.Analysis} // an analysis is replaced, not changed
	for _, it := range f.Iterations {
		if it.Start != nil {
			s := *it.Start
			it.Start = &s
		}
		if it.End != nil {
			e := *it.End
			it.End = &e
		}
		c.Iterations = append(c.Iterations, it)
	}
	for i, d := range f.Sheets {
		c.Sheets[i] = d.Clone()
	}
	return c
}

// Tasks returns the tasks of every canvas, in order.
func (f *File) Tasks() []TaskNode {
	var out []TaskNode
	for _, d := range f.Sheets {
		for _, t := range d.Tasks() {
			t.Sheet = d
			out = append(out, t)
		}
	}
	return out
}

// AllTags returns every tag used in any canvas, sorted.
func (f *File) AllTags() []string {
	var tags []string
	for _, d := range f.Sheets {
		for _, t := range d.AllTags() {
			if !slices.Contains(tags, t) {
				tags = append(tags, t)
			}
		}
	}
	slices.Sort(tags)
	return tags
}

// Walk calls fn for every node of every canvas.
func (f *File) Walk(fn func(n *Node)) {
	for _, d := range f.Sheets {
		d.Walk(func(n, _ *Node, _ int) bool { fn(n); return true })
	}
}
