package main

import (
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
	if a.presenting && a.doc != nil {
		a.presentView(c)
		a.runPending()
		return
	}
	ui.Row(c).Fill().AlignItems(ui.Stretch).Background(t.Background).Children(func() {
		ui.Split(c, &a.sidebarWidth, func() { a.sidebar(c) }, func() {
			ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
				if a.libOpen {
					a.libraryView(c)
					if a.aiOpen {
						a.panelResizer(c)
						a.panel(c)
					}
					return
				}
				ui.Column(c).Grow(1).Children(func() {
					a.toolbar(c)
					ui.Divider(c)
					a.conflictBar(c)
					a.brokenBar(c)
					switch {
					case a.doc == nil:
						a.empty(c)
					case a.mode == viewMap:
						a.canvasView(c)
					case a.mode == viewOutline:
						ui.Column(c).Grow(1).Children(func() {
							a.outlineView(c)
							ui.Divider(c)
							a.sheetTabs(c)
						})
					case a.mode == viewBoard:
						a.boardView(c)
					case a.mode == viewTimeline:
						a.timelineView(c)
					case a.mode == viewOverview:
						a.overviewView(c)
					default:
						a.agendaView(c)
					}
				})
				a.panelResizer(c)
				a.panel(c)
			})
		}).Fill()
		if a.lightbox != "" && a.doc != nil {
			a.lightboxView(c)
		}
		a.paletteView(c)
		a.cheatSheet(c)
		a.historyView(c)
		a.mcpView(c)
		a.settingsView(c)
		a.fromTextView(c)
		a.templatesView(c)
		a.tourView(c)
	})
	if len(a.pending) > 0 {
		a.runPending()
		c.Invalidate()
	}
}

// panelResizer is the line left of the panel, which the pointer drags to
// make the panel wider or narrower.
func (a *app) panelResizer(c *ui.Context) {
	t := c.Theme()
	line := ui.Box(c).Width(1).FillHeight().Background(t.Border)
	line.Children(func() {
		edge := ui.Box(c.Key("panel-resizer")).Absolute().Left(-3).Top(0).Width(7).FillHeight().
			Cursor(ui.CursorResizeEW).Label("调整面板宽度")
		if edge.Hovered() || a.resizingPanel {
			edge.Background(t.Accent.Alpha(0.35))
		}
		if dx, _, ok := edge.Dragged(); ok {
			a.resizingPanel = true
			w, _ := c.Size()
			a.panelWidth = max(min(a.panelWidth-dx, 720, w-a.sidebarWidth-360), panelMin)
		} else if a.resizingPanel {
			a.resizingPanel = false
			a.cfg.PanelWidth = a.panelWidth
			a.cfg.save()
		}
	})
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
	if hex, ok := tagColors[tag]; ok {
		return ui.Hex(hex)
	}
	var h uint32 = 2166136261
	for _, b := range []byte(tag) {
		h = (h ^ uint32(b)) * 16777619
	}
	return ui.Hex(tagPalette[h%uint32(len(tagPalette))])
}

func (a *app) toolbar(c *ui.Context) {
	t := c.Theme()
	// The room between the sidebar and the panel: the name and the status
	// give way first, then the search narrows, and the buttons keep their
	// size.
	w, _ := c.Size()
	room := w - a.sidebarWidth - a.panelWidth
	ui.Row(c).Padding(8, 12).Gap(10).Wrap().GapY(6).Children(func() {
		if a.doc != nil {
			ui.Text(c, mapName(a.path)).Bold().FontSize(15).NoWrap().MaxWidth(220).Ellipsis("…").Shrink(1).MinWidth(40)
			status, col := "已自动保存", t.TextMuted
			switch {
			case a.saveErr != "":
				status, col = a.saveErr, t.Danger
			case a.notice != "" && a.now().Sub(a.noticeAt) < noticeTime:
				status, col = a.notice, t.Accent
				c.After(noticeTime - a.now().Sub(a.noticeAt))
			case a.dirty:
				status = "编辑中，稍后自动保存…"
			case !a.savedAt.IsZero():
				status = "已自动保存 " + a.savedAt.Format("15:04")
			}
			if room >= 820 || a.saveErr != "" || col == t.Accent {
				ui.Text(c, status).FontSize(12).TextColor(col).NoWrap().MaxWidth(240).Ellipsis("…").Shrink(1).MinWidth(0).
					Tooltip("编辑停顿后自动保存到文件，无需手动保存")
			}
		}
		ui.Spacer(c)
		if a.doc == nil {
			return
		}
		mapLabel := "思维导图"
		if room < 960 {
			mapLabel = "导图"
		}
		ui.Segmented(c, &a.mode, mapLabel, "大纲", "看板", "日程", "时间线", "总览").Shrink(0)
		if iconButton(c, iconPlay, "开始演说", "开始演说 ⌘↩", 32, true).Border(1, t.Border).Clicked() {
			a.later(func() { a.startPresenting("") })
		}
		search := ui.TextInput(c.Key("search"), &a.search).Placeholder("搜索…").Tooltip("搜索标题、备注、标签，回车跳到下一个").Label("搜索").Width(searchWidth(room)).Shrink(1).MinWidth(70)
		if a.search != "" && search.Shortcut(0, ui.KeyEscape) {
			a.search = ""
		}
		if search.Submitted() {
			a.mode = viewMap
			a.nextMatch()
		}
		if room >= 1000 && ui.Button(c, "⌘K").Label("命令面板").Tooltip("命令面板：查找命令、节点和导图 ⌘K").Shrink(0).Clicked() {
			a.openPalette(paletteAll)
		}
		aiLabel := "✦ AI"
		if room < 900 {
			aiLabel = "✦"
		}
		ai := ui.Button(c, aiLabel).Label("AI 助手").Tooltip("AI 助手").Shrink(0)
		if a.aiOpen {
			ai.Background(t.Accent).TextColor(t.AccentText)
		}
		if ai.Clicked() {
			a.aiOpen = !a.aiOpen
		}
		ui.Row(c).Gap(2).Shrink(0).Children(func() {
			if iconButton(c, iconUndo, "撤销", "撤销 ⌘Z", 32, a.hist.CanUndo()).Clicked() && a.hist.CanUndo() {
				a.undo()
			}
			if iconButton(c, iconRedo, "重做", "重做 ⌘⇧Z", 32, a.hist.CanRedo()).Clicked() && a.hist.CanRedo() {
				a.redo()
			}
		})
	})
}

// searchWidth is the search field's width for the toolbar's room.
func searchWidth(room float32) float32 {
	switch {
	case room >= 900:
		return 160
	case room >= 740:
		return 130
	}
	return 90
}
