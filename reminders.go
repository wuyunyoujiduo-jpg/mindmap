package main

import (
	"strings"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// Reminders: once a day, a notification of the tasks due today or
// overdue, not done, of every map; and for a task given a time to be
// reminded at (model.Task.Remind), a notification of its own at that time
// of its day, or as soon after as the app runs.

// remindTimes are the times offered for a reminder: three as keys, the
// others under one.
var remindTimes = []string{"09:00", "14:00", "18:00", "08:00", "10:00", "12:00", "16:00", "20:00", "21:00"}

// reminderCheck shows the reminders not shown today, at most once a minute.
func (a *app) reminderCheck() {
	if a.cfg.NoReminders || a.now().Sub(a.remindedAt) < time.Minute {
		return
	}
	now := a.now()
	a.remindedAt = now
	day := today(now)
	if a.reminded == nil || !a.remindedDay.Equal(day) {
		a.reminded, a.remindedDay = map[string]bool{}, day
	}
	clock := now.Format("15:04")
	var due, overdue []string
	for _, it := range a.taskItems() {
		task := it.node.Task
		if task == nil || task.Status == model.StatusDone || task.Due == nil {
			continue
		}
		key := it.key()
		// Its own time: on its day, once that time has come.
		if r := task.Remind; r != "" && sameDay(*task.Due, day) && clock >= r && !a.reminded[key+"@"+r] {
			a.reminded[key+"@"+r] = true
			a.plat.Notify("到时间了 · "+strings.TrimPrefix(r, "0"), it.node.Title+"（"+it.mapName+"）")
		}
		if a.reminded[key] {
			continue
		}
		switch {
		case task.Due.Before(day):
			overdue = append(overdue, it.node.Title)
		case task.Due.Before(day.AddDate(0, 0, 1)):
			due = append(due, it.node.Title)
		default:
			continue
		}
		a.reminded[key] = true
	}
	if len(due)+len(overdue) == 0 {
		return
	}
	var body []string
	if len(overdue) > 0 {
		body = append(body, "已逾期："+strings.Join(overdue, "、"))
	}
	if len(due) > 0 {
		body = append(body, "今天到期："+strings.Join(due, "、"))
	}
	a.plat.Notify("MyMind · 今天的任务", strings.Join(body, "\n"))
}
