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
	RepeatYear  = "year"
)

// Repeat says how a task comes again: every Every days, weeks, months or
// years, and, by the week, on which Days of it (1 Monday to 7 Sunday; none
// is the weekday of the task's own day). By the month it is on the
// MonthDays (-1 the last; none, the day of the task's own), or, with Nth,
// on the Nth of the weekday Days[0] (-1 the last of it), as TickTick's
// 每月第二个周一. By the year, the task's own day of its month. It ends
// after a day (Until) or after Count times in all, and comes again
// counted from the day it was done (FromDone) rather than from its own.
// Done once, a task that repeats is to do again on its next day
// (Task.SetStatus).
type Repeat struct {
	Every     int        `json:"every,omitempty"` // 1 when zero
	Unit      string     `json:"unit"`
	Days      []int      `json:"days,omitempty"`
	MonthDays []int      `json:"monthDays,omitempty"`
	Nth       int        `json:"nth,omitempty"`
	Until     *time.Time `json:"until,omitempty"`
	Count     int        `json:"count,omitempty"`
	FromDone  bool       `json:"fromDone,omitempty"`
}

// Clone is a copy of the repeat that shares nothing with it.
func (r *Repeat) Clone() *Repeat {
	if r == nil {
		return nil
	}
	c := *r
	c.Days, c.MonthDays = slices.Clone(r.Days), slices.Clone(r.MonthDays)
	if r.Until != nil {
		u := *r.Until
		c.Until = &u
	}
	return &c
}

// Ended reports whether the task, done times times in all, comes no more
// on the day next: past its last day, or done as often as it was to be.
func (r *Repeat) Ended(next time.Time, times int) bool {
	return r.Until != nil && dayOf(next).After(dayOf(*r.Until)) || r.Count > 0 && times >= r.Count
}

// Workdays reports whether it is every working day, Monday to Friday.
func (r *Repeat) Workdays() bool {
	if r == nil || r.Unit != RepeatWeek || r.every() != 1 {
		return false
	}
	days := slices.Clone(r.Days)
	slices.Sort(days)
	return slices.Equal(days, []int{1, 2, 3, 4, 5})
}

// monthDays are the days of the month y, m that it falls on, in order.
func (r *Repeat) monthDays(y int, m time.Month, from time.Time) []time.Time {
	first := time.Date(y, m, 1, 0, 0, 0, 0, from.Location())
	last := first.AddDate(0, 1, -1).Day()
	var out []time.Time
	if r.Nth != 0 {
		wd := isoWeekday(from)
		if len(r.Days) > 0 {
			wd = r.Days[0]
		}
		var all []time.Time
		for d := first; d.Month() == m; d = d.AddDate(0, 0, 1) {
			if isoWeekday(d) == wd {
				all = append(all, d)
			}
		}
		switch {
		case r.Nth < 0 && len(all) > 0:
			out = append(out, all[len(all)-1])
		case r.Nth > 0 && r.Nth <= len(all):
			out = append(out, all[r.Nth-1])
		}
		return out
	}
	days := r.MonthDays
	if len(days) == 0 {
		days = []int{from.Day()}
	}
	seen := map[int]bool{}
	for _, d := range days {
		switch {
		case d < 0 || d > last: // the last day, or the last there is
			d = last
		case d == 0:
			continue
		}
		if !seen[d] {
			seen[d] = true
			out = append(out, first.AddDate(0, 0, d-1))
		}
	}
	slices.SortFunc(out, func(a, b time.Time) int { return a.Compare(b) })
	return out
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
	if due != nil && !r.FromDone {
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
		for i := 0; i < 1200; i++ {
			// The same day of the month, or its last day where it has none.
			first := time.Date(from.Year(), from.Month()+time.Month(i*n), 1, 0, 0, 0, 0, from.Location())
			for _, d := range r.monthDays(first.Year(), first.Month(), from) {
				if d.After(after) {
					return d
				}
			}
		}
		return after.AddDate(0, 0, 1)
	case RepeatYear:
		for i := 0; ; i++ {
			// The same day of its month, or the month's last (29 February).
			first := time.Date(from.Year()+i*n, from.Month(), 1, 0, 0, 0, 0, from.Location())
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

// Label says the repeat in a few words: 每天, 工作日, 每周五, 每 2 周的周一、三,
// 每月第二个周一, 每年；and how it ends: 每天，到 12月31日；每周，共 10 次.
func (r *Repeat) Label() string {
	if r == nil {
		return "不重复"
	}
	s := r.label()
	if r.FromDone {
		s += "（从做完那天算）"
	}
	switch {
	case r.Until != nil:
		s += fmt.Sprintf("，到 %d月%d日", r.Until.Month(), r.Until.Day())
	case r.Count > 0:
		s += fmt.Sprintf("，共 %d 次", r.Count)
	}
	return s
}

var nthNames = []string{"", "第一个", "第二个", "第三个", "第四个", "第五个"}

func (r *Repeat) label() string {
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
		if r.Workdays() {
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
		switch {
		case r.Nth != 0 && len(r.Days) > 0 && r.Days[0] >= 1 && r.Days[0] <= 7:
			nth := "最后一个"
			if r.Nth > 0 && r.Nth < len(nthNames) {
				nth = nthNames[r.Nth]
			}
			return every("月") + nth + "周" + weekdayNames[r.Days[0]]
		case len(r.MonthDays) > 0:
			days := slices.Clone(r.MonthDays)
			slices.SortFunc(days, func(a, b int) int { // the last day last
				if a < 0 {
					a = 99
				}
				if b < 0 {
					b = 99
				}
				return a - b
			})
			var names []string
			for _, d := range days {
				if d < 0 {
					names = append(names, "最后一天")
				} else {
					names = append(names, fmt.Sprintf("%d 日", d))
				}
			}
			return every("月") + " " + strings.Join(names, "、")
		}
		return every("月")
	case RepeatYear:
		return every("年")
	}
	return every("天")
}

// Coming are the days a task that repeats comes again after its day due,
// from the day from to the day to, both counted: those it will be to do
// on, once done each time on its day, it having been done times times.
func (r *Repeat) Coming(due time.Time, times int, from, to time.Time) []time.Time {
	var days []time.Time
	d := dayOf(due)
	from, to = dayOf(from), dayOf(to)
	for i := range 400 { // a day's repeat, a year on
		d = r.Next(&d, d)
		if d.After(to) || r.Ended(d, times+i+1) {
			break
		}
		if !d.Before(from) {
			days = append(days, d)
		}
	}
	return days
}
