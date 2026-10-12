package main

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
)

// libraryView is the library in the main area: a field to keep text and
// links, the filters, the clips found, and the clip chosen.
func (a *app) libraryView(c *ui.Context) {
	t := c.Theme()
	l := a.lib
	root := ui.Column(c.Key("library")).Grow(1).Label("资料库")
	if files := root.DroppedFiles(); len(files) > 0 {
		a.later(func() { a.dropClips(files) })
	}
	root.Children(func() {
		ui.Row(c.Key("lib-head")).Height(toolbarHeight).Padding(0, 10, 0, 16).Gap(10).AlignItems(ui.Center).Shrink(0).
			DragWindow().Children(func() {
			ui.Text(c, "资料").Bold().FontSize(textTitle).NoWrap()
			ui.Text(c, fmt.Sprintf("%d 张导图 · %d 条文档和链接", len(a.files), len(l.Clips))).FontSize(12).TextColor(t.TextMuted).NoWrap()
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
			ui.Spacer(c)
			search := searchBox(c, "lib-search", &a.libQuery, "搜索资料", "搜索资料库", "按标题、内容、摘要、标签和关键词查找资料和导图", 200)
			if a.libQuery != "" && search.Shortcut(0, ui.KeyEscape) {
				a.libQuery = ""
			}
			if search.Changed() {
				a.libQueryAt = a.now()
			}
			a.searchQueryVector(c)
			ui.MenuButton(c, "回顾", a.reviewMenu).Label("回顾").Tooltip("AI 回顾最近收藏的资料，或把资料生成导图").Shrink(0)
			if k := len(l.Unorganized()); k > 0 && a.libWorking == "" {
				if ui.Button(c, fmt.Sprintf("AI 整理 %d 条", k)).Tooltip("让 AI 给还没整理的资料起标题、写摘要、分类和打标签").Shrink(0).Clicked() {
					a.organizeAll()
				}
			}
			if iconButton(c, iconShot, "截图", "截图：框选屏幕、标注后收进资料库（"+keyLabel(a.shotKey())+"）", 30, true).Clicked() {
				a.later(a.screenshot)
			}
			if iconTextButton(c, iconSparkle, "AI", "AI 助手", "和 AI 聊资料库里的内容，它会检索相关资料来回答", a.aiOpen).Clicked() {
				a.aiOpen = !a.aiOpen
			}
			if iconButton(c, iconClose, "关闭资料库", "回到导图", 30, true).Clicked() {
				a.libOpen = false
			}
		})
		ui.Divider(c)
		a.pluginBanner(c)
		a.clipComposer(c)
		a.clipFilters(c)
		if a.libErr != "" {
			ui.Row(c).Padding(0, 12, 6).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.libErr).FontSize(12).TextColor(t.Danger).Wrap().Grow(1)
				if ui.Link(c, "知道了", "").FontSize(12).Clicked() {
					a.libErr = ""
				}
			})
		}
		ui.Divider(c)
		hits := a.libHits()
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			if len(hits) == 0 {
				ui.Column(c).Grow(1).Basis(0).Padding(8, 10, 16).Children(func() {
					msg := "还没有资料。在上面记点什么、粘贴链接，或把图片和文件拖进来。\n在任何应用里复制内容后按 " + keyLabel(a.clipKey()) + "，也能直接收进资料库。"
					if len(l.Clips) > 0 || a.libKind == kindMaps {
						msg = "没有符合条件的资料。"
					}
					ui.Text(c, msg).FontSize(13).TextColor(t.TextMuted).Wrap().Padding(20, 10)
				})
			} else {
				// A virtual list: only the rows in view are built.
				ui.List(c.Key("clips"), nil, len(hits)).Grow(1).Basis(0).ItemKey(func(i int) any { return hits[i].Clip.ID }).
					Rows(func(r ui.ListRow) {
						ui.Box(r.Context).Padding(3, 10).Children(func() {
							if clip := hits[r.Index].Clip; clip.Kind == kindMap {
								a.mapRow(r.Context, clip)
							} else {
								a.clipRow(r.Context, clip)
							}
						})
					})
			}
			if clip := l.Find(a.libSel); clip != nil {
				ui.Box(c).Width(1).FillHeight().Background(t.Border)
				ui.Scroll(c.Key("clip-detail:"+clip.ID)).Grow(1).Basis(0).Padding(14, 16, 20).Gap(12).Children(func() {
					a.clipDetail(c, clip)
				})
			}
		})
	})
	switch ui.AlertDialog(c, &a.libConfirm, "删除这条资料？", "删除后无法恢复。", "取消", "删除") {
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
	k := libHitsKey{a.libQuery, a.libKind, a.libCat, a.libTag, a.lib, a.lib.Version, e.Name(), a.qvecs[q] != nil, a.mapClipsSig}
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
}

func (a *app) searchClips() []library.Hit {
	keep := func(c *library.Clip) bool {
		switch {
		case !ofKind(c, a.libKind):
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
	if a.libKind != kindAll {
		return hits
	}
	// All is material: the maps are listed with the clips.
	if strings.TrimSpace(a.libQuery) == "" {
		hits = append(hits, a.searchMaps("", keep)...) // newest first, together
		slices.SortStableFunc(hits, func(x, y library.Hit) int { return y.Clip.Updated.Compare(x.Clip.Updated) })
		return hits
	}
	var maps []library.Hit // those that match well come first
	for _, h := range a.searchMaps(a.libQuery, keep) {
		if h.Coverage >= 0.5 && len(maps) < 5 {
			maps = append(maps, h)
		}
	}
	return append(maps, hits...)
}

// clipComposer is the field to keep text or a link, and the buttons to
// keep the clipboard or a file.
func (a *app) clipComposer(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Padding(8, 12, 6).Gap(8).AlignItems(ui.End).Children(func() {
		in := ui.TextAreaBase(c.Key("lib-draft"), &a.libDraft).Lines(1, 5).
			Placeholder("记点什么，或粘贴一个链接…（⌘↩ 收藏）").Label("新资料").
			Padding(8).Radius(8).Border(1, t.Border).Background(t.Background).FontSize(13).Grow(1)
		keep := in.Shortcut(ui.Cmd, ui.KeyEnter)
		if ui.PrimaryButton(c, "收藏").Disabled(strings.TrimSpace(a.libDraft) == "").Shrink(0).Clicked() {
			keep = true
		}
		if keep && strings.TrimSpace(a.libDraft) != "" {
			if clip, err := a.addClipText(a.libDraft, "手动"); err != nil {
				a.libErr = err.Error()
			} else {
				a.libDraft, a.libSel = "", clip.ID
			}
		}
		if ui.Button(c, "粘贴").Tooltip("把剪贴板里的文字、链接、图片或文件收进来（任何应用里按 " + keyLabel(a.clipKey()) + "）").Shrink(0).Clicked() {
			if clips, err := a.clipClipboard("剪贴板"); err != nil {
				a.libErr = err.Error()
			} else {
				a.libSel = clips[0].ID
			}
		}
		if ui.Button(c, "文件…").Tooltip("收藏图片或文件（也可以直接拖进来）").Shrink(0).Clicked() {
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

// clipFilters are the kinds, categories and the tag shown.
func (a *app) clipFilters(c *ui.Context) {
	t := c.Theme()
	l := a.lib
	ui.Row(c).Padding(0, 12, 8).Gap(6).Wrap().GapY(6).AlignItems(ui.Center).Children(func() {
		ui.Segmented(c, &a.libKind, "全部", "导图", "文档", "链接").FontSize(12).Shrink(0)
		ui.Box(c).Width(4)
		chip := func(key, label string, active bool) bool {
			b := ui.Box(c.Key("cat:"+key)).Padding(3, 10).Radius(999).Cursor(ui.CursorPointer).Label(label)
			switch {
			case active:
				b.Background(t.Accent)
			case b.Hovered():
				b.Background(t.SurfaceHover)
			default:
				b.Border(1, t.Border)
			}
			col := t.Text
			if active {
				col = t.AccentText
			}
			b.Children(func() { ui.Text(c, label).FontSize(12).TextColor(col).NoWrap() })
			return b.Clicked()
		}
		if chip("", "全部分类", a.libCat == "") {
			a.libCat = ""
		}
		for _, k := range a.allCategories() {
			if chip(k.Name, fmt.Sprintf("%s %d", k.Name, k.N), a.libCat == k.Name) {
				a.libCat = k.Name
			}
		}
		if slices.ContainsFunc(l.Clips, func(c *library.Clip) bool { return c.Starred }) && chip("★", "★ 星标", a.libCat == "★") {
			a.libCat = "★"
		}
		if k := len(l.Unorganized()); k > 0 && chip("-", fmt.Sprintf("未整理 %d", k), a.libCat == "-") {
			a.libCat = "-"
		}
		if a.libTag != "" && tagChip(c.Key("lib-tag"), a.libTag, "#"+a.libTag+" ✕", true, 12).Tooltip("不再按这个标签筛选").Clicked() {
			a.libTag = ""
		}
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
	row := ui.Row(c).Padding(8, 10).Gap(10).Radius(8).Cursor(ui.CursorPointer).Label("资料 " + clip.Label()).
		Drag(clipDrag{clip.ID}) // onto a folder of the sidebar
	switch {
	case a.libSel == clip.ID:
		row.Background(t.Accent.Alpha(0.14))
	case row.Hovered():
		row.Background(t.SurfaceHover)
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
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				tile(c, kindTile(clip.Kind), kindColor(clip.Kind), 20)
				ui.Text(c, clip.Label()).FontSize(13).Bold().SingleLine().Shrink(1).MinWidth(0)
				if clip.Starred {
					glyph(c, iconStarFilled, 12, t.Warning)
				}
			})
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
				if r := []rune(sub); len(r) > 3000 { // the rows are drawn every frame
					sub = string(r[:3000])
				}
				ui.Text(c, library.Excerpt(sub, a.libQuery, 140)).FontSize(12).TextColor(t.TextMuted).Wrap().MaxLines(2)
			}
			ui.Row(c).Gap(6).Wrap().GapY(4).AlignItems(ui.Center).Children(func() {
				if clip.Category != "" {
					ui.Box(c).Padding(1, 7).Radius(4).Background(t.Text.Alpha(0.07)).Children(func() {
						ui.Text(c, clip.Category).FontSize(11).TextColor(t.TextMuted).NoWrap()
					})
				}
				for i, tag := range clip.Tags {
					if i == 4 {
						break
					}
					tagChip(c.Key("t"+tag), tag, "#"+tag, false, 11)
				}
				ui.Text(c, clip.Created.Format("1/2 15:04")).FontSize(11).TextColor(t.TextMuted).NoWrap()
				if s, ok := a.libStatus[clip.ID]; ok {
					col := t.TextMuted
					if strings.Contains(s, "失败") {
						col = t.Danger
					}
					ui.Text(c, s).FontSize(11).TextColor(col).SingleLine().Shrink(1).MinWidth(0)
				}
			})
		})
		if clip.Kind == library.KindImage {
			if b := a.fileBitmap(a.lib.FilePath(clip)); b != nil {
				ui.Image(c, b).Size(64, 48).Fit(ui.Cover).Radius(6).Shrink(0).PassThrough()
			}
		}
	})
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
	a.clipFolderMenu(m, clip)
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
		ui.Row(c.Key("kind-pill")).Padding(2, 8, 2, 6).Gap(4).Radius(999).Background(t.Text.Alpha(0.06)).AlignItems(ui.Center).Shrink(0).Children(func() {
			glyph(c, kindIcon(clip.Kind), 12, t.TextMuted)
			ui.Text(c, library.KindLabel(clip.Kind)).FontSize(textCaption).TextColor(t.TextMuted).NoWrap()
		})
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

	// Properties.
	ui.Column(c).Gap(2).Children(func() {
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
	card(c, t.Accent.Alpha(0.06)).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(14, 14).Draw(func(p *ui.Painter, r ui.Rect) { drawIcon(p, r, 13, iconSparkle, t.Accent) })
			ui.Text(c, "AI 整理").FontSize(textSmall).Bold().TextColor(t.Accent).Grow(1)
			label := "整理"
			if clip.Organized != nil {
				label = "重新整理"
			}
			if a.libWorking == clip.ID {
				ui.Spinner(c)
			} else if ghostButton(c, label, "让 AI 起标题、写说明和摘要、分类、打标签、生成同义词和分析").Clicked() {
				a.organizeClip(clip.ID)
			}
		})
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
	card(c, ui.Hex("#f5b942").Alpha(0.1)).Children(func() {
		sectionHeader(c, "我的备注")
		note := clip.Note
		if inlineArea(c.Key("clip-note"), &note, textBody, 2, 10, "为什么收藏它、想怎么用…", "资料备注").Margin(0, -8).Changed() {
			clip.Note = note
			a.saveClip(clip)
		}
	})

	// The content.
	switch {
	case clip.Kind == library.KindText:
		ui.Column(c).Gap(4).Children(func() {
			sectionHeader(c, "内容")
			text := clip.Text
			if inlineArea(c.Key("clip-text"), &text, textBody+1, 3, 30, "", "资料内容").Margin(0, -8).Changed() {
				clip.Text = text
				a.saveClip(clip)
			}
		})
	case clip.Kind == library.KindLink:
		ui.Column(c).Gap(4).Children(func() {
			sectionHeader(c, "网页正文")
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
		ui.Column(c).Gap(4).Children(func() {
			sectionHeader(c, label)
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
	a.clipBacklinksSection(c, clip)
}

// libraryRow is the library's entry in the sidebar.
func (a *app) libraryRow(c *ui.Context) {
	home := navRow(c.Key("home-row"), iconCategory, tilePurple, "概览", "", a.libOpen && a.libHome)
	if home.Clicked() {
		a.later(func() {
			a.showLibrary()
			a.libHome = true
		})
	}
	all := navRow(c.Key("library-row"), iconLibrary, tileBlue, "全部资料", fmt.Sprint(len(a.lib.Clips)+len(a.files)),
		a.libOpen && !a.libHome && a.libCat == "")
	if all.Clicked() {
		a.later(func() { a.openList(kindAll, "") })
	}
}

// categoryList is the sidebar's section of the categories of all the
// material: the clips' and the maps'.
func (a *app) categoryList(c *ui.Context) {
	cats := a.allCategories()
	if len(cats) == 0 {
		return
	}
	a.section(c, "分类", &a.catsOpen, "", func() {
		for _, k := range cats {
			name := k.Name
			r := navRow(c.Key("cat-row:"+name), iconTag, catColor(name), name, fmt.Sprint(k.N), a.libOpen && !a.libHome && a.libCat == name)
			r.Label("分类 " + name)
			if r.Clicked() {
				a.later(func() { a.openList(kindAll, name) })
			}
		}
	})
}

// allCategories are the categories of the clips and the maps.
func (a *app) allCategories() []library.Count {
	return library.CategoriesOf(slices.Concat(a.lib.Clips, a.mapClips()))
}

// showLibrary opens the library in the main area.
func (a *app) showLibrary() {
	a.commitEditIfAny()
	a.lightbox, a.presenting = "", false
	a.libOpen, a.libHome = true, false
}
