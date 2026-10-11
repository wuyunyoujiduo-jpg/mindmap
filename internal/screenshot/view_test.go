package screenshot

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func newTestSession(w, h int) (*session, *screenShot) {
	sh := &screenShot{bmp: ui.NewBitmap(checker(w*2, h*2)), scale: 2, ed: &editor{}}
	s := &session{shots: []*screenShot{sh}, owner: -1, onEnd: func() {}}
	return s, sh
}

func TestOverlaySelectAndAnnotate(t *testing.T) {
	s, sh := newTestSession(800, 600)
	tt := ui.NewTester(s.view(sh), 800, 600)

	tt.Press(100, 100)
	tt.Move(200, 150)
	tt.Move(300, 200)
	tt.Release(300, 200)
	if !sh.ed.hasSel || sh.ed.sel != (ui.Rect{X: 100, Y: 100, W: 200, H: 100}) {
		t.Fatalf("选区 %v %v", sh.ed.hasSel, sh.ed.sel)
	}
	if s.owner != 0 {
		t.Errorf("框选后应独占编辑，owner=%d", s.owner)
	}

	if err := tt.Click("矩形"); err != nil {
		t.Fatal(err)
	}
	if sh.ed.tool != toolRect {
		t.Fatalf("点矩形后工具 %v", sh.ed.tool)
	}
	tt.Press(120, 120)
	tt.Move(160, 160)
	tt.Release(180, 170)
	if len(sh.ed.marks) != 1 {
		t.Fatalf("应画上一个矩形，marks=%+v", sh.ed.marks)
	}

	if err := tt.Click("撤销 " + cmdLabel("Z")); err != nil {
		t.Fatal(err)
	}
	if len(sh.ed.marks) != 0 {
		t.Errorf("撤销后 %d", len(sh.ed.marks))
	}
}

func TestOverlayTextAnnotation(t *testing.T) {
	s, sh := newTestSession(800, 600)
	tt := ui.NewTester(s.view(sh), 800, 600)
	tt.Press(100, 100)
	tt.Release(400, 300)
	if err := tt.Click("文字"); err != nil {
		t.Fatal(err)
	}
	tt.ClickAt(150, 150)
	if !sh.ed.editing {
		t.Fatal("点选区应开始输入文字")
	}
	tt.Type("标注")
	tt.Key(0, ui.KeyEnter) // 回车提交文字，不结束截图
	if sh.ed.editing || len(sh.ed.marks) != 1 || sh.ed.marks[0].text != "标注" {
		t.Fatalf("提交后 editing=%v marks=%+v", sh.ed.editing, sh.ed.marks)
	}
	if s.ended {
		t.Error("提交文字不应结束截图")
	}
}
