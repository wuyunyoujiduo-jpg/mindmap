package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// lensMap writes the draft's map, 产品调研, and opens it: four branches,
// each topic with the days since it changed, the clips it links to and
// its tasks.
func lensMap(tb testing.TB, a *app) {
	tb.Helper()
	f := model.NewFile("产品调研")
	root := f.Sheets[0].Root
	clip := 0
	topic := func(parent *model.Node, title string, days, sources int) *model.Node {
		n := model.NewNode(title)
		n.Changed = testNow.AddDate(0, 0, -days)
		for range sources {
			clip++
			n.Links = append(n.Links, fmt.Sprintf("%s%d", clipScheme, clip))
		}
		parent.Children = append(parent.Children, n)
		return n
	}
	task := func(parent *model.Node, title string, days int, status model.Status, due int) *model.Node {
		n := topic(parent, title, days, 0)
		n.Task = &model.Task{Status: status}
		if due != 0 {
			d := today(testNow).AddDate(0, 0, due)
			n.Task.Due = &d
		}
		return n
	}
	a1 := topic(root, "用户访谈", 3, 5)
	topic(a1, "老用户最在意同步", 3, 3)
	first := topic(a1, "新手卡在第一步", 12, 2)
	task(first, "录三段上手的屏", 12, model.StatusDone, 0)
	task(first, "改第一屏的字", 12, model.StatusDoing, 4)
	task(first, "找五个新手试", 12, model.StatusTodo, 0)
	b := topic(root, "定价", 40, 2)
	topic(b, "竞品的套餐", 40, 2)
	per := topic(b, "按人还是按量", 120, 0)
	task(per, "算两种的收入", 120, model.StatusTodo, -6)
	task(per, "问三个团队", 120, model.StatusTodo, 9)
	d := topic(root, "竞品", 75, 4)
	cal := topic(d, "日历视图", 6, 3)
	task(cal, "截它的周视图", 6, model.StatusDone, 0)
	task(cal, "记下拖动的手感", 6, model.StatusDone, 0)
	team := topic(d, "导图的协作", 160, 1)
	team.Note = "见[这条](<clip:9001>)和[同一条](<clip:9001>)，还有[导图](<mindmap:别的#x>)" // one more clip, once
	e := topic(root, "待验证", 20, 0)
	task(e, "离线优先值不值", 95, model.StatusTodo, 0)
	topic(e, "导入旧笔记", 20, 0)
	path := filepath.Join(a.dir, model.FileName("产品调研"))
	if err := f.Save(path); err != nil {
		tb.Fatal(err)
	}
	a.refreshFiles()
	if err := a.open(path); err != nil {
		tb.Fatal(err)
	}
	a.mode, a.panelOff = viewMap, false
}

// Which topic gets which step, lens by lens, on the draft's map; and the
// right column's figures and list.
func TestLensLooks(t *testing.T) {
	a, _ := newTestApp(t)
	lensMap(t, a)
	step := func(d *lensData, title string) int {
		t.Helper()
		lt, ok := d.topics[find(t, a, title).ID]
		if !ok {
			t.Fatalf("%s: %q has no look", d.lens, title)
		}
		return lt.step
	}
	titles := func(d *lensData) (out []string) {
		for _, l := range d.lines {
			out = append(out, l.title+" "+l.cap)
		}
		return out
	}
	same := func(what string, got, want []string) {
		t.Helper()
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("%s:\n got %q\nwant %q", what, got, want)
		}
	}

	age := lensOf(a.file, a.doc, lensAge, testNow)
	for title, want := range map[string]int{
		"用户访谈": ageWeek, "日历视图": ageWeek, "新手卡在第一步": ageMonth, "待验证": ageMonth,
		"定价": ageSeason, "竞品": ageSeason, "按人还是按量": agePlain, "导图的协作": agePlain, "离线优先值不值": agePlain,
	} {
		if got := step(age, title); got != want {
			t.Errorf("新旧: %q is step %d, of %d", title, got, want)
		}
	}
	if _, ok := age.topics[a.doc.Root.ID]; ok {
		t.Error("the middle has a look: it stays in the ink")
	}
	if age.figures[0].n != 5 || age.figures[1].n != 5 { // 用户访谈, 老用户…, 日历视图 and its two tasks; 按人… and its two, 导图的协作, 离线…
		t.Errorf("新旧's figures: %+v", age.figures)
	}
	same("the longest untouched first", titles(age)[:3], []string{"导图的协作 5 个月", "按人还是按量 4 个月", "算两种的收入 4 个月"})

	src := lensOf(a.file, a.doc, lensSrc, testNow)
	for title, want := range map[string]int{
		"用户访谈": srcMany, "老用户最在意同步": srcMany, "竞品": srcMany, "日历视图": srcMany,
		"新手卡在第一步": srcSome, "定价": srcSome, "导图的协作": srcSome, "按人还是按量": srcNone, "待验证": srcNone,
	} {
		if got := step(src, title); got != want {
			t.Errorf("来源: %q is step %d, of %d", title, got, want)
		}
	}
	if n := src.topics[find(t, a, "导图的协作").ID].n; n != 2 {
		t.Errorf("a link and a clip named twice in the note are %d sources, of 2", n)
	}
	if src.figures[0].n != 8 || src.figures[1].n != 11 {
		t.Errorf("来源's figures: %+v", src.figures)
	}
	same("those with no source, in the map's order", titles(src)[:4], []string{"录三段上手的屏 ", "改第一屏的字 ", "找五个新手试 ", "按人还是按量 "})

	task := lensOf(a.file, a.doc, lensTask, testNow)
	for title, want := range map[string]int{
		"日历视图": taskDone, "截它的周视图": taskDone, "新手卡在第一步": taskDoing, "找五个新手试": taskDoing, "离线优先值不值": taskDoing,
		"用户访谈": taskDoing, "按人还是按量": taskLate, "算两种的收入": taskLate, "定价": taskLate, "问三个团队": taskDoing,
		"竞品的套餐": taskNone, "导图的协作": taskNone, "竞品": taskDone, "待验证": taskDoing,
	} {
		if got := step(task, title); got != want {
			t.Errorf("进度: %q is step %d, of %d", title, got, want)
		}
	}
	if task.figures[0].n != 5 || task.figures[1].n != 1 {
		t.Errorf("进度's figures: %+v", task.figures)
	}
	same("not done, the late first, then by the day", titles(task), []string{
		"算两种的收入 逾期 6 天", "改第一屏的字 10/14", "问三个团队 10/19", "找五个新手试 待做", "离线优先值不值 待做"})

	// What a lens says of a topic, in words.
	a.lens = lensAge
	for lens, facts := range map[string]map[string]string{
		lensAge:  {"用户访谈": "3 天前改过", "导图的协作": "5 个月前改过", "定价": "1 个月前改过"},
		lensSrc:  {"用户访谈": "5 条来源", "待验证": "没有来源"},
		lensTask: {"新手卡在第一步": "任务 1/3 做完", "定价": "任务 0/2 做完，有 1 件逾期", "算两种的收入": "任务：待做，已逾期", "导图的协作": "不是任务"},
	} {
		a.lens = lens
		for title, want := range facts {
			if got := a.lensFact(find(t, a, title).ID); got != want {
				t.Errorf("%s of %q: %q, of %q", lens, title, got, want)
			}
		}
	}
	a.lens = lensBranch
	if got := a.lensFact(find(t, a, "定价").ID); got != "" {
		t.Errorf("by the branches a topic has a lens's fact: %q", got)
	}
}

// An edit says when its topic changed, and only its topic; the letters of
// one step are one time's; undo takes the time back with the change; the
// file keeps it.
func TestEditStampsItsTopic(t *testing.T) {
	a, _ := newTestApp(t)
	lensMap(t, a)
	at := testNow
	a.now = func() time.Time { return at }
	n, other := find(t, a, "定价"), find(t, a, "竞品")
	was, otherWas := n.Changed, other.Changed

	a.mutate("title:"+n.ID, func() { n.Title = "定" })
	at = at.Add(3 * time.Second)
	a.mutate("title:"+n.ID, func() { n.Title = "定价策略" })
	if !n.Changed.Equal(at) {
		t.Fatalf("the topic written in is of %v, now is %v", n.Changed, at)
	}
	if !other.Changed.Equal(otherWas) {
		t.Fatalf("a topic not touched is of %v", other.Changed)
	}
	// A move is where it stands, not what it says.
	a.mutate("", func() { _ = a.doc.Move(other.ID, n.ID, 0) })
	if !other.Changed.Equal(otherWas) {
		t.Fatalf("a topic moved is said changed: %v", other.Changed)
	}
	// A topic made is of now; a task ticked is of now.
	at = at.Add(time.Minute)
	var made *model.Node
	a.mutate("", func() { made, _ = a.doc.AddChild(n.ID, "新的") })
	tick := find(t, a, "找五个新手试")
	a.mutate("", func() { tick.Task.SetStatus(model.StatusDone, at) })
	if !made.Changed.Equal(at) || !tick.Changed.Equal(at) {
		t.Fatalf("made %v, ticked %v, now %v", made.Changed, tick.Changed, at)
	}
	a.save()
	f, err := model.Load(a.path)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Node(n.ID).Changed; !got.Equal(n.Changed) {
		t.Fatalf("the file keeps %v, of %v", got, n.Changed)
	}
	// Undone to before the title: the time it had.
	for range 4 {
		a.undo()
	}
	if back := find(t, a, "定价"); !back.Changed.Equal(was) {
		t.Fatalf("undone, the topic is of %v, it was of %v", back.Changed, was)
	}
	// And the next edit after an undo is of its own topic alone.
	at = at.Add(time.Minute)
	n = find(t, a, "竞品")
	a.mutate("", func() { n.Note = "看了三家" })
	if !n.Changed.Equal(at) || !find(t, a, "定价").Changed.Equal(was) {
		t.Fatalf("after an undo: %v and %v", n.Changed, find(t, a, "定价").Changed)
	}
}

// The row on the canvas chooses the lens; the right column says it with
// nothing chosen, a line of its list chooses that topic and brings it
// into view, whose page then says the lens's fact; the bare canvas is
// nothing chosen again, and the middle clicked shows its own page.
func TestLensColumn(t *testing.T) {
	a, tt := newTestApp(t)
	lensMap(t, a)
	tt.Frame()
	tt.Frame()
	if _, ok := tt.Find("镜头：新旧"); !ok {
		t.Fatal("the canvas has no row of lenses")
	}
	if _, ok := tt.Find("图例"); ok {
		t.Fatal("by the branches there is a legend")
	}
	a.selectNode(find(t, a, "定价").ID)
	tt.Frame()
	if err := tt.Click("镜头：新旧"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if a.lens != lensAge || a.selected != a.doc.Root.ID {
		t.Fatalf("lens %q, chosen %q", a.lens, a.selected)
	}
	for _, s := range []string{"最久没动的", "本周动过", "三个月没动", "导图的协作", "5 个月", "本周", "更久"} {
		if !tt.HasText(s) {
			t.Errorf("新旧 does not say %q", s)
		}
	}
	old := find(t, a, "导图的协作")
	a.cam().X -= 4000 // out of view
	tt.Frame()
	clickInPanel(t, tt, "到主题 导图的协作")
	tt.Frame()
	tt.Frame()
	if a.selected != old.ID {
		t.Fatalf("the line chose %q", a.selected)
	}
	b, cam := a.boxes.Boxes[old.ID], a.cam()
	if x := cam.X + b.X*cam.Zoom; x < 0 || x+b.W*cam.Zoom > a.vp.W {
		t.Fatalf("the topic chosen is not in view: at %v of %v", x, a.vp.W)
	}
	if !tt.HasText("5 个月前改过") || tt.HasText("最久没动的") {
		t.Fatalf("the topic's page does not say the lens's fact: %q", tt.Texts())
	}
	// The bare canvas: nothing chosen, the lens again.
	tt.ClickAt(a.vp.X+a.vp.W/2, a.vp.Y+14)
	tt.Frame()
	tt.Frame()
	if a.selected != a.doc.Root.ID || !tt.HasText("最久没动的") {
		t.Fatalf("a click on the bare canvas: chosen %q", a.selected)
	}
	// The middle clicked: its own page.
	if err := clickNode(t, tt, a, "产品调研"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if tt.HasText("最久没动的") {
		t.Fatal("the middle chosen by hand shows the lens's page still")
	}
	// The other lenses, and back.
	if err := tt.Click("镜头：来源"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	for _, s := range []string{"没有来源的", "有依据", "3 条以上", "1–2 条"} {
		if !tt.HasText(s) {
			t.Errorf("来源 does not say %q", s)
		}
	}
	if _, ok := tt.Find("5 条来源"); !ok {
		t.Error("来源 shows no count on a topic of five clips")
	}
	if err := tt.Click("镜头：进度"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	for _, s := range []string{"还没做完的", "件要做", "件逾期", "逾期 6 天", "有逾期", "不是任务"} {
		if !tt.HasText(s) {
			t.Errorf("进度 does not say %q", s)
		}
	}
	// A double click on the row makes no floating topic of the canvas under it.
	floats := len(a.doc.Floating)
	r, _ := tt.Find("镜头")
	for range 2 {
		tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	}
	tt.Frame()
	tt.Frame()
	if len(a.doc.Floating) != floats || a.editing != "" {
		t.Fatal("a double click on the lens's name made a topic")
	}
	if err := tt.Click("镜头：分支"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if a.lens != lensBranch || tt.HasText("还没做完的") {
		t.Fatalf("back by the branches: lens %q", a.lens)
	}
}

// A lens moves nothing: every topic's box is where it was by the branches.
func TestLensMovesNothing(t *testing.T) {
	a, tt := newTestApp(t)
	lensMap(t, a)
	tt.Frame()
	tt.Frame()
	was := a.boxes.Boxes
	for _, l := range []string{lensAge, lensSrc, lensTask} {
		a.setLens(l)
		tt.Frame()
		tt.Frame()
		for id, b := range a.boxes.Boxes {
			if b != was[id] {
				n, _ := a.doc.Find(id)
				t.Fatalf("%s moved %q: %v, it was %v", l, n.Title, b, was[id])
			}
		}
	}
}

// The lens is kept with the map's view: a map never looked at through one
// is by its branches, and one left through a lens opens through it, after
// the app is opened again too.
func TestLensKeptPerMap(t *testing.T) {
	a, tt := newTestApp(t)
	guide := a.path
	lensMap(t, a)
	research := a.path
	a.setLens(lensSrc)
	tt.Frame()
	if err := a.open(guide); err != nil {
		t.Fatal(err)
	}
	if a.lens != lensBranch {
		t.Fatalf("a map never looked at through a lens opens through %q", a.lens)
	}
	a.setLens(lensTask)
	if err := a.open(research); err != nil {
		t.Fatal(err)
	}
	if a.lens != lensSrc {
		t.Fatalf("the map left through 来源 opens through %q", a.lens)
	}
	a.flush()
	b := newApp(&fakePlatform{}, config{Dir: a.dir, PanelOpen: true})
	if err := b.open(research); err != nil {
		t.Fatal(err)
	}
	if b.lens != lensSrc {
		t.Fatalf("opened again, the app shows it through %q", b.lens)
	}
	if err := b.open(guide); err != nil || b.lens != lensTask {
		t.Fatalf("and the other through %q (%v)", b.lens, err)
	}
}

// A picture of the map is of the map as it is: the same through any lens.
func TestLensNotInPictures(t *testing.T) {
	a, tt := newTestApp(t)
	lensMap(t, a)
	tt.Frame()
	plain, _, err := a.drawImage()
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range []string{lensAge, lensSrc, lensTask} {
		a.setLens(l)
		tt.Frame()
		img, _, err := a.drawImage()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(img.Pix, plain.Pix) {
			t.Fatalf("a picture made through %s is not the map's own", l)
		}
		tt.Frame()
		if a.lensShown == nil {
			t.Fatalf("after the picture the canvas is no more through %s", l)
		}
	}
}

// The lens's row and the canvas's own bar never lie over each other, nor
// the legend over either, however narrow the window: by what they
// measure, in whatever font.
func TestLensRowKeepsClear(t *testing.T) {
	a, tt := newTestApp(t)
	lensMap(t, a)
	a.setLens(lensTask) // the longest legend
	meet := func(x, y ui.Rect) bool {
		return x.X < y.X+y.W && y.X < x.X+x.W && x.Y < y.Y+y.H && y.Y < x.Y+x.H
	}
	span := func(first, last string) ui.Rect {
		t.Helper()
		f, ok1 := tt.Find(first)
		l, ok2 := tt.Find(last)
		if !ok1 || !ok2 {
			t.Fatalf("no %q or no %q", first, last)
		}
		top, foot := min(f.Y, l.Y), max(f.Y+f.H, l.Y+l.H)
		return ui.Rect{X: f.X, Y: top, W: l.X + l.W - f.X, H: foot - top}
	}
	var beside, over bool
	for _, w := range []int{1500, 1280, 1100, 1000, 920, 860} {
		for _, panel := range []bool{true, false} {
			a.panelOff = !panel
			tt.SetSize(w, 760)
			for range 4 {
				tt.Frame()
			}
			row, bar := span("镜头", "镜头：进度"), span("缩小", "导出")
			legend, ok := tt.Find("图例")
			if !ok {
				t.Fatalf("width %d: no legend", w)
			}
			if meet(row, bar) || meet(legend, bar) || meet(legend, row) {
				t.Fatalf("width %d, panel %v: the row %v, the bar %v and the legend %v meet", w, panel, row, bar, legend)
			}
			if row.X < a.vp.X || row.X+row.W > a.vp.X+a.vp.W || legend.X < a.vp.X {
				t.Fatalf("width %d, panel %v: the row %v or the legend %v out of the canvas %v", w, panel, row, legend, a.vp)
			}
			if row.Y+row.H > bar.Y {
				over = true
			} else {
				beside = true
			}
			if w == 920 && panel {
				snapshot(t, tt, "lens-narrow")
			}
		}
	}
	if !beside || !over {
		t.Fatalf("the row was beside the bar %v and over it %v: both are to be seen", beside, over)
	}
}

// A lens costs a map of five thousand topics one reading a change, and a
// frame nothing; and by the branches nothing at all.
func TestLensWorkedOutOnceAChange(t *testing.T) {
	a, tt := newTestApp(t)
	n := manyMaps(t, a, 1, 5000)
	for range 3 {
		tt.Frame()
	}
	if a.lensRuns != 0 {
		t.Fatalf("by the branches a lens was worked out %d times", a.lensRuns)
	}
	a.setLens(lensAge)
	for range 6 {
		tt.Frame()
	}
	if a.lensRuns != 1 {
		t.Fatalf("six frames worked the lens out %d times", a.lensRuns)
	}
	a.mutate("", func() { n.Note = "改了" })
	for range 6 {
		tt.Frame()
	}
	if a.lensRuns != 2 {
		t.Fatalf("a change and six frames worked the lens out %d times in all", a.lensRuns)
	}
	if d := a.lensLooks(); len(d.topics) != 5000 || len(d.lines) > lensLines {
		t.Fatalf("%d topics looked at, %d listed", len(d.topics), len(d.lines))
	}
}

// What a step costs to tell which topics it changed, on five thousand.
func BenchmarkStampStep(b *testing.B) {
	a, _ := newTestApp(b)
	n := manyMaps(b, a, 1, 5000)
	b.ResetTimer()
	for i := range b.N {
		a.mutate("", func() { n.Title = fmt.Sprint("功能 ", i) })
	}
}

func BenchmarkLensOf(b *testing.B) {
	a, _ := newTestApp(b)
	manyMaps(b, a, 1, 5000)
	for _, l := range []string{lensAge, lensSrc, lensTask} {
		b.Run(l, func(b *testing.B) {
			for range b.N {
				lensOf(a.file, a.doc, l, testNow)
			}
		})
	}
}

// Each lens on the draft's map, light, and one dark: to hold beside the
// draft.
func TestLensSnapshots(t *testing.T) {
	a, tt := newTestApp(t)
	lensMap(t, a)
	a.cfg.HideTree = true // as the draft, the left column put away
	tt.SetSize(1180, 700)
	for _, l := range []struct{ id, name string }{{lensBranch, "branch"}, {lensAge, "age"}, {lensSrc, "src"}, {lensTask, "task"}} {
		a.setLens(l.id)
		for range 3 {
			tt.Frame()
		}
		snapshot(t, tt, "lens-"+l.name)
	}
	a.selectNode(find(t, a, "按人还是按量").ID)
	for range 3 {
		tt.Frame()
	}
	snapshot(t, tt, "lens-task-topic")
	tt.SetDark(true)
	a.setLens(lensAge)
	for range 3 {
		tt.Frame()
	}
	snapshot(t, tt, "lens-age-dark")
	a.setLens(lensSrc)
	for range 3 {
		tt.Frame()
	}
	snapshot(t, tt, "lens-src-dark")
}
