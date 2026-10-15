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
	// Selected text shows in the accent color, strong enough on the
	// panels' gray: the system's highlight (a pale orange, say) hardly did.
	root := ui.Row(c).Fill().AlignItems(ui.Stretch).SelectionColor(t.Accent.Alpha(0.32))
	if !c.Vibrancy() { // else the sidebar's glass shows through
		root.Background(t.Background)
	}
	root.Children(func() {
		ui.Split(c, &a.sidebarWidth, func() { a.sidebar(c) }, func() {
			ui.Row(c).Fill().AlignItems(ui.Stretch).Background(t.Background).Children(func() {
				if a.libOpen {
					if a.libHome {
						a.homeView(c)
					} else {
						a.libraryView(c)
					}
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
					case a.mode == viewTable:
						a.tableView(c)
					default:
						a.agendaView(c)
					}
				})
				if a.panelShown(c) {
					a.panelResizer(c)
					a.panel(c)
				}
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
		a.shotAlertView(c)
		a.cardWindow(c)
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
	// The room between the sidebar and the panel: the status gives way
	// first, then the search narrows, and the buttons keep their size.
	w, _ := c.Size()
	room := w - a.sidebarWidth - a.panelWidth
	if a.aiBeside(c) {
		room -= a.panelWidth + 1
	}
	// The toolbar is the window's title bar: it drags the window, and its
	// title is the map's, with how saving goes below it.
	gap := float32(10)
	if room < 820 { // the seven views take what room there is
		gap = 4
	}
	ui.Row(c.Key("toolbar")).Height(toolbarHeight).Padding(0, 10, 0, 14).Gap(gap).AlignItems(ui.Center).DragWindow().Children(func() {
		if a.doc == nil {
			ui.Spacer(c)
			return
		}
		ui.Column(c).Gap(1).Shrink(1).MinWidth(40).Children(func() {
			ui.Text(c, mapName(a.path)).Bold().FontSize(textTitle).NoWrap().MaxWidth(240).Ellipsis("…")
			status, col := "已自动保存", t.TextMuted
			switch {
			case a.saveErr != "":
				status, col = a.saveErr, t.Danger
			case a.notice != "" && a.now().Sub(a.noticeAt) < noticeTime:
				status, col = a.notice, t.Accent
				c.After(noticeTime - a.now().Sub(a.noticeAt))
			case a.dirty:
				status = "编辑中…"
			case !a.savedAt.IsZero():
				status = "已保存 " + a.savedAt.Format("15:04")
			}
			if room >= 700 || a.saveErr != "" || col == t.Accent {
				// A click on how saving goes shows the versions kept.
				st := ui.Box(c.Key("save-status")).Cursor(ui.CursorPointer).Label("历史版本").
					Tooltip("自动保存，无需手动保存 · 点击查看历史版本 ⌘⇧H")
				if st.Hovered() && col == t.TextMuted {
					col = t.Text
				}
				if st.Clicked() {
					a.later(a.openHistory)
				}
				st.Children(func() {
					ui.Text(c, status).FontSize(textCaption).TextColor(col).NoWrap().MaxWidth(240).Ellipsis("…").PassThrough()
				})
			}
		})
		ui.Spacer(c)
		mapLabel := "思维导图"
		if room < 960 {
			mapLabel = "导图"
		}
		views := []string{mapLabel, "大纲", "看板", "日程", "时间线", "总览", "表格"}
		if room >= 760 {
			seg := ui.Segmented(c, &a.mode, views...).Shrink(0)
			if room < 900 {
				seg.FontSize(textSmall) // the seven views in less room
			}
		} else { // narrow: a menu of the views
			ui.MenuButton(c, views[a.mode], func(m *ui.Menu) {
				for i, v := range views {
					if m.Item(v).Checked(a.mode == i).Chosen() {
						a.mode = i
					}
				}
			}).Label("视图").Shrink(0)
		}
		ui.Spacer(c)
		a.searchField(c, searchWidth(room))
		if room >= 450 && iconButton(c, iconPlay, "开始演说", "开始演说 ⌘↩", 30, true).Clicked() {
			a.later(func() { a.startPresenting("") })
		}
		aiText := "AI"
		if room < 900 {
			aiText = ""
		}
		if iconTextButton(c, iconSparkle, aiText, "AI 助手", "AI 助手", a.aiOpen).Clicked() {
			a.aiOpen = !a.aiOpen
		}
		ui.Row(c).Gap(0).Shrink(0).Children(func() {
			if iconButton(c, iconUndo, "撤销", "撤销 ⌘Z", 30, a.hist.CanUndo()).Clicked() && a.hist.CanUndo() {
				a.undo()
			}
			if iconButton(c, iconRedo, "重做", "重做 ⌘⇧Z", 30, a.hist.CanRedo()).Clicked() && a.hist.CanRedo() {
				a.redo()
			}
		})
	})
}

// searchField is the toolbar's search: a field with a magnifier, which
// Return steps through what it finds and Escape clears.
func (a *app) searchField(c *ui.Context, width float32) {
	search := searchBox(c, "search", &a.search, "搜索", "搜索", "搜索标题、备注、标签，回车跳到下一个", width)
	if a.search != "" && search.Shortcut(0, ui.KeyEscape) {
		a.search = ""
	}
	if search.Submitted() {
		a.mode = viewMap
		a.nextMatch()
	}
}

// searchWidth is the search field's width for the toolbar's room.
func searchWidth(room float32) float32 {
	switch {
	case room >= 900:
		return 160
	case room >= 820:
		return 130
	case room >= 740:
		return 100 // room for the seven views
	}
	return 90
}

// panelMinCanvas is the narrowest the map is left beside the panel: a
// window narrower hides the panel, as Finder hides its preview.
const panelMinCanvas = 420

// aiBesideCanvas is the room the map keeps for the AI to have a column of
// its own beside the inspector; with less, the AI goes below it.
const aiBesideCanvas = 640

// aiBeside tells whether the AI shows in a column beside the inspector.
func (a *app) aiBeside(c *ui.Context) bool {
	w, _ := c.Size()
	return a.aiOpen && a.doc != nil && !a.libOpen && w-a.sidebarWidth-2*a.panelWidth-1 >= aiBesideCanvas
}

// panelShown tells whether the right panel shows: not turned off (⌥⌘I),
// and with room for the map, unless the AI is asked.
func (a *app) panelShown(c *ui.Context) bool {
	if a.aiOpen {
		return true
	}
	w, _ := c.Size()
	return !a.panelOff && w-a.sidebarWidth-a.panelWidth >= panelMinCanvas
}
