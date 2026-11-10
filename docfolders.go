package main

import (
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
)

// What is collected (the clips of the library) is 收集's, not the maps'
// list's: there it shows only where it was put at hand, in 快捷访问. A map
// has it by a topic's link (refs.go).

// docRow is a clip in 快捷访问 of the sidebar: a click shows it in the
// list of 资料.
func (a *app) docRow(c *ui.Context, clip *library.Clip, depth int) {
	t := c.Theme()
	current := a.libOpen && a.libSel == clip.ID
	a.sideItems = append(a.sideItems, sideItem{clip: clip.ID})
	row := ui.Row(c).Padding(5, 8, 5, 22+float32(depth)*14).Gap(6).Radius(6).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Label("文档 " + clip.Label()).Drag(fileDrag{Clip: clip.ID}).FillWidth().MinWidth(0).
		Tooltip(clip.Label())
	switch {
	case current:
		row.Background(chosen(t))
	case row.Hovered():
		row.Background(t.Text.Alpha(0.06))
	}
	id := clip.ID
	if row.Clicked() {
		a.later(func() {
			a.showLibrary()
			a.libSel, a.libKind, a.libCat, a.libTag, a.libQuery = id, kindAll, "", "", ""
			a.sideFocus = "side"
		})
	}
	row.ContextMenu(func(m *ui.Menu) { a.clipMenu(m, clip) })
	row.Children(func() {
		sideTile(c, kindIcon(clip.Kind), clipColor(clip.Kind))
		ui.Text(c, midEllipsis(clip.Label(), a.sidebarWidth-70-float32(depth)*14)).FontSize(13).Grow(1).Shrink(1).MinWidth(0).SingleLine()
	})
}

// dropOnFolder takes a map dropped on a row into the folder dir.
func (a *app) dropOnFolder(row ui.Element, dir string) {
	if d, ok := ui.Drop[fileDrag](row); ok && d.Path != "" {
		a.later(func() { a.moveMap(d.Path, dir) })
	}
}

// draggedOver tells whether a map is dragged over a row.
func draggedOver(row ui.Element) bool {
	d, over := ui.DragOver[fileDrag](row)
	return over && d.Path != ""
}

// The kinds the list of 资料 shows (app.libKind).
const (
	kindAll  = iota
	kindMaps // the maps
	kindDocs // text and files
	kindLinks
	kindImages
	kindMemos // the notes of 灵感
)

// ofKind tells whether a clip, or a map as one, is of a kind of the list.
func ofKind(c *library.Clip, kind int) bool {
	switch kind {
	case kindMaps:
		return c.Kind == kindMap
	case kindMemos:
		return c.Kind == library.KindText
	case kindDocs:
		return c.Kind != kindMap && c.Kind != library.KindLink && c.Kind != library.KindImage && c.Kind != library.KindText
	case kindImages:
		return c.Kind == library.KindImage
	case kindLinks:
		return c.Kind == library.KindLink
	}
	return true
}

// sideItem is a map or a document of the sidebar's tree, in its order,
// for the keys.
type sideItem struct {
	path string // a map's
	clip string // or a clip's ID
}

// sideKeys are the sidebar's keys, as Finder's, while it has the focus
// (a click in it gives it): the arrows go to the map or document above or
// below, Return renames a map, Delete moves it to the trash or deletes a
// document. They act on the list as drawn the frame before.
func (a *app) sideKeys(list ui.Element) {
	items := a.sideDrawn
	at := slices.IndexFunc(items, func(it sideItem) bool {
		if a.libOpen {
			return it.clip != "" && it.clip == a.libSel
		}
		return it.path != "" && it.path == a.path
	})
	open := func(i int) {
		if i < 0 || i >= len(items) {
			return
		}
		it := items[i]
		a.later(func() {
			if it.clip != "" {
				a.showLibrary()
				a.libSel, a.libKind, a.libCat, a.libTag, a.libQuery = it.clip, kindAll, "", "", ""
				return
			}
			a.libOpen = false
			_ = a.open(it.path)
		})
	}
	switch {
	case list.Shortcut(0, ui.KeyDown):
		open(at + 1)
	case list.Shortcut(0, ui.KeyUp):
		if at < 0 {
			at = len(items)
		}
		open(at - 1)
	case at < 0:
	case list.Shortcut(0, ui.KeyEnter):
		if p := items[at].path; p != "" {
			a.renaming, a.renameText = p, mapName(p)
		}
	case list.Shortcut(0, ui.KeyBackspace), list.Shortcut(ui.Cmd, ui.KeyBackspace), list.Shortcut(0, ui.KeyDelete):
		if p := items[at].path; p != "" {
			a.deletePath, a.confirmDelete = p, true
		} else {
			a.libSel, a.libConfirm = items[at].clip, true
		}
	}
}
