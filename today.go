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

// homeView is 工作台.
func (a *app) homeView(c *ui.Context) {
	now := a.now()
	ui.Column(c.Key("home")).Grow(1).Label("工作台").Children(func() {
		pageColumn(c, "home-column", true).Children(func() {
			due, inbox := len(a.dueToday(now)), 0
			for _, w := range a.waiting() {
				if w.task != nil {
					inbox++
				}
			}
			sub := dateLabel(now)
			if due > 0 {
				sub += fmt.Sprintf(" · 今天 %d 件事", due)
			}
			if inbox > 0 {
				sub += fmt.Sprintf(" · 收件箱 %d 个任务", inbox)
			}
			pageHeader(c, "home-head", "工作台", sub, func() {
				if iconTextButton(c, iconSparkle, "问 AI", "AI 助手", "问问导图和资料里的内容", a.aiOpen).Clicked() {
					a.aiOpen = !a.aiOpen
				}
			})
			a.homeLine(c)
			ui.Scroll(c.Key("home-scroll")).Grow(1).Basis(0).Margin(14, 0, 0).Children(func() {
				ui.Row(c).Gap(14).AlignItems(ui.Start).FillWidth().Padding(0, 0, 32).Children(func() {
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(14).Children(func() {
						a.homeTasks(c, now)
						a.homeMaps(c, now)
					})
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(14).Children(func() {
						a.homeWaiting(c)
					})
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

// homeLine is the line to write in, the first thing on the desk: what is
// written says what it is, a task when it says a day, else a note, and
// the line says which as it is written.
func (a *app) homeLine(c *ui.Context) {
	t := c.Theme()
	box := captureBox(c, "home-line")
	box.Children(func() {
		glyph(c, iconPlus, 15, t.Accent)
		in := ui.TextInputBase(c.Key("home-line-input"), &a.homeDraft).Placeholder("记点什么：一个想法，或「周五 给客户发报价单」这样的任务").
			FontSize(textBody + 1).Grow(1).Label("记一条灵感或任务")
		captureFocus(c, box, in)
		in.HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyEnter {
				a.later(a.saveHomeLine)
				return true
			}
			return false
		})
		if text := strings.TrimSpace(a.homeDraft); text != "" {
			if _, due, _ := parseTaskLine(text, a.now()); taskLike(text, a.now()) {
				label := "任务"
				if due != nil {
					label += " · " + dayLabel(*due, a.now())
				}
				bg, ink := tinted(t, t.Success)
				pill(c, label, bg, ink)
			} else {
				bg, ink := tinted(t, ui.Hex("#E8A13A"))
				pill(c, "灵感", bg, ink)
			}
		}
		if strings.TrimSpace(a.homeDraft) != "" && accentButton(c, "记下 ↩", "记下（回车）").Clicked() {
			a.later(a.saveHomeLine)
		}
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

// homeTasks are the tasks of today, first on the desk: how far the day
// is, then each with the dot of its map.
func (a *app) homeTasks(c *ui.Context, now time.Time) {
	t := c.Theme()
	due := a.dueToday(now)
	done := 0
	for _, it := range a.taskItems() {
		if it.done() && it.node.Task.DoneAt != nil && sameDay(*it.node.Task.DoneAt, now) {
			done++
		}
	}
	tray(c, "home-tasks").Children(func() {
		count := ""
		if len(due)+done > 0 {
			count = fmt.Sprintf("%d / %d", done, len(due)+done)
		}
		trayHead(c, "head:今天要做", "今天要做", count, func() {
			if textLink(c, "全部任务", "全部任务 今天要做").Clicked() {
				a.later(a.openTasks)
			}
		})
		if len(due) == 0 {
			msg := "今天没有要做的事。"
			if done > 0 {
				msg = "今天的都做完了。"
			}
			emptyNote(c, msg)
			return
		}
		day := today(now)
		listCard(c, "home-task-list").Children(func() {
			for i, it := range due {
				if i == homeTaskCount {
					break
				}
				row := listCardRow(c, "home-task:"+it.key()).Cursor(ui.CursorPointer).Label("任务 " + it.node.Title)
				if row.Clicked() {
					a.later(func() {
						a.openTasks()
						a.showTask(it)
					})
				}
				row.Children(func() {
					a.taskCircle(c, it, 17)
					ui.Text(c, it.node.Title).FontSize(textBody).Grow(1).Shrink(1).MinWidth(0).SingleLine().PassThrough()
					if d := it.date(); d.Before(day) {
						n := int(day.Sub(today(*d)).Hours()/24 + 0.5)
						ui.Text(c, fmt.Sprintf("过期 %d 天", n)).FontSize(textSmall).TextColor(t.Danger).Shrink(0).PassThrough()
					}
					mapDot(c, a.mapColor(it.path), 7).Tooltip(it.mapName)
					ui.Text(c, it.mapName).FontSize(textSmall).TextColor(t.TextMuted).Shrink(0).MaxWidth(120).SingleLine().PassThrough()
				})
			}
		})
		if more := len(due) - homeTaskCount; more > 0 {
			l := ui.Box(c.Key("home-more-tasks")).Padding(8, 10, 4).Cursor(ui.CursorPointer).Label("还有任务")
			if l.Clicked() {
				a.later(a.openTasks)
			}
			l.Children(func() {
				ui.Text(c, fmt.Sprintf("还有 %d 件", more)).FontSize(textSmall).TextColor(t.TextMuted).PassThrough()
			})
		}
	})
}

// homeMaps are the maps opened last, each a card in its color: its name,
// its tasks and how far they are.
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
		if len(files) == 0 {
			emptyNote(c, "还没有导图。")
			return
		}
		ui.Grid(c).Columns(2).Gap(6).FillWidth().Children(func() {
			for _, fi := range files {
				col := a.mapColor(fi.Path)
				card := raise(ui.Column(c.Key("home-map:"+fi.Path)).Padding(12, 14).Gap(8).Radius(10).Cursor(ui.CursorPointer).
					Label("导图 "+mapName(fi.Path)), t)
				if card.Hovered() {
					card.Shadow(0, 4, 12, 0, ui.RGBA(40, 32, 16, 0.1))
				}
				if card.Clicked() {
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
				card.Children(func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).PassThrough().Children(func() {
						mapDot(c, col, 9)
						ui.Text(c, mapName(fi.Path)).FontSize(textBody).FontWeight(600).SingleLine().Grow(1).Shrink(1).MinWidth(0)
					})
					sub := "没有任务"
					if total > 0 {
						sub = fmt.Sprintf("%d / %d 个任务", done, total)
					}
					ui.Text(c, sub+" · "+relTime(now, modTime(fi.Path))).FontSize(textSmall).TextColor(t.TextMuted).SingleLine().PassThrough()
					ui.Box(c).Height(3).FillWidth().Radius(2).Background(t.Text.Alpha(0.07)).PassThrough().Children(func() {
						if total > 0 {
							ui.Box(c).Height(3).WidthPercent(float32(done) / float32(total) * 100).Radius(2).Background(col)
						}
					})
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
	card := ui.Column(c.Key("waiting:"+key)).Padding(6, 6, 6, 10).Gap(8).Radius(7).FillWidth().MinHeight(40).Center().Label("记下的 " + text)
	// A row says what it is and when; what can be done with it shows under
	// the pointer, so that a list of them reads as a list.
	hover := card.Hovered()
	if hover {
		card.Background(t.Text.Alpha(0.04))
	}
	card.Children(func() {
		actions := func() {
			if has {
				b := ui.Box(c.Key("take")).Height(28).Padding(0, 12).Radius(8).Center().Background(t.Text).Cursor(ui.CursorPointer).Label("放过去 " + text)
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
				b := softButton(c, label, "自己选放到哪张导图的哪个主题下").Height(26).Label(label + " " + text)
				if !hover && !has {
					b.Width(0).Padding(0).Opacity(0)
				}
				if b.Clicked() {
					it := *w.task
					a.later(func() { a.moveTaskTo(it) })
				}
			} else {
				b := softButton(c, "放到导图…", "放到某张导图的某个主题下；不放也行，它在收集里").Height(26).Label("放到导图 " + text)
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
			ui.Row(c).Gap(8).AlignItems(ui.Center).FillWidth().Padding(0, 0, 0, 26).Children(func() {
				if has {
					glyph(c, iconSparkle, 13, ui.Hex("#8B6FE0"))
					bg, ink := tinted(t, a.mapColor(place.path))
					ui.Row(c).Padding(4, 10).Gap(6).Radius(8).AlignItems(ui.Center).Background(bg).Shrink(1).MinWidth(0).Tooltip(place.label).Children(func() {
						mapDot(c, a.mapColor(place.path), 8)
						ui.Text(c, shortPlace(place)).FontSize(textSmall).FontWeight(600).TextColor(ink).SingleLine()
					})
				}
				ui.Spacer(c)
				actions()
			})
		}
	})
}
