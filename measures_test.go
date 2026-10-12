package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// Measures: added in the table, set there for the leaves, added up for
// the nodes above, shown by the nodes on the map.
func TestMeasuresTable(t *testing.T) {
	a, tt := newTestApp(t)
	a.mode = viewTable
	tt.Frame()
	snapshot(t, tt, "table-empty")
	if err := tt.Click("添加度量 工时"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.file.Measures) != 1 || a.file.Measures[0] != "工时" {
		t.Fatalf("measures %v", a.file.Measures)
	}
	login, search := find(t, a, "登录"), find(t, a, "搜索")
	if err := tt.Click("工时 登录"); err != nil {
		t.Fatal(err)
	}
	tt.Type("3")
	if err := tt.Click("工时 搜索"); err != nil {
		t.Fatal(err)
	}
	tt.Type("4.5")
	tt.Key(0, ui.KeyTab)
	tt.Frame()
	if login.Values["工时"] != 3 || search.Values["工时"] != 4.5 {
		t.Fatalf("values %v %v", login.Values, search.Values)
	}
	if got := a.doc.Root.Total("工时"); got != 7.5 {
		t.Fatalf("total %v", got)
	}
	if !tt.HasText("7.5") {
		t.Fatalf("no total shown: %q", tt.Texts())
	}
	snapshot(t, tt, "table")
	// A second measure, typed.
	if err := tt.Click("新度量"); err != nil {
		t.Fatal(err)
	}
	tt.Type("成本")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if len(a.file.Measures) != 2 {
		t.Fatalf("measures %v", a.file.Measures)
	}
	// Undone as any change.
	a.undo()
	if len(a.file.Measures) != 1 {
		t.Fatal("not undone")
	}
	// On the map, by the nodes.
	if got := a.measuresText(a.doc.Root); got != "工时 7.5" {
		t.Fatalf("map %q", got)
	}
	a.mode = viewMap
	tt.Frame()
	if !tt.HasText("工时 7.5") {
		t.Fatalf("not on the map: %q", tt.Texts())
	}
	snapshot(t, tt, "map-measures")
	a.cfg.HideMeasures = true
	if a.measuresText(a.doc.Root) != "" {
		t.Fatal("shown while hidden")
	}
}
