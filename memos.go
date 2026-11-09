package main

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
)

// 灵感, as flomo has it: a box to write in, and what was written below it,
// the last first, a card each; #tags written in the text sort them. They
// are the library's texts, found by its search, sorted by its AI and
// dropped on nodes as any clip; the shortcut of quick capture writes them
// too.

// memosState is the page of 灵感.
type memosState struct {
	open      bool
	draft     string
	tag       string // the tag the cards are filtered by
	query     string
	editing   string // the card being edited, by its clip's ID
	editText  string
	deleting  string
	confirmed bool
}

// memoTag is a #tag in a text: # then what is up to a space or another #.
var memoTag = regexp.MustCompile(`#[^\s#，。,.!！?？;；:：、（）()\[\]{}"'“”]+`)

// memoTags are the #tags written in a text, without their #.
func memoTags(text string) []string {
	var out []string
	for _, m := range memoTag.FindAllString(text, -1) {
		if t := strings.TrimPrefix(m, "#"); !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	return out
}

// isMemo tells whether a clip is one of 灵感: a text of the library.
func isMemo(c *library.Clip) bool { return c.Kind == library.KindText }

// addMemo keeps a note of 灵感: text, its #tags its tags.
func (a *app) addMemo(text, source string) (*library.Clip, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("没有内容")
	}
	c := &library.Clip{Kind: library.KindText, Text: text, Source: source, Tags: memoTags(text)}
	return a.kept(c, a.lib.Add(c, a.now()))
}

// openMemos shows 灵感 in the main area.
func (a *app) openMemos() {
	a.showLibrary()
	a.memos.open = true
}

// memosRow is the sidebar's row of 灵感.
func (a *app) memosRow(c *ui.Context) {
	n := 0
	for _, cl := range a.lib.Clips {
		if isMemo(cl) {
			n++
		}
	}
	count := ""
	if n > 0 {
		count = fmt.Sprint(n)
	}
	row := navRow(c.Key("memos-row"), iconPencil, tileOrange, "灵感", count, a.libOpen && a.memos.open).
		Tooltip("随手记下的想法，#标签 归类（" + keyLabel(a.captureKey()) + " 在任何地方记）")
	if row.Clicked() {
		a.later(a.openMemos)
	}
}

// memoList are the notes shown, the last first: of the tag chosen, with
// the words searched.
func (a *app) memoList() []*library.Clip {
	q := strings.ToLower(strings.TrimSpace(a.memos.query))
	var out []*library.Clip
	for _, c := range a.lib.Clips {
		if !isMemo(c) {
			continue
		}
		if a.memos.tag != "" && !slices.Contains(c.Tags, a.memos.tag) && !slices.Contains(memoTags(c.Text), a.memos.tag) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(c.Text+" "+c.Note), q) {
			continue
		}
		out = append(out, c)
	}
	slices.SortStableFunc(out, func(x, y *library.Clip) int { return y.Created.Compare(x.Created) })
	return out
}

// memosView is 灵感: the box, the tags and the days written on, and the
// cards.
func (a *app) memosView(c *ui.Context) {
	t := c.Theme()
	list := a.memoList()
	ui.Column(c.Key("memos")).Grow(1).Label("灵感").Children(func() {
		ui.Row(c.Key("memos-head")).Height(toolbarHeight).Padding(0, 16, 0, 20).Gap(12).AlignItems(ui.Center).Shrink(0).
			DragWindow().Children(func() {
			ui.Column(c).Gap(1).Children(func() {
				ui.Text(c, "灵感").Bold().FontSize(textTitle)
				sub := fmt.Sprintf("%d 条", len(list))
				if a.memos.tag != "" {
					sub = "#" + a.memos.tag + " · " + sub
				}
				ui.Text(c, sub).FontSize(11).TextColor(t.TextMuted)
			})
			ui.Spacer(c)
			searchBox(c, "memo-search", &a.memos.query, "搜索灵感", "搜索灵感", "", 220)
		})
		ui.Divider(c)
		ui.Scroll(c.Key("memo-scroll")).Grow(1).Children(func() {
			ui.Column(c).Padding(20, 24, 40).Gap(14).MaxWidth(680).Margin(0, ui.Auto).FillWidth().Children(func() {
				a.memoComposer(c)
				a.memoHeatmap(c)
				a.memoTagBar(c)
				if len(list) == 0 {
					ui.Text(c, "还没有灵感。在上面写下现在的想法，#标签 归类。").FontSize(textBody).TextColor(t.TextMuted).Margin(24, 0)
				}
				for _, m := range list {
					a.memoCard(c, m)
				}
			})
		})
	})
	switch ui.AlertDialog(c, &a.memos.confirmed, "删除这条灵感？", "删除后无法恢复。", "取消", "删除") {
	case 1:
		if err := a.lib.Delete(a.memos.deleting); err != nil {
			a.tell("没有删除：" + err.Error())
		}
		a.noteLibraryWrite()
		a.memos.deleting = ""
	}
}

// memoComposer is the box to write in: ⌘↩ or the button keeps it.
func (a *app) memoComposer(c *ui.Context) {
	t := c.Theme()
	empty := strings.TrimSpace(a.memos.draft) == ""
	keep := func() {
		text := a.memos.draft
		a.memos.draft = ""
		a.later(func() {
			if _, err := a.addMemo(text, "灵感"); err != nil {
				a.tell(err.Error())
			}
		})
	}
	ui.Column(c.Key("memo-box")).Padding(12, 14, 10).Gap(8).Radius(12).Border(1, hairline(t)).Background(t.Background).
		FillWidth().Children(func() {
		in := ui.TextAreaBase(c.Key("memo-draft"), &a.memos.draft).Lines(3, 14).FontSize(textBody).FillWidth().
			Placeholder("现在的想法是…  #标签 归类").Label("写下灵感")
		if in.Shortcut(ui.Cmd, ui.KeyEnter) && !empty {
			keep()
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if ghostButton(c, "# 标签", "插入标签").Clicked() {
				a.memos.draft += "#"
			}
			ui.Spacer(c)
			ui.Text(c, "⌘↩").FontSize(textCaption).TextColor(t.TextMuted)
			if ui.PrimaryButton(c, "记录").Disabled(empty).Clicked() {
				keep()
			}
		})
	})
}

// memoHeatmap is the days written on, the last 16 weeks, a square a day,
// the darker the more.
func (a *app) memoHeatmap(c *ui.Context) {
	t := c.Theme()
	per := map[string]int{}
	total := 0
	for _, cl := range a.lib.Clips {
		if isMemo(cl) {
			per[cl.Created.In(time.Local).Format("2006-01-02")]++
			total++
		}
	}
	if total == 0 {
		return
	}
	const weeks = 16
	end := today(a.now())
	start := weekStart(end).AddDate(0, 0, -7*(weeks-1))
	days := 0
	for _, n := range per {
		if n > 0 {
			days++
		}
	}
	ui.Row(c.Key("memo-heat")).Gap(16).AlignItems(ui.Center).Padding(2, 2).Children(func() {
		ui.Box(c).Size(weeks*13, 7*13).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
			for w := range weeks {
				for d := range 7 {
					day := start.AddDate(0, 0, w*7+d)
					if day.After(end) {
						continue
					}
					n := per[day.Format("2006-01-02")]
					col := t.Text.Alpha(0.06)
					if n > 0 {
						col = tileGreen.Alpha(min(0.35+0.2*float32(n), 1))
					}
					p.Fill(ui.Rect{X: r.X + float32(w)*13, Y: r.Y + float32(d)*13, W: 10, H: 10}, col, 2)
				}
			}
		})
		ui.Column(c).Gap(4).Children(func() {
			ui.Text(c, fmt.Sprint(total)).FontSize(textHeadline).Bold()
			ui.Text(c, fmt.Sprintf("条灵感 · %d 天", days)).FontSize(textCaption).TextColor(t.TextMuted)
		})
	})
}

// memoTagBar is the tags of 灵感, the most used first: a click shows only
// those of the tag.
func (a *app) memoTagBar(c *ui.Context) {
	t := c.Theme()
	counts := map[string]int{}
	var order []string
	for _, cl := range a.lib.Clips {
		if !isMemo(cl) {
			continue
		}
		seen := map[string]bool{}
		for _, tag := range append(memoTags(cl.Text), cl.Tags...) {
			if seen[tag] {
				continue
			}
			seen[tag] = true
			if counts[tag] == 0 {
				order = append(order, tag)
			}
			counts[tag]++
		}
	}
	if len(order) == 0 {
		return
	}
	slices.SortStableFunc(order, func(x, y string) int { return counts[y] - counts[x] })
	ui.Row(c.Key("memo-tags")).Gap(6).Wrap().GapY(6).Children(func() {
		chip := func(key, label string, on bool) ui.Element {
			b := ui.Box(c.Key("memo-tag:"+key)).Padding(3, 10).Radius(999).Cursor(ui.CursorPointer).Label(label)
			switch {
			case on:
				b.Background(t.Text.Alpha(0.85))
			case b.Hovered():
				b.Background(t.Text.Alpha(0.08))
			default:
				b.Background(t.Text.Alpha(0.045))
			}
			b.Children(func() {
				col := t.Text
				if on {
					col = t.Background
				}
				ui.Text(c, label).FontSize(textSmall).TextColor(col).PassThrough()
			})
			return b
		}
		if chip("", "全部", a.memos.tag == "").Clicked() {
			a.memos.tag = ""
		}
		for _, tag := range order {
			if chip(tag, fmt.Sprintf("#%s %d", tag, counts[tag]), a.memos.tag == tag).Clicked() {
				if a.memos.tag == tag {
					a.memos.tag = ""
				} else {
					a.memos.tag = tag
				}
			}
		}
	})
}

// memoCard is a note: when it was written, its text with its #tags, which
// a click filters by; its menu edits, copies, puts it on a map or deletes
// it.
func (a *app) memoCard(c *ui.Context, m *library.Clip) {
	t := c.Theme()
	card := ui.Column(c.Key("memo:"+m.ID)).Padding(12, 16, 14).Gap(8).Radius(12).Background(t.Background).
		Border(1, hairline(t)).FillWidth()
	card.Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, m.Created.In(time.Local).Format("2006-01-02 15:04")).FontSize(textCaption).TextColor(t.TextMuted)
			ui.Spacer(c)
			iconButton(c.Key("memo-more:"+m.ID), iconMore, "更多", "更多", 22, true).Menu(func(mm *ui.Menu) {
				if mm.Item("编辑").Chosen() {
					a.memos.editing, a.memos.editText = m.ID, m.Text
				}
				if mm.Item("复制").Chosen() {
					a.plat.CopyText(m.Text)
					a.tell("已复制")
				}
				if mm.Item("在资料库中打开").Chosen() {
					id := m.ID
					a.later(func() { a.openList(kindAll, ""); a.libSel = id })
				}
				if a.doc != nil && mm.Item("放进「"+mapName(a.path)+"」").Chosen() {
					a.later(func() { a.clipIntoMap(m) })
				}
				mm.Separator()
				if mm.Item("删除").Chosen() {
					a.memos.deleting, a.memos.confirmed = m.ID, true
				}
			})
		})
		if a.memos.editing == m.ID {
			in := ui.TextAreaBase(c.Key("memo-edit"), &a.memos.editText).Lines(3, 20).FontSize(textBody).FillWidth().
				AutoFocus().Label("编辑灵感")
			save := in.Shortcut(ui.Cmd, ui.KeyEnter)
			ui.Row(c).Gap(8).Children(func() {
				ui.Spacer(c)
				if ui.Button(c, "取消").Clicked() || in.Shortcut(0, ui.KeyEscape) {
					a.memos.editing = ""
				}
				if ui.PrimaryButton(c, "保存").Clicked() {
					save = true
				}
			})
			if save {
				text := strings.TrimSpace(a.memos.editText)
				a.memos.editing = ""
				if text != "" && text != m.Text {
					old := memoTags(m.Text)
					m.Text = text
					// The tags written in the text follow it; the others stay.
					tags := slices.DeleteFunc(slices.Clone(m.Tags), func(s string) bool { return slices.Contains(old, s) })
					for _, tag := range memoTags(text) {
						if !slices.Contains(tags, tag) {
							tags = append(tags, tag)
						}
					}
					m.Tags = tags
					a.saveClip(m)
				}
			}
			return
		}
		a.memoText(c, m.Text)
		if extra := slices.DeleteFunc(slices.Clone(m.Tags), func(s string) bool { return slices.Contains(memoTags(m.Text), s) }); len(extra) > 0 {
			ui.Row(c).Gap(6).Wrap().Children(func() {
				for _, tag := range extra { // the AI's
					b := ui.Text(c.Key("extra:"+tag), "#"+tag).FontSize(textCaption).TextColor(t.TextMuted).Cursor(ui.CursorPointer)
					if b.Clicked() {
						a.memos.tag = tag
					}
				}
			})
		}
	})
}

// memoText shows a note's text, its #tags in the accent.
func (a *app) memoText(c *ui.Context, text string) {
	t := c.Theme()
	for i, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			ui.Box(c.Key(fmt.Sprint("gap:", i))).Height(6)
			continue
		}
		var spans []ui.Span
		last := 0
		for _, m := range memoTag.FindAllStringIndex(line, -1) {
			if m[0] > last {
				spans = append(spans, ui.Span{Text: line[last:m[0]], Size: textBody, Color: t.Text})
			}
			spans = append(spans, ui.Span{Text: line[m[0]:m[1]], Size: textBody, Color: t.Accent})
			last = m[1]
		}
		if last < len(line) {
			spans = append(spans, ui.Span{Text: line[last:], Size: textBody, Color: t.Text})
		}
		ui.RichText(c.Key(fmt.Sprint("line:", i)), spans...).LineHeight(1.6).Wrap().Selectable().FillWidth()
	}
}
