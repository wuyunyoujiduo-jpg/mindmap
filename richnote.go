package main

import (
	"regexp"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// A note is Markdown, edited as it shows: "# " makes a heading, "- " a
// list, **bold** bold, as typed (MyGo's rich text editor, with its
// Markdown shortcuts), and the note kept is the Markdown the editor
// writes.

// noteDoc is the document a place (the panel, the popover) edits a note
// in: of the node, and of the note as it was read or last written, so
// that a note changed elsewhere (undo, the AI) is read again.
type noteDoc struct {
	id, src string
	doc     *richtext.Document
}

// tableLine is a Markdown table's divider: the editor has no tables (it
// would keep their rows as text apart by tabs), so a note with one is
// edited as text.
var tableLine = regexp.MustCompile(`(?m)^\s*\|?\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)+\|?\s*$`)

// richNote tells whether the note edits as rich text.
func richNote(note string) bool { return !tableLine.MatchString(note) }

// noteEditor edits n's note at place, as rich text, or as Markdown text
// where the note has what the editor does not show.
func (a *app) noteEditor(c *ui.Context, place string, n *model.Node, size float32, placeholder string) ui.Element {
	if !richNote(n.Note) {
		note := n.Note
		in := ui.TextArea(c.Key(place+"-text"), &note).Lines(4, 16).Label("备注内容").Placeholder(placeholder)
		if in.Changed() {
			a.mutate("note:"+n.ID, func() { n.Note = note })
		}
		return in
	}
	if a.noteDocs == nil {
		a.noteDocs = map[string]*noteDoc{}
	}
	d := a.noteDocs[place]
	if d == nil || d.id != n.ID || d.src != n.Note {
		text := richtext.ParseMarkdown(keepLines(n.Note))
		d = &noteDoc{id: n.ID, src: n.Note, doc: richtext.NewDocument(text)}
		d.doc.SetSelection(text.Len(), text.Len()) // typing goes on at the end
		a.noteDocs[place] = d
	}
	ed := richtext.Editor(c.Key(place+"-rich"), d.doc, richtext.EditorOptions{
		Label: "备注内容", Placeholder: placeholder, Font: ui.Font{Size: size},
	})
	if ed.Changed() {
		md := strings.TrimRight(d.doc.Text().Markdown(), "\n")
		d.src = md
		id := n.ID
		a.mutate("note:"+id, func() { n.Note = md })
	}
	return ed
}

// blockStart is a line Markdown starts a block of its own with.
var blockStart = regexp.MustCompile(`^\s*([-*+]\s|\d+[.)]\s|#{1,6}\s|>|` + "```" + `|\|)`)

// keepLines keeps a note's lines apart, as written in plain text: in
// Markdown, lines of text one after another are a paragraph, joined.
func keepLines(note string) string {
	lines := strings.Split(note, "\n")
	var b strings.Builder
	fenced := false
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			fenced = !fenced
		}
		b.WriteString(l)
		if i == len(lines)-1 {
			break
		}
		b.WriteByte('\n')
		next := lines[i+1]
		if !fenced && strings.TrimSpace(l) != "" && strings.TrimSpace(next) != "" &&
			!blockStart.MatchString(l) && !blockStart.MatchString(next) {
			b.WriteByte('\n') // a paragraph of its own
		}
	}
	return b.String()
}
