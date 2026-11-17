package model

import (
	"strings"
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
	c.Links = []string{"https://example.com/a", "mindmap:项目 甲#abc", "internal/model/tree.go:12"}
	d.AddChild(c.ID, "折叠的子节点")
	a.AddSummary("小结")
	fl := d.AddFloating("漂着的", Point{X: 320, Y: -120.5})
	d.AddChild(fl.ID, "它下面的")
	// A note is a quote, a line a line: read back as the note it was
	// (formats' TestMarkdownRoundTrip), where an item's own lines came
	// back a topic each.
	want := `# 我的项目

> 总体说明

- 功能
  - [x] 登录 #核心 #安全_审计
  - [ ] 搜索（进行中 · 优先级 高 · 截止 2026-10-13）
    > 支持拼音
    >
    > 和模糊匹配
  - 〔概要〕小结
- 多行 标题 <https://example.com/a> <mindmap:项目%20甲#abc> <file:internal/model/tree.go:12>
  - 折叠的子节点

- 〔自由主题 320,-120.5〕漂着的
  - 它下面的
`
	if got := Markdown(d); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	// Read back: what is pasted or answered by an AI takes the notes, the
	// task's marks, the links and the summary, and leaves a #word at a
	// line's end words; a map written out has its tags and floating
	// topics too.
	pasted := ParseOutline(want)
	if len(pasted) != 1 || pasted[0].Note != "总体说明" || len(pasted[0].Children) != 3 {
		t.Fatalf("pasted %+v", pasted)
	}
	pa := pasted[0].Children[0]
	if pa.SummaryTopic() == nil || pa.SummaryTopic().Title != "小结" || len(pa.Children) != 2 {
		t.Fatalf("the summary read as %+v", pa)
	}
	if got := pa.Children[0]; got.Title != "登录 #核心 #安全_审计" || got.Tags != nil {
		t.Fatalf("pasted, a #word is a tag: %+v", got)
	}
	if got := pa.Children[1]; got.Title != "搜索" || got.Note != "支持拼音\n\n和模糊匹配" || got.Task == nil ||
		got.Task.Status != StatusDoing || got.Task.Priority != PriorityHigh || got.Task.Due == nil || !got.Task.Due.Equal(due) {
		t.Fatalf("the task read as %+v %+v", got, got.Task)
	}
	if got := pasted[0].Children[1].Links; len(got) != 3 || got[1] != "mindmap:项目 甲#abc" || got[2] != "internal/model/tree.go:12" {
		t.Fatalf("links %q", got)
	}
	nodes, floating := ParseCanvas(want)
	if len(nodes) != 1 || len(nodes[0].Children) != 2 || len(floating) != 1 {
		t.Fatalf("a canvas read as %d nodes, %d floating", len(nodes), len(floating))
	}
	if f := floating[0]; f.Title != "漂着的" || f.Pos == nil || *f.Pos != (Point{X: 320, Y: -120.5}) || len(f.Children) != 1 {
		t.Fatalf("the floating topic read as %+v", f)
	}
	if got := nodes[0].Children[0].Children[0]; got.Title != "登录" || len(got.Tags) != 2 || got.Tags[0] != "核心" {
		t.Fatalf("tags %+v", got)
	}
	// Brackets that say nothing of a task are words; and so is a line's
	// end in angle brackets that is no link.
	for line, title := range map[string]string{
		"- [ ] 写周报（给老板看的）":               "写周报（给老板看的）",
		"- 读 List<String> 的源码 <stdio.h>": "读 List<String> 的源码 <stdio.h>",
		"- [ ] 订票（优先级 特高）":               "订票（优先级 特高）",
		"- 不是任务（进行中）":                    "不是任务（进行中）",
	} {
		if n := ParseOutline(line)[0]; n.Title != title || strings.Contains(line, "[ ]") && n.Task.Priority != PriorityNone {
			t.Fatalf("%q read as %q", line, n.Title)
		}
	}
	f := &File{Sheets: []*Document{New("一"), New("二")}}
	if got := MarkdownFile(f); got != "# 一\n\n---\n\n# 二\n" {
		t.Fatalf("file %q", got)
	}
}
