package model

import (
	"strings"
	"time"
)

// A task's status marker, its progress (task:0, to start, to task:6, done)
// or the idea's light bulb, and its status go together: the marker makes
// the node a task, of the status it shows, and a status given changes the
// marker, where the node has one. A node has one status marker at most.
const (
	progressGroup = "task:"
	progressStart = progressGroup + "0"
	progressFirst = progressGroup + "1"
	progressDone  = progressGroup + "6"
	ideaMarker    = "symbol:idea"
)

func isStatusMarker(m string) bool {
	return m == ideaMarker || strings.HasPrefix(m, progressGroup)
}

// statusMarker is the index in n.Markers of its status marker, or -1.
func (n *Node) statusMarker() int {
	for i, m := range n.Markers {
		if isStatusMarker(m) {
			return i
		}
	}
	return -1
}

// HasStatusMarker tells whether n has a status marker.
func (n *Node) HasStatusMarker() bool { return n.statusMarker() >= 0 }

// KeepStatusMarker takes off n's other status markers when m, just put on
// it, is one.
func (n *Node) KeepStatusMarker(m string) {
	if !isStatusMarker(m) {
		return
	}
	var out []string
	for _, x := range n.Markers {
		if x == m || !isStatusMarker(x) {
			out = append(out, x)
		}
	}
	n.Markers = out
}

// FollowProgress makes n a task of the status its status marker shows, if
// it has one: the light bulb an idea, progress 0 to do, 1 to 5 under way,
// 6 done.
func (n *Node) FollowProgress(now time.Time) {
	i := n.statusMarker()
	if i < 0 {
		return
	}
	s := StatusDoing
	switch n.Markers[i] {
	case ideaMarker:
		s = StatusIdea
	case progressStart:
		s = StatusTodo
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

// SetStatus makes n a task of the status, changing its status marker, if
// it has one, to match.
func (n *Node) SetStatus(s Status, now time.Time) {
	if n.Task == nil {
		n.Task = &Task{}
	}
	n.Task.SetStatus(s, now)
	i := n.statusMarker()
	if i < 0 {
		return
	}
	m := n.Markers[i]
	switch s {
	case StatusIdea:
		n.putStatusMarker(i, ideaMarker)
	case StatusDone:
		n.putStatusMarker(i, progressDone)
	case StatusDoing:
		if !strings.HasPrefix(m, progressGroup) || m == progressStart || m == progressDone {
			n.putStatusMarker(i, progressFirst)
		}
	default:
		n.putStatusMarker(i, progressStart)
	}
}

// putStatusMarker puts m in place of the status marker at i: in its place
// for a marker of the same group, else where its group goes, the progress
// after the colors and the priority, a symbol last (and in place of the
// node's other symbol).
func (n *Node) putStatusMarker(i int, m string) {
	if n.Markers[i] == m {
		return
	}
	if strings.HasPrefix(m, progressGroup) == strings.HasPrefix(n.Markers[i], progressGroup) {
		n.Markers[i] = m
		return
	}
	var out []string
	placed := false
	for j, x := range n.Markers {
		if j == i || m == ideaMarker && strings.HasPrefix(x, "symbol:") {
			continue
		}
		if !placed && m != ideaMarker && !strings.HasPrefix(x, "dot:") && !strings.HasPrefix(x, "priority:") {
			out, placed = append(out, m), true
		}
		out = append(out, x)
	}
	if !placed {
		out = append(out, m)
	}
	n.Markers = out
}

// DropTask makes n no longer a task, taking its status marker off too.
func (n *Node) DropTask() {
	n.Task = nil
	if i := n.statusMarker(); i >= 0 {
		n.Markers = append(n.Markers[:i:i], n.Markers[i+1:]...)
		if len(n.Markers) == 0 {
			n.Markers = nil
		}
	}
}
