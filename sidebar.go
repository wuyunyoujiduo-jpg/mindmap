package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// fileDrag is the value a map's row carries while dragged onto a folder.
type fileDrag struct{ Path string }

// sidebar is the left side: the starred maps, the maps of the folder and
// its folders, and the tags of the open map.
func (a *app) sidebar(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Surface).Children(func() {
		ui.Row(c).Padding(10, 12, 6).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, filepath.Base(a.dir)).Bold().Grow(1).NoWrap().Ellipsis("…").Tooltip(a.dir)
			if ui.Button(c, "打开…").Tooltip("选择存放导图的文件夹").Clicked() {
				a.pickWorkspace()
			}
		})
		ui.Row(c).Padding(0, 12, 6).Gap(6).Children(func() {
			if ui.Button(c, "+ 导图").Tooltip("在当前文件夹新建导图").Grow(1).Clicked() {
				a.newMap()
			}
			if ui.Button(c, "+ 文件夹").Tooltip("在当前文件夹新建文件夹").Grow(1).Clicked() {
				a.newFolder(a.currentFolder())
			}
		})
		ui.Scroll(c.Key("sidebar")).Grow(1).Padding(0, 6, 12).Gap(2).Children(func() {
			a.favorites(c)
			a.section(c, "导图", &a.filesOpen, a.dir, func() {
				a.folderContents(c, a.tree, 0)
			})
			if a.file != nil {
				a.tagList(c)
			}
		})
	})
	switch ui.AlertDialog(c, &a.confirmDelete, "把「"+mapName(a.deletePath)+"」移到废纸篓？", "可以在废纸篓中找回。", "取消", "移到废纸篓") {
	case 1:
		a.deleteMap(a.deletePath)
	}
}

func (a *app) pickWorkspace() {
	a.plat.PickFolder(func(dir string) {
		if dir != "" {
			a.flush()
			a.openDir(dir)
			a.cfg.save()
			if len(a.files) > 0 {
				_ = a.open(a.files[0].Path)
			}
		}
	})
}

// section is a heading that shows and hides what fn builds; maps dropped on
// it move into dropDir, when it is not "".
func (a *app) section(c *ui.Context, title string, open *bool, dropDir string, fn func()) {
	t := c.Theme()
	head := ui.Row(c.Key("section:"+title)).Padding(8, 8, 4).Gap(4).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(title)
	if dropDir != "" {
		if _, over := ui.DragOver[fileDrag](head); over {
			head.Background(t.Accent.Alpha(0.15)).Radius(6)
		}
		if d, ok := ui.Drop[fileDrag](head); ok {
			a.later(func() { a.moveMap(d.Path, dropDir) })
		}
	}
	if head.Clicked() {
		*open = !*open
	}
	head.Children(func() {
		arrow := "▾"
		if !*open {
			arrow = "▸"
		}
		ui.Text(c, arrow).FontSize(10).TextColor(t.TextMuted).Width(10)
		ui.Text(c, title).FontSize(12).Bold().TextColor(t.TextMuted)
	})
	if *open {
		fn()
	}
}

// favorites is the section of starred maps, which stays at the top.
func (a *app) favorites(c *ui.Context) {
	var favs []model.FileInfo
	for _, p := range a.cfg.Favorites {
		if i := slices.IndexFunc(a.files, func(f model.FileInfo) bool { return f.Path == p }); i >= 0 {
			favs = append(favs, a.files[i])
		}
	}
	if len(favs) == 0 {
		return
	}
	a.section(c, "★ 收藏", &a.favoritesOpen, "", func() {
		for _, f := range favs {
			a.fileRow(c.Key("fav:"+f.Path), f, 0, true)
		}
	})
}

// folderContents shows a folder's folders, then its maps, indented by
// depth.
func (a *app) folderContents(c *ui.Context, f model.Folder, depth int) {
	for _, sub := range f.Folders {
		a.folderRow(c.Key("folder:"+sub.Path), sub, depth)
	}
	for _, file := range f.Files {
		a.fileRow(c.Key("file:"+file.Path), file, depth, false)
	}
	if depth == 0 && len(f.Files) == 0 && len(f.Folders) == 0 {
		ui.Text(c, "还没有导图").FontSize(12).TextColor(c.Theme().TextMuted).Margin(4, 12)
	}
}

func (a *app) folderRow(c *ui.Context, f model.Folder, depth int) {
	t := c.Theme()
	open := !a.folderClosed[f.Path]
	row := ui.Row(c).Padding(5, 8, 5, 8+float32(depth)*14).Gap(6).Radius(6).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Label(f.Name)
	if row.Hovered() {
		row.Background(t.SurfaceHover)
	}
	if _, over := ui.DragOver[fileDrag](row); over {
		row.Background(t.Accent.Alpha(0.15))
	}
	if d, ok := ui.Drop[fileDrag](row); ok {
		dir := f.Path
		a.later(func() { a.moveMap(d.Path, dir) })
	}
	if row.Clicked() {
		a.folderClosed[f.Path] = open
	}
	path := f.Path
	row.ContextMenu(func(m *ui.Menu) {
		if m.Item("在此新建导图").Chosen() {
			a.later(func() { a.newMapIn(path) })
		}
		if m.Item("新建子文件夹").Chosen() {
			a.later(func() { a.newFolder(path) })
		}
		if m.Item("重命名").Chosen() {
			a.renaming, a.renameText = path, filepath.Base(path)
		}
		m.Separator()
		if m.Item("在 Finder 中显示").Chosen() {
			a.plat.ShowInFolder(path)
		}
	})
	row.Children(func() {
		arrow := "▾"
		if !open {
			arrow = "▸"
		}
		ui.Text(c, arrow).FontSize(10).TextColor(t.TextMuted).Width(10)
		ui.Text(c, "📁").FontSize(12)
		if a.renaming == path {
			a.renameInput(c)
			return
		}
		ui.Text(c, f.Name).FontSize(13).Grow(1).NoWrap().Ellipsis("…")
		if n := len(f.AllFiles()); n > 0 {
			ui.Text(c, fmt.Sprint(n)).FontSize(11).TextColor(t.TextMuted)
		}
	})
	if open {
		a.folderContents(c, f, depth+1)
	}
}

func (a *app) fileRow(c *ui.Context, f model.FileInfo, depth int, inFavorites bool) {
	t := c.Theme()
	current := f.Path == a.path
	fav := slices.Contains(a.cfg.Favorites, f.Path)
	row := ui.Row(c).Padding(5, 8, 5, 26+float32(depth)*14).Gap(6).Radius(6).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Label(f.Name).Drag(fileDrag{f.Path})
	switch {
	case current:
		row.Background(t.Accent.Alpha(0.16))
	case row.Hovered():
		row.Background(t.SurfaceHover)
	}
	if row.Pressed() && !current && a.renaming != f.Path {
		path := f.Path
		a.later(func() { _ = a.open(path) })
	}
	path := f.Path
	row.ContextMenu(func(m *ui.Menu) {
		label := "收藏"
		if fav {
			label = "取消收藏"
		}
		if m.Item(label).Chosen() {
			a.toggleFavorite(path)
		}
		if m.Item("重命名").Chosen() {
			a.renaming, a.renameText = path, mapName(path)
		}
		m.Submenu("移动到", func(m *ui.Menu) {
			for _, dir := range a.tree.FolderPaths() {
				name := filepath.Base(dir)
				if dir != a.dir {
					rel, _ := filepath.Rel(a.dir, dir)
					name = strings.ReplaceAll(rel, string(filepath.Separator), " / ")
				}
				if m.Item(name).Disabled(dir == filepath.Dir(path)).Chosen() {
					to := dir
					a.later(func() { a.moveMap(path, to) })
				}
			}
		})
		m.Separator()
		if m.Item("在 Finder 中显示").Chosen() {
			a.plat.ShowInFolder(path)
		}
		if m.Item("移到废纸篓…").Chosen() {
			a.deletePath, a.confirmDelete = path, true
		}
	})
	row.Children(func() {
		if a.renaming == path && !inFavorites {
			a.renameInput(c)
			return
		}
		text := ui.Text(c, f.Name).FontSize(13).Grow(1).NoWrap().Ellipsis("…")
		if current {
			text.Bold()
		}
		if fav && !inFavorites {
			ui.Text(c, "★").FontSize(11).TextColor(ui.Hex("#F2C200"))
		}
	})
}

// renameInput edits the name of the map or folder being renamed.
func (a *app) renameInput(c *ui.Context) {
	in := ui.TextInputBase(c.Key("rename"), &a.renameText).FontSize(13).Grow(1).AutoFocus()
	switch {
	case in.Shortcut(0, ui.KeyEscape):
		a.renaming = ""
	case in.Submitted(), in.PressedOutside():
		path, name := a.renaming, a.renameText
		a.renaming = ""
		a.later(func() { a.rename(path, name) })
	}
}

// rename renames a map file, or a folder, keeping what refers to it.
func (a *app) rename(path, name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	to := filepath.Join(filepath.Dir(path), name)
	if strings.HasSuffix(path, model.Ext) {
		to = filepath.Join(filepath.Dir(path), model.FileName(name))
	}
	if to == path {
		return
	}
	if _, err := os.Stat(to); err == nil {
		a.saveErr = "已有同名的「" + name + "」"
		return
	}
	a.flush()
	if err := os.Rename(path, to); err != nil {
		a.saveErr = err.Error()
		return
	}
	a.moved(path, to)
}

// moveMap moves a map file into the folder dir.
func (a *app) moveMap(path, dir string) {
	if filepath.Dir(path) == dir {
		return
	}
	if path == a.path {
		a.flush()
	}
	to, err := model.MoveFile(path, dir)
	if err != nil {
		a.saveErr = err.Error()
		return
	}
	a.folderClosed[dir] = false
	a.moved(path, to)
}

// moved follows a file or folder from one path to another: the open map,
// the favorites and the config.
func (a *app) moved(from, to string) {
	follow := func(p string) string {
		if p == from {
			return to
		}
		if rel, err := filepath.Rel(from, p); err == nil && inside(p, from) {
			return filepath.Join(to, rel)
		}
		return p
	}
	a.path = follow(a.path)
	if a.path != "" {
		a.modTime = modTime(a.path)
	}
	for i, p := range a.cfg.Favorites {
		a.cfg.Favorites[i] = follow(p)
	}
	a.cfg.File = a.path
	a.cfg.save()
	a.refreshFiles()
}

// newFolder creates a folder in dir and starts renaming it.
func (a *app) newFolder(dir string) {
	name := "新建文件夹"
	for i := 2; ; i++ {
		if _, err := os.Stat(filepath.Join(dir, name)); errors.Is(err, os.ErrNotExist) {
			break
		}
		name = fmt.Sprintf("新建文件夹 %d", i)
	}
	path := filepath.Join(dir, name)
	if err := os.Mkdir(path, 0o755); err != nil {
		a.saveErr = err.Error()
		return
	}
	a.folderClosed[dir] = false
	a.refreshFiles()
	a.renaming, a.renameText = path, name
}

func (a *app) toggleFavorite(path string) {
	if i := slices.Index(a.cfg.Favorites, path); i >= 0 {
		a.cfg.Favorites = slices.Delete(a.cfg.Favorites, i, i+1)
	} else {
		a.cfg.Favorites = append(a.cfg.Favorites, path)
		a.favoritesOpen = true
	}
	a.cfg.save()
}

// tagList is the section of the open map's tags, a row each with how many
// nodes carry it; choosing one highlights its nodes and filters the tasks.
// Past a few tags, a field filters the list.
func (a *app) tagList(c *ui.Context) {
	t := c.Theme()
	tags := a.file.AllTags()
	counts := map[string]int{}
	a.file.Walk(func(n *model.Node) {
		for _, tag := range n.Tags {
			counts[tag]++
		}
	})
	title := "标签"
	if len(tags) > 0 {
		title = fmt.Sprintf("标签 %d", len(tags))
	}
	a.section(c, title, &a.tagsOpen, "", func() {
		if len(tags) == 0 {
			ui.Text(c, "在节点详情中添加标签").FontSize(12).TextColor(t.TextMuted).Margin(2, 12)
			return
		}
		if len(tags) > 8 {
			ui.TextInput(c.Key("tag-query"), &a.tagQuery).Placeholder("筛选标签").Margin(2, 6, 4)
		}
		if a.tagFilter != "" {
			if ui.Link(c, "清除筛选：#"+a.tagFilter, "").FontSize(12).Margin(0, 12, 4).Clicked() {
				a.tagFilter = ""
			}
		}
		for _, tag := range filterTags(tags, a.tagQuery) {
			active := a.tagFilter == tag
			row := ui.Row(c.Key("tag:"+tag)).Padding(4, 8, 4, 12).Gap(8).Radius(6).AlignItems(ui.Center).
				Cursor(ui.CursorPointer).Label(tag)
			switch {
			case active:
				row.Background(tagColor(tag).Alpha(0.18))
			case row.Hovered():
				row.Background(t.SurfaceHover)
			}
			if row.Clicked() {
				if active {
					a.tagFilter = ""
				} else {
					a.tagFilter = tag
				}
			}
			row.Children(func() {
				ui.Box(c).Size(8, 8).Radius(999).Background(tagColor(tag))
				text := ui.Text(c, tag).FontSize(13).Grow(1).NoWrap().Ellipsis("…")
				if active {
					text.Bold()
				}
				ui.Text(c, fmt.Sprint(counts[tag])).FontSize(11).TextColor(t.TextMuted)
			})
		}
	})
}

// filterTags returns the tags containing query, ignoring case.
func filterTags(tags []string, query string) []string {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return tags
	}
	var out []string
	for _, tag := range tags {
		if strings.Contains(strings.ToLower(tag), q) {
			out = append(out, tag)
		}
	}
	return out
}
