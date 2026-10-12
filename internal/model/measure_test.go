package model

import (
	"encoding/json"
	"testing"
)

func TestMeasures(t *testing.T) {
	f := NewFile("项目")
	root := f.Sheets[0].Root
	a := &Node{ID: "a", Title: "前端"}
	b := &Node{ID: "b", Title: "登录"}
	c := &Node{ID: "c", Title: "搜索"}
	a.Children = []*Node{b, c}
	root.Children = append(root.Children, a)
	if !f.AddMeasure("工时") || f.AddMeasure("工时") || !f.AddMeasure("成本") {
		t.Fatal("add")
	}
	b.SetValue("工时", 3, true)
	c.SetValue("工时", 4.5, true)
	a.SetValue("工时", 1, true)
	if got := root.Total("工时"); got != 8.5 {
		t.Fatalf("total %v", got)
	}
	if !a.Measured("工时") || root.Measured("成本") {
		t.Fatal("measured")
	}
	cp := a.Copy()
	cp.Children[0].SetValue("工时", 100, true)
	if b.Values["工时"] != 3 {
		t.Fatal("a copy shares its values")
	}
	if !f.RenameMeasure("工时", "人天") || root.Total("人天") != 8.5 || root.Total("工时") != 0 {
		t.Fatal("rename")
	}
	data, _ := json.Marshal(f)
	var g File
	if err := json.Unmarshal(data, &g); err != nil || g.Sheets[0].Root.Total("人天") != 8.5 || len(g.Measures) != 2 {
		t.Fatalf("round trip %v", err)
	}
	if h := f.Clone(); h.Measures[0] != "人天" {
		t.Fatal("clone")
	} else {
		h.Measures[0] = "x"
		if f.Measures[0] != "人天" {
			t.Fatal("a clone shares its measures")
		}
	}
	f.RemoveMeasure("人天")
	if len(f.Measures) != 1 || b.Values != nil {
		t.Fatalf("remove %v %v", f.Measures, b.Values)
	}
	for v, want := range map[float64]string{12: "12", 3.5: "3.5", 1.25: "1.25", 0: "0", 2.0001: "2"} {
		if FormatValue(v) != want {
			t.Errorf("FormatValue(%v) = %q", v, FormatValue(v))
		}
	}
}
