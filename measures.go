package main

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// Measures, as MindMup's: numbers the map's nodes carry, as hours, cost or
// story points, whose totals add up the tree. They are set in the table
// view, a sheet of the map, and in the node's panel; the map shows the
// totals by the nodes.

// measureSuggestions are offered when a map has none.
var measureSuggestions = []string{"工时", "成本", "故事点"}

// measuresText is a node's totals, as the map shows them: "工时 12 · 成本 300".
func (a *app) measuresText(n *model.Node) string {
	if a.file == nil || len(a.file.Measures) == 0 || a.cfg.HideMeasures {
		return ""
	}
	var parts []string
	for _, m := range a.file.Measures {
		if n.Measured(m) {
			parts = append(parts, m+" "+model.FormatValue(n.Total(m)))
		}
	}
	return strings.Join(parts, " · ")
}

// addMeasure adds a measure to the map.
func (a *app) addMeasure(name string) {
	name = strings.TrimSpace(name)
	if a.file == nil || name == "" {
		return
	}
	a.mutate("", func() { a.file.AddMeasure(name) })
}

// valueCell is a node's own value of a measure, edited in place: a number,
// or empty for none. What is typed is kept as it is while being typed.
func (a *app) valueCell(c *ui.Context, n *model.Node, m string, width float32) {
	t := c.Theme()
	key := n.ID + "\x00" + m
	text := ""
	if v, ok := n.Values[m]; ok {
		text = model.FormatValue(v)
	}
	if a.cellKey == key {
		text = a.cellDraft
	}
	in := ui.TextInputBase(c.Key("v:"+key), &text).Width(width).Padding(3, 6).Radius(5).FontSize(textBody).
		Placeholder("—").Label(m + " " + n.Title)
	switch {
	case in.Focused():
		in.Background(t.Background).Border(1, t.Accent.Alpha(0.6))
	case in.Hovered():
		in.Background(t.Text.Alpha(0.05))
	}
	if in.Focused() {
		a.cellKey = key
		if in.Changed() {
			a.cellDraft = text
			s := strings.TrimSpace(text)
			if s == "" {
				a.mutate("value:"+key, func() { n.SetValue(m, 0, false) })
			} else if v, err := strconv.ParseFloat(s, 64); err == nil {
				a.mutate("value:"+key, func() { n.SetValue(m, v, true) })
			}
		} else if a.cellDraft == "" && text != "" {
			a.cellDraft = text
		}
	} else if a.cellKey == key {
		a.cellKey, a.cellDraft = "", ""
	}
}

// measureMenu renames or removes a measure.
func (a *app) measureMenu(m *ui.Menu, name string) {
	if m.Item("重命名…").Chosen() {
		a.renamingMeasure, a.measureDraft = name, name
	}
	if m.Item("删除这个度量").Chosen() {
		a.later(func() { a.mutate("", func() { a.file.RemoveMeasure(name) }) })
	}
}

// measureHeader is a column's name, renamed in place when asked.
func (a *app) measureHeader(c *ui.Context, name string, width float32) {
	t := c.Theme()
	if a.renamingMeasure == name {
		in := ui.TextInputBase(c.Key("rename-measure"), &a.measureDraft).Width(width).FontSize(textSmall).Bold().
			AutoFocus().Label("度量名称")
		if in.Submitted() || in.PressedOutside() {
			from, to := name, a.measureDraft
			a.renamingMeasure = ""
			a.later(func() { a.mutate("", func() { a.file.RenameMeasure(from, to) }) })
		}
		if in.Shortcut(0, ui.KeyEscape) {
			a.renamingMeasure = ""
		}
		return
	}
	h := ui.Row(c.Key("mh:"+name)).Width(width).Padding(0, 6).Gap(4).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Label("度量 " + name).Tooltip("右键重命名或删除")
	h.ContextMenu(func(m *ui.Menu) { a.measureMenu(m, name) })
	h.Menu(func(m *ui.Menu) { a.measureMenu(m, name) })
	h.Children(func() {
		ui.Text(c, name).FontSize(textSmall).Bold().TextColor(t.TextMuted).NoWrap().Grow(1).PassThrough()
	})
}

// newMeasureField adds a measure by the name typed.
func (a *app) newMeasureField(c *ui.Context, width float32) {
	t := c.Theme()
	in := ui.TextInputBase(c.Key("new-measure"), &a.newMeasure).Width(width).Padding(3, 6).Radius(5).
		FontSize(textSmall).Placeholder("+ 度量").Label("新度量")
	if in.Focused() {
		in.Border(1, t.Accent.Alpha(0.6))
	}
	if in.Submitted() {
		name := a.newMeasure
		a.newMeasure = ""
		a.later(func() { a.addMeasure(name) })
	}
}

const (
	tableCol  = 96 // a measure's column
	tableRowH = 34
)

// tableView is the map as a sheet: its nodes down the side, indented as
// in the tree, the measures across, each node's own value to edit and,
// for the nodes with others below, the total under it.
func (a *app) tableView(c *ui.Context) {
	t := c.Theme()
	d := a.doc
	measures := a.file.Measures
	ui.Column(c.Key("table")).Grow(1).Background(t.Background).Children(func() {
		if len(measures) == 0 {
			ui.Column(c).Grow(1).Center().Gap(12).Padding(40).Children(func() {
				ui.Text(c, "给节点记上数字，自动汇总到上级").FontSize(textTitle).Bold()
				ui.Text(c, "比如每个功能的工时、成本或故事点：上级节点自动加总，一眼看出每个模块还剩多少工作量。").
					FontSize(textBody).TextColor(t.TextMuted).Wrap().MaxWidth(420)
				ui.Row(c).Gap(8).Children(func() {
					for _, s := range measureSuggestions {
						if ui.Button(c, "+ "+s).Label("添加度量 "+s).Clicked() {
							name := s
							a.later(func() { a.addMeasure(name) })
						}
					}
					a.newMeasureField(c, 120)
				})
			})
			return
		}
		// The header.
		ui.Row(c.Key("table-head")).Height(36).Padding(0, 16).Gap(8).AlignItems(ui.Center).Shrink(0).
			BorderWidth(0, 0, 1, 0).BorderColor(hairline(t)).Children(func() {
			ui.Text(c, "主题").FontSize(textSmall).Bold().TextColor(t.TextMuted).Grow(1)
			for _, m := range measures {
				a.measureHeader(c, m, tableCol)
			}
			a.newMeasureField(c, tableCol)
		})
		ui.Scroll(c.Key("table-rows")).Grow(1).Children(func() {
			var row func(n *model.Node, depth int)
			row = func(n *model.Node, depth int) {
				selected := a.selected == n.ID
				r := ui.Row(c.Key("tr:"+n.ID)).Height(tableRowH).Padding(0, 16).Gap(8).AlignItems(ui.Center).
					BorderWidth(0, 0, 1, 0).BorderColor(hairline(t).Alpha(0.5))
				if selected {
					r.Background(t.Accent.Alpha(0.08))
				}
				r.Children(func() {
					title := ui.Row(c.Key("tt")).Grow(1).Shrink(1).MinWidth(0).Gap(4).AlignItems(ui.Center).
						Padding(0, 0, 0, float32(depth)*18).Cursor(ui.CursorPointer).Label("选择 " + n.Title)
					if title.Clicked() {
						id := n.ID
						a.later(func() { a.selectNode(id) })
					}
					title.Children(func() {
						if len(n.Children) > 0 {
							fold := ui.Box(c.Key("fold")).Size(16, 16).Cursor(ui.CursorPointer).Label("折叠 " + n.Title)
							arrow := "▾"
							if n.Collapsed {
								arrow = "▸"
							}
							if fold.Clicked() {
								node := n
								a.later(func() { a.mutate("", func() { node.Collapsed = !node.Collapsed }) })
							}
							fold.Children(func() { ui.Text(c, arrow).FontSize(textSmall).TextColor(t.TextMuted).PassThrough() })
						} else {
							ui.Box(c).Size(16, 16)
						}
						tx := ui.Text(c, n.Title).FontSize(textBody).NoWrap().Ellipsis("…").Shrink(1).MinWidth(0).PassThrough()
						if depth == 0 || len(n.Children) > 0 {
							tx.Bold()
						}
					})
					for _, m := range measures {
						ui.Column(c.Key("c:"+m)).Width(tableCol).Children(func() {
							if len(n.Children) > 0 && n.Measured(m) {
								// A total, with the node's own value under it to edit.
								ui.Text(c, model.FormatValue(n.Total(m))).FontSize(textBody).Bold().Padding(0, 6).
									Tooltip("下级合计，含它自己的值")
							} else {
								a.valueCell(c, n, m, tableCol)
							}
						})
					}
					ui.Box(c).Width(tableCol)
				})
				if !n.Collapsed {
					for _, ch := range n.Children {
						row(ch, depth+1)
					}
				}
			}
			row(d.Root, 0)
		})
	})
}

// measuresSection is the node panel's part of measures: the node's own
// values, and the totals of what is below.
func (a *app) measuresSection(c *ui.Context, n *model.Node) {
	if a.file == nil || len(a.file.Measures) == 0 {
		return
	}
	t := c.Theme()
	ui.Column(c.Key("measures")).Gap(6).Children(func() {
		sectionHeader(c, "度量")
		for _, m := range a.file.Measures {
			ui.Row(c.Key(m)).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, m).FontSize(textSmall).TextColor(t.TextMuted).Width(56).NoWrap()
				a.valueCell(c, n, m, 90)
				if len(n.Children) > 0 && n.Measured(m) {
					ui.Text(c, "合计 "+model.FormatValue(n.Total(m))).FontSize(textSmall).TextColor(t.TextMuted)
				}
			})
		}
	})
}
