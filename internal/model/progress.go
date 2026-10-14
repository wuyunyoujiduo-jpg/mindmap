package model

import (
	"strings"
	"time"
)

// A task's progress marker, task:0 (to start) to task:6 (done), and its
// status go together: the marker makes the node a task, of the status it
// shows, and a status given moves the marker, where the node has one.
const (
	progressGroup = "task:"
	progressStart = progressGroup + "0"
	progressFirst = progressGroup + "1"
	progressDone  = progressGroup + "6"
)

// progressMarker is the index in n.Markers of its progress marker, or -1.
func (n *Node) progressMarker() int {
	for i, m := range n.Markers {
		if strings.HasPrefix(m, progressGroup) {
			return i
		}
	}
	return -1
}

// HasProgress tells whether n has a progress marker.
func (n *Node) HasProgress() bool { return n.progressMarker() >= 0 }

// FollowProgress makes n a task of the status its progress marker shows,
// if it has one: 0 to start, 1 to 5 under way, 6 done.
func (n *Node) FollowProgress(now time.Time) {
	i := n.progressMarker()
	if i < 0 {
		return
	}
	s := StatusDoing
	switch n.Markers[i] {
	case progressStart:
		s = StatusTodo
		if n.Task != nil && n.Task.Status == StatusIdea {
			s = StatusIdea
		}
	case progressDone:
		s = StatusDone
	}
	if n.Task == nil {
		n.Task = &Task{}
	}
	if n.Task.Status != s {
		n.Task.SetStatus(s, now)
	}
}

// SetStatus makes n a task of the status, moving its progress marker, if
// it has one, to match.
func (n *Node) SetStatus(s Status, now time.Time) {
	if n.Task == nil {
		n.Task = &Task{}
	}
	n.Task.SetStatus(s, now)
	i := n.progressMarker()
	if i < 0 {
		return
	}
	switch m := n.Markers[i]; {
	case s == StatusDone:
		n.Markers[i] = progressDone
	case s == StatusDoing:
		if m == progressStart || m == progressDone {
			n.Markers[i] = progressFirst
		}
	default:
		n.Markers[i] = progressStart
	}
}

// DropTask makes n no longer a task, taking its progress marker off too.
func (n *Node) DropTask() {
	n.Task = nil
	if i := n.progressMarker(); i >= 0 {
		n.Markers = append(n.Markers[:i:i], n.Markers[i+1:]...)
		if len(n.Markers) == 0 {
			n.Markers = nil
		}
	}
}
