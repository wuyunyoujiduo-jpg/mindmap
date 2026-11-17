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

// ParseOutline makes nodes of text: a line each, nested by indentation,
// as an outline or a Markdown list is written. Bullets, checkboxes and
// numbers before a line's text are taken off; a checked box makes a done
// task and an empty one a task to do, and what its line says of it in
// brackets at its end, as Markdown writes it, is the task's (taskMarks).
// Headings ("# ") nest by their level. A quoted line ("> ", or ">" alone)
// is of the note of the line above it, and a quote with no line above it
// is lines as any, without the mark; a line that begins 〔概要〕 is the
// topic of the summary of the node it is under, after that node's
// children. What Markdown wrote with a \ before it, for a title's own
// words not to be read so, is those words again (unescapeTitle).
func ParseOutline(text string) []*Node {
	roots, _ := parseOutline(text, false)
	return roots
}

// ParseCanvas reads a canvas as Markdown writes one, and gives its nodes
// and its floating topics: as ParseOutline, and the tags at a line's end
// (#tag) and the mark of a floating topic at its head too. Those two are
// read of a map written out only, not of whatever is pasted or an AI
// answers: there a #word at a line's end is words.
func ParseCanvas(text string) (nodes, floating []*Node) {
	return parseOutline(text, true)
}

func parseOutline(text string, written bool) (roots, floating []*Node) {
	type level struct {
		indent int
		node   *Node
	}
	var stack []level
	noted := map[*Node]bool{}
	loose := false // in a quote that no line stands above
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := strings.TrimRight(strings.ReplaceAll(raw, "\t", "    "), " ")
		trimmed := strings.TrimLeft(line, " ")
		if trimmed == "" || strings.HasPrefix(trimmed, "---") {
			continue
		}
		// A quote is "> " and what follows, or ">" alone, as writeNote
		// writes a note: every line that began with > was one, and
		// "会议纪要" over ">= 5 人" came in as a note "= 5 人".
		quote := trimmed == ">" || strings.HasPrefix(trimmed, "> ")
		if quote && len(stack) > 0 && !loose { // of the note of the line above
			n := stack[len(stack)-1].node
			q := strings.TrimPrefix(strings.TrimLeft(raw, " \t")[1:], " ")
			if noted[n] {
				n.Note += "\n"
			}
			n.Note += q
			noted[n] = true
			continue
		}
		// A quote with no line above it is no one's note: its lines are
		// lines, without the mark, where the first was a topic "> …" and
		// the rest its note.
		if loose = quote; quote {
			line = strings.Repeat(" ", len(line)-len(trimmed)) + strings.TrimSpace(trimmed[1:])
			if trimmed = strings.TrimLeft(line, " "); trimmed == "" {
				continue
			}
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
		summary, floats := false, false
		var at *Point
		if strings.HasPrefix(trimmed, SummaryPrefix) {
			summary, trimmed = true, strings.TrimSpace(trimmed[len(SummaryPrefix):])
		} else if written {
			trimmed, at, floats = floatingMark(trimmed)
		}
		switch {
		case strings.HasPrefix(trimmed, "[ ] "):
			task, trimmed = &Task{Status: StatusTodo}, trimmed[4:]
		case strings.HasPrefix(trimmed, "[x] "), strings.HasPrefix(trimmed, "[X] "):
			task, trimmed = &Task{Status: StatusDone}, trimmed[4:]
		}
		title, links := trailingLinks(strings.TrimSpace(trimmed))
		var tags []string
		if written {
			title, tags = trailingTags(title)
		}
		if task != nil {
			title = taskMarks(title, task)
		}
		if written && title == untitled {
			title = ""
		}
		title = unescapeTitle(title)
		n := NewNode(title)
		n.Task, n.Links, n.Tags = task, links, tags
		for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}
		switch {
		case floats:
			n.Pos = at
			floating = append(floating, n)
		case len(stack) == 0:
			roots = append(roots, n)
		default:
			p := stack[len(stack)-1].node
			if summary && p.Summary == nil && len(p.Children) > 0 {
				p.Summary = &Mark{Topic: n}
			} else {
				p.Children = append(p.Children, n)
			}
		}
		stack = append(stack, level{indent, n})
	}
	return roots, floating
}

// trailingLinks takes the links written at a line's end, after a space,
// as "<clip:id>", "<mindmap:map#id>", "<skill:name>", "<https://…>" or
// "<file:path>", off its title, as the AI writes them in outlines and
// Markdown writes a node's; "List<String>" stays a title.
func trailingLinks(line string) (string, []string) {
	var links []string
	for strings.HasSuffix(line, ">") {
		i := strings.LastIndex(line, " <")
		if i < 0 {
			break
		}
		l := line[i+2 : len(line)-1]
		known := strings.HasPrefix(l, fileStart)
		for _, s := range linkStarts {
			known = known || strings.HasPrefix(l, s)
		}
		if strings.ContainsAny(l, " <>") || !known {
			break
		}
		// A space of a map's name or a file's path, written as %20.
		if strings.HasPrefix(l, fileStart) {
			l = strings.ReplaceAll(l[len(fileStart):], "%20", " ")
		} else if strings.HasPrefix(l, "mindmap:") {
			l = strings.ReplaceAll(l, "%20", " ")
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
		if t := n.SummaryTopic(); t != nil && len(n.Children) > 0 {
			walk(t, n, depth+1)
		}
	}
	if d.Root != nil {
		walk(d.Root, nil, 0)
	}
	for _, f := range d.Floating {
		walk(f, nil, 0)
	}
}
