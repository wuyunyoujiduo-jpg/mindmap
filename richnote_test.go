package main

import (
	"testing"

	"github.com/egoist/mygo/ui/richtext"
)

func TestKeepLines(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"第一行\n第二行", "第一行\n\n第二行"},
		{"- 一\n- 二", "- 一\n- 二"},
		{"说明\n- 一\n- 二", "说明\n- 一\n- 二"},
		{"```\na\nb\n```", "```\na\nb\n```"},
		{"a\n\nb", "a\n\nb"},
	} {
		if got := keepLines(c.in); got != c.want {
			t.Errorf("keepLines(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// Lines of plain text stay lines through the editor.
	if md := richtext.ParseMarkdown(keepLines("第一行\n第二行")).Markdown(); md != "第一行\n\n第二行\n" && md != "第一行\n\n第二行" {
		t.Fatalf("markdown %q", md)
	}
	if richNote("| a | b |\n|---|---|\n| 1 | 2 |") {
		t.Fatal("a table edits as rich text")
	}
}
