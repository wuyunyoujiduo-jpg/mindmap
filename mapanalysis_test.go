package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

func TestMapAnalysis(t *testing.T) {
	a, tt := newTestApp(t)
	fake := &fakeClaude{replies: []string{claudeText(`{"title": "x", "desc": "一个做登录和权限的项目",
		"summary": "梳理了登录、搜索和性能优化。", "category": "技术", "tags": ["项目"],
		"keywords": ["登录"], "synonyms": ["单点登录", "SSO"], "analysis": "- 进展过半", "text": ""}`)}}
	a.sendAI = fake.send
	a.cfg.AIKey = "sk-ant-test"
	path := a.path
	mapBefore, _ := os.ReadFile(path)

	// Analyzed from the canvas panel; the map file is not touched.
	a.panelTab = tabCanvas
	tt.Frame()
	if err := tt.Click("分析导图"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	an := a.analysisOf(path)
	if an == nil || an.Desc != "一个做登录和权限的项目" || an.Hash != a.file.ContentHash() {
		t.Fatalf("analysis %+v", an)
	}
	if !tt.HasText("一个做登录和权限的项目") || !tt.HasText("重新分析") {
		t.Fatalf("not shown: %q", tt.Texts())
	}
	snapshot(t, tt, "map-analysis")
	if after, _ := os.ReadFile(path); string(after) != string(mapBefore) {
		t.Fatal("the map file changed")
	}
	prompt := fake.requests[0].Messages[0].Content[0].OfText.Text
	if !strings.Contains(prompt, "导图") || !strings.Contains(prompt, "## 画布：") {
		t.Fatalf("prompt %q", prompt)
	}

	// One 资料: the sidebar's 资料 lists the maps with the clips, and the
	// categories are of both.
	if err := tt.Click("资料"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.libOpen || a.libKind != 0 || !tt.HasText("分类 技术") {
		t.Fatalf("资料: open %v kind %d, %q", a.libOpen, a.libKind, tt.Texts())
	}
	if hits := a.libHits(); len(hits) == 0 || !slices.ContainsFunc(hits, func(h library.Hit) bool { return h.Clip.Kind == library.KindMap }) {
		t.Fatal("the maps are not listed with the clips")
	}
	snapshot(t, tt, "materials")
	if err := tt.Click("分类 技术"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.libCat != "技术" || len(a.libHits()) != 1 {
		t.Fatalf("category %q: %d", a.libCat, len(a.libHits()))
	}
	a.libCat = ""

	// Found by a synonym, in the library, its 导图 filter and the palette.
	a.libOpen, a.libQuery = true, "SSO"
	hits := a.libHits()
	if len(hits) == 0 || hits[0].Clip.Kind != library.KindMap {
		t.Fatalf("library hits %+v", hits)
	}
	a.libQuery, a.libKind = "", 5
	if hits := a.libHits(); len(hits) != len(a.files) {
		t.Fatalf("maps listed %d of %d", len(hits), len(a.files))
	}
	tt.Frame()
	snapshot(t, tt, "library-maps")
	if err := tt.Click("导图 " + mapName(path)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.libOpen || a.path != path {
		t.Fatal("the map row did not open the map")
	}
	a.paletteMode, a.paletteQuery = paletteAll, "单点登录"
	found := false
	for _, it := range a.paletteItems() {
		found = found || it.kind == "导图" && it.title == mapName(path)
	}
	if !found {
		t.Fatalf("palette %+v", a.paletteItems())
	}

	// The AI chat finds and reads maps.
	text, n := a.searchMapsText("SSO", 8)
	if n == 0 || !strings.Contains(text, mapName(path)) {
		t.Fatalf("search_maps %q", text)
	}
	read, _ := a.readMapText(a.mapKey(path))
	if !strings.Contains(read, "## 画布：") || !strings.Contains(read, "AI 摘要") {
		t.Fatalf("read_map %q", read)
	}

	// Renamed in the app, the analysis follows; renamed in Finder, the map
	// takes it back by its content.
	a.flush()
	moved := filepath.Join(filepath.Dir(path), "改名"+model.Ext)
	if err := os.Rename(path, moved); err != nil {
		t.Fatal(err)
	}
	a.moved(path, moved)
	if a.analysisOf(moved) == nil || a.analysisOf(path) != nil {
		t.Fatal("did not follow the rename")
	}
	finder := filepath.Join(filepath.Dir(path), "访达改名"+model.Ext)
	if err := os.Rename(moved, finder); err != nil {
		t.Fatal(err)
	}
	a.refreshFiles()
	a.mapClips()
	if a.analysisOf(finder) == nil || a.analysisOf(moved) != nil {
		t.Fatal("not taken back after a rename in Finder")
	}
	entries, _ := os.ReadDir(a.mapMetaDir())
	if len(entries) != 1 {
		t.Fatalf("%d analyses kept", len(entries))
	}
}
