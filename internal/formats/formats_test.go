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
