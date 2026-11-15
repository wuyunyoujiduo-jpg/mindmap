package formats

import (
	"bytes"
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

func TestOPMLRoundTrip(t *testing.T) {
	f := model.NewFile("项目")
	d := f.Sheets[0]
	a, _ := d.AddChild(d.Root.ID, "功能 & <登录>")
	a.Note = "备注\n第二行"
	b, _ := d.AddChild(a.ID, "完成的")
	b.Task = &model.Task{Status: model.StatusDone}
	data, err := OPML(f, "项目")
	if err != nil || !strings.Contains(string(data), `text="功能 &amp; &lt;登录&gt;"`) {
		t.Fatalf("%v %s", err, data)
	}
	g, err := ParseOPML(data, "x")
	if err != nil {
		t.Fatal(err)
	}
	ga := g.Sheets[0].Root.Children[0]
	if g.Sheets[0].Root.Title != "项目" || ga.Title != "功能 & <登录>" || ga.Note != "备注\n第二行" || ga.Children[0].Task.Status != model.StatusDone {
		t.Fatalf("%+v", ga)
	}
	// Several top outlines go under a root.
	g, _ = ParseOPML([]byte(`<opml version="2.0"><body><outline text="甲"/><outline text="乙"><outline text="丙"/></outline></body></opml>`), "导入")
	if r := g.Sheets[0].Root; r.Title != "导入" || len(r.Children) != 2 || r.Children[1].Children[0].Title != "丙" {
		t.Fatalf("%+v", r)
	}
}

func TestMarkdownImport(t *testing.T) {
	f := ParseMarkdown("# 计划\n## 第一周\n- [ ] 设计\n- [x] 调研\n## 第二周\n- 开发", "x")
	r := f.Sheets[0].Root
	if r.Title != "计划" || len(r.Children) != 2 || len(r.Children[0].Children) != 2 || r.Children[0].Children[1].Task.Status != model.StatusDone {
		t.Fatalf("%+v", r)
	}
	f = ParseMarkdown("- 一\n- 二", "笔记")
	if f.Sheets[0].Root.Title != "笔记" || len(f.Sheets[0].Root.Children) != 2 {
		t.Fatal("no root")
	}
}

func TestICS(t *testing.T) {
	f := model.NewFile("项目")
	d := f.Sheets[0]
	n, _ := d.AddChild(d.Root.ID, "发布, 第一版; 很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长的标题")
	due := time.Date(2026, 10, 20, 0, 0, 0, 0, time.Local)
	n.Task = &model.Task{Status: model.StatusDone, Due: &due}
	d.AddChild(d.Root.ID, "没有日期")
	out := string(ICS(f, "项目", time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)))
	for _, want := range []string{"BEGIN:VEVENT", "DTSTART;VALUE=DATE:20261020", "DTEND;VALUE=DATE:20261021", `SUMMARY:✓ 发布\, 第一版\; `, "UID:" + n.ID} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Count(out, "BEGIN:VEVENT") != 1 {
		t.Error("undated task in")
	}
	for _, l := range strings.Split(out, "\r\n") {
		if len(l) > 75 {
			t.Errorf("line of %d bytes", len(l))
		}
	}
}

func TestPDF(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 160, 90))
	for x := 0; x < 160; x++ {
		img.Set(x, 45, color.RGBA{255, 0, 0, 255})
	}
	var b bytes.Buffer
	if err := PDF(&b, []image.Image{img, img}, 800); err != nil {
		t.Fatal(err)
	}
	s := b.String()
	if !strings.HasPrefix(s, "%PDF-1.4") || !strings.Contains(s, "/Count 2") || !strings.Contains(s, "/MediaBox [0 0 800.00 450.00]") ||
		!strings.HasSuffix(s, "%%EOF\n") {
		t.Fatalf("pdf head %q", s[:200])
	}
	// The cross-reference table points at each object.
	xref := strings.LastIndex(s, "xref\n")
	for i := 1; i <= 8; i++ {
		if !strings.Contains(s[:xref], "\n"+itoa(i)+" 0 obj\n") && !strings.HasPrefix(s[strings.Index(s, "\n")+1:], itoa(i)+" 0 obj") {
			t.Errorf("no object %d", i)
		}
	}
}

func itoa(i int) string { return string(rune('0' + i)) }

// A document, not an outline: its prose is notes, its tables' rows topics.
func TestMarkdownDocument(t *testing.T) {
	doc := "<!-- @author someone -->\n# 数据权限规则\n\n本文回答“这个员工能看到哪些记录”。\n见 [功能说明](../features/x.md)。\n\n## 1. 决策表\n\n" +
		"| 编号 | 规则 | 说明 |\n|---|---|---|\n" +
		"| R-PERM-01 | **全局一个开关**，`data_permission` 表（单行），默认关闭 | 关闭时不读授权表 |\n" +
		"| R-PERM-02 | 档案自动过滤，单据按字段标记：员工、商品、客户档案开启后自动按自己的编号过滤 | a \\| b |\n\n" +
		"## 2. 实现\n\n- 第一步\n  接着说\n- 一个很长很长的条目：它把一整段话写成了列表里的一项，放在导图上是读不了的，所以只取开头，全文放进备注里面\n  - 下一级\n\n```sql\nselect 1\n```\n"
	f := ParseMarkdown(doc, "permission")
	r := f.Sheets[0].Root
	if r.Title != "数据权限规则" || !strings.Contains(r.Note, "本文回答") || strings.Contains(r.Note, "author") || len(r.Children) != 2 {
		t.Fatalf("root %q note %q children %d", r.Title, r.Note, len(r.Children))
	}
	table := r.Children[0]
	if table.Title != "1. 决策表" || len(table.Children) != 2 {
		t.Fatalf("table %+v", table)
	}
	if row := table.Children[0]; row.Title != "R-PERM-01 全局一个开关" || !strings.Contains(row.Note, "**说明**：关闭时不读授权表") || !strings.Contains(row.Note, "**规则**：**全局一个开关**") {
		t.Fatalf("row %q: %q", row.Title, row.Note)
	}
	if row := table.Children[1]; row.Title != "R-PERM-02 档案自动过滤" || !strings.Contains(row.Note, "a | b") {
		t.Fatalf("row %q: %q", row.Title, row.Note)
	}
	impl := r.Children[1]
	if len(impl.Children) != 2 || impl.Children[0].Title != "第一步 接着说" || !strings.Contains(impl.Note, "select 1") {
		t.Fatalf("list %+v note %q", impl.Children, impl.Note)
	}
	if long := impl.Children[1]; long.Title != "一个很长很长的条目" || !strings.Contains(long.Note, "全文放进备注里面") || len(long.Children) != 1 {
		t.Fatalf("long item %q: %q", long.Title, long.Note)
	}
}
