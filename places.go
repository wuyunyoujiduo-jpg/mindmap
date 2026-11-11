package main

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// The app has three places, and no sidebar: 今天 (what is to do and what
// was written down), 项目 (the maps, by folder; one opened takes the
// page) and 收集 (what was collected). A switch at the top of the window
// goes between them (placesBar).
const (
	placeToday = iota
	placeMaps
	placeCollect
)

// place is the place shown: a map opened is of 项目.
func (a *app) place() int {
	switch {
	case !a.libOpen || a.projOpen:
		return placeMaps
	case a.libHome || a.tasks.open:
		return placeToday
	}
	return placeCollect
}

// showProjects shows 项目, the maps by folder.
func (a *app) showProjects() {
	a.showLibrary()
	a.projOpen = true
}

// sideWidth is the room a sidebar takes: none, there is no sidebar.
func (a *app) sideWidth() float32 { return 0 }

// placesBar is the top of the window on the pages of lists: the switch of
// the three places in its middle, the search of everything and the
// settings at its end. It is the window's title bar there, and drags it.
func (a *app) placesBar(c *ui.Context) {
	t := c.Theme()
	bar := c.TitleBar()
	now := a.place()
	ui.Row(c.Key("places-bar")).Height(toolbarHeight).Padding(0, max(14, bar.Right), 0, max(14, bar.Left)).AlignItems(ui.Center).
		Shrink(0).FillWidth().DragWindow().Children(func() {
		ui.Box(c).Grow(1).Basis(0)
		ui.Row(c.Key("places")).Padding(3).Gap(2).Radius(radiusPill).Background(t.Text.Alpha(0.05)).Shrink(0).Label("位置").Children(func() {
			part := func(p int, name, tip string, click func()) {
				on := p == now
				b := ui.Box(c.Key("place:"+name)).Height(30).Padding(0, 18).Radius(radiusPill).Center().Cursor(ui.CursorPointer).
					Label(name).Tooltip(tip).Background(t.Text.Alpha(0)).Transition(easeLook)
				if !on && b.Hovered() {
					b.Background(t.Text.Alpha(0.05))
				}
				if b.Clicked() {
					a.later(click)
				}
				press(b, 0.96)
				b.Children(func() {
					if on {
						slidePill(c, "places", radiusPill)
						b.ZIndex(-1) // under the others, which the pill slides behind
					}
					txt := ui.Text(c, name).FontSize(textBody).PassThrough()
					if on {
						txt.FontWeight(600)
					} else {
						txt.TextColor(t.TextMuted)
					}
				})
			}
			part(placeToday, "今天", "今天要做的，和刚记下的", func() {
				a.showLibrary()
				a.libHome = true
			})
			part(placeMaps, "项目", "导图：一张就是一个项目", a.showProjects)
			part(placeCollect, "收集", "灵感、链接、文档和图片", func() { a.openList(kindAll, "") })
		})
		ui.Row(c).Grow(1).Basis(0).Gap(2).AlignItems(ui.Center).Children(func() {
			ui.Spacer(c)
			if iconButton(c, iconSearch, "搜索全部", "搜索导图、主题、灵感和资料，#标签 按标签找", 32, true).Clicked() {
				a.later(func() { a.openPalette(paletteAll) })
			}
			if iconButton(c, iconGear, "设置", "设置", 32, true).Clicked() {
				a.later(a.openSettings)
			}
		})
	})
}

// projectsView is 项目: the maps as cards, those of the folder first, then
// each folder under its name; the folder they are all in, and how the
// sync goes, at the page's foot.
func (a *app) projectsView(c *ui.Context) {
	t := c.Theme()
	// What each map has to do, and how much of it is done.
	type count struct{ done, total int }
	counts := map[string]count{}
	for _, it := range a.taskItems() {
		n := counts[it.path]
		n.total++
		if it.done() {
			n.done++
		}
		counts[it.path] = n
	}
	w, _ := c.Size()
	cols := max(int((min(w, pageWidth+56)-56+16)/256), 1)

	card := func(f model.FileInfo) {
		path, col := f.Path, a.mapColor(f.Path)
		el := raise(ui.Column(c.Key("map:"+path)).Padding(16, 18, 18).Gap(12).Radius(radiusCard).MinWidth(0).Cursor(ui.CursorPointer).
			Label(f.Name).Drag(fileDrag{Path: path}), t)
		// Another map dropped on a map goes under its center.
		if d, ok := ui.Drop[fileDrag](el); ok && d.Path != path {
			a.later(func() { a.dropOnMap(path, d) })
		}
		if _, over := ui.DragOver[fileDrag](el); over {
			chosenCard(el, t)
		} else {
			hoverLift(el, t)
		}
		if el.Clicked() && a.renaming != path { // not on a press, which may start dragging it
			a.later(func() { _ = a.open(path) })
		}
		hover := el.Hovered()
		el.ContextMenu(func(m *ui.Menu) { a.mapMenu(m, path) })
		el.Children(func() {
			ui.Row(c).Height(36).Gap(6).AlignItems(ui.Center).FillWidth().Children(func() {
				bg, ink := tinted(t, col)
				ui.Box(c).Size(36, 36).Radius(radiusRow).Background(bg).Shrink(0).PassThrough().
					Draw(func(p *ui.Painter, r ui.Rect) { drawIcon(p, r, 18, iconMap, ink) })
				ui.Spacer(c)
				if a.starred(path) && !hover {
					ui.Text(c, "★").FontSize(textSmall).TextColor(ui.Hex("#D4A836")).PassThrough()
				}
				a.rowMore(c, hover, "导图菜单", func(m *ui.Menu) { a.mapMenu(m, path) })
			})
			if a.renaming == path {
				a.renameInput(c)
			} else {
				ui.Text(c, f.Name).FontSize(textTitle).FontWeight(600).SingleLine().FillWidth().PassThrough()
			}
			n := counts[path]
			ui.Box(c).Height(4).FillWidth().Radius(2).Background(t.Text.Alpha(0.07)).PassThrough().Children(func() {
				pct := float32(0)
				if n.total > 0 {
					pct = float32(n.done) / float32(n.total) * 100
				}
				ui.Box(c.Key("done")).Height(4).WidthPercent(pct).Radius(2).Background(col).
					Transition(ui.ElementTransition{Duration: 500 * time.Millisecond, Ease: easeDetent, Width: true})
			})
			sub := "还没有任务"
			if n.total > 0 {
				sub = fmt.Sprintf("%d / %d 件任务", n.done, n.total)
			}
			ui.Text(c, sub).FontSize(textSmall).TextColor(t.TextMuted).SingleLine().PassThrough()
		})
	}
	grid := func(key string, files []model.FileInfo) {
		files = slices.Clone(files) // those starred first, each in its folder
		slices.SortStableFunc(files, func(x, y model.FileInfo) int { return b2i(a.starred(y.Path)) - b2i(a.starred(x.Path)) })
		ui.Grid(c.Key(key)).Columns(cols).Gap(16).FillWidth().Children(func() {
			for _, f := range files {
				card(f)
			}
		})
	}
	// A folder: its name, which opens and closes it and takes a map
	// dropped on it, then its maps, then its folders, named by their way.
	var folder func(f model.Folder)
	folder = func(f model.Folder) {
		path, open := f.Path, a.folderOpen(f.Path)
		name := f.Name
		if rel, err := filepath.Rel(a.dir, path); err == nil {
			name = strings.ReplaceAll(rel, string(filepath.Separator), " / ")
		}
		ui.Column(c.Key("folder:" + path)).Gap(10).FillWidth().Children(func() {
			head := ui.Row(c.Key("head")).Height(32).Padding(0, 6, 0, 8).Gap(8).Radius(radiusRow).AlignItems(ui.Center).FillWidth().
				Cursor(ui.CursorPointer).Label(f.Name).Background(t.Text.Alpha(0)).Transition(easeLook)
			hover := head.Hovered()
			switch {
			case draggedOver(head):
				head.Background(t.Accent.Alpha(0.15))
			case hover:
				head.Background(t.Text.Alpha(0.05))
			}
			a.dropOnFolder(head, path)
			if head.Clicked() && a.renaming != path {
				a.setFolderOpen(path, !open)
			}
			if a.renaming == path {
				head.ScrollIntoView() // a folder just made shows, wherever it sorts
			}
			head.ContextMenu(func(m *ui.Menu) { a.folderMenu(m, path) })
			head.Children(func() {
				arrow := "▾"
				if !open {
					arrow = "▸"
				}
				ui.Text(c, arrow).FontSize(10).TextColor(t.TextMuted).Width(10).PassThrough()
				glyph(c, iconFolder, 15, t.TextMuted)
				if a.renaming == path {
					a.renameInput(c)
					return
				}
				ui.Text(c, name).FontSize(textSmall).FontWeight(600).TextColor(t.Text.MixOklab(t.TextMuted, 0.45)).SingleLine().PassThrough()
				ui.Text(c, fmt.Sprint(len(f.AllFiles()))).FontSize(textSmall).TextColor(t.TextMuted).PassThrough()
				ui.Spacer(c)
				a.rowMore(c, hover, "文件夹菜单", func(m *ui.Menu) { a.folderMenu(m, path) })
			})
			if open && len(f.Files) > 0 {
				grid("maps", f.Files)
			}
		})
		if open {
			for _, sub := range f.Folders {
				folder(sub)
			}
		}
	}

	ui.Column(c.Key("projects")).Grow(1).Label("项目").Children(func() {
		pageColumn(c, "projects-column", true).Children(func() {
			pageHeader(c, "projects-head", "项目", fmt.Sprintf("%d 张导图，一张就是一个项目", len(a.files)), func() {
				accentButton(c, "＋ 新建", "新建导图、文件夹，或导入").Label("新建").Menu(func(m *ui.Menu) {
					if m.Item("新建导图").Shortcut(ui.Cmd, ui.KeyN).Chosen() {
						a.later(a.newMap)
					}
					if m.Item("从模板新建…").Chosen() {
						a.templatesOpen = true
					}
					if m.Item("新建文件夹").Chosen() {
						a.later(func() { a.newFolder(a.dir) })
					}
					m.Separator()
					if m.Item("导入 XMind…").Chosen() {
						a.askImportXMind()
					}
					if m.Item("导入 Markdown / OPML…").Chosen() {
						a.askImportOutline()
					}
				})
			})
			ui.Scroll(c.Key("projects-scroll")).Grow(1).Margin(0, -12).Children(func() {
				ui.Column(c.Key("projects-list")).Padding(6, 12, 48).Gap(22).FillWidth().Children(func() {
					if len(a.tree.Files) == 0 && len(a.tree.Folders) == 0 {
						raise(ui.Column(c.Key("none")).Padding(28).Gap(12).Radius(radiusCard).AlignItems(ui.Center).FillWidth(), t).Children(func() {
							ui.Text(c, "还没有导图").FontSize(textTitle).TextColor(t.TextMuted)
							if inkButton(c, "新建导图", true).Clicked() {
								a.later(a.newMap)
							}
						})
						a.removeMapMeta(a.tree.Path)
					}
					if len(a.tree.Files) > 0 {
						grid("root-maps", a.tree.Files)
					}
					for _, sub := range a.tree.Folders {
						folder(sub)
					}
				})
			})
			// Where the maps are kept, and how the sync goes.
			ui.Row(c.Key("projects-foot")).Height(40).Gap(8).AlignItems(ui.Center).Shrink(0).FillWidth().Children(func() {
				where := ui.Row(c.Key("folder-switch")).Height(30).Padding(0, 10).Gap(8).Radius(radiusPill).AlignItems(ui.Center).Shrink(1).MinWidth(0).
					Cursor(ui.CursorPointer).Label("切换文件夹").Tooltip("导图所在的文件夹：" + a.dir).Background(t.Text.Alpha(0)).Transition(easeLook)
				if where.Hovered() {
					where.Background(t.Text.Alpha(0.06))
				}
				where.Menu(func(m *ui.Menu) {
					if m.Item("打开其他文件夹…").Shortcut(ui.Cmd, ui.KeyO).Chosen() {
						a.pickWorkspace()
					}
					if m.Item("在 Finder 中显示").Chosen() {
						a.plat.ShowInFolder(a.dir)
					}
				})
				where.Children(func() {
					glyph(c, iconFolder, 15, t.TextMuted)
					ui.Text(c, filepath.Base(a.dir)).FontSize(textSmall).TextColor(t.TextMuted).SingleLine().Shrink(1).MinWidth(0).PassThrough()
				})
				ui.Spacer(c)
				ui.Box(c.Key("sync")).Shrink(0).Children(func() { a.syncFoot(c) })
			})
		})
	})
}

// deleteConfirm asks before a map or a folder goes to the trash.
func (a *app) deleteConfirm(c *ui.Context) {
	title, says := "把「"+mapName(a.deletePath)+"」移到废纸篓？", "可以在废纸篓中找回。"
	folder := a.deletePath != "" && !strings.HasSuffix(a.deletePath, model.Ext)
	if folder {
		title = "把文件夹「" + filepath.Base(a.deletePath) + "」移到废纸篓？"
		if n := len(a.mapsIn(a.deletePath)); n > 0 {
			says = fmt.Sprintf("里面的 %d 张导图会一起移走，可以在废纸篓中找回。", n)
		}
	}
	if ui.AlertDialog(c, &a.confirmDelete, title, says, "取消", "移到废纸篓") == 1 {
		if folder {
			a.deleteFolder(a.deletePath)
		} else {
			a.deleteMap(a.deletePath)
		}
	}
}
