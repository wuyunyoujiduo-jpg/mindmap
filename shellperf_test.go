package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// 108 to 113 of docs/审查清单.md: what a frame of 今天, the calendars and
// 项目 worked out anew each time is worked out once, and again when what
// it is of changed. The tests count the work, not its time.

// busyFolder is an app on a folder as the audit measured: 41 maps and
// 200 tasks, on the days round today, some at a time of their day, some
// that repeat, some done.
func busyFolder(t *testing.T) (*app, *ui.Tester) {
	t.Helper()
	a, tt := newTestApp(t)
	for m := range 40 {
		f := model.NewFile(fmt.Sprintf("项目 %02d", m))
		root := f.Sheets[0].Root
		for b := range 4 {
			branch := model.NewNode(fmt.Sprintf("部分 %d", b))
			for k := range 3 {
				branch.Children = append(branch.Children, model.NewNode(fmt.Sprintf("主题 %d-%d", b, k)))
			}
			root.Children = append(root.Children, branch)
		}
		for k := range 5 {
			n := model.NewNode(fmt.Sprintf("任务 %d-%d", m, k))
			n.SetStatus(model.StatusTodo, testNow)
			due := today(testNow).AddDate(0, 0, (m+k)%9-2)
			n.Task.Due = &due
			switch k {
			case 1:
				n.Task.Time, n.Task.EndTime = fmt.Sprintf("%02d:00", 8+m%10), fmt.Sprintf("%02d:30", 9+m%10)
			case 2:
				n.Task.Repeat = &model.Repeat{Every: 1, Unit: model.RepeatDay}
			case 3:
				n.SetStatus(model.StatusDone, testNow)
			}
			root.Children[k%4].Children = append(root.Children[k%4].Children, n)
		}
		if err := f.Save(filepath.Join(a.dir, model.FileName(fmt.Sprintf("项目 %02d", m)))); err != nil {
			t.Fatal(err)
		}
	}
	a.refreshFiles()
	if len(a.files) != 41 {
		t.Fatalf("%d maps, want 41", len(a.files))
	}
	if n := len(a.taskItems()); n < 200 {
		t.Fatalf("%d tasks, want 200 at least", n)
	}
	return a, tt
}

// shellPage is a page the audit measured, and how it is shown.
type shellPage struct {
	name string
	show func()
}

func shellPages(a *app) []shellPage {
	return []shellPage{
		{"今天", func() { a.openTasks(); a.tasks.view = tasksToday }},
		{"全部", func() { a.openTasks(); a.tasks.view = tasksAll }},
		{"周", func() { a.openTasks(); a.tasks.view = tasksWeek }},
		{"月", func() { a.openTasks(); a.tasks.view = tasksMonth }},
		{"项目", func() { a.showProjects() }},
	}
}

// clockOf gives the app a clock the test moves: the tick looks at the
// files every two seconds of it.
func clockOf(a *app) *time.Time {
	now := testNow
	a.now = func() time.Time { return now }
	return &now
}

// work is what a frame may work out: counted before and after.
type work struct{ looks, gathered, waited, found, faced, laid, words, sorted int }

func workOf(a *app) work {
	lunarWords.Lock()
	defer lunarWords.Unlock()
	return work{fileLooks, a.tasks.serial, a.memo.waited, a.memo.finds, a.memo.faced, a.memo.laid, lunarWords.made, a.memo.sorted}
}

// A frame of a page, with nothing changed, looks at no file and works
// nothing out again: with 41 maps and 200 tasks a frame of 今天 looked at
// 166 files (a map's for each dot of its color), the month at 99, 项目
// at 121, and each laid the calendars and the cards' faces out anew. The
// pointer moving over the hours is a frame each.
func TestShellFramesWorkNothingOut(t *testing.T) {
	a, tt := busyFolder(t)
	now := clockOf(a)
	for _, p := range shellPages(a) {
		p.show()
		*now = now.Add(2 * time.Second) // a tick: every map's file is looked at, once
		before := fileLooks
		tt.Frame()
		tt.Frame()
		if n := fileLooks - before; n > len(a.files)+8 {
			t.Errorf("%s: the tick's frame looked at %d files, of %d maps", p.name, n, len(a.files))
		}
		*now = now.Add(time.Second) // between two ticks
		tt.Frame()
		was := workOf(a)
		start := time.Now()
		const frames = 20
		for i := range frames {
			tt.Move(400+float32(i*20), 300+float32(i*10)) // the pointer over the page
			tt.Frame()
		}
		if got := workOf(a); got != was {
			t.Errorf("%s: %d frames worked out %+v more than %+v", p.name, frames, got, was)
		}
		t.Logf("%-6s %6.2f ms a frame", p.name, float64(time.Since(start).Microseconds())/1000/frames)
	}
	// 项目 with no map at all touches no file either (it deleted a file
	// that never was, at every frame).
	b, tb := newTestApp(t)
	for _, f := range b.files {
		_ = os.Remove(f.Path)
	}
	b.path, b.file, b.doc = "", nil, nil
	b.refreshFiles()
	b.showProjects()
	tb.Frame()
	tb.Frame()
	if !tb.HasText("还没有导图") {
		t.Fatalf("项目, empty: %q", tb.Texts())
	}
}

// The tasks stand while no map changes, however long: they were gathered
// again every second of drawing, every map's file looked at and every
// topic walked. A minute of frames gathers nothing, and looks at each
// map's file once a tick.
func TestTasksGatheredWhenAMapChanges(t *testing.T) {
	a, tt := busyFolder(t)
	now := clockOf(a)
	a.openTasks()
	a.tasks.view = tasksAll
	tt.Frame()
	tt.Frame()
	was, looks := a.tasks.serial, fileLooks
	for range 60 {
		*now = now.Add(time.Second)
		tt.Frame()
	}
	if a.tasks.serial != was {
		t.Fatalf("gathered %d times in a minute with nothing changed", a.tasks.serial-was)
	}
	if n, most := fileLooks-looks, 30*(len(a.files)+8); n > most {
		t.Fatalf("%d looks at files in a minute, %d at most", n, most)
	}
	titles := func() map[string]bool {
		m := map[string]bool{}
		for _, it := range a.taskItems() {
			m[it.node.Title] = true
		}
		return m
	}
	// An agent writes a map's file: by the next tick the task is there, the
	// map's color is its new one, and its card's face is of the map as
	// written.
	path := filepath.Join(a.dir, model.FileName("项目 07"))
	face := a.mapFaceOf(path)
	write := func(title, color string) {
		t.Helper()
		f, err := model.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		n := model.NewNode(title)
		n.SetStatus(model.StatusTodo, *now)
		f.Sheets[0].Root.Children = append(f.Sheets[0].Root.Children, n)
		f.Color = color
		if err := f.Save(path); err != nil {
			t.Fatal(err)
		}
		at := now.Add(time.Hour) // another time than it had, whatever the disk's clock counts in
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}
	write("别处写的", "#3B8FE0")
	tt.Frame()
	if titles()["别处写的"] {
		t.Fatal("seen before the tick: the file was looked at in a frame")
	}
	*now = now.Add(2 * time.Second)
	tt.Frame()
	tt.Frame()
	if !titles()["别处写的"] {
		t.Fatal("a map written elsewhere does not show by the next tick")
	}
	if _, ok := tt.Find("任务 别处写的"); !ok {
		t.Fatalf("it is not on the page: %q", tt.Texts())
	}
	if a.mapColor(path) != ui.Hex("#3B8FE0") {
		t.Fatal("its color is the old one")
	}
	if f := a.mapFaceOf(path); f == face || f.topics != face.topics+1 {
		t.Fatalf("its face is the old one: %d topics, had %d", f.topics, face.topics)
	}
	// The sync wrote maps and scanned the folder: at once, no tick waited for.
	write("同步来的", "#3DAA7D")
	a.refreshFiles()
	if !titles()["同步来的"] || a.mapColor(path) != ui.Hex("#3DAA7D") {
		t.Fatal("what the sync brought does not show at once")
	}
	// An edit of the open map, and one of another map from 今天: at once.
	a.mutate("", func() {
		n := model.NewNode("这里写的")
		n.SetStatus(model.StatusTodo, *now)
		a.doc.Root.Children = append(a.doc.Root.Children, n)
	})
	if !titles()["这里写的"] {
		t.Fatal("an edit of the open map does not show at once")
	}
	for _, it := range a.taskItems() {
		if it.node.Title == "同步来的" {
			a.editTask(it, "", func(n *model.Node) { n.Title = "改过的" })
		}
	}
	if !titles()["改过的"] || titles()["同步来的"] {
		t.Fatal("a task changed from 今天 does not show at once")
	}
	// A map deleted elsewhere leaves by the next tick.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(2 * time.Second)
	tt.Frame()
	if titles()["改过的"] {
		t.Fatal("the tasks of a map deleted elsewhere stay")
	}
}

// 今天 works out what waits, and today's tasks, once: its head, its
// band and its left column asked four or five times a frame, each a look
// through every map's links.
func TestTodayWorkedOutOnce(t *testing.T) {
	a, tt := busyFolder(t)
	now := clockOf(a)
	day := today(*now)
	a.addTask("收件箱里的", &day)
	cl, err := a.addClipText("等着归位的一条", "手动")
	if err != nil {
		t.Fatal(err)
	}
	a.markToPlace(cl)
	a.openTasks()
	tt.Frame()
	tt.Frame()
	*now = now.Add(time.Second)
	tt.Frame()
	was := a.memo.waited
	for range 20 {
		tt.Frame()
	}
	if a.memo.waited != was {
		t.Fatalf("what waits was worked out %d times in 20 frames", a.memo.waited-was)
	}
	if n := len(a.waiting()); n != 2 {
		t.Fatalf("%d wait, want the task and the clip", n)
	}
	// What changes it is seen at once: a clip no longer to place, a task
	// ticked off, a task written.
	a.unkeep(cl) // waits still
	cl.ToPlace = false
	a.saveClip(cl)
	if n := len(a.waiting()); n != 1 {
		t.Fatalf("a clip left in 收集 still waits: %d", n)
	}
	due := len(a.dueToday(*now))
	a.addTask("又一件", &day)
	if n := len(a.waiting()); n != 2 || len(a.dueToday(*now)) != due+1 {
		t.Fatalf("a task written does not wait or is not today's: %d wait, %d due of %d", n, len(a.dueToday(*now)), due+1)
	}
	for _, it := range a.taskItems() {
		if it.node.Title == "又一件" {
			a.setTaskDone(it, true)
		}
	}
	if n, done := len(a.waiting()), len(a.doneToday(*now)); n != 1 || done == 0 || len(a.dueToday(*now)) != due {
		t.Fatalf("a task ticked off: %d wait, %d done today, %d due of %d", n, done, len(a.dueToday(*now)), due)
	}
	// The day turned: today's tasks are another day's.
	*now = now.AddDate(0, 0, 3)
	if len(a.dueToday(*now)) == due {
		t.Fatal("today's tasks did not follow the day")
	}
}

// The bar with words in it, and the search of everything open, look once
// for what is written: each frame built some seventy commands, walked the
// open map and searched 收集.
func TestBarAndPaletteSearchOnce(t *testing.T) {
	a, tt := busyFolder(t)
	now := clockOf(a)
	if _, err := a.addClipText("部分的说明：一条资料", "手动"); err != nil {
		t.Fatal(err)
	}
	a.goToday()
	a.barOpen, a.homeDraft = true, "部分"
	tt.Frame()
	tt.Frame()
	*now = now.Add(time.Second)
	tt.Frame()
	if !tt.HasText("已经有的") {
		t.Fatalf("the bar found nothing: %q", tt.Texts())
	}
	was, looks := a.memo.finds, fileLooks
	for range 20 {
		tt.Frame()
	}
	if a.memo.finds != was || fileLooks != looks {
		t.Fatalf("the bar searched %d times and looked at %d files in 20 frames", a.memo.finds-was, fileLooks-looks)
	}
	// Other words, a clip kept, a map edited: looked for again, once each.
	hits := func() int { return len(a.barHits(a.homeDraft, 100)) }
	a.homeDraft = "一条资料"
	if hits() != 1 || a.memo.finds != was+1 {
		t.Fatalf("other words: %d found, looked for %d times", hits(), a.memo.finds-was)
	}
	a.homeDraft = "独一无二的词"
	if hits() != 0 {
		t.Fatal("found what is nowhere")
	}
	if _, err := a.addClipText("独一无二的词 在收集里", "手动"); err != nil {
		t.Fatal(err)
	}
	if hits() != 1 {
		t.Fatalf("a clip kept is not found: %d", hits())
	}
	a.mutate("", func() {
		a.doc.Root.Children = append(a.doc.Root.Children, model.NewNode("独一无二的词 在导图里"))
	})
	if hits() != 2 {
		t.Fatalf("a topic written is not found: %d", hits())
	}
	// The palette open with the bar's words kept: both stand, neither
	// taking the other's place.
	a.barOpen = false
	a.openPalette(paletteAll)
	a.paletteQuery = "任务 3"
	tt.Frame()
	tt.Frame()
	was = a.memo.finds
	for range 20 {
		tt.Frame()
	}
	if a.memo.finds != was {
		t.Fatalf("the palette searched %d times in 20 frames", a.memo.finds-was)
	}
	// What a command says of the map follows what is chosen in it.
	a.paletteOpen = false
	a.libOpen = false
	a.selected = ""
	board := func() bool {
		a.paletteQuery, a.paletteMode = "在看板中显示", paletteAll
		return len(a.paletteItems()) > 0
	}
	if board() {
		t.Fatal("a command of the task chosen, with none chosen")
	}
	for _, tn := range a.file.Tasks() {
		a.selected = tn.Node.ID
		break
	}
	if !board() {
		t.Fatal("the commands did not follow what is chosen")
	}
}

// The small ones of 113: the maps opened last are put in order once; the
// holidays' files are read once, not at every coming back to the app; a
// day of the Chinese calendar is found without counting from 1900.
func TestSmallThingsOnce(t *testing.T) {
	a, _ := busyFolder(t)
	a.recentFiles()
	was := a.memo.sorted
	for range 10 {
		files := a.recentFiles()
		files[0], files[1] = files[1], files[0] // a caller may sort what it is given
	}
	if a.memo.sorted != was {
		t.Fatalf("sorted %d times", a.memo.sorted-was)
	}
	first := a.recentFiles()[0].Path
	last := a.files[len(a.files)-1].Path
	if last == first {
		last = a.files[0].Path
	}
	a.remember(last)
	if got := a.recentFiles()[0].Path; got != last {
		t.Fatalf("a map opened is not first: %q", got)
	}
	// The holidays.
	defer clearHolidays()
	a.pluginGet = func(context.Context, string) ([]byte, error) { return nil, fmt.Errorf("404") }
	a.holidaysFetch()
	loads := holidaysLoads()
	for range 10 {
		a.holidaysFetch()
	}
	if holidaysLoads() != loads || loads <= 0 {
		t.Fatalf("the notices were read %d times more, of %d", holidaysLoads()-loads, loads)
	}
	// The Chinese calendar: every day of the table and a year past each of
	// its ends, as counting from 1900 a year at a time gives it.
	walked := func(d time.Time) (lunarDate, bool) {
		off := int(d.Sub(time.Date(1900, 1, 31, 0, 0, 0, 0, time.UTC)).Hours() / 24)
		if off < 0 {
			return lunarDate{}, false
		}
		y := 1900
		for ; y <= 2100 && off >= lunarYearDays(y); y++ {
			off -= lunarYearDays(y)
		}
		if y > 2100 {
			return lunarDate{}, false
		}
		for m := 1; m <= 12; m++ {
			n := lunarMonthDays(y, m)
			if off < n {
				return lunarDate{month: m, day: off + 1, days: n}, true
			}
			off -= n
			if m == lunarLeap(y) {
				if n = lunarLeapDays(y); off < n {
					return lunarDate{month: m, day: off + 1, days: n, leap: true}, true
				}
				off -= n
			}
		}
		return lunarDate{}, false
	}
	for d := time.Date(1899, 1, 1, 0, 0, 0, 0, time.UTC); d.Year() < 2103; d = d.AddDate(0, 0, 1) {
		got, ok := lunarOf(d)
		if want, wok := walked(d); got != want || ok != wok {
			t.Fatalf("%s is %+v %v, want %+v %v", d.Format("2006-01-02"), got, ok, want, wok)
		}
	}
	for day, want := range map[string]string{"2026-02-17": "春节", "2026-09-25": "中秋", "2026-10-10": "九月", "2025-07-25": "闰六月"} {
		d, _ := time.ParseInLocation("2006-01-02", day, time.Local)
		if got, _ := lunarLabel(d); got != want {
			t.Errorf("%s is %q, want %q", day, got, want)
		}
	}
}

// The timeline's tasks by the day each begins on: as the sort it was, and
// not a span worked out twice at every comparison.
func TestSortTasksByStart(t *testing.T) {
	var tasks []model.TaskNode
	day := today(testNow)
	for i := range 300 {
		n := model.NewNode(fmt.Sprint(i))
		due := day.AddDate(0, 0, (i*7)%13)
		n.Task = &model.Task{Due: &due}
		if i%5 == 0 {
			start := due.AddDate(0, 0, -2)
			n.Task.Start = &start
		}
		tasks = append(tasks, model.TaskNode{Node: n})
	}
	sortTasksByStart(tasks)
	for i := 1; i < len(tasks); i++ {
		x, _, _ := span(tasks[i-1].Node.Task)
		y, _, _ := span(tasks[i].Node.Task)
		xi, yi := 0, 0
		fmt.Sscan(tasks[i-1].Node.Title, &xi)
		fmt.Sscan(tasks[i].Node.Title, &yi)
		if y.Before(x) || y.Equal(x) && yi < xi {
			t.Fatalf("%d before %d: %v, %v", xi, yi, x, y)
		}
	}
}

// BenchmarkSortTasksByStart: 2000 tasks the last first, as a map that
// dates its tasks backwards gives them.
func BenchmarkSortTasksByStart(b *testing.B) {
	day := today(testNow)
	var tasks []model.TaskNode
	for i := range 2000 {
		n := model.NewNode(fmt.Sprint(i))
		due := day.AddDate(0, 0, 2000-i)
		n.Task = &model.Task{Due: &due}
		tasks = append(tasks, model.TaskNode{Node: n})
	}
	for b.Loop() {
		sortTasksByStart(append([]model.TaskNode(nil), tasks...))
	}
}

// BenchmarkLunarOf: the forty-two days of a month's page.
func BenchmarkLunarOf(b *testing.B) {
	first := time.Date(2026, 10, 26, 0, 0, 0, 0, time.Local)
	for b.Loop() {
		for i := range 42 {
			lunarOf(first.AddDate(0, 0, i))
		}
	}
}
