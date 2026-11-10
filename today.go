package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/layout"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// 今天 is the page the app opens on: what spans the parts of the app, a
// little of each. A line to write a note, or a task; the tasks of today;
// what was noted last; the maps opened last; the automations that want a
// look. The parts themselves are a click away.

// homeView is 今天.
func (a *app) homeView(c *ui.Context) {
	t := c.Theme()
	now := a.now()
	ui.Column(c.Key("home")).Grow(1).Label("今天").Children(func() {
		ui.Row(c.Key("home-head")).Height(toolbarHeight).Padding(0, 14, 0, 20).Gap(10).AlignItems(ui.Center).Shrink(0).
			DragWindow().Children(func() {
			ui.Spacer(c)
			if iconTextButton(c, iconSparkle, "问 AI", "AI 助手", "问问导图和资料里的内容", a.aiOpen).Clicked() {
				a.aiOpen = !a.aiOpen
			}
		})
		ui.Scroll(c.Key("home-scroll")).Grow(1).Padding(4, 48, 32).Children(func() {
			ui.Column(c).MaxWidth(820).Margin(0, ui.Auto).FillWidth().Gap(28).Children(func() {
				ui.Column(c).Gap(4).Children(func() {
					ui.Text(c, greeting(now)).FontSize(24).FontWeight(600)
					ui.Text(c, dateLabel(now)).FontSize(textBody).TextColor(t.TextMuted)
				})
				a.homeLine(c)
				ui.Row(c).Gap(40).AlignItems(ui.Start).FillWidth().Children(func() {
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() { a.homeTasks(c, now) })
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() { a.homeNoted(c) })
				})
				a.homeMaps(c, now)
				a.homeAutomations(c)
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

// homeLine is the line to write in: a note of 灵感, or, when it says a
// day (明天 写周报), a task.
func (a *app) homeLine(c *ui.Context) {
	t := c.Theme()
	box := ui.Row(c.Key("home-line")).Height(42).Padding(0, 14).Gap(10).Radius(10).AlignItems(ui.Center).FillWidth().
		Background(t.Text.Alpha(0.045))
	box.Children(func() {
		glyph(c, iconPencil, 15, t.Accent)
		in := ui.TextInputBase(c.Key("home-line-input"), &a.homeDraft).Placeholder("记一条灵感，或「明天 …」加一个任务").
			FontSize(textBody).Grow(1).Label("记一条灵感或任务")
		if in.Focused() {
			box.Border(1, t.Accent.Alpha(0.5))
		}
		in.HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyEnter {
				a.later(a.saveHomeLine)
				return true
			}
			return false
		})
		// Where what is written goes, said as it is written.
		if text := strings.TrimSpace(a.homeDraft); text != "" {
			if _, due, _ := parseTaskLine(text, a.now()); taskLike(text, a.now()) {
				label := "任务"
				if due != nil {
					label += " · " + dayLabel(*due, a.now())
				}
				pill(c, label, t.Background, t.Accent)
			} else {
				pill(c, "灵感", t.Background, t.TextMuted)
			}
		}
		ui.Text(c, "↩").FontSize(textCaption).TextColor(t.TextMuted)
	})
}

// saveHomeLine keeps what was typed in 今天's line.
func (a *app) saveHomeLine() {
	text := strings.TrimSpace(a.homeDraft)
	if text == "" {
		return
	}
	if taskLike(text, a.now()) {
		a.addTask(text, nil)
	} else {
		if _, err := a.addMemo(text, "今天"); err != nil {
			a.tell("没有记下：" + err.Error())
			return
		}
		a.tell("已记到灵感")
	}
	a.homeDraft = ""
}

// homeTaskCount is how many tasks of today 今天 shows.
const homeTaskCount = 3

// homeTasks are the tasks of today and those overdue, the first few.
func (a *app) homeTasks(c *ui.Context, now time.Time) {
	t := c.Theme()
	day := today(now)
	var due []taskItem
	for _, it := range a.taskItems() {
		if d := it.date(); d != nil && !it.done() && d.Before(day.AddDate(0, 0, 1)) && it.node.Task.Status != model.StatusIdea {
			due = append(due, it)
		}
	}
	count := ""
	if len(due) > 0 {
		count = fmt.Sprint(len(due))
	}
	if pageHead(c, "今天要做", count, "打开任务") {
		a.later(a.openTasks)
	}
	if len(due) == 0 {
		ui.Text(c, "今天没有要做的事。").FontSize(textBody).TextColor(t.TextMuted).Margin(12, 2)
		return
	}
	for i, it := range due {
		if i == homeTaskCount {
			break
		}
		row := ui.Row(c.Key("home-task:"+it.key())).Height(38).Padding(0, 2).Gap(12).AlignItems(ui.Center).FillWidth().
			Cursor(ui.CursorPointer).Label("任务 " + it.node.Title)
		if row.Clicked() {
			a.later(func() {
				a.openTasks()
				a.showTask(it)
			})
		}
		row.Children(func() {
			a.taskCircle(c, it, 16)
			ui.Text(c, it.node.Title).FontSize(textBody).Grow(1).Shrink(1).MinWidth(0).SingleLine().PassThrough()
			if d := it.date(); d.Before(day) {
				n := int(day.Sub(today(*d)).Hours()/24 + 0.5)
				ui.Text(c, fmt.Sprintf("过期 %d 天", n)).FontSize(textSmall).TextColor(ui.Hex("#B4574A")).Shrink(0).PassThrough()
			} else {
				ui.Text(c, it.mapName).FontSize(textSmall).TextColor(t.TextMuted).Shrink(0).SingleLine().PassThrough()
			}
		})
	}
	if more := len(due) - homeTaskCount; more > 0 {
		l := ui.Box(c.Key("home-more-tasks")).Padding(6, 30, 0).Cursor(ui.CursorPointer).Label("还有任务")
		if l.Clicked() {
			a.later(a.openTasks)
		}
		l.Children(func() {
			ui.Text(c, fmt.Sprintf("还有 %d 件", more)).FontSize(textSmall).TextColor(t.TextMuted).PassThrough()
		})
	}
}

// homeNoted are the notes and the clips kept last.
func (a *app) homeNoted(c *ui.Context) {
	t := c.Theme()
	if pageHead(c, "最近记下的", "", "全部灵感") {
		a.later(a.openMemos)
	}
	if len(a.lib.Clips) == 0 {
		ui.Text(c, "用上面那一行，或在任何地方按 "+keyLabel(a.captureKey())+" 记一条。").FontSize(textBody).TextColor(t.TextMuted).Margin(12, 2).Wrap()
		return
	}
	for i, clip := range a.lib.Clips { // the last kept first
		if i == 3 {
			break
		}
		row := ui.Row(c.Key("home-noted:"+clip.ID)).Padding(10, 2, 0).Gap(10).AlignItems(ui.Start).FillWidth().
			Cursor(ui.CursorPointer).Label("最近 " + clip.Label())
		if row.Clicked() {
			a.later(func() {
				if isMemo(clip) {
					a.openMemos()
				} else {
					a.showClip(clip.ID)
				}
			})
		}
		text := clip.Label()
		if isMemo(clip) {
			text, _, _ = strings.Cut(strings.TrimSpace(clip.Text), "\n")
		}
		ic := kindIcon(clip.Kind)
		if isMemo(clip) {
			ic = iconPencil
		}
		row.Children(func() {
			ui.Box(c).Margin(2, 0, 0).Shrink(0).PassThrough().Children(func() { glyph(c, ic, 14, t.TextMuted) })
			ui.Text(c, text).FontSize(textBody).LineHeight(1.55).Grow(1).Shrink(1).MinWidth(0).MaxLines(2).PassThrough()
		})
	}
}

// homeMaps are the maps opened last, each a small picture of it.
func (a *app) homeMaps(c *ui.Context, now time.Time) {
	files := a.recentFiles()
	if len(files) == 0 {
		return
	}
	ui.Column(c).Gap(12).FillWidth().Children(func() {
		pageHead(c, "最近打开的导图", "", "")
		a.homeMapCards(c, now, files)
	})
}

// homeMapCards are the maps' cards, four in a row.
func (a *app) homeMapCards(c *ui.Context, now time.Time, files []model.FileInfo) {
	t := c.Theme()
	ui.Row(c.Key("home-maps")).Gap(14).FillWidth().Children(func() {
		for i := range 4 {
			if i >= len(files) {
				ui.Box(c).Grow(1).Basis(0)
				continue
			}
			fi := files[i]
			card := ui.Column(c.Key("home-map:" + fi.Path)).Grow(1).Basis(0).MinWidth(0).Gap(6).Cursor(ui.CursorPointer).
				Label("导图 " + mapName(fi.Path))
			hover := card.Hovered()
			if card.Clicked() {
				a.later(func() { _ = a.open(fi.Path) })
			}
			card.Children(func() {
				thumb := ui.Box(c).Height(92).FillWidth().Radius(10).Background(t.Text.Alpha(0.035)).PassThrough()
				if hover {
					thumb.Background(t.Text.Alpha(0.06))
				}
				f := a.file
				if fi.Path != a.path {
					f = a.indexed(fi.Path)
				}
				if f != nil && len(f.Sheets) > 0 {
					d := f.Sheets[0]
					thumb.Draw(func(p *ui.Painter, r ui.Rect) { drawThumb(p, r, d, t) })
				}
				ui.Text(c, mapName(fi.Path)).FontSize(textBody).FontWeight(600).SingleLine().Margin(2, 0, 0).PassThrough()
				ui.Text(c, relTime(now, modTime(fi.Path))).FontSize(textSmall).TextColor(t.TextMuted).SingleLine().PassThrough()
			})
		}
	})
}

// drawThumb draws a small picture of a map in r: its topics as blocks in
// their colors, joined by lines, the central one solid.
func drawThumb(p *ui.Painter, r ui.Rect, d *model.Document, t *ui.Theme) {
	if d.Root == nil {
		return
	}
	lk := lookOf(d.Style)
	depth, branch := map[string]int{}, map[string]int{}
	var walk func(n *model.Node, dep, br int)
	walk = func(n *model.Node, dep, br int) {
		depth[n.ID], branch[n.ID] = dep, br
		for i, ch := range n.Children {
			if dep == 0 {
				br = i
			}
			walk(ch, dep+1, br)
		}
	}
	walk(d.Root, 0, 0)
	measure := func(n *model.Node, dep int) (float32, float32) {
		w := float32(min(len([]rune(n.Title)), 8))*9 + 16
		if dep == 0 {
			return w + 8, 22
		}
		return w, 16
	}
	o := layoutOptions(d.Structure)
	o.Margin, o.GapX, o.GapY = 0, 24, 6
	lr := layout.Compute(d, measure, o)
	if len(lr.Boxes) == 0 {
		return
	}
	pad := float32(12)
	k := min((r.W-2*pad)/max(lr.Width, 1), (r.H-2*pad)/max(lr.Height, 1), 1)
	ox := r.X + (r.W-lr.Width*k)/2
	oy := r.Y + (r.H-lr.Height*k)/2
	at := func(b layout.Rect) ui.Rect { return ui.Rect{X: ox + b.X*k, Y: oy + b.Y*k, W: b.W * k, H: b.H * k} }
	line := t.Text.Alpha(0.18)
	for _, e := range lr.Edges {
		from, to := at(lr.Boxes[e.From]), at(lr.Boxes[e.To])
		x0, x1 := from.X+from.W, to.X
		if e.Left {
			x0, x1 = from.X, to.X+to.W
		}
		y0, y1 := from.Y+from.H/2, to.Y+to.H/2
		mid := (x0 + x1) / 2
		path := new(ui.Path).MoveTo(x0, y0).CubeTo(mid, y0, mid, y1, x1, y1)
		p.StrokePath(path, 1, line)
	}
	for id, b := range lr.Boxes {
		box := at(b)
		switch dep := depth[id]; {
		case dep == 0:
			p.Fill(box, t.Text.MixOklab(t.Background, 0.1), box.H*0.3)
		case dep == 1:
			p.Fill(box, lk.branch(branch[id]).Alpha(0.3), box.H*0.3)
		default:
			p.Fill(box, t.Text.Alpha(0.1), box.H*0.3)
		}
	}
}
