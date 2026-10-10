package main

import (
	"strings"
	"unicode"

	"github.com/egoist/mygo/ui"
)

// Highlighting code in notes and replies, for the languages a project
// most likely uses: comments, strings, numbers and keywords in colors.

// keywords of the common languages, together: a word that is one in any
// shows as one.
var keywords = func() map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(`
		break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var
		async await class constructor delete do export extends finally function in instanceof let new of static super this throw try typeof void while with yield catch
		def elif except from global is lambda nonlocal not or and pass raise as assert del True False None self
		fn impl let mut pub use mod crate enum trait where match loop move ref unsafe dyn
		public private protected final abstract implements package throws boolean int long float double char byte short
		true false null nil undefined
		SELECT FROM WHERE INSERT UPDATE DELETE CREATE TABLE INTO VALUES JOIN ON GROUP BY ORDER LIMIT AND OR NOT NULL`) {
		m[w] = true
	}
	return m
}()

// hashComments are the languages commented with "#".
var hashComments = map[string]bool{"py": true, "python": true, "sh": true, "bash": true, "shell": true, "zsh": true,
	"yaml": true, "yml": true, "toml": true, "ruby": true, "rb": true, "dockerfile": true, "make": true, "makefile": true, "r": true}

// highlight colors code as spans of the text size.
func highlight(code, lang string, size float32, t *ui.Theme) []ui.Span {
	comment, str, num, kw := ui.Hex("#6a9955"), ui.Hex("#c2410c"), ui.Hex("#0e7490"), ui.Hex("#7c3aed")
	if t.Dark {
		comment, str, num, kw = ui.Hex("#6a9955"), ui.Hex("#ce9178"), ui.Hex("#b5cea8"), ui.Hex("#c586c0")
	}
	hash := hashComments[lang]
	var spans []ui.Span
	add := func(s string, col ui.Color) {
		if s == "" {
			return
		}
		if n := len(spans); n > 0 && spans[n-1].Color == col {
			spans[n-1].Text += s
			return
		}
		spans = append(spans, ui.Span{Text: s, Size: size, Color: col})
	}
	r := []rune(code)
	for i := 0; i < len(r); {
		ch := r[i]
		switch {
		case ch == '/' && i+1 < len(r) && r[i+1] == '/' || ch == '#' && hash || ch == '-' && i+1 < len(r) && r[i+1] == '-' && lang == "sql":
			j := i
			for j < len(r) && r[j] != '\n' {
				j++
			}
			add(string(r[i:j]), comment)
			i = j
		case ch == '/' && i+1 < len(r) && r[i+1] == '*':
			j := i + 2
			for j+1 < len(r) && !(r[j] == '*' && r[j+1] == '/') {
				j++
			}
			j = min(j+2, len(r))
			add(string(r[i:j]), comment)
			i = j
		case ch == '"' || ch == '\'' || ch == '`':
			j := i + 1
			for j < len(r) && r[j] != ch && (ch == '`' || r[j] != '\n') {
				if r[j] == '\\' {
					j++
				}
				j++
			}
			j = min(j+1, len(r))
			add(string(r[i:j]), str)
			i = j
		case unicode.IsDigit(ch) && (i == 0 || !isWord(r[i-1])):
			j := i
			for j < len(r) && (isWord(r[j]) || r[j] == '.') {
				j++
			}
			add(string(r[i:j]), num)
			i = j
		case isWord(ch):
			j := i
			for j < len(r) && isWord(r[j]) {
				j++
			}
			w := string(r[i:j])
			if keywords[w] {
				add(w, kw)
			} else {
				add(w, t.Text)
			}
			i = j
		default:
			add(string(ch), t.Text)
			i++
		}
	}
	if len(spans) == 0 {
		spans = []ui.Span{{Text: " ", Size: size}}
	}
	return spans
}

func isWord(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }
