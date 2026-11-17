package model

import "slices"

// Summaries: a brace beside a node's children and, beyond it, a topic of
// its own, which takes children as any node does.

// SummaryTopic is the topic of the node's summary, or nil.
func (n *Node) SummaryTopic() *Node {
	if n == nil || n.Summary == nil {
		return nil
	}
	return n.Summary.Topic
}

// AddSummary gives the node a summary of its children, with its topic,
// which it returns; a node with one keeps it.
func (n *Node) AddSummary(title string) *Node {
	if t := n.SummaryTopic(); t != nil {
		return t
	}
	n.Summary = &Mark{Topic: NewNode(title)}
	return n.Summary.Topic
}

// IsSummaryTopic reports whether the node with the ID is a summary's
// topic.
func (d *Document) IsSummaryTopic(id string) bool {
	n, p := d.Find(id)
	return isSummaryTopic(n, p)
}

func isSummaryTopic(n, p *Node) bool { return n != nil && p != nil && p.SummaryTopic() == n }

// settleSummary makes the topic of the node's summary its child when it
// has no other: a summary sums up the children, and with none of them it
// was drawn nowhere, on no canvas, outline nor export, though it and what
// is under it were still there, its tasks in the lists with nowhere to go
// to. It reports whether it did.
func (n *Node) settleSummary() bool {
	t := n.SummaryTopic()
	if t == nil || len(n.Children) > 0 {
		return false
	}
	n.Summary, n.Children = nil, []*Node{t}
	return true
}

// SettleSummaries does so for every node of the file left with a summary
// and no child, however it lost them, and returns the topics made
// children.
func (f *File) SettleSummaries() []*Node {
	var out []*Node
	var walk func(n *Node)
	walk = func(n *Node) {
		if t := n.SummaryTopic(); n.settleSummary() {
			out = append(out, t)
		}
		for _, c := range n.Children {
			walk(c)
		}
		if t := n.SummaryTopic(); t != nil {
			walk(t)
		}
	}
	for _, d := range f.Sheets {
		if d.Root != nil {
			walk(d.Root)
		}
		for _, fl := range d.Floating {
			walk(fl)
		}
	}
	return out
}

// Summarize makes the child with the ID the topic of the node's summary
// again, as it was before the node lost its other children
// (settleSummary): when the node has no summary, and another child to sum
// up.
func (n *Node) Summarize(id string) bool {
	i := slices.IndexFunc(n.Children, func(c *Node) bool { return c.ID == id })
	if i < 0 || n.Summary != nil || len(n.Children) < 2 {
		return false
	}
	t := n.Children[i]
	n.Children = slices.Delete(n.Children, i, i+1)
	n.Summary = &Mark{Topic: t}
	return true
}

// summaryTopicOfLabel makes a summary saved with a label only, before
// summaries had topics, the topic of that label.
func (n *Node) summaryTopicOfLabel() {
	if n.Summary == nil || n.Summary.Topic != nil {
		return
	}
	title := n.Summary.Label
	if title == "" {
		title = "概要"
	}
	n.Summary = &Mark{Topic: NewNode(title)}
}
