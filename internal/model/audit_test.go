package model

import (
	"testing"
	"time"
)

// A version compared with another names every branch added and every one
// removed, not the first of each alone.
func TestDiffNamesEveryBranch(t *testing.T) {
	f := NewFile("项目")
	d := f.Sheets[0]
	a, _ := d.AddChild(d.Root.ID, "功能")
	b, _ := d.AddChild(d.Root.ID, "架构")
	g1, _ := d.AddChild(a.ID, "旧一")
	g2, _ := d.AddChild(b.ID, "旧二")
	old := f.Clone()
	d.Delete(g1.ID)
	d.Delete(g2.ID)
	n, _ := d.AddChild(a.ID, "新一")
	d.AddChild(n.ID, "新一的下级")
	d.AddChild(b.ID, "新二")
	d.AddChild(d.Root.ID, "新三")
	added, removed := 0, 0
	for _, c := range Diff(old, f) {
		switch c.Kind {
		case Added:
			added++
		case Removed:
			removed++
		}
	}
	if added != 3 || removed != 2 {
		t.Fatalf("added %d (3 branches), removed %d (2)", added, removed)
	}
}

// A task on a floating topic is a task of the canvas as any other.
func TestFloatingTasksListed(t *testing.T) {
	d := New("项目")
	n, _ := d.AddChild(d.Root.ID, "写方案")
	n.Task = &Task{Status: StatusTodo}
	d.Floating = append(d.Floating, &Node{ID: NewID(), Title: "漂着的事", Task: &Task{Status: StatusTodo}})
	if got := d.Tasks(); len(got) != 2 || got[1].Node.Title != "漂着的事" {
		t.Fatalf("tasks %+v", got)
	}
}

// A canvas copied is all its own: its floating topics have IDs of their
// own, and its relations join its own topics.
func TestDuplicateSheetIsItsOwn(t *testing.T) {
	f := NewFile("项目")
	d := f.Sheets[0]
	a, _ := d.AddChild(d.Root.ID, "甲")
	fl := &Node{ID: NewID(), Title: "漂"}
	d.Floating = append(d.Floating, fl)
	d.Relations = append(d.Relations, Relation{ID: NewID(), From: a.ID, To: fl.ID})
	c := f.DuplicateSheet(0)
	if c.Floating[0].ID == fl.ID || c.Root.Children[0].ID == a.ID {
		t.Fatal("the copy shares IDs with the canvas copied")
	}
	r := c.Relations[0]
	if r.ID == d.Relations[0].ID || r.From != c.Root.Children[0].ID || r.To != c.Floating[0].ID {
		t.Fatalf("the copy's relation %+v joins the first canvas's topics", r)
	}
	if d.Relations[0].From != a.ID || d.Floating[0].ID != fl.ID {
		t.Fatal("the canvas copied changed")
	}
}

// A weekly repeat whose days are not 1 to 7 still has a next day: Sunday
// written as 0 is Sunday, and with no day of the week it is the weekday
// of the day due.
func TestRepeatOddDays(t *testing.T) {
	day := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, time.UTC) }
	now := day(11, 11) // a Wednesday
	due := day(11, 11)
	done := make(chan [2]time.Time, 1)
	go func() {
		a := (&Repeat{Unit: RepeatWeek, Days: []int{0}}).Next(&due, now)
		b := (&Repeat{Unit: RepeatWeek, Days: []int{9}}).Next(&due, now)
		done <- [2]time.Time{a, b}
	}()
	select {
	case got := <-done:
		if !got[0].Equal(day(11, 15)) || !got[1].Equal(day(11, 18)) {
			t.Fatalf("next %v, %v", got[0], got[1])
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Next never returned")
	}
}

// A task that repeats, ticked by its marker, is to do again, and its
// marker says so: left at done, the next marker changed ticked it again.
func TestRepeatingTaskMarkerFollows(t *testing.T) {
	now := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)
	due := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	n := &Node{ID: NewID(), Title: "打卡", Task: &Task{Status: StatusTodo, Due: &due, Repeat: &Repeat{Unit: RepeatDay}}, Markers: []string{progressStart}}
	n.SetStatus(StatusDone, now)
	if n.Task.Status != StatusTodo || n.Markers[0] != progressStart || n.Task.Times != 1 {
		t.Fatalf("after a tick: status %v, marker %q, times %d", n.Task.Status, n.Markers[0], n.Task.Times)
	}
}
