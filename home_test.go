package main

import (
	"fmt"
	"testing"
	"time"
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
	for _, want := range []string{"导图", "文档", "近 7 天更新", "待 AI 整理", "最近 30 天", "分类分布", "最近更新", "分类 技术"} {
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
