package formats

import (
	"regexp"
	"strings"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// A document in Markdown is more than an outline: paragraphs under its
// headings, tables, fenced code. Read as an outline, each of their lines
// was a topic, a row of a table with its bars and a paragraph whole: a map
// no one reads. Here its headings and lists are the map's topics, what is
// written under a heading is that topic's note, and a table's rows are
// topics each, named by their first cells with every cell in the note.

var (
	htmlComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	headingLine = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)
	listLine    = regexp.MustCompile(`^\s*([-*+•]|\d{1,3}[.)])\s+\S`)
	tableRule   = regexp.MustCompile(`^\s*\|?\s*:?-{2,}:?\s*(\|\s*:?-{2,}:?\s*)*\|?\s*$`)
	mdLink      = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)
	boldLead    = regexp.MustCompile(`^\*\*(.+?)\*\*`)
)

// The longest a topic made of a line of a document is; what is longer is
// named by how it begins, and kept whole in the note.
const (
	maxTopicRunes = 46
	leadRunes     = 40
)

// isDocument tells whether a Markdown text is a document rather than an
// outline: it has headings, and under them prose or a table.
func isDocument(text string) bool {
	heading, prose := false, false
	fenced := false
	for _, raw := range strings.Split(htmlComment.ReplaceAllString(text, ""), "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(line, "```"):
			fenced = !fenced
			prose = true
		case fenced, line == "", strings.HasPrefix(line, "---"):
		case headingLine.MatchString(line):
			heading = true
		case listLine.MatchString(raw):
		case raw != line && !strings.HasPrefix(line, "|"): // goes on from a list's item
		case strings.HasPrefix(line, ">"): // a quote: an outline's note, as a map is written out (model.Markdown)
		default:
			prose = true
		}
	}
	return heading && prose
}

// parseDocument makes a map of a document in Markdown.
func parseDocument(text, title string) *model.File {
	lines := strings.Split(strings.ReplaceAll(htmlComment.ReplaceAllString(text, ""), "\r\n", "\n"), "\n")
	before := model.NewNode("") // holds what is written before any heading
	type level struct {
		depth int
		node  *model.Node
	}
	var roots []*model.Node
	var stack []level
	cur := before
	var para []string
	note := func(n *model.Node, s string) {
		if s = strings.TrimSpace(s); s == "" {
			return
		}
		if n.Note != "" {
			n.Note += "\n\n"
		}
		n.Note += s
	}
	flush := func() {
		note(cur, strings.Join(para, "\n"))
		para = nil
	}
	for i := 0; i < len(lines); i++ {
		raw := strings.ReplaceAll(lines[i], "\t", "    ")
		line := strings.TrimSpace(raw)
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "```"): // code, kept as written in the note
			flush()
			block := []string{raw}
			for i++; i < len(lines); i++ {
				block = append(block, lines[i])
				if strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
					break
				}
			}
			note(cur, strings.Join(block, "\n"))
		case headingLine.MatchString(line):
			flush()
			m := headingLine.FindStringSubmatch(line)
			n := model.NewNode(plain(m[2]))
			for len(stack) > 0 && stack[len(stack)-1].depth >= len(m[1]) {
				stack = stack[:len(stack)-1]
			}
			if len(stack) == 0 {
				roots = append(roots, n)
			} else {
				p := stack[len(stack)-1].node
				p.Children = append(p.Children, n)
			}
			stack = append(stack, level{len(m[1]), n})
			cur = n
		case strings.HasPrefix(line, "|") && i+1 < len(lines) && tableRule.MatchString(lines[i+1]):
			flush()
			head := cells(line)
			for i += 2; i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "|"); i++ {
				if n := rowTopic(head, cells(lines[i])); n != nil {
					cur.Children = append(cur.Children, n)
				}
			}
			i--
		case listLine.MatchString(raw):
			flush()
			var block []string
			for ; i < len(lines); i++ {
				l := strings.ReplaceAll(lines[i], "\t", "    ")
				t := strings.TrimSpace(l)
				switch {
				case listLine.MatchString(l):
					block = append(block, l)
				case strings.HasPrefix(t, ">") && l != t && len(block) > 0:
					block = append(block, l) // a quote under an item: its note (model.ParseOutline)
				case t != "" && l != t && len(block) > 0 && !strings.HasPrefix(t, "|") && !strings.HasPrefix(t, "```"):
					block[len(block)-1] += " " + t // an item's line going on
				default:
					goto done
				}
			}
		done:
			i--
			for _, n := range model.ParseOutline(strings.Join(block, "\n")) {
				shorten(n)
				cur.Children = append(cur.Children, n)
			}
		default:
			para = append(para, line)
		}
	}
	flush()
	f := model.NewFile(title)
	root := f.Sheets[0].Root
	if len(roots) == 1 {
		f.Sheets[0].Root, root = roots[0], roots[0]
	} else {
		root.Children = roots
	}
	if before.Note != "" { // what leads the document is of the whole
		if root.Note != "" {
			before.Note += "\n\n" + root.Note
		}
		root.Note = before.Note
	}
	root.Children = append(before.Children, root.Children...)
	return f
}

// cells are a table row's cells, as written.
func cells(row string) []string {
	row = strings.TrimSpace(row)
	row = strings.TrimSuffix(strings.TrimPrefix(row, "|"), "|")
	var out []string
	var b strings.Builder
	for i := 0; i < len(row); i++ {
		switch {
		case row[i] == '\\' && i+1 < len(row) && row[i+1] == '|':
			b.WriteByte('|')
			i++
		case row[i] == '|':
			out = append(out, strings.TrimSpace(b.String()))
			b.Reset()
		default:
			b.WriteByte(row[i])
		}
	}
	return append(out, strings.TrimSpace(b.String()))
}

// rowTopic is a table's row as a topic: named by its first cell, with how
// the second begins when the first is a number or a short name, and with
// every cell under its column's name in the note.
func rowTopic(head, row []string) *model.Node {
	if strings.TrimSpace(strings.Join(row, "")) == "" {
		return nil
	}
	title := plain(row[0])
	switch {
	case len([]rune(title)) <= 14 && len(row) > 1 && plain(row[1]) != "":
		title = strings.TrimSpace(title + " " + lead(row[1], 26))
	default:
		title = lead(row[0], leadRunes)
	}
	n := model.NewNode(title)
	var lines []string
	for i, c := range row {
		if c == "" {
			continue
		}
		name := ""
		if i < len(head) {
			name = plain(head[i])
		}
		if name != "" {
			c = "**" + name + "**：" + c
		}
		lines = append(lines, c)
	}
	n.Note = strings.Join(lines, "\n\n")
	return n
}

// shorten names a topic too long to read on a map by how it begins, and
// keeps its words whole in its note; and so those under it.
func shorten(n *model.Node) {
	if len([]rune(n.Title)) > maxTopicRunes {
		whole := n.Title
		n.Title = lead(whole, leadRunes)
		if n.Note != "" {
			whole += "\n\n" + n.Note
		}
		n.Note = whole
	} else {
		n.Title = plain(n.Title)
	}
	for _, c := range n.Children {
		shorten(c)
	}
}

// plain is a line of Markdown without its marks: the words of a link, of
// what is bold or code.
func plain(s string) string {
	s = mdLink.ReplaceAllString(s, "$1")
	s = strings.NewReplacer("**", "", "__", "", "`", "", "<br>", " ", "<br/>", " ").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

// lead is how a text begins, in at most n runes: what it has in bold at
// its head, else up to its first stop, else cut.
func lead(s string, n int) string {
	s = strings.TrimSpace(s)
	if m := boldLead.FindStringSubmatch(s); m != nil {
		if b := plain(m[1]); b != "" && len([]rune(b)) <= n {
			return b
		}
	}
	r := []rune(plain(s))
	if len(r) <= n {
		return string(r)
	}
	for i, c := range r[:n] {
		if i >= 4 && strings.ContainsRune("：。；，（:;(", c) {
			return string(r[:i])
		}
	}
	return string(r[:n-1]) + "…"
}
