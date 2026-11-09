package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestHome(t *testing.T) {
	a, tt := newTestApp(t)
	for i := range 4 {
		clip, err := a.addClipText(fmt.Sprintf("资料 %d", i), "手动")
		if err != nil {
			t.Fatal(err)
		}
		clip.Created = testNow.Add(-time.Duration(i) * time.Hour)
	}
	if err := tt.Click("今天"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.libOpen || !a.libHome {
		t.Fatal("今天 not shown")
	}
	for _, want := range []string{greeting(testNow), dateLabel(testNow), "今天要做", "最近记下的", "最近打开的导图", "导图 使用指南"} {
		if !tt.HasText(want) {
			t.Fatalf("no %q: %q", want, tt.Texts())
		}
	}
	snapshot(t, tt, "home")

	// The line: a day makes it a task, else it is a note of 灵感.
	if err := tt.Click("记一条灵感或任务"); err != nil {
		t.Fatal(err)
	}
	tt.Type("今天 写周报 !高")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.homeDraft != "" || !tt.HasText("任务 写周报") {
		t.Fatalf("no task: %q %q", a.homeDraft, tt.Texts())
	}
	tt.Type("灵感来了")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if len(a.lib.Clips) != 5 || a.lib.Clips[0].Text != "灵感来了" || !isMemo(a.lib.Clips[0]) {
		t.Fatalf("no note: %d", len(a.lib.Clips))
	}
	snapshot(t, tt, "home-filled")

	// A map leads to it; 打开任务 to the tasks.
	if err := tt.Click("导图 使用指南"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.libOpen {
		t.Fatal("the map not opened")
	}
	a.showLibrary()
	a.libHome = true
	tt.Frame()
	if err := tt.Click("打开任务 今天要做"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.tasks.open {
		t.Fatal("tasks not opened")
	}
}

// The sidebar's keys go through its maps and documents; dark, the pages
// read as well.
func TestHomeDaysKeysAndDark(t *testing.T) {
	a, tt := newTestApp(t)
	for i := range 3 {
		clip, err := a.addClipText(fmt.Sprintf("第 %d 条", i), "手动")
		if err != nil {
			t.Fatal(err)
		}
		clip.Created = testNow.Add(-time.Duration(i) * 24 * time.Hour)
	}
	// The sidebar's keys: down from the map to the next item.
	a.unfiledOpen = true
	a.libOpen = false
	tt.Frame()
	if err := tt.Click("使用指南"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Key(0, ui.KeyUp)
	tt.Frame()
	if !a.libOpen || a.libSel == "" {
		t.Fatalf("up from the map: open %v, %q", a.libOpen, a.libSel)
	}
	tt.Key(0, ui.KeyBackspace)
	tt.Frame()
	if !a.libConfirm {
		t.Fatal("Delete did not ask to delete the document")
	}
	a.libConfirm = false

	tt.SetDark(true)
	a.libOpen, a.libHome = true, true
	tt.Frame()
	snapshot(t, tt, "home-dark")
	a.libHome = false
	tt.Frame()
	snapshot(t, tt, "library-dark")
}

func TestMidEllipsis(t *testing.T) {
	if s := midEllipsis("短名字", 200); s != "短名字" {
		t.Fatalf("short %q", s)
	}
	s := midEllipsis("资料 6：关于工作的一些记录和后续安排", 120)
	if []rune(s)[0] != '资' || !strings.HasSuffix(s, "安排") || !strings.Contains(s, "…") {
		t.Fatalf("cut %q", s)
	}
}
