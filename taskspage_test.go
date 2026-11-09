package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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

	if err := tt.Click("任务"); err != nil {
		t.Fatal(err)
	}
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
