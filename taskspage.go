package main

import (
	"fmt"
	"slices"
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
	tasksInbox   // the tasks written down, not yet put in a map
	tasksDone    // those done, the last first
	tasksWaiting // what was written down and waits for a place
	tasksOfMap   // not a list of the left column: how a project's tasks are grouped
	tasksReview  // the week looked back on: what was done, what slipped, what was kept
	tasksLate    // what is late and not done, alone
	tasksAgents  // what the AI agents reported, by day and project (agents.go)
)

// The lists of the tasks' page, as its tabs show them.
var taskLists = []struct {
	view       int
	name, icon string
}{{tasksToday, "今天", "calendar"}, {tasksWeekAhead, "最近 7 天", "calendar-week"}, {tasksInbox, "收件箱", "inbox"}, {tasksAll, "全部", "list"}, {tasksDone, "已完成", "circle-check"}}

// tasksState is the tasks' page.
type tasksState struct {
	open bool
	view int
	day  time.Time // the day the calendar shows, or is in
	// todayWas is the day it was when the page was last drawn: the day
	// shown follows it over midnight where it was today.
	todayWas time.Time
	add      string // the task being written
	showDone bool
	// repeatOwn is the task whose repeat is being made up in its details.
	repeatOwn string
	items     []taskItem
	// read is when the items were gathered, and gathered the same: what
	// sets read to another time has them gathered again. of is what they
	// were gathered of, looked when every map's file was last looked at,
	// and serial counts the gatherings, for what is worked out of them.
	read, gathered time.Time
	of             tasksOf
	looked         lookedAt
	serial         int
	// remindOwn is the task whose time to remind at is being written,
	// remindText what is written, remindBad that it was not understood.
	remindOwn, remindText string
	remindBad             bool
	// spanOwn is the task whose time of day is being written, as remind's.
	spanOwn, spanText string
	spanBad           bool
	// weekScroll is how far the week's hours are scrolled; weekAt the week
	// it was set for, a week newly shown opening at its first hours.
	weekScroll ui.ScrollState
	weekAt     time.Time
	weekDays   int
	// On the hours: the span being chosen by pressing (pick), the task
	// being written where it was chosen (draft), and a block's end being
	// dragged (stretch).
	pick    *hourPick
	draft   *hourDraft
	stretch *hourStretch
	// What the agents reported, as last gathered; how far back the list
	// looks (agentRanges); and when it was looked at before this time,
	// what came since being marked as new.
	agents        []agentItem
	agentsRead    time.Time
	agentsOf      tasksOf
	agentRange    int
	agentsSeenWas time.Time
	// allMaps has the left column show every project with tasks, not the
	// few starred and opened last.
	allMaps bool
	// reviewBack is how many weeks back the review looks.
	reviewBack int
	// reveal is the task the keyboard went to, to scroll to once.
	reveal string
	// sel is the task whose details show, as path#id; onMap the map whose
	// tasks show alone, "" for all.
	sel, onMap string
	// selRail is the task chosen from what waits in the column beside
	// the tasks: while its details show, that column stays.
	selRail string
	// The details edited: the title and the note, kept as they are typed
	// and saved as they are left; the subtask being written.
	title, note, draftOf string
	// titleWas and noteWas are the task's name and note as the draft was
	// taken of them: what is typed is what differs from them, and a task
	// changed meanwhile by an agent or the sync differs from them too.
	titleWas, noteWas string
	subtask           string
	// places are where the AI would put the tasks of the inbox, by their
	// nodes' IDs (sortout.go); placing while it is asked.
	places   map[string]taskPlace
	placing  bool
	placeErr string
	table    bool // the list has room to be a table, with a column for the project and the day
	// dueSeen are the tasks due or late as the reminders last looked, by
	// their keys (reminders.go); nil before the day's first look.
	dueSeen map[string]bool
}

// tasksOf is what the tasks were gathered of: the maps as read, the folder
// as scanned, the open map as edited.
type tasksOf struct{ maps, scans, edits int }

func (it taskItem) key() string { return it.path + "#" + it.node.ID }

// taskItem is a task of a map.
type taskItem struct {
	path, mapName, sheet string
	node                 *model.Node // the map's, as read: not edited here
	where                string
}

// date is the task's day: the day it is due, or else the one it begins
// on. It is the calendar's day here (calendarDay): what the lists, the
// counts and the calendars all compare.
func (it taskItem) date() *time.Time {
	if t := it.node.Task; t.Due != nil {
		return calendarDay(t.Due)
	}
	return calendarDay(it.node.Task.Start)
}

// due is the day the task is due, the calendar's day here, or nil.
func (it taskItem) due() *time.Time { return calendarDay(it.node.Task.Due) }

// calendarDay is a day kept with a task as the calendar's day here: one
// written where the clocks say otherwise is that day still, not the hour
// of the day before or after that its midnight is here (model.DayIn).
func calendarDay(d *time.Time) *time.Time {
	if d == nil || d.Location() == time.Local {
		return d
	}
	day := model.DayIn(*d, time.Local)
	return &day
}

func (it taskItem) done() bool { return it.node.Task.Status == model.StatusDone }

// late reports whether the task is to do and its day is before day: the
// one rule of what is late, for the left column's count, the lists, the
// projects' figures, the week's review and the day's notification. They
// had each their own, by the day due alone or by either day, and the
// figure of one was not the list of the other.
func (it taskItem) late(day time.Time) bool {
	d := it.date()
	return countsAsTask(it) && d != nil && d.Before(day)
}

// doneAt is when the task was done, for what counts the done: one ticked
// off, or one that repeats and was done a time, which is to do again on
// its next day and was done all the same (it never counted among 做完,
// 已完成 nor the week's review).
func (it taskItem) doneAt() *time.Time {
	if t := it.node.Task; it.done() || t.Repeat != nil {
		return t.DoneAt
	}
	return nil
}

// doneOn reports whether the task was done on the day.
func (it taskItem) doneOn(day time.Time) bool {
	at := it.doneAt()
	return at != nil && sameDay(at.In(day.Location()), day)
}

// taskWhen is when a task is: its days, its time of day and when it
// reminds, kept before they are changed to be given back (撤销).
type taskWhen struct {
	due, start      *time.Time
	at, end, remind string
}

func whenOf(t *model.Task) taskWhen {
	day := func(d *time.Time) *time.Time {
		if d == nil {
			return nil
		}
		c := *d
		return &c
	}
	return taskWhen{day(t.Due), day(t.Start), t.Time, t.EndTime, t.Remind}
}

func (w taskWhen) put(t *model.Task) {
	if t != nil {
		t.Due, t.Start, t.Time, t.EndTime, t.Remind = w.due, w.start, w.at, w.end, w.remind
	}
}

// taskDrag is a task dragged to a day.
type taskDrag struct{ path, id string }

// openTasks shows the tasks in the main area.
func (a *app) openTasks() {
	a.showLibrary()
	a.tasks.open = true
	if a.tasks.day.IsZero() {
		a.tasks.day = today(a.now())
	}
	a.tasks.read = time.Time{}
}

// taskItems are the tasks of all the maps, the open one as it is edited.
// They are gathered again when a map was read anew (every map's file is
// looked at once a tick, indexed), when the folder was scanned, when the
// open map changed (a.edits), and when what changed a task asks for it
// (tasks.read set to another time); else they stand, frame after frame.
// What is given is not to be changed in place.
func (a *app) taskItems() []taskItem {
	if at := a.looked(); at != a.tasks.looked || at.tick.IsZero() {
		a.tasks.looked = at
		for _, fi := range a.files {
			if fi.Path != a.path {
				a.indexed(fi.Path)
			}
		}
	}
	of := tasksOf{a.memo.maps, a.memo.scans, a.edits}
	if of == a.tasks.of && a.tasks.read.Equal(a.tasks.gathered) && !a.tasks.read.IsZero() {
		return a.tasks.items
	}
	a.tasks.of, a.tasks.read, a.tasks.gathered = of, a.now(), a.now()
	a.tasks.serial++
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

// editTask changes a task in its map, saved as an edit of it, and tells
// whether it did: what says the change is not to say it of one not made.
func (a *app) editTask(it taskItem, note string, fn func(n *model.Node)) bool {
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
	} else {
		a.remindOwn(it.key()) // changed here today: the day is not said for it alone
	}
	a.tasks.read = time.Time{}
	return err == nil
}

// setTaskDone ticks a task off, or back on. Ticked off, it says so with
// the way back, whether it is done or, repeating, to do again on its next
// day: all that the tick changed is as it was, its days, its state, when
// and how many times it was done.
func (a *app) setTaskDone(it taskItem, done bool) {
	st := model.StatusTodo
	if done {
		st = model.StatusDone
	}
	// As it is before the edit, and the day it comes again counted from
	// that: a task of the open map is the map's own node, which the edit
	// changes, and its day read after was the next one's already, the day
	// said the one after next.
	task := it.node.Task
	was, status, times, doneAt := whenOf(task), task.Status, task.Times, task.DoneAt
	now := a.now()
	rep, next := task.Repeat, time.Time{}
	if rep != nil {
		next = rep.Next(it.due(), now)
		if rep.Ended(next, times+1) {
			rep = nil // its last time: done as any task
		}
	}
	if !a.editTask(it, "完成任务", func(n *model.Node) { n.SetStatus(st, now) }) || !done || status == model.StatusDone {
		return
	}
	said := "已完成「" + it.node.Title + "」"
	if rep != nil {
		said = "做完一次，下一次：" + dayLabel(next, now)
	}
	a.tellUndo(said, func() {
		a.editTask(it, "撤销完成", func(n *model.Node) {
			if n.Task == nil {
				return
			}
			n.SetStatus(status, a.now()) // its marker with it
			was.put(n.Task)
			n.Task.Status, n.Task.Times, n.Task.DoneAt = status, times, doneAt
		})
	})
}

// setTaskDay dates a task: its due day, or none; a start after it goes.
func (a *app) setTaskDay(it taskItem, day *time.Time) bool {
	return a.editTask(it, "改任务日期", func(n *model.Node) {
		if n.Task == nil {
			n.SetStatus(model.StatusTodo, a.now())
		}
		n.Task.Due = day
		if day != nil && n.Task.Start != nil && n.Task.Start.After(*day) {
			n.Task.Start = nil
		}
	})
}

// moveTaskDay dates a task from where it is shown, dropped on a day or by
// its menu or the search's key, and says so with the way back: its days,
// its time and when it reminds as they were. (The hours' moves said so;
// these changed a day and said nothing, or said it with no way back.)
func (a *app) moveTaskDay(it taskItem, day *time.Time) {
	due := it.due()
	if day == nil && due == nil || day != nil && due != nil && sameDay(*due, *day) {
		return // its day already
	}
	was := whenOf(it.node.Task)
	if !a.setTaskDay(it, day) {
		return
	}
	said := "已清除日期"
	if day != nil {
		said = "已改到" + dayLabel(*day, a.now())
	}
	a.tellUndo(said+"：「"+it.node.Title+"」", func() {
		a.editTask(it, "撤销改日期", func(n *model.Node) { was.put(n.Task) })
	})
}

// postponeToToday puts tasks that are late on today, and says so with the
// way to take it back: each has again the days it had.
func (a *app) postponeToToday(items []taskItem) {
	day := today(a.now())
	type was struct {
		it         taskItem
		due, start *time.Time
	}
	var before []was
	for _, it := range items {
		task := it.node.Task
		if task == nil || it.done() {
			continue
		}
		before = append(before, was{it, task.Due, task.Start})
		late := task.Due == nil // late by the day it starts on: that day moves
		a.editTask(it, "顺延到今天", func(n *model.Node) {
			d := day
			if late {
				n.Task.Start = &d
				return
			}
			n.Task.Due = &d
			if n.Task.Start != nil && n.Task.Start.After(d) {
				n.Task.Start = nil
			}
		})
	}
	if len(before) == 0 {
		return
	}
	a.tellUndo(fmt.Sprintf("%d 件任务顺延到今天", len(before)), func() {
		for _, w := range before {
			a.editTask(w.it, "撤销顺延", func(n *model.Node) {
				if n.Task != nil {
					n.Task.Due, n.Task.Start = w.due, w.start
				}
			})
		}
	})
}

// dayLabel names a day near now: 今天, 明天, 周三, 11月20日.
func dayLabel(d, now time.Time) string {
	day := today(now)
	// In days, not in hours by 24: a day of 23 hours, where the clocks
	// change, made tomorrow today.
	switch n := daysBetween(day, d); {
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
// weekOf is the first day of the calendars' week of the day d: Monday,
// or Sunday as the settings say (config.WeekFromSunday).
func (a *app) weekOf(d time.Time) time.Time {
	if a.cfg.WeekFromSunday {
		return today(d).AddDate(0, 0, -int(d.Weekday()))
	}
	return weekStart(d)
}

func weekStart(d time.Time) time.Time {
	wd := int(d.Weekday())
	if wd == 0 {
		wd = 7
	}
	return today(d).AddDate(0, 0, 1-wd)
}

// todayWidth is the width of 今天's column where it is a list: narrower
// than a page's, for it is read down.
const todayWidth = 760

// tasksView is 今天, the tasks' page.
func (a *app) tasksView(c *ui.Context) {
	// The day shown follows midnight where it was today: the app left open
	// over a night, the calendars showed yesterday as their day, and a
	// line with no day was kept on it, late as it was written.
	if d := today(a.now()); !a.tasks.todayWas.Equal(d) {
		if a.tasks.day.IsZero() || a.tasks.day.Equal(a.tasks.todayWas) {
			a.tasks.day = d
		}
		a.tasks.todayWas = d
	}
	// Off the hours, nothing is being chosen or drawn there: a press held
	// as the page was left wrote a task where it began, on coming back.
	if a.tasks.view != tasksWeek && a.tasks.view != tasksDay {
		a.tasks.pick, a.tasks.stretch = nil, nil
	}
	all := a.taskItems()
	items := all
	if a.tasks.onMap != "" {
		items = slices.DeleteFunc(slices.Clone(all), func(it taskItem) bool { return it.path != a.tasks.onMap })
	}
	now := a.now()
	// The page's column, as every page's: its heading, the line to add a
	// task, then the lists with the week beside them. The week and the
	// month take the page's width under the heading. The details of the
	// task chosen stand on the right, the page's height.
	calendar := a.tasks.view == tasksWeek || a.tasks.view == tasksMonth || a.tasks.view == tasksDay
	// What waits for a place stands with today's tasks, of no one project:
	// under them, cut from them by a band; in a wide window beside them, in
	// a column of its own, the tasks' column wider for the room there is.
	band := a.tasks.view == tasksToday && a.tasks.onMap == ""
	roomy := band && a.todayRoom(c) >= railRoom
	// A task chosen in the list has its details beside the list: what
	// waits gives its column up and is under the tasks meanwhile. It stood
	// between the two, the user found, the details of a task a column away
	// from the task. Chosen from what waits, it stays, the details beside
	// it: a click does not take away what was clicked.
	rail := roomy && (a.tasks.sel == "" || a.tasks.sel == a.tasks.selRail)
	// With room, the list is a table: a column for a task's project and
	// one for its day, named once over them.
	plain := a.tasks.view == tasksWaiting || a.tasks.view == tasksReview || a.tasks.view == tasksAgents // a page of its own, not a list of tasks
	a.tasks.table = !calendar && !plain && a.todayRoom(c) >= tableRoom
	listWidth := float32(todayWidth)
	switch {
	case a.tasks.sel != "" && !rail && !calendar:
		// With a task chosen the list takes all the room up to its
		// details, as TickTick's does: capped, on a large screen it left a
		// blank column between the two, which the user asked about.
		listWidth = 4000
	case roomy:
		listWidth = 920
	}
	ui.Row(c.Key("tasks")).Grow(1).AlignItems(ui.Stretch).Label("任务").Children(func() {
		ui.Column(c).Grow(3).Basis(0).MinWidth(0).Children(func() {
			top := pageColumn(c, "tasks-top", false)
			if !calendar {
				top.MaxWidth(listWidth + 56) // one calm column, as what is under it
			}
			top.Children(func() {
				a.tasksHead(c, all)
				if !plain {
					a.tasksAdd(c, now) // onto the day chosen; a list's, onto its own
				}
			})
			if calendar {
				ui.Column(c).Grow(1).Basis(0).MinHeight(0).FillWidth().Margin(10, 0, 0).Children(func() {
					switch a.tasks.view {
					case tasksWeek:
						a.weekView(c, items)
					case tasksDay:
						a.hoursView(c, items, []time.Time{today(a.tasks.day)})
					default:
						a.monthView(c, items)
					}
				})
				return
			}
			// One column, read down: what is to do, by its lists, then what was
			// written down and waits for a place.
			ui.Scroll(c.Key(fmt.Sprint("today-scroll:", a.tasks.view))).Grow(1).Basis(0).MinHeight(0).Children(func() {
				foot := float32(40)
				if band && !rail {
					foot = 22
				}
				pageColumn(c, "tasks-column", false).MaxWidth(listWidth+56).Padding(4, 28, foot).Gap(6).Children(func() {
					// Which list shows is chosen in the left column (todaySide).
					if a.tasks.view == tasksWaiting {
						a.homeWaiting(c)
						return
					}
					if a.tasks.view == tasksReview {
						a.weekReview(c, all)
						return
					}
					if a.tasks.view == tasksAgents {
						a.agentsView(c)
						return
					}
					if band { // what the agents reported since it was last looked at
						a.agentsLine(c)
						a.diaLine(c)
					}
					a.taskList(c, items)
				})
				if band && !rail {
					a.waitingBand(c, items)
				}
			})
		})
		if rail {
			a.waitingRail(c, items)
		}
		for _, it := range all {
			if it.key() == a.tasks.sel {
				// The task chosen, in a sheet over the page's right.
				sheet(c, "task-sheet", 400, func() { a.taskDetail(c, it) })
			}
		}
	})
}

// tasksHead is the tasks' heading: the map whose tasks show, the weeks or
// months to go through, and how they show.
func (a *app) tasksHead(c *ui.Context, all []taskItem) {
	now := a.now()
	th := c.Theme()
	// The day, large, and under it how it stands; at the end what narrows
	// the tasks and how they show.
	ui.Row(c.Key("tasks-head")).MinHeight(82).Padding(16, 0, 10).Gap(10).AlignItems(ui.End).Shrink(0).FillWidth().DragWindow().Children(func() {
		ui.Column(c).Gap(3).Grow(1).Shrink(1).MinWidth(0).Children(func() {
			name := "今天"
			for _, l := range taskLists {
				if l.view == a.tasks.view {
					name = l.name
				}
			}
			switch a.tasks.view {
			case tasksWaiting:
				name = "等着归位"
			case tasksReview:
				name = "本周回顾"
			case tasksLate:
				name = "已逾期"
			case tasksAgents:
				name = "AI 动态"
			case tasksWeek, tasksMonth, tasksDay:
				name = "日程"
			}
			if a.tasks.onMap != "" {
				name = mapName(a.tasks.onMap)
			}
			ui.Row(c).Gap(10).AlignItems(ui.End).Children(func() {
				ui.Text(c, name).FontSize(26).FontWeight(700).LetterSpacing(-0.7).LineHeight(1.2).NoWrap()
				if path := a.tasks.onMap; path != "" { // a project's tasks: the way to its map, and to its record
					record := a.tasks.view == tasksAgents
					if record {
						ui.Text(c, "项目记录").FontSize(textBody).TextColor(ink2(th)).Margin(0, 0, 4).NoWrap()
					}
					ui.Row(c).Gap(4).Margin(0, 0, 2).Children(func() {
						if textLink(c, "打开导图 ›", "打开导图").Clicked() {
							a.later(func() { _ = a.open(path) })
						}
						switch {
						case record:
							if textLink(c, "任务 ›", "看这个项目的任务").Clicked() {
								a.tasks.view = tasksAll
							}
						case len(a.mapFile(path).LogOrNil()) > 0:
							if textLink(c, "项目记录 ›", "AI 在这个项目上做过什么、定过什么").Clicked() {
								a.later(func() { a.showAgents(path) })
							}
						}
					})
					return
				}
				ui.Text(c, fmt.Sprintf("%d月%d日 %s", now.Month(), now.Day(), weekdays[now.Weekday()])).FontSize(textBody).TextColor(ink2(th)).Margin(0, 0, 4).NoWrap()
			})
			if path := a.tasks.onMap; path != "" {
				open, done := 0, 0
				for _, it := range all { // as the left column counts them (countsAsTask)
					switch {
					case it.path != path:
					case it.done():
						done++
					case countsAsTask(it):
						open++
					}
				}
				if a.tasks.view != tasksAgents { // the record's figures head its own page
					figures(c, figure{open, "要做"}, figure{done, "做完"}).Margin(6, 0, 0)
				}
				return
			}
			if a.tasks.view == tasksReview || a.tasks.view == tasksAgents { // its own figures, of the week, head its page
				return
			}
			// The day in three figures: the figure in the ink, what it counts faint.
			figures(c, figure{len(a.dueToday(now)), "要做"}, figure{len(a.doneToday(now)), "做完"}, figure{len(a.waiting()), "等着归位"}).Margin(6, 0, 0).
				Label(fmt.Sprintf("%d 件要做，做完 %d 件，%d 条等着归位", len(a.dueToday(now)), len(a.doneToday(now)), len(a.waiting())))
		})
		if a.cfg.HideTree { // with no left column, the map to narrow by is here
			a.taskMapMenu(c, all)
		}
		if a.tasks.view == tasksWeek || a.tasks.view == tasksMonth || a.tasks.view == tasksDay {
			a.calendarNav(c)
		}
		mode := 0
		switch a.tasks.view {
		case tasksDay:
			mode = 1
		case tasksWeek:
			mode = 2
		case tasksMonth:
			mode = 3
		}
		if m := tabGroup(c, "tasks-mode", []string{"列表", "日", "周", "月"}, mode); m != mode {
			a.tasks.view = []int{tasksToday, tasksDay, tasksWeek, tasksMonth}[m]
		}

	})
}

// tasksAdd is the line to add a task, which says the day and the
// priority it reads as it is written.
func (a *app) tasksAdd(c *ui.Context, now time.Time) {
	t := c.Theme()
	box := captureBox(c, "tasks-add")
	box.Children(func() {
		glyph(c, iconPlus, 15, t.Accent)
		// The day a line with none goes on, said where it is not today's
		// list's own: the calendars each have one, and the week's was
		// nowhere to be read.
		day := a.addDay(now)
		ph := "添加任务，如「明天 写周报 !高」"
		switch a.tasks.view {
		case tasksDay, tasksWeek:
			ph = "添加任务，不写日期就记在" + dayLabel(*day, now) + "；也可以在下面空白的时间格直接新建"
		case tasksMonth:
			ph = "添加任务，不写日期就记在" + dayLabel(*day, now)
		}
		in := ui.TextInputBase(c.Key("task-add"), &a.tasks.add).Placeholder(ph).FontSize(textBody + 1).Grow(1).Label("添加任务")
		captureFocus(c, box, in)
		// What the line says, as it is written: its day and time, how it
		// repeats, its priority, each as it will be kept (readTaskLine).
		if strings.TrimSpace(a.tasks.add) != "" {
			tl := readTaskLine(a.tasks.add, now, day, "", "")
			if when := tl.day(now); when != "" {
				pill(c, when, t.Background, t.Text)
			}
			if tl.repeat != nil {
				pill(c, tl.repeat.Label(), t.Background, t.Text)
			}
			if tl.prio != model.PriorityNone {
				pill(c, tl.prio.Label(), t.Background, taskPriorityColor(t, tl.prio))
			}
			ui.Text(c, "↩").FontSize(textCaption).TextColor(t.TextMuted)
		}
		if in.Submitted() && strings.TrimSpace(a.tasks.add) != "" {
			line := a.tasks.add
			a.tasks.add = ""
			a.later(func() { a.addTask(line, day) })
		}
	})
}

// addDay is the day a task written in the line to add one goes on where
// its line says none: on today's list, today; in the day shown, that day;
// in the week, today when it is in the week shown, else the week's first
// day (it was the day last looked at, which the week does not show); in
// the month, the day chosen, said on the line; on the other lists, none.
func (a *app) addDay(now time.Time) *time.Time {
	var d time.Time
	switch a.tasks.view {
	case tasksToday:
		d = today(now)
	case tasksDay, tasksMonth:
		d = a.tasks.day
	case tasksWeek:
		d = a.weekOf(a.tasks.day)
		if t := today(now); !t.Before(d) && t.Before(d.AddDate(0, 0, 7)) {
			d = t
		}
	default:
		return nil
	}
	return &d
}

// taskMapMenu chooses the map whose tasks show alone, as TickTick's lists.
func (a *app) taskMapMenu(c *ui.Context, items []taskItem) {
	open := map[string]int{}
	for _, it := range items {
		if countsAsTask(it) {
			open[it.path]++
		}
	}
	label := "全部导图"
	if a.tasks.onMap != "" {
		label = mapName(a.tasks.onMap)
	}
	dropButton(c, "tasks-map", label, "按导图筛选", "只看一张导图的任务").Menu(func(m *ui.Menu) {
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
	})
}

// selectTask shows a task's details, saving what was typed in those of
// the one before.
func (a *app) showTask(it taskItem) {
	a.saveTaskDrafts()
	a.tasks.sel = it.key()
	a.tasks.title, a.tasks.note, a.tasks.draftOf, a.tasks.subtask = it.node.Title, it.node.Note, it.key(), ""
	a.tasks.titleWas, a.tasks.noteWas = it.node.Title, it.node.Note
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
		// Only what was typed is kept: a draft as it was taken says
		// nothing, and written back it undid what an agent or the sync
		// had changed of the task meanwhile (and, a name that was only
		// trimmed never being the task's, asked for a frame without end).
		title, note := strings.TrimSpace(a.tasks.title), a.tasks.note
		// Trimmed on both sides: a name kept with a space at its end was
		// written again, trimmed, by details merely opened.
		setTitle := title != "" && title != strings.TrimSpace(a.tasks.titleWas) && title != strings.TrimSpace(it.node.Title)
		setNote := note != a.tasks.noteWas && note != it.node.Note
		nowTitle, nowNote := it.node.Title, it.node.Note
		if setTitle || setNote {
			a.editTask(it, "改任务", func(n *model.Node) {
				if setTitle {
					n.Title = title
				}
				if setNote {
					n.Note = note
				}
			})
		}
		// The draft is of the task as it now is: what was not typed
		// follows it, a name emptied is the task's again.
		if setTitle {
			a.tasks.titleWas = title
		} else {
			a.tasks.title, a.tasks.titleWas = nowTitle, nowTitle
		}
		if setNote {
			a.tasks.noteWas = note
		} else {
			a.tasks.note, a.tasks.noteWas = nowNote, nowNote
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
		a.tasks.titleWas, a.tasks.noteWas = it.node.Title, it.node.Note
	}
	// Escape puts the details away, as everything that opens; while
	// something is written in it, it first leaves the field (escRest).
	if !a.barOpen && c.Shortcut(0, ui.KeyEscape) {
		if c.FocusTakesText() {
			a.escLeave = true
		} else {
			a.saveTaskDrafts()
			a.tasks.sel, a.tasks.draftOf = "", ""
		}
	}
	a.escRest(c)
	ui.Column(c.Key("task-detail")).Grow(1).Basis(0).MinHeight(0).FillWidth().Children(func() {
		// Its name heads the pane, in the title bar's band: the circle that
		// ticks it off, the name to change, and the way out.
		ui.Row(c.Key("task-detail-head")).Height(pageHeadHeight).Padding(16, 12, 0, 20).Gap(10).AlignItems(ui.Center).Shrink(0).Children(func() {
			a.taskCircle(c, it, 20)
			in := inlineInput(c, &a.tasks.title, textTitle, true, "任务名", "任务名").Grow(1).Shrink(1).MinWidth(0)
			if in.Submitted() || !in.Focused() && (strings.TrimSpace(a.tasks.title) != strings.TrimSpace(a.tasks.titleWas) || it.node.Title != a.tasks.titleWas) {
				a.later(a.saveTaskDrafts)
			}
			if iconButton(c, iconClose, "关闭详情", "关闭", 26, true).Clicked() {
				a.saveTaskDrafts()
				a.tasks.sel, a.tasks.draftOf = "", ""
			}
		})
		ui.Scroll(c.Key("task-detail-scroll")).Grow(1).Children(func() {
			ui.Column(c).Padding(6, 20, 24).Gap(16).FillWidth().Children(func() {
				ui.Column(c.Key("props")).Gap(12).FillWidth().Children(func() {
					propRow(c, "状态", func() {
						st := slices.Index(model.Statuses, task.Status)
						if n := segmented(c, "task-status", "状态", []string{"想法", "待做", "进行中", "完成"}, st); n != st {
							s := model.Statuses[n]
							a.later(func() { a.editTask(it, "改状态", func(n *model.Node) { n.SetStatus(s, a.now()) }) })
						}
					})
					propRow(c, "优先级", func() {
						p := int(task.Priority)
						if n := segmented(c, "task-prio", "优先级", []string{"无", "低", "中", "高"}, p); n != p {
							pr := model.Priority(n)
							a.later(func() { a.editTask(it, "改优先级", func(n *model.Node) { n.Task.Priority = pr }) })
						}
					})
					dateProp(c, "日期", "截止日期", task.Due, a.now(), func(d *time.Time) { a.later(func() { a.setTaskDay(it, d) }) })
					a.timeProp(c, it)
					a.repeatProp(c, it)
					a.remindProp(c, it)
				})
				section(c, iconPencil, "备注", nil, func() {
					in := area(c.Key("task-note"), &a.tasks.note).Lines(3, 14).FontSize(textBody).FillWidth().
						Placeholder("写点细节、链接、想法…").Label("任务备注")
					if !in.Focused() && (a.tasks.note != a.tasks.noteWas || it.node.Note != a.tasks.noteWas) {
						a.later(a.saveTaskDrafts)
					}
				})
				a.subtasks(c, it)
				a.taskThoughts(c, it)
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
		in := ui.TextInputBase(c.Key("sub-add"), &a.tasks.subtask).Placeholder("+ 添加子任务").FontSize(textBody).
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
		s := a.weekOf(d)
		e := s.AddDate(0, 0, 6)
		label = fmt.Sprintf("%d月%d日 – %d月%d日", s.Month(), s.Day(), e.Month(), e.Day())
	default:
		label = fmt.Sprintf("%d年%d月", d.Year(), d.Month())
	}
	ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
		if iconButton(c, iconBack, "上一个", "往前", 26, true).Clicked() {
			step(-1)
		}
		ui.Text(c, label).FontSize(textBody).FontWeight(700).MinWidth(120).TextAlign(ui.Center)
		if iconButton(c, iconForward, "下一个", "往后", 26, true).Clicked() {
			step(1)
		}
		if ghostButton(c, "今天", "回到今天").Clicked() {
			a.tasks.day = today(a.now())
		}
	})
	// ← and → go back and forth, while nothing is written in.
	if !c.FocusTakesText() && a.tasks.draft == nil {
		if c.Shortcut(0, ui.KeyLeft) {
			step(-1)
		}
		if c.Shortcut(0, ui.KeyRight) {
			step(1)
		}
	}
	_ = t
}

// taskList is the lists: today (with what is overdue), the next 7 days a
// day each, all by when, a project's, the inbox, what is late, what is
// done.
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
	view := a.tasks.view
	if a.tasks.onMap != "" && (view == tasksToday || view == tasksWeekAhead || view == tasksAll) {
		view = tasksOfMap // a project's: all that is not done, by when
	}
	// What is listed is what is counted: a task to do (countsAsTask: an
	// idea with no day is none yet), late by the one rule (late), and done
	// today whether it has a day or not, one that repeats and was done a
	// time among them (doneOn), as the page's head and the left column
	// count them.
	switch view {
	case tasksOfMap:
		over, todayG, later, undated := add("已逾期", true, at(day)), add("今天", false, at(day)), add("之后", false, nil), add("没定日期", false, nil)
		for _, it := range items {
			if it.doneOn(now) {
				done = append(done, it)
			}
			d := it.date()
			switch {
			case !countsAsTask(it):
			case d == nil:
				undated.items = append(undated.items, it)
			case d.Before(day):
				over.items = append(over.items, it)
			case sameDay(*d, day):
				todayG.items = append(todayG.items, it)
			default:
				later.items = append(later.items, it)
			}
		}
	case tasksToday:
		over, todayG := add("已逾期", true, at(day)), add("今天", false, at(day))
		var next []taskItem
		for _, it := range items {
			if it.doneOn(now) {
				done = append(done, it)
			}
			d := it.date()
			switch {
			case d == nil || !countsAsTask(it):
			case d.Before(day):
				over.items = append(over.items, it)
			case sameDay(*d, day):
				todayG.items = append(todayG.items, it)
			case d.Before(day.AddDate(0, 0, 8)): // tomorrow and the six days after it: seven
				next = append(next, it)
			}
		}
		// What comes in the days after, under today's: the page is not
		// left half empty, and tomorrow is seen coming.
		if len(next) > 0 {
			add("接下来 7 天", false, at(day.AddDate(0, 0, 1))).items = next // dropped there, a task is tomorrow's
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
			} else if i := daysBetween(day, *d); i < 7 {
				days[i].items = append(days[i].items, it)
			}
		}
	case tasksLate:
		g := add("已逾期", true, at(day))
		for _, it := range items {
			if it.late(day) {
				g.items = append(g.items, it)
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
			if it.done() || it.doneAt() != nil { // one that repeats, by when it was last done
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
		// After tomorrow, the rest of the seven days from today on, by
		// their name: it said 未来 7 天 and held six.
		tomorrow, week := add("明天", false, at(day.AddDate(0, 0, 1))), add("7 天内", false, nil)
		later, none := add("之后", false, nil), add("没定日期", false, nil)
		for _, it := range items {
			if it.done() || it.doneAt() != nil {
				done = append(done, it)
			}
			if !countsAsTask(it) {
				continue // done, or an idea, not yet a task to do
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
	ui.Box(c.Key(fmt.Sprint("task-list:", a.tasks.view))).FillWidth().Children(func() {
		ui.Column(c).Padding(6, 0, 8).Gap(10).FillWidth().Children(func() {
			if empty && len(done) == 0 {
				emptyState(c, "tasks", "circle-check", "没有要做的事", "在上面写一句加任务，或在导图里把节点设为任务、给它一个日期", nil)
				return
			}
			if a.tasks.view == tasksInbox {
				a.placeBar(c, len(groups[0].items))
			}
			if a.tasks.table {
				taskTableHead(c, a.tasks.onMap != "")
			}
			for _, g := range groups {
				// Empty, a group of a day still shows, quietly: a place to drop
				// a task on; others empty do not.
				if len(g.items) == 0 && (g.danger || g.drop == nil) && len(groups) > 1 {
					continue
				}
				// On today's list and a project's, a group with nothing is not
				// named: only the week ahead keeps its empty days, to drop on.
				if len(g.items) == 0 && (view == tasksToday || view == tasksOfMap) && !empty {
					continue
				}
				// The whole group, its heading and its tasks, takes a task
				// dropped on it, to its day.
				box := tray(c, "group:"+g.title)
				if g.drop != nil {
					a.dropTaskOn(box, *g.drop)
				}
				box.Children(func() {
					ui.Row(c).Height(32).Padding(6, 8, 0).Gap(6).AlignItems(ui.Center).FillWidth().Children(func() {
						// A group's name is small and quiet: the tasks are what is read.
						col := t.TextMuted
						switch {
						case g.danger:
							col = t.Danger
						case len(g.items) == 0:
							col = faint(t)
						}
						ui.Text(c, g.title).FontSize(textCaption).FontWeight(700).LetterSpacing(0.3).TextColor(col)
						if len(g.items) > 0 {
							ui.Text(c, fmt.Sprint(len(g.items))).FontSize(textCaption).TextColor(faint(t)).FontFeatures("tnum")
						}
						if g.danger && len(g.items) > 0 {
							// What is late is put on today in one go, as TickTick's 顺延.
							ui.Spacer(c)
							items := g.items
							move := ui.Box(c.Key("postpone")).Height(24).Padding(0, 8).Radius(radiusSmall).Center().Cursor(ui.CursorPointer).Shrink(0).
								Label("顺延到今天").Tooltip(fmt.Sprintf("把这 %d 件逾期的任务都改到今天", len(items))).
								Background(t.Accent.Alpha(0)).Transition(easeLook)
							if move.Hovered() {
								move.Background(t.Accent.Alpha(0.09))
							}
							if move.Clicked() {
								a.later(func() { a.postponeToToday(items) })
							}
							move.Children(func() {
								ui.Text(c, "顺延到今天").FontSize(textCaption).FontWeight(700).TextColor(t.Accent).NoWrap().PassThrough()
							})
						}
					})
					if len(g.items) > 0 {
						listCard(c, "rows").Children(func() {
							for _, it := range g.items {
								if view == tasksDone {
									a.doneRow(c, it)
								} else {
									a.taskRow(c, it, g.danger)
								}
							}
						})
					}
				})
			}
			if len(done) > 0 {
				tray(c, "done-tray").Children(func() {
					head := ui.Row(c.Key("group:done")).Height(34).Padding(0, 10, 2).Gap(6).AlignItems(ui.Center).FillWidth().Cursor(ui.CursorPointer).
						Label("已完成")
					if head.Clicked() {
						a.tasks.showDone = !a.tasks.showDone
					}
					head.Children(func() {
						ui.Text(c, fmt.Sprintf("已完成 %d", len(done))).FontSize(textBody).FontWeight(700).TextColor(t.TextMuted)
						ui.Spacer(c)
						word := "展开"
						if a.tasks.showDone {
							word = "收起"
						}
						ui.Text(c, word).FontSize(textSmall).TextColor(t.TextMuted)
					})
					if a.tasks.showDone {
						listCard(c, "done-rows").Children(func() {
							for _, it := range done {
								a.doneRow(c, it)
							}
						})
					}
				})
			}
			a.taskKeys(c, func(yield func(taskItem) bool) {
				for _, g := range groups {
					for _, it := range g.items {
						if view == tasksDone && !it.done() {
							continue // done a time: not a task of this list to tick
						}
						if !yield(it) {
							return
						}
					}
				}
				if a.tasks.showDone {
					for _, it := range done {
						if it.done() && !yield(it) {
							return
						}
					}
				}
			})
			// The list's foot says how much it holds, as a table's does.
			// One that repeats, done a time and listed again to do, is one.
			listed := map[string]bool{}
			for _, g := range groups {
				for _, it := range g.items {
					listed[it.key()] = true
				}
			}
			for _, it := range done {
				listed[it.key()] = true
			}
			if len(listed) > 0 {
				foot := fmt.Sprintf("共 %d 件", len(listed))
				if len(done) > 0 {
					foot += fmt.Sprintf(" · 做完 %d 件", len(done))
				}
				ui.Row(c).Height(36).Padding(0, 8).AlignItems(ui.Center).FillWidth().Children(func() {
					ui.Text(c, foot).FontSize(textCaption).TextColor(faint(t)).FontFeatures("tnum")
				})
			}
		})
	})
}

// remindProp is when a task reminds of itself: a time of its day, three
// as keys and the others under one, as how it repeats.
func (a *app) remindProp(c *ui.Context, it taskItem) {
	t := c.Theme()
	task := it.node.Task
	set := func(r string) {
		a.later(func() { a.editTask(it, "改提醒", func(n *model.Node) { n.Task.Remind = r }) })
	}
	propRow(c, "提醒", func() {
		if task.Due == nil {
			ui.Text(c, "先定个日期，再选几点提醒").FontSize(textSmall).TextColor(faint(t))
			return
		}
		chip := func(key, text, label string, on bool) bool {
			b := ui.Box(c.Key(key)).Height(28).Padding(0, 12).Radius(radiusPill).Center().Cursor(ui.CursorPointer).Label(label).Shrink(0).
				Background(t.Text.Alpha(0.05)).Transition(easeLook)
			switch {
			case on:
				b.Background(t.Accent.Alpha(0.14))
			case b.Hovered():
				b.Background(t.Text.Alpha(0.09))
			}
			press(b, 0.96)
			b.Children(func() {
				txt := ui.Text(c, text).FontSize(textSmall).NoWrap().PassThrough().FontFeatures("tnum")
				if on {
					txt.FontWeight(700).TextColor(t.Accent.MixOklab(t.Text, 0.3))
				}
			})
			return b.Clicked()
		}
		if chip("remind:none", "不提醒", "提醒 不提醒", task.Remind == "") {
			set("")
		}
		for _, r := range remindTimes[:3] {
			if chip("remind:"+r, strings.TrimPrefix(r, "0"), "提醒 "+r, task.Remind == r) {
				set(r)
			}
		}
		more := "更多"
		if task.Remind != "" && !slices.Contains(remindTimes[:3], task.Remind) {
			more = strings.TrimPrefix(task.Remind, "0")
		}
		dropButton(c, "remind-more", more, "提醒时间", "别的时间").Menu(func(m *ui.Menu) {
			for _, r := range remindTimes[3:] {
				if m.Item(strings.TrimPrefix(r, "0")).Checked(task.Remind == r).Chosen() {
					set(r)
				}
			}
			m.Separator()
			if m.Item("自定义…").Chosen() {
				a.tasks.remindOwn, a.tasks.remindText = it.key(), strings.TrimPrefix(task.Remind, "0")
			}
		})
	})
	// A time of one's own, written: 15:30, 下午3点半.
	if a.tasks.remindOwn == it.key() && task.Due != nil {
		ui.Row(c.Key("remind-own")).Padding(0, 0, 0, 52).Gap(8).AlignItems(ui.Center).FillWidth().Children(func() {
			in := ui.TextInputBase(c.Key("remind-text"), &a.tasks.remindText).Placeholder("如 15:30").Label("自定义提醒时间").AutoFocus().
				FontSize(textSmall).Width(96).Padding(5, 8).Radius(radiusSmall).Background(blockFill(t))
			if in.Submitted() {
				if _, r := parseRemind(strings.TrimSpace(a.tasks.remindText)); r != "" {
					set(r)
					a.tasks.remindOwn = ""
				} else {
					a.tasks.remindBad = true
				}
			}
			if in.Changed() {
				a.tasks.remindBad = false
			}
			if a.tasks.remindBad {
				ui.Text(c, "没看懂这个时间，写成 15:30 这样").FontSize(textCaption).TextColor(t.Danger)
			} else {
				ui.Text(c, "写好后记下").FontSize(textCaption).TextColor(faint(t))
			}
		})
	}
}

// setTaskTime gives a task its time of day, from and until as 15:04, or
// none (from ""): of the whole day. When it reminds follows where it was
// the time it had, or nothing: to the new time, or, of the whole day, off.
// A time to remind at chosen apart from it stays.
func setTaskTime(task *model.Task, from, until string) {
	was := task.Time
	task.Time, task.EndTime = from, until
	switch {
	case from == "" && was != "" && task.Remind == was:
		task.Remind = ""
	case from != "" && (task.Remind == "" || task.Remind == was):
		task.Remind = from
	}
}

// clockMinutes is a time of day written 15:04 as minutes from midnight,
// and clockText the other way.
func clockMinutes(s string) int {
	var h, m int
	fmt.Sscanf(s, "%d:%d", &h, &m)
	return h*60 + m
}

func clockText(m int) string { return fmt.Sprintf("%02d:%02d", m/60, m%60) }

// spanLengths are the lengths offered for a task's time, in minutes.
var spanLengths = []int{30, 60, 90, 120, 180}

func spanLengthName(m int) string {
	switch {
	case m < 60:
		return fmt.Sprintf("%d 分钟", m)
	case m%60 == 0:
		return fmt.Sprintf("%d 小时", m/60)
	}
	return fmt.Sprintf("%.1f 小时", float64(m)/60)
}

// timeProp is when in its day a task is done: the whole day, or from a
// time, to another or not, as TickTick's: the beginning chosen by the half
// hour, morning, afternoon and evening each a submenu, the end by how long
// it takes; or written, 9:00-10:30, 下午3点到5点. A time newly given is
// when it reminds too, where it reminded of nothing or of the time before;
// and made of the whole day, it reminds no more at the time it had (it
// went on saying 今天 9:00 on its row, and reminding then).
func (a *app) timeProp(c *ui.Context, it taskItem) {
	t := c.Theme()
	task := it.node.Task
	set := func(from, until string) {
		a.later(func() {
			a.editTask(it, "改时间", func(n *model.Node) { setTaskTime(n.Task, from, until) })
		})
	}
	propRow(c, "时间", func() {
		if task.Due == nil {
			ui.Text(c, "先定个日期，再定几点").FontSize(textSmall).TextColor(faint(t))
			return
		}
		if task.Time != "" || a.tasks.spanOwn == it.key() {
			if chipButton(c, "全天", "时间 全天", "不定几点，这一天做").Clicked() {
				a.tasks.spanOwn = ""
				set("", "")
			}
		} else {
			ui.Text(c, "全天").FontSize(textSmall).TextColor(t.TextMuted).Padding(0, 4, 0, 0)
		}
		from := "几点开始"
		if task.Time != "" {
			from = strings.TrimPrefix(task.Time, "0")
		}
		dropButton(c, "time-from", from, "开始时间", "从几点开始").Menu(func(m *ui.Menu) {
			for _, part := range []struct {
				name     string
				from, to int
			}{{"上午", 6, 12}, {"下午", 12, 18}, {"晚上", 18, 24}} {
				m.Submenu(part.name, func(m *ui.Menu) {
					for h := part.from; h < part.to; h++ {
						for _, mm := range []int{0, 30} {
							at := clockText(h*60 + mm)
							if m.Item(strings.TrimPrefix(at, "0")).Checked(task.Time == at).Chosen() {
								until := ""
								if task.EndTime != "" { // as long as it was
									if e := clockMinutes(at) + clockMinutes(task.EndTime) - clockMinutes(task.Time); e < 24*60 {
										until = clockText(e)
									}
								}
								set(at, until)
							}
						}
					}
				})
			}
			m.Separator()
			if m.Item("自定义…").Chosen() {
				a.tasks.spanOwn, a.tasks.spanText, a.tasks.spanBad = it.key(), task.Span(), false
			}
		})
		if task.Time == "" {
			return
		}
		ui.Text(c, "到").FontSize(textSmall).TextColor(t.TextMuted)
		until := "不定结束"
		if task.EndTime != "" {
			until = strings.TrimPrefix(task.EndTime, "0")
		}
		dropButton(c, "time-until", until, "结束时间", "做多久").Menu(func(m *ui.Menu) {
			begin := clockMinutes(task.Time)
			for _, l := range spanLengths {
				if begin+l >= 24*60 {
					break
				}
				at := clockText(begin + l)
				if m.Item(strings.TrimPrefix(at, "0") + "（" + spanLengthName(l) + "）").Checked(task.EndTime == at).Chosen() {
					set(task.Time, at)
				}
			}
			m.Separator()
			if m.Item("不定结束").Checked(task.EndTime == "").Chosen() {
				set(task.Time, "")
			}
		})
	})
	// A time of one's own, written: 9:00-10:30, 下午3点到5点.
	if a.tasks.spanOwn == it.key() && task.Due != nil {
		ui.Row(c.Key("span-own")).Padding(0, 0, 0, 52).Gap(8).AlignItems(ui.Center).FillWidth().Children(func() {
			in := ui.TextInputBase(c.Key("span-text"), &a.tasks.spanText).Placeholder("如 9:00-10:30").Label("自定义时间").AutoFocus().
				FontSize(textSmall).Width(132).Padding(5, 8).Radius(radiusSmall).Background(blockFill(t))
			if in.Submitted() {
				if _, from, until := parseClock(strings.TrimSpace(a.tasks.spanText)); from != "" {
					set(from, until)
					a.tasks.spanOwn = ""
				} else {
					a.tasks.spanBad = true
				}
			}
			if in.Changed() {
				a.tasks.spanBad = false
			}
			if a.tasks.spanBad {
				ui.Text(c, "没看懂，写成 9:00-10:30 这样").FontSize(textCaption).TextColor(t.Danger)
			} else {
				ui.Text(c, "写好后记下").FontSize(textCaption).TextColor(faint(t))
			}
		})
	}
}

// countsAsTask reports whether a task not done is counted among what is
// to do: an idea with no day is not, wherever it is counted (the lists'
// counts and the projects' are one count: 全部 said 12 beside a project
// of 24).
func countsAsTask(it taskItem) bool {
	return !it.done() && !(it.node.Task.Status == model.StatusIdea && it.date() == nil)
}

// taskKeys is the list's keys: the arrows go from a task to the next and
// the one before, its details following; the space bar ticks off the one
// chosen. Not while something is written in.
func (a *app) taskKeys(c *ui.Context, shown func(yield func(taskItem) bool)) {
	if a.barOpen || c.FocusTakesText() {
		return
	}
	step := 0
	switch {
	case c.Shortcut(0, ui.KeyDown):
		step = 1
	case c.Shortcut(0, ui.KeyUp):
		step = -1
	}
	tick := c.Shortcut(0, ui.KeySpace)
	if step == 0 && !tick {
		return
	}
	var list []taskItem
	at := -1
	for it := range shown {
		if it.key() == a.tasks.sel {
			at = len(list)
		}
		list = append(list, it)
	}
	if len(list) == 0 {
		return
	}
	if tick {
		if at >= 0 {
			it := list[at]
			a.later(func() { a.setTaskDone(it, !it.done()) })
		}
		return
	}
	switch {
	case at < 0 && step > 0:
		at = 0
	case at < 0:
		at = len(list) - 1
	default:
		at = min(max(at+step, 0), len(list)-1)
	}
	it := list[at]
	a.later(func() { a.showTask(it); a.tasks.reveal = it.key() })
}

// taskRow is a task in a list: its circle, in its priority's color, which
// ticks it off; its title, which opens it in its map; its map and day.
// It is a row of its group's card.
func (a *app) taskRow(c *ui.Context, it taskItem, overdue bool) { a.taskRowAs(c, it, overdue, false) }

// doneRow is a task among those done: one ticked off as any row; one
// that repeats, done a time and to do again, as done that time: ticked,
// its words struck, and after them when it comes next, with its ↻. The
// task itself is in its list, on that day, to tick.
func (a *app) doneRow(c *ui.Context, it taskItem) { a.taskRowAs(c, it, false, !it.done()) }

// taskRowAs is taskRow; once shows a task that repeats as done a time.
func (a *app) taskRowAs(c *ui.Context, it taskItem, overdue, once bool) {
	t := c.Theme()
	task := it.node.Task
	key, label := "task:", "任务 "
	if once {
		key, label = "once:", "做完一次 "
	}
	row := ui.Row(c.Key(key+it.path+"#"+it.node.ID)).MinHeight(54).Padding(8, 6).Gap(12).AlignItems(ui.Center).
		FillWidth().Cursor(ui.CursorPointer).Label(label+it.node.Title).
		BorderWidth(0, 0, 1, 0).BorderColor(t.Border.Alpha(0.55)).Background(t.Text.Alpha(0)).Transition(rowMotion)
	if !once {
		row.Drag(taskDrag{it.path, it.node.ID})
	}
	if row.Hovered() {
		row.Background(t.Text.Alpha(0.035))
	}
	switch {
	case row.DoubleClicked():
		a.later(func() { a.libOpen = false; a.goToNode(it.path, it.sheet, it.node.ID) })
	case row.Clicked():
		a.later(func() { a.showTask(it) })
	}
	if a.tasks.sel == it.key() && !once {
		// Chosen, it is a round block; its line goes, keeping its room.
		row.Background(chosen(t)).Radius(radiusRow).BorderColor(t.Text.Alpha(0))
		if a.tasks.reveal == it.key() { // gone to by the keyboard, it is brought into view
			row.ScrollIntoView()
			a.tasks.reveal = ""
		}
	}
	row.ContextMenu(func(m *ui.Menu) { a.taskMenu(m, it) })
	table := a.tasks.table
	row.Children(func() {
		if once { // done that time: a mark, not a key (the task to tick is in its list)
			ui.Box(c.Key("ticked")).Size(18, 18).Shrink(0).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) {
				rr := ui.Rect{X: r.X + 1, Y: r.Y + 1, W: r.W - 2, H: r.H - 2}
				p.Fill(rr, t.TextMuted.Alpha(0.5), rr.W/2)
				drawIcon(p, r, 18*0.62, iconCheck, t.Background)
			})
		} else {
			a.taskCircle(c, it, 18)
		}
		ui.Column(c).Grow(1).Shrink(1).Basis(0).MinWidth(0).Gap(2).PassThrough().Children(func() {
			// Its words, and after them what it is marked with: how
			// pressing, and its state when that is not the plain one.
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				title := ui.Text(c, it.node.Title).FontSize(textBody).SingleLine().Shrink(1).MinWidth(0)
				if it.done() || once {
					title.TextColor(t.TextMuted).Strikethrough()
				}
				if !once {
					taskPills(c, it)
				}
			})
			sub := it.mapName
			if it.where != "" && it.where != it.mapName {
				sub += " · " + it.where
			}
			if table { // its map has a column of its own: here only where in it
				if it.where != "" && it.where != it.mapName && a.tasks.onMap == "" {
					ui.Text(c, it.where).FontSize(textCaption).TextColor(t.TextMuted).SingleLine()
				}
				return
			}
			ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
				mapDot(c, a.mapColor(it.path), 6) // its map's color
				ui.Text(c, sub).FontSize(textCaption).TextColor(t.TextMuted).SingleLine().Shrink(1).MinWidth(0)
			})
		})
		if table {
			// The column of its project: its map, or, on a project's own
			// page, where in the map.
			ui.Row(c).Width(taskColProject).Shrink(0).Gap(7).AlignItems(ui.Center).PassThrough().Children(func() {
				if a.tasks.onMap != "" {
					ui.Text(c, it.where).FontSize(textSmall).TextColor(t.TextMuted).SingleLine().Shrink(1).MinWidth(0)
					return
				}
				mapDot(c, a.mapColor(it.path), 7)
				ui.Text(c, it.mapName).FontSize(textSmall).TextColor(ink2(t)).SingleLine().Shrink(1).MinWidth(0)
			})
		}
		if p, ok := a.tasks.places[it.node.ID]; ok && !once { // where the AI would put it
			ui.Text(c, "→ "+p.label).FontSize(textSmall).TextColor(t.Accent).SingleLine().Shrink(2).MinWidth(60).PassThrough()
			if ghostButton(c.Key("take:"+it.node.ID), "放过去", "移到「"+p.label+"」下").Label("放过去 " + it.node.Title).Clicked() {
				a.later(func() { a.takePlace(it) })
			}
			if iconButton(c.Key("skip:"+it.node.ID), iconClose, "不放 "+it.node.Title, "不采纳这个建议", 22, true).Clicked() {
				delete(a.tasks.places, it.node.ID)
			}
		}
		if d := it.date(); d != nil {
			col := t.TextMuted
			if !once && (overdue || !it.done() && d.Before(today(a.now()))) {
				col = t.Danger
			}
			label := dayLabel(*d, a.now())
			switch {
			case once: // done this time: the day it comes next
				label = "下次 " + label
			case task.Due == nil:
				label = "开始 " + label
			}
			if span := task.Span(); span != "" && task.Due != nil && !once {
				label += " " + span // when in its day
			} else if task.Remind != "" && task.Due != nil && !it.done() && !once {
				label += " " + strings.TrimPrefix(task.Remind, "0") // when it reminds
			}
			if task.Repeat != nil {
				label += " ↻"
			}
			if table { // the column of its day, its words at the column's end
				ui.Row(c).Width(taskColDay).Shrink(0).PassThrough().Children(func() {
					ui.Spacer(c)
					if col == t.Danger {
						ui.Box(c).Padding(2, 8).Radius(6).Background(t.Danger.Alpha(0.1)).Children(func() {
							ui.Text(c, label).FontSize(textCaption).TextColor(t.Danger).FontFeatures("tnum").NoWrap()
						})
					} else {
						ui.Text(c, label).FontSize(textSmall).TextColor(col).FontFeatures("tnum").NoWrap()
					}
				})
			} else if col == t.Danger {
				// Late, its day is a small pale tag.
				ui.Box(c).Padding(2, 8).Radius(6).Background(t.Danger.Alpha(0.1)).Shrink(0).PassThrough().Children(func() {
					ui.Text(c, label).FontSize(textCaption).TextColor(t.Danger).FontFeatures("tnum").NoWrap()
				})
			} else {
				ui.Text(c, label).FontSize(textSmall).TextColor(col).FontFeatures("tnum").Shrink(0).PassThrough()
			}
		} else if table {
			// With no day, its column keeps its room: the project's stood
			// in it, out of line with the rows above.
			ui.Box(c).Width(taskColDay).Shrink(0).PassThrough()
		}
	})
}

// The widths of a task's columns, where the list has room to be a table.
const (
	taskColProject = 180
	taskColDay     = 104
	tableRoom      = 1000 // the room today's page needs for it
)

// taskTableHead names the columns of the tasks' table, once, over its
// groups: the task, its project (or, on a project's page, where in the
// map), its day.
func taskTableHead(c *ui.Context, ofMap bool) {
	t := c.Theme()
	second := "项目"
	if ofMap {
		second = "位置"
	}
	ui.Row(c.Key("table-head")).Height(28).Padding(0, 6).Gap(12).AlignItems(ui.Center).FillWidth().
		BorderWidth(0, 0, 1, 0).BorderColor(hairline(t)).Children(func() {
		name := func(s string) ui.Element {
			return ui.Text(c, s).FontSize(textCaption).FontWeight(700).LetterSpacing(0.3).TextColor(faint(t)).NoWrap()
		}
		ui.Box(c).Width(18).Shrink(0)
		ui.Box(c).Grow(1).Basis(0).Children(func() { name("任务") })
		ui.Box(c).Width(taskColProject).Shrink(0).Children(func() { name(second) })
		ui.Row(c).Width(taskColDay).Shrink(0).Children(func() {
			ui.Spacer(c)
			name("日期")
		})
	})
}

// taskPills are the small tags after a task's words: how pressing it is,
// when it is, in its priority's color, and its state when that is not the
// plain one (an idea, or being done).
func taskPills(c *ui.Context, it taskItem) {
	t := c.Theme()
	task := it.node.Task
	if task == nil || it.done() {
		return
	}
	// A priority has its color; a state is gray with a dot of its own, so
	// that a row has one color on it, not three.
	pill := func(key, text string, col ui.Color, dot bool) {
		bg, ink := tinted(t, col)
		if dot {
			bg, ink = t.Text.Alpha(0.06), ink2(t)
		}
		ui.Row(c.Key("pill:"+key)).Height(19).Padding(0, 7).Gap(4).Radius(6).AlignItems(ui.Center).Shrink(0).Background(bg).PassThrough().Children(func() {
			if dot {
				ui.Box(c).Size(5, 5).Radius(3).Background(col)
			}
			ui.Text(c, text).FontSize(11).FontWeight(700).TextColor(ink).NoWrap()
		})
	}
	if task.Priority != model.PriorityNone {
		pill("priority", task.Priority.Label(), taskPriorityColor(t, task.Priority), false)
	}
	switch task.Status {
	case model.StatusDoing:
		pill("state", "进行中", t.Success, true)
	case model.StatusIdea:
		pill("state", "想法", t.TextMuted, true)
	}
}

// taskCircle ticks a task off, or back on: an open circle in its
// priority's color, a check when done.
func (a *app) taskCircle(c *ui.Context, it taskItem, size float32) {
	t := c.Theme()
	col := taskPriorityColor(t, it.node.Task.Priority)
	done := it.done()
	box := ui.Box(c.Key("circle")).Size(size, size).Shrink(0).Cursor(ui.CursorPointer).Label("完成 " + it.node.Title)
	// Ticked, it fills from its middle and the check comes after; under
	// the pointer it shows a hint of the fill.
	target := float32(0)
	if done {
		target = 1
	}
	fill := box.AnimateTo("done", target, ui.Animation{Duration: motionFast, Ease: easeDetent, NonMotion: true})
	hint := box.AnimateTo("hover", b2f(box.Hovered() && !done), ui.Animation{Duration: motionQuick, NonMotion: true})
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		rr := ui.Rect{X: r.X + 1, Y: r.Y + 1, W: r.W - 2, H: r.H - 2}
		if hint > 0 {
			p.Fill(rr, col.Alpha(0.16*hint), rr.W/2)
		}
		if fill < 1 {
			p.Stroke(rr, col.Alpha(1-fill), rr.W/2, 1.6)
		}
		if fill > 0 {
			k := 0.5 + 0.5*fill // it grows as it fills
			in := ui.Rect{X: rr.X + rr.W*(1-k)/2, Y: rr.Y + rr.H*(1-k)/2, W: rr.W * k, H: rr.H * k}
			p.Fill(in, t.TextMuted.Alpha(0.5*fill), in.W/2)
			drawIcon(p, r, size*0.62*k, iconCheck, t.Background.Alpha(fill))
		}
	})
	press(box, 0.88)
	if box.Clicked() {
		a.later(func() { a.setTaskDone(it, !done) })
	}
}

func b2f(b bool) float32 {
	if b {
		return 1
	}
	return 0
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
	set := func(d *time.Time) { a.later(func() { a.moveTaskDay(it, d) }) } // said, with the way back
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
	m.Separator()
	if m.Item("移到导图…").Chosen() {
		a.later(func() { a.moveTaskTo(it) })
	}
	if m.Item("存为灵感").Disabled(len(a.taskClips(it.node)) > 0).Chosen() {
		a.later(func() { a.taskToMemo(it, true) })
	}
	if m.Item("改成灵感（去掉任务）").Chosen() {
		a.later(func() { a.taskToMemo(it, false) })
	}
	m.Separator()
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

// dropTaskOn makes el take tasks dropped on it, which go to the day, and
// say so with the way back (moveTaskDay).
func (a *app) dropTaskOn(el ui.Element, day time.Time) {
	t := a.theme()
	if _, over := ui.DragOver[taskDrag](el); over {
		el.Background(t.Accent.Alpha(0.12))
	}
	if d, ok := ui.Drop[taskDrag](el); ok {
		for _, it := range a.tasks.items {
			if it.path == d.path && it.node.ID == d.id {
				a.later(func() { a.moveTaskDay(it, &day) })
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

// monthView is the month of the day shown: a cell a day, a few of its
// tasks in it and how many more; a double click on a day shows it.
func (a *app) monthView(c *ui.Context, items []taskItem) {
	t := c.Theme()
	first := time.Date(a.tasks.day.Year(), a.tasks.day.Month(), 1, 0, 0, 0, 0, a.tasks.day.Location())
	start := a.weekOf(first)
	laid := a.monthLaid(items, start)
	ui.Column(c.Key("month")).Grow(1).Padding(0, 12, 12).Children(func() {
		ui.Row(c).Shrink(0).Children(func() {
			for i := range 7 {
				w := weekdays[start.AddDate(0, 0, i).Weekday()][len("周"):]
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
					list := laid.list[week*7+i]
					cell.Children(func() {
						ui.Row(c).Padding(2, 4).Gap(6).AlignItems(ui.Baseline).Children(func() {
							num := ui.Text(c, fmt.Sprint(d.Day())).FontSize(textSmall)
							switch {
							case sameDay(d, a.now()):
								num.TextColor(t.Accent).FontWeight(700)
							case !in:
								num.TextColor(t.TextMuted.Alpha(0.6))
							}
							lunarText(c, d, !in)
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
							a.taskChip(c, it, false)
						}
						if len(list) < 3 {
							for j, it := range laid.again[week*7+i] {
								if len(list)+j < 3 {
									a.taskGhost(c, it, d)
								}
							}
						}
					})
				}
			})
		}
	})
}

// monthLaid is the tasks of the month's forty-two days put on them: each
// day's own, as a calendar orders them (byTime), and those that come
// again on it. As the hours' (hoursLaid), it is worked out once for the
// tasks as gathered, the project they are narrowed to, the first day and
// today, not at each frame.
type monthLaid struct {
	of          hoursOf
	list, again [42][]taskItem
}

// monthLaid puts the tasks on the days from start (monthLaid the type).
func (a *app) monthLaid(items []taskItem, start time.Time) *monthLaid {
	of := hoursOf{a.tasks.serial, a.tasks.onMap, start, 42, today(a.now()), holidaysLoads()}
	if l := &a.memo.month; l.of == of && a.tasks.serial > 0 {
		return l
	}
	a.memo.laid++
	byDay := map[string][]taskItem{}
	again := map[string][]taskItem{} // those that come again on a day
	for _, it := range items {
		if d := it.date(); d != nil {
			k := d.Format("2006-01-02")
			byDay[k] = append(byDay[k], it)
		}
		if rp := it.node.Task.Repeat; rp != nil && it.node.Task.Due != nil && !it.done() {
			for _, d := range rp.Coming(*it.node.Task.Due, it.node.Task.Times, a.now(), start, start.AddDate(0, 0, 41)) {
				k := d.Format("2006-01-02")
				again[k] = append(again[k], it)
			}
		}
	}
	l := monthLaid{of: of}
	for i := range l.list {
		d := start.AddDate(0, 0, i)
		k := d.Format("2006-01-02")
		l.list[i], l.again[i] = byTime(byDay[k], d), again[k]
	}
	a.memo.month = l
	return &a.memo.month
}

// taskGhost is a task that repeats on a day it comes again, in the week
// and the month: faint, for it is not to do yet; a click shows the task.
func (a *app) taskGhost(c *ui.Context, it taskItem, d time.Time) {
	t := c.Theme()
	chip := ui.Row(c.Key("again:"+it.path+"#"+it.node.ID+d.Format("0102"))).Padding(5, 8).Gap(6).Radius(radiusSmall).AlignItems(ui.Center).FillWidth().
		Background(t.Text.Alpha(0)).Cursor(ui.CursorPointer).Label("到时再做 " + it.node.Title).
		Tooltip(it.node.Task.Repeat.Label() + "：做完上一次，这一天再出现").Transition(easeLook)
	if chip.Hovered() {
		chip.Background(t.Text.Alpha(0.07))
	}
	if chip.Clicked() {
		a.later(func() { a.showTask(it) })
	}
	chip.Children(func() {
		ui.Text(c, "↻").FontSize(textCaption).TextColor(t.TextMuted).PassThrough()
		ui.Text(c, it.node.Title).FontSize(textSmall).TextColor(t.TextMuted).SingleLine().Grow(1).Shrink(1).MinWidth(0).PassThrough()
		mapDot(c, a.mapColor(it.path).Alpha(0.5), 6)
	})
}

// byTime are the tasks of the day d, as a calendar's: those of the whole
// day first, as they come, then those with a time, by it.
func byTime(items []taskItem, d time.Time) []taskItem {
	var out []taskItem
	for _, it := range items {
		if dd := it.date(); dd != nil && sameDay(*dd, d) {
			out = append(out, it)
		}
	}
	slices.SortStableFunc(out, func(x, y taskItem) int {
		tx, ty := x.node.Task.Time, y.node.Task.Time
		if x.node.Task.Due == nil {
			tx = ""
		}
		if y.node.Task.Due == nil {
			ty = ""
		}
		return strings.Compare(tx, ty)
	})
	return out
}

// taskChip is a task in the calendar: a small circle and its title, on a
// faint fill; dragged to another day, it goes there. One with a time of
// its day says it: over its title in the week (span), before it in the
// month's small cells.
func (a *app) taskChip(c *ui.Context, it taskItem, span bool) {
	t := c.Theme()
	// A white card, as the lists' are: its circle says its priority, a dot
	// its map.
	chip := ui.Row(c.Key("chip:"+it.path+"#"+it.node.ID)).Padding(5, 8).Gap(6).Radius(radiusSmall).AlignItems(ui.Center).FillWidth().
		Background(blockFill(t)).Transition(easeLook).Cursor(ui.CursorPointer).
		Label("任务 " + it.node.Title).Tooltip(it.mapName + " · " + it.node.Title).Drag(taskDrag{it.path, it.node.ID})
	switch {
	case a.tasks.sel == it.key():
		chosenCard(chip, t)
	case chip.Hovered():
		chip.Background(t.Text.Alpha(0.08))
	}
	switch {
	case chip.DoubleClicked():
		a.later(func() { a.libOpen = false; a.goToNode(it.path, it.sheet, it.node.ID) })
	case chip.Clicked():
		a.later(func() { a.showTask(it) })
	}
	chip.ContextMenu(func(m *ui.Menu) { a.taskMenu(m, it) })
	when := ""
	if it.node.Task.Due != nil {
		when = it.node.Task.Span()
	}
	chip.Children(func() {
		a.taskCircle(c, it, 13)
		words := func() {
			if when != "" && !span {
				ui.Text(c, strings.SplitN(when, "–", 2)[0]).FontSize(textCaption).TextColor(t.TextMuted).FontFeatures("tnum").NoWrap().Shrink(0).PassThrough()
			}
			title := ui.Text(c, it.node.Title).FontSize(textSmall).SingleLine().Grow(1).Shrink(1).MinWidth(0).PassThrough()
			if it.done() {
				title.TextColor(t.TextMuted).Strikethrough()
			}
		}
		if when != "" && span {
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).PassThrough().Children(func() {
				ui.Text(c, when).FontSize(textCaption).TextColor(t.Accent.MixOklab(t.Text, 0.25)).FontFeatures("tnum").SingleLine().PassThrough()
				words()
			})
		} else {
			words()
		}
		mapDot(c, a.mapColor(it.path), 6)
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
		if !countsAsTask(it) {
			continue
		}
		n[tasksAll]++
		if d := it.date(); d != nil {
			if it.late(day) {
				n[tasksLate]++
			}
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

// repeatProp is how a task repeats, in its details, as TickTick's: not,
// by the day, the week, the working days, the month or the year, or as
// made up below: every so many days, weeks, months or years; by the week
// on which days; by the month on which dates (the last too) or on the
// first to the last of a weekday. Whatever it is, it may end on a day or
// after so many times, and be counted from the day it was done.
func (a *app) repeatProp(c *ui.Context, it taskItem) {
	t := c.Theme()
	task := it.node.Task
	rep := task.Repeat
	set := func(r *model.Repeat) {
		a.later(func() {
			a.editTask(it, "改重复", func(n *model.Node) {
				n.Task.Repeat = r
				if r != nil && n.Task.Due == nil { // what repeats has a day
					d := today(a.now())
					n.Task.Due = &d
				}
			})
		})
	}
	// change sets a copy of the repeat, changed by fn.
	change := func(fn func(r *model.Repeat)) {
		r := rep.Clone()
		fn(r)
		set(r)
	}
	due := today(a.now())
	if task.Due != nil {
		due = *task.Due
	}
	wd := int(due.Weekday())
	if wd == 0 {
		wd = 7
	}
	const (
		kindNone = iota
		kindDay
		kindWeek
		kindWorkdays
		kindMonth
		kindYear
		kindOwn
		kindLegal
	)
	kinds := []string{"不重复", "每天", "每周", "工作日", "每月", "每年", "自定义", "法定工作日"}
	kind := kindNone
	switch {
	case rep == nil:
	case a.tasks.repeatOwn == it.key():
		kind = kindOwn
	case rep.Unit == model.RepeatWorkday:
		kind = kindLegal
	case rep.Workdays():
		kind = kindWorkdays
	case rep.Every > 1 || rep.Unit == model.RepeatWeek && len(rep.Days) > 1 ||
		rep.Unit == model.RepeatMonth && (len(rep.MonthDays) > 0 || rep.Nth != 0):
		kind = kindOwn
	case rep.Unit == model.RepeatDay:
		kind = kindDay
	case rep.Unit == model.RepeatWeek:
		kind = kindWeek
	case rep.Unit == model.RepeatMonth:
		kind = kindMonth
	case rep.Unit == model.RepeatYear:
		kind = kindYear
	}
	chip := func(key, text, label string, on bool) ui.Element {
		b := ui.Box(c.Key(key)).Height(28).Padding(0, 12).Radius(radiusPill).Center().Cursor(ui.CursorPointer).Label(label).Shrink(0).
			Background(t.Text.Alpha(0.05)).Transition(easeLook)
		switch {
		case on:
			b.Background(t.Accent.Alpha(0.14))
		case b.Hovered():
			b.Background(t.Text.Alpha(0.09))
		}
		press(b, 0.96)
		b.Children(func() {
			txt := ui.Text(c, text).FontSize(textSmall).NoWrap().PassThrough().FontFeatures("tnum")
			if on {
				txt.FontWeight(700).TextColor(t.Accent.MixOklab(t.Text, 0.3))
			}
		})
		return b
	}
	// day is a date's small square key, of a grid of the month's dates.
	day := func(key, text, label string, on bool) ui.Element {
		return chip(key, text, label, on).Padding(0).Width(30)
	}
	chosen := -1
	// The three most used are a key each, one click; the others are under
	// one key that opens them: six chips in two rows were too heavy, and
	// all six in a menu a click more for the common ones.
	propRow(c, "重复", func() {
		for _, i := range []int{kindNone, kindDay, kindWeek} {
			if chip("repeat:"+kinds[i], kinds[i], "重复 "+kinds[i], i == kind).Clicked() {
				chosen = i
			}
		}
		more := "更多"
		if kind >= kindWorkdays {
			more = kinds[kind]
		}
		dropButton(c, "repeat", more, "重复方式", "工作日、每月、每年，或自己定").Menu(func(m *ui.Menu) {
			for _, i := range []int{kindWorkdays, kindLegal, kindMonth, kindYear, kindOwn} {
				name := kinds[i]
				switch i {
				case kindMonth:
					name = fmt.Sprintf("每月（%d 日）", due.Day())
				case kindYear:
					name = fmt.Sprintf("每年（%d月%d日）", due.Month(), due.Day())
				case kindLegal:
					name = "法定工作日（跳过节假日，调休上班照常）"
				}
				if m.Item(name).Checked(i == kind).Chosen() {
					chosen = i
				}
			}
		})
		if chosen < 0 {
			return
		}
		a.tasks.repeatOwn = ""
		// What it ends by and counts from stays as the kind changes.
		keep := func(r *model.Repeat) *model.Repeat {
			if rep != nil {
				r.Until, r.Count, r.FromDone = rep.Clone().Until, rep.Count, rep.FromDone
			}
			return r
		}
		switch chosen {
		case kindNone:
			set(nil)
		case kindDay:
			set(keep(&model.Repeat{Unit: model.RepeatDay}))
		case kindWorkdays:
			set(keep(&model.Repeat{Unit: model.RepeatWeek, Days: []int{1, 2, 3, 4, 5}}))
		case kindWeek:
			set(keep(&model.Repeat{Unit: model.RepeatWeek}))
		case kindMonth:
			set(keep(&model.Repeat{Unit: model.RepeatMonth}))
		case kindYear:
			set(keep(&model.Repeat{Unit: model.RepeatYear}))
		case kindLegal:
			set(keep(&model.Repeat{Unit: model.RepeatWorkday}))
		case kindOwn:
			a.tasks.repeatOwn = it.key()
			if rep == nil {
				set(&model.Repeat{Unit: model.RepeatWeek, Days: []int{wd}})
			}
		}
	})
	if rep == nil {
		return
	}
	// What is under the row of its kinds, in line with them.
	sub := func(key string, fn func()) {
		ui.Row(c.Key(key)).Padding(0, 0, 0, 52).Gap(6).Wrap().GapY(6).AlignItems(ui.Center).FillWidth().Children(fn)
	}
	word := func(s string) { ui.Text(c, s).FontSize(textSmall).TextColor(t.TextMuted) }
	stepper := func(key, label string, n, least int, fn func(int)) {
		if iconButton(c.Key(key+":less"), iconMinus, label+" 减一", "少一", 24, true).Clicked() && n > least {
			fn(n - 1)
		}
		ui.Text(c, fmt.Sprint(n)).FontSize(textSmall).FontWeight(700).FontFeatures("tnum").MinWidth(16).TextAlign(ui.Center)
		if iconButton(c.Key(key+":more"), iconPlus, label+" 加一", "多一", 24, true).Clicked() && n < 99 {
			fn(n + 1)
		}
	}
	if kind == kindOwn {
		sub("repeat-every", func() {
			word("每")
			stepper("every", "间隔", max(rep.Every, 1), 1, func(n int) { change(func(r *model.Repeat) { r.Every = n }) })
			for _, u := range []struct{ unit, name string }{{model.RepeatDay, "天"}, {model.RepeatWeek, "周"}, {model.RepeatMonth, "月"}, {model.RepeatYear, "年"}} {
				if chip("unit:"+u.unit, u.name, "按"+u.name+"重复", rep.Unit == u.unit).Clicked() {
					change(func(r *model.Repeat) {
						r.Unit, r.Days, r.MonthDays, r.Nth = u.unit, nil, nil, 0
						if u.unit == model.RepeatWeek {
							r.Days = []int{wd}
						}
					})
				}
			}
		})
		switch rep.Unit {
		case model.RepeatWeek:
			sub("repeat-days", func() {
				for d, name := range []string{"一", "二", "三", "四", "五", "六", "日"} {
					d++
					on := slices.Contains(rep.Days, d)
					if chip("on:"+name, name, "周"+name, on).Clicked() {
						change(func(r *model.Repeat) {
							if on {
								r.Days = slices.DeleteFunc(r.Days, func(x int) bool { return x == d })
							} else {
								r.Days = append(r.Days, d)
								slices.Sort(r.Days)
							}
						})
					}
				}
			})
		case model.RepeatMonth:
			byWeekday := rep.Nth != 0
			sub("repeat-month-by", func() {
				if chip("month:dates", "按日期", "每月按日期", !byWeekday).Clicked() && byWeekday {
					change(func(r *model.Repeat) { r.Nth, r.Days, r.MonthDays = 0, nil, []int{due.Day()} })
				}
				if chip("month:weekday", "按星期", "每月按星期", byWeekday).Clicked() && !byWeekday {
					nth := (due.Day()-1)/7 + 1
					if nth > 4 {
						nth = -1
					}
					change(func(r *model.Repeat) { r.Nth, r.Days, r.MonthDays = nth, []int{wd}, nil })
				}
			})
			if byWeekday {
				sub("repeat-nth", func() {
					for i, name := range []string{"第一个", "第二个", "第三个", "第四个", "最后一个"} {
						n := i + 1
						if n == 5 {
							n = -1
						}
						if chip("nth:"+name, name, "每月"+name, rep.Nth == n).Clicked() {
							change(func(r *model.Repeat) { r.Nth = n })
						}
					}
				})
				sub("repeat-nth-day", func() {
					for d, name := range []string{"一", "二", "三", "四", "五", "六", "日"} {
						d++
						if chip("nthday:"+name, "周"+name, "每月的周"+name, len(rep.Days) > 0 && rep.Days[0] == d).Clicked() {
							change(func(r *model.Repeat) { r.Days = []int{d} })
						}
					}
				})
				break
			}
			sub("repeat-dates", func() {
				days := rep.MonthDays
				if len(days) == 0 {
					days = []int{due.Day()}
				}
				for d := 1; d <= 31; d++ {
					on := slices.Contains(days, d)
					if day(fmt.Sprint("date:", d), fmt.Sprint(d), fmt.Sprintf("每月 %d 日", d), on).Clicked() {
						change(func(r *model.Repeat) { r.MonthDays = toggleDay(days, d) })
					}
				}
				if chip("date:last", "最后一天", "每月最后一天", slices.Contains(days, -1)).Clicked() {
					change(func(r *model.Repeat) { r.MonthDays = toggleDay(days, -1) })
				}
			})
		case model.RepeatYear:
			sub("repeat-year", func() { word(fmt.Sprintf("每年的 %d月%d日，跟着任务的日期", due.Month(), due.Day())) })
		}
	}
	// How it ends, and from which day it counts, whatever its kind.
	sub("repeat-end", func() {
		word("结束")
		if chip("end:never", "永不", "重复 永不结束", rep.Until == nil && rep.Count == 0).Clicked() {
			change(func(r *model.Repeat) { r.Until, r.Count = nil, 0 })
		}
		if chip("end:day", "到某天", "重复到某天结束", rep.Until != nil).Clicked() && rep.Until == nil {
			end := due.AddDate(0, 1, 0)
			change(func(r *model.Repeat) { r.Until, r.Count = &end, 0 })
		}
		if chip("end:count", "按次数", "重复几次后结束", rep.Count > 0).Clicked() && rep.Count == 0 {
			change(func(r *model.Repeat) { r.Until, r.Count = nil, max(task.Times+1, 10) })
		}
		switch {
		case rep.Until != nil:
			end := *rep.Until
			if ui.DateInput(c.Key("repeat-until"), &end).Label("重复结束日期").FontSize(textSmall).Background(blockFill(t)).BorderColor(ui.RGBA(0, 0, 0, 0)).Changed() {
				end = today(end)
				change(func(r *model.Repeat) { r.Until = &end })
			}
		case rep.Count > 0:
			word("共")
			stepper("count", "重复次数", rep.Count, max(task.Times+1, 1), func(n int) { change(func(r *model.Repeat) { r.Count = n }) })
			word(fmt.Sprintf("次，已做 %d 次", task.Times))
		}
	})
	sub("repeat-from", func() {
		if chip("from-done", "从做完那天算", "重复从做完那天算", rep.FromDone).Tooltip("下一次从做完的那天往后数，而不是从原定的日期").Clicked() {
			change(func(r *model.Repeat) { r.FromDone = !r.FromDone })
		}
	})
	// By the notices: where the next day's year has none yet, it is by the
	// week, and said so.
	if rep.Unit == model.RepeatWorkday && task.Due != nil {
		if y := rep.Next(task.Due, a.now()).Year(); !holidayYearKnown(y) {
			sub("repeat-legal", func() {
				says := fmt.Sprintf("%d 年的放假安排还没拿到，先按周一到周五算", y)
				if a.cfg.NoHolidays {
					says = "设置里关了节假日，先按周一到周五算"
				}
				ui.Text(c, says).FontSize(textSmall).TextColor(t.Warning).Wrap()
			})
		}
	}
	sub("repeat-says", func() {
		says := rep.Label()
		if task.Due != nil {
			if next := rep.Next(task.Due, a.now()); rep.Ended(next, task.Times+1) {
				says += "；这是最后一次"
			} else {
				says += "；做完一次，下一次是 " + dayLabel(next, a.now())
			}
		}
		ui.Text(c, says).FontSize(textSmall).TextColor(t.TextMuted).Wrap()
	})
}

// toggleDay is days with d put in, or taken out; never none: the last
// taken out leaves the one there was.
func toggleDay(days []int, d int) []int {
	if slices.Contains(days, d) {
		if len(days) == 1 {
			return slices.Clone(days)
		}
		return slices.DeleteFunc(slices.Clone(days), func(x int) bool { return x == d })
	}
	return append(slices.Clone(days), d)
}

// A map renamed or moved is followed by the project chosen on 今天 and by
// the task whose details show (sidebar.go's pathFollowers): left at the
// old path, the project's page was empty under its old name.
func init() {
	pathFollowers = append(pathFollowers, func(a *app, follow func(string) string) {
		a.tasks.onMap = follow(a.tasks.onMap)
		if i := strings.LastIndex(a.tasks.sel, "#"); i >= 0 {
			a.tasks.sel = follow(a.tasks.sel[:i]) + a.tasks.sel[i:]
		}
	})
}
