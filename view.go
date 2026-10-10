package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// view builds the window: the files and tags on the left, the map or the
// tasks in the middle, and the selected node's details on the right.
func (a *app) view(c *ui.Context) {
	a.tick(c)
	t := c.Theme()
	ui.Row(c).Fill().AlignItems(ui.Stretch).Background(t.Background).Children(func() {
		ui.Split(c, &a.sidebarWidth, func() { a.sidebar(c) }, func() {
			ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
				ui.Column(c).Grow(1).Children(func() {
					a.toolbar(c)
					ui.Divider(c)
					switch {
					case a.doc == nil:
						a.empty(c)
					case a.mode == viewMap:
						a.canvasView(c)
					case a.mode == viewBoard:
						a.boardView(c)
					default:
						a.agendaView(c)
					}
				})
				ui.Box(c).Width(1).FillHeight().Background(t.Border)
				a.panel(c)
			})
		}).Fill()
	})
	if len(a.pending) > 0 {
		a.runPending()
		c.Invalidate()
	}
	switch ui.AlertDialog(c, &a.confirmDelete, "把「"+mapName(a.deletePath)+"」移到废纸篓？", "可以在废纸篓中找回。", "取消", "移到废纸篓") {
	case 1:
		a.deleteMap(a.deletePath)
	}
}

func mapName(path string) string {
	return strings.TrimSuffix(filepath.Base(path), model.Ext)
}

func (a *app) empty(c *ui.Context) {
	ui.Column(c).Grow(1).Center().Gap(12).Children(func() {
		ui.Text(c, "没有打开的导图").FontSize(16).TextColor(c.Theme().TextMuted)
		if ui.PrimaryButton(c, "新建导图").Clicked() {
			a.newMap()
		}
	})
}

func (a *app) sidebar(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Surface).Children(func() {
		ui.Row(c).Padding(10, 12).Gap(6).Children(func() {
			ui.Text(c, filepath.Base(a.dir)).Bold().Grow(1).NoWrap().Ellipsis("…").Tooltip(a.dir)
			if ui.Button(c, "打开…").Tooltip("选择存放导图的文件夹").Clicked() {
				a.plat.PickFolder(func(dir string) {
					if dir != "" {
						a.flush()
						a.openDir(dir)
						a.cfg.save()
						if len(a.files) > 0 {
							_ = a.open(a.files[0].Path)
						}
					}
				})
			}
		})
		sel := a.path
		ui.Sidebar(c, &sel, func() {
			ui.SidebarSection(c, "导图", &a.filesOpen, func() {
				for _, f := range a.files {
					item := ui.SidebarItem(c.Key(f.Path), f.Path, nil, f.Name)
					path := f.Path
					item.ContextMenu(func(m *ui.Menu) {
						if m.Item("在 Finder 中显示").Chosen() {
							a.plat.ShowInFolder(path)
						}
						m.Separator()
						if m.Item("移到废纸篓…").Chosen() {
							a.deletePath, a.confirmDelete = path, true
						}
					})
				}
			})
		}).Grow(1).OnChange(func() {
			if sel != a.path {
				_ = a.open(sel)
			}
		})
		if ui.Button(c, "+ 新建导图").Margin(0, 12, 8).Clicked() {
			a.newMap()
		}
		if a.doc != nil {
			a.tagList(c)
		}
	})
}

// tagList shows the tags of the map; choosing one highlights its nodes and
// filters the tasks.
func (a *app) tagList(c *ui.Context) {
	t := c.Theme()
	tags := a.file.AllTags()
	counts := map[string]int{}
	a.file.Walk(func(n *model.Node) {
		for _, tag := range n.Tags {
			counts[tag]++
		}
	})
	ui.Divider(c)
	ui.Column(c).Padding(8, 12, 12).Gap(6).MaxHeightPercent(40).Children(func() {
		ui.Row(c).Children(func() {
			ui.Text(c, "按标签筛选").FontSize(12).Bold().TextColor(t.TextMuted).Grow(1)
			if a.tagFilter != "" && ui.Link(c, "清除筛选", "").FontSize(12).Clicked() {
				a.tagFilter = ""
			}
		})
		if len(tags) == 0 {
			ui.Text(c, "还没有标签").FontSize(12).TextColor(t.TextMuted)
			return
		}
		ui.Scroll(c).Children(func() {
			ui.Row(c).Wrap().Gap(6).Children(func() {
				for _, tag := range tags {
					chip := tagChip(c.Key(tag), tag, fmt.Sprintf("%s %d", tag, counts[tag]), a.tagFilter == tag, 12)
					if chip.Clicked() {
						if a.tagFilter == tag {
							a.tagFilter = ""
						} else {
							a.tagFilter = tag
						}
					}
				}
			})
		})
	})
}

// tagChip draws a tag as a small pill, colored by its name.
func tagChip(c *ui.Context, tag, label string, active bool, size float32) ui.Element {
	col := tagColor(tag)
	chip := ui.Box(c).Padding(size*0.15, size*0.55).Radius(999).Cursor(ui.CursorPointer)
	if active {
		chip.Background(col).Children(func() {
			ui.Text(c, label).FontSize(size).TextColor(ui.Hex("#ffffff")).NoWrap()
		})
	} else {
		chip.Background(col.Alpha(0.14)).Children(func() {
			ui.Text(c, label).FontSize(size).TextColor(col).NoWrap()
		})
	}
	return chip
}

var tagPalette = []string{"#2563eb", "#16a34a", "#d97706", "#db2777", "#7c3aed", "#0891b2", "#dc2626", "#4d7c0f"}

func tagColor(tag string) ui.Color {
	var h uint32 = 2166136261
	for _, b := range []byte(tag) {
		h = (h ^ uint32(b)) * 16777619
	}
	return ui.Hex(tagPalette[h%uint32(len(tagPalette))])
}

func (a *app) toolbar(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Padding(8, 12).Gap(10).Children(func() {
		if a.doc != nil {
			ui.Text(c, mapName(a.path)).Bold().FontSize(15).NoWrap().MaxWidth(220).Ellipsis("…")
			status, col := "已自动保存", t.TextMuted
			switch {
			case a.saveErr != "":
				status, col = a.saveErr, t.Danger
			case a.dirty:
				status = "编辑中，稍后自动保存…"
			case !a.savedAt.IsZero():
				status = "已自动保存 " + a.savedAt.Format("15:04")
			}
			ui.Text(c, status).FontSize(12).TextColor(col).NoWrap().MaxWidth(240).Ellipsis("…").
				Tooltip("编辑停顿后自动保存到文件，无需手动保存")
		}
		ui.Spacer(c)
		if a.doc == nil {
			return
		}
		ui.Segmented(c, &a.mode, "思维导图", "看板", "日程")
		search := ui.TextInput(c.Key("search"), &a.search).Placeholder("搜索（回车跳到下一个）").Label("搜索").Width(200)
		if a.search != "" && search.Shortcut(0, ui.KeyEscape) {
			a.search = ""
		}
		if search.Submitted() {
			a.mode = viewMap
			a.nextMatch()
		}
		ui.Row(c).Gap(2).Children(func() {
			if ui.Button(c, "↶").Label("撤销").Tooltip("撤销 ⌘Z").Width(32).Disabled(!a.hist.CanUndo()).Clicked() {
				a.undo()
			}
			if ui.Button(c, "↷").Label("重做").Tooltip("重做 ⌘⇧Z").Width(32).Disabled(!a.hist.CanRedo()).Clicked() {
				a.redo()
			}
		})
	})
}
