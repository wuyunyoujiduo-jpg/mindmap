package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// TestPhonePages goes through a phone's pages with a few weeks' use in
// them, as the user looks at them (snapshots with MINDMAP_SNAPSHOTS, and
// DARK=1 for the dark): a task says its map once, a clip is a page of its
// own, details open with no field taking the keyboard.
func TestPhonePages(t *testing.T) {
	was := onPhone
	onPhone = true
	defer func() { onPhone = was }()
	a, tt := newTestApp(t)
	tt.SetSize(390, 844)
	tt.SetInsets(47, 0, 34, 0)
	if os.Getenv("DARK") != "" {
		tt.SetDark(true)
	}
	day := today(a.now())
	at := func(d int) *time.Time { x := day.AddDate(0, 0, d); return &x }
	for _, m := range []struct {
		name  string
		tasks []struct {
			t string
			d int
		}
	}{
		{"ergora", []struct {
			t string
			d int
		}{{"用户管理", -3}, {"待收集场景", -1}, {"访销订单详情页", 0}, {"权限矩阵评审", 2}}},
		{"ergora上线", []struct {
			t string
			d int
		}{{"灰度发布", 1}, {"回滚预案", 4}}},
		{"星星打工人", nil},
	} {
		f := model.NewFile(m.name)
		for _, k := range []string{"架构", "页面", "接口"} {
			f.Sheets[0].Root.Children = append(f.Sheets[0].Root.Children, model.NewNode(k))
		}
		for _, tk := range m.tasks {
			n := model.NewNode(tk.t)
			n.SetStatus(model.StatusTodo, a.now())
			n.Task.Due = at(tk.d)
			f.Sheets[0].Root.Children[0].Children = append(f.Sheets[0].Root.Children[0].Children, n)
		}
		if err := f.Save(filepath.Join(a.dir, model.FileName(m.name))); err != nil {
			t.Fatal(err)
		}
	}
	a.refreshFiles()
	for _, s := range []string{"导图的工具条只在选中主题时出现", "周会改到周四下午三点", "#方法 先写结论再写过程，读的人先知道要什么", "https://example.com/cache 缓存设计的五个原则", "店长要能看到隔壁店的库存吗？"} {
		a.addClipText(s, "手动")
	}
	shot := func(name string) {
		tt.Frame()
		tt.Frame()
		tt.Advance(time.Second)
		tt.Frame()
		snapshot(t, tt, "p-"+name)
	}
	a.goToday()
	shot("today")
	if !tt.HasText("ergora · 架构") || tt.HasText("ergora · ergora › 架构") {
		t.Fatalf("a task's place: %q", tt.Texts())
	}
	a.drawer = true
	shot("drawer")
	a.drawer = false
	a.tasks.view = tasksDay
	shot("day")
	a.tasks.view = tasksToday
	a.showProjects()
	shot("projects")
	a.goCollect()
	shot("collect")
	a.libSel = a.lib.Clips[0].ID
	shot("clip")
	if _, ok := tt.Find("位置"); ok || !tt.HasText("收集") {
		t.Fatal("a clip on a phone is no page of its own")
	}
	a.libSel = ""
	_ = a.open(a.files[0].Path)
	shot("map")
	a.showView(viewOutline)
	shot("outline")
	a.showView(viewMap)
	a.panelSheet = true
	shot("panel")
	if _, typing := tt.TextCaret(); typing {
		t.Fatal("the topic's title took the focus as its sheet rose: the keyboard would come up")
	}
	a.panelSheet = false
	a.goToday()
	tt.Frame()
	_ = tt.Click("访销订单详情页")
	shot("task")
}
