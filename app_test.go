package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

type fakePlatform struct {
	folder     string
	trashed    []string
	fullScreen bool
	clipImage  []byte
	clipFiles  []string
	picked     string
}

func (f *fakePlatform) ClipboardImage() []byte      { return f.clipImage }
func (f *fakePlatform) ClipboardFiles() []string    { return f.clipFiles }
func (f *fakePlatform) PickImage(done func(string)) { done(f.picked) }

func (f *fakePlatform) PickFolder(done func(string)) { done(f.folder) }
func (f *fakePlatform) ShowInFolder(string)          {}
func (f *fakePlatform) SetFullScreen(on bool)        { f.fullScreen = on }
func (f *fakePlatform) Trash(path string) error {
	f.trashed = append(f.trashed, path)
	return os.RemoveAll(path)
}

var testNow = time.Date(2026, 10, 10, 9, 0, 0, 0, time.Local)

// newTestApp opens an app on a fresh folder, which gets the guide map.
func newTestApp(t *testing.T) (*app, *ui.Tester) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	a := newApp(&fakePlatform{}, config{Dir: dir})
	a.now = func() time.Time { return testNow }
	if a.doc == nil {
		t.Fatal("no map opened")
	}
	tt := ui.NewTester(a.view, 1280, 800)
	return a, tt
}

// snapshot saves the frame as a PNG when MINDMAP_SNAPSHOTS names a folder.
func snapshot(t *testing.T, tt *ui.Tester, name string) {
	dir := os.Getenv("MINDMAP_SNAPSHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

// clickNode clicks the middle of a node's box on the canvas, which the
// sidebar's rows of the same text cannot take.
func clickNode(t *testing.T, tt *ui.Tester, a *app, title string) error {
	t.Helper()
	tt.Frame()
	n := find(t, a, title)
	b, ok := a.boxes.Boxes[n.ID]
	if !ok {
		return fmt.Errorf("node %q not shown", title)
	}
	cam := a.cam()
	// Bring the node into view first, as fonts differ from machine to
	// machine.
	cam.X += keepIn(cam.X+b.X*cam.Zoom, b.W*cam.Zoom, a.vp.W)
	cam.Y += keepIn(cam.Y+b.Y*cam.Zoom, b.H*cam.Zoom, a.vp.H)
	tt.Frame()
	tt.ClickAt(a.vp.X+cam.X+(b.X+b.W/2)*cam.Zoom, a.vp.Y+cam.Y+(b.Y+b.H/2)*cam.Zoom)
	return nil
}

func find(t *testing.T, a *app, title string) *model.Node {
	t.Helper()
	var found *model.Node
	a.doc.Walk(func(n, _ *model.Node, _ int) bool {
		if n.Title == title {
			found = n
			return false
		}
		return true
	})
	if found == nil {
		t.Fatalf("no node %q", title)
	}
	return found
}

func TestGuideMapShows(t *testing.T) {
	a, tt := newTestApp(t)
	for _, s := range []string{"我的项目", "使用指南", "架构", "待优化", "列表加载慢"} {
		if !tt.HasText(s) {
			t.Errorf("missing %q in %q", s, tt.Texts())
		}
	}
	if len(a.files) != 1 || a.files[0].Name != "使用指南" {
		t.Fatalf("files %+v", a.files)
	}
	snapshot(t, tt, "map")
}

func TestKeyboardEditing(t *testing.T) {
	a, tt := newTestApp(t)
	if err := clickNode(t, tt, a, "架构"); err != nil {
		t.Fatal(err)
	}
	if n, _ := a.selectedNode(); n.Title != "架构" {
		t.Fatalf("selected %q", n.Title)
	}
	// Tab adds a child and edits its title.
	tt.Key(0, ui.KeyTab)
	if a.editing == "" {
		t.Fatal("Tab did not start editing a new child")
	}
	tt.Type("数据库")
	tt.Key(0, ui.KeyEnter)
	arch := find(t, a, "架构")
	if got := arch.Children[len(arch.Children)-1].Title; got != "数据库" {
		t.Fatalf("new child %q", got)
	}
	if a.editing != "" {
		t.Fatal("still editing after Enter")
	}
	// Enter on the canvas adds a sibling.
	tt.Key(0, ui.KeyEnter)
	tt.Type("缓存")
	tt.Key(0, ui.KeyEnter)
	if got := arch.Children[len(arch.Children)-1].Title; got != "缓存" {
		t.Fatalf("new sibling %q, children %v", got, len(arch.Children))
	}
	snapshot(t, tt, "edited")

	// Navigation and delete.
	tt.Key(0, ui.KeyUp)
	if n, _ := a.selectedNode(); n.Title != "数据库" {
		t.Fatalf("after Up: %q", n.Title)
	}
	tt.Key(0, ui.KeyDelete)
	if slices.ContainsFunc(arch.Children, func(n *model.Node) bool { return n.Title == "数据库" }) {
		t.Fatal("not deleted")
	}
	// Undo brings it back.
	tt.Key(ui.Cmd, ui.KeyZ)
	arch = find(t, a, "架构")
	if !slices.ContainsFunc(arch.Children, func(n *model.Node) bool { return n.Title == "数据库" }) {
		t.Fatal("undo did not restore")
	}
	tt.Key(0, ui.KeyLeft)
	if n, _ := a.selectedNode(); n.Title != "架构" {
		t.Fatalf("after Left: %q", n.Title)
	}
}

func TestEscapeCancelsEdit(t *testing.T) {
	a, tt := newTestApp(t)
	clickNode(t, tt, a, "架构")
	clickNode(t, tt, a, "架构")
	if a.editing == "" {
		t.Fatal("double click did not edit")
	}
	tt.Type("XYZ")
	tt.Key(0, ui.KeyEscape)
	if a.editing != "" || find(t, a, "架构") == nil {
		t.Fatal("escape did not cancel")
	}
}

func TestPanelEditsNode(t *testing.T) {
	a, tt := newTestApp(t)
	if err := clickNode(t, tt, a, "支持导出图片"); err != nil {
		t.Fatal(err)
	}
	n, _ := a.selectedNode()
	if n.Title != "支持导出图片" {
		t.Fatalf("selected %q", n.Title)
	}
	if err := tt.Click("备注"); err != nil {
		t.Fatal(err)
	}
	tt.Type("PNG 和 SVG")
	if n.Note != "PNG 和 SVG" {
		t.Fatalf("note %q", n.Note)
	}
	if err := tt.Click("标签"); err != nil {
		t.Fatal(err)
	}
	tt.Type("导出")
	tt.Key(0, ui.KeyEnter)
	if !n.HasTag("导出") {
		t.Fatalf("tags %v", n.Tags)
	}
	if err := tt.Click("进行中"); err != nil {
		t.Fatal(err)
	}
	if n.Task.Status != model.StatusDoing {
		t.Fatalf("status %v", n.Task.Status)
	}
	if err := tt.Click("截止日期"); err != nil {
		t.Fatal(err)
	}
	if n.Task.Due == nil || !n.Task.Due.Equal(today(testNow)) {
		t.Fatalf("due %v", n.Task.Due)
	}
	snapshot(t, tt, "panel")

	// Typing in one field is one undo step.
	a.undo()
	a.undo()
	a.undo()
	a.undo()
	n = find(t, a, "支持导出图片")
	if n.Note != "" || n.Task.Status != model.StatusIdea {
		t.Fatalf("after undo: note %q status %v", n.Note, n.Task.Status)
	}
}

func TestAutosaveAndReload(t *testing.T) {
	a, tt := newTestApp(t)
	find(t, a, "架构").Title = "x" // not through mutate: not saved
	a.mutate("", func() { find(t, a, "前端").Title = "界面" })
	if !a.dirty {
		t.Fatal("not dirty")
	}
	a.now = func() time.Time { return testNow.Add(time.Second) }
	tt.Frame()
	if a.dirty {
		t.Fatal("not saved after the delay")
	}
	f, err := model.Load(a.path)
	if err != nil {
		t.Fatal(err)
	}
	d := f.Sheets[0]
	if _, err := findIn(d, "界面"); err != nil {
		t.Fatal(err)
	}

	// Another program changes the file.
	d.Root.Title = "AI 改过的项目"
	if err := f.Save(a.path); err != nil {
		t.Fatal(err)
	}
	future := testNow.Add(time.Minute)
	os.Chtimes(a.path, future, future)
	a.now = func() time.Time { return testNow.Add(10 * time.Second) }
	tt.Frame()
	if a.doc.Root.Title != "AI 改过的项目" {
		t.Fatalf("not reloaded: %q", a.doc.Root.Title)
	}
}

func findIn(d *model.Document, title string) (*model.Node, error) {
	var found *model.Node
	d.Walk(func(n, _ *model.Node, _ int) bool {
		if n.Title == title {
			found = n
		}
		return found == nil
	})
	if found == nil {
		return nil, os.ErrNotExist
	}
	return found, nil
}

func TestBoardAndAgenda(t *testing.T) {
	a, tt := newTestApp(t)
	if err := tt.Click("看板"); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"想法", "待做", "进行中", "完成", "列表加载慢", "搜索"} {
		if !tt.HasText(s) {
			t.Errorf("board misses %q", s)
		}
	}
	snapshot(t, tt, "board")
	if err := tt.Click("日程"); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"未来 7 天  2", "未安排日期  1"} {
		if !tt.HasText(s) {
			t.Errorf("agenda misses %q in %q", s, tt.Texts())
		}
	}
	snapshot(t, tt, "agenda")
	_ = a
}

func TestTagFilterAndSearch(t *testing.T) {
	a, tt := newTestApp(t)
	if err := tt.Click("核心"); err != nil { // the sidebar's row comes first
		t.Fatal(err)
	}
	if a.tagFilter != "核心" {
		t.Fatalf("tag filter %q", a.tagFilter)
	}
	snapshot(t, tt, "filtered")
	a.tagFilter = ""
	a.search = "加载"
	a.nextMatch()
	if n, _ := a.selectedNode(); n.Title != "列表加载慢" {
		t.Fatalf("search selected %q", n.Title)
	}
}

func TestCollapse(t *testing.T) {
	a, tt := newTestApp(t)
	clickNode(t, tt, a, "功能")
	if err := tt.Click("折叠"); err != nil {
		t.Fatal(err)
	}
	if !find(t, a, "功能").Collapsed || tt.HasText("登录") {
		t.Fatal("not collapsed")
	}
	if !a.boxes.Left[find(t, a, "功能").ID] {
		t.Fatal("功能 is not on the left")
	}
	tt.Key(0, ui.KeyLeft) // away from the root: opens it and goes into it
	if find(t, a, "功能").Collapsed {
		t.Fatal("Left did not open")
	}
	if n, _ := a.selectedNode(); n.Title != "登录" {
		t.Fatalf("selected %q", n.Title)
	}
	tt.Key(0, ui.KeyRight) // toward the root: the parent
	if n, _ := a.selectedNode(); n.Title != "功能" {
		t.Fatalf("selected %q", n.Title)
	}
}

func TestNewMapAndDelete(t *testing.T) {
	a, tt := newTestApp(t)
	if err := tt.Click("+ 导图"); err != nil {
		t.Fatal(err)
	}
	if len(a.files) != 2 || mapName(a.path) != "新导图" || a.editing != a.doc.Root.ID {
		t.Fatalf("files %d, path %q, editing %q", len(a.files), a.path, a.editing)
	}
	tt.Type("第二个项目")
	tt.Key(0, ui.KeyEnter)
	if a.doc.Root.Title != "第二个项目" {
		t.Fatalf("root %q", a.doc.Root.Title)
	}
	a.deleteMap(a.path)
	if len(a.files) != 1 || mapName(a.path) != "使用指南" {
		t.Fatalf("after delete: files %d, path %q", len(a.files), a.path)
	}
}

func TestAgendaGroups(t *testing.T) {
	day := func(n int) *time.Time { d := today(testNow).AddDate(0, 0, n); return &d }
	mk := func(title string, task model.Task) model.TaskNode {
		return model.TaskNode{Node: &model.Node{Title: title, Task: &task}}
	}
	groups := agenda([]model.TaskNode{
		mk("late", model.Task{Status: model.StatusTodo, Due: day(-1)}),
		mk("today", model.Task{Status: model.StatusTodo, Due: day(0)}),
		mk("started", model.Task{Status: model.StatusDoing, Start: day(-3)}),
		mk("soon", model.Task{Status: model.StatusTodo, Due: day(3)}),
		mk("later", model.Task{Status: model.StatusTodo, Due: day(30)}),
		mk("none", model.Task{Status: model.StatusIdea}),
		mk("done", model.Task{Status: model.StatusDone, Due: day(-1)}),
	}, testNow)
	want := [][]string{{"late"}, {"today", "started"}, {"soon"}, {"later"}, {"none"}}
	for i, g := range groups {
		var got []string
		for _, tn := range g.tasks {
			got = append(got, tn.Node.Title)
		}
		if !slices.Equal(got, want[i]) {
			t.Errorf("%s: %v, want %v", g.title, got, want[i])
		}
	}
}

func TestDragNodeToReparent(t *testing.T) {
	a, tt := newTestApp(t)
	from, ok1 := tt.Find("支持导出图片")
	to, ok2 := tt.Find("功能")
	if !ok1 || !ok2 {
		t.Fatal("nodes not found")
	}
	tt.Press(from.X+5, from.Y+5)
	for i := 1; i <= 10; i++ {
		f := float32(i) / 10
		tt.Move(from.X+5+(to.X+5-from.X-5)*f, from.Y+5+(to.Y+5-from.Y-5)*f)
	}
	tt.Release(to.X+5, to.Y+5)
	tt.Frame()
	feat := find(t, a, "功能")
	if !slices.ContainsFunc(feat.Children, func(n *model.Node) bool { return n.Title == "支持导出图片" }) {
		t.Fatalf("not moved under 功能: %v", feat.Children)
	}
}

func TestZoomAndDark(t *testing.T) {
	a, tt := newTestApp(t)
	tt.SetDark(true)
	a.setZoom(1.3)
	tt.Frame()
	if err := clickNode(t, tt, a, "架构"); err != nil {
		t.Fatal(err)
	}
	if n, p := a.selectedNode(); n.Title != "架构" || p != a.doc.Root {
		t.Fatalf("selected %q", n.Title)
	}
	tt.Frame()
	snapshot(t, tt, "dark-zoom")
	if !tt.HasText("130%") {
		t.Fatalf("zoom label missing: %q", tt.Texts())
	}
}

func TestPanCanvas(t *testing.T) {
	a, tt := newTestApp(t)
	cam := a.cam()
	if !cam.placed {
		t.Fatal("map not placed")
	}
	x0, y0 := cam.X, cam.Y
	// Dragging the empty part of the canvas does not move the map: only
	// the central node does.
	r, _ := tt.Find("我的项目")
	px, py := r.X, r.Y+250 // below the map
	tt.Move(px, py)
	if tt.Cursor() != ui.CursorDefault {
		t.Fatalf("cursor over the empty canvas is %v", tt.Cursor())
	}
	tt.Press(px, py)
	for i := 1; i <= 5; i++ {
		tt.Move(px+float32(i)*20, py+float32(i)*10)
	}
	tt.Release(px+100, py+50)
	if cam.X != x0 || cam.Y != y0 {
		t.Fatalf("dragging the empty canvas moved the map by %v, %v", cam.X-x0, cam.Y-y0)
	}
	// The wheel moves it too, and Cmd with the wheel zooms.
	x1 := cam.X
	tt.Scroll(px, py, 30, 0)
	if cam.X != x1-30 {
		t.Fatalf("scrolling moved x from %v to %v", x1, cam.X)
	}
	tt.HoldModifiers(ui.Cmd)
	tt.Scroll(px, py, 0, -40)
	tt.HoldModifiers(0)
	if cam.Zoom <= 1 {
		t.Fatalf("zoom %v after Cmd+scroll", cam.Zoom)
	}
	snapshot(t, tt, "panned")
	a.recenter()
	tt.Frame()
	if cam.Zoom != 1 || !cam.placed {
		t.Fatal("not recentered")
	}
}

func TestDraggingNodeDoesNotPan(t *testing.T) {
	a, tt := newTestApp(t)
	cam := a.cam()
	x0 := cam.X
	r, _ := tt.Find("前端")
	root, _ := tt.Find("我的项目")
	bx, by := root.X, root.Y+250 // empty canvas below the map
	tt.Press(r.X+5, r.Y+5)
	for i := 1; i <= 5; i++ {
		f := float32(i) / 5
		tt.Move(r.X+5+(bx-r.X-5)*f, r.Y+5+(by-r.Y-5)*f)
	}
	tt.Release(bx, by)
	if cam.X != x0 {
		t.Fatal("dragging a node moved the canvas")
	}
	if _, p := a.doc.Find(find(t, a, "前端").ID); p.Title != "架构" {
		t.Fatalf("dropped on nothing, moved under %q", p.Title)
	}
}

func TestSheets(t *testing.T) {
	a, tt := newTestApp(t)
	if len(a.file.Sheets) != 2 || !tt.HasText("项目结构") || !tt.HasText("迭代计划") {
		t.Fatalf("tabs %q", tt.Texts())
	}
	clickNode(t, tt, a, "架构")
	if err := tt.Click("迭代计划"); err != nil {
		t.Fatal(err)
	}
	if _, err := findIn(a.doc, "架构"); a.doc.Title != "迭代计划" || !tt.HasText("发布 0.2") || err == nil || tt.HasText("前端") {
		t.Fatalf("showing %q: %q", a.doc.Title, tt.Texts())
	}
	// Each canvas keeps its selection.
	tt.Click("项目结构")
	if n, _ := a.selectedNode(); n.Title != "架构" {
		t.Fatalf("selection %q after coming back", n.Title)
	}
	// + adds a canvas and edits its root.
	if err := tt.Click("新建画布"); err != nil {
		t.Fatal(err)
	}
	if len(a.file.Sheets) != 3 || a.doc.Title != "画布 3" || a.editing != a.doc.Root.ID {
		t.Fatalf("sheets %d, shown %q", len(a.file.Sheets), a.doc.Title)
	}
	tt.Type("调研")
	tt.Key(0, ui.KeyEnter)
	// Rename by double click.
	r, _ := tt.Find("画布 3")
	tt.ClickAt(r.X+2, r.Y+2)
	tt.ClickAt(r.X+2, r.Y+2)
	if a.renamingSheet == "" {
		t.Fatal("double click did not rename")
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("技术调研")
	tt.Key(0, ui.KeyEnter)
	if a.doc.Title != "技术调研" {
		t.Fatalf("title %q", a.doc.Title)
	}
	snapshot(t, tt, "sheets")
	// Undo takes back the rename, then the root's title, then the canvas.
	a.undo()
	if a.doc.Title != "画布 3" {
		t.Fatalf("after undo %q", a.doc.Title)
	}
	a.undo()
	a.undo()
	if len(a.file.Sheets) != 2 {
		t.Fatalf("%d sheets after undo", len(a.file.Sheets))
	}
	// Saved with every canvas.
	a.save()
	f, err := model.Load(a.path)
	if err != nil || len(f.Sheets) != 2 || f.Sheets[1].Title != "迭代计划" {
		t.Fatalf("saved %v %v", f, err)
	}
	a.deleteSheet(a.file.Sheets[1].ID)
	a.deleteSheet(a.file.Sheets[0].ID)
	if len(a.file.Sheets) != 1 {
		t.Fatal("deleted the last canvas")
	}
}

func TestBoardShowsTasksOfEveryCanvas(t *testing.T) {
	a, tt := newTestApp(t)
	tt.Click("看板")
	if !tt.HasText("发布 0.2") {
		t.Fatalf("board misses the second canvas's task: %q", tt.Texts())
	}
	r, _ := tt.Find("发布 0.2")
	tt.ClickAt(r.X+2, r.Y+2)
	tt.ClickAt(r.X+2, r.Y+2)
	if a.mode != viewMap || a.doc.Title != "迭代计划" {
		t.Fatalf("double click shows %v %q", a.mode, a.doc.Title)
	}
	if n, _ := a.selectedNode(); n.Title != "发布 0.2" {
		t.Fatalf("selected %q", n.Title)
	}
}

func TestOpensVersion1File(t *testing.T) {
	a, tt := newTestApp(t)
	path := filepath.Join(a.dir, "旧"+model.Ext)
	os.WriteFile(path, []byte(`{"version":1,"root":{"id":"r","title":"旧项目","children":[{"id":"a","title":"模块"}]}}`), 0o644)
	a.refreshFiles()
	if err := a.open(path); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("旧项目") || !tt.HasText("画布 1") {
		t.Fatalf("texts %q", tt.Texts())
	}
}

func TestDragRootMovesMap(t *testing.T) {
	a, tt := newTestApp(t)
	cam := a.cam()
	x0, y0 := cam.X, cam.Y
	r, _ := tt.Find("我的项目") // the root's text, not its edge
	px, py := r.X+r.W/2, r.Y+r.H/2
	tt.Move(px, py)
	if tt.Cursor() != ui.CursorGrab {
		t.Fatalf("cursor over the root's text is %v", tt.Cursor())
	}
	tt.Press(px, py)
	for i := 1; i <= 5; i++ {
		tt.Move(px+float32(i)*20, py+float32(i)*12)
	}
	if tt.Cursor() != ui.CursorGrabbing {
		t.Fatalf("cursor while dragging the root is %v", tt.Cursor())
	}
	tt.Release(px+100, py+60)
	if dx, dy := cam.X-x0, cam.Y-y0; dx < 80 || dy < 50 {
		t.Fatalf("dragging the root moved the map by %v, %v", dx, dy)
	}
	if after, _ := tt.Find("我的项目"); after.X-r.X < 80 {
		t.Fatalf("root drawn at %v, was %v", after.X, r.X)
	}
	if n, _ := a.selectedNode(); n != a.doc.Root {
		t.Fatalf("selected %q", n.Title)
	}

	// Other nodes show a hand, over their text too.
	c, _ := tt.Find("功能")
	tt.Move(c.X+c.W/2, c.Y+c.H/2)
	if tt.Cursor() != ui.CursorPointer {
		t.Fatalf("cursor over a node's text is %v", tt.Cursor())
	}
}

func TestRootCenteredAndLongTitlesWrap(t *testing.T) {
	a, tt := newTestApp(t)
	root, _ := tt.Find("我的项目")
	vp := a.vp
	if cx := root.X + root.W/2; cx < vp.X+vp.W/2-20 || cx > vp.X+vp.W/2+20 {
		t.Fatalf("root centered at x %v, canvas %v..%v", cx, vp.X, vp.X+vp.W)
	}
	if cy := root.Y + root.H/2; cy < vp.Y+vp.H/2-20 || cy > vp.Y+vp.H/2+20 {
		t.Fatalf("root centered at y %v, canvas %v..%v", cy, vp.Y, vp.Y+vp.H)
	}
	// Branches on both sides.
	left, right := 0, 0
	for _, c := range a.doc.Root.Children {
		if a.boxes.Left[c.ID] {
			left++
		} else {
			right++
		}
	}
	if left == 0 || right == 0 {
		t.Fatalf("%d branches left, %d right", left, right)
	}

	// A long title wraps inside its node.
	long := "支持配置接入 AI，右侧有对话框支持对文件修改，打开默认选择当前画布，AI 是否需要依赖本地工具"
	n := find(t, a, "前端")
	a.mutate("", func() { n.Title = long })
	tt.Frame()
	box := a.boxes.Boxes[n.ID]
	// Wrapped: no wider than the widest title (40 for the note's mark), and
	// taller than a node of one line, however tall the fonts make lines.
	if box.W > 2*nodePadX+titleMax+40 || box.H < nodeHeight+10 {
		t.Fatalf("long node box %+v", box)
	}
	text, ok := tt.Find(long)
	if !ok {
		t.Fatal("long title not shown")
	}
	cam := a.cam()
	if right := cam.X + (box.X+box.W)*cam.Zoom; text.X+text.W > a.vp.X+right+1 {
		t.Fatalf("text ends at %v, box at %v", text.X+text.W, a.vp.X+right)
	}
	snapshot(t, tt, "balanced")
}

func TestStructures(t *testing.T) {
	a, tt := newTestApp(t)
	if err := tt.Click("结构：思维导图 ▾"); err != nil {
		t.Fatal(err)
	}
	snapshot(t, tt, "structure-picker")
	if err := tt.Click("逻辑图"); err != nil {
		t.Fatal(err)
	}
	if a.doc.Structure != model.StructureLogic || a.structureOpen {
		t.Fatalf("structure %q, open %v", a.doc.Structure, a.structureOpen)
	}
	tt.Frame()
	if len(a.boxes.Left) != 0 {
		t.Fatal("logic chart has branches on the left")
	}
	root := a.boxes.Boxes[a.doc.Root.ID]
	for _, c := range a.doc.Root.Children {
		if a.boxes.Boxes[c.ID].X <= root.X {
			t.Fatalf("%s left of the root", c.Title)
		}
	}
	snapshot(t, tt, "logic")
	tt.Click("结构：逻辑图 ▾")
	tt.Click("括号图")
	if a.doc.Structure != model.StructureBracket {
		t.Fatalf("structure %q", a.doc.Structure)
	}
	tt.Frame()
	snapshot(t, tt, "bracket")
	// Saved with the canvas, and undone like any edit.
	a.save()
	f, _ := model.Load(a.path)
	if f.Sheets[0].Structure != model.StructureBracket || f.Sheets[1].Structure != model.StructureMindMap {
		t.Fatalf("saved %q %q", f.Sheets[0].Structure, f.Sheets[1].Structure)
	}
	a.undo()
	if a.doc.Structure != model.StructureLogic {
		t.Fatalf("after undo %q", a.doc.Structure)
	}
}

func TestCanvasStyle(t *testing.T) {
	a, tt := newTestApp(t)
	snapshot(t, tt, "style-default")
	if err := tt.Click("画布"); err != nil {
		t.Fatal(err)
	}
	if a.panelTab != tabCanvas || !tt.HasText("配色方案") {
		t.Fatalf("tab %d: %q", a.panelTab, tt.Texts())
	}
	snapshot(t, tt, "style-panel")
	if err := tt.Click("海洋"); err != nil {
		t.Fatal(err)
	}
	if a.doc.Style.Scheme != "ocean" {
		t.Fatalf("scheme %q", a.doc.Style.Scheme)
	}
	if err := tt.Click("彩虹分支"); err != nil {
		t.Fatal(err)
	}
	if !a.doc.Style.SingleColor {
		t.Fatal("rainbow still on")
	}
	if err := tt.Click("#7C5CE0"); err != nil {
		t.Fatal(err)
	}
	if a.doc.Style.Color != "#7C5CE0" || lookOf(a.doc.Style).branch(3) != ui.Hex("#7C5CE0") {
		t.Fatalf("color %q", a.doc.Style.Color)
	}
	a.setStyle("line", func(s *model.CanvasStyle) { s.Line = "thick" })
	a.setStyle("font", func(s *model.CanvasStyle) { s.Font = "Kaiti SC" })
	tt.Frame()
	snapshot(t, tt, "style-single")
	a.save()
	f, _ := model.Load(a.path)
	if got := f.Sheets[0].Style; got.Scheme != "ocean" || !got.SingleColor || got.Line != "thick" || got.Font != "Kaiti SC" {
		t.Fatalf("saved %+v", got)
	}
	if f.Sheets[1].Style != (model.CanvasStyle{}) {
		t.Fatal("the other canvas changed too")
	}
	a.undo()
	if a.doc.Style.Font != "" {
		t.Fatal("undo did not take back the font")
	}
	// Text stays readable on fills.
	if textOn(ui.Hex("#1F1F1F")) != ui.Hex("#FFFFFF") || textOn(ui.Hex("#FFE08A")) != ui.Hex("#1F2329") {
		t.Fatal("text color on fills")
	}
}

func TestNodeStyle(t *testing.T) {
	a, tt := newTestApp(t)
	clickNode(t, tt, a, "后端")
	if err := tt.Click("样式"); err != nil {
		t.Fatal(err)
	}
	if a.panelTab != tabStyle || !tt.HasText("形状") {
		t.Fatalf("tab %d", a.panelTab)
	}
	n := find(t, a, "后端")
	if err := tt.Click("胶囊"); err != nil {
		t.Fatal(err)
	}
	if n.Style == nil || n.Style.Shape != model.ShapePill {
		t.Fatalf("style %+v", n.Style)
	}
	tt.Click("B")
	tt.Click("I")
	if !n.Style.Bold || !n.Style.Italic {
		t.Fatalf("style %+v", n.Style)
	}
	tt.Click("居中")
	if n.Style.Align != "center" {
		t.Fatalf("align %q", n.Style.Align)
	}
	a.setNodeStyle(n, "fill", func(s *model.NodeStyle) { s.Fill = "#FFE08A" })
	a.setNodeStyle(n, "size", func(s *model.NodeStyle) { s.FontSize = 24 })
	tt.Frame()
	big := a.boxes.Boxes[n.ID]
	if big.H <= nodeHeight {
		t.Fatalf("24px text in a box %v high", big.H)
	}
	if err := tt.Click("固定宽度"); err != nil {
		t.Fatal(err)
	}
	if n.Style.Width <= 0 {
		t.Fatal("width not fixed")
	}
	a.setNodeStyle(n, "width", func(s *model.NodeStyle) { s.Width = 200 })
	tt.Frame()
	if w := a.boxes.Boxes[n.ID].W; w < 200 {
		t.Fatalf("box %v wide for a 200 title", w)
	}
	snapshot(t, tt, "node-style")
	a.save()
	f, _ := model.Load(a.path)
	got, _ := f.Sheets[0].Find(n.ID)
	if got.Style == nil || got.Style.Shape != model.ShapePill || got.Style.Fill != "#FFE08A" || got.Style.Width != 200 {
		t.Fatalf("saved %+v", got.Style)
	}
	if err := tt.Click("清除样式"); err != nil {
		t.Fatal(err)
	}
	if n = find(t, a, "后端"); n.Style != nil {
		t.Fatalf("style %+v after clearing", n.Style)
	}
	a.undo()
	if n = find(t, a, "后端"); n.Style == nil || n.Style.Width != 200 {
		t.Fatal("undo did not bring the style back")
	}
}

func TestMarkers(t *testing.T) {
	a, tt := newTestApp(t)
	clickNode(t, tt, a, "后端")
	if err := tt.Click("标记"); err != nil {
		t.Fatal(err)
	}
	n := find(t, a, "后端")
	w0 := a.boxes.Boxes[n.ID].W
	for _, m := range []string{"优先级 1", "红旗", "任务进度 3/6", "喜欢"} {
		if err := tt.Click(m); err != nil {
			t.Fatal(err)
		}
	}
	if !slices.Equal(n.Markers, []string{"priority:1", "task:3", "flag:red", "symbol:heart"}) {
		t.Fatalf("markers %v", n.Markers)
	}
	// One of each group: another priority replaces the first.
	tt.Click("优先级 2")
	if n.Markers[0] != "priority:2" || len(n.Markers) != 4 {
		t.Fatalf("markers %v", n.Markers)
	}
	tt.Frame()
	if w := a.boxes.Boxes[n.ID].W; w < w0+4*18 {
		t.Fatalf("box %v wide with four markers, %v without", w, w0)
	}
	if !tt.HasText("图例") {
		t.Fatal("no legend")
	}
	snapshot(t, tt, "markers")
	// Clicking the priority on the node goes to the next one.
	if err := tt.Click("优先级 2"); err != nil { // the panel's: takes it off
		t.Fatal(err)
	}
	tt.Click("优先级 2")
	r, _ := tt.Find("我的项目")
	_ = r
	if got := nextMarker("priority:2"); got != "priority:3" || nextMarker("priority:7") != "priority:1" {
		t.Fatalf("next marker %v", got)
	}
	// Clicking a marker that is on takes it off.
	tt.Click("红旗")
	if slices.Contains(n.Markers, "flag:red") {
		t.Fatal("not taken off")
	}
	a.save()
	f, _ := model.Load(a.path)
	got, _ := f.Sheets[0].Find(n.ID)
	if !slices.Equal(got.Markers, n.Markers) {
		t.Fatalf("saved %v", got.Markers)
	}
}

func TestSidebarFoldersAndFavorites(t *testing.T) {
	a, tt := newTestApp(t)
	guide := a.path
	// A folder, renamed as it is made.
	if err := tt.Click("+ 文件夹"); err != nil {
		t.Fatal(err)
	}
	if a.renaming == "" {
		t.Fatal("new folder not being renamed")
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("工作")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	work := filepath.Join(a.dir, "工作")
	if info, err := os.Stat(work); err != nil || !info.IsDir() {
		t.Fatalf("folder: %v", err)
	}
	// A map moved into it follows: the open map keeps its new path.
	a.moveMap(guide, work)
	tt.Frame()
	if a.path != filepath.Join(work, filepath.Base(guide)) {
		t.Fatalf("open map at %q", a.path)
	}
	if len(a.tree.Folders) != 1 || len(a.tree.Folders[0].Files) != 1 {
		t.Fatalf("tree %+v", a.tree)
	}
	// Starred maps show first.
	a.toggleFavorite(a.path)
	tt.Frame()
	if !tt.HasText("★ 收藏") || !slices.Contains(a.cfg.Favorites, a.path) {
		t.Fatalf("favorites %v", a.cfg.Favorites)
	}
	// Renaming keeps the favorite and the open map.
	a.rename(a.path, "指南")
	tt.Frame()
	if mapName(a.path) != "指南" || a.cfg.Favorites[0] != a.path {
		t.Fatalf("path %q, favorites %v", a.path, a.cfg.Favorites)
	}
	// A map made in a folder goes there.
	a.newMapIn(work)
	if filepath.Dir(a.path) != work {
		t.Fatalf("new map at %q", a.path)
	}
	tt.Key(0, ui.KeyEscape)
	// Many tags make a list with a filter field.
	a.mutate("", func() {
		for i := range 12 {
			n, _ := a.doc.AddChild(a.doc.Root.ID, fmt.Sprint("n", i))
			n.Tags = []string{fmt.Sprintf("标签%02d", i)}
		}
	})
	tt.Frame()
	snapshot(t, tt, "sidebar")
	if !tt.HasText("标签 17") { // the guide's 5 and these 12
		t.Fatalf("texts %q", tt.Texts())
	}
	if got := filterTags(a.file.AllTags(), "标签1"); !slices.Equal(got, []string{"标签10", "标签11"}) {
		t.Fatalf("filtered %v", got)
	}
}

func TestOutline(t *testing.T) {
	a, tt := newTestApp(t)
	if err := tt.Click("大纲"); err != nil {
		t.Fatal(err)
	}
	if a.mode != viewOutline {
		t.Fatalf("mode %d", a.mode)
	}
	arch := find(t, a, "架构")
	// Focus the 前端 line and type after it.
	front := find(t, a, "前端")
	a.outlineFocus = front.ID
	tt.Frame()
	if a.selected != front.ID {
		t.Fatalf("selected %q", a.selected)
	}
	tt.Key(0, ui.KeyEnd)
	tt.Type("界面")
	if front.Title != "前端界面" {
		t.Fatalf("title %q", front.Title)
	}
	// Enter adds a line after it, at its depth.
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if len(arch.Children) != 3 || arch.Children[1].ID != a.outlineFocus {
		t.Fatalf("children %d, focus %q", len(arch.Children), a.outlineFocus)
	}
	tt.Type("网关")
	added := arch.Children[1]
	if added.Title != "网关" {
		t.Fatalf("new line %q", added.Title)
	}
	// Tab indents it under 前端界面; Shift+Tab brings it back.
	tt.Key(0, ui.KeyTab)
	tt.Frame()
	if len(front.Children) != 1 || front.Children[0] != added {
		t.Fatalf("not indented: %d children", len(front.Children))
	}
	snapshot(t, tt, "outline")
	tt.Key(ui.Shift, ui.KeyTab)
	tt.Frame()
	if _, p := a.doc.Find(added.ID); p != arch {
		t.Fatalf("outdented under %q", p.Title)
	}
	// Up and Down move between lines.
	tt.Key(0, ui.KeyUp)
	tt.Frame()
	if a.outlineFocus != front.ID {
		t.Fatalf("focus %q after Up", a.outlineFocus)
	}
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	// Backspace on an empty line deletes it.
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Key(0, ui.KeyBackspace)
	tt.Frame()
	if added.Title != "" {
		t.Fatalf("title %q after clearing", added.Title)
	}
	tt.Key(0, ui.KeyBackspace)
	tt.Frame()
	if n, _ := a.doc.Find(added.ID); n != nil {
		t.Fatal("empty line not deleted")
	}
	if a.outlineFocus != front.ID {
		t.Fatalf("focus %q after deleting", a.outlineFocus)
	}
	// Same tree as the map.
	tt.Click("思维导图")
	if !tt.HasText("前端界面") {
		t.Fatal("map does not show the outline's edit")
	}
}

func TestPresentation(t *testing.T) {
	a, tt := newTestApp(t)
	slides := slidesOf(a.doc)
	// Title, the main topics, then each topic with subtopics, and the
	// leaves with notes.
	if slides[0].kind != slideTitle || slides[1].kind != slideList || len(slides[1].items) != 4 {
		t.Fatalf("first slides %+v", slides[:2])
	}
	var titles []string
	for _, s := range slides {
		titles = append(titles, s.node.Title)
	}
	for _, want := range []string{"使用指南", "右侧面板写备注、打标签", "架构", "前端", "功能", "待优化", "列表加载慢"} {
		if !slices.Contains(titles, want) {
			t.Fatalf("no slide for %q in %v", want, titles)
		}
	}
	if slices.Contains(titles, "支持导出图片") {
		t.Fatal("a leaf without a note got a slide")
	}

	tt.Click("演说")
	if a.panelTab != tabPresent || !tt.HasText("演说模式预览") {
		t.Fatalf("tab %d", a.panelTab)
	}
	snapshot(t, tt, "present-panel")
	plat := a.plat.(*fakePlatform)
	if err := tt.Click("▶ 从头开始演说"); err != nil {
		t.Fatal(err)
	}
	if !a.presenting || !plat.fullScreen || a.slideIndex != 0 {
		t.Fatalf("presenting %v, full screen %v, slide %d", a.presenting, plat.fullScreen, a.slideIndex)
	}
	snapshot(t, tt, "present-title")
	tt.Key(0, ui.KeyRight)
	if a.slideIndex != 1 || !tt.HasText("待优化") {
		t.Fatalf("slide %d: %q", a.slideIndex, tt.Texts())
	}
	snapshot(t, tt, "present-list")
	tt.Key(0, ui.KeyLeft)
	if a.slideIndex != 0 {
		t.Fatalf("slide %d after Left", a.slideIndex)
	}
	tt.Key(0, ui.KeyEscape)
	if a.presenting || plat.fullScreen {
		t.Fatal("Escape did not end")
	}

	// Revealed one item at a time, as cards, in the light theme.
	a.mutate("", func() { a.doc.Present = model.Present{Reveal: true, Layout: "cards", Theme: "light", Aspect: "16:9"} })
	a.startPresenting(find(t, a, "架构").ID)
	tt.Frame()
	if a.slides[a.slideIndex].node.Title != "架构" || a.slideStep != 0 {
		t.Fatalf("started at %q step %d", a.slides[a.slideIndex].node.Title, a.slideStep)
	}
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	snapshot(t, tt, "present-cards")
	if a.slideStep != 1 {
		t.Fatalf("step %d", a.slideStep)
	}
	tt.Key(0, ui.KeySpace)
	tt.Key(0, ui.KeySpace) // past both items: the next slide
	if a.slides[a.slideIndex].node.Title == "架构" || a.slideStep != 0 {
		t.Fatalf("at %q step %d", a.slides[a.slideIndex].node.Title, a.slideStep)
	}
	tt.Key(0, ui.KeyLeft) // back to 架构, every item shown
	if a.slides[a.slideIndex].node.Title != "架构" || a.slideStep != 2 {
		t.Fatalf("back at %q step %d", a.slides[a.slideIndex].node.Title, a.slideStep)
	}
}

// fakeClaude answers with replies decoded from JSON, as the API sends them,
// and keeps the requests.
type fakeClaude struct {
	replies  []string
	requests []anthropic.BetaMessageNewParams
}

func (f *fakeClaude) send(_ context.Context, _ config, p anthropic.BetaMessageNewParams) (*anthropic.BetaMessage, error) {
	f.requests = append(f.requests, p)
	if len(f.replies) == 0 {
		return nil, errors.New("no reply")
	}
	var m anthropic.BetaMessage
	err := json.Unmarshal([]byte(f.replies[0]), &m)
	f.replies = f.replies[1:]
	return &m, err
}

func TestAIProposesEdits(t *testing.T) {
	a, tt := newTestApp(t)
	opt := find(t, a, "待优化")
	slow := find(t, a, "列表加载慢")
	fake := &fakeClaude{replies: []string{fmt.Sprintf(`{
		"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-opus-5-5",
		"stop_reason": "tool_use",
		"content": [
			{"type": "text", "text": "我把性能问题拆成了两个任务。"},
			{"type": "tool_use", "id": "toolu_1", "name": "propose_edits", "input": {
				"summary": "拆分性能优化任务",
				"operations": [
					{"op": "add", "id": "", "parent": %[1]q, "ref": "r1", "title": "性能", "note": "", "tags": ["性能"], "status": "", "due": ""},
					{"op": "move", "id": %[2]q, "parent": "r1", "ref": "", "title": "", "note": "", "tags": [], "status": "", "due": ""},
					{"op": "add", "id": "", "parent": "r1", "ref": "", "title": "图片懒加载", "note": "先看首页", "tags": [], "status": "todo", "due": "2026-10-20"},
					{"op": "update", "id": %[2]q, "parent": "", "ref": "", "title": "", "note": "", "tags": [], "status": "doing", "due": ""}
				]}}
		],
		"usage": {"input_tokens": 10, "output_tokens": 10}
	}`, opt.ID, slow.ID), `{
		"id": "msg_2", "type": "message", "role": "assistant", "model": "claude-opus-5-5",
		"stop_reason": "end_turn",
		"content": [{"type": "text", "text": "好的。"}],
		"usage": {"input_tokens": 10, "output_tokens": 2}
	}`}}
	a.sendAI = fake.send
	a.cfg.AIKey = "sk-ant-test"
	if err := tt.Click("✦ AI"); err != nil {
		t.Fatal(err)
	}
	a.ai.draft = "把待优化里的性能问题拆成任务"
	a.aiSend()
	tt.Frame()

	// The request: the model, the tool, and the map with its IDs.
	req := fake.requests[0]
	if req.Model != "claude-opus-5-5" || len(req.Tools) != 1 || req.Tools[0].OfTool.Name != "propose_edits" {
		t.Fatalf("request %+v", req)
	}
	body, _ := json.Marshal(req)
	if !strings.Contains(string(body), "["+slow.ID+"] 列表加载慢") || !strings.Contains(string(body), `"fallbacks":"default"`) {
		t.Fatalf("request body %s", body)
	}
	if !tt.HasText("拆分性能优化任务") || !tt.HasText("应用") {
		t.Fatalf("no proposal: %q", tt.Texts())
	}
	snapshot(t, tt, "ai-proposal")

	// Applying makes every edit, as one step.
	if err := tt.Click("应用"); err != nil {
		t.Fatal(err)
	}
	perf := find(t, a, "性能")
	if _, p := a.doc.Find(perf.ID); p != opt {
		t.Fatal("性能 not under 待优化")
	}
	if len(perf.Children) != 2 || perf.Children[0] != slow || slow.Task.Status != model.StatusDoing {
		t.Fatalf("性能's children %d, 列表加载慢 %v", len(perf.Children), slow.Task.Status)
	}
	lazy := perf.Children[1]
	if lazy.Title != "图片懒加载" || lazy.Note != "先看首页" || lazy.Task == nil || lazy.Task.Due.Format("2006-01-02") != "2026-10-20" {
		t.Fatalf("new task %+v %+v", lazy, lazy.Task)
	}
	if !tt.HasText("✓ 已应用 4 项修改（可撤销）") {
		t.Fatalf("texts %q", tt.Texts())
	}

	// The next message answers the tool call first.
	a.ai.draft = "谢谢"
	a.aiSend()
	second := fake.requests[1]
	last := second.Messages[len(second.Messages)-1]
	if len(second.Messages) != 3 || last.Content[0].OfToolResult == nil || last.Content[0].OfToolResult.ToolUseID != "toolu_1" {
		t.Fatalf("second request %d messages, last %+v", len(second.Messages), last.Content[0])
	}

	// Undo takes back the whole proposal.
	a.undo()
	if _, err := findIn(a.doc, "性能"); err == nil {
		t.Fatal("undo left the edits")
	}
}

func TestAIEditsAllOrNone(t *testing.T) {
	a, _ := newTestApp(t)
	before, _ := a.file.Marshal()
	_, err := a.applyEdits([]aiEdit{
		{Op: "update", ID: a.doc.Root.ID, Title: "改了"},
		{Op: "delete", ID: "no-such-node"},
	})
	if err == nil {
		t.Fatal("bad edit applied")
	}
	if after, _ := a.file.Marshal(); string(after) != string(before) {
		t.Fatal("a failed proposal changed the map")
	}
	if a.hist.CanUndo() {
		t.Fatal("a failed proposal made a step")
	}
}

func TestAINeedsKey(t *testing.T) {
	a, tt := newTestApp(t)
	t.Setenv("ANTHROPIC_API_KEY", "")
	a.cfg.AIKey = ""
	tt.Click("✦ AI")
	if !tt.HasText("API Key") {
		t.Fatal("no settings without a key")
	}
	a.ai.draft = "hi"
	a.aiSend()
	if len(a.ai.claude)+len(a.ai.ds) != 0 {
		t.Fatal("sent without a key")
	}
	if !a.aiOpen {
		t.Fatal("AI not open")
	}
	tt.Frame()
	if err := tt.Click("关闭"); err != nil {
		t.Fatal(err)
	}
	if a.aiOpen || !tt.HasText("样式") {
		t.Fatalf("closing did not bring the tabs back: open %v, %q", a.aiOpen, tt.Texts())
	}
}

func TestToolbarFitsNarrowWindow(t *testing.T) {
	a, tt := newTestApp(t)
	for _, w := range []int{1280, 1100, 1000, 920} {
		tt.SetSize(w, 760)
		tt.Frame()
		redo, ok := tt.Find("↷")
		if !ok {
			t.Fatalf("width %d: no redo", w)
		}
		right := float32(w) - a.panelWidth
		if redo.X+redo.W > right {
			t.Fatalf("width %d: redo ends at %v, past the toolbar's %v", w, redo.X+redo.W, right)
		}
		if w == 920 {
			snapshot(t, tt, "toolbar-narrow")
		}
	}
}

func TestDeepSeek(t *testing.T) {
	a, tt := newTestApp(t)
	slow := find(t, a, "列表加载慢")
	var requests []map[string]any
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		auth = r.Header.Get("Authorization")
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		requests = append(requests, body)
		if len(requests) == 1 {
			args, _ := json.Marshal(map[string]any{"summary": "改为进行中", "operations": []map[string]any{
				{"op": "update", "id": slow.ID, "parent": "", "ref": "", "title": "", "note": "", "tags": []string{}, "status": "doing", "due": ""},
			}})
			fmt.Fprintf(w, `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":"好的。","reasoning_content":"用户要改状态。","tool_calls":[{"id":"call_1","type":"function","function":{"name":"propose_edits","arguments":%q}}]}}]}`, args)
			return
		}
		fmt.Fprint(w, `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"已经改好了。","reasoning_content":"无需工具。"}}]}`)
	}))
	defer srv.Close()
	a.cfg.AIProvider, a.cfg.DSKey, a.cfg.DSBaseURL = providerDeepSeek, "sk-test", srv.URL

	tt.Click("✦ AI")
	if !tt.HasText("DeepSeek V4.1 Flash（推荐）") {
		t.Fatalf("model not shown: %q", tt.Texts())
	}
	a.ai.draft = "把列表加载慢改成进行中"
	a.aiSend()
	tt.Frame()
	if auth != "Bearer sk-test" {
		t.Fatalf("auth %q", auth)
	}
	first := requests[0]
	if first["model"] != "deepseek-flash" {
		t.Fatalf("model %v", first["model"])
	}
	msgs := first["messages"].([]any)
	if msgs[0].(map[string]any)["role"] != "system" || !strings.Contains(msgs[1].(map[string]any)["content"].(string), "["+slow.ID+"] 列表加载慢") {
		t.Fatalf("messages %v", msgs)
	}
	tools := first["tools"].([]any)
	if tools[0].(map[string]any)["function"].(map[string]any)["name"] != "propose_edits" {
		t.Fatalf("tools %v", tools)
	}
	if !tt.HasText("改为进行中") {
		t.Fatalf("no proposal: %q", tt.Texts())
	}
	snapshot(t, tt, "deepseek")
	if err := tt.Click("应用"); err != nil {
		t.Fatal(err)
	}
	if slow = find(t, a, "列表加载慢"); slow.Task.Status != model.StatusDoing {
		t.Fatalf("status %v", slow.Task.Status)
	}

	// The next request sends back the reasoning of the turn with the tool
	// call, then the tool's result, then the message.
	a.ai.draft = "谢谢"
	a.aiSend()
	tt.Frame()
	msgs = requests[1]["messages"].([]any)
	assistant := msgs[2].(map[string]any)
	tool := msgs[3].(map[string]any)
	if assistant["reasoning_content"] != "用户要改状态。" || assistant["tool_calls"] == nil {
		t.Fatalf("assistant %v", assistant)
	}
	if tool["role"] != "tool" || tool["tool_call_id"] != "call_1" || !strings.Contains(tool["content"].(string), "已应用") {
		t.Fatalf("tool %v", tool)
	}
	if !tt.HasText("已经改好了。") {
		t.Fatal("no answer")
	}
	// A turn without tools keeps no reasoning.
	if last := a.ai.ds[len(a.ai.ds)-1]; last.ReasoningContent != "" {
		t.Fatalf("kept reasoning %q", last.ReasoningContent)
	}
}

func TestDeepSeekErrors(t *testing.T) {
	a, tt := newTestApp(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(402)
		fmt.Fprint(w, `{"error":{"message":"Insufficient Balance"}}`)
	}))
	defer srv.Close()
	a.cfg.AIProvider, a.cfg.DSKey, a.cfg.DSBaseURL = providerDeepSeek, "sk-test", srv.URL
	tt.Click("✦ AI")
	a.ai.draft = "你好"
	a.aiSend()
	tt.Frame()
	if !tt.HasText("DeepSeek 账户余额不足。") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if len(a.ai.ds) != 0 {
		t.Fatal("a failed turn stayed in the conversation")
	}
}

func TestEditingKeepsNodeWidth(t *testing.T) {
	a, tt := newTestApp(t)
	n := find(t, a, "前端")
	clickNode(t, tt, a, "前端")
	before := a.boxes.Boxes[n.ID]
	clickNode(t, tt, a, "前端") // the second click of a double click edits
	if a.editing != n.ID {
		t.Fatal("not editing")
	}
	tt.Frame()
	during := a.boxes.Boxes[n.ID]
	if d := during.W - before.W; d < 0 || d > 6 {
		t.Fatalf("width %v before editing, %v while editing", before.W, during.W)
	}
	tt.Type("和移动端界面")
	tt.Frame()
	if a.boxes.Boxes[n.ID].W <= during.W {
		t.Fatal("does not grow as the title does")
	}
	// A new node is as wide as its placeholder, not wider.
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeyTab)
	tt.Frame()
	added := a.boxes.Boxes[a.editing]
	if added.W > 2*nodePadX+80 {
		t.Fatalf("new node %v wide", added.W)
	}
}

// testPNG is a w×h picture, half red and half blue.
func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			c := color.RGBA{220, 60, 60, 255}
			if x > w/2 {
				c = color.RGBA{60, 90, 220, 255}
			}
			img.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestPasteImage(t *testing.T) {
	a, tt := newTestApp(t)
	plat := a.plat.(*fakePlatform)
	plat.clipImage = testPNG(t, 400, 300)
	n := find(t, a, "前端")
	clickNode(t, tt, a, "前端")
	before := a.boxes.Boxes[n.ID]
	tt.Key(ui.Cmd, ui.KeyV)
	tt.Frame()
	if n.Image == nil || n.Image.W != 400 || n.Image.H != 300 {
		t.Fatalf("image %+v", n.Image)
	}
	file := filepath.Join(model.AssetsDir(a.path), n.Image.File)
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("no file: %v", err)
	}
	after := a.boxes.Boxes[n.ID]
	if after.H < before.H+150 || after.W < 200 {
		t.Fatalf("box %+v with a picture, %+v without", after, before)
	}
	snapshot(t, tt, "image-node")

	// A click on the picture shows it large; Escape closes it.
	if err := tt.Click("图片：前端"); err != nil {
		t.Fatal(err)
	}
	if a.lightbox != n.ID {
		t.Fatal("no lightbox")
	}
	snapshot(t, tt, "image-lightbox")
	tt.Key(0, ui.KeyEscape)
	if a.lightbox != "" {
		t.Fatal("Escape did not close it")
	}

	// Saved with the map; undo takes it off.
	a.save()
	f, _ := model.Load(a.path)
	saved, _ := f.Sheets[0].Find(n.ID)
	if saved.Image == nil || saved.Image.File != n.Image.File {
		t.Fatalf("saved %+v", saved.Image)
	}
	a.undo()
	if find(t, a, "前端").Image != nil {
		t.Fatal("undo kept the picture")
	}
	a.redo()

	// Renaming the map takes its pictures along; its folder is no folder
	// of maps.
	a.rename(a.path, "带图片的导图")
	tt.Frame()
	if _, err := os.Stat(filepath.Join(model.AssetsDir(a.path), n.Image.File)); err != nil {
		t.Fatalf("pictures left behind: %v", err)
	}
	if len(a.tree.Folders) != 0 {
		t.Fatalf("assets folder listed: %+v", a.tree.Folders)
	}
	if a.bitmap(find(t, a, "前端").Image) == nil {
		t.Fatal("picture not found after renaming")
	}
}

func TestPickImage(t *testing.T) {
	a, tt := newTestApp(t)
	path := filepath.Join(t.TempDir(), "照片.png")
	os.WriteFile(path, testPNG(t, 120, 200), 0o644)
	a.plat.(*fakePlatform).picked = path
	clickNode(t, tt, a, "后端")
	if err := tt.Click("添加图片…"); err != nil {
		t.Fatal(err)
	}
	n := find(t, a, "后端")
	if n.Image == nil || n.Image.H != 200 {
		t.Fatalf("image %+v", n.Image)
	}
	if err := tt.Click("删除图片"); err != nil {
		t.Fatal(err)
	}
	if n.Image != nil {
		t.Fatal("not removed")
	}
	if err := a.attachImageFile(n, filepath.Join(t.TempDir(), "notes.txt")); err == nil {
		t.Fatal("took a text file")
	}
}

func TestSwitchingMapsKeepsTheirState(t *testing.T) {
	a, tt := newTestApp(t)
	guide := a.path
	// Leave the guide on its second canvas, in the outline, zoomed, with a
	// node selected and an edit to undo.
	if err := tt.Click("迭代计划"); err != nil {
		t.Fatal(err)
	}
	plan := a.doc.ID
	clickNode(t, tt, a, "发布 0.2")
	ship := a.selected
	a.setZoom(1.5)
	zx := a.cam().X
	a.mutate("", func() { a.doc.Root.Title = "改过的版本" })
	tt.Frame()

	// Another map, then back.
	a.newMap()
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.doc.ID == plan || a.hist.CanUndo() && a.doc.Root.Title == "改过的版本" {
		t.Fatal("the new map shows the guide's state")
	}
	if err := a.open(guide); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.doc.ID != plan || a.selected != ship {
		t.Fatalf("back on %q, selected %q", a.doc.Title, a.selected)
	}
	if c := a.cam(); c.Zoom != 1.5 || c.X != zx {
		t.Fatalf("camera %+v", *c)
	}
	if !a.hist.CanUndo() {
		t.Fatal("history lost")
	}
	a.undo()
	if a.doc.Root.Title != "下一个版本" {
		t.Fatalf("undo gave %q", a.doc.Root.Title)
	}

	// Leave it in the outline; a new run of the app opens it so.
	a.mode = viewOutline
	a.flush()
	b := newApp(&fakePlatform{}, a.cfg)
	if b.path != guide || b.doc.ID != plan || b.mode != viewOutline || b.selected != ship {
		t.Fatalf("new run: %q on %q, mode %d, selected %q", b.path, b.doc.Title, b.mode, b.selected)
	}
	if c := b.cam(); c.Zoom != 1.5 {
		t.Fatalf("new run camera %+v", *c)
	}

	// Renaming the map keeps its state.
	b.rename(b.path, "指南改名")
	b.newMap()
	if err := b.open(filepath.Join(b.dir, model.FileName("指南改名"))); err != nil {
		t.Fatal(err)
	}
	if b.doc.ID != plan {
		t.Fatalf("after renaming, on %q", b.doc.Title)
	}
}

func TestGlobalTags(t *testing.T) {
	a, tt := newTestApp(t)
	// The guide's tags join the library, shared by all maps.
	if a.libraryIndex("核心") < 0 {
		t.Fatalf("library %v", a.cfg.Tags)
	}
	// A tag made in the sidebar is in the library before any node has it.
	if err := tt.Click("＋ 新建标签"); err != nil {
		t.Fatal(err)
	}
	tt.Type("待评审")
	tt.Key(0, ui.KeyEnter)
	if a.libraryIndex("待评审") < 0 {
		t.Fatalf("library %v", a.cfg.Tags)
	}
	tt.Key(0, ui.KeyEscape)

	// The node panel puts it on with a click, and takes it off.
	if err := clickNode(t, tt, a, "支持导出图片"); err != nil {
		t.Fatal(err)
	}
	n, _ := a.selectedNode()
	if err := tt.Click("标签 待评审"); err != nil {
		t.Fatal(err)
	}
	if !n.HasTag("待评审") {
		t.Fatalf("tags %v", n.Tags)
	}
	snapshot(t, tt, "tags")
	tt.Click("标签 待评审")
	if n.HasTag("待评审") {
		t.Fatalf("tags %v", n.Tags)
	}
	// So does the node's context menu.
	tt.Frame()
	cam := a.cam()
	b := a.boxes.Boxes[n.ID]
	tt.RightClickAt(a.vp.X+cam.X+(b.X+b.W/2)*cam.Zoom, a.vp.Y+cam.Y+(b.Y+b.H/2)*cam.Zoom)
	if err := tt.ChooseMenuItem("标签", "待评审"); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	tt.Frame()
	if !n.HasTag("待评审") {
		t.Fatalf("tags %v", n.Tags)
	}

	// Another map with the tag, then a rename and a color in the sidebar.
	other := filepath.Join(a.dir, model.FileName("其他"))
	f := model.NewFile("其他")
	f.Sheets[0].Root.Tags = []string{"待评审"}
	if err := f.Save(other); err != nil {
		t.Fatal(err)
	}
	a.refreshFiles()
	a.setTagColor("待评审", "#dc2626")
	if tagColor("待评审") != ui.Hex("#dc2626") {
		t.Fatal("color not used")
	}
	a.renameTag("待评审", "评审中")
	if !n.HasTag("评审中") || n.HasTag("待评审") || a.libraryIndex("评审中") < 0 || a.libraryIndex("待评审") >= 0 {
		t.Fatalf("tags %v library %v", n.Tags, a.cfg.Tags)
	}
	if tagColor("评审中") != ui.Hex("#dc2626") {
		t.Fatal("color lost in rename")
	}
	g, _ := model.Load(other)
	if !slices.Equal(g.Sheets[0].Root.Tags, []string{"评审中"}) {
		t.Fatalf("other map %v", g.Sheets[0].Root.Tags)
	}
	// A rename is undone in the open map.
	a.undo()
	if n = find(t, a, "支持导出图片"); !n.HasTag("待评审") {
		t.Fatalf("after undo %v", n.Tags)
	}
	a.redo()
	n = find(t, a, "支持导出图片")

	// Deleting takes it off every map.
	a.deleteTag("评审中")
	g, _ = model.Load(other)
	if n.HasTag("评审中") || len(g.Sheets[0].Root.Tags) != 0 || a.libraryIndex("评审中") >= 0 {
		t.Fatalf("after delete %v %v", n.Tags, g.Sheets[0].Root.Tags)
	}
	// The library is kept with the settings.
	if !slices.ContainsFunc(loadConfig().Tags, func(d tagDef) bool { return d.Name == "核心" }) {
		t.Fatal("library not saved")
	}
}
