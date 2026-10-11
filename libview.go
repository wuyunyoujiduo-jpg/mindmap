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
		ui.Row(c).Padding(8, 12).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "📚 资料库").Bold().FontSize(15).NoWrap()
			ui.Text(c, fmt.Sprintf("%d 条", len(l.Clips))).FontSize(12).TextColor(t.TextMuted).NoWrap()
			if a.libWorking != "" {
				ui.Spinner(c)
				ui.Text(c, fmt.Sprintf("整理中，还有 %d 条", len(a.libQueue)+1)).FontSize(12).TextColor(t.TextMuted).NoWrap()
			}
			ui.Spacer(c)
			search := ui.TextInput(c.Key("lib-search"), &a.libQuery).Placeholder("搜索资料…").Label("搜索资料库").
				Tooltip("按标题、内容、摘要、标签和关键词查找").Width(200).Shrink(1).MinWidth(90)
			if a.libQuery != "" && search.Shortcut(0, ui.KeyEscape) {
				a.libQuery = ""
			}
			if k := len(l.Unorganized()); k > 0 && a.libWorking == "" {
				if ui.Button(c, fmt.Sprintf("AI 整理 %d 条", k)).Tooltip("让 AI 给还没整理的资料起标题、写摘要、分类和打标签").Shrink(0).Clicked() {
					a.organizeAll()
				}
			}
			ai := ui.Button(c, "✦ AI").Label("AI 助手").Tooltip("和 AI 聊资料库里的内容，它会检索相关资料来回答").Shrink(0)
			if a.aiOpen {
				ai.Background(t.Accent).TextColor(t.AccentText)
			}
			if ai.Clicked() {
				a.aiOpen = !a.aiOpen
			}
			if ui.Button(c, "✕").Label("关闭资料库").Tooltip("回到导图").Width(32).Shrink(0).Clicked() {
				a.libOpen = false
			}
		})
		ui.Divider(c)
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
			ui.Scroll(c.Key("clips")).Grow(1).Basis(0).Padding(8, 10, 16).Gap(6).Children(func() {
				if len(hits) == 0 {
					msg := "还没有资料。在上面记点什么、粘贴链接，或把图片和文件拖进来。\n在任何应用里复制内容后按 " + keyLabel(a.clipKey()) + "，也能直接收进资料库。"
					if len(l.Clips) > 0 {
						msg = "没有符合条件的资料。"
					}
					ui.Text(c, msg).FontSize(13).TextColor(t.TextMuted).Wrap().Padding(20, 10)
				}
				for i, h := range hits {
					if i == 300 {
						ui.Textf(c, "还有 %d 条，换个关键词缩小范围", len(hits)-i).FontSize(12).TextColor(t.TextMuted)
						break
					}
					a.clipRow(c.Key(h.Clip.ID), h.Clip)
				}
			})
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

// keyLabel shows an accelerator as macOS writes it.
func keyLabel(accel string) string {
	r := strings.NewReplacer("CmdOrCtrl+", "⌘", "Command+", "⌘", "Cmd+", "⌘", "Control+", "⌃", "Ctrl+", "⌃", "Alt+", "⌥", "Option+", "⌥", "Shift+", "⇧", "Space", "Space")
	return r.Replace(accel)
}

// libHits are the clips the search and the filters find.
func (a *app) libHits() []library.Hit {
	kinds := []string{"", library.KindText, library.KindLink, library.KindImage, library.KindFile}
	kind := kinds[min(a.libKind, len(kinds)-1)]
	keep := func(c *library.Clip) bool {
		switch {
		case kind != "" && c.Kind != kind:
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
	return a.lib.Search(a.libQuery, keep)
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
		ui.Segmented(c, &a.libKind, "全部", "文字", "链接", "图片", "文件").FontSize(12).Shrink(0)
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
		for _, k := range l.Categories() {
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

func kindIcon(kind string) string {
	switch kind {
	case library.KindLink:
		return "🔗"
	case library.KindImage:
		return "🖼"
	case library.KindFile:
		return "📎"
	}
	return "📝"
}

// clipRow is a clip in the list.
func (a *app) clipRow(c *ui.Context, clip *library.Clip) {
	t := c.Theme()
	row := ui.Row(c).Padding(8, 10).Gap(10).Radius(8).Cursor(ui.CursorPointer).Label("资料 " + clip.Label())
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
				ui.Text(c, kindIcon(clip.Kind)).FontSize(12)
				ui.Text(c, clip.Label()).FontSize(13).Bold().NoWrap().Ellipsis("…").Shrink(1).MinWidth(0)
				if clip.Starred {
					ui.Text(c, "★").FontSize(12).TextColor(t.Warning)
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
					ui.Text(c, s).FontSize(11).TextColor(col).NoWrap().Ellipsis("…").Shrink(1).MinWidth(0)
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

// clipDetail shows and edits the clip chosen.
func (a *app) clipDetail(c *ui.Context, clip *library.Clip) {
	t := c.Theme()
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, kindIcon(clip.Kind)+" "+library.KindLabel(clip.Kind)).FontSize(12).TextColor(t.TextMuted)
		ui.Text(c, "· "+cmpOr(clip.Source, "手动")+" · "+clip.Created.Format("2006-01-02 15:04")).FontSize(12).TextColor(t.TextMuted).Grow(1)
		star := ui.Button(c, map[bool]string{true: "★", false: "☆"}[clip.Starred]).Label("星标").Tooltip("星标").Width(30)
		if star.Clicked() {
			clip.Starred = !clip.Starred
			a.saveClip(clip)
		}
		ui.MenuButton(c, "⋯", func(m *ui.Menu) { a.clipMenu(m, clip) }).Label("资料菜单")
		if ui.Button(c, "✕").Label("关闭详情").Width(30).Clicked() {
			a.libSel = ""
		}
	})
	if s, ok := a.libStatus[clip.ID]; ok {
		col := t.TextMuted
		if strings.Contains(s, "失败") {
			col = t.Danger
		}
		ui.Text(c, s).FontSize(12).TextColor(col).Wrap()
	}
	title := clip.Title
	ui.Field(c, "标题", func() {
		if ui.TextInput(c.Key("clip-title"), &title).Placeholder(clip.Label()).Label("资料标题").Changed() {
			clip.Title = title
			a.saveClip(clip)
		}
	})
	desc := clip.Desc
	ui.Field(c, "说明", func() {
		in := ui.TextInput(c.Key("clip-desc"), &desc).Placeholder("一句话说明这是什么（AI 整理时自动生成）").Label("资料说明")
		if in.Changed() {
			clip.Desc = desc
			a.saveClip(clip)
		}
	})
	if clip.Kind == library.KindImage {
		if b := a.fileBitmap(a.lib.FilePath(clip)); b != nil {
			w, h := b.Size()
			k := min(1, 360/float32(max(w, 1)), 280/float32(max(h, 1)))
			img := ui.Image(c, b).Size(float32(w)*k, float32(h)*k).Fit(ui.Contain).Radius(8).Cursor(ui.CursorPointer).Tooltip("用默认应用打开")
			if img.Clicked() {
				a.plat.OpenPath(a.lib.FilePath(clip))
			}
		}
	}
	if clip.URL != "" {
		ui.Field(c, "链接", func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if ui.Link(c, clip.URL, "").FontSize(12).MaxLines(2).Shrink(1).Clicked() {
					a.plat.OpenURL(clip.URL)
				}
			})
		})
	}
	if clip.Kind == library.KindFile {
		ui.Field(c, "文件", func() {
			if ui.Link(c, "📎 "+clip.Name, "").FontSize(12).Clicked() {
				a.plat.OpenPath(a.lib.FilePath(clip))
			}
		})
	}
	if clip.Summary != "" {
		ui.Field(c, "AI 摘要", func() {
			ui.Text(c, clip.Summary).FontSize(13).Wrap().Selectable()
		})
	}
	ui.Field(c, "分类", func() {
		cat := clip.Category
		in := ui.TextInput(c.Key("clip-cat"), &cat).Placeholder("如 技术、产品、灵感").Label("资料分类")
		if in.Changed() {
			clip.Category = strings.TrimSpace(cat)
			a.saveClip(clip)
		}
	})
	ui.Field(c, "标签", func() {
		ui.Row(c).Gap(6).Wrap().GapY(6).AlignItems(ui.Center).Children(func() {
			for _, tag := range clip.Tags {
				tag := tag
				chip := tagChip(c.Key("tag:"+tag), tag, "#"+tag, false, 12).Tooltip("点击筛选；右键移除")
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
			in := ui.TextInput(c.Key("clip-tag"), &a.libTagText).Placeholder("+ 标签").Label("添加标签").Width(110)
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
	})
	note := clip.Note
	ui.Field(c, "我的备注", func() {
		if ui.TextArea(c.Key("clip-note"), &note).Lines(2, 8).Placeholder("为什么收藏它、想怎么用…").Label("资料备注").Changed() {
			clip.Note = note
			a.saveClip(clip)
		}
	})
	if clip.Kind == library.KindText {
		text := clip.Text
		ui.Field(c, "内容", func() {
			if ui.TextArea(c.Key("clip-text"), &text).Lines(4, 20).Label("资料内容").Changed() {
				clip.Text = text
				a.saveClip(clip)
			}
		})
	} else if clip.Text != "" {
		label := "网页正文"
		switch clip.Kind {
		case library.KindImage:
			label = "图中内容（AI 识别）"
		case library.KindFile:
			label = "文件内容"
		}
		ui.Field(c, label, func() {
			ui.Text(c, library.Excerpt(clip.Text, "", 3000)).FontSize(12).TextColor(t.TextMuted).Wrap().Selectable()
		})
	}
	if len(clip.Keywords) > 0 {
		ui.Text(c, "检索关键词："+strings.Join(clip.Keywords, "、")).FontSize(11).TextColor(t.TextMuted).Wrap()
	}
	ui.Row(c).Gap(8).Wrap().GapY(6).Children(func() {
		label := "AI 整理"
		if clip.Organized != nil {
			label = "AI 重新整理"
		}
		if ui.Button(c, label).Disabled(a.libWorking == clip.ID).Tooltip("让 AI 起标题、写摘要、分类、打标签").Clicked() {
			a.organizeClip(clip.ID)
		}
		if ui.Button(c, "问 AI").Tooltip("打开 AI 助手，就这条资料提问").Clicked() {
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
}

// libraryRow is the library's entry in the sidebar.
func (a *app) libraryRow(c *ui.Context) {
	t := c.Theme()
	row := ui.Row(c.Key("library-row")).Padding(6, 8, 6, 12).Gap(8).Radius(6).AlignItems(ui.Center).Cursor(ui.CursorPointer).
		Label("资料库").Tooltip("收藏的文字、链接、图片和文件，AI 自动整理；复制后按 " + keyLabel(a.clipKey()) + " 收进来")
	switch {
	case a.libOpen:
		row.Background(t.Accent.Alpha(0.14))
	case row.Hovered():
		row.Background(t.SurfaceHover)
	}
	if row.Clicked() {
		a.later(a.showLibrary)
	}
	row.Children(func() {
		ui.Text(c, "📚").FontSize(13)
		ui.Text(c, "资料库").FontSize(13).Grow(1)
		if n := len(a.lib.Clips); n > 0 {
			ui.Text(c, fmt.Sprint(n)).FontSize(11).TextColor(t.TextMuted)
		}
	})
}

// showLibrary opens the library in the main area.
func (a *app) showLibrary() {
	a.commitEditIfAny()
	a.lightbox, a.presenting = "", false
	a.libOpen = true
}
