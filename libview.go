package main

import (
	"fmt"
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
		// Its head says what is shown: all of it, in figures, or what the
		// left column and the search narrow it to, and how many that leaves.
		sub, figs := "灵感和资料", []figure{{len(l.Clips), "条"}, {memos, "条灵感"}, {len(l.Unorganized()), "条未整理"}}
		if len(a.libFilters()) > 0 { // the conditions are chips under the head, each with its ✕
			sub, figs = "", []figure{{len(a.libHits()), "条符合"}}
		}
		pageHead(c, "lib-head", "收集", sub, figs, func() {
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
					var clear func()
					if len(l.Clips) > 0 || a.libKind == kindMaps {
						icon, title, msg = "search", "没有符合条件的资料", "换一个分类或标签，或者清掉搜索的词。"
						if fs := a.libFilters(); len(fs) > 0 { // which conditions, and the way out of them
							title = "没有符合「" + a.libFilterWords() + "」的资料"
							msg = "点上面某个条件的 ✕ 去掉它，或者全部清除。"
							if len(fs) > 1 {
								msg = "这几个条件是同时生效的。" + msg
							}
							clear = a.clearLibFilters
						}
					}
					if a.libReview != reviewOff {
						icon, title, msg = "history", "还没有可以回顾的", "一周以前的灵感会在这里，每天换几条。"
					}
					var act func()
					if clear != nil && a.libReview == reviewOff {
						act = func() {
							if textKey(c, "全部清除", "清除全部筛选", "回到全部").Clicked() {
								clear()
							}
						}
					}
					emptyState(c, "wall", icon, title, msg, act)
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
	tall := make([]float32, cols)
	in := make([][]*library.Clip, cols)
	tops := make([][]float32, cols) // where each card of a column begins, as guessed
	for _, h := range hits[:shown] {
		k := 0
		for j := range tall {
			if tall[j] < tall[k] {
				k = j
			}
		}
		in[k] = append(in[k], h.Clip)
		tops[k] = append(tops[k], tall[k])
		tall[k] += clipTall(h.Clip, cols) + 8
	}
	// The arrows go where they point on the wall: up and down its column,
	// left and right to the card beside, the one whose top is nearest; the
	// details follow. Not while something is written in.
	if !a.barOpen && !c.FocusTakesText() && shown > 0 {
		dx, dy := 0, 0
		switch {
		case c.Shortcut(0, ui.KeyDown):
			dy = 1
		case c.Shortcut(0, ui.KeyUp):
			dy = -1
		case c.Shortcut(0, ui.KeyRight):
			dx = 1
		case c.Shortcut(0, ui.KeyLeft):
			dx = -1
		}
		if dx != 0 || dy != 0 {
			col, row := -1, -1
			for k := range in {
				for r, cl := range in[k] {
					if cl.ID == a.libSel {
						col, row = k, r
					}
				}
			}
			switch {
			case col < 0: // none chosen: the first
				col, row = 0, 0
			case dy != 0:
				row = min(max(row+dy, 0), len(in[col])-1)
			default:
				to := col + dx
				for to >= 0 && to < cols && len(in[to]) == 0 {
					to += dx
				}
				if to >= 0 && to < cols {
					y, near := tops[col][row], 0
					for r, top := range tops[to] {
						if abs32(top-y) < abs32(tops[to][near]-y) {
							near = r
						}
					}
					col, row = to, near
				}
			}
			if len(in[col]) > 0 {
				a.libSel, a.libTagText = in[col][row].ID, ""
				a.libReveal = a.libSel
			}
		}
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
	if clip.Kind == library.KindImage || isVideo(clip) {
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
	a.closeVideos(id)
	taken, err := a.lib.Trash(id, a.now())
	if err != nil {
		a.libFail(err.Error())
		return
	}
	a.noteLibraryWrite()
	a.clipGone(id)
	if a.libSel == id {
		a.libSel = ""
	}
	a.emptyTrashLater()
	a.tellUndo("已删除", func() {
		// Into the library as it is now: read again since, as after a
		// sync, the one the clip was taken from is no longer shown.
		if err := a.lib.PutBack(taken); err != nil {
			a.tell("没能恢复：" + err.Error())
			return
		}
		a.noteLibraryWrite()
	})
}

// trashLife is how long the file of a clip deleted waits in the library's
// trash to be taken back: longer than the toast that offers it shows.
const trashLife = 10 * time.Minute

// emptyTrashLater clears the trash, once what was just put there has
// waited its time, of what has: the room a large file took is given back
// without the app being left first.
func (a *app) emptyTrashLater() {
	if a.post == nil {
		return // no window: cleared as a library is next opened
	}
	post := a.post
	time.AfterFunc(trashLife+time.Second, func() {
		post(func() {
			if a.lib != nil {
				a.lib.EmptyTrash(a.now().Add(-trashLife))
			}
		})
	})
}

// collectState is more of what 收集 holds from a frame to the next.
type collectState struct {
	// The category of the clip catFor, as it is typed in its details.
	catFor, catText string
	// The pictures read (thumbs.go).
	pics pictures
}

// collectShown tells whether 收集's page shows.
func (a *app) collectShown() bool {
	return a.libOpen && !a.projOpen && !a.skillsOpen && !a.tasks.open && !a.libHome
}

// libFail says what went wrong with a thing collected where it is looked
// at: on 收集's page in the line under its head, which stays till it is
// read; on any other page in a toast. Kept for 收集's page alone, what
// failed on 今天 or on a map was said where nobody was looking.
func (a *app) libFail(msg string) {
	if a.collectShown() {
		a.libErr = msg
		return
	}
	a.tell(msg)
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
		case a.libCat == secretCat && (c.Kind == kindMap || !c.Secret()):
			return false
		case a.libCat != "" && a.libCat != secretCat && a.libCat != "-" && a.libCat != "★" && c.Category != a.libCat:
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

// dropClips keeps files dropped on the library: each of them, one that
// cannot be kept not stopping those after it, and how many could not is
// said.
func (a *app) dropClips(files []string) {
	clips, failed, err := a.addClipFiles(files, "拖入")
	if n := len(clips); n > 0 {
		a.libSel = clips[n-1].ID
	}
	if failed > 0 {
		a.libFail(filesFailed(failed, len(clips), err))
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
		// Each condition a chip with its own ✕, which takes off that one;
		// with more than one, the way out of them all after them.
		fs := a.libFilters()
		for _, f := range fs {
			if filterChip(c, f.word) {
				a.later(f.clear)
			}
		}
		if len(fs) > 1 && textLink(c, "全部清除", "清除筛选").Clicked() {
			a.later(a.clearLibFilters)
		}
		a.vaultState(c)
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
			text := library.Mask(clip.Text)
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
			if isVideo(clip) {
				a.videoCard(c, clip, row.Hovered())
			}
			if clip.Kind == library.KindImage {
				// Its picture small, read away from the window's work; a
				// pale block in its room until it is.
				if b, loading := a.cardPicture(a.lib.FilePath(clip)); b != nil {
					ui.Image(c, b).Height(110).FillWidth().Fit(ui.Cover).Radius(8).PassThrough()
				} else if loading {
					ui.Box(c.Key("picture-loading")).Height(110).FillWidth().Radius(8).Background(blockFill(t)).PassThrough()
				}
			}
			sub := clip.Desc
			if sub == "" && summaryShown(clip) { // as the details show it
				sub = clip.Summary
			}
			if sub == "" {
				sub = cmpOr(clip.Note, clip.Text)
			}
			if sub == "" {
				sub = clip.URL
			}
			sub = library.Mask(sub)
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
			kind := clipKindLabel(clip)
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

// clipMenu is the menu of a clip.
func (a *app) clipMenu(m *ui.Menu, clip *library.Clip) {
	// In parts, as the topics' menu: opening it, where it goes, copying
	// it, and deleting it apart at the foot. Twelve rows in one list were
	// too many to read, the user found.
	if clip.URL != "" && m.Item("打开链接").Chosen() {
		a.plat.OpenURL(clip.URL)
	}
	if p := a.lib.FilePath(clip); p != "" && m.Item("打开文件").Chosen() {
		a.plat.OpenPath(p)
	}
	star := "加星标"
	if clip.Starred {
		star = "取消星标"
	}
	if m.Item(star).Chosen() {
		clip.Starred = !clip.Starred
		a.saveClip(clip)
	}
	// A password that nothing tells from a word: the whole of it sealed.
	// It is here with no second password set too, and leads to setting
	// one: looked for here, it was not to be found (the user asked how).
	if clip.Kind == library.KindText || clip.Kind == library.KindLink {
		switch {
		case clip.Locked && a.vault.On():
			if m.Item("取消整条加密").Chosen() {
				a.later(func() { a.lockWhole(clip, false) })
			}
		case a.vault.On():
			if m.Item("整条加密").Chosen() {
				a.later(func() { a.lockWhole(clip, true) })
			}
		default:
			if m.Item("整条加密…").Chosen() {
				a.later(func() { a.askVault(askSetup, func() { a.lockWhole(clip, true) }) })
			}
		}
	}
	m.Separator()
	if a.doc != nil && m.Item("加到当前导图").Chosen() {
		a.clipIntoMap(clip)
	}
	// To be put on a map later: it waits on 今天 till then. Not asked of
	// every clip: one is at home here.
	switch {
	case clip.Kind == kindMap:
	case clip.ToPlace:
		if m.Item("不用归位了").Chosen() {
			a.later(func() { clip.ToPlace = false; a.saveClip(clip) })
		}
	case len(a.clipBacklinks(clip.ID)) == 0:
		if m.Item("标为待归位").Chosen() {
			a.later(func() { a.markToPlace(clip) })
		}
	}
	if m.Item("转成任务").Chosen() {
		a.later(func() { a.clipToTask(clip) })
	}
	m.Separator()
	m.Submenu("复制", func(m *ui.Menu) {
		if m.Item("复制内容").Chosen() {
			a.later(func() { a.copyClip(clip) })
		}
		if m.Item("复制资料链接").Chosen() {
			a.plat.CopyText(clipScheme + clip.ID)
			a.tell("已复制，可以粘贴到节点的链接里")
		}
		if m.Item("生成分享卡片…").Chosen() {
			a.openCard(clip)
		}
	})
	if m.Item("问 AI").Chosen() {
		a.libSel, a.aiOpen = clip.ID, true
	}
	if m.Item("AI 重新整理").Disabled(providerKey(a.cfg) == "").Chosen() {
		a.organizeClip(clip.ID)
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
		a.libFail("没有存下：" + err.Error())
	}
	a.noteLibraryWrite()
}

// clipIntoMap adds a clip to the open map and shows it there.
func (a *app) clipIntoMap(clip *library.Clip) {
	if err := a.clipToMap(clip); err != nil {
		a.libFail(err.Error())
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
		kind := clipKindLabel(clip)
		if isMemo(clip) {
			kind = "灵感" // as its card says
		}
		ui.Text(c, kind).FontSize(textCaption).Bold().TextColor(kindColor(clip.Kind)).NoWrap().Margin(0, 0, 0, 4)
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
		esc := !a.barOpen && a.videoBig == "" && c.Shortcut(0, ui.KeyEscape) // the video larger closes first
		if esc && c.FocusTakesText() {                                       // out of the field first; the next closes
			esc, a.escLeave = false, true
		}
		a.escRest(c)
		if iconButton(c, iconClose, "关闭详情", "关闭", 28, true).Clicked() || esc {
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
		// A part with a secret in it is in dots, and not written in, till
		// the secrets are asked for (fieldText).
		a.lineField(t, clip, "title", clip.Title, func(title *string) ui.Element {
			return inlineInput(c.Key("clip-title"), title, textHeadline, true, clip.Label(), "资料标题")
		}, func(title string) {
			clip.Title = title
			a.saveClip(clip)
			a.fieldTyped(clip, "title", title, clip.Title)
		})
		a.lineField(t, clip, "desc", clip.Desc, func(desc *string) ui.Element {
			return inlineArea(c.Key("clip-desc"), desc, textBody, 1, 4, "加一句说明，说清这是什么（AI 整理时自动生成）", "资料说明").TextColor(t.TextMuted)
		}, func(desc string) {
			clip.Desc = desc
			a.saveClip(clip)
			a.fieldTyped(clip, "desc", desc, clip.Desc)
		})
	})

	if clip.Kind == library.KindImage {
		// Whole, once read; a small one of it meanwhile, of the same shape
		// (its card's is cut to the card's).
		b, _ := a.filePicture(a.lib.FilePath(clip), false)
		if b == nil {
			b, _ = a.filePicture(a.lib.FilePath(clip), true)
		}
		if b != nil {
			w, h := b.Size()
			k := min(1, 420/float32(max(w, 1)), 320/float32(max(h, 1)))
			img := ui.Image(c, b).Size(float32(w)*k, float32(h)*k).Fit(ui.Contain).Radius(10).Cursor(ui.CursorPointer).Tooltip("用默认应用打开")
			if img.Clicked() {
				a.plat.OpenPath(a.lib.FilePath(clip))
			}
		}
	}

	if isVideo(clip) {
		a.videoDetail(c, clip)
	}

	// What the AI made of it: above the content when it summed it up (a
	// page, a document, which the summary reads faster), else below, a
	// line or two that the content needs not wait for.
	aiPart := func() {
		// What the AI made of it.
		sectionCard(c, iconSparkle, tilePurple, "AI 整理", func() {
			label := "整理"
			if clip.Organized != nil {
				label = "重新整理"
			}
			if a.libWorking == clip.ID { // one sign of a wait: the shapes below
			} else if ghostButton(c, label, "让 AI 起标题、写说明、分类、打标签；内容长的再写摘要和分析").Clicked() {
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
			if clip.Organized == nil {
				if a.libNeedKey != clip.ID && !failed {
					ui.Text(c, "还没整理。点「整理」，AI 会起标题、分类、打标签，内容长的再写摘要。").FontSize(textSmall).TextColor(t.TextMuted).Wrap()
				}
				return
			}
			// A short note has no summary: one written before this was so is
			// kept, and not shown.
			if summaryShown(clip) {
				ui.Text(c, library.Mask(clip.Summary)).FontSize(textBody).LineHeight(1.55).Wrap().Selectable()
			}
			// What is long and seldom read is folded: the points of a long
			// text, and the words it is found by. Laid out, they made the
			// details a page of the AI's words over a line of the user's.
			if analysisShown(clip) {
				if a.foldKey(c, "analysis:"+clip.ID, "分析要点", "") {
					analysis, _ := a.fieldText(clip, "analysis", clip.Analysis) // a secret in it in dots, till shown
					analysis, _ = library.Marks(analysis)                       // the marks are the file's
					markdownView(c.Key("clip-analysis"), analysis, textBody)
				}
			}
			if a.libSynFor != clip.ID || !a.libSynEditing {
				a.libSynFor, a.libSynText = clip.ID, strings.Join(clip.Synonyms, "、")
			}
			if n := len(clip.Synonyms) + len(clip.Keywords); n > 0 && a.foldKey(c, "words:"+clip.ID, "检索词", fmt.Sprint(n)) {
				ui.Column(c).Gap(2).Children(func() {
					sectionHeader(c, "同义词 · 可修改")
					in := inlineInput(c.Key("clip-syn"), &a.libSynText, textSmall, false, "AI 整理时自动生成", "资料同义词").Margin(0, -6)
					if in.Changed() {
						a.libSynEditing = true
					}
					if a.libSynEditing && (in.Submitted() || in.PressedOutside()) {
						clip.Synonyms = cleanTags(strings.FieldsFunc(a.libSynText, func(r rune) bool { return r == '、' || r == ',' || r == '，' || r == ';' || r == '；' }))
						a.libSynEditing = false
						a.saveClip(clip)
					}
					if len(clip.Keywords) > 0 {
						ui.Text(c, "关键词："+strings.Join(clip.Keywords, "、")).FontSize(textCaption).TextColor(t.TextMuted).Wrap().Margin(4, 0, 0)
					}
				})
			}
		})
	}
	// Where it goes by what the clip is made of, not by whether it was
	// summed up yet: it does not move under the pointer as the answer comes.
	aiFirst := clipContentLen(clip, nil) >= summaryLen && !briefClip(clip)
	if aiFirst {
		aiPart()
	}

	// The content. What looks like a key or a password in it is in dots
	// until asked for, and with a second password set is sealed: the
	// password is asked then (secretKeys, fieldText).
	keys := false // the keys of its secrets were drawn
	content := func(key, hint, label string, size float32, maxLines int, changed func(text string)) ui.Element {
		text, write := a.fieldText(clip, "text", clip.Text)
		if !write {
			keys = true
			el := ui.Text(c.Key(key+"-dots"), text).FontSize(size).LineHeight(1.55).Wrap()
			a.secretKeys(c, clip)
			return el
		}
		// Written in without the marks of its secrets, which show in a
		// look of their own (secretField).
		f := a.secretField(text)
		text = f.shown
		in := inlineArea(c.Key(key), &text, size, 3, maxLines, hint, label).Margin(0, -8)
		if in.Changed() {
			_, caret := in.TextSelection()
			full := f.typed(text, caret)
			changed(full)
			a.fieldTyped(clip, "text", full, clip.Text)
			f = a.secretField(full)
		}
		f.style(in, t)
		a.markKey(c, clip, "text", in, f, changed)
		if library.HasSecret(clip.Text) {
			keys = true
			a.secretKeys(c, clip)
		}
		return in
	}
	switch {
	case clip.Kind == library.KindText:
		sectionCard(c, iconText, tileOrange, "内容", nil, func() {
			content("clip-text", "", "资料内容", textBody+1, 30, func(text string) { a.setMemoText(clip, text) })
		})
	case clip.Kind == library.KindLink:
		sectionCard(c, iconLink, tileGreen, "网页正文", nil, func() {
			in := content("clip-page", "没读到网页正文时（需要登录的页面、公众号文章等），可以把正文复制粘贴到这里", "网页正文", textBody, 16, func(text string) {
				clip.Text = text
				a.saveClip(clip)
			})
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
			text, _ := a.fieldText(clip, "text", clip.Text)
			text, _ = library.Marks(text)
			ui.Text(c, library.Excerpt(text, "", 3000)).FontSize(textSmall).LineHeight(1.5).TextColor(t.TextMuted).Wrap().Selectable()
			if library.HasSecret(clip.Text) {
				keys = true
				a.secretKeys(c, clip)
			}
		})
	}

	// The user's note.
	sectionCard(c, iconPencil, tileYellow, "我的备注", nil, func() {
		note, write := a.fieldText(clip, "note", clip.Note)
		if !write {
			ui.Text(c.Key("clip-note-dots"), note).FontSize(textBody).LineHeight(1.55).Wrap()
		} else {
			f := a.secretField(note)
			note = f.shown
			in := inlineArea(c.Key("clip-note"), &note, textBody, 1, 10, "为什么收藏它、想怎么用…", "资料备注").Margin(0, -8)
			set := func(note string) {
				clip.Note = note
				a.saveClip(clip)
			}
			if in.Changed() {
				_, caret := in.TextSelection()
				full := f.typed(note, caret)
				set(full)
				a.fieldTyped(clip, "note", full, clip.Note)
				f = a.secretField(full)
			}
			f.style(in, t)
			a.markKey(c, clip, "note", in, f, set)
		}
		if !keys { // the secret is here, or in the title
			a.secretKeys(c, clip)
		}
	})

	// Where it is used, or where it may go.
	a.clipPlacesSection(c, clip)
	a.clipMentions(c, clip)

	// Properties.
	sectionCard(c, iconTag, tileBlue, "信息", nil, func() {
		propRow(c, "分类", func() {
			// The field holds what is typed, a space at its end too, and
			// the clip the same without the spaces at its ends, which the
			// field shows once it is left: the field showing the clip's at
			// every letter, the space of "ML ops" was gone before the o
			// came.
			st := &a.collect
			if st.catFor != clip.ID || strings.TrimSpace(st.catText) != clip.Category {
				st.catFor, st.catText = clip.ID, clip.Category
			}
			in := inlineInput(c.Key("clip-cat"), &st.catText, textBody, false, "未分类", "资料分类")
			if in.Changed() {
				clip.Category = strings.TrimSpace(st.catText)
				a.saveClip(clip)
			} else if !in.Focused() {
				st.catText = clip.Category
			}
		})
		propRow(c, "标签", func() {
			for _, tag := range clip.Tags {
				tag := tag
				off := func() {
					clip.Tags = slices.DeleteFunc(slices.Clone(clip.Tags), func(s string) bool { return s == tag })
					a.saveClip(clip)
				}
				chip, x := clipTagChip(c.Key("tag:"+tag), tag)
				if chip.Clicked() {
					a.libTag = tag
				}
				if x {
					a.later(off)
				}
				chip.ContextMenu(func(m *ui.Menu) {
					if m.Item("移除标签").Chosen() {
						off()
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

	if !aiFirst {
		aiPart()
	}
	a.relatedSection(c, clip)
}

// clipTagChip is a tag of a clip in its details: the tag, which narrows
// 收集 to it, and a ✕ of its own that takes it off the clip. It was taken
// off by the pointer's other key alone, which a phone has not, and which
// nothing on the tag said. It reports the ✕ pressed.
func clipTagChip(c *ui.Context, tag string) (chip ui.Element, off bool) {
	t := c.Theme()
	size := float32(textSmall)
	ink := t.Text.MixOklab(t.TextMuted, 0.7)
	chip = ui.Row(c).Padding(size*0.15, size*0.25, size*0.15, size*0.5).Gap(size * 0.35).Radius(999).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Background(t.Text.Alpha(0.055)).Label("标签 " + tag).Tooltip("只看这个标签")
	chip.Children(func() {
		ui.Box(c).Size(size*0.45, size*0.45).Radius(999).Background(tagColor(tag)).Shrink(0).PassThrough()
		ui.Text(c, "#"+tag).FontSize(size).TextColor(ink).NoWrap().PassThrough()
		x := ui.Box(c.Key("off")).Size(16, 16).Radius(8).Center().Cursor(ui.CursorPointer).Label("去掉标签 " + tag).Tooltip("去掉这个标签").
			Background(t.Text.Alpha(0)).Transition(easeLook)
		if x.Hovered() {
			x.Background(t.Text.Alpha(0.1))
		}
		press(x, 0.9)
		x.Children(func() { glyph(c, iconClose, 7, ink) })
		off = x.Clicked()
	})
	return chip, off
}

// showLibrary opens the library in the main area.
func (a *app) showLibrary() {
	a.commitEditIfAny()
	a.lightbox, a.presenting = "", false
	a.libOpen, a.libHome, a.tasks.open, a.projOpen, a.skillsOpen = true, false, false, false, false
	a.libReview = reviewOff
}

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// A libFilter is one condition that narrows 收集 now: its word, and what
// takes it off alone.
type libFilter struct {
	word  string
	clear func()
}

// libFilters are the conditions that narrow 收集 now, each with its way
// off: the kind, 星标, 未整理, 含密钥 or a category, the tag, the day, what
// is searched. None when all of it shows. The page says each as a chip
// with its ✕ (clipFilters): one ✕ for them all left no way to take off
// one, the user found.
func (a *app) libFilters() []libFilter {
	if a.libReview != reviewOff {
		return nil
	}
	var fs []libFilter
	if name := map[int]string{kindMemos: "灵感", kindLinks: "链接", kindDocs: "文档", kindImages: "图片", kindVideos: "视频", kindMaps: "导图"}[a.libKind]; name != "" {
		fs = append(fs, libFilter{name, func() { a.libKind = kindAll }})
	}
	cat := map[string]string{"★": "星标", "-": "未整理", secretCat: "含密钥"}[a.libCat]
	if cat == "" {
		cat = a.libCat
	}
	if cat != "" {
		fs = append(fs, libFilter{cat, func() { a.libCat = "" }})
	}
	if a.libTag != "" {
		fs = append(fs, libFilter{"#" + a.libTag, func() { a.libTag = "" }})
	}
	if !a.libDay.IsZero() {
		fs = append(fs, libFilter{a.libDay.Format("1月2日"), func() { a.libDay = time.Time{} }})
	}
	if q := strings.TrimSpace(a.libQuery); q != "" {
		fs = append(fs, libFilter{"搜「" + q + "」", func() { a.libQuery = "" }})
	}
	return fs
}

// libFilterWords are the conditions' words, joined.
func (a *app) libFilterWords() string {
	var words []string
	for _, f := range a.libFilters() {
		words = append(words, f.word)
	}
	return strings.Join(words, " · ")
}

// clearLibFilters shows all of 收集 again.
func (a *app) clearLibFilters() {
	a.libKind, a.libCat, a.libTag, a.libQuery, a.libDay = kindAll, "", "", "", time.Time{}
}

// filterChip is a condition that narrows a page, with the ✕ that takes it
// off: the word in the accent on its tint, the ✕ a key of its own that
// darkens under the pointer. It reports the ✕ pressed.
func filterChip(c *ui.Context, word string) bool {
	t := c.Theme()
	ink := t.Accent.MixOklab(t.Text, 0.3)
	off := false
	ui.Row(c.Key("filter:"+word)).Height(28).Padding(0, 4, 0, 11).Gap(3).Radius(radiusSmall).AlignItems(ui.Center).Shrink(0).
		Background(t.Accent.Alpha(0.12)).Label("条件 " + word).Children(func() {
		ui.Text(c, word).FontSize(textSmall).FontWeight(700).TextColor(ink).NoWrap().MaxWidth(220).SingleLine()
		x := ui.Box(c.Key("off")).Size(20, 20).Radius(6).Center().Cursor(ui.CursorPointer).Label("去掉 " + word).Tooltip("去掉这个条件").
			Background(t.Accent.Alpha(0)).Transition(easeLook)
		if x.Hovered() {
			x.Background(t.Accent.Alpha(0.2))
		}
		press(x, 0.9)
		x.Children(func() { glyph(c, iconClose, 9, ink) })
		off = x.Clicked()
	})
	return off
}

// foldKey is a part folded under a quiet line, its name and how many it
// holds, a click opening it or closing it: what is long and seldom read
// on a clip's details (its analysis, the words it is found by). It tells
// whether the part is open.
func (a *app) foldKey(c *ui.Context, key, name, count string) bool {
	t := c.Theme()
	open := a.folds[key]
	row := ui.Row(c.Key("fold:"+key)).Gap(6).AlignItems(ui.Center).Padding(4, 0).Cursor(ui.CursorPointer).Label(name)
	if row.Clicked() {
		if a.folds == nil {
			a.folds = map[string]bool{}
		}
		a.folds[key] = !open
	}
	ink := t.TextMuted
	if row.Hovered() {
		ink = t.Text
	}
	row.Children(func() {
		ui.Box(c).Size(10, 10).Shrink(0).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) {
			cx, cy := r.X+r.W/2, r.Y+r.H/2
			path := new(ui.Path)
			if open {
				path.MoveTo(cx-3.5, cy-1.75).LineTo(cx, cy+1.75).LineTo(cx+3.5, cy-1.75)
			} else {
				path.MoveTo(cx-1.75, cy-3.5).LineTo(cx+1.75, cy).LineTo(cx-1.75, cy+3.5)
			}
			p.StrokePath(path, 1.3, ink)
		})
		ui.Text(c, name).FontSize(textSmall).FontWeight(600).TextColor(ink).PassThrough()
		if count != "" {
			ui.Text(c, count).FontSize(textCaption).TextColor(t.TextMuted).PassThrough()
		}
	})
	return open
}
