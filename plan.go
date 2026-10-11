package main

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/layout"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// Planning tasks: what they wait on, how long they take, which iteration
// they are in, and reminders as they come due.

// planEditor is the planning part of a task's editor: its estimate,
// iteration and the tasks before it.
func (a *app) planEditor(c *ui.Context, n *model.Node) {
	t := c.Theme()
	task := n.Task
	est := task.Estimate
	ui.Field(c, "估时（小时）", func() {
		if ui.NumberInput(c, &est, 0, 10000, 0.5).Label("估时").Changed() {
			a.mutate("estimate:"+n.ID, func() { task.Estimate = est })
		}
	})

	ui.Field(c, "迭代", func() {
		if len(a.file.Iterations) == 0 {
			ui.Text(c, "在「总览」里新建迭代（比如「第 3 周」）后，可以把任务排进去。").FontSize(12).TextColor(t.TextMuted).Wrap()
			return
		}
		options := []string{"未安排"}
		cur := "未安排"
		for _, it := range a.file.Iterations {
			options = append(options, it.Name)
			if it.ID == task.Iteration {
				cur = it.Name
			}
		}
		if ui.Select(c, &cur, options).Label("迭代").Changed() {
			a.mutate("", func() {
				task.Iteration = ""
				for _, it := range a.file.Iterations {
					if it.Name == cur {
						task.Iteration = it.ID
					}
				}
			})
		}
	})

	ui.Field(c, "前置任务（完成后才能开始）", func() {
		ui.Column(c).Gap(4).Children(func() {
			for _, id := range task.After {
				b := a.file.Node(id)
				if b == nil {
					continue
				}
				ui.Row(c.Key("after:" + id)).Gap(6).AlignItems(ui.Center).Children(func() {
					col := t.TextMuted
					if b.Task != nil {
						col = statusColor(b.Task.Status)
					}
					ui.Box(c).Size(8, 8).Radius(999).Background(col).Shrink(0)
					go_ := ui.Row(c).Grow(1).Basis(0).MinWidth(0).Cursor(ui.CursorPointer).Label("前往 " + b.Title)
					if go_.Clicked() {
						id := id
						a.later(func() { a.selectNode(id) })
					}
					go_.Children(func() { ui.Text(c, b.Title).FontSize(12).TextColor(t.Accent).MaxLines(1) })
					if iconButton(c, iconClose, "移除前置任务", "移除前置任务", 22, true).Clicked() {
						id := id
						a.later(func() { a.removeAfter(n, id) })
					}
				})
			}
			if ui.Button(c, "添加前置任务…").FontSize(12).Clicked() {
				a.linkFrom = n.ID
				a.openPalette(paletteAfter)
			}
			if bl := a.file.Blockers(n); len(bl) > 0 && task.Status != model.StatusDone {
				var names []string
				for _, b := range bl {
					names = append(names, "「"+b.Title+"」")
				}
				ui.Text(c, "🔒 等待 "+strings.Join(names, "、")+" 完成").FontSize(12).TextColor(ui.Hex("#d97706")).Wrap()
			}
		})
	})
}

// blocked reports whether a task waits on tasks not done.
func (a *app) blocked(n *model.Node) bool {
	return n.Task != nil && n.Task.Status != model.StatusDone && len(a.file.Blockers(n)) > 0
}

// addAfter makes a task wait on another, unless that would make a cycle.
func (a *app) addAfter(n *model.Node, id string) {
	if n.ID == id || n.Task == nil || slices.Contains(n.Task.After, id) {
		return
	}
	if a.waitsOn(id, n.ID, map[string]bool{}) {
		a.saveErr = "不能添加：会形成循环依赖"
		return
	}
	a.mutate("", func() {
		n.Task.After = append(slices.Clone(n.Task.After), id)
		if b := a.file.Node(id); b != nil && b.Task == nil {
			b.Task = &model.Task{Status: model.StatusTodo} // what is waited on is a task
		}
	})
}

// waitsOn reports whether the task with the ID from waits, directly or not,
// on the one with the ID to.
func (a *app) waitsOn(from, to string, seen map[string]bool) bool {
	if from == to {
		return true
	}
	if seen[from] {
		return false
	}
	seen[from] = true
	n := a.file.Node(from)
	if n == nil || n.Task == nil {
		return false
	}
	for _, id := range n.Task.After {
		if a.waitsOn(id, to, seen) {
			return true
		}
	}
	return false
}

func (a *app) removeAfter(n *model.Node, id string) {
	a.mutate("", func() {
		n.Task.After = slices.DeleteFunc(slices.Clone(n.Task.After), func(x string) bool { return x == id })
		if len(n.Task.After) == 0 {
			n.Task.After = nil
		}
	})
}

// drawDependencies draws an arrow from each task shown to the tasks shown
// that wait on it, over the canvas's edges.
func (a *app) drawDependencies(p *ui.Painter, r layout.Result, ox, oy, z float32) {
	a.shownDoc().Walk(func(n, _ *model.Node, _ int) bool {
		if n.Task == nil || len(n.Task.After) == 0 {
			return true
		}
		to, ok := r.Boxes[n.ID]
		if !ok {
			return true
		}
		for _, id := range n.Task.After {
			from, ok := r.Boxes[id]
			if !ok {
				continue
			}
			col := ui.Hex("#d97706")
			if b := a.file.Node(id); b != nil && b.Task != nil && b.Task.Status == model.StatusDone {
				col = ui.Hex("#9ca3af")
			}
			x0, y0 := ox+(from.X+from.W/2)*z, oy+(from.Y+from.H)*z
			x1, y1 := ox+(to.X+to.W/2)*z, oy+to.Y*z
			if from.Y > to.Y { // below: from its top to the other's bottom
				y0, y1 = oy+from.Y*z, oy+(to.Y+to.H)*z
			}
			mid := (y0 + y1) / 2
			path := new(ui.Path).MoveTo(x0, y0).CubeTo(x0, mid, x1, mid, x1, y1)
			p.StrokePath(path, 1.5*z, col.Alpha(0.8))
			// The arrow's head: the curve ends going straight down, or up.
			ang := math.Pi / 2
			if y1 < y0 {
				ang = -math.Pi / 2
			}
			s := float64(6 * z)
			head := new(ui.Path).MoveTo(x1, y1).
				LineTo(x1-float32(s*math.Cos(ang-0.45)), y1-float32(s*math.Sin(ang-0.45))).
				LineTo(x1-float32(s*math.Cos(ang+0.45)), y1-float32(s*math.Sin(ang+0.45))).Close()
			p.FillPath(head, col)
		}
		return true
	})
}

// iterationFilter keeps the tasks of the iteration the board and timeline
// show: all, those not planned ("-"), or one.
func (a *app) inIteration(n *model.Node) bool {
	switch a.iterFilter {
	case "":
		return true
	case "-":
		return n.Task.Iteration == ""
	}
	return n.Task.Iteration == a.iterFilter
}

// iterationPicker chooses the iteration the board and timeline show.
func (a *app) iterationPicker(c *ui.Context) {
	if a.file == nil || len(a.file.Iterations) == 0 {
		return
	}
	options := []string{"全部迭代", "未安排"}
	cur := "全部迭代"
	if a.iterFilter == "-" {
		cur = "未安排"
	}
	for _, it := range a.file.Iterations {
		options = append(options, it.Name)
		if it.ID == a.iterFilter {
			cur = it.Name
		}
	}
	if ui.Select(c, &cur, options).Label("显示迭代").Changed() {
		switch cur {
		case "全部迭代":
			a.iterFilter = ""
		case "未安排":
			a.iterFilter = "-"
		default:
			for _, it := range a.file.Iterations {
				if it.Name == cur {
					a.iterFilter = it.ID
				}
			}
		}
	}
}

// iterationsEditor lists the iterations of the map, to add, rename,
// date and delete.
func (a *app) iterationsEditor(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Gap(8).Children(func() {
		for i := range a.file.Iterations {
			it := &a.file.Iterations[i]
			ui.Row(c.Key("it:" + it.ID)).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
				name := it.Name
				if ui.TextInput(c, &name).Width(140).Label("迭代名称").Changed() {
					a.mutate("itname:"+it.ID, func() { it.Name = name })
				}
				a.iterDate(c.Key("s"), it, &it.Start, "开始")
				ui.Text(c, "–").TextColor(t.TextMuted)
				a.iterDate(c.Key("e"), it, &it.End, "结束")
				done, total, hours := a.iterationProgress(it.ID)
				ui.Text(c, fmt.Sprintf("%d/%d 完成 · 剩余 %s 小时", done, total, hours)).FontSize(12).TextColor(t.TextMuted)
				if ui.Button(c, "删除").FontSize(12).TextColor(t.Danger).Clicked() {
					id := it.ID
					a.later(func() { a.deleteIteration(id) })
				}
			})
		}
		if ui.Button(c, "＋ 新建迭代").Clicked() {
			a.later(a.addIteration)
		}
	})
}

func (a *app) iterDate(c *ui.Context, it *model.Iteration, date **time.Time, label string) {
	if *date == nil {
		if ui.Button(c, label+"日期").FontSize(12).Clicked() {
			a.mutate("", func() { d := today(a.now()); *date = &d })
		}
		return
	}
	d := **date
	if ui.DateInput(c, &d).Label(it.Name + label).Changed() {
		a.mutate("", func() { *date = &d })
	}
}

// addIteration adds an iteration after the last, of two weeks.
func (a *app) addIteration() {
	start := today(a.now())
	if k := len(a.file.Iterations); k > 0 && a.file.Iterations[k-1].End != nil {
		start = a.file.Iterations[k-1].End.AddDate(0, 0, 1)
	}
	end := start.AddDate(0, 0, 13)
	name := "迭代 " + strconv.Itoa(len(a.file.Iterations)+1)
	a.mutate("", func() {
		a.file.Iterations = append(a.file.Iterations, model.Iteration{ID: model.NewID(), Name: name, Start: &start, End: &end})
	})
}

func (a *app) deleteIteration(id string) {
	a.mutate("", func() {
		a.file.Iterations = slices.DeleteFunc(a.file.Iterations, func(it model.Iteration) bool { return it.ID == id })
		a.file.Walk(func(n *model.Node) {
			if n.Task != nil && n.Task.Iteration == id {
				n.Task.Iteration = ""
			}
		})
	})
	if a.iterFilter == id {
		a.iterFilter = ""
	}
}

// iterationProgress counts an iteration's tasks and the hours left.
func (a *app) iterationProgress(id string) (done, total int, hoursLeft string) {
	var left float64
	for _, tn := range a.file.Tasks() {
		task := tn.Node.Task
		if task.Iteration != id {
			continue
		}
		total++
		if task.Status == model.StatusDone {
			done++
		} else {
			left += task.Estimate
		}
	}
	return done, total, strconv.FormatFloat(left, 'f', -1, 64)
}

// Reminders: once a day, a notification for each task of the open map
// due today or overdue, not done.

// reminderCheck shows the reminders not shown today, at most once a minute.
func (a *app) reminderCheck() {
	if a.file == nil || a.cfg.NoReminders || a.now().Sub(a.remindedAt) < time.Minute {
		return
	}
	a.remindedAt = a.now()
	day := today(a.now())
	if a.reminded == nil || !a.remindedDay.Equal(day) {
		a.reminded, a.remindedDay = map[string]bool{}, day
	}
	var due, overdue []string
	for _, tn := range a.file.Tasks() {
		task := tn.Node.Task
		if task.Status == model.StatusDone || task.Due == nil || a.reminded[tn.Node.ID] {
			continue
		}
		switch {
		case task.Due.Before(day):
			overdue = append(overdue, tn.Node.Title)
		case task.Due.Before(day.AddDate(0, 0, 1)):
			due = append(due, tn.Node.Title)
		default:
			continue
		}
		a.reminded[tn.Node.ID] = true
	}
	if len(due)+len(overdue) == 0 {
		return
	}
	var body []string
	if len(overdue) > 0 {
		body = append(body, "已逾期："+strings.Join(overdue, "、"))
	}
	if len(due) > 0 {
		body = append(body, "今天到期："+strings.Join(due, "、"))
	}
	a.plat.Notify("MindMap · "+mapName(a.path), strings.Join(body, "\n"))
}
