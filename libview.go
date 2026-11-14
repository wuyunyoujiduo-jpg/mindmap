package main

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
)

// libraryView is the library in the main area: a field to keep text and
// links, the filters, the clips found, and the clip chosen.
func (a *app) libraryView(c *ui.Context) {
	l := a.lib
	root := ui.Column(c.Key("library")).Grow(1).Label("收集")
	if files := root.DroppedFiles(); len(files) > 0 {
		a.later(func() { a.dropClips(files) })
	}
	sel := l.Find(a.libSel)
	root.Children(func() {
		// One calm column in the middle of the page; with a clip chosen, the
		// list and, beside it, the clip, the page's height.
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
				// The wall takes the room there is: more columns in a wide window.
				pageColumn(c, "lib-column", true).MaxWidth(100000).Children(func() { a.libraryColumn(c) })
			})
			if sel != nil {
				// The clip chosen, in a column at the page's right.
				sheet(c, "clip-sheet", 420, func() {
					ui.Scroll(c.Key("clip-detail:"+sel.ID)).Grow(1).Basis(0).Padding(14, 18, 20).Gap(12).Children(func() {
						a.clipDetail(c, sel)
					})
				})
			}
		})
	})
	a.libraryDialogs(c)
}

// libraryColumn is the library but the clip chosen: its heading, the line
// to keep something, the filters and the clips found.
func (a *app) libraryColumn(c *ui.Context) {
	t := c.Theme()
	l := a.lib
	func() {
		memos := 0
		for _, cl := range l.Clips {
			if ofKind(cl, kindMemos) {
				memos++
			}
		}
		pageHead(c, "lib-head", "收集", "灵感和资料", []figure{{len(l.Clips), "条"}, {memos, "条灵感"}, {len(l.Unorganized()), "条未整理"}}, func() {
			// What is being organized and indexed is told in cards at the
			// window's top right (jobsView), and on the clip itself.
			if n := len(l.Broken); n > 0 {
				ui.Text(c, fmt.Sprintf("%d 条资料的文件损坏", n)).FontSize(12).TextColor(t.Danger).NoWrap().
					Tooltip("这些文件无法读取，已跳过：.library/clips/ 下的 " + strings.Join(l.Broken, "、"))
			}
			if a.embedErr != "" {
				ui.Text(c, "语义检索不可用").FontSize(12).TextColor(t.Warning).NoWrap().Tooltip(a.embedErr + "（在设置里检查向量模型）")
			}
			search := searchBox(c, "lib-search", &a.libQuery, "搜索资料", "搜索收集", "按标题、内容、摘要、标签和关键词查找资料", 200)
			if a.libQuery != "" && search.Shortcut(0, ui.KeyEscape) {
				a.libQuery = ""
			}
			if search.Changed() {
				a.libQueryAt = a.now()
			}
			a.searchQueryVector(c)
		})
		a.pluginBanner(c)
		a.clipFilters(c)
		if a.libErr != "" {
			ui.Row(c).Padding(0, 0, 6).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.libErr).FontSize(12).TextColor(t.Danger).Wrap().Grow(1)
				if ui.Link(c, "知道了", "").FontSize(12).Clicked() {
					a.libErr = ""
				}
			})
		}
		a.clipPlaceBar(c)
		hits := a.libHits()
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			if len(hits) == 0 {
				ui.Column(c).Grow(1).Basis(0).Padding(8, 0, 16).Children(func() {
					icon, title := "lightbulb", "还没有资料"
					msg := "在上面记点什么、粘贴链接，或把图片和文件拖进来。\n在任何应用里复制内容后按 " + keyLabel(a.clipKey()) + "，也能直接存到收集。"
					if len(l.Clips) > 0 || a.libKind == kindMaps {
						icon, title, msg = "search", "没有符合条件的资料", "换一个分类或标签，或者清掉搜索的词。"
					}
					if a.libReview != reviewOff {
						icon, title, msg = "history", "还没有可以回顾的", "一周以前的灵感会在这里，每天换几条。"
					}
					emptyState(c, "wall", icon, title, msg, nil)
				})
			} else {
				a.clipWall(c, hits)
			}
		})
	}()
}

// wallStep is how many clips the wall shows at first, and more at a time.
const wallStep = 120

// clipWall is the wall of cards: three columns, one beside a clip chosen,
// each card as tall as what it says and put in the column that is
// shortest, as notes pinned to a wall are, so that a short note leaves no
// hole under it. The newest are at the top, read across.
func (a *app) clipWall(c *ui.Context, hits []library.Hit) {
	// As many columns as the room has, each about a card wide: three in
	// an ordinary window, five on a large screen, one or more beside a
	// clip chosen. A card is never stretched across a wide window.
	w, _ := c.Size()
	room := w - 12 - 56 - 20
	if a.sideShown() {
		room -= sideWidthOf
	}
	if a.lib.Find(a.libSel) != nil {
		room -= min(420, w*0.4)
	}
	cols := min(max(int(room/300), 1), 6)
	shown := min(len(hits), max(a.wallShown, wallStep))
	// The arrows go from a card to the next and the one before, its
	// details following; not while something is written in.
	if !a.barOpen && !c.FocusTakesText() && shown > 0 {
		step := 0
		switch {
		case c.Shortcut(0, ui.KeyDown), c.Shortcut(0, ui.KeyRight):
			step = 1
		case c.Shortcut(0, ui.KeyUp), c.Shortcut(0, ui.KeyLeft):
			step = -1
		}
		if step != 0 {
			at := -1
			for i, h := range hits[:shown] {
				if h.Clip.ID == a.libSel {
					at = i
				}
			}
			switch {
			case at < 0 && step > 0:
				at = 0
			case at < 0:
				at = shown - 1
			default:
				at = min(max(at+step, 0), shown-1)
			}
			a.libSel, a.libTagText = hits[at].Clip.ID, ""
			a.libReveal = a.libSel
		}
	}
	tall := make([]float32, cols)
	in := make([][]*library.Clip, cols)
	for _, h := range hits[:shown] {
		k := 0
		for j := range tall {
			if tall[j] < tall[k] {
				k = j
			}
		}
		in[k] = append(in[k], h.Clip)
		tall[k] += clipTall(h.Clip, cols) + 8
	}
	ui.Scroll(c.Key(fmt.Sprint("clips:", cols))).Grow(1).Basis(0).Children(func() {
		// The cards stand apart on the page, each told by a hairline: with
		// columns of uneven ends one gray ground under them all showed as
		// holes, and the user asked for them apart.
		ui.Row(c.Key("ground")).Padding(2, 1).Gap(12).AlignItems(ui.Start).FillWidth().Children(func() {
			for k := range cols {
				ui.Column(c.Key(k)).Grow(1).Basis(0).MinWidth(0).Gap(12).Children(func() {
					for _, clip := range in[k] {
						if clip.Kind == kindMap {
							ui.Box(c.Key(clip.ID)).FillWidth().Children(func() { a.mapRow(c, clip) })
						} else {
							a.clipRow(c.Key(clip.ID), clip)
						}
					}
				})
			}
		})
		if rest := len(hits) - shown; rest > 0 {
			ui.Column(c).Padding(12, 0, 4).AlignItems(ui.Center).FillWidth().Children(func() {
				if textKey(c, fmt.Sprintf("再显示 %d 条", min(rest, wallStep)), "再显示", "还有 "+fmt.Sprint(rest)+" 条").Clicked() {
					a.wallShown = shown + wallStep
				}
			})
		}
		ui.Box(c).Height(24)
	})
}

// clipTall is about how tall a clip's card is on a wall of cols columns:
// enough to tell which column is shortest.
func clipTall(clip *library.Clip, cols int) float32 {
	per := 24 // the characters of a line
	if cols == 1 {
		per = 44
	}
	lines := func(s string, most int) float32 {
		n := 0
		for _, l := range strings.Split(strings.TrimSpace(s), "\n") {
			n += 1 + len([]rune(l))/per
		}
		return float32(min(n, most))
	}
	if isMemo(clip) {
		return 32 + lines(clip.Text, 7)*24 + 34
	}
	h := float32(32 + 24 + 34)
	if clip.Kind == library.KindImage {
		h += 120
	}
	if sub := cmpOr(cmpOr(cmpOr(clip.Desc, clip.Summary), cmpOr(clip.Note, clip.Text)), clip.URL); sub != "" {
		h += lines(sub, 3) * 22
	}
	return h
}

// libraryDialogs are the library's questions.
func (a *app) libraryDialogs(c *ui.Context) {
	gone := "删除后无法恢复。"
	if n := len(a.clipBacklinks(a.libSel)); n > 0 {
		gone = fmt.Sprintf("它挂在 %d 个节点上，删除后这些链接会失效。删除后无法恢复。", n)
	}
	// What nothing links to goes at once, with a way back; what a topic
	// links to is asked about first, for the links would break.
	if a.libConfirm && len(a.clipBacklinks(a.libSel)) == 0 {
		a.libConfirm = false
		a.deleteClipUndoably(a.libSel)
		return
	}
	switch ui.AlertDialog(c, &a.libConfirm, "删除这条资料？", gone, "取消", "删除") {
	case 1:
		a.deleteClipUndoably(a.libSel)
	}
}

// deleteClipUndoably deletes a clip and says so, with the way to take it
// back.
func (a *app) deleteClipUndoably(id string) {
	restore, err := a.lib.Take(id)
	if err != nil {
		a.libErr = err.Error()
		return
	}
	a.noteLibraryWrite()
	if a.libSel == id {
		a.libSel = ""
	}
	a.tellUndo("已删除", func() {
		if err := restore(); err != nil {
			a.libErr = err.Error()
			return
		}
		a.noteLibraryWrite()
	})
}

// libHits are the clips the search and the filters find.
func (a *app) libHits() []library.Hit {
	e := a.embedder()
	q := strings.TrimSpace(a.libQuery)
	a.mapClips()
	k := libHitsKey{a.libQuery, a.libKind, a.libCat, a.libTag, a.lib, a.lib.Version, e.Name(), a.qvecs[q] != nil, a.mapClipsSig, a.libDay,
		a.libReview, a.libWalk, today(a.now())}
	if a.libHitsFor == k {
		return a.libHitsList
	}
	a.libHitsFor, a.libHitsList = k, a.searchClips()
	return a.libHitsList
}

// libHitsKey is what the clips found depend on.
type libHitsKey struct {
	query    string
	kind     int
	cat, tag string
	lib      *library.Library
	version  int
	model    string
	vector   bool
	maps     string // the maps' state (mapClips)
	day      time.Time
	review   int // the way of looking back, its turn and its day
	walk     int
	today    time.Time
}

func (a *app) searchClips() []library.Hit {
	if a.libReview != reviewOff { // looking back: a few old notes, whatever is filtered
		var hits []library.Hit
		for _, c := range a.reviewClips() {
			hits = append(hits, library.Hit{Clip: c})
		}
		return hits
	}
	keep := func(c *library.Clip) bool {
		switch {
		case !ofKind(c, a.libKind):
			return false
		case !a.libDay.IsZero() && !sameDay(dayOf(c).In(a.libDay.Location()), a.libDay):
			return false
		case c.Kind == kindMap && (a.libCat == "-" || a.libCat == "★"):
			return false
		case a.libCat == "-" && !c.NeedsOrganizing():
			return false
		case a.libCat == "★" && !c.Starred:
			return false
		case a.libCat != "" && a.libCat != "-" && a.libCat != "★" && c.Category != a.libCat:
			return false
		case a.libTag != "" && !slices.Contains(c.Tags, a.libTag):
			return false
		}
		return true
	}
	if a.libKind == kindMaps {
		return a.searchMaps(a.libQuery, keep)
	}
	var hits []library.Hit
	if e := a.embedder(); e.On() {
		hits = a.lib.SearchBoth(a.libQuery, a.qvecs[strings.TrimSpace(a.libQuery)], e.Name(), keep)
	} else {
		hits = a.lib.Search(a.libQuery, keep)
	}
	// The maps are not material: the sidebar, 今天 and ⌘K have them.
	return hits
}

// dropClips keeps files dropped on the library.
func (a *app) dropClips(files []string) {
	for _, f := range files {
		if info, err := os.Stat(f); err != nil || info.IsDir() {
			continue
		}
		clip, err := a.addClipFile(f, "拖入")
		if err != nil {
			a.libErr = err.Error()
			return
		}
		a.libSel = clip.ID
	}
}

// clipFilters are what the wall is narrowed by: the kinds, with the ways
// to look back beside them; then the tags, the most used first.
func (a *app) clipFilters(c *ui.Context) {
	t := c.Theme()
	// Which kind and which tag show is chosen in the left column (libSide);
	// here, what narrows further, and how many are found.
	ui.Row(c.Key("lib-tags")).Padding(2, 0, 8).MinHeight(40).Gap(6).Wrap().GapY(6).AlignItems(ui.Center).Children(func() {
		chip := func(key, label string) bool {
			b := ui.Box(c.Key("cat:"+key)).Height(26).Padding(0, 10).Radius(radiusSmall).Center().Cursor(ui.CursorPointer).Label(label).
				Background(t.Accent.Alpha(0.12)).Transition(easeLook)
			b.Children(func() {
				ui.Text(c, label).FontSize(textCaption).FontWeight(700).TextColor(t.Accent.MixOklab(t.Text, 0.3)).NoWrap()
			})
			return b.Clicked()
		}
		if a.libReview != reviewOff {
			name := map[int]string{reviewDaily: "每日回顾", reviewWalk: "随机漫步"}[a.libReview]
			if chip("review", name+" ✕") {
				a.libReview = reviewOff
			}
			if a.libReview == reviewWalk && softButton(c, "换一条", "再随机看一条").Clicked() {
				a.libWalk++
			}
			return
		}
		if a.cfg.HideTree && a.libTag != "" && chip("tag-chosen", "#"+a.libTag+" ✕") { // with no left column to show it
			a.libTag = ""
		}
		if !a.libDay.IsZero() && chip("day", a.libDay.Format("1月2日")+" ✕") {
			a.libDay = time.Time{}
		}
		ui.Spacer(c)
		a.materialBar(c)
	})
}

func kindIcon(kind string) icon {
	switch kind {
	case library.KindLink:
		return iconLink
	case library.KindImage:
		return iconImage
	case library.KindFile:
		return iconFile
	}
	return iconText
}

// clipRow is a clip in the list.
func (a *app) clipRow(c *ui.Context, clip *library.Clip) {
	t := c.Theme()
	memo := isMemo(clip)
	kcol := clipColor(clip.Kind)
	row := ui.Column(c).FillWidth().Padding(16, 18).Gap(12).Radius(radiusCard).Cursor(ui.CursorPointer).
		Label("资料 " + clip.Label()).Drag(clipDrag{clip.ID})
	// A white card, as all are: a note is its text, the others have their
	// kind's icon by their title.
	wallCard(row, t)
	if a.libSel == clip.ID {
		chosenCard(row, t)
		if a.libReveal == clip.ID { // gone to by the keyboard
			row.ScrollIntoView()
			a.libReveal = ""
		}
	}
	if row.Clicked() {
		if a.libSel == clip.ID {
			a.libSel = ""
		} else {
			a.libSel, a.libTagText = clip.ID, ""
		}
	}
	row.ContextMenu(func(m *ui.Menu) { a.clipMenu(m, clip) })
	row.Children(func() {
		if memo {
			text := clip.Text
			if r := []rune(text); len(r) > 600 {
				text = string(r[:600]) + "…"
			}
			ui.Column(c).Gap(6).FillWidth().PassThrough().Children(func() {
				a.noteText(c, library.Excerpt(text, a.libQuery, 400), 15, 7)
			})
		} else {
			ui.Row(c).Gap(10).AlignItems(ui.Center).PassThrough().Children(func() {
				glyph(c, kindTile(clip.Kind), 16, t.TextMuted)
				ui.Text(c, clip.Label()).FontSize(14).FontWeight(700).Grow(1).Shrink(1).MinWidth(0).MaxLines(2)
				if clip.Starred {
					glyph(c, iconStarFilled, 12, t.Warning)
				}
			})
			if clip.Kind == library.KindImage {
				if b := a.fileBitmap(a.lib.FilePath(clip)); b != nil {
					ui.Image(c, b).Height(110).FillWidth().Fit(ui.Cover).Radius(8).PassThrough()
				}
			}
			sub := cmpOr(clip.Desc, clip.Summary)
			if sub == "" {
				sub = cmpOr(clip.Note, clip.Text)
			}
			if sub == "" {
				sub = clip.URL
			}
			if label := clip.Label(); clip.Title == "" && !strings.HasSuffix(label, "…") && strings.TrimSpace(sub) == label {
				sub = "" // the label says it all
			}
			if sub != "" {
				if r := []rune(sub); len(r) > 3000 { // the cards are drawn every frame
					sub = string(r[:3000])
				}
				ui.Text(c, library.Excerpt(sub, a.libQuery, 140)).FontSize(textBody).LineHeight(1.55).TextColor(t.TextMuted).Wrap().MaxLines(3).PassThrough()
			}
		}
		a.clipPlaceLine(c, clip)
		ui.Row(c).Gap(6).Wrap().GapY(4).AlignItems(ui.Center).Children(func() {
			_, ink := tinted(t, kcol)
			kind := library.KindLabel(clip.Kind)
			if memo {
				kind = "灵感"
			}
			mapDot(c, kcol, 6)
			ui.Text(c, kind).FontSize(textCaption).FontWeight(700).TextColor(ink).NoWrap()
			ui.Text(c, clip.Created.Format("1/2 15:04")).FontSize(textCaption).TextColor(t.TextMuted).NoWrap()
			for i, tag := range clip.Tags {
				if i == 3 {
					break
				}
				b := ui.Text(c.Key("t"+tag), "#"+tag).FontSize(textCaption).TextColor(t.TextMuted).NoWrap().Cursor(ui.CursorPointer)
				if b.Clicked() {
					a.libTag = tag
				}
			}
			if s, ok := a.libStatus[clip.ID]; ok {
				col := t.TextMuted
				if strings.Contains(s, "失败") {
					col = t.Danger
				}
				ui.Text(c, s).FontSize(textCaption).TextColor(col).SingleLine().Shrink(1).MinWidth(0)
			}
			// The map it is on, by its color.
			if back := a.clipBacklinks(clip.ID); len(back) > 0 {
				ui.Spacer(c)
				ui.Row(c).Gap(5).AlignItems(ui.Center).Shrink(1).MinWidth(0).Children(func() {
					mapDot(c, a.mapColor(back[0].path), 7)
					ui.Text(c, "在 "+mapName(back[0].path)).FontSize(textCaption).TextColor(t.TextMuted).SingleLine()
				})
			}
		})
	})
}

// clipColor is the color of a kind of clip: amber notes, blue links,
// rose pictures, green documents.
func clipColor(kind string) ui.Color {
	switch kind {
	case library.KindText:
		return ui.Hex("#E8A13A")
	case library.KindLink:
		return ui.Hex("#3B8FE0")
	case library.KindImage:
		return ui.Hex("#D95C8C")
	}
	return ui.Hex("#3DAA7D")
}

// fileBitmap is the picture of a file, decoded once.
func (a *app) fileBitmap(path string) *ui.Bitmap {
	if path == "" {
		return nil
	}
	if b, ok := a.bitmaps[path]; ok {
		return b
	}
	var b *ui.Bitmap
	if data, err := os.ReadFile(path); err == nil {
		b, _ = ui.DecodeBitmap(data)
	}
	a.bitmaps[path] = b
	return b
}

// clipMenu is the menu of a clip.
func (a *app) clipMenu(m *ui.Menu, clip *library.Clip) {
	if clip.URL != "" && m.Item("打开链接").Chosen() {
		a.plat.OpenURL(clip.URL)
	}
	if p := a.lib.FilePath(clip); p != "" && m.Item("打开文件").Chosen() {
		a.plat.OpenPath(p)
	}
	if m.Item("生成分享卡片…").Chosen() {
		a.openCard(clip)
	}
	if m.Item("复制资料链接").Chosen() {
		a.plat.CopyText(clipScheme + clip.ID)
		a.tell("已复制，可以粘贴到节点的链接里")
	}
	if m.Item("复制内容").Chosen() {
		a.plat.CopyText(clipCopyText(clip))
		a.tell("已复制")
	}
	star := "加星标"
	if clip.Starred {
		star = "取消星标"
	}
	if m.Item(star).Chosen() {
		clip.Starred = !clip.Starred
		a.saveClip(clip)
	}
	if m.Item("AI 重新整理").Disabled(providerKey(a.cfg) == "").Chosen() {
		a.organizeClip(clip.ID)
	}
	if a.doc != nil && m.Item("加到当前导图").Chosen() {
		a.clipIntoMap(clip)
	}
	if clip.Kept && m.Item("放回「刚记下的」").Chosen() {
		a.later(func() { a.unkeep(clip) })
	}
	if m.Item("转成任务").Chosen() {
		a.later(func() { a.clipToTask(clip) })
	}
	m.Separator()
	if m.Item("删除").Chosen() {
		a.libSel, a.libConfirm = clip.ID, true
	}
}

// clipCopyText is what copying a clip puts on the clipboard.
func clipCopyText(c *library.Clip) string {
	var parts []string
	for _, s := range []string{c.Title, c.URL, c.Note, c.Text} {
		if s = strings.TrimSpace(s); s != "" && !slices.Contains(parts, s) {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "\n\n")
}

func (a *app) saveClip(c *library.Clip) {
	if err := a.lib.Save(c, a.now()); err != nil {
		a.libErr = err.Error()
	}
	a.noteLibraryWrite()
}

// clipIntoMap adds a clip to the open map and shows it there.
func (a *app) clipIntoMap(clip *library.Clip) {
	if err := a.clipToMap(clip); err != nil {
		a.libErr = err.Error()
		return
	}
	a.libOpen = false
	a.reveal = true
}

// clipDetail shows and edits the clip chosen, as a page to read: its
// title and line large and edited in place, its properties in rows, what
// the AI made of it in a card of its own, and the user's note in another.
func (a *app) clipDetail(c *ui.Context, clip *library.Clip) {
	t := c.Theme()
	// The bar: what it is and when, and what to do with it.
	ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
		tile(c, kindTile(clip.Kind), kindColor(clip.Kind), 22)
		ui.Text(c, library.KindLabel(clip.Kind)).FontSize(textCaption).Bold().TextColor(kindColor(clip.Kind)).NoWrap().Margin(0, 0, 0, 4)
		ui.Text(c, cmpOr(clip.Source, "手动")+" · "+clip.Created.Format("2006-01-02 15:04")).FontSize(textCaption).
			TextColor(t.TextMuted).SingleLine().Grow(1).Shrink(1).MinWidth(0).Margin(0, 0, 0, 4)
		star := iconStar
		if clip.Starred {
			star = iconStarFilled
		}
		if iconButton(c, star, "星标", map[bool]string{true: "取消星标", false: "加星标"}[clip.Starred], 28, true).Clicked() {
			clip.Starred = !clip.Starred
			a.saveClip(clip)
		}
		if iconButton(c, iconShare, "分享卡片", "生成分享卡片", 28, true).Clicked() {
			a.openCard(clip)
		}
		iconButton(c, iconMore, "资料菜单", "更多", 28, true).Menu(func(m *ui.Menu) { a.clipMenu(m, clip) })
		if iconButton(c, iconClose, "关闭详情", "关闭", 28, true).Clicked() || !a.barOpen && c.Shortcut(0, ui.KeyEscape) {
			a.libSel = ""
		}
	})
	status, failed := a.libStatus[clip.ID], strings.Contains(a.libStatus[clip.ID], "失败")
	// What is being read or written shows in its own part below (AI 整理),
	// in the shape of what will come: nothing above the title, which would
	// push the page down and back.
	working := status != "" && !failed

	// The title and the line saying what it is, large, edited in place.
	ui.Column(c).Gap(2).Margin(4, -6, 0).Children(func() {
		title := clip.Title
		if inlineInput(c.Key("clip-title"), &title, textHeadline, true, clip.Label(), "资料标题").Changed() {
			clip.Title = title
			a.saveClip(clip)
		}
		desc := clip.Desc
		in := inlineArea(c.Key("clip-desc"), &desc, textBody, 1, 4, "加一句说明，说清这是什么（AI 整理时自动生成）", "资料说明")
		in.TextColor(t.TextMuted)
		if in.Changed() {
			clip.Desc = desc
			a.saveClip(clip)
		}
	})

	if clip.Kind == library.KindImage {
		if b := a.fileBitmap(a.lib.FilePath(clip)); b != nil {
			w, h := b.Size()
			k := min(1, 420/float32(max(w, 1)), 320/float32(max(h, 1)))
			img := ui.Image(c, b).Size(float32(w)*k, float32(h)*k).Fit(ui.Contain).Radius(10).Cursor(ui.CursorPointer).Tooltip("用默认应用打开")
			if img.Clicked() {
				a.plat.OpenPath(a.lib.FilePath(clip))
			}
		}
	}

	// Where it is used, or where it may go.
	a.clipPlacesSection(c, clip)

	// Properties.
	sectionCard(c, iconTag, tileBlue, "信息", nil, func() {
		propRow(c, "分类", func() {
			cat := clip.Category
			in := inlineInput(c.Key("clip-cat"), &cat, textBody, false, "未分类", "资料分类")
			if in.Changed() {
				clip.Category = strings.TrimSpace(cat)
				a.saveClip(clip)
			}
		})
		propRow(c, "标签", func() {
			for _, tag := range clip.Tags {
				tag := tag
				chip := tagChip(c.Key("tag:"+tag), tag, "#"+tag, false, textSmall).Tooltip("点击筛选；右键移除")
				if chip.Clicked() {
					a.libTag = tag
				}
				chip.ContextMenu(func(m *ui.Menu) {
					if m.Item("移除标签").Chosen() {
						clip.Tags = slices.DeleteFunc(slices.Clone(clip.Tags), func(s string) bool { return s == tag })
						a.saveClip(clip)
					}
				})
			}
			in := ui.TextInputBase(c.Key("clip-tag"), &a.libTagText).Placeholder("+ 添加").Label("添加标签").
				FontSize(textSmall).Width(80).Padding(3, 6).Radius(6)
			if in.Focused() {
				in.Border(1, t.Accent.Alpha(0.6))
			} else if in.Hovered() {
				in.Background(t.Text.Alpha(0.05))
			}
			if in.Submitted() {
				for _, tag := range cleanTags(strings.Fields(strings.ReplaceAll(a.libTagText, "#", " "))) {
					if !slices.Contains(clip.Tags, tag) {
						clip.Tags = append(clip.Tags, tag)
					}
				}
				a.libTagText = ""
				a.saveClip(clip)
			}
		})
		if clip.URL != "" {
			propRow(c, "链接", func() {
				if ui.Link(c, clip.URL, "").FontSize(textSmall).MaxLines(1).Ellipsis("…").Shrink(1).MinWidth(0).Clicked() {
					a.plat.OpenURL(clip.URL)
				}
			})
		}
		if clip.Kind == library.KindFile {
			propRow(c, "文件", func() {
				if ui.Link(c, clip.Name, "").FontSize(textSmall).MaxLines(1).Ellipsis("…").Shrink(1).MinWidth(0).Clicked() {
					a.plat.OpenPath(a.lib.FilePath(clip))
				}
			})
		}
	})

	// What the AI made of it.
	sectionCard(c, iconSparkle, tilePurple, "AI 整理", func() {
		label := "整理"
		if clip.Organized != nil {
			label = "重新整理"
		}
		if a.libWorking == clip.ID { // one sign of a wait: the shapes below
		} else if ghostButton(c, label, "让 AI 起标题、写说明和摘要、分类、打标签、生成同义词和分析").Clicked() {
			a.organizeClip(clip.ID)
		}
	}, func() {
		had := clip.Summary != "" || clip.Analysis != "" || len(clip.Synonyms) > 0
		if waitShown(c, "organize:"+clip.ID, working) {
			// What it is doing, then, where nothing is known yet, the
			// shape of what it writes: two lines, three points, a line of
			// words. What was there before stays, fainter, till the new comes.
			ui.Row(c.Key("org-status")).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c).Size(12, 12)
				ui.Text(c, cmpOr(status, "AI 整理中…")).FontSize(textSmall).TextColor(t.TextMuted)
			})
			if !had {
				skeletonShapes(c, "organize", func(shade ui.Color) {
					ui.Box(c).Height(12).WidthPercent(96).Radius(6).Background(shade)
					ui.Box(c).Height(12).WidthPercent(72).Radius(6).Background(shade)
					for i, w := range []float32{58, 66, 44} {
						ui.Row(c.Key(i)).Gap(9).AlignItems(ui.Center).FillWidth().Margin(2, 0, 0).Children(func() {
							ui.Box(c).Size(5, 5).Radius(3).Shrink(0).Background(shade)
							ui.Box(c).Height(12).WidthPercent(w).Radius(6).Background(shade)
						})
					}
					ui.Box(c).Height(10).WidthPercent(36).Radius(5).Margin(6, 0, 0).Background(shade)
					ui.Box(c).Height(12).WidthPercent(62).Radius(6).Background(shade)
				})
				return
			}
		}
		// What kept it from organizing, said here where it was asked for.
		if a.libNeedKey == clip.ID && providerKey(a.cfg) == "" {
			ui.Row(c.Key("need-key")).Padding(8, 10).Gap(8).Radius(8).AlignItems(ui.Center).
				Background(t.Warning.Alpha(0.12)).Children(func() {
				ui.Text(c, "AI 整理要先填写 API Key。").FontSize(textSmall).Wrap().Grow(1)
				if inkButton(c, "填写 API Key", true).Clicked() {
					a.openKeySettings()
				}
			})
		} else if failed {
			ui.Row(c.Key("org-failed")).Padding(8, 10).Gap(8).Radius(8).AlignItems(ui.Center).
				Background(t.Danger.Alpha(0.08)).Children(func() {
				ui.Text(c, status).FontSize(textSmall).TextColor(t.Danger).Wrap().Grow(1).Selectable()
			})
		}
		if clip.Summary == "" && clip.Analysis == "" && len(clip.Synonyms) == 0 {
			if a.libNeedKey != clip.ID && !failed {
				ui.Text(c, "还没整理。点「整理」，AI 会写摘要、分析要点，并生成检索用的同义词。").FontSize(textSmall).TextColor(t.TextMuted).Wrap()
			}
			return
		}
		// A short note has no summary: one written before this was so is
		// kept, and not shown.
		if clip.Summary != "" && !briefClip(clip) {
			ui.Text(c, clip.Summary).FontSize(textBody).LineHeight(1.55).Wrap().Selectable()
		}
		if clip.Analysis != "" && !briefClip(clip) {
			markdownView(c.Key("clip-analysis"), clip.Analysis, textBody)
		}
		if a.libSynFor != clip.ID || !a.libSynEditing {
			a.libSynFor, a.libSynText = clip.ID, strings.Join(clip.Synonyms, "、")
		}
		ui.Column(c).Gap(2).Margin(4, 0, 0).Children(func() {
			sectionHeader(c, "同义词 · 用来检索，可修改")
			in := inlineInput(c.Key("clip-syn"), &a.libSynText, textSmall, false, "AI 整理时自动生成", "资料同义词").Margin(0, -6)
			if in.Changed() {
				a.libSynEditing = true
			}
			if a.libSynEditing && (in.Submitted() || in.PressedOutside()) {
				clip.Synonyms = cleanTags(strings.FieldsFunc(a.libSynText, func(r rune) bool { return r == '、' || r == ',' || r == '，' || r == ';' || r == '；' }))
				a.libSynEditing = false
				a.saveClip(clip)
			}
		})
	})

	// The user's note.
	sectionCard(c, iconPencil, tileYellow, "我的备注", nil, func() {
		note := clip.Note
		if inlineArea(c.Key("clip-note"), &note, textBody, 1, 10, "为什么收藏它、想怎么用…", "资料备注").Margin(0, -8).Changed() {
			clip.Note = note
			a.saveClip(clip)
		}
	})

	// The content.
	switch {
	case clip.Kind == library.KindText:
		sectionCard(c, iconText, tileOrange, "内容", nil, func() {
			text := clip.Text
			if inlineArea(c.Key("clip-text"), &text, textBody+1, 3, 30, "", "资料内容").Margin(0, -8).Changed() {
				a.setMemoText(clip, text)
			}
		})
	case clip.Kind == library.KindLink:
		sectionCard(c, iconLink, tileGreen, "网页正文", nil, func() {
			text := clip.Text
			in := inlineArea(c.Key("clip-page"), &text, textBody, 3, 16,
				"没读到网页正文时（需要登录的页面、公众号文章等），可以把正文复制粘贴到这里", "网页正文").Margin(0, -8)
			if in.Changed() {
				clip.Text = text
				a.saveClip(clip)
			}
			if in.PressedOutside() && clip.Text != "" && clip.Organized == nil && providerKey(a.cfg) != "" && !a.cfg.LibraryManual {
				a.enqueueClip(clip.ID, true)
			}
		})
	case clip.Text != "":
		label := "文件内容"
		if clip.Kind == library.KindImage {
			label = "图中内容（AI 识别）"
		}
		sectionCard(c, kindTile(clip.Kind), kindColor(clip.Kind), label, nil, func() {
			ui.Text(c, library.Excerpt(clip.Text, "", 3000)).FontSize(textSmall).LineHeight(1.5).TextColor(t.TextMuted).Wrap().Selectable()
		})
	}

	// What else to do with it.
	ui.Row(c).Gap(6).Wrap().GapY(6).Children(func() {
		if iconTextButton(c, iconSparkle, "问 AI", "问 AI", "打开 AI 助手，就这条资料提问", false).Clicked() {
			a.aiOpen = true
		}
		if a.doc != nil && button(c, "加到导图").Tooltip("加到「"+mapName(a.path)+"」的选中节点下").Clicked() {
			a.later(func() { a.clipIntoMap(clip) })
		}
		if button(c, "复制").Clicked() {
			a.plat.CopyText(clipCopyText(clip))
			a.tell("已复制")
		}
	})
	if len(clip.Keywords) > 0 {
		ui.Text(c, "检索关键词："+strings.Join(clip.Keywords, "、")).FontSize(textCaption).TextColor(t.TextMuted).Wrap()
	}
	a.relatedSection(c, clip)
}

// showLibrary opens the library in the main area.
func (a *app) showLibrary() {
	a.commitEditIfAny()
	a.lightbox, a.presenting = "", false
	a.libOpen, a.libHome, a.tasks.open, a.projOpen = true, false, false, false
	a.libReview = reviewOff
}
