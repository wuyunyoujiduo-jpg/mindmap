package main

import (
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

type fakePlatform struct {
	folder  string
	trashed []string
}

func (f *fakePlatform) PickFolder(done func(string)) { done(f.folder) }
func (f *fakePlatform) ShowInFolder(string)          {}
func (f *fakePlatform) Trash(path string) error {
	f.trashed = append(f.trashed, path)
	return os.Remove(path)
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
	if err := tt.Click("架构"); err != nil {
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
	r, ok := tt.Find("架构")
	if !ok {
		t.Fatal("no 架构")
	}
	tt.ClickAt(r.X+2, r.Y+2)
	tt.ClickAt(r.X+2, r.Y+2)
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
	if err := tt.Click("支持导出图片"); err != nil {
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
	if err := tt.Click("核心 2"); err != nil {
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
	tt.Click("功能")
	if err := tt.Click("折叠"); err != nil {
		t.Fatal(err)
	}
	if !find(t, a, "功能").Collapsed || tt.HasText("登录") {
		t.Fatal("not collapsed")
	}
	tt.Key(0, ui.KeyRight) // opens and goes into it
	if find(t, a, "功能").Collapsed {
		t.Fatal("Right did not open")
	}
}

func TestNewMapAndDelete(t *testing.T) {
	a, tt := newTestApp(t)
	if err := tt.Click("+ 新建导图"); err != nil {
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
	if err := tt.Click("列表加载慢"); err != nil {
		t.Fatal(err)
	}
	if _, p := a.selectedNode(); p.Title != "待优化" {
		t.Fatalf("parent changed to %q", p.Title)
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
	// Drag the empty part of the canvas.
	r, _ := tt.Find("我的项目")
	px, py := r.X, r.Y+250 // below the map
	tt.Press(px, py)
	for i := 1; i <= 5; i++ {
		tt.Move(px+float32(i)*20, py+float32(i)*10)
	}
	tt.Release(px+100, py+50)
	if dx, dy := cam.X-x0, cam.Y-y0; dx < 80 || dy < 40 {
		t.Fatalf("dragging moved the map by %v, %v", dx, dy)
	}
	if after, _ := tt.Find("我的项目"); after.X-r.X < 80 {
		t.Fatalf("root moved from %v to %v", r.X, after.X)
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
	tt.Press(r.X+5, r.Y+5)
	tt.Move(r.X+40, r.Y+60)
	tt.Release(r.X+40, r.Y+60)
	if cam.X != x0 {
		t.Fatal("dragging a node moved the canvas")
	}
}

func TestSheets(t *testing.T) {
	a, tt := newTestApp(t)
	if len(a.file.Sheets) != 2 || !tt.HasText("项目结构") || !tt.HasText("迭代计划") {
		t.Fatalf("tabs %q", tt.Texts())
	}
	tt.Click("架构")
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
