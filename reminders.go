package main

import (
	"strings"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// Reminders: once a day, a notification for each task of the open map
// due today or overdue, not done.

// reminderCheck shows the reminders not shown today, at most once a minute.
func (a *app) reminderCheck() {
	if a.file == nil || a.cfg.NoReminders || a.now().Sub(a.remindedAt) < time.Minute {
		return
	}
	a.remindedAt = a.now()
	day := today(a.now())
	if a.reminded == nil || !a.remindedDay.Equal(day) {
		a.reminded, a.remindedDay = map[string]bool{}, day
	}
	var due, overdue []string
	for _, tn := range a.file.Tasks() {
		task := tn.Node.Task
		if task.Status == model.StatusDone || task.Due == nil || a.reminded[tn.Node.ID] {
			continue
		}
		switch {
		case task.Due.Before(day):
			overdue = append(overdue, tn.Node.Title)
		case task.Due.Before(day.AddDate(0, 0, 1)):
			due = append(due, tn.Node.Title)
		default:
			continue
		}
		a.reminded[tn.Node.ID] = true
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
	a.plat.Notify("MyMind · "+mapName(a.path), strings.Join(body, "\n"))
}
