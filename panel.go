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
	if a.aiOpen && a.doc != nil && !a.libOpen {
		// The AI beside the inspector, in a column of its own, or below
		// it where the window has no room for two: it hides neither.
		if a.aiBeside(c) {
			ui.Column(c.Key("inspector")).Width(a.panelWidth).FillHeight().Background(t.Surface).Children(func() { a.inspector(c) })
			ui.Box(c).Width(1).FillHeight().Background(t.Border)
			ui.Column(c.Key("ai-column")).Width(a.panelWidth).FillHeight().Background(t.Surface).Children(func() { a.aiPanel(c) })
			return
		}
		ui.Column(c.Key("stacked")).Width(a.panelWidth).FillHeight().Background(t.Surface).Children(func() {
			ui.Column(c.Key("inspector")).Grow(1).Basis(0).MinHeight(0).Children(func() { a.inspector(c) })
			ui.Box(c).Height(1).FillWidth().Background(t.Border)
			ui.Column(c.Key("ai-column")).Grow(1.3).Basis(0).MinHeight(0).Children(func() { a.aiPanel(c) })
		})
		return
	}
	ui.Column(c).Width(a.panelWidth).FillHeight().Background(t.Surface).Children(func() {
		if a.aiOpen && (a.doc != nil || a.libOpen) {
			a.aiPanel(c) // scrolls its conversation itself
			return
		}
		a.inspector(c)
	})
}

// inspector is the panel's tabs: the node, its style and markers, the
// canvas, the presentation.
func (a *app) inspector(c *ui.Context) {
	t := c.Theme()
	func() {
		// The inspector's tabs sit in the title bar's band, which drags the
		// window, with a hairline under them.
		ui.Row(c.Key("panel-head")).Height(toolbarHeight).Padding(0, 12).AlignItems(ui.Center).Shrink(0).
			BorderWidth(0, 0, 1, 0).BorderColor(hairline(t)).DragWindow().Children(func() {
			// One page, the topic's: its style and markers open from the bar
			// over the canvas, the canvas's and the talk's from its corner,
			// each with the way back.
			if a.panelTab == tabNode || a.doc == nil {
				ui.Text(c, "主题").FontSize(textBody).FontWeight(600)
				return
			}
			back := ui.Row(c.Key("panel-back")).Height(28).Padding(0, 8, 0, 4).Gap(4).Radius(6).AlignItems(ui.Center).
				Cursor(ui.CursorPointer).Label("返回主题")
			if back.Hovered() {
				back.Background(t.SurfaceHover)
			}
			if back.Clicked() {
				a.panelTab = tabNode
			}
			back.Children(func() {
				glyph(c, iconBack, 14, t.TextMuted)
				ui.Text(c, "主题").FontSize(textSmall).TextColor(t.TextMuted).PassThrough()
			})
			ui.Text(c, []string{"主题", "样式", "标记", "画布", "演说"}[a.panelTab]).FontSize(textBody).FontWeight(600).Margin(0, 0, 0, 6)
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
				ui.Column(c).Padding(16, 16, 0).Gap(12).Children(func() {
					a.mapAnalysisSection(c)
					a.mapMaterialSection(c)
					a.tagList(c) // the map's tags, which filter its nodes
				})
				a.canvasPanel(c)
				ui.Column(c).Padding(0, 16, 16).Gap(12).Children(func() {
					ui.Divider(c)
					a.projectSection(c)
					a.githubSection(c)
				})
			}
		})
	}()
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
			if inlineInput(c, &title, textHeadline, true, "无标题", "标题").Changed() {
				a.mutate("title:"+n.ID, func() { n.Title = title })
			}

			// What it is, as rows of properties, then what is written of it
			// and what it links to, each part under a hairline.
			ui.Column(c.Key("props")).Gap(2).FillWidth().Children(func() {
				a.taskEditor(c, n)
				propRow(c, "标签", func() { a.tagPicker(c, n) })
			})

			// The note shows as it is written: Markdown typed formats as it
			// goes.
			sectionCard(c, iconPencil, tileYellow, "备注", func() { a.notePictureButton(c, "panel", n) }, func() {
				a.noteEditor(c, "panel", n, textBody, "实现要点、决策、注意事项…（# 标题、- 列表、**粗体** 输入即显示）").
					FillWidth().MinHeight(96).MaxHeight(420)
			})

			sectionCard(c, iconLink, tileBlue, "链接、附件与资料", nil, func() {
				a.linksSection(c, n)
				a.imageSection(c, n)
				a.filesSection(c, n)
				a.nodeRelatedSection(c, n)
				a.annotationsSection(c, n)
			})

			sectionCard(c, iconLevels, tileBlue, "度量与迭代", nil, func() {
				a.measuresSection(c, n)
				if n.Task != nil {
					a.planEditor(c, n)
				}
			})

			ui.Text(c, a.nodeStats(n)).FontSize(textCaption).TextColor(t.TextMuted)
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

// taskEditor is the node's rows of a task: its state, its dates and its
// priority; a node not a task has a button to make it one.
func (a *app) taskEditor(c *ui.Context, n *model.Node) {
	t := c.Theme()
	if n.Task == nil {
		propRow(c, "状态", func() {
			b := ui.Box(c.Key("make-task")).Padding(3, 8).Margin(0, 0, 0, -8).Radius(6).Cursor(ui.CursorPointer).
				Label("作为任务").Tooltip("设为任务后，可以安排状态、优先级和日期，并出现在看板和任务里")
			if b.Hovered() {
				b.Background(t.SurfaceHover)
			}
			if b.Clicked() {
				a.later(func() { a.toggleTask(n) })
			}
			b.Children(func() { ui.Text(c, "设为任务").FontSize(textBody).TextColor(t.TextMuted).PassThrough() })
		})
		return
	}
	task := n.Task
	status := 0
	for i, s := range model.Statuses {
		if s == task.Status {
			status = i
		}
	}
	propRow(c, "状态", func() {
		ui.MenuButton(c, model.Statuses[status].Label(), func(m *ui.Menu) {
			for _, s := range model.Statuses {
				if m.Item(s.Label()).Checked(s == task.Status).Chosen() {
					a.setStatus(n, s)
				}
			}
			m.Separator()
			if m.Item("不再作为任务").Chosen() {
				a.toggleTask(n)
			}
		}).Label("任务状态").FontSize(textSmall)
	})
	a.dateField(c, "截止", "截止日期", &task.Due)
	if task.Start != nil || task.Due != nil {
		a.dateField(c, "开始", "开始日期", &task.Start)
	}
	if task.Overdue(a.now()) {
		propRow(c, "", func() { ui.Text(c, "已逾期").FontSize(textSmall).TextColor(t.Danger) })
	}
	prio := task.Priority.Label()
	propRow(c, "优先级", func() {
		labels := make([]string, len(model.Priorities))
		for i, p := range model.Priorities {
			labels[i] = p.Label()
		}
		if ui.Select(c, &prio, labels).Label("优先级").FontSize(textSmall).Changed() {
			a.mutate("", func() { task.Priority = model.PriorityFromLabel(prio) })
		}
	})
}

// dateField is a row of an optional date: the date to change and to clear,
// or a button to set it.
func (a *app) dateField(c *ui.Context, name, label string, date **time.Time) {
	t := c.Theme()
	propRow(c, name, func() {
		if *date == nil {
			b := ui.Box(c.Key("set:"+label)).Padding(3, 8).Margin(0, 0, 0, -8).Radius(6).Cursor(ui.CursorPointer).Label(label)
			if b.Hovered() {
				b.Background(t.SurfaceHover)
			}
			if b.Clicked() {
				a.mutate("", func() {
					d := today(a.now())
					*date = &d
				})
			}
			b.Children(func() { ui.Text(c, "设日期").FontSize(textBody).TextColor(t.TextMuted).PassThrough() })
			return
		}
		d := **date
		if ui.DateInput(c, &d).Label(label).FontSize(textSmall).Changed() {
			a.mutate("", func() { *date = &d })
		}
		if iconButton(c, iconClose, "清除"+label, "清除"+label, 22, true).Clicked() {
			a.mutate("", func() { *date = nil })
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
