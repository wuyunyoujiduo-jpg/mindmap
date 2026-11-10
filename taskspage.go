package main

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// 任务, as TickTick has it: the tasks of every map in one place, as lists
// (today, the next 7 days, all) and as a calendar (a day, a week, a
// month). A task is ticked off by its circle, dated by dragging it to a
// day or from its menu, and added in a line with its day in words
// ("明天 写周报"), to the inbox.

// Views of the tasks.
const (
	tasksToday = iota
	tasksWeekAhead
	tasksAll
	tasksDay
	tasksWeek
	tasksMonth
	tasksInbox // the tasks written down, not yet put in a map
	tasksDone  // those done, the last first
)

// The lists of the tasks' page, as its tabs show them.
var taskLists = []struct {
	view int
	name string
}{{tasksToday, "今天"}, {tasksWeekAhead, "最近 7 天"}, {tasksInbox, "收件箱"}, {tasksAll, "全部"}, {tasksDone, "已完成"}}

// tasksState is the tasks' page.
type tasksState struct {
	open     bool
	view     int
	day      time.Time // the day the calendar shows, or is in
	add      string    // the task being written
	showDone bool
	items    []taskItem
	read     time.Time // when the items were gathered
	edits    int       // the open map's edits then
	// sel is the task whose details show, as path#id; onMap the map whose
	// tasks show alone, "" for all.
	sel, onMap string
	// The details edited: the title and the note, kept as they are typed
	// and saved as they are left; the subtask being written.
	title, note, draftOf string
	subtask              string
}

func (it taskItem) key() string { return it.path + "#" + it.node.ID }

// taskItem is a task of a map.
type taskItem struct {
	path, mapName, sheet string
	node                 *model.Node // the map's, as read: not edited here
	where                string
}

func (it taskItem) date() *time.Time {
	if t := it.node.Task; t.Due != nil {
		return t.Due
	}
	return it.node.Task.Start
}

func (it taskItem) done() bool { return it.node.Task.Status == model.StatusDone }

// taskDrag is a task dragged to a day.
type taskDrag struct{ path, id string }

// tasksRow is the sidebar's row of the tasks, with how many are due.
func (a *app) tasksRow(c *ui.Context) {
	count := ""
	if n := a.tasksDue(); n > 0 {
		count = fmt.Sprint(n)
	}
	row := navRow(c.Key("tasks-row"), iconCheck, tileRed, "任务", count, a.libOpen && a.tasks.open).
		Tooltip("所有导图里的任务：今天、最近 7 天、日 / 周 / 月视图")
	if row.Clicked() {
		a.later(a.openTasks)
	}
}

// openTasks shows the tasks in the main area.
func (a *app) openTasks() {
	a.showLibrary()
	a.tasks.open = true
	if a.tasks.day.IsZero() {
		a.tasks.day = today(a.now())
	}
	a.tasks.read = time.Time{}
}

// taskItems are the tasks of all the maps, the open one as it is edited,
// gathered again each second or as the open map changes.
func (a *app) taskItems() []taskItem {
	now := a.now()
	if now.Sub(a.tasks.read) < time.Second && a.tasks.edits == a.edits && !a.tasks.read.IsZero() {
		return a.tasks.items
	}
	a.tasks.read, a.tasks.edits = now, a.edits
	var items []taskItem
	for _, fi := range a.files {
		f := a.file
		if fi.Path != a.path {
			f = a.indexed(fi.Path)
		}
		if f == nil {
			continue
		}
		for _, tn := range f.Tasks() {
			items = append(items, taskItem{path: fi.Path, mapName: mapName(fi.Path), sheet: tn.Sheet.ID, node: tn.Node,
				where: strings.Join(tn.Parents, " › ")})
		}
	}
	slices.SortStableFunc(items, func(x, y taskItem) int {
		dx, dy := x.date(), y.date()
		switch {
		case dx == nil && dy == nil:
		case dx == nil:
			return 1
		case dy == nil:
			return -1
		case !dx.Equal(*dy):
			return dx.Compare(*dy)
		}
		return int(y.node.Task.Priority) - int(x.node.Task.Priority)
	})
	a.tasks.items = items
	return items
}

// taskCounts are the tasks due today and those overdue, for the sidebar.
func (a *app) tasksDue() int {
	day := today(a.now())
	n := 0
	for _, it := range a.taskItems() {
		if d := it.date(); d != nil && !it.done() && d.Before(day.AddDate(0, 0, 1)) && it.node.Task.Status != model.StatusIdea {
			n++
		}
	}
	return n
}

// editTask changes a task in its map, saved as an edit of it.
func (a *app) editTask(it taskItem, note string, fn func(n *model.Node)) {
	err := a.editMapAt(it.path, note, func(f *model.File) error {
		n := f.Node(it.node.ID)
		if n == nil {
			return fmt.Errorf("找不到任务「%s」", it.node.Title)
		}
		fn(n)
		return nil
	})
	if err != nil {
		a.tell("没有改成：" + err.Error())
	}
	a.tasks.read = time.Time{}
}

func (a *app) setTaskDone(it taskItem, done bool) {
	st := model.StatusTodo
	if done {
		st = model.StatusDone
	}
	a.editTask(it, "完成任务", func(n *model.Node) { n.SetStatus(st, a.now()) })
}

// setTaskDay dates a task: its due day, or none; a start after it goes.
func (a *app) setTaskDay(it taskItem, day *time.Time) {
	a.editTask(it, "改任务日期", func(n *model.Node) {
		if n.Task == nil {
			n.SetStatus(model.StatusTodo, a.now())
		}
		n.Task.Due = day
		if day != nil && n.Task.Start != nil && n.Task.Start.After(*day) {
			n.Task.Start = nil
		}
	})
}

// addTask adds a task written in a line, with its day in words, to the
// inbox; day, when not nil, is the day it goes on when the line says none.
func (a *app) addTask(line string, day *time.Time) {
	title, due, prio := parseTaskLine(line, a.now())
	if title == "" {
		return
	}
	if due == nil {
		due = day
	}
	n := model.NewNode(title)
	n.SetStatus(model.StatusTodo, a.now())
	n.Task.Due, n.Task.Priority = due, prio
	path := a.inboxPath()
	if err := a.ensureInbox(); err != nil {
		a.tell("没有加上：" + err.Error())
		return
	}
	if err := a.editMapAt(path, "加任务", func(f *model.File) error {
		f.Sheets[0].Root.Children = append(f.Sheets[0].Root.Children, n)
		return nil
	}); err != nil {
		a.tell("没有加上：" + err.Error())
		return
	}
	when := "无日期"
	if due != nil {
		when = dayLabel(*due, a.now())
	}
	a.tell("已加到收件箱：" + title + "（" + when + "）")
	a.tasks.read = time.Time{}
}

var (
	taskMonthDay = regexp.MustCompile(`(\d{1,2})\s*[月/.-]\s*(\d{1,2})\s*[日号]?`)
	taskDayOnly  = regexp.MustCompile(`(^|\s)(\d{1,2})\s*[日号](\s|$)`)
	taskWeekday  = regexp.MustCompile(`(下)?(?:周|星期|礼拜)([一二三四五六日天])`)
	taskPrio     = regexp.MustCompile(`[!！](高|中|低|1|2|3)`)
)

// parseTaskLine reads a task written in a line: its title, and the day
// and priority it says, in words: 今天, 明天, 后天, 大后天, 周五, 下周一,
// 11月20日, 11/20, 20号; !高 !中 !低.
func parseTaskLine(line string, now time.Time) (string, *time.Time, model.Priority) {
	s := " " + strings.TrimSpace(line) + " "
	day := today(now)
	var due *time.Time
	set := func(t time.Time) { due = &t }
	for _, w := range []struct {
		word string
		add  int
	}{{"大后天", 3}, {"后天", 2}, {"明天", 1}, {"今天", 0}, {"今晚", 0}} {
		if strings.Contains(s, w.word) {
			s = strings.Replace(s, w.word, " ", 1)
			set(day.AddDate(0, 0, w.add))
			break
		}
	}
	if due == nil {
		if m := taskWeekday.FindStringSubmatchIndex(s); m != nil {
			want := map[string]int{"一": 1, "二": 2, "三": 3, "四": 4, "五": 5, "六": 6, "日": 7, "天": 7}[s[m[4]:m[5]]]
			wd := int(day.Weekday())
			if wd == 0 {
				wd = 7
			}
			ahead := (want - wd + 7) % 7
			if m[2] >= 0 { // 下周: in the week after this one
				ahead = want - wd + 7
			}
			set(day.AddDate(0, 0, ahead))
			s = s[:m[0]] + " " + s[m[1]:]
		}
	}
	if due == nil {
		if m := taskMonthDay.FindStringSubmatchIndex(s); m != nil {
			mo, _ := strconv.Atoi(s[m[2]:m[3]])
			d, _ := strconv.Atoi(s[m[4]:m[5]])
			if mo >= 1 && mo <= 12 && d >= 1 && d <= 31 {
				t := time.Date(day.Year(), time.Month(mo), d, 0, 0, 0, 0, day.Location())
				if t.Before(day.AddDate(0, -1, 0)) { // long past: next year's
					t = t.AddDate(1, 0, 0)
				}
				set(t)
				s = s[:m[0]] + " " + s[m[1]:]
			}
		}
	}
	if due == nil {
		if m := taskDayOnly.FindStringSubmatchIndex(s); m != nil {
			d, _ := strconv.Atoi(s[m[4]:m[5]])
			if d >= 1 && d <= 31 {
				t := time.Date(day.Year(), day.Month(), d, 0, 0, 0, 0, day.Location())
				if t.Before(day) {
					t = t.AddDate(0, 1, 0)
				}
				set(t)
				s = s[:m[0]] + " " + s[m[1]:]
			}
		}
	}
	prio := model.PriorityNone
	if m := taskPrio.FindStringSubmatchIndex(s); m != nil {
		prio = map[string]model.Priority{"高": model.PriorityHigh, "1": model.PriorityHigh, "中": model.PriorityMedium,
			"2": model.PriorityMedium, "低": model.PriorityLow, "3": model.PriorityLow}[s[m[2]:m[3]]]
		s = s[:m[0]] + " " + s[m[1]:]
	}
	return strings.Join(strings.Fields(s), " "), due, prio
}

// dayLabel names a day near now: 今天, 明天, 周三, 11月20日.
func dayLabel(d, now time.Time) string {
	day := today(now)
	switch n := int(d.Sub(day).Hours() / 24); {
	case n == 0:
		return "今天"
	case n == 1:
		return "明天"
	case n == -1:
		return "昨天"
	case n > 1 && n < 7:
		return weekdayName(d)
	}
	if d.Year() != day.Year() {
		return d.Format("2006年1月2日")
	}
	return fmt.Sprintf("%d月%d日", d.Month(), d.Day())
}

func weekdayName(d time.Time) string {
	return "周" + []string{"日", "一", "二", "三", "四", "五", "六"}[d.Weekday()]
}

// weekStart is the Monday of the week of d.
func weekStart(d time.Time) time.Time {
	wd := int(d.Weekday())
	if wd == 0 {
		wd = 7
	}
	return today(d).AddDate(0, 0, 1-wd)
}

// tasksView is the tasks' page.
func (a *app) tasksView(c *ui.Context) {
	t := c.Theme()
	all := a.taskItems()
	items := all
	if a.tasks.onMap != "" {
		items = slices.DeleteFunc(slices.Clone(all), func(it taskItem) bool { return it.path != a.tasks.onMap })
	}
	now := a.now()
	ui.Column(c.Key("tasks")).Grow(1).Label("任务").Children(func() {
		ui.Row(c.Key("tasks-head")).Height(toolbarHeight).Padding(0, 16, 0, 28).Gap(12).AlignItems(ui.Center).Shrink(0).
			DragWindow().Children(func() {
			ui.Text(c, "任务").FontSize(textHeadline).FontWeight(600)
			ui.Spacer(c)
			a.taskMapMenu(c, all)
			if a.tasks.view == tasksWeek || a.tasks.view == tasksMonth || a.tasks.view == tasksDay {
				a.calendarNav(c)
			}
			mode := 0
			switch a.tasks.view {
			case tasksWeek:
				mode = 1
			case tasksMonth:
				mode = 2
			}
			if m := tabGroup(c, "tasks-mode", []string{"列表", "周", "月"}, mode); m != mode {
				a.tasks.view = []int{tasksToday, tasksWeek, tasksMonth}[m]
			}
		})
		if a.tasks.view != tasksWeek && a.tasks.view != tasksMonth {
			counts := a.taskListCounts(items)
			ui.Row(c.Key("tasks-lists")).Padding(0, 28).Gap(22).Shrink(0).FillWidth().
				BorderWidth(0, 0, 1, 0).BorderColor(hairline(t)).Children(func() {
				for _, l := range taskLists {
					if underlineTab(c, "list:"+l.name, l.name, counts[l.view], a.tasks.view == l.view).Clicked() {
						a.tasks.view = l.view
					}
				}
			})
		}
		ui.Row(c.Key("tasks-add")).Margin(14, 28, 4).Height(40).Padding(0, 12).Gap(10).Radius(9).AlignItems(ui.Center).Shrink(0).
			Background(t.Text.Alpha(0.045)).Children(func() {
			glyph(c, iconPlus, 15, t.Accent)
			ph := "添加任务，如「明天 写周报 !高」，回车放进收件箱"
			if a.tasks.view == tasksDay || a.tasks.view == tasksWeek || a.tasks.view == tasksMonth {
				ph = "添加任务，不写日期就记在" + dayLabel(a.tasks.day, now) + "，回车保存"
			}
			in := ui.TextInputBase(c.Key("task-add"), &a.tasks.add).Placeholder(ph).FontSize(textBody).Grow(1).Label("添加任务")
			// What the line says, as it is written: its day and priority.
			if strings.TrimSpace(a.tasks.add) != "" {
				_, due, prio := parseTaskLine(a.tasks.add, now)
				if due != nil {
					pill(c, dayLabel(*due, now), t.Background, t.Text)
				}
				if prio != model.PriorityNone {
					pill(c, prio.Label(), t.Background, taskPriorityColor(t, prio))
				}
				ui.Text(c, "↩").FontSize(textCaption).TextColor(t.TextMuted)
			}
			if in.Submitted() && strings.TrimSpace(a.tasks.add) != "" {
				line := a.tasks.add
				a.tasks.add = ""
				var day *time.Time
				switch a.tasks.view {
				case tasksDay, tasksWeek, tasksMonth:
					d := a.tasks.day
					day = &d
				case tasksToday:
					d := today(now)
					day = &d
				}
				a.later(func() { a.addTask(line, day) })
			}
		})
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
				switch a.tasks.view {
				case tasksWeek:
					a.weekView(c, items)
				case tasksMonth:
					a.monthView(c, items)
				default:
					a.taskList(c, items)
				}
			})
			for _, it := range all {
				if it.key() == a.tasks.sel {
					a.taskDetail(c, it)
				}
			}
		})
	})
}

// taskMapMenu chooses the map whose tasks show alone, as TickTick's lists.
func (a *app) taskMapMenu(c *ui.Context, items []taskItem) {
	open := map[string]int{}
	for _, it := range items {
		if !it.done() {
			open[it.path]++
		}
	}
	label := "全部导图"
	if a.tasks.onMap != "" {
		label = mapName(a.tasks.onMap)
	}
	ui.MenuButton(c, label, func(m *ui.Menu) {
		if m.Item("全部导图").Checked(a.tasks.onMap == "").Chosen() {
			a.tasks.onMap = ""
		}
		m.Separator()
		for _, fi := range a.files {
			if n := open[fi.Path]; n > 0 || a.tasks.onMap == fi.Path {
				path := fi.Path
				if m.Item(fmt.Sprintf("%s  %d", mapName(path), n)).Checked(a.tasks.onMap == path).Chosen() {
					a.tasks.onMap = path
				}
			}
		}
	}).Label("按导图筛选").Tooltip("只看一张导图的任务").Shrink(0)
}

// selectTask shows a task's details, saving what was typed in those of
// the one before.
func (a *app) showTask(it taskItem) {
	a.saveTaskDrafts()
	a.tasks.sel = it.key()
	a.tasks.title, a.tasks.note, a.tasks.draftOf, a.tasks.subtask = it.node.Title, it.node.Note, it.key(), ""
}

// saveTaskDrafts saves the title and the note typed in the details.
func (a *app) saveTaskDrafts() {
	if a.tasks.draftOf == "" {
		return
	}
	for _, it := range a.tasks.items {
		if it.key() != a.tasks.draftOf {
			continue
		}
		title, note := strings.TrimSpace(a.tasks.title), a.tasks.note
		if title != "" && title != it.node.Title || note != it.node.Note {
			a.editTask(it, "改任务", func(n *model.Node) {
				if title != "" {
					n.Title = title
				}
				n.Note = note
			})
		}
	}
}

// taskDetail is a task's details beside the list, as TickTick's: its
// title, status, priority, day, note and subtasks, and where it is.
func (a *app) taskDetail(c *ui.Context, it taskItem) {
	t := c.Theme()
	task := it.node.Task
	if a.tasks.draftOf != it.key() {
		a.tasks.title, a.tasks.note, a.tasks.draftOf = it.node.Title, it.node.Note, it.key()
	}
	ui.Column(c.Key("task-detail")).Width(340).Shrink(0).BorderWidth(0, 0, 0, 1).BorderColor(hairline(t)).
		Background(t.Background).Children(func() {
		ui.Row(c).Padding(10, 12, 0).Children(func() {
			ui.Spacer(c)
			if iconButton(c, iconClose, "关闭详情", "关闭", 24, true).Clicked() {
				a.saveTaskDrafts()
				a.tasks.sel, a.tasks.draftOf = "", ""
			}
		})
		ui.Scroll(c.Key("task-detail-scroll")).Grow(1).Children(func() {
			ui.Column(c).Padding(0, 18, 24).Gap(16).FillWidth().Children(func() {
				ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
					a.taskCircle(c, it, 20)
					in := inlineInput(c, &a.tasks.title, textTitle, true, "任务名", "任务名")
					if in.Submitted() || !in.Focused() && a.tasks.title != it.node.Title {
						a.later(a.saveTaskDrafts)
					}
				})
				propRow(c, "状态", func() {
					st := slices.Index(model.Statuses, task.Status)
					if ui.Segmented(c.Key("task-status"), &st, "想法", "待做", "进行中", "完成").FontSize(12).Changed() {
						s := model.Statuses[st]
						a.later(func() { a.editTask(it, "改状态", func(n *model.Node) { n.SetStatus(s, a.now()) }) })
					}
				})
				propRow(c, "优先级", func() {
					p := int(task.Priority)
					if ui.Segmented(c.Key("task-prio"), &p, "无", "低", "中", "高").FontSize(12).Changed() {
						pr := model.Priority(p)
						a.later(func() { a.editTask(it, "改优先级", func(n *model.Node) { n.Task.Priority = pr }) })
					}
				})
				propRow(c, "日期", func() {
					day := today(a.now())
					due := day
					if task.Due != nil {
						due = *task.Due
					}
					if ui.DateInput(c.Key("task-due"), &due).Label("截止日期").Changed() {
						d := today(due)
						a.later(func() { a.setTaskDay(it, &d) })
					}
					for _, q := range []struct {
						label string
						d     time.Time
					}{{"今天", day}, {"明天", day.AddDate(0, 0, 1)}, {"下周一", weekStart(day).AddDate(0, 0, 7)}} {
						if ghostButton(c, q.label, "截止于"+q.label).Clicked() {
							d := q.d
							a.later(func() { a.setTaskDay(it, &d) })
						}
					}
					if task.Due != nil && ghostButton(c, "清除", "不设截止日期").Clicked() {
						a.later(func() { a.setTaskDay(it, nil) })
					}
				})
				section(c, iconPencil, "备注", nil, func() {
					in := ui.TextArea(c.Key("task-note"), &a.tasks.note).Lines(3, 14).FontSize(textBody).FillWidth().
						Placeholder("写点细节、链接、想法…").Label("任务备注")
					if !in.Focused() && a.tasks.note != it.node.Note {
						a.later(a.saveTaskDrafts)
					}
				})
				a.subtasks(c, it)
				section(c, iconMap, "位置", nil, func() {
					where := it.mapName
					if it.where != "" {
						where += " › " + it.where
					}
					ui.Text(c, where).FontSize(textSmall).TextColor(t.TextMuted).Wrap()
					if ghostButton(c, "在导图中打开", "跳到这个任务在导图里的位置").Margin(0, 0, 0, -8).Clicked() {
						a.saveTaskDrafts()
						a.later(func() { a.libOpen = false; a.goToNode(it.path, it.sheet, it.node.ID) })
					}
				})
			})
		})
	})
}

// subtasks are the nodes below a task, ticked off as tasks, and a line to
// add one.
func (a *app) subtasks(c *ui.Context, it taskItem) {
	t := c.Theme()
	done := 0
	for _, ch := range it.node.Children {
		if ch.Task != nil && ch.Task.Status == model.StatusDone {
			done++
		}
	}
	title := "子任务"
	if n := len(it.node.Children); n > 0 {
		title = fmt.Sprintf("子任务 %d/%d", done, n)
	}
	section(c, iconCheck, title, nil, func() {
		for _, ch := range it.node.Children {
			ch := ch
			isDone := ch.Task != nil && ch.Task.Status == model.StatusDone
			row := ui.Row(c.Key("sub:"+ch.ID)).Gap(8).AlignItems(ui.Center).Padding(3, 0)
			row.Children(func() {
				box := ui.Box(c.Key("tick")).Size(16, 16).Shrink(0).Cursor(ui.CursorPointer).Label("完成子任务 " + ch.Title)
				box.Draw(func(p *ui.Painter, r ui.Rect) {
					rr := ui.Rect{X: r.X + 1, Y: r.Y + 1, W: r.W - 2, H: r.H - 2}
					if isDone {
						p.Fill(rr, t.TextMuted.Alpha(0.5), 4)
						drawIcon(p, r, 11, iconCheck, t.Background)
						return
					}
					p.Stroke(rr, t.TextMuted, 4, 1.4)
				})
				if box.Clicked() {
					st := model.StatusDone
					if isDone {
						st = model.StatusTodo
					}
					id := ch.ID
					a.later(func() {
						a.editTask(it, "完成子任务", func(n *model.Node) {
							for _, k := range n.Children {
								if k.ID == id {
									k.SetStatus(st, a.now())
								}
							}
						})
					})
				}
				txt := ui.Text(c, ch.Title).FontSize(textBody).Grow(1).Shrink(1).MinWidth(0).Wrap()
				if isDone {
					txt.TextColor(t.TextMuted).Strikethrough()
				}
			})
		}
		in := ui.TextInputBase(c.Key("sub-add"), &a.tasks.subtask).Placeholder("+ 添加子任务，回车").FontSize(textBody).
			FillWidth().Label("添加子任务")
		if in.Submitted() && strings.TrimSpace(a.tasks.subtask) != "" {
			title := strings.TrimSpace(a.tasks.subtask)
			a.tasks.subtask = ""
			a.later(func() {
				a.editTask(it, "加子任务", func(n *model.Node) {
					k := model.NewNode(title)
					k.SetStatus(model.StatusTodo, a.now())
					n.Children = append(n.Children, k)
					n.Collapsed = false
				})
			})
		}
	})
}

// calendarNav goes back and forth by a day, a week or a month, and back to
// today.
func (a *app) calendarNav(c *ui.Context) {
	t := c.Theme()
	step := func(n int) {
		switch a.tasks.view {
		case tasksDay:
			a.tasks.day = a.tasks.day.AddDate(0, 0, n)
		case tasksWeek:
			a.tasks.day = a.tasks.day.AddDate(0, 0, 7*n)
		default:
			first := time.Date(a.tasks.day.Year(), a.tasks.day.Month(), 1, 0, 0, 0, 0, a.tasks.day.Location())
			a.tasks.day = first.AddDate(0, n, 0)
		}
	}
	d := a.tasks.day
	var label string
	switch a.tasks.view {
	case tasksDay:
		label = fmt.Sprintf("%d月%d日 %s", d.Month(), d.Day(), weekdayName(d))
	case tasksWeek:
		s := weekStart(d)
		e := s.AddDate(0, 0, 6)
		label = fmt.Sprintf("%d月%d日 – %d月%d日", s.Month(), s.Day(), e.Month(), e.Day())
	default:
		label = fmt.Sprintf("%d年%d月", d.Year(), d.Month())
	}
	ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
		if iconButton(c, iconBack, "上一个", "往前", 26, true).Clicked() {
			step(-1)
		}
		ui.Text(c, label).FontSize(textBody).FontWeight(600).MinWidth(120).TextAlign(ui.Center)
		if iconButton(c, iconForward, "下一个", "往后", 26, true).Clicked() {
			step(1)
		}
		if ghostButton(c, "今天", "回到今天").Clicked() {
			a.tasks.day = today(a.now())
		}
	})
	_ = t
}

// taskList is the lists: today (with what is overdue), the next 7 days a
// day each, all by when, or the day of the calendar.
func (a *app) taskList(c *ui.Context, items []taskItem) {
	t := c.Theme()
	now := a.now()
	day := today(now)
	type group struct {
		title  string
		danger bool
		drop   *time.Time // the day a task dropped on the group goes to
		items  []taskItem
	}
	var groups []*group
	var done []taskItem
	add := func(title string, danger bool, drop *time.Time) *group {
		g := &group{title: title, danger: danger, drop: drop}
		groups = append(groups, g)
		return g
	}
	at := func(d time.Time) *time.Time { return &d }
	switch a.tasks.view {
	case tasksToday:
		over, todayG := add("已逾期", true, at(day)), add("今天", false, at(day))
		for _, it := range items {
			d := it.date()
			switch {
			case d == nil:
			case it.done():
				if it.node.Task.DoneAt != nil && sameDay(*it.node.Task.DoneAt, now) {
					done = append(done, it)
				}
			case d.Before(day):
				over.items = append(over.items, it)
			case sameDay(*d, day):
				todayG.items = append(todayG.items, it)
			}
		}
	case tasksWeekAhead:
		over := add("已逾期", true, at(day))
		days := make([]*group, 7)
		for i := range days {
			d := day.AddDate(0, 0, i)
			title := dayLabel(d, now)
			if i > 1 {
				title = fmt.Sprintf("%s  %d月%d日", weekdayName(d), d.Month(), d.Day())
			}
			days[i] = add(title, false, at(d))
		}
		for _, it := range items {
			d := it.date()
			if d == nil {
				continue
			}
			if it.done() {
				if !d.Before(day) && d.Before(day.AddDate(0, 0, 7)) {
					done = append(done, it)
				}
				continue
			}
			if d.Before(day) {
				over.items = append(over.items, it)
			} else if i := int(today(*d).Sub(day).Hours() / 24); i < 7 {
				days[i].items = append(days[i].items, it)
			}
		}
	case tasksDay:
		d := a.tasks.day
		g := add(dayLabel(d, now), false, at(d))
		var over *group
		if sameDay(d, day) {
			over = add("已逾期", true, at(day))
			groups[0], groups[1] = over, g
		}
		for _, it := range items {
			date := it.date()
			if date == nil {
				continue
			}
			switch {
			case sameDay(*date, d) && it.done():
				done = append(done, it)
			case sameDay(*date, d):
				g.items = append(g.items, it)
			case over != nil && !it.done() && date.Before(day):
				over.items = append(over.items, it)
			}
		}
	case tasksInbox:
		g := add("收件箱", false, nil)
		for _, it := range items {
			if it.path != a.inboxPath() {
				continue
			}
			if it.done() {
				done = append(done, it)
			} else {
				g.items = append(g.items, it)
			}
		}
	case tasksDone:
		g := add("已完成", false, nil)
		for _, it := range items {
			if it.done() {
				g.items = append(g.items, it)
			}
		}
		slices.SortStableFunc(g.items, func(x, y taskItem) int {
			dx, dy := x.node.Task.DoneAt, y.node.Task.DoneAt
			switch {
			case dx == nil && dy == nil:
				return 0
			case dx == nil:
				return 1
			case dy == nil:
				return -1
			}
			return dy.Compare(*dx)
		})
	default: // all, by when
		over, todayG := add("已逾期", true, at(day)), add("今天", false, at(day))
		tomorrow, week := add("明天", false, at(day.AddDate(0, 0, 1))), add("未来 7 天", false, nil)
		later, none := add("以后", false, nil), add("无日期", false, nil)
		for _, it := range items {
			if it.done() {
				done = append(done, it)
				continue
			}
			if it.node.Task.Status == model.StatusIdea && it.date() == nil {
				continue // an idea, not yet a task to do
			}
			d := it.date()
			switch {
			case d == nil:
				none.items = append(none.items, it)
			case d.Before(day):
				over.items = append(over.items, it)
			case sameDay(*d, day):
				todayG.items = append(todayG.items, it)
			case sameDay(*d, day.AddDate(0, 0, 1)):
				tomorrow.items = append(tomorrow.items, it)
			case d.Before(day.AddDate(0, 0, 8)):
				week.items = append(week.items, it)
			default:
				later.items = append(later.items, it)
			}
		}
	}
	empty := true
	for _, g := range groups {
		empty = empty && len(g.items) == 0
	}
	ui.Scroll(c.Key(fmt.Sprint("task-list:", a.tasks.view))).Grow(1).Children(func() {
		ui.Column(c).Padding(8, 20, 32).Gap(2).FillWidth().Children(func() {
			if empty && len(done) == 0 {
				ui.Column(c).Padding(60, 0).Gap(8).AlignItems(ui.Center).FillWidth().Children(func() {
					glyph(c, iconCheck, 36, t.TextMuted.Alpha(0.6))
					ui.Text(c, "没有要做的事").FontSize(textTitle).TextColor(t.TextMuted)
					ui.Text(c, "在上面写一句加任务，或在导图里把节点设为任务、给它一个日期").FontSize(textSmall).TextColor(t.TextMuted)
				})
				return
			}
			for _, g := range groups {
				// Empty, a group of a day still shows, quietly: a place to drop
				// a task on; others empty do not.
				if len(g.items) == 0 && (g.danger || g.drop == nil) && len(groups) > 1 {
					continue
				}
				// The whole group, its heading and its tasks, takes a task
				// dropped on it, to its day.
				box := ui.Column(c.Key("group:" + g.title)).Gap(2).FillWidth().Radius(8)
				if g.drop != nil {
					a.dropTaskOn(box, *g.drop)
				}
				box.Children(func() {
					ui.Row(c).Padding(14, 4, 6).Gap(8).AlignItems(ui.Center).FillWidth().Children(func() {
						col := t.Text
						switch {
						case g.danger:
							col = t.Danger
						case len(g.items) == 0:
							col = t.TextMuted
						}
						ui.Text(c, g.title).FontSize(textSmall).FontWeight(600).TextColor(col)
						if len(g.items) > 0 {
							ui.Text(c, fmt.Sprint(len(g.items))).FontSize(textCaption).TextColor(t.TextMuted)
						}
					})
					for _, it := range g.items {
						a.taskRow(c, it, g.danger)
					}
				})
			}
			if len(done) > 0 {
				head := ui.Row(c.Key("group:done")).Padding(18, 4, 6).Gap(6).AlignItems(ui.Center).Cursor(ui.CursorPointer).
					Label("已完成")
				if head.Clicked() {
					a.tasks.showDone = !a.tasks.showDone
				}
				head.Children(func() {
					arrow := "›"
					if a.tasks.showDone {
						arrow = "⌄"
					}
					ui.Text(c, arrow).FontSize(textSmall).TextColor(t.TextMuted)
					ui.Text(c, fmt.Sprintf("已完成 %d", len(done))).FontSize(textSmall).FontWeight(600).TextColor(t.TextMuted)
				})
				if a.tasks.showDone {
					for _, it := range done {
						a.taskRow(c, it, false)
					}
				}
			}
		})
	})
}

// taskRow is a task in a list: its circle, in its priority's color, which
// ticks it off; its title, which opens it in its map; its map and day.
func (a *app) taskRow(c *ui.Context, it taskItem, overdue bool) {
	t := c.Theme()
	task := it.node.Task
	row := ui.Row(c.Key("task:"+it.path+"#"+it.node.ID)).Padding(7, 8).Gap(10).Radius(8).AlignItems(ui.Center).
		FillWidth().Cursor(ui.CursorPointer).Label("任务 " + it.node.Title).Drag(taskDrag{it.path, it.node.ID})
	if row.Hovered() {
		row.Background(t.Text.Alpha(0.04))
	}
	switch {
	case row.DoubleClicked():
		a.later(func() { a.libOpen = false; a.goToNode(it.path, it.sheet, it.node.ID) })
	case row.Clicked():
		a.later(func() { a.showTask(it) })
	}
	if a.tasks.sel == it.key() {
		row.Background(chosen(t))
	}
	row.ContextMenu(func(m *ui.Menu) { a.taskMenu(m, it) })
	row.Children(func() {
		a.taskCircle(c, it, 18)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).PassThrough().Children(func() {
			title := ui.Text(c, it.node.Title).FontSize(textBody).SingleLine()
			if it.done() {
				title.TextColor(t.TextMuted).Strikethrough()
			}
			sub := it.mapName
			if it.where != "" {
				sub += " · " + it.where
			}
			ui.Text(c, sub).FontSize(textCaption).TextColor(t.TextMuted).SingleLine()
		})
		if d := it.date(); d != nil {
			col := t.TextMuted
			if overdue || !it.done() && d.Before(today(a.now())) {
				col = t.Danger
			}
			label := dayLabel(*d, a.now())
			if task.Due == nil {
				label = "开始 " + label
			}
			ui.Text(c, label).FontSize(textSmall).TextColor(col).Shrink(0).PassThrough()
		}
	})
}

// taskCircle ticks a task off, or back on: an open circle in its
// priority's color, a check when done.
func (a *app) taskCircle(c *ui.Context, it taskItem, size float32) {
	t := c.Theme()
	col := taskPriorityColor(t, it.node.Task.Priority)
	done := it.done()
	box := ui.Box(c.Key("circle")).Size(size, size).Shrink(0).Cursor(ui.CursorPointer).Label("完成 " + it.node.Title)
	hover := box.Hovered()
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		rr := ui.Rect{X: r.X + 1, Y: r.Y + 1, W: r.W - 2, H: r.H - 2}
		if done {
			p.Fill(rr, t.TextMuted.Alpha(0.5), rr.W/2)
			drawIcon(p, r, size*0.62, iconCheck, t.Background)
			return
		}
		if hover {
			p.Fill(rr, col.Alpha(0.15), rr.W/2)
		}
		p.Stroke(rr, col, rr.W/2, 1.6)
	})
	if box.Clicked() {
		a.later(func() { a.setTaskDone(it, !done) })
	}
}

func taskPriorityColor(t *ui.Theme, p model.Priority) ui.Color {
	switch p {
	case model.PriorityHigh:
		return ui.Hex("#D9605B")
	case model.PriorityMedium:
		return ui.Hex("#D4A836")
	case model.PriorityLow:
		return ui.Hex("#5B80D6")
	}
	return t.TextMuted
}

// taskMenu dates a task, sets its priority, or opens it.
func (a *app) taskMenu(m *ui.Menu, it taskItem) {
	day := today(a.now())
	set := func(d *time.Time) { a.later(func() { a.setTaskDay(it, d) }) }
	at := func(d time.Time) *time.Time { return &d }
	if m.Item("今天").Chosen() {
		set(at(day))
	}
	if m.Item("明天").Chosen() {
		set(at(day.AddDate(0, 0, 1)))
	}
	if m.Item("下周一").Chosen() {
		set(at(weekStart(day).AddDate(0, 0, 7)))
	}
	if m.Item("清除日期").Disabled(it.node.Task.Due == nil).Chosen() {
		set(nil)
	}
	m.Separator()
	m.Submenu("优先级", func(m *ui.Menu) {
		for _, p := range []model.Priority{model.PriorityHigh, model.PriorityMedium, model.PriorityLow, model.PriorityNone} {
			if m.Item(p.Label()).Checked(it.node.Task.Priority == p).Chosen() {
				a.later(func() { a.editTask(it, "改优先级", func(n *model.Node) { n.Task.Priority = p }) })
			}
		}
	})
	if m.Item("移到导图…").Chosen() {
		a.later(func() { a.moveTaskTo(it) })
	}
	if m.Item("在导图中打开").Chosen() {
		a.later(func() { a.libOpen = false; a.goToNode(it.path, it.sheet, it.node.ID) })
	}
}

// moveTaskTo asks which topic of which map a task goes under, and moves
// it there, its own topics with it: the way out of the inbox, without
// leaving the tasks' page.
func (a *app) moveTaskTo(it taskItem) {
	a.saveTaskDrafts()
	a.goToNode(it.path, it.sheet, it.node.ID)
	if a.path != it.path || a.selected != it.node.ID {
		a.tell("没有找到这个任务")
		return
	}
	a.openTasks()
	a.tasks.sel, a.tasks.draftOf = "", ""
	a.openPalette(paletteMove)
}

// dropTaskOn makes el take tasks dropped on it, which go to the day.
func (a *app) dropTaskOn(el ui.Element, day time.Time) {
	t := a.theme()
	if _, over := ui.DragOver[taskDrag](el); over {
		el.Background(t.Accent.Alpha(0.12))
	}
	if d, ok := ui.Drop[taskDrag](el); ok {
		for _, it := range a.tasks.items {
			if it.path == d.path && it.node.ID == d.id {
				a.later(func() { a.setTaskDay(it, &day) })
				return
			}
		}
	}
}

// theme is the theme the window was last drawn in.
func (a *app) theme() *ui.Theme {
	if a.lastTheme != nil {
		return a.lastTheme
	}
	return ui.LightTheme()
}

// weekView is the week of the day shown: a column a day, its tasks in it,
// which tasks dragged to another day go to.
func (a *app) weekView(c *ui.Context, items []taskItem) {
	t := c.Theme()
	start := weekStart(a.tasks.day)
	ui.Row(c.Key("week")).Grow(1).AlignItems(ui.Stretch).Padding(0, 12, 12).Children(func() {
		for i := range 7 {
			d := start.AddDate(0, 0, i)
			isToday := sameDay(d, a.now())
			col := ui.Column(c.Key(fmt.Sprint("week-day:", i))).Grow(1).Basis(0).MinWidth(0).Padding(8, 4).Gap(4)
			if i > 0 {
				col.BorderWidth(0, 0, 0, 1).BorderColor(hairline(t))
			}
			a.dropTaskOn(col, d)
			if col.DoubleClicked() {
				a.tasks.day, a.tasks.view = d, tasksDay
			}
			col.Children(func() {
				ui.Row(c).Padding(4, 6, 8).Gap(6).AlignItems(ui.Baseline).Children(func() {
					ui.Text(c, weekdayName(d)).FontSize(textSmall).TextColor(t.TextMuted)
					num := ui.Text(c, fmt.Sprint(d.Day())).FontSize(textTitle).FontWeight(600)
					if isToday {
						num.TextColor(t.Accent)
					}
				})
				ui.Scroll(c.Key("scroll")).Grow(1).Children(func() {
					ui.Column(c).Gap(4).FillWidth().Children(func() {
						for _, it := range items {
							if dd := it.date(); dd != nil && sameDay(*dd, d) {
								a.taskChip(c, it)
							}
						}
					})
				})
			})
		}
	})
}

// monthView is the month of the day shown: a cell a day, a few of its
// tasks in it and how many more; a double click on a day shows it.
func (a *app) monthView(c *ui.Context, items []taskItem) {
	t := c.Theme()
	first := time.Date(a.tasks.day.Year(), a.tasks.day.Month(), 1, 0, 0, 0, 0, a.tasks.day.Location())
	start := weekStart(first)
	byDay := map[string][]taskItem{}
	for _, it := range items {
		if d := it.date(); d != nil {
			k := d.Format("2006-01-02")
			byDay[k] = append(byDay[k], it)
		}
	}
	ui.Column(c.Key("month")).Grow(1).Padding(0, 12, 12).Children(func() {
		ui.Row(c).Shrink(0).Children(func() {
			for _, w := range []string{"一", "二", "三", "四", "五", "六", "日"} {
				ui.Text(c, "周"+w).FontSize(textCaption).TextColor(t.TextMuted).Grow(1).Basis(0).Padding(8, 8, 4)
			}
		})
		for week := range 6 {
			ui.Row(c.Key(fmt.Sprint("month-week:", week))).Grow(1).Basis(0).MinHeight(0).AlignItems(ui.Stretch).
				BorderWidth(1, 0, 0, 0).BorderColor(hairline(t)).Children(func() {
				for i := range 7 {
					d := start.AddDate(0, 0, week*7+i)
					in := d.Month() == first.Month()
					cell := ui.Column(c.Key(d.Format("cell:2006-01-02"))).Grow(1).Basis(0).MinWidth(0).Padding(4).Gap(2).Clip()
					if i > 0 {
						cell.BorderWidth(0, 0, 0, 1).BorderColor(hairline(t))
					}
					if !in {
						cell.Background(t.Text.Alpha(0.02))
					}
					a.dropTaskOn(cell, d)
					if cell.DoubleClicked() {
						day := d
						a.later(func() { a.tasks.day, a.tasks.view = day, tasksDay })
					}
					list := byDay[d.Format("2006-01-02")]
					cell.Children(func() {
						ui.Row(c).Padding(2, 4).Children(func() {
							num := ui.Text(c, fmt.Sprint(d.Day())).FontSize(textSmall)
							switch {
							case sameDay(d, a.now()):
								num.TextColor(t.Accent).FontWeight(700)
							case !in:
								num.TextColor(t.TextMuted.Alpha(0.6))
							}
						})
						for j, it := range list {
							if j == 3 && len(list) > 4 {
								more := ui.Text(c, fmt.Sprintf("还有 %d 项", len(list)-3)).FontSize(textCaption).
									TextColor(t.TextMuted).Padding(0, 6).Cursor(ui.CursorPointer)
								if more.Clicked() {
									day := d
									a.later(func() { a.tasks.day, a.tasks.view = day, tasksDay })
								}
								break
							}
							a.taskChip(c, it)
						}
					})
				}
			})
		}
	})
}

// taskChip is a task in the calendar: a small circle and its title, on a
// faint fill; dragged to another day, it goes there.
func (a *app) taskChip(c *ui.Context, it taskItem) {
	t := c.Theme()
	chip := ui.Row(c.Key("chip:"+it.path+"#"+it.node.ID)).Padding(3, 6).Gap(6).Radius(6).AlignItems(ui.Center).FillWidth().
		Background(taskPriorityColor(t, it.node.Task.Priority).Alpha(0.1)).Cursor(ui.CursorPointer).
		Label("任务 " + it.node.Title).Tooltip(it.mapName + " · " + it.node.Title).Drag(taskDrag{it.path, it.node.ID})
	switch {
	case chip.DoubleClicked():
		a.later(func() { a.libOpen = false; a.goToNode(it.path, it.sheet, it.node.ID) })
	case chip.Clicked():
		a.later(func() { a.showTask(it) })
	}
	chip.ContextMenu(func(m *ui.Menu) { a.taskMenu(m, it) })
	chip.Children(func() {
		a.taskCircle(c, it, 13)
		title := ui.Text(c, it.node.Title).FontSize(textSmall).SingleLine().Shrink(1).MinWidth(0).PassThrough()
		if it.done() {
			title.TextColor(t.TextMuted).Strikethrough()
		}
	})
}

// ensureInbox makes the inbox, where tasks and notes written in a line go,
// when there is none.
func (a *app) ensureInbox() error {
	path := a.inboxPath()
	if modTime(path).IsZero() {
		f := model.NewFile(inboxName)
		f.Sheets[0].Root.Note = "随手记下的想法和待办，之后整理到对应的导图里（右键节点「移到其他导图…」）。"
		if err := f.Save(path); err != nil {
			return err
		}
		a.refreshFiles()
	}
	return nil
}

// taskListCounts are how many tasks each list of the page holds, open.
func (a *app) taskListCounts(items []taskItem) map[int]string {
	day := today(a.now())
	n := map[int]int{}
	for _, it := range items {
		if it.done() {
			continue
		}
		if it.path == a.inboxPath() {
			n[tasksInbox]++
		}
		if it.node.Task.Status == model.StatusIdea && it.date() == nil {
			continue
		}
		n[tasksAll]++
		if d := it.date(); d != nil {
			if d.Before(day.AddDate(0, 0, 1)) {
				n[tasksToday]++
			}
			if d.Before(day.AddDate(0, 0, 7)) {
				n[tasksWeekAhead]++
			}
		}
	}
	out := map[int]string{}
	for k, v := range n {
		if v > 0 {
			out[k] = fmt.Sprint(v)
		}
	}
	return out
}
