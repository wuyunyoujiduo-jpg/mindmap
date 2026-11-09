package model

import (
	"strings"
)

// Markdown writes a canvas as a Markdown outline: the root as a heading,
// its note below it, and every node as a list item, nested by depth.
// Tasks are checkboxes, with their status, priority and dates after the
// title when they say more; tags follow as #tag; a note goes under its
// item. Folded nodes are written too.
func Markdown(d *Document) string {
	var b strings.Builder
	b.WriteString("# " + oneLine(d.Root.Title) + "\n")
	if d.Root.Note != "" {
		b.WriteString("\n" + d.Root.Note + "\n")
	}
	if len(d.Root.Children) > 0 {
		b.WriteString("\n")
	}
	writeItemsOf(&b, d.Root.Children, d.Root.SummaryTopic())
	return b.String()
}

// MarkdownNodes writes nodes, with all below them, as a Markdown list.
func MarkdownNodes(nodes []*Node) string {
	var b strings.Builder
	writeItems(&b, nodes)
	return b.String()
}

// SummaryPrefix starts a summary's topic in outlines written out: it comes
// after the children it sums up, at their depth.
const SummaryPrefix = "〔概要〕"

func writeItems(b *strings.Builder, nodes []*Node) {
	writeItemsOf(b, nodes, nil)
}

// writeItemsOf writes nodes, and after them the topic of the summary of
// their parent, when it has one.
func writeItemsOf(b *strings.Builder, nodes []*Node, summary *Node) {
	var walk func(n *Node, depth int, prefix string)
	walk = func(n *Node, depth int, prefix string) {
		indent := strings.Repeat("  ", depth)
		b.WriteString(indent + "- " + prefix + itemLine(n) + "\n")
		if n.Note != "" {
			for _, line := range strings.Split(strings.TrimRight(n.Note, "\n"), "\n") {
				if strings.TrimSpace(line) == "" {
					b.WriteString("\n")
					continue
				}
				b.WriteString(indent + "  " + line + "\n")
			}
		}
		for _, c := range n.Children {
			walk(c, depth+1, "")
		}
		if t := n.SummaryTopic(); t != nil && len(n.Children) > 0 {
			walk(t, depth+1, SummaryPrefix)
		}
	}
	for _, c := range nodes {
		walk(c, 0, "")
	}
	if summary != nil && len(nodes) > 0 {
		walk(summary, 0, SummaryPrefix)
	}
}

// MarkdownFile writes every canvas of a file, one after another.
func MarkdownFile(f *File) string {
	parts := make([]string, len(f.Sheets))
	for i, d := range f.Sheets {
		parts[i] = Markdown(d)
	}
	return strings.Join(parts, "\n---\n\n")
}

func itemLine(n *Node) string {
	var b strings.Builder
	if n.Task != nil {
		if n.Task.Status == StatusDone {
			b.WriteString("[x] ")
		} else {
			b.WriteString("[ ] ")
		}
	}
	title := oneLine(n.Title)
	if title == "" {
		title = "（无标题）"
	}
	b.WriteString(title)
	if n.Task != nil {
		var info []string
		if n.Task.Status == StatusIdea || n.Task.Status == StatusDoing {
			info = append(info, n.Task.Status.Label())
		}
		if n.Task.Priority != PriorityNone {
			info = append(info, "优先级 "+n.Task.Priority.Label())
		}
		if n.Task.Start != nil {
			info = append(info, "开始 "+n.Task.Start.Format("2006-01-02"))
		}
		if n.Task.Due != nil {
			info = append(info, "截止 "+n.Task.Due.Format("2006-01-02"))
		}
		if len(info) > 0 {
			b.WriteString("（" + strings.Join(info, " · ") + "）")
		}
	}
	for _, tag := range n.Tags {
		b.WriteString(" #" + strings.ReplaceAll(tag, " ", "_"))
	}
	return b.String()
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
