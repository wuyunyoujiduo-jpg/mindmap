package main

import (
	"fmt"
	"strconv"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// The overview: how the map's tasks stand, by branch and iteration; what
// is overdue, due this week, blocked, and done lately.

func (a *app) overviewView(c *ui.Context) {
	t := c.Theme()
	now := a.now()
	day := today(now)
	weekEnd := day.AddDate(0, 0, 7)
	var all, done, doing, overdue, doneWeek int
	var hoursLeft float64
	var late, dueSoon, blocked, recent []model.TaskNode
	for _, tn := range a.file.Tasks() {
		task := tn.Node.Task
		all++
		switch task.Status {
		case model.StatusDone:
			done++
			if task.DoneAt != nil && task.DoneAt.After(day.AddDate(0, 0, -7)) {
				doneWeek++
				recent = append(recent, tn)
			}
			continue
		case model.StatusDoing:
			doing++
		}
		hoursLeft += task.Estimate
		if task.Overdue(now) {
			overdue++
			late = append(late, tn)
		} else if task.Due != nil && task.Due.Before(weekEnd) {
			dueSoon = append(dueSoon, tn)
		}
		if len(a.file.Blockers(tn.Node)) > 0 {
			blocked = append(blocked, tn)
		}
	}
	sortTasks(late)
	sortTasks(dueSoon)
	pct := 0
	if all > 0 {
		pct = done * 100 / all
	}
	ui.Scroll(c.Key("overview")).Grow(1).Children(func() {
		ui.Column(c).Padding(20, 24).Gap(22).MaxWidth(980).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "总览 · "+mapName(a.path)).FontSize(18).Bold().Grow(1)
				on := !a.cfg.NoReminders
				ui.Text(c, "到期提醒").FontSize(12).TextColor(t.TextMuted).Margin(0, 8, 0, 0)
				if ui.Switch(c, &on).Label("到期提醒").Tooltip("任务今天到期或已逾期时，每天提醒一次（系统通知）").Changed() {
					a.cfg.NoReminders = !on
					a.cfg.save()
				}
			})
			ui.Row(c).Gap(12).Wrap().Children(func() {
				tile := func(label, value string, col ui.Color) {
					ui.Column(c.Key(label)).Padding(12, 16).Gap(4).Radius(10).Background(t.Surface).Border(1, t.Border).MinWidth(130).Children(func() {
						ui.Text(c, label).FontSize(12).TextColor(t.TextMuted)
						ui.Text(c, value).FontSize(24).Bold().TextColor(col)
					})
				}
				tile("任务", strconv.Itoa(all), t.Text)
				tile("已完成", fmt.Sprintf("%d · %d%%", done, pct), ui.Hex("#16a34a"))
				tile("进行中", strconv.Itoa(doing), ui.Hex("#f59e0b"))
				tile("已逾期", strconv.Itoa(overdue), map[bool]ui.Color{true: t.Danger, false: t.Text}[overdue > 0])
				tile("近 7 天完成", strconv.Itoa(doneWeek), t.Accent)
				tile("剩余估时", strconv.FormatFloat(hoursLeft, 'f', -1, 64)+" 小时", t.Text)
			})
			a.section2(c, "各分支进度", func() {
				for _, d := range a.file.Sheets {
					for _, b := range d.Root.Children {
						bd, bt := b.Progress()
						if b.Task != nil {
							bt++
							if b.Task.Status == model.StatusDone {
								bd++
							}
						}
						if bt == 0 {
							continue
						}
						name := b.Title
						if len(a.file.Sheets) > 1 {
							name = d.Title + " › " + name
						}
						id, sheet := b.ID, d.ID
						row := ui.Row(c.Key(b.ID)).Gap(12).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(name)
						if row.Clicked() {
							a.later(func() { a.goToNode(a.path, sheet, id) })
						}
						row.Children(func() {
							ui.Text(c, name).FontSize(13).Width(220).MaxLines(1)
							progressBar(c, float32(bd)/float32(bt), 280)
							ui.Text(c, fmt.Sprintf("%d/%d", bd, bt)).FontSize(12).TextColor(t.TextMuted)
						})
					}
				}
			})
			a.section2(c, "迭代", func() { a.iterationsEditor(c) })
			list := func(title string, tasks []model.TaskNode, col ui.Color) {
				if len(tasks) == 0 {
					return
				}
				a.section2(c, fmt.Sprintf("%s %d", title, len(tasks)), func() {
					ui.Row(c).Wrap().Gap(10).Children(func() {
						for _, tn := range tasks {
							ui.Box(c.Key(tn.Node.ID)).Width(300).Children(func() { a.taskCard(c, tn, true) })
						}
					})
				})
				_ = col
			}
			list("已逾期", late, t.Danger)
			list("7 天内到期", dueSoon, t.Text)
			list("被阻塞（等待前置任务）", blocked, t.Text)
			list("近 7 天完成", recent, t.Text)
			if all == 0 {
				ui.Text(c, "这个导图还没有任务。选中节点，在右侧勾选「作为任务」即可。").TextColor(t.TextMuted)
			}
		})
	})
}

// section2 is a titled part of the overview.
func (a *app) section2(c *ui.Context, title string, fn func()) {
	t := c.Theme()
	ui.Column(c.Key(title)).Gap(10).Children(func() {
		ui.Text(c, title).FontSize(14).Bold().TextColor(t.TextMuted)
		fn()
	})
}

// progressBar shows a share done.
func progressBar(c *ui.Context, frac float32, width float32) {
	t := c.Theme()
	ui.Box(c).Width(width).Height(8).Radius(999).Background(t.Border).Children(func() {
		ui.Box(c).Width(width * max(0, min(frac, 1))).Height(8).Radius(999).Background(ui.Hex("#22c55e"))
	})
}
