package formats

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// A map written out and read back is the map it was. (Before 0.105.2 a
// note came back from Markdown a topic a line, the tags and a task's marks
// as words of the title, and the links and the floating topics not at
// all; from OPML the floating topics, the tags, the links and all of a
// task but that it was done were gone.)

// sampleMap is a map with what an outline can say of one: notes, tags,
// tasks, links, a summary with a topic under it, a floating topic with
// one under it, on two canvases.
func sampleMap() *model.File {
	f := model.NewFile("发布计划")
	d := f.Sheets[0]
	d.Root.Note = "总的说明\n\n第二段，带 - 一个横线和 # 井号"
	d.Root.Tags = []string{"项目"}
	a, _ := d.AddChild(d.Root.ID, "功能")
	a.Note = "- 像列表的一行\n  缩进的一行\n> 引用的一行"
	a.Links = []string{"https://example.com/spec", "clip:abc123", "mindmap:别的 导图#x1y2", "skill:release", "internal/model/tree.go:60"}
	due := time.Date(2026, 10, 13, 0, 0, 0, 0, time.Local)
	start := time.Date(2026, 10, 9, 0, 0, 0, 0, time.Local)
	b, _ := d.AddChild(a.ID, "登录")
	b.Tags = []string{"核心", "安全"}
	b.Task = &model.Task{Status: model.StatusDoing, Priority: model.PriorityHigh, Start: &start, Due: &due}
	c, _ := d.AddChild(a.ID, "搜索（拼音）")
	c.Task = &model.Task{Status: model.StatusDone}
	e, _ := d.AddChild(a.ID, "一个想法")
	e.Task = &model.Task{Status: model.StatusIdea, Priority: model.PriorityLow}
	sum := a.AddSummary("第一期")
	sum.Note = "概要的备注"
	_, _ = d.AddChild(sum.ID, "概要下的一条")
	g, _ := d.AddChild(d.Root.ID, "")
	_, _ = d.AddChild(g.ID, "没有标题的主题下面")
	d.Root.AddSummary("全图的概要")
	fl := d.AddFloating("漂着的想法", model.Point{X: 320, Y: -120.5})
	fl.Tags = []string{"待定"}
	k, _ := d.AddChild(fl.ID, "它下面的任务")
	k.Task = &model.Task{Status: model.StatusTodo, Due: &due}
	d.AddFloating("另一个", model.Point{X: -400, Y: 260})

	second := f.AddSheet(-1)
	second.Root.Title = "第二张画布"
	_, _ = second.AddChild(second.Root.ID, "它的主题")
	second.AddFloating("第二张上漂着的", model.Point{X: 10, Y: 20})
	return f
}

// shape is a file without what differs between a map and its copy read
// back: the IDs, which are new.
func shape(t *testing.T, f *model.File) string {
	t.Helper()
	c := f.Clone()
	for _, d := range c.Sheets {
		d.ID = ""
	}
	c.Walk(func(n *model.Node) { n.ID = "" })
	b, err := json.MarshalIndent(c.Sheets, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestMarkdownRoundTrip(t *testing.T) {
	f := sampleMap()
	text := model.MarkdownFile(f)
	if isDocument(text) {
		t.Fatalf("a map written out reads as a document:\n%s", text)
	}
	g := ParseMarkdown(text, "x")
	// Markdown has no names for the canvases.
	for i, d := range g.Sheets {
		d.Title = f.Sheets[i].Title
	}
	if want, got := shape(t, f), shape(t, g); got != want {
		t.Fatalf("not the map it was.\nwritten:\n%s\nread back:\n%s\nwas:\n%s", text, got, want)
	}
	// And written out again it is the same text.
	if again := model.MarkdownFile(g); again != text {
		t.Fatalf("written again:\n%s\nwas:\n%s", again, text)
	}
	// One canvas copied (复制当前画布为 Markdown) has its floating topics.
	if one := model.Markdown(f.Sheets[0]); !strings.Contains(one, "漂着的想法") || !strings.Contains(one, "它下面的任务") {
		t.Fatalf("no floating topics in\n%s", one)
	}
}

func TestOPMLKeepsTheMap(t *testing.T) {
	f := sampleMap()
	d := f.Sheets[0]
	task := d.Root.Children[0].Children[0].Task
	at := time.Date(2026, 10, 9, 18, 30, 0, 0, time.UTC)
	until := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	task.Time, task.EndTime, task.Remind, task.Times, task.DoneAt = "09:00", "10:30", "08:45", 2, &at
	task.Repeat = &model.Repeat{Unit: model.RepeatWeek, Every: 2, Days: []int{1, 3}, Until: &until}
	data, err := OPML(f, "发布计划")
	if err != nil {
		t.Fatal(err)
	}
	g, err := ParseOPML(data, "x")
	if err != nil {
		t.Fatal(err)
	}
	if want, got := shape(t, f), shape(t, g); got != want {
		t.Fatalf("not the map it was.\nwritten:\n%s\nread back:\n%s\nwas:\n%s", data, got, want)
	}
	// Another outliner's file, which marks a task done and no more.
	g, _ = ParseOPML([]byte(`<opml version="2.0"><body><outline text="甲" _complete="true"/><outline text="乙"/></body></opml>`), "导入")
	if r := g.Sheets[0].Root; len(r.Children) != 2 || r.Children[0].Task == nil || r.Children[0].Task.Status != model.StatusDone || r.Children[1].Task != nil {
		t.Fatalf("%+v", r.Children)
	}
}

func TestICSTimesAndRepeats(t *testing.T) {
	day := time.Date(2026, 10, 20, 0, 0, 0, 0, time.Local) // a Tuesday
	until := time.Date(2026, 12, 31, 0, 0, 0, 0, time.Local)
	f := model.NewFile("项目")
	d := f.Sheets[0]
	add := func(title string, task model.Task) *model.Node {
		n, _ := d.AddChild(d.Root.ID, title)
		task.Due = &day
		n.Task = &task
		return n
	}
	event := func(n *model.Node) string {
		t.Helper()
		out := string(ICS(f, "项目", day))
		i := strings.Index(out, "UID:"+n.ID)
		if i < 0 {
			t.Fatalf("no event of %q", n.Title)
		}
		return out[i : i+strings.Index(out[i:], "END:VEVENT")]
	}
	has := func(n *model.Node, wants ...string) {
		t.Helper()
		ev := event(n)
		for _, w := range wants {
			if not := strings.TrimPrefix(w, "!"); not != w {
				if strings.Contains(ev, not) {
					t.Errorf("%s: has %q in\n%s", n.Title, not, ev)
				}
			} else if !strings.Contains(ev, w+"\r\n") {
				t.Errorf("%s: no %q in\n%s", n.Title, w, ev)
			}
		}
	}
	has(add("开会", model.Task{Time: "09:00", EndTime: "10:30"}), "DTSTART:20261020T090000", "DTEND:20261020T103000", "!VALUE=DATE")
	has(add("电话", model.Task{Time: "14:15"}), "DTSTART:20261020T141500", "DTEND:20261020T151500")
	has(add("全天", model.Task{}), "DTSTART;VALUE=DATE:20261020", "DTEND;VALUE=DATE:20261021", "!RRULE")
	has(add("每天", model.Task{Repeat: &model.Repeat{Unit: model.RepeatDay}}), "RRULE:FREQ=DAILY")
	has(add("隔周一三", model.Task{Time: "09:00", Repeat: &model.Repeat{Unit: model.RepeatWeek, Every: 2, Days: []int{1, 3}, Until: &until}}),
		"RRULE:FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,WE;UNTIL=20261231T235959")
	has(add("每月一号和月底", model.Task{Repeat: &model.Repeat{Unit: model.RepeatMonth, MonthDays: []int{1, -1}, Count: 5}, Times: 2}),
		"RRULE:FREQ=MONTHLY;BYMONTHDAY=1,-1;COUNT=3")
	has(add("每月第二个周一", model.Task{Repeat: &model.Repeat{Unit: model.RepeatMonth, Nth: 2, Days: []int{1}}}), "RRULE:FREQ=MONTHLY;BYDAY=2MO")
	has(add("每月最后一个周二", model.Task{Repeat: &model.Repeat{Unit: model.RepeatMonth, Nth: -1}}), "RRULE:FREQ=MONTHLY;BYDAY=-1TU")
	has(add("每月三十号", model.Task{Repeat: &model.Repeat{Unit: model.RepeatMonth, MonthDays: []int{30}}}), "RRULE:FREQ=MONTHLY;BYMONTHDAY=28,29,30;BYSETPOS=-1")
	has(add("每月二十号", model.Task{Repeat: &model.Repeat{Unit: model.RepeatMonth}}), "RRULE:FREQ=MONTHLY;BYMONTHDAY=20")
	has(add("每年", model.Task{Repeat: &model.Repeat{Unit: model.RepeatYear, Until: &until}}), "RRULE:FREQ=YEARLY;UNTIL=20261231")
	// What a rule cannot say is left out, not said wrongly.
	has(add("做完后三天", model.Task{Repeat: &model.Repeat{Unit: model.RepeatDay, Every: 3, FromDone: true}}), "!RRULE")
	has(add("法定工作日", model.Task{Repeat: &model.Repeat{Unit: model.RepeatWorkday}}), "!RRULE")
	has(add("十五号和三十号", model.Task{Repeat: &model.Repeat{Unit: model.RepeatMonth, MonthDays: []int{15, 30}}}), "!RRULE")
}
