package main

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// 镜头, the lenses: one map colored another way, nothing moving. 分支 is
// the canvas as it is; 新旧, 来源 and 进度 color every topic in one hue
// by a few steps (when it last changed, how many clips of 收集 it links
// to, how its tasks stand), draw the lines in the hairline's gray and
// leave the middle in the ink. What they tell is in the map already: no
// AI is asked, and no file read.
//
// The way in is a quiet row at the canvas's lower left (lensBar): its
// name, a thin line, the choices as small words. It is a row of words on
// the canvas, with no ground and no shadow: as a white bar with the
// accent's tint it read as a second row of tabs under the map's, and with
// its name looking as a choice, as one more of them (the user's, of the
// drafts).

// The lenses, as kept with a map's view (fileView.Lens).
const (
	lensBranch = ""     // 分支: by the branches' colors, as the canvas is
	lensAge    = "age"  // 新旧: by when the topic last changed
	lensSrc    = "src"  // 来源: by how many clips it links to
	lensTask   = "task" // 进度: by how its tasks stand
)

var lenses = []struct{ id, name, tip string }{
	{lensBranch, "分支", "每个分支一种颜色：平常的样子"},
	{lensAge, "新旧", "按每个主题上次改动的时间着色"},
	{lensSrc, "来源", "按每个主题链接了多少条收集里的资料着色"},
	{lensTask, "进度", "按任务做完、在做还是逾期着色"},
}

func lensNamed(id string) bool {
	return slices.ContainsFunc(lenses, func(l struct{ id, name, tip string }) bool { return l.id == id })
}

func lensName(id string) string {
	for _, l := range lenses {
		if l.id == id {
			return l.name
		}
	}
	return ""
}

// The steps of each lens: 0 is the plain one (white, a hairline, muted
// words), the others a tint of the lens's one hue, or, of 进度, of the
// three colors that mean done, doing and late everywhere in the app.
const (
	agePlain  = iota // longer
	ageSeason        // within three months
	ageMonth         // this month
	ageWeek          // this week
)

const (
	srcNone = iota
	srcSome // one or two
	srcMany // three or more
)

const (
	taskNone = iota // not a task, and none below
	taskDone
	taskDoing
	taskLate
)

// Days of the steps of 新旧, fixed (the user's choice): a week, a month,
// three months.
const (
	ageWeekDays   = 7
	ageMonthDays  = 30
	ageSeasonDays = 90
)

// lensTint is how much of its hue a step is, over the leaf: the draft's.
// A lens is one hue by steps, and no second color for a second thing: a
// topic is read against its neighbors at a glance, which two hues at once
// would not let it be (进度's three are the app's own for done, doing and
// late, each one step).
func lensTint(lens string, step int) (hue func(*ui.Theme) ui.Color, part float32) {
	accent := func(t *ui.Theme) ui.Color { return t.Accent }
	switch lens {
	case lensAge:
		return accent, []float32{0, 0.09, 0.20, 0.34}[step]
	case lensSrc:
		return teal, []float32{0, 0.13, 0.30}[step]
	case lensTask:
		switch step {
		case taskDone:
			return func(t *ui.Theme) ui.Color { return t.Success }, 0.22
		case taskDoing:
			return accent, 0.14
		case taskLate:
			return func(t *ui.Theme) ui.Color { return t.Danger }, 0.16
		}
	}
	return accent, 0
}

// lensFill is a step's fill; plain tells the step that has none, which is
// white with a hairline, its words muted.
func lensFill(t *ui.Theme, lens string, step int) (fill ui.Color, plain bool) {
	hue, part := lensTint(lens, step)
	if part == 0 {
		return t.Background, true
	}
	if t.Dark {
		part *= 1.4 // on the dark leaf a tint reads as less: the steps were hard to tell apart
	}
	return t.Background.MixOklab(hue(t), part), false
}

// lensLegend is what the steps of a lens mean, the strongest first.
func lensLegend(lens string) []struct {
	step int
	name string
} {
	type l = struct {
		step int
		name string
	}
	switch lens {
	case lensAge:
		return []l{{ageWeek, "本周"}, {ageMonth, "本月"}, {ageSeason, "三个月内"}, {agePlain, "更久"}}
	case lensSrc:
		return []l{{srcMany, "3 条以上"}, {srcSome, "1–2 条"}, {srcNone, "没有来源"}}
	case lensTask:
		return []l{{taskDone, "做完"}, {taskDoing, "在做"}, {taskLate, "有逾期"}, {taskNone, "不是任务"}}
	}
	return nil
}

// lensTopic is what a lens tells of one topic.
type lensTopic struct {
	step int
	// n is the days since it changed (新旧) or the clips it links to
	// (来源); done, total and late are its tasks, its own and those below
	// (进度).
	n                 int
	done, total, late int
	own               *model.Task // its own task, of 进度
}

// lensLine is a topic of the right column's list, to go to.
type lensLine struct {
	id, title, cap string
	late           bool
}

// lensData is a lens over a canvas, worked out once a change of the map
// and kept: every topic's step, and what the right column says of the
// whole. A frame looks a topic up in it and no more.
type lensData struct {
	lens  string
	doc   *model.Document
	edits int
	day   time.Time

	topics  map[string]lensTopic
	figures [2]figure
	part    string     // the list's name
	lines   []lensLine // at most lensLines of them
	more    int        // those not listed
	none    string     // said in the list's place when it is empty
}

// lensLines is how many topics the right column lists.
const lensLines = 40

// lensNow is the lens the canvas shows through: none while a picture is
// made of it, which is of the map as it is.
func (a *app) lensNow() string {
	if a.doc == nil || a.exportCam != nil {
		return lensBranch
	}
	return a.lens
}

// lensLooks are the looks of the lens shown, nil by the branches: worked
// out when the map, the canvas, the lens or the day is another.
func (a *app) lensLooks() *lensData {
	lens := a.lensNow()
	if lens == lensBranch {
		return nil
	}
	day := today(a.now())
	if d := a.lensMemo; d != nil && d.lens == lens && d.doc == a.doc && d.edits == a.edits && d.day.Equal(day) {
		return d
	}
	a.lensRuns++
	a.lensMemo = lensOf(a.file, a.doc, lens, a.now())
	a.lensMemo.edits = a.edits
	return a.lensMemo
}

// lensOf works a lens out over a canvas.
func lensOf(f *model.File, doc *model.Document, lens string, now time.Time) *lensData {
	d := &lensData{lens: lens, doc: doc, day: today(now), topics: map[string]lensTopic{}}
	root := doc.Root
	switch lens {
	case lensAge:
		var week, old int
		var olds []lensLine
		var days []int
		doc.Walk(func(n, _ *model.Node, _ int) bool {
			if n == root {
				return true
			}
			k := daysSince(f.ChangedAt(n), now)
			step := agePlain
			switch {
			case k <= ageWeekDays:
				step = ageWeek
				week++
			case k <= ageMonthDays:
				step = ageMonth
			case k <= ageSeasonDays:
				step = ageSeason
			default:
				old++
				olds = append(olds, lensLine{id: n.ID, title: n.Title, cap: agoShort(k)})
				days = append(days, k)
			}
			d.topics[n.ID] = lensTopic{step: step, n: k}
			return true
		})
		// The longest untouched first, those of a day in the map's order.
		order := make([]int, len(olds))
		for i := range order {
			order[i] = i
		}
		slices.SortStableFunc(order, func(x, y int) int { return days[y] - days[x] })
		if len(order) > lensLines {
			order, d.more = order[:lensLines], len(order)-lensLines
		}
		for _, i := range order {
			d.lines = append(d.lines, olds[i])
		}
		d.figures = [2]figure{{week, "本周动过"}, {old, "三个月没动"}}
		d.part, d.none = "最久没动的", "每个主题三个月内都动过"
	case lensSrc:
		var with, without int
		doc.Walk(func(n, _ *model.Node, _ int) bool {
			if n == root {
				return true
			}
			k := sourcesOf(n)
			step := srcNone
			switch {
			case k >= 3:
				step = srcMany
			case k >= 1:
				step = srcSome
			}
			if k > 0 {
				with++
			} else {
				without++
				if len(d.lines) < lensLines {
					d.lines = append(d.lines, lensLine{id: n.ID, title: n.Title})
				}
			}
			d.topics[n.ID] = lensTopic{step: step, n: k}
			return true
		})
		d.more = without - len(d.lines)
		d.figures = [2]figure{{with, "有依据"}, {without, "没有来源"}}
		d.part, d.none = "没有来源的", "每个主题都有来源"
	case lensTask:
		var open, lateN int
		type due struct {
			line lensLine
			at   *time.Time
		}
		var todo []due
		var walk func(n *model.Node) lensTopic
		walk = func(n *model.Node) lensTopic {
			t := lensTopic{own: n.Task}
			if k := n.Task; k != nil {
				t.total++
				switch {
				case k.Status == model.StatusDone:
					t.done++
				case k.Overdue(now):
					t.late++
				}
				if k.Status != model.StatusDone {
					open++
					l := lensLine{id: n.ID, title: n.Title, cap: k.Status.Label()}
					if k.Due != nil {
						l.cap = k.Due.Format("1/2")
					}
					if k.Overdue(now) {
						lateN++
						l.late, l.cap = true, fmt.Sprintf("逾期 %d 天", max(daysSince(*k.Due, now), 1))
					}
					todo = append(todo, due{l, k.Due})
				}
			}
			for _, c := range n.Children {
				b := walk(c)
				t.done, t.total, t.late = t.done+b.done, t.total+b.total, t.late+b.late
			}
			if s := n.SummaryTopic(); s != nil {
				walk(s) // a topic of its own, not of what it sums up
			}
			switch {
			case t.total == 0:
				t.step = taskNone
			case t.late > 0:
				t.step = taskLate
			case t.done == t.total:
				t.step = taskDone
			default:
				t.step = taskDoing
			}
			if n != root {
				d.topics[n.ID] = t
			}
			return t
		}
		walk(root)
		for _, fl := range doc.Floating {
			walk(fl)
		}
		// Late first, the longest late at the head; then by the day due,
		// those with none last, in the map's order.
		slices.SortStableFunc(todo, func(x, y due) int {
			switch {
			case x.line.late != y.line.late:
				if x.line.late {
					return -1
				}
				return 1
			case x.at != nil && y.at != nil:
				return x.at.Compare(*y.at)
			case x.at != nil:
				return -1
			case y.at != nil:
				return 1
			}
			return 0
		})
		for i, t := range todo {
			if i == lensLines {
				d.more = len(todo) - lensLines
				break
			}
			d.lines = append(d.lines, t.line)
		}
		d.figures = [2]figure{{open, "件要做"}, {lateN, "件逾期"}}
		d.part, d.none = "还没做完的", "没有没做完的任务"
	}
	return d
}

// daysSince is how many days of the calendar ago t was, none for a time
// to come or not known.
func daysSince(t, now time.Time) int {
	if t.IsZero() {
		return 0
	}
	return max(int(math.Round(today(now).Sub(today(t.In(now.Location()))).Hours()/24)), 0)
}

// ago says how long since a topic changed: 今天改过, 12 天前改过, 3 个月前改过.
func ago(days int) string {
	switch {
	case days < 1:
		return "今天改过"
	case days <= ageMonthDays:
		return fmt.Sprintf("%d 天前改过", days)
	}
	return agoShort(days) + "前改过"
}

// agoShort is the same as a figure: 3 个月, 2 年.
func agoShort(days int) string {
	if days >= 730 {
		return fmt.Sprintf("%d 年", days/365)
	}
	return fmt.Sprintf("%d 个月", max(int(math.Round(float64(days)/30)), 1))
}

// sourcesOf counts the clips of 收集 a topic links to: by its links, and
// by the [[ links of its note; a clip named twice is one.
func sourcesOf(n *model.Node) int {
	var first string
	var ids []string // only for a topic of several
	add := func(id string) {
		switch {
		case id == "" || id == first || slices.Contains(ids, id):
		case first == "":
			first = id
		default:
			ids = append(ids, id)
		}
	}
	for _, l := range n.Links {
		if id, ok := strings.CutPrefix(l, clipScheme); ok {
			add(id)
		}
	}
	if strings.Contains(n.Note, "]("+clipScheme) || strings.Contains(n.Note, "](<"+clipScheme) {
		for _, m := range noteLink.FindAllStringSubmatch(n.Note, -1) {
			if id, ok := strings.CutPrefix(m[1], clipScheme); ok {
				add(id)
			}
		}
	}
	if first == "" {
		return 0
	}
	return 1 + len(ids)
}

// lensFact is what the lens shown tells of a topic, in a few words: under
// the pointer on the canvas, and a line of the topic's page. "" by the
// branches, and of the middle.
func (a *app) lensFact(id string) string {
	d := a.lensLooks()
	if d == nil || a.mode != viewMap {
		return ""
	}
	t, ok := d.topics[id]
	if !ok {
		return ""
	}
	switch d.lens {
	case lensAge:
		return ago(t.n)
	case lensSrc:
		if t.n == 0 {
			return "没有来源"
		}
		return fmt.Sprintf("%d 条来源", t.n)
	case lensTask:
		var s string
		switch {
		case t.total == 0:
			return "不是任务"
		case t.total == 1 && t.own != nil:
			s = "任务：" + t.own.Status.Label()
			if t.late > 0 {
				s += "，已逾期"
			}
			return s
		}
		s = fmt.Sprintf("任务 %d/%d 做完", t.done, t.total)
		if t.late > 0 {
			s += fmt.Sprintf("，有 %d 件逾期", t.late)
		}
		return s
	}
	return ""
}

// setLens looks at the map through a lens. With one of the three the
// middle is chosen, which is nothing chosen: the right column says what
// the lens shows.
func (a *app) setLens(id string) {
	if a.doc == nil || !lensNamed(id) {
		return
	}
	a.lens, a.lensRoot = id, false
	if id != lensBranch {
		a.commitEditIfAny()
		a.selected, a.multi = a.shownRoot().ID, nil
	}
}

// menuLens is a lens chosen in the menu 视图: on the map, which is shown
// for it.
func (a *app) menuLens(id string) {
	if a.doc == nil {
		return
	}
	a.toMap()
	a.showView(viewMap)
	a.setLens(id)
}

// lensOver tells whether the right column says the lens rather than a
// topic: with a lens on the canvas and nothing chosen, which is the
// middle chosen and not by hand.
func (a *app) lensOver() bool {
	return a.mode == viewMap && a.lensNow() != lensBranch && len(a.multi) == 0 && a.editing == "" &&
		!a.lensRoot && a.selected == a.shownRoot().ID
}

// lensGo chooses a topic of the lens's list and brings it into view,
// leaving the branch focused on when it is not of it.
func (a *app) lensGo(id string) {
	if a.shownDoc().Path(id) == nil {
		a.focusOn("")
	}
	a.selectNode(id)
}

// lensPanel is the right column with a lens on and nothing chosen: the
// lens's name, the map in two figures, and a short list to go to.
func (a *app) lensPanel(c *ui.Context) {
	t := c.Theme()
	d := a.lensLooks()
	if d == nil {
		return
	}
	ui.Column(c.Key("lens:" + d.lens)).Padding(16).Gap(14).Label("镜头 " + lensName(d.lens)).Children(func() {
		ui.Text(c, lensName(d.lens)).FontSize(textHeadline).FontWeight(700).LetterSpacing(-0.4)
		figures(c, d.figures[0], d.figures[1])
		ui.Column(c.Key("list")).FillWidth().Children(func() {
			ui.Text(c, d.part).FontSize(textSmall).FontWeight(700).Margin(0, 0, 4)
			if len(d.lines) == 0 {
				ui.Text(c, d.none).FontSize(textSmall).TextColor(t.TextMuted).Margin(6, 0)
				return
			}
			for _, l := range d.lines {
				row := ui.Row(c.Key(l.id)).Gap(8).Padding(9, 2).AlignItems(ui.Center).FillWidth().
					BorderWidth(0, 0, 1, 0).BorderColor(hairline(t)).Cursor(ui.CursorPointer).Label("到主题 " + l.title)
				col := t.Text
				if row.Hovered() {
					col = t.Accent
				}
				if row.Clicked() {
					id := l.id
					a.later(func() { a.lensGo(id) })
				}
				row.Children(func() {
					title := l.title
					if strings.TrimSpace(title) == "" {
						title = "无标题"
					}
					ui.Text(c, title).FontSize(textBody).TextColor(col).SingleLine().Grow(1).Shrink(1).MinWidth(0).PassThrough()
					if l.cap != "" {
						cap := t.TextMuted
						if l.late {
							cap = t.Danger
						}
						ui.Text(c, l.cap).FontSize(textCaption).TextColor(cap).FontFeatures("tnum").NoWrap().Shrink(0).PassThrough()
					}
				})
			}
			if d.more > 0 {
				ui.Text(c, fmt.Sprintf("还有 %d 个", d.more)).FontSize(textCaption).TextColor(t.TextMuted).Margin(8, 2, 0)
			}
		})
	})
}

// lensBar is the lens's row at the canvas's lower left, and, with a lens
// on, what its steps mean at the lower right. Neither is a card: words on
// the canvas.
//
// The canvas's own bar is at its foot on the right. The row stands beside
// it while both fit, and over it when they do not, by what the two
// measure (their boxes of the last frame), not by a width guessed; the
// legend is over the bar, and over the row when the row is there.
func (a *app) lensBar(c *ui.Context) {
	t := c.Theme()
	const edge, apart = 14, 8
	lens := a.lensNow()
	bar, row := a.canvasBar.Bounds(c), a.lensRow.Bounds(c)
	foot := float32(edge) // the row's, from the canvas's foot
	over := bar.W > 0 && row.W > 0 && a.vp.W > 0 && edge+row.W+2*apart+bar.W+edge > a.vp.W
	switch {
	case over:
		foot = edge + bar.H + apart
	case bar.H > row.H && row.H > 0:
		foot = edge + (bar.H-row.H)/2 // in the bar's middle
	}
	ui.Row(c.Key("lens")).Absolute().Left(edge).Bottom(foot).Gap(2).AlignItems(ui.Center).Bind(&a.lensRow).Children(func() {
		// Its name: a mark and two small letters, apart, and a thin line
		// after them. Not a choice, and not to look as one.
		ui.Row(c.Key("name")).Gap(5).AlignItems(ui.Center).Padding(0, 10, 0, 2).Label("镜头").Children(func() {
			glyph(c, iconLens, 13, t.TextMuted)
			ui.Text(c, "镜头").FontSize(11).FontWeight(700).LetterSpacing(0.6).TextColor(t.TextMuted).NoWrap()
		})
		ui.Box(c).Width(1).Height(14).Background(hairline(t)).Margin(0, 6, 0, 0).Shrink(0)
		for _, l := range lenses {
			on := l.id == lens
			// As wide as its word in bold, chosen or not: the words after
			// it stay where they are whichever is chosen.
			w, _ := a.measureText(c, 0, ui.Span{Text: l.name, Size: textCaption, Weight: 700})
			b := ui.Box(c.Key("lens:"+l.id)).Padding(3, 8).MinWidth(float32(math.Ceil(float64(w))) + 17).Radius(radiusSmall).Center().
				Cursor(ui.CursorPointer).Label("镜头：" + l.name).Tooltip(l.tip).Transition(easeLook)
			col := t.TextMuted
			switch {
			case on:
				b.Background(blockFill(t))
				col = t.Text
			case b.Hovered():
				col = ink2(t)
			}
			if b.Clicked() {
				id := l.id
				a.later(func() { a.setLens(id) })
			}
			b.Children(func() {
				word := ui.Text(c, l.name).FontSize(textCaption).TextColor(col).NoWrap().PassThrough()
				if on {
					word.FontWeight(700)
				}
			})
		}
	})
	steps := lensLegend(lens)
	if len(steps) == 0 {
		return
	}
	up := edge + bar.H + apart
	if over {
		up += row.H + apart
	}
	ui.Row(c.Key("lens-legend")).Absolute().Right(edge).Bottom(up).Gap(12).AlignItems(ui.Center).PassThrough().
		Label("图例").Children(func() {
		for _, s := range steps {
			fill, _ := lensFill(t, lens, s.step)
			ui.Row(c.Key(s.name)).Gap(5).AlignItems(ui.Center).PassThrough().Children(func() {
				ui.Box(c).Size(14, 10).Radius(3).Background(fill).Border(1, hairline(t)).Shrink(0)
				ui.Text(c, s.name).FontSize(11).TextColor(faint(t)).NoWrap()
			})
		}
	})
}

// overLensRow tells whether the pointer is on the lens's row: a click
// there is not one on the bare canvas under it, which makes a topic at a
// double click.
func (a *app) overLensRow(c *ui.Context) bool {
	r := a.lensRow.Bounds(c)
	x, y := a.vp.X+a.pointer[0], a.vp.Y+a.pointer[1]
	return r.W > 0 && x >= r.X && x <= r.X+r.W && y >= r.Y && y <= r.Y+r.H
}

// lensCount is a topic's count under 来源, a small tablet on its corner at
// its end: on the corner and not in the topic, which would be wider for
// it, and a lens moves nothing.
func (a *app) lensCount(c *ui.Context, n *model.Node, box [4]float32, left bool, count int) {
	t := c.Theme()
	cam := a.cam()
	z := cam.Zoom
	text := fmt.Sprint(count)
	w, _ := a.measureText(c, 0, ui.Span{Text: text, Size: 10, Weight: 700})
	bw, bh := (w+9)*z, 14*z
	x := cam.X + (box[0]+box[2])*z - bw*0.45
	if left {
		x = cam.X + box[0]*z - bw*0.55
	}
	ui.Box(c.Key("lens-count:"+n.ID)).Absolute().Left(x).Top(cam.Y+box[1]*z-bh*0.5).Size(bw, bh).Radius(bh/2).Center().
		Background(t.Background).Border(1, hairline(t)).PassThrough().Label(fmt.Sprintf("%d 条来源", count)).Children(func() {
		ui.Text(c, text).FontSize(10 * z).FontWeight(700).TextColor(teal(t).MixOklab(t.Text, 0.35)).FontFeatures("tnum").NoWrap()
	})
}
