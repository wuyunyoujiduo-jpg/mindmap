package main

import (
	"fmt"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// The week as hours, as a calendar's (the user's reference was TickTick's
// and Feishu's): a column a day under its weekday and date, the tasks of
// the whole day in a band over the hours, and those with a time of their
// day (model.Task.Time) as blocks on the hours they take, side by side
// where they meet; the hours down the left, now a line across today's
// column. A task dropped on an hour goes to that day and time, keeping
// how long it takes; dropped on the band, it is of the whole day.

const (
	hourHeight  = 48 // an hour's height on the week
	hourGutter  = 60 // the room of the hours' names at the left
	weekOpensAt = 7  // the hour a week shows first, earlier tasks aside
	slotMinutes = 30 // what a drop or a block's least height is counted in
	allDayShown = 3  // the tasks of the whole day a day shows, before 还有
)

// weekTask is a task with a time on a day of the week, or one that comes
// again on it: from and to are its minutes in the day, lane its place
// among those it meets, of lanes.
type weekTask struct {
	it          taskItem
	ghost       bool
	from, to    int
	lane, lanes int
}

// weekView is the week of the day shown, as hours.
func (a *app) weekView(c *ui.Context, items []taskItem) {
	start := weekStart(a.tasks.day)
	now := a.now()
	var days []time.Time
	for i := range 7 {
		days = append(days, start.AddDate(0, 0, i))
	}
	// What each day holds: of the whole day, and timed.
	allDay := make([][]weekTask, 7)
	timed := make([][]weekTask, 7)
	earliest := 24 * 60
	for i, d := range days {
		add := func(it taskItem, ghost bool) {
			task := it.node.Task
			if task.Time == "" || task.Due == nil {
				allDay[i] = append(allDay[i], weekTask{it: it, ghost: ghost})
				return
			}
			from := clockMinutes(task.Time)
			to := from + slotMinutes
			if task.EndTime != "" && clockMinutes(task.EndTime) > from {
				to = max(clockMinutes(task.EndTime), from+slotMinutes/2)
			}
			timed[i] = append(timed[i], weekTask{it: it, ghost: ghost, from: from, to: min(to, 24*60)})
			earliest = min(earliest, from)
		}
		for _, it := range byTime(items, d) {
			add(it, false)
		}
		for _, it := range items {
			if a.comesAgain(it, d, d) {
				add(it, true)
			}
		}
		timed[i] = lanes(timed[i])
	}
	// A week newly shown opens at its first hours: seven, or earlier for
	// what is earlier, or now, an hour before it.
	if !a.tasks.weekAt.Equal(start) {
		a.tasks.weekAt = start
		first := weekOpensAt * 60
		if earliest < first {
			first = earliest
		}
		if !now.Before(start) && now.Before(start.AddDate(0, 0, 7)) {
			if m := now.Hour()*60 + now.Minute(); m < first+60 || m > first+9*60 {
				first = max(m-60, 0)
			}
		}
		a.tasks.weekScroll.X, a.tasks.weekScroll.Y = 0, max(float32(first/60*hourHeight)-12, 0) // its name in sight
	}
	ui.Column(c.Key("week")).Grow(1).MinHeight(0).Margin(10, 28, 0).Children(func() {
		a.weekHead(c, days, now)
		a.weekAllDay(c, days, allDay)
		ui.Scroll(c.Key("week-hours")).Grow(1).MinHeight(0).TrackScroll(&a.tasks.weekScroll).Children(func() {
			ui.Row(c).FillWidth().Height(24 * hourHeight).Children(func() {
				a.weekHours(c, days, now)
				for i, d := range days {
					a.weekDay(c, i, d, timed[i], now)
				}
			})
		})
	})
}

// weekHead names the days: the weekday, small, over the date; today's in
// the accent. A double click shows the day.
func (a *app) weekHead(c *ui.Context, days []time.Time, now time.Time) {
	t := c.Theme()
	_, offset := now.Zone()
	zone := fmt.Sprintf("GMT%+03d", offset/3600)
	if offset%3600 != 0 {
		zone += fmt.Sprintf(":%02d", absInt(offset%3600)/60)
	}
	ui.Row(c.Key("week-head")).FillWidth().Shrink(0).BorderWidth(0, 0, 1, 0).BorderColor(hairline(t)).Children(func() {
		ui.Column(c).Width(hourGutter).Shrink(0).Padding(0, 8, 10, 0).Justify(ui.End).AlignItems(ui.End).Children(func() {
			ui.Text(c, zone).FontSize(textCaption).TextColor(faint(t)).FontFeatures("tnum").NoWrap()
		})
		for i, d := range days {
			isToday := sameDay(d, now)
			rest := d.Weekday() == time.Saturday || d.Weekday() == time.Sunday
			col := ui.Column(c.Key(fmt.Sprint("week-day:", i))).Grow(1).Basis(0).MinWidth(0).Padding(6, 10, 10).Gap(4).
				BorderWidth(0, 0, 0, 1).BorderColor(t.Border.Alpha(0.55)).Cursor(ui.CursorPointer).Tooltip("双击看这一天")
			if col.DoubleClicked() {
				day := d
				a.later(func() { a.tasks.day, a.tasks.view = day, tasksDay })
			}
			ink, word := t.Text, t.TextMuted
			switch {
			case isToday:
				ink, word = t.Accent, t.Accent
			case rest:
				ink, word = faint(t), faint(t)
			}
			col.Children(func() {
				ui.Text(c, "周"+weekdayName(d)[len("周"):]).FontSize(textCaption).TextColor(word).NoWrap()
				ui.Text(c, fmt.Sprint(d.Day())).FontSize(textTitle).FontWeight(700).FontFeatures("tnum").TextColor(ink).NoWrap()
			})
		}
	})
}

// weekAllDay is the band of the tasks of the whole day, over the hours:
// a few a day and how many more; there only while the week has any, but
// as a place to drop on.
func (a *app) weekAllDay(c *ui.Context, days []time.Time, allDay [][]weekTask) {
	t := c.Theme()
	ui.Row(c.Key("week-allday")).FillWidth().Shrink(0).MinHeight(34).BorderWidth(0, 0, 1, 0).BorderColor(hairline(t)).Children(func() {
		ui.Row(c).Width(hourGutter).Shrink(0).Padding(9, 8, 0, 0).Justify(ui.End).AlignItems(ui.Start).Children(func() {
			ui.Text(c, "全天").FontSize(textCaption).TextColor(t.TextMuted).NoWrap()
		})
		for i, d := range days {
			col := ui.Column(c.Key(fmt.Sprint("allday:", i))).Grow(1).Basis(0).MinWidth(0).Padding(4).Gap(3).
				BorderWidth(0, 0, 0, 1).BorderColor(t.Border.Alpha(0.55)).Background(t.Text.Alpha(0)).Transition(easeLook)
			if sameDay(d, a.now()) {
				col.Background(t.Text.Alpha(0.025))
			}
			a.dropTaskAt(col, d, -1)
			list := allDay[i]
			col.Children(func() {
				for j, w := range list {
					if j == allDayShown && len(list) > allDayShown+1 {
						more := ui.Text(c.Key("more"), fmt.Sprintf("还有 %d 项", len(list)-allDayShown)).FontSize(textCaption).
							TextColor(t.TextMuted).Padding(0, 6).Cursor(ui.CursorPointer)
						if more.Clicked() {
							day := d
							a.later(func() { a.tasks.day, a.tasks.view = day, tasksDay })
						}
						break
					}
					if w.ghost {
						a.taskGhost(c, w.it, d)
					} else {
						a.taskChip(c, w.it, false)
					}
				}
			})
		}
	})
}

// weekHours names the hours down the left, each at its line; now's time
// in the danger's color, the hour by it given way.
func (a *app) weekHours(c *ui.Context, days []time.Time, now time.Time) {
	t := c.Theme()
	nowAt := -1
	if !now.Before(days[0]) && now.Before(days[6].AddDate(0, 0, 1)) {
		nowAt = now.Hour()*60 + now.Minute()
	}
	ui.Box(c.Key("week-gutter")).Width(hourGutter).Shrink(0).FillHeight().Children(func() {
		for h := 1; h < 24; h++ {
			if nowAt >= 0 && absInt(nowAt-h*60) < 15 {
				continue
			}
			ui.Text(c.Key(fmt.Sprint("hour:", h)), fmt.Sprintf("%02d:00", h)).Absolute().Top(float32(h*hourHeight) - 8).Right(10).
				FontSize(textCaption).TextColor(t.TextMuted).FontFeatures("tnum").NoWrap()
		}
		if nowAt >= 0 {
			ui.Text(c.Key("now"), now.Format("15:04")).Absolute().Top(float32(nowAt*hourHeight)/60 - 8).Right(10).
				FontSize(textCaption).FontWeight(600).TextColor(t.Danger).FontFeatures("tnum").NoWrap()
		}
	})
}

// weekDay is a day's column of hours: a line an hour, a place to drop on
// every half hour, its timed tasks as blocks, and now across it if today.
func (a *app) weekDay(c *ui.Context, i int, d time.Time, timed []weekTask, now time.Time) {
	t := c.Theme()
	col := ui.Box(c.Key(fmt.Sprint("hours:", i))).Grow(1).Basis(0).MinWidth(0).FillHeight().
		BorderWidth(0, 0, 0, 1).BorderColor(t.Border.Alpha(0.55))
	isToday := sameDay(d, now)
	if isToday {
		col.Background(t.Text.Alpha(0.025))
	}
	col.Children(func() {
		ui.Column(c).FillWidth().FillHeight().Children(func() {
			for m := 0; m < 24*60; m += slotMinutes {
				slot := ui.Box(c.Key(fmt.Sprint("slot:", m))).FillWidth().Height(hourHeight * slotMinutes / 60).Shrink(0).
					Background(t.Text.Alpha(0)).Transition(easeLook)
				if (m+slotMinutes)%60 == 0 && m+slotMinutes < 24*60 {
					slot.BorderWidth(0, 0, 1, 0).BorderColor(hairline(t).Alpha(0.6))
				}
				a.dropTaskAt(slot, d, m)
			}
		})
		for _, w := range timed {
			a.weekBlock(c, w, d)
		}
		if isToday {
			at := float32((now.Hour()*60+now.Minute())*hourHeight) / 60
			ui.Box(c.Key("now-line")).Absolute().Top(at).Left(0).Right(0).Height(1.5).Background(t.Danger).PassThrough()
			ui.Box(c.Key("now-dot")).Absolute().Top(at-3.25).Left(-4).Size(8, 8).Radius(4).Background(t.Danger).PassThrough()
		}
	})
}

// weekBlock is a timed task on its hours: on its map's pastel, its title
// and, with room, its time; a click shows it, dragged it goes to another
// hour. One that comes again is faint, its title after ↻.
func (a *app) weekBlock(c *ui.Context, w weekTask, d time.Time) {
	t := c.Theme()
	it := w.it
	top := float32(w.from*hourHeight) / 60
	height := float32((w.to-w.from)*hourHeight)/60 - 2
	// Its place among those it meets, by the fraction of the column.
	key := "block:" + it.key()
	if w.ghost {
		key = "again:" + it.key() + d.Format("0102")
	}
	bg, ink := tinted(t, a.mapColor(it.path))
	b := ui.Column(c.Key(key)).Absolute().Top(top+1).Height(height).
		LeftPercent(100*float32(w.lane)/float32(w.lanes)).WidthPercent(100/float32(w.lanes)).
		Padding(3, 6).Radius(radiusSmall).Clip().Cursor(ui.CursorPointer).Background(bg).
		BorderWidth(0, 0, 0, 3).BorderColor(a.mapColor(it.path)).Transition(easeLook).
		Tooltip(it.mapName + " · " + it.node.Title + "  " + it.node.Task.Span())
	span := it.node.Task.Span()
	if w.ghost {
		b.Background(bg.Alpha(0.35)).BorderColor(a.mapColor(it.path).Alpha(0.4)).Label("到时再做 " + it.node.Title)
	} else {
		b.Label("任务 " + it.node.Title).Drag(taskDrag{it.path, it.node.ID})
		b.ContextMenu(func(m *ui.Menu) { a.taskMenu(m, it) })
	}
	if a.tasks.sel == it.key() && !w.ghost {
		b.Background(bg.MixOklab(ink, 0.12))
	} else if b.Hovered() {
		b.Background(bg.MixOklab(ink, 0.06))
	}
	switch {
	case b.DoubleClicked() && !w.ghost:
		a.later(func() { a.libOpen = false; a.goToNode(it.path, it.sheet, it.node.ID) })
	case b.Clicked():
		a.later(func() { a.showTask(it) })
	}
	title := it.node.Title
	if w.ghost {
		title = "↻ " + title
	}
	b.Children(func() {
		roomy := height >= 34
		ui.Row(c).Gap(5).AlignItems(ui.Center).PassThrough().Children(func() {
			txt := ui.Text(c, title).FontSize(textCaption).FontWeight(600).TextColor(ink).SingleLine().Shrink(1).MinWidth(0).PassThrough()
			if it.done() || w.ghost {
				txt.TextColor(ink.Alpha(0.6))
			}
			if it.done() {
				txt.Strikethrough()
			}
			if !roomy {
				ui.Text(c, span).FontSize(textCaption).TextColor(ink.Alpha(0.7)).FontFeatures("tnum").NoWrap().Shrink(0).PassThrough()
			}
		})
		if roomy {
			ui.Text(c, span).FontSize(textCaption).TextColor(ink.Alpha(0.7)).FontFeatures("tnum").SingleLine().PassThrough()
		}
	})
}

// lanes places the timed tasks of a day side by side where they meet:
// each in the first lane free at its beginning, and those that meet,
// directly or through others, sharing the column in as many lanes as
// the most of them at once.
func lanes(ws []weekTask) []weekTask {
	slices.SortStableFunc(ws, func(x, y weekTask) int {
		if x.from != y.from {
			return x.from - y.from
		}
		return y.to - x.to
	})
	for i := 0; i < len(ws); {
		// A group: those that meet one another, through others too.
		end, j := ws[i].to, i+1
		for j < len(ws) && ws[j].from < end {
			end = max(end, ws[j].to)
			j++
		}
		var free []int // the minute each lane is free from
		for k := i; k < j; k++ {
			lane := slices.IndexFunc(free, func(f int) bool { return f <= ws[k].from })
			if lane < 0 {
				lane = len(free)
				free = append(free, 0)
			}
			free[lane] = ws[k].to
			ws[k].lane = lane
		}
		for k := i; k < j; k++ {
			ws[k].lanes = len(free)
		}
		i = j
	}
	return ws
}

// dropTaskAt takes a task dropped on a day: at the minute at of it,
// keeping how long it takes, or, at -1, of the whole day.
func (a *app) dropTaskAt(el ui.Element, day time.Time, at int) {
	t := a.theme()
	if _, over := ui.DragOver[taskDrag](el); over {
		el.Background(t.Accent.Alpha(0.12))
	}
	d, ok := ui.Drop[taskDrag](el)
	if !ok {
		return
	}
	for _, it := range a.tasks.items {
		if it.path != d.path || it.node.ID != d.id {
			continue
		}
		a.later(func() {
			a.editTask(it, "改任务时间", func(n *model.Node) {
				if n.Task == nil {
					n.SetStatus(model.StatusTodo, a.now())
				}
				day := day
				n.Task.Due = &day
				if n.Task.Start != nil && n.Task.Start.After(day) {
					n.Task.Start = nil
				}
				was := n.Task.Time
				if at < 0 {
					n.Task.Time, n.Task.EndTime = "", ""
					return
				}
				long := 0
				if n.Task.Time != "" && n.Task.EndTime != "" {
					long = clockMinutes(n.Task.EndTime) - clockMinutes(n.Task.Time)
				}
				n.Task.Time, n.Task.EndTime = clockText(at), ""
				if long > 0 && at+long < 24*60 {
					n.Task.EndTime = clockText(at + long)
				}
				if n.Task.Remind == "" || n.Task.Remind == was {
					n.Task.Remind = n.Task.Time
				}
			})
		})
		return
	}
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
