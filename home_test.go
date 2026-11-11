package main

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

func TestHome(t *testing.T) {
	a, tt := newTestApp(t)
	for i := range 4 {
		clip, err := a.addClipText(fmt.Sprintf("资料 %d", i), "手动")
		if err != nil {
			t.Fatal(err)
		}
		clip.Created = testNow.Add(-time.Duration(i) * time.Hour)
	}
	a.showProjects()
	tt.Frame()
	if err := tt.Click("今天"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.libOpen || !a.libHome {
		t.Fatal("今天 not shown")
	}
	for _, want := range []string{"今天", "刚记下的", "放到导图… 资料 0"} {
		if !tt.HasText(want) {
			t.Fatalf("no %q: %q", want, tt.Texts())
		}
	}
	snapshot(t, tt, "home")

	// The line: a day makes it a task, else it is a note of 灵感.
	if err := tt.Click("记一条灵感或任务"); err != nil {
		t.Fatal(err)
	}
	tt.Type("今天 写周报 !高")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.homeDraft != "" || !tt.HasText("任务 写周报") {
		t.Fatalf("no task: %q %q", a.homeDraft, tt.Texts())
	}
	// On 今天 a plain line is a task too, until the bar is told otherwise.
	tt.Type("灵感来了")
	tt.Frame()
	if !tt.HasText("记为 任务 · 今天") {
		t.Fatalf("not read as a task on 今天: %q", tt.Texts())
	}
	if err := tt.Click("记为什么"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("记为 灵感") {
		t.Fatalf("not made a thought: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if len(a.lib.Clips) != 5 || a.lib.Clips[0].Text != "灵感来了" || !isMemo(a.lib.Clips[0]) {
		t.Fatalf("no note: %d", len(a.lib.Clips))
	}
	snapshot(t, tt, "home-filled")
	// What was written lately is there: the inbox's task, which waits for
	// a map, and the notes, which need nothing; older ones leave by
	// themselves, as those put on a map.
	if n := len(a.waiting()); n != 6 || tt.HasText("待归位") {
		t.Fatalf("lately %d: %q", n, tt.Texts())
	}
	if !tt.HasText("10月10日") || !tt.HasText(weekdays[testNow.Weekday()]) {
		t.Fatalf("the day's line: %q", tt.Texts())
	}
	a.lib.Clips[0].Created = testNow.AddDate(0, 0, -8)
	if n := len(a.waiting()); n != 5 {
		t.Fatalf("an old note stays: %d", n)
	}
	a.lib.Clips[0].Created = testNow
	// What can be done with a row shows under the pointer.
	row, _ := tt.Find("记下的 灵感来了")
	tt.Move(row.X+row.W/2, row.Y+row.H/2)
	tt.Frame()
	if err := tt.Click("放到导图… 灵感来了"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.paletteOpen || a.paletteMode != paletteAttach {
		t.Fatal("no topic asked for")
	}
	a.paletteOpen = false
	tt.Frame()
}

// The list's keys go through its maps; dark, the pages read as well.
func TestHomeDaysKeysAndDark(t *testing.T) {
	a, tt := newTestApp(t)
	for i := range 3 {
		clip, err := a.addClipText(fmt.Sprintf("第 %d 条", i), "手动")
		if err != nil {
			t.Fatal(err)
		}
		clip.Created = testNow.Add(-time.Duration(i) * 24 * time.Hour)
	}
	tt.SetDark(true)
	a.libOpen, a.libHome = true, true
	tt.Frame()
	snapshot(t, tt, "home-dark")
	a.libHome = false
	tt.Frame()
	snapshot(t, tt, "library-dark")
}

func TestMidEllipsis(t *testing.T) {
	if s := midEllipsis("短名字", 200); s != "短名字" {
		t.Fatalf("short %q", s)
	}
	s := midEllipsis("资料 6：关于工作的一些记录和后续安排", 120)
	if []rune(s)[0] != '资' || !strings.HasSuffix(s, "安排") || !strings.Contains(s, "…") {
		t.Fatalf("cut %q", s)
	}
}

// #标签 in the palette finds what carries the tag: notes, clips, topics.
func TestPaletteTag(t *testing.T) {
	a, _ := newTestApp(t)
	a.addMemo("列表可以用虚拟滚动 #性能", "灵感")
	a.addMemo("周末去爬山", "灵感")
	a.paletteMode, a.paletteQuery = paletteAll, "#性能"
	var kinds []string
	for _, it := range a.paletteItems() {
		kinds = append(kinds, it.kind+":"+it.title)
	}
	if len(kinds) != 2 || kinds[0] != "灵感:列表可以用虚拟滚动 #性能" || kinds[1] != "主题:列表加载慢" {
		t.Fatalf("items %q", kinds)
	}
}

// The window's controls sit over the map's bar: what is in it keeps clear.
func TestRailClearsWindowControls(t *testing.T) {
	_, tt := newTestApp(t)
	tt.SetTitleBar(ui.TitleBar{Height: 38, Left: 80})
	tt.Frame()
	tt.Frame()
	r, ok := tt.Find("导图颜色")
	if !ok || r.X < 80+12 {
		t.Fatalf("the map's title starts at %v, under the window's controls", r.X)
	}
}

// The topic's panel is put away at first, the canvas having the room; the
// toolbar's button shows it, and the choice is kept.
func TestPanelPutAway(t *testing.T) {
	dir := t.TempDir()
	a := newApp(&fakePlatform{}, config{Dir: dir})
	a.now = func() time.Time { return testNow }
	tt := ui.NewTester(a.view, 1280, 800)
	a.openDir(dir)
	if len(a.files) > 0 {
		_ = a.open(a.files[0].Path)
	}
	a.libOpen = false
	tt.Frame()
	if _, ok := tt.Find("备注内容"); ok || !a.panelOff {
		t.Fatalf("the panel shows at first: %q", tt.Texts())
	}
	snapshot(t, tt, "map-panel-away")
	if err := tt.Click("主题面板"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, ok := tt.Find("备注内容"); !ok || !a.cfg.PanelOpen {
		t.Fatalf("the panel is not shown: %q", tt.Texts())
	}
}

// TestHomeTicksAndReads: a task ticked on 今天 is done; the line says what
// it reads of what is written in it.
func TestHomeTicksAndReads(t *testing.T) {
	a, tt := newTestApp(t)
	a.addTask("今天 写周报", nil)
	a.showLibrary()
	a.libHome = true
	tt.Frame()
	if len(a.doneToday(testNow)) != 0 {
		t.Fatal("nothing is done yet")
	}
	if err := tt.Click("完成 写周报"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if len(a.doneToday(testNow)) != 1 || !tt.HasText("已完成") {
		t.Fatalf("the task done is not shown: %q", tt.Texts())
	}
	if err := tt.Click("记一条灵感或任务"); err != nil {
		t.Fatal(err)
	}
	tt.Type("周五 给客户发报价单 #报价")
	tt.Frame()
	for _, want := range []string{"记下", "任务 · 周五", "加图片", "编号列表"} {
		if !tt.HasText(want) {
			t.Fatalf("the line does not say %q: %q", want, tt.Texts())
		}
	}
	snapshot(t, tt, "home-typed")

	// A # offers the tags used last; a click writes one.
	for i, tag := range []string{"会议", "产品"} {
		cl, _ := a.addMemo("旧的一条 #"+tag, "手动")
		cl.Created = testNow.Add(time.Duration(i) * time.Minute)
	}
	a.homeDraft = "新的想法 #"
	tt.Frame()
	if got := a.recentTags("", 6); len(got) < 2 || got[0] != "产品" {
		t.Fatalf("tags offered %q", got)
	}
	if err := tt.Click("用标签 会议"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.homeDraft != "新的想法 #会议 " || tt.HasText("用标签 产品") {
		t.Fatalf("draft %q", a.homeDraft)
	}
	a.homeDraft = "要点"
	tt.Frame()
	tt.Click("圆点列表")
	tt.Frame()
	tt.Click("编号列表")
	tt.Frame()
	if a.homeDraft != "要点\n- \n1. " {
		t.Fatalf("lists %q", a.homeDraft)
	}

	// An @ offers the notes kept last; the one chosen is kept as a
	// reference, and shows under the note's words, going to it.
	a.homeDraft = "接着上次的想法 @"
	tt.Frame()
	old := a.clipsNamed("", 1)[0]
	if err := tt.Click("引用 " + refName(old)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	a.homeDraft += "**要紧**"
	a.saveHomeLine()
	tt.Frame()
	kept := a.clipsNamed("接着上次", 1)
	if len(kept) != 1 || !strings.Contains(kept[0].Text, "<clip:"+old.ID+">") {
		t.Fatalf("reference not kept: %+v", kept)
	}
	if !tt.HasText("前往引用 "+refName(old)) || tt.HasText("<clip:") {
		t.Fatalf("reference not shown: %q", tt.Texts())
	}
	snapshot(t, tt, "home-refs")
}

// TestGroundChosen: the window's ground is white until another is chosen
// in the settings, which is kept.
func TestGroundChosen(t *testing.T) {
	a, tt := newTestApp(t)
	a.showLibrary()
	a.libHome = true
	tt.Frame()
	if groundNow().key != "" || ground(a.theme()) != ui.Hex("#F5F5F7") {
		t.Fatalf("the ground is %q", groundNow().name)
	}
	snapshot(t, tt, "ground-gray")
	a.openSettings()
	tt.Frame()
	tt.Scroll(720, 400, 0, 560) // the ground is under the version
	tt.Frame()
	if err := tt.Click("底色 淡紫"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.cfg.Ground != "violet" || loadConfig().Ground != "violet" {
		t.Fatalf("ground %q", a.cfg.Ground)
	}
	a.settingsOpen = false
	tt.Frame()
	snapshot(t, tt, "ground-violet")
}

// The one bar, at the foot of every page of lists: one line until it is
// written in; what there is already shows over it; a ? asks the AI.
func TestOneBar(t *testing.T) {
	a, tt := newTestApp(t)
	if _, err := a.addMemo("缓存要加随机过期时间", "手动"); err != nil {
		t.Fatal(err)
	}
	a.showProjects()
	tt.Frame()
	if !tt.HasText("记灵感、加任务，或者问 AI…") || tt.HasText("加粗") {
		t.Fatalf("the bar is not one line on 项目: %q", tt.Texts())
	}
	snapshot(t, tt, "bar-closed")
	if err := tt.Click("记一条灵感或任务"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if !a.barOpen || !tt.HasText("B") {
		t.Fatal("the bar did not open with its tools")
	}
	tt.Type("缓存")
	tt.Frame()
	if !tt.HasText("已经有的") {
		t.Fatalf("what there is already is not shown: %q", tt.Texts())
	}
	snapshot(t, tt, "bar-found")
	a.homeDraft = "每周五 写周报"
	tt.Frame()
	if !tt.HasText("任务 · 周五 · 每周五") {
		t.Fatalf("not read as a task that repeats: %q", tt.Texts())
	}
	snapshot(t, tt, "bar-task")
	a.homeDraft = "https://example.com/a 一篇文章"
	tt.Frame()
	if !tt.HasText("收藏链接") {
		t.Fatal("not read as a link")
	}
	// A ? asks the AI, in its panel.
	a.homeDraft = "? 缓存怎么设计"
	tt.Frame()
	if !tt.HasText("问 AI") {
		t.Fatal("not read as a question")
	}
	a.saveHomeLine()
	tt.Frame()
	if !a.aiOpen || a.homeDraft != "" {
		t.Fatalf("the AI was not asked: open %v, draft %q", a.aiOpen, a.homeDraft)
	}
}

// What waits for a place says one by its words alone, with no AI asked,
// and one key puts it there.
func TestWaitingSaysItsPlace(t *testing.T) {
	a, tt := newTestApp(t)
	clip, err := a.addClipText("登录 功能", "手动")
	if err != nil {
		t.Fatal(err)
	}
	a.showLibrary()
	a.libHome = true
	tt.Frame()
	tt.Frame()
	p, ok := a.nearPlace("c:"+clip.ID, clip)
	if !ok || !strings.Contains(p.label, "登录") {
		t.Fatalf("no place by its words: %+v %v", p, ok)
	}
	if !tt.HasText("→ " + shortPlace(p)) {
		t.Fatalf("the card does not say it: %q", tt.Texts())
	}
	snapshot(t, tt, "today-places")
	if err := tt.Click("放过去 " + clip.Label()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.clipBacklinks(clip.ID)) != 1 {
		t.Fatal("not put there")
	}
}

// A thought is made a task by a link, and a task kept as a thought: each
// stays what it is, in its own place.
func TestThoughtAndTask(t *testing.T) {
	a, tt := newTestApp(t)
	memo, err := a.addMemo("换一家云服务商 #运维\n比较价格和迁移成本", "手动")
	if err != nil {
		t.Fatal(err)
	}
	a.openTasks()
	tt.Frame()
	if err := tt.Click("转成任务 换一家云服务商 #运维"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Frame()
	tt.Frame()
	var made *taskItem
	for _, it := range a.taskItems() {
		if it.node.Title == "换一家云服务商" {
			made = &it
		}
	}
	if made == nil || made.path != a.inboxPath() || !slices.Contains(made.node.Links, clipScheme+memo.ID) {
		t.Fatalf("no task of the thought: %+v", made)
	}
	if a.lib.Find(memo.ID) == nil {
		t.Fatal("the thought did not stay")
	}
	// The task's details read the thought it came of.
	a.tasks.sel = made.key()
	tt.Frame()
	if !tt.HasText("比较价格和迁移成本") {
		t.Fatalf("the thought is not read in the task: %q", tt.Texts())
	}
	snapshot(t, tt, "task-thought")

	// A task kept as a thought links to it; made one, it is a task no more.
	a.addTask("给车做保养", nil)
	var car taskItem
	for _, it := range a.taskItems() {
		if it.node.Title == "给车做保养" {
			car = it
		}
	}
	n := len(a.lib.Clips)
	a.taskToMemo(car, true)
	if len(a.lib.Clips) != n+1 || len(a.taskClips(find2(a, "给车做保养"))) != 1 {
		t.Fatalf("not kept as a thought: %d %d %+v", n, len(a.lib.Clips), find2(a, "给车做保养").Links)
	}
	a.taskToMemo(car, false)
	if find2(a, "给车做保养") != nil {
		t.Fatal("the task stayed")
	}
}

// find2 is the task of a title among every map's, or nil.
func find2(a *app, title string) *model.Node {
	a.tasks.read = time.Time{}
	for _, it := range a.taskItems() {
		if it.node.Title == title {
			return it.node
		}
	}
	return nil
}
