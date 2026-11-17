package main

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// 91 of docs/审查清单.md. The day is notified of once; a task written
// today, or with a time already past, says nothing; and the app started
// again says nothing again.
func TestRemindersOnce(t *testing.T) {
	a, tt := newTestApp(t)
	fp := a.plat.(*fakePlatform)
	day := today(testNow)
	at := func(h, m int) {
		a.now = func() time.Time { return day.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute) }
		a.tasks.read = time.Time{}
		tt.Frame()
	}
	at(9, 0) // the day's first look: nothing is due, nothing is said
	a.addTask("第一件", &day)
	at(9, 5)
	a.addTask("第二件", &day)
	at(9, 10)
	if len(fp.notes) != 0 {
		t.Fatalf("tasks written today were notified of: %q", fp.notes)
	}
	// 9:20 written at half past nine is not a time that came.
	at(9, 29)
	a.addTask("9:20 第三件", &day)
	a.addTask("9:40 第四件", &day)
	at(9, 31)
	if len(fp.notes) != 0 {
		t.Fatalf("a time already past was reminded of: %q", fp.notes)
	}
	at(9, 41)
	at(9, 45)
	if len(fp.notes) != 1 || !strings.HasPrefix(fp.notes[0], "到时间了 · 9:40") || !strings.Contains(fp.notes[0], "第四件") {
		t.Fatalf("its time come: %q", fp.notes)
	}
	// The day after: one notification of the day, what is late by the day
	// it began on too (the lists' rule).
	late := day.AddDate(0, 0, -1)
	a.addTask("没做的", nil)
	a.editTask(taskNamed(t, a, "没做的"), "", func(n *model.Node) { n.Task.Start = &late })
	day = day.AddDate(0, 0, 1)
	fp.notes = nil
	at(8, 0)
	at(8, 5)
	if len(fp.notes) != 1 || !strings.Contains(fp.notes[0], "已逾期：") || !strings.Contains(fp.notes[0], "没做的") || !strings.Contains(fp.notes[0], "第一件") {
		t.Fatalf("the day's notification: %q", fp.notes)
	}
	a.addTask("9点 开会", &day)
	at(9, 1)
	if len(fp.notes) != 2 || !strings.HasPrefix(fp.notes[1], "到时间了 · 9:00") {
		t.Fatalf("at nine: %q", fp.notes)
	}
	// Started again, as by an update: what was said is not said again.
	fp2 := &fakePlatform{}
	b := newApp(fp2, config{Dir: a.dir, PanelOpen: true})
	b.noLocalModel, b.noMotion = true, true
	b.now = a.now
	tb := ui.NewTester(b.view, 1280, 800)
	tb.Frame()
	tb.Frame()
	if len(fp2.notes) != 0 {
		t.Fatalf("said again after a restart: %q", fp2.notes)
	}
}
