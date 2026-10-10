package xmind

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

func pngOf(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func importBytes(t *testing.T, data []byte, assets string) *model.File {
	t.Helper()
	f, err := Import(bytes.NewReader(data), int64(len(data)), assets)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.assets")
	os.MkdirAll(src, 0o755)
	os.WriteFile(filepath.Join(src, "pic.png"), pngOf(40, 30), 0o644)

	d := model.New("项目")
	d.Title = "画布 A"
	d.Structure = model.StructureLogic
	d.Style.Scheme = "ocean"
	a, _ := d.AddChild(d.Root.ID, "功能")
	a.Note = "备注"
	a.Tags = []string{"核心"}
	a.Markers = []string{"priority:2", "flag:red", "symbol:heart"}
	a.Image = &model.Image{File: "pic.png", W: 40, H: 30}
	due := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	b, _ := d.AddChild(a.ID, "登录")
	b.Task = &model.Task{Status: model.StatusDoing, Priority: model.PriorityHigh, Due: &due}
	b.Style = &model.NodeStyle{Bold: true}
	a.Collapsed = true
	f := &model.File{Version: model.FormatVersion, Sheets: []*model.Document{d, model.New("第二")}}

	var buf bytes.Buffer
	if err := Export(&buf, f, src); err != nil {
		t.Fatal(err)
	}
	// What XMind reads: content.json, with its markers and the picture.
	z, _ := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	names := map[string]bool{}
	for _, f := range z.File {
		names[f.Name] = true
	}
	for _, n := range []string{"content.json", "metadata.json", "manifest.json", "resources/pic.png"} {
		if !names[n] {
			t.Fatalf("no %s in %v", n, names)
		}
	}
	rc, _ := z.Open("content.json")
	var sheets []sheet
	json.NewDecoder(rc).Decode(&sheets)
	ft := sheets[0].RootTopic.Children.Attached[0]
	var ids []string
	for _, m := range ft.Markers {
		ids = append(ids, m.MarkerID)
	}
	if !slices.Equal(ids, []string{"priority-2", "flag-red"}) || ft.Branch != "folded" ||
		sheets[0].RootTopic.StructureClass != "org.xmind.ui.logic.right" {
		t.Fatalf("topic %+v markers %v", ft, ids)
	}
	if m := ft.Children.Attached[0].Markers; len(m) != 1 || m[0].MarkerID != "task-half" {
		t.Fatalf("task marker %v", m)
	}

	// Back again, all is kept.
	assets := filepath.Join(dir, "back.assets")
	g := importBytes(t, buf.Bytes(), assets)
	if len(g.Sheets) != 2 || g.Sheets[0].Title != "画布 A" || g.Sheets[0].Structure != model.StructureLogic ||
		g.Sheets[0].Style.Scheme != "ocean" {
		t.Fatalf("sheets %+v", g.Sheets[0])
	}
	ga := g.Sheets[0].Root.Children[0]
	if ga.Title != "功能" || ga.Note != "备注" || !slices.Equal(ga.Tags, []string{"核心"}) ||
		!slices.Equal(ga.Markers, a.Markers) || !ga.Collapsed {
		t.Fatalf("node %+v", ga)
	}
	if ga.Image == nil || ga.Image.W != 40 || ga.Image.H != 30 {
		t.Fatalf("image %+v", ga.Image)
	}
	if _, err := os.Stat(filepath.Join(assets, ga.Image.File)); err != nil {
		t.Fatal(err)
	}
	gb := ga.Children[0]
	if gb.Task == nil || gb.Task.Status != model.StatusDoing || !gb.Task.Due.Equal(due) || gb.Style == nil || !gb.Style.Bold {
		t.Fatalf("task %+v style %+v", gb.Task, gb.Style)
	}
	if gb.ID == b.ID {
		t.Fatal("IDs are kept; they should be new")
	}
}

func archive(t *testing.T, files map[string][]byte) []byte {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for name, data := range files {
		w, _ := z.Create(name)
		w.Write(data)
	}
	z.Close()
	return buf.Bytes()
}

// A file as XMind 2020 and later write it.
func TestImportXMindZen(t *testing.T) {
	content := `[{"id":"s1","class":"sheet","title":"画布 1","rootTopic":{"id":"r","class":"topic","title":"中心主题",
	"structureClass":"org.xmind.ui.map.clockwise","children":{"attached":[
	  {"id":"a","title":"分支 1","labels":["L1"],"markers":[{"markerId":"priority-1"},{"markerId":"task-done"},{"markerId":"smiley-smile"}],
	   "notes":{"plain":{"content":"笔记\n"},"realHTML":{"content":"<div>笔记</div>"}},
	   "image":{"src":"xap:resources/abc.png","width":100,"height":80},
	   "children":{"attached":[{"id":"a1","title":"子主题"}]}},
	  {"id":"b","title":"分支 2","branch":"folded"}],
	 "detached":[{"id":"f","title":"自由主题"}]}}}]`
	data := archive(t, map[string][]byte{"content.json": []byte(content), "resources/abc.png": pngOf(10, 8)})
	f := importBytes(t, data, filepath.Join(t.TempDir(), "x.assets"))
	root := f.Sheets[0].Root
	var titles []string
	for _, c := range root.Children {
		titles = append(titles, c.Title)
	}
	if root.Title != "中心主题" || !slices.Equal(titles, []string{"分支 1", "分支 2", "自由主题"}) {
		t.Fatalf("root %q children %v", root.Title, titles)
	}
	a := root.Children[0]
	if a.Note != "笔记" || !slices.Equal(a.Tags, []string{"L1"}) || !slices.Equal(a.Markers, []string{"priority:1", "task:6"}) ||
		a.Image == nil || a.Image.W != 10 || len(a.Children) != 1 {
		t.Fatalf("branch %+v", a)
	}
	if root.Children[1].Collapsed { // folded, but with nothing to fold
		t.Fatal("collapsed without children")
	}
}

// A file as XMind 8 wrote it.
func TestImportXMind8(t *testing.T) {
	content := `<?xml version="1.0" encoding="UTF-8" standalone="no"?>
<xmap-content xmlns="urn:xmind:xmap:xmlns:content:2.0" xmlns:xhtml="http://www.w3.org/1999/xhtml" version="2.0">
<sheet id="s"><topic id="r" structure-class="org.xmind.ui.logic.right"><title>旧版中心</title>
<children><topics type="attached">
<topic id="a"><title>旧分支</title><labels><label>标</label></labels>
<marker-refs><marker-ref marker-id="flag-red"/></marker-refs>
<notes><plain>旧笔记</plain></notes>
<xhtml:img xhtml:src="xap:attachments/p.png" svg:height="20" svg:width="30" xmlns:svg="http://www.w3.org/2000/svg"/>
<children><topics type="attached"><topic id="a1"><title>孙</title></topic></topics></children>
</topic></topics></children></topic><title>旧画布</title></sheet></xmap-content>`
	data := archive(t, map[string][]byte{"content.xml": []byte(content), "attachments/p.png": pngOf(30, 20)})
	f := importBytes(t, data, filepath.Join(t.TempDir(), "x.assets"))
	d := f.Sheets[0]
	if d.Title != "旧画布" || d.Root.Title != "旧版中心" || d.Structure != model.StructureLogic {
		t.Fatalf("sheet %+v", d)
	}
	a := d.Root.Children[0]
	if a.Title != "旧分支" || a.Note != "旧笔记" || !slices.Equal(a.Tags, []string{"标"}) ||
		!slices.Equal(a.Markers, []string{"flag:red"}) || a.Image == nil || len(a.Children) != 1 {
		t.Fatalf("branch %+v", a)
	}
}

func TestNotXMind(t *testing.T) {
	data := []byte("not a zip")
	if _, err := Import(bytes.NewReader(data), int64(len(data)), ""); err != ErrNotXMind {
		t.Fatal(err)
	}
}
