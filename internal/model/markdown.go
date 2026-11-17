package model

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Markdown writes a canvas as a Markdown outline: the root as a heading
// and every node as a list item, nested by depth. Tasks are checkboxes,
// with their status, priority and dates after the title when they say
// more; tags follow as #tag, and what the node links to as <link>; a note
// goes under its line as a quote, a line a line; a summary's topic comes
// after the children it sums up, and the floating topics last, each marked
// with its place. Folded nodes are written too.
//
// ParseCanvas reads all of it back: written otherwise, a note came back a
// topic a line, the tags and the task's marks as words of the title, and
// the links and the floating topics not at all.
func Markdown(d *Document) string {
	var b strings.Builder
	b.WriteString("# " + itemLine(d.Root) + "\n")
	if d.Root.Note != "" {
		b.WriteString("\n")
		writeNote(&b, "", d.Root.Note)
	}
	if len(d.Root.Children) > 0 {
		b.WriteString("\n")
	}
	writeItemsOf(&b, d.Root.Children, d.Root.SummaryTopic())
	if len(d.Floating) > 0 {
		b.WriteString("\n")
	}
	for _, f := range d.Floating {
		mark := FloatingPrefix
		if f.Pos != nil {
			mark += " " + coord(f.Pos.X) + "," + coord(f.Pos.Y)
		}
		writeItem(&b, f, 0, mark+"〕")
	}
	return b.String()
}

func coord(v float32) string { return strconv.FormatFloat(float64(v), 'f', -1, 32) }

// MarkdownNodes writes nodes, with all below them, as a Markdown list.
func MarkdownNodes(nodes []*Node) string {
	var b strings.Builder
	writeItems(&b, nodes)
	return b.String()
}

// SummaryPrefix starts a summary's topic in outlines written out: it comes
// after the children it sums up, at their depth. FloatingPrefix starts a
// floating topic, with its place after it when it has one, as
// 〔自由主题 320,-120〕.
const (
	SummaryPrefix  = "〔概要〕"
	FloatingPrefix = "〔自由主题"
)

// untitled stands for a title that is empty, which a list item cannot be.
const untitled = "（无标题）"

func writeItems(b *strings.Builder, nodes []*Node) {
	writeItemsOf(b, nodes, nil)
}

// writeItemsOf writes nodes, and after them the topic of the summary of
// their parent, when it has one.
func writeItemsOf(b *strings.Builder, nodes []*Node, summary *Node) {
	for _, c := range nodes {
		writeItem(b, c, 0, "")
	}
	if summary != nil && len(nodes) > 0 {
		writeItem(b, summary, 0, SummaryPrefix)
	}
}

// writeItem writes a node as a list item at a depth, after a prefix, and
// all below it.
func writeItem(b *strings.Builder, n *Node, depth int, prefix string) {
	indent := strings.Repeat("  ", depth)
	b.WriteString(indent + "- " + prefix + itemLine(n) + "\n")
	if n.Note != "" {
		writeNote(b, indent+"  ", n.Note)
	}
	for _, c := range n.Children {
		writeItem(b, c, depth+1, "")
	}
	if t := n.SummaryTopic(); t != nil && len(n.Children) > 0 {
		writeItem(b, t, depth+1, SummaryPrefix)
	}
}

// writeNote writes a note as a quote, a line a line: a list item's own
// lines, as it was written before, read back as topics.
func writeNote(b *strings.Builder, indent, note string) {
	for _, line := range strings.Split(strings.TrimRight(note, "\n"), "\n") {
		b.WriteString(strings.TrimRight(indent+"> "+strings.TrimRight(line, "\r"), " ") + "\n")
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
	title := escapeTitle(oneLine(n.Title))
	if title == "" {
		title = untitled
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
	for _, l := range n.Links {
		if l = strings.TrimSpace(l); l != "" && !strings.ContainsAny(l, "<>\n") {
			b.WriteString(" <" + writtenLink(l) + ">")
		}
	}
	return b.String()
}

// The links a line ends with are told from words in angle brackets by how
// they begin (trailingLinks): a file of the project, which begins with
// nothing of its own, is written after "file:".
var linkStarts = []string{"clip:", "mindmap:", "skill:", "http://", "https://"}

const fileStart = "file:"

func writtenLink(l string) string {
	for _, s := range linkStarts {
		if strings.HasPrefix(l, s) {
			return strings.ReplaceAll(l, " ", "%20")
		}
	}
	return fileStart + strings.ReplaceAll(l, " ", "%20")
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// trailingTags takes the tags written at a line's end, each after a space
// as #tag, off its title: #42 too, as another program writes a tag. A
// title's own "Issue #42" is written \#42 (escapeTitle), and comes back a
// title: a number was taken for no tag at all once, and a file's #42 that
// was one stayed in its title.
func trailingTags(line string) (string, []string) {
	var tags []string
	for {
		i := strings.LastIndex(line, " #")
		if i <= 0 {
			break
		}
		tag := line[i+2:]
		if !tagWord(tag) {
			break
		}
		tags = append([]string{tag}, tags...)
		line = strings.TrimRight(line[:i], " ")
	}
	return line, tags
}

// tagWord tells whether what follows a # at a line's end is read as a
// tag: a word, of figures alone too.
func tagWord(tag string) bool {
	return tag != "" && !strings.ContainsAny(tag, " \t#")
}

// escapeTitle writes a title so that none of its own words is read back
// as a mark of the outline: a #word at its end, which would be a tag, and
// 〔概要〕 or 〔自由主题 at its head, a summary's topic or a floating one's,
// each get a \ before them (and one more before those that have one, so
// that every title comes back as it was). unescapeTitle takes it off.
func escapeTitle(title string) string {
	if marked(strings.TrimLeft(title, `\`)) {
		title = `\` + title
	}
	if i := strings.LastIndex(title, " "); i >= 0 && tagMarked(title[i+1:]) {
		title = title[:i+1] + `\` + title[i+1:]
	}
	return title
}

func unescapeTitle(title string) string {
	if strings.HasPrefix(title, `\`) && marked(strings.TrimLeft(title, `\`)) {
		title = title[1:]
	}
	if i := strings.LastIndex(title, " "); i >= 0 && strings.HasPrefix(title[i+1:], `\`) && tagMarked(title[i+1:]) {
		title = title[:i+1] + title[i+2:]
	}
	return title
}

// marked tells whether a title begins as a summary's topic or a floating
// one is written.
func marked(title string) bool {
	return strings.HasPrefix(title, SummaryPrefix) || strings.HasPrefix(title, FloatingPrefix)
}

// tagMarked tells whether a title's last word is a tag as written, after
// any \ before it.
func tagMarked(word string) bool {
	word = strings.TrimLeft(word, `\`)
	return strings.HasPrefix(word, "#") && tagWord(word[1:])
}

// taskMarks reads what a task's line says of it in brackets at its end, as
// itemLine writes it (进行中 · 优先级 高 · 截止 2026-10-12), into the task,
// and returns the title without it. Brackets that say anything else are
// words of the title.
func taskMarks(title string, task *Task) string {
	const open, shut = "（", "）"
	i := strings.LastIndex(title, open)
	if i <= 0 || !strings.HasSuffix(title, shut) {
		return title
	}
	t := *task
	for _, part := range strings.Split(title[i+len(open):len(title)-len(shut)], " · ") {
		word, rest, _ := strings.Cut(part, " ")
		switch {
		case rest == "" && (part == StatusIdea.Label() || part == StatusDoing.Label()) && t.Status != StatusDone:
			if part == StatusIdea.Label() {
				t.Status = StatusIdea
			} else {
				t.Status = StatusDoing
			}
		case word == "优先级":
			p, ok := map[string]Priority{PriorityLow.Label(): PriorityLow, PriorityMedium.Label(): PriorityMedium, PriorityHigh.Label(): PriorityHigh}[rest]
			if !ok {
				return title
			}
			t.Priority = p
		case word == "开始" || word == "截止":
			day, err := time.ParseInLocation("2006-01-02", rest, time.Local)
			if err != nil {
				return title
			}
			if word == "开始" {
				t.Start = &day
			} else {
				t.Due = &day
			}
		default:
			return title
		}
	}
	*task = t
	return strings.TrimSpace(title[:i])
}

// floatingMark reads the mark of a floating topic at a line's head, with
// its place when it says one.
func floatingMark(line string) (rest string, at *Point, ok bool) {
	if !strings.HasPrefix(line, FloatingPrefix) {
		return line, nil, false
	}
	end := strings.Index(line, "〕")
	if end < 0 {
		return line, nil, false
	}
	if in := strings.TrimSpace(line[len(FloatingPrefix):end]); in != "" {
		var x, y float32
		if _, err := fmt.Sscanf(in, "%g,%g", &x, &y); err != nil {
			return line, nil, false
		}
		at = &Point{X: x, Y: y}
	}
	return strings.TrimSpace(line[end+len("〕"):]), at, true
}
