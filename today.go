package main

import (
	"fmt"
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
	t := c.Theme()
	now := a.now()
	ui.Column(c.Key("home")).Grow(1).Label("工作台").Children(func() {
		ui.Row(c.Key("home-head")).Height(toolbarHeight-14).Padding(0, 14, 0, 20).Gap(10).AlignItems(ui.Center).Shrink(0).
			DragWindow().Children(func() {
			ui.Spacer(c)
			if iconTextButton(c, iconSparkle, "问 AI", "AI 助手", "问问导图和资料里的内容", a.aiOpen).Clicked() {
				a.aiOpen = !a.aiOpen
			}
		})
		ui.Scroll(c.Key("home-scroll")).Grow(1).Padding(0, 44, 32).Children(func() {
			ui.Column(c).MaxWidth(1080).Margin(0, ui.Auto).FillWidth().Gap(22).Children(func() {
				due, waiting := len(a.dueToday(now)), len(a.waiting())
				ui.Column(c).Gap(3).Children(func() {
					ui.Text(c, greeting(now)).FontSize(26).FontWeight(700)
					sub := dateLabel(now)
					if due > 0 {
						sub += fmt.Sprintf(" · 今天 %d 件事", due)
					}
					if waiting > 0 {
						sub += fmt.Sprintf(" · %d 条等你归位", waiting)
					}
					ui.Text(c, sub).FontSize(textBody).TextColor(t.TextMuted)
				})
				a.homeLine(c)
				ui.Row(c).Gap(28).AlignItems(ui.Start).FillWidth().Children(func() {
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(24).Children(func() {
						a.homeTasks(c, now)
						a.homeMaps(c, now)
					})
					ui.Column(c).Grow(1.15).Basis(0).MinWidth(0).Gap(24).Children(func() {
						a.homeWaiting(c)
						a.homeAutomations(c)
					})
				})
			})
		})
	})
}

// greeting is said by the hour.
func greeting(now time.Time) string {
	switch h := now.Hour(); {
	case h < 5:
		return "夜深了"
	case h < 11:
		return "早上好"
	case h < 13:
		return "中午好"
	case h < 18:
		return "下午好"
	}
	return "晚上好"
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
	box := ui.Row(c.Key("home-line")).Height(52).Padding(0, 8, 0, 18).Gap(12).Radius(16).AlignItems(ui.Center).FillWidth().
		Background(cardColor(t)).Border(1.5, t.Accent).Shadow(0, 6, 20, 0, t.Accent.Alpha(0.12))
	box.Children(func() {
		glyph(c, iconPlus, 16, t.Accent)
		in := ui.TextInputBase(c.Key("home-line-input"), &a.homeDraft).Placeholder("记点什么：一个想法，或「周五 给客户发报价单」这样的任务").
			FontSize(15).Grow(1).Label("记一条灵感或任务")
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
		if accentButton(c, "记下 ↩", "记下（回车）").Clicked() {
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

// deskHead is the heading of a part of the desk: its title, a count in a
// pill, a word on it, and a link at the end; it tells whether the link
// was clicked.
func deskHead(c *ui.Context, title, count, hint, link string) bool {
	t := c.Theme()
	clicked := false
	ui.Row(c.Key("head:" + title)).Gap(8).AlignItems(ui.Center).FillWidth().MinHeight(24).Children(func() {
		ui.Text(c, title).FontSize(textTitle).FontWeight(700)
		if count != "" {
			ui.Text(c, count).FontSize(textSmall).TextColor(t.TextMuted)
		}
		if hint != "" {
			ui.Text(c, hint).FontSize(textSmall).TextColor(t.TextMuted).Shrink(1).MinWidth(0).SingleLine()
		}
		ui.Spacer(c)
		if link != "" {
			l := ui.Box(c.Key("link:" + title)).Cursor(ui.CursorPointer).Label(link + " " + title)
			if l.Clicked() {
				clicked = true
			}
			l.Children(func() { ui.Text(c, link).FontSize(textSmall).FontWeight(600).TextColor(t.Accent).PassThrough() })
		}
	})
	return clicked
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
	ui.Column(c.Key("home-tasks")).Gap(6).FillWidth().Children(func() {
		count := ""
		if len(due)+done > 0 {
			count = fmt.Sprintf("%d / %d", done, len(due)+done)
		}
		if deskHead(c, "今天要做", count, "", "全部任务") {
			a.later(a.openTasks)
		}
		if len(due)+done > 0 {
			k := float32(done) / float32(len(due)+done)
			ui.Box(c).Height(6).FillWidth().Radius(3).Background(t.Text.Alpha(0.07)).Margin(2, 0, 4).Children(func() {
				ui.Box(c).Height(6).WidthPercent(k * 100).Radius(3).Background(t.Success)
			})
		}
		if len(due) == 0 {
			msg := "今天没有要做的事。"
			if done > 0 {
				msg = "今天的都做完了。"
			}
			ui.Text(c, msg).FontSize(textBody).TextColor(t.TextMuted).Margin(6, 2)
			return
		}
		day := today(now)
		for i, it := range due {
			if i == homeTaskCount {
				break
			}
			row := ui.Row(c.Key("home-task:"+it.key())).Height(38).Padding(0, 2).Gap(10).AlignItems(ui.Center).FillWidth().
				Cursor(ui.CursorPointer).Label("任务 " + it.node.Title)
			if row.Clicked() {
				a.later(func() {
					a.openTasks()
					a.showTask(it)
				})
			}
			row.Children(func() {
				a.taskCircle(c, it, 18)
				ui.Text(c, it.node.Title).FontSize(14).Grow(1).Shrink(1).MinWidth(0).SingleLine().PassThrough()
				if d := it.date(); d.Before(day) {
					n := int(day.Sub(today(*d)).Hours()/24 + 0.5)
					ui.Text(c, fmt.Sprintf("过期 %d 天", n)).FontSize(textSmall).FontWeight(600).TextColor(t.Danger).Shrink(0).PassThrough()
				}
				mapDot(c, a.mapColor(it.path), 8).Tooltip(it.mapName)
			})
		}
		if more := len(due) - homeTaskCount; more > 0 {
			l := ui.Box(c.Key("home-more-tasks")).Padding(4, 30, 0).Cursor(ui.CursorPointer).Label("还有任务")
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
	ui.Column(c.Key("home-maps")).Gap(10).FillWidth().Children(func() {
		if deskHead(c, "我的导图", "", "", "＋ 新建") {
			a.later(a.newMap)
		}
		if len(files) == 0 {
			ui.Text(c, "还没有导图。").FontSize(textBody).TextColor(t.TextMuted)
			return
		}
		ui.Grid(c).Columns(2).Gap(12).FillWidth().Children(func() {
			for _, fi := range files {
				col := a.mapColor(fi.Path)
				bg, ink := tinted(t, col)
				card := ui.Column(c.Key("home-map:" + fi.Path)).Padding(14).Gap(7).Radius(14).Background(bg).Cursor(ui.CursorPointer).
					Label("导图 " + mapName(fi.Path))
				if card.Hovered() {
					card.Background(col.Alpha(0.2))
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
					ui.Text(c, mapName(fi.Path)).FontSize(textTitle).FontWeight(700).SingleLine().PassThrough()
					sub := "没有任务"
					if total > 0 {
						sub = fmt.Sprintf("%d 个任务", total)
					}
					ui.Text(c, sub+" · "+relTime(now, modTime(fi.Path))).FontSize(textSmall).TextColor(ink).SingleLine().PassThrough()
					ui.Box(c).Height(5).FillWidth().Radius(3).Background(t.Text.Alpha(0.08)).PassThrough().Children(func() {
						if total > 0 {
							ui.Box(c).Height(5).WidthPercent(float32(done) / float32(total) * 100).Radius(3).Background(col)
						}
					})
				})
			}
		})
	})
}

// waitingItem is something written down that has no place yet: a task of
// the inbox, or a note or a clip no topic links to.
type waitingItem struct {
	task *taskItem
	clip *library.Clip
}

// waiting are the things that wait for a place: the open tasks of the
// inbox, and the notes and clips of the last two weeks that no topic
// links to and that were not said to stay in the collection. Not all
// that is kept belongs on a map: 留在收集 takes one off the list for good.
func (a *app) waiting() []waitingItem {
	var out []waitingItem
	for _, it := range a.taskItems() {
		if it.path == a.inboxPath() && !it.done() {
			out = append(out, waitingItem{task: &it})
		}
	}
	since := a.now().AddDate(0, 0, -14)
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

// homeWaiting is 待归位: what waits for a place, each a card with where
// the AI would put it, to take, to choose another, or to leave where it
// is.
func (a *app) homeWaiting(c *ui.Context) {
	t := c.Theme()
	items := a.waiting()
	suggested := len(a.tasks.places) + len(a.clipPlaces)
	ui.Column(c.Key("home-waiting")).Gap(12).FillWidth().Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).FillWidth().MinHeight(24).Children(func() {
			ui.Text(c, "待归位").FontSize(textTitle).FontWeight(700)
			if len(items) > 0 {
				bg, ink := tinted(t, ui.Hex("#8B6FE0"))
				pill(c, fmt.Sprint(len(items)), bg, ink)
			}
			ui.Text(c, "记下的东西，可以放进导图，也可以就留在收集里").FontSize(textSmall).TextColor(t.TextMuted).Shrink(1).MinWidth(0).SingleLine()
			ui.Spacer(c)
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
				if iconTextButton(c, iconSparkle, "AI 建议", "AI 建议去处", "让 AI 建议每条该放到哪张导图的哪个主题下，你确认后才放", false).Clicked() {
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
			ui.Text(c, "都归位了。新记下的东西会先出现在这里。").FontSize(textBody).TextColor(t.TextMuted).Margin(6, 2)
			return
		}
		for i, w := range items {
			if i == homeWaitingCount {
				ui.Text(c, fmt.Sprintf("还有 %d 条，在任务的收件箱和收集里", len(items)-i)).FontSize(textSmall).TextColor(t.TextMuted).Margin(0, 4)
				break
			}
			a.waitingCard(c, w)
		}
	})
}

// waitingCard is one thing that waits: what it is, and under it where it
// may go.
func (a *app) waitingCard(c *ui.Context, w waitingItem) {
	t := c.Theme()
	key, text, kind := "", "", ""
	var place taskPlace
	has := false
	ic := iconPencil
	if w.task != nil {
		key, text, kind = "t:"+w.task.node.ID, w.task.node.Title, "任务"
		if d := w.task.date(); d != nil {
			kind += " · " + dayLabel(*d, a.now())
		}
		place, has = a.tasks.places[w.task.node.ID]
	} else {
		cl := w.clip
		key, text, kind = "c:"+cl.ID, cl.Label(), library.KindLabel(cl.Kind)
		if isMemo(cl) {
			text, _, _ = strings.Cut(strings.TrimSpace(cl.Text), "\n")
			kind = "灵感"
		} else {
			ic = kindIcon(cl.Kind)
		}
		place, has = a.clipPlaces[cl.ID]
	}
	ui.Column(c.Key("waiting:"+key)).Padding(11, 16).Gap(10).Radius(14).Background(cardColor(t)).FillWidth().
		Shadow(0, 1, 2, 0, ui.RGBA(31, 34, 51, 0.06)).Children(func() {
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
				if softButton(c, label, "自己选放到哪张导图的哪个主题下").Height(28).Label(label + " " + text).Clicked() {
					it := *w.task
					a.later(func() { a.moveTaskTo(it) })
				}
			} else if addButton(c, "留在收集", "不放进导图，就留在收集里；以后不再问").Label("留在收集 " + text).Clicked() {
				cl := w.clip
				a.later(func() {
					cl.Kept = true
					delete(a.clipPlaces, cl.ID)
					a.saveClip(cl)
				})
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
			ui.Text(c, text).FontSize(14).LineHeight(1.5).Grow(1).Shrink(1).MinWidth(0).MaxLines(2)
			ui.Text(c, kind).FontSize(textSmall).TextColor(t.TextMuted).Shrink(0)
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
