package main

import (
	"fmt"
	"slices"
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
			ui.Column(c.Key("inspector")).Width(a.panelWidth).FillHeight().Background(t.Background).Children(func() { a.inspector(c) })
			ui.Box(c).Width(1).FillHeight().Background(t.Border)
			ui.Column(c.Key("ai-column")).Width(a.panelWidth).FillHeight().Background(t.Background).Children(func() { a.aiPanel(c) })
			return
		}
		ui.Column(c.Key("stacked")).Width(a.panelWidth).FillHeight().Background(t.Background).Children(func() {
			ui.Column(c.Key("inspector")).Grow(1).Basis(0).MinHeight(0).Children(func() { a.inspector(c) })
			ui.Box(c).Height(1).FillWidth().Background(t.Border)
			ui.Column(c.Key("ai-column")).Grow(1.3).Basis(0).MinHeight(0).Children(func() { a.aiPanel(c) })
		})
		return
	}
	ui.Column(c).Width(a.panelWidth).FillHeight().Background(t.Background).Children(func() {
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
		head := ui.Row(c.Key("panel-head")).Height(toolbarHeight).Padding(0, 12).AlignItems(ui.Center).Shrink(0).DragWindow()
		if a.panelTab == tabNode || a.doc == nil {
			head.Height(20) // a strip to drag the window by, over the topic's page
		} else {
			head.BorderWidth(0, 0, 1, 0).BorderColor(hairline(t))
		}
		head.Children(func() {
			// One page, the topic's: its style and markers open from the bar
			// over the canvas, the canvas's and the talk's from its corner,
			// each with the way back.
			if a.panelTab == tabNode || a.doc == nil {
				return // the topic's page needs no heading: its title is one
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
			if inlineInput(c, &title, textHeadline, true, "无标题", "标题").Margin(0, -6).Changed() {
				a.mutate("title:"+n.ID, func() { n.Title = title })
			}

			// Only what the topic has shows: its task's rows, its tags, its
			// note, what it links to. The rest is a row of quiet buttons
			// that add it, so that a plain topic has a plain page.
			shown := func(kind string, has bool) bool { return has || a.panelShow[n.ID+":"+kind] }
			show := func(kind string) {
				if a.panelShow == nil || len(a.panelShow) > 64 {
					a.panelShow = map[string]bool{}
				}
				a.panelShow[n.ID+":"+kind] = true
			}
			tags := shown("tags", len(n.Tags) > 0)
			links := shown("links", len(n.Links) > 0 || len(a.backlinks(n.ID)) > 0)
			image := shown("image", n.Image != nil)
			files := shown("files", len(n.Files) > 0)

			if n.Task != nil || tags {
				ui.Column(c.Key("props")).Gap(8).FillWidth().Children(func() {
					if n.Task != nil {
						a.taskEditor(c, n)
					}
					if tags {
						propRow(c, "标签", func() {
							if a.panelShow[n.ID+":tags"] {
								a.tagPicker(c, n) // all the map's tags, and a new one
								return
							}
							// Its own tags, a click taking one off, and a way to the rest.
							for _, tag := range n.Tags {
								if tagChip(c.Key("own:"+tag), tag, tag, true, 12).Label("标签 " + tag).Clicked() {
									a.toggleTag(n, tag)
								}
							}
							if addButton(c, "＋", "加标签").Label("＋ 标签").Clicked() {
								show("tags")
							}
						})
					}
				})
			}

			// The note shows as it is written: Markdown typed formats as it
			// goes.
			sectionCard(c, iconPencil, tileYellow, "备注", func() { a.notePictureButton(c, "panel", n) }, func() {
				a.noteEditor(c, "panel", n, textBody, "实现要点、决策、注意事项…（# 标题、- 列表、**粗体** 输入即显示）").
					FillWidth().MinHeight(72).MaxHeight(420)
			})

			clips, nodes := a.relatedOfCached(n)
			rels := slices.ContainsFunc(a.doc.Relations, func(r model.Relation) bool { return r.From == n.ID || r.To == n.ID })
			if links || image || files || len(clips)+len(nodes) > 0 || n.Summary != nil || n.Boundary != nil || rels {
				sectionCard(c, iconLink, tileBlue, "链接、附件与资料", nil, func() {
					if links {
						a.linksSection(c, n)
					}
					if image {
						a.imageSection(c, n)
					}
					if files {
						a.filesSection(c, n)
					}
					a.nodeRelatedSection(c, n)
					a.annotationsSection(c, n)
				})
			}

			// What can be added, each a quiet button, gone once it is there.
			ui.Row(c.Key("panel-add")).Wrap().GapY(2).Margin(0, -8).AlignItems(ui.Center).Children(func() {
				add := func(text, tip, kind string) {
					if addButton(c, text, tip).Clicked() {
						show(kind)
					}
				}
				if n.Task == nil && addButton(c, "＋ 任务", "设为任务：安排状态、日期和优先级，出现在看板和任务里").Label("作为任务").Clicked() {
					a.later(func() { a.toggleTask(n) })
				}
				if !tags {
					add("＋ 标签", "给这个主题打标签", "tags")
				}
				if !links {
					add("＋ 链接", "链接到网址、文件、其他主题或资料", "links")
				}
				if !image {
					add("＋ 图片", "给主题配一张图片（也可以选中后 ⌘V 粘贴）", "image")
				}
				if !files {
					add("＋ 附件", "把文件附在主题上（也可以拖到主题上）", "files")
				}
				if n.Task != nil && n.Task.Start == nil && !a.panelShow[n.ID+":plan"] {
					add("＋ 开始日期", "这个任务从哪天开始：时间线上显示成一段", "plan")
				}
				if n.Task != nil && addButton(c, "不再作为任务", "去掉它的状态、日期和优先级，留下主题").Clicked() {
					a.later(func() { a.toggleTask(n) })
				}
			})

			ui.Text(c, a.nodeStats(n)).FontSize(textCaption).TextColor(t.TextMuted)
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
		return
	}
	task := n.Task
	status := 0
	for i, s := range model.Statuses {
		if s == task.Status {
			status = i
		}
	}
	names := make([]string, len(model.Statuses))
	for i, st := range model.Statuses {
		names[i] = st.Label()
	}
	propRow(c, "状态", func() {
		if st := segmented(c, "task-status", "状态", names, status); st != status {
			a.setStatus(n, model.Statuses[st])
		}
	})
	propRow(c, "优先级", func() {
		labels := make([]string, len(model.Priorities))
		prio := 0
		for i, p := range model.Priorities {
			labels[i] = p.Label()
			if p == task.Priority {
				prio = i
			}
		}
		if p := segmented(c, "task-prio", "优先级", labels, prio); p != prio {
			a.mutate("", func() { task.Priority = model.Priorities[p] })
		}
	})
	dateProp(c, "截止", "截止日期", task.Due, a.now(), func(d *time.Time) { a.mutate("", func() { task.Due = d }) })
	if task.Start != nil || a.panelShow[n.ID+":plan"] {
		dateProp(c, "开始", "开始日期", task.Start, a.now(), func(d *time.Time) { a.mutate("", func() { task.Start = d }) })
	}
	if task.Overdue(a.now()) {
		propRow(c, "", func() { ui.Text(c, "已逾期").FontSize(textSmall).TextColor(t.Danger) })
	}
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
