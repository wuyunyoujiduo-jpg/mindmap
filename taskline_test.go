package main

import (
	"math/rand"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// A line that ends in a month and a day with no 日 (发布 11月2, which 发布
// 11月20日 is on its way) took the frame down: the day's pattern ends after
// the spaces behind it, and what follows was looked for past the line.
func TestMonthDayAtLineEnd(t *testing.T) {
	now := testNow
	for in, want := range map[string][2]string{
		"发布 11月2":    {"发布", "2026-11-02"},
		"发布 11月20":   {"发布", "2026-11-20"},
		"11月20 发布":   {"发布", "2026-11-20"},
		"11月20日发布":   {"发布", "2026-11-20"},
		"发布 11月20日":  {"发布", "2026-11-20"},
		"11月":        {"11月", ""},
		"3月5":        {"", "2027-03-05"},
		"发布 11月2x":   {"发布 11月2x", ""},
		"11月200 件库存": {"11月200 件库存", ""},
	} {
		tl := readTaskLine(in, now, nil, "", "")
		due := ""
		if tl.due != nil {
			due = tl.due.Format("2006-01-02")
		}
		if tl.title != want[0] || due != want[1] {
			t.Errorf("%q → %q on %q, want %q on %q", in, tl.title, due, want[0], want[1])
		}
		taskLike(in, now)
	}
	// As it is typed into 添加任务, a letter at a time, and kept.
	a, tt := newTestApp(t)
	a.openTasks()
	a.tasks.view = tasksAll
	tt.Frame()
	if err := tt.Click("添加任务"); err != nil {
		t.Fatal(err)
	}
	for _, r := range "发布 11月2" {
		tt.Type(string(r))
		tt.Frame()
	}
	if a.tasks.add != "发布 11月2" || !tt.HasText("11月2日") {
		t.Fatalf("the line %q says %q", a.tasks.add, tt.Texts())
	}
	// The bar reads it too, on every page.
	a.barOpen, a.homeDraft = true, "发布 11月2"
	tt.Frame()
	tt.Frame()
	a.barOpen, a.homeDraft = false, ""
	tt.Frame()
	// Written on to its end and kept: the 20th.
	if err := tt.Click("添加任务"); err != nil {
		t.Fatal(err)
	}
	tt.Type("0日")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if due := taskNamed(t, a, "发布").node.Task.Due; due == nil || due.Format("01-02") != "11-20" {
		t.Fatalf("kept on %v", due)
	}
}

// How a task repeats needs no space after it; it is none only where the
// next character goes on with it into other words.
func TestRepeatWordsNeedNoSpace(t *testing.T) {
	now := testNow // Saturday the 10th of October 2026
	day := today(now)
	on := func(n int) string { return day.AddDate(0, 0, n).Format("2006-01-02") }
	for _, c := range []struct{ in, title, repeat, due, from string }{
		{"每天喝水", "喝水", "每天", on(0), ""},
		{"每周五写周报", "写周报", "每周五", on(6), ""},
		{"每月15号交房租", "交房租", "每月 15 日", on(5), ""},
		{"工作日打卡", "打卡", "工作日", on(2), ""},
		{"法定工作日打卡", "打卡", "法定工作日", on(2), ""}, // no notice read: as the week is
		{"每年体检", "体检", "每年", on(0), ""},
		{"每2周大扫除", "大扫除", "每 2 周", on(0), ""},
		{"每三天浇花", "浇花", "每 3 天", on(0), ""},
		{"每月最后一天对账", "对账", "每月 最后一天", "2026-10-31", ""},
		{"每天8点跑步", "跑步", "每天", on(0), "08:00"},
		{"工作日9点 站会", "站会", "工作日", on(2), "09:00"},
		{"每天早上8点跑步", "跑步", "每天", on(0), "08:00"},
		{"每周五回家", "回家", "每周五", on(6), ""},
		{"每周写周报", "写周报", "每周", on(0), ""},
		// The spaces that were needed still read.
		{"每天 吃药", "吃药", "每天", on(0), ""},
		{"每周五 写周报", "写周报", "每周五", on(6), ""},
		// Midnight is the day after's, and how it repeats says the day: the
		// time stays words, and the repeat stands.
		{"每天晚上12点 睡觉", "晚上12点 睡觉", "每天", on(0), ""},
		{"晚上12点 每天 睡觉", "晚上12点 睡觉", "每天", on(0), ""},
		{"每周五 晚上12点 关灯", "晚上12点 关灯", "每周五", on(6), ""},
		// Other words that begin as a repeat does.
		{"工作日报 已提交", "工作日报 已提交", "", "", ""},
		{"每天都很累 想休息", "每天都很累 想休息", "", "", ""},
		{"每周一次 复盘", "每周一次 复盘", "", "", ""},
		{"工作日志整理一下", "工作日志整理一下", "", "", ""},
		{"每天都要喝水", "每天都要喝水", "", "", ""},
		{"每周一次的例会", "每周一次的例会", "", "", ""},
		{"每周五都要开会", "每周五都要开会", "", "", ""},
		{"每月两次 对账", "每月两次 对账", "", "", ""},
		{"每周末 大扫除", "每周末 大扫除", "", "", ""},
		{"每年的今天", "每年的", "", on(0), ""},
	} {
		tl := readTaskLine(c.in, now, nil, "", "")
		repeat, due := "", ""
		if tl.repeat != nil {
			repeat = tl.repeat.Label()
		}
		if tl.due != nil {
			due = tl.due.Format("2006-01-02")
		}
		if tl.title != c.title || repeat != c.repeat || due != c.due || tl.from != c.from {
			t.Errorf("%q → %q, %q on %q at %q; want %q, %q on %q at %q", c.in, tl.title, repeat, due, tl.from, c.title, c.repeat, c.due, c.from)
		}
		// A line is a task's by how it repeats where it begins with that.
		if got, want := taskLike(c.in, now), c.repeat != ""; got != want && strings.HasPrefix(c.in, "每") && c.in != "每年的今天" || got && !want {
			t.Errorf("%q reads as a task: %v", c.in, got)
		}
	}
	// The line to add a task says what it read, as it is written.
	a, tt := newTestApp(t)
	a.openTasks()
	a.tasks.view = tasksAll
	a.tasks.add = "每天8点跑步"
	tt.Frame()
	tt.Frame()
	for _, want := range []string{"今天 8:00", "每天"} {
		if !tt.HasText(want) {
			t.Fatalf("the line does not say %q: %q", want, tt.Texts())
		}
	}
	a.tasks.add = "每天都很累 想休息"
	tt.Frame()
	if tt.HasText("每天") {
		t.Fatalf("a sentence shown as a repeat: %q", tt.Texts())
	}
}

// A time of day followed by 前, 后, 左右 and their like, or by what it
// counts points of, is no time of the task: at the line's head it was.
func TestClockThatIsNone(t *testing.T) {
	now := testNow
	day := today(now)
	for in, want := range map[string][2]string{
		"2点前交报告":     {"2点前交报告", ""},
		"10点前回邮件":    {"10点前回邮件", ""},
		"3点建议":       {"3点建议", ""},
		"3点左右 出发":    {"3点左右 出发", ""},
		"明天 15:30前交": {"15:30前交", ""},
		"9点多 再看":     {"9点多 再看", ""},
		"9点开会":       {"开会", "09:00"},
		"9点 开会":      {"开会", "09:00"},
		"15:30 交":    {"交", "15:30"},
		"下午3点到5点 写":  {"写", "15:00"},
	} {
		tl := readTaskLine(in, now, &day, "", "")
		if tl.title != want[0] || tl.from != want[1] {
			t.Errorf("%q → %q at %q, want %q at %q", in, tl.title, tl.from, want[0], want[1])
		}
	}
	if _, from, _ := parseClock("2点前"); from != "" {
		t.Errorf("2点前 of a task's details: %q", from)
	}
}

// lineWords are what the lines' reader knows, and what stands between
// them: the lines made of them at random are to be read, whatever they say.
var lineWords = []string{
	"每天", "每周", "每周五", "每月", "每月15号", "每月最后一天", "每年", "每2周", "每三天", "每隔", "每", "工作日", "法定工作日", "每个工作日",
	"今天", "明天", "后天", "大后天", "今晚", "周五", "下周一", "星期天", "上周五", "11月2", "11月20日", "11月", "月", "2月30日", "13月1日", "11/20", "20号", "31号", "日", "号",
	"9点", "9点半", "晚上12点", "下午3点", "中午12点", "15:30", "9：05", "25:61", "到", "-", "~", "—", "点", "时", "分", "半", "上午", "晚上",
	"前", "后", "左右", "多", "钟", "建议", "都", "的", "报", "志", "一次", "两次", "回", "末",
	"!高", "！低", "!3", "!", "[[", "]]", "<clip:ab12>", "<mindmap:a#b>", "- [ ]", "\n", "#标签", "?", "？",
	" ", "  ", "\t", "　", "写周报", "开会", "发布", "x", "1", "2", "12", "0", "99", ".", "/", "é", "\xff", "😀",
}

// TestTaskLinesNeverPanic reads a few hundred thousand lines made of the
// reader's own words at random: none may take the app down, as one that
// ended in 11月2 did.
func TestTaskLinesNeverPanic(t *testing.T) {
	lines := 300000
	if testing.Short() {
		lines = 20000
	}
	rnd := rand.New(rand.NewSource(20261117))
	now := testNow
	day := today(now)
	var line string
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("%q: %v", line, r)
		}
	}()
	var b strings.Builder
	for i := range lines {
		b.Reset()
		for range 1 + rnd.Intn(6) {
			b.WriteString(lineWords[rnd.Intn(len(lineWords))])
		}
		line = b.String()
		var where *time.Time
		if i%2 == 0 {
			where = &day
		}
		tl := readTaskLine(line, now.AddDate(0, 0, i%400), where, "", "")
		tl.when(now)
		taskLike(line, now)
		checklist(line)
		parseClock(line)
	}
}

// FuzzReadTaskLine is the same for go test -fuzz.
func FuzzReadTaskLine(f *testing.F) {
	for _, s := range []string{"发布 11月2", "明天9点到10点半 开会 !高", "每天晚上12点 睡觉", "工作日报 已提交", "2点前交报告", "- [ ] 买菜\n- [ ] 做饭", "每月15号交房租"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, line string) {
		tl := readTaskLine(line, testNow, nil, "", "")
		tl.when(testNow)
		taskLike(line, testNow)
		parseClock(line)
	})
}

// The day is said when something is due, not marked as said at its first
// look: what the sync brings for today later is said then, once. What was
// written or dated here today is no cause to say it.
func TestDayIsSaidWhenSomethingComes(t *testing.T) {
	a, tt := newTestApp(t)
	fp := a.plat.(*fakePlatform)
	day := today(testNow)
	at := func(h, m int) {
		a.now = func() time.Time { return day.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute) }
		tt.Frame()
	}
	at(9, 0) // the day's first look: nothing is due
	a.addTask("自己写的", &day)
	at(9, 5)
	// Dated here on the canvas: the open map's own.
	a.mutate("", func() { find(t, a, "搜索").Task.Due = &day })
	at(9, 10)
	if len(fp.notes) != 0 {
		t.Fatalf("said for what was written or dated here: %q", fp.notes)
	}
	// Another computer's task for today, brought by the sync.
	f := model.NewFile("别处的项目")
	n := model.NewNode("同步来的")
	n.SetStatus(model.StatusTodo, a.now())
	n.Task.Due = &day
	f.Sheets[0].Root.Children = append(f.Sheets[0].Root.Children, n)
	if err := f.Save(filepath.Join(a.dir, model.FileName("别处的项目"))); err != nil {
		t.Fatal(err)
	}
	a.refreshFiles() // as the sync does once it wrote
	at(9, 15)
	if len(fp.notes) != 1 || !strings.Contains(fp.notes[0], "同步来的") || !strings.Contains(fp.notes[0], "自己写的") {
		t.Fatalf("what came for today was not said, with the day: %q", fp.notes)
	}
	// Once: another that comes says nothing more.
	n2 := model.NewNode("又来一件")
	n2.SetStatus(model.StatusTodo, a.now())
	n2.Task.Due = &day
	f.Sheets[0].Root.Children = append(f.Sheets[0].Root.Children, n2)
	if err := f.Save(filepath.Join(a.dir, model.FileName("别处的项目"))); err != nil {
		t.Fatal(err)
	}
	a.refreshFiles()
	at(9, 20)
	if len(fp.notes) != 1 {
		t.Fatalf("the day said twice: %q", fp.notes)
	}
}
