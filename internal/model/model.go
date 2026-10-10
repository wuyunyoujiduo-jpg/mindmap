// Package model holds a mind map's data: a tree of nodes, each with a
// note, tags and an optional task. It knows nothing about the UI.
package model

import (
	"crypto/rand"
	"encoding/hex"
	"slices"
	"strings"
	"time"
)

// FormatVersion is written into every saved file. Version 1 held one map
// (a root); version 2 holds several canvases (sheets).
const FormatVersion = 2

// Status is where a task stands.
type Status string

const (
	StatusIdea  Status = "idea"  // 想法
	StatusTodo  Status = "todo"  // 待做
	StatusDoing Status = "doing" // 进行中
	StatusDone  Status = "done"  // 完成
)

// Statuses lists the statuses in workflow order.
var Statuses = []Status{StatusIdea, StatusTodo, StatusDoing, StatusDone}

// Label is the status's name in the UI.
func (s Status) Label() string {
	switch s {
	case StatusIdea:
		return "想法"
	case StatusTodo:
		return "待做"
	case StatusDoing:
		return "进行中"
	case StatusDone:
		return "完成"
	}
	return string(s)
}

// StatusFromLabel is the inverse of Status.Label.
func StatusFromLabel(label string) Status {
	for _, s := range Statuses {
		if s.Label() == label {
			return s
		}
	}
	return StatusTodo
}

// Priority orders tasks; 0 is unset.
type Priority int

const (
	PriorityNone Priority = iota
	PriorityLow
	PriorityMedium
	PriorityHigh
)

// Priorities lists the priorities from none to high.
var Priorities = []Priority{PriorityNone, PriorityLow, PriorityMedium, PriorityHigh}

// Label is the priority's name in the UI.
func (p Priority) Label() string {
	switch p {
	case PriorityLow:
		return "低"
	case PriorityMedium:
		return "中"
	case PriorityHigh:
		return "高"
	}
	return "无"
}

// PriorityFromLabel is the inverse of Priority.Label.
func PriorityFromLabel(label string) Priority {
	for _, p := range Priorities {
		if p.Label() == label {
			return p
		}
	}
	return PriorityNone
}

// Task makes a node something to do.
type Task struct {
	Status   Status     `json:"status"`
	Priority Priority   `json:"priority,omitempty"`
	Start    *time.Time `json:"start,omitempty"`
	Due      *time.Time `json:"due,omitempty"`
}

// Overdue reports whether the task is unfinished past its due day.
func (t *Task) Overdue(now time.Time) bool {
	if t == nil || t.Due == nil || t.Status == StatusDone {
		return false
	}
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	return t.Due.Before(today)
}

// Node is one topic of the map.
type Node struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Note      string   `json:"note,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Task      *Task    `json:"task,omitempty"`
	Collapsed bool     `json:"collapsed,omitempty"`
	Children  []*Node  `json:"children,omitempty"`
}

// Document is one canvas of a file: a tree of nodes under its root.
type Document struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Root  *Node  `json:"root"`
}

// NewID returns a new random node ID.
func NewID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// NewNode returns a node with a new ID.
func NewNode(title string) *Node {
	return &Node{ID: NewID(), Title: title}
}

// New returns a canvas whose root has the given title.
func New(title string) *Document {
	return &Document{ID: NewID(), Root: NewNode(title)}
}

// Walk calls fn for every node depth first, parents before children, with
// the node's parent (nil for the root) and depth. It stops early when fn
// returns false.
func (d *Document) Walk(fn func(n, parent *Node, depth int) bool) {
	var walk func(n, parent *Node, depth int) bool
	walk = func(n, parent *Node, depth int) bool {
		if !fn(n, parent, depth) {
			return false
		}
		for _, c := range n.Children {
			if !walk(c, n, depth+1) {
				return false
			}
		}
		return true
	}
	if d.Root != nil {
		walk(d.Root, nil, 0)
	}
}

// Find returns the node with the ID and its parent, or nil.
func (d *Document) Find(id string) (node, parent *Node) {
	d.Walk(func(n, p *Node, _ int) bool {
		if n.ID == id {
			node, parent = n, p
			return false
		}
		return true
	})
	return node, parent
}

// Path returns the nodes from the root down to the node with the ID, or
// nil when there is none.
func (d *Document) Path(id string) []*Node {
	var path []*Node
	var walk func(n *Node) bool
	walk = func(n *Node) bool {
		path = append(path, n)
		if n.ID == id {
			return true
		}
		for _, c := range n.Children {
			if walk(c) {
				return true
			}
		}
		path = path[:len(path)-1]
		return false
	}
	if d.Root == nil || !walk(d.Root) {
		return nil
	}
	return path
}

// Contains reports whether the node with the ID is n or below it.
func (n *Node) Contains(id string) bool {
	if n.ID == id {
		return true
	}
	for _, c := range n.Children {
		if c.Contains(id) {
			return true
		}
	}
	return false
}

// HasTag reports whether the node carries the tag.
func (n *Node) HasTag(tag string) bool {
	return slices.Contains(n.Tags, tag)
}

// AllTags returns every tag in use, sorted.
func (d *Document) AllTags() []string {
	seen := map[string]bool{}
	var tags []string
	d.Walk(func(n, _ *Node, _ int) bool {
		for _, t := range n.Tags {
			if !seen[t] {
				seen[t] = true
				tags = append(tags, t)
			}
		}
		return true
	})
	slices.Sort(tags)
	return tags
}

// Matches reports whether the node's title, note or tags contain the
// query, ignoring case.
func (n *Node) Matches(query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return true
	}
	if strings.Contains(strings.ToLower(n.Title), q) || strings.Contains(strings.ToLower(n.Note), q) {
		return true
	}
	for _, t := range n.Tags {
		if strings.Contains(strings.ToLower(t), q) {
			return true
		}
	}
	return false
}

// Clone returns a deep copy of the canvas.
func (d *Document) Clone() *Document {
	return &Document{ID: d.ID, Title: d.Title, Root: d.Root.clone()}
}

// renew gives the node and everything below it new IDs.
func (n *Node) renew() {
	n.ID = NewID()
	for _, c := range n.Children {
		c.renew()
	}
}

func (n *Node) clone() *Node {
	if n == nil {
		return nil
	}
	c := *n
	c.Tags = slices.Clone(n.Tags)
	if n.Task != nil {
		t := *n.Task
		if n.Task.Start != nil {
			s := *n.Task.Start
			t.Start = &s
		}
		if n.Task.Due != nil {
			due := *n.Task.Due
			t.Due = &due
		}
		c.Task = &t
	}
	c.Children = make([]*Node, len(n.Children))
	for i, ch := range n.Children {
		c.Children[i] = ch.clone()
	}
	if len(c.Children) == 0 {
		c.Children = nil
	}
	return &c
}
