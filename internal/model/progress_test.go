package model

import (
	"slices"
	"testing"
	"time"
)

func TestProgressMarker(t *testing.T) {
	now := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	n := NewNode("写文档")
	for _, c := range []struct {
		marker string
		want   Status
	}{{"task:0", StatusTodo}, {"task:3", StatusDoing}, {"task:6", StatusDone}, {"task:idea", StatusIdea}, {"task:1", StatusDoing}} {
		n.Markers = []string{"priority:1", c.marker}
		n.FollowProgress(now)
		if n.Task == nil || n.Task.Status != c.want {
			t.Fatalf("%s: task %+v, want %s", c.marker, n.Task, c.want)
		}
	}
	if n.Task.DoneAt != nil {
		t.Fatal("still done")
	}
	// The status changes the marker.
	n.Markers = []string{"priority:1", "task:1", "flag:red"}
	for _, c := range []struct {
		status Status
		want   []string
	}{
		{StatusDone, []string{"priority:1", "task:6", "flag:red"}},
		{StatusIdea, []string{"priority:1", "task:idea", "flag:red"}},
		{StatusDoing, []string{"priority:1", "task:1", "flag:red"}},
		{StatusTodo, []string{"priority:1", "task:0", "flag:red"}},
	} {
		n.SetStatus(c.status, now)
		if !slices.Equal(n.Markers, c.want) {
			t.Fatalf("%s: markers %v", c.status, n.Markers)
		}
	}
	n.Markers[1] = "task:4"
	n.SetStatus(StatusDoing, now) // under way already: as it was
	if n.Markers[1] != "task:4" {
		t.Fatalf("markers %v", n.Markers)
	}
	// One status marker: the circle put on takes the progress off.
	n.Markers = append(n.Markers, "task:idea")
	n.KeepStatusMarker("task:idea")
	if !slices.Equal(n.Markers, []string{"priority:1", "flag:red", "task:idea"}) {
		t.Fatalf("markers %v", n.Markers)
	}
	n.Markers = []string{"priority:1", "flag:red"}
	// Without a marker, none is added.
	m := NewNode("x")
	m.SetStatus(StatusDone, now)
	if m.Markers != nil || m.Task.Status != StatusDone {
		t.Fatalf("%v %+v", m.Markers, m.Task)
	}
	n.DropTask()
	if n.Task != nil || !slices.Equal(n.Markers, []string{"priority:1", "flag:red"}) {
		t.Fatalf("dropped: %+v %v", n.Task, n.Markers)
	}

	// An idea marked with the light bulb, as before, takes the circle.
	old := NewNode("旧想法")
	old.Task = &Task{Status: StatusIdea}
	old.Markers = []string{"priority:2", "flag:red", "symbol:idea"}
	old.UpdateIdeaMarker()
	if !slices.Equal(old.Markers, []string{"priority:2", "task:idea", "flag:red"}) {
		t.Fatalf("updated %v", old.Markers)
	}
	bulb := NewNode("灯泡")
	bulb.Markers = []string{"symbol:idea"}
	bulb.UpdateIdeaMarker() // not an idea: a symbol
	if !slices.Equal(bulb.Markers, []string{"symbol:idea"}) {
		t.Fatalf("symbol %v", bulb.Markers)
	}
}
