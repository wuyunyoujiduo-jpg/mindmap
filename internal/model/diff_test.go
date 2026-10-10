package model

import (
	"strings"
	"testing"
	"time"
)

func TestDiff(t *testing.T) {
	f := NewFile("项目")
	d := f.Sheets[0]
	a, _ := d.AddChild(d.Root.ID, "功能")
	b, _ := d.AddChild(a.ID, "登录")
	b.Task = &Task{Status: StatusTodo}
	c, _ := d.AddChild(d.Root.ID, "架构")
	gone, _ := d.AddChild(c.ID, "旧模块")
	d.AddChild(gone.ID, "旧子模块")
	old := f.Clone()

	b.Title = "登录注册"
	b.Task.Status = StatusDone
	due := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	b.Task.Due = &due
	b.Note = "用 OAuth"
	b.Tags = []string{"核心"}
	d.Delete(gone.ID)
	n, _ := d.AddChild(c.ID, "缓存")
	d.AddChild(n.ID, "Redis")
	d.Move(b.ID, c.ID, -1)
	f.AddSheet(0)

	ch := Diff(old, f)
	var got []string
	for _, x := range ch {
		got = append(got, string(x.Kind)+":"+x.Title+":"+x.Detail)
	}
	s := strings.Join(got, "\n")
	for _, want := range []string{
		"新画布:", "新增:缓存:连同 1 个下级", "删除:旧模块:连同 1 个下级", "改名:登录注册:「登录」→「登录注册」",
		"移动:登录注册:从「功能」移到「架构」", "状态:登录注册:待做 → 完成", "任务:登录注册:截止 无 → 10/20",
		"备注:登录注册:添加了备注", "标签:登录注册:+核心",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in\n%s", want, s)
		}
	}
	if strings.Contains(s, "Redis") || strings.Contains(s, "旧子模块") {
		t.Errorf("children of added/removed listed:\n%s", s)
	}
	if sum := Summary(ch); !strings.HasPrefix(sum, "新画布 1 · 新增 1 · 删除 1") {
		t.Errorf("summary %q", sum)
	}
	if Summary(Diff(f, f)) != "没有变化" {
		t.Error("no change")
	}
}
