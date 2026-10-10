package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// visibleTasks returns the tasks that pass the search and tag filter.
func (a *app) visibleTasks() []model.TaskNode {
	var out []model.TaskNode
	for _, tn := range a.file.Tasks() {
		if a.highlighted(tn.Node) {
			out = append(out, tn)
		}
	}
	return out
}

// sortTasks puts the tasks due first first, then the most important.
func sortTasks(tasks []model.TaskNode) {
	sort.SliceStable(tasks, func(i, j int) bool {
		ti, tj := tasks[i].Node.Task, tasks[j].Node.Task
		switch {
		case ti.Due != nil && tj.Due == nil:
			return true
		case ti.Due == nil && tj.Due != nil:
			return false
		case ti.Due != nil && !ti.Due.Equal(*tj.Due):
			return ti.Due.Before(*tj.Due)
		}
		return ti.Priority > tj.Priority
	})
}

// boardView shows the tasks in a column per status; dragging a card to
// another column changes its status.
func (a *app) boardView(c *ui.Context) {
	t := c.Theme()
	tasks := a.visibleTasks()
	sortTasks(tasks)
	ui.Row(c).Grow(1).Padding(12).Gap(12).AlignItems(ui.Stretch).Children(func() {
		for _, s := range model.Statuses {
			var col []model.TaskNode
			for _, tn := range tasks {
				if tn.Node.Task.Status == s {
					col = append(col, tn)
				}
			}
			column := ui.Column(c.Key(string(s))).Grow(1).Basis(0).Radius(10).Background(t.Surface).Border(1, t.Border)
			if _, over := ui.DragOver[nodeDrag](column); over {
				column.Border(2, statusColor(s))
			}
			if d, ok := ui.Drop[nodeDrag](column); ok {
				if sheet := a.file.SheetOf(d.ID); sheet != nil {
					if n, _ := sheet.Find(d.ID); n != nil && n.Task != nil && n.Task.Status != s {
						a.setStatus(n, s)
					}
				}
			}
			column.Children(func() {
				ui.Row(c).Padding(10, 12).Gap(8).Children(func() {
					ui.Box(c).Size(8, 8).Radius(999).Background(statusColor(s))
					ui.Text(c, s.Label()).Bold()
					ui.Text(c, fmt.Sprint(len(col))).TextColor(t.TextMuted)
				})
				ui.Scroll(c).Grow(1).Padding(0, 8, 8).Gap(8).Children(func() {
					for _, tn := range col {
						a.taskCard(c.Key(tn.Node.ID), tn, false)
					}
					if len(col) == 0 {
						ui.Text(c, "拖动卡片到这里").FontSize(12).TextColor(t.TextMuted).Margin(8, 4)
					}
				})
			})
		}
	})
}

// taskCard shows a task: its title, where it is in the map, its tags and
// dates. A click selects it; a double click shows it in the map.
func (a *app) taskCard(c *ui.Context, tn model.TaskNode, showStatus bool) {
	t := c.Theme()
	n := tn.Node
	task := n.Task
	card := ui.Column(c).Padding(10).Gap(6).Radius(8).Background(t.Background).Border(1, t.Border).
		Drag(nodeDrag{n.ID}).Cursor(ui.CursorPointer).Label(n.Title)
	if n.ID == a.selected {
		card.Border(2, t.Accent)
	}
	if card.Pressed() {
		a.selectTask(tn)
	}
	if card.DoubleClicked() {
		a.mode = viewMap
		a.selectTask(tn)
		a.selectNode(n.ID)
		a.canvas.Focus()
	}
	if card.Dragging() {
		card.Opacity(0.4)
	}
	card.ContextMenu(func(m *ui.Menu) {
		a.selectTask(tn)
		if m.Item("在导图中显示").Chosen() {
			a.mode = viewMap
			a.selectNode(n.ID)
		}
		m.Submenu("任务状态", func(m *ui.Menu) {
			for _, s := range model.Statuses {
				if m.Item(s.Label()).Checked(task.Status == s).Chosen() {
					a.setStatus(n, s)
				}
			}
		})
	})
	card.Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Start).Children(func() {
			if showStatus {
				ui.Box(c).Size(8, 8).Radius(999).Background(statusColor(task.Status)).Margin(6, 0, 0, 0)
			}
			title := ui.Text(c, n.Title).Grow(1).Wrap()
			if task.Status == model.StatusDone {
				title.Strikethrough().TextColor(t.TextMuted)
			}
			if task.Priority != model.PriorityNone {
				ui.Text(c, task.Priority.Label()).FontSize(11).TextColor(priorityColor(c, task.Priority)).NoWrap()
			}
		})
		where := tn.Parents
		if len(where) > 0 {
			where = where[1:] // the root says little
		}
		if len(a.file.Sheets) > 1 && tn.Sheet != nil {
			where = append([]string{tn.Sheet.Title}, where...)
		}
		if len(where) > 0 {
			ui.Text(c, strings.Join(where, " › ")).FontSize(11).TextColor(t.TextMuted).NoWrap().Ellipsis("…")
		}
		if len(n.Tags) > 0 || task.Start != nil || task.Due != nil {
			ui.Row(c).Wrap().Gap(6).AlignItems(ui.Center).Children(func() {
				for _, tag := range n.Tags {
					tagChip(c.Key("tag:"+tag), tag, tag, false, 11).PassThrough()
				}
				if d := dateRange(task); d != "" {
					col := t.TextMuted
					if task.Overdue(a.now()) {
						col = t.Danger
					}
					ui.Text(c, d).FontSize(11).TextColor(col).NoWrap()
				}
			})
		}
	})
}

func priorityColor(c *ui.Context, p model.Priority) ui.Color {
	switch p {
	case model.PriorityHigh:
		return c.Theme().Danger
	case model.PriorityMedium:
		return ui.Hex("#d97706")
	}
	return c.Theme().TextMuted
}

func dateRange(task *model.Task) string {
	switch {
	case task.Start != nil && task.Due != nil:
		return task.Start.Format("1/2") + " – " + task.Due.Format("1/2")
	case task.Due != nil:
		return "截止 " + task.Due.Format("1/2")
	case task.Start != nil:
		return "开始 " + task.Start.Format("1/2")
	}
	return ""
}

// agendaGroup is a span of days of the agenda.
type agendaGroup struct {
	title string
	tasks []model.TaskNode
}

// agenda sorts the unfinished tasks into overdue, today, this week, later
// and unscheduled, by their due date, else their start date.
func agenda(tasks []model.TaskNode, now time.Time) []agendaGroup {
	day := today(now)
	weekEnd := day.AddDate(0, 0, 7)
	groups := []agendaGroup{{title: "已逾期"}, {title: "今天"}, {title: "未来 7 天"}, {title: "以后"}, {title: "未安排日期"}}
	for _, tn := range tasks {
		task := tn.Node.Task
		if task.Status == model.StatusDone {
			continue
		}
		date := task.Due
		if date == nil {
			date = task.Start
		}
		var g int
		switch {
		case date == nil:
			g = 4
		case date.Before(day):
			g = 0
			if task.Due == nil { // started in the past, with no deadline
				g = 1
			}
		case date.Before(day.AddDate(0, 0, 1)):
			g = 1
		case date.Before(weekEnd):
			g = 2
		default:
			g = 3
		}
		groups[g].tasks = append(groups[g].tasks, tn)
	}
	return groups
}

func (a *app) agendaView(c *ui.Context) {
	t := c.Theme()
	tasks := a.visibleTasks()
	sortTasks(tasks)
	groups := agenda(tasks, a.now())
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(16, 24).Gap(18).MaxWidth(760).Children(func() {
			any := false
			for _, g := range groups {
				if len(g.tasks) == 0 {
					continue
				}
				any = true
				ui.Column(c.Key(g.title)).Gap(8).Children(func() {
					head := ui.Text(c, fmt.Sprintf("%s  %d", g.title, len(g.tasks))).Bold()
					if g.title == "已逾期" {
						head.TextColor(t.Danger)
					}
					for _, tn := range g.tasks {
						a.taskCard(c.Key(tn.Node.ID), tn, true)
					}
				})
			}
			if !any {
				ui.Text(c, "没有未完成的任务。在节点的详情面板中勾选「作为任务」来添加。").TextColor(t.TextMuted).Wrap()
			}
		})
	})
}
