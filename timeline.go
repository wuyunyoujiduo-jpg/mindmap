package main

import (
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// The timeline: the tasks with dates as bars on days, grouped by branch,
// with today, weekends, iterations and what each task waits on. Dragging
// a bar moves its dates; dragging its right end changes its due date.

const (
	tlLabelW = 220
	tlRowH   = 30
	tlHeadH  = 52
)

// tlRow is a line of the timeline: a group's heading or a task.
type tlRow struct {
	group string
	task  model.TaskNode
}

// tlDrag is a bar being dragged: by how much, and whether by its end.
type tlDrag struct {
	id   string
	dx   float32
	edge bool
}

// span is the days a task covers on the timeline.
func span(task *model.Task) (from, to time.Time, ok bool) {
	switch {
	case task.Start != nil && task.Due != nil:
		return today(*task.Start), today(*task.Due), true
	case task.Due != nil:
		return today(*task.Due), today(*task.Due), true
	case task.Start != nil:
		return today(*task.Start), today(*task.Start), true
	}
	return time.Time{}, time.Time{}, false
}

func daysBetween(a, b time.Time) int {
	return int(math.Round(today(b).Sub(today(a)).Hours() / 24))
}

func (a *app) timelineView(c *ui.Context) {
	t := c.Theme()
	day := today(a.now())
	dayW := float32(28)
	if a.tlWeeks {
		dayW = 10
	}
	var rows []tlRow
	var unplanned []model.TaskNode
	first, last := day.AddDate(0, 0, -3), day.AddDate(0, 0, 21)
	groups := map[string][]model.TaskNode{}
	var order []string
	for _, tn := range a.visibleTasks() {
		if !a.inIteration(tn.Node) {
			continue
		}
		from, to, ok := span(tn.Node.Task)
		if !ok {
			unplanned = append(unplanned, tn)
			continue
		}
		if from.Before(first) {
			first = from.AddDate(0, 0, -2)
		}
		if to.After(last) {
			last = to.AddDate(0, 0, 7)
		}
		g := "（中心）"
		if len(tn.Parents) > 1 {
			g = tn.Parents[1]
		}
		if len(a.file.Sheets) > 1 {
			g = tn.Sheet.Title + " › " + g
		}
		if _, ok := groups[g]; !ok {
			order = append(order, g)
		}
		groups[g] = append(groups[g], tn)
	}
	for _, it := range a.file.Iterations {
		if it.Start != nil && it.Start.Before(first) && a.iterFilter == it.ID {
			first = today(*it.Start)
		}
		if it.End != nil && it.End.After(last) && a.iterFilter == it.ID {
			last = today(*it.End)
		}
	}
	for _, g := range order {
		rows = append(rows, tlRow{group: g})
		tasks := groups[g]
		sortTasksByStart(tasks)
		for _, tn := range tasks {
			rows = append(rows, tlRow{task: tn})
		}
	}
	days := daysBetween(first, last) + 1
	width := tlLabelW + float32(days)*dayW
	x := func(d time.Time) float32 { return tlLabelW + float32(daysBetween(first, d))*dayW }
	rowOf := map[string]int{}
	for i, r := range rows {
		if r.task.Node != nil {
			rowOf[r.task.Node.ID] = i
		}
	}

	ui.Column(c).Grow(1).Children(func() {
		ui.Row(c).Padding(8, 12).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "时间线").Bold()
			scale := 0
			if a.tlWeeks {
				scale = 1
			}
			if ui.Segmented(c, &scale, "按天", "按周").Changed() {
				a.tlWeeks = scale == 1
			}
			a.iterationPicker(c)
			ui.Spacer(c)
			ui.Text(c, "拖动条形调整日期，拖右端改截止日期").FontSize(12).TextColor(t.TextMuted)
		})
		ui.Divider(c)
		if len(rows) == 0 && len(unplanned) == 0 {
			ui.Text(c, "没有任务。在节点详情里勾选「作为任务」并设置日期后会显示在这里。").TextColor(t.TextMuted).Margin(24)
			return
		}
		ui.ScrollBoth(c.Key("timeline")).Grow(1).TrackScroll(&a.tlScroll).Children(func() {
			grid := ui.Column(c).Width(width).Height(tlHeadH + float32(len(rows))*tlRowH + 8)
			grid.Draw(func(p *ui.Painter, r ui.Rect) {
				// Weekends, iterations, the days' heads and today.
				for i := 0; i < days; i++ {
					d := first.AddDate(0, 0, i)
					dx := r.X + tlLabelW + float32(i)*dayW
					if wd := d.Weekday(); wd == time.Saturday || wd == time.Sunday {
						p.Fill(ui.Rect{X: dx, Y: r.Y + tlHeadH, W: dayW, H: r.H - tlHeadH}, t.Text.Alpha(0.035), 0)
					}
					if !a.tlWeeks || d.Weekday() == time.Monday {
						label := d.Format("2")
						if d.Day() == 1 || i == 0 || a.tlWeeks {
							label = d.Format("1/2")
						}
						p.Text(dx+3, r.Y+tlHeadH-18, label, 11, t.TextMuted)
						p.Line(dx, r.Y+tlHeadH-4, dx, r.Y+tlHeadH, 1, t.Border)
					}
				}
				for k, it := range a.file.Iterations {
					if it.Start == nil || it.End == nil {
						continue
					}
					x0, x1 := r.X+x(*it.Start), r.X+x(*it.End)+dayW
					col := ui.Hex(tagPalette[k%len(tagPalette)])
					p.Fill(ui.Rect{X: x0, Y: r.Y + 4, W: x1 - x0, H: 18}, col.Alpha(0.15), 4)
					p.Text(x0+6, r.Y+7, it.Name, 11, col)
				}
				p.Line(r.X+tlLabelW, r.Y+tlHeadH, r.X+r.W, r.Y+tlHeadH, 1, t.Border)
				tx := r.X + x(day) + dayW/2
				p.Line(tx, r.Y+tlHeadH-4, tx, r.Y+r.H, 1.5, t.Danger.Alpha(0.7))
				// What each task waits on: from the end of one bar to the
				// start of the next.
				for i, row := range rows {
					if row.task.Node == nil {
						continue
					}
					for _, id := range row.task.Node.Task.After {
						j, ok := rowOf[id]
						if !ok {
							continue
						}
						_, to, _ := span(rows[j].task.Node.Task)
						from, _, _ := span(row.task.Node.Task)
						x0, y0 := r.X+x(to)+dayW, r.Y+tlHeadH+float32(j)*tlRowH+tlRowH/2
						x1, y1 := r.X+x(from), r.Y+tlHeadH+float32(i)*tlRowH+tlRowH/2
						path := new(ui.Path).MoveTo(x0, y0).CubeTo(x0+12, y0, x1-12, y1, x1, y1)
						p.StrokePath(path, 1.2, ui.Hex("#d97706").Alpha(0.8))
					}
				}
			})
			grid.Children(func() {
				ui.Box(c).Height(tlHeadH)
				// Only the rows in view, and the room of the others.
				_, h := c.Size()
				from := max(0, int((a.tlScroll.Y-tlHeadH)/tlRowH)-5)
				to := min(len(rows), from+int(h/tlRowH)+10)
				from = min(from, to)
				ui.Box(c).Height(float32(from) * tlRowH)
				for i := from; i < to; i++ {
					a.timelineRow(c.Key(fmt.Sprint("row", i)), rows[i], first, dayW, width)
				}
			})
			if len(unplanned) > 0 {
				ui.Column(c).Padding(12).Gap(6).Children(func() {
					ui.Text(c, fmt.Sprintf("未安排日期 %d", len(unplanned))).FontSize(12).Bold().TextColor(t.TextMuted)
					ui.Row(c).Wrap().Gap(6).Children(func() {
						for i, tn := range unplanned {
							if i == 200 {
								ui.Text(c, fmt.Sprintf("还有 %d 个", len(unplanned)-i)).FontSize(12).TextColor(t.TextMuted)
								break
							}
							n := tn.Node
							chip := ui.Row(c.Key(n.ID)).Padding(3, 8).Gap(6).Radius(6).Background(t.Surface).Border(1, t.Border).
								AlignItems(ui.Center).Cursor(ui.CursorPointer).Tooltip("点击排到今天开始，持续 3 天").Label("安排 " + n.Title)
							if chip.Clicked() {
								a.later(func() {
									a.mutate("", func() {
										s, d := today(a.now()), today(a.now()).AddDate(0, 0, 2)
										n.Task.Start, n.Task.Due = &s, &d
									})
								})
							}
							chip.Children(func() {
								ui.Box(c).Size(8, 8).Radius(999).Background(statusColor(n.Task.Status))
								ui.Text(c, n.Title).FontSize(12)
							})
						}
					})
				})
			}
		})
	})
}

func sortTasksByStart(tasks []model.TaskNode) {
	for i := 1; i < len(tasks); i++ {
		for j := i; j > 0; j-- {
			a, _, _ := span(tasks[j].Node.Task)
			b, _, _ := span(tasks[j-1].Node.Task)
			if !a.Before(b) {
				break
			}
			tasks[j], tasks[j-1] = tasks[j-1], tasks[j]
		}
	}
}

// timelineRow is a group's heading, or a task's label and bar.
func (a *app) timelineRow(c *ui.Context, row tlRow, first time.Time, dayW, width float32) {
	t := c.Theme()
	if row.task.Node == nil {
		ui.Row(c).Height(tlRowH).Width(width).AlignItems(ui.Center).Padding(0, 12).Children(func() {
			ui.Text(c, row.group).FontSize(12).Bold().TextColor(t.TextMuted)
		})
		return
	}
	n := row.task.Node
	task := n.Task
	from, to, _ := span(task)
	left := tlLabelW + float32(daysBetween(first, from))*dayW
	w := float32(daysBetween(from, to)+1) * dayW
	if d := a.tlDrag; d.id == n.ID {
		if d.edge {
			w = max(w+d.dx, dayW)
		} else {
			left += d.dx
		}
	}
	line := ui.Row(c).Height(tlRowH).Width(width)
	if n.ID == a.selected {
		line.Background(t.Accent.Alpha(0.07))
	}
	line.Children(func() {
		label := ui.Row(c).Width(tlLabelW).Height(tlRowH).Padding(0, 8, 0, 24).Gap(6).AlignItems(ui.Center).
			Cursor(ui.CursorPointer).Label(n.Title)
		if label.Clicked() {
			a.selectTask(row.task)
		}
		if label.DoubleClicked() {
			a.mode = viewMap
			a.selectNode(n.ID)
		}
		label.Children(func() {
			if len(a.file.Blockers(n)) > 0 && task.Status != model.StatusDone {
				ui.Text(c, "🔒").FontSize(10)
			}
			title := ui.Text(c, n.Title).FontSize(12).MaxLines(1)
			if task.Status == model.StatusDone {
				title.Strikethrough().TextColor(t.TextMuted)
			}
		})
		col := statusColor(task.Status)
		milestone := task.Start == nil && task.Due != nil
		bar := ui.Box(c.Key("bar")).Absolute().Left(left).Top(6).Height(tlRowH - 12).Width(w).Radius(4).
			Background(col.Alpha(0.85)).Cursor(ui.CursorGrab).Tooltip(n.Title + " · " + dateRange(task)).Label("条 " + n.Title)
		if milestone {
			const m = 13 // a diamond on the due day
			bar.Size(m, m).Top((tlRowH - m) / 2).Left(left + dayW/2 - m/2).Radius(2).Rotate(45)
		}
		if task.Overdue(a.now()) {
			bar.Border(2, t.Danger)
		}
		if bar.Pressed() {
			a.selectTask(row.task)
		}
		if dx, _, ok := bar.Dragged(); ok {
			if a.tlDrag.id != n.ID {
				a.tlDrag = tlDrag{id: n.ID}
			}
			a.tlDrag.dx += dx
		} else if a.tlDrag.id == n.ID && !a.tlDrag.edge {
			a.moveDates(n, int(math.Round(float64(a.tlDrag.dx/dayW))), false)
			a.tlDrag = tlDrag{}
		}
		if !milestone {
			bar.Children(func() {
				if w > 70 {
					ui.Text(c, n.Title).FontSize(11).TextColor(textOn(col)).MaxLines(1).Margin(0, 6).PassThrough()
				}
				end := ui.Box(c.Key("end")).Absolute().Right(0).Top(0).Bottom(0).Width(6).Cursor(ui.CursorResizeEW).
					Label("调整截止 " + n.Title)
				if dx, _, ok := end.Dragged(); ok {
					if a.tlDrag.id != n.ID {
						a.tlDrag = tlDrag{id: n.ID, edge: true}
					}
					a.tlDrag.dx += dx
				} else if a.tlDrag.id == n.ID && a.tlDrag.edge {
					a.moveDates(n, int(math.Round(float64(a.tlDrag.dx/dayW))), true)
					a.tlDrag = tlDrag{}
				}
			})
		}
	})
}

// moveDates moves a task's dates by days, or only its due date (end).
func (a *app) moveDates(n *model.Node, days int, end bool) {
	if days == 0 {
		return
	}
	shift := func(d *time.Time) *time.Time {
		if d == nil {
			return nil
		}
		x := d.AddDate(0, 0, days)
		return &x
	}
	a.mutate("", func() {
		task := n.Task
		if end {
			if task.Due == nil {
				task.Due = shift(task.Start)
			} else {
				task.Due = shift(task.Due)
			}
			if task.Start != nil && task.Due.Before(*task.Start) {
				s := *task.Due
				task.Start = &s
			}
			return
		}
		task.Start, task.Due = shift(task.Start), shift(task.Due)
	})
}
