package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/diagram"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// What the reading of 0.105.0 found of the maps (docs/审查清单.md, 三),
// mended: each test fails without its mending.

// branchWithSummary makes a topic with children and a summary, whose topic
// has a task under it.
func branchWithSummary(t *testing.T, a *app, kids ...string) (p, sum *model.Node) {
	t.Helper()
	a.mutate("", func() {
		p, _ = a.doc.AddChild(a.doc.Root.ID, "要概括的")
		for _, k := range kids {
			_, _ = a.doc.AddChild(p.ID, k)
		}
		sum = p.AddSummary("小结")
		c, _ := a.doc.AddChild(sum.ID, "小结下的任务")
		c.Task = &model.Task{Status: model.StatusTodo}
	})
	return p, sum
}

// The last child of a topic with a summary deleted: the summary's topic,
// and what is under it, was on no canvas, outline nor export, though
// still in the file. It is a child of the topic now, said so, with 撤销.
func TestSummaryStaysSeenWithoutChildren(t *testing.T) {
	a, tt := newTestApp(t)
	p, sum := branchWithSummary(t, a, "唯一的子主题")
	tt.Frame()
	a.selected, a.multi = p.Children[0].ID, nil
	a.deleteSelected()
	tt.Frame()
	if p.Summary != nil || len(p.Children) != 1 || p.Children[0] != sum {
		t.Fatalf("the summary's topic is not a child: summary %v, children %d", p.Summary, len(p.Children))
	}
	if _, ok := a.boxes.Boxes[sum.ID]; !ok {
		t.Fatal("the summary's topic is not on the canvas")
	}
	if !strings.Contains(model.Markdown(a.doc), "小结下的任务") {
		t.Fatal("not in the Markdown")
	}
	if !strings.Contains(a.notice, "小结") || a.noticeUndo == nil {
		t.Fatalf("not said: %q", a.notice)
	}
	a.noticeUndo()
	tt.Frame()
	p, _ = a.doc.Find(p.ID)
	if p == nil || p.SummaryTopic() == nil || len(p.Children) != 1 || p.Children[0].Title != "唯一的子主题" {
		t.Fatal("撤销 did not bring the child and the summary back")
	}

	// Moved away, the last child leaves the summary a child too; and one
	// of two leaves it a summary.
	other, _ := a.doc.AddChild(a.doc.Root.ID, "别处")
	a.moveNode(p.Children[0].ID, other.ID)
	if p.Summary != nil || len(p.Children) != 1 || p.Children[0].Title != "小结" {
		t.Fatal("moved away: the summary's topic is not a child")
	}
	q, _ := branchWithSummary(t, a, "甲", "乙")
	a.selected, a.multi = q.Children[0].ID, nil
	a.deleteSelected()
	if q.Summary == nil || len(q.Children) != 1 {
		t.Fatal("a summary with a child left was made a child")
	}
}

// 插入 → 概要 is not offered on the topic in the middle of a structure
// that draws no summary there.
func TestNoSummaryOfTheMiddleWhereNoneIsDrawn(t *testing.T) {
	a, tt := newTestApp(t)
	tt.Frame()
	menu := func() error {
		t.Helper()
		if err := clickNode(t, tt, a, a.doc.Root.Title); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		b, cam := a.boxes.Boxes[a.doc.Root.ID], a.cam()
		tt.RightClickAt(a.vp.X+cam.X+(b.X+b.W/2)*cam.Zoom, a.vp.Y+cam.Y+(b.Y+b.H/2)*cam.Zoom)
		err := tt.ChooseMenuItem("插入", "概要")
		tt.CloseMenu()
		tt.Frame()
		return err
	}
	if err := menu(); err == nil || a.doc.Root.Summary != nil {
		t.Fatalf("a mind map took a summary of its middle (%v)", err)
	}
	a.mutate("", func() { a.doc.Structure = model.StructureLogic })
	tt.Frame()
	if err := menu(); err != nil || a.doc.Root.Summary == nil {
		t.Fatalf("a logic chart took none: %v", err)
	}
}

// The 撤销 of the three toasts of a floating topic takes back that change
// and nothing else: an edit made meanwhile stays.
func TestFloatingToastsUndoTheirOwnChange(t *testing.T) {
	a, tt := newTestApp(t)
	tt.Frame()
	meanwhile := func() *model.Node {
		n, _ := a.doc.AddChild(a.doc.Root.ID, "其间加的")
		a.mutate("", func() { n.Title = "其间加的主题" })
		return n
	}
	// 设为自由主题, and back to its place among its siblings.
	arch := find(t, a, "架构")
	kid := arch.Children[0]
	second := ""
	if len(arch.Children) > 1 {
		second = arch.Children[1].ID
	}
	a.floatNode(kid)
	if !a.doc.IsFloating(kid.ID) || a.noticeUndo == nil {
		t.Fatalf("not floating, or no 撤销: %q", a.notice)
	}
	back := a.noticeUndo
	m := meanwhile()
	back()
	tt.Frame()
	if a.doc.IsFloating(kid.ID) || arch.Children[0].ID != kid.ID || kid.Pos != nil {
		t.Fatal("撤销 did not put the topic back where it was")
	}
	if second != "" && arch.Children[1].ID != second {
		t.Fatal("its siblings are out of order")
	}
	if n, _ := a.doc.Find(m.ID); n == nil || n.Title != "其间加的主题" {
		t.Fatal("撤销 took back the edit made meanwhile")
	}

	// 挂到…下, and floating again where it was.
	var f *model.Node
	a.mutate("", func() { f = a.doc.AddFloating("漂着的", model.Point{X: 500, Y: 300}) })
	a.joinFloating(f.ID, arch.ID)
	if a.doc.IsFloating(f.ID) {
		t.Fatal("not joined")
	}
	back = a.noticeUndo
	m = meanwhile()
	back()
	if !a.doc.IsFloating(f.ID) || f.Pos == nil || f.Pos.X != 500 || f.Pos.Y != 300 {
		t.Fatalf("撤销 did not float it again where it was: %+v", f.Pos)
	}
	if n, _ := a.doc.Find(m.ID); n == nil {
		t.Fatal("撤销 took back the edit made meanwhile")
	}

	// 放到画布上: the topic goes, the clip stays.
	cl, err := a.addClipText("竞品的定价页", "手动")
	if err != nil {
		t.Fatal(err)
	}
	a.addFloatingClip(cl.ID, model.Point{X: 600, Y: 100})
	placed := a.doc.Floating[len(a.doc.Floating)-1].ID
	back = a.noticeUndo
	m = meanwhile()
	back()
	tt.Frame()
	if n, _ := a.doc.Find(placed); n != nil {
		t.Fatal("撤销 left the topic on the canvas")
	}
	if n, _ := a.doc.Find(m.ID); n == nil || a.lib.Find(cl.ID) == nil {
		t.Fatal("撤销 took back more than the topic")
	}
	// Taken back twice, or with nothing to take back: no step is made.
	edits := a.edits
	back()
	if a.edits != edits {
		t.Fatal("a step for nothing")
	}
}

// 设为自由主题 is not offered for a summary's topic, and records no step
// for one; with a branch alone on the canvas it says where the topic went.
func TestFloatOfASummaryAndInFocus(t *testing.T) {
	a, tt := newTestApp(t)
	_, sum := branchWithSummary(t, a, "甲", "乙")
	tt.Frame()
	edits := a.edits
	a.floatNode(sum)
	if a.edits != edits || a.doc.IsFloating(sum.ID) {
		t.Fatal("a step recorded for a summary's topic that cannot float")
	}
	tt.Frame()
	b, cam := a.boxes.Boxes[sum.ID], a.cam()
	cam.X += keepIn(cam.X+b.X*cam.Zoom, b.W*cam.Zoom, a.vp.W)
	cam.Y += keepIn(cam.Y+b.Y*cam.Zoom, b.H*cam.Zoom, a.vp.H)
	tt.Frame()
	tt.RightClickAt(a.vp.X+cam.X+(b.X+b.W/2)*cam.Zoom, a.vp.Y+cam.Y+(b.Y+b.H/2)*cam.Zoom)
	if err := tt.ChooseMenuItem("插入", "设为自由主题"); err == nil {
		t.Fatal("offered for a summary's topic")
	}
	tt.CloseMenu()
	tt.Frame()

	arch := find(t, a, "架构")
	kid := arch.Children[0]
	a.focusOn(arch.ID)
	tt.Frame()
	a.floatNode(kid)
	tt.Frame()
	if !a.doc.IsFloating(kid.ID) {
		t.Fatal("not floating")
	}
	if !strings.Contains(a.notice, "聚焦") || a.noticeUndo == nil {
		t.Fatalf("gone without a word: %q", a.notice)
	}
	if a.selected != arch.ID {
		t.Fatal("what is chosen is not on the canvas")
	}
	a.noticeUndo()
	if a.doc.IsFloating(kid.ID) || !slices.Contains(arch.Children, kid) {
		t.Fatal("撤销 did not bring it back")
	}
}

// A branch moved to another map and the step undone: it was in both maps,
// by the same IDs. The undo takes it out of the other map, a redo puts it
// there again, and the toast's 撤销 does both sides by the topics' IDs.
func TestMoveToMapUndoneOnBothSides(t *testing.T) {
	a, tt := newTestApp(t)
	other, _, kid := newMapFile(t, a, "归档", "旧需求")
	n := find(t, a, "前端")
	id, parent := n.ID, find(t, a, "架构")
	at := slices.Index(parent.Children, n)
	if at < 0 {
		t.Fatal("前端 is not under 架构")
	}
	in := func(path string) bool {
		t.Helper()
		f, err := model.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		return f.Node(id) != nil
	}
	a.selectNode(id)
	a.moveToMap(other, "", kid.ID)
	tt.Frame()
	if a.file.Node(id) != nil || !in(other) {
		t.Fatal("not moved")
	}
	a.undo() // ⌘Z
	tt.Frame()
	if a.file.Node(id) == nil {
		t.Fatal("the undo did not bring it back")
	}
	if in(other) {
		t.Fatal("undone, the branch is in both maps")
	}
	a.redo()
	tt.Frame()
	if a.file.Node(id) != nil || !in(other) {
		t.Fatal("redone, the branch is in neither map, or in both")
	}
	a.undo()
	if a.file.Node(id) == nil || in(other) {
		t.Fatal("undone again")
	}

	// The toast's 撤销, with an edit made meanwhile.
	a.selectNode(id)
	a.moveToMap(other, "", kid.ID)
	back := a.noticeUndo
	if back == nil {
		t.Fatalf("no 撤销 on %q", a.notice)
	}
	extra, _ := a.doc.AddChild(a.doc.Root.ID, "其间加的")
	a.mutate("", func() { extra.Title = "其间加的主题" })
	back()
	tt.Frame()
	parent = find(t, a, "架构")
	if got := slices.IndexFunc(parent.Children, func(c *model.Node) bool { return c.ID == id }); got != at {
		t.Fatalf("back at %d of its siblings, was at %d", got, at)
	}
	if in(other) {
		t.Fatal("撤销 left the branch in the other map")
	}
	if x, _ := a.doc.Find(extra.ID); x == nil {
		t.Fatal("撤销 took back the edit made meanwhile")
	}
}

// A branch cut and pasted is itself still: the links to its topics find
// them. Pasted a second time it is a copy; and on the outline as on the
// canvas, where what was drawn of it follows it.
func TestCutAndPasteKeepLinks(t *testing.T) {
	a, tt := newTestApp(t)
	tt.Frame()
	n := find(t, a, "前端")
	kid, _ := a.doc.AddChild(n.ID, "前端下的一个")
	id, kidID := n.ID, kid.ID
	link := a.nodeLinkTo(a.path, kidID)
	a.addLink(find(t, a, "后端"), link)
	set := func(node string) {
		t.Helper()
		f := &diaFile{Path: a.mapKey(a.path), Set: diagram.Set{Node: node, Objects: []diagram.Object{{Name: "退货单", Kind: "单据"}}}}
		if err := a.writeDia(f); err != nil {
			t.Fatal(err)
		}
	}
	set(id)

	a.selectNode(id)
	a.copyNodes(true)
	if a.file.Node(id) != nil {
		t.Fatal("not cut")
	}
	a.selectNode(find(t, a, "功能").ID)
	if !a.paste() {
		t.Fatal("not pasted")
	}
	tt.Frame()
	if p := a.file.Node(id); p == nil || p.Title != "前端" || a.file.Node(kidID) == nil {
		t.Fatal("cut and pasted, the branch has other IDs: the links to it find nothing")
	}
	if _, target, _ := a.parseLink(link); target != a.path+"#"+kidID {
		t.Fatalf("the link leads to %q", target)
	}
	if a.diaOf(id) == nil {
		t.Fatal("its 图解 did not stay with it")
	}
	a.selectNode(find(t, a, "架构").ID)
	a.paste()
	tt.Frame()
	if n, _ := a.selectedNode(); n == nil || n.ID == id || n.Title != "前端" {
		t.Fatal("pasted a second time, it is not a copy")
	}
	ids := map[string]bool{}
	a.file.Walk(func(n *model.Node) {
		if ids[n.ID] {
			t.Fatalf("two topics with the ID of %q", n.Title)
		}
		ids[n.ID] = true
	})

	// On the outline: cut, and pasted after a line, by its own IDs.
	a.showView(viewOutline)
	tt.Frame()
	a.selected, a.multi = id, nil
	a.copyNodes(true)
	a.outlinePaste(find(t, a, "后端").ID)
	tt.Frame()
	if a.file.Node(id) == nil || a.clipCut != nil || a.diaOf(id) == nil {
		t.Fatal("on the outline a cut and a paste gave the branch other IDs, or lost its 图解")
	}
	// The cut undone and then pasted there: a copy, which takes a set of
	// its own after the one it was cut from.
	a.copyNodes(true)
	a.undo()
	if a.file.Node(id) == nil {
		t.Fatal("the cut was not undone")
	}
	a.outlinePaste(find(t, a, "后端").ID)
	tt.Frame()
	cp, _ := a.selectedNode()
	if cp == nil || cp.ID == id || cp.Title != "前端" {
		t.Fatalf("with the cut undone, the paste is not a copy: %+v", cp)
	}
	if a.diaOf(cp.ID) == nil || a.diaOf(id) == nil {
		t.Fatal("pasted on the outline, the copy of a branch cut has no 图解")
	}
}

// The edits of one drag, or of one stay in a field, are one step to undo;
// a second drag, or the field written in again after it was left, is
// another. They were one step without end.
func TestStepsEndWithTheDragAndTheField(t *testing.T) {
	a, tt := newTestApp(t)
	var f *model.Node
	a.mutate("", func() { f = a.doc.AddFloating("漂着的", model.Point{X: -260, Y: 240}) })
	a.selected = a.doc.Root.ID
	tt.Frame()
	tt.Frame()
	drag := func(dx, dy float32) {
		t.Helper()
		b, cam := a.boxes.Boxes[f.ID], a.cam()
		x, y := a.vp.X+cam.X+(b.X+b.W/2)*cam.Zoom, a.vp.Y+cam.Y+(b.Y+b.H/2)*cam.Zoom
		tt.Press(x, y)
		for i := 1; i <= 8; i++ {
			tt.Move(x+dx*float32(i)/8, y+dy*float32(i)/8)
		}
		tt.Release(x+dx, y+dy)
		tt.Frame()
		tt.Frame()
		f, _ = a.doc.Find(f.ID)
	}
	start := *f.Pos
	drag(40, 0)
	first := *f.Pos
	if first == start {
		t.Fatal("the drag moved nothing")
	}
	drag(0, 40)
	if *f.Pos == first {
		t.Fatal("the second drag moved nothing")
	}
	a.undo()
	f, _ = a.doc.Find(f.ID)
	if *f.Pos != first {
		t.Fatalf("one undo took back both drags: at %+v, the first left it at %+v", *f.Pos, first)
	}
	a.undo()
	if f, _ = a.doc.Find(f.ID); *f.Pos != start {
		t.Fatalf("the second undo left it at %+v", *f.Pos)
	}

	// A title in the topic's column: written, left by a click on the
	// canvas, and written again.
	n := find(t, a, "架构")
	id := n.ID
	a.selectNode(n.ID)
	tt.Frame()
	title := func(add string) {
		t.Helper()
		if err := tt.Click("标题"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		tt.Key(0, ui.KeyEnd)
		tt.Type(add)
		tt.Frame()
	}
	title("甲乙")
	if n.Title != "架构甲乙" {
		t.Fatalf("title %q", n.Title)
	}
	tt.ClickAt(a.vp.X+a.vp.W-30, a.vp.Y+a.vp.H/2) // the bare canvas
	tt.Frame()
	a.selectNode(n.ID)
	tt.Frame()
	title("丙")
	if n.Title != "架构甲乙丙" {
		t.Fatalf("not written again: %q", n.Title)
	}
	a.undo()
	if n, _ = a.doc.Find(id); n == nil || n.Title != "架构甲乙" {
		t.Fatalf("one undo took back both stays in the field: %q", n.Title)
	}
	a.undo()
	if n, _ = a.doc.Find(id); n.Title != "架构" {
		t.Fatalf("after the second undo %q", n.Title)
	}
}

// The set of a topic deleted is not kept for ever: found in no map it is
// marked, kept a month more (the topic may come back), and deleted then.
func TestDiagramsOfTopicsGoneAreSwept(t *testing.T) {
	a, tt := newTestApp(t)
	tt.Frame()
	now := testNow
	a.now = func() time.Time { return now }
	set := func(title string) (string, string) {
		t.Helper()
		n, _ := a.doc.AddChild(a.doc.Root.ID, title)
		f := &diaFile{Path: a.mapKey(a.path), Set: diagram.Set{Node: n.ID, Objects: []diagram.Object{{Name: "退货单", Kind: "单据"}}}}
		if err := a.writeDia(f); err != nil {
			t.Fatal(err)
		}
		return n.ID, filepath.Join(a.diaDir(), diaName(n.ID))
	}
	stays, staysFile := set("留着的")
	goes, goesFile := set("要删的")
	back, backFile := set("删了又回来的")
	a.flush()
	sweep := func(after time.Duration) {
		now = now.Add(after)
		a.diaSwept = time.Time{}
		a.sweepDias()
	}
	del := func(id string) {
		a.selected, a.multi = id, nil
		a.deleteSelected()
	}
	del(goes)
	del(back)
	a.flush()
	sweep(0)
	if a.dias[stays].Gone != nil || a.dias[goes] == nil || a.dias[goes].Gone == nil || a.dias[back].Gone == nil {
		t.Fatalf("after the first look: %+v", a.dias)
	}
	if !exists(goesFile) || !exists(backFile) {
		t.Fatal("deleted at once: an undo would bring the topic back without its set")
	}
	a.undo() // the second topic is back
	if a.file.Node(back) == nil {
		t.Fatal("not undone")
	}
	sweep(10 * 24 * time.Hour)
	if a.dias[back] == nil || a.dias[back].Gone != nil || !exists(goesFile) {
		t.Fatal("a topic back keeps its set, and one gone ten days is kept still")
	}
	sweep(25 * 24 * time.Hour)
	if exists(goesFile) || a.dias[goes] != nil {
		t.Fatal("the set of a topic gone for more than a month is still there")
	}
	if !exists(staysFile) || !exists(backFile) || a.diaOf(stays) == nil || a.diaOf(back) == nil {
		t.Fatal("the sets of topics that are there were swept")
	}
	// A map that cannot be read: nothing is known to be gone.
	del(back)
	a.flush()
	bad := filepath.Join(a.dir, "坏的.mindmap.json")
	if err := os.WriteFile(bad, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.refreshFiles()
	sweep(40 * 24 * time.Hour)
	sweep(40 * 24 * time.Hour)
	if !exists(backFile) {
		t.Fatal("swept though a map could not be read")
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// A set drawn while its map was renamed is the map's as it is named now.
func TestDiagramDrawnWhileTheMapIsRenamed(t *testing.T) {
	a, tt := newTestApp(t)
	fake := &fakeClaude{replies: []string{claudeText(refundSet)}}
	a.sendAI = fake.send
	a.cfg.AIKey = "sk-ant-test"
	root := refundBranch(t, a)
	a.selectNode(root.ID)
	tt.Frame()
	var answer func()
	a.async = func(work func() (any, error), done func(any, error)) { answer = func() { done(work()) } }
	a.drawDia(root.ID, "")
	if answer == nil {
		t.Fatal("the AI was not asked")
	}
	a.flush()
	to := filepath.Join(a.dir, model.FileName("改了名的导图"))
	if err := os.Rename(a.path, to); err != nil {
		t.Fatal(err)
	}
	a.moved(a.path, to)
	answer()
	tt.Frame()
	if a.path != to || a.diaOf(root.ID) == nil {
		t.Fatalf("the set was kept under the path the map had: %q", a.diaErr)
	}
}

// The picture of a map however large has bounds: a thousand topics in a
// column were 2918×49132 pixels. It is drawn smaller, said so, and its key
// waits while it is written.
func TestExportImageOfALargeMapIsBounded(t *testing.T) {
	a, tt := newTestApp(t)
	a.mutate("", func() {
		p, _ := a.doc.AddChild(a.doc.Root.ID, "很长的一列")
		for i := range 700 {
			_, _ = a.doc.AddChild(p.ID, fmt.Sprintf("第 %d 个主题", i+1))
		}
	})
	tt.Frame()
	var waiting []func()
	a.async = func(work func() (any, error), done func(any, error)) {
		waiting = append(waiting, func() { done(work()) })
	}
	out := filepath.Join(t.TempDir(), "大图.png")
	a.plat.(*fakePlatform).savePath = out
	a.askExportImage()
	if len(waiting) != 1 || !a.imageBusy {
		t.Fatalf("the picture is written on the window's time (%d waiting, err %q)", len(waiting), a.saveErr)
	}
	tt.Frame()
	if _, ok := tt.Find("正在导出图片"); !ok {
		t.Fatal("no sign of the wait on the export key")
	}
	a.askExportImage() // again meanwhile: not begun twice
	if len(waiting) != 1 {
		t.Fatal("a second picture begun while the first is written")
	}
	waiting[0]()
	tt.Frame()
	if _, ok := tt.Find("导出"); !ok || a.imageBusy {
		t.Fatal("the export key did not come back")
	}
	file, err := os.Open(out)
	if err != nil {
		t.Fatal(err, a.saveErr)
	}
	defer file.Close()
	cfg, err := png.DecodeConfig(file)
	if err != nil {
		t.Fatal(err)
	}
	if max(cfg.Width, cfg.Height) > exportMaxSide || cfg.Width*cfg.Height > exportMaxPixels {
		t.Fatalf("a picture of %d×%d", cfg.Width, cfg.Height)
	}
	if cfg.Height < exportMaxSide-8 {
		t.Fatalf("smaller than it need be: %d×%d", cfg.Width, cfg.Height)
	}
	if !strings.Contains(a.notice, "%") {
		t.Fatalf("not said that it was drawn smaller: %q", a.notice)
	}
	// A map that fits is drawn at twice its size, as ever.
	if s := exportFit(1200, 900); s != exportScale {
		t.Fatalf("a small map at %v", s)
	}
	if s := exportFit(6000, 6000); s*6000*s*6000 > exportMaxPixels*1.001 {
		t.Fatalf("a square map at %v", s)
	}
}

var _ = ui.KeyEnter
