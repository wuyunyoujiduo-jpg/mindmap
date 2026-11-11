package main

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

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

// sideWidth is the room the left column takes.
func (a *app) sideWidth() float32 {
	if a.sideShown() {
		return sideWidthOf
	}
	return 0
}

// topBar is the top of the window, on every page: the left column's
// key, the switch of the three places, the one bar, which takes the room,
// then the AI's key, the search of everything and the settings. It is the
// window's title bar, and drags it.
func (a *app) topBar(c *ui.Context) {
	t := c.Theme()
	bar := c.TitleBar()
	now := a.place()
	ui.Row(c.Key("top-bar")).Height(toolbarHeight).Padding(0, max(10, bar.Right), 0, max(12, bar.Left+6)).Gap(10).AlignItems(ui.Center).
		Shrink(0).FillWidth().DragWindow().ZIndex(20).Children(func() {
		side := "收起侧栏"
		if a.cfg.HideTree {
			side = "显示侧栏"
		}
		if iconButton(c, iconSidebar, side, side, 32, true).Clicked() {
			a.cfg.HideTree = !a.cfg.HideTree
			a.cfg.save()
		}
		well(ui.Row(c.Key("places")).Padding(2).Gap(2).Radius(radiusPill).Shrink(0).Label("位置"), t).Children(func() {
			part := func(p int, name, tip string, click func()) {
				on := p == now
				b := ui.Box(c.Key("place:"+name)).Height(30).Padding(0, 16).Radius(radiusPill - 2).Center().Cursor(ui.CursorPointer).
					Label(name).Tooltip(tip).Background(t.Text.Alpha(0)).Transition(easeLook)
				if !on && b.Hovered() {
					b.Background(t.Text.Alpha(0.05))
				}
				if b.Clicked() {
					a.later(click)
				}
				press(b, 0.97)
				b.Children(func() {
					if on {
						slidePill(c, "places", radiusPill-2)
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
		a.oneBar(c)
		// The AI's panel, from every page.
		ai := ui.Box(c.Key("ask-ai")).Size(32, 32).Radius(radiusPill).Center().Cursor(ui.CursorPointer).Label("AI 助手").
			Tooltip("问 AI：问问导图和资料里的内容").Background(t.Text.Alpha(0)).Transition(easeLook)
		switch {
		case a.aiOpen:
			ai.Background(t.Accent.Alpha(0.14))
		case ai.Hovered():
			ai.Background(t.Text.Alpha(0.07))
		}
		if ai.Clicked() {
			a.aiOpen = !a.aiOpen
		}
		press(ai, 0.92)
		ai.Children(func() { glyph(c, iconSparkle, 16, t.Accent) })
		if iconButton(c, iconSearch, "搜索全部", "搜索导图、主题、灵感和资料，#标签 按标签找", 32, true).Clicked() {
			a.later(func() { a.openPalette(paletteAll) })
		}
		if iconButton(c, iconGear, "设置", "设置", 32, true).Clicked() {
			a.later(a.openSettings)
		}
	})
}

// projectsView is 项目: the folders as a row of pills at the top, as the
// kinds of a wall are chosen (全部 is every map); under them the maps of
// the one chosen, as cards, those starred first; a folder's own folders
// are a row of pills under it. The folder the maps are all kept in, and
// how the sync goes, are at the page's foot.
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
	// The folder chosen, and the way to it from the top.
	var chain []model.Folder
	at := a.tree
	for {
		i := slices.IndexFunc(at.Folders, func(f model.Folder) bool {
			return f.Path == a.projFolder || inside(a.projFolder, f.Path)
		})
		if a.projFolder == "" || i < 0 {
			break
		}
		at = at.Folders[i]
		chain = append(chain, at)
	}
	if a.projFolder != "" && (len(chain) == 0 || chain[len(chain)-1].Path != a.projFolder) {
		a.projFolder, chain, at = "", nil, a.tree // it is gone
	}
	if a.renaming != "" && a.renaming != a.projFolder && !strings.HasSuffix(a.renaming, model.Ext) {
		a.projFolder = filepath.Dir(a.renaming) // a folder just made shows among its like
		if a.projFolder == a.dir {
			a.projFolder = ""
		}
	}
	w, _ := c.Size()
	if a.treeShown() {
		w -= sideWidthOf // the directory's
	}
	cols := max(int((min(w, pageWidth+56)-56+16)/256), 1)

	card := func(f model.FileInfo) {
		path, col := f.Path, a.mapColor(f.Path)
		el := raise(ui.Column(c.Key("map:"+path)).Padding(8, 8, 12).Gap(10).Radius(radiusCard).MinWidth(0).Cursor(ui.CursorPointer).
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
		n := counts[path]
		// The map itself, small: its face, by which it is told from the others.
		file := a.file
		if path != a.path {
			file = a.indexed(path)
		}
		var root *model.Node
		topics := 0
		if file != nil && len(file.Sheets) > 0 {
			root = file.Sheets[0].Root
			topics = countTopics(root) - 1
		}
		el.Children(func() {
			ui.Box(c.Key("face")).Height(104).FillWidth().Radius(radiusCard - 4).Background(cardColor(t)).PassThrough().
				Draw(func(pt *ui.Painter, r ui.Rect) {
					pt.Clip(r, radiusCard-4, func() { drawMapFace(pt, r, root, col) })
				})
			ui.Row(c).Padding(0, 4, 0, 8).Gap(6).AlignItems(ui.Center).FillWidth().Children(func() {
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(2).Children(func() {
					if a.renaming == path {
						a.renameInput(c)
					} else {
						ui.Text(c, f.Name).FontSize(15).FontWeight(600).SingleLine().FillWidth().PassThrough()
					}
					// A word on it: where it is, when not in the folder shown; how
					// much it holds; how its tasks stand.
					var says []string
					if dir := filepath.Dir(path); dir != at.Path && dir != a.dir {
						says = append(says, filepath.Base(dir))
					}
					if n.total > 0 {
						says = append(says, fmt.Sprintf("任务 %d / %d", n.done, n.total))
					} else {
						says = append(says, fmt.Sprintf("%d 个主题", max(topics, 0)))
					}
					ui.Text(c, strings.Join(says, " · ")).FontSize(textCaption).TextColor(t.TextMuted).SingleLine().FillWidth().PassThrough()
				})
				if a.starred(path) && !hover {
					ui.Text(c, "★").FontSize(textSmall).TextColor(ui.Hex("#D4A836")).PassThrough()
				}
				if n.total > 0 && !hover { // how far its tasks are, as a ring
					done := float32(n.done) / float32(n.total)
					ui.Box(c).Size(18, 18).Shrink(0).PassThrough().Draw(func(pt *ui.Painter, r ui.Rect) {
						cx, cy, rad := r.X+r.W/2, r.Y+r.H/2, float32(7)
						pt.StrokePath(new(ui.Path).Circle(cx, cy, rad), 2.5, t.Text.Alpha(0.09))
						if done > 0 {
							pt.StrokePath(new(ui.Path).Circle(cx, cy, rad).Trim(0, done).Cap(ui.RoundCap), 2.5, col)
						}
					})
				}
				a.rowMore(c, hover, "导图菜单", func(m *ui.Menu) { a.mapMenu(m, path) })
			})
		})
	}
	// A folder's pill: a click shows its maps, a map dropped on it goes
	// into it, and its menu renames, moves and deletes it.
	pill := func(key, name, count, path string, on bool) {
		b := ui.Row(c.Key(key)).Height(32).Padding(0, 14).Gap(6).Radius(radiusPill).AlignItems(ui.Center).Cursor(ui.CursorPointer).
			Label(name).Shrink(0).Background(t.Text.Alpha(0.05)).Transition(easeLook)
		switch {
		case path != "" && draggedOver(b):
			b.Background(t.Accent.Alpha(0.18))
		case on:
			soft(b.Background(pillColor(t)), t)
		case b.Hovered():
			b.Background(t.Text.Alpha(0.09))
		}
		press(b, 0.96)
		if path != "" {
			a.dropOnFolder(b, path)
			b.ContextMenu(func(m *ui.Menu) { a.folderMenu(m, path) })
		} else if d, ok := ui.Drop[fileDrag](b); ok && d.Path != "" { // onto 全部: out of its folder
			a.later(func() { a.moveMap(d.Path, a.dir) })
		}
		if b.Clicked() && a.renaming != path {
			if on && path != "" { // again: back to the folder it is in
				path = filepath.Dir(path)
				if path == a.dir {
					path = ""
				}
			}
			a.projFolder = path
		}
		b.Children(func() {
			if path != "" && a.renaming == path {
				ui.Box(c).Width(120).Children(func() { a.renameInput(c) })
				return
			}
			txt := ui.Text(c, name).FontSize(textBody).NoWrap().PassThrough()
			if on {
				txt.FontWeight(600)
			}
			ui.Text(c, count).FontSize(textSmall).TextColor(t.TextMuted).PassThrough()
		})
	}

	ui.Column(c.Key("projects")).Grow(1).Label("项目").Children(func() {
		pageColumn(c, "projects-column", true).Children(func() {
			title, sub := "项目", fmt.Sprintf("%d 张导图，一张就是一个项目", len(a.files))
			if a.projFolder != "" && a.treeShown() {
				title, sub = at.Name, fmt.Sprintf("%d 张导图", len(at.AllFiles()))
			}
			pageHeader(c, "projects-head", title, sub, func() {
				if a.cfg.HideTree {
					if iconButton(c, iconSidebar, "显示目录", "显示目录：文件夹和导图的列表", 32, true).Clicked() {
						a.cfg.HideTree = false
						a.cfg.save()
					}
				}
				if a.projFolder != "" { // the folder shown: rename, move, delete
					path := a.projFolder
					iconButton(c, iconMore, "文件夹菜单", "这个文件夹：重命名、移动、删除…", 32, true).Menu(func(m *ui.Menu) { a.folderMenu(m, path) })
				}
				accentButton(c, "＋ 新建", "新建导图、文件夹，或导入").Label("新建").Menu(func(m *ui.Menu) {
					if m.Item("新建导图").Shortcut(ui.Cmd, ui.KeyN).Chosen() {
						a.later(func() { a.newMapIn(at.Path) })
					}
					if m.Item("从模板新建…").Chosen() {
						a.templatesOpen = true
					}
					if m.Item("新建文件夹").Chosen() {
						a.later(func() { a.newFolder(at.Path) })
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
			// The folders, where the directory is put away: those of the top,
			// then those of each one chosen.
			if len(a.tree.Folders) > 0 && !a.treeShown() {
				ui.Column(c.Key("folders")).Gap(8).Padding(2, 0, 14).FillWidth().Shrink(0).Children(func() {
					for depth := 0; depth <= len(chain); depth++ {
						parent := a.tree
						if depth > 0 {
							parent = chain[depth-1]
						}
						if len(parent.Folders) == 0 {
							break
						}
						ui.Row(c.Key(fmt.Sprint("row", depth))).Gap(6).Wrap().GapY(6).AlignItems(ui.Center).FillWidth().Children(func() {
							if depth == 0 {
								pill("all", "全部", fmt.Sprint(len(a.files)), "", a.projFolder == "")
							} else {
								ui.Text(c, parent.Name+" ›").FontSize(textSmall).TextColor(t.TextMuted).Margin(0, 4, 0, 8)
							}
							for _, f := range parent.Folders {
								on := depth < len(chain) && chain[depth].Path == f.Path
								pill("folder:"+f.Path, f.Name, fmt.Sprint(len(f.AllFiles())), f.Path, on)
							}
						})
					}
				})
			}
			ui.Scroll(c.Key("projects-scroll:"+a.projFolder)).Grow(1).Margin(0, -12).Children(func() {
				ui.Column(c.Key("projects-list")).Padding(6, 12, 32).FillWidth().Children(func() {
					files := at.AllFiles() // those starred first, then by their names
					slices.SortStableFunc(files, func(x, y model.FileInfo) int { return b2i(a.starred(y.Path)) - b2i(a.starred(x.Path)) })
					if len(files) == 0 {
						says := "还没有导图"
						if a.projFolder != "" {
							says = "这个文件夹里还没有导图"
						}
						raise(ui.Column(c.Key("none")).Padding(28).Gap(12).Radius(radiusCard).AlignItems(ui.Center).FillWidth(), t).Children(func() {
							ui.Text(c, says).FontSize(textTitle).TextColor(t.TextMuted)
							if inkButton(c, "新建导图", true).Clicked() {
								a.later(func() { a.newMapIn(at.Path) })
							}
						})
						if a.projFolder == "" {
							a.removeMapMeta(a.tree.Path)
						}
						return
					}
					ui.Grid(c.Key("maps")).Columns(cols).Gap(16).FillWidth().Children(func() {
						for _, f := range files {
							card(f)
						}
					})
				})
			})
			if a.treeShown() {
				return // the directory has them at its foot
			}
			// Where the maps are kept, and how the sync goes.
			ui.Row(c.Key("projects-foot")).Height(36).Gap(8).AlignItems(ui.Center).Shrink(0).FillWidth().Children(func() {
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

// countTopics is how many topics a topic and those under it are.
func countTopics(n *model.Node) int {
	if n == nil {
		return 0
	}
	k := 1
	for _, ch := range n.Children {
		k += countTopics(ch)
	}
	return k
}

// drawMapFace draws a map small, in r, as the face of its card: its
// center in the map's color, its main topics around it, each in a color
// of its own and as long as its name, with those under it as short
// strokes. It is the map's shape, not its words.
func drawMapFace(p *ui.Painter, r ui.Rect, root *model.Node, col ui.Color) {
	s := min(r.W/300, 1.3) // the whole of it within the face, whatever the card's width
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	rw, rh := 52*s, 18*s
	var kids []*model.Node
	if root != nil {
		kids = root.Children
	}
	if len(kids) > 6 {
		kids = kids[:6]
	}
	right := (len(kids) + 1) / 2
	for i, k := range kids {
		side, j, of := float32(1), i, right
		if i >= right {
			side, j, of = -1, i-right, len(kids)-right
		}
		gap := min(28*s, (r.H-22*s)/float32(of))
		by := cy + (float32(j)-float32(of-1)/2)*gap
		bw := min(max(float32(len([]rune(k.Title)))*5.5+16, 26), 62) * s
		bh := 12 * s
		near := cx + side*(rw/2+30*s) // the topic's edge toward the center
		bx := near
		if side < 0 {
			bx = near - bw
		}
		bc := ui.Hex(mapPalette[(colorIndex(col)+i+1)%len(mapPalette)])
		p.StrokePath(new(ui.Path).MoveTo(cx+side*rw/2, cy).CubeTo(cx+side*(rw/2+16*s), cy, near-side*16*s, by, near, by), 1.5*s, bc.Alpha(0.75))
		p.Fill(ui.Rect{X: bx, Y: by - bh/2, W: bw, H: bh}, bc.Alpha(0.36), bh/2)
		far := near + side*bw
		m := min(len(k.Children), 3)
		for q := range m {
			sy := by + (float32(q)-float32(m-1)/2)*7*s
			sw := 14 * s
			sx := far + side*8*s
			if side < 0 {
				sx -= sw
			}
			p.Line(far, by, far+side*8*s, sy, 1*s, bc.Alpha(0.45))
			p.Fill(ui.Rect{X: sx, Y: sy - 2*s, W: sw, H: 4 * s}, bc.Alpha(0.3), 2*s)
		}
	}
	p.Fill(ui.Rect{X: cx - rw/2, Y: cy - rh/2, W: rw, H: rh}, col, rh/2)
}

// colorIndex is a map's color's place among the maps' colors, or 0.
func colorIndex(col ui.Color) int {
	for i, hex := range mapPalette {
		if ui.Hex(hex) == col {
			return i
		}
	}
	return 0
}
