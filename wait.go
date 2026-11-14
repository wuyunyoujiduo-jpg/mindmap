package main

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
)

// How the app waits (DESIGN.md's waiting): what is of this computer is
// done as it is clicked and shows nothing; a short wait for one action
// shows on its key, whose words and width stay; long work that can be
// counted is a card at the window's top right, which stays on every page
// and turns into its result; work that cannot be counted says the stage
// it is in. Nothing that shows comes before 300ms or goes before 500ms
// more, so that nothing flickers.

const (
	waitDelay = 300 * time.Millisecond // before a wait shows
	waitHold  = 500 * time.Millisecond // the least it shows, once shown
	waitLong  = 10 * time.Second       // after which a wait says it is long
)

type waitState struct{ since, shown time.Time }

var waits = map[string]*waitState{}

// waitShown tells whether the wait named key shows: not before it has
// lasted waitDelay, and then for waitHold at least, though it be over.
// The one helper every spinner, skeleton and card goes through.
func waitShown(c *ui.Context, key string, busy bool) bool {
	if motionOff {
		return busy // the tests look at where things end
	}
	now := time.Now()
	s := waits[key]
	if busy {
		if s == nil {
			s = &waitState{since: now}
			waits[key] = s
		}
		if s.shown.IsZero() {
			if wait := waitDelay - now.Sub(s.since); wait > 0 {
				c.After(wait)
				return false
			}
			s.shown = now
		}
		return true
	}
	if s == nil {
		return false
	}
	if !s.shown.IsZero() {
		if left := waitHold - now.Sub(s.shown); left > 0 {
			c.After(left)
			return true
		}
	}
	delete(waits, key)
	return false
}

// waitSince is since when the wait named key has lasted, if it has begun.
func waitSince(key string) (time.Time, bool) {
	if s := waits[key]; s != nil {
		return s.since, true
	}
	return time.Time{}, false
}

// busyKey is a key that starts something to wait for: while it waits its
// words stay, fainter, with a spinner over them, its width as it was, and
// a second press does nothing. primary is the key that acts, in the accent.
func busyKey(c *ui.Context, text, tip string, busy, enabled, primary bool) ui.Element {
	t := c.Theme()
	shown := waitShown(c, "key:"+text, busy)
	b := ui.Box(c.Key("busy:"+text)).Height(30).Padding(0, 14).Radius(radiusPill).Center().Shrink(0).
		Label(text).Tooltip(tip).Transition(easeLook)
	ink := t.Text
	switch {
	case primary && (enabled || busy):
		b.Background(t.Accent)
		ink = t.AccentText
		if b.Hovered() && !busy {
			b.Background(t.AccentHover)
		}
	case primary:
		b.Background(t.Accent.Alpha(0.32))
		ink = t.AccentText
	default:
		b.Background(t.Text.Alpha(0.06))
		if b.Hovered() && enabled && !busy {
			b.Background(t.Text.Alpha(0.1))
		}
		if !enabled && !busy {
			ink = faint(t)
		}
	}
	if enabled && !busy {
		b.Cursor(ui.CursorPointer)
		press(b, 0.97)
	}
	b.Children(func() {
		txt := ui.Text(c, text).FontSize(textSmall).TextColor(ink).NoWrap().PassThrough()
		if primary {
			txt.FontWeight(700)
		}
		if shown {
			txt.TextColor(ink.Alpha(0.25))
			ui.Box(c.Key("spin")).Absolute().Top(0).Right(0).Bottom(0).Left(0).Center().PassThrough().Children(func() {
				ui.Spinner(c).Size(13, 13)
			})
		}
	})
	return b
}

// busyClicked tells whether a busyKey was pressed and may act.
func busyClicked(b ui.Element, busy, enabled bool) bool {
	return enabled && !busy && b.Clicked()
}

// A jobCard is long work shown at the window's top right: what it is, how
// far, and when done what came of it.
type jobCard struct {
	key, icon   string
	title, line string
	frac        float32 // how far, 0 to 1; negative when it cannot be counted
	since       time.Time
	done        bool
	failed      bool
	cancel      func() // stops it, while it runs
	close       func() // puts the card away, once done
	open        func() // what a click on it shows
}

// timeLeft says about how long work that is frac done since since has
// left, once there is a second of it to tell from.
func timeLeft(now, since time.Time, frac float32) string {
	el := now.Sub(since)
	if since.IsZero() || el < time.Second || frac <= 0.02 {
		return "正在估算…"
	}
	left := time.Duration(float64(el) * float64(1-frac) / float64(frac))
	switch {
	case left < 5*time.Second:
		return "马上就好"
	case left < time.Minute:
		return fmt.Sprintf("约剩 %d 秒", int(left.Seconds()/5+1)*5)
	}
	return fmt.Sprintf("约剩 %d 分钟", int(left.Minutes()+1))
}

// jobsView shows the long work going on, and what was just done, as cards
// at the top right, over whatever page shows and taking no room from it.
func (a *app) jobsView(c *ui.Context) {
	t := c.Theme()
	var cards []jobCard
	for _, j := range a.jobs() {
		if j.done || waitShown(c, "job:"+j.key, true) {
			cards = append(cards, j)
		} else {
			_ = j
		}
	}
	for key := range waits { // those over: forgotten
		if len(key) > 4 && key[:4] == "job:" && !hasJob(a.jobs(), key[4:]) {
			delete(waits, key)
		}
	}
	if len(cards) == 0 {
		return
	}
	w, _ := c.Size()
	// At the window's foot on the right, where nothing of a page is
	// pressed: under the top row they lay over a page's own keys. Beside
	// the AI's window when that is open there.
	right := float32(14)
	if a.aiOpen {
		right += min(392, w-32) + 12
	}
	ui.Column(c.Key("jobs")).Absolute().Bottom(16).Right(right).Width(min(332, w-28)).Gap(8).ZIndex(24).Children(func() {
		for _, j := range cards {
			card := lifted(ui.Row(c.Key("job:"+j.key)).Padding(12, 14, 12, 12).Gap(12).Radius(16).AlignItems(ui.Center).FillWidth().
				Background(cardColor(t)).Label(j.title), t).
				Transition(ui.ElementTransition{Duration: motionSlow, Ease: easeDetent, Enter: &ui.Motion{Y: 24, Scale: 0.96},
					Exit: &ui.Motion{Scale: 0.96}, ExitDuration: motionQuick})
			hover := card.Hovered()
			if j.open != nil {
				card.Cursor(ui.CursorPointer)
				if card.Clicked() {
					a.later(j.open)
				}
			}
			card.Children(func() {
				// Its tile: the work's icon, or, done, a check on green.
				tile := ui.Box(c).Size(34, 34).Radius(10).Center().Shrink(0).Background(t.Accent.Alpha(0.1)).Transition(easeLook)
				switch {
				case j.failed:
					tile.Background(t.Danger.Alpha(0.12)).Children(func() { glyph(c, iconClose, 15, t.Danger) })
				case j.done:
					tile.Background(t.Success).Children(func() { glyph(c, iconCheck, 17, ui.Hex("#FFFFFF")) })
				default:
					tile.Children(func() { kubaiIcon(c, j.icon, 24, true) })
				}
				ui.Column(c).Gap(3).Grow(1).Shrink(1).Basis(0).MinWidth(0).Children(func() {
					ui.Text(c, j.title).FontSize(textSmall).FontWeight(700).SingleLine()
					if j.line != "" {
						col := t.TextMuted
						if j.failed {
							col = t.Danger
						}
						ui.Text(c, j.line).FontSize(textCaption).TextColor(col).FontFeatures("tnum").MaxLines(2)
					}
					if !j.done && j.frac >= 0 {
						ui.Box(c).Height(4).FillWidth().Radius(2).Margin(4, 0, 0).Background(t.Accent.Alpha(0.12)).Children(func() {
							ui.Box(c).Height(4).WidthPercent(max(min(j.frac, 1), 0.02) * 100).Radius(2).Background(t.Accent).
								Transition(ui.ElementTransition{Duration: motionSlow, Ease: easeDetent, Width: true})
						})
					}
				})
				// Under the pointer, the mark that stops it or puts it away.
				end, tip := j.cancel, "取消"
				if j.done {
					end, tip = j.close, "关闭"
				}
				if end != nil && (hover || j.done) {
					if iconButton(c, iconClose, tip+" "+j.title, tip, 24, true).Clicked() {
						a.later(end)
					}
				}
			})
		}
	})
}

func hasJob(js []jobCard, key string) bool {
	for _, j := range js {
		if j.key == key && !j.done {
			return true
		}
	}
	return false
}

// stageLines says what a wait that cannot be counted is doing: the steps
// done, each with a check, the one going on with a spinner; and, once it
// has lasted long, that it takes longer than usual.
func stageLines(c *ui.Context, key string, done []string, doing string) {
	t := c.Theme()
	ui.Column(c.Key("stages:"+key)).Gap(6).Padding(6, 4).FillWidth().Label("进行中").Children(func() {
		for i, s := range done {
			ui.Row(c.Key(i)).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(14, 14).Center().Shrink(0).Children(func() { glyph(c, iconCheck, 12, t.Success) })
				ui.Text(c, s).FontSize(textSmall).TextColor(t.TextMuted)
			})
		}
		ui.Row(c.Key("doing")).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(14, 14).Center().Shrink(0).Children(func() { ui.Spinner(c).Size(12, 12) })
			ui.Text(c, doing).FontSize(textSmall)
		})
		waitShown(c, "stage:"+key, true)
		if since, ok := waitSince("stage:" + key); ok && time.Since(since) > waitLong {
			ui.Text(c, "比平时久一些。可以先做别的，好了会在这里显示。").FontSize(textCaption).TextColor(faint(t)).Margin(0, 0, 0, 22)
		}
	})
}

// stageOver forgets a stage's wait, when what it waited for has come.
func stageOver(key string) { delete(waits, "stage:"+key) }

// jobs are the long works going on and those just done, as cards: the
// library's index, a run of clips being organized, the plugin coming down.
func (a *app) jobs() []jobCard {
	var out []jobCard
	now := a.now()
	if a.embedTotal > 1 {
		done := a.embedTotal - a.embedLeft
		frac := float32(done) / float32(a.embedTotal)
		out = append(out, jobCard{key: "embed", icon: "layers", title: "建立语义索引",
			line: fmt.Sprintf("%d / %d 条 · %s", done, a.embedTotal, timeLeft(now, a.embedSince, frac)), frac: frac, since: a.embedSince,
			open: func() { a.openList(kindAll, "") }})
	} else if a.embedResult != "" {
		out = append(out, jobCard{key: "embed", title: a.embedResult, line: "现在可以按意思搜索了", done: true,
			close: func() { a.embedResult = "" }, open: func() { a.embedResult = ""; a.openList(kindAll, "") }})
	}
	if a.orgTotal > 1 {
		frac := float32(a.orgDone) / float32(a.orgTotal)
		out = append(out, jobCard{key: "organize", icon: "sparkles", title: "AI 整理资料",
			line: fmt.Sprintf("第 %d 条，共 %d 条 · %s", min(a.orgDone+1, a.orgTotal), a.orgTotal, timeLeft(now, a.orgSince, frac)), frac: frac, since: a.orgSince,
			cancel: func() { // those not begun are left; the one going on ends
				a.libQueue = nil
				a.orgTotal = a.orgDone + 1
			},
			open: func() { a.openList(kindAll, "") }})
	} else if a.orgResult != "" {
		out = append(out, jobCard{key: "organize", title: a.orgResult, line: "标题、摘要、分类和标签都写好了", done: true,
			close: func() { a.orgResult = "" }, open: func() { a.orgResult = ""; a.openList(kindAll, "") }})
	}
	switch {
	case a.pluginBusy:
		line, frac := "正在连接…", float32(-1)
		if a.pluginSize > 0 {
			frac = float32(a.pluginDone) / float32(a.pluginSize)
			line = fmt.Sprintf("%.1f / %.1f MB · %s", float64(a.pluginDone)/1e6, float64(a.pluginSize)/1e6, timeLeft(now, a.pluginSince, frac))
		}
		out = append(out, jobCard{key: "plugin", icon: "import", title: "下载语义检索插件", line: line, frac: frac, since: a.pluginSince, cancel: a.pluginCancel})
	case a.pluginResult != "":
		out = append(out, jobCard{key: "plugin", title: a.pluginResult, line: "收集现在可以按意思搜索", done: true, close: func() { a.pluginResult = "" }})
	}
	return out
}

// toastLook is how a toast looks: a small card at the window's foot, in
// its middle, told by its shadow as all that floats; a check on a green
// tile before its words (a cross on red for what failed), and after them
// the key of what is next with it, as 撤销.
func toastLook(c *ui.Context) {
	t := c.Theme()
	ui.ToastViewportBase(c, func(viewport ui.Element, toasts []ui.Toast) {
		viewport.Padding(0, 16, 22).AlignItems(ui.Center).Gap(8)
		for _, one := range toasts {
			toast := ui.ToastBase(c, one)
			lifted(toast.Root.Row().Gap(10).Padding(7, 14, 7, 8).Radius(14).AlignItems(ui.Center).Background(cardColor(t)), t)
			toast.Root.Transition(ui.ElementTransition{Duration: motionSlow, Ease: easeDetent, Enter: &ui.Motion{Y: 10, Scale: 0.97},
				Exit: &ui.Motion{Scale: 0.97}, ExitDuration: motionMedium})
			toast.Root.Children(func() {
				tile := ui.Box(c).Size(24, 24).Radius(8).Center().Shrink(0)
				if one.Type == "error" {
					tile.Background(t.Danger.Alpha(0.14)).Children(func() { glyph(c, iconClose, 12, t.Danger) })
				} else {
					tile.Background(t.Success.Alpha(0.16)).Children(func() { glyph(c, iconCheck, 13, t.Success) })
				}
				ui.Text(c, one.Title).FontSize(textSmall).MaxWidth(420).MaxLines(2)
				if one.Action != "" {
					act := toast.ActionButton().Height(26).Padding(0, 10).Radius(radiusSmall).Center().Cursor(ui.CursorPointer).
						Background(t.Accent.Alpha(0.1)).Transition(easeLook)
					if act.Hovered() {
						act.Background(t.Accent.Alpha(0.18))
					}
					act.Children(func() { ui.Text(c, one.Action).FontSize(textSmall).FontWeight(700).TextColor(t.Accent).NoWrap() })
				}
			})
		}
	})
}
