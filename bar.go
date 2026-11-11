package main

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// The one bar, in the top row of every page, between the places and the
// keys: what is
// written in it is kept (a thought; a task where it begins with a day, or
// says how it repeats; a link), looked for as it is written (what there
// is already shows under the bar), or, begun with a ?, asked of the AI. It
// is one line until written in, then the card to write in, with its
// tools (writeCard), floating forward of the page.
func (a *app) oneBar(c *ui.Context) {
	t := c.Theme()
	text := strings.TrimSpace(a.homeDraft)
	// Its room is one line's in the top row, whatever it shows: open, the
	// card lies over the page under it, so that nothing moves.
	ui.Row(c.Key("one-bar")).Grow(1).Basis(0).MinWidth(120).Height(36).Justify(ui.Center).Children(func() {
		ui.Column(c.Key("bar")).Grow(1).MaxWidth(640).Height(36).Children(func() {
			if !a.barOpen && text == "" {
				// One line: a click opens the card, and writes in it.
				field := ui.Row(c.Key("closed")).Height(36).Padding(0, 6, 0, 14).Gap(10).Radius(radiusPill).AlignItems(ui.Center).FillWidth().
					Background(cardColor(t)).Shadow(0, 0, 0, 0.5, t.Text.Alpha(0.1)).Cursor(ui.CursorText).Label("记一条灵感或任务").Transition(easeLook)
				if field.Clicked() {
					a.barOpen, a.writeRefocus = true, "home-line"
				}
				field.Children(func() {
					ui.Text(c, "记灵感、加任务，或者问 AI…").FontSize(textBody).TextColor(t.TextMuted).SingleLine().Grow(1).Shrink(1).MinWidth(0).PassThrough()
				})
				return
			}
			open := ui.Column(c.Key("open")).Absolute().Top(-6).Left(0).Right(0).Gap(8).ZIndex(20)
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
				placeholder: "记灵感、加任务，或者问 AI…",
				sendLabel:   "记下", fileLabel: "加图片", save: a.saveHomeLine, lines: 1, over: true,
				focused: func() { a.barOpen = true },
				esc: func() {
					if strings.TrimSpace(a.homeDraft) == "" {
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
				reads: func() { // what it is kept as: a click makes a task of a thought, or a thought of a task
					now := a.now()
					label, tip := "记为 灵感", "现在会记成灵感（收集里的一条笔记）。点一下改成任务"
					switch {
					case asking:
						if ask != "" {
							softTag(c, "问 AI")
						}
						return
					case a.barTask(text):
						rest, rep, first := parseRepeat(text, now)
						_, due, _ := parseTaskLine(rest, now)
						if due == nil {
							due = first
						}
						if due == nil && a.place() == placeToday {
							d := today(now)
							due = &d
						}
						label, tip = "记为 任务", "现在会记成任务（要做的事，能打勾、定日子、重复）。点一下改成灵感"
						if due != nil {
							label += " · " + dayLabel(*due, now)
						}
						if rep != nil {
							label += " · " + rep.Label()
						}
					case linkInTitle.MatchString(text):
						label = "收藏链接"
					}
					chip := softTag(c, label).Cursor(ui.CursorPointer).Label("记为什么").Tooltip(tip)
					if chip.Clicked() {
						if a.barTask(text) {
							a.barKind = barThought
						} else {
							a.barKind = barATask
						}
						a.writeRefocus = "home-line"
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

// What the bar keeps its words as, when chosen by hand; else by the words
// and the place (barTask).
const (
	barAuto = iota
	barATask
	barThought
)

// barTask tells whether what is written in the bar is kept as a task: as
// chosen on the bar; else where it reads as one (a day, a way to repeat),
// and, on 今天, whatever is one plain line.
func (a *app) barTask(text string) bool {
	switch a.barKind {
	case barATask:
		return true
	case barThought:
		return false
	}
	if taskLike(text, a.now()) {
		return true
	}
	// A task is one plain line: what has a link, a tag, a reference, bold
	// or lines is a note's.
	note := linkInTitle.MatchString(text) || strings.ContainsAny(text, "\n#") || strings.Contains(text, "**") || len(a.writeRefs) > 0
	return a.place() == placeToday && !note
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
