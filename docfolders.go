package main

import (
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
