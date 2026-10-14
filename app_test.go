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
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/versions"
)

type fakePlatform struct {
	folder      string
	trashed     []string
	fullScreen  bool
	clipImage   []byte
	clipFiles   []string
	picked      string
	savePath    string
	openPath    string
	copied      string
	opened      []string
	notes       []string
	capture     bool
	captureKey  string
	trayUpdates int
	shots       int
	icon, tray  []byte
	checks      int
	noAuto      bool
}

func (f *fakePlatform) SaveFile(name, ext string, done func(string)) { done(f.savePath) }
func (f *fakePlatform) OpenFile(title string, exts []string, done func(string)) {
	done(f.openPath)
}
func (f *fakePlatform) CopyText(s string)              { f.copied = s }
func (f *fakePlatform) ClipboardText() string          { return f.copied }
func (f *fakePlatform) OpenURL(url string)             { f.opened = append(f.opened, url) }
func (f *fakePlatform) OpenPath(path string)           { f.opened = append(f.opened, "path:"+path) }
func (f *fakePlatform) ShowCapture()                   { f.capture = true }
func (f *fakePlatform) HideCapture()                   { f.capture = false }
func (f *fakePlatform) ShowMain()                      {}
func (f *fakePlatform) SetCaptureKey(key string) error { f.captureKey = key; return nil }
func (f *fakePlatform) SetTray(on bool)                {}
func (f *fakePlatform) SetClipKey(key string) error    { return nil }
func (f *fakePlatform) SetShotKey(key string) error    { return nil }
func (f *fakePlatform) Screenshot(fromApp bool)        { f.shots++ }
func (f *fakePlatform) OpenShotSettings()              { f.opened = append(f.opened, "shot-settings") }
func (f *fakePlatform) ShotLog() string                { return "2026-10-11 15:04:05 截图开始" }
func (f *fakePlatform) SetAppIcon(png, tray []byte)    { f.icon, f.tray = png, tray }
func (f *fakePlatform) CheckUpdates()                  { f.checks++ }
func (f *fakePlatform) AutoUpdates() bool              { return !f.noAuto }
func (f *fakePlatform) SetAutoUpdates(on bool)         { f.noAuto = !on }
func (f *fakePlatform) UpdateTray()                    { f.trayUpdates++ }
func (f *fakePlatform) Notify(title, body string) {
	f.notes = append(f.notes, title+"："+body)
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
	clock = func() time.Time { return testNow }
	a := newApp(&fakePlatform{}, config{Dir: dir})
	a.noLocalModel = true // the built-in model only where a test asks for it
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
	return clickNodeMods(t, tt, a, 0, title)
}

func clickNodeMods(t *testing.T, tt *ui.Tester, a *app, mods ui.Modifiers, title string) error {
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
	tt.ClickAtWith(mods, a.vp.X+cam.X+(b.X+b.W/2)*cam.Zoom, a.vp.Y+cam.Y+(b.Y+b.H/2)*cam.Zoom)
	return nil
}

// clickInPanel clicks an element of the right panel, scrolling it into the
// window first.
func clickInPanel(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	for range 10 {
		r, ok := tt.Find(label)
		if !ok {
			t.Fatalf("no %q", label)
		}
		if r.Y+r.H < 780 {
			break
		}
		tt.Scroll(1100, 400, 0, 200)
	}
	if err := tt.Click(label); err != nil {
		t.Fatal(err)
	}
}

// clickNodeWith clicks a node holding modifier keys.
func clickNodeWith(t *testing.T, tt *ui.Tester, a *app, mods ui.Modifiers, title string) {
	t.Helper()
	tt.HoldModifiers(mods)
	defer tt.HoldModifiers(0)
	if err := clickNodeMods(t, tt, a, mods, title); err != nil {
		t.Fatal(err)
	}
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
	snapshot(t, tt, "edit-new")
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
	if err := tt.Click("备注内容"); err != nil {
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
	clickInPanel(t, tt, "进行中")
	if n.Task.Status != model.StatusDoing {
		t.Fatalf("status %v", n.Task.Status)
	}
	clickInPanel(t, tt, "截止日期")
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
	a.panelTab = tabCanvas // the map's tags are there
	tt.Frame()
	if err := tt.Click("筛选标签 核心"); err != nil {
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
	tt.Click("新建")
	if err := tt.ChooseMenuItem("新建导图"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
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
	// Onto its middle: near its top or bottom edge, it would go before or
	// after it.
	x, y := to.X+5, to.Y+to.H/2
	tt.Press(from.X+5, from.Y+5)
	for i := 1; i <= 10; i++ {
		f := float32(i) / 10
		tt.Move(from.X+5+(x-from.X-5)*f, from.Y+5+(y-from.Y-5)*f)
	}
	tt.Release(x, y)
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
	clickInPanel(t, tt, "海洋")
	if a.doc.Style.Scheme != "ocean" {
		t.Fatalf("scheme %q", a.doc.Style.Scheme)
	}
	clickInPanel(t, tt, "彩虹分支")
	if !a.doc.Style.SingleColor {
		t.Fatal("rainbow still on")
	}
	clickInPanel(t, tt, "#7C5CE0")
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
	// Clicking the node's marker goes on to the next one.
	a.cam().X -= 150 // the node in view
	tt.Frame()
	tt.Click("红旗")
	if slices.Contains(n.Markers, "flag:red") {
		t.Fatalf("not taken off: %v", n.Markers)
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
	tt.Click("新建")
	if err := tt.ChooseMenuItem("新建文件夹"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
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
	if !tt.HasText("快捷访问") || !slices.Contains(a.cfg.Favorites, a.path) {
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
	a.panelTab = tabCanvas // the map's tags are there
	tt.Frame()
	snapshot(t, tt, "sidebar")
	if !tt.HasText("本导图标签 17") { // the guide's 5 and these 12
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
	a.mode = viewMap
	tt.Frame()
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

func (f *fakeClaude) send(_ context.Context, _ config, p anthropic.BetaMessageNewParams, onText func(string)) (*anthropic.BetaMessage, error) {
	f.requests = append(f.requests, p)
	if len(f.replies) == 0 {
		return nil, errors.New("no reply")
	}
	var m anthropic.BetaMessage
	err := json.Unmarshal([]byte(f.replies[0]), &m)
	f.replies = f.replies[1:]
	if onText != nil && err == nil { // as streamed
		for _, b := range m.Content {
			if b.Type == "text" {
				onText(b.Text)
			}
		}
	}
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
	if err := tt.Click("AI 助手"); err != nil {
		t.Fatal(err)
	}
	a.ai.draft = "把待优化里的性能问题拆成任务"
	a.aiSend()
	tt.Frame()

	// The request: the model, the tool, and the map with its IDs.
	req := fake.requests[0]
	if req.Model != "claude-opus-5-5" || len(req.Tools) != 6 || req.Tools[0].OfTool.Name != "propose_edits" {
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
	}, nil)
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
	tt.Click("AI 助手")
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
	if err := tt.Click("关闭 AI 助手"); err != nil {
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
		redo, ok := tt.Find("重做")
		if !ok {
			t.Fatalf("width %d: no redo", w)
		}
		right := float32(w) // the panel hides in a narrow window
		if float32(w)-a.sidebarWidth-a.panelWidth >= panelMinCanvas {
			right -= a.panelWidth
		}
		if redo.X+redo.W > right {
			t.Fatalf("width %d: redo ends at %v, past the toolbar's %v", w, redo.X+redo.W, right)
		}
		// With room to spare between the views and the search, for fonts
		// elsewhere, as on CI, are a little wider.
		if last, ok := tt.Find("表格"); ok && a.mode != viewTable {
			search, _ := tt.Find("搜索")
			if spare := search.X - 22 - (last.X + last.W); spare < 10 {
				t.Fatalf("width %d: %v spare between the views and the search", w, spare)
			}
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

	tt.Click("AI 助手")
	if !tt.HasText("DeepSeek V4.1 Flash") {
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
	tt.Click("AI 助手")
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
	// A tag made in the map's tags is in the library before any node has it.
	a.panelTab = tabCanvas
	tt.Frame()
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
	a.panelTab = tabNode
	if err := clickNode(t, tt, a, "支持导出图片"); err != nil {
		t.Fatal(err)
	}
	n, _ := a.selectedNode()
	clickInPanel(t, tt, "标签 待评审")
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

func TestLongTitleWrapsWhileEditing(t *testing.T) {
	a, tt := newTestApp(t)
	if err := clickNode(t, tt, a, "架构"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyTab)
	tt.Type("短")
	tt.Frame()
	id := a.editing
	short := a.boxes.Boxes[id]
	long := strings.Repeat("一段很长的节点标题", 8)
	tt.Type(long)
	tt.Frame()
	tt.Frame()
	b := a.boxes.Boxes[id]
	// It stops widening at the title's wrap width and grows in height
	// instead, so the start of the title stays in view.
	if b.W > 2*nodePadX+titleMax+4+1 || b.H <= short.H {
		t.Fatalf("box while editing %v, short %v", b, short)
	}
	snapshot(t, tt, "edit-wrap")
	// Shift+Enter does not add a line: Enter ends the edit either way.
	tt.Key(ui.Shift, ui.KeyEnter)
	if a.editing != "" {
		t.Fatal("still editing")
	}
	n, _ := a.doc.Find(id)
	if n.Title != "短"+long {
		t.Fatalf("title %q", n.Title)
	}
}

func TestTaskSummary(t *testing.T) {
	a, _ := newTestApp(t)
	due := testNow.AddDate(0, 0, -1)
	got := a.taskSummary(&model.Task{Status: model.StatusDoing, Priority: model.PriorityHigh, Due: &due})
	want := "任务：进行中 · 优先级 高 · 截止 " + due.Format("1/2") + "（已逾期）"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if got := a.taskSummary(&model.Task{Status: model.StatusIdea}); got != "任务：想法" {
		t.Fatalf("got %q", got)
	}
}

func TestExportImage(t *testing.T) {
	a, tt := newTestApp(t)
	clickNode(t, tt, a, "架构")
	cam := *a.cam()
	selected := a.selected
	out := filepath.Join(t.TempDir(), "导出")
	a.plat.(*fakePlatform).savePath = out
	a.askExportImage()
	data, err := os.ReadFile(out + ".png")
	if err != nil {
		t.Fatal(err, a.saveErr)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	w, h := a.boxes.Width, a.boxes.Height
	if b := img.Bounds(); abs(float32(b.Dx())-w*exportScale) > 4 || abs(float32(b.Dy())-h*exportScale) > 4 {
		t.Fatalf("image %v for a map of %v×%v", b, w, h)
	}
	// The screen is as it was.
	if *a.cam() != cam || a.selected != selected || a.exportCam != nil {
		t.Fatalf("camera %v selected %q", *a.cam(), a.selected)
	}
	if dir := os.Getenv("MINDMAP_SNAPSHOTS"); dir != "" {
		os.WriteFile(filepath.Join(dir, "export.png"), data, 0o644)
	}
	// The canvas still takes keys.
	tt.Key(0, ui.KeyTab)
	if a.editing == "" {
		t.Fatal("canvas lost its handle")
	}
}

func abs(x float32) float32 { return max(x, -x) }

func TestResizePanel(t *testing.T) {
	a, tt := newTestApp(t)
	r, ok := tt.Find("调整面板宽度")
	if !ok {
		t.Fatal("no resizer")
	}
	x, y := r.X+r.W/2, r.Y+r.H/2
	tt.Press(x, y)
	tt.Move(x-60, y)
	tt.Move(x-120, y)
	tt.Release(x-120, y)
	if a.panelWidth < 410 || a.panelWidth > 430 {
		t.Fatalf("panel %v", a.panelWidth)
	}
	if loadConfig().PanelWidth != a.panelWidth {
		t.Fatal("width not kept")
	}
	// It stops at its narrowest.
	r, _ = tt.Find("调整面板宽度")
	x = r.X + r.W/2
	tt.Press(x, y)
	tt.Move(x+500, y)
	tt.Release(x+500, y)
	if a.panelWidth != panelMin {
		t.Fatalf("panel %v", a.panelWidth)
	}
}

func TestMarkdownInChat(t *testing.T) {
	src := "**进展**\n\n- 画布「ergora」有 **会话** 和 `特性`\n  - 子项\n1. 第一\n2. 第二\n\n> 引用\n\n```\ncode()\n```\n\n| 列 | 值 |\n|---|---|\n| a | 1 |\n\n---\n## 小标题\n普通 *斜体* ~~删除~~ [链接](https://x.y)"
	var kinds []string
	for _, b := range parseMarkdown(src) {
		kinds = append(kinds, b.kind)
	}
	want := []string{"p", "li", "li", "ol", "ol", "quote", "code", "table", "hr", "h", "p"}
	if !slices.Equal(kinds, want) {
		t.Fatalf("blocks %v", kinds)
	}
	spans := inlineSpans("a **b** `c` [d](u) *e*", 13, ui.Color{}, ui.Color{}, ui.Color{}, mdSpanStyle{})
	var texts []string
	for _, s := range spans {
		texts = append(texts, s.Text)
	}
	if !slices.Equal(texts, []string{"a ", "b", " ", "c", " ", "d", " ", "e"}) || spans[1].Weight != 700 || !spans[7].Italic {
		t.Fatalf("spans %q", texts)
	}

	a, tt := newTestApp(t)
	a.aiOpen, a.cfg.AIKey = true, "sk-test"
	a.panelWidth = 380
	a.ai.msgs = append(a.ai.msgs, chatMsg{role: "user", text: "总结这个画布的进展和风险"}, chatMsg{role: "assistant", text: src})
	tt.Frame()
	if tt.HasText("**进展**") || !tt.HasText("进展") {
		t.Fatalf("texts %q", tt.Texts())
	}
	snapshot(t, tt, "ai-markdown")
}

func TestMarkdownAndXMindFromTheApp(t *testing.T) {
	a, tt := newTestApp(t)
	fp := a.plat.(*fakePlatform)
	// Copy the canvas shown as Markdown.
	a.copyMarkdown()
	if !strings.HasPrefix(fp.copied, "# 我的项目\n") || !strings.Contains(fp.copied, "- 功能") {
		t.Fatalf("copied %q", fp.copied)
	}
	tt.Frame()
	if !tt.HasText("已复制为 Markdown，可以粘贴给 AI 或文档") {
		t.Fatal("no notice")
	}
	// Export as Markdown and XMind.
	out := t.TempDir()
	fp.savePath = filepath.Join(out, "导出")
	a.askExportMarkdown()
	md, err := os.ReadFile(fp.savePath + ".md")
	if err != nil || !strings.Contains(string(md), "---") { // both canvases
		t.Fatalf("%v %q", err, md)
	}
	fp.savePath = filepath.Join(out, "导出.xmind")
	a.askExportXMind()
	// Import it back: a new map named after it, opened.
	fp.openPath = fp.savePath
	before := len(a.files)
	a.askImportXMind()
	if len(a.files) != before+1 || mapName(a.path) != "导出" {
		t.Fatalf("files %d→%d, open %q (%s)", before, len(a.files), a.path, a.saveErr)
	}
	if len(a.file.Sheets) != 2 || a.doc.Root.Title != "我的项目" || len(a.allTags()) == 0 {
		t.Fatalf("imported %+v", a.doc.Root)
	}
	// A second import does not overwrite the first.
	a.askImportXMind()
	if mapName(a.path) != "导出 2" {
		t.Fatalf("open %q", a.path)
	}
	// A file that is not one reports it.
	bad := filepath.Join(out, "坏.xmind")
	os.WriteFile(bad, []byte("x"), 0o644)
	fp.openPath = bad
	a.askImportXMind()
	if !strings.Contains(a.saveErr, "不是 XMind 文件") {
		t.Fatalf("error %q", a.saveErr)
	}
}

func TestFocusBranch(t *testing.T) {
	a, tt := newTestApp(t)
	clickNode(t, tt, a, "架构")
	tt.Key(ui.Alt, ui.KeyEnter)
	if a.focusNode() == nil || a.focusNode().Title != "架构" {
		t.Fatal("not focused")
	}
	tt.Frame()
	if _, ok := a.boxes.Boxes[find(t, a, "功能").ID]; ok {
		t.Fatal("other branches still shown")
	}
	if !tt.HasText("聚焦") || !tt.HasText("我的项目") {
		t.Fatalf("no focus bar: %q", tt.Texts())
	}
	snapshot(t, tt, "focus")
	// Left from the middle goes to a child, not out of the branch.
	tt.Key(0, ui.KeyLeft)
	tt.Key(0, ui.KeyLeft)
	if n, _ := a.selectedNode(); !find(t, a, "架构").Contains(n.ID) {
		t.Fatalf("left the branch: %q", n.Title)
	}
	// The crumb of the root shows the whole map again; so does Esc.
	if err := tt.Click("我的项目"); err != nil {
		t.Fatal(err)
	}
	if a.focusNode() != nil {
		t.Fatal("still focused")
	}
	a.focusOn(find(t, a, "功能").ID)
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	if a.focusNode() != nil {
		t.Fatal("Esc did not leave")
	}
}

func TestMultiSelect(t *testing.T) {
	a, tt := newTestApp(t)
	clickNode(t, tt, a, "前端")
	clickNodeWith(t, tt, a, ui.Shift, "后端")
	clickNodeWith(t, tt, a, ui.Cmd, "登录")
	if got := len(a.selection()); got != 3 {
		t.Fatalf("selected %d", got)
	}
	tt.Frame()
	if !tt.HasText("已选 3 个节点") {
		t.Fatalf("no batch panel: %q", tt.Texts())
	}
	snapshot(t, tt, "multi")
	if err := tt.Click("进行中"); err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"前端", "后端", "登录"} {
		if n := find(t, a, title); n.Task == nil || n.Task.Status != model.StatusDoing {
			t.Fatalf("%s: %+v", title, n.Task)
		}
	}
	if err := tt.Click("标签 核心"); err != nil {
		t.Fatal(err)
	}
	if !find(t, a, "前端").HasTag("核心") {
		t.Fatal("tag not put on all")
	}
	// A click with Shift again takes one out; a plain click selects one.
	clickNodeWith(t, tt, a, ui.Shift, "后端")
	if len(a.selection()) != 2 {
		t.Fatalf("selected %d", len(a.selection()))
	}
	// Deleting deletes them all, in one step.
	tt.Key(0, ui.KeyDelete)
	if _, err := findIn(a.doc, "前端"); err == nil {
		t.Fatal("not deleted")
	}
	if _, err := findIn(a.doc, "登录"); err == nil {
		t.Fatal("not deleted")
	}
	a.undo()
	find(t, a, "前端")
	find(t, a, "登录")
	clickNode(t, tt, a, "后端")
	if len(a.multi) != 0 {
		t.Fatal("plain click kept the others")
	}
}

func TestCopyPasteBranches(t *testing.T) {
	a, tt := newTestApp(t)
	fp := a.plat.(*fakePlatform)
	clickNode(t, tt, a, "架构")
	tt.Key(ui.Cmd, ui.KeyC)
	if a.clip == nil || !strings.Contains(fp.copied, "- 架构") || !strings.Contains(fp.copied, "  - 前端") {
		t.Fatalf("copied %q", fp.copied)
	}
	// Into another map.
	first := a.path
	a.newMap()
	a.commitEdit()
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyV)
	root := a.doc.Root
	if len(root.Children) != 1 || root.Children[0].Title != "架构" || len(root.Children[0].Children) != 2 {
		t.Fatalf("pasted %+v", root.Children)
	}
	// Text from elsewhere pastes as an outline.
	fp.copied = "调研\n  竞品\n  - [ ] 用户访谈"
	fp.copied = strings.ReplaceAll(fp.copied, "\\n", "\n")
	a.selected = root.ID
	tt.Key(ui.Cmd, ui.KeyV)
	if len(root.Children) != 2 || root.Children[1].Title != "调研" || len(root.Children[1].Children) != 2 ||
		root.Children[1].Children[1].Task == nil {
		t.Fatalf("outline pasted %+v", root.Children)
	}
	// Cut moves.
	_ = a.open(first)
	clickNode(t, tt, a, "前端")
	tt.Key(ui.Cmd, ui.KeyX)
	if _, err := findIn(a.doc, "前端"); err == nil {
		t.Fatal("not cut")
	}
	clickNode(t, tt, a, "功能")
	tt.Key(ui.Cmd, ui.KeyV)
	if find(t, a, "前端") == nil || !find(t, a, "功能").Contains(find(t, a, "前端").ID) {
		t.Fatal("not pasted")
	}
}

func TestExpandLevelsAndProgress(t *testing.T) {
	a, tt := newTestApp(t)
	clickNode(t, tt, a, "我的项目")
	tt.Key(ui.Alt, ui.Key1)
	if !find(t, a, "功能").Collapsed || !find(t, a, "架构").Collapsed {
		t.Fatal("not folded to level 1")
	}
	tt.Key(ui.Alt, ui.Key0)
	if find(t, a, "功能").Collapsed {
		t.Fatal("not opened")
	}
	tt.Frame()
	pg := a.progress[find(t, a, "功能").ID]
	if pg[1] == 0 {
		t.Fatalf("no progress for 功能: %v", a.progress)
	}
	if !tt.HasText(progressText(pg)) {
		t.Fatalf("progress not shown: %q", tt.Texts())
	}
	snapshot(t, tt, "progress")
}

func TestPaletteAndSearch(t *testing.T) {
	a, tt := newTestApp(t)
	// Another map to find things in.
	other := filepath.Join(a.dir, model.FileName("后台"))
	f := model.NewFile("后台系统")
	n, _ := f.Sheets[0].AddChild(f.Sheets[0].Root.ID, "权限管理")
	n.Note = "基于角色的访问控制"
	f.Save(other)
	a.refreshFiles()

	a.openPalette(paletteAll)
	tt.Frame()
	tt.Type("看板")
	tt.Frame()
	snapshot(t, tt, "palette")
	tt.Key(0, ui.KeyEnter)
	if a.mode != viewBoard || a.paletteOpen {
		t.Fatalf("mode %d open %v", a.mode, a.paletteOpen)
	}
	// Maps.
	a.openPalette(paletteMaps)
	tt.Frame()
	tt.Type("后台")
	tt.Key(0, ui.KeyEnter)
	if a.path != other {
		t.Fatalf("open %q", a.path)
	}
	// Every map: a note's words find its node, in the other map.
	_ = a.open(filepath.Join(a.dir, model.FileName("使用指南")))
	a.openPalette(paletteSearch)
	tt.Frame()
	tt.Type("访问控制")
	tt.Frame()
	if !tt.HasText("权限管理　— 基于角色的访问控制") {
		t.Fatalf("texts %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)
	if a.path != other || a.selected != n.ID || a.mode != viewMap {
		t.Fatalf("at %q %q", a.path, a.selected)
	}
	// Recent maps come first.
	if a.recentFiles()[0].Path != other {
		t.Fatal("recent order")
	}
	// The cheat sheet.
	a.cheatsOpen = true
	tt.Frame()
	if !tt.HasText("命令面板") || !tt.HasText("聚焦所选分支") {
		t.Fatal("no cheat sheet")
	}
	snapshot(t, tt, "cheats")
}

func TestNotePreview(t *testing.T) {
	a, tt := newTestApp(t)
	clickNode(t, tt, a, "支持导出图片")
	n, _ := a.selectedNode()
	a.mutate("", func() { n.Note = "**粗体** 和 `代码`\n- 一\n- 二" })
	a.mutate("", func() { n.Note = strings.ReplaceAll(n.Note, "\\n", "\n") })
	tt.Frame()
	if err := tt.Click("预览"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("粗体") || tt.HasText("**粗体** 和 `代码`") {
		t.Fatalf("texts %q", tt.Texts())
	}
	snapshot(t, tt, "note-preview")
}

// changeOnDisk changes the open map's file as another program would.
func changeOnDisk(t *testing.T, a *app, fn func(f *model.File), after time.Duration) {
	t.Helper()
	f, err := model.Load(a.path)
	if err != nil {
		t.Fatal(err)
	}
	fn(f)
	if err := f.Save(a.path); err != nil {
		t.Fatal(err)
	}
	when := testNow.Add(after)
	os.Chtimes(a.path, when, when)
	a.now = func() time.Time { return when }
	a.checked = time.Time{}
}

// A map an AI changed through MCP, which kept the versions before and
// after, isn't kept twice when the app loads it.
func TestReloadAfterAI(t *testing.T) {
	a, tt := newTestApp(t)
	os.RemoveAll(a.versions.Dir)
	a.lastSnap = nil
	a.keepVersion(versions.Open)
	changeOnDisk(t, a, func(f *model.File) {
		a.versions.Save(a.path, f.Clone(), versions.BeforeAI, "", testNow.Add(time.Second))
		d := f.Sheets[0]
		n, _ := d.AddChild(d.Root.ID, "AI 加的")
		n.Links = []string{"src/ai.go"}
		a.versions.Save(a.path, f, versions.AI, "加模块", testNow.Add(2*time.Second))
	}, time.Minute)
	tt.Frame()
	find(t, a, "AI 加的")
	vs := a.versions.List(a.path)
	if len(vs) != 2 || vs[0].Reason != versions.AI {
		t.Fatalf("versions %+v", vs)
	}
	tt.Frame()
	if err := tt.Click("历史版本"); err != nil { // the saving status under the title
		t.Fatal(err)
	}
	tt.Frame()
	if !a.historyOpen {
		t.Fatal("history not opened")
	}
	if sum := model.Summary(a.historyDiff); sum != "新增 1" {
		t.Fatalf("summary %q", sum)
	}
}

func TestHistoryAndConflict(t *testing.T) {
	a, tt := newTestApp(t)
	os.RemoveAll(a.versions.Dir) // kept as opened, on the clock of the test
	a.lastSnap = nil
	a.keepVersion(versions.Open)
	if vs := a.versions.List(a.path); len(vs) != 1 || vs[0].Reason != versions.Open {
		t.Fatalf("versions %+v", vs)
	}
	// Another program adds a node: the versions before and after are kept,
	// and the history shows what changed.
	changeOnDisk(t, a, func(f *model.File) {
		d := f.Sheets[0]
		d.AddChild(d.Root.ID, "AI 加的模块")
		n, _ := findIn(d, "登录")
		n.Task.Status = model.StatusDoing
	}, time.Minute)
	tt.Frame()
	find(t, a, "AI 加的模块")
	vs := a.versions.List(a.path)
	if len(vs) != 2 || vs[0].Reason != versions.External {
		t.Fatalf("versions %+v", vs)
	}
	// The newest version shows what it changed; the one before, compared
	// with now, the same.
	a.openHistory()
	tt.Frame()
	if sum := model.Summary(a.historyDiff); sum != "新增 1 · 状态 1" {
		t.Fatalf("summary %q", sum)
	}
	if !tt.HasText("AI 加的模块") || !tt.HasText("外部修改") {
		t.Fatalf("texts %q", tt.Texts())
	}
	snapshot(t, tt, "history")
	a.historyPick(1)
	if sum := model.Summary(a.historyDiff); sum != "没有变化" {
		t.Fatalf("the version before changed %q", sum)
	}
	tt.Frame()
	if err := tt.Click("到现在的变化"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if sum := model.Summary(a.historyDiff); !a.historyToNow || sum != "新增 1 · 状态 1" {
		t.Fatalf("to now %q", sum)
	}
	// Restoring the version before brings the map back, as a step undo
	// takes back.
	if err := tt.Click("恢复到这个版本"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, err := findIn(a.doc, "AI 加的模块"); err == nil {
		t.Fatal("not restored")
	}
	a.undo()
	find(t, a, "AI 加的模块")

	// A change outside while the map has unsaved changes: nothing is saved
	// over it until a version is chosen.
	a.mutate("", func() { find(t, a, "前端").Title = "界面层" })
	changeOnDisk(t, a, func(f *model.File) { f.Sheets[0].Root.Title = "外部改名" }, 2*time.Minute)
	tt.Frame()
	if !a.conflict {
		t.Fatal("no conflict")
	}
	a.now = func() time.Time { return testNow.Add(3 * time.Minute) }
	tt.Frame()
	if f, _ := model.Load(a.path); f.Sheets[0].Root.Title != "外部改名" {
		t.Fatal("saved over the outside change")
	}
	snapshot(t, tt, "conflict")
	if err := tt.Click("保留我的版本"); err != nil {
		t.Fatal(err)
	}
	f, _ := model.Load(a.path)
	if _, err := findIn(f.Sheets[0], "界面层"); err != nil || a.conflict {
		t.Fatal("mine not kept")
	}
	if vs := a.versions.List(a.path); vs[0].Reason != versions.Conflict && vs[1].Reason != versions.Conflict {
		t.Fatalf("the outside version not kept: %+v", vs[:2])
	}
}

func TestLinks(t *testing.T) {
	a, tt := newTestApp(t)
	fp := a.plat.(*fakePlatform)
	proj := t.TempDir()
	os.MkdirAll(filepath.Join(proj, "src"), 0o755)
	os.WriteFile(filepath.Join(proj, "src", "login.go"), []byte("package x"), 0o644)
	a.mutate("", func() { a.file.Project = proj })

	// What links are.
	for link, want := range map[string]int{
		"https://github.com/o/r/pull/12": linkWeb, "src/login.go:42": linkFile, "/abs/x.go": linkFile,
		"mindmap:#abc": linkNode, "mindmap:工作/后台#abc": linkNode,
	} {
		if kind, _, _ := a.parseLink(link); kind != want {
			t.Errorf("%s: kind %d", link, kind)
		}
	}
	if _, target, line := a.parseLink("src/login.go:42:7"); target != filepath.Join(proj, "src", "login.go") || line != 42 {
		t.Errorf("target %q line %d", target, line)
	}
	if _, l := a.linkLabel("https://github.com/o/r/pull/12"); l != "r PR #12" {
		t.Errorf("label %q", l)
	}

	// Adding links in the panel; the code opens in the editor at its line.
	clickNode(t, tt, a, "登录")
	clickInPanel(t, tt, "添加链接")
	tt.Type("src/login.go:42")
	tt.Key(0, ui.KeyEnter)
	n := find(t, a, "登录")
	if len(n.Links) != 1 {
		t.Fatalf("links %v", n.Links)
	}
	tt.Frame()
	if !tt.HasText("src/login.go:42") {
		t.Fatalf("texts %q", tt.Texts())
	}
	a.openLink(n.Links[0])
	if want := "vscode://file" + filepath.ToSlash(filepath.Join(proj, "src", "login.go")) + ":42"; fp.opened[len(fp.opened)-1] != want {
		t.Fatalf("opened %v, want %s", fp.opened, want)
	}
	a.cfg.Editor = "cursor"
	a.openLink(n.Links[0])
	if !strings.HasPrefix(fp.opened[len(fp.opened)-1], "cursor://file/") {
		t.Fatalf("opened %v", fp.opened)
	}
	// The mark on the canvas opens it too.
	tt.Frame()
	if err := tt.Click("打开链接"); err != nil {
		t.Fatal(err)
	}
	snapshot(t, tt, "links")

	// Linking to another node, through the palette, and back.
	clickInPanel(t, tt, "链接到节点…")
	if a.paletteMode != paletteLink {
		t.Fatal("not choosing a node")
	}
	tt.Type("后端")
	tt.Key(0, ui.KeyEnter)
	back := find(t, a, "后端")
	if len(n.Links) != 2 || n.Links[1] != "mindmap:#"+back.ID {
		t.Fatalf("links %v", n.Links)
	}
	if bl := a.backlinks(back.ID); len(bl) != 1 || bl[0].id != n.ID {
		t.Fatalf("backlinks %+v", bl)
	}
	a.openLink(n.Links[1])
	if a.selected != back.ID {
		t.Fatal("did not go to the node")
	}
	tt.Frame()
	if !tt.HasText("被 1 个节点链接") {
		t.Fatalf("no backlinks shown: %q", tt.Texts())
	}
	// The MCP window explains how to connect agents.
	a.mcpOpen = true
	tt.Frame()
	if !tt.HasText("连接 AI 编程工具（MCP）") {
		t.Fatal("no MCP window")
	}
	snapshot(t, tt, "mcp")
}

func TestDependenciesAndIterations(t *testing.T) {
	a, tt := newTestApp(t)
	search := find(t, a, "搜索")
	login := find(t, a, "登录")
	slow := find(t, a, "列表加载慢")
	// 搜索 waits on 列表加载慢, through the palette.
	clickNode(t, tt, a, "搜索")
	clickInPanel(t, tt, "添加前置任务…")
	if a.paletteMode != paletteAfter {
		t.Fatal("not choosing")
	}
	tt.Type("列表")
	tt.Key(0, ui.KeyEnter)
	if !slices.Equal(search.Task.After, []string{slow.ID}) || !a.blocked(search) {
		t.Fatalf("after %v", search.Task.After)
	}
	// No cycles.
	a.addAfter(slow, search.ID)
	if len(slow.Task.After) != 0 || !strings.Contains(a.saveErr, "循环") {
		t.Fatalf("cycle allowed: %v", slow.Task.After)
	}
	tt.Frame()
	snapshot(t, tt, "depends")
	// Done unblocks, and notes when.
	a.setStatus(slow, model.StatusDone)
	if a.blocked(search) || slow.Task.DoneAt == nil {
		t.Fatal("still blocked")
	}
	a.setStatus(slow, model.StatusTodo)
	if slow.Task.DoneAt != nil {
		t.Fatal("done time kept")
	}

	// Iterations, in the overview.
	tt.Key(ui.Cmd, ui.Key6)
	a.mode = viewOverview
	tt.Frame()
	if err := tt.Click("＋ 新建迭代"); err != nil {
		t.Fatal(err)
	}
	if len(a.file.Iterations) != 1 || a.file.Iterations[0].Name != "迭代 1" {
		t.Fatalf("iterations %+v", a.file.Iterations)
	}
	it := a.file.Iterations[0]
	a.mutate("", func() {
		login.Task.Iteration = it.ID
		login.Task.Estimate = 3
		search.Task.Iteration = it.ID
		search.Task.Estimate = 5
	})
	if done, total, hours := a.iterationProgress(it.ID); done != 1 || total != 2 || hours != "5" {
		t.Fatalf("progress %d/%d %s", done, total, hours)
	}
	tt.Frame()
	if !tt.HasText("1/2 完成 · 剩余 5 小时") {
		t.Fatalf("texts %q", tt.Texts())
	}
	snapshot(t, tt, "overview")
	// The board shows one iteration.
	a.mode = viewBoard
	a.iterFilter = it.ID
	if got := len(a.visibleTasks()); got != 2 {
		t.Fatalf("board shows %d", got)
	}
	tt.Frame()
	snapshot(t, tt, "board-iteration")
	a.iterFilter = ""
}

func TestTimeline(t *testing.T) {
	a, tt := newTestApp(t)
	search := find(t, a, "搜索")
	day := today(testNow)
	s, d := day, day.AddDate(0, 0, 3)
	a.mutate("", func() { search.Task.Start, search.Task.Due = &s, &d })
	a.mode = viewTimeline
	tt.Frame()
	r, ok := tt.Find("条 搜索")
	if !ok {
		t.Fatalf("no bar: %q", tt.Texts())
	}
	snapshot(t, tt, "timeline")
	// Two days later.
	tt.Press(r.X+r.W/2, r.Y+r.H/2)
	tt.Move(r.X+r.W/2+30, r.Y+r.H/2)
	tt.Move(r.X+r.W/2+56, r.Y+r.H/2)
	tt.Release(r.X+r.W/2+56, r.Y+r.H/2)
	tt.Frame()
	if !search.Task.Start.Equal(day.AddDate(0, 0, 2)) || !search.Task.Due.Equal(day.AddDate(0, 0, 5)) {
		t.Fatalf("dates %v – %v", search.Task.Start, search.Task.Due)
	}
	// Its end, a day longer.
	e, _ := tt.Find("调整截止 搜索")
	tt.Press(e.X+e.W/2, e.Y+e.H/2)
	tt.Move(e.X+e.W/2+15, e.Y+e.H/2)
	tt.Move(e.X+e.W/2+28, e.Y+e.H/2)
	tt.Release(e.X+e.W/2+28, e.Y+e.H/2)
	tt.Frame()
	if !search.Task.Start.Equal(day.AddDate(0, 0, 2)) || !search.Task.Due.Equal(day.AddDate(0, 0, 6)) {
		t.Fatalf("after the end: %v – %v", search.Task.Start, search.Task.Due)
	}
	// A task without dates is planned with a click.
	if err := tt.Click("安排 支持导出图片"); err != nil {
		t.Fatal(err)
	}
	if find(t, a, "支持导出图片").Task.Start == nil {
		t.Fatal("not planned")
	}
}

func TestReminders(t *testing.T) {
	a, tt := newTestApp(t)
	fp := a.plat.(*fakePlatform)
	day := today(testNow)
	yesterday := day.AddDate(0, 0, -1)
	a.mutate("", func() {
		find(t, a, "搜索").Task.Due = &day
		find(t, a, "列表加载慢").Task.Due = &yesterday
	})
	a.remindedAt = time.Time{}
	tt.Frame()
	if len(fp.notes) != 1 || !strings.Contains(fp.notes[0], "已逾期：列表加载慢") || !strings.Contains(fp.notes[0], "今天到期：搜索") {
		t.Fatalf("notes %q", fp.notes)
	}
	// Once a day.
	a.remindedAt = time.Time{}
	tt.Frame()
	if len(fp.notes) != 1 {
		t.Fatalf("notes %q", fp.notes)
	}
}

func TestQuickCaptureAndInbox(t *testing.T) {
	a, tt := newTestApp(t)
	fp := a.plat.(*fakePlatform)
	guide := a.path
	// The capture window, a window of its own.
	ct := ui.NewTester(a.captureView, 560, 170)
	fp.capture = true
	ct.Type("调研竞品\n  - [ ] 看 XMind 的导出")
	ct.Key(0, ui.KeyEnter)
	if fp.capture {
		t.Fatal("window not hidden")
	}
	f, err := model.Load(a.inboxPath())
	if err != nil {
		t.Fatal(err)
	}
	root := f.Sheets[0].Root
	if len(root.Children) != 1 || root.Children[0].Title != "调研竞品" || root.Children[0].Children[0].Task == nil {
		t.Fatalf("inbox %+v", root.Children)
	}
	tt.Frame()
	if !tt.HasText("收件箱") || a.inboxCount() != 1 {
		t.Fatalf("no inbox row: %q", tt.Texts())
	}
	snapshot(t, tt, "inbox-row")
	// Under the node selected, as a step undo takes back.
	clickNode(t, tt, a, "架构")
	if err := a.capture("缓存层", captureHere); err != nil {
		t.Fatal(err)
	}
	find(t, a, "缓存层")
	a.undo()
	if _, err := findIn(a.doc, "缓存层"); err == nil {
		t.Fatal("not undone")
	}
	// Sorting out the inbox: its node moves to a node of the guide.
	if err := tt.Click("收件箱"); err != nil {
		t.Fatal(err)
	}
	if a.path != a.inboxPath() {
		t.Fatalf("open %q", a.path)
	}
	clickNode(t, tt, a, "调研竞品")
	a.openPalette(paletteMove)
	tt.Frame()
	tt.Type("待优化")
	tt.Key(0, ui.KeyEnter)
	if len(a.doc.Root.Children) != 0 {
		t.Fatal("not moved out of the inbox")
	}
	g, _ := model.Load(guide)
	opt, err := findIn(g.Sheets[0], "待优化")
	if err != nil || opt.Children[len(opt.Children)-1].Title != "调研竞品" {
		t.Fatalf("not moved in: %v", err)
	}
}

func TestTrayAndSettings(t *testing.T) {
	a, tt := newTestApp(t)
	fp := a.plat.(*fakePlatform)
	day := today(testNow)
	yesterday := day.AddDate(0, 0, -1)
	a.mutate("", func() {
		find(t, a, "搜索").Task.Due = &day
		find(t, a, "列表加载慢").Task.Due = &yesterday
	})
	var labels []string
	for _, e := range a.trayMenu() {
		labels = append(labels, e.label)
	}
	all := strings.Join(labels, "|")
	if !strings.Contains(all, "今天到期和已逾期 2") || !strings.Contains(all, "⚠ 列表加载慢") || !strings.Contains(all, "○ 搜索") {
		t.Fatalf("menu %q", labels)
	}
	if a.trayTitle() != "2" {
		t.Fatalf("title %q", a.trayTitle())
	}
	if _, err := png.Decode(bytes.NewReader(trayIcon(""))); err != nil {
		t.Fatal(err)
	}
	a.trayAt = time.Time{}
	tt.Frame()
	if fp.trayUpdates == 0 {
		t.Fatal("tray not updated")
	}
	// Settings: the capture shortcut.
	a.settingsOpen = true
	tt.Frame()
	if err := tt.Click("快速记录快捷键"); err != nil {
		t.Fatal(err)
	}
	if a.recordingKey != "快速记录快捷键" || fp.captureKey != "" || !tt.HasText("请按下新的快捷键…") {
		t.Fatalf("not recording: %q, %q", a.recordingKey, fp.captureKey)
	}
	tt.Key(0, ui.KeyM) // a key without a modifier is not a shortcut
	tt.Key(ui.Super|ui.Shift, ui.KeyM)
	if fp.captureKey != "Shift+Command+M" || a.cfg.CaptureKey != "Shift+Command+M" || a.recordingKey != "" {
		t.Fatalf("key %q", fp.captureKey)
	}
	if !tt.HasText(keyLabel("Shift+Command+M")) {
		t.Fatalf("not shown: %q", tt.Texts())
	}
	// Esc cancels, and the default comes back.
	tt.Click("快速记录快捷键")
	tt.Key(0, ui.KeyEscape)
	if fp.captureKey != "Shift+Command+M" || a.recordingKey != "" {
		t.Fatalf("cancel: %q", fp.captureKey)
	}
	if err := tt.Click("恢复默认"); err != nil {
		t.Fatal(err)
	}
	if fp.captureKey != defaultCaptureKey || a.cfg.CaptureKey != "" {
		t.Fatalf("default: %q", fp.captureKey)
	}
	// Closed while recording: the shortcut comes back.
	tt.Click("快速记录快捷键")
	if fp.captureKey != "" {
		t.Fatal("not let go while recording")
	}
	a.settingsOpen = false
	a.keysBack()
	if fp.captureKey != defaultCaptureKey || a.recordingKey != "" {
		t.Fatalf("not back: %q", fp.captureKey)
	}
	snapshot(t, tt, "settings")
}

func reply(text string) string {
	b, _ := json.Marshal(text)
	return `{"id": "msg_r", "type": "message", "role": "assistant", "model": "claude-opus-5-5", "stop_reason": "end_turn",
		"content": [{"type": "text", "text": ` + string(b) + `}], "usage": {"input_tokens": 1, "output_tokens": 1}}`
}

func TestAIActions(t *testing.T) {
	a, tt := newTestApp(t)
	fake := &fakeClaude{replies: []string{reply("## 本周完成\n- 登录"), reply("风险 1"), reply("拆好了"), reply("整理好了"), reply("架构")}}
	a.sendAI = fake.send
	a.cfg.AIKey = "sk-ant-test"
	sent := func(i int) string {
		body, _ := json.Marshal(fake.requests[i])
		return string(body)
	}
	// The week's report: with what changed since a version of last week.
	os.RemoveAll(a.versions.Dir)
	old := a.file.Clone()
	a.versions.Save(a.path, old, versions.Auto, "", testNow.AddDate(0, 0, -8))
	login := find(t, a, "登录")
	a.mutate("", func() { a.doc.AddChild(a.doc.Root.ID, "新模块") })
	a.setStatus(find(t, a, "搜索"), model.StatusDone)
	a.aiReport(7)
	tt.Frame()
	if body := sent(0); !strings.Contains(body, "周报") || !strings.Contains(body, "新增：新模块") || !strings.Contains(body, "搜索（10/10 完成）") {
		t.Fatalf("report request %s", body)
	}
	if !tt.HasText("本周完成") {
		t.Fatalf("reply not shown: %q", tt.Texts())
	}
	snapshot(t, tt, "ai-report")
	// The risk scan: with the checks' findings.
	a.aiRiskScan()
	if body := sent(1); !strings.Contains(body, "自动检查发现") || !strings.Contains(body, "停留在「想法」的任务（1）：支持导出图片") {
		t.Fatalf("risk request %s", body)
	}
	// Breaking a node down sends it alone.
	a.selected = login.ID
	a.aiBreakDown()
	if body := sent(2); !strings.Contains(body, "拆解成") || !strings.Contains(body, "选中的节点及其下级") {
		t.Fatalf("break-down request %s", body)
	}
	// Text pasted.
	a.aiFromText("需求：支持导出 PDF")
	if body := sent(3); !strings.Contains(body, "待整理的文本") || !strings.Contains(body, "支持导出 PDF") {
		t.Fatalf("text request %s", body)
	}
	// A repository: its tree and README, and it becomes the project.
	repo := t.TempDir()
	os.MkdirAll(filepath.Join(repo, "internal", "auth"), 0o755)
	os.MkdirAll(filepath.Join(repo, "node_modules", "x"), 0o755)
	os.WriteFile(filepath.Join(repo, "README.md"), []byte("# 后台服务\n负责用户和权限"), 0o644)
	os.WriteFile(filepath.Join(repo, "internal", "auth", "login.go"), []byte("package auth"), 0o644)
	a.aiFromRepo(repo)
	body := sent(4)
	if !strings.Contains(body, "internal/") || !strings.Contains(body, "login.go") || !strings.Contains(body, "负责用户和权限") ||
		strings.Contains(body, "node_modules") || a.file.Project != repo {
		t.Fatalf("repo request %s", body)
	}
}

func TestMapFromFoldersAndText(t *testing.T) {
	a, tt := newTestApp(t)
	repo := t.TempDir()
	for _, d := range []string{"cmd/server", "internal/auth", "internal/store", ".git/x", "vendor/y"} {
		os.MkdirAll(filepath.Join(repo, d), 0o755)
	}
	a.mapFromFolders(repo)
	cmd := find(t, a, "cmd")
	if cmd.Links[0] != "cmd" || find(t, a, "auth").Links[0] != "internal/auth" {
		t.Fatalf("links %v", cmd.Links)
	}
	if _, err := findIn(a.doc, "vendor"); err == nil {
		t.Fatal("vendor mapped")
	}
	// Without the AI, text becomes nodes by its indentation.
	a.fromTextOpen = true
	tt.Frame()
	a.fromText = "上线计划\n  - [ ] 压测\n  - [ ] 灰度"
	if err := tt.Click("按缩进直接转换"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	plan := find(t, a, "上线计划")
	if len(plan.Children) != 2 || plan.Children[0].Task == nil {
		t.Fatalf("plan %+v", plan.Children)
	}
}

func TestImportExportFormats(t *testing.T) {
	a, _ := newTestApp(t)
	fp := a.plat.(*fakePlatform)
	dir := t.TempDir()
	// Markdown in.
	md := filepath.Join(dir, "需求.md")
	os.WriteFile(md, []byte("# 新版本\n## 功能\n- [ ] 导出 PDF\n- 分享"), 0o644)
	fp.openPath = md
	a.askImportOutline()
	if mapName(a.path) != "需求" || a.doc.Root.Title != "新版本" || find(t, a, "导出 PDF").Task == nil {
		t.Fatalf("imported %q %q", a.path, a.doc.Root.Title)
	}
	// OPML out and in.
	fp.savePath = filepath.Join(dir, "大纲")
	a.askExportOPML()
	fp.openPath = fp.savePath + ".opml"
	a.askImportOutline()
	if mapName(a.path) != "大纲" || find(t, a, "分享") == nil {
		t.Fatalf("opml %q", a.path)
	}
	// The calendar.
	_ = a.open(filepath.Join(a.dir, model.FileName("使用指南")))
	fp.savePath = filepath.Join(dir, "日历")
	a.askExportICS()
	ics, _ := os.ReadFile(fp.savePath + ".ics")
	if !strings.Contains(string(ics), "SUMMARY:搜索") || !strings.Contains(string(ics), "SUMMARY:列表加载慢") {
		t.Fatalf("ics %s", ics)
	}
	// The slides as a PDF, which a PDF reader opens.
	fp.savePath = filepath.Join(dir, "演说")
	a.askExportSlides()
	if a.saveErr != "" {
		t.Fatal(a.saveErr)
	}
	pdf := fp.savePath + ".pdf"
	if out, err := exec.Command("pdfinfo", pdf).CombinedOutput(); err == nil {
		if !strings.Contains(strings.Join(strings.Fields(string(out)), " "), fmt.Sprintf("Pages: %d", len(slidesOf(a.doc)))) {
			t.Fatalf("pdfinfo:\n%s", out)
		}
	} else if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("pdfinfo: %v\n%s", err, out)
	}
}

func TestGitHubIssues(t *testing.T) {
	a, tt := newTestApp(t)
	issues := `[
		{"number": 3, "title": "登录失败", "state": "open", "html_url": "https://github.com/o/r/issues/3", "body": "复现步骤…", "labels": [{"name": "bug"}]},
		{"number": 2, "title": "一个 PR", "state": "open", "html_url": "https://github.com/o/r/pull/2", "pull_request": {"url": "x"}},
		{"number": 1, "title": "旧的", "state": "closed", "html_url": "https://github.com/o/r/issues/1"}
	]`
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		if r.URL.Path != "/repos/o/r/issues" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, issues)
	}))
	defer srv.Close()
	a.cfg.GitHubAPI, a.cfg.GitHubToken = srv.URL, "ghp_test"
	a.mutate("", func() { a.file.GitHub = "o/r" })
	a.syncGitHub()
	tt.Frame()
	if auth != "Bearer ghp_test" {
		t.Fatalf("auth %q", auth)
	}
	n := find(t, a, "#3 登录失败")
	if n.Task.Status != model.StatusTodo || n.Links[0] != "https://github.com/o/r/issues/3" || n.Tags[0] != "bug" || n.Note != "复现步骤…" {
		t.Fatalf("issue %+v", n)
	}
	if _, err := findIn(a.doc, "#2 一个 PR"); err == nil {
		t.Fatal("pull request synced")
	}
	if _, err := findIn(a.doc, "#1 旧的"); err == nil {
		t.Fatal("closed issue added")
	}
	// Closed and renamed on GitHub: followed.
	issues = `[{"number": 3, "title": "登录偶尔失败", "state": "closed", "html_url": "https://github.com/o/r/issues/3"}]`
	a.syncGitHub()
	n = find(t, a, "#3 登录偶尔失败")
	if n.Task.Status != model.StatusDone || !strings.Contains(a.ghResult, "更新 1") {
		t.Fatalf("after %+v %q", n.Task, a.ghResult)
	}
	// Errors are told.
	a.mutate("", func() { a.file.GitHub = "o/missing" })
	a.syncGitHub()
	if !strings.Contains(a.ghResult, "找不到仓库") {
		t.Fatalf("result %q", a.ghResult)
	}
	a.panelTab = tabCanvas
	tt.Frame()
	snapshot(t, tt, "github")
}

func TestMoreStructures(t *testing.T) {
	a, tt := newTestApp(t)
	for _, s := range []model.Structure{model.StructureOrg, model.StructureTimeline, model.StructureFishbone} {
		a.setStructure(s)
		a.cam().placed = false
		tt.Frame()
		root := a.boxes.Boxes[a.doc.Root.ID]
		arch := a.boxes.Boxes[find(t, a, "架构").ID]
		front := a.boxes.Boxes[find(t, a, "前端").ID]
		switch s {
		case model.StructureOrg:
			if arch.Y <= root.Y+root.H || front.Y <= arch.Y+arch.H {
				t.Fatalf("org: root %v arch %v front %v", root, arch, front)
			}
		case model.StructureTimeline:
			if arch.X <= root.X+root.W {
				t.Fatalf("timeline: branch left of the root: %v %v", root, arch)
			}
		case model.StructureFishbone:
			if arch.X+arch.W >= root.X {
				t.Fatalf("fishbone: branch right of the head: %v %v", root, arch)
			}
		}
		// No two boxes overlap.
		var ids []string
		for id := range a.boxes.Boxes {
			ids = append(ids, id)
		}
		for i, x := range ids {
			for _, y := range ids[i+1:] {
				p, q := a.boxes.Boxes[x], a.boxes.Boxes[y]
				if p.X < q.X+q.W && q.X < p.X+p.W && p.Y < q.Y+q.H && q.Y < p.Y+p.H {
					n1, _ := a.doc.Find(x)
					n2, _ := a.doc.Find(y)
					t.Fatalf("%s: %q and %q overlap", s, n1.Title, n2.Title)
				}
			}
		}
		snapshot(t, tt, "structure-"+string(s))
	}
}

func TestAnnotations(t *testing.T) {
	a, tt := newTestApp(t)
	// A relation line: from the menu's start, ended by a click.
	front, login := find(t, a, "前端"), find(t, a, "登录")
	a.startRelation(front)
	clickNode(t, tt, a, "登录")
	tt.Frame()
	if len(a.doc.Relations) != 1 || a.doc.Relations[0].From != front.ID || a.doc.Relations[0].To != login.ID || a.relFrom != "" {
		t.Fatalf("relations %+v", a.doc.Relations)
	}
	// Its label, in the panel of either end.
	tt.Frame()
	clickInPanel(t, tt, "关联线标签")
	tt.Type("调用")
	if a.doc.Relations[0].Label != "调用" {
		t.Fatalf("label %q", a.doc.Relations[0].Label)
	}
	// A summary and a boundary.
	arch := find(t, a, "架构")
	a.mutate("", func() {
		arch.Summary = &model.Mark{Label: "两层"}
		find(t, a, "功能").Boundary = &model.Mark{Label: "MVP"}
	})
	// A floating topic, by a double click on the canvas.
	tt.Frame()
	x, y := a.vp.X+40, a.vp.Y+a.vp.H-80
	tt.Move(x, y)
	tt.ClickAt(x, y)
	tt.ClickAt(x, y)
	tt.Frame()
	if len(a.doc.Floating) != 1 || a.editing != a.doc.Floating[0].ID {
		t.Fatalf("floating %+v editing %q", a.doc.Floating, a.editing)
	}
	tt.Type("灵感")
	tt.Key(0, ui.KeyEnter)
	fl := a.doc.Floating[0]
	if fl.Title != "灵感" {
		t.Fatalf("title %q", fl.Title)
	}
	tt.Frame()
	snapshot(t, tt, "annotations")
	// Dragged, it goes there.
	pos := *fl.Pos
	b := a.boxes.Boxes[fl.ID]
	cam := a.cam()
	sx, sy := a.vp.X+cam.X+(b.X+b.W/2)*cam.Zoom, a.vp.Y+cam.Y+(b.Y+b.H/2)*cam.Zoom
	tt.Press(sx, sy)
	tt.Move(sx+20, sy-10)
	tt.Move(sx+40, sy-20)
	tt.Release(sx+40, sy-20)
	if fl.Pos.X <= pos.X || fl.Pos.Y >= pos.Y {
		t.Fatalf("pos %v → %v", pos, *fl.Pos)
	}
	// The picture has them all, the floating topic too.
	data, err := a.exportImage()
	if err != nil {
		t.Fatal(err)
	}
	if dir := os.Getenv("MINDMAP_SNAPSHOTS"); dir != "" {
		os.WriteFile(filepath.Join(dir, "annotations-export.png"), data, 0o644)
	}
	// Folding a branch keeps the others shown.
	a.mutate("", func() { find(t, a, "使用指南").Collapsed = true })
	tt.Frame()
	for _, title := range []string{"前端", "登录", "待优化", "灵感"} {
		if _, ok := a.boxes.Boxes[find(t, a, title).ID]; !ok {
			t.Errorf("%s not laid out", title)
		}
		if !tt.HasText(title) {
			t.Errorf("%s not shown after a folded branch", title)
		}
	}
}

func TestAttachments(t *testing.T) {
	a, tt := newTestApp(t)
	fp := a.plat.(*fakePlatform)
	dir := t.TempDir()
	doc := filepath.Join(dir, "设计稿.pdf")
	os.WriteFile(doc, []byte("%PDF-1.4"), 0o644)
	pic := filepath.Join(dir, "截图.png")
	os.WriteFile(pic, testPNG(t, 40, 30), 0o644)
	n := find(t, a, "前端")
	a.dropFiles(n, []string{doc, pic})
	if len(n.Files) != 1 || n.Files[0].Name != "设计稿.pdf" || n.Image == nil {
		t.Fatalf("files %+v image %v", n.Files, n.Image)
	}
	if _, err := os.Stat(filepath.Join(model.AssetsDir(a.path), n.Files[0].File)); err != nil {
		t.Fatal(err)
	}
	// A second picture, the node having one, attaches as a file.
	a.dropFiles(n, []string{pic})
	if len(n.Files) != 2 {
		t.Fatalf("files %+v", n.Files)
	}
	tt.Frame()
	if err := tt.Click("打开附件"); err != nil {
		t.Fatal(err)
	}
	if last := fp.opened[len(fp.opened)-1]; !strings.HasPrefix(last, "path:") || !strings.HasSuffix(last, ".pdf") {
		t.Fatalf("opened %v", fp.opened)
	}
	// Copied into another map, its files go along.
	a.lightbox = ""
	a.selected, a.multi = n.ID, nil
	a.copyNodes(false)
	a.newMap()
	a.commitEdit()
	tt.Frame()
	a.paste()
	got := a.doc.Root.Children[0]
	if _, err := os.Stat(filepath.Join(model.AssetsDir(a.path), got.Files[0].File)); err != nil {
		t.Fatalf("not copied: %v", err)
	}
	snapshot(t, tt, "attachments")
}

func TestHighlight(t *testing.T) {
	th := ui.LightTheme()
	spans := highlight("func main() { // 入口\n\ts := \"你好\" + 42\n}", "go", 12, th)
	var kinds []string
	for _, s := range spans {
		if strings.TrimSpace(s.Text) == "" {
			continue
		}
		kinds = append(kinds, s.Text)
	}
	joined := strings.Join(kinds, "|")
	for _, want := range []string{"func", "// 入口", `"你好"`, "42"} {
		if !strings.Contains(joined, want) {
			t.Errorf("no %q in %q", want, joined)
		}
	}
	for _, s := range spans {
		if s.Text == "func" && s.Color == th.Text {
			t.Error("keyword not colored")
		}
	}
	if py := highlight("# 注释\nx = 1", "python", 12, th); py[0].Text != "# 注释" {
		t.Errorf("python comment %q", py[0].Text)
	}
}

func TestTemplates(t *testing.T) {
	a, tt := newTestApp(t)
	a.templatesOpen = true
	tt.Frame()
	snapshot(t, tt, "templates")
	if err := tt.Click("模板 AI 开发管理"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if mapName(a.path) != "AI 开发管理" || a.doc.Root.Title != "项目名称" || find(t, a, "功能 1").Task == nil {
		t.Fatalf("made %q %q", a.path, a.doc.Root.Title)
	}
	// The map, as a template of one's own, and a map from it.
	a.commitEdit()
	a.mutate("", func() { a.doc.Root.Title = "我的项目模板" })
	a.saveAsTemplate()
	var mine *template
	for _, tp := range a.templates() {
		if tp.path != "" {
			mine = &tp
		}
	}
	if mine == nil {
		t.Fatal("not saved")
	}
	a.newFromTemplate(*mine)
	if a.doc.Root.Title != "我的项目模板" || mapName(a.path) != "AI 开发管理 2" {
		t.Fatalf("from mine %q %q", a.path, a.doc.Root.Title)
	}
}

// The app's icon is chosen in the settings, and kept.
func TestChooseAppIcon(t *testing.T) {
	a, tt := newTestApp(t)
	fp := a.plat.(*fakePlatform)
	a.openSettings()
	tt.Frame()
	snapshot(t, tt, "settings-icon")
	if err := tt.Click("图标 知识星图"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.cfg.Icon != "orbit" || !bytes.Equal(fp.icon, appIconPNG("orbit")) || !bytes.Equal(fp.tray, trayIcon("orbit")) {
		t.Fatalf("icon %q, %d bytes set", a.cfg.Icon, len(fp.icon))
	}
	if err := tt.Click("图标 知识卡片"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.cfg.Icon != "" || !bytes.Equal(fp.icon, appIconPNG("")) {
		t.Fatalf("icon %q", a.cfg.Icon)
	}
	for _, ic := range appIcons {
		if _, err := png.Decode(bytes.NewReader(appIconPNG(ic.key))); err != nil {
			t.Fatal(ic.name, err)
		}
	}
}

// The settings check for updates, and turn the daily check off.
func TestUpdateSettings(t *testing.T) {
	a, tt := newTestApp(t)
	fp := a.plat.(*fakePlatform)
	a.openSettings()
	tt.Frame()
	if !tt.HasText(appName + " " + version) {
		t.Fatalf("no version: %q", tt.Texts())
	}
	if err := tt.Click("检查更新…"); err != nil {
		t.Fatal(err)
	}
	if fp.checks != 1 {
		t.Fatalf("checks %d", fp.checks)
	}
	if err := tt.Click("自动检查更新"); err != nil {
		t.Fatal(err)
	}
	if !fp.noAuto {
		t.Fatal("daily check still on")
	}
}

func TestShortcutLabels(t *testing.T) {
	for _, c := range []struct{ acc, mac, win string }{
		{"Control+Alt+A", "⌃⌥A", "Ctrl+Alt+A"},
		{"CmdOrCtrl+Shift+M", "⇧⌘M", "Ctrl+Shift+M"},
		{"Control+Alt+Space", "⌃⌥Space", "Ctrl+Alt+Space"},
		{"Option+Command+Left", "⌥⌘←", "Alt+Win+Left"},
	} {
		if got := accelLabel(c.acc, "darwin"); got != c.mac {
			t.Errorf("%s on macOS: %q", c.acc, got)
		}
		if got := accelLabel(c.acc, "windows"); got != c.win {
			t.Errorf("%s on Windows: %q", c.acc, got)
		}
	}
}

// A long line of the outline wraps instead of running out of the window.
func TestOutlineWraps(t *testing.T) {
	a, tt := newTestApp(t)
	n := a.doc.Root.Children[0]
	a.mutate("", func() {
		n.Title = strings.Repeat("及时高质量完成需求，针对线上问题积极跟进处理。", 6)
	})
	a.mode = viewOutline
	tt.Frame()
	el, ok := tt.Find("大纲 " + n.Title)
	if !ok {
		t.Fatalf("no line: %q", tt.Texts())
	}
	if el.H < 40 || el.X+el.W > float32(1280)-a.panelWidth {
		t.Fatalf("line at %+v", el)
	}
	snapshot(t, tt, "outline-wrap")
}

func TestReorderSiblings(t *testing.T) {
	a, tt := newTestApp(t)
	arch := find(t, a, "架构")
	titles := func() (s []string) {
		for _, c := range arch.Children {
			s = append(s, c.Title)
		}
		return s
	}
	drag := func(title, onto string, after bool) {
		t.Helper()
		from, ok1 := tt.Find(title)
		to, ok2 := tt.Find(onto)
		if !ok1 || !ok2 {
			t.Fatal("nodes not found")
		}
		x, y := to.X+5, to.Y+3 // the top edge
		if after {
			y = to.Y + to.H - 3
		}
		tt.Press(from.X+5, from.Y+5)
		for i := 1; i <= 10; i++ {
			f := float32(i) / 10
			tt.Move(from.X+5+(x-from.X-5)*f, from.Y+5+(y-from.Y-5)*f)
		}
		snapshot(t, tt, fmt.Sprintf("reorder-%v", after))
		tt.Release(x, y)
		tt.Frame()
	}
	a.selectNode(arch.Children[len(arch.Children)-1].ID)
	a.addSibling()
	tt.Frame()
	tt.Type("缓存")
	tt.Key(0, ui.KeyEnter)
	start := titles()
	if len(start) < 3 {
		t.Fatalf("children %v", start)
	}
	// Onto the first's top edge: before it, not under it.
	drag(start[2], start[0], false)
	if got := titles(); !slices.Equal(got[:3], []string{start[2], start[0], start[1]}) {
		t.Fatalf("before: %v", got)
	}
	// Onto the last's bottom edge: after it.
	drag(start[2], start[1], true)
	if got := titles(); !slices.Equal(got[:3], []string{start[0], start[1], start[2]}) {
		t.Fatalf("after: %v", got)
	}

	// Shift+Enter adds a sibling above the one selected.
	if err := clickNode(t, tt, a, start[1]); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Shift, ui.KeyEnter)
	tt.Type("上面")
	tt.Key(0, ui.KeyEnter)
	if got := titles(); !slices.Equal(got[:4], []string{start[0], "上面", start[1], start[2]}) {
		t.Fatalf("Shift+Enter: %v", got)
	}
}

func TestProgressMarkerMakesTask(t *testing.T) {
	a, tt := newTestApp(t)
	clickNode(t, tt, a, "后端")
	if err := tt.Click("标记"); err != nil {
		t.Fatal(err)
	}
	n := find(t, a, "后端")
	if err := tt.Click("任务开始"); err != nil {
		t.Fatal(err)
	}
	if n.Task == nil || n.Task.Status != model.StatusTodo {
		t.Fatalf("task %+v", n.Task)
	}
	tt.Click("任务完成")
	if n.Task.Status != model.StatusDone || n.Task.DoneAt == nil {
		t.Fatalf("task %+v", n.Task)
	}
	// The status set elsewhere moves the marker.
	a.setStatus(n, model.StatusDoing)
	if !slices.Contains(n.Markers, "task:1") {
		t.Fatalf("markers %v", n.Markers)
	}
	tt.Frame()
	snapshot(t, tt, "progress-task")
	// Not a task any more: the marker goes too.
	a.toggleTask(n)
	if n.Task != nil || n.Markers != nil {
		t.Fatalf("task %+v, markers %v", n.Task, n.Markers)
	}
}

func TestShowTaskOnBoard(t *testing.T) {
	a, tt := newTestApp(t)
	n := find(t, a, "后端")
	a.toggleMarker(n, "task:0")
	tt.Frame()
	if n.Task == nil {
		t.Fatal("not a task")
	}
	// The progress marker takes the check's place.
	if _, ok := tt.Find("完成任务 后端"); ok {
		t.Fatal("a check beside the progress marker")
	}
	if err := clickNode(t, tt, a, "后端"); err != nil {
		t.Fatal(err)
	}
	a.showOnBoard(n.ID)
	tt.Frame()
	if a.mode != viewBoard || a.selected != n.ID || a.boardReveal != "" {
		t.Fatalf("mode %v, selected %v, reveal %q", a.mode, a.selected, a.boardReveal)
	}
	if !tt.HasText("后端") {
		t.Fatal("card not shown")
	}
	snapshot(t, tt, "board-reveal")
}

func TestIdeaMarker(t *testing.T) {
	a, tt := newTestApp(t)
	n := find(t, a, "后端")
	a.toggleMarker(n, "task:3")
	a.toggleMarker(n, "symbol:idea")
	if n.Task == nil || n.Task.Status != model.StatusIdea || !slices.Equal(n.Markers, []string{"symbol:idea"}) {
		t.Fatalf("task %+v, markers %v", n.Task, n.Markers)
	}
	a.setStatus(n, model.StatusDoing)
	if !slices.Equal(n.Markers, []string{"task:1"}) {
		t.Fatalf("markers %v", n.Markers)
	}
	tt.Frame()
}

func TestRelationLine(t *testing.T) {
	a, tt := newTestApp(t)
	if err := clickNode(t, tt, a, "前端"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyL)
	if a.relFrom == "" {
		t.Fatal("not drawing a line")
	}
	to, ok := tt.Find("功能")
	if !ok {
		t.Fatal("no 功能")
	}
	tt.Move(to.X-40, to.Y+to.H/2)
	tt.Frame()
	if a.relTo == nil {
		t.Fatal("the line does not follow the pointer")
	}
	snapshot(t, tt, "relation-drawing")
	tt.ClickAt(to.X+to.W/2, to.Y+to.H/2)
	tt.Frame()
	rels := a.doc.Relations
	if len(rels) != 1 || rels[0].From != find(t, a, "前端").ID || rels[0].To != find(t, a, "功能").ID || a.relFrom != "" {
		t.Fatalf("relations %+v", rels)
	}
	snapshot(t, tt, "relation")
}

func TestDeleteFloatingTopic(t *testing.T) {
	a, tt := newTestApp(t)
	a.addFloatingAt(300, 120)
	tt.Frame()
	tt.Key(0, ui.KeyEnter) // ends the edit, the title kept
	id := a.selected
	if !a.doc.IsFloating(id) {
		t.Fatal("not floating")
	}
	a.canvas.Focus()
	tt.Frame()
	tt.Key(0, ui.KeyDelete)
	if n, _ := a.doc.Find(id); n != nil {
		t.Fatal("the floating topic was not deleted")
	}
}

func TestSelectBendDeleteRelation(t *testing.T) {
	a, tt := newTestApp(t)
	from, to := find(t, a, "前端"), find(t, a, "功能")
	a.selected = from.ID
	a.startRelation(from)
	a.endRelation(to.ID)
	tt.Frame()
	rel := a.doc.Relations[0]
	cam := a.cam()
	k, ok := relationCurveOf(rel, a.boxes, cam.X, cam.Y, cam.Zoom)
	if !ok {
		t.Fatal("no curve")
	}
	// A click on the line selects it.
	mx, my := k.at(0.5)
	if got := a.relationAt(mx, my); got != rel.ID {
		t.Fatalf("relation at its middle %q", got)
	}
	tt.ClickAt(a.vp.X+mx, a.vp.Y+my)
	tt.Frame()
	if a.selRel != rel.ID {
		t.Fatal("not selected")
	}
	snapshot(t, tt, "relation-selected")
	// Its handle bends it.
	h, ok := tt.Find("拖动改变连线形状")
	if !ok {
		t.Fatal("no handle")
	}
	hx, hy := h.X+h.W/2, h.Y+h.H/2
	tt.Press(hx, hy)
	for i := 1; i <= 10; i++ {
		tt.Move(hx, hy-float32(i)*6)
	}
	tt.Release(hx, hy-60)
	tt.Frame()
	bent := a.doc.Relations[0]
	if bent.Bend == nil {
		t.Fatal("not bent")
	}
	k2, _ := relationCurveOf(bent, a.boxes, cam.X, cam.Y, cam.Zoom)
	if _, y2 := k2.at(0.5); y2 > my-40 {
		t.Fatalf("middle at %v, was %v", y2, my)
	}
	snapshot(t, tt, "relation-bent")
	// Delete takes it off, the nodes staying.
	a.canvas.Focus()
	tt.Frame()
	tt.Key(0, ui.KeyDelete)
	if len(a.doc.Relations) != 0 || find(t, a, "前端") == nil {
		t.Fatalf("relations %+v", a.doc.Relations)
	}
}
