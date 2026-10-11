package screenshot

import (
	"bytes"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// The markup of a shot of a Retina display: 800×600 pixels, shown at
// 400×300 points, in a window of 640×420 whose toolbar is 52 high. The
// shot sits at (120, 86).
func TestAnnotator(t *testing.T) {
	a := NewAnnotator(checker(800, 600))
	a.Scale = 2
	var got []byte
	var act Action = -1
	a.Deliver = func(p []byte, ac Action) { got, act = p, ac }
	tt := ui.NewTester(a.View, 640, 420)
	tt.Frame()
	if dir := os.Getenv("MINDMAP_SNAPSHOTS"); dir != "" {
		defer func() {
			f, _ := os.Create(filepath.Join(dir, "annotate.png"))
			png.Encode(f, tt.Image())
			f.Close()
		}()
	}

	// A box drawn with the first tool, the rectangle.
	tt.Press(200, 150)
	tt.Move(250, 200)
	tt.Release(300, 250)
	if len(a.ed.marks) != 1 || a.ed.marks[0].tool != toolRect {
		t.Fatalf("marks %+v", a.ed.marks)
	}
	if m := a.ed.marks[0]; m.first() != (pt{80, 64}) || m.last() != (pt{180, 164}) {
		t.Fatalf("not where drawn: %+v", m.pts)
	}
	// Text, written where clicked.
	if err := tt.Click("文字"); err != nil {
		t.Fatal(err)
	}
	tt.Press(400, 300)
	tt.Release(400, 300)
	tt.Frame()
	if !a.ed.editing {
		t.Fatal("no text being written")
	}
	tt.Type("重点")
	tt.Key(0, ui.KeyEnter)
	if a.ed.editing || len(a.ed.marks) != 2 || a.ed.marks[1].text != "重点" {
		t.Fatalf("text %+v", a.ed.marks)
	}
	// An arrow, then undone.
	tt.Click("箭头")
	tt.Press(150, 300)
	tt.Release(250, 350)
	if len(a.ed.marks) != 3 {
		t.Fatalf("arrow %+v", a.ed.marks)
	}
	tt.Click("撤销 ⌘Z")
	if len(a.ed.marks) != 2 {
		t.Fatal("not undone")
	}
	// Outside the shot, nothing is drawn.
	tt.Click("矩形")
	tt.Press(20, 120)
	tt.Release(60, 160)
	if len(a.ed.marks) != 2 {
		t.Fatal("drawn outside the shot")
	}

	if err := tt.Click("收进资料库"); err != nil {
		t.Fatal(err)
	}
	if act != Library || len(got) == 0 {
		t.Fatalf("delivered %v, %d bytes", act, len(got))
	}
	img, err := png.Decode(bytes.NewReader(got))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 800 || b.Dy() != 600 {
		t.Fatalf("exported at %v, not the shot's own pixels", b)
	}
	// The box's left edge, at (80, 100) in points, (160, 200) in pixels.
	c := color.RGBAModel.Convert(img.At(160, 200)).(color.RGBA)
	if !isMarkColor(c) {
		t.Fatalf("no box where drawn: %v", c)
	}
	// Once delivered, nothing more is.
	act = -1
	tt.Click("复制截图")
	if act != -1 {
		t.Fatal("delivered twice")
	}
}

func TestAnnotatorCancelAndKeys(t *testing.T) {
	a := NewAnnotator(checker(400, 300))
	canceled := false
	a.Cancel = func() { canceled = true }
	var act Action = -1
	a.Deliver = func(_ []byte, ac Action) { act = ac }
	tt := ui.NewTester(a.View, 640, 420)
	tt.Key(ui.Cmd, ui.KeyC)
	if act != Copy {
		t.Fatalf("⌘C gave %v", act)
	}

	b := NewAnnotator(checker(400, 300))
	b.Cancel = func() { canceled = true }
	tt = ui.NewTester(b.View, 640, 420)
	tt.Key(0, ui.KeyEscape)
	if !canceled {
		t.Fatal("Esc did not cancel")
	}
	if w, h := b.Size(1440, 900); w != 520 || h != 384 {
		t.Fatalf("window %v×%v", w, h)
	}
}
