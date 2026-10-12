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

	// Kept in a folder: shown there in the sidebar, beside the maps.
	a.moveClipTo(clip.ID, dir)
	if clip.Folder != "项目资料" {
		t.Fatalf("folder %q", clip.Folder)
	}
	tt.Frame()
	if !tt.HasText(clip.Label()) || !tt.HasText("本导图标签 5") {
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
	a.moveClipTo(clip.ID, "")
	if clip.Folder != "" || len(a.clipsIn(to)) != 0 {
		t.Fatal("not taken out")
	}
}
