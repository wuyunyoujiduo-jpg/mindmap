package main

import (
	"fmt"
	"path/filepath"
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// treeWidth is the width of the maps' directory.
const treeWidth = 236

// treeShown tells whether the maps' directory is beside the page: in 项目
// only, by its cards and by a map opened, unless put away. 今天 and 收集
// have the window's whole width.
func (a *app) treeShown() bool { return !a.cfg.HideTree && a.place() == placeMaps }

// mapTree is the maps' directory, at the left in 项目: the folders, which
// open and close, and their maps, those starred first. A click on a map
// opens it, so that going from one map to another is one click; a click
// on a folder shows its maps as cards, on the page of cards. At its foot,
// the folder the maps are all kept in and how the sync goes.
func (a *app) mapTree(c *ui.Context) {
	t := c.Theme()
	bar := c.TitleBar()
	side := ui.Column(c.Key("map-tree")).Width(treeWidth).Shrink(0).FillHeight().Background(ground(t))
	side.Children(func() {
		head := float32(toolbarHeight)
		if !a.libOpen { // by a map it is under the window's controls
			head = max(toolbarHeight, bar.Height+8)
		}
		ui.Row(c.Key("tree-head")).Height(head).Padding(0, 8, 0, max(16, b2f(!a.libOpen)*bar.Left)).Gap(2).AlignItems(ui.Center).Shrink(0).DragWindow().Children(func() {
			if a.libOpen {
				ui.Text(c, "目录").FontSize(textSmall).FontWeight(600).TextColor(t.TextMuted).Grow(1).PassThrough()
			} else { // by a map: the way back to the cards
				if textKey(c, "‹ 项目", "回到项目", "回到项目：所有导图的卡片").Margin(0, 0, 0, -8).Clicked() {
					a.later(a.showProjects)
				}
				ui.Spacer(c)
			}
			iconButton(c, iconPlus, "目录新建", "新建导图、文件夹", 28, true).Menu(func(m *ui.Menu) {
				if m.Item("新建导图").Chosen() {
					a.later(func() { a.newMapIn(a.treeFolder()) })
				}
				if m.Item("新建文件夹").Chosen() {
					a.later(func() { a.newFolder(a.treeFolder()) })
				}
			})
			if iconButton(c, iconSidebar, "收起目录", "收起目录", 28, true).Clicked() {
				a.cfg.HideTree = true
				a.cfg.save()
			}
		})
		a.sideDrawn, a.sideItems = a.sideItems, nil // the keys go by the list drawn
		ui.Scroll(c.Key("tree-scroll")).Grow(1).Children(func() {
			// The keys on a column in the scroll, which takes the arrows first.
			list := ui.Column(c.Key("tree-list")).Padding(0, 8, 12).Gap(1).FillWidth().Label("目录").FocusBind(&a.sideFocus, "side")
			a.sideKeys(list)
			list.Children(func() {
				if a.libOpen { // every map, as the cards' 全部
					a.treeRow(c.Key("all"), 0, "全部导图", fmt.Sprint(len(a.files)), "", a.projFolder == "", iconLevels, t.TextMuted, func() { a.projFolder = "" }, nil)
				}
				a.treeFolderRows(c, a.tree, 0)
			})
		})
		ui.Column(c.Key("tree-foot")).Padding(6, 8, 8).Gap(2).Shrink(0).Children(func() {
			where := ui.Row(c.Key("folder-switch")).Height(30).Padding(0, 8).Gap(8).Radius(radiusSmall).AlignItems(ui.Center).FillWidth().
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
				ui.Text(c, filepath.Base(a.dir)).FontSize(textSmall).TextColor(t.TextMuted).SingleLine().Grow(1).Shrink(1).MinWidth(0).PassThrough()
			})
			a.syncFoot(c)
		})
	})
}

// treeFolder is the folder new things of the directory go into: the one
// whose cards show, or the open map's.
func (a *app) treeFolder() string {
	switch {
	case a.libOpen && a.projFolder != "":
		return a.projFolder
	case a.libOpen:
		return a.dir
	}
	return a.currentFolder()
}

// treeRow is a row of the directory: indented by its depth, an icon, a
// name, a count at its end, lit when it is the one shown.
func (a *app) treeRow(c *ui.Context, depth int, name, count, path string, on bool, ic icon, col ui.Color, click func(), menu func(m *ui.Menu)) ui.Element {
	t := c.Theme()
	row := ui.Row(c).Height(32).Padding(0, 4, 0, 8+float32(depth)*14).Gap(8).Radius(radiusSmall).AlignItems(ui.Center).FillWidth().
		Cursor(ui.CursorPointer).Label(name).Background(t.Text.Alpha(0)).Transition(easeLook)
	hover := row.Hovered()
	switch {
	case on:
		row.Background(chosen(t))
	case hover:
		row.Background(t.Text.Alpha(0.05))
	}
	if row.Clicked() && (path == "" || a.renaming != path) { // not on a press, which may start dragging it
		a.later(click)
	}
	if menu != nil {
		row.ContextMenu(menu)
	}
	row.Children(func() {
		sideTile(c, ic, col)
		if path != "" && a.renaming == path {
			a.renameInput(c)
			return
		}
		txt := ui.Text(c, midEllipsis(name, treeWidth-96-float32(depth)*14)).FontSize(textSmall + 0.5).Grow(1).Shrink(1).MinWidth(0).SingleLine().PassThrough()
		if on {
			txt.FontWeight(600)
		}
		if path != "" && a.starred(path) && !hover {
			ui.Text(c, "★").FontSize(11).TextColor(ui.Hex("#D4A836")).Margin(0, 6, 0, 0).PassThrough()
		}
		if count != "" && !(hover && menu != nil) {
			ui.Text(c, count).FontSize(textCaption).TextColor(t.TextMuted).Margin(0, 6, 0, 0).PassThrough()
		}
		if menu != nil {
			a.rowMore(c, hover, "目录菜单", menu)
		}
	})
	return row
}

// treeFolderRows shows a folder's folders, each with what it holds when
// open, then its maps, those starred first.
func (a *app) treeFolderRows(c *ui.Context, f model.Folder, depth int) {
	t := c.Theme()
	for _, sub := range f.Folders {
		path, open := sub.Path, a.folderOpen(sub.Path)
		ic := iconFolder
		row := a.treeRow(c.Key("folder:"+path), depth, sub.Name, fmt.Sprint(len(sub.AllFiles())), path, a.libOpen && a.projFolder == path, ic, t.TextMuted,
			func() {
				if a.libOpen && a.projFolder != path { // its cards, and what it holds
					a.projFolder = path
					a.setFolderOpen(path, true)
					return
				}
				a.setFolderOpen(path, !open)
			}, func(m *ui.Menu) { a.folderMenu(m, path) })
		if draggedOver(row) {
			row.Background(t.Accent.Alpha(0.15))
		}
		a.dropOnFolder(row, path)
		if a.renaming == path {
			row.ScrollIntoView() // a folder just made shows, wherever it sorts
		}
		if open {
			a.treeFolderRows(c, sub, depth+1)
		}
	}
	files := slices.Clone(f.Files)
	slices.SortStableFunc(files, func(x, y model.FileInfo) int { return b2i(a.starred(y.Path)) - b2i(a.starred(x.Path)) })
	for _, file := range files {
		path := file.Path
		a.sideItems = append(a.sideItems, path)
		row := a.treeRow(c.Key("file:"+path), depth, file.Name, "", path, path == a.path && !a.libOpen, iconMap, a.mapColor(path),
			func() {
				_ = a.open(path)
				a.sideFocus = "side" // its keys go to the directory
			}, func(m *ui.Menu) { a.mapMenu(m, path) })
		row.Drag(fileDrag{Path: path})
		// Another map dropped on a map goes under its center.
		if d, ok := ui.Drop[fileDrag](row); ok && d.Path != path {
			a.later(func() { a.dropOnMap(path, d) })
		}
		if _, over := ui.DragOver[fileDrag](row); over {
			row.Background(t.Accent.Alpha(0.15))
		}
	}
}

// sideKeys are the directory's keys, as Finder's, while it has the focus
// (a click in it gives it): the arrows go to the map above or below,
// Return renames it, Delete moves it to the trash. They act on the list as
// drawn the frame before.
func (a *app) sideKeys(list ui.Element) {
	items := a.sideDrawn
	at := slices.Index(items, a.path)
	open := func(i int) {
		if i < 0 || i >= len(items) {
			return
		}
		path := items[i]
		a.later(func() { _ = a.open(path) })
	}
	switch {
	case a.libOpen: // by the cards, the keys are the page's
	case list.Shortcut(0, ui.KeyDown):
		open(at + 1)
	case list.Shortcut(0, ui.KeyUp):
		if at < 0 {
			at = len(items)
		}
		open(at - 1)
	case at < 0:
	case list.Shortcut(0, ui.KeyEnter):
		a.renaming, a.renameText = items[at], mapName(items[at])
	case list.Shortcut(0, ui.KeyBackspace), list.Shortcut(ui.Cmd, ui.KeyBackspace), list.Shortcut(0, ui.KeyDelete):
		a.deletePath, a.confirmDelete = items[at], true
	}
}
