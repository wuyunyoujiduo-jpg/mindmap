package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// The keyboard goes through the tasks, ticks one off and puts its details
// away; what is ticked off, moved to a project or tagged can be taken back;
// the places and the bar have their keys.
func TestKeysAndWaysBack(t *testing.T) {
	a, tt := newTestApp(t)
	day := today(a.now())
	search := find(t, a, "搜索")
	a.mutate("", func() { d := day; search.Task.Due = &d })
	other := model.NewFile("发布计划")
	otherPath := filepath.Join(a.dir, model.FileName("发布计划"))
	if err := other.Save(otherPath); err != nil {
		t.Fatal(err)
	}
	a.refreshFiles()
	a.openTasks()
	tt.Frame()

	// Down chooses the first task, Escape puts its details away.
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	tt.Frame()
	if a.tasks.sel == "" || !tt.HasText("关闭详情") {
		t.Fatalf("down chose nothing: %q", a.tasks.sel)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.tasks.sel != "" {
		t.Fatal("escape left the details open")
	}
	// The space bar ticks off the one chosen, and it can be taken back.
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	tt.Frame()
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	tt.Frame()
	search = find(t, a, "搜索")
	if search.Task.Status != model.StatusDone || a.noticeUndo == nil {
		t.Fatalf("space: %v, undo %v", search.Task.Status, a.noticeUndo != nil)
	}
	a.noticeUndo()
	tt.Frame()
	if search = find(t, a, "搜索"); search.Task.Status == model.StatusDone {
		t.Fatal("not taken back")
	}
	// Dropped on a project, a task goes to it, and comes back.
	id := search.ID
	a.tasks.sel = ""
	tt.Frame()
	a.dropTaskOnMap(taskDrag{a.path, id}, otherPath)
	tt.Frame()
	if a.file.Node(id) != nil || a.indexed(otherPath).Node(id) == nil || a.noticeUndo == nil {
		t.Fatal("not moved to the other map")
	}
	a.noticeUndo()
	tt.Frame()
	if a.file.Node(id) == nil || a.indexed(otherPath).Node(id) != nil {
		t.Fatal("not moved back")
	}
	snapshot(t, tt, "keys-today")

	// A clip takes a tag dropped on, and loses it again.
	cl := &library.Clip{Kind: library.KindText, Text: "换一家云服务商"}
	if err := a.lib.Add(cl, a.now()); err != nil {
		t.Fatal(err)
	}
	a.tagClipUndoably(cl.ID, "运维")
	if !slices.Contains(a.lib.Find(cl.ID).Tags, "运维") || a.noticeUndo == nil {
		t.Fatal("no tag")
	}
	a.noticeUndo()
	if slices.Contains(a.lib.Find(cl.ID).Tags, "运维") {
		t.Fatal("tag not taken back")
	}

	// The places and the bar.
	tt.Key(ui.Cmd|ui.Alt, ui.Key2)
	tt.Frame()
	tt.Frame()
	if !tt.HasText("一张导图就是一个项目") {
		t.Fatalf("not on 项目: %q", tt.Texts())
	}
	tt.Key(ui.Cmd|ui.Alt, ui.Key3)
	tt.Frame()
	tt.Frame()
	if !tt.HasText("灵感和资料") {
		t.Fatalf("not on 收集: %q", tt.Texts())
	}
	// The arrows go through the wall, Escape closes what they opened.
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if a.libSel != cl.ID {
		t.Fatalf("down chose %q", a.libSel)
	}
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.libSel != "" {
		t.Fatal("escape left the clip open")
	}
	tt.Key(ui.Cmd, ui.KeyJ)
	tt.Frame()
	if !a.barOpen {
		t.Fatal("the bar did not open")
	}
}

// On the wall the arrows go where they point: right to the card beside,
// left back to it, down its column and up again.
func TestWallArrowsGoByPlace(t *testing.T) {
	a, tt := newTestApp(t)
	for i := range 9 {
		cl := &library.Clip{Kind: library.KindText, Text: fmt.Sprint("第 ", i, " 条")}
		if err := a.lib.Add(cl, a.now().Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	tt.SetSize(2000, 1000) // wide: several columns stay beside the details
	a.openList(kindAll, "")
	tt.Frame()
	key := func(k ui.Key) string {
		tt.Key(0, k)
		tt.Frame()
		tt.Frame()
		return a.libSel
	}
	first := key(ui.KeyDown)
	if first == "" {
		t.Fatal("down chose nothing")
	}
	beside := key(ui.KeyRight)
	if beside == first {
		t.Fatal("right stayed")
	}
	if got := key(ui.KeyLeft); got != first {
		t.Fatalf("left went to %q, not back to %q", got, first)
	}
	below := key(ui.KeyDown)
	if below == first || below == beside {
		t.Fatalf("down went to %q", below)
	}
	if got := key(ui.KeyUp); got != first {
		t.Fatalf("up went to %q, not back to %q", got, first)
	}
	// The card below is the one under the first on the screen.
	r1, _ := tt.Find("资料 " + a.lib.Find(first).Label())
	r2, ok := tt.Find("资料 " + a.lib.Find(below).Label())
	if !ok || r2.X != r1.X || r2.Y <= r1.Y {
		t.Fatalf("below is at %v, the first at %v", r2, r1)
	}
}

// A picture of the wall is read away from the window's work, small: its
// card is there at once, a pale block in the picture's room until then.
func TestWallPicturesReadAside(t *testing.T) {
	a, tt := newTestApp(t)
	img := image.NewRGBA(image.Rect(0, 0, 2000, 1200))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	cl, err := a.lib.AddData(buf.Bytes(), "shot.png", library.KindImage, "", a.now())
	if err != nil {
		t.Fatal(err)
	}
	var waiting []func()
	a.async = func(work func() (any, error), done func(any, error)) {
		waiting = append(waiting, func() { done(work()) })
	}
	a.openList(kindAll, "")
	tt.Frame()
	path := a.lib.FilePath(cl)
	if _, read := a.bitmaps["thumb:"+path]; read || len(waiting) != 1 || !tt.HasText("资料 "+cl.Label()) {
		t.Fatalf("read %v, waiting %d", read, len(waiting))
	}
	tt.Frame() // asked for once, however many frames pass
	if len(waiting) != 1 {
		t.Fatalf("asked for %d times", len(waiting))
	}
	waiting[0]()
	tt.Frame()
	b := a.bitmaps["thumb:"+path]
	if b == nil {
		t.Fatal("no picture")
	}
	if w, h := b.Size(); w > 2000/2 || h < thumbTall {
		t.Fatalf("not small: %d×%d", w, h)
	}
	snapshot(t, tt, "wall-picture")
}

// A key kept with a note is in dots on its card and its title, and where
// it goes to an AI; its details show it when asked.
func TestSecretsAreMasked(t *testing.T) {
	a, tt := newTestApp(t)
	const key = "sk-abcdefghijklmnopqrstuvwxyz012345"
	cl := &library.Clip{Kind: library.KindText, Text: "接口的密钥 " + key + " 先放这"}
	if err := a.lib.Add(cl, a.now()); err != nil {
		t.Fatal(err)
	}
	a.openList(kindAll, "")
	tt.Frame()
	shows := func() bool {
		for _, s := range tt.Texts() {
			if strings.Contains(s, key) {
				return true
			}
		}
		return false
	}
	if shows() || strings.Contains(cl.Label(), key) {
		t.Fatalf("the key shows on the wall: %q", tt.Texts())
	}
	if p := organizePromptFor(cl, clipPromptBody); strings.Contains(p, key) || !strings.Contains(p, "先放这") {
		t.Fatalf("the key goes to the AI: %q", p)
	}
	if strings.Contains(cardText(cl), key) {
		t.Fatal("the key is on the card to share")
	}
	a.libSel = cl.ID
	tt.Frame()
	if shows() {
		t.Fatal("the key shows in the details unasked")
	}
	snapshot(t, tt, "secret-masked")
	if err := tt.Click("显示密钥"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.secretShown != cl.ID || tt.HasText("显示密钥") { // whole again, in the field it is written in
		t.Fatalf("the key does not show when asked: %q", tt.Texts())
	}
	if a.lib.Find(cl.ID).Text != "接口的密钥 "+key+" 先放这" {
		t.Fatal("the clip was changed")
	}
}

// A task given a time reminds of itself at that time of its day, once.
func TestTaskRemindsAtItsTime(t *testing.T) {
	a, tt := newTestApp(t)
	plat := a.plat.(*fakePlatform)
	base := a.now()
	day := today(base)
	search := find(t, a, "搜索")
	a.mutate("", func() { d := day; search.Task.Due = &d })
	a.openTasks()
	tt.Frame()
	if err := tt.Click("任务 搜索"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("提醒 14:00"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if search = find(t, a, "搜索"); search.Task.Remind != "14:00" {
		t.Fatalf("remind %q", search.Task.Remind)
	}
	snapshot(t, tt, "task-remind")
	at := func(h, m int) {
		a.now = func() time.Time { return day.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute) }
		a.tasks.read = time.Time{}
		a.reminderCheck()
	}
	timed := func() int {
		n := 0
		for _, s := range plat.notes {
			if strings.HasPrefix(s, "到时间了 · 14:00") && strings.Contains(s, "搜索") {
				n++
			}
		}
		return n
	}
	at(13, 0)
	if timed() != 0 {
		t.Fatalf("too early: %q", plat.notes)
	}
	at(14, 1)
	at(14, 5)
	if timed() != 1 {
		t.Fatalf("reminded %d times: %q", timed(), plat.notes)
	}
}

// The search of everything finds a task and ticks it off or puts it on
// tomorrow where it is.
func TestSearchActsOnTasks(t *testing.T) {
	a, tt := newTestApp(t)
	day := today(a.now())
	search := find(t, a, "搜索")
	a.mutate("", func() { d := day; search.Task.Due = &d })
	a.openPalette(paletteAll)
	a.paletteQuery = "搜索"
	tt.Frame()
	tt.Frame()
	if !tt.HasText("改到明天 搜索") && !tt.HasText("任务") {
		t.Fatalf("no task found: %q", tt.Texts())
	}
	snapshot(t, tt, "search-task")
	if err := tt.Click("改到明天 搜索"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if search = find(t, a, "搜索"); !search.Task.Due.Equal(day.AddDate(0, 0, 1)) || !a.paletteOpen {
		t.Fatalf("due %v, open %v", search.Task.Due, a.paletteOpen)
	}
	if err := tt.Click("完成 搜索"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if search = find(t, a, "搜索"); search.Task.Status != model.StatusDone {
		t.Fatalf("status %v", search.Task.Status)
	}
}

// The week looked back on: what was done in it, what is late, what was kept.
func TestWeekReview(t *testing.T) {
	a, tt := newTestApp(t)
	day := today(a.now())
	search, login := find(t, a, "搜索"), find(t, a, "登录")
	a.mutate("", func() {
		search.SetStatus(model.StatusDone, a.now())
		late := day.AddDate(0, 0, -9)
		login.SetStatus(model.StatusTodo, a.now())
		login.Task.Due = &late
	})
	if err := a.lib.Add(&library.Clip{Kind: library.KindText, Text: "周报改成三段式"}, a.now()); err != nil {
		t.Fatal(err)
	}
	a.openTasks()
	tt.Frame()
	if err := tt.Click("本周回顾"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	for _, want := range []string{"这周做完的", "任务 搜索", "拖着的", "任务 登录", "这周记下的灵感", "周报改成三段式", "顺延到今天"} {
		if !tt.HasText(want) {
			t.Fatalf("no %q: %q", want, tt.Texts())
		}
	}
	snapshot(t, tt, "week-review")
}

// A thought that names another shows on it, and it on the thought.
func TestClipsNameEachOther(t *testing.T) {
	a, tt := newTestApp(t)
	first := &library.Clip{Kind: library.KindText, Text: "换一家云服务商"}
	if err := a.lib.Add(first, a.now()); err != nil {
		t.Fatal(err)
	}
	second := &library.Clip{Kind: library.KindText, Text: "迁移成本要算进去 <clip:" + first.ID + ">"}
	if err := a.lib.Add(second, a.now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	a.openList(kindAll, "")
	a.libSel = first.ID
	tt.Frame()
	if !tt.HasText("提到它的") || !tt.HasText("前往 "+second.Label()) {
		t.Fatalf("the first does not say who names it: %q", tt.Texts())
	}
	snapshot(t, tt, "clip-mentions")
	if err := tt.Click("前往 " + second.Label()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if a.libSel != second.ID || !tt.HasText("它提到的") {
		t.Fatalf("at %q: %q", a.libSel, tt.Texts())
	}
}

// A map shows the tasks not done alone, the rest dimmed.
func TestMapShowsOpenTasksAlone(t *testing.T) {
	a, tt := newTestApp(t)
	search, login := find(t, a, "搜索"), find(t, a, "登录")
	a.mutate("", func() { login.SetStatus(model.StatusDone, a.now()) })
	tt.Frame()
	if err := tt.Click("筛选任务"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("没做完的任务"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.filtering() || !a.highlighted(search) || a.highlighted(login) || a.highlighted(a.doc.Root) {
		t.Fatalf("open %v, done %v, root %v", a.highlighted(search), a.highlighted(login), a.highlighted(a.doc.Root))
	}
	snapshot(t, tt, "map-open-tasks")
}

func TestParseRemind(t *testing.T) {
	for _, c := range []struct{ in, rest, remind string }{
		{"明天 9点 写周报", "明天 写周报", "09:00"},
		{"明天9点半 写周报", "明天 写周报", "09:30"},
		{"下午3点 开会", "开会", "15:00"},
		{"今晚 8点 跑步", "今晚 跑步", "20:00"},
		{"周五 15:30 评审", "周五 评审", "15:30"},
		{"晚上8点15 吃药", "吃药", "20:15"},
		{"写3点总结", "写3点总结", ""},
		{"写周报", "写周报", ""},
		{"准备 3点建议", "准备 3点建议", ""},
		{"25点 不存在", "25点 不存在", ""},
	} {
		rest, remind := parseRemind(c.in)
		if strings.Join(strings.Fields(rest), " ") != c.rest || remind != c.remind {
			t.Errorf("%q → %q, %q; want %q, %q", c.in, rest, remind, c.rest, c.remind)
		}
	}
}

// A time written with a task is when it reminds; the week's review goes
// back a week and is copied as a report; the clips with a key have their
// list; Escape leaves a field before it closes the pane.
func TestSmallWaysIn(t *testing.T) {
	a, tt := newTestApp(t)
	plat := a.plat.(*fakePlatform)
	a.addTask("明天 9点半 交报销", nil)
	tt.Frame()
	var got *taskItem
	for _, it := range a.taskItems() {
		if it.node.Title == "交报销" {
			got = &it
		}
	}
	if got == nil || got.node.Task.Remind != "09:30" || !got.node.Task.Due.Equal(today(a.now()).AddDate(0, 0, 1)) {
		t.Fatalf("task %+v", got)
	}

	search := find(t, a, "搜索")
	a.mutate("", func() { search.SetStatus(model.StatusDone, a.now()) })
	a.openTasks()
	a.tasks.view = tasksReview
	tt.Frame()
	if err := tt.Click("复制成周报"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plat.copied, "## 做完的（1）") || !strings.Contains(plat.copied, "- 搜索") {
		t.Fatalf("report %q", plat.copied)
	}
	if err := tt.Click("上一周"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.tasks.reviewBack != 1 || !tt.HasText("这周还没有可以回顾的") {
		t.Fatalf("back %d: %q", a.tasks.reviewBack, tt.Texts())
	}

	cl := &library.Clip{Kind: library.KindText, Text: "TOKEN=abcdefgh12345678"}
	if err := a.lib.Add(cl, a.now()); err != nil {
		t.Fatal(err)
	}
	if err := a.lib.Add(&library.Clip{Kind: library.KindText, Text: "没有密钥的一条"}, a.now()); err != nil {
		t.Fatal(err)
	}
	a.openList(kindAll, "")
	tt.Frame()
	if err := tt.Click("含密钥 1"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if hits := a.libHits(); len(hits) != 1 || hits[0].Clip.ID != cl.ID {
		t.Fatalf("%d clips with a key", len(hits))
	}
	snapshot(t, tt, "secrets-list")

	// In a field of the details, Escape leaves the field; again, it closes.
	a.libSel, a.secretShown = cl.ID, cl.ID
	tt.Frame()
	if err := tt.Click("资料内容"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	tt.Frame()
	if a.libSel != cl.ID {
		t.Fatal("escape closed the details from a field")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.libSel != "" {
		t.Fatal("the second escape did not close the details")
	}
}

// A task moved to a project and taken back is where it was among the
// others, not after them all.
func TestMoveBackKeepsItsPlace(t *testing.T) {
	a, tt := newTestApp(t)
	search := find(t, a, "搜索")
	d := a.file.SheetOf(search.ID)
	_, parent := d.Find(search.ID)
	if len(parent.Children) < 2 {
		a.mutate("", func() { parent.Children = append(parent.Children, model.NewNode("别的")) })
	}
	a.mutate("", func() { // the first of its parent
		i := slices.IndexFunc(parent.Children, func(n *model.Node) bool { return n.ID == search.ID })
		parent.Children[0], parent.Children[i] = parent.Children[i], parent.Children[0]
		day := today(a.now())
		search.Task.Due = &day
	})
	other := model.NewFile("发布计划")
	otherPath := filepath.Join(a.dir, model.FileName("发布计划"))
	if err := other.Save(otherPath); err != nil {
		t.Fatal(err)
	}
	a.refreshFiles()
	a.openTasks()
	tt.Frame()
	id, pid := search.ID, parent.ID
	a.dropTaskOnMap(taskDrag{a.path, id}, otherPath)
	tt.Frame()
	if a.noticeUndo == nil || a.file.Node(id) != nil {
		t.Fatal("not moved")
	}
	a.noticeUndo()
	tt.Frame()
	p := a.file.Node(pid)
	if p == nil || len(p.Children) == 0 || p.Children[0].ID != id {
		t.Fatalf("not back at the head of its parent: %v", p)
	}
}

// The app is light, dark, or as the system is, by the settings.
func TestAppearanceIsChosen(t *testing.T) {
	a, tt := newTestApp(t)
	plat := a.plat.(*fakePlatform)
	a.openSettings()
	for tab := range 8 {
		a.settingsTab = tab
		tt.Frame()
		if tt.HasText("深浅 深色") {
			break
		}
	}
	if err := tt.Click("深浅 深色"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.cfg.Appearance != "dark" || plat.look != "dark" {
		t.Fatalf("appearance %q, platform %q", a.cfg.Appearance, plat.look)
	}
	snapshot(t, tt, "settings-appearance")
	if err := tt.Click("深浅 跟随系统"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.cfg.Appearance != "" || plat.look != "" {
		t.Fatalf("appearance %q, platform %q", a.cfg.Appearance, plat.look)
	}
}

// A kind and a tag of the left column narrow together, the page's head
// says both, and with nothing left the page names them and clears them.
func TestKindAndTagTogether(t *testing.T) {
	a, tt := newTestApp(t)
	for i, cl := range []*library.Clip{
		{Kind: library.KindText, Text: "一条灵感 #待识别", Tags: []string{"待识别"}},
		{Kind: library.KindLink, URL: "https://example.com/a", Title: "一个链接", Tags: []string{"别的"}},
	} {
		if err := a.lib.Add(cl, a.now().Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	a.openList(kindAll, "")
	tt.Frame()
	if !tt.HasText("灵感和资料") {
		t.Fatalf("head: %q", tt.Texts())
	}
	if err := tt.Click("#待识别 1"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("链接"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if a.libKind != kindLinks || a.libTag != "待识别" {
		t.Fatalf("kind %d, tag %q: choosing a kind dropped the tag", a.libKind, a.libTag)
	}
	for _, want := range []string{"条件 链接", "条件 #待识别", "没有符合「链接 · #待识别」的资料", "0 条符合", "全部清除"} {
		if !tt.HasText(want) {
			t.Fatalf("no %q: %q", want, tt.Texts())
		}
	}
	snapshot(t, tt, "library-filters")
	// A condition's own ✕ takes off that one, and leaves the other.
	if err := tt.Click("去掉 链接"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if a.libKind != kindAll || a.libTag != "待识别" || len(a.libHits()) != 1 || tt.HasText("全部清除") {
		t.Fatalf("kind %d, tag %q, %d found", a.libKind, a.libTag, len(a.libHits()))
	}
	if err := tt.Click("链接"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("清除筛选"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if a.libKind != kindAll || a.libTag != "" || len(a.libHits()) != 2 {
		t.Fatalf("not cleared: kind %d, tag %q", a.libKind, a.libTag)
	}
}

// What is collected waits for nothing; asked to be placed later, it waits
// on 今天 until it is on a map or let off.
func TestPlacingIsAsked(t *testing.T) {
	a, tt := newTestApp(t)
	cl := &library.Clip{Kind: library.KindText, Text: "换一家云服务商"}
	if err := a.lib.Add(cl, a.now()); err != nil {
		t.Fatal(err)
	}
	if n := len(a.waiting()); n != 0 {
		t.Fatalf("%d wait unasked", n)
	}
	a.openList(kindAll, "")
	a.libSel = cl.ID
	tt.Frame()
	if err := tt.Click("稍后归位"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Frame()
	tt.Frame()
	if w := a.waiting(); len(w) != 1 || w[0].clip.ID != cl.ID || a.noticeUndo == nil || !tt.HasText("待归位") {
		t.Fatalf("%d wait", len(w))
	}
	snapshot(t, tt, "clip-to-place")
	a.noticeUndo()
	if n := len(a.waiting()); n != 0 {
		t.Fatalf("%d wait after taking it back", n)
	}
}

// 今天's left column: the inbox is no project there, a project's count is
// the lists' count, a project chosen leaves no list chosen, what is late
// has its list, a project's row opens its map, and the many are folded.
func TestTodaySideProjects(t *testing.T) {
	a, tt := newTestApp(t)
	day := today(a.now())
	login := find(t, a, "登录")
	a.mutate("", func() {
		late := day.AddDate(0, 0, -3)
		login.SetStatus(model.StatusTodo, a.now())
		login.Task.Due = &late
	})
	a.addTask("交报销", nil) // into the inbox
	search := find(t, a, "搜索")
	a.mutate("", func() { search.SetStatus(model.StatusDone, a.now()) }) // so that a ring shows how far
	for i := range 8 {
		f := model.NewFile(fmt.Sprint("项目", i))
		n := model.NewNode("一件事")
		n.SetStatus(model.StatusTodo, a.now())
		f.Sheets[0].Root.Children = append(f.Sheets[0].Root.Children, n)
		if err := f.Save(filepath.Join(a.dir, model.FileName(fmt.Sprint("项目", i)))); err != nil {
			t.Fatal(err)
		}
	}
	a.refreshFiles()
	// One starred, with nothing to do: it heads the column all the same.
	quiet := model.NewFile("常看的")
	quietPath := filepath.Join(a.dir, model.FileName("常看的"))
	if err := quiet.Save(quietPath); err != nil {
		t.Fatal(err)
	}
	a.refreshFiles()
	a.toggleFavorite(quietPath)
	a.openTasks()
	tt.Frame()
	tt.Frame()
	if !tt.HasText("快捷访问") || !tt.HasText("只看 常看的") {
		t.Fatalf("no 快捷访问: %q", tt.Texts())
	}
	if tt.HasText("只看 收件箱") {
		t.Fatal("the inbox is listed among the projects")
	}
	// The projects' counts add up to no more than 全部's.
	sum := 0
	for _, it := range a.taskItems() {
		if countsAsTask(it) && it.path != a.inboxPath() {
			sum++
		}
	}
	all, _ := strconv.Atoi(a.taskListCounts(a.taskItems())[tasksAll])
	inbox, _ := strconv.Atoi(a.taskListCounts(a.taskItems())[tasksInbox])
	if sum+inbox != all {
		t.Fatalf("projects %d + inbox %d, 全部 %d", sum, inbox, all)
	}
	// Folded: a few, and the way to them all.
	if !tt.HasText("全部项目") || tt.HasText("只看 项目7") && tt.HasText("只看 项目0") && tt.HasText("只看 项目3") && tt.HasText("只看 项目5") && tt.HasText("只看 项目1") && tt.HasText("只看 项目2") && tt.HasText("只看 项目4") && tt.HasText("只看 项目6") {
		t.Fatalf("not folded: %q", tt.Texts())
	}
	snapshot(t, tt, "today-side")
	if err := tt.Click("全部项目"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("只看 项目7") || !tt.HasText("只看 项目0") {
		t.Fatalf("not all shown: %q", tt.Texts())
	}
	// What is late, alone.
	if err := tt.Click("已逾期 1"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Frame()
	tt.Frame()
	if a.tasks.view != tasksLate || !tt.HasText("任务 登录") || tt.HasText("任务 交报销") {
		t.Fatalf("late: %q", tt.Texts())
	}
	// A project's row opens its map by the key under the pointer.
	// Under the pointer, a project's count gives way to the key to its map.
	row, ok := tt.Find("只看 项目0")
	if !ok {
		t.Fatal("no row")
	}
	tt.Move(row.X+row.W/2, row.Y+row.H/2)
	tt.Frame()
	tt.Frame()
	if err := tt.Click("打开导图 项目0"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Frame()
	if mapName(a.path) != "项目0" {
		t.Fatalf("at %q", a.path)
	}
}
