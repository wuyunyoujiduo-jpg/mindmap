package main

import (
	"os"
	"path/filepath"
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
		text := a.notePictures(richtext.ParseMarkdown(keepLines(n.Note)))
		d = &noteDoc{id: n.ID, src: n.Note, doc: richtext.NewDocument(text)}
		d.doc.SetSelection(text.Len(), text.Len()) // typing goes on at the end
		a.noteDocs[place] = d
	}
	ed := richtext.Editor(c.Key(place+"-rich"), d.doc, richtext.EditorOptions{
		Label: "备注内容", Placeholder: placeholder, Font: ui.Font{Size: size},
	}).Clip().Height(noteHeight(d.doc.Text(), size))
	// A picture file dropped on the note goes into it.
	for _, f := range ed.DroppedFiles() {
		if isImageFile(f) {
			if data, err := os.ReadFile(f); err == nil {
				a.insertNotePicture(place, n, data, strings.ToLower(filepath.Ext(f)))
			}
		}
	}
	if ed.Changed() {
		a.saveNoteDoc(d, n)
	}
	return ed
}

// saveNoteDoc keeps the note the document holds, as Markdown.
func (a *app) saveNoteDoc(d *noteDoc, n *model.Node) {
	md := strings.TrimRight(d.doc.Text().Markdown(), "\n")
	d.src = md
	a.mutate("note:"+n.ID, func() { n.Note = md })
}

// insertNotePicture puts a picture into the note edited at place, at its
// caret, its file into the map's assets.
func (a *app) insertNotePicture(place string, n *model.Node, data []byte, ext string) {
	d := a.noteDocs[place]
	if d == nil || d.id != n.ID {
		return
	}
	name, b, err := a.saveAsset(data, ext)
	if err != nil {
		a.tell(err.Error())
		return
	}
	d.doc.InsertAttachment(notePicture(name, b))
	a.saveNoteDoc(d, n)
}

// notePictureButton inserts into the note edited at place the picture on
// the clipboard, or one picked.
func (a *app) notePictureButton(c *ui.Context, place string, n *model.Node) {
	if !richNote(n.Note) {
		return
	}
	if ui.Button(c, "插入图片").FontSize(11).Padding(1, 8).Tooltip("插入剪贴板里的图片；没有就选择图片文件。也可以把图片文件拖进备注").Clicked() {
		if data := a.plat.ClipboardImage(); len(data) > 0 {
			a.insertNotePicture(place, n, data, ".png")
			return
		}
		id := n.ID
		a.plat.PickImage(func(path string) {
			node := a.file.Node(id)
			if path == "" || node == nil {
				return
			}
			if data, err := os.ReadFile(path); err == nil {
				a.insertNotePicture(place, node, data, strings.ToLower(filepath.Ext(path)))
			}
		})
	}
}

// A note's picture is written as Markdown's image, of the file in the
// map's assets folder: ![图片](asset:<file>).
const assetScheme = "asset:"

// notePictureMark is a note's picture in its Markdown.
var notePictureMark = regexp.MustCompile(`!\[[^\]]*\]\(` + assetScheme + `[^)]*\)`)

// notePictureWidth is how wide a note shows a picture at most.
const notePictureWidth = 320

func notePicture(name string, b *ui.Bitmap) *richtext.Attachment {
	w, h := b.Size()
	k := min(1, float32(notePictureWidth)/float32(w))
	return &richtext.Attachment{Image: b, Name: "图片", URL: assetScheme + name, Width: float32(w) * k, Height: float32(h) * k}
}

// notePictures gives the pictures of a note read from Markdown their
// images, from the map's assets.
func (a *app) notePictures(text richtext.Text) richtext.Text {
	for _, run := range text.Runs(0, text.Len()) {
		att := run.Attributes.Attachment
		if att == nil || !strings.HasPrefix(att.URL, assetScheme) {
			continue
		}
		if b := a.bitmap(&model.Image{File: strings.TrimPrefix(att.URL, assetScheme)}); b != nil {
			*att = *notePicture(strings.TrimPrefix(att.URL, assetScheme), b)
		}
	}
	return text
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

// noteHeight is about as high as the note shows, to show it whole up to a
// screenful (beyond, it scrolls): a line a paragraph, wrapped ones too, and
// its pictures.
func noteHeight(text richtext.Text, size float32) float32 {
	h := size * 1.6 // the padding
	for _, p := range text.Paragraphs() {
		lines := 1
		for _, r := range p.Runs {
			lines += len([]rune(r.Text)) / 28
			if att := r.Attributes.Attachment; att != nil && att.Height > 0 {
				h += att.Height
			}
		}
		h += float32(lines) * size * 1.7
	}
	return min(max(h, 96), 520)
}
