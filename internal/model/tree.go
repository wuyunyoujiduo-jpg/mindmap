package model

import (
	"strings"
)

// Copy returns a deep copy of the node and all below it, with new IDs, to
// paste elsewhere.
// Clone copies a node and all below it, keeping their IDs, as a node moved
// to another map is the same node: links to it, and tasks waiting on it,
// still find it.
func (n *Node) Clone() *Node { return n.clone() }

func (n *Node) Copy() *Node {
	c := n.clone()
	c.renew()
	return c
}

// Progress counts the tasks below a node, not the node itself: how many
// there are and how many are done.
func (n *Node) Progress() (done, total int) {
	for _, c := range n.Children {
		if c.Task != nil {
			total++
			if c.Task.Status == StatusDone {
				done++
			}
		}
		d, t := c.Progress()
		done, total = done+d, total+t
	}
	return done, total
}

// ExpandTo shows the nodes down to the depth below the root (1: its
// children) and folds the nodes there; 0 opens all.
func (d *Document) ExpandTo(depth int) {
	d.Walk(func(n, _ *Node, k int) bool {
		n.Collapsed = depth > 0 && k >= depth && len(n.Children) > 0
		return true
	})
}

// Ancestors reports whether the node with the ID a is above, or is, the
// node with the ID b.
func (d *Document) Ancestors(a, b string) bool {
	for _, n := range d.Path(b) {
		if n.ID == a {
			return true
		}
	}
	return false
}

// ParseOutline makes nodes of text: a line each, nested by indentation,
// as an outline or a Markdown list is written. Bullets, checkboxes and
// numbers before a line's text are taken off; a checked box makes a done
// task and an empty one a task to do. Headings ("# ") nest by their level.
func ParseOutline(text string) []*Node {
	type level struct {
		indent int
		node   *Node
	}
	var roots []*Node
	var stack []level
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := strings.TrimRight(strings.ReplaceAll(raw, "\t", "    "), " ")
		trimmed := strings.TrimLeft(line, " ")
		if trimmed == "" || strings.HasPrefix(trimmed, "---") {
			continue
		}
		indent := len(line) - len(trimmed)
		if h := len(trimmed) - len(strings.TrimLeft(trimmed, "#")); h > 0 && h <= 6 && strings.HasPrefix(trimmed[h:], " ") {
			indent = (h - 7) * 100 // headings above any list, h1 above h2
			trimmed = strings.TrimSpace(trimmed[h:])
		}
		var task *Task
		for _, p := range []string{"- ", "* ", "+ ", "• "} {
			if strings.HasPrefix(trimmed, p) {
				trimmed = strings.TrimSpace(trimmed[len(p):])
				break
			}
		}
		if i := strings.Index(trimmed, ". "); i > 0 && i <= 3 && strings.Trim(trimmed[:i], "0123456789") == "" {
			trimmed = strings.TrimSpace(trimmed[i+2:])
		}
		switch {
		case strings.HasPrefix(trimmed, "[ ] "):
			task, trimmed = &Task{Status: StatusTodo}, trimmed[4:]
		case strings.HasPrefix(trimmed, "[x] "), strings.HasPrefix(trimmed, "[X] "):
			task, trimmed = &Task{Status: StatusDone}, trimmed[4:]
		}
		title, links := trailingLinks(strings.TrimSpace(trimmed))
		n := NewNode(title)
		n.Task = task
		n.Links = links
		for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			roots = append(roots, n)
		} else {
			p := stack[len(stack)-1].node
			p.Children = append(p.Children, n)
		}
		stack = append(stack, level{indent, n})
	}
	return roots
}

// trailingLinks takes the links written at a line's end, after a space,
// as "<clip:id>", "<mindmap:map#id>" or "<https://…>", off its title, as
// the AI writes them in outlines; "List<String>" stays a title.
func trailingLinks(line string) (string, []string) {
	var links []string
	for strings.HasSuffix(line, ">") {
		i := strings.LastIndex(line, " <")
		if i < 0 {
			break
		}
		l := line[i+2 : len(line)-1]
		if strings.ContainsAny(l, " <>") || !(strings.HasPrefix(l, "clip:") || strings.HasPrefix(l, "mindmap:") ||
			strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://")) {
			break
		}
		links = append([]string{l}, links...)
		line = strings.TrimSpace(line[:i])
	}
	return line, links
}

// WalkShown calls fn for the nodes shown: the root, the floating topics
// and all below them, but not what is under a folded node.
func (d *Document) WalkShown(fn func(n, parent *Node, depth int)) {
	var walk func(n, parent *Node, depth int)
	walk = func(n, parent *Node, depth int) {
		fn(n, parent, depth)
		if n.Collapsed {
			return
		}
		for _, c := range n.Children {
			walk(c, n, depth+1)
		}
	}
	if d.Root != nil {
		walk(d.Root, nil, 0)
	}
	for _, f := range d.Floating {
		walk(f, nil, 0)
	}
}
