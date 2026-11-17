package main

import (
	"fmt"
	"testing"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// What the maps cost, on maps of a size: each a test that counts what a
// frame or an edit does, and a benchmark that times it.

// wideBranch opens a new map whose one main topic has n children, and
// which shows that topic: its children are far above and below the window.
func wideBranch(a *app, n int) *model.Node {
	a.newMap()
	a.commitEditIfAny()
	var hub *model.Node
	a.mutate("", func() {
		hub, _ = a.doc.AddChild(a.doc.Root.ID, "很宽的主题")
		for i := range n {
			hub.Children = append(hub.Children, model.NewNode(fmt.Sprintf("子主题 %d", i)))
		}
	})
	a.mode, a.selected = viewMap, hub.ID
	return hub
}

// A topic with five hundred children: its branches, each longer than a
// path may be, are drawn where they show, a few parts each, and not
// sampled whole.
func TestWideBranchDrawsWhatShows(t *testing.T) {
	a, tt := newTestApp(t)
	hub := wideBranch(a, 500)
	tt.Frame()
	tt.Frame()
	curveParts = 0
	a.cam().X++ // the canvas is drawn again as it is moved
	tt.Frame()
	// Whole, the five hundred curves were some 900,000 points a frame.
	if curveParts < 500 || curveParts > 3000 {
		t.Fatalf("a frame drew %d parts of the %d branches", curveParts, len(hub.Children))
	}
	// And they show: a branch's color, on the way from the topic to its
	// children's column.
	box := a.boxes.Boxes[hub.ID]
	child := a.boxes.Boxes[hub.Children[0].ID]
	cam, img := a.cam(), tt.Image()
	x := int(a.vp.X + cam.X + (box.X+box.W+child.X)/2*cam.Zoom)
	white, drawn := img.RGBAAt(x, int(a.vp.Y)+2), false
	for y := int(a.vp.Y); y < int(a.vp.Y+a.vp.H); y++ {
		drawn = drawn || img.RGBAAt(x, y) != white
	}
	if !drawn {
		t.Fatal("no branch between the topic and its children")
	}
	snapshot(t, tt, "map-wide-branch")
}

func BenchmarkMapWideBranch(b *testing.B) {
	a, tt := newTestApp(b)
	wideBranch(a, 500)
	tt.Frame()
	tt.Frame()
	for b.Loop() {
		a.cam().X++ // a frame of the canvas being moved
		tt.Frame()
	}
}

// The steps of the maps not open are kept for the few left last.
func TestHistoriesOfMapsLeft(t *testing.T) {
	a, tt := newTestApp(t)
	var paths []string
	for i := range histMaps + 3 {
		a.mutate("", func() { a.doc.AddChild(a.doc.Root.ID, fmt.Sprint("改动 ", i)) })
		paths = append(paths, a.path)
		a.newMap()
		tt.Frame()
	}
	a.flush()
	if len(a.hists) != histMaps+1 || len(a.histAt) != histMaps+1 {
		t.Fatalf("steps kept for %d maps (%d), of %d left and the one open", len(a.hists), len(a.histAt), histMaps)
	}
	if a.hists[paths[0]] != nil || a.hists[paths[len(paths)-1]] == nil {
		t.Fatal("not the maps left last kept their steps")
	}
	// A map left last is come back to with its steps.
	last := paths[len(paths)-1]
	if err := a.open(last); err != nil {
		t.Fatal(err)
	}
	n := len(a.doc.Root.Children)
	a.undo()
	if len(a.doc.Root.Children) != n-1 {
		t.Fatal("the map come back to had lost its steps")
	}
}
