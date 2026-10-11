// Package formats converts maps to and from other formats: OPML outlines,
// iCalendar events and PDF documents of pictures.
package formats

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"strings"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// OPML: an outline of "outline" elements, with "text" and, as many
// outliners write, the note in "_note".

type opmlDoc struct {
	XMLName xml.Name       `xml:"opml"`
	Version string         `xml:"version,attr"`
	Title   string         `xml:"head>title"`
	Body    []*opmlOutline `xml:"body>outline"`
}

type opmlOutline struct {
	Text     string         `xml:"text,attr"`
	Note     string         `xml:"_note,attr,omitempty"`
	Complete string         `xml:"_complete,attr,omitempty"`
	Children []*opmlOutline `xml:"outline"`
}

// OPML writes a file's canvases as an OPML outline, a top-level outline
// each.
func OPML(f *model.File, title string) ([]byte, error) {
	doc := opmlDoc{Version: "2.0", Title: title}
	var conv func(n *model.Node) *opmlOutline
	conv = func(n *model.Node) *opmlOutline {
		o := &opmlOutline{Text: n.Title, Note: n.Note}
		if n.Task != nil && n.Task.Status == model.StatusDone {
			o.Complete = "true"
		}
		for _, c := range n.Children {
			o.Children = append(o.Children, conv(c))
		}
		return o
	}
	for _, d := range f.Sheets {
		doc.Body = append(doc.Body, conv(d.Root))
	}
	out, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), out...), nil
}

// ParseOPML reads an OPML outline: one top-level outline becomes the root,
// several go under a root named title.
func ParseOPML(data []byte, title string) (*model.File, error) {
	var doc opmlDoc
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("无法读取 OPML：%w", err)
	}
	if len(doc.Body) == 0 {
		return nil, errors.New("OPML 里没有内容")
	}
	var conv func(o *opmlOutline) *model.Node
	conv = func(o *opmlOutline) *model.Node {
		n := model.NewNode(strings.TrimSpace(o.Text))
		n.Note = strings.TrimSpace(o.Note)
		if o.Complete == "true" {
			n.Task = &model.Task{Status: model.StatusDone}
		}
		for _, c := range o.Children {
			n.Children = append(n.Children, conv(c))
		}
		return n
	}
	if doc.Title != "" {
		title = doc.Title
	}
	f := model.NewFile(title)
	if len(doc.Body) == 1 {
		f.Sheets[0].Root = conv(doc.Body[0])
	} else {
		for _, o := range doc.Body {
			f.Sheets[0].Root.Children = append(f.Sheets[0].Root.Children, conv(o))
		}
	}
	return f, nil
}

// ParseMarkdown reads a Markdown outline: a first heading of the top level
// becomes the root, under which the rest goes nested by headings and list
// indentation; without one, a root named title holds it all.
func ParseMarkdown(text, title string) *model.File {
	nodes := model.ParseOutline(text)
	f := model.NewFile(title)
	if len(nodes) == 1 {
		f.Sheets[0].Root = nodes[0]
	} else {
		f.Sheets[0].Root.Children = nodes
	}
	return f
}

// ICS writes the tasks with dates of a file as all-day events of an
// iCalendar: from the start day to the due day, or on the one given. Done
// tasks are marked ✓.
func ICS(f *model.File, calendar string, now time.Time) []byte {
	var b strings.Builder
	line := func(s string) { b.WriteString(fold(s) + "\r\n") }
	line("BEGIN:VCALENDAR")
	line("VERSION:2.0")
	line("PRODID:-//MyMind//CN")
	line("CALSCALE:GREGORIAN")
	line("X-WR-CALNAME:" + escapeICS(calendar))
	stamp := now.UTC().Format("20060102T150405Z")
	for _, tn := range f.Tasks() {
		t := tn.Node.Task
		start, end := t.Start, t.Due
		if start == nil {
			start = end
		}
		if end == nil {
			end = start
		}
		if start == nil {
			continue
		}
		title := tn.Node.Title
		if t.Status == model.StatusDone {
			title = "✓ " + title
		}
		desc := strings.Join(tn.Parents, " › ")
		if tn.Node.Note != "" {
			desc += "\n\n" + tn.Node.Note
		}
		line("BEGIN:VEVENT")
		line("UID:" + tn.Node.ID + "@mindmap")
		line("DTSTAMP:" + stamp)
		line("DTSTART;VALUE=DATE:" + start.Format("20060102"))
		line("DTEND;VALUE=DATE:" + end.AddDate(0, 0, 1).Format("20060102"))
		line("SUMMARY:" + escapeICS(title))
		if desc != "" {
			line("DESCRIPTION:" + escapeICS(desc))
		}
		line("STATUS:" + map[bool]string{true: "CONFIRMED", false: "TENTATIVE"}[t.Status == model.StatusDone || t.Status == model.StatusDoing])
		line("END:VEVENT")
	}
	line("END:VCALENDAR")
	return []byte(b.String())
}

func escapeICS(s string) string {
	r := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\n", `\n`)
	return r.Replace(s)
}

// fold breaks a line of iCalendar at 75 bytes, between characters.
func fold(s string) string {
	if len(s) <= 75 {
		return s
	}
	var b strings.Builder
	n := 0
	for _, r := range s {
		size := len(string(r))
		if n+size > 75 {
			b.WriteString("\r\n ")
			n = 1
		}
		b.WriteRune(r)
		n += size
	}
	return b.String()
}

// PDF writes pictures as the pages of a PDF, a page each, at its size in
// points of the width given (the height following the picture).
func PDF(w io.Writer, pages []image.Image, pageWidth float64) error {
	var buf bytes.Buffer
	offsets := []int{}
	obj := func(body string) {
		offsets = append(offsets, buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", len(offsets), body)
	}
	buf.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	// 1: catalog, 2: pages; then per page: page, content, image.
	n := len(pages)
	kids := make([]string, n)
	for i := range pages {
		kids[i] = fmt.Sprintf("%d 0 R", 3+i*3)
	}
	obj("<< /Type /Catalog /Pages 2 0 R >>")
	obj(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), n))
	for i, img := range pages {
		bounds := img.Bounds()
		pw := pageWidth
		ph := pageWidth * float64(bounds.Dy()) / float64(bounds.Dx())
		var jp bytes.Buffer
		if err := jpeg.Encode(&jp, img, &jpeg.Options{Quality: 92}); err != nil {
			return err
		}
		page, content, im := 3+i*3, 4+i*3, 5+i*3
		obj(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.2f %.2f] /Resources << /XObject << /Im%d %d 0 R >> >> /Contents %d 0 R >>",
			pw, ph, i, im, content))
		_ = page
		draw := fmt.Sprintf("q %.2f 0 0 %.2f 0 0 cm /Im%d Do Q", pw, ph, i)
		obj(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(draw), draw))
		offsets = append(offsets, buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>\nstream\n",
			len(offsets), bounds.Dx(), bounds.Dy(), jp.Len())
		buf.Write(jp.Bytes())
		buf.WriteString("\nendstream\nendobj\n")
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xref)
	_, err := w.Write(buf.Bytes())
	return err
}
