package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// Escapes show the marks as they are; in code they stay.
func TestMarkdownEscapes(t *testing.T) {
	text := func(s string) string {
		var b strings.Builder
		for _, sp := range inlineSpans(s, 13, ui.Color{}, ui.Color{}, ui.Color{}, mdSpanStyle{}) {
			b.WriteString(sp.Text)
		}
		return b.String()
	}
	for in, want := range map[string]string{
		`ERGORA\_CONFIG\_IMPORT=true`: "ERGORA_CONFIG_IMPORT=true",
		`a \*not italic\* b`:          "a *not italic* b",
		"`a\\_b`":                     `a\_b`,
		`C:\path`:                     `C:\path`,
	} {
		if got := text(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

// Entities show as a page shows them, and spaces written as entities at
// a line's start make the line a list item's second line when it follows
// the item straight on: the user's note showed "&#32;&#32;都要有出处". An
// escaped dash is a dash, and a first line indented after a list is a
// paragraph of its own.
func TestMarkdownEntities(t *testing.T) {
	src := "\\- 先说读了哪些文件；每个数字、名称、流程\n\n&#32;&#32;都要有出处，拿不准的不写进视频。\n\n- a list\n&#32;&#32;goes on\n\n&nbsp;&nbsp;首行缩进的一段\n\nalone &amp; &lt;b&gt; `&amp;`"
	var got []string
	for _, b := range parseMarkdown(src) {
		var s strings.Builder
		for _, sp := range inlineSpans(b.text, 13, ui.Color{}, ui.Color{}, ui.Color{}, mdSpanStyle{}) {
			s.WriteString(sp.Text)
		}
		got = append(got, b.kind+":"+s.String())
	}
	want := []string{
		"p:- 先说读了哪些文件；每个数字、名称、流程",
		"p:都要有出处，拿不准的不写进视频。",
		"li:a list goes on",
		"p:首行缩进的一段",
		"p:alone & <b> &amp;",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got\n%q\nwant\n%q", got, want)
	}
}
