package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDocumentsInFolders(t *testing.T) {
	a, tt := newTestApp(t)
	dir := filepath.Join(a.dir, "项目资料")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	a.refreshFiles()
	clip, err := a.addClipText("登录模块的接口说明：用户名密码登录、扫码登录", "手动")
	if err != nil {
		t.Fatal(err)
	}

	// In no folder: under 未归档, first in the folders, closed at first.
	tt.Frame()
	if tt.HasText(clip.Label()) {
		t.Fatal("未归档 is open at first")
	}
	a.unfiledOpen = true
	tt.Frame()
	if len(a.unfiledClips()) != 1 || !tt.HasText("未归档") || !tt.HasText(clip.Label()) {
		t.Fatalf("未归档: %d, %q", len(a.unfiledClips()), tt.Texts())
	}

	// Kept in a folder: shown there in the sidebar, beside the maps.
	a.moveClipTo(clip.ID, dir)
	if clip.Folder != "项目资料" || len(a.unfiledClips()) != 0 {
		t.Fatalf("folder %q", clip.Folder)
	}
	tt.Frame()
	if !tt.HasText(clip.Label()) || tt.HasText("本导图标签 5") { // the map's tags are in the panel
		t.Fatalf("sidebar %q", tt.Texts())
	}
	snapshot(t, tt, "sidebar-docs")

	// A click shows it in 资料.
	if err := tt.Click("文档 " + clip.Label()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.libOpen || a.libSel != clip.ID {
		t.Fatal("the document is not shown")
	}
	snapshot(t, tt, "sidebar-library")

	// 文档 lists text, pictures and files; not maps or links.
	a.libKind = kindDocs
	if hits := a.libHits(); len(hits) != 1 || hits[0].Clip != clip {
		t.Fatalf("documents %d", len(hits))
	}

	// The folder renamed, the document follows it.
	to := filepath.Join(a.dir, "资料夹")
	if err := os.Rename(dir, to); err != nil {
		t.Fatal(err)
	}
	a.moved(dir, to)
	if clip.Folder != "资料夹" || len(a.clipsIn(to)) != 1 {
		t.Fatalf("after the rename: %q", clip.Folder)
	}
	// A folder gone, its documents are under 未归档 again.
	if err := os.RemoveAll(to); err != nil {
		t.Fatal(err)
	}
	if len(a.unfiledClips()) != 1 {
		t.Fatal("a document of a folder gone is not under 未归档")
	}
	a.moveClipTo(clip.ID, "")
	if clip.Folder != "" || len(a.clipsIn(to)) != 0 {
		t.Fatal("not taken out")
	}
}

// A map, and a document, dragged onto a folder of the sidebar go into it.
func TestDragIntoFolder(t *testing.T) {
	a, tt := newTestApp(t)
	dir := filepath.Join(a.dir, "team")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	clip, err := a.addClipText("季度评价模板", "手动")
	if err != nil {
		t.Fatal(err)
	}
	a.refreshFiles()
	a.unfiledOpen = true
	tt.Frame()
	drag := func(from, to string) {
		t.Helper()
		src, ok1 := tt.Find(from)
		dst, ok2 := tt.Find(to)
		if !ok1 || !ok2 {
			t.Fatalf("no %q or %q", from, to)
		}
		tt.Press(src.X+20, src.Y+src.H/2)
		for i := 1; i <= 10; i++ {
			tt.Move(src.X+20, src.Y+src.H/2+(dst.Y-src.Y)*float32(i)/10)
		}
		tt.Release(dst.X+20, dst.Y+dst.H/2)
		tt.Frame()
	}
	drag("文档 "+clip.Label(), "team")
	if clip.Folder != "team" {
		t.Fatalf("document's folder %q", clip.Folder)
	}
	drag("使用指南", "team")
	if filepath.Dir(a.path) != dir {
		t.Fatalf("map at %q", a.path)
	}
}
