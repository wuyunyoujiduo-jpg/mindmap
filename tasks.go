package main

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// visibleTasks returns the tasks that pass the search and tag filter.
func (a *app) visibleTasks() []model.TaskNode {
	var out []model.TaskNode
	for _, tn := range a.file.Tasks() {
		if a.highlighted(tn.Node) && a.inIteration(tn.Node) {
			out = append(out, tn)
		}
	}
	return out
}

// keyMarker is the red flag: the nodes it marks are the board's 重点问题,
// whatever their status, and whether tasks or not.
const keyMarker = "flag:red"

// keyIssues returns the nodes of every canvas marked as 重点问题 that pass
// the search and tag filter, in map order.
func (a *app) keyIssues() []model.TaskNode {
	var out []model.TaskNode
	for _, d := range a.file.Sheets {
		var walk func(n *model.Node, parents []string)
		walk = func(n *model.Node, parents []string) {
			if slices.Contains(n.Markers, keyMarker) && a.highlighted(n) && (n.Task == nil || a.inIteration(n)) {
				out = append(out, model.TaskNode{Node: n, Parents: slices.Clone(parents), Sheet: d})
			}
			parents = append(parents, n.Title)
			for _, c := range n.Children {
				walk(c, parents)
			}
		}
		if d.Root != nil {
			walk(d.Root, nil)
		}
	}
	return out
}

// setKeyIssue marks a node as 重点问题, or takes the mark off.
func (a *app) setKeyIssue(n *model.Node, on bool) {
	if slices.Contains(n.Markers, keyMarker) == on {
		return
	}
	a.toggleMarker(n, keyMarker)
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
	ui.Column(c).Grow(1).Children(func() {
		if a.file != nil && len(a.file.Iterations) > 0 {
			ui.Row(c).Padding(10, 12, 0).Gap(8).AlignItems(ui.Center).Children(func() {
				a.iterationPicker(c)
				if a.iterFilter != "" && a.iterFilter != "-" {
					done, total, hours := a.iterationProgress(a.iterFilter)
					ui.Text(c, fmt.Sprintf("%d/%d 完成 · 剩余 %s 小时", done, total, hours)).FontSize(12).TextColor(t.TextMuted)
				}
			})
		}
		a.boardColumns(c, tasks)
	})
}

func (a *app) boardColumns(c *ui.Context, tasks []model.TaskNode) {
	t := c.Theme()
	ui.Row(c).Grow(1).Padding(12).Gap(12).AlignItems(ui.Stretch).Children(func() {
		a.keyIssueColumn(c)
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
					if n, _ := sheet.Find(d.ID); n != nil && (n.Task == nil || n.Task.Status != s) { // a 重点问题 not a task becomes one
						a.setStatus(n, s)
					}
				}
			}
			column.Children(func() {
				ui.Row(c).Padding(10, 12).Gap(8).AlignItems(ui.Center).Children(func() {
					statusIcon(c, s, 15)
					ui.Text(c, s.Label()).Bold()
					ui.Text(c, fmt.Sprint(len(col))).TextColor(t.TextMuted)
				})
				if len(col) == 0 {
					ui.Text(c, "拖动卡片到这里").FontSize(12).TextColor(t.TextMuted).Margin(8, 12)
					ui.Spacer(c)
					return
				}
				// A virtual list: only the cards in view are built.
				st := a.boardList(s)
				if i := slices.IndexFunc(col, func(tn model.TaskNode) bool { return tn.Node.ID == a.boardReveal }); i >= 0 {
					st.ScrollIntoView(i)
					a.boardReveal = ""
				}
				ui.List(c, st, len(col)).Grow(1).ItemKey(func(i int) any { return col[i].Node.ID }).Rows(func(r ui.ListRow) {
					ui.Box(r.Context).Padding(0, 8, 8).Children(func() {
						a.taskCard(r.Context, col[r.Index], false)
					})
				})
			})
		}
	})
}

func (a *app) boardList(s model.Status) *ui.ListState {
	if a.boardLists == nil {
		a.boardLists = map[model.Status]*ui.ListState{}
	}
	if a.boardLists[s] == nil {
		a.boardLists[s] = &ui.ListState{}
	}
	return a.boardLists[s]
}

// showOnBoard shows the task with the ID on the board, selected and in
// view: a filter by iteration that hides it is cleared.
func (a *app) showOnBoard(id string) {
	n, _ := a.doc.Find(id)
	if n == nil || n.Task == nil {
		return
	}
	if !a.inIteration(n) {
		a.iterFilter = ""
	}
	a.selectNode(id)
	a.mode = viewBoard
	a.boardReveal = id
}

// keyIssueColumn is the board's column of 重点问题: the nodes marked with
// the red flag, which stay in their status's column too. A card dropped
// on it is marked.
func (a *app) keyIssueColumn(c *ui.Context) {
	t := c.Theme()
	col := a.keyIssues()
	red := ui.Hex("#ef4444")
	column := ui.Column(c.Key("key-issues")).Grow(1).Basis(0).Radius(10).Background(red.Alpha(0.05)).Border(1, red.Alpha(0.35))
	if _, over := ui.DragOver[nodeDrag](column); over {
		column.Border(2, red)
	}
	if d, ok := ui.Drop[nodeDrag](column); ok {
		if sheet := a.file.SheetOf(d.ID); sheet != nil {
			if n, _ := sheet.Find(d.ID); n != nil {
				a.setKeyIssue(n, true)
			}
		}
	}
	column.Children(func() {
		ui.Row(c).Padding(10, 12).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(14, 14).Draw(func(p *ui.Painter, r ui.Rect) { drawMarker(p, r, keyMarker) })
			ui.Text(c, "重点问题").Bold()
			ui.Text(c, fmt.Sprint(len(col))).TextColor(t.TextMuted)
		})
		if len(col) == 0 {
			ui.Text(c, "节点打上红旗，或把卡片拖到这里").FontSize(12).TextColor(t.TextMuted).Margin(8, 12).Wrap()
			ui.Spacer(c)
			return
		}
		ui.List(c, a.boardList("key"), len(col)).Grow(1).ItemKey(func(i int) any { return col[i].Node.ID }).Rows(func(r ui.ListRow) {
			ui.Box(r.Context).Padding(0, 8, 8).Children(func() {
				a.taskCard(r.Context, col[r.Index], col[r.Index].Node.Task != nil)
			})
		})
	})
}

// taskCard shows a task: its title, where it is in the map, its tags and
// dates. A click selects it; a double click shows it in the map.
func (a *app) taskCard(c *ui.Context, tn model.TaskNode, showStatus bool) {
	t := c.Theme()
	n := tn.Node
	task := n.Task
	if task == nil { // a 重点问题 that is not a task
		task = &model.Task{}
	}
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
				if m.Item(s.Label()).Checked(n.Task != nil && task.Status == s).Chosen() {
					a.setStatus(n, s)
				}
			}
		})
		key := slices.Contains(n.Markers, keyMarker)
		label := "标为重点问题"
		if key {
			label = "移出重点问题"
		}
		if m.Item(label).Chosen() {
			a.setKeyIssue(n, !key)
		}
	})
	card.Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Start).Children(func() {
			if showStatus {
				statusIcon(c, task.Status, 13).Margin(3, 0, 0, 0)
			}
			title := ui.Text(c, n.Title).Grow(1).Wrap()
			if task.Status == model.StatusDone {
				title.Strikethrough().TextColor(t.TextMuted)
			}
			if task.Priority != model.PriorityNone {
				ui.Text(c, task.Priority.Label()).FontSize(11).TextColor(priorityColor(c, task.Priority)).NoWrap()
			}
		})
		if bl := a.file.Blockers(n); len(bl) > 0 && task.Status != model.StatusDone {
			more := ""
			if len(bl) > 1 {
				more = fmt.Sprintf(" 等 %d 项", len(bl))
			}
			ui.Text(c, "🔒 等待「"+bl[0].Title+"」"+more).FontSize(11).TextColor(ui.Hex("#d97706")).MaxLines(1)
		}
		where := tn.Parents
		if len(where) > 0 {
			where = where[1:] // the root says little
		}
		if len(a.file.Sheets) > 1 && tn.Sheet != nil {
			where = append([]string{tn.Sheet.Title}, where...)
		}
		if len(where) > 0 {
			ui.Text(c, strings.Join(where, " › ")).FontSize(11).TextColor(t.TextMuted).SingleLine()
		}
		if len(n.Tags) > 0 || task.Start != nil || task.Due != nil || task.Estimate > 0 || task.Iteration != "" {
			ui.Row(c).Wrap().Gap(6).AlignItems(ui.Center).Children(func() {
				for _, tag := range n.Tags {
					tagChip(c.Key("tag:"+tag), tag, tag, false, 11).PassThrough()
				}
				if task.Estimate > 0 {
					ui.Text(c, strconv.FormatFloat(task.Estimate, 'f', -1, 64)+"h").FontSize(11).TextColor(t.TextMuted).NoWrap()
				}
				if it := a.file.Iteration(task.Iteration); it != nil {
					ui.Text(c, it.Name).FontSize(11).TextColor(t.Accent).NoWrap()
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
	// The groups' headings and cards as the rows of a virtual list: only
	// those in view are built.
	type row struct {
		head  string
		count int
		tn    model.TaskNode
	}
	var rows []row
	for _, g := range groups {
		if len(g.tasks) > 0 {
			rows = append(rows, row{head: g.title, count: len(g.tasks)})
			for _, tn := range g.tasks {
				rows = append(rows, row{tn: tn})
			}
		}
	}
	if len(rows) > 0 {
		ui.List(c, nil, len(rows)).Grow(1).ItemKey(func(i int) any {
			if rows[i].head != "" {
				return "head:" + rows[i].head
			}
			return rows[i].tn.Node.ID
		}).Rows(func(r ui.ListRow) {
			c := r.Context
			it := rows[r.Index]
			ui.Box(c).Padding(0, 24).Children(func() {
				ui.Box(c).MaxWidth(760).Children(func() {
					if it.head != "" {
						head := ui.Text(c, fmt.Sprintf("%s  %d", it.head, it.count)).Bold()
						if it.head == "已逾期" {
							head.TextColor(t.Danger)
						}
						head.Margin(16, 0, 8)
						return
					}
					ui.Box(c).Padding(0, 0, 8).Children(func() { a.taskCard(c, it.tn, true) })
				})
			})
		})
		return
	}
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(16, 24).Gap(18).MaxWidth(760).Children(func() {
			any := false
			if !any {
				ui.Text(c, "没有未完成的任务。在节点的详情面板中勾选「作为任务」来添加。").TextColor(t.TextMuted).Wrap()
			}
		})
	})
}
