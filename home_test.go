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
	cats := []string{"技术", "产品", "工作", "技术", "学习", "技术", "工作"}
	for i, cat := range cats {
		clip, err := a.addClipText(fmt.Sprintf("资料 %d：关于%s的一些记录", i, cat), "手动")
		if err != nil {
			t.Fatal(err)
		}
		clip.Category = cat
		clip.Created = testNow.Add(-time.Duration(i*3) * 24 * time.Hour)
		clip.Updated = clip.Created
	}
	if err := tt.Click("概览"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.libOpen || !a.libHome {
		t.Fatal("概览 not shown")
	}
	for _, want := range []string{"导图", "文档", "待归档", "待 AI 整理", "最近 30 天", "分类分布", "最近更新", "分类 技术"} {
		if !tt.HasText(want) {
			t.Fatalf("no %q: %q", want, tt.Texts())
		}
	}
	snapshot(t, tt, "home")

	// A category leads to the list, filtered; 显示全部 to all of it.
	if err := tt.Click("分类 技术"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.libHome || a.libCat != "技术" || len(a.libHits()) != 3 {
		t.Fatalf("category: home %v, %q, %d", a.libHome, a.libCat, len(a.libHits()))
	}
	snapshot(t, tt, "home-list")
	a.libHome = true
	tt.Frame()
	if err := tt.Click("显示全部 最近更新"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.libHome || a.libCat != "" {
		t.Fatal("显示全部 did not show the list")
	}
}

// A day of 概览's dots lists what it holds; the sidebar's keys go through
// its maps and documents; dark, the pages read as well.
func TestHomeDaysKeysAndDark(t *testing.T) {
	a, tt := newTestApp(t)
	for i := range 3 {
		clip, err := a.addClipText(fmt.Sprintf("第 %d 条", i), "手动")
		if err != nil {
			t.Fatal(err)
		}
		clip.Created = testNow.Add(-time.Duration(i) * 24 * time.Hour)
	}
	a.showLibrary()
	a.libHome = true
	tt.Frame()
	day := testNow.Add(-24 * time.Hour)
	if err := tt.Click(fmt.Sprintf("%s：编辑导图 0 张 · 收藏资料 1 条", day.Format("1月2日"))); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if hits := a.libHits(); a.libHome || len(hits) != 1 || hits[0].Clip.Label() != "第 1 条" {
		t.Fatalf("the day's list: home %v, %d", a.libHome, len(hits))
	}
	if !tt.HasText(day.Format("1月2日") + " ✕") {
		t.Fatal("no chip of the day")
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
