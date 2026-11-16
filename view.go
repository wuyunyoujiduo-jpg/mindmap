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
	a.sweepVideos()
	if a.tagFocus > 0 {
		a.tagFocus--
	}
	if a.tagCaretTTL > 0 {
		if a.tagCaretTTL--; a.tagCaretTTL == 0 {
			a.tagCaret = "" // its field went: not put on another one's later
		}
	}
	motionOff = a.noMotion
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
	root := ui.Row(c).Fill().AlignItems(ui.Stretch).SelectionColor(t.Accent.Alpha(0.32))
	root.Background(ground(t))
	root.Children(func() {
		// The window is a quiet gray: the top row on it, and under it the one
		// white leaf everything else is on, cut into columns by thin lines:
		// what of the place shows, the page, what is chosen.
		ui.Column(c.Key("main")).Grow(1).Basis(0).MinWidth(0).FillHeight().Children(func() {
			a.topBar(c)
			leaf(c, "leaf").Grow(1).Basis(0).MinHeight(0).Margin(0, 6, 6).Children(func() {
				ui.Row(c.Key("columns")).Grow(1).Basis(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
					if a.sideShown() {
						switch a.place() {
						case placeMaps:
							a.mapTree(c)
						case placeToday:
							a.todaySide(c)
						case placeSkills:
							a.skillsSide(c)
						default:
							a.libSide(c)
						}
					}
					a.pageView(c)
				})
			})
		})
		if a.aiOpen && !a.presenting {
			a.aiFloat(c)
		}
		a.jobsView(c)
		a.toasts(c)
		toastLook(c)
		if a.lightbox != "" && a.doc != nil {
			a.lightboxView(c)
		}
		if a.videoBig != "" {
			a.videoBigView(c)
		}
		a.deleteConfirm(c)
		a.paletteView(c)
		a.cheatSheet(c)
		a.historyView(c)
		a.mcpView(c)
		a.settingsView(c)
		a.fromTextView(c)
		a.mergeView(c)
		a.templatesView(c)
		a.tourView(c)
		a.shotAlertView(c)
		a.cardWindow(c)
	})
	if len(a.pending) > 0 {
		a.runPending()
		c.Invalidate()
	}
	if len(a.grabs) > 0 { // a video's pictures being taken: a step a frame
		a.stepGrab(c)
		c.Invalidate()
	}
}

// pageView is the page beside the directory: a place's, or a map's.
func (a *app) pageView(c *ui.Context) {
	// Every page is on the leaf's white.
	page := ui.Row(c.Key("page")).Grow(1).Basis(0).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch)
	func() {
		page.Children(func() {
			if a.libOpen {
				if a.projOpen {
					a.projectsView(c)
				} else if a.skillsOpen {
					a.skillsPage(c) // 技能: the AI agents' skills
				} else if a.tasks.open || a.libHome {
					a.tasksView(c) // 今天: what is to do, and what was written down
				} else {
					a.libraryView(c)
				}
				return
			}
			// No wider than its share of the leaf, whatever is on it: a bar
			// of a long name is to cut the name short, not to push its keys
			// over the right column.
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				a.toolbar(c)
				if a.doc == nil || a.mode != viewMap && a.mode != viewOutline && a.mode != viewDiagram {
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
				case a.mode == viewDiagram:
					a.diagramView(c)
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
}

// aiFloat is the AI's window, floating at the window's foot on the right,
// over whatever page shows: it is called from the spark of the top row,
// takes no room from the page, and stays as the places change. It is the
// one thing on the page with a shadow, for it floats.
func (a *app) aiFloat(c *ui.Context) {
	t := c.Theme()
	w, h := c.Size()
	lifted(ui.Column(c.Key("ai-float")).Absolute().Right(16).Bottom(16).Width(min(392, w-32)).Height(min(580, (h-toolbarHeight)*0.78)).
		Radius(16).Clip().ZIndex(25).Background(cardColor(t)).Label("AI 窗口"), t).
		Transition(ui.ElementTransition{Duration: motionFast, Ease: easeDetent, Enter: &ui.Motion{Y: 16, Scale: 0.96},
			Exit: &ui.Motion{Y: 12, Scale: 0.96}, ExitDuration: motionQuick}).
		Children(func() { a.aiPanel(c) })
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
	ui.Row(c.Key("toolbar")).Height(toolbarHeight).Padding(0, 10, 0, 16).Gap(gap).AlignItems(ui.Center).Children(func() {
		if a.doc == nil {
			ui.Spacer(c)
			return
		}
		// The views' tabs are in the bar's middle whatever stands at its
		// two ends: with the room, each end takes half of what the tabs
		// leave. Laid one after another, the tabs moved as the view was
		// changed, the map's own keys at the right coming and going: the
		// user saw them jump on a click of one, which a thing under the
		// pointer must not do.
		ends := float32(230) // the right end at its widest: the structure, the two keys
		if a.searchOpen || a.search != "" {
			ends += searchWidth(room) + gap
		}
		centered := room >= 360+2*(ends+gap)
		// The right column put away, the bar is longer by it at its right:
		// that length is the right end's to begin with, so that the tabs
		// are where they are with the column shown. A set of 图解 shows
		// the column though it is put away, and the tabs moved by half of
		// it on the click.
		var away float32
		if w-a.sideWidth()-a.panelWidth >= panelMinCanvas && !a.panelShown(c) {
			away = a.panelWidth + 1 // and its line
		}
		end := func(k string) ui.Element {
			r := ui.Row(c.Key(k)).Gap(gap).AlignItems(ui.Center)
			if centered {
				return r.Grow(1).Basis(0).MinWidth(0)
			}
			return r.Shrink(1).MinWidth(40)
		}
		end("bar-left").Children(func() {
			// The name gives way to the tabs and the keys, cut short with
			// an ellipsis: unshrinking, a long one lay over the tabs in a
			// narrow bar and pushed the keys out of it.
			ui.Column(c).Gap(1).Shrink(1).MinWidth(40).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Shrink(1).MinWidth(0).Children(func() {
					// The map's color, which a click changes.
					dot := ui.Box(c.Key("map-color")).Size(14, 14).Radius(5).Background(a.mapColor(a.path)).Shrink(0).
						Cursor(ui.CursorPointer).Label("导图颜色").Tooltip("这张导图的颜色：它的任务、从它来的东西都带这个颜色")
					dot.Menu(a.mapColorMenu)
					// The map's name switches maps: those opened last, the list,
					// a new one.
					name := ui.Row(c.Key("map-switch")).Gap(4).AlignItems(ui.Center).Radius(6).Padding(1, 4).Margin(0, 0, 0, -4).Shrink(1).MinWidth(0).
						Cursor(ui.CursorPointer).Label("切换导图").Tooltip("切换到其他导图")
					if name.Hovered() {
						name.Background(t.Text.Alpha(0.06))
					}
					name.Menu(a.mapSwitchMenu)
					name.Children(func() {
						ui.Text(c, mapName(a.path)).FontWeight(700).LetterSpacing(-0.3).FontSize(textTitle).SingleLine().MaxWidth(220).Shrink(1).MinWidth(0).PassThrough()
						chevron(c, t.TextMuted)
					})
				})
				status, col := "已自动保存", t.TextMuted
				switch {
				case a.saveErr != "":
					status, col = a.saveErr, t.Danger
				case a.dirty:
					status = "编辑中…"
				case !a.savedAt.IsZero():
					status = "已保存 " + a.savedAt.Format("15:04")
				}
				if room >= 700 || a.saveErr != "" || col == t.Accent {
					ui.Row(c.Key("under-name")).Gap(6).AlignItems(ui.Center).Children(func() {
						// Where the map is kept, when in a folder: its folder, which a
						// click shows the maps of. The directory opens to it too
						// (app.open), but put away it says nothing.
						if dir := filepath.Dir(a.path); room >= 700 && dir != a.dir && inside(a.path, a.dir) {
							where := ui.Row(c.Key("map-folder")).Gap(4).AlignItems(ui.Center).Cursor(ui.CursorPointer).Shrink(1).MinWidth(0).
								Label("所在文件夹 " + a.relDir(a.path)).Tooltip("这张导图在文件夹「" + a.relDir(a.path) + "」里：点一下看这个文件夹的导图")
							ink := t.TextMuted
							if where.Hovered() {
								ink = t.Text
							}
							if where.Clicked() {
								a.later(func() {
									a.showProjects()
									a.projFolder = dir
								})
							}
							where.Children(func() {
								glyph(c, iconFolder, 11, ink)
								ui.Text(c, strings.ReplaceAll(a.relDir(a.path), string(filepath.Separator), " › ")).FontSize(textCaption).TextColor(ink).
									NoWrap().MaxWidth(150).Ellipsis("…").PassThrough()
								ui.Text(c, "·").FontSize(textCaption).TextColor(faint(t)).PassThrough()
							})
						}
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
					})
				}
			})
		})
		if !centered {
			ui.Spacer(c)
		}
		a.viewTabs(c, room)
		if !centered {
			ui.Spacer(c).Basis(away).Shrink(0)
		}
		right := end("bar-right").Shrink(0)
		if centered {
			right.Basis(away).Justify(ui.End)
		}
		right.Children(func() {
			if a.searchOpen || a.search != "" {
				a.searchField(c, searchWidth(room))
			}
			if a.mode == viewMap && room >= 560 {
				a.structurePicker(c, room)
			} else if room >= 560 {
				a.structureRoom(c, room)
			}
			if room >= 450 && iconButton(c, iconPlay, "开始演说", "开始演说 ⌘↩", 30, true).Clicked() {
				a.later(func() { a.startPresenting("") })
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
	}{{viewMap, "导图"}, {viewOutline, "大纲"}, {viewBoard, "看板"}, {viewDiagram, "图解"}}
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
					a.showView(v.mode)
				}
			}
		}).Label("视图").Shrink(0)
		return
	}
	well(ui.Row(c.Key("view-tabs")).Padding(2).Gap(2).Radius(radiusPill).Shrink(0), t).Children(func() {
		opens := false // the tab being built opens a menu: it ends in a chevron
		pad := float32(14)
		if room < 700 { // a narrow bar: the tabs closer, that its keys stay in it
			pad = 9
		}
		tab := func(key, name string, on bool) ui.Element {
			more := opens
			b := ui.Row(c.Key("view:"+key)).Height(28).Padding(0, pad).Gap(5).Radius(radiusPill).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(name).
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
				col := t.Text
				if on {
					txt.FontWeight(700)
				} else {
					col = t.Text.MixOklab(t.TextMuted, 0.6)
					txt.TextColor(col)
				}
				if more {
					chevron(c, col)
				}
			})
			return b
		}
		for _, v := range mainViews {
			if tab(v.name, v.name, a.mode == v.mode).Clicked() {
				a.showView(v.mode)
			}
		}
		more, on := "更多", false
		for _, v := range moreViews {
			if a.mode == v.mode {
				more, on = v.name, true
			}
		}
		opens = true
		// As wide whichever of its views it names, so that the tabs before
		// it stay where they are: as its longest name in bold, measured,
		// for a width in figures is one font's.
		var widest float32
		for _, v := range moreViews {
			w, _ := a.measureText(c, 0, ui.Span{Text: v.name, Size: textSmall, Weight: 700})
			widest = max(widest, w)
		}
		tab("more", more, on).MinWidth(2*pad + widest + 5 + 10).Justify(ui.Center).Label("更多视图").Menu(func(m *ui.Menu) {
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
func (a *app) aiBeside(c *ui.Context) bool { return false }

// panelShown tells whether the right panel shows: not turned off (⌥⌘I),
// and with room for the map, unless the AI is asked.
func (a *app) panelShown(c *ui.Context) bool {
	w, _ := c.Size()
	if a.mode == viewDiagram && !a.libOpen && a.doc != nil && a.diaOf(a.dia.node) != nil {
		// A diagram says in it where what is chosen comes from: it shows
		// though put away, while there is room.
		return w-a.sideWidth()-a.panelWidth >= panelMinCanvas
	}
	return !a.panelOff && w-a.sideWidth()-a.panelWidth >= panelMinCanvas
}

// showView turns the map to a view. 图解 opens what was drawn of the topic
// chosen when there is a set of it; else its own first page, which lists
// the map's sets and draws one of the topic chosen.
func (a *app) showView(mode int) {
	if mode != viewDiagram {
		a.mode = mode
		return
	}
	if a.mode == viewDiagram {
		return
	}
	if a.diaOf(a.selected) != nil {
		a.openDia(a.selected, "")
		return
	}
	a.commitEditIfAny()
	a.mode, a.dia = viewDiagram, diaState{}
}
