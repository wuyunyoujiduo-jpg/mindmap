package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
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
	a.outlineFocus = ""
	// The tour and the settings, as a phone has them.
	for step := 2; step <= 3; step++ {
		a.tourStep = step
		tt.Frame()
		tt.Frame()
		snapshot(t, tt, fmt.Sprintf("shot-tour%d", step))
	}
	a.tourStep = 0
	a.openSettings()
	tt.Frame()
	tt.Frame()
	if !tt.HasText("返回") {
		t.Fatal("the settings are no page on a phone")
	}
	snapshot(t, tt, "shot-settings")
}

// TestPhoneMapFingers: on a phone a finger pans the map, on its blank and
// across a topic, and two fingers zoom it.
func TestPhoneMapFingers(t *testing.T) {
	a, tt := newTestApp(t)
	tt.SetSize(390, 844)
	if err := a.open(a.files[0].Path); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	swipe := func(x, y, dx, dy float32) {
		tt.FingerDown(x, y)
		tt.Frame()
		for i := 1; i <= 4; i++ {
			tt.FingerMove(x+dx*float32(i)/4, y+dy*float32(i)/4)
			tt.Advance(16 * time.Millisecond)
			tt.Frame()
		}
		tt.FingerUp(x+dx, y+dy)
		tt.Frame()
		tt.Frame()
	}
	// The blank at the top of the canvas.
	cam := *a.cam()
	swipe(200, 160, -60, 40)
	if moved := a.cam(); moved.X-cam.X > -50 || moved.Y-cam.Y < 30 {
		t.Fatalf("the blank: from %v,%v to %v,%v", cam.X, cam.Y, moved.X, moved.Y)
	}
	// Across a topic, at once: the map pans, the topic stays in the tree.
	n := a.doc.Root.Children[0]
	b := a.boxes.Boxes[n.ID]
	c := a.cam()
	x, y := a.vp.X+c.X+(b.X+b.W/2)*c.Zoom, a.vp.Y+c.Y+(b.Y+b.H/2)*c.Zoom
	cam = *a.cam()
	swipe(x, y, 0, 70)
	if moved := a.cam(); moved.Y-cam.Y < 50 {
		t.Fatalf("across a topic: from %v to %v", cam.Y, moved.Y)
	}
	if _, p := a.doc.Find(n.ID); p != a.doc.Root {
		t.Fatal("a swipe moved the topic out of its place")
	}
	// A topic held still is taken up, and let go over another goes under it.
	from, to := a.doc.Root.Children[0], a.doc.Root.Children[1]
	tt.Frame()
	c = a.cam()
	at := func(n *model.Node) (float32, float32) {
		b := a.boxes.Boxes[n.ID]
		return a.vp.X + c.X + (b.X+b.W/2)*c.Zoom, a.vp.Y + c.Y + (b.Y+b.H/2)*c.Zoom
	}
	x, y = at(from)
	tx, ty := at(to)
	tt.FingerDown(x, y)
	tt.Frame()
	tt.Advance(900 * time.Millisecond)
	tt.Frame()
	tt.Frame()
	if a.carry != from.ID {
		t.Fatalf("not taken up: %q", a.carry)
	}
	for i := 1; i <= 6; i++ {
		tt.FingerMove(x+(tx-x)*float32(i)/6, y+(ty-y)*float32(i)/6)
		tt.Advance(16 * time.Millisecond)
		tt.Frame()
	}
	if a.carryOver != to.ID {
		t.Fatalf("over %q, not %q", a.carryOver, to.ID)
	}
	tt.FingerUp(tx, ty)
	tt.Frame()
	tt.Frame()
	if _, p := a.doc.Find(from.ID); p == nil || p.ID != to.ID {
		t.Fatal("let go over a topic, it did not go under it")
	}
	if !strings.Contains(a.notice, "已移到") {
		t.Fatalf("said %q", a.notice)
	}
	// Two fingers.
	z := a.cam().Zoom
	tt.Pinch(200, 400, 1.5)
	tt.Frame()
	if a.cam().Zoom <= z {
		t.Fatalf("pinched: zoom %v to %v", z, a.cam().Zoom)
	}
}

// TestPhoneSweep snapshots, as a phone has them, every part of the
// settings and the windows the app opens, for a look at what does not fit
// a phone (MINDMAP_SNAPSHOTS); it fails on what runs past the screen.
func TestPhoneSweep(t *testing.T) {
	was := onPhone
	onPhone = true
	defer func() { onPhone = was }()
	a, tt := newTestApp(t)
	tt.SetSize(390, 844)
	shot := func(name string) {
		t.Helper()
		tt.Frame()
		tt.Frame()
		snapshot(t, tt, "sweep-"+name)
	}
	a.drawer = true
	a.openSettings() // from the drawer's foot
	if a.drawer {
		t.Fatal("the settings opened over the drawer, which stayed open under them")
	}
	shot("settings") // the list of the parts (phonesettings.go)
	if !tt.HasText("设置") || !tt.HasText("同步") || !tt.HasText("AI 助手") || tt.HasText("连接") {
		t.Fatalf("a phone's settings are a list of their parts: %q", tt.Texts())
	}
	_ = tt.Click("同步")
	tt.Frame()
	if a.settingsHome || settingsTabs[a.settingsTab] != "同步" {
		t.Fatal("a part's row opens its page")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if !a.settingsOpen || !a.settingsHome {
		t.Fatal("back from a part goes to the list, not out of the settings")
	}
	a.settingsHome = false
	for i := range len(settingsTabs) + 1 {
		a.settingsTab = i
		shot(fmt.Sprint("settings-", i))
	}
	a.settingsOpen = false
	for _, w := range []struct {
		name string
		open func()
		shut func()
	}{
		{"palette", func() { a.openPalette(paletteAll) }, func() { a.paletteOpen = false }},
		{"templates", func() { a.templatesOpen = true }, func() { a.templatesOpen = false }},
		{"history", a.openHistory, func() { a.historyOpen = false }},
		{"ai", func() { a.aiOpen = true }, func() { a.aiOpen = false }},
		{"cheats", func() { a.cheatsOpen = true }, func() { a.cheatsOpen = false }},
	} {
		w.open()
		shot(w.name)
		w.shut()
		tt.Frame()
	}
	a.goToday()
	a.tasks.view = tasksReview
	shot("review")
	a.tasks.view = tasksWaiting
	shot("waiting")
	a.tasks.view = tasksToday
	a.showLibrary()
	a.libReview = reviewDaily
	shot("lib-review")
}

// TestPhoneBack: Android's back goes from a map to the place it was
// opened from, and from a clip's page to 收集, rather than leaving the app;
// a topic's 更多 is a sheet of choices.
func TestPhoneBack(t *testing.T) {
	a, tt := newTestApp(t)
	tt.SetSize(390, 844)
	a.goToday()
	tt.Frame()
	if err := a.open(a.files[0].Path); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Key(0, ui.KeyBack)
	tt.Frame()
	tt.Frame()
	if !a.libOpen || a.place() != placeToday {
		t.Fatalf("back from a map opened from 今天: place %d, in a map %v", a.place(), !a.libOpen)
	}
	a.showProjects()
	tt.Frame()
	_ = a.open(a.files[0].Path)
	tt.Frame()
	if err := tt.Click("更多"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.topicMore || !tt.HasText("粘贴为子主题") || tt.HasText("插入 › 外框") {
		t.Fatalf("更多: %v %q", a.topicMore, tt.Texts())
	}
	a.topicMore = false
	tt.Frame()
	tt.Key(0, ui.KeyBack)
	tt.Frame()
	tt.Frame()
	if !a.libOpen || a.place() != placeMaps {
		t.Fatalf("back from a map opened from 项目: place %d", a.place())
	}
}
