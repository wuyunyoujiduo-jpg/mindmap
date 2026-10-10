package main

import (
	"regexp"
	"strings"

	"github.com/egoist/mygo/ui"
)

// The AI answers in Markdown, which its chat shows formatted: headings,
// lists, quotes, code, tables and rules, with bold, italic, struck,
// code and linked text within lines.

// mdBlock is a block of a Markdown text.
type mdBlock struct {
	kind  string // "p", "h", "li", "ol", "quote", "code", "table", "hr"
	level int    // a heading's level; a list item's indent
	mark  string // an ordered item's number
	text  string
	rows  [][]string // a table's, the header first
}

var (
	mdHeading  = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	mdBullet   = regexp.MustCompile(`^(\s*)[-*+]\s+(.*)$`)
	mdOrdered  = regexp.MustCompile(`^(\s*)(\d+)[.)]\s+(.*)$`)
	mdRule     = regexp.MustCompile(`^\s*(?:(?:-\s*){3,}|(?:\*\s*){3,}|(?:_\s*){3,})$`)
	mdTableSep = regexp.MustCompile(`^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$`)
)

// parseMarkdown splits a Markdown text into blocks.
func parseMarkdown(src string) []mdBlock {
	var blocks []mdBlock
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	para := func(text string) {
		if n := len(blocks); n > 0 && blocks[n-1].kind == "p" {
			blocks[n-1].text += "\n" + text
			return
		}
		blocks = append(blocks, mdBlock{kind: "p", text: text})
	}
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			blocks = append(blocks, mdBlock{kind: "gap"})
		case strings.HasPrefix(trimmed, "```"):
			var code []string
			for i++; i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```"); i++ {
				code = append(code, lines[i])
			}
			blocks = append(blocks, mdBlock{kind: "code", text: strings.Join(code, "\n")})
		case mdRule.MatchString(line):
			blocks = append(blocks, mdBlock{kind: "hr"})
		case mdHeading.MatchString(trimmed):
			m := mdHeading.FindStringSubmatch(trimmed)
			blocks = append(blocks, mdBlock{kind: "h", level: len(m[1]), text: m[2]})
		case mdBullet.MatchString(line):
			m := mdBullet.FindStringSubmatch(line)
			blocks = append(blocks, mdBlock{kind: "li", level: len(m[1]) / 2, text: m[2]})
		case mdOrdered.MatchString(line):
			m := mdOrdered.FindStringSubmatch(line)
			blocks = append(blocks, mdBlock{kind: "ol", level: len(m[1]) / 2, mark: m[2] + ".", text: m[3]})
		case strings.HasPrefix(trimmed, ">"):
			text := strings.TrimSpace(strings.TrimPrefix(trimmed, ">"))
			if n := len(blocks); n > 0 && blocks[n-1].kind == "quote" {
				blocks[n-1].text += "\n" + text
			} else {
				blocks = append(blocks, mdBlock{kind: "quote", text: text})
			}
		case strings.HasPrefix(trimmed, "|") && i+1 < len(lines) && mdTableSep.MatchString(lines[i+1]):
			b := mdBlock{kind: "table", rows: [][]string{tableCells(trimmed)}}
			for i += 2; i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "|"); i++ {
				b.rows = append(b.rows, tableCells(strings.TrimSpace(lines[i])))
			}
			i--
			blocks = append(blocks, b)
		default:
			para(trimmed)
		}
	}
	// Blank lines only separate blocks.
	var out []mdBlock
	for _, b := range blocks {
		if b.kind != "gap" {
			out = append(out, b)
		}
	}
	return out
}

func tableCells(line string) []string {
	line = strings.TrimSuffix(strings.TrimPrefix(line, "|"), "|")
	cells := strings.Split(line, "|")
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	return cells
}

// mdSpanStyle is how a run of a line is styled.
type mdSpanStyle struct {
	bold, italic, strike, code, link bool
}

var mdInline = regexp.MustCompile("`[^`]+`|\\*\\*[^*]+\\*\\*|__[^_]+__|~~[^~]+~~|\\[[^\\]]+\\]\\([^)]+\\)|\\*[^*\\s][^*]*\\*")

// inlineSpans styles the runs of a line: **bold**, *italic*, ~~struck~~,
// `code` and [links](url), which show their text.
func inlineSpans(text string, size float32, color, accent, codeBg ui.Color, base mdSpanStyle) []ui.Span {
	var spans []ui.Span
	add := func(s string, st mdSpanStyle) {
		if s == "" {
			return
		}
		sp := ui.Span{Text: s, Size: size, Color: color, Italic: st.italic, Strikethrough: st.strike}
		if st.bold {
			sp.Weight = 700
		}
		if st.code {
			sp.Font, sp.Size, sp.Background = "monospace", size-1, codeBg
		}
		if st.link {
			sp.Color, sp.Underline = accent, true
		}
		spans = append(spans, sp)
	}
	last := 0
	for _, m := range mdInline.FindAllStringIndex(text, -1) {
		add(text[last:m[0]], base)
		tok := text[m[0]:m[1]]
		st := base
		switch {
		case strings.HasPrefix(tok, "`"):
			st.code = true
			add(tok[1:len(tok)-1], st)
		case strings.HasPrefix(tok, "**"), strings.HasPrefix(tok, "__"):
			st.bold = true
			spans = append(spans, inlineSpans(tok[2:len(tok)-2], size, color, accent, codeBg, st)...)
		case strings.HasPrefix(tok, "~~"):
			st.strike = true
			spans = append(spans, inlineSpans(tok[2:len(tok)-2], size, color, accent, codeBg, st)...)
		case strings.HasPrefix(tok, "["):
			st.link = true
			add(tok[1:strings.Index(tok, "]")], st)
		default:
			st.italic = true
			spans = append(spans, inlineSpans(tok[1:len(tok)-1], size, color, accent, codeBg, st)...)
		}
		last = m[1]
	}
	add(text[last:], base)
	return spans
}

// markdownView shows a Markdown text at the font size.
func markdownView(c *ui.Context, src string, size float32) {
	t := c.Theme()
	codeBg := t.Text.Alpha(0.08)
	line := func(c *ui.Context, text string, size float32, color ui.Color, bold bool) ui.Element {
		spans := inlineSpans(text, size, color, t.Accent, codeBg, mdSpanStyle{bold: bold})
		if len(spans) == 0 {
			spans = []ui.Span{{Text: " ", Size: size}}
		}
		return ui.RichText(c, spans...).Wrap().Selectable()
	}
	ui.Column(c).Gap(6).FillWidth().Children(func() {
		for i, b := range parseMarkdown(src) {
			c := c.Key(i)
			switch b.kind {
			case "h":
				hs := size + max(0, float32(4-b.level))
				line(c, b.text, hs, t.Text, true).Margin(4, 0, 0)
			case "li", "ol":
				mark := "•"
				if b.kind == "ol" {
					mark = b.mark
				}
				ui.Row(c).Gap(6).Padding(0, 0, 0, float32(b.level)*14).Children(func() {
					ui.Text(c, mark).FontSize(size).TextColor(t.TextMuted).Shrink(0)
					line(c, b.text, size, t.Text, false).Grow(1)
				})
			case "quote":
				ui.Box(c).Padding(2, 0, 2, 10).BorderWidth(0, 0, 0, 3).BorderColor(t.Border).FillWidth().Children(func() {
					line(c, b.text, size, t.TextMuted, false)
				})
			case "code":
				ui.Box(c).Padding(8, 10).Radius(6).Background(codeBg).FillWidth().Children(func() {
					ui.Text(c, b.text).Font("monospace").FontSize(size - 1).Wrap().Selectable()
				})
			case "hr":
				ui.Divider(c)
			case "table":
				ui.Column(c).Radius(6).Border(1, t.Border).FillWidth().Children(func() {
					for r, row := range b.rows {
						rr := ui.Row(c.Key(r)).Padding(4, 8).Gap(8)
						if r == 0 {
							rr.Background(codeBg)
						}
						rr.Children(func() {
							for k, cell := range row {
								line(c.Key(k), cell, size-1, t.Text, r == 0).Grow(1).Basis(0)
							}
						})
					}
				})
			default:
				line(c, b.text, size, t.Text, false)
			}
		}
	})
}
