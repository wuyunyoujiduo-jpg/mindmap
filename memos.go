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
	mode      int // all, 每日回顾, 随机漫步
	walk      int // how many times 随机漫步 went on
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

// The ways 灵感 shows the notes: all of them, 每日回顾 (a few of the old
// ones, the same all day), and 随机漫步 (one at a time, at random).
const (
	memosAll = iota
	memosDaily
	memosWalk
)

// memosView is 灵感, as flomo: on a light gray, a rail with the numbers,
// the days written on, the ways to look back and the tags; beside it the
// box to write in and the notes, each a white card.
func (a *app) memosView(c *ui.Context) {
	t := c.Theme()
	list := a.memoShown()
	page, _ := memoColors(t)
	ui.Column(c.Key("memos")).Grow(1).Label("灵感").Background(page).Children(func() {
		ui.Row(c.Key("memos-head")).Height(toolbarHeight).Shrink(0).DragWindow()
		ui.Scroll(c.Key("memo-scroll")).Grow(1).Children(func() {
			ui.Row(c).Padding(0, 32, 48).Gap(36).AlignItems(ui.Start).MaxWidth(1000).Margin(0, ui.Auto).FillWidth().Children(func() {
				ui.Column(c.Key("memo-rail")).Width(232).Shrink(0).Gap(18).Children(func() { a.memoRail(c) })
				ui.Column(c.Key("memo-stream")).Grow(1).Basis(0).MinWidth(0).Gap(14).Children(func() {
					ui.Row(c).Gap(12).AlignItems(ui.Center).MinHeight(32).Children(func() {
						title := "全部灵感"
						switch {
						case a.memos.mode == memosDaily:
							title = "每日回顾"
						case a.memos.mode == memosWalk:
							title = "随机漫步"
						case a.memos.tag != "":
							title = "#" + a.memos.tag
						}
						ui.Text(c, title).FontSize(textTitle).FontWeight(600)
						ui.Spacer(c)
						searchBox(c, "memo-search", &a.memos.query, "搜索灵感", "搜索灵感", "", 220)
					})
					if a.memos.mode == memosAll {
						a.memoComposer(c)
					}
					switch {
					case a.memos.mode == memosWalk && len(list) > 0:
						a.memoCard(c, list[0])
						ui.Row(c).Gap(8).Children(func() {
							ui.Spacer(c)
							if softButton(c, "换一条", "再随机看一条").Clicked() {
								a.memos.walk++
							}
						})
					case len(list) == 0 && a.memos.mode == memosDaily:
						ui.Text(c, "一周以前的灵感会在这里，每天换几条。").FontSize(textBody).TextColor(t.TextMuted).Margin(24, 4)
					case len(list) == 0:
						ui.Text(c, "还没有灵感。在上面写下现在的想法，#标签 归类。").FontSize(textBody).TextColor(t.TextMuted).Margin(24, 4)
					default:
						for _, m := range list {
							a.memoCard(c, m)
						}
					}
				})
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

// memoShown are the notes the way chosen shows: all those of the tag and
// the words searched; a few of those older than a week, by the day; or
// one of them at random.
func (a *app) memoShown() []*library.Clip {
	list := a.memoList()
	if a.memos.mode == memosAll {
		return list
	}
	week := a.now().AddDate(0, 0, -7)
	old := slices.DeleteFunc(slices.Clone(list), func(m *library.Clip) bool { return m.Created.After(week) })
	if len(old) == 0 {
		return nil
	}
	day := today(a.now())
	seed := day.Year()*400 + day.YearDay()
	if a.memos.mode == memosWalk {
		return []*library.Clip{old[(seed*7919+a.memos.walk*104729)%len(old)]}
	}
	n := min(3, len(old))
	out := make([]*library.Clip, 0, n)
	for i := range len(old) {
		if len(out) == n {
			break
		}
		m := old[(seed*31+i*17)%len(old)]
		if !slices.Contains(out, m) {
			out = append(out, m)
		}
	}
	return out
}

// memoComposer is the box to write in, a white card: the text, a row of
// quiet tools, and the button that keeps it, gray until there is text.
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
	_, cardBg := memoColors(t)
	box := ui.Column(c.Key("memo-box")).Padding(14, 16, 10).Gap(8).Radius(12).Background(cardBg).FillWidth()
	box.Children(func() {
		in := ui.TextAreaBase(c.Key("memo-draft"), &a.memos.draft).Lines(3, 14).FontSize(15).LineHeight(1.7).FillWidth().
			Placeholder("现在的想法是…").Label("写下灵感")
		if in.Focused() {
			box.Border(1, t.Accent.Alpha(0.35))
		}
		if in.Shortcut(ui.Cmd, ui.KeyEnter) && !empty {
			keep()
		}
		ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
			tool := func(text, tip, insert string) {
				b := ui.Box(c.Key("memo-tool:"+text)).Size(30, 28).Radius(6).Center().Cursor(ui.CursorPointer).Label(tip).Tooltip(tip)
				if b.Hovered() {
					b.Background(t.Text.Alpha(0.06))
				}
				if b.Clicked() {
					if a.memos.draft != "" && !strings.HasSuffix(a.memos.draft, "\n") && insert != "#" {
						a.memos.draft += "\n"
					}
					a.memos.draft += insert
				}
				b.Children(func() { ui.Text(c, text).FontSize(14).TextColor(t.TextMuted).PassThrough() })
			}
			tool("#", "插入标签", "#")
			tool("•", "列表", "- ")
			tool("1.", "编号列表", "1. ")
			ui.Spacer(c)
			ui.Text(c, "⌘↩").FontSize(textCaption).TextColor(t.TextMuted).Margin(0, 8, 0, 0)
			send := ui.Box(c.Key("memo-send")).Height(30).Padding(0, 16).Radius(8).Center().Label("记录").Tooltip("记录（⌘↩）")
			if empty {
				send.Background(t.Text.Alpha(0.08))
			} else {
				send.Background(t.Accent).Cursor(ui.CursorPointer)
				if send.Hovered() {
					send.Background(t.AccentHover)
				}
			}
			if send.Clicked() && !empty {
				keep()
			}
			send.Children(func() {
				col := ui.Hex("#FFFFFF")
				if empty {
					col = t.TextMuted
				}
				ui.Text(c, "记录").FontSize(textSmall).FontWeight(600).TextColor(col).PassThrough()
			})
		})
	})
}

// memoRail is the column beside the notes: how many notes, tags and days;
// the days written on; the ways to look back; the tags.
func (a *app) memoRail(c *ui.Context) {
	t := c.Theme()
	per := map[string]int{}
	total := 0
	counts := map[string]int{}
	var order []string
	for _, cl := range a.lib.Clips {
		if !isMemo(cl) {
			continue
		}
		per[cl.Created.In(time.Local).Format("2006-01-02")]++
		total++
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
	slices.SortStableFunc(order, func(x, y string) int { return counts[y] - counts[x] })
	ui.Row(c.Key("memo-numbers")).Gap(22).Padding(4, 4, 0).Children(func() {
		for _, n := range []struct {
			v     int
			label string
		}{{total, "条灵感"}, {len(order), "个标签"}, {len(per), "天"}} {
			ui.Column(c).Gap(2).Children(func() {
				ui.Text(c, fmt.Sprint(n.v)).FontSize(22).FontWeight(600).TextColor(t.Text.MixOklab(t.TextMuted, 0.3))
				ui.Text(c, n.label).FontSize(textCaption).TextColor(t.TextMuted)
			})
		}
	})
	// The days written on, the last 16 weeks: a square a day, a week a
	// column, the months under them.
	const weeks = 16
	end := today(a.now())
	start := weekStart(end).AddDate(0, 0, -7*(weeks-1))
	cell, gap := float32(11), float32(3)
	ui.Box(c.Key("memo-heat")).Size(weeks*(cell+gap), 7*(cell+gap)+16).Margin(0, 4).Label("最近 16 周记录的天数").
		Draw(func(p *ui.Painter, r ui.Rect) {
			for w := range weeks {
				for d := range 7 {
					day := start.AddDate(0, 0, w*7+d)
					if day.After(end) {
						continue
					}
					n := per[day.Format("2006-01-02")]
					col := t.Text.Alpha(0.06)
					if n > 0 {
						col = t.Accent.Alpha(min(0.3+0.22*float32(n), 1))
					}
					x, y := r.X+float32(w)*(cell+gap), r.Y+float32(d)*(cell+gap)
					p.Fill(ui.Rect{X: x, Y: y, W: cell, H: cell}, col, 3)
				}
			}
		}).Children(func() {
		for w := range weeks {
			day := start.AddDate(0, 0, w*7)
			if day.Day() <= 7 {
				ui.Text(c, fmt.Sprintf("%d月", day.Month())).FontSize(textCaption).TextColor(t.TextMuted).
					Absolute().Left(float32(w) * (cell + gap)).Top(7*(cell+gap) + 2)
			}
		}
	})
	ui.Column(c.Key("memo-ways")).Gap(1).Children(func() {
		for _, w := range []struct {
			mode int
			ic   icon
			name string
		}{{memosAll, iconCategory, "全部灵感"}, {memosDaily, iconSparkle, "每日回顾"}, {memosWalk, iconTarget, "随机漫步"}} {
			on := a.memos.mode == w.mode && (w.mode != memosAll || a.memos.tag == "")
			if navRow(c.Key("memo-way:"+w.name), w.ic, tileOrange, w.name, "", on).Clicked() {
				a.memos.mode, a.memos.tag = w.mode, ""
			}
		}
	})
	if len(order) == 0 {
		return
	}
	ui.Column(c.Key("memo-tags")).Gap(1).Children(func() {
		ui.Text(c, "标签").FontSize(textCaption).FontWeight(600).TextColor(t.TextMuted).Margin(4, 10, 6)
		for _, tag := range order {
			on := a.memos.tag == tag
			row := ui.Row(c.Key("memo-tag:"+tag)).Height(30).Padding(0, 10).Gap(8).Radius(7).AlignItems(ui.Center).FillWidth().
				Cursor(ui.CursorPointer).Label(fmt.Sprintf("#%s %d", tag, counts[tag]))
			switch {
			case on:
				row.Background(chosen(t))
			case row.Hovered():
				row.Background(t.Text.Alpha(0.05))
			}
			if row.Clicked() {
				a.memos.mode = memosAll
				if on {
					a.memos.tag = ""
				} else {
					a.memos.tag = tag
				}
			}
			row.Children(func() {
				ui.Text(c, "#").FontSize(14).TextColor(t.TextMuted).PassThrough()
				txt := ui.Text(c, tag).FontSize(textBody).Grow(1).SingleLine().PassThrough()
				if on {
					txt.FontWeight(600)
				}
				ui.Text(c, fmt.Sprint(counts[tag])).FontSize(textSmall).TextColor(t.TextMuted).PassThrough()
			})
		}
	})
}

// memoCard is a note, a white card: when it was written and where from,
// its menu; its text with its #tags. A double click, or 编辑 in its menu,
// edits it where it is.
func (a *app) memoCard(c *ui.Context, m *library.Clip) {
	t := c.Theme()
	_, cardBg := memoColors(t)
	card := ui.Column(c.Key("memo:"+m.ID)).Padding(14, 18, 16).Gap(8).Radius(12).Background(cardBg).FillWidth()
	if card.DoubleClicked() && a.memos.editing != m.ID {
		a.memos.editing, a.memos.editText = m.ID, m.Text
	}
	hover := card.Hovered()
	card.Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).MinHeight(22).Children(func() {
			meta := m.Created.In(time.Local).Format("2006-01-02 15:04")
			if m.Source != "" && m.Source != "灵感" {
				meta += "  |  " + m.Source
			}
			ui.Text(c, meta).FontSize(textSmall).TextColor(t.TextMuted)
			ui.Spacer(c)
			more := iconButton(c.Key("memo-more:"+m.ID), iconMore, "更多", "更多", 22, true)
			if !hover {
				more.Opacity(0.5)
			}
			more.Menu(func(mm *ui.Menu) {
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
			in := ui.TextAreaBase(c.Key("memo-edit"), &a.memos.editText).Lines(3, 20).FontSize(15).LineHeight(1.7).FillWidth().
				AutoFocus().Label("编辑灵感")
			save := in.Shortcut(ui.Cmd, ui.KeyEnter)
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Spacer(c)
				if ghostButton(c, "取消", "不保存（Esc）").Clicked() || in.Shortcut(0, ui.KeyEscape) {
					a.memos.editing = ""
				}
				if accentButton(c, "保存", "保存（⌘↩）").Clicked() {
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
					b := pill(c.Key("extra:"+tag), "#"+tag, t.Accent.Alpha(0.1), t.Accent).Cursor(ui.CursorPointer)
					if b.Clicked() {
						a.memos.mode, a.memos.tag = memosAll, tag
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
				spans = append(spans, ui.Span{Text: line[last:m[0]], Size: 15, Color: t.Text})
			}
			spans = append(spans, ui.Span{Text: line[m[0]:m[1]], Size: 15, Color: t.Accent})
			last = m[1]
		}
		if last < len(line) {
			spans = append(spans, ui.Span{Text: line[last:], Size: 15, Color: t.Text})
		}
		ui.RichText(c.Key(fmt.Sprint("line:", i)), spans...).LineHeight(1.75).Wrap().Selectable().FillWidth()
	}
}

// memoColors are 灵感's page and its cards': white cards on a light gray,
// or, dark, lighter cards on the dark.
func memoColors(t *ui.Theme) (page, card ui.Color) {
	if t.Dark {
		return t.Background, t.Surface
	}
	return t.Surface, t.Background
}
