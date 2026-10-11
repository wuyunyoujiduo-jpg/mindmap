package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func newEditor() *editor { return &editor{bounds: screen} }

// drag 模拟按下、拖动、松开。
func drag(e *editor, from, to pt) result {
	r := e.down(from, 1)
	e.move(pt{(from.X + to.X) / 2, (from.Y + to.Y) / 2})
	e.up(to)
	return r
}

func TestCreateSelection(t *testing.T) {
	e := newEditor()
	drag(e, pt{300.4, 200}, pt{100, 50.6})
	if !e.hasSel || e.sel != (ui.Rect{X: 100, Y: 51, W: 200, H: 149}) {
		t.Fatalf("选区 %v %v", e.hasSel, e.sel)
	}
}

func TestTinyDragIsNotASelection(t *testing.T) {
	e := newEditor()
	drag(e, pt{100, 100}, pt{102, 101})
	if e.hasSel {
		t.Error("误点不应形成选区")
	}
}

func TestMoveAndResizeSelection(t *testing.T) {
	e := newEditor()
	drag(e, pt{100, 100}, pt{300, 200})
	drag(e, pt{200, 150}, pt{250, 170}) // 中间拖动
	if e.sel != (ui.Rect{X: 150, Y: 120, W: 200, H: 100}) {
		t.Fatalf("移动后 %v", e.sel)
	}
	drag(e, pt{350, 220}, pt{400, 300}) // 右下角
	if e.sel != (ui.Rect{X: 150, Y: 120, W: 250, H: 180}) {
		t.Fatalf("缩放后 %v", e.sel)
	}
}

func TestClickOutsideRestartsOnlyWithoutMarks(t *testing.T) {
	e := newEditor()
	drag(e, pt{100, 100}, pt{300, 200})
	drag(e, pt{500, 500}, pt{600, 600})
	if e.sel != (ui.Rect{X: 500, Y: 500, W: 100, H: 100}) {
		t.Fatalf("应重新框选 %v", e.sel)
	}
	e.setTool(toolRect)
	drag(e, pt{510, 510}, pt{550, 550})
	e.setTool(toolRect) // 取消工具
	drag(e, pt{10, 10}, pt{50, 50})
	if e.sel != (ui.Rect{X: 500, Y: 500, W: 100, H: 100}) {
		t.Errorf("有标注时不应丢掉选区 %v", e.sel)
	}
}

func TestDrawMarksClampedToSelection(t *testing.T) {
	e := newEditor()
	drag(e, pt{100, 100}, pt{300, 200})
	e.setTool(toolArrow)
	drag(e, pt{150, 150}, pt{900, 900})
	if len(e.marks) != 1 || e.marks[0].last() != (pt{300, 200}) {
		t.Fatalf("箭头 %+v", e.marks)
	}
	drag(e, pt{20, 20}, pt{40, 40}) // 选区外不画
	if len(e.marks) != 1 {
		t.Errorf("选区外画上了 %d", len(e.marks))
	}
	e.down(pt{150, 150}, 1) // 误点
	e.up(pt{151, 150})
	if len(e.marks) != 1 {
		t.Errorf("误点留下了标注 %d", len(e.marks))
	}
}

func TestPenKeepsAllPoints(t *testing.T) {
	e := newEditor()
	drag(e, pt{100, 100}, pt{300, 200})
	e.setTool(toolPen)
	e.down(pt{110, 110}, 1)
	e.move(pt{120, 115})
	if got := len(e.visibleMarks()); got != 1 {
		t.Errorf("画的过程中应显示 %d", got)
	}
	e.move(pt{130, 130})
	e.up(pt{140, 120})
	if len(e.marks) != 1 || len(e.marks[0].pts) != 5 {
		t.Fatalf("画笔 %+v", e.marks)
	}
}

func TestTextMark(t *testing.T) {
	e := newEditor()
	drag(e, pt{100, 100}, pt{300, 200})
	e.setTool(toolText)
	e.down(pt{150, 150}, 1)
	if !e.editing || e.editAt != (pt{150, 150 - textLineHeight/2}) {
		t.Fatalf("开始输入 %v %v", e.editing, e.editAt)
	}
	e.editText = "你好"
	e.down(pt{200, 180}, 1) // 点别处：提交上一个，开始下一个
	if len(e.marks) != 1 || e.marks[0].text != "你好" || !e.editing {
		t.Fatalf("提交 %+v editing=%v", e.marks, e.editing)
	}
	e.commitText() // 空文字不留
	if len(e.marks) != 1 {
		t.Errorf("空文字留下了 %d", len(e.marks))
	}
}

func TestUndo(t *testing.T) {
	e := newEditor()
	drag(e, pt{100, 100}, pt{300, 200})
	e.setTool(toolRect)
	drag(e, pt{110, 110}, pt{150, 150})
	drag(e, pt{160, 160}, pt{190, 190})
	e.undo()
	if len(e.marks) != 1 {
		t.Fatalf("撤销后 %d", len(e.marks))
	}
	e.setTool(toolText)
	e.down(pt{150, 150}, 1)
	e.editText = "x"
	e.undo() // 输入中撤销：丢掉正在输入的文字
	if e.editing || len(e.marks) != 1 {
		t.Errorf("输入中撤销 editing=%v marks=%d", e.editing, len(e.marks))
	}
}

func TestDoubleClickCopies(t *testing.T) {
	e := newEditor()
	drag(e, pt{100, 100}, pt{300, 200})
	if r := e.down(pt{150, 150}, 2); r != resultCopy {
		t.Errorf("双击选区应复制，得到 %v", r)
	}
	if r := e.down(pt{500, 500}, 2); r == resultCopy {
		t.Error("双击选区外不应复制")
	}
}

func TestCursor(t *testing.T) {
	e := newEditor()
	if e.cursor(pt{10, 10}) != ui.CursorCrosshair {
		t.Error("没有选区时是十字")
	}
	drag(e, pt{100, 100}, pt{300, 200})
	cases := map[pt]ui.Cursor{
		{100, 100}: ui.CursorResizeNWSE,
		{300, 100}: ui.CursorResizeNESW,
		{200, 100}: ui.CursorResizeNS,
		{300, 150}: ui.CursorResizeEW,
		{200, 150}: ui.CursorMove,
		{500, 500}: ui.CursorCrosshair,
	}
	for p, want := range cases {
		if got := e.cursor(p); got != want {
			t.Errorf("%v: %v，期望 %v", p, got, want)
		}
	}
}
