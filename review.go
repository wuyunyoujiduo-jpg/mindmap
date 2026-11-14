package main

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
)

// weekReview is the week looked back on, a list of 今天's left column: in
// figures, then what was done in it, what is late and still to do (put on
// today in one go), the thoughts kept in it, and how much still waits for
// a place. Nothing is asked of an AI: it is what the maps and 收集 hold.
func (a *app) weekReview(c *ui.Context, all []taskItem) {
	t := c.Theme()
	now := a.now()
	day := today(now)
	back := a.tasks.reviewBack
	from := weekStart(day).AddDate(0, 0, -7*back)
	to := from.AddDate(0, 0, 7)
	var done, late []taskItem
	for _, it := range all {
		task := it.node.Task
		switch {
		case it.done():
			if at := task.DoneAt; at != nil && !at.Before(from) && at.Before(to) {
				done = append(done, it)
			}
		case back == 0 && task.Due != nil && task.Due.Before(day): // what is late is late now, of no past week
			late = append(late, it)
		}
	}
	slices.SortStableFunc(done, func(x, y taskItem) int { return y.node.Task.DoneAt.Compare(*x.node.Task.DoneAt) })
	var kept []*library.Clip
	for _, cl := range a.lib.Clips {
		if isMemo(cl) && !cl.Created.Before(from) && cl.Created.Before(to) {
			kept = append(kept, cl)
		}
	}
	slices.SortStableFunc(kept, func(x, y *library.Clip) int { return y.Created.Compare(x.Created) })
	waiting := 0
	if back == 0 {
		waiting = len(a.waiting())
	}

	last := to.AddDate(0, 0, -1)
	which := "这一周"
	switch {
	case back == 1:
		which = "上一周"
	case back > 1:
		which = fmt.Sprintf("%d 周前", back)
	}
	// Which week, the way to the one before and back, and the week as a
	// report to paste elsewhere.
	ui.Row(c.Key("review-week")).Gap(4).AlignItems(ui.Center).FillWidth().Children(func() {
		if iconButton(c, iconBack, "上一周", "往前一周", 24, true).Clicked() {
			a.tasks.reviewBack++
		}
		if iconButton(c, iconForward, "下一周", "往后一周", 24, back > 0).Clicked() && back > 0 {
			a.tasks.reviewBack--
		}
		ui.Text(c, fmt.Sprintf("%s：%d月%d日 – %d月%d日", which, from.Month(), from.Day(), last.Month(), last.Day())).FontSize(textSmall).TextColor(t.TextMuted).Margin(0, 0, 0, 4)
		ui.Spacer(c)
		if len(done)+len(late)+len(kept) > 0 && textLink(c, "复制成周报", "把这一周复制成一段 Markdown").Clicked() {
			a.plat.CopyText(weekReport(which, from, last, done, late, kept))
			a.tell("周报已复制，可以粘贴到别处")
		}
	})
	figures(c, figure{len(done), "件做完"}, figure{len(late), "件拖着"}, figure{len(kept), "条灵感"}, figure{waiting, "条等着归位"})
	if len(done)+len(late)+len(kept) == 0 {
		emptyState(c, "review", "history", "这周还没有可以回顾的", "做完的任务、逾期没做的任务和这周记下的灵感，会汇总在这里。", nil)
		return
	}
	part := func(key, title string, n int, right func(), body func()) {
		if n == 0 {
			return
		}
		ui.Column(c.Key(key)).Margin(14, 0, 0).FillWidth().Children(func() {
			trayHead(c, "head", title, fmt.Sprint(n), right)
			body()
		})
	}
	part("review-done", "这周做完的", len(done), nil, func() {
		listCard(c, "rows").Children(func() {
			for _, it := range done {
				a.taskRow(c, it, false)
			}
		})
	})
	part("review-late", "拖着的", len(late), func() {
		if textLink(c, "顺延到今天", "把拖着的都改到今天").Clicked() {
			items := late
			a.later(func() { a.postponeToToday(items) })
		}
	}, func() {
		listCard(c, "rows").Children(func() {
			for _, it := range late {
				a.taskRow(c, it, true)
			}
		})
	})
	part("review-kept", "这周记下的灵感", len(kept), nil, func() {
		for i, cl := range kept {
			if i == 12 {
				ui.Text(c, fmt.Sprintf("还有 %d 条，在收集里", len(kept)-i)).FontSize(textCaption).TextColor(faint(t)).Margin(6, 8, 0)
				break
			}
			id := cl.ID
			text, _, _ := strings.Cut(strings.TrimSpace(library.Mask(cl.Text)), "\n")
			row := ui.Row(c.Key("kept:"+id)).MinHeight(40).Padding(6, 8).Gap(10).AlignItems(ui.Center).FillWidth().Cursor(ui.CursorPointer).
				Label("灵感 "+cl.Label()).BorderWidth(0, 0, 1, 0).BorderColor(t.Border.Alpha(0.55)).Background(t.Text.Alpha(0)).Transition(rowMotion)
			if row.Hovered() {
				row.Background(t.Text.Alpha(0.035))
			}
			if row.Clicked() {
				a.later(func() { a.showClip(id) })
			}
			row.Children(func() {
				ui.Box(c).Size(6, 6).Radius(3).Background(clipColor(cl.Kind)).Shrink(0)
				ui.Text(c, text).FontSize(textBody).SingleLine().Grow(1).Shrink(1).MinWidth(0).PassThrough()
				ui.Text(c, reviewDay(cl.Created, now)).FontSize(textCaption).TextColor(t.TextMuted).NoWrap().PassThrough()
			})
		}
	})
	if waiting > 0 {
		ui.Row(c.Key("review-waiting")).Margin(16, 0, 0).Padding(0, 8).Gap(8).AlignItems(ui.Center).FillWidth().Children(func() {
			ui.Text(c, fmt.Sprintf("还有 %d 条等着归位", waiting)).FontSize(textSmall).TextColor(t.TextMuted)
			if textLink(c, "去整理 ›", "去整理").Clicked() {
				a.tasks.view = tasksWaiting
			}
		})
	}
}

// reviewDay is the day of the week a thing was kept on, 今天 for today.
func reviewDay(at, now time.Time) string {
	if sameDay(at, now) {
		return "今天"
	}
	return weekdayName(at)
}

// weekReport is a week as a report in Markdown: what was done, by map,
// what is late, the thoughts kept.
func weekReport(which string, from, last time.Time, done, late []taskItem, kept []*library.Clip) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# 周报（%d月%d日 – %d月%d日）\n", from.Month(), from.Day(), last.Month(), last.Day())
	if len(done) > 0 {
		fmt.Fprintf(&b, "\n## 做完的（%d）\n", len(done))
		var maps []string
		by := map[string][]taskItem{}
		for _, it := range done {
			if _, ok := by[it.mapName]; !ok {
				maps = append(maps, it.mapName)
			}
			by[it.mapName] = append(by[it.mapName], it)
		}
		for _, m := range maps {
			fmt.Fprintf(&b, "\n**%s**\n", m)
			for _, it := range by[m] {
				fmt.Fprintf(&b, "- %s\n", it.node.Title)
			}
		}
	}
	if len(late) > 0 {
		fmt.Fprintf(&b, "\n## 拖着的（%d）\n\n", len(late))
		for _, it := range late {
			fmt.Fprintf(&b, "- %s（%s，原定 %d月%d日）\n", it.node.Title, it.mapName, it.node.Task.Due.Month(), it.node.Task.Due.Day())
		}
	}
	if len(kept) > 0 {
		fmt.Fprintf(&b, "\n## 记下的灵感（%d）\n\n", len(kept))
		for _, cl := range kept {
			text, _, _ := strings.Cut(strings.TrimSpace(library.Mask(cl.Text)), "\n")
			fmt.Fprintf(&b, "- %s\n", text)
		}
	}
	return b.String()
}
