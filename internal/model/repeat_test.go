package model

import (
	"testing"
	"time"
)

func TestRepeatNext(t *testing.T) {
	day := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, time.UTC) }
	now := day(11, 11).Add(10 * time.Hour) // a Wednesday
	for _, c := range []struct {
		name string
		r    Repeat
		due  time.Time
		want time.Time
	}{
		{"每天", Repeat{Unit: RepeatDay}, day(11, 11), day(11, 12)},
		{"每天, long overdue", Repeat{Unit: RepeatDay}, day(11, 2), day(11, 12)},
		{"每 3 天, done early", Repeat{Unit: RepeatDay, Every: 3}, day(11, 13), day(11, 16)},
		{"每周五", Repeat{Unit: RepeatWeek, Days: []int{5}}, day(11, 13), day(11, 20)},
		{"每周, its own weekday", Repeat{Unit: RepeatWeek}, day(11, 11), day(11, 18)},
		{"工作日, on a Friday", Repeat{Unit: RepeatWeek, Days: []int{1, 2, 3, 4, 5}}, day(11, 13), day(11, 16)},
		{"每 2 周的周一、三", Repeat{Unit: RepeatWeek, Every: 2, Days: []int{1, 3}}, day(11, 11), day(11, 23)},
		{"每月", Repeat{Unit: RepeatMonth}, day(11, 11), day(12, 11)},
		{"每月 31 号", Repeat{Unit: RepeatMonth}, day(10, 31), day(11, 30)},
	} {
		due := c.due
		if got := c.r.Next(&due, now); !got.Equal(c.want) {
			t.Errorf("%s: next %s, want %s", c.name, got.Format("01-02"), c.want.Format("01-02"))
		}
		if c.r.Label() == "" {
			t.Errorf("%s: no label", c.name)
		}
	}
	if got := (&Repeat{Unit: RepeatWeek, Every: 2, Days: []int{1, 3, 5}}).Label(); got != "每 2 周的周一、三、五" {
		t.Errorf("label %q", got)
	}
	// Done, a task that repeats is to do again on its next day.
	due := day(11, 13)
	task := &Task{Status: StatusTodo, Due: &due, Repeat: &Repeat{Unit: RepeatWeek, Days: []int{5}}}
	task.SetStatus(StatusDone, now)
	if task.Status != StatusTodo || !task.Due.Equal(day(11, 20)) || task.Times != 1 || task.DoneAt == nil {
		t.Fatalf("after done: %+v due %v", task, task.Due)
	}
}

func TestRepeatComing(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 11, d, 0, 0, 0, 0, time.UTC) }
	got := (&Repeat{Unit: RepeatWeek, Days: []int{5}}).Coming(day(13), day(1), day(30))
	if len(got) != 2 || !got[0].Equal(day(20)) || !got[1].Equal(day(27)) {
		t.Fatalf("every Friday after the 13th, in November: %v", got)
	}
	if got := (&Repeat{Unit: RepeatDay}).Coming(day(13), day(16), day(18)); len(got) != 3 {
		t.Fatalf("every day, three days: %v", got)
	}
}
