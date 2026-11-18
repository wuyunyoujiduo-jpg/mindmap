package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// phoneSeed fills a test app with a few weeks' use: three maps with
// tasks due round today, and a few clips.
func phoneSeed(t *testing.T, a *app) {
	t.Helper()
	day := today(a.now())
	at := func(d int) *time.Time { x := day.AddDate(0, 0, d); return &x }
	for _, m := range []struct {
		name  string
		tasks map[string]int
	}{
		{"ergora", map[string]int{"用户管理": -3, "待收集场景": -1, "访销订单详情页": 0, "权限矩阵评审": 2}},
		{"ergora上线", map[string]int{"灰度发布": 1, "回滚预案": 4}},
		{"星星打工人", nil},
	} {
		f := model.NewFile(m.name)
		for _, k := range []string{"架构", "页面", "接口"} {
			f.Sheets[0].Root.Children = append(f.Sheets[0].Root.Children, model.NewNode(k))
		}
		for title, d := range m.tasks {
			n := model.NewNode(title)
			n.SetStatus(model.StatusTodo, a.now())
			n.Task.Due = at(d)
			f.Sheets[0].Root.Children[0].Children = append(f.Sheets[0].Root.Children[0].Children, n)
		}
		if err := f.Save(filepath.Join(a.dir, model.FileName(m.name))); err != nil {
			t.Fatal(err)
		}
	}
	a.refreshFiles()
	for _, s := range []string{"导图的工具条只在选中主题时出现", "周会改到周四下午三点", "#方法 先写结论再写过程", "https://example.com/cache 缓存设计的五个原则"} {
		a.addClipText(s, "手动")
	}
}

// desktopWords are what a phone's screen is not to say: a keyboard's keys
// and what a pointer alone does.
var desktopWords = []string{"⌘", "⌥", "⇧", "快捷键", "右键", "鼠标", "悬停", "双击", "回车", "Esc", "Tab 键", "菜单栏", "Dock", "拖到主题上"}

// TestPhoneAudit goes through every page and window a phone shows, as the
// user's corrections of 0.108 found them one by one: what runs past the
// screen or sits cut at its side, what speaks of a keyboard or a pointer,
// and what Android's back does not go back from, leaving the app instead.
// MINDMAP_AUDIT=1 logs the keys smaller than a finger too.
func TestPhoneAudit(t *testing.T) {
	was := onPhone
	onPhone = true
	defer func() { onPhone = was }()
	a, tt := newTestApp(t)
	const W, H = 390, 844
	tt.SetSize(W, H)
	tt.SetInsets(47, 0, 34, 0)
	phoneSeed(t, a)
	ergora := filepath.Join(a.dir, model.FileName("ergora"))
	settle := func() {
		tt.Frame()
		tt.Frame()
		tt.Advance(time.Second)
		tt.Frame()
	}
	// back is Android's: Escape, and the Back key where Escape was not
	// taken (MyGo's surface.back); a change of what shows tells it.
	back := func() {
		was := strings.Join(tt.Texts(), "|")
		tt.Key(0, ui.KeyEscape)
		settle()
		if strings.Join(tt.Texts(), "|") == was {
			tt.Key(0, ui.KeyBack)
			settle()
		}
	}
	home := func() {
		a.settingsOpen, a.paletteOpen, a.historyOpen, a.templatesOpen, a.aiOpen = false, false, false, false, false
		a.drawer, a.barOpen, a.panelSheet, a.topicMore, a.tourStep = false, false, false, false, 0
		a.libSel = ""
		a.goToday()
		a.tasks.view = tasksToday
		settle()
	}
	var found []string
	say := func(state, f string, args ...any) { found = append(found, state+": "+fmt.Sprintf(f, args...)) }
	look := func(state string, sideways bool) {
		snapshot(t, tt, "audit-"+state)
		for _, e := range tt.AccessAll() {
			b := e.Bounds
			if b.W <= 0 || b.H <= 0 {
				continue
			}
			name := cmpOr(e.Name, e.Placeholder)
			if !sideways && (b.X < -0.5 || b.X+b.W > W+0.5) {
				say(state, "past the screen's side: %q %v %+v", name, e.Role, b)
			}
			for _, w := range desktopWords {
				for _, s := range []string{e.Name, e.Placeholder, e.Description} {
					if strings.Contains(s, w) {
						say(state, "a computer's word %q in %q", w, s)
					}
				}
			}
			if testing.Verbose() && e.Role == ui.RoleButton && (b.W < 32 || b.H < 32) && name != "" {
				t.Logf("%s: a small key %q %.0f×%.0f", state, name, b.W, b.H)
			}
		}
		for _, s := range tt.Texts() {
			for _, w := range desktopWords {
				if strings.Contains(s, w) {
					say(state, "a computer's word %q in %q", w, s)
				}
			}
		}
	}
	type state struct {
		name   string
		open   func()
		gone   func() bool // after back
		inner  bool        // back goes a level, not home
		places bool        // a place: back goes to 今天
		// sideways: it scrolls sideways, as the board's columns and
		// 收集's kinds do, what is past the side being scrolled to
		sideways bool
	}
	states := []state{
		{name: "today", open: func() {}},
		{name: "today-day", open: func() { a.tasks.view = tasksDay }, places: true},
		{name: "today-review", open: func() { a.tasks.view = tasksReview }, places: true},
		{name: "today-waiting", open: func() { a.tasks.view = tasksWaiting }, places: true},
		{name: "projects", open: a.showProjects, places: true},
		{name: "collect", open: a.goCollect, places: true, sideways: true},
		{name: "drawer-today", open: func() { a.drawer = true }, gone: func() bool { return !a.drawer }},
		{name: "drawer-projects", open: func() { a.showProjects(); settle(); a.drawer = true }, gone: func() bool { return !a.drawer }},
		{name: "drawer-collect", open: func() { a.goCollect(); settle(); a.drawer = true }, gone: func() bool { return !a.drawer }},
		{name: "bar", open: func() { a.barOpen = true }, gone: func() bool { return !a.barOpen }},
		{name: "task", open: func() { _ = tt.Click("访销订单详情页") }, gone: func() bool { return !tt.HasText("提醒") }},
		{name: "clip", open: func() { a.goCollect(); settle(); a.libSel = a.lib.Clips[0].ID }, gone: func() bool { return a.libSel == "" }, sideways: true},
		{name: "map", open: func() { _ = a.open(ergora) }, gone: func() bool { return a.libOpen }},
		{name: "map-topic", open: func() { _ = a.open(ergora); settle(); a.selectNode(a.doc.Root.Children[0].ID) }, gone: func() bool { return a.libOpen || a.selected == "" }},
		{name: "map-panel", open: func() { _ = a.open(ergora); settle(); a.panelSheet = true }, gone: func() bool { return !a.panelSheet }},
		{name: "map-more", open: func() { _ = a.open(ergora); settle(); a.topicMore = true }, gone: func() bool { return !a.topicMore }},
		{name: "outline", open: func() { _ = a.open(ergora); settle(); a.showView(viewOutline) }, gone: func() bool { return a.libOpen }},
		{name: "board", open: func() { _ = a.open(ergora); settle(); a.showView(viewBoard) }, gone: func() bool { return a.libOpen }, sideways: true},
		{name: "timeline", open: func() { _ = a.open(ergora); settle(); a.showView(viewTimeline) }, gone: func() bool { return a.libOpen }},
		{name: "overview", open: func() { _ = a.open(ergora); settle(); a.showView(viewOverview) }, gone: func() bool { return a.libOpen }},
		{name: "diagram", open: func() { _ = a.open(ergora); settle(); a.showView(viewDiagram) }, gone: func() bool { return a.libOpen }},
		{name: "ai", open: func() { a.aiOpen = true }, gone: func() bool { return !a.aiOpen }},
		{name: "palette", open: func() { a.openPalette(paletteAll) }, gone: func() bool { return !a.paletteOpen }},
		{name: "templates", open: func() { a.templatesOpen = true }, gone: func() bool { return !a.templatesOpen }},
		{name: "history", open: func() { _ = a.open(ergora); settle(); a.openHistory() }, gone: func() bool { return !a.historyOpen }},
		{name: "settings", open: a.openSettings, gone: func() bool { return !a.settingsOpen }},
		{name: "tour", open: func() { a.tourStep = 1 }, gone: func() bool { return a.tourStep == 0 }},
	}
	for _, s := range states[1:] {
		if s.gone == nil && !s.places {
			t.Fatalf("%s: what back does is not said", s.name)
		}
	}
	for i := range settingsTabs {
		i := i
		states = append(states, state{name: fmt.Sprint("settings-", settingsTabs[i]), open: func() {
			a.openSettings()
			a.settingsTab, a.settingsHome = i, false
		}, gone: func() bool { return a.settingsHome && a.settingsOpen }, inner: true})
	}
	for _, s := range states {
		home()
		s.open()
		settle()
		look(s.name, s.sideways)
		if s.name == "today" { // the app's first page: back leaves it, as Android's apps do
			continue
		}
		back()
		switch {
		case s.places:
			if !a.libOpen || a.place() != placeToday || a.tasks.view != tasksToday {
				say(s.name, "back did not go to 今天 (place %d, view %d)", a.place(), a.tasks.view)
			}
		case !s.gone():
			say(s.name, "back did not close it")
		}
	}
	for _, f := range found {
		t.Error(f)
	}
}
