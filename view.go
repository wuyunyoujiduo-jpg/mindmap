package main

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// view builds the window: the files and tags on the left, the map or the
// tasks in the middle, and the selected node's details on the right.
func (a *app) view(c *ui.Context) {
	a.tick(c)
	if a.noMotion {
		c.DisableAnimations()
	}
	groundKey = a.cfg.Ground
	t := quietTheme(c.Theme())
	c.SetTheme(t)
	a.lastTheme = t
	if a.presenting && a.doc != nil {
		a.presentView(c)
		a.runPending()
		return
	}
	// Selected text shows in the accent color, strong enough on the
	// panels' gray: the system's highlight (a pale orange, say) hardly did.
	root := ui.Column(c).Fill().AlignItems(ui.Stretch).SelectionColor(t.Accent.Alpha(0.32))
	root.Background(ground(t))
	root.Children(func() {
		if a.libOpen {
			a.placesBar(c) // the three places; a map opened has its own bar
			a.oneBar(c)    // writes, looks for, asks: on every page of lists
		}
		func() {
			// The pages of lists lie on the ground; a map is on white.
			page := ui.Row(c.Key("page")).Grow(1).Basis(0).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch)
			if !a.libOpen {
				page.Background(t.Background)
			}
			func() {
				page.Children(func() {
					if a.treeShown() {
						a.mapTree(c) // the directory, in 项目
						if !a.libOpen {
							ui.Box(c).Width(1).FillHeight().Background(hairline(t))
						}
					}
					if a.libOpen {
						if a.projOpen {
							a.projectsView(c)
						} else if a.tasks.open || a.libHome {
							a.tasksView(c) // 今天: what is to do, and what was written down
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
						if a.doc == nil || a.mode != viewMap && a.mode != viewOutline {
							ui.Divider(c) // the map and the outline are the page itself
						}
						a.conflictBar(c)
						a.brokenBar(c)
						switch {
						case a.doc == nil:
							a.empty(c)
						case a.mode == viewOutline:
							ui.Column(c).Grow(1).Children(func() {
								a.outlineView(c)
								a.sheetTabs(c)
							})
						case a.mode == viewBoard:
							a.boardView(c)
						case a.mode == viewTimeline:
							a.timelineView(c)
						case a.mode == viewOverview:
							a.overviewView(c)
						default:
							a.canvasView(c)
						}
					})
					if a.panelShown(c) {
						a.panelResizer(c)
						a.panel(c)
					}
				})
			}()
		}()
		if !a.libOpen && a.barOpen && !a.presenting {
			a.barOver(c)
		}
		if a.lightbox != "" && a.doc != nil {
			a.lightboxView(c)
		}
		a.deleteConfirm(c)
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
			a.panelWidth = max(min(a.panelWidth-dx, 720, w-a.sideWidth()-360), panelMin)
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
		if inkButton(c, "新建导图", true).Clicked() {
			a.newMap()
		}
	})
}

// tagChip draws a tag as a small pill, colored by its name.
func tagChip(c *ui.Context, tag, label string, active bool, size float32) ui.Element {
	// Quietly: a gray pill with a dot of the tag's color, which says which
	// tag it is without a patch of color; the one chosen is tinted with it.
	t := c.Theme()
	col := tagColor(tag)
	chip := ui.Row(c).Padding(size*0.15, size*0.6, size*0.15, size*0.5).Gap(size * 0.35).Radius(999).AlignItems(ui.Center).
		Cursor(ui.CursorPointer)
	bg, ink := t.Text.Alpha(0.055), t.Text.MixOklab(t.TextMuted, 0.7)
	if active {
		bg, ink = col.Alpha(0.16), col.MixOklab(t.Text, 0.45)
	}
	chip.Background(bg).Children(func() {
		ui.Box(c).Size(size*0.45, size*0.45).Radius(999).Background(col).Shrink(0).PassThrough()
		ui.Text(c, label).FontSize(size).TextColor(ink).NoWrap().PassThrough()
	})
	return chip
}

var tagPalette = []string{"#4F74B8", "#4F9668", "#B7843F", "#B3618A", "#7A68BD", "#3F8C9C", "#B95A54", "#6E8A45"}

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
	room := w - a.sideWidth() - a.panelWidth
	if a.aiBeside(c) {
		room -= a.panelWidth + 1
	}
	// The toolbar is the window's title bar: it drags the window, and its
	// title is the map's, with how saving goes below it.
	gap := float32(10)
	if room < 820 { // the seven views take what room there is
		gap = 4
	}
	ui.Row(c.Key("toolbar")).Height(toolbarHeight).Padding(0, max(10, c.TitleBar().Right), 0, max(14, b2f(!a.treeShown())*(c.TitleBar().Left+12))).Gap(gap).AlignItems(ui.Center).DragWindow().Children(func() {
		if a.doc == nil {
			ui.Spacer(c)
			return
		}
		if a.cfg.HideTree { // the directory, put away, comes back from here
			if iconButton(c, iconSidebar, "显示目录", "显示目录：文件夹和导图的列表", 30, true).Clicked() {
				a.cfg.HideTree = false
				a.cfg.save()
			}
		}
		// Back to 项目, the maps as cards.
		if textKey(c, "‹ 项目", "回到项目", "回到项目：所有导图").Clicked() {
			a.later(a.showProjects)
		}
		ui.Column(c).Gap(1).Shrink(1).MinWidth(40).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				// The map's color, which a click changes.
				dot := ui.Box(c.Key("map-color")).Size(14, 14).Radius(5).Background(a.mapColor(a.path)).Shrink(0).
					Cursor(ui.CursorPointer).Label("导图颜色").Tooltip("这张导图的颜色：它的任务、从它来的东西都带这个颜色")
				dot.Menu(a.mapColorMenu)
				// The map's name switches maps: those opened last, the list,
				// a new one.
				name := ui.Row(c.Key("map-switch")).Gap(4).AlignItems(ui.Center).Radius(6).Padding(1, 4).Margin(0, 0, 0, -4).
					Cursor(ui.CursorPointer).Label("切换导图").Tooltip("切换到其他导图")
				if name.Hovered() {
					name.Background(t.Text.Alpha(0.06))
				}
				name.Menu(a.mapSwitchMenu)
				name.Children(func() {
					ui.Text(c, mapName(a.path)).Bold().FontSize(textTitle).NoWrap().MaxWidth(220).Ellipsis("…").PassThrough()
					ui.Text(c, "⌄").FontSize(textSmall).TextColor(t.TextMuted).PassThrough()
				})
			})
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
		a.viewTabs(c, room)
		ui.Spacer(c)
		if a.searchOpen || a.search != "" {
			a.searchField(c, searchWidth(room))
		}
		if a.mode == viewMap && room >= 560 {
			a.structurePicker(c, room < 900)
		}
		if room >= 450 && iconButton(c, iconPlay, "开始演说", "开始演说 ⌘↩", 30, true).Clicked() {
			a.later(func() { a.startPresenting("") })
		}
		// The one bar, over the map: write a thought down, look for, ask.
		if room >= 740 && iconButton(c, iconPencil, "记一句", "记一句、搜索，或者问 AI", 30, true).Clicked() { // where there is room for it
			a.barOpen, a.writeRefocus = true, "home-line"
		}
		aiText := "问 AI"
		if room < 760 {
			aiText = ""
		}
		if iconTextButton(c, iconSparkle, aiText, "AI 助手", "问 AI：关于这张导图和收集的资料", a.aiOpen).Clicked() {
			a.aiOpen = !a.aiOpen
		}
		// The panel of the topic chosen, put away or shown.
		tip := "显示主题面板：备注、任务、标签、链接 ⌘⌥I"
		if !a.panelOff {
			tip = "收起主题面板 ⌘⌥I"
		}
		if iconButton(c, iconPanel, "主题面板", tip, 30, true).Clicked() {
			a.togglePanel()
		}
	})
}

// togglePanel shows the panel of the topic chosen, or puts it away, and
// keeps which.
func (a *app) togglePanel() {
	a.panelOff = !a.panelOff
	a.cfg.PanelOpen = !a.panelOff
	a.cfg.save()
}

// The views a map's toolbar shows, the others in its 更多 menu.
var (
	mainViews = []struct {
		mode int
		name string
	}{{viewMap, "导图"}, {viewOutline, "大纲"}, {viewBoard, "看板"}}
	moreViews = []struct {
		mode int
		name string
	}{{viewTimeline, "时间线"}, {viewOverview, "总览"}}
)

// viewTabs switch the map's view: four as tabs, the rest in a menu; in a
// narrow window, all in a menu.
func (a *app) viewTabs(c *ui.Context, room float32) {
	t := c.Theme()
	if room < 560 {
		name := "导图"
		for _, v := range append(slices.Clone(mainViews), moreViews...) {
			if v.mode == a.mode {
				name = v.name
			}
		}
		ui.MenuButton(c, name, func(m *ui.Menu) {
			for _, v := range append(slices.Clone(mainViews), moreViews...) {
				if m.Item(v.name).Checked(a.mode == v.mode).Chosen() {
					a.mode = v.mode
				}
			}
		}).Label("视图").Shrink(0)
		return
	}
	ui.Row(c.Key("view-tabs")).Padding(2).Gap(2).Radius(radiusPill).Background(t.Text.Alpha(0.05)).Shrink(0).Children(func() {
		tab := func(key, name string, on bool) ui.Element {
			b := ui.Box(c.Key("view:"+key)).Height(28).Padding(0, 14).Radius(radiusPill).Center().Cursor(ui.CursorPointer).Label(name).
				Background(t.Text.Alpha(0)).Transition(easeLook)
			if !on && b.Hovered() {
				b.Background(t.Text.Alpha(0.05))
			}
			b.Children(func() {
				if on {
					slidePill(c, "view-tabs", radiusPill)
					b.ZIndex(-1) // under the others, which the pill slides behind
				}
				txt := ui.Text(c, name).FontSize(textSmall).PassThrough()
				if on {
					txt.FontWeight(600)
				} else {
					txt.TextColor(t.Text.MixOklab(t.TextMuted, 0.6))
				}
			})
			return b
		}
		for _, v := range mainViews {
			if tab(v.name, v.name, a.mode == v.mode).Clicked() {
				a.mode = v.mode
			}
		}
		more, on := "更多 ⌄", false
		for _, v := range moreViews {
			if a.mode == v.mode {
				more, on = v.name+" ⌄", true
			}
		}
		tab("more", more, on).Label("更多视图").Menu(func(m *ui.Menu) {
			for _, v := range moreViews {
				if m.Item(v.name).Checked(a.mode == v.mode).Chosen() {
					a.mode = v.mode
				}
			}
		})
	})
}

// searchField is the toolbar's search: a field with a magnifier, which
// Return steps through what it finds and Escape clears.
func (a *app) searchField(c *ui.Context, width float32) {
	search := searchBox(c, "search", &a.search, "搜索", "搜索导图", "搜索标题、备注、标签，回车跳到下一个", width)
	if a.searchOpen && a.search == "" {
		search.AutoFocus()
	}
	if search.Shortcut(0, ui.KeyEscape) {
		a.search, a.searchOpen = "", false
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
	return a.aiOpen && a.doc != nil && !a.libOpen && w-a.sideWidth()-2*a.panelWidth-1 >= aiBesideCanvas
}

// panelShown tells whether the right panel shows: not turned off (⌥⌘I),
// and with room for the map, unless the AI is asked.
func (a *app) panelShown(c *ui.Context) bool {
	if a.aiOpen {
		return true
	}
	w, _ := c.Size()
	return !a.panelOff && w-a.sideWidth()-a.panelWidth >= panelMinCanvas
}
