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
