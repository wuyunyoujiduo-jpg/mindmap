package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestPhoneLayout lays the window out as a phone's (phone.go): the places
// at the foot, the left column over the page and gone once a list is
// chosen, what is chosen over the page with the way back, and nothing
// past the screen's right edge.
func TestPhoneLayout(t *testing.T) {
	a, _ := newTestApp(t)
	const w, h = 390, 844
	tt := ui.NewTester(a.view, w, h)
	tt.SetInsets(24, 0, 16, 0) // the system's bars
	inside := func(label string) {
		t.Helper()
		r, ok := tt.Find(label)
		if !ok {
			t.Fatalf("no %q: %q", label, tt.Texts())
		}
		if r.X < 0 || r.X+r.W > w+0.5 {
			t.Fatalf("%q past the screen: %+v", label, r)
		}
	}

	a.goToday()
	tt.Frame()
	if !narrowLayout {
		t.Fatal("a phone's window not laid out as one")
	}
	// The places' row, at the foot: the top row has none.
	foot, _ := tt.Find("位置")
	if foot.Y < h/2 || foot.X+foot.W > w {
		t.Fatalf("the places at %+v", foot)
	}
	tab := func(name string) {
		t.Helper()
		for i, p := range phonePlaces {
			if p.name == name {
				tt.ClickAt(foot.X+foot.W*(float32(i)+0.5)/float32(len(phonePlaces)), foot.Y+foot.H/2)
				return
			}
		}
		t.Fatalf("no place %s", name)
	}
	inside("日程")
	inside("列表加载慢")
	snapshot(t, tt, "phone-today")

	// The left column, over the page, gone once a list is chosen.
	if err := tt.Click("侧栏"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(motionSlow * 2)
	tt.Frame()
	if !a.drawer {
		t.Fatal("the drawer did not open")
	}
	snapshot(t, tt, "phone-drawer")
	if err := tt.Click("最近 7 天"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if a.drawer || a.tasks.view != tasksWeekAhead {
		t.Fatalf("drawer %v, list %d", a.drawer, a.tasks.view)
	}

	// A task chosen rises from the foot, and its ✕ puts it away.
	a.goToday()
	tt.Frame()
	if err := tt.Click("列表加载慢"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.tasks.sel == "" {
		t.Fatal("the task not chosen")
	}
	for range 5 { // the sheet rises
		tt.Advance(300 * time.Millisecond)
		tt.Frame()
	}
	inside("关闭详情")
	snapshot(t, tt, "phone-task")
	if err := tt.Click("关闭详情"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.tasks.sel != "" {
		t.Fatal("✕ did not put the task away")
	}

	// The other places, from the foot.
	for _, p := range []string{"收集", "项目"} {
		tab(p)
		tt.Frame()
		tt.Frame()
		if i := slicesIndexPlace(a.place()); i < 0 || phonePlaces[i].name != p {
			t.Fatalf("tab %s showed place %d", p, a.place())
		}
		snapshot(t, tt, "phone-"+p)
	}

	// A map, and its topic's panel over the canvas.
	if err := a.open(a.files[0].Path); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.panelShown(nil) {
		t.Fatal("the panel over the canvas from the start")
	}
	// The chosen topic's keys at the canvas's foot; its details from there.
	if _, ok := tt.Find("主题操作"); !ok {
		t.Fatal("no keys of the chosen topic")
	}
	if err := tt.Click("详情"); err != nil {
		t.Fatal(err)
	}
	for range 5 { // the sheet rises
		tt.Advance(300 * time.Millisecond)
		tt.Frame()
	}
	if !a.panelSheet {
		t.Fatal("详情 did not open the topic's sheet")
	}
	if r, ok := tt.Find("主题面板"); !ok || r.Y > h/2 {
		t.Fatalf("the topic's sheet at %+v", r)
	}
	snapshot(t, tt, "phone-panel")
	a.panelSheet = false
	tt.Frame()
	snapshot(t, tt, "phone-map")

	// The settings and the AI, over the page.
	a.openSettings()
	tt.Frame()
	snapshot(t, tt, "phone-settings")
	a.settingsOpen = false
	a.aiOpen = true
	tt.Frame()
	inside("AI 窗口")
	snapshot(t, tt, "phone-ai")
	a.aiOpen = false

	// A desktop's window again: as it was.
	tt.SetSize(1280, 800)
	tt.Frame()
	if narrowLayout || a.drawer {
		t.Fatal("still a phone's")
	}
	if r, _ := tt.Find("位置"); r.Y > 60 {
		t.Fatalf("the places at %+v on a desktop", r)
	}
}

func slicesIndexPlace(p int) int {
	for i, pl := range phonePlaces {
		if pl.place == p {
			return i
		}
	}
	return -1
}

// TestPhoneShot snapshots the phone's pages, for a look beside the draft.
func TestPhoneShot(t *testing.T) {
	a, _ := newTestApp(t)
	tt := ui.NewTester(a.view, 390, 844)
	tt.SetInsets(47, 0, 34, 0)
	for _, text := range []string{"导图的工具条只在选中主题时出现", "周会改到周四下午三点", "#方法 先写结论再写过程，读的人先知道要什么"} {
		if _, err := a.addClipText(text, "手动"); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range phonePlaces {
		a.goPlace(p.place)
		tt.Frame()
		tt.Frame()
		snapshot(t, tt, "shot-"+p.name)
	}
	a.goToday()
	tt.Frame()
	if err := tt.Click("记一条随记或任务"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if !a.barOpen || tt.HasText("项目") && func() bool { r, _ := tt.Find("位置"); return r.H > 0 }() {
		t.Fatal("the bar did not grow in place of the places")
	}
	snapshot(t, tt, "shot-bar")
	a.barOpen = false
	a.goToday()
	tt.Frame()
	if err := tt.Click("列表加载慢"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for range 5 {
		tt.Advance(300 * time.Millisecond)
		tt.Frame()
	}
	snapshot(t, tt, "shot-task")
	a.tasks.sel = ""
	a.tasks.view = tasksDay
	tt.Frame()
	tt.Frame()
	snapshot(t, tt, "shot-day")
	a.tasks.view = tasksToday
	_ = a.open(a.files[0].Path)
	tt.Frame()
	tt.Frame()
	snapshot(t, tt, "shot-map")
	// The outline, a line written in: its keys over the keyboard.
	a.showView(viewOutline)
	tt.Frame()
	for _, r := range outlineRows(a.doc) {
		a.outlineFocus = r.node.ID
		break
	}
	tt.Frame()
	tt.Frame()
	if _, ok := tt.Find("大纲的键"); !ok {
		t.Fatal("no outline keys while a line is written in")
	}
	snapshot(t, tt, "shot-outline")
}
