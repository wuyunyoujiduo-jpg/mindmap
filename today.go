package main

import (
	"fmt"
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

// homeView is 工作台: the day heads it, large and light, with how far it
// is; under it the line to write in, today's tasks beside what was
// written lately, and the maps.
func (a *app) homeView(c *ui.Context) {
	now := a.now()
	t := c.Theme()
	ui.Column(c.Key("home")).Grow(1).Label("工作台").Children(func() {
		pageColumn(c, "home-column", true).Children(func() {
			due, done := len(a.dueToday(now)), len(a.doneToday(now))
			ui.Row(c.Key("home-head")).Height(92).Padding(0, 0, 16).Gap(14).AlignItems(ui.End).Shrink(0).FillWidth().DragWindow().Children(func() {
				ui.Row(c).Gap(12).AlignItems(ui.End).Children(func() {
					ui.Text(c, weekdays[now.Weekday()]).FontSize(28).LineHeight(1.2).FontWeight(600).NoWrap()
					ui.Text(c, fmt.Sprintf("%d月%d日", now.Month(), now.Day())).FontSize(28).LineHeight(1.2).FontWeight(300).NoWrap()
				})
				ui.Row(c.Key("sum")).Gap(5).AlignItems(ui.Center).Padding(0, 0, 6).Children(func() {
					word := func(s string) { ui.Text(c, s).FontSize(textBody).TextColor(t.TextMuted).NoWrap() }
					num := func(n int) { ui.Text(c, fmt.Sprint(n)).FontSize(textBody).FontWeight(600).NoWrap() }
					word("今天")
					num(due)
					word("件事")
					if done > 0 {
						word("· 做完")
						num(done)
					}
				})
				ui.Spacer(c)
				if iconTextButton(c, iconSparkle, "问 AI", "AI 助手", "问问导图和资料里的内容", a.aiOpen).Margin(0, 0, 2).Clicked() {
					a.aiOpen = !a.aiOpen
				}
			})
			a.homeLine(c)
			ui.Scroll(c.Key("home-scroll")).Grow(1).Basis(0).Margin(14, 0, 0).Children(func() {
				ui.Column(c).Gap(14).FillWidth().Padding(0, 0, 28).Children(func() {
					ui.Row(c).Gap(14).AlignItems(ui.Start).FillWidth().Children(func() {
						ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() { a.homeTasks(c, now) })
						ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() { a.homeWaiting(c) })
					})
					a.homeMaps(c, now)
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

// homeLine is the line to write in, the first thing on the desk: a white
// card on a tray. What is written says what it is, a task when it says a
// day, else a note, and the line says which as it is written; while it
// is written in the tray comes out from under the card and says how.
func (a *app) homeLine(c *ui.Context) {
	t := c.Theme()
	focused := false
	ui.Column(c.Key("home-capture")).Radius(16).Background(t.Surface).FillWidth().Shrink(0).Children(func() {
		box := raise(ui.Row(c.Key("home-line")).Height(54).Padding(0, 10, 0, 18).Gap(12).Radius(16).AlignItems(ui.Center).FillWidth(), t)
		box.Children(func() {
			mark, turn := t.TextMuted.Alpha(0.7), float32(0)
			if a.homeFocus {
				mark, turn = t.Accent, 90
			}
			glyph(c, iconPlus, 16, mark).AnimatePose(ui.Pose{Rotate: turn}, ui.Animation{Duration: 300 * time.Millisecond, Ease: easeDetent})
			in := ui.TextInputBase(c.Key("home-line-input"), &a.homeDraft).Placeholder("记点什么：一个想法，或「周五 给客户发报价单」这样的任务").
				FontSize(textTitle).Grow(1).Label("记一条灵感或任务")
			focused = in.Focused()
			captureFocus(c, box, in)
			in.HandleInput(func(ev ui.InputEvent) bool {
				if ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyEnter {
					a.later(a.saveHomeLine)
					return true
				}
				return false
			})
			text := strings.TrimSpace(a.homeDraft)
			if text != "" {
				if _, due, _ := parseTaskLine(text, a.now()); taskLike(text, a.now()) {
					label := "任务"
					if due != nil {
						label += " · " + dayLabel(*due, a.now())
					}
					readPill(c, label, t.Accent.Alpha(0.14), t.Accent)
				} else {
					readPill(c, "灵感", t.Surface, t.Text.MixOklab(t.TextMuted, 0.7))
				}
				for _, tag := range memoTags(text) {
					readPill(c, "#"+tag, t.Surface, t.Text.MixOklab(t.TextMuted, 0.7))
				}
				if inkButton(c, "记下 ↩", true).Tooltip("记下（回车）").Transition(ui.ElementTransition{Enter: &ui.Motion{X: 6}}).Clicked() {
					a.later(a.saveHomeLine)
				}
			}
		})
		// The tray under the card: what a line becomes.
		// It opens with the first letter, not with the focus: a click elsewhere
		// must find things where they were.
		open := b2f(strings.TrimSpace(a.homeDraft) != "")
		hint := ui.Row(c.Key("home-hint")).Padding(0, 18).Gap(14).AlignItems(ui.Center).FillWidth().Clip()
		hint.Height(32 * hint.AnimateTo("open", open, ui.Animation{Duration: 260 * time.Millisecond, Ease: easeDetent}))
		hint.Children(func() {
			say := func(key, text string) {
				ui.Row(c.Key(text)).Gap(5).AlignItems(ui.Center).Shrink(0).Children(func() {
					if key != "" {
						ui.Box(c).Padding(1, 5).Radius(5).Background(cardColor(t)).Border(1, cardEdge(t)).Children(func() {
							ui.Text(c, key).FontSize(textCaption).TextColor(t.Text.MixOklab(t.TextMuted, 0.7))
						})
					}
					ui.Text(c, text).FontSize(textSmall).TextColor(t.TextMuted).NoWrap()
				})
			}
			say("↩", "记下")
			say("", "以「今天 / 明天 / 周五」开头，会成为那天的任务")
			say("#", "开头的词是标签")
		})
	})
	if focused != a.homeFocus {
		a.homeFocus = focused
		c.Invalidate()
	}
}

// readPill is what the line reads of what is written in it: a pill that
// pops in.
func readPill(c *ui.Context, text string, bg, ink ui.Color) {
	ui.Box(c.Key("read:"+text)).Height(22).Padding(0, 9).Radius(999).Center().Background(bg).Shrink(0).
		Transition(ui.ElementTransition{Duration: 260 * time.Millisecond, Ease: easeDetent, Enter: &ui.Motion{Scale: 0.8}}).Children(func() {
		ui.Text(c, text).FontSize(textSmall).FontWeight(500).TextColor(ink).NoWrap()
	})
}

// saveHomeLine keeps what was typed in the desk's line.
func (a *app) saveHomeLine() {
	text := strings.TrimSpace(a.homeDraft)
	if text == "" {
		return
	}
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

// mapChip names a map on a wash of its color, its dot before its name.
func (a *app) mapChip(c *ui.Context, path, text string) ui.Element {
	t := c.Theme()
	col := a.mapColor(path)
	return ui.Row(c).Height(20).Padding(0, 8, 0, 7).Gap(5).Radius(999).AlignItems(ui.Center).Background(col.Alpha(0.13)).Shrink(1).MinWidth(0).PassThrough().Children(func() {
		mapDot(c, col, 6)
		ui.Text(c, text).FontSize(11.5).TextColor(t.Text.MixOklab(t.TextMuted, 0.7)).SingleLine()
	})
}

// homeTasks are the tasks of today, first on the desk: how far the day
// is, as a count and a line that fills; then each with its map, and under
// them those done today, which go there as they are ticked.
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
		if all > 0 {
			ui.Box(c.Key("meter")).Height(3).Radius(2).Margin(0, 10, 8).Background(t.Text.Alpha(0.07)).Children(func() {
				ui.Box(c.Key("fill")).Height(3).Radius(2).WidthPercent(float32(len(done)) / float32(all) * 100).Background(t.Text).
					Transition(ui.ElementTransition{Duration: 500 * time.Millisecond, Ease: easeDetent, Width: true})
			})
		}
		if all == 0 {
			emptyNote(c, "今天没有要做的事。")
			return
		}
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
				a.taskCircle(c, it, 18)
				title := ui.Text(c, it.node.Title).FontSize(textBody).Grow(1).Shrink(1).MinWidth(0).SingleLine().PassThrough()
				if it.done() {
					title.TextColor(t.TextMuted.Alpha(0.75)).Strikethrough()
				} else if d := it.date(); d != nil && d.Before(day) {
					n := int(day.Sub(today(*d)).Hours()/24 + 0.5)
					ui.Text(c, fmt.Sprintf("过期 %d 天", n)).FontSize(textSmall).TextColor(t.Danger).Shrink(0).PassThrough()
				}
				chip := a.mapChip(c, it.path, it.mapName).MaxWidth(130)
				if it.done() {
					chip.Opacity(0.45)
				}
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
				ui.Text(c.Key("all-done"), "今天的都做完了。").FontSize(textSmall).TextColor(t.TextMuted).Margin(10, 10, 8)
			}
			if more := len(due) - homeTaskCount; more > 0 {
				l := ui.Box(c.Key("home-more-tasks")).Padding(6, 10).Cursor(ui.CursorPointer).Label("还有任务")
				if l.Clicked() {
					a.later(a.openTasks)
				}
				l.Children(func() {
					ui.Text(c, fmt.Sprintf("还有 %d 件", more)).FontSize(textSmall).TextColor(t.TextMuted).PassThrough()
				})
			}
			if len(done) > 0 {
				ui.Box(c.Key("done-rule")).Height(1).Margin(4, 10).Background(hairline(t))
				ui.Text(c.Key("done-head"), "做完的").FontSize(11.5).TextColor(t.TextMuted).Margin(6, 10, 2)
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

// homeMaps are the maps opened last, each a card: the map drawn small in
// its color, its name, its tasks and how far they are; and a card to make
// a new one.
func (a *app) homeMaps(c *ui.Context, now time.Time) {
	t := c.Theme()
	var files []model.FileInfo
	for _, fi := range a.recentFiles() {
		if fi.Path != a.inboxPath() && len(files) < 3 {
			files = append(files, fi)
		}
	}
	tray(c, "home-maps").Children(func() {
		trayHead(c, "head:我的导图", "我的导图", "最近打开的", func() {
			if textLink(c, "全部导图", "全部导图 我的导图").Clicked() {
				a.later(func() {
					a.libOpen = false
					a.cfg.ShowTree = true
				})
			}
		})
		ui.Grid(c).Columns(4).Gap(6).FillWidth().Children(func() {
			for _, fi := range files {
				col := a.mapColor(fi.Path)
				card := raise(ui.Column(c.Key("home-map:"+fi.Path)).Padding(12, 14).Gap(8).Radius(10).Cursor(ui.CursorPointer).
					Label("导图 "+mapName(fi.Path)), t)
				// Under the pointer it lifts a little, its shadow longer.
				lift := ui.Pose{}
				if card.Hovered() {
					card.Shadow(0, 12, 22, -10, ui.RGBA(40, 32, 16, 0.16))
					lift.Y = -2
				}
				if card.Pressed() {
					lift = ui.Pose{Scale: 0.985}
				}
				card.AnimatePose(lift, ui.Animation{Duration: 300 * time.Millisecond, Ease: easeDetent})
				if card.Clicked() {
					a.later(func() { _ = a.open(fi.Path) })
				}
				total, done := 0, 0
				f := a.file
				if fi.Path != a.path {
					f = a.indexed(fi.Path)
				}
				var shape []int // the main topics, and how many each has below
				if f != nil {
					for _, tn := range f.Tasks() {
						total++
						if tn.Node.Task.Status == model.StatusDone {
							done++
						}
					}
					if len(f.Sheets) > 0 && f.Sheets[0].Root != nil {
						for i, n := range f.Sheets[0].Root.Children {
							if i == 4 {
								break
							}
							shape = append(shape, min(len(n.Children), 3))
						}
					}
				}
				card.Children(func() {
					ui.Box(c).Height(44).FillWidth().PassThrough().Draw(func(p *ui.Painter, r ui.Rect) { drawMapGlyph(p, r, shape, col) })
					ui.Row(c).Gap(7).AlignItems(ui.Center).PassThrough().Children(func() {
						mapDot(c, col, 8)
						ui.Text(c, mapName(fi.Path)).FontSize(textBody).FontWeight(600).SingleLine().Grow(1).Shrink(1).MinWidth(0)
					})
					sub := "没有任务"
					if total > 0 {
						sub = fmt.Sprintf("%d / %d 个任务", done, total)
					}
					ui.Text(c, sub+" · "+relTime(now, modTime(fi.Path))).FontSize(textSmall).TextColor(t.TextMuted).SingleLine().PassThrough()
					ui.Box(c).Height(3).FillWidth().Radius(2).Background(t.Text.Alpha(0.07)).PassThrough().Children(func() {
						if total > 0 {
							ui.Box(c.Key("done")).Height(3).WidthPercent(float32(done) / float32(total) * 100).Radius(2).Background(col).
								Transition(ui.ElementTransition{Duration: 500 * time.Millisecond, Ease: easeDetent, Width: true})
						}
					})
				})
			}
			add := ui.Box(c.Key("home-map-new")).MinHeight(118).Radius(10).Center().Border(1, t.Text.Alpha(0.14)).Cursor(ui.CursorPointer).
				Label("新建导图").Background(t.Text.Alpha(0)).Transition(easeLook)
			ink := t.TextMuted
			if add.Hovered() {
				add.Background(cardColor(t))
				ink = t.Text
			}
			if add.Clicked() {
				a.later(a.newMap)
			}
			add.Children(func() { ui.Text(c, "＋ 新建导图").FontSize(textBody).TextColor(ink).PassThrough() })
		})
	})
}

// drawMapGlyph draws a map small, at the left of r: its center, its main
// topics on both sides, and those below them faintly, in the map's color.
func drawMapGlyph(p *ui.Painter, r ui.Rect, shape []int, col ui.Color) {
	const w, h = 120, 44
	k := r.H / h
	at := func(x, y float32) (float32, float32) { return r.X + x*k, r.Y + y*k }
	dot := func(x, y, rad float32, c ui.Color) {
		cx, cy := at(x, y)
		p.Fill(ui.Rect{X: cx - rad*k, Y: cy - rad*k, W: 2 * rad * k, H: 2 * rad * k}, c, rad*k)
	}
	link := func(x0, y0, x1, y1, bend float32, c ui.Color) {
		ax, ay := at(x0, y0)
		bx, by := at(x1, y1)
		c1x, _ := at(x0+bend, y0)
		c2x, _ := at(x1-bend, y1)
		p.StrokePath(new(ui.Path).MoveTo(ax, ay).CubeTo(c1x, ay, c2x, by, bx, by), 1.4, c)
	}
	line, faint := col.Alpha(0.55), col.Alpha(0.3)
	per := (len(shape) + 1) / 2
	for i, kids := range shape {
		dir, side := float32(1), i/2
		if i%2 == 1 {
			dir = -1
		}
		n := per
		if dir < 0 {
			n = len(shape) / 2
		}
		y := 22 + (float32(side)-float32(n-1)/2)*20
		x := 60 + 32*dir
		link(60, 22, x, y, 14*dir, line)
		for j := range kids {
			yy, xx := y+(float32(j)-float32(kids-1)/2)*8, x+26*dir
			link(x, y, xx, yy, 10*dir, faint)
			dot(xx, yy, 1.6, faint)
		}
		dot(x, y, 2.6, col)
	}
	dot(60, 22, 4, col)
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
		listCard(c, "home-lately").Children(func() {
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
	card := ui.Column(c.Key("waiting:"+key)).Padding(6, 6, 6, 10).Gap(8).Radius(7).FillWidth().MinHeight(40).Center().Label("记下的 " + text).
		Background(t.Text.Alpha(0)).Transition(rowMotion)
	// A row says what it is and when; what can be done with it shows under
	// the pointer, so that a list of them reads as a list.
	hover := card.Hovered()
	if hover {
		card.Background(t.Text.Alpha(0.045))
	}
	card.Children(func() {
		actions := func() {
			if has {
				b := ui.Box(c.Key("take")).Height(24).Padding(0, 10).Radius(7).Center().Background(t.Text).Cursor(ui.CursorPointer).Label("放过去 " + text)
				if b.Clicked() {
					a.later(func() {
						if w.task != nil {
							a.takePlace(*w.task)
						} else {
							a.takeClipPlace(w.clip)
						}
					})
				}
				b.Children(func() {
					ui.Text(c, "放过去").FontSize(textSmall).FontWeight(600).TextColor(t.Background).PassThrough()
				})
			}
			if w.task != nil {
				label := "选一个主题…"
				if has {
					label = "换一个"
				}
				b := softButton(c, label, "自己选放到哪张导图的哪个主题下").Height(24).Radius(7).Label(label + " " + text)
				if !hover && !has {
					b.Width(0).Padding(0).Opacity(0)
				}
				if b.Clicked() {
					it := *w.task
					a.later(func() { a.moveTaskTo(it) })
				}
			} else {
				b := softButton(c, "放到导图…", "放到某张导图的某个主题下；不放也行，它在收集里").Height(24).Radius(7).Label("放到导图 " + text)
				if !hover && !has {
					b.Width(0).Padding(0).Opacity(0)
				}
				if b.Clicked() {
					ref := clipScheme + w.clip.ID
					a.later(func() {
						a.attachRef = ref
						a.openPalette(paletteAttach)
					})
				}
			}
		}
		ui.Row(c).Gap(10).AlignItems(ui.Center).FillWidth().Children(func() {
			ui.Box(c).Shrink(0).Children(func() {
				if w.task != nil {
					a.taskCircle(c, *w.task, 16)
				} else {
					glyph(c, ic, 16, t.TextMuted)
				}
			})
			ui.Text(c, text).FontSize(textBody).LineHeight(1.5).Grow(1).Shrink(1).MinWidth(0).MaxLines(2)
			if !hover || has {
				ui.Text(c, kind).FontSize(textSmall).TextColor(t.TextMuted).Shrink(0).Margin(0, 4, 0, 0)
			}
			if !has {
				actions()
			}
		})
		if has {
			ui.Row(c.Key("sugg")).Gap(8).AlignItems(ui.Center).FillWidth().Padding(0, 0, 2, 26).
				Transition(ui.ElementTransition{Duration: 300 * time.Millisecond, Ease: easeDetent, Enter: &ui.Motion{Y: -6}}).Children(func() {
				glyph(c, iconSparkle, 13, t.Accent)
				a.mapChip(c, place.path, shortPlace(place)).Tooltip(place.label)
				ui.Spacer(c)
				actions()
			})
		}
	})
}
