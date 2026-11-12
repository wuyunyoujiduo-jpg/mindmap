package main

import (
	"fmt"
	"slices"
	"sort"
	"strings"

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
			if slices.Contains(n.Markers, keyMarker) && a.highlighted(n) {
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
	tasks := a.visibleTasks()
	sortTasks(tasks)
	ui.Column(c).Grow(1).Children(func() {
		a.boardColumns(c, tasks)
	})
}

func (a *app) boardColumns(c *ui.Context, tasks []model.TaskNode) {
	t := c.Theme()
	// Columns on the page itself, not in gray boxes: a heading over a
	// hairline, then the cards; a column a task is dragged over tints.
	ui.Row(c).Grow(1).Padding(14, 16).Gap(16).AlignItems(ui.Stretch).Children(func() {
		a.keyIssueColumn(c)
		for _, s := range model.Statuses {
			var col []model.TaskNode
			for _, tn := range tasks {
				if tn.Node.Task.Status == s {
					col = append(col, tn)
				}
			}
			// A hollow in the page, faintly of its state's color.
			column := ui.Column(c.Key(string(s))).Grow(1).Basis(0).Radius(radiusCard).Padding(0, 6).Background(blockFill(t))
			if _, over := ui.DragOver[nodeDrag](column); over {
				column.Background(statusColor(s).Alpha(0.08))
			}
			if d, ok := ui.Drop[nodeDrag](column); ok {
				if sheet := a.file.SheetOf(d.ID); sheet != nil {
					if n, _ := sheet.Find(d.ID); n != nil && (n.Task == nil || n.Task.Status != s) { // a 重点问题 not a task becomes one
						a.setStatus(n, s)
					}
				}
			}
			column.Children(func() {
				boardHead(c, func() { statusIcon(c, s, 14) }, s.Label(), len(col))
				if len(col) == 0 {
					ui.Text(c, "拖动卡片到这里").FontSize(12).TextColor(t.TextMuted).Margin(8, 4)
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
					ui.Box(r.Context).Padding(0, 0, 8).Children(func() {
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
// view.
func (a *app) showOnBoard(id string) {
	n, _ := a.doc.Find(id)
	if n == nil || n.Task == nil {
		return
	}
	a.selectNode(id)
	a.mode = viewBoard
	a.boardReveal = id
}

// keyIssueColumn is the board's column of 重点问题: the nodes marked with
// the red flag, which stay in their status's column too. A card dropped
// on it is marked.
// boardHead is a column's heading over a hairline: its mark, its name,
// how many.
func boardHead(c *ui.Context, mark func(), name string, n int) {
	t := c.Theme()
	ui.Row(c).Padding(12, 6, 10).Gap(8).AlignItems(ui.Center).Children(func() {
		mark()
		ui.Text(c, name).FontSize(textBody).FontWeight(700)
		ui.Text(c, fmt.Sprint(n)).FontSize(textCaption).TextColor(faint(t)).FontFeatures("tnum")
	})
}

func (a *app) keyIssueColumn(c *ui.Context) {
	t := c.Theme()
	col := a.keyIssues()
	red := ui.Hex("#D9605B")
	column := ui.Column(c.Key("key-issues")).Grow(1).Basis(0).Radius(radiusCard).Background(t.Text.Alpha(0.04)).Padding(0, 6)
	if _, over := ui.DragOver[nodeDrag](column); over {
		column.Background(red.Alpha(0.08))
	}
	if d, ok := ui.Drop[nodeDrag](column); ok {
		if sheet := a.file.SheetOf(d.ID); sheet != nil {
			if n, _ := sheet.Find(d.ID); n != nil {
				a.setKeyIssue(n, true)
			}
		}
	}
	column.Children(func() {
		boardHead(c, func() {
			ui.Box(c).Size(14, 14).Draw(func(p *ui.Painter, r ui.Rect) { drawMarker(p, r, keyMarker) })
		}, "重点问题", len(col))
		if len(col) == 0 {
			ui.Text(c, "节点打上红旗，或把卡片拖到这里").FontSize(12).TextColor(t.TextMuted).Margin(8, 4).Wrap()
			ui.Spacer(c)
			return
		}
		ui.List(c, a.boardList("key"), len(col)).Grow(1).ItemKey(func(i int) any { return col[i].Node.ID }).Rows(func(r ui.ListRow) {
			ui.Box(r.Context).Padding(0, 0, 8).Children(func() {
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
	card := paper(ui.Column(c).Padding(11, 13).Gap(6).Radius(radiusRow+2), t).
		Drag(nodeDrag{n.ID}).Cursor(ui.CursorPointer).Label(n.Title)
	if n.ID == a.selected {
		chosenCard(card, t)
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
			title := ui.Text(c, n.Title).FontSize(textBody).FontWeight(700).LineHeight(1.4).Grow(1).Wrap()
			if task.Status == model.StatusDone {
				title.Strikethrough().TextColor(t.TextMuted)
			}
			if task.Priority != model.PriorityNone { // on a wash of its color
				pc := priorityColor(c, task.Priority)
				ui.Box(c).Padding(1, 7).Radius(radiusPill).Background(pc.Alpha(0.13)).Shrink(0).Children(func() {
					ui.Text(c, task.Priority.Label()).FontSize(11).FontWeight(700).TextColor(pc.MixOklab(t.Text, 0.25)).NoWrap()
				})
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
			ui.Text(c, strings.Join(where, " › ")).FontSize(11).TextColor(t.TextMuted).SingleLine()
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
