package model

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
