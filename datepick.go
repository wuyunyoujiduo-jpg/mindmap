package main

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
)

// The small calendar a day is chosen in (a task's day, when a repeat
// ends) is the app's own, as the calendars of 今天 are: the system's
// (MyGo's DateInput) knows nothing of the days of rest. It goes by their
// two rules: a day of rest on its ground with its figure in the muted
// ink (restDay, restFill), and a mark for what is not as the week has
// it, 休 on a day the notice gives off and 班 on a weekend day worked; its
// weeks begin as the settings say.

// datePick is a date field's calendar: open or not, and the day the keys
// are on.
type datePick struct {
	open   bool
	cursor time.Time
}

const (
	pickCellW = 36
	pickCellH = 32
)

// dateField is a day to change: a key that says it, and under it, opened,
// the month to choose another in. It reports whether one was chosen, and
// then *d is that day, with the time of day it had.
func (a *app) dateField(c *ui.Context, key, label string, d *time.Time) bool {
	t := c.Theme()
	text := fmt.Sprintf("%d月%d日 %s", d.Month(), d.Day(), weekdays[d.Weekday()])
	if d.Year() != a.now().Year() {
		text = fmt.Sprintf("%d年%s", d.Year(), text)
	}
	b := ui.Row(c.Key(key)).Height(28).Padding(0, 9, 0, 10).Gap(8).Radius(radiusSmall).AlignItems(ui.Center).Shrink(0).
		Background(blockFill(t)).Transition(easeLook).Cursor(ui.CursorPointer).Focusable().Label(label).Tooltip("换一天")
	st := ui.Local(b, "pick", func() datePick { return datePick{} })
	if b.Hovered() || st.open {
		b.Background(t.Text.Alpha(0.08))
	}
	if b.Clicked() || !st.open && (b.Shortcut(0, ui.KeyEnter) || b.Shortcut(0, ui.KeySpace)) {
		st.open = !st.open
		st.cursor = *d
	}
	b.Children(func() {
		ui.Text(c, text).FontSize(textSmall).FontFeatures("tnum").NoWrap().PassThrough()
		ui.Box(c).Size(14, 14).Shrink(0).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) { // a calendar's page
			p.Stroke(ui.Rect{X: r.X + 1, Y: r.Y + 2, W: r.W - 2, H: r.H - 3}, t.TextMuted, 2, 1.2)
			p.Fill(ui.Rect{X: r.X + 1, Y: r.Y + 5, W: r.W - 2, H: 1.2}, t.TextMuted, 0)
		})
	})
	changed := false
	ui.PopoverBase(c.Key(key+":calendar"), b, &st.open, func(panel ui.Element) {
		lifted(panel.Padding(10).Radius(radiusCard).Background(cardColor(t)), t).Children(func() {
			if day, ok := a.monthPick(c, *d, &st.cursor); ok {
				h, m, s := d.Clock()
				*d = time.Date(day.Year(), day.Month(), day.Day(), h, m, s, d.Nanosecond(), d.Location())
				st.open, changed = false, true
			}
		})
	})
	return changed
}

// monthPick is the month of *cursor to choose a day in, chosen being the
// day that is: a click chooses, as Enter does of the day the arrows are
// on; the keys at its head, Page Up and Page Down turn the months.
func (a *app) monthPick(c *ui.Context, chosen time.Time, cursor *time.Time) (day time.Time, ok bool) {
	t := c.Theme()
	cur := *cursor
	now := a.now()
	month := time.Date(cur.Year(), cur.Month(), 1, 0, 0, 0, 0, cur.Location())
	turn := func(by int) {
		// The same day of the month turned to, or its last.
		last := time.Date(cur.Year(), cur.Month()+time.Month(by)+1, 0, 0, 0, 0, 0, cur.Location())
		*cursor = time.Date(last.Year(), last.Month(), min(cur.Day(), last.Day()), 0, 0, 0, 0, cur.Location())
	}
	grid := ui.Column(c.Key("month-pick")).Gap(2).Shrink(0).Focusable().AutoFocus().Label(fmt.Sprintf("%d年%d月", month.Year(), month.Month()))
	switch {
	case grid.Shortcut(0, ui.KeyLeft):
		*cursor = cur.AddDate(0, 0, -1)
	case grid.Shortcut(0, ui.KeyRight):
		*cursor = cur.AddDate(0, 0, 1)
	case grid.Shortcut(0, ui.KeyUp):
		*cursor = cur.AddDate(0, 0, -7)
	case grid.Shortcut(0, ui.KeyDown):
		*cursor = cur.AddDate(0, 0, 7)
	case grid.Shortcut(0, ui.KeyPageUp):
		turn(-1)
	case grid.Shortcut(0, ui.KeyPageDown):
		turn(1)
	case grid.Shortcut(0, ui.KeyEnter), grid.Shortcut(0, ui.KeySpace):
		day, ok = cur, true
	}
	grid.Children(func() {
		ui.Row(c).AlignItems(ui.Center).Padding(0, 2, 4, 8).Children(func() {
			ui.Text(c, fmt.Sprintf("%d年%d月", month.Year(), month.Month())).FontSize(textBody).FontWeight(700).FontFeatures("tnum").Grow(1)
			if !sameDay(cur, now) {
				back := ui.Box(c.Key("pick-today")).Height(24).Padding(0, 8).Radius(radiusSmall).Center().Cursor(ui.CursorPointer).Shrink(0).
					Label("回到今天").Background(t.Accent.Alpha(0)).Transition(easeLook)
				if back.Hovered() {
					back.Background(t.Accent.Alpha(0.09))
				}
				if back.Clicked() {
					*cursor = today(now)
				}
				back.Children(func() {
					ui.Text(c, "今天").FontSize(textCaption).FontWeight(700).TextColor(t.Accent).NoWrap().PassThrough()
				})
			}
			if iconButton(c, iconBack, "上个月", "上个月", 26, true).Clicked() {
				turn(-1)
			}
			if iconButton(c, iconForward, "下个月", "下个月", 26, true).Clicked() {
				turn(1)
			}
		})
		start := a.weekOf(month)
		ui.Row(c).Children(func() {
			for i := range 7 {
				// Saturday and Sunday named faintly, as the month's are.
				wd, ink := start.AddDate(0, 0, i).Weekday(), t.TextMuted
				if wd == time.Saturday || wd == time.Sunday {
					ink = faint(t)
				}
				ui.Text(c, weekdays[wd][len("周"):]).Width(pickCellW).TextAlign(ui.Center).FontSize(textCaption).TextColor(ink).Padding(0, 0, 4)
			}
		})
		for w := range 6 {
			ui.Row(c.Key(fmt.Sprint("pick-week:", w))).Children(func() {
				for i := range 7 {
					d := start.AddDate(0, 0, w*7+i)
					if a.pickDay(c, d, d.Month() == month.Month(), sameDay(d, chosen), sameDay(d, cur), sameDay(d, now)) {
						day, ok = d, true
					}
				}
			})
		}
	})
	return day, ok
}

// pickDay is a day of the small calendar: its figure, on the ground of
// rest if it is a day of rest, and at its corner 休 or 班 where the
// notice names it. The day chosen is on the accent, today's figure in it.
func (a *app) pickDay(c *ui.Context, d time.Time, in, chosen, atCursor, isToday bool) bool {
	t := c.Theme()
	rest := restDay(d)
	h, named := holidayOn(d)
	says := fmt.Sprintf("%d月%d日 %s", d.Month(), d.Day(), weekdays[d.Weekday()])
	mark, markInk := "", t.Success
	if named {
		mark, says = "休", says+" "+h.Name+"假期"
		if !h.Off {
			mark, markInk, says = "班", t.Danger, fmt.Sprintf("%d月%d日 %s %s调休，上班", d.Month(), d.Day(), weekdays[d.Weekday()], h.Name)
		}
	}
	// A cell is a little apart from the next, so that the grounds of the
	// days of rest read as days and the day chosen as one of them.
	cell := ui.Box(c.Key(d.Format("pick:2006-01-02"))).Size(pickCellW, pickCellH).Padding(1).Cursor(ui.CursorPointer).Label(says).Tooltip(says)
	clicked := cell.Clicked()
	hovered := cell.Hovered()
	cell.Children(func() {
		face := ui.Box(c).FillWidth().FillHeight().Radius(radiusSmall).Center().PassThrough().Transition(easeLook)
		ink := t.Text
		switch {
		case !in:
			ink = faint(t)
		case rest:
			ink = t.TextMuted
		}
		switch {
		case chosen:
			face.Background(t.Accent)
			ink, markInk = t.AccentText, t.AccentText
		case hovered || atCursor:
			face.Background(t.Text.Alpha(0.09))
		case rest:
			face.Background(restFill(t))
		}
		if isToday && !chosen {
			ink = t.Accent
		}
		face.Children(func() {
			num := ui.Text(c, fmt.Sprint(d.Day())).FontSize(textSmall).FontFeatures("tnum").TextColor(ink)
			if isToday || chosen {
				num.FontWeight(700)
			}
			if mark != "" {
				ui.Text(c, mark).Absolute().Top(1).Right(2).FontSize(9).FontWeight(700).TextColor(markInk)
			}
		})
	})
	return clicked
}
