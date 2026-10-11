package library

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/ledongthuc/pdf"
)

// Text is read out of documents, so that they are found by what they say
// and the AI can read them: PDF, Word, PowerPoint and Excel files, and
// plain text.

// MaxFileText is how much of a document's text a clip keeps.
const MaxFileText = 200000

// ExtractText reads the text of a document, "" when it has none to read.
func ExtractText(path string, data []byte) string {
	var text string
	switch ext := strings.ToLower(filepath.Ext(path)); {
	case ext == ".pdf":
		text = pdfText(data)
	case ext == ".docx":
		text = officeText(data, func(n string) bool { return n == "word/document.xml" }, "p")
	case ext == ".pptx":
		text = officeText(data, func(n string) bool {
			return strings.HasPrefix(n, "ppt/slides/slide") && strings.HasSuffix(n, ".xml")
		}, "p")
	case ext == ".xlsx":
		text = officeText(data, func(n string) bool { return n == "xl/sharedStrings.xml" }, "si")
	case isTextFile(path):
		if len(data) <= 4<<20 {
			text, _ = Decode(data, "")
		}
	}
	text = strings.TrimSpace(text)
	if r := []rune(text); len(r) > MaxFileText {
		text = string(r[:MaxFileText]) + "…"
	}
	return text
}

// pdfText reads the text of a PDF, "" when it cannot, as for scans.
func pdfText(data []byte) (text string) {
	defer func() {
		if recover() != nil { // the reader panics on some broken files
			text = ""
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return ""
	}
	var b strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		rows, err := p.GetTextByRow()
		if err != nil {
			continue
		}
		for _, row := range rows {
			for _, w := range row.Content {
				b.WriteString(w.S)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
		if b.Len() > MaxFileText*4 {
			break
		}
	}
	return reBlankLines.ReplaceAllString(b.String(), "\n\n")
}

var reBlankLines = regexp.MustCompile(`\n\s*\n+`)

// officeText reads the text of the XML parts of an Office file that part
// picks, a paragraph a line, the paragraphs being the elements named para.
func officeText(data []byte, part func(name string) bool, para string) string {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return ""
	}
	var files []*zip.File
	for _, f := range zr.File {
		if part(f.Name) {
			files = append(files, f)
		}
	}
	// Slides in their order: slide2 before slide10.
	sort.SliceStable(files, func(i, j int) bool {
		a, b := files[i].Name, files[j].Name
		if len(a) != len(b) {
			return len(a) < len(b)
		}
		return a < b
	})
	var out []string
	for _, f := range files {
		rc, err := f.Open()
		if err != nil {
			continue
		}
		dec := xml.NewDecoder(io.LimitReader(rc, 64<<20))
		var line strings.Builder
		for {
			tok, err := dec.Token()
			if err != nil {
				break
			}
			switch t := tok.(type) {
			case xml.CharData:
				line.Write(t)
			case xml.StartElement:
				if t.Name.Local == "tab" {
					line.WriteString("\t")
				}
			case xml.EndElement:
				if t.Name.Local == para {
					if s := strings.TrimSpace(line.String()); s != "" {
						out = append(out, s)
					}
					line.Reset()
				}
			}
		}
		rc.Close()
		out = append(out, "")
	}
	return strings.TrimSpace(strings.Join(slices.Compact(out), "\n"))
}
