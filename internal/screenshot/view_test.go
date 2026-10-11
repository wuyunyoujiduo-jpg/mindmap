package screenshot

import (
	"image/png"
	"os"
	"path/filepath"
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

	if dir := os.Getenv("MINDMAP_SNAPSHOTS"); dir != "" {
		f, _ := os.Create(filepath.Join(dir, "overlay.png"))
		png.Encode(f, tt.Image())
		f.Close()
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

// Marks take the color chosen, and Enter keeps the shot in the library.
func TestOverlayColorsAndEnter(t *testing.T) {
	s, sh := newTestSession(800, 600)
	var act Action = -1
	s.deliver = func(_ []byte, a Action) { act = a }
	tt := ui.NewTester(s.view(sh), 800, 600)
	tt.Press(100, 100)
	tt.Release(400, 300)
	if err := tt.Click("黄色"); err != nil {
		t.Fatal(err)
	}
	if sh.ed.tool != toolRect {
		t.Fatalf("a color chosen picks a tool: %v", sh.ed.tool)
	}
	tt.Press(150, 150)
	tt.Release(250, 250)
	if len(sh.ed.marks) != 1 || sh.ed.marks[0].col != markColors[1] {
		t.Fatalf("marks %+v", sh.ed.marks)
	}
	_ = act
}

// A window macOS kept below the menu bar, 30 points down: the shot lies
// as it was on the screen, and what is framed is what was under it.
func TestOverlayBelowMenuBar(t *testing.T) {
	s, sh := newTestSession(800, 600)
	sh.off = pt{0, 30}
	tt := ui.NewTester(s.view(sh), 800, 570)
	tt.Press(100, 100)
	tt.Release(300, 200)
	size, sel, _ := sh.exportArea()
	if sel != (ui.Rect{X: 100, Y: 130, W: 200, H: 100}) || size != (pt{800, 600}) {
		t.Fatalf("sel %v size %v", sel, size)
	}
	img := renderSelection(sh.bmp, size, sel, nil, sh.scale)
	src := checker(1600, 1200)
	for _, p := range [][2]int{{10, 10}, {150, 90}} {
		got, want := img.RGBAAt(p[0], p[1]), src.RGBAAt(200+p[0], 260+p[1])
		if diff(got.R, want.R) > 8 || diff(got.G, want.G) > 8 {
			t.Fatalf("at %v: %v, the screen had %v", p, got, want)
		}
	}
}
