package main

import (
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
)

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

// sideKeys are the list's keys, as Finder's, while it has the focus (a
// click in it gives it): the arrows go to the map above or below, Return
// renames it, Delete moves it to the trash. They act on the list as drawn
// the frame before.
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
