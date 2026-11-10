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
	t := c.Theme()
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
				pageColumn(c, "lib-column", true).Children(func() { a.libraryColumn(c) })
			})
			if sel != nil {
				ui.Box(c).Width(1).FillHeight().Background(hairline(t))
				ui.Scroll(c.Key("clip-detail:"+sel.ID)).Grow(1).Basis(0).Padding(14, 16, 20).Gap(12).Children(func() {
					a.clipDetail(c, sel)
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
		pageHeader(c, "lib-head", "收集", fmt.Sprintf("%d 条灵感和资料", len(l.Clips)), func() {
			if a.libWorking != "" {
				ui.Spinner(c)
				ui.Text(c, fmt.Sprintf("整理中，还有 %d 条", len(a.libQueue)+1)).FontSize(12).TextColor(t.TextMuted).NoWrap()
			} else if a.embedBusy {
				ui.Spinner(c)
				ui.Text(c, "建立语义索引…").FontSize(12).TextColor(t.TextMuted).NoWrap()
			}
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
			if iconButton(c, iconShot, "截图", "截图：框选屏幕、标注后存到收集（"+keyLabel(a.shotKey())+"）", 30, true).Clicked() {
				a.later(a.screenshot)
			}
			if iconTextButton(c, iconSparkle, "问 AI", "AI 助手", "和 AI 聊收集里的内容，它会检索相关资料来回答", a.aiOpen).Clicked() {
				a.aiOpen = !a.aiOpen
			}
		})
		a.pluginBanner(c)
		a.clipComposer(c)
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
					msg := "还没有资料。在上面记点什么、粘贴链接，或把图片和文件拖进来。\n在任何应用里复制内容后按 " + keyLabel(a.clipKey()) + "，也能直接存到收集。"
					if len(l.Clips) > 0 || a.libKind == kindMaps {
						msg = "没有符合条件的资料。"
					}
					if a.libReview != reviewOff {
						msg = "一周以前的灵感会在这里，每天换几条。"
					}
					ui.Text(c, msg).FontSize(13).TextColor(t.TextMuted).Wrap().Padding(20, 10)
				})
			} else {
				// A wall of cards, three across, one beside a clip chosen; a
				// virtual list of its rows, so that only those in view are built.
				cols := 3
				if a.lib.Find(a.libSel) != nil {
					cols = 1
				}
				rows := (len(hits) + cols - 1) / cols
				ui.List(c.Key(fmt.Sprint("clips:", cols)), nil, rows).Grow(1).Basis(0).ItemKey(func(i int) any { return hits[i*cols].Clip.ID }).
					Rows(func(r ui.ListRow) {
						ui.Row(r.Context).Padding(6, 2).Gap(12).AlignItems(ui.Stretch).FillWidth().Children(func() {
							for k := range cols {
								i := r.Index*cols + k
								if i >= len(hits) {
									ui.Box(r.Context).Grow(1).Basis(0).MinWidth(0)
									continue
								}
								if clip := hits[i].Clip; clip.Kind == kindMap {
									ui.Box(r.Context.Key(clip.ID)).Grow(1).Basis(0).MinWidth(0).Children(func() { a.mapRow(r.Context, clip) })
								} else {
									a.clipRow(r.Context.Key(clip.ID), clip)
								}
							}
						})
					})
			}
		})
	}()
}

// libraryDialogs are the library's questions.
func (a *app) libraryDialogs(c *ui.Context) {
	l := a.lib
	gone := "删除后无法恢复。"
	if n := len(a.clipBacklinks(a.libSel)); n > 0 {
		gone = fmt.Sprintf("它挂在 %d 个节点上，删除后这些链接会失效。删除后无法恢复。", n)
	}
	switch ui.AlertDialog(c, &a.libConfirm, "删除这条资料？", gone, "取消", "删除") {
	case 1:
		if err := l.Delete(a.libSel); err != nil {
			a.libErr = err.Error()
		}
		a.noteLibraryWrite()
		a.libSel = ""
	}
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

// clipComposer is the field to keep text or a link, and the buttons to
// keep the clipboard or a file.
func (a *app) clipComposer(c *ui.Context) {
	t := c.Theme()
	empty := strings.TrimSpace(a.libDraft) == ""
	box := captureBox(c, "lib-compose").Margin(0, 0, 12)
	box.Children(func() {
		glyph(c, iconPlus, 15, t.Accent)
		in := ui.TextAreaBase(c.Key("lib-draft"), &a.libDraft).Lines(1, 5).
			Placeholder("记点什么，#标签 归类；也可以粘贴链接、图片或文件…").Label("新资料").
			OnPaste(func(text string) bool { // a picture or files copied: kept at once
				if text != "" {
					return false
				}
				clips, err := a.clipClipboard("剪贴板")
				if err != nil {
					return false
				}
				a.libSel = clips[0].ID
				return true
			}).
			Padding(5, 0).FontSize(textBody + 1).Grow(1)
		captureFocus(c, box, in)
		keep := in.Shortcut(ui.Cmd, ui.KeyEnter)
		if !empty { // the button when there is something to keep
			ui.Text(c, "⌘↩").FontSize(textCaption).TextColor(t.TextMuted)
			if ui.PrimaryButton(c, "收藏").Shrink(0).Clicked() {
				keep = true
			}
		}
		if keep && !empty {
			if clip, err := a.addClipText(a.libDraft, "手动"); err != nil {
				a.libErr = err.Error()
			} else {
				a.libDraft, a.libSel = "", clip.ID
			}
		}
		if iconButton(c, iconClip, "文件…", "收藏图片或文件（也可以直接拖进来）", 28, true).Clicked() {
			a.plat.OpenFile("选择要收藏的文件", nil, func(path string) {
				if path != "" {
					a.dropClips([]string{path})
				}
			})
		}
	})
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
	l := a.lib
	ui.Row(c.Key("lib-kinds")).Gap(22).FillWidth().AlignItems(ui.End).Shrink(0).
		BorderWidth(0, 0, 1, 0).BorderColor(hairline(t)).Children(func() {
		for _, k := range []struct {
			kind int
			name string
		}{{kindAll, "全部"}, {kindMemos, "灵感"}, {kindLinks, "链接"}, {kindDocs, "文档"}, {kindImages, "图片"}} {
			n := 0
			for _, cl := range l.Clips {
				if ofKind(cl, k.kind) {
					n++
				}
			}
			on := a.libKind == k.kind && a.libReview == reviewOff
			if underlineTab(c, "kind:"+k.name, k.name, fmt.Sprint(n), on).Clicked() {
				a.libKind, a.libReview = k.kind, reviewOff
			}
		}
		ui.Spacer(c)
		// Looking back at the old notes, as flomo does.
		ghostButton(c, "回顾 ⌄", "回头看以前记下的灵感").Margin(0, 0, 6).Menu(func(m *ui.Menu) {
			if m.Item("每日回顾：几条以前的灵感，每天换").Checked(a.libReview == reviewDaily).Chosen() {
				a.libReview, a.libSel = reviewDaily, ""
			}
			if m.Item("随机漫步：一次看一条").Checked(a.libReview == reviewWalk).Chosen() {
				a.libReview, a.libSel = reviewWalk, ""
			}
		})
	})
	ui.Row(c.Key("lib-tags")).Padding(10, 0, 6).MinHeight(46).Gap(6).Wrap().GapY(6).AlignItems(ui.Center).Children(func() {
		chip := func(key, label string, active bool) bool {
			b := ui.Box(c.Key("cat:"+key)).Padding(4, 10).Radius(999).Cursor(ui.CursorPointer).Label(label)
			switch { // quiet pills, the one chosen dark
			case active:
				b.Background(t.Text.Alpha(0.85))
			case b.Hovered():
				b.Background(t.Text.Alpha(0.08))
			default:
				b.Background(t.Text.Alpha(0.045))
			}
			col := t.Text
			if active {
				col = t.Background
			}
			b.Children(func() { ui.Text(c, label).FontSize(12).TextColor(col).NoWrap() })
			return b.Clicked()
		}
		if a.libReview != reviewOff {
			name := map[int]string{reviewDaily: "每日回顾", reviewWalk: "随机漫步"}[a.libReview]
			if chip("review", name+" ✕", true) {
				a.libReview = reviewOff
			}
			if a.libReview == reviewWalk && softButton(c, "换一条", "再随机看一条").Clicked() {
				a.libWalk++
			}
			return
		}
		toggle := func(key, label, cat string) {
			if chip(key, label, a.libCat == cat) {
				if a.libCat == cat {
					a.libCat = ""
				} else {
					a.libCat = cat
				}
			}
		}
		if slices.ContainsFunc(l.Clips, func(c *library.Clip) bool { return c.Starred }) {
			toggle("★", "★ 星标", "★")
		}
		if k := len(l.Unorganized()); k > 0 {
			toggle("-", fmt.Sprintf("未整理 %d", k), "-")
		}
		// The tags: the few most used, the rest in a menu.
		tags := a.clipTags()
		const shown = 6
		pick := func(tag string) {
			if a.libTag == tag {
				a.libTag = ""
			} else {
				a.libTag = tag
			}
		}
		chosen := a.libTag // as the frame began: a click changes it
		listed := false
		for i, tc := range tags {
			if i == shown {
				break
			}
			listed = listed || tc.name == chosen
			if chip("tag:"+tc.name, fmt.Sprintf("#%s %d", tc.name, tc.n), chosen == tc.name) {
				pick(tc.name)
			}
		}
		if chosen != "" && !listed && chip("tag-chosen", "#"+chosen+" ✕", true) {
			a.libTag = ""
		}
		if len(tags) > shown {
			ghostButton(c, "更多标签 ⌄", fmt.Sprintf("全部 %d 个标签", len(tags))).Menu(func(m *ui.Menu) {
				for i, tc := range tags {
					if i < shown {
						continue
					}
					if i == shown+40 {
						break
					}
					if m.Item(fmt.Sprintf("#%s  %d", tc.name, tc.n)).Checked(a.libTag == tc.name).Chosen() {
						pick(tc.name)
					}
				}
			})
		}
		if !a.libDay.IsZero() && chip("day", a.libDay.Format("1月2日")+" ✕", true) {
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
	row := ui.Column(c).Grow(1).Basis(0).MinWidth(0).Padding(14, 16).Gap(9).Radius(14).Cursor(ui.CursorPointer).
		Label("资料 " + clip.Label())
	// A white card, as all are: a note is its text, the others have their
	// kind's icon by their title.
	row.Background(cardColor(t)).Shadow(0, 1, 2, 0, ui.RGBA(31, 34, 51, 0.06))
	switch {
	case a.libSel == clip.ID:
		row.Border(2, t.Accent)
	case row.Hovered():
		row.Shadow(0, 4, 14, 0, ui.RGBA(31, 34, 51, 0.1))
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
			ui.Text(c, library.Excerpt(text, a.libQuery, 400)).FontSize(14).LineHeight(1.65).Wrap().MaxLines(7).PassThrough()
		} else {
			ui.Row(c).Gap(10).AlignItems(ui.Center).PassThrough().Children(func() {
				glyph(c, kindTile(clip.Kind), 16, t.TextMuted)
				ui.Text(c, clip.Label()).FontSize(14).FontWeight(600).Grow(1).Shrink(1).MinWidth(0).MaxLines(2)
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
		ui.Spacer(c)
		ui.Row(c).Gap(6).Wrap().GapY(4).AlignItems(ui.Center).Children(func() {
			_, ink := tinted(t, kcol)
			kind := library.KindLabel(clip.Kind)
			if memo {
				kind = "灵感"
			}
			mapDot(c, kcol, 6)
			ui.Text(c, kind).FontSize(textCaption).FontWeight(600).TextColor(ink).NoWrap()
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
		if iconButton(c, iconClose, "关闭详情", "关闭", 28, true).Clicked() {
			a.libSel = ""
		}
	})
	status, failed := a.libStatus[clip.ID], strings.Contains(a.libStatus[clip.ID], "失败")
	if status != "" && !failed {
		ui.Text(c, status).FontSize(textSmall).TextColor(t.TextMuted).Wrap()
	}

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
		if a.libWorking == clip.ID {
			ui.Spinner(c)
		} else if ghostButton(c, label, "让 AI 起标题、写说明和摘要、分类、打标签、生成同义词和分析").Clicked() {
			a.organizeClip(clip.ID)
		}
	}, func() {
		// What kept it from organizing, said here where it was asked for.
		if a.libNeedKey == clip.ID && providerKey(a.cfg) == "" {
			ui.Row(c.Key("need-key")).Padding(8, 10).Gap(8).Radius(8).AlignItems(ui.Center).
				Background(t.Warning.Alpha(0.12)).Children(func() {
				ui.Text(c, "AI 整理要先填写 API Key。").FontSize(textSmall).Wrap().Grow(1)
				if ui.PrimaryButton(c, "填写 API Key").FontSize(textSmall).Clicked() {
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
		if clip.Summary != "" {
			ui.Text(c, clip.Summary).FontSize(textBody).LineHeight(1.55).Wrap().Selectable()
		}
		if clip.Analysis != "" {
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
		if a.doc != nil && ui.Button(c, "加到导图").Tooltip("加到「"+mapName(a.path)+"」的选中节点下").Clicked() {
			a.later(func() { a.clipIntoMap(clip) })
		}
		if ui.Button(c, "复制").Clicked() {
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
	a.libOpen, a.libHome, a.tasks.open = true, false, false
	a.libReview = reviewOff
}
