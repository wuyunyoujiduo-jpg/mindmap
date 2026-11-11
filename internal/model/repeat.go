package model

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// The units a task repeats by.
const (
	RepeatDay   = "day"
	RepeatWeek  = "week"
	RepeatMonth = "month"
)

// Repeat says how a task comes again: every Every days, weeks or months,
// and, by the week, on which Days of it (1 Monday to 7 Sunday; none is the
// weekday of the task's own day). Done once, a task that repeats is to do
// again on its next day (Task.SetStatus).
type Repeat struct {
	Every int    `json:"every,omitempty"` // 1 when zero
	Unit  string `json:"unit"`
	Days  []int  `json:"days,omitempty"`
}

func (r *Repeat) every() int {
	if r.Every < 1 {
		return 1
	}
	return r.Every
}

// isoWeekday is a day's place in its week: 1 Monday to 7 Sunday.
func isoWeekday(d time.Time) int {
	if wd := int(d.Weekday()); wd != 0 {
		return wd
	}
	return 7
}

func dayOf(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// Next is the day the task comes again once done at now, its day being
// due (today's when nil): the first of its days after both.
func (r *Repeat) Next(due *time.Time, now time.Time) time.Time {
	today := dayOf(now)
	from := today
	if due != nil {
		from = dayOf(*due)
	}
	after := from // the next is after its day, and after today
	if today.After(after) {
		after = today
	}
	n := r.every()
	switch r.Unit {
	case RepeatWeek:
		days := r.Days
		if len(days) == 0 {
			days = []int{isoWeekday(from)}
		}
		week := from.AddDate(0, 0, 1-isoWeekday(from)) // the Monday of its week
		for d := after.AddDate(0, 0, 1); ; d = d.AddDate(0, 0, 1) {
			weeks := int(d.AddDate(0, 0, 1-isoWeekday(d)).Sub(week).Hours()/24+0.5) / 7
			if weeks%n == 0 && slices.Contains(days, isoWeekday(d)) {
				return d
			}
		}
	case RepeatMonth:
		for i := 1; ; i++ {
			// The same day of the month, or its last day where it has none.
			first := time.Date(from.Year(), from.Month()+time.Month(i*n), 1, 0, 0, 0, 0, from.Location())
			d := first.AddDate(0, 0, from.Day()-1)
			if d.Month() != first.Month() {
				d = first.AddDate(0, 1, -1)
			}
			if d.After(after) {
				return d
			}
		}
	}
	d := from
	for !d.After(after) {
		d = d.AddDate(0, 0, n)
	}
	return d
}

var weekdayNames = []string{"", "一", "二", "三", "四", "五", "六", "日"}

// Label says the repeat in a few words: 每天, 工作日, 每周五, 每 2 周的周一、三.
func (r *Repeat) Label() string {
	if r == nil {
		return "不重复"
	}
	n := r.every()
	every := func(unit string) string {
		if n == 1 {
			return "每" + unit
		}
		return fmt.Sprintf("每 %d %s", n, unit)
	}
	switch r.Unit {
	case RepeatWeek:
		days := slices.Clone(r.Days)
		slices.Sort(days)
		if n == 1 && slices.Equal(days, []int{1, 2, 3, 4, 5}) {
			return "工作日"
		}
		if len(days) == 0 {
			return every("周")
		}
		var names []string
		for _, d := range days {
			if d >= 1 && d <= 7 {
				names = append(names, weekdayNames[d])
			}
		}
		if n == 1 {
			return "每周" + strings.Join(names, "、")
		}
		return every("周") + "的周" + strings.Join(names, "、")
	case RepeatMonth:
		return every("月")
	}
	return every("天")
}
