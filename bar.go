package main

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// The one bar, at the top of the pages of lists, under the places: what is
// written in it is kept (a thought; a task where it begins with a day, or
// says how it repeats; a link), looked for as it is written (what there
// is already shows under the bar), or, begun with a ?, asked of the AI. It
// is one line until written in, then the card to write in, with its
// tools (writeCard).
func (a *app) oneBar(c *ui.Context) {
	t := c.Theme()
	text := strings.TrimSpace(a.homeDraft)
	ui.Column(c.Key("one-bar")).FillWidth().Shrink(0).AlignItems(ui.Center).Padding(4, 16, 14).ZIndex(20).Children(func() {
		// Its room is one line's, whatever it shows: open, the card lies over
		// the page under it, so that nothing moves as it opens and closes.
		ui.Column(c.Key("bar")).FillWidth().MaxWidth(640).Height(52).Children(func() {
			if !a.barOpen && text == "" {
				// One line: a click opens the card, and writes in it.
				pill := soft(ui.Row(c.Key("closed")).Height(52).Padding(0, 8, 0, 22).Gap(10).Radius(radiusPill).AlignItems(ui.Center).FillWidth().
					Background(cardColor(t)).Cursor(ui.CursorText).Label("记一条灵感或任务"), t).Transition(easeLook)
				if pill.Clicked() {
					a.barOpen, a.writeRefocus = true, "home-line"
				}
				pill.Children(func() {
					ui.Text(c, "记一句、搜索，或者问 AI…").FontSize(textTitle).TextColor(t.TextMuted).SingleLine().Grow(1).Shrink(1).MinWidth(0).PassThrough()
					ui.Box(c).Height(36).Padding(0, 16).Radius(radiusPill).Center().Background(t.Text.Alpha(0.05)).Shrink(0).PassThrough().Children(func() {
						ui.Text(c, "记下").FontSize(textSmall).FontWeight(600).TextColor(t.TextMuted).NoWrap()
					})
				})
				return
			}
			open := ui.Column(c.Key("open")).Absolute().Top(0).Left(0).Right(0).Gap(8).ZIndex(20)
			if open.PressedOutside() && text == "" {
				a.barOpen = false
			}
			open.Children(func() { a.barCard(c, text) })
		})
	})
}

// barCard is the bar, open: the card to write in, and under it what
// there is already of what is written.
func (a *app) barCard(c *ui.Context, text string) {
	t := c.Theme()
	func() {
		func() {
			ask, asking := barQuestion(text)
			a.writeCard(c, writeCard{
				key: "home-line", label: "记一条灵感或任务", draft: &a.homeDraft, enterSaves: true,
				placeholder: "记一句、搜索，或者问 AI…",
				sendLabel:   "记下", fileLabel: "加图片", save: a.saveHomeLine, lines: 1, over: true,
				focused: func() { a.barOpen = true },
				esc: func() {
					if strings.TrimSpace(a.homeDraft) == "" || !a.libOpen {
						a.barOpen = false
					}
				},
				paste: func(text string) bool { // a picture or files copied: kept at once
					if text != "" {
						return false
					}
					clips, err := a.clipClipboard("剪贴板")
					if err != nil {
						return false
					}
					a.libSel = clips[0].ID
					return true
				},
				reads: func() { // what the Enter key does with the words
					now := a.now()
					switch {
					case text == "":
					case asking:
						if ask != "" {
							softTag(c, "问 AI")
						}
					case taskLike(text, now):
						rest, rep, first := parseRepeat(text, now)
						_, due, _ := parseTaskLine(rest, now)
						if due == nil {
							due = first
						}
						label := "任务"
						if due != nil {
							label += " · " + dayLabel(*due, now)
						}
						if rep != nil {
							label += " · " + rep.Label()
						}
						softTag(c, label)
					case linkInTitle.MatchString(text):
						softTag(c, "收藏链接")
					}
				},
			})
			// What there is already, as it is written: a click goes to it.
			if hits := a.barHits(text, 4); len(hits) > 0 && !asking {
				lifted(ui.Column(c.Key("found")).Padding(6).Gap(1).Radius(radiusCard).Background(cardColor(t)).FillWidth(), t).
					Transition(ui.ElementTransition{Duration: 240 * time.Millisecond, Ease: easeDetent, Enter: &ui.Motion{Y: -8}}).Children(func() {
					ui.Text(c, "已经有的").FontSize(textCaption).TextColor(t.TextMuted).Margin(6, 12, 2)
					for i, it := range hits {
						row := ui.Row(c.Key(i)).MinHeight(40).Padding(6, 12).Gap(10).Radius(radiusRow).AlignItems(ui.Center).FillWidth().
							Cursor(ui.CursorPointer).Label("已有 " + it.title).Background(t.Text.Alpha(0)).Transition(easeLook)
						if row.Hovered() {
							row.Background(t.Text.Alpha(0.05))
						}
						if row.Clicked() {
							run := it.run
							a.homeDraft, a.barOpen = "", false
							a.later(run)
						}
						row.Children(func() {
							ui.Text(c, it.kind).FontSize(textCaption).TextColor(t.TextMuted).Width(32).Shrink(0).PassThrough()
							ui.Text(c, it.title).FontSize(textBody).SingleLine().Grow(1).Shrink(1).MinWidth(0).PassThrough()
							if it.detail != "" {
								ui.Text(c, it.detail).FontSize(textSmall).TextColor(t.TextMuted).SingleLine().Shrink(2).MinWidth(0).MaxWidth(220).PassThrough()
							}
						})
					}
				})
			}
		}()
	}()
}

// barQuestion tells whether what is written in the bar asks the AI: it
// begins with a question mark, and what is asked follows.
func barQuestion(text string) (string, bool) {
	for _, mark := range []string{"?", "？"} {
		if q, ok := strings.CutPrefix(text, mark); ok {
			return strings.TrimSpace(q), true
		}
	}
	return "", false
}

// barHits are the maps, topics, notes and clips that have what is written
// in the bar: n at most.
func (a *app) barHits(text string, n int) []paletteItem {
	if text == "" || strings.Contains(text, "\n") || a.lib == nil {
		return nil
	}
	mode, query := a.paletteMode, a.paletteQuery
	a.paletteMode, a.paletteQuery = paletteAll, text
	items := a.paletteItems()
	a.paletteMode, a.paletteQuery = mode, query
	var out []paletteItem
	for _, it := range items {
		if it.kind != "命令" && len(out) < n {
			out = append(out, it)
		}
	}
	return out
}

// barOver is the bar over a map, asked for from the map's own bar: the
// card to write in under the top of the window, put away by a press
// beside it, by Escape, and once it has kept what was written.
func (a *app) barOver(c *ui.Context) {
	w, _ := c.Size()
	bw := min(640, w-32)
	open := ui.Column(c.Key("bar-over")).Absolute().Top(toolbarHeight + 6).Left((w - bw) / 2).Width(bw).Gap(8).ZIndex(30)
	if open.PressedOutside() {
		a.barOpen = false
	}
	open.Children(func() { a.barCard(c, strings.TrimSpace(a.homeDraft)) })
}
