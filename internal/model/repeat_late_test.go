package model

import (
	"testing"
	"time"
)

// A task that is late comes again after today: not on the days it missed,
// nor on today, where it is to do itself; and one late by hundreds of
// times is counted, not walked.
func TestRepeatComingOfALateTask(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 11, d, 0, 0, 0, 0, time.UTC) }
	daily := &Repeat{Unit: RepeatDay}
	// Due the 10th, today the 17th: the 18th on.
	got := daily.Coming(day(10), 0, day(17).Add(9*time.Hour), day(9), day(20))
	if len(got) != 3 || !got[0].Equal(day(18)) || !got[2].Equal(day(20)) {
		t.Fatalf("a week late: %v", got)
	}
	if got := daily.Coming(day(10), 0, day(17), day(17), day(17)); len(got) != 0 {
		t.Fatalf("on today, where the task itself is to do: %v", got)
	}
	// Late by three years of days: it showed none, the 400 turns spent on
	// the days gone by.
	long := time.Date(2023, 11, 1, 0, 0, 0, 0, time.UTC)
	if got := daily.Coming(long, 0, day(17), day(16), day(22)); len(got) != 5 || !got[0].Equal(day(18)) {
		t.Fatalf("years late: %v", got)
	}
	for _, c := range []struct {
		name string
		r    Repeat
		want time.Time
	}{
		{"每 3 天", Repeat{Unit: RepeatDay, Every: 3}, day(18)},
		{"每月", Repeat{Unit: RepeatMonth}, time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)},
		{"每 5 个月", Repeat{Unit: RepeatMonth, Every: 5}, time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC)},
		{"每年", Repeat{Unit: RepeatYear}, time.Date(2027, 11, 1, 0, 0, 0, 0, time.UTC)},
		{"每 2 年", Repeat{Unit: RepeatYear, Every: 2}, time.Date(2027, 11, 1, 0, 0, 0, 0, time.UTC)},
	} {
		// As the days walked one turn at a time give it.
		want := long
		for !want.After(day(17)) {
			switch c.r.Unit {
			case RepeatDay:
				want = want.AddDate(0, 0, c.r.every())
			case RepeatMonth:
				want = want.AddDate(0, c.r.every(), 0)
			default:
				want = want.AddDate(c.r.every(), 0, 0)
			}
		}
		if got := c.r.Next(&long, day(17)); !got.Equal(want) || !got.Equal(c.want) {
			t.Errorf("%s: next %s, want %s (walked: %s)", c.name, got.Format("2006-01-02"), c.want.Format("2006-01-02"), want.Format("2006-01-02"))
		}
	}
}

// A task's day is a day of the calendar: kept as a midnight to the east,
// it is that day to the west too, not the evening before.
func TestDueIsADayOfTheCalendar(t *testing.T) {
	east, west := time.FixedZone("east", 8*3600), time.FixedZone("west", -5*3600)
	due := time.Date(2026, 11, 17, 0, 0, 0, 0, east) // written in the east
	now := time.Date(2026, 11, 17, 9, 0, 0, 0, west) // read in the west, the same day
	task := &Task{Status: StatusTodo, Due: &due}
	if task.Overdue(now) {
		t.Fatal("due today, it reads as late to the west")
	}
	if !task.Overdue(now.AddDate(0, 0, 1)) {
		t.Fatal("the day after, it is late")
	}
	if got := DayIn(due, west); got.Day() != 17 || got.Location() != west {
		t.Fatalf("its day in the west: %v", got)
	}
	// What repeats comes again on the calendar's next day there.
	if got := (&Repeat{Unit: RepeatDay}).Next(&due, now); !got.Equal(time.Date(2026, 11, 18, 0, 0, 0, 0, west)) {
		t.Fatalf("next: %v", got)
	}
}

// A task done for its last time and to do after all was done once less;
// and when one that repeats was last done stays said as its state changes.
func TestRepeatLastTimeTakenBack(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 11, d, 0, 0, 0, 0, time.UTC) }
	due := day(10)
	task := &Task{Status: StatusTodo, Due: &due, Repeat: &Repeat{Unit: RepeatDay, Count: 3}, Times: 2}
	task.SetStatus(StatusDone, day(10))
	if task.Status != StatusDone || task.Times != 3 {
		t.Fatalf("the last time: %v, %d times", task.Status, task.Times)
	}
	task.SetStatus(StatusTodo, day(10))
	if task.Times != 2 || task.DoneAt != nil {
		t.Fatalf("taken back: %d times, done at %v", task.Times, task.DoneAt)
	}
	task.SetStatus(StatusDone, day(10))
	if task.Status != StatusDone || task.Times != 3 {
		t.Fatalf("done again: %v, %d times of 3", task.Status, task.Times)
	}
	// Done once, then under way: it was done today still.
	again := &Task{Status: StatusTodo, Due: &due, Repeat: &Repeat{Unit: RepeatDay}}
	again.SetStatus(StatusDone, day(10))
	again.SetStatus(StatusDoing, day(10))
	if again.DoneAt == nil || again.Times != 1 {
		t.Fatalf("under way after a time done: done at %v, %d times", again.DoneAt, again.Times)
	}
}
