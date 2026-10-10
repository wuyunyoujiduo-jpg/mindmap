package model

import (
	"testing"
	"time"
)

func TestMarkdown(t *testing.T) {
	d := New("我的项目")
	d.Root.Note = "总体说明"
	a, _ := d.AddChild(d.Root.ID, "功能")
	b, _ := d.AddChild(a.ID, "登录")
	b.Tags = []string{"核心", "安全 审计"}
	b.Task = &Task{Status: StatusDone}
	due := time.Date(2026, 10, 13, 0, 0, 0, 0, time.Local)
	s, _ := d.AddChild(a.ID, "搜索")
	s.Task = &Task{Status: StatusDoing, Priority: PriorityHigh, Due: &due}
	s.Note = "支持拼音\n\n和模糊匹配"
	c, _ := d.AddChild(d.Root.ID, "多行\n标题")
	c.Collapsed = true
	d.AddChild(c.ID, "折叠的子节点")
	want := `# 我的项目

总体说明

- 功能
  - [x] 登录 #核心 #安全_审计
  - [ ] 搜索（进行中 · 优先级 高 · 截止 2026-10-13）
    支持拼音

    和模糊匹配
- 多行 标题
  - 折叠的子节点
`
	if got := Markdown(d); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	f := &File{Sheets: []*Document{New("一"), New("二")}}
	if got := MarkdownFile(f); got != "# 一\n\n---\n\n# 二\n" {
		t.Fatalf("file %q", got)
	}
}
