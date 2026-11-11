package main

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// The desk is the page the app opens on, laid out as the work goes: write
// it down, do what is for today, and put what was written where it
// belongs. On top the line to write in; on the left what is to do today
// and the maps; on the right what waits for a place, with where the AI
// would put it.

// homeView is 工作台: its name and the day, the line to write in, then
// today's tasks and the maps beside the stream of what was written lately.
func (a *app) homeView(c *ui.Context) {
	now := a.now()
	t := c.Theme()
	ui.Column(c.Key("home")).Grow(1).Label("工作台").Children(func() {
		pageColumn(c, "home-column", true).Children(func() {
			pageHeader(c, "home-head", "工作台", fmt.Sprintf("%s · %d月%d日", weekdays[now.Weekday()], now.Month(), now.Day()), func() {
				find := key(ui.Row(c.Key("home-find")).Width(240).Height(30).Padding(0, 10).Gap(8).Radius(radiusKey).AlignItems(ui.Center).
					Cursor(ui.CursorPointer).Label("搜索"), t)
				if find.Clicked() {
					a.later(func() { a.openPalette(paletteAll) })
				}
				find.Children(func() {
					glyph(c, iconSearch, 13, faint(t))
					ui.Text(c, "搜索").FontSize(textSmall).TextColor(faint(t)).Grow(1).PassThrough()
					ui.Text(c, "⌘K").FontSize(textCaption).TextColor(faint(t)).PassThrough()
				})
				if iconTextButton(c, iconSparkle, "问 AI", "AI 助手", "问问导图和资料里的内容", a.aiOpen).Clicked() {
					a.aiOpen = !a.aiOpen
				}
			})
			a.homeLine(c)
			ui.Scroll(c.Key("home-scroll")).Grow(1).Basis(0).Margin(16, 0, 0).Children(func() {
				ui.Row(c).Gap(16).AlignItems(ui.Start).FillWidth().Padding(0, 0, 28).Children(func() {
					ui.Column(c).Grow(5).Basis(0).MinWidth(0).Gap(20).Children(func() {
						a.homeTasks(c, now)
						a.homeMaps(c, now)
					})
					ui.Column(c).Grow(6).Basis(0).MinWidth(0).Children(func() { a.homeWaiting(c) })
				})
			})
		})
	})
}

var weekdays = []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}

// dateLabel is a day as 11月9日 周一.
func dateLabel(d time.Time) string {
	return fmt.Sprintf("%d月%d日 %s", d.Month(), d.Day(), weekdays[d.Weekday()])
}

// homeLine is the line to write in, the first thing on the desk. What
// is written says what it is, a task when it says a day, else a note,
// and the card says which as it is written.
func (a *app) homeLine(c *ui.Context) {
	a.writeCard(c, writeCard{
		key: "home-line", label: "记一条灵感或任务", draft: &a.homeDraft, enterSaves: true,
		placeholder: "现在的想法是…  以「周五」这样的日子开头就是任务",
		sendLabel:   "记下 ↩", fileLabel: "加图片", save: a.saveHomeLine,
		reads: func() {
			text := strings.TrimSpace(a.homeDraft)
			if _, due, _ := parseTaskLine(text, a.now()); text != "" && taskLike(text, a.now()) {
				label := "任务"
				if due != nil {
					label += " · " + dayLabel(*due, a.now())
				}
				softTag(c, label)
			}
		},
	})
}

// softTag is a word on a pale wash of the accent, as a tag in a note: it
// pops in.
func softTag(c *ui.Context, text string) ui.Element {
	t := c.Theme()
	bg, ink := t.Accent.Alpha(0.1), t.Accent.MixOklab(t.Text, 0.25)
	return ui.Box(c.Key("tag:"+text)).Height(22).Padding(0, 7).Radius(radiusChip).Center().Background(bg).Shrink(0).
		Transition(ui.ElementTransition{Duration: 240 * time.Millisecond, Ease: easeDetent, Enter: &ui.Motion{Scale: 0.85}}).Children(func() {
		ui.Text(c, text).FontSize(textSmall).TextColor(ink).NoWrap()
	})
}

// titleAndSite takes a link out of a title: the words, and the link's
// site to show small beside them, so that a list reads as words.
func titleAndSite(title string) (string, string) {
	m := linkInTitle.FindString(title)
	if m == "" {
		return title, ""
	}
	site := strings.TrimPrefix(strings.TrimPrefix(m, "https://"), "http://")
	site, _, _ = strings.Cut(site, "/")
	words := strings.TrimSpace(strings.Replace(title, m, "", 1))
	if words == "" {
		words = site
	}
	return words, site
}

var linkInTitle = regexp.MustCompile(`https?://\S+`)

// saveHomeLine keeps what was typed in the desk's line.
func (a *app) saveHomeLine() {
	text := strings.TrimSpace(a.homeDraft)
	if text == "" {
		return
	}
	text = a.resolveRefs(text)
	if taskLike(text, a.now()) {
		a.addTask(text, nil)
	} else {
		if _, err := a.addMemo(text, "工作台"); err != nil {
			a.tell("没有记下：" + err.Error())
			return
		}
		a.tell("已记到灵感")
	}
	a.homeDraft = ""
}

// dueToday are the tasks of today and those overdue, not done.
func (a *app) dueToday(now time.Time) []taskItem {
	day := today(now)
	var due []taskItem
	for _, it := range a.taskItems() {
		if d := it.date(); d != nil && !it.done() && d.Before(day.AddDate(0, 0, 1)) && it.node.Task.Status != model.StatusIdea {
			due = append(due, it)
		}
	}
	return due
}

// homeTaskCount is how many tasks of today the desk shows.
const homeTaskCount = 5

// doneToday are the tasks ticked off today.
func (a *app) doneToday(now time.Time) []taskItem {
	var done []taskItem
	for _, it := range a.taskItems() {
		if it.done() && it.node.Task.DoneAt != nil && sameDay(*it.node.Task.DoneAt, now) {
			done = append(done, it)
		}
	}
	return done
}

// homeTasks are the tasks of today, first on the desk: how far the day
// is, then a row each, with the map it is of; under them those done
// today, where a task goes as it is ticked.
func (a *app) homeTasks(c *ui.Context, now time.Time) {
	t := c.Theme()
	due, done := a.dueToday(now), a.doneToday(now)
	all := len(due) + len(done)
	tray(c, "home-tasks").Children(func() {
		count := ""
		if all > 0 {
			count = fmt.Sprintf("%d / %d", len(done), all)
		}
		trayHead(c, "head:今天要做", "今天要做", count, func() {
			if textLink(c, "全部任务", "全部任务 今天要做").Clicked() {
				a.later(a.openTasks)
			}
		})
		day := today(now)
		row := func(it taskItem) {
			row := listCardRow(c, "home-task:"+it.key()).Cursor(ui.CursorPointer).Label("任务 " + it.node.Title)
			if row.Clicked() {
				a.later(func() {
					a.openTasks()
					a.showTask(it)
				})
			}
			row.Children(func() {
				a.taskCircle(c, it, 20)
				words, site := titleAndSite(it.node.Title)
				ui.Row(c).Gap(8).AlignItems(ui.Center).Grow(1).Shrink(1).MinWidth(0).PassThrough().Children(func() {
					title := ui.Text(c, words).FontSize(15).Shrink(1).MinWidth(0).SingleLine()
					if it.done() {
						title.TextColor(t.TextMuted).Strikethrough()
					}
					if site != "" {
						siteChip(c, site)
					}
				})
				if d := it.date(); !it.done() && d != nil && d.Before(day) {
					n := int(day.Sub(today(*d)).Hours()/24 + 0.5)
					ui.Text(c, fmt.Sprintf("过期 %d 天", n)).FontSize(textSmall).TextColor(t.Danger).Shrink(0).PassThrough()
				}
				ui.Row(c).Gap(7).AlignItems(ui.Center).Shrink(0).MaxWidth(120).PassThrough().Children(func() {
					mapDot(c, a.mapColor(it.path), 8)
					ui.Text(c, it.mapName).FontSize(textSmall).TextColor(t.TextMuted).SingleLine()
				})
			})
		}
		listCard(c, "home-task-list").Children(func() {
			for i, it := range due {
				if i == homeTaskCount {
					break
				}
				row(it)
			}
			if len(due) == 0 {
				msg := "今天没有要做的事。"
				if len(done) > 0 {
					msg = "今天的都做完了。"
				}
				ui.Text(c.Key("all-done"), msg).FontSize(textBody).TextColor(t.TextMuted).Margin(12, 12)
			}
			if more := len(due) - homeTaskCount; more > 0 {
				l := ui.Box(c.Key("home-more-tasks")).Padding(6, 12).Cursor(ui.CursorPointer).Label("还有任务")
				if l.Clicked() {
					a.later(a.openTasks)
				}
				l.Children(func() {
					ui.Text(c, fmt.Sprintf("还有 %d 件", more)).FontSize(textSmall).TextColor(t.TextMuted).PassThrough()
				})
			}
			if len(done) > 0 {
				ui.Box(c.Key("done-rule")).Height(1).Margin(6, 12).Background(hairline(t))
				ui.Text(c.Key("done-head"), "做完的").FontSize(textCaption).TextColor(t.TextMuted).Margin(6, 12, 2)
				for i, it := range done {
					if i == homeTaskCount {
						break
					}
					row(it)
				}
			}
		})
	})
}

// siteChip is the site of a link taken out of a title, small beside it.
func siteChip(c *ui.Context, site string) {
	t := c.Theme()
	ui.Row(c).Height(22).Padding(0, 7).Gap(5).Radius(6).AlignItems(ui.Center).Background(t.Text.Alpha(0.06)).Shrink(0).MaxWidth(170).Children(func() {
		ui.Box(c).Size(6, 6).Radius(2).Background(t.TextMuted).Shrink(0)
		ui.Text(c, site).FontSize(12.5).TextColor(t.Text.MixOklab(t.TextMuted, 0.5)).SingleLine()
	})
}

// homeMaps are the maps opened last, a row each, as plain as the tasks:
// the map's dot and name, its tasks and how far they are.
func (a *app) homeMaps(c *ui.Context, now time.Time) {
	t := c.Theme()
	var files []model.FileInfo
	for _, fi := range a.recentFiles() {
		if fi.Path != a.inboxPath() && len(files) < 4 {
			files = append(files, fi)
		}
	}
	tray(c, "home-maps").Children(func() {
		trayHead(c, "head:我的导图", "我的导图", "", func() {
			if textLink(c, "＋ 新建", "＋ 新建 我的导图").Clicked() {
				a.later(a.newMap)
			}
		})
		listCard(c, "home-map-list").Children(func() {
			if len(files) == 0 {
				ui.Text(c, "还没有导图。").FontSize(textBody).TextColor(t.TextMuted).Margin(12, 12)
			}
			for _, fi := range files {
				col := a.mapColor(fi.Path)
				row := listCardRow(c, "home-map:"+fi.Path).Cursor(ui.CursorPointer).Label("导图 " + mapName(fi.Path))
				if row.Clicked() {
					a.later(func() { _ = a.open(fi.Path) })
				}
				total, done := 0, 0
				f := a.file
				if fi.Path != a.path {
					f = a.indexed(fi.Path)
				}
				if f != nil {
					for _, tn := range f.Tasks() {
						total++
						if tn.Node.Task.Status == model.StatusDone {
							done++
						}
					}
				}
				row.Children(func() {
					ui.Box(c).Size(20, 20).Center().Shrink(0).PassThrough().Children(func() { mapDot(c, col, 8) })
					ui.Text(c, mapName(fi.Path)).FontSize(15).SingleLine().Grow(1).Shrink(1).MinWidth(0).PassThrough()
					sub := "没有任务"
					if total > 0 {
						sub = fmt.Sprintf("%d / %d", done, total)
					}
					ui.Text(c, sub).FontSize(textSmall).TextColor(t.TextMuted).Shrink(0).PassThrough().Tooltip(relTime(now, modTime(fi.Path)) + "打开过")
					if total > 0 {
						ui.Box(c).Width(56).Height(4).Radius(2).Background(t.Text.Alpha(0.07)).Shrink(0).PassThrough().Children(func() {
							ui.Box(c.Key("done")).Height(4).WidthPercent(float32(done) / float32(total) * 100).Radius(2).Background(col).
								Transition(ui.ElementTransition{Duration: 500 * time.Millisecond, Ease: easeDetent, Width: true})
						})
					}
				})
			}
		})
	})
}

// waitingItem is something written down lately: a task of the inbox, or
// a note or a clip no topic links to.
type waitingItem struct {
	task *taskItem
	clip *library.Clip
}

// waiting are the things written down lately: the open tasks of the
// inbox, which wait for a map, and the notes and clips of the last week
// that no topic links to. These leave by themselves: nothing has to be
// done with them.
func (a *app) waiting() []waitingItem {
	var out []waitingItem
	for _, it := range a.taskItems() {
		if it.path == a.inboxPath() && !it.done() {
			out = append(out, waitingItem{task: &it})
		}
	}
	since := a.now().AddDate(0, 0, -7)
	for _, cl := range a.lib.Clips {
		if !cl.Kept && cl.Created.After(since) && len(a.clipBacklinks(cl.ID)) == 0 {
			out = append(out, waitingItem{clip: cl})
		}
	}
	return out
}

// shortPlace is a place as a chip says it: the map and the topic.
func shortPlace(p taskPlace) string {
	parts := strings.Split(p.label, " › ")
	if len(parts) <= 2 {
		return p.label
	}
	return parts[0] + " › " + parts[len(parts)-1]
}

// homeWaitingCount is how many waiting things the desk shows.
const homeWaitingCount = 6

// homeWaiting is 刚记下的: what was written down lately, each a card with
// where the AI would put it, to take or to choose another. Only the
// inbox's tasks wait for a place, a map to be done on; a note or a clip
// has its place, 收集, and may be put on a map as well.
func (a *app) homeWaiting(c *ui.Context) {
	t := c.Theme()
	listed := map[string]bool{} // the tasks of 今天要做 are not listed twice
	for i, it := range a.dueToday(a.now()) {
		if i < homeTaskCount {
			listed[it.key()] = true
		}
	}
	items := slices.DeleteFunc(a.waiting(), func(w waitingItem) bool { return w.task != nil && listed[w.task.key()] })
	suggested := len(a.tasks.places) + len(a.clipPlaces)
	tray(c, "home-waiting").Children(func() {
		trayHead(c, "head:刚记下的", "刚记下的", "收件箱的任务，和这几天记的", func() {
			switch {
			case a.tasks.placing || a.clipPlacing:
				ui.Spinner(c)
			case suggested > 0:
				if ghostButton(c, "全部归位", "按 AI 的建议都放过去").Clicked() {
					a.later(func() {
						if len(a.tasks.places) > 0 {
							a.takeAllPlaces()
						}
						for id := range a.clipPlaces {
							if cl := a.lib.Find(id); cl != nil {
								a.takeClipPlace(cl)
							}
						}
						a.clipPlaces = nil
					})
				}
			case len(items) > 0:
				if iconTextButton(c, iconSparkle, "AI 归位", "AI 归位", "让 AI 建议每条该放到哪张导图的哪个主题下，你确认后才放", false).Clicked() {
					a.later(func() {
						a.suggestPlaces()
						var clips []*library.Clip
						for _, w := range a.waiting() {
							if w.clip != nil {
								clips = append(clips, w.clip)
							}
						}
						if len(clips) > 0 {
							a.suggestClipPlaces(clips)
						}
					})
				}
			}
		})
		if len(items) == 0 {
			emptyNote(c, "这几天记下的会出现在这里。")
			return
		}
		ui.Column(c.Key("home-lately")).Gap(10).FillWidth().Children(func() {
			for i, w := range items {
				if i == homeWaitingCount {
					break
				}
				a.waitingCard(c, w, i)
			}
		})
		if more := len(items) - homeWaitingCount; more > 0 {
			ui.Text(c, fmt.Sprintf("还有 %d 条，在任务的收件箱和收集里", more)).FontSize(textSmall).TextColor(t.TextMuted).Margin(8, 10, 4)
		}
	})
}

// waitingCard is one thing written down, the i-th row of its card: what
// it is, and under it where it may go.
func (a *app) waitingCard(c *ui.Context, w waitingItem, i int) {
	t := c.Theme()
	key, text, kind := "", "", ""
	var place taskPlace
	has := false
	ic := iconPencil
	if w.task != nil {
		key, text, kind = "t:"+w.task.node.ID, w.task.node.Title, "收件箱"
		if d := w.task.date(); d != nil {
			kind = dayLabel(*d, a.now())
		}
		place, has = a.tasks.places[w.task.node.ID]
	} else {
		cl := w.clip
		key, text, kind = "c:"+cl.ID, cl.Label(), relTime(a.now(), cl.Created) // its icon says its kind
		if isMemo(cl) {
			if first, _, _ := strings.Cut(strings.TrimSpace(cl.Text), "\n"); first != "" {
				text = first // else its title or its note, as its label has it
			}
		} else {
			ic = kindIcon(cl.Kind)
		}
		place, has = a.clipPlaces[cl.ID]
	}
	_ = ic
	full := text // with what it refers to, which shows under its words
	text = strings.TrimSpace(clipRef.ReplaceAllString(text, ""))
	// A card each, as a note is: when over the words, what can be done
	// with it under the pointer, where the AI would put it under the words.
	card := raise(ui.Column(c.Key("waiting:"+key)).Padding(14, 20, 16).Gap(4).Radius(12).FillWidth().Label("记下的 "+text), t).
		Transition(rowMotion)
	hover := card.Hovered()
	card.Children(func() {
		ui.Row(c).Height(26).Gap(8).AlignItems(ui.Center).FillWidth().Children(func() {
			if w.task != nil {
				a.taskCircle(c, *w.task, 16)
			}
			ui.Text(c, kind).FontSize(textSmall).TextColor(t.TextMuted)
			ui.Spacer(c)
			if !has {
				label, name := "放到导图…", "放到导图 "+text
				if w.task != nil {
					label, name = "选一个主题…", "选一个主题… "+text
				}
				b := textKey(c, label, name, "放到某张导图的某个主题下；不放也行")
				if !hover {
					b.Opacity(0)
				}
				if b.Clicked() {
					a.later(func() { a.placeWaiting(w) })
				}
			}
		})
		words, site := titleAndSite(full)
		var tags []string // those its words do not already say
		if w.clip != nil {
			for _, tag := range w.clip.Tags {
				if !strings.Contains(full, "#"+tag) && len(tags) < 3 {
					tags = append(tags, tag)
				}
			}
		}
		if site != "" || len(tags) > 0 {
			ui.Row(c).Gap(6).Wrap().GapY(4).AlignItems(ui.Center).FillWidth().Children(func() {
				for _, tag := range tags {
					softTag(c, "#"+tag)
				}
				if site != "" {
					siteChip(c, site)
				}
			})
		}
		a.noteText(c, words, 15.5, 4)
		if has {
			ui.Row(c.Key("sugg")).Gap(8).AlignItems(ui.Center).FillWidth().Padding(8, 0, 0).
				Transition(ui.ElementTransition{Duration: 300 * time.Millisecond, Ease: easeDetent, Enter: &ui.Motion{Y: -6}}).Children(func() {
				softTag(c, "→ "+shortPlace(place)).Tooltip(place.label)
				ui.Spacer(c)
				if textKey(c, "换一个", "换一个 "+text, "自己选放到哪").Clicked() {
					a.later(func() { a.placeWaiting(w) })
				}
				go_ := ui.Box(c.Key("take")).Height(26).Padding(0, 10).Radius(6).Center().Background(t.Accent).Cursor(ui.CursorPointer).
					Label("放过去 " + text).Transition(easeLook)
				if go_.Hovered() {
					go_.Background(t.AccentHover)
				}
				if go_.Clicked() {
					a.later(func() {
						if w.task != nil {
							a.takePlace(*w.task)
						} else {
							a.takeClipPlace(w.clip)
						}
					})
				}
				press(go_, 0.96)
				go_.Children(func() {
					ui.Text(c, "放过去").FontSize(textSmall).FontWeight(600).TextColor(t.AccentText).PassThrough()
				})
			})
		}
	})
}

// placeWaiting asks where something written down goes: a topic of a map.
func (a *app) placeWaiting(w waitingItem) {
	if w.task != nil {
		a.moveTaskTo(*w.task)
		return
	}
	a.attachRef = clipScheme + w.clip.ID
	a.openPalette(paletteAttach)
}

// textKey is a few words that act, with a shade under the pointer and
// no more.
func textKey(c *ui.Context, text, label, tip string) ui.Element {
	t := c.Theme()
	b := ui.Box(c.Key("textkey:"+label)).Height(26).Padding(0, 10).Radius(6).Center().Cursor(ui.CursorPointer).Label(label).Tooltip(tip).
		Shrink(0).Background(t.Text.Alpha(0)).Transition(easeLook)
	if b.Hovered() {
		b.Background(t.Text.Alpha(0.06))
	}
	b.Children(func() {
		ui.Text(c, text).FontSize(textSmall).TextColor(t.Text.MixOklab(t.TextMuted, 0.5)).NoWrap().PassThrough()
	})
	return press(b, 0.96)
}
