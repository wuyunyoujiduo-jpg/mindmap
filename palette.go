package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
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
	paletteLink   // a node to link the node a.linkFrom to
	paletteMove   // a node to move the nodes selected under
	paletteClip   // a clip of the library to link the node a.linkFrom to
	paletteAttach // a node, of any map, to link a.attachRef to
	paletteSkill  // a skill of the AI agents to link the node a.linkFrom to
	paletteGrow   // a topic, of any map, for a branch grown of a tag to go under (grow.go)
)

// paletteItem is a line of the palette.
type paletteItem struct {
	kind   string // "命令", "主题", "导图"
	title  string
	detail string // a shortcut, or where a node is
	run    func()
	// task, for a task found, is it: its row ticks it off and puts it
	// on tomorrow where it is, the search staying open.
	task *taskItem
}

// command is an action of the palette.
type command struct {
	title, keys string
	run         func()
	ok          func() bool // whether it applies now; nil: always
}

func (a *app) commands() []command {
	hasDoc := func() bool { return a.doc != nil }
	notPhone := func() bool { return !onPhone } // what a phone has not (phone.go)
	view := func(m int) func() { return func() { a.showView(m) } }
	cmds := []command{
		{"新建导图", "⌘N", a.newMap, nil},
		{"从模板新建…", "⌘⇧N", func() { a.templatesOpen = true }, nil},
		{"把当前导图存为模板", "", a.saveAsTemplate, hasDoc},
		{"新建画布", "⌘T", func() { a.mode = viewMap; a.addSheet() }, hasDoc},
		{"切换到思维导图", "⌘1", view(viewMap), hasDoc},
		{"切换到大纲", "⌘2", view(viewOutline), hasDoc},
		{"切换到看板", "⌘3", view(viewBoard), hasDoc},
		{"切换到时间线", "⌘4", view(viewTimeline), hasDoc},
		{"切换到总览", "⌘5", view(viewOverview), hasDoc},
		{"开始演说", "⌘↩", func() { a.startPresenting("") }, hasDoc},
		{"搜索所有导图", "⌘⇧F", func() { a.openPalette(paletteSearch) }, nil},
		{"快速切换导图", "⌘P", func() { a.openPalette(paletteMaps) }, nil},
		{"快捷键速查", "⌘?", func() { a.cheatsOpen = true }, nil},
		{"打开 AI 助手", "", func() { a.aiOpen = true }, hasDoc},
		{"添加子主题", "Tab", a.addChild, hasDoc},
		{"添加同级主题", "↩", a.addSibling, hasDoc},
		{"在上方添加同级主题", "⇧↩", func() { a.addSiblingAt(false) }, hasDoc},
		{"关联线到…（所选主题连到另一个主题）", "⌘⇧L", func() {
			if n, _ := a.selectedNode(); n != nil {
				a.startRelation(n)
			}
		}, hasDoc},
		{"在看板中显示所选任务", "", func() { a.showOnBoard(a.selected) }, func() bool {
			n, _ := a.selectedNode()
			return n != nil && n.Task != nil
		}},
		{"复制主题", "⌘C", func() { a.copyNodes(false) }, hasDoc},
		{"粘贴为子主题", "⌘V", func() { a.paste() }, hasDoc},
		{"聚焦所选分支", "⌥↩", func() { a.focusOn(a.selected) }, hasDoc},
		{"图解：把所选主题这一支画成流程、状态、判定表和名词", "", func() {
			if a.diaOf(a.selected) != nil {
				a.openDia(a.selected, "")
			} else {
				a.drawDia(a.selected, "")
			}
		}, func() bool {
			n, _ := a.selectedNode()
			return hasDoc() && !a.libOpen && (a.diaOf(a.selected) != nil || diaWorth(n))
		}},
		{"退出聚焦", "Esc", func() { a.focusOn("") }, func() bool { return a.focusNode() != nil }},
		{"全部折叠（只看中心主题）", "", a.foldAll, hasDoc},
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
		{"导入 Markdown / OPML", "", a.askImportOutline, nil},
		{"导入文件夹里的文档（每篇一个画布）", "", a.askImportFolder, nil},
		{"导出为 OPML", "", a.askExportOPML, hasDoc},
		{"导出任务到日历（.ics）", "", a.askExportICS, hasDoc},
		{"导出演说为 PDF", "", a.askExportSlides, hasDoc},
		{"历史版本", "⌘⇧H", a.openHistory, hasDoc},
		{"AI：生成周报", "", func() { a.aiReport(7) }, hasDoc},
		{"AI：生成日报", "", func() { a.aiReport(1) }, hasDoc},
		{"AI：风险扫描", "", a.aiRiskScan, hasDoc},
		{"AI：拆解所选任务", "", a.aiBreakDown, hasDoc},
		{"从文本生成导图…", "", func() { a.fromTextOpen = true }, hasDoc},
		{"从代码仓库生成架构…", "", a.askRepo, hasDoc},
		{"快速记录", keyLabel(a.captureKey()), a.plat.ShowCapture, nil},
		{"打开收件箱", "", func() { _ = a.open(a.inboxPath()) }, func() bool {
			inbox := a.inboxPath() // among the maps as scanned: the file is not looked for
			return slices.ContainsFunc(a.files, func(f model.FileInfo) bool { return f.Path == inbox })
		}},
		{"移到其他导图…", "", func() { a.openPalette(paletteMove) }, hasDoc},
		// No key said: ⌘⇧L, which it said, is the canvas's 关联线, and the
		// place 收集 has the places' key (⌥⌘3, 前往).
		{"打开收集", "", a.showLibrary, nil},
		{"收藏剪贴板到收集", keyLabel(a.clipKey()), a.clipFromShortcut, nil},
		{"截图", keyLabel(a.shotKey()), a.screenshot, notPhone},
		{"AI 整理收集的资料", "", func() { a.showLibrary(); a.organizeAll() }, func() bool { return len(a.lib.Unorganized()) > 0 }},
		{"设置", "⌘,", a.openSettings, nil},
		{"连接 AI 编程工具（MCP）", "", func() { a.mcpOpen = true }, notPhone},
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
	a.floatAt = nil
}

// paletteItems are the lines the palette shows for what is typed. They
// are found once for what is asked and what it is found in (foundKey),
// not at each frame: the bar with words in it, and the palette open,
// built the commands, walked the open map and searched 收集 as they were
// drawn.
func (a *app) paletteItems() []paletteItem { return a.foundIn(&a.memo.found[0]) }

// found is what was found, and of what.
type found struct {
	of    foundKey
	items []paletteItem
}

// foundIn finds the palette's lines, or gives those kept in f where they
// were found of what stands now. The palette and the bar keep theirs
// apart (memo.found): the bar's words stay in it while the palette is
// open, and each would have taken the other's place at every frame.
func (a *app) foundIn(f *found) []paletteItem {
	key, ok := a.foundOf()
	if ok && key == f.of {
		return f.items
	}
	a.memo.finds++
	items := a.paletteFind()
	*f = found{}
	if ok {
		*f = found{key, items}
	}
	return items
}

// foundKey is what the palette's lines were found of: what is asked (the
// words, the mode, what is being placed), and all they are found in and
// say: the open map as edited and what of it is chosen, the maps as read
// and the folder as scanned, the tasks as gathered, 收集 as it stands,
// the maps' analyses and the diagrams, the day and the shortcuts said.
type foundKey struct {
	query                     string
	mode                      int
	edits, maps, scans, tasks int
	lib                       *library.Library
	version, metas, dias      int
	doc                       *model.Document
	selected, focus, attach   string
	libOpen                   bool
	day                       time.Time
	keys                      [3]string
}

// foundOf is the key of what the palette would find now, and whether it
// is one to keep by: a skill's and a clip's likeliest places are worked
// out of more than the key says, and are found at each asking.
func (a *app) foundOf() (foundKey, bool) {
	if a.lib == nil || a.paletteMode == paletteSkill || a.paletteMode == paletteAttach && strings.TrimSpace(a.paletteQuery) == "" {
		return foundKey{}, false
	}
	a.taskItems() // looks at the maps' files, once a tick
	if at := a.looked(); at != a.memo.metasLooked {
		a.memo.metasLooked = at
		a.loadMapMetas() // the analyses' folder, once a tick too
	}
	k := foundKey{query: a.paletteQuery, mode: a.paletteMode, edits: a.edits, maps: a.memo.maps, scans: a.memo.scans,
		tasks: a.tasks.serial, lib: a.lib, version: a.lib.Version, metas: a.mapMetaVersion, dias: a.diaVersion,
		doc: a.doc, selected: a.selected, attach: a.attachRef, libOpen: a.libOpen, day: today(a.now()),
		keys: [3]string{a.captureKey(), a.clipKey(), a.shotKey()}}
	if a.doc != nil {
		k.focus = a.focus[a.doc.ID]
	}
	return k, true
}

// paletteFind finds the palette's lines (paletteItems keeps them).
func (a *app) paletteFind() []paletteItem {
	q := strings.ToLower(strings.TrimSpace(a.paletteQuery))
	match := func(s string) bool { return q == "" || strings.Contains(strings.ToLower(s), q) }
	var items []paletteItem
	if a.paletteMode == paletteAll {
		for _, cmd := range a.commands() {
			if (cmd.ok == nil || cmd.ok()) && match(cmd.title) {
				items = append(items, paletteItem{"命令", cmd.title, cmd.keys, cmd.run, nil})
			}
		}
		// The tasks not done of every map, to do something with at once.
		if q != "" {
			n := 0
			for _, it := range a.taskItems() {
				if n == 8 {
					break
				}
				if it.done() || !strings.Contains(strings.ToLower(it.node.Title), q) && !strings.Contains(strings.ToLower(it.node.Note), q) {
					continue
				}
				n++
				where := it.mapName
				if d := it.date(); d != nil {
					where += " · " + dayLabel(*d, a.now())
				}
				items = append(items, paletteItem{kind: "任务", title: it.node.Title, detail: where, task: &it, run: func() {
					a.openTasks()
					a.tasks.view, a.tasks.onMap = tasksAll, ""
					a.showTask(it)
					a.tasks.reveal = it.key()
				}})
			}
		}
		if q != "" && a.file != nil {
			items = append(items, a.nodeItems(a.path, a.file, q, 30, a.goToNode)...)
		}
	}
	if a.paletteMode == paletteAttach {
		ref := a.attachRef
		what := "资料"
		if strings.HasPrefix(ref, skillScheme) {
			what = "技能"
		}
		attach := func(path, _, id string) {
			if err := a.attachTo(path, id, ref, what); err != nil {
				a.saveErr = err.Error()
			}
		}
		if q == "" { // the likeliest places first
			if c := a.lib.Find(strings.TrimPrefix(ref, clipScheme)); c != nil {
				for _, p := range a.placesFor(c, 8) {
					p := p
					items = append(items, paletteItem{"建议", p.title, "", func() { attach(p.path, p.sheet, p.id) }, nil})
				}
			}
		}
		for _, f := range a.files {
			file := a.mapFile(f.Path)
			if file == nil {
				continue
			}
			if q == "" { // and every map's center
				d := file.Sheets[0]
				path, sheet, id := f.Path, d.ID, d.Root.ID
				items = append(items, paletteItem{"导图", d.Root.Title, mapName(path), func() { attach(path, sheet, id) }, nil})
				continue
			}
			items = append(items, a.nodeItems(f.Path, file, q, 200-len(items), attach)...)
			if len(items) >= 200 {
				break
			}
		}
	}
	if a.paletteMode == paletteClip && a.lib != nil {
		for i, h := range a.lib.Search(a.paletteQuery, nil) {
			if i == 100 {
				break
			}
			id := h.Clip.ID
			items = append(items, paletteItem{"资料", h.Clip.Label(), library.Mask(cmpOr(h.Clip.Desc, h.Clip.Category)), func() {
				if at := a.floatAt; at != nil { // onto the canvas, where its menu was opened
					a.addFloatingClip(id, *at)
				} else if n := a.file.Node(a.linkFrom); n != nil {
					a.addLink(n, clipScheme+id)
				}
			}, nil})
		}
	}
	if a.paletteMode == paletteSkill {
		for _, e := range a.skillsNamed(q, 100) {
			name := e.Name
			items = append(items, paletteItem{"技能", name, e.About, func() {
				if n := a.file.Node(a.linkFrom); n != nil {
					a.addLink(n, skillScheme+name)
				}
			}, nil})
		}
	}
	if a.paletteMode == paletteAll && q != "" || a.paletteMode == paletteMaps {
		files := a.recentFiles()
		if q != "" {
			files = a.files // all of them, by name or by what their analysis says
		}
		for _, f := range files {
			name, path := mapName(f.Path), f.Path
			an := a.analysisOf(path)
			if match(name) || an.Matches(q) {
				where := a.relDir(path)
				if an != nil && an.Desc != "" {
					where = an.Desc
				}
				items = append(items, paletteItem{"导图", name, where, func() { a.openMap(path) }, nil})
			}
		}
	}
	if a.paletteMode == paletteAll && q != "" && a.lib != nil {
		for i, h := range a.lib.Search(a.paletteQuery, nil) {
			if i == 5 || h.Coverage < 0.5 {
				break
			}
			id := h.Clip.ID
			items = append(items, paletteItem{"资料", h.Clip.Label(), library.Mask(cmpOr(h.Clip.Desc, h.Clip.Category)), func() {
				a.libOpen, a.libSel = true, id
			}, nil})
		}
	}
	// #标签: what carries the tag, wherever it is: notes and clips of the
	// collection, and topics of every map.
	if tag, ok := strings.CutPrefix(strings.TrimSpace(a.paletteQuery), "#"); ok && a.paletteMode == paletteAll {
		tag = strings.ToLower(strings.TrimSpace(tag))
		items = nil // a tag is asked for: the commands and names give way
		has := func(tags []string) bool {
			for _, x := range tags {
				if x := strings.ToLower(x); tag != "" && (x == tag || strings.HasPrefix(x, tag)) {
					return true
				}
			}
			return false
		}
		if a.lib != nil {
			for _, cl := range a.lib.Clips {
				if len(items) >= 60 {
					break
				}
				if has(cl.Tags) || has(memoTags(cl.Text)) {
					id, kind := cl.ID, "资料"
					if isMemo(cl) {
						kind = "灵感"
					}
					items = append(items, paletteItem{kind, firstSentence(cl.Label()), "#" + strings.Join(cl.Tags, " #"), func() { a.showClip(id) }, nil})
				}
			}
		}
		for _, f := range a.files {
			file := a.file
			if f.Path != a.path {
				file = a.indexed(f.Path)
			}
			if file == nil {
				continue
			}
			for _, d := range file.Sheets {
				d.Walk(func(n, _ *model.Node, _ int) bool {
					if len(items) < 200 && has(n.Tags) {
						path, sheet, id := f.Path, d.ID, n.ID
						items = append(items, paletteItem{"主题", n.Title, mapName(path) + " · #" + strings.Join(n.Tags, " #"), func() { a.goToNode(path, sheet, id) }, nil})
					}
					return true
				})
			}
		}
		return items
	}
	place := a.moveToMap // what a topic chosen as a place does
	if a.paletteMode == paletteGrow {
		place = a.growUnder
	}
	if (a.paletteMode == paletteMove || a.paletteMode == paletteGrow) && q == "" { // the maps' roots
		for _, f := range a.recentFiles() {
			file := a.file
			if f.Path != a.path {
				file = a.indexed(f.Path)
			}
			if file == nil {
				continue
			}
			for _, d := range file.Sheets {
				path, sheet, id := f.Path, d.ID, d.Root.ID
				where := mapName(path)
				if len(file.Sheets) > 1 {
					where += " · " + d.Title
				}
				items = append(items, paletteItem{"导图", d.Root.Title, where, func() { place(path, sheet, id) }, nil})
			}
		}
	}
	if (a.paletteMode == paletteSearch || a.paletteMode == paletteLink || a.paletteMode == paletteMove || a.paletteMode == paletteGrow) && q != "" {
		act := a.goToNode
		if a.paletteMode == paletteMove || a.paletteMode == paletteGrow {
			act = place
		}
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
			items = append(items, paletteItem{"主题", title, where, func() { act(path, sheet, id) }, nil})
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
	// The map may be the one open already, under a place's page: the
	// page gives way to it, or the node is chosen where it cannot be seen.
	a.libOpen = false
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

// recentFiles are the maps, those opened last first: put in that order
// once for the folder as scanned and the list of those opened, and given
// as a copy, which a caller may sort or cut (a frame asked two or three
// times, each a sort).
func (a *app) recentFiles() []model.FileInfo {
	if a.memo.recentFor != a.memo.scans || !slices.Equal(a.memo.recentOrder, a.cfg.Recent) || a.memo.scans == 0 {
		a.memo.recentFor, a.memo.recentOrder = a.memo.scans, slices.Clone(a.cfg.Recent)
		a.memo.sorted++
		rank := make(map[string]int, len(a.cfg.Recent))
		for i, p := range a.cfg.Recent {
			if _, ok := rank[p]; !ok {
				rank[p] = i
			}
		}
		at := func(p string) int {
			if i, ok := rank[p]; ok {
				return i
			}
			return len(a.cfg.Recent)
		}
		a.memo.recent = slices.Clone(a.files)
		slices.SortStableFunc(a.memo.recent, func(x, y model.FileInfo) int { return at(x.Path) - at(y.Path) })
	}
	return slices.Clone(a.memo.recent)
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

// fileLooks counts the looks at a file's time (modTime), for the tests: a
// frame of a page is to take none.
var fileLooks int

// memo is what is worked out once and shown for many frames; each part
// says what has it made again.
type memo struct {
	// scans counts the looks at the maps' folder (refreshFiles); maps the
	// maps read anew (indexed): what is worked out of the maps not open
	// stands while neither moved, of the open one while a.edits did not.
	scans, maps int
	// What waits for a place (waiting), and today's tasks (dueToday,
	// doneToday), of the tasks as gathered and the library as it stands;
	// waited counts the times it was worked out, for the tests.
	waitFor  waitKey
	waitList []waitingItem
	waited   int
	dayFor   dayKey
	due      []taskItem
	done     []taskItem
	// What the palette and the bar found (foundIn), each of what was asked
	// of it; metasLooked is when the analyses' folder was last looked at
	// for them.
	found       [2]found
	finds       int // counts the searches, for the tests
	metasLooked lookedAt
	// The maps opened last first (recentFiles), of the folder as scanned
	// and the list of those opened; sorted counts the sorts, for the tests.
	sorted      int
	recentFor   int
	recentOrder []string
	recent      []model.FileInfo
	// The maps' faces on 项目's cards (mapFaceOf), by path; faced counts
	// those worked out, for the tests.
	faces map[string]*mapFace
	faced int
	// The tasks put on the hours' days (hoursLaid) and on the month's
	// (monthLaid); laid counts the times, for the tests.
	hours hoursLaid
	month monthLaid
	laid  int
	// holidaysAt is when it was last looked whether a year's notice of the
	// holidays is due (holidaysFetch).
	holidaysAt time.Time
}

// lookedAt is a moment the maps' files are looked at: the tick's (every
// two seconds, a.checked) and the folder's last scan. A file is not
// looked at twice in one: a frame draws a map's dot a hundred times.
type lookedAt struct {
	tick  time.Time
	scans int
}

func (a *app) looked() lookedAt { return lookedAt{a.checked, a.memo.scans} }

// indexed is a map read for the search, kept until its file changes: its
// file's time is looked at once a tick (looked), and at once after the
// app wrote a map or scanned the folder; what another program wrote shows
// by the next tick. Before the first tick nothing is trusted.
func (a *app) indexed(path string) *model.File {
	at := a.looked()
	e, ok := a.index[path]
	if ok && e.at == at && !at.tick.IsZero() {
		return e.file
	}
	mt := modTime(path)
	if !ok || !e.mod.Equal(mt) {
		f, err := model.Load(path)
		if err != nil {
			f = nil
		}
		if e.file != nil || f != nil {
			a.memo.maps++
		}
		e = indexEntry{mod: mt, file: f}
		if err != nil && !mt.IsZero() {
			e.mod = time.Time{} // not read: tried again at the next look, it may be half written
		}
	}
	e.at = at
	if a.index == nil {
		a.index = map[string]indexEntry{}
	}
	a.index[path] = e
	return e.file
}

type indexEntry struct {
	mod  time.Time
	file *model.File
	at   lookedAt // when its file's time was last looked at
}

// paletteView shows the palette over the window.
func (a *app) paletteView(c *ui.Context) {
	t := c.Theme()
	modal(c, "palette", &a.paletteOpen, func() {
		items := a.paletteItems()
		if a.paletteIndex >= len(items) {
			a.paletteIndex = max(len(items)-1, 0)
		}
		placeholder := map[int]string{
			paletteAll:    "搜索导图、主题、灵感和资料，#标签 按标签找，或输入命令…",
			paletteMaps:   "切换到导图…",
			paletteSearch: "搜索所有导图里的主题、备注和标签…",
			paletteLink:   "输入关键词，选择要链接到的主题…",
			paletteMove:   "移到哪个主题下？输入关键词查找，或选择导图的中心主题…",
			paletteClip:   "在收集里查找要链接的资料…",
			paletteAttach: "把资料挂到哪个主题？输入关键词查找，或选择建议的主题…",
			paletteSkill:  "查找要链接的 AI 技能…",
			paletteGrow:   "长到哪个主题下？输入关键词查找，或选择导图的中心主题…",
		}[a.paletteMode]
		if a.paletteMode == paletteAttach && strings.HasPrefix(a.attachRef, skillScheme) {
			placeholder = "哪个主题要用这个技能？输入关键词查找，或选一张导图的中心主题…"
		}
		if a.paletteMode == paletteClip && a.floatAt != nil {
			placeholder = "在收集里查找要放到画布上的那一条…"
		}
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
						row := ui.Row(c.Key(fmt.Sprint("item", i))).Padding(8, 12).Gap(10).Radius(radiusRow).AlignItems(ui.Center).
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
							if it.task != nil { // a task: ticked off and put on tomorrow here
								task := *it.task
								a.taskCircle(c, task, 16)
								ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
									ui.Text(c, it.title).FontSize(14).MaxLines(1)
									ui.Text(c, it.detail).FontSize(11).TextColor(t.TextMuted).MaxLines(1)
								})
								if ghostButton(c, "明天", "改到明天").Label("改到明天 " + task.node.Title).Clicked() {
									d := today(a.now()).AddDate(0, 0, 1)
									a.later(func() { a.moveTaskDay(task, &d) }) // said, with the way back
								}
								return
							}
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
		{"⌘T", "新建画布"}, {"⌘1–6", "导图 / 大纲 / 看板 / 时间线 / 总览 / 图解"}, {"⌘↩", "开始演说"}, {"⌘?", "快捷键速查"},
		{"⌘⌥I", "显示 / 收起主题面板"}, {"⌘⇧H", "历史版本"}, {"⌘F，然后 ↩", "在导图里搜索，跳到下一个"},
	}},
	{"今天和收集", [][2]string{
		{"⌥⌘1 / ⌥⌘2 / ⌥⌘3 / ⌥⌘4", "今天 / 项目 / 收集 / 技能"}, {"⌘J", "记一条灵感或任务"}, {"[[", "写灵感或主题备注时：链接到灵感、资料、导图或主题"}, {"↑ ↓", "在任务或卡片间移动，详情跟着换"},
		{"← →", "收集里到旁边一栏的卡片"}, {"Space", "勾掉选中的任务"}, {"Esc", "关闭右侧的详情"},
		{"↩ / ⇧↩", "输入条和添加任务的一行：记下 / 换行"}, {"⌘↩", "写灵感的卡片和主题备注：记下 / 完成"},
		{"双击周视图里某一天的表头", "只看这一天"}, {"在时间格上按住往下拖", "新建任务，拖过多长就是多长"},
	}},
	{"编辑主题", [][2]string{
		{"Tab", "添加子主题"}, {"↩ / ⇧↩", "在下方 / 上方添加同级主题"}, {"Space / F2", "编辑标题"}, {"Delete", "删除"},
		{"⌘C / ⌘X / ⌘V", "复制 / 剪切 / 粘贴（可跨导图，也可粘贴文本大纲）"}, {"⌘Z / ⌘⇧Z", "撤销 / 重做"},
		{"⌘↑ / ⌘↓", "上移 / 下移（也可拖到同级主题的上沿或下沿）"}, {"⌘⇧L", "关联线：连到另一个主题"}, {"⌘/", "折叠 / 展开"}, {"⇧ 或 ⌘ + 点击", "多选主题"},
	}},
	{"浏览", [][2]string{
		{"← → ↑ ↓", "在主题间移动"}, {"⌥↩", "聚焦所选分支"}, {"Esc", "取消多选 / 退出聚焦"},
		{"⌥1–9 / ⌥0", "展开到第 N 层 / 全部展开"}, {"⌘= / ⌘- / ⌘0", "放大 / 缩小 / 实际大小"}, {"⌘⇧0", "回到中心"},
		{"双指捏合 / ⌘ + 滚轮", "缩放"},
	}},
	{"图解", [][2]string{
		{"← → ↑ ↓", "在方框、表格的行、名词之间移动；讲一遍时是上一步 / 下一步"}, {"Esc", "退出讲一遍、走例子或考我；再按取消选中；再按回到导图"},
	}},
	{"导入导出", [][2]string{
		{"⌘⇧E", "导出为图片"}, {"⌘⇧C", "复制当前画布为 Markdown"}, {"⌘⇧I", "导入 XMind"},
	}},
}

// cheatSheet shows every shortcut.
func (a *app) cheatSheet(c *ui.Context) {
	t := c.Theme()
	modal(c, "cheats", &a.cheatsOpen, func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "快捷键").FontSize(18).Bold().Grow(1)
			if iconButton(c, iconClose, "关闭", "关闭", 28, true).Clicked() {
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
