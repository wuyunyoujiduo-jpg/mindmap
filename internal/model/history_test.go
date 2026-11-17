package model

import (
	"fmt"
	"runtime"
	"testing"
)

// bigFile is a map of about n topics, a few of them tasks.
func bigFile(n int) *File {
	d := New("大图")
	for i := 0; i*50 < n; i++ {
		b := NewNode(fmt.Sprintf("分支 %d", i))
		for j := range 49 {
			c := NewNode(fmt.Sprintf("主题 %d-%d", i, j))
			if j%7 == 0 {
				c.Task = &Task{Status: StatusTodo}
			}
			b.Children = append(b.Children, c)
		}
		d.Root.Children = append(d.Root.Children, b)
	}
	return &File{Sheets: []*Document{d}}
}

// heapNow is what the heap holds, after a collection.
func heapNow() int64 {
	var m runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m)
	return int64(m.HeapAlloc)
}

// The steps of a large map are bounded by what they hold: two hundred
// copies of a map of five thousand topics were 245 MB.
func TestHistoryBudget(t *testing.T) {
	f := bigFile(5000)
	d := f.Sheets[0]
	const budget, keep = 16 << 20, 5
	before := heapNow()
	h := NewHistory(200).Budget(budget, keep)
	for i := range 200 {
		h.Record(State{File: f, Sheet: d.ID})
		d.Root.Children[0].Title = fmt.Sprint("改 ", i)
	}
	held := heapNow() - before
	t.Logf("%d steps of 5000 topics: counted %.1f MB, held %.1f MB", h.Steps(), float64(h.Size())/(1<<20), float64(held)/(1<<20))
	if h.Size() > budget || h.Steps() < keep || h.Steps() == 200 {
		t.Fatalf("%d steps holding %d bytes, for a budget of %d", h.Steps(), h.Size(), budget)
	}
	// What is counted is about what is held: within a half of it.
	if held > 3*budget/2 || held < h.Size()/2 {
		t.Fatalf("the steps hold %d bytes, counted as %d", held, h.Size())
	}
	// The steps left are the last, in order.
	s, ok := h.Undo(State{File: f, Sheet: d.ID})
	if !ok || s.File.Sheets[0].Root.Children[0].Title != "改 198" {
		t.Fatalf("undo gave %q", s.File.Sheets[0].Root.Children[0].Title)
	}
	runtime.KeepAlive(h)

	// The last steps stay, whatever they weigh.
	h = NewHistory(200).Budget(1, 3)
	for range 10 {
		h.Record(State{File: f, Sheet: d.ID})
	}
	if h.Steps() != 3 {
		t.Fatalf("%d steps kept of the last 3", h.Steps())
	}
	// A small map keeps its two hundred.
	small := bigFile(100)
	h = NewHistory(200).Budget(budget, keep)
	for range 250 {
		h.Record(State{File: small, Sheet: small.Sheets[0].ID})
	}
	if h.Steps() != 200 {
		t.Fatalf("%d steps of a small map", h.Steps())
	}
}
