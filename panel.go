package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// Tabs of the panel.
const (
	tabNode = iota
	tabStyle
	tabMarkers
	tabCanvas
	tabPresent
)

// panel is the right side: the selected node's details, or the shown
// canvas's style.
// panelMin is the narrowest the panel is dragged to.
const panelMin = 260

func (a *app) panel(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Width(a.panelWidth).FillHeight().Background(t.Surface).Children(func() {
		if a.aiOpen && a.doc != nil {
			a.aiPanel(c) // scrolls its conversation itself
			return
		}
		ui.Box(c).Padding(10, 16, 4).Children(func() {
			ui.Segmented(c, &a.panelTab, "节点", "样式", "标记", "画布", "演说").FillWidth()
		})
		ui.Scroll(c.Key(fmt.Sprint("panel", a.panelTab))).Grow(1).Children(func() {
			switch {
			case a.doc != nil && a.panelTab == tabNode && len(a.multi) > 0:
				a.multiPanel(c)
			case a.doc == nil || a.panelTab == tabNode:
				a.nodePanel(c)
			case a.panelTab == tabStyle:
				a.nodeStylePanel(c)
			case a.panelTab == tabMarkers:
				a.markerPanel(c)
			case a.panelTab == tabPresent:
				a.presentPanel(c)
			default:
				a.canvasPanel(c)
				ui.Column(c).Padding(0, 16, 16).Gap(12).Children(func() {
					ui.Divider(c)
					a.projectSection(c)
				})
			}
		})
	})
}

// nodePanel shows and edits the selected node: its title, note, tags and
// task.
func (a *app) nodePanel(c *ui.Context) {
	t := c.Theme()
	func() {
		n, _ := a.selectedNode()
		if n == nil {
			ui.Column(c).Padding(20).Children(func() {
				ui.Text(c, "选择一个节点查看详情").TextColor(t.TextMuted)
			})
			return
		}
		ui.Column(c.Key(n.ID)).Padding(16).Gap(14).Children(func() {
			a.breadcrumbs(c, n)

			title := n.Title
			ui.Field(c, "标题", func() {
				if ui.TextInput(c, &title).Changed() {
					a.mutate("title:"+n.ID, func() { n.Title = title })
				}
			})

			note := n.Note
			ui.Column(c).Gap(6).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "备注").FontSize(12).Bold().TextColor(t.TextMuted).Grow(1)
					mode := 0
					if a.notePreview && n.Note != "" {
						mode = 1
					}
					if ui.Segmented(c, &mode, "编辑", "预览").FontSize(11).Changed() {
						a.notePreview = mode == 1
					}
				})
				if a.notePreview && n.Note != "" {
					// The note as Markdown; a double click edits it.
					box := ui.Box(c.Key("note-preview")).Padding(8, 10).Radius(6).Border(1, t.Border).FillWidth().
						Tooltip("双击编辑")
					if box.DoubleClicked() {
						a.notePreview = false
					}
					box.Children(func() { markdownView(c, n.Note, 13) })
					return
				}
				if ui.TextArea(c, &note).Placeholder("实现要点、决策、注意事项…（支持 Markdown）").Lines(5, 16).Label("备注内容").Changed() {
					a.mutate("note:"+n.ID, func() { n.Note = note })
				}
			})

			a.imageSection(c, n)
			a.linksSection(c, n)

			ui.Field(c, "标签", func() { a.tagPicker(c, n) })

			ui.Divider(c)
			a.taskEditor(c, n)

			ui.Divider(c)
			ui.Text(c, a.nodeStats(n)).FontSize(12).TextColor(t.TextMuted)
			if n != a.doc.Root {
				if ui.Button(c, "删除节点").TextColor(t.Danger).Clicked() {
					a.selected = n.ID
					a.deleteSelected()
				}
			}
		})
	}()
}

func cleanTags(tags []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, tag := range tags {
		tag = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(tag), "#"))
		if tag != "" && !seen[tag] {
			seen[tag] = true
			out = append(out, tag)
		}
	}
	return out
}

// breadcrumbs shows the titles above the node, each selecting its node.
func (a *app) breadcrumbs(c *ui.Context, n *model.Node) {
	path := a.doc.Path(n.ID)
	if len(path) < 2 {
		return
	}
	t := c.Theme()
	ui.Row(c).Wrap().Gap(4).Children(func() {
		for i, p := range path[:len(path)-1] {
			if i > 0 {
				ui.Text(c, "›").FontSize(12).TextColor(t.TextMuted)
			}
			if ui.Link(c, p.Title, "").FontSize(12).Clicked() {
				a.selectNode(p.ID)
			}
		}
	})
}

func (a *app) taskEditor(c *ui.Context, n *model.Node) {
	t := c.Theme()
	isTask := n.Task != nil
	if ui.Checkbox(c, &isTask, "作为任务").Changed() {
		a.toggleTask(n)
	}
	if n.Task == nil {
		ui.Text(c, "设为任务后，可以安排状态、优先级和日期，并出现在看板和日程中。").FontSize(12).TextColor(t.TextMuted).Wrap()
		return
	}
	task := n.Task

	status := 0
	for i, s := range model.Statuses {
		if s == task.Status {
			status = i
		}
	}
	ui.Field(c, "状态", func() {
		labels := make([]string, len(model.Statuses))
		for i, s := range model.Statuses {
			labels[i] = s.Label()
		}
		if ui.Segmented(c, &status, labels...).Changed() {
			a.setStatus(n, model.Statuses[status])
		}
	})

	prio := task.Priority.Label()
	ui.Field(c, "优先级", func() {
		labels := make([]string, len(model.Priorities))
		for i, p := range model.Priorities {
			labels[i] = p.Label()
		}
		if ui.Select(c, &prio, labels).Label("优先级").Changed() {
			a.mutate("", func() { task.Priority = model.PriorityFromLabel(prio) })
		}
	})

	a.dateField(c, "开始日期", &task.Start)
	a.dateField(c, "截止日期", &task.Due)
	if task.Overdue(a.now()) {
		ui.Text(c, "⚠ 已逾期").FontSize(12).TextColor(t.Danger)
	}
	a.planEditor(c, n)
}

// dateField edits an optional date: a switch sets or clears it.
func (a *app) dateField(c *ui.Context, label string, date **time.Time) {
	ui.Row(c.Key(label)).Gap(8).AlignItems(ui.Center).Children(func() {
		has := *date != nil
		if ui.Checkbox(c, &has, label).Changed() {
			a.mutate("", func() {
				if has {
					d := today(a.now())
					*date = &d
				} else {
					*date = nil
				}
			})
		}
		ui.Spacer(c)
		if *date != nil {
			d := **date
			if ui.DateInput(c, &d).Label(label).Changed() {
				a.mutate("", func() { *date = &d })
			}
		}
	})
}

func today(now time.Time) time.Time {
	y, m, d := now.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, now.Location())
}

// nodeStats counts what is below a node.
func (a *app) nodeStats(n *model.Node) string {
	var nodes, tasks, done int
	var walk func(n *model.Node)
	walk = func(n *model.Node) {
		for _, c := range n.Children {
			nodes++
			if c.Task != nil {
				tasks++
				if c.Task.Status == model.StatusDone {
					done++
				}
			}
			walk(c)
		}
	}
	walk(n)
	s := fmt.Sprintf("下级节点 %d 个", nodes)
	if tasks > 0 {
		s += fmt.Sprintf(" · 任务 %d/%d 完成", done, tasks)
	}
	return s
}
