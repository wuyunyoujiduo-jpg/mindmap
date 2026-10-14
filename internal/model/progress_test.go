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
	}{{"task:0", StatusTodo}, {"task:3", StatusDoing}, {"task:6", StatusDone}, {"symbol:idea", StatusIdea}, {"task:1", StatusDoing}} {
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
		{StatusIdea, []string{"priority:1", "flag:red", "symbol:idea"}},
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
	// One status marker: the bulb put on takes the progress off.
	n.Markers = append(n.Markers, "symbol:idea")
	n.KeepStatusMarker("symbol:idea")
	if !slices.Equal(n.Markers, []string{"priority:1", "flag:red", "symbol:idea"}) {
		t.Fatalf("markers %v", n.Markers)
	}
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
}
