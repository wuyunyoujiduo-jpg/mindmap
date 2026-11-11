package main

import (
	"github.com/egoist/mygo/ui"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// The maps' list has the maps and their folders, not what was collected:
// that is 收集's.
func TestListHasMapsOnly(t *testing.T) {
	a, tt := newTestApp(t)
	dir := filepath.Join(a.dir, "项目资料")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	clip, err := a.addClipText("登录模块的接口说明：用户名密码登录、扫码登录", "手动")
	if err != nil {
		t.Fatal(err)
	}
	clip.Folder = "项目资料" // as an earlier version kept it
	a.refreshFiles()
	a.setFolderOpen(dir, true)
	a.showProjects()
	tt.Frame()
	if _, ok := tt.Find("文档 " + clip.Label()); ok || tt.HasText("未归档") {
		t.Fatalf("a document in the maps' list: %q", tt.Texts())
	}
	if !tt.HasText("项目资料") {
		t.Fatalf("no folder: %q", tt.Texts())
	}
	// It is in 收集, as all that is collected.
	a.openList(kindAll, "")
	if hits := a.libHits(); len(hits) != 1 || hits[0].Clip != clip {
		t.Fatalf("收集 has %d", len(hits))
	}
}

// A map dragged onto a folder of the sidebar goes into it.
func TestDragIntoFolder(t *testing.T) {
	a, tt := newTestApp(t)
	dir := filepath.Join(a.dir, "team")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	a.refreshFiles()
	a.showProjects()
	tt.Frame()
	src, ok1 := tt.Find("使用指南")
	dst, ok2 := tt.Find("team")
	if !ok1 || !ok2 {
		t.Fatal("no map or folder")
	}
	tt.Press(src.X+20, src.Y+src.H/2)
	for i := 1; i <= 10; i++ {
		tt.Move(src.X+20, src.Y+src.H/2+(dst.Y-src.Y)*float32(i)/10)
	}
	tt.Release(dst.X+20, dst.Y+dst.H/2)
	tt.Frame()
	if filepath.Dir(a.path) != dir {
		t.Fatalf("map at %q", a.path)
	}
}

// A folder is renamed and moved to the trash from its menu: its maps go
// with it, and what pointed at them.
func TestFolderRenameAndDelete(t *testing.T) {
	a, tt := newTestApp(t)
	plat := a.plat.(*fakePlatform)
	dir := filepath.Join(a.dir, "旧项目")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	a.refreshFiles()
	guide := a.path
	a.newMap() // another map, which stays outside the folder
	a.flush()
	if err := a.open(guide); err != nil {
		t.Fatal(err)
	}
	a.moveMap(a.path, dir)
	a.toggleFavorite(a.path)
	a.showProjects()
	tt.Frame()

	// Renamed from the menu: the open map and 快捷访问 follow.
	row, ok := tt.Find("旧项目")
	if !ok {
		t.Fatalf("no folder row: %q", tt.Texts())
	}
	_ = row
	if err := tt.RightClick("旧项目"); err != nil { // its menu
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("重命名"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.renaming != dir {
		t.Fatalf("renaming %q", a.renaming)
	}
	a.renaming = ""
	a.rename(dir, "归档项目")
	to := filepath.Join(a.dir, "归档项目")
	if filepath.Dir(a.path) != to || !slices.Contains(a.cfg.Favorites, a.path) {
		t.Fatalf("after the rename: %q, %v", a.path, a.cfg.Favorites)
	}
	tt.Frame()
	snapshot(t, tt, "folder-row")

	// To the trash, once confirmed: with its map, another one opened.
	row, _ = tt.Find("归档项目")
	if err := tt.RightClick("归档项目"); err != nil { // a right click opens the same menu
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("移到废纸篓…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.confirmDelete || !tt.HasText("把文件夹「归档项目」移到废纸篓？") || !tt.HasText("里面的 1 张导图会一起移走，可以在废纸篓中找回。") {
		t.Fatalf("not asked: %q", tt.Texts())
	}
	snapshot(t, tt, "folder-delete")
	if err := tt.Click("移到废纸篓"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !slices.Contains(plat.trashed, to) {
		t.Fatalf("trashed %v", plat.trashed)
	}
	if len(a.cfg.Favorites) != 0 || a.path == "" || inside(a.path, to) {
		t.Fatalf("left: favorites %v, open %q", a.cfg.Favorites, a.path)
	}
	if slices.ContainsFunc(a.files, func(f model.FileInfo) bool { return inside(f.Path, to) }) {
		t.Fatal("the folder's maps are still listed")
	}
}

// The maps' directory is beside 项目's cards and beside a map opened: a
// click on a map goes to it, a folder shows its cards; it can be put away.
func TestMapTree(t *testing.T) {
	a, tt := newTestApp(t)
	guide := a.path
	dir := filepath.Join(a.dir, "工作")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	a.newMapIn(dir)
	a.flush()
	other := a.path
	a.renaming = ""
	a.refreshFiles()
	a.setFolderOpen(dir, true)
	tt.Frame()
	if !a.treeShown() || !tt.HasText("目录") || a.sideWidth() != sideWidthOf {
		t.Fatalf("no directory beside the map: %q", tt.Texts())
	}
	snapshot(t, tt, "tree-map")
	// One click from a map to another.
	if err := tt.Click(mapName(guide)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.path != guide || a.libOpen {
		t.Fatalf("not at the map clicked: %q", a.path)
	}
	// By the cards, a folder shows its own.
	a.showProjects()
	tt.Frame()
	if err := tt.Click("工作"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.projFolder != dir || !tt.HasText("1 张导图") {
		t.Fatalf("the folder's cards are not shown: %q", a.projFolder)
	}
	snapshot(t, tt, "tree-projects")
	_ = other
	// Put away, the folders are a row of pills again; 今天 has none.
	if err := tt.Click("收起侧栏"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.treeShown() || !a.cfg.HideTree || !tt.HasText("全部") {
		t.Fatal("the directory is not put away")
	}
	a.cfg.HideTree = false
	a.showLibrary()
	a.libHome = true
	tt.Frame()
	if a.treeShown() {
		t.Fatal("the directory shows in 今天")
	}
}

// The one bar is in the top row over a map too.
func TestBarOverMap(t *testing.T) {
	a, tt := newTestApp(t)
	if err := tt.Click("记一条灵感或任务"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if !a.barOpen || !tt.HasText("加粗") {
		t.Fatalf("no bar over the map: %q", tt.Texts())
	}
	tt.Type("画布上记的一句")
	tt.Frame()
	snapshot(t, tt, "bar-over-map")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.barOpen || len(a.lib.Clips) != 1 || a.lib.Clips[0].Text != "画布上记的一句" {
		t.Fatalf("not kept and put away: open %v, clips %d", a.barOpen, len(a.lib.Clips))
	}
}
