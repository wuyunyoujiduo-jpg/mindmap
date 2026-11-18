package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// TestPhoneTaps taps every key of a phone's pages with a finger, motion
// on and frames in time, as on a phone, and fails on a key whose tap
// changes nothing: 更多 under a topic flashed and did nothing on the
// user's phone (0.108.11). MINDMAP_TAPS=1 runs it: it taps some hundreds.
func TestPhoneTaps(t *testing.T) {
	if os.Getenv("MINDMAP_TAPS") == "" {
		t.Skip("MINDMAP_TAPS=1 taps every key")
	}
	was := onPhone
	onPhone = true
	defer func() { onPhone = was }()
	a, tt := newTestApp(t)
	a.noMotion = false
	const W, H = 390, 844
	tt.SetSize(W, H)
	tt.SetInsets(47, 0, 34, 0)
	phoneSeed(t, a)
	ergora := filepath.Join(a.dir, model.FileName("ergora"))
	step := func(n int) {
		for range n {
			tt.Advance(16 * time.Millisecond)
			tt.Frame()
		}
	}
	home := func() {
		tt.CloseMenu()
		a.settingsOpen, a.paletteOpen, a.historyOpen, a.templatesOpen, a.aiOpen = false, false, false, false, false
		a.drawer, a.barOpen, a.panelSheet, a.tourStep = false, false, false, 0
		a.libSel, a.tasks.sel, a.homeDraft = "", "", ""
		a.goToday()
		a.tasks.view, a.tasks.onMap = tasksToday, ""
		step(30)
	}
	sign := func() string {
		cam := a.cam()
		return fmt.Sprint(strings.Join(tt.Texts(), "|"), tt.Menu(), a.selected, a.mode, a.libOpen, a.place(), a.drawer, a.panelSheet,
			a.settingsOpen, a.settingsHome, a.settingsTab, a.paletteOpen, a.aiOpen, a.barOpen, a.tasks.view, a.tasks.sel, a.libSel, a.libCat,
			cam.X, cam.Y, cam.Zoom, a.editing, a.tourStep)
	}
	type state struct {
		name string
		open func()
	}
	inMap := func(then func()) func() {
		return func() {
			_ = a.open(ergora)
			step(20)
			if then != nil {
				then()
			}
		}
	}
	states := []state{
		{"today", func() {}},
		{"today-day", func() { a.tasks.view = tasksDay }},
		{"projects", a.showProjects},
		{"collect", a.goCollect},
		{"drawer", func() { a.drawer = true }},
		{"task", func() { _ = tt.Click("访销订单详情页") }},
		{"clip", func() { a.goCollect(); step(10); a.libSel = a.lib.Clips[0].ID }},
		{"map", inMap(nil)},
		{"map-topic", inMap(func() { a.selectNode(a.doc.Root.Children[0].ID) })},
		{"map-panel", inMap(func() { a.selectNode(a.doc.Root.Children[0].ID); a.panelSheet = true })},
		{"outline", inMap(func() { a.showView(viewOutline) })},
		{"settings", a.openSettings},
	}
	skip := []string{"删除", "回收站", "清除", "恢复到这个版本", "下载", "安装", "重新查看", "再看一遍", "导出", "AI 拆解", "立即", "检查"}
	var dead []string
	tapped := 0
	for _, s := range states {
		home()
		s.open()
		step(30)
		type key struct {
			name string
			at   ui.Rect
		}
		var keys []key
		seen := map[string]bool{}
		for _, e := range tt.AccessAll() {
			b := e.Bounds
			switch e.Role {
			case ui.RoleGroup, ui.RoleButton, ui.RoleLink, ui.RolePopUpButton, ui.RoleMenuButton, ui.RoleTab, ui.RoleSwitch, ui.RoleCheckBox, ui.RoleRadio:
			default:
				continue
			}
			id := fmt.Sprint(e.Name, b)
			if e.Name == "" || seen[id] || e.Disabled || b.W < 4 || b.H < 4 || b.H > 120 || b.X < 0 || b.X+b.W > W || b.Y < 47 || b.Y+b.H > H-20 {
				continue
			}
			if slicesContainsAny(e.Name, skip) {
				continue
			}
			seen[id] = true
			keys = append(keys, key{e.Name, b})
		}
		for _, k := range keys {
			home()
			s.open()
			step(30)
			before := sign()
			x, y := k.at.X+k.at.W/2, k.at.Y+k.at.H/2
			tt.FingerDown(x, y)
			step(4)
			tt.FingerUp(x, y)
			step(30)
			tapped++
			if sign() == before {
				dead = append(dead, fmt.Sprintf("%s: %s at %.0f,%.0f", s.name, k.name, x, y))
			}
		}
	}
	t.Logf("tapped %d keys", tapped)
	for _, d := range dead {
		t.Error("a tap changed nothing: " + d)
	}
}

func slicesContainsAny(s string, words []string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}
