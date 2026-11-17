package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Reminders: once a day, a notification of the tasks due today or
// overdue, to do, of every map; and for a task given a time to be
// reminded at (model.Task.Remind), a notification of its own at that time
// of its day, or as soon after as the app runs.
//
// Each is said once. The day's notification is one of the day, told by a
// mark of the day and not of each task: a task written or put on today
// later is no cause to say the day again (today's line to add one dates
// each of its tasks today, and each was notified as it was written). The
// mark is made when the day was said, not when it was looked at: with
// nothing due at the day's first look it was made all the same, and what
// the sync brought for today later was never said. What is no cause to
// say it is a task made or changed here today (remindOwn): the user has
// just seen it. A
// time to remind at that was past already when it was first seen, 9:20
// written at half past nine, is not a time that came, and says nothing.
// And what was said is kept beside the settings (remindedPath), so that
// the app started again, as by an update, does not say it all once more.

// remindTimes are the times offered for a reminder: three as keys, the
// others under one.
var remindTimes = []string{"09:00", "14:00", "18:00", "08:00", "10:00", "12:00", "16:00", "20:00", "21:00"}

// remindedDigest marks, in what was reminded of a day, the day's own
// notification.
const remindedDigest = "day"

// remindedOwn marks, before a task's key, one made or changed here on the
// day: the day is not said for it alone.
const remindedOwn = "own:"

// remindedOf brings what was reminded of today to hand: as kept, on the
// day's first asking.
func (a *app) remindedOf(day time.Time) {
	if a.reminded == nil || !a.remindedDay.Equal(day) {
		a.reminded, a.remindedDay = remindedLoad(day), day
		a.tasks.dueSeen = nil
	}
}

// remindOwn marks a task as made or changed here today (remindedOwn), by
// its key.
func (a *app) remindOwn(key string) {
	if a.cfg.NoReminders {
		return
	}
	day := today(a.now())
	a.remindedOf(day)
	if k := remindedOwn + key; !a.reminded[k] {
		a.reminded[k] = true
		remindedSave(day, a.reminded)
	}
}

// remindedPath is the file that keeps what was reminded of today.
func remindedPath() string {
	p := configPath()
	if p == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(p), "reminded.json")
}

// remindedFile is what was reminded of on a day: the day's notification
// and each task at its time, by their marks.
type remindedFile struct {
	Day  string   `json:"day"` // 2006-01-02
	Said []string `json:"said"`
}

// remindedLoad is what was reminded of on the day, as kept: nothing of
// another day.
func remindedLoad(day time.Time) map[string]bool {
	said := map[string]bool{}
	p := remindedPath()
	if p == "" {
		return said
	}
	var f remindedFile
	if b, err := os.ReadFile(p); err == nil && json.Unmarshal(b, &f) == nil && f.Day == day.Format("2006-01-02") {
		for _, k := range f.Said {
			said[k] = true
		}
	}
	return said
}

// remindedSave keeps what was reminded of on the day.
func remindedSave(day time.Time, said map[string]bool) {
	p := remindedPath()
	if p == "" {
		return
	}
	f := remindedFile{Day: day.Format("2006-01-02")}
	for k := range said {
		f.Said = append(f.Said, k)
	}
	slices.Sort(f.Said)
	b, err := json.Marshal(f)
	if err != nil || os.MkdirAll(filepath.Dir(p), 0o755) != nil {
		return
	}
	tmp := p + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, p)
	}
}

// reminderCheck shows the reminders not shown today, at most once a minute.
func (a *app) reminderCheck() {
	if a.cfg.NoReminders || a.now().Sub(a.remindedAt) < time.Minute {
		return
	}
	now := a.now()
	// When it last looked, today: a time to remind at is one that came
	// since. Not looked yet today (the app just started, or the day
	// turned), every time of the day so far is one that came.
	last := ""
	if !a.remindedAt.IsZero() && sameDay(a.remindedAt, now) && !a.remindedAt.After(now) {
		last = a.remindedAt.Format("15:04")
	}
	// The open map changed here since the last look: a task of it that is
	// due now and was not then was dated here, on the canvas or in its
	// panel, and is the user's own as one written on 今天 is.
	edited := last != "" && !a.changed.Before(a.remindedAt)
	a.remindedAt = now
	day := today(now)
	a.remindedOf(day)
	said := len(a.reminded)
	clock := now.Format("15:04")
	var due, overdue []string
	seen, was := map[string]bool{}, a.tasks.dueSeen
	news := false // something is due that was not made or changed here today
	for _, it := range a.taskItems() {
		if !countsAsTask(it) {
			continue
		}
		d := it.date()
		if d == nil {
			continue
		}
		// Its own time: on its day, once that time has come.
		if r, at := it.node.Task.Remind, it.due(); r != "" && at != nil && sameDay(*at, day) && clock >= r && !a.reminded[it.key()+"@"+r] {
			a.reminded[it.key()+"@"+r] = true
			if last == "" || r > last {
				a.plat.Notify("到时间了 · "+strings.TrimPrefix(r, "0"), it.node.Title+"（"+it.mapName+"）")
			}
		}
		switch { // by the lists' own rules (late, date): what it says is what they show
		case it.late(day):
			overdue = append(overdue, it.node.Title)
		case sameDay(*d, day):
			due = append(due, it.node.Title)
		default:
			continue
		}
		seen[it.key()] = true
		if edited && was != nil && !was[it.key()] && it.path == a.path {
			a.reminded[remindedOwn+it.key()] = true
		}
		if !a.reminded[remindedOwn+it.key()] {
			news = true
		}
	}
	a.tasks.dueSeen = seen
	if !a.reminded[remindedDigest] && news {
		a.reminded[remindedDigest] = true
		var body []string
		if len(overdue) > 0 {
			body = append(body, "已逾期："+strings.Join(overdue, "、"))
		}
		if len(due) > 0 {
			body = append(body, "今天到期："+strings.Join(due, "、"))
		}
		a.plat.Notify("MyMind · 今天的任务", strings.Join(body, "\n"))
	}
	if len(a.reminded) != said {
		remindedSave(day, a.reminded)
	}
}
