package main

import (
	"testing"

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
	inside("月")
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

	// A task chosen covers the page, and 返回 goes back to the list.
	a.goToday()
	tt.Frame()
	if err := tt.Click("列表加载慢"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.tasks.sel == "" {
		t.Fatal("the task not chosen")
	}
	inside("返回")
	snapshot(t, tt, "phone-task")
	if err := tt.Click("返回"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.tasks.sel != "" || tt.HasText("返回") {
		t.Fatal("返回 did not put the task away")
	}

	// The other places, from the foot.
	for _, p := range []string{"收集", "技能", "项目"} {
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
	if err := tt.Click("主题面板"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	inside("返回")
	snapshot(t, tt, "phone-panel")
	if err := tt.Click("返回"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.panelSheet {
		t.Fatal("返回 did not put the panel away")
	}
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
