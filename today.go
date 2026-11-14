package main

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// The desk is the page the app opens on, laid out as the work goes: write
// it down, do what is for today, and put what was written where it
// belongs. On top the line to write in; on the left what is to do today
// and the maps; on the right what waits for a place, with where the AI
// would put it.

var weekdays = []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}

// dateLabel is a day as 11月9日 周一.
func dateLabel(d time.Time) string {
	return fmt.Sprintf("%d月%d日 %s", d.Month(), d.Day(), weekdays[d.Weekday()])
}

// softTag is a word on a pale wash of the accent, as a tag in a note: it
// pops in.
func softTag(c *ui.Context, text string) ui.Element {
	t := c.Theme()
	bg, ink := t.Accent.Alpha(0.1), t.Accent.MixOklab(t.Text, 0.25)
	return ui.Box(c.Key("tag:"+text)).Height(24).Padding(0, 8).Radius(6).Center().Background(bg).Shrink(0).
		Transition(ui.ElementTransition{Duration: motionFast, Ease: easeDetent, Enter: &ui.Motion{Scale: 0.85}}).Children(func() {
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
	switch ask, asking := barQuestion(text); {
	case asking:
		if ask == "" {
			return
		}
		// Asked of the AI, in its panel.
		a.aiOpen, a.ai.draft = true, ask
		a.homeDraft, a.barOpen = "", false
		a.later(a.aiSend)
		return
	case a.barTask(text):
		var day *time.Time
		if a.place() == placeToday { // written on 今天, for today unless it says a day
			d := today(a.now())
			day = &d
		}
		a.addTask(text, day)
	case a.place() == placeCollect || linkInTitle.MatchString(text):
		// In 收集, and a link anywhere: kept as what it is, and shown there.
		clip, err := a.addClipText(text, "手动")
		if err != nil {
			a.tell("没有记下：" + err.Error())
			return
		}
		if a.place() == placeCollect {
			a.libSel = clip.ID
		} else {
			a.tell("已收藏")
		}
	default:
		if _, err := a.addMemo(text, "工作台"); err != nil {
			a.tell("没有记下：" + err.Error())
			return
		}
		a.tell("已记到灵感")
	}
	a.homeDraft, a.barKind = "", barAuto
	if !a.libOpen {
		a.barOpen = false // over a map it is put away once it has kept
	}
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

// siteChip is the site of a link taken out of a title, small beside it.
func siteChip(c *ui.Context, site string) {
	t := c.Theme()
	ui.Row(c).Height(22).Padding(0, 7).Gap(5).Radius(6).AlignItems(ui.Center).Background(t.Text.Alpha(0.06)).Shrink(0).MaxWidth(170).Children(func() {
		ui.Box(c).Size(6, 6).Radius(2).Background(t.TextMuted).Shrink(0)
		ui.Text(c, site).FontSize(12.5).TextColor(t.Text.MixOklab(t.TextMuted, 0.5)).SingleLine()
	})
}

// waitingItem is something written down lately: a task of the inbox, or
// a note or a clip no topic links to.
type waitingItem struct {
	task *taskItem
	clip *library.Clip
}

// waiting is what waits for a place: the open tasks of the inbox, which
// wait for a map, and the clips the user asked to place later
// (Clip.ToPlace) that no topic links to yet. A clip waits for nothing by
// itself: it is at home in 收集, found by its tags and the search. Before
// 0.88 every clip of the week waited, and the page read as a debt.
func (a *app) waiting() []waitingItem {
	var out []waitingItem
	for _, it := range a.taskItems() {
		if it.path == a.inboxPath() && !it.done() {
			out = append(out, waitingItem{task: &it})
		}
	}
	for _, cl := range a.lib.Clips {
		if cl.ToPlace && len(a.clipBacklinks(cl.ID)) == 0 {
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
		trayHead(c, "head:刚记下的", "刚记下的", "收件箱的任务，和标了待归位的资料", func() {
			switch {
			case a.tasks.placing || a.clipPlacing: // said under the heading, by its steps
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
				if sparkKey(c, "AI 归位", "让 AI 建议每条该放到哪张导图的哪个主题下，你确认后才放").Clicked() {
					a.later(a.askAllPlaces)
				}
			}
		})
		// What was just left in 收集, and the way back: it stays said until
		// another is left, or the page is.
		// The AI asked where each would go: what it is doing, or why it
		// did not, here, with the way to try again.
		placing := a.tasks.placing || a.clipPlacing
		if !placing {
			stageOver("place-all")
		}
		switch failed := cmpOr(a.tasks.placeErr, a.clipPlaceErr); {
		case placing:
			stageLines(c, "place-all", []string{fmt.Sprintf("读了 %d 条等着归位的，和各导图的主题", len(items))}, "AI 正在给每条找去处…")
		case placeFailed(failed):
			ui.Row(c.Key("place-failed")).Padding(2, 4, 8).Gap(8).AlignItems(ui.Center).FillWidth().Children(func() {
				ui.Text(c, failed).FontSize(textSmall).TextColor(t.Danger).Grow(1).Shrink(1).MinWidth(0).Wrap()
				if sparkKey(c, "重试", "再让 AI 看一遍").Clicked() {
					a.later(a.askAllPlaces)
				}
				if addButton(c, "知道了", "").Clicked() {
					a.tasks.placeErr, a.clipPlaceErr = "", ""
				}
			})
		}
		a.keptLastRow(c)
		if len(items) == 0 {
			emptyNote(c, "收件箱里的任务，和你标了「待归位」的资料，会出现在这里。")
			return
		}
		// Each thing a card of its own on the white page, as 收集's wall.
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

// askAllPlaces asks the AI where each thing that waits would go: the tasks
// of the inbox and the notes. Nothing is moved until each is agreed to.
func (a *app) askAllPlaces() {
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
}

// keptLastRow says what was just left in 收集, and gives the way back: it
// stays said until another is left, or the page is.
func (a *app) keptLastRow(c *ui.Context) {
	t := c.Theme()
	cl := a.lib.Find(a.keptLast)
	if cl == nil || cl.ToPlace {
		return
	}
	ui.Row(c.Key("kept-last")).MinHeight(36).Margin(0, 0, 8).Padding(4, 12, 4, 20).Gap(8).Radius(radiusRow).AlignItems(ui.Center).FillWidth().
		Background(blockFill(t)).Transition(rowMotion).Children(func() {
		ui.Text(c, "不用归位了，它还在收集里：").FontSize(textSmall).TextColor(t.TextMuted).Shrink(0)
		ui.Text(c, refName(cl)).FontSize(textSmall).SingleLine().Grow(1).Shrink(1).MinWidth(0)
		if ghostButton(c, "撤回", "放回「等着归位」").Clicked() {
			a.later(func() { a.unkeep(cl) })
		}
	})
}

// bandCount is how many waiting things today's page shows.
const bandCount = 4

// waitingOnToday is what waits for a place and is not among the tasks
// shown: today's page says it once.
func (a *app) waitingOnToday(shown []taskItem) []waitingItem {
	now := a.now()
	listed := map[string]bool{}
	for _, it := range shown {
		if d := it.date(); d != nil && !it.done() && !d.After(today(now)) {
			listed[it.key()] = true // today's and the late are above already
		}
	}
	return slices.DeleteFunc(a.waiting(), func(w waitingItem) bool { return w.task != nil && listed[w.task.key()] })
}

// railRoom is the least room today's page needs to stand what waits
// beside the tasks rather than under them.
const railRoom = 1180

// todayRoom is the room today's page has: the window's, less the left
// column and the details of a task chosen.
func (a *app) todayRoom(c *ui.Context) float32 {
	w, _ := c.Size()
	room := w - 12
	if a.sideShown() {
		room -= sideWidthOf
	}
	if a.tasks.sel != "" {
		room -= min(400, w*0.4)
	}
	return room
}

// waitingHead is the heading of what waits on today's page, with the
// AI's key and the way to the list where each is placed.
func (a *app) waitingHead(c *ui.Context, n int) {
	all := func() { a.tasks.view, a.tasks.onMap = tasksWaiting, "" }
	trayHead(c, "head:等着归位", "等着归位", fmt.Sprintf("%d 条", n), func() {
		// The AI's key is here too: it asks, and shows the list, where
		// each of its places is agreed to on its card.
		switch {
		case a.tasks.placing || a.clipPlacing:
			ui.Spinner(c)
		default:
			if sparkKey(c, "AI 归位", "让 AI 建议每条该放到哪张导图的哪个主题下；到「等着归位」里逐条确认后才放").Clicked() {
				a.later(func() {
					a.askAllPlaces()
					all()
				})
			}
		}
		if textLink(c, "去整理 ›", "去整理").Tooltip("每条该放到哪张导图：在「等着归位」里一条条放").Clicked() {
			a.later(all)
		}
	})
}

// waitingMore is the way to those the page has no room for.
func (a *app) waitingMore(c *ui.Context, more int) {
	if more <= 0 {
		return
	}
	ui.Box(c).Padding(6, 4, 0).Children(func() {
		if textLink(c, fmt.Sprintf("还有 %d 条 ›", more), "还有更多").Clicked() {
			a.later(func() { a.tasks.view, a.tasks.onMap = tasksWaiting, "" })
		}
	})
}

// waitingBand is what waits for a place, under today's tasks: cut from
// them by a line and a breath of shade the page's width, each thing one
// line (its words, where it would go, when), a few of them; a click opens
// one, and the rest are a click away, on the left column's list. The
// cards with all that can be done are that list's: here they crowded the
// tasks. In a wide window it stands beside the tasks (waitingRail).
func (a *app) waitingBand(c *ui.Context, shown []taskItem) {
	t := c.Theme()
	items := a.waitingOnToday(shown)
	if cl := a.lib.Find(a.keptLast); len(items) == 0 && (cl == nil || cl.ToPlace) {
		return
	}
	ui.Column(c.Key("waiting-band")).FillWidth().Padding(14, 0, 28).BorderWidth(1, 0, 0, 0).BorderColor(hairline(t)).
		Background(t.Text.Alpha(0.018)).Children(func() {
		pageColumn(c, "band-column", false).MaxWidth(todayWidth + 56).Gap(2).Children(func() {
			a.waitingHead(c, len(items))
			a.keptLastRow(c)
			for i, w := range items {
				if i == bandCount {
					break
				}
				a.waitingLine(c, w, false)
			}
			a.waitingMore(c, len(items)-bandCount)
		})
	})
}

// railCount is how many waiting things the rail shows.
const railCount = 12

// waitingRail is the band in a wide window: a column of its own at the
// tasks' right, the page's height, so that the room a large screen has is
// used and the tasks are not pushed down. Each thing is a line or two,
// where it would go under its words.
func (a *app) waitingRail(c *ui.Context, shown []taskItem) {
	t := c.Theme()
	items := a.waitingOnToday(shown)
	ui.Column(c.Key("waiting-rail")).Grow(2).Basis(0).MinWidth(340).FillHeight().BorderWidth(0, 0, 0, 1).BorderColor(hairline(t)).
		Background(t.Text.Alpha(0.018)).Label("等着归位").Children(func() {
		ui.Scroll(c.Key("rail-scroll")).Grow(1).Basis(0).MinHeight(0).Children(func() {
			ui.Column(c).Padding(18, 20, 32).Gap(2).FillWidth().MaxWidth(620).Children(func() {
				a.waitingHead(c, len(items))
				a.keptLastRow(c)
				if len(items) == 0 {
					emptyNote(c, "收件箱里的任务，和你标了「待归位」的资料，会出现在这里。")
				}
				for i, w := range items {
					if i == railCount {
						break
					}
					a.waitingLine(c, w, true)
				}
				a.waitingMore(c, len(items)-railCount)
			})
		})
	})
}

// waitingLine is one waiting thing as a line of the band; in the rail,
// which is narrower, where it would go is under its words.
func (a *app) waitingLine(c *ui.Context, w waitingItem, rail bool) {
	t := c.Theme()
	key, text, when := "", "", ""
	var place taskPlace
	has := false
	if w.task != nil {
		key, text, when = "t:"+w.task.node.ID, w.task.node.Title, "任务"
		place, has = a.tasks.places[w.task.node.ID]
		if !has {
			place, has = a.nearPlace(key, &library.Clip{Title: w.task.node.Title, Text: w.task.node.Note})
		}
	} else {
		cl := w.clip
		key, text, when = "c:"+cl.ID, cl.Label(), relTime(a.now(), cl.Created)
		if isMemo(cl) {
			if first, _, _ := strings.Cut(strings.TrimSpace(library.Mask(cl.Text)), "\n"); first != "" {
				text = first
			}
		}
		place, has = a.clipPlaces[cl.ID]
		if !has {
			place, has = a.nearPlace(key, cl)
		}
	}
	text, _ = titleAndSite(strings.TrimSpace(refToken.ReplaceAllString(text, "")))
	row := ui.Row(c.Key("line:"+key)).MinHeight(36).Padding(6, 8).Gap(10).Radius(radiusRow).AlignItems(ui.Center).FillWidth().
		Cursor(ui.CursorPointer).Label("记下的 " + text).Background(t.Text.Alpha(0)).Transition(rowMotion)
	if row.Hovered() {
		row.Background(t.Text.Alpha(0.05))
	}
	if row.Clicked() {
		a.later(func() { a.openWaiting(w) })
	}
	row.ContextMenu(func(m *ui.Menu) { a.waitingMenu(m, w, has) })
	row.Children(func() {
		if w.task != nil {
			a.taskCircle(c, *w.task, 15)
		} else {
			ui.Box(c).Size(15, 15).Center().Shrink(0).PassThrough().Children(func() { mapDot(c, clipColor(w.clip.Kind), 7) })
		}
		// Where it would go, short enough for its room: the whole of it
		// is said under the pointer.
		where := shortPlace(place)
		if r := []rune(where); len(r) > 18 {
			where = string(r[:17]) + "…"
		}
		if rail {
			ui.Column(c).Gap(5).Grow(1).Shrink(1).Basis(0).MinWidth(0).PassThrough().Children(func() {
				ui.Text(c, text).FontSize(textBody).LineHeight(1.45).MaxLines(2).Wrap()
				if has {
					softTag(c, "→ "+where).Tooltip(place.label).AlignSelf(ui.Start)
				}
			})
		} else {
			ui.Text(c, text).FontSize(textBody).SingleLine().Grow(1).Shrink(1).MinWidth(0).PassThrough()
			if has {
				softTag(c, "→ "+where).Tooltip(place.label)
			}
		}
		ui.Text(c, when).FontSize(textCaption).TextColor(faint(t)).NoWrap().Shrink(0).PassThrough()
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
			if first, _, _ := strings.Cut(strings.TrimSpace(library.Mask(cl.Text)), "\n"); first != "" {
				text = first // else its title or its note, as its label has it
			}
		} else {
			ic = kindIcon(cl.Kind)
		}
		place, has = a.clipPlaces[cl.ID]
	}
	// With none of the AI's, where its words alone would put it: each card
	// says a place, and one key puts it there.
	near := false
	if !has {
		if w.task != nil {
			place, has = a.nearPlace("t:"+w.task.node.ID, &library.Clip{Title: w.task.node.Title, Text: w.task.node.Note})
		} else {
			place, has = a.nearPlace("c:"+w.clip.ID, w.clip)
		}
		near = has
	}
	_ = ic
	full := text // with what it refers to, which shows under its words
	text = strings.TrimSpace(refToken.ReplaceAllString(text, ""))
	// A card each, as a note is: when over the words, what can be done
	// with it under the pointer, where the AI would put it under the words.
	card := wallCard(ui.Column(c.Key("waiting:"+key)).Padding(14, 14, 18, 20).Gap(6).Radius(radiusCard).FillWidth().Label("记下的 "+text), t).
		Transition(rowMotion).Cursor(ui.CursorPointer)
	card.ContextMenu(func(m *ui.Menu) { a.waitingMenu(m, w, has) })
	if card.Clicked() {
		a.later(func() { a.openWaiting(w) })
	}
	card.Children(func() {
		ui.Row(c).Height(26).Gap(8).AlignItems(ui.Center).FillWidth().Children(func() {
			if w.task != nil {
				a.taskCircle(c, *w.task, 16)
			}
			ui.Text(c, kind).FontSize(textSmall).TextColor(t.TextMuted)
			ui.Spacer(c)
			// One key for all that can be done, as the pointer's other key.
			more := ui.Box(c.Key("more")).Size(28, 28).Radius(radiusPill).Label("操作 " + text).Tooltip("转成任务、放到导图、留在收集…").Shrink(0).
				Cursor(ui.CursorPointer).Background(t.Text.Alpha(0)).Transition(easeLook)
			ink := t.TextMuted.Alpha(0.7)
			if more.Hovered() {
				more.Background(t.Text.Alpha(0.06))
				ink = t.Text
			}
			more.Draw(func(p *ui.Painter, r ui.Rect) { drawIcon(p, r, 15, iconMore, ink) })
			more.Menu(func(m *ui.Menu) { a.waitingMenu(m, w, has) })
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
		a.noteText(c, words, 15, 4)
		if has {
			ui.Row(c.Key("sugg")).Gap(8).AlignItems(ui.Center).FillWidth().Padding(8, 0, 0).
				Transition(ui.ElementTransition{Duration: motionFast, Ease: easeDetent, Enter: &ui.Motion{Y: -6}}).Children(func() {
				softTag(c, "→ "+shortPlace(place)).Tooltip(place.label)
				ui.Spacer(c)
				if textKey(c, "换一个", "换一个 "+text, "自己选放到哪").Clicked() {
					a.later(func() { a.placeWaiting(w) })
				}
				go_ := ui.Box(c.Key("take")).Height(28).Padding(0, 12).Radius(radiusPill).Center().Cursor(ui.CursorPointer).
					Label("放过去 " + text).Transition(easeLook)
				jewel(go_, t, true)
				if go_.Clicked() {
					a.later(func() {
						if near { // taken as the AI's would be
							if w.task != nil {
								if a.tasks.places == nil {
									a.tasks.places = map[string]taskPlace{}
								}
								a.tasks.places[w.task.node.ID] = place
							} else {
								if a.clipPlaces == nil {
									a.clipPlaces = map[string]taskPlace{}
								}
								a.clipPlaces[w.clip.ID] = place
							}
							delete(a.nearPlaces, key)
						}
						if w.task != nil {
							a.takePlace(*w.task)
						} else {
							a.takeClipPlace(w.clip)
						}
					})
				}
				press(go_, 0.96)
				go_.Children(func() {
					ui.Text(c, "放过去").FontSize(textSmall).FontWeight(700).TextColor(t.AccentText).PassThrough()
				})
			})
		}
	})
}

// waitingMenu is what can be done with something written down: under
// the key at its card's corner, and under the pointer's other key. A
// thought is made a task, put on a map, left in 收集; a task given its
// day, kept as a thought.
func (a *app) waitingMenu(m *ui.Menu, w waitingItem, has bool) {
	if it := w.task; it != nil {
		if m.Item("看详情").Chosen() {
			a.later(func() { a.openWaiting(w) })
		}
		m.Separator()
		a.taskMenu(m, *it)
		return
	}
	place := "放到导图…"
	if has {
		place = "放到别处…"
	}
	cl := w.clip
	if m.Item("打开").Chosen() {
		a.later(func() { a.openWaiting(w) })
	}
	m.Separator()
	if m.Item("转成任务").Chosen() {
		a.later(func() { a.clipToTask(cl) })
	}
	if m.Item(place).Chosen() {
		a.later(func() { a.placeWaiting(w) })
	}
	if m.Item("不用归位了，留在收集").Chosen() {
		a.later(func() {
			cl.ToPlace = false
			a.saveClip(cl)
			a.keptLast = cl.ID
		})
	}
	m.Separator()
	if m.Item("复制内容").Chosen() {
		a.plat.CopyText(clipCopyText(cl))
		a.tell("已复制")
	}
	star := "加星标"
	if cl.Starred {
		star = "取消星标"
	}
	if m.Item(star).Chosen() {
		cl.Starred = !cl.Starred
		a.saveClip(cl)
	}
	m.Separator()
	if m.Item("删除").Chosen() {
		a.later(func() {
			a.openWaiting(w)
			a.libConfirm = true
		})
	}
}

// openWaiting opens something written down: a task's details beside the
// list, a thought in 收集.
func (a *app) openWaiting(w waitingItem) {
	if w.task != nil {
		a.tasks.sel, a.tasks.draftOf = w.task.key(), ""
		return
	}
	a.openList(kindAll, "")
	a.libSel = w.clip.ID
}

// unkeep has a clip left in 收集 wait for a place again.
func (a *app) unkeep(cl *library.Clip) {
	cl.ToPlace = true
	a.saveClip(cl)
	if a.keptLast == cl.ID {
		a.keptLast = ""
	}
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
	b := ui.Box(c.Key("textkey:"+label)).Height(28).Padding(0, 12).Radius(radiusPill).Center().Cursor(ui.CursorPointer).Label(label).Tooltip(tip).
		Shrink(0).Background(t.Text.Alpha(0)).Transition(easeLook)
	if b.Hovered() {
		b.Background(t.Text.Alpha(0.06))
	}
	b.Children(func() {
		ui.Text(c, text).FontSize(textSmall).TextColor(t.Text.MixOklab(t.TextMuted, 0.5)).NoWrap().PassThrough()
	})
	return press(b, 0.96)
}

// sparkKey is a key of the AI's that stands by a heading: the spark and
// its words in the accent, on no block until pointed at.
func sparkKey(c *ui.Context, text, tip string) ui.Element {
	t := c.Theme()
	b := ui.Row(c.Key("spark:"+text)).Height(28).Padding(0, 8).Gap(5).Radius(radiusPill).AlignItems(ui.Center).Cursor(ui.CursorPointer).
		Label(text).Tooltip(tip).Shrink(0).Background(t.Accent.Alpha(0)).Transition(easeLook)
	if b.Hovered() {
		b.Background(t.Accent.Alpha(0.09))
	}
	b.Children(func() {
		glyph(c, iconSparkle, 13, t.Accent)
		ui.Text(c, text).FontSize(textSmall).FontWeight(700).TextColor(t.Accent).NoWrap().PassThrough()
	})
	return press(b, 0.96)
}

// nearPlace is where something written down would go by its words alone,
// with no AI: the topic of the maps most about it, not of the inbox. It
// is looked for once, and again when a map changes.
func (a *app) nearPlace(key string, cl *library.Clip) (taskPlace, bool) {
	if a.nearPlaces == nil || a.nearEdits != a.edits {
		a.nearPlaces, a.nearEdits = map[string]*taskPlace{}, a.edits
	}
	p, ok := a.nearPlaces[key]
	if !ok {
		for _, f := range a.placesFor(cl, 4) {
			if f.path != a.inboxPath() && sharesWord(clipQuery(cl), f.title) {
				p = &taskPlace{path: f.path, sheet: f.sheet, node: f.id, label: f.title}
				break
			}
		}
		a.nearPlaces[key] = p
	}
	if p == nil {
		return taskPlace{}, false
	}
	return *p, true
}

// sharesWord tells whether a text and a place's name have two letters
// running in common, letters of words and not digits or marks: a place
// that only shares a number with what waits is not its place.
func sharesWord(text, name string) bool {
	word := func(r rune) bool { return unicode.IsLetter(r) }
	rs := []rune(strings.ToLower(text))
	name = strings.ToLower(name)
	for i := 0; i+1 < len(rs); i++ {
		if word(rs[i]) && word(rs[i+1]) && strings.Contains(name, string(rs[i:i+2])) {
			return true
		}
	}
	return false
}

// markToPlace has a clip wait for a place on a map, and says so with the
// way back.
func (a *app) markToPlace(cl *library.Clip) {
	cl.ToPlace = true
	a.saveClip(cl)
	id := cl.ID
	a.tellUndo("已放进「等着归位」", func() {
		if cl := a.lib.Find(id); cl != nil {
			cl.ToPlace = false
			a.saveClip(cl)
		}
	})
}
