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

// fileDrag is the value a map's row carries while dragged: onto a folder,
// another map, or a topic of the canvas.
type fileDrag struct {
	Path string // the map's
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

// rowMore is the "···" of a row of the list, shown while the pointer is on
// the row: its menu, which a right click opens too.
func (a *app) rowMore(c *ui.Context, hover bool, label string, menu func(m *ui.Menu)) {
	more := iconButton(c, iconMore, label, "重命名、移动、删除…", 22, true)
	if !hover {
		more.Width(0).Opacity(0) // no room taken, and its menu, if open, stays
	}
	more.Menu(menu)
}

// folderMenu is what can be done with a folder of the list.
func (a *app) folderMenu(m *ui.Menu, path string) {
	if m.Item("在此新建导图").Chosen() {
		a.later(func() { a.newMapIn(path) })
	}
	if m.Item("新建子文件夹").Chosen() {
		a.later(func() { a.newFolder(path) })
	}
	m.Separator()
	if m.Item("重命名").Chosen() {
		a.renaming, a.renameText = path, filepath.Base(path)
	}
	if m.Item("在 Finder 中显示").Chosen() {
		a.plat.ShowInFolder(path)
	}
	m.Separator()
	if m.Item("移到废纸篓…").Chosen() {
		a.deletePath, a.confirmDelete = path, true
	}
}

// mapsIn are the maps of a folder and of those in it.
func (a *app) mapsIn(dir string) []model.FileInfo {
	var out []model.FileInfo
	for _, f := range a.files {
		if inside(f.Path, dir) {
			out = append(out, f)
		}
	}
	return out
}

// deleteFolder moves a folder, with the maps in it, to the trash.
func (a *app) deleteFolder(dir string) {
	if dir == "" || dir == a.dir || !inside(dir, a.dir) {
		return
	}
	a.flush()
	maps := a.mapsIn(dir)
	if err := a.plat.Trash(dir); err != nil {
		a.saveErr = err.Error()
		return
	}
	for _, f := range maps {
		a.removeMapMeta(f.Path)
	}
	gone := func(p string) bool { return inside(p, dir) }
	a.cfg.Favorites = slices.DeleteFunc(a.cfg.Favorites, gone)
	a.cfg.OpenFolders = slices.DeleteFunc(a.cfg.OpenFolders, gone)
	a.cfg.save()
	a.refreshFiles()
	if a.path != "" && inside(a.path, dir) {
		a.path, a.file, a.doc, a.dirty = "", nil, nil, false
		if len(a.files) > 0 {
			_ = a.open(a.files[0].Path)
		}
	}
	a.tell("已把文件夹「" + filepath.Base(dir) + "」移到废纸篓")
}

// mapMenu is what can be done with a map of the list.
func (a *app) mapMenu(m *ui.Menu, path string) {
	label := "加星标（排在前面）"
	if a.starred(path) {
		label = "取消星标"
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
	if strings.HasSuffix(path, model.Ext) {
		model.MoveAssets(path, to) // its pictures go with it
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
	a.setFolderOpen(dir, true)
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
	for _, f := range a.files { // the versions of the maps follow them
		if to := follow(f.Path); to != f.Path {
			a.versions.Rename(f.Path, to)
		}
	}
	a.moveMapMetas(follow) // and their analyses
	a.path = follow(a.path)
	a.renameViews(follow)
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
		a.showShotAlert("没有新建文件夹", "在「"+dir+"」里新建「"+name+"」失败：\n"+err.Error(), false)
		return
	}
	a.setFolderOpen(dir, true)
	a.refreshFiles()
	a.renaming, a.renameText = path, name
	a.tell("已新建文件夹「" + name + "」，给它起个名字")
}

// toggleFavorite stars a map, or takes its star off: those starred come
// first, in their folder and where maps are switched.
func (a *app) toggleFavorite(path string) {
	if i := slices.Index(a.cfg.Favorites, path); i >= 0 {
		a.cfg.Favorites = slices.Delete(a.cfg.Favorites, i, i+1)
	} else {
		a.cfg.Favorites = append(a.cfg.Favorites, path)
	}
	a.cfg.save()
}

// starred tells whether a map is starred.
func (a *app) starred(path string) bool { return slices.Contains(a.cfg.Favorites, path) }

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// tagList is the section of the tags, shared by all maps: a row each with
// how many nodes of the open map carry it. Choosing one highlights its
// nodes and filters the tasks; its context menu renames, recolors and
// deletes it. Past a few tags, a field filters the list.
func (a *app) tagList(c *ui.Context) {
	t := c.Theme()
	tags := a.allTags()
	counts := map[string]int{}
	a.file.Walk(func(n *model.Node) {
		for _, tag := range n.Tags {
			counts[tag]++
		}
	})
	title := "本导图标签"
	if len(tags) > 0 {
		title = fmt.Sprintf("本导图标签 %d", len(tags))
	}
	sectionCard(c, iconTag, tileBlue, title, nil, func() {
		if len(tags) > 8 {
			ui.TextInput(c.Key("tag-query"), &a.tagQuery).Placeholder("筛选标签").Margin(2, 6, 4)
		}
		if a.tagFilter != "" {
			if ui.Link(c, "清除筛选：#"+a.tagFilter, "").FontSize(12).Margin(0, 12, 4).Clicked() {
				a.tagFilter = ""
			}
		}
		for _, tag := range filterTags(tags, a.tagQuery) {
			a.tagRow(c.Key("tag:"+tag), tag, counts[tag])
		}
		if a.tagAdding {
			in := ui.TextInput(c.Key("tag-new"), &a.tagNew).Placeholder("标签名").AutoFocus().Margin(2, 6, 4)
			switch {
			case in.Shortcut(0, ui.KeyEscape):
				a.tagAdding, a.tagNew = false, ""
			case in.Submitted():
				a.addTag(a.tagNew)
				a.tagNew = "" // stays open for the next
			case in.PressedOutside():
				a.addTag(a.tagNew)
				a.tagAdding, a.tagNew = false, ""
			}
		} else if ui.Link(c, "＋ 新建标签", "").FontSize(12).Margin(2, 12, 4).Clicked() {
			a.tagAdding = true
		}
		if len(tags) == 0 {
			ui.Text(c, "标签在所有导图间共用。选中节点后，在右侧「节点」或右键菜单里打标签。").
				FontSize(12).TextColor(t.TextMuted).Wrap().Margin(2, 12)
		}
	})
}

func (a *app) tagRow(c *ui.Context, tag string, count int) {
	t := c.Theme()
	if a.tagRenaming == tag {
		in := ui.TextInput(c.Key("tag-rename"), &a.tagRenameText).AutoFocus().Margin(2, 6)
		switch {
		case in.Shortcut(0, ui.KeyEscape):
			a.tagRenaming = ""
		case in.Submitted(), in.PressedOutside():
			to := a.tagRenameText
			a.tagRenaming = ""
			a.later(func() { a.renameTag(tag, to) })
		}
		return
	}
	active := a.tagFilter == tag
	row := ui.Row(c).Padding(4, 8, 4, 12).Gap(8).Radius(radiusSmall).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Label("筛选标签 " + tag).Tooltip("点击筛选，右键重命名、改颜色或删除")
	switch {
	case active:
		row.Background(tagColor(tag).Alpha(0.18))
	case row.Hovered():
		row.Background(t.Text.Alpha(0.06))
	}
	if row.Clicked() {
		if active {
			a.tagFilter = ""
		} else {
			a.tagFilter = tag
		}
	}
	row.ContextMenu(func(m *ui.Menu) {
		if m.Item("重命名").Chosen() {
			a.tagRenaming, a.tagRenameText = tag, tag
		}
		m.Submenu("颜色", func(m *ui.Menu) {
			cur := ""
			if i := a.libraryIndex(tag); i >= 0 {
				cur = a.cfg.Tags[i].Color
			}
			if m.Item("自动").Checked(cur == "").Chosen() {
				a.setTagColor(tag, "")
			}
			for _, col := range tagColorChoices {
				if m.Item(col.name).Checked(cur == col.hex).Chosen() {
					a.setTagColor(tag, col.hex)
				}
			}
		})
		m.Separator()
		if m.Item("删除（从所有导图的节点上移除）").Chosen() {
			a.later(func() { a.deleteTag(tag) })
		}
	})
	row.Children(func() {
		ui.Box(c).Size(8, 8).Radius(999).Background(tagColor(tag))
		text := ui.Text(c, tag).FontSize(13).Grow(1).SingleLine()
		if active {
			text.Bold()
		}
		if count > 0 {
			ui.Text(c, fmt.Sprint(count)).FontSize(11).TextColor(t.TextMuted)
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

// sideTile is a row's icon in the maps' list, of the rail's family: thin
// lines in the muted ink. A map's carries a dot of the map's color, the
// one place its color shows there.
func sideTile(c *ui.Context, ic icon, col ui.Color) {
	t := c.Theme()
	ui.Box(c).Size(18, 18).Shrink(0).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) {
		switch ic {
		case iconMap:
			drawRailIcon(p, r, 17, railMap, t.TextMuted)
			p.FillPath(new(ui.Path).Circle(r.X+r.W*0.29, r.Y+r.H/2, 2.6), col) // its topic, in its color
		default:
			drawIcon(p, r, 15, ic, t.TextMuted)
		}
	})
}

// folderOpen tells whether a folder of the sidebar is shown open: those
// opened, as the config keeps them through restarts and updates; others
// are closed.
func (a *app) folderOpen(path string) bool { return slices.Contains(a.cfg.OpenFolders, path) }

// setFolderOpen opens or closes a folder of the sidebar, as kept.
func (a *app) setFolderOpen(path string, open bool) {
	if a.folderOpen(path) == open {
		return
	}
	if open {
		a.cfg.OpenFolders = append(a.cfg.OpenFolders, path)
	} else {
		a.cfg.OpenFolders = slices.DeleteFunc(a.cfg.OpenFolders, func(p string) bool { return p == path })
	}
	a.cfg.save()
}
