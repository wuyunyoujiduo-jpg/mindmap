package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// newMapFile writes a map of a root and one child into the maps folder.
func newMapFile(t *testing.T, a *app, name, child string) (path string, root, kid *model.Node) {
	t.Helper()
	f := model.NewFile(name)
	d := f.Sheets[0]
	kid, _ = d.AddChild(d.Root.ID, child)
	path = filepath.Join(a.dir, model.FileName(name))
	if err := f.Save(path); err != nil {
		t.Fatal(err)
	}
	a.refreshFiles()
	return path, d.Root, kid
}

// Links follow a node to a renamed map; a whole map has a link of its own;
// the index knows who links to what.
func TestLinksAcrossMaps(t *testing.T) {
	a, tt := newTestApp(t)
	other, root, kid := newMapFile(t, a, "解析器", "词法")
	n := find(t, a, "前端")
	a.addLink(n, a.nodeLinkTo(other, kid.ID))
	a.addLink(n, a.mapLink(other))
	if icon, label := a.linkLabel(n.Links[1]); icon != "🗺" || label != "解析器" {
		t.Fatalf("map link %s %s", icon, label)
	}
	if bl := a.refIndex().to["map:"+other]; len(bl) != 2 {
		t.Fatalf("map backlinks %+v", bl)
	}
	// Renamed in Finder: the link still finds its node.
	renamed := filepath.Join(a.dir, model.FileName("解析器 v2"))
	if err := os.Rename(other, renamed); err != nil {
		t.Fatal(err)
	}
	a.refreshFiles()
	if _, target, _ := a.parseLink(n.Links[0]); target != renamed+"#"+kid.ID {
		t.Fatalf("target %q", target)
	}
	if _, label := a.linkLabel(n.Links[0]); label != "解析器 v2 › 词法" {
		t.Fatalf("label %q", label)
	}
	a.openLink(n.Links[0])
	if a.path != renamed || a.selected != kid.ID {
		t.Fatalf("opened %q at %q", a.path, a.selected)
	}
	_ = root
	tt.Frame()
}

// A node moved to another map keeps its ID; the other map's undo steps
// are dropped; a document or a map dropped on a node is linked to it.
func TestMoveKeepsIDsAndDropMaterial(t *testing.T) {
	a, tt := newTestApp(t)
	other, _, kid := newMapFile(t, a, "归档", "旧需求")
	n := find(t, a, "前端")
	id := n.ID
	a.hists[other] = nil
	a.selectNode(id)
	a.moveToMap(other, "", kid.ID)
	f, _ := model.Load(other)
	if f.Node(id) == nil {
		t.Fatal("the node lost its ID")
	}
	if _, ok := a.hists[other]; ok {
		t.Fatal("the other map's undo steps kept")
	}
	if vs := a.versions.List(other); len(vs) == 0 {
		t.Fatal("no version of the other map")
	}

	clip, err := a.addClipText("词法分析笔记", "手动")
	if err != nil {
		t.Fatal(err)
	}
	target := find(t, a, "后端")
	if err := a.attachTo(a.path, target.ID, clipScheme+clip.ID, "资料"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(target.Links) != 1 || target.Links[0] != clipScheme+clip.ID {
		t.Fatalf("links %v", target.Links)
	}
	if bl := a.clipBacklinks(clip.ID); len(bl) != 1 || bl[0].id != target.ID {
		t.Fatalf("backlinks %+v", bl)
	}
}

// A web address written in a topic's title is a link: shown as one, and
// opened by the node's mark, without being added to its links.
func TestTitleLinks(t *testing.T) {
	for title, want := range map[string][]string{
		"访销订单详情页 http://pms.example.com/zentao/bug-view-241795.html": {"http://pms.example.com/zentao/bug-view-241795.html"},
		"见 https://a.example.com/x?y=1。然后":                           {"https://a.example.com/x?y=1"},
		"(https://a.example.com/x), 和 http://b.example.com.":         {"https://a.example.com/x", "http://b.example.com"},
		"没有链接 a://b http://":                                         nil,
	} {
		var got []string
		for _, r := range titleLinks(title) {
			got = append(got, title[r[0]:r[1]])
		}
		if !slices.Equal(got, want) {
			t.Errorf("%q: %q", title, got)
		}
	}
	a, tt := newTestApp(t)
	fp := a.plat.(*fakePlatform)
	n := find(t, a, "前端")
	u := "http://pms.example.com/zentao/bug-view-241795.html"
	a.mutate("", func() { n.Title = "访销订单详情页 " + u })
	a.selected, a.reveal = n.ID, true
	tt.Frame()
	tt.Frame()
	snapshot(t, tt, "title-link")
	if err := tt.Click("打开链接"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(n.Links) != 0 || !slices.Contains(fp.opened, u) {
		t.Fatalf("links %v opened %v", n.Links, fp.opened)
	}
}
