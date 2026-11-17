package main

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// The week and the day as hours, as a calendar's (the user's reference
// was TickTick's and Feishu's): a column a day under its weekday, date and
// day of the Chinese calendar (lunar.go), the tasks of the whole day in a
// band over the hours, and those with a time of their day
// (model.Task.Time) as blocks on the hours they take, side by side where
// they meet; the hours down the left, now a line across today's column.
//
// What is done there is done there, the user asked, all of it:
//   - a press on a blank hour writes a task there, of an hour; pressed
//     and drawn down, of the hours drawn over (pick, draft); a click on
//     the band's blank, a task of the whole day;
//   - a task dropped on an hour goes to that day and time, keeping how
//     long it takes, a block showing where as it is dragged; dropped on
//     the band, it is of the whole day (dropTaskAt);
//   - a block's foot drawn down or up is when it ends (stretch).

const (
	hourHeight  = 48 // an hour's height
	hourGutter  = 60 // the room of the hours' names at the left
	weekOpensAt = 7  // the hour a week shows first, earlier tasks aside
	slotMinutes = 30 // what a press or a drop is counted in
	stretchStep = 15 // what a block's end is counted in
	allDayShown = 3  // the tasks of the whole day a day shows, before 还有
)

// weekTask is a task with a time on a day, or one that comes again on it:
// from and to are its minutes in the day, lane its place among those it
// meets, of lanes.
type weekTask struct {
	it          taskItem
	ghost       bool
	from, to    int
	lane, lanes int
}

// hourPick is a span of a day being chosen by pressing on its hours: the
// minutes pressed at and now under the pointer; onBlock when the press
// began on a task, which is then its own.
type hourPick struct {
	day      time.Time
	from, to int
	onBlock  bool
}

// hourDraft is a task being written where it was chosen: its day, and its
// minutes, from -1 for one of the whole day.
type hourDraft struct {
	day      time.Time
	from, to int
	text     string
	focused  bool // it has been written in, and leaving it keeps it
}

// hourStretch is a block's end being dragged, in minutes.
type hourStretch struct {
	key string
	to  float32
}

// hoursLaid is the tasks of some days put on them: those of the whole day,
// those with a time in their lanes, and the earliest minute any begins at.
// It is worked out once for the tasks as gathered, the project they are
// narrowed to, the days and today (of): the pointer moving over the hours
// is a frame each, and each put every task on its day anew and asked of
// every one that repeats, for every day, when it comes again.
type hoursLaid struct {
	of            hoursOf
	allDay, timed [][]weekTask
	earliest      int
}

// hoursOf is what the tasks were put on the days of; holidays is the
// reading of the notices, by which 法定工作日 comes again.
type hoursOf struct {
	tasks    int
	onMap    string
	first    time.Time
	days     int
	today    time.Time
	holidays int
}

// hoursLaid puts the tasks on the days (hoursLaid the type).
func (a *app) hoursLaid(items []taskItem, days []time.Time) *hoursLaid {
	now := a.now()
	of := hoursOf{a.tasks.serial, a.tasks.onMap, days[0], len(days), today(now), holidaysLoads()}
	if l := &a.memo.hours; l.of == of && a.tasks.serial > 0 {
		return l
	}
	a.memo.laid++
	allDay := make([][]weekTask, len(days))
	timed := make([][]weekTask, len(days))
	earliest := 24 * 60
	add := func(i int, it taskItem, ghost bool) {
		task := it.node.Task
		if task.Time == "" || task.Due == nil {
			allDay[i] = append(allDay[i], weekTask{it: it, ghost: ghost})
			return
		}
		from := clockMinutes(task.Time)
		to := from + slotMinutes
		if task.EndTime != "" && clockMinutes(task.EndTime) > from {
			to = max(clockMinutes(task.EndTime), from+stretchStep)
		}
		timed[i] = append(timed[i], weekTask{it: it, ghost: ghost, from: from, to: min(to, 24*60)})
		earliest = min(earliest, from)
	}
	for i, d := range days {
		for _, it := range byTime(items, d) {
			add(i, it, false)
		}
	}
	// Those that come again, asked once for all the days and not a day at
	// a time; on each day as the tasks come, after its own.
	last := days[len(days)-1]
	for _, it := range items {
		task := it.node.Task
		if task.Repeat == nil || task.Due == nil || it.done() {
			continue
		}
		for _, d := range task.Repeat.Coming(*task.Due, task.Times, now, days[0], last) {
			if i := slices.IndexFunc(days, func(day time.Time) bool { return sameDay(day, d) }); i >= 0 {
				add(i, it, true)
			}
		}
	}
	for i := range timed {
		timed[i] = lanes(timed[i])
	}
	a.memo.hours = hoursLaid{of, allDay, timed, earliest}
	return &a.memo.hours
}

// weekView is the week of the day shown, as hours.
func (a *app) weekView(c *ui.Context, items []taskItem) {
	start := a.weekOf(a.tasks.day)
	var days []time.Time
	for i := range 7 {
		days = append(days, start.AddDate(0, 0, i))
	}
	a.hoursView(c, items, days)
}

// hoursView is days as hours: the week's seven, or the day's one.
func (a *app) hoursView(c *ui.Context, items []taskItem, days []time.Time) {
	now := a.now()
	laid := a.hoursLaid(items, days)
	allDay, timed, earliest := laid.allDay, laid.timed, laid.earliest
	// Days newly shown open at their first hours: seven, or earlier for
	// what is earlier, or with now in sight.
	start, end := days[0], days[len(days)-1].AddDate(0, 0, 1)
	if !a.tasks.weekAt.Equal(start) || a.tasks.weekDays != len(days) {
		a.tasks.weekAt, a.tasks.weekDays = start, len(days)
		// What was being chosen or drawn was of the days shown before: the
		// days turned with the press held (← and →), its day or its block
		// is not here to end it, and it stayed, the next press taken as its
		// own.
		a.tasks.pick, a.tasks.stretch = nil, nil
		first := min(weekOpensAt*60, earliest)
		if !now.Before(start) && now.Before(end) {
			if m := now.Hour()*60 + now.Minute(); m < first+60 || m > first+9*60 {
				first = max(m-60, 0)
			}
		}
		a.tasks.weekScroll.X, a.tasks.weekScroll.Y = 0, max(float32(first/60*hourHeight)-12, 0) // its name in sight
	}
	if !now.Before(start) && now.Before(end) { // now's line goes on with the minutes
		c.After(time.Minute - time.Duration(now.Second())*time.Second)
	}
	// Leaving the hours, what was being written is let go.
	if d := a.tasks.draft; d != nil && (d.day.Before(start) || !d.day.Before(end)) {
		a.tasks.draft = nil
	}
	if p := a.tasks.pick; p != nil && (p.day.Before(start) || !p.day.Before(end)) {
		a.tasks.pick = nil
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

// weekHead names the days: the weekday, small, over the date and its day
// of the Chinese calendar; today's in the accent. A double click on a day
// of the week shows it alone.
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
				BorderWidth(0, 0, 0, 1).BorderColor(t.Border.Alpha(0.55))
			if len(days) > 1 {
				col.Cursor(ui.CursorPointer).Tooltip("只看这一天")
				if col.DoubleClicked() {
					day := d
					a.later(func() { a.tasks.day, a.tasks.view = day, tasksDay })
				}
			}
			ink, word := t.Text, t.TextMuted
			switch {
			case isToday:
				ink, word = t.Accent, t.Accent
			case rest:
				ink, word = faint(t), faint(t)
			}
			col.Children(func() {
				ui.Text(c, weekdays[d.Weekday()]).FontSize(textCaption).TextColor(word).NoWrap()
				ui.Row(c).Gap(6).AlignItems(ui.Baseline).Children(func() {
					ui.Text(c, fmt.Sprint(d.Day())).FontSize(textTitle).FontWeight(700).FontFeatures("tnum").TextColor(ink).NoWrap()
					lunarText(c, d, rest && !isToday)
				})
			})
		}
	})
}

// lunarText is the day's word of the Chinese calendar, small: a feast or a
// holiday in the accent.
func lunarText(c *ui.Context, d time.Time, faded bool) {
	t := c.Theme()
	word, feast := lunarLabel(d)
	if word == "" {
		return
	}
	col := t.TextMuted
	switch {
	case feast:
		col = t.Accent.MixOklab(t.Text, 0.2)
	case faded:
		col = faint(t)
	}
	ui.Text(c, word).FontSize(textCaption).TextColor(col).NoWrap().Shrink(1).MinWidth(0).SingleLine()
	holidayMark(c, d)
}

// weekAllDay is the band of the tasks of the whole day, over the hours:
// a few a day and how many more, and under them a blank to click to write
// one there.
func (a *app) weekAllDay(c *ui.Context, days []time.Time, allDay [][]weekTask) {
	t := c.Theme()
	ui.Row(c.Key("week-allday")).FillWidth().Shrink(0).BorderWidth(0, 0, 1, 0).BorderColor(hairline(t)).Children(func() {
		ui.Row(c).Width(hourGutter).Shrink(0).Padding(9, 8, 0, 0).Justify(ui.End).AlignItems(ui.Start).Children(func() {
			ui.Text(c, "全天").FontSize(textCaption).TextColor(t.TextMuted).NoWrap()
		})
		for i, d := range days {
			col := ui.Column(c.Key(fmt.Sprint("allday:", i))).Grow(1).Basis(0).MinWidth(0).MinHeight(34).Padding(4).Gap(3).
				BorderWidth(0, 0, 0, 1).BorderColor(t.Border.Alpha(0.55)).Background(t.Text.Alpha(0)).Transition(easeLook)
			if sameDay(d, a.now()) {
				col.Background(t.Text.Alpha(0.025))
			}
			a.dropTaskAt(col, d, dropWholeDay)
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
				if dr := a.tasks.draft; dr != nil && dr.from < 0 && sameDay(dr.day, d) {
					a.hourDraftInput(c, dr)
					return
				}
				// The blank under them: a click writes a task of the day.
				blank := ui.Box(c.Key("allday-blank")).FillWidth().Grow(1).MinHeight(22).Radius(radiusSmall).
					Cursor(ui.CursorPointer).Label(fmt.Sprintf("新建全天任务 %d月%d日", d.Month(), d.Day())).
					Tooltip("新建这天的全天任务").Background(t.Text.Alpha(0)).Transition(easeLook)
				if blank.Hovered() {
					blank.Background(t.Text.Alpha(0.04))
				}
				if blank.Clicked() {
					day := d
					a.later(func() { a.tasks.draft = &hourDraft{day: day, from: -1, to: -1} })
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
	if !now.Before(days[0]) && now.Before(days[len(days)-1].AddDate(0, 0, 1)) {
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

// minuteAt is the minute of the day at y down a day's hours.
func minuteAt(y float32) int {
	return min(max(int(y*60/hourHeight), 0), 24*60-1)
}

// pickSpan is the span a press chose: the half hours it was drawn over,
// or, clicked, the hour from the half hour clicked.
func pickSpan(p hourPick) (from, to int) {
	lo, hi := min(p.from, p.to), max(p.from, p.to)
	from = lo / slotMinutes * slotMinutes
	to = (hi/slotMinutes + 1) * slotMinutes
	if hi-lo < slotMinutes/3 {
		to = from + 60
	}
	return from, min(to, 24*60)
}

// weekDay is a day's column of hours: a line an hour, its timed tasks as
// blocks, now across it if today, and under them the blank pressed on to
// write a task there.
func (a *app) weekDay(c *ui.Context, i int, d time.Time, timed []weekTask, now time.Time) {
	t := c.Theme()
	col := ui.Box(c.Key(fmt.Sprint("hours:", i))).Grow(1).Basis(0).MinWidth(0).FillHeight().
		BorderWidth(0, 0, 0, 1).BorderColor(t.Border.Alpha(0.55))
	isToday := sameDay(d, now)
	if isToday {
		col.Background(t.Text.Alpha(0.025))
	}
	_, py, _ := col.PointerPosition()
	at := minuteAt(py)
	dragOver := false
	if _, over := ui.DragOver[taskDrag](col); over {
		dragOver = true
	}
	if dd, ok := ui.Drop[taskDrag](col); ok {
		a.moveTaskWhen(dd, d, at/slotMinutes*slotMinutes)
	}
	onBlock := false
	var blank ui.Element
	col.Children(func() {
		// The blank, under all: pressed, it chooses where to write a task.
		blank = ui.Box(c.Key("blank")).Absolute().Top(0).Left(0).Right(0).Bottom(0).
			Label(fmt.Sprintf("时间格 %d月%d日", d.Month(), d.Day())).Tooltip("在这个时间新建任务；选多长，就是多长")
		// The hours' lines, drawn in one box: a box a line was 161 of them
		// a week, each built and keyed at every frame.
		line := hairline(t).Alpha(0.6)
		ui.Box(c.Key("lines")).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) {
			for h := 1; h < 24; h++ {
				p.Fill(ui.Rect{X: r.X, Y: r.Y + float32(h*hourHeight), W: r.W, H: 1}, line, 0)
			}
		})
		for _, w := range timed {
			if a.weekBlock(c, w, d) {
				onBlock = true
			}
		}
		// Where a press is choosing, or a task dragged would go.
		if p := a.tasks.pick; p != nil && !p.onBlock && sameDay(p.day, d) {
			from, to := pickSpan(*p)
			a.hourShade(c, from, to, clockText(from)+"–"+clockText(to))
		}
		if dragOver {
			from := at / slotMinutes * slotMinutes
			long := slotMinutes
			if dd, ok := ui.DragOver[taskDrag](col); ok {
				if it, ok := a.dragged(dd); ok && it.node.Task.Time != "" && it.node.Task.EndTime != "" {
					long = max(clockMinutes(it.node.Task.EndTime)-clockMinutes(it.node.Task.Time), stretchStep)
				}
			}
			a.hourShade(c, from, min(from+long, 24*60), "移到 "+strings.TrimPrefix(clockText(from), "0"))
		}
		if dr := a.tasks.draft; dr != nil && dr.from >= 0 && sameDay(dr.day, d) {
			ui.Box(c.Key("draft")).Absolute().Top(float32(dr.from*hourHeight)/60+1).Height(max(float32((dr.to-dr.from)*hourHeight)/60-2, 30)).
				Left(2).Right(2).Padding(3, 4).Radius(radiusSmall).Background(t.Background).Border(1, t.Accent.Alpha(0.6)).ZIndex(5).Children(func() {
				ui.Text(c, strings.TrimPrefix(clockText(dr.from), "0")+"–"+strings.TrimPrefix(clockText(dr.to), "0")).
					FontSize(textCaption).TextColor(t.Accent).FontFeatures("tnum").Padding(0, 4)
				a.hourDraftInput(c, dr)
			})
		}
		if isToday {
			y := float32((now.Hour()*60+now.Minute())*hourHeight) / 60
			ui.Box(c.Key("now-line")).Absolute().Top(y).Left(0).Right(0).Height(1.5).Background(t.Danger).PassThrough().ZIndex(4)
			ui.Box(c.Key("now-dot")).Absolute().Top(y-3.25).Left(-4).Size(8, 8).Radius(4).Background(t.Danger).PassThrough().ZIndex(4)
		}
	})
	// A press on the blank chooses a span; let go, a task is written there.
	_, by, _ := blank.PointerPosition()
	m := minuteAt(by)
	p := a.tasks.pick
	switch pressed := blank.Pressed(); {
	case pressed && p == nil:
		a.tasks.pick = &hourPick{day: d, from: m, to: m, onBlock: onBlock || a.tasks.stretch != nil}
	case pressed && sameDay(p.day, d):
		p.to = m
	case !pressed && p != nil && sameDay(p.day, d):
		a.tasks.pick = nil
		if !p.onBlock {
			a.hourDraftAt(d, *p)
		}
	case p == nil && blank.Clicked() && !onBlock: // a click, pressed and let go at once
		a.hourDraftAt(d, hourPick{day: d, from: m, to: m})
	}
}

// hourDraftAt opens the line to write a task in where p chose.
func (a *app) hourDraftAt(d time.Time, p hourPick) {
	from, to := pickSpan(p)
	a.later(func() { a.tasks.draft, a.tasks.sel = &hourDraft{day: d, from: from, to: to}, "" })
}

// hourShade is a span of a day's hours shown as chosen: where a task will
// be written, or go.
func (a *app) hourShade(c *ui.Context, from, to int, label string) {
	t := c.Theme()
	ui.Box(c.Key("shade")).Absolute().Top(float32(from*hourHeight)/60+1).Height(float32((to-from)*hourHeight)/60-2).
		Left(2).Right(2).Padding(3, 6).Radius(radiusSmall).Background(t.Accent.Alpha(0.14)).PassThrough().ZIndex(3).Children(func() {
		ui.Text(c, label).FontSize(textCaption).FontWeight(600).TextColor(t.Accent.MixOklab(t.Text, 0.25)).FontFeatures("tnum").NoWrap()
	})
}

// hourDraftInput is the line a task is written in where it was chosen:
// kept as it is written (记下) or when it is left with words in it; let go
// by Escape or left blank.
func (a *app) hourDraftInput(c *ui.Context, dr *hourDraft) {
	t := c.Theme()
	in := ui.TextInputBase(c.Key("draft-text"), &dr.text).Placeholder("写下要做的事").Label("新任务").AutoFocus().
		FontSize(textSmall).FillWidth().Padding(3, 4).Radius(radiusSmall).Background(t.Background)
	keep := func() {
		text := strings.TrimSpace(dr.text)
		a.tasks.draft = nil
		if text == "" {
			return
		}
		day := dr.day
		at, till := "", ""
		if dr.from >= 0 {
			at, till = clockText(dr.from), clockText(dr.to)
			if dr.to >= 24*60 {
				till = "23:59"
			}
		}
		a.later(func() { a.addTaskAt(text, &day, at, till) })
	}
	switch {
	case in.Focused() && c.Shortcut(0, ui.KeyEscape):
		a.tasks.draft = nil
	case in.Submitted():
		keep()
	case in.Focused():
		dr.focused = true
	case dr.focused: // left
		keep()
	}
}

// weekBlock is a timed task on its hours: on its map's pastel, its title
// and, with room, its time; a click shows it, dragged it goes to another
// hour, and its foot drawn is when it ends. One that comes again is faint,
// its title after ↻. It tells whether the pointer is on it.
func (a *app) weekBlock(c *ui.Context, w weekTask, d time.Time) bool {
	t := c.Theme()
	it := w.it
	to := float32(w.to)
	if s := a.tasks.stretch; s != nil && s.key == it.key() && !w.ghost {
		to = s.to
	}
	top := float32(w.from*hourHeight) / 60
	height := (to-float32(w.from))*hourHeight/60 - 2
	key := "block:" + it.key()
	if w.ghost {
		key = "again:" + it.key() + d.Format("0102")
	}
	left, width := 100*float32(w.lane)/float32(w.lanes), 100/float32(w.lanes)
	_, ink := tinted(t, a.mapColor(it.path))
	// Its pastel laid on the page's white, whole: the hours' lines are not
	// to be seen through it.
	strength := float32(0.14) // tinted's
	if t.Dark {
		strength = 0.22
	}
	bg := t.Background.MixOklab(a.mapColor(it.path), strength)
	b := ui.Column(c.Key(key)).Absolute().Top(top+1).Height(height).LeftPercent(left).WidthPercent(width).
		Padding(3, 6).Radius(radiusSmall).Clip().Cursor(ui.CursorPointer).Background(bg).ZIndex(1).
		BorderWidth(0, 0, 0, 3).BorderColor(a.mapColor(it.path)).Transition(easeLook)
	span := it.node.Task.Span()
	if s := a.tasks.stretch; s != nil && s.key == it.key() && !w.ghost {
		span = strings.TrimPrefix(it.node.Task.Time, "0") + "–" + strings.TrimPrefix(clockText(stretchTo(s.to, w.from)), "0")
	}
	b.Tooltip(it.mapName + " · " + it.node.Title + "  " + span)
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
	on := b.Hovered() || b.Pressed()
	if w.ghost || it.node.Task.Time == "" {
		return on
	}
	// Its foot, drawn: when it ends, by the quarter hour.
	foot := ui.Box(c.Key("foot:" + it.key())).Absolute().Top(top + height - 4).Height(8).LeftPercent(left).WidthPercent(width).
		Cursor(ui.CursorResizeNS).ZIndex(2).Label("调整结束时间 " + it.node.Title).Tooltip("调整结束时间")
	if _, dy, ok := foot.Dragged(); ok {
		if a.tasks.stretch == nil || a.tasks.stretch.key != it.key() {
			a.tasks.stretch = &hourStretch{key: it.key(), to: float32(w.to)}
		}
		a.tasks.stretch.to += dy * 60 / hourHeight
	}
	if s := a.tasks.stretch; s != nil && s.key == it.key() && !foot.Pressed() {
		a.tasks.stretch = nil
		end := stretchTo(s.to, w.from)
		if end != clockMinutes(it.node.Task.EndTime) {
			was := whenOf(it.node.Task)
			a.later(func() {
				till := clockText(min(end, 24*60-1))
				if !a.editTask(it, "改结束时间", func(n *model.Node) { n.Task.EndTime = till }) {
					return
				}
				// Said, with the way back, as a move on the hours is.
				span := strings.TrimPrefix(was.at, "0") + "–" + strings.TrimPrefix(till, "0")
				a.tellUndo("已改到 "+span+"：「"+it.node.Title+"」", func() {
					a.editTask(it, "撤销改结束时间", func(n *model.Node) { was.put(n.Task) })
				})
			})
		}
	}
	return on || foot.Hovered() || foot.Pressed()
}

// stretchTo is where a block's end drawn to to rests: on the quarter
// hour, a quarter after its beginning at least, and in its day.
func stretchTo(to float32, from int) int {
	end := int(math.Round(float64(to)/stretchStep)) * stretchStep
	return min(max(end, from+stretchStep), 24*60)
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

// Where a task dropped on a day goes, when not at a minute of it.
const (
	dropWholeDay = -1 // of the whole day: its time goes
	dropDayAlone = -2 // to the day, its time of day kept: the lists' groups, the month's cells
)

// dropTaskAt makes el take tasks dropped on it, which go to the day: at
// the minute at, of the whole day (dropWholeDay), or the day alone
// changed (dropDayAlone); each said with the way back. It is the one way a
// task is dropped on a day, of the lists, the month and the hours (the
// first two had a copy of it).
func (a *app) dropTaskAt(el ui.Element, day time.Time, at int) {
	t := a.theme()
	if _, over := ui.DragOver[taskDrag](el); over {
		el.Background(t.Accent.Alpha(0.12))
	}
	if d, ok := ui.Drop[taskDrag](el); ok {
		a.moveTaskWhen(d, day, at)
	}
}

// moveTaskWhen puts the task dragged on the day: at the minute at,
// keeping how long it takes, of the whole day or the day alone changed
// (dropWholeDay, dropDayAlone); with the way back.
func (a *app) moveTaskWhen(d taskDrag, day time.Time, at int) {
	it, ok := a.dragged(d)
	if !ok {
		return
	}
	if at == dropDayAlone {
		a.later(func() { a.moveTaskDay(it, &day) })
		return
	}
	was := whenOf(it.node.Task) // the move takes its start off when that is after the new day
	a.later(func() {
		ok := a.editTask(it, "改任务时间", func(n *model.Node) {
			if n.Task == nil {
				n.SetStatus(model.StatusTodo, a.now())
			}
			day := day
			n.Task.Due = &day
			if n.Task.Start != nil && n.Task.Start.After(day) {
				n.Task.Start = nil
			}
			if at < 0 { // of the whole day: it reminds no more at the hour it had
				setTaskTime(n.Task, "", "")
				return
			}
			long := 0
			if n.Task.Time != "" && n.Task.EndTime != "" {
				long = clockMinutes(n.Task.EndTime) - clockMinutes(n.Task.Time)
			}
			until := ""
			if long > 0 && at+long < 24*60 {
				until = clockText(at + long)
			}
			setTaskTime(n.Task, clockText(at), until)
		})
		if !ok {
			return
		}
		when := fmt.Sprintf("%d月%d日", day.Month(), day.Day())
		if at >= 0 {
			when += " " + strings.TrimPrefix(clockText(at), "0")
		} else {
			when += " 全天"
		}
		a.tellUndo("已移到 "+when+"：「"+it.node.Title+"」", func() {
			a.editTask(it, "撤销移动", func(n *model.Node) { was.put(n.Task) })
		})
	})
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
