package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// The command palette: one field that finds commands, nodes and maps, and
// runs or opens what is chosen, with the keyboard alone. ⌘K opens it on
// everything, ⌘P on the maps, ⌘⇧F on the nodes of every map.

// Palette modes.
const (
	paletteAll = iota
	paletteMaps
	paletteSearch
	paletteLink // a node to link the node a.linkFrom to
)

// paletteItem is a line of the palette.
type paletteItem struct {
	kind   string // "命令", "节点", "导图"
	title  string
	detail string // a shortcut, or where a node is
	run    func()
}

// command is an action of the palette.
type command struct {
	title, keys string
	run         func()
	ok          func() bool // whether it applies now; nil: always
}

func (a *app) commands() []command {
	hasDoc := func() bool { return a.doc != nil }
	view := func(m int) func() { return func() { a.mode = m } }
	cmds := []command{
		{"新建导图", "⌘N", a.newMap, nil},
		{"新建画布", "⌘T", func() { a.mode = viewMap; a.addSheet() }, hasDoc},
		{"切换到思维导图", "⌘1", view(viewMap), hasDoc},
		{"切换到大纲", "⌘2", view(viewOutline), hasDoc},
		{"切换到看板", "⌘3", view(viewBoard), hasDoc},
		{"切换到日程", "⌘4", view(viewAgenda), hasDoc},
		{"开始演说", "⌘↩", func() { a.startPresenting("") }, hasDoc},
		{"搜索所有导图", "⌘⇧F", func() { a.openPalette(paletteSearch) }, nil},
		{"快速切换导图", "⌘P", func() { a.openPalette(paletteMaps) }, nil},
		{"快捷键速查", "⌘?", func() { a.cheatsOpen = true }, nil},
		{"打开 AI 助手", "", func() { a.aiOpen = true }, hasDoc},
		{"添加子节点", "Tab", a.addChild, hasDoc},
		{"添加同级节点", "↩", a.addSibling, hasDoc},
		{"复制节点", "⌘C", func() { a.copyNodes(false) }, hasDoc},
		{"粘贴为子节点", "⌘V", func() { a.paste() }, hasDoc},
		{"聚焦所选分支", "⌥↩", func() { a.focusOn(a.selected) }, hasDoc},
		{"退出聚焦", "Esc", func() { a.focusOn("") }, func() bool { return a.focusNode() != nil }},
		{"全部展开", "⌥0", func() { a.expandTo(0) }, hasDoc},
		{"展开到第 1 层", "⌥1", func() { a.expandTo(1) }, hasDoc},
		{"展开到第 2 层", "⌥2", func() { a.expandTo(2) }, hasDoc},
		{"展开到第 3 层", "⌥3", func() { a.expandTo(3) }, hasDoc},
		{"回到中心", "⌘⇧0", a.recenter, hasDoc},
		{"导出为图片", "⌘⇧E", a.askExportImage, hasDoc},
		{"导出为 Markdown", "", a.askExportMarkdown, hasDoc},
		{"导出为 XMind", "", a.askExportXMind, hasDoc},
		{"复制当前画布为 Markdown", "⌘⇧C", a.copyMarkdown, hasDoc},
		{"导入 XMind", "⌘⇧I", a.askImportXMind, nil},
		{"历史版本", "⌘⇧H", a.openHistory, hasDoc},
		{"连接 AI 编程工具（MCP）", "", func() { a.mcpOpen = true }, nil},
		{"撤销", "⌘Z", a.undo, hasDoc},
		{"重做", "⌘⇧Z", a.redo, hasDoc},
	}
	for _, s := range model.Statuses {
		cmds = append(cmds, command{"所选设为「" + s.Label() + "」", "", func() { a.setStatusAll(s) }, hasDoc})
	}
	return cmds
}

// openPalette opens the palette in a mode.
func (a *app) openPalette(mode int) {
	a.commitEditIfAny()
	a.paletteOpen, a.paletteMode, a.paletteQuery, a.paletteIndex = true, mode, "", 0
}

// paletteItems are the lines the palette shows for what is typed.
func (a *app) paletteItems() []paletteItem {
	q := strings.ToLower(strings.TrimSpace(a.paletteQuery))
	match := func(s string) bool { return q == "" || strings.Contains(strings.ToLower(s), q) }
	var items []paletteItem
	if a.paletteMode == paletteAll {
		for _, cmd := range a.commands() {
			if (cmd.ok == nil || cmd.ok()) && match(cmd.title) {
				items = append(items, paletteItem{"命令", cmd.title, cmd.keys, cmd.run})
			}
		}
		if q != "" && a.file != nil {
			items = append(items, a.nodeItems(a.path, a.file, q, 30, a.goToNode)...)
		}
	}
	if a.paletteMode == paletteAll && q != "" || a.paletteMode == paletteMaps {
		for _, f := range a.recentFiles() {
			name := mapName(f.Path)
			if match(name) {
				path := f.Path
				items = append(items, paletteItem{"导图", name, a.relDir(path), func() { a.openMap(path) }})
			}
		}
	}
	if (a.paletteMode == paletteSearch || a.paletteMode == paletteLink) && q != "" {
		act := a.goToNode
		if a.paletteMode == paletteLink {
			act = func(path, _, id string) {
				if n, _ := a.doc.Find(a.linkFrom); n != nil && id != n.ID {
					a.addLink(n, a.nodeLinkTo(path, id))
				}
			}
		}
		for _, f := range a.files {
			file := a.file
			if f.Path != a.path {
				file = a.indexed(f.Path)
			}
			if file != nil {
				items = append(items, a.nodeItems(f.Path, file, q, 200-len(items), act)...)
			}
			if len(items) >= 200 {
				break
			}
		}
	}
	return items
}

// nodeItems are the nodes of a map matching the query: their title, note
// or tags.
func (a *app) nodeItems(path string, f *model.File, q string, limit int, act func(path, sheet, id string)) []paletteItem {
	var items []paletteItem
	for _, d := range f.Sheets {
		d.Walk(func(n, _ *model.Node, _ int) bool {
			if len(items) >= limit {
				return false
			}
			if !n.Matches(q) {
				return true
			}
			where := mapName(path)
			if len(f.Sheets) > 1 {
				where += " · " + d.Title
			}
			if p := d.Path(n.ID); len(p) > 1 {
				where += " · " + p[len(p)-2].Title
			}
			title := n.Title
			if title == "" {
				title = "（无标题）"
			}
			if !strings.Contains(strings.ToLower(title), q) && n.Note != "" {
				title += "　— " + snippet(n.Note, q)
			}
			sheet, id := d.ID, n.ID
			items = append(items, paletteItem{"节点", title, where, func() { act(path, sheet, id) }})
			return true
		})
	}
	return items
}

// snippet is the part of a text around the query.
func snippet(text, q string) string {
	text = strings.Join(strings.Fields(text), " ")
	r := []rune(text)
	i := strings.Index(strings.ToLower(text), q)
	if i < 0 {
		i = 0
	}
	at := len([]rune(text[:i]))
	from, to := max(at-10, 0), min(at+30, len(r))
	if q == "" {
		from, to = 0, min(300, len(r)) // the start of the text, as a tooltip shows it
	}
	s := string(r[from:to])
	if from > 0 {
		s = "…" + s
	}
	if to < len(r) {
		s += "…"
	}
	return s
}

// goToNode opens a map, its canvas and selects the node in the map view.
func (a *app) goToNode(path, sheet, id string) {
	if path != a.path {
		if err := a.open(path); err != nil {
			return
		}
	}
	a.mode = viewMap
	a.showSheet(sheet)
	if a.focusNode() != nil && !a.focusNode().Contains(id) {
		a.focusOn("")
	}
	a.multi = nil
	a.selectNode(id)
}

func (a *app) openMap(path string) {
	if path != a.path {
		_ = a.open(path)
	}
}

// recentFiles are the maps, those opened last first.
func (a *app) recentFiles() []model.FileInfo {
	files := slices.Clone(a.files)
	rank := func(p string) int {
		if i := slices.Index(a.cfg.Recent, p); i >= 0 {
			return i
		}
		return len(a.cfg.Recent)
	}
	slices.SortStableFunc(files, func(x, y model.FileInfo) int { return rank(x.Path) - rank(y.Path) })
	return files
}

// remember puts a map first among those opened last.
func (a *app) remember(path string) {
	a.cfg.Recent = slices.DeleteFunc(a.cfg.Recent, func(p string) bool { return p == path })
	a.cfg.Recent = append([]string{path}, a.cfg.Recent...)
	if len(a.cfg.Recent) > 30 {
		a.cfg.Recent = a.cfg.Recent[:30]
	}
}

// relDir is the folder of a map, within the folder of maps.
func (a *app) relDir(path string) string {
	rel := strings.TrimPrefix(strings.TrimPrefix(filepath.Dir(path), a.dir), string(os.PathSeparator))
	if rel == "" {
		return "导图"
	}
	return rel
}

// indexed is a map read for the search, kept until its file changes.
func (a *app) indexed(path string) *model.File {
	mt := modTime(path)
	if e, ok := a.index[path]; ok && e.mod.Equal(mt) {
		return e.file
	}
	f, err := model.Load(path)
	if err != nil {
		return nil
	}
	if a.index == nil {
		a.index = map[string]indexEntry{}
	}
	a.index[path] = indexEntry{mod: mt, file: f}
	return f
}

type indexEntry struct {
	mod  time.Time
	file *model.File
}

// paletteView shows the palette over the window.
func (a *app) paletteView(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c.Key("palette"), &a.paletteOpen, func() {
		items := a.paletteItems()
		if a.paletteIndex >= len(items) {
			a.paletteIndex = max(len(items)-1, 0)
		}
		placeholder := map[int]string{
			paletteAll:    "输入命令、节点或导图名…",
			paletteMaps:   "切换到导图…",
			paletteSearch: "搜索所有导图里的节点、备注和标签…",
			paletteLink:   "输入关键词，选择要链接到的节点…",
		}[a.paletteMode]
		w, _ := c.Size()
		ui.Column(c).Width(min(640, w-80)).Gap(8).Children(func() {
			in := ui.TextInput(c.Key("palette-query"), &a.paletteQuery).Placeholder(placeholder).AutoFocus().FontSize(15)
			if in.Changed() {
				a.paletteIndex = 0
			}
			run := func(i int) {
				if i < len(items) {
					a.paletteOpen = false
					a.later(items[i].run)
				}
			}
			switch {
			case in.Shortcut(0, ui.KeyDown):
				a.paletteIndex = min(a.paletteIndex+1, len(items)-1)
			case in.Shortcut(0, ui.KeyUp):
				a.paletteIndex = max(a.paletteIndex-1, 0)
			case in.Submitted():
				run(a.paletteIndex)
			}
			ui.Scroll(c.Key("palette-list")).MaxHeight(420).Children(func() {
				ui.Column(c).Gap(1).Children(func() {
					for i, it := range items {
						row := ui.Row(c.Key(fmt.Sprint("item", i))).Padding(6, 10).Gap(10).Radius(6).AlignItems(ui.Center).
							Cursor(ui.CursorPointer).Label(it.title)
						if i == a.paletteIndex {
							row.Background(t.Accent.Alpha(0.14))
							if a.paletteShown != i { // moved to by the keys
								row.ScrollIntoView()
								a.paletteShown = i
							}
						} else if row.Hovered() {
							row.Background(t.SurfaceHover)
						}
						if row.Clicked() {
							run(i)
						}
						row.Children(func() {
							ui.Text(c, it.kind).FontSize(11).TextColor(t.TextMuted).Width(30).Shrink(0)
							if it.kind == "命令" { // a command, with its keys at the end
								ui.Text(c, it.title).FontSize(14).Grow(1)
								ui.Text(c, it.detail).FontSize(12).TextColor(t.TextMuted).NoWrap()
								return
							}
							ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
								ui.Text(c, it.title).FontSize(14).MaxLines(1)
								ui.Text(c, it.detail).FontSize(11).TextColor(t.TextMuted).MaxLines(1)
							})
						})
					}
					if len(items) == 0 {
						msg := "没有找到"
						if a.paletteMode == paletteSearch && strings.TrimSpace(a.paletteQuery) == "" {
							msg = "输入关键词，在文件夹里所有导图中查找"
						}
						ui.Text(c, msg).FontSize(13).TextColor(t.TextMuted).Margin(8, 10)
					}
				})
			})
			ui.Text(c, "↑↓ 选择 · ↩ 执行 · Esc 关闭 · ⌘K 全部 · ⌘P 导图 · ⌘⇧F 搜索所有导图").FontSize(11).TextColor(t.TextMuted)
		})
	})
}

// shortcuts are the keys the cheat sheet lists, by group.
var shortcuts = []struct {
	group string
	keys  [][2]string
}{
	{"全局", [][2]string{
		{"⌘K", "命令面板"}, {"⌘P", "快速切换导图"}, {"⌘⇧F", "搜索所有导图"}, {"⌘N", "新建导图"},
		{"⌘T", "新建画布"}, {"⌘1–4", "导图 / 大纲 / 看板 / 日程"}, {"⌘↩", "开始演说"}, {"⌘?", "快捷键速查"},
	}},
	{"编辑节点", [][2]string{
		{"Tab", "添加子节点"}, {"↩", "添加同级节点"}, {"Space / F2", "编辑标题"}, {"Delete", "删除"},
		{"⌘C / ⌘X / ⌘V", "复制 / 剪切 / 粘贴（可跨导图，也可粘贴文本大纲）"}, {"⌘Z / ⌘⇧Z", "撤销 / 重做"},
		{"⌘↑ / ⌘↓", "上移 / 下移"}, {"⌘/", "折叠 / 展开"}, {"⇧ 或 ⌘ + 点击", "多选节点"},
	}},
	{"浏览", [][2]string{
		{"← → ↑ ↓", "在节点间移动"}, {"⌥↩", "聚焦所选分支"}, {"Esc", "取消多选 / 退出聚焦"},
		{"⌥1–9 / ⌥0", "展开到第 N 层 / 全部展开"}, {"⌘= / ⌘- / ⌘0", "放大 / 缩小 / 实际大小"}, {"⌘⇧0", "回到中心"},
		{"⌘ + 滚轮", "缩放"},
	}},
	{"导入导出", [][2]string{
		{"⌘⇧E", "导出为图片"}, {"⌘⇧C", "复制当前画布为 Markdown"}, {"⌘⇧I", "导入 XMind"},
	}},
}

// cheatSheet shows every shortcut.
func (a *app) cheatSheet(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c.Key("cheats"), &a.cheatsOpen, func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "快捷键").FontSize(18).Bold().Grow(1)
			if ui.Button(c, "关闭").Clicked() {
				a.cheatsOpen = false
			}
		})
		ui.Scroll(c.Key("cheat-list")).MaxHeight(520).Children(func() {
			ui.Row(c).Gap(28).Wrap().AlignItems(ui.Start).Children(func() {
				for _, g := range shortcuts {
					ui.Column(c.Key(g.group)).Gap(6).Width(300).Children(func() {
						ui.Text(c, g.group).FontSize(12).Bold().TextColor(t.TextMuted)
						for _, k := range g.keys {
							ui.Row(c.Key(k[0])).Gap(10).Children(func() {
								ui.Box(c).Padding(1, 6).Radius(4).Background(t.Surface).Border(1, t.Border).Shrink(0).Children(func() {
									ui.Text(c, k[0]).FontSize(12).NoWrap()
								})
								ui.Text(c, k[1]).FontSize(13).Wrap().Grow(1)
							})
						}
					})
				}
			})
		})
	})
}
