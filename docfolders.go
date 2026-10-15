package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// Documents (the clips of the library) can be kept in the folders of the
// maps, beside the maps: the sidebar shows them there. A clip says its
// folder (Clip.Folder); its files stay in the library.

// folderKey is a folder's path as a clip keeps it.
func (a *app) folderKey(dir string) string {
	rel, err := filepath.Rel(a.dir, dir)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(rel)
}

// clipsIn are the clips kept in a folder.
func (a *app) clipsIn(dir string) []*library.Clip {
	key := a.folderKey(dir)
	var clips []*library.Clip
	for _, c := range a.lib.Clips {
		if c.Folder != "" && c.Folder == key {
			clips = append(clips, c)
		}
	}
	return clips
}

// moveClipTo keeps a clip in a folder, "" taking it out of any.
func (a *app) moveClipTo(id, dir string) {
	c := a.lib.Find(id)
	if c == nil {
		return
	}
	key := ""
	if dir != "" {
		key = a.folderKey(dir)
	}
	if c.Folder == key {
		return
	}
	c.Folder = key
	a.saveClip(c)
	if dir != "" {
		a.setFolderOpen(dir, true)
		a.tell("已放进「" + filepath.Base(dir) + "」")
	}
}

// moveClipFolders has the clips follow their folders moved: follow gives
// a path's new place.
func (a *app) moveClipFolders(follow func(string) string) {
	for _, c := range a.lib.Clips {
		if c.Folder == "" {
			continue
		}
		from := filepath.Join(a.dir, filepath.FromSlash(c.Folder))
		if to := follow(from); to != from {
			c.Folder = a.folderKey(to)
			a.saveClip(c)
		}
	}
}

// folderMenu lists the folders to move a clip to.
func (a *app) clipFolderMenu(m *ui.Menu, clip *library.Clip) {
	id := clip.ID
	m.Submenu("放到文件夹", func(m *ui.Menu) {
		for _, dir := range a.tree.FolderPaths() {
			name := filepath.Base(dir)
			if dir != a.dir {
				rel, _ := filepath.Rel(a.dir, dir)
				name = strings.ReplaceAll(rel, string(filepath.Separator), " / ")
			}
			if m.Item(name).Checked(clip.Folder == a.folderKey(dir)).Chosen() {
				to := dir
				a.later(func() { a.moveClipTo(id, to) })
			}
		}
		if clip.Folder != "" {
			m.Separator()
			if m.Item("移出文件夹").Chosen() {
				a.later(func() { a.moveClipTo(id, "") })
			}
		}
	})
}

// docRow is a clip in a folder of the sidebar: a click shows it in the
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
		row.Background(t.Accent.Alpha(0.16))
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
		sideIcon(c, kindIcon(clip.Kind), current)
		ui.Text(c, midEllipsis(clip.Label(), a.sidebarWidth-70-float32(depth)*14)).FontSize(13).Grow(1).Shrink(1).MinWidth(0).SingleLine()
	})
}

// dropOnFolder takes maps and clips dropped on a row into the folder dir.
func (a *app) dropOnFolder(row ui.Element, dir string) {
	if d, ok := ui.Drop[fileDrag](row); ok {
		a.later(func() {
			if d.Clip != "" {
				a.moveClipTo(d.Clip, dir)
			} else {
				a.moveMap(d.Path, dir)
			}
		})
	}
}

// draggedOver tells whether a map or a clip is dragged over a row.
func draggedOver(row ui.Element) bool {
	_, over := ui.DragOver[fileDrag](row)
	return over
}

// The kinds the list of 资料 shows (app.libKind).
const (
	kindAll  = iota
	kindMaps // the maps
	kindDocs // text, pictures and files
	kindLinks
)

// ofKind tells whether a clip, or a map as one, is of a kind of the list.
func ofKind(c *library.Clip, kind int) bool {
	switch kind {
	case kindMaps:
		return c.Kind == kindMap
	case kindDocs:
		return c.Kind != kindMap && c.Kind != library.KindLink
	case kindLinks:
		return c.Kind == library.KindLink
	}
	return true
}

// docCount is how many maps and clips a folder and those in it hold.
func (a *app) docCount(f model.Folder) int {
	n := len(f.Files) + len(a.clipsIn(f.Path))
	for _, sub := range f.Folders {
		n += a.docCount(sub)
	}
	return n
}

// unfiledClips are the documents not put anywhere yet: in no folder (never
// put in one, or in one no more there), and linked from no map.
func (a *app) unfiledClips() []*library.Clip {
	var out []*library.Clip
	refs := a.refIndex()
	for _, c := range a.lib.Clips {
		if len(refs.to[clipScheme+c.ID]) > 0 {
			continue // placed in a map
		}
		if c.Folder == "" {
			out = append(out, c)
			continue
		}
		if _, err := os.Stat(filepath.Join(a.dir, filepath.FromSlash(c.Folder))); err != nil {
			out = append(out, c)
		}
	}
	return out
}

// unfiledMax is how many of 未归档's documents the sidebar lists.
const unfiledMax = 50

// unfiledRow is the folder of the sidebar, always first, of the documents
// in no folder; a document dropped on it leaves its folder.
func (a *app) unfiledRow(c *ui.Context) {
	t := c.Theme()
	docs := a.unfiledClips()
	if len(docs) == 0 {
		return // nothing to file: no row
	}
	open := a.unfiledOpen // closed at first: a count, not a long list
	row := ui.Row(c).Padding(5, 8).Gap(6).Radius(6).AlignItems(ui.Center).Cursor(ui.CursorPointer).
		Label("未归档").Tooltip("还没放进文件夹、也没挂到导图上的文档；拖到文件夹或导图的节点上来归档")
	switch {
	case draggedOver(row):
		row.Background(t.Accent.Alpha(0.15))
	case row.Hovered():
		row.Background(t.Text.Alpha(0.06))
	}
	if d, ok := ui.Drop[fileDrag](row); ok {
		a.later(func() {
			if d.Clip != "" {
				a.moveClipTo(d.Clip, "")
			} else {
				a.moveMap(d.Path, a.dir) // a map: to the maps folder itself
			}
		})
	}
	if row.Clicked() {
		a.unfiledOpen = !open
	}
	row.Children(func() {
		arrow := "▾"
		if !open {
			arrow = "▸"
		}
		ui.Text(c, arrow).FontSize(10).TextColor(t.TextMuted).Width(10)
		sideIcon(c, iconInbox, false)
		ui.Text(c, "未归档").FontSize(13).Grow(1).NoWrap()
		if n := len(docs); n > 0 {
			ui.Text(c, fmt.Sprint(n)).FontSize(11).TextColor(t.TextMuted)
		}
	})
	if !open {
		return
	}
	for i, clip := range docs {
		if i == unfiledMax {
			if ui.Link(c, fmt.Sprintf("查看全部 %d 条…", len(docs)), "").FontSize(12).Margin(2, 0, 4, 36).Clicked() {
				a.later(func() {
					a.showLibrary()
					a.libKind, a.libCat, a.libTag = kindAll, "", ""
				})
			}
			break
		}
		a.docRow(c.Key("unfiled:"+clip.ID), clip, 1)
	}
}

var _ = fmt.Sprint

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
