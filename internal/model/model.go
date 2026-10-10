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
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Note      string     `json:"note,omitempty"`
	Tags      []string   `json:"tags,omitempty"`
	Markers   []string   `json:"markers,omitempty"` // as "flag:red", one of each group
	Task      *Task      `json:"task,omitempty"`
	Collapsed bool       `json:"collapsed,omitempty"`
	Style     *NodeStyle `json:"style,omitempty"`
	Image     *Image     `json:"image,omitempty"`
	Children  []*Node    `json:"children,omitempty"`
}

// Image is a picture shown in a node. Its file is in the map's assets
// folder (AssetsDir), so the map's JSON stays small and diffs well.
type Image struct {
	File string `json:"file"` // the file's name in the assets folder
	W    int    `json:"w"`    // its size in pixels
	H    int    `json:"h"`
}

// NodeStyle overrides how one node looks. What it leaves zero follows the
// canvas's style.
type NodeStyle struct {
	Shape       Shape   `json:"shape,omitempty"`
	Fill        string  `json:"fill,omitempty"`   // #rrggbb
	Border      string  `json:"border,omitempty"` // #rrggbb
	BorderWidth float32 `json:"borderWidth,omitempty"`
	Width       float32 `json:"width,omitempty"` // of the title, which wraps at it; 0 fits the text
	FontSize    float32 `json:"fontSize,omitempty"`
	Color       string  `json:"color,omitempty"` // of the text, #rrggbb
	Bold        bool    `json:"bold,omitempty"`
	Italic      bool    `json:"italic,omitempty"`
	Strike      bool    `json:"strike,omitempty"`
	Underline   bool    `json:"underline,omitempty"`
	Align       string  `json:"align,omitempty"` // "left", "center" or "right"; "" is left
}

// Shape is the outline of a node.
type Shape string

const (
	ShapeAuto      Shape = ""          // the canvas's: rounded
	ShapeRect      Shape = "rect"      // square corners
	ShapeRounded   Shape = "rounded"   // rounded corners
	ShapePill      Shape = "pill"      // round ends
	ShapeUnderline Shape = "underline" // no box, a line under the text
	ShapeNone      Shape = "none"      // the text alone
)

// Shapes lists the shapes in the order the app offers them.
var Shapes = []Shape{ShapeAuto, ShapeRect, ShapeRounded, ShapePill, ShapeUnderline, ShapeNone}

// Label is the shape's name in the UI.
func (s Shape) Label() string {
	switch s {
	case ShapeRect:
		return "矩形"
	case ShapeRounded:
		return "圆角"
	case ShapePill:
		return "胶囊"
	case ShapeUnderline:
		return "下划线"
	case ShapeNone:
		return "无边框"
	}
	return "自动"
}

// Document is one canvas of a file: a tree of nodes under its root.
type Document struct {
	ID        string      `json:"id"`
	Title     string      `json:"title"`
	Structure Structure   `json:"structure,omitempty"`
	Style     CanvasStyle `json:"style,omitzero"`
	Present   Present     `json:"present,omitzero"`
	Root      *Node       `json:"root"`
}

// Present is how a canvas shows as slides.
type Present struct {
	Theme  string `json:"theme,omitempty"`  // "" (dark), "light" or "color"
	Aspect string `json:"aspect,omitempty"` // "" (fills the screen), "16:9" or "4:3"
	Layout string `json:"layout,omitempty"` // of list slides: "" (list) or "cards"
	// Reveal shows a list slide's items one at a time.
	Reveal bool `json:"reveal,omitempty"`
}

// CanvasStyle is how a canvas looks: its color scheme, font and lines.
// The zero value is the default look.
type CanvasStyle struct {
	Scheme string `json:"scheme,omitempty"` // a color scheme's ID; "" is the default
	Font   string `json:"font,omitempty"`   // a font family; "" is the system's
	Line   string `json:"line,omitempty"`   // the branches' width: "thin", "" (normal), "thick" or "bolder"
	// SingleColor draws every branch in Color instead of a color of the
	// scheme for each (rainbow branches).
	SingleColor bool   `json:"singleColor,omitempty"`
	Color       string `json:"color,omitempty"` // #rrggbb
}

// Structure is how a canvas lays out its map.
type Structure string

const (
	// StructureMindMap puts the root in the middle and its branches on
	// both sides, joined by curves. It is the default, "".
	StructureMindMap Structure = ""
	// StructureLogic grows the map to the right, each node's children
	// joined to it by a trunk of straight lines.
	StructureLogic Structure = "logic"
	// StructureBracket grows the map to the right, each node's children
	// held by a curly bracket.
	StructureBracket Structure = "bracket"
)

// Structures lists the structures in the order the app offers them.
var Structures = []Structure{StructureMindMap, StructureLogic, StructureBracket}

// Label is the structure's name in the UI.
func (s Structure) Label() string {
	switch s {
	case StructureLogic:
		return "逻辑图"
	case StructureBracket:
		return "括号图"
	}
	return "思维导图"
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
	return &Document{ID: d.ID, Title: d.Title, Structure: d.Structure, Style: d.Style, Present: d.Present, Root: d.Root.clone()}
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
	c.Markers = slices.Clone(n.Markers)
	if n.Style != nil {
		st := *n.Style
		c.Style = &st
	}
	if n.Image != nil {
		img := *n.Image
		c.Image = &img
	}
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
