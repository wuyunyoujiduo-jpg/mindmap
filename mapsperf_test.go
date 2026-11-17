package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// What the maps cost, on maps of a size: each a test that counts what a
// frame or an edit does, and a benchmark that times it.

// manyMaps writes maps of topics each into the maps folder, a topic of
// each linking to a topic of the first, and opens the first with that
// topic chosen, its page showing.
func manyMaps(tb testing.TB, a *app, maps, topics int) *model.Node {
	tb.Helper()
	var first string
	var chosen *model.Node
	for m := range maps {
		f := model.NewFile(fmt.Sprintf("项目 %d", m))
		d := f.Sheets[0]
		for i := 0; i < topics; i += 50 {
			b := model.NewNode(fmt.Sprintf("模块 %d-%d", m, i))
			for j := range 49 {
				c := model.NewNode(fmt.Sprintf("功能 %d-%d-%d 缓存 索引 接口", m, i, j))
				c.Note = "实现要点：先写接口，再补测试。"
				b.Children = append(b.Children, c)
			}
			b.Collapsed = i > 0 // a map of a thousand topics is not all open
			d.Root.Children = append(d.Root.Children, b)
		}
		path := filepath.Join(a.dir, model.FileName(fmt.Sprintf("项目 %d", m)))
		if m == 0 {
			first, chosen = path, d.Root.Children[0].Children[0]
		} else {
			n := d.Root.Children[0].Children[1]
			n.Links = []string{"mindmap:项目 0#" + chosen.ID}
		}
		if err := f.Save(path); err != nil {
			tb.Fatal(err)
		}
	}
	a.refreshFiles()
	if err := a.open(first); err != nil {
		tb.Fatal(err)
	}
	a.mode, a.panelOff = viewMap, false
	a.selectNode(chosen.ID)
	n, _ := a.doc.Find(chosen.ID)
	return n
}

// With a topic's page showing, a change of the open map reads that map
// again for the links' index and no other, and looks for what the topic
// may be about only when its title has changed and the writing rests:
// not at a key, nor as a thing is dragged. What links to the topic
// follows the links, of this map and of the others.
func TestPanelFollowsWhatItShows(t *testing.T) {
	a, tt := newTestApp(t)
	n := manyMaps(t, a, 12, 200)
	at := testNow
	a.now = func() time.Time { return at }
	rest := func() { // the writing rests, and a frame comes
		at = at.Add(time.Second)
		tt.Frame()
	}
	rest()
	if got := len(a.backlinks(n.ID)); got != 11 {
		t.Fatalf("%d topics link to the one chosen, of 11", got)
	}
	if !tt.HasText("被 11 处链接") {
		t.Fatalf("its page does not say what links to it: %q", tt.Texts())
	}

	// A key typed into its title.
	refScans, relatedRuns, othersIndexed = 0, 0, 0
	a.mutate("title:"+n.ID, func() { n.Title += "字" })
	tt.Frame()
	if refScans != 1 || relatedRuns != 0 || othersIndexed != 0 {
		t.Fatalf("a key typed: %d maps read for the links, what it is about looked for %d times, the folder indexed %d times",
			refScans, relatedRuns, othersIndexed)
	}
	// The writing rests: looked for once, in the index there is.
	rest()
	tt.Frame()
	if relatedRuns != 1 || othersIndexed != 0 {
		t.Fatalf("the writing rested: looked for %d times, the folder indexed %d times", relatedRuns, othersIndexed)
	}
	// A floating topic dragged, a frame a change; and a note written.
	var fl *model.Node
	a.mutate("", func() { fl = a.doc.AddFloating("漂着的", model.Point{X: 300, Y: 200}) })
	rest()
	refScans, relatedRuns = 0, 0
	for range 10 {
		a.mutate("drag:"+fl.ID, func() { fl.Pos.X += 4 })
		at = at.Add(16 * time.Millisecond)
		tt.Frame()
	}
	a.mutate("note:"+n.ID, func() { n.Note += "又一句" })
	rest()
	if relatedRuns != 0 || othersIndexed != 0 || refScans != 11 {
		t.Fatalf("a drag and a note: looked for %d times, the folder indexed %d times, %d maps read", relatedRuns, othersIndexed, refScans)
	}

	// A link of another map taken off: that map alone is read again, and
	// the page says one fewer.
	other := filepath.Join(a.dir, model.FileName("项目 5"))
	f, err := model.Load(other)
	if err != nil {
		t.Fatal(err)
	}
	f.Walk(func(m *model.Node) { m.Links = nil })
	if err := f.Save(other); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Hour)
	_ = os.Chtimes(other, later, later)
	a.refreshFiles()
	refScans = 0
	rest()
	if got := len(a.backlinks(n.ID)); got != 10 || refScans != 1 {
		t.Fatalf("a link of another map taken off: %d link to it, %d maps read", got, refScans)
	}
	// A topic of this map links to it, and is retitled: said so at once.
	sib, _ := a.doc.Find(a.doc.Root.Children[0].Children[2].ID)
	a.addLink(sib, a.nodeLinkTo(a.path, n.ID))
	a.mutate("title:"+sib.ID, func() { sib.Title = "新名字的主题" })
	tt.Frame()
	back := a.backlinks(n.ID)
	if len(back) != 11 || !slices.ContainsFunc(back, func(b backlink) bool { return b.title == "新名字的主题" }) {
		t.Fatalf("a link of this map: %d link to it, none by its new title", len(back))
	}
	if !tt.HasText("被 11 处链接") {
		t.Fatal("the page does not follow")
	}
}

// A key typed into the title of the topic whose page shows, in a folder
// of 60 maps of 1,000 topics.
func BenchmarkPanelTyping(b *testing.B) {
	a, tt := newTestApp(b)
	n := manyMaps(b, a, 60, 1000)
	tt.Frame()
	tt.Frame()
	for b.Loop() {
		a.mutate("title:"+n.ID, func() { n.Title += "字" })
		tt.Frame()
	}
}

// The same, and the writing rests after each key: what the topic may be
// about is looked for again.
func BenchmarkPanelTypingRested(b *testing.B) {
	a, tt := newTestApp(b)
	n := manyMaps(b, a, 60, 1000)
	at := testNow
	a.now = func() time.Time { return at }
	a.cfg.MapAutoOff = true // no analysis asked for as the clock goes on
	tt.Frame()
	tt.Frame()
	for b.Loop() {
		a.mutate("title:"+n.ID, func() { n.Title += "字" })
		at = at.Add(700 * time.Millisecond)
		tt.Frame()
	}
}

// wideBranch opens a new map whose one main topic has n children, and
// which shows that topic: its children are far above and below the window.
func wideBranch(a *app, n int) *model.Node {
	a.newMap()
	a.commitEditIfAny()
	var hub *model.Node
	a.mutate("", func() {
		hub, _ = a.doc.AddChild(a.doc.Root.ID, "很宽的主题")
		for i := range n {
			hub.Children = append(hub.Children, model.NewNode(fmt.Sprintf("子主题 %d", i)))
		}
	})
	a.mode, a.selected = viewMap, hub.ID
	return hub
}

// A topic with five hundred children: its branches, each longer than a
// path may be, are drawn where they show, a few parts each, and not
// sampled whole.
func TestWideBranchDrawsWhatShows(t *testing.T) {
	a, tt := newTestApp(t)
	hub := wideBranch(a, 500)
	tt.Frame()
	tt.Frame()
	curveParts = 0
	a.cam().X++ // the canvas is drawn again as it is moved
	tt.Frame()
	// Whole, the five hundred curves were some 900,000 points a frame.
	if curveParts < 500 || curveParts > 3000 {
		t.Fatalf("a frame drew %d parts of the %d branches", curveParts, len(hub.Children))
	}
	// And they show: a branch's color, on the way from the topic to its
	// children's column.
	box := a.boxes.Boxes[hub.ID]
	child := a.boxes.Boxes[hub.Children[0].ID]
	cam, img := a.cam(), tt.Image()
	x := int(a.vp.X + cam.X + (box.X+box.W+child.X)/2*cam.Zoom)
	white, drawn := img.RGBAAt(x, int(a.vp.Y)+2), false
	for y := int(a.vp.Y); y < int(a.vp.Y+a.vp.H); y++ {
		drawn = drawn || img.RGBAAt(x, y) != white
	}
	if !drawn {
		t.Fatal("no branch between the topic and its children")
	}
	snapshot(t, tt, "map-wide-branch")
}

func BenchmarkMapWideBranch(b *testing.B) {
	a, tt := newTestApp(b)
	wideBranch(a, 500)
	tt.Frame()
	tt.Frame()
	for b.Loop() {
		a.cam().X++ // a frame of the canvas being moved
		tt.Frame()
	}
}

// longOutline opens a new map of about n topics, all open, and shows its
// outline: fifty main topics, the rest their children.
func longOutline(a *app, n int) {
	a.newMap()
	a.commitEditIfAny()
	a.mutate("", func() {
		for i := range 50 {
			b := model.NewNode(fmt.Sprintf("第 %d 章", i))
			for j := range n/50 - 1 {
				b.Children = append(b.Children, model.NewNode(fmt.Sprintf("第 %d 章的第 %d 行", i, j)))
			}
			a.doc.Root.Children = append(a.doc.Root.Children, b)
		}
	})
	a.mode = viewOutline
}

// An outline of five thousand lines builds the lines in and near the
// view, and its keys work on a line far down: the focus sent there
// scrolls to it, it is written in, Return adds a line after it, Tab
// indents, the arrows go on, and a line dragged from selects lines whole.
func TestLongOutline(t *testing.T) {
	a, tt := newTestApp(t)
	longOutline(a, 5000)
	tt.Frame()
	tt.Frame()
	rows := outlineRows(a.doc)
	if len(rows) != 5000 {
		t.Fatalf("%d lines", len(rows))
	}
	outlineBuilt = 0
	a.mutate("", func() { a.doc.Root.Children[0].Title = "第一章" }) // a change: the page is built again
	tt.Frame()
	if outlineBuilt == 0 || outlineBuilt > 200 {
		t.Fatalf("a frame built %d of %d lines", outlineBuilt, len(rows))
	}
	if !tt.HasText("大纲 第一章") || tt.HasText("大纲 "+rows[4001].node.Title) {
		t.Fatal("the first lines do not show, or a line far down is built")
	}

	// The focus sent to a line far down: it is scrolled to, and written in.
	far := rows[4001].node
	a.outlineFocus = far.ID
	tt.Frame()
	tt.Frame()
	box, shown := tt.Find("大纲 " + far.Title)
	if !shown || box.Y < 0 || box.Y+box.H > 800 {
		t.Fatalf("the line the focus went to is not in view: %v %v", shown, box)
	}
	if a.selected != far.ID {
		t.Fatalf("it has not the focus: %q is chosen", a.selected)
	}
	tt.Key(0, ui.KeyEnd)
	tt.Type("，写在这里")
	if !strings.HasSuffix(far.Title, "行，写在这里") {
		t.Fatalf("written into %q", far.Title)
	}
	// Scrolled away from, the line with the focus is written in still.
	tt.Scroll(640, 400, 0, -4000)
	tt.Frame()
	tt.Frame()
	if box, _ = tt.Find("大纲 " + far.Title); box.Y >= 0 && box.Y < 800 && box.H > 0 {
		t.Fatalf("the wheel did not scroll the line away: %v", box)
	}
	tt.Type("。")
	if !strings.HasSuffix(far.Title, "写在这里。") {
		t.Fatalf("scrolled away, the line reads %q", far.Title)
	}
	// Return adds a line after it; Tab puts it under it; Shift+Tab back.
	_, parent := a.doc.Find(far.ID)
	at := slices.Index(parent.Children, far)
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	tt.Frame()
	added := parent.Children[at+1]
	if a.outlineFocus != added.ID || added.Title != "" {
		t.Fatalf("no line added after it: the focus is %q", a.outlineFocus)
	}
	tt.Type("新的一行")
	if added.Title != "新的一行" {
		t.Fatalf("the new line reads %q", added.Title)
	}
	tt.Key(0, ui.KeyTab)
	tt.Frame()
	if len(far.Children) != 1 || far.Children[0] != added {
		t.Fatal("Tab did not put the line under the one above")
	}
	tt.Type("，缩进后还在写")
	if added.Title != "新的一行，缩进后还在写" {
		t.Fatalf("after Tab the line reads %q", added.Title)
	}
	tt.Key(ui.Shift, ui.KeyTab)
	tt.Frame()
	if _, p := a.doc.Find(added.ID); p != parent {
		t.Fatal("Shift+Tab did not bring it back")
	}
	// The arrows, line by line, further than the window is high.
	for range 60 {
		tt.Key(0, ui.KeyDown)
		tt.Frame()
	}
	rows = outlineRows(a.doc)
	down := rows[a.outlineIndex(rows, added.ID)+60].node
	if a.outlineFocus != down.ID {
		t.Fatalf("sixty lines down, the focus is on %q", a.outlineFocus)
	}
	if box, shown = tt.Find("大纲 " + down.Title); !shown || box.Y < 0 || box.Y+box.H > 800 {
		t.Fatalf("the line the arrows went to is not in view: %v %v", shown, box)
	}
	tt.Type("到了")
	if !strings.HasPrefix(down.Title, "到了") && !strings.HasSuffix(down.Title, "到了") {
		t.Fatalf("typed after the arrows: %q", down.Title)
	}
	// Shift and the arrows select lines whole; Escape goes back to the line.
	tt.Key(ui.Shift, ui.KeyDown)
	tt.Frame()
	tt.Key(ui.Shift, ui.KeyDown)
	tt.Frame()
	if !a.outlineBlock || len(a.multi) != 2 {
		t.Fatalf("Shift and the arrows: block %v, %d more lines", a.outlineBlock, len(a.multi))
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.outlineBlock {
		t.Fatal("Escape left the lines selected")
	}
	// A drag from a line onto the third above it selects the four.
	i := a.outlineIndex(rows, a.outlineFocus)
	b1, b2 := a.outlineRowAt[rows[i].node.ID], a.outlineRowAt[rows[i-3].node.ID]
	if b1.H == 0 || b2.H == 0 {
		t.Fatalf("the lines by the focus are not built: %v %v", b1, b2)
	}
	tt.Press(b1.X+b1.W-40, b1.Y+b1.H/2)
	tt.Move(b1.X+b1.W-40, b2.Y+b2.H/2)
	tt.Release(b1.X+b1.W-40, b2.Y+b2.H/2)
	tt.Frame()
	for _, r := range rows[i-3 : i+1] {
		if !a.isSelected(r.node.ID) {
			t.Fatalf("%q not selected by the drag", r.node.Title)
		}
	}
	if a.isSelected(rows[i-4].node.ID) || a.isSelected(rows[i+1].node.ID) || a.outlineFocus != outlineBlockFocus {
		t.Fatalf("the drag selected past its ends, or the focus is %q", a.outlineFocus)
	}
	if len(a.outlineRowAt) > 200 {
		t.Fatalf("%d lines' places kept", len(a.outlineRowAt))
	}
	// The focus sent back to the heading, from far down.
	tt.Key(0, ui.KeyEscape)
	a.outlineFocus = a.doc.Root.ID
	tt.Frame()
	tt.Frame()
	tt.Type("改")
	if !strings.Contains(a.doc.Root.Title, "改") {
		t.Fatalf("the heading, come back to, reads %q", a.doc.Root.Title)
	}
	snapshot(t, tt, "outline-long")
}

// A frame of an outline of five thousand lines, with a change made.
func BenchmarkOutline5000(b *testing.B) {
	a, tt := newTestApp(b)
	longOutline(a, 5000)
	tt.Frame()
	tt.Frame()
	n := a.doc.Root.Children[0]
	for b.Loop() {
		a.mutate("title:"+n.ID, func() { n.Title += "字" })
		tt.Frame()
	}
}

// The steps of the maps not open are kept for the few left last.
func TestHistoriesOfMapsLeft(t *testing.T) {
	a, tt := newTestApp(t)
	var paths []string
	for i := range histMaps + 3 {
		a.mutate("", func() { a.doc.AddChild(a.doc.Root.ID, fmt.Sprint("改动 ", i)) })
		paths = append(paths, a.path)
		a.newMap()
		tt.Frame()
	}
	a.flush()
	if len(a.hists) != histMaps+1 || len(a.histAt) != histMaps+1 {
		t.Fatalf("steps kept for %d maps (%d), of %d left and the one open", len(a.hists), len(a.histAt), histMaps)
	}
	if a.hists[paths[0]] != nil || a.hists[paths[len(paths)-1]] == nil {
		t.Fatal("not the maps left last kept their steps")
	}
	// A map left last is come back to with its steps.
	last := paths[len(paths)-1]
	if err := a.open(last); err != nil {
		t.Fatal(err)
	}
	n := len(a.doc.Root.Children)
	a.undo()
	if len(a.doc.Root.Children) != n-1 {
		t.Fatal("the map come back to had lost its steps")
	}
}
