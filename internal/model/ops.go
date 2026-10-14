package model

import (
	"errors"
	"slices"
)

// Errors of the editing operations.
var (
	ErrNotFound  = errors.New("节点不存在")
	ErrRoot      = errors.New("不能对根节点执行此操作")
	ErrCycle     = errors.New("不能把节点移动到它自己的下级")
	ErrLastSheet = errors.New("至少要保留一个画布")
)

// AddChild appends a new child to the node with the ID, opening it, and
// returns the child.
func (d *Document) AddChild(parentID, title string) (*Node, error) {
	p, _ := d.Find(parentID)
	if p == nil {
		return nil, ErrNotFound
	}
	n := NewNode(title)
	p.Children = append(p.Children, n)
	p.Collapsed = false
	return n, nil
}

// AddSibling inserts a new node right after the node with the ID. For the
// root, which has no siblings, it adds a child instead.
func (d *Document) AddSibling(id, title string) (*Node, error) {
	n, p := d.Find(id)
	if n == nil {
		return nil, ErrNotFound
	}
	if p == nil {
		return d.AddChild(id, title)
	}
	i := slices.Index(p.Children, n)
	s := NewNode(title)
	p.Children = slices.Insert(p.Children, i+1, s)
	return s, nil
}

// AddSiblingBefore inserts a new node right before the node with the ID;
// for the root, it adds a child, as AddSibling does.
func (d *Document) AddSiblingBefore(id, title string) (*Node, error) {
	n, p := d.Find(id)
	if n == nil {
		return nil, ErrNotFound
	}
	if p == nil {
		return d.AddChild(id, title)
	}
	s := NewNode(title)
	p.Children = slices.Insert(p.Children, slices.Index(p.Children, n), s)
	return s, nil
}

// Delete removes the node with the ID and everything below it. It returns
// the ID of the node to select next: the next sibling, else the previous
// one, else the parent.
func (d *Document) Delete(id string) (next string, err error) {
	n, p := d.Find(id)
	if n == nil {
		return "", ErrNotFound
	}
	if p == nil {
		if i := slices.Index(d.Floating, n); i >= 0 { // a floating topic
			d.Floating = slices.Delete(d.Floating, i, i+1)
			d.dropRelations(n)
			return d.Root.ID, nil
		}
		return "", ErrRoot
	}
	d.dropRelations(n)
	i := slices.Index(p.Children, n)
	p.Children = slices.Delete(p.Children, i, i+1)
	switch {
	case i < len(p.Children):
		return p.Children[i].ID, nil
	case i > 0:
		return p.Children[i-1].ID, nil
	}
	return p.ID, nil
}

// Move puts the node with the ID under the new parent at index (clamped;
// -1 appends).
func (d *Document) Move(id, newParentID string, index int) error {
	n, p := d.Find(id)
	if n == nil {
		return ErrNotFound
	}
	floating := p == nil && d.IsFloating(id)
	if p == nil && !floating {
		return ErrRoot
	}
	if n.Contains(newParentID) {
		return ErrCycle
	}
	np, _ := d.Find(newParentID)
	if np == nil {
		return ErrNotFound
	}
	i := -1
	if floating { // a floating topic joins the tree
		d.Floating = slices.DeleteFunc(d.Floating, func(f *Node) bool { return f == n })
		n.Pos = nil
	} else {
		i = slices.Index(p.Children, n)
		p.Children = slices.Delete(p.Children, i, i+1)
	}
	if np == p && i >= 0 && index > i {
		index-- // the node itself no longer comes before the index
	}
	if index < 0 || index > len(np.Children) {
		index = len(np.Children)
	}
	np.Children = slices.Insert(np.Children, index, n)
	np.Collapsed = false
	return nil
}

// Shift moves the node with the ID by delta places among its siblings.
func (d *Document) Shift(id string, delta int) error {
	n, p := d.Find(id)
	if n == nil {
		return ErrNotFound
	}
	if p == nil {
		return ErrRoot
	}
	i := slices.Index(p.Children, n)
	j := min(max(i+delta, 0), len(p.Children)-1)
	if i == j {
		return nil
	}
	p.Children = slices.Delete(p.Children, i, i+1)
	p.Children = slices.Insert(p.Children, j, n)
	return nil
}

// Indent makes the node with the ID the last child of its previous
// sibling.
func (d *Document) Indent(id string) error {
	n, p := d.Find(id)
	if n == nil {
		return ErrNotFound
	}
	if p == nil {
		return ErrRoot
	}
	i := slices.Index(p.Children, n)
	if i == 0 {
		return nil
	}
	return d.Move(id, p.Children[i-1].ID, -1)
}

// Outdent makes the node with the ID the sibling after its parent.
func (d *Document) Outdent(id string) error {
	n, p := d.Find(id)
	if n == nil {
		return ErrNotFound
	}
	if p == nil {
		return ErrRoot
	}
	pp, gp := d.Find(p.ID)
	if gp == nil {
		return nil // already a child of the root
	}
	return d.Move(id, gp.ID, slices.Index(gp.Children, pp)+1)
}

// TaskNode is a node that is a task, with the path of titles above it and,
// from File.Tasks, its canvas.
type TaskNode struct {
	Node    *Node
	Parents []string
	Sheet   *Document
}

// Tasks returns every node that is a task, in map order.
func (d *Document) Tasks() []TaskNode {
	var out []TaskNode
	var walk func(n *Node, parents []string)
	walk = func(n *Node, parents []string) {
		if n.Task != nil {
			out = append(out, TaskNode{Node: n, Parents: slices.Clone(parents)})
		}
		parents = append(parents, n.Title)
		for _, c := range n.Children {
			walk(c, parents)
		}
	}
	if d.Root != nil {
		walk(d.Root, nil)
	}
	return out
}

// dropRelations removes the relations of a node and of all below it.
func (d *Document) dropRelations(n *Node) {
	if len(d.Relations) == 0 {
		return
	}
	gone := map[string]bool{}
	var walk func(m *Node)
	walk = func(m *Node) {
		gone[m.ID] = true
		for _, c := range m.Children {
			walk(c)
		}
	}
	walk(n)
	d.Relations = slices.DeleteFunc(d.Relations, func(r Relation) bool { return gone[r.From] || gone[r.To] })
	if len(d.Relations) == 0 {
		d.Relations = nil
	}
}

// Float takes a node, with all below it, out of the tree as a floating
// topic at a place.
func (d *Document) Float(id string, at Point) error {
	n, p := d.Find(id)
	if n == nil {
		return ErrNotFound
	}
	if p == nil {
		return ErrRoot
	}
	i := slices.Index(p.Children, n)
	p.Children = slices.Delete(p.Children, i, i+1)
	n.Pos = &at
	d.Floating = append(d.Floating, n)
	return nil
}

// AddFloating adds a floating topic at a place.
func (d *Document) AddFloating(title string, at Point) *Node {
	n := NewNode(title)
	n.Pos = &at
	d.Floating = append(d.Floating, n)
	return n
}
