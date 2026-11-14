package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// The keyboard goes through the tasks, ticks one off and puts its details
// away; what is ticked off, moved to a project or tagged can be taken back;
// the places and the bar have their keys.
func TestKeysAndWaysBack(t *testing.T) {
	a, tt := newTestApp(t)
	day := today(a.now())
	search := find(t, a, "搜索")
	a.mutate("", func() { d := day; search.Task.Due = &d })
	other := model.NewFile("发布计划")
	otherPath := filepath.Join(a.dir, model.FileName("发布计划"))
	if err := other.Save(otherPath); err != nil {
		t.Fatal(err)
	}
	a.refreshFiles()
	a.openTasks()
	tt.Frame()

	// Down chooses the first task, Escape puts its details away.
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	tt.Frame()
	if a.tasks.sel == "" || !tt.HasText("关闭详情") {
		t.Fatalf("down chose nothing: %q", a.tasks.sel)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.tasks.sel != "" {
		t.Fatal("escape left the details open")
	}
	// The space bar ticks off the one chosen, and it can be taken back.
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	tt.Frame()
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	tt.Frame()
	search = find(t, a, "搜索")
	if search.Task.Status != model.StatusDone || a.noticeUndo == nil {
		t.Fatalf("space: %v, undo %v", search.Task.Status, a.noticeUndo != nil)
	}
	a.noticeUndo()
	tt.Frame()
	if search = find(t, a, "搜索"); search.Task.Status == model.StatusDone {
		t.Fatal("not taken back")
	}
	// Dropped on a project, a task goes to it, and comes back.
	id := search.ID
	a.tasks.sel = ""
	tt.Frame()
	a.dropTaskOnMap(taskDrag{a.path, id}, otherPath)
	tt.Frame()
	if a.file.Node(id) != nil || a.indexed(otherPath).Node(id) == nil || a.noticeUndo == nil {
		t.Fatal("not moved to the other map")
	}
	a.noticeUndo()
	tt.Frame()
	if a.file.Node(id) == nil || a.indexed(otherPath).Node(id) != nil {
		t.Fatal("not moved back")
	}
	snapshot(t, tt, "keys-today")

	// A clip takes a tag dropped on, and loses it again.
	cl := &library.Clip{Kind: library.KindText, Text: "换一家云服务商"}
	if err := a.lib.Add(cl, a.now()); err != nil {
		t.Fatal(err)
	}
	a.tagClipUndoably(cl.ID, "运维")
	if !slices.Contains(a.lib.Find(cl.ID).Tags, "运维") || a.noticeUndo == nil {
		t.Fatal("no tag")
	}
	a.noticeUndo()
	if slices.Contains(a.lib.Find(cl.ID).Tags, "运维") {
		t.Fatal("tag not taken back")
	}

	// The places and the bar.
	tt.Key(ui.Cmd, ui.Key2)
	tt.Frame()
	tt.Frame()
	if !tt.HasText("一张导图就是一个项目") {
		t.Fatalf("not on 项目: %q", tt.Texts())
	}
	tt.Key(ui.Cmd, ui.Key3)
	tt.Frame()
	tt.Frame()
	if !tt.HasText("灵感和资料") {
		t.Fatalf("not on 收集: %q", tt.Texts())
	}
	// The arrows go through the wall, Escape closes what they opened.
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if a.libSel != cl.ID {
		t.Fatalf("down chose %q", a.libSel)
	}
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.libSel != "" {
		t.Fatal("escape left the clip open")
	}
	tt.Key(ui.Cmd, ui.KeyK)
	tt.Frame()
	if !a.barOpen {
		t.Fatal("the bar did not open")
	}
}

// On the wall the arrows go where they point: right to the card beside,
// left back to it, down its column and up again.
func TestWallArrowsGoByPlace(t *testing.T) {
	a, tt := newTestApp(t)
	for i := range 9 {
		cl := &library.Clip{Kind: library.KindText, Text: fmt.Sprint("第 ", i, " 条")}
		if err := a.lib.Add(cl, a.now().Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	tt.SetSize(2000, 1000) // wide: several columns stay beside the details
	a.openList(kindAll, "")
	tt.Frame()
	key := func(k ui.Key) string {
		tt.Key(0, k)
		tt.Frame()
		tt.Frame()
		return a.libSel
	}
	first := key(ui.KeyDown)
	if first == "" {
		t.Fatal("down chose nothing")
	}
	beside := key(ui.KeyRight)
	if beside == first {
		t.Fatal("right stayed")
	}
	if got := key(ui.KeyLeft); got != first {
		t.Fatalf("left went to %q, not back to %q", got, first)
	}
	below := key(ui.KeyDown)
	if below == first || below == beside {
		t.Fatalf("down went to %q", below)
	}
	if got := key(ui.KeyUp); got != first {
		t.Fatalf("up went to %q, not back to %q", got, first)
	}
	// The card below is the one under the first on the screen.
	r1, _ := tt.Find("资料 " + a.lib.Find(first).Label())
	r2, ok := tt.Find("资料 " + a.lib.Find(below).Label())
	if !ok || r2.X != r1.X || r2.Y <= r1.Y {
		t.Fatalf("below is at %v, the first at %v", r2, r1)
	}
}

// A picture of the wall is read away from the window's work, small: its
// card is there at once, a pale block in the picture's room until then.
func TestWallPicturesReadAside(t *testing.T) {
	a, tt := newTestApp(t)
	img := image.NewRGBA(image.Rect(0, 0, 2000, 1200))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	cl, err := a.lib.AddData(buf.Bytes(), "shot.png", library.KindImage, "", a.now())
	if err != nil {
		t.Fatal(err)
	}
	var waiting []func()
	a.async = func(work func() (any, error), done func(any, error)) {
		waiting = append(waiting, func() { done(work()) })
	}
	a.openList(kindAll, "")
	tt.Frame()
	path := a.lib.FilePath(cl)
	if _, read := a.bitmaps["thumb:"+path]; read || len(waiting) != 1 || !tt.HasText("资料 "+cl.Label()) {
		t.Fatalf("read %v, waiting %d", read, len(waiting))
	}
	tt.Frame() // asked for once, however many frames pass
	if len(waiting) != 1 {
		t.Fatalf("asked for %d times", len(waiting))
	}
	waiting[0]()
	tt.Frame()
	b := a.bitmaps["thumb:"+path]
	if b == nil {
		t.Fatal("no picture")
	}
	if w, h := b.Size(); w > 2000/2 || h < thumbTall {
		t.Fatalf("not small: %d×%d", w, h)
	}
	snapshot(t, tt, "wall-picture")
}
