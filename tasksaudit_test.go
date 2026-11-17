package main

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// What the reading of the whole found of 今天 (docs/审查清单.md, 83 to 114):
// a test an entry, each failing as the entry says without its mending.

// taskNamed is the task of the name, as the tasks' page has it now.
func taskNamed(t *testing.T, a *app, title string) taskItem {
	t.Helper()
	a.tasks.read = time.Time{}
	for _, it := range a.taskItems() {
		if it.node.Title == title {
			return it
		}
	}
	t.Fatalf("no task %q", title)
	return taskItem{}
}

// A line is read one way (readTaskLine): the sentences the reading listed.
func TestTaskLinesRead(t *testing.T) {
	now := testNow // Saturday the 10th of October 2026, 9:00
	day := today(now)
	on := func(n int) string { return day.AddDate(0, 0, n).Format("2006-01-02") }
	for _, c := range []struct {
		in          string
		today       bool // written where a line with no day is today's
		title, due  string
		from, until string
		repeat      string
	}{
		// A way to repeat is a word of its own.
		{in: "工作日报 已提交", title: "工作日报 已提交"},
		{in: "工作日志整理一下", title: "工作日志整理一下"},
		{in: "每天都很累 想休息", title: "每天都很累 想休息"},
		{in: "每周一次 复盘", title: "每周一次 复盘"},
		{in: "工作日 站会", title: "站会", due: on(2), repeat: "工作日"}, // a Saturday: from Monday
		{in: "每天 吃药", title: "吃药", due: on(0), repeat: "每天"},
		// A day of a month is written with 月 or a slash.
		{in: "升级到 1.2 版本", title: "升级到 1.2 版本"},
		{in: "买 2-3 个苹果", title: "买 2-3 个苹果"},
		{in: "看 3.5 寸屏", title: "看 3.5 寸屏"},
		{in: "11月20日 发版", title: "发版", due: "2026-11-20"},
		{in: "10/12 体检", title: "体检", due: "2026-10-12"},
		// A time after a day's word, or at the line's head, needs no space.
		{in: "明天9点开会", title: "开会", due: on(1), from: "09:00"},
		{in: "周五9点半开会", title: "开会", due: on(6), from: "09:30"},
		{in: "9点开会", today: true, title: "开会", due: on(0), from: "09:00"},
		{in: "写3点总结", today: true, title: "写3点总结", due: on(0)},
		{in: "准备 3点建议", today: true, title: "准备 3点建议", due: on(0)},
		// Midnight is the day after's first hour; a span ends in its day.
		{in: "晚上12点 交稿", title: "交稿", due: on(1), from: "00:00"},
		{in: "明天 晚上12点 交稿", title: "交稿", due: on(2), from: "00:00"},
		{in: "中午12点 吃饭", title: "吃饭", due: on(0), from: "12:00"},
		{in: "晚上11点到1点 值班", title: "值班", due: on(0), from: "23:00"},
		{in: "22:00-01:00 值班", title: "值班", due: on(0), from: "22:00"},
		{in: "9点到9点 站会", title: "站会", due: on(0), from: "09:00"},
		{in: "晚上10点到12点 加班", title: "加班", due: on(0), from: "22:00", until: "23:59"},
		{in: "下午3点到5点 写方案", title: "写方案", due: on(0), from: "15:00", until: "17:00"},
		{in: "每周五 晚上12点 关灯", title: "晚上12点 关灯", due: on(6), repeat: "每周五"}, // the day after a day of its own: not read
		// A day the calendar has not is no day (October has a 31st).
		{in: "31号 交报告", title: "交报告", due: "2026-10-31"},
		{in: "2月30日 体检", title: "2月30日 体检"},
		{in: "2月29日 体检", title: "2月29日 体检"}, // 2027 has none
		// By the week and the month, from its day; what repeats has a day.
		{in: "每周 开例会", title: "开例会", due: on(0), repeat: "每周"},
		{in: "每月 交房租", title: "交房租", due: on(0), repeat: "每月"},
		{in: "每2周 复盘", title: "复盘", due: on(0), repeat: "每 2 周"},
		{in: "每年 11月20日 体检", title: "体检", due: "2026-11-20", repeat: "每年"},
	} {
		var where *time.Time
		if c.today {
			where = &day
		}
		tl := readTaskLine(c.in, now, where, "", "")
		due, repeat := "", ""
		if tl.due != nil {
			due = tl.due.Format("2006-01-02")
		}
		if tl.repeat != nil {
			repeat = tl.repeat.Label()
		}
		if tl.title != c.title || due != c.due || tl.from != c.from || tl.until != c.until || repeat != c.repeat || tl.remind != c.from {
			t.Errorf("%q → %q on %q from %q to %q, %q (reminding %q); want %q on %q from %q to %q, %q",
				c.in, tl.title, due, tl.from, tl.until, repeat, tl.remind, c.title, c.due, c.from, c.until, c.repeat)
		}
	}
	// None of these reads as a task where a line is a note unless it says so.
	for _, s := range []string{"工作日报 已提交", "工作日志整理一下", "每天都很累 想休息", "升级到 1.2 版本", "买 2-3 个苹果", "看 3.5 寸屏"} {
		if taskLike(s, now) {
			t.Errorf("%q reads as a task", s)
		}
	}
	for _, s := range []string{"每周 开例会", "每月 交房租", "工作日 站会", "- [ ] 买菜\n- [ ] 做饭"} {
		if !taskLike(s, now) {
			t.Errorf("%q does not read as a task", s)
		}
	}
	// A month of thirty days has no 31st: no day, and no first of the next.
	nov := time.Date(2026, 11, 17, 10, 0, 0, 0, time.Local)
	if title, due, _ := parseTaskLine("31号 交报告", nov); due != nil || title != "31号 交报告" {
		t.Errorf("31号 in November: %q, %v", title, due)
	}
	if _, due, _ := parseTaskLine("30号 交报告", nov); due == nil || due.Format("01-02") != "11-30" {
		t.Errorf("30号 in November: %v", due)
	}
	jan := time.Date(2027, 1, 31, 10, 0, 0, 0, time.Local)
	if _, due, _ := parseTaskLine("30号 交报告", jan); due != nil {
		t.Errorf("30号 on the 31st of January, February having none: %v", due)
	}
	// A time written of a task that has its day: midnight is not read.
	if _, from, _ := parseClock("晚上12点"); from != "" {
		t.Errorf("晚上12点 of a day: %q", from)
	}
}

// What a line shows as it is written is what is kept of it: the pills of
// the line to add a task and the bar's tag read as the task made does.
func TestTaskLineShownAsKept(t *testing.T) {
	a, tt := newTestApp(t)
	a.openTasks()
	a.tasks.view = tasksAll // a list with no day of its own
	a.tasks.add = "明天9点到10点半 开会 !高"
	tt.Frame()
	tt.Frame()
	for _, want := range []string{"明天 9:00–10:30", "高"} {
		if !tt.HasText(want) {
			t.Fatalf("the line to add a task does not say %q: %q", want, tt.Texts())
		}
	}
	snapshot(t, tt, "task-line-read")
	a.tasks.add = "每周 开例会"
	tt.Frame()
	if !tt.HasText("今天") || !tt.HasText("每周") {
		t.Fatalf("what repeats says no day: %q", tt.Texts())
	}
	a.tasks.add = ""
	a.addTask("明天9点到10点半 开会 !高", nil)
	task := taskNamed(t, a, "开会").node.Task
	if task.Span() != "9:00–10:30" || task.Remind != "09:00" || !sameDay(*task.Due, today(testNow).AddDate(0, 0, 1)) || task.Priority != model.PriorityHigh {
		t.Fatalf("kept: %+v", task)
	}
	if !strings.Contains(a.notice, "明天 9:00–10:30") {
		t.Fatalf("said %q", a.notice)
	}
	// The bar's tag, with its time.
	a.goToday()
	a.barOpen, a.homeDraft = true, "明天9点 写周报"
	tt.Frame()
	tt.Frame()
	if !tt.HasText("记为 任务 · 明天 9:00") {
		t.Fatalf("the bar's tag: %q", tt.Texts())
	}
	snapshot(t, tt, "bar-line-read")
}

// A checklist written in the bar is a task a line.
func TestChecklistInTheBar(t *testing.T) {
	a, tt := newTestApp(t)
	a.goCollect()
	a.barOpen, a.homeDraft = true, "- [ ] 买菜\n- [ ] 明天 做饭"
	tt.Frame()
	tt.Frame()
	if !tt.HasText("记为 2 件任务") {
		t.Fatalf("the bar's tag: %q", tt.Texts())
	}
	a.saveHomeLine()
	if it := taskNamed(t, a, "买菜"); it.node.Task.Due != nil {
		t.Fatalf("买菜 has a day: %v", it.node.Task.Due)
	}
	if it := taskNamed(t, a, "做饭"); it.node.Task.Due == nil {
		t.Fatal("做饭 has no day")
	}
	if !strings.Contains(a.notice, "2 件任务") {
		t.Fatalf("said %q", a.notice)
	}
}

// 90. A task that repeats, ticked: the day said is the day it went to,
// and 撤销 gives back all the tick changed, of its last time too.
func TestRepeatingTaskTickedAndTakenBack(t *testing.T) {
	a, _ := newTestApp(t)
	day := today(testNow)
	due, start := day.AddDate(0, 0, 1), day.AddDate(0, 0, -2) // due Sunday the 11th, ticked Saturday the 10th
	a.mutate("", func() {
		n := find(t, a, "搜索") // of the open map: the tick edits this very node
		n.Task.Status, n.Task.Start, n.Task.Due = model.StatusDoing, &start, &due
		n.Task.Repeat = &model.Repeat{Unit: model.RepeatDay}
	})
	a.setTaskDone(taskNamed(t, a, "搜索"), true)
	task := find(t, a, "搜索").Task
	if !task.Due.Equal(day.AddDate(0, 0, 2)) || task.Times != 1 || task.DoneAt == nil {
		t.Fatalf("ticked: due %v, %d times, done at %v", task.Due, task.Times, task.DoneAt)
	}
	if want := "做完一次，下一次：" + weekdayName(day.AddDate(0, 0, 2)); a.notice != want { // Monday, not Tuesday
		t.Fatalf("said %q, want %q", a.notice, want)
	}
	if a.noticeUndo == nil {
		t.Fatal("no way back")
	}
	a.noticeUndo()
	task = find(t, a, "搜索").Task
	if !task.Due.Equal(due) || task.Start == nil || !task.Start.Equal(start) || task.Status != model.StatusDoing || task.Times != 0 || task.DoneAt != nil {
		t.Fatalf("taken back: due %v start %v %v, %d times, done at %v", task.Due, task.Start, task.Status, task.Times, task.DoneAt)
	}
	// Its last time: done, and taken back it was done twice of three.
	a.mutate("", func() {
		n := find(t, a, "搜索")
		n.Task.Repeat, n.Task.Times = &model.Repeat{Unit: model.RepeatDay, Count: 3}, 2
	})
	a.setTaskDone(taskNamed(t, a, "搜索"), true)
	if task = find(t, a, "搜索").Task; task.Status != model.StatusDone || task.Times != 3 || a.noticeUndo == nil {
		t.Fatalf("the last time: %v, %d times, undo %v", task.Status, task.Times, a.noticeUndo != nil)
	}
	a.noticeUndo()
	if task = find(t, a, "搜索").Task; task.Status != model.StatusDoing || task.Times != 2 || task.DoneAt != nil || !task.Due.Equal(due) {
		t.Fatalf("the last time taken back: %v, %d times, done at %v, due %v", task.Status, task.Times, task.DoneAt, task.Due)
	}
	a.setTaskDone(taskNamed(t, a, "搜索"), true)
	if task = find(t, a, "搜索").Task; task.Times != 3 {
		t.Fatalf("done again: %d times of 3", task.Times)
	}
}

// 93. The line to add a task dates by the day it says: the week's, today
// when it is in the week shown, else the week's first day; and the day
// shown follows midnight where it was today.
func TestAddLineDay(t *testing.T) {
	a, tt := newTestApp(t)
	a.openTasks()
	a.tasks.view = tasksWeek
	tt.Frame()
	day := today(testNow) // Saturday
	// Left open over a night: the next day is the day.
	next := day.AddDate(0, 0, 1)
	a.now = func() time.Time { return next.Add(9 * time.Hour) }
	tt.Frame()
	if !a.tasks.day.Equal(next) || !a.addDay(a.now()).Equal(next) {
		t.Fatalf("the morning after: the day shown is %v, a line goes on %v", a.tasks.day, a.addDay(a.now()))
	}
	tt.Click("添加任务")
	tt.Type("写周报")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	tt.Frame()
	if it := taskNamed(t, a, "写周报"); it.late(next) || !sameDay(*it.date(), next) {
		t.Fatalf("written the morning after, it is on %v", it.date())
	}
	// A day of the week looked at earlier is not where a line goes.
	a.tasks.day = a.weekOf(next)
	if got := a.addDay(a.now()); !got.Equal(next) {
		t.Fatalf("in this week: %v, want today", got)
	}
	// Another week: its first day, which the line says.
	a.tasks.day = next.AddDate(0, 0, 10)
	if got, want := a.addDay(a.now()), a.weekOf(next.AddDate(0, 0, 10)); !got.Equal(want) {
		t.Fatalf("in another week: %v, want %v", got, want)
	}
	// A day chosen is kept over midnight: only today moves.
	chosen := a.tasks.day
	a.now = func() time.Time { return next.AddDate(0, 0, 1).Add(9 * time.Hour) }
	tt.Frame()
	if !a.tasks.day.Equal(chosen) {
		t.Fatalf("a day chosen moved to %v", a.tasks.day)
	}
}

// 94. Made of the whole day, a task reminds no more at the hour it had.
func TestWholeDayDropsItsReminder(t *testing.T) {
	a, tt := newTestApp(t)
	day := today(testNow)
	a.addTask("今天 9点 站会", nil)
	a.addTask("今天 10点 评审", nil)
	a.editTask(taskNamed(t, a, "评审"), "", func(n *model.Node) { n.Task.Remind = "08:00" }) // chosen apart from its time
	a.openTasks()
	a.tasks.view = tasksWeek
	tt.Frame()
	for _, name := range []string{"站会", "评审"} {
		it := taskNamed(t, a, name)
		a.moveTaskWhen(taskDrag{it.path, it.node.ID}, day, -1)
		tt.Frame()
	}
	if task := taskNamed(t, a, "站会").node.Task; task.Time != "" || task.Remind != "" {
		t.Fatalf("of the whole day, it is at %q and reminds at %q", task.Time, task.Remind)
	}
	if task := taskNamed(t, a, "评审").node.Task; task.Remind != "08:00" {
		t.Fatalf("a time to remind at chosen apart went: %q", task.Remind)
	}
	// The details' 全天 the same, and the hour given back with the move.
	task := &model.Task{Time: "09:00", EndTime: "10:00", Remind: "09:00"}
	setTaskTime(task, "", "")
	if task.Remind != "" || task.EndTime != "" {
		t.Fatalf("全天: %+v", task)
	}
	setTaskTime(task, "14:00", "")
	if task.Remind != "14:00" {
		t.Fatalf("a time newly given: %+v", task)
	}
}

// 95. What a figure counts is what its list holds.
func TestCountsAreTheLists(t *testing.T) {
	a, tt := newTestApp(t)
	day := today(testNow)
	// The welcome map: 搜索 and 列表加载慢 and 发布 0.2 to do, 登录 done, and
	// 支持导出图片 an idea with no day.
	a.openTasks()
	a.tasks.onMap = a.path
	tt.Frame()
	tt.Frame()
	if !tt.HasText("3 要做") || tt.HasText("任务 支持导出图片") {
		t.Fatalf("a project's head and list, the left column saying 3: %q", tt.Texts())
	}
	a.tasks.onMap = ""
	a.showProjects()
	tt.Frame()
	tt.Frame()
	if !tt.HasText("任务 1 / 4") {
		t.Fatalf("the project's card: %q", tt.Texts())
	}
	// An idea with a day is a task to do, wherever it is counted.
	a.mutate("", func() { find(t, a, "支持导出图片").Task.Due = &day })
	a.tasks.read = time.Time{}
	if head, side := len(a.dueToday(testNow)), a.taskListCounts(a.taskItems())[tasksToday]; fmt.Sprint(head) != side || head != 1 {
		t.Fatalf("今天: the head says %d, the left column %q", head, side)
	}
	// Done today with no day: counted, and listed under 已完成.
	a.addTask("没日期的事", nil)
	a.editTask(taskNamed(t, a, "没日期的事"), "", func(n *model.Node) { n.Task.Due = nil })
	a.setTaskDone(taskNamed(t, a, "没日期的事"), true)
	// One that repeats, ticked today: done today, and to do again.
	a.addTask("每天 吃药", nil)
	a.setTaskDone(taskNamed(t, a, "吃药"), true)
	a.openTasks()
	a.tasks.view, a.tasks.showDone = tasksToday, true
	tt.Frame()
	tt.Frame()
	if n := len(a.doneToday(testNow)); n != 2 || !tt.HasText("2 做完") || !tt.HasText("已完成 2") {
		t.Fatalf("done today %d: %q", n, tt.Texts())
	}
	if _, ok := tt.Find("做完一次 吃药"); !ok || !tt.HasText("任务 吃药") {
		t.Fatalf("done a time, and to do again tomorrow: %q", tt.Texts())
	}
	snapshot(t, tt, "tasks-done-once")
	a.tasks.view = tasksDone
	tt.Frame()
	if _, ok := tt.Find("做完一次 吃药"); !ok {
		t.Fatalf("已完成 has not what repeats: %q", tt.Texts())
	}
	a.tasks.view = tasksReview
	tt.Frame()
	if _, ok := tt.Find("做完一次 吃药"); !ok {
		t.Fatalf("the week's review has not what repeats: %q", tt.Texts())
	}
	// Late by the day it begins on: the lists' rule is the review's.
	late := day.AddDate(0, 0, -2)
	a.addTask("早该开始的", nil)
	a.editTask(taskNamed(t, a, "早该开始的"), "", func(n *model.Node) { n.Task.Due, n.Task.Start = nil, &late })
	a.tasks.read = time.Time{}
	tt.Frame()
	tt.Frame()
	if a.taskListCounts(a.taskItems())[tasksLate] != "1" || !tt.HasText("任务 早该开始的") {
		t.Fatalf("late by its start: the left column %q, the review %q", a.taskListCounts(a.taskItems())[tasksLate], tt.Texts())
	}
	if report := weekReport("这一周", day, day, nil, []taskItem{taskNamed(t, a, "早该开始的")}, nil); !strings.Contains(report, "早该开始的") {
		t.Fatalf("the report: %q", report)
	}
}

// 96, 83. A task that repeats and is late comes again after today; and a
// day kept where the clocks say otherwise is that day here.
func TestLateRepeatAndDaysOfTheCalendar(t *testing.T) {
	a, _ := newTestApp(t)
	day := today(testNow)
	a.addTask("每天 浇花", nil)
	weekAgo := day.AddDate(0, 0, -7)
	a.editTask(taskNamed(t, a, "浇花"), "", func(n *model.Node) { n.Task.Due = &weekAgo })
	it := taskNamed(t, a, "浇花")
	for n, want := range map[int]bool{-3: false, 0: false, 1: true, 2: true} {
		d := day.AddDate(0, 0, n)
		ghost := func(w weekTask) bool { return w.ghost && w.it.key() == it.key() }
		if got := slices.ContainsFunc(a.hoursLaid(a.taskItems(), []time.Time{d}).allDay[0], ghost); got != want {
			t.Errorf("a week late, %d days from today: comes again %v", n, got)
		}
	}
	// Written eight hours to the east, today: its midnight is yesterday
	// afternoon here, and it is today's all the same.
	_, off := testNow.Zone()
	east := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.FixedZone("east", off+8*3600))
	a.addTask("今天 东边写的", nil)
	a.editTask(taskNamed(t, a, "东边写的"), "", func(n *model.Node) { n.Task.Due = &east })
	it = taskNamed(t, a, "东边写的")
	if d := it.date(); !d.Equal(day) || it.late(day) || it.node.Task.Overdue(testNow) {
		t.Fatalf("its day here is %v, late %v", d, it.late(day))
	}
	if n := a.taskListCounts(a.taskItems())[tasksLate]; n != "1" { // 浇花 alone
		t.Fatalf("late: %q", n)
	}
	if !slices.ContainsFunc(a.dueToday(testNow), func(x taskItem) bool { return x.node.Title == "东边写的" }) {
		t.Fatal("not among today's")
	}
}

// 97. ⌘⇧L is the canvas's 关联线 alone.
func TestRelationKeyIsItsOwn(t *testing.T) {
	a, _ := newTestApp(t)
	var with []string
	for _, cmd := range a.commands() {
		if cmd.keys == "⌘⇧L" {
			with = append(with, cmd.title)
		}
	}
	if len(with) != 1 || !strings.Contains(with[0], "关联线") {
		t.Fatalf("⌘⇧L is said of %q", with)
	}
	for _, g := range shortcuts {
		for _, k := range g.keys {
			if k[0] == "⌘⇧L" && !strings.Contains(k[1], "关联线") {
				t.Fatalf("the cheat sheet says ⌘⇧L is %q", k[1])
			}
		}
	}
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), `"CmdOrCtrl+Shift+L"`) {
		t.Fatal("a menu item takes ⌘⇧L before the canvas")
	}
}

// 98. A press or a block's end held as the days turn is let go.
func TestHoursStateLetGo(t *testing.T) {
	a, tt := newTestApp(t)
	a.openTasks()
	a.tasks.view = tasksWeek
	tt.Frame()
	day := today(testNow)
	a.tasks.pick = &hourPick{day: day, from: 600, to: 660}
	a.tasks.stretch = &hourStretch{key: "gone#gone", to: 700}
	a.tasks.day = a.tasks.day.AddDate(0, 0, 7) // → with the press held
	tt.Frame()
	tt.Frame()
	if a.tasks.pick != nil || a.tasks.stretch != nil || a.tasks.draft != nil {
		t.Fatalf("after the week turned: pick %+v, stretch %+v, draft %+v", a.tasks.pick, a.tasks.stretch, a.tasks.draft)
	}
	// And as the hours are left.
	a.tasks.day = day
	tt.Frame()
	a.tasks.pick = &hourPick{day: day, from: 600, to: 660}
	a.tasks.view = tasksToday
	tt.Frame()
	a.tasks.view = tasksWeek
	tt.Frame()
	tt.Frame()
	if a.tasks.pick != nil || a.tasks.draft != nil {
		t.Fatalf("back on the hours: pick %+v, draft %+v", a.tasks.pick, a.tasks.draft)
	}
}

// 100. A task's day changed off the hours says so, with the way back.
func TestDayChangesGoBack(t *testing.T) {
	a, tt := newTestApp(t)
	day := today(testNow)
	a.addTask("今天 14:00-15:00 评审方案", nil)
	start := day.AddDate(0, 0, 3)
	a.editTask(taskNamed(t, a, "评审方案"), "", func(n *model.Node) { n.Task.Start = &start; n.Task.Due = nil })
	a.openTasks()
	a.tasks.view = tasksAll
	tt.Frame()
	tt.Frame()
	// Its menu: tomorrow, which takes off a start after it.
	if err := tt.RightClick("任务 评审方案"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("明天"); err != nil {
		t.Fatalf("%v: %q", err, tt.Menu())
	}
	tt.Frame()
	tt.Frame()
	task := taskNamed(t, a, "评审方案").node.Task
	if task.Due == nil || !sameDay(*task.Due, day.AddDate(0, 0, 1)) || task.Start != nil {
		t.Fatalf("to tomorrow: due %v, start %v", task.Due, task.Start)
	}
	if a.noticeUndo == nil || !strings.Contains(a.notice, "已改到明天") {
		t.Fatalf("said %q, undo %v", a.notice, a.noticeUndo != nil)
	}
	a.noticeUndo()
	if task = taskNamed(t, a, "评审方案").node.Task; task.Due != nil || task.Start == nil || !task.Start.Equal(start) || task.Time != "14:00" {
		t.Fatalf("taken back: %+v", task)
	}
	// Dropped on a day (a group of a list, a cell of the month).
	a.noticeUndo = nil
	a.moveTaskDay(taskNamed(t, a, "评审方案"), &day)
	if a.noticeUndo == nil || !strings.Contains(a.notice, "已改到今天") {
		t.Fatalf("dropped on today: %q", a.notice)
	}
	a.noticeUndo, a.notice = nil, ""
	a.moveTaskDay(taskNamed(t, a, "评审方案"), &day) // its day already: nothing done, nothing said
	if a.notice != "" {
		t.Fatalf("said of nothing: %q", a.notice)
	}
	// Its day cleared.
	tt.Frame()
	if err := tt.RightClick("任务 评审方案"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("清除日期"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if task = taskNamed(t, a, "评审方案").node.Task; task.Due != nil || a.noticeUndo == nil || !strings.Contains(a.notice, "已清除日期") {
		t.Fatalf("cleared: due %v, said %q", task.Due, a.notice)
	}
	a.noticeUndo()
	if task = taskNamed(t, a, "评审方案").node.Task; task.Due == nil || !sameDay(*task.Due, day) {
		t.Fatalf("cleared, taken back: %v", task.Due)
	}
	// The search's 明天.
	a.openPalette(paletteAll)
	a.paletteQuery = "评审方案"
	a.noticeUndo = nil
	tt.Frame()
	tt.Frame()
	if err := tt.Click("改到明天 评审方案"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if a.noticeUndo == nil || !strings.Contains(a.notice, "已改到明天") {
		t.Fatalf("the search's 明天: %q", a.notice)
	}
	a.noticeUndo()
	a.paletteOpen = false
	// A block's end drawn down on the hours.
	a.tasks.view, a.tasks.day = tasksWeek, day
	tt.Frame()
	tt.Frame()
	it := taskNamed(t, a, "评审方案")
	a.noticeUndo = nil
	a.tasks.stretch = &hourStretch{key: it.key(), to: 16 * 60} // drawn to four, and let go
	tt.Frame()
	tt.Frame()
	if task = taskNamed(t, a, "评审方案").node.Task; task.EndTime != "16:00" || a.noticeUndo == nil || !strings.Contains(a.notice, "14:00–16:00") {
		t.Fatalf("stretched: ends %q, said %q", task.EndTime, a.notice)
	}
	a.noticeUndo()
	if task = taskNamed(t, a, "评审方案").node.Task; task.EndTime != "15:00" {
		t.Fatalf("stretched, taken back: %q", task.EndTime)
	}
}

// 101, 105, 106. One word a thing; nothing said of what is gone.
func TestTodaysWords(t *testing.T) {
	a, tt := newTestApp(t)
	day := today(testNow)
	a.addTask("没日子的", nil)
	a.addTask("明天 交作业", nil)
	a.addTask("第三件", nil)
	a.addTask("第七件", nil)
	a.addTask("第八件", nil)
	for name, n := range map[string]int{"第三件": 2, "第七件": 7, "第八件": 8} {
		d := day.AddDate(0, 0, n)
		a.editTask(taskNamed(t, a, name), "", func(n *model.Node) { n.Task.Due = &d })
	}
	a.openTasks()
	a.tasks.view = tasksAll
	tt.Frame()
	tt.Frame()
	for _, gone := range []string{"无日期", "以后", "未来 7 天"} {
		if tt.HasText(gone) {
			t.Fatalf("全部 says %q: %q", gone, tt.Texts())
		}
	}
	for _, want := range []string{"没定日期", "之后", "7 天内"} {
		if !tt.HasText(want) {
			t.Fatalf("全部 does not say %q: %q", want, tt.Texts())
		}
	}
	a.addTask("又一件没日子的", nil)
	if !strings.Contains(a.notice, "没定日期") {
		t.Fatalf("said %q", a.notice)
	}
	// 接下来 7 天 holds seven: tomorrow to the seventh day on.
	a.tasks.view = tasksToday
	tt.Frame()
	tt.Frame()
	if !tt.HasText("接下来 7 天") || !tt.HasText("任务 第七件") || tt.HasText("任务 第八件") {
		t.Fatalf("接下来 7 天: %q", tt.Texts())
	}
	// The tour names the keys as they are set, and what there is.
	a.cfg.ClipKey = "Control+Alt+B"
	var said []string
	for _, card := range tourCards {
		for _, p := range card.points {
			said = append(said, a.tourKeys(p))
		}
	}
	all := strings.Join(said, "\n") + welcomeFile(testNow).Sheets[0].Root.Children[0].Children[1].Note
	for _, gone := range []string{"工具栏", "作为任务", "⌃⌥V", "{clip}", "{capture}"} {
		if strings.Contains(all, gone) {
			t.Fatalf("the tour or the welcome map says %q", gone)
		}
	}
	if !strings.Contains(all, keyLabel("Control+Alt+B")) || !strings.Contains(all, keyLabel(a.captureKey())) || !strings.Contains(all, "＋ 任务") {
		t.Fatalf("the tour: %q", all)
	}
}
