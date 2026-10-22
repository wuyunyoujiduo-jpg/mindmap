package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// Lines of the outline select whole, by Shift and the arrows or by
// dragging across them, and copy, cut and paste nested as they are.
func TestOutlineBlock(t *testing.T) {
	a, tt := newTestApp(t)
	plat := a.plat.(*fakePlatform)
	a.mode = viewOutline
	arch := find(t, a, "架构")
	front := find(t, a, "前端")
	a.outlineFocus = arch.ID
	tt.Frame()
	tt.Key(ui.Shift, ui.KeyDown)
	tt.Frame()
	if !a.outlineBlock || a.outlineFocus != outlineBlockFocus || !a.isSelected(arch.ID) || !a.isSelected(front.ID) {
		t.Fatalf("block %v focus %q", a.outlineBlock, a.outlineFocus)
	}
	snapshot(t, tt, "outline-block")
	// ⌘C as the app has it: the Edit menu's command.
	plat.copied = ""
	tt.Command("copy")
	tt.Frame()
	// 架构 holds 前端: it copies once, with all that is under it.
	if !strings.HasPrefix(plat.copied, "- 架构") || !strings.Contains(plat.copied, "\n  - 前端") || strings.Count(plat.copied, "前端") != 1 {
		t.Fatalf("copied %q", plat.copied)
	}
	// Escape goes back to editing the last line.
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.outlineBlock || a.outlineFocus != front.ID {
		t.Fatalf("after Escape: %v %q", a.outlineBlock, a.outlineFocus)
	}

	// Dragging from one line onto another selects them, and those between.
	rows := outlineRows(a.doc)
	first, last := rows[0].node, rows[3].node
	b1, b2 := a.outlineRowAt[first.ID], a.outlineRowAt[last.ID]
	tt.Press(b1.X+b1.W-40, b1.Y+b1.H/2)
	tt.Move(b1.X+b1.W-40, b2.Y+b2.H/2)
	tt.Release(b1.X+b1.W-40, b2.Y+b2.H/2)
	tt.Frame()
	for _, r := range rows[:4] {
		if !a.isSelected(r.node.ID) {
			t.Fatalf("%q not selected by the drag", r.node.Title)
		}
	}
	if a.isSelected(rows[4].node.ID) || a.outlineFocus != outlineBlockFocus {
		t.Fatalf("drag selected past, or focus %q", a.outlineFocus)
	}
	// The keys: copied after a drag too, then deleted.
	plat.copied = ""
	tt.Command("copy")
	tt.Frame()
	if !strings.Contains(plat.copied, rows[0].node.Title) || !strings.Contains(plat.copied, rows[3].node.Title) {
		t.Fatalf("copied after the drag %q", plat.copied)
	}
	n := len(rows)
	tt.Key(0, ui.KeyBackspace)
	tt.Frame()
	if a.outlineBlock || len(outlineRows(a.doc)) >= n-3 {
		t.Fatalf("rows %d of %d after deleting", len(outlineRows(a.doc)), n)
	}

	// Lines pasted come in after the line, nested.
	rows = outlineRows(a.doc)
	at := rows[0].node
	a.outlineFocus = at.ID
	tt.Frame()
	text := "- 甲\n  - 甲一\n- 乙\n"
	plat.copied = text
	tt.SetClipboard(text)
	tt.Key(ui.Cmd, ui.KeyV)
	tt.Frame()
	_, parent := a.doc.Find(at.ID)
	i := 0
	for i < len(parent.Children) && parent.Children[i] != at {
		i++
	}
	if i+2 >= len(parent.Children) || parent.Children[i+1].Title != "甲" || len(parent.Children[i+1].Children) != 1 ||
		parent.Children[i+2].Title != "乙" || at.Title != rows[0].node.Title {
		t.Fatalf("pasted: %q", strings.Join(tt.Texts(), "|"))
	}
}
