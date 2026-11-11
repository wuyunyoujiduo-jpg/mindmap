package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

func TestParseTaskLine(t *testing.T) {
	now := time.Date(2026, 10, 10, 9, 0, 0, 0, time.Local) // a Saturday
	day := today(now)
	for _, c := range []struct {
		in, title string
		due       time.Time
		prio      model.Priority
	}{
		{"明天 写周报 !高", "写周报", day.AddDate(0, 0, 1), model.PriorityHigh},
		{"写周报", "写周报", time.Time{}, model.PriorityNone},
		{"后天交方案", "交方案", day.AddDate(0, 0, 2), model.PriorityNone},
		{"周一 开会", "开会", day.AddDate(0, 0, 2), model.PriorityNone},
		{"下周三 评审 !2", "评审", day.AddDate(0, 0, 4), model.PriorityMedium}, // the week from Monday the 12th
		{"11月20日 发版", "发版", time.Date(2026, 11, 20, 0, 0, 0, 0, time.Local), model.PriorityNone},
		{"10/12 体检", "体检", time.Date(2026, 10, 12, 0, 0, 0, 0, time.Local), model.PriorityNone},
		{"25号 交房租", "交房租", time.Date(2026, 10, 25, 0, 0, 0, 0, time.Local), model.PriorityNone},
		{"5号 交房租", "交房租", time.Date(2026, 11, 5, 0, 0, 0, 0, time.Local), model.PriorityNone},
	} {
		title, due, prio := parseTaskLine(c.in, now)
		got := time.Time{}
		if due != nil {
			got = *due
		}
		if title != c.title || !got.Equal(c.due) || prio != c.prio {
			t.Errorf("%q: %q %v %v, want %q %v %v", c.in, title, got, prio, c.title, c.due, c.prio)
		}
	}
}

func TestTasksPage(t *testing.T) {
	a, tt := newTestApp(t)
	day := today(a.now())
	at := func(d time.Time) *time.Time { return &d }
	// Tasks of the open map, and of another.
	login := find(t, a, "登录")
	search := find(t, a, "搜索")
	a.mutate("", func() {
		login.SetStatus(model.StatusTodo, a.now())
		login.Task.Due = at(day.AddDate(0, 0, -2))
		search.Task.Due = at(day)
	})
	other := model.NewFile("发布计划")
	n := model.NewNode("准备发布说明")
	n.SetStatus(model.StatusTodo, a.now())
	n.Task.Due, n.Task.Priority = at(day.AddDate(0, 0, 3)), model.PriorityHigh
	other.Sheets[0].Root.Children = append(other.Sheets[0].Root.Children, n)
	otherPath := filepath.Join(a.dir, model.FileName("发布计划"))
	if err := other.Save(otherPath); err != nil {
		t.Fatal(err)
	}
	a.refreshFiles()

	a.openTasks()
	tt.Frame()
	if !a.tasks.open || !tt.HasText("已逾期") || !tt.HasText("登录") || !tt.HasText("搜索") || tt.HasText("准备发布说明") {
		t.Fatalf("today: %q", tt.Texts())
	}
	snapshot(t, tt, "tasks-today")
	// The next 7 days hold the other map's.
	a.tasks.view = tasksWeekAhead
	tt.Frame()
	if !tt.HasText("准备发布说明") || !tt.HasText("发布计划") {
		t.Fatalf("7 days: %q", tt.Texts())
	}
	snapshot(t, tt, "tasks-7days")

	// Added in a line, to the inbox, on its day.
	tt.Click("添加任务")
	tt.Type("明天 写周报 !高")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	inbox, err := model.Load(a.inboxPath())
	if err != nil {
		t.Fatal(err)
	}
	var added *model.Node
	for _, c := range inbox.Sheets[0].Root.Children {
		if c.Title == "写周报" {
			added = c
		}
	}
	if added == nil || added.Task == nil || !added.Task.Due.Equal(day.AddDate(0, 0, 1)) || added.Task.Priority != model.PriorityHigh {
		t.Fatalf("added %+v", added)
	}
	tt.Frame()
	if !tt.HasText("写周报") {
		t.Fatalf("not listed: %q", tt.Texts())
	}

	// Ticked off, in the other map's file.
	if err := tt.Click("完成 准备发布说明"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	f, _ := model.Load(otherPath)
	if got := f.Node(n.ID); got.Task.Status != model.StatusDone {
		t.Fatalf("status %v", got.Task.Status)
	}
	// Dated anew, as a drop on a day does; and in the open map, undoably.
	for _, it := range a.taskItems() {
		if it.node.ID == login.ID {
			a.setTaskDay(it, at(day.AddDate(0, 0, 1)))
		}
	}
	if !login.Task.Due.Equal(day.AddDate(0, 0, 1)) {
		t.Fatalf("login due %v", login.Task.Due)
	}

	// The calendar: the week, the month.
	a.tasks.view = tasksWeek
	tt.Frame()
	if !tt.HasText("登录") || !tt.HasText("写周报") {
		t.Fatalf("week: %q", tt.Texts())
	}
	snapshot(t, tt, "tasks-week")
	a.tasks.view = tasksMonth
	tt.Frame()
	if !tt.HasText("搜索") {
		t.Fatalf("month: %q", tt.Texts())
	}
	snapshot(t, tt, "tasks-month")
	a.tasks.view = tasksDay
	tt.Frame()
	snapshot(t, tt, "tasks-day")
	_ = os.Remove
}

// Dragged with the pointer to another day, a task is dated to it.
func TestTasksDrag(t *testing.T) {
	a, tt := newTestApp(t)
	day := today(a.now())
	search := find(t, a, "搜索")
	a.mutate("", func() { d := day; search.Task.Due = &d })
	a.openTasks()
	a.tasks.view = tasksWeek
	tt.Frame()
	src, ok := tt.Find("任务 搜索")
	if !ok {
		t.Fatalf("no chip: %q", tt.Texts())
	}
	// The column of the day after (or before, on a Sunday).
	target := day.AddDate(0, 0, 1)
	if weekStart(target) != weekStart(day) {
		target = day.AddDate(0, 0, -1)
	}
	dx := float32(1)
	if target.Before(day) {
		dx = -1
	}
	x0, y0 := src.X+src.W/2, src.Y+src.H/2
	x1 := x0 + dx*src.W*1.1
	tt.Press(x0, y0)
	for i := 1; i <= 6; i++ {
		tt.Move(x0+(x1-x0)*float32(i)/6, y0+40)
	}
	tt.Release(x1, y0+40)
	tt.Frame()
	tt.Frame()
	if !search.Task.Due.Equal(target) {
		t.Fatalf("due %v, want %v", search.Task.Due, target)
	}
}

// In the lists, a task dragged onto a day's group, its tasks included, goes
// to that day; an empty day of the next 7 shows to take it.
func TestTasksDragInList(t *testing.T) {
	a, tt := newTestApp(t)
	day := today(a.now())
	search, login := find(t, a, "搜索"), find(t, a, "登录")
	a.mutate("", func() {
		d, e := day, day.AddDate(0, 0, 1)
		search.Task.Due = &d
		login.SetStatus(model.StatusTodo, a.now())
		login.Task.Due = &e
	})
	a.openTasks()
	a.tasks.view = tasksWeekAhead
	tt.Frame()
	src, _ := tt.Find("任务 搜索")
	dst, ok := tt.Find("任务 登录") // tomorrow's, the drop is on its row
	if !ok {
		t.Fatalf("rows: %q", tt.Texts())
	}
	drag := func(from, to ui.Rect) {
		x0, y0, x1, y1 := from.X+40, from.Y+from.H/2, to.X+60, to.Y+to.H/2
		tt.Press(x0, y0)
		for i := 1; i <= 6; i++ {
			tt.Move(x0+(x1-x0)*float32(i)/6, y0+(y1-y0)*float32(i)/6)
		}
		tt.Release(x1, y1)
		tt.Frame()
		tt.Frame()
	}
	drag(src, dst)
	if !search.Task.Due.Equal(day.AddDate(0, 0, 1)) {
		t.Fatalf("due %v", search.Task.Due)
	}
	// An empty day, 3 days on, shows and takes it.
	label := weekdayName(day.AddDate(0, 0, 3))
	var head ui.Rect
	found := false
	for _, s := range tt.Texts() {
		if strings.HasPrefix(s, label+"  ") { // the list's heading of the day, not the week's tile
			head, found = tt.Find(s)
			break
		}
	}
	if !found {
		t.Fatalf("no empty day: %q", tt.Texts())
	}
	src, _ = tt.Find("任务 搜索")
	drag(src, head)
	if !search.Task.Due.Equal(day.AddDate(0, 0, 3)) {
		t.Fatalf("due %v, want 3 days on", search.Task.Due)
	}
	// The month's cells too.
	a.tasks.view = tasksMonth
	tt.Frame()
	src, _ = tt.Find("任务 搜索")
	target := day.AddDate(0, 0, 7)
	cell, ok := tt.Find(fmt.Sprint(target.Day())) // its day's number, once in the month
	if !ok {
		t.Fatal("no day", target.Day())
	}
	drag(src, ui.Rect{X: cell.X - 40, Y: cell.Y + 30, W: 1, H: 1})
	if !search.Task.Due.Equal(target) {
		t.Fatalf("month: due %v, want %v", search.Task.Due, target)
	}
}

// A click shows a task's details: renamed, noted, given a subtask and a
// priority, in its map.
func TestTaskDetail(t *testing.T) {
	a, tt := newTestApp(t)
	day := today(a.now())
	search := find(t, a, "搜索")
	a.mutate("", func() { d := day; search.Task.Due = &d })
	a.openTasks()
	tt.Frame()
	if err := tt.Click("任务 搜索"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.tasks.sel == "" || !tt.HasText("子任务") || !tt.HasText("在导图中打开") {
		t.Fatalf("details: %q", tt.Texts())
	}
	// The title, saved as it is left.
	tt.Click("任务名")
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("全文搜索")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if search.Title != "全文搜索" {
		t.Fatalf("title %q", search.Title)
	}
	// A note, saved as the focus leaves it.
	tt.Click("任务备注")
	tt.Type("用 FTS5")
	tt.Click("添加子任务")
	tt.Frame()
	if search.Note != "用 FTS5" {
		t.Fatalf("note %q", search.Note)
	}
	tt.Type("建索引")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if len(search.Children) != 1 || search.Children[0].Title != "建索引" || search.Children[0].Task == nil {
		t.Fatalf("subtasks %+v", search.Children)
	}
	// Ticked off, and the priority set.
	if err := tt.Click("完成子任务 建索引"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if search.Children[0].Task.Status != model.StatusDone || !tt.HasText("子任务 1/1") {
		t.Fatalf("subtask %v", search.Children[0].Task.Status)
	}
	if err := tt.Click("高"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if search.Task.Priority != model.PriorityHigh {
		t.Fatalf("priority %v", search.Task.Priority)
	}
	snapshot(t, tt, "tasks-detail")
}

// A task of the inbox moves under a topic of a map, from the tasks' page.
func TestTaskMoveToMap(t *testing.T) {
	a, tt := newTestApp(t)
	guide := a.path
	a.addTask("明天 写周报", nil)
	tt.Frame()
	a.openTasks()
	a.tasks.view = tasksInbox
	tt.Frame()
	var it taskItem
	for _, x := range a.taskItems() {
		if x.node.Title == "写周报" {
			it = x
		}
	}
	if it.node == nil {
		t.Fatal("no task in the inbox")
	}
	a.moveTaskTo(it)
	tt.Frame()
	if !a.paletteOpen || !a.tasks.open {
		t.Fatalf("palette %v, tasks %v", a.paletteOpen, a.tasks.open)
	}
	tt.Type("待优化")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	g, err := model.Load(guide)
	if err != nil {
		t.Fatal(err)
	}
	opt, err := findIn(g.Sheets[0], "待优化")
	if err != nil || opt.Children[len(opt.Children)-1].Title != "写周报" || opt.Children[len(opt.Children)-1].Task == nil {
		t.Fatalf("not moved: %v", err)
	}
	if !a.tasks.open {
		t.Fatal("left the tasks' page")
	}
}

// The AI says where the inbox's tasks belong; taken, they move there.
func TestInboxPlaces(t *testing.T) {
	a, tt := newTestApp(t)
	guide := a.path
	a.addTask("明天 首页加载优化", nil)
	a.addTask("买咖啡豆", nil)
	tt.Frame()
	a.openTasks()
	a.tasks.view = tasksInbox
	tt.Frame()
	if !tt.HasText("AI 归位") {
		t.Fatalf("no button: %q", tt.Texts())
	}
	fake := &fakeClaude{}
	a.sendAI = func(ctx context.Context, cfg config, p anthropic.BetaMessageNewParams, on func(string)) (*anthropic.BetaMessage, error) {
		// The topic of 待优化, by its number in the list sent.
		prompt := p.Messages[0].Content[0].OfText.Text
		ref := ""
		for _, line := range strings.Split(prompt, "\n") {
			if strings.HasSuffix(line, "› 待优化") {
				ref = strings.Fields(line)[0]
			}
		}
		fake.replies = []string{reply(fmt.Sprintf(`{"places":[{"task":1,"topic":"%s"},{"task":9,"topic":"t999"}]}`, ref))}
		return fake.send(ctx, cfg, p, on)
	}
	a.cfg.AIKey = "sk-ant-test"
	if err := tt.Click("AI 归位"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.tasks.places) != 1 || !tt.HasText("放过去 首页加载优化") {
		t.Fatalf("places %v, err %q", a.tasks.places, a.tasks.placeErr)
	}
	snapshot(t, tt, "tasks-places")
	if err := tt.Click("放过去 首页加载优化"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	a.flush()
	g, _ := model.Load(guide)
	opt, err := findIn(g.Sheets[0], "待优化")
	if err != nil || opt.Children[len(opt.Children)-1].Title != "首页加载优化" {
		t.Fatalf("not moved: %v", err)
	}
	in, _ := model.Load(a.inboxPath())
	if len(in.Sheets[0].Root.Children) != 1 || in.Sheets[0].Root.Children[0].Title != "买咖啡豆" {
		t.Fatalf("inbox %d", len(in.Sheets[0].Root.Children))
	}
}

func TestTaskLike(t *testing.T) {
	for text, want := range map[string]bool{
		"明天 写周报": true, "写周报 !高": true, "周五 交报告": true, "- [ ] 买票\n- [ ] 订酒店": true, "11月20日 发版": true,
		"今天很好": false, "明天": false, "产品要做减法 #产品": false, "第一行\n明天 第二行": false, "": false,
	} {
		if got := taskLike(text, testNow); got != want {
			t.Errorf("%q: %v", text, got)
		}
	}
}
