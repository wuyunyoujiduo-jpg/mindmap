package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/diagram"
)

// 对比, the fifth kind of a set (diagram.go): options side by side, a
// column an option and a row what they are compared by, which a tree does
// not say. As the other kinds it is drawn by the AI from a topic's branch
// and never edited: a cell says what the map says of an option and the
// topics it says so in, and one the map does not speak of is left open, a
// dashed 导图里没说, for the user to fill in the map.
//
// It says nothing of which option is the better: the AI is told not to
// (diaAsk), and a row that judges is dropped (diagram.Verdict). That is
// the user's, and the one thing on a set written by hand: 我的倾向, a mark
// on one option, kept beside the table by the option's name (Set.Lean) so
// that drawing the set again keeps it.
//
// Read: told a row a sentence, made of the cells' own words
// (compareSay); asked back with a column covered; and with the rows in
// which every option says the same put away. More than compareWide
// options do not fit side by side, and are a card each down the page.

// compareWide is how many options a table holds side by side.
const compareWide = 4

// compareDim is how wide the column of what is compared by is.
const compareDim = 150

// comparePick names a cell of the comparison as what is chosen: its row
// and its column, here from 0, there from 1.
func comparePick(row, col int) string {
	return "cell:" + strconv.Itoa(row+1) + ":" + strconv.Itoa(col+1)
}

// compareCell is the cell a pick names, its row and column from 0.
func compareCell(t *diagram.Compare, pick string) (row, col int, ok bool) {
	parts := strings.Split(pick, ":")
	if t == nil || len(parts) != 3 || parts[0] != "cell" {
		return 0, 0, false
	}
	row, _ = strconv.Atoi(parts[1])
	col, _ = strconv.Atoi(parts[2])
	if row < 1 || row > len(t.Rows) || col < 1 || col > len(t.Options) {
		return 0, 0, false
	}
	return row - 1, col - 1, true
}

// compareShown are the rows of the comparison that show, from 0: all, or
// those in which the options differ.
func (a *app) compareShown(f *diaFile) []int {
	var out []int
	for i, r := range f.Compare.Rows {
		if !a.dia.diff || !r.Same {
			out = append(out, i)
		}
	}
	return out
}

// compareQuizRows are the rows asked back with a column covered (both
// from 1): those in which the map speaks of the option, and the others do
// not say the same beside it.
func compareQuizRows(t *diagram.Compare, col int) []int {
	var rows []int
	if t != nil && col >= 1 && col <= len(t.Options) {
		for i, r := range t.Rows {
			if !r.Same && !r.Cells[col-1].Gap {
				rows = append(rows, i+1)
			}
		}
	}
	return rows
}

// compareCoverFrom is the first column from the one given (from 1), going
// round, that has something to ask back; 0 when none has.
func compareCoverFrom(t *diagram.Compare, from int) int {
	n := len(t.Options)
	for k := range n {
		if col := ((from-1+k)%n+n)%n + 1; len(compareQuizRows(t, col)) > 0 {
			return col
		}
	}
	return 0
}

// compareCover is the column to cover next: the one asked last while some
// of it was missed, else the one after it.
func compareCover(f *diaFile) int {
	from := 1
	if q := f.CompareQuiz; q != nil {
		if from = q.Col; len(q.Missed) == 0 {
			from++
		}
	}
	return compareCoverFrom(f.Compare, from)
}

// compareSay tells a row in a sentence, of the cells' own words: nothing
// is in it that the table does not say.
func compareSay(t *diagram.Compare, row int) string {
	r := t.Rows[row]
	if r.Same {
		return fmt.Sprintf("%s：%d 个方案都一样，%s。", r.Dim, len(t.Options), strings.TrimRight(r.Cells[0].Text, "。"))
	}
	parts := make([]string, len(r.Cells))
	for i, cell := range r.Cells {
		parts[i] = t.Options[i] + "，" + strings.TrimRight(cell.Text, "。")
		if cell.Gap {
			parts[i] = t.Options[i] + "，导图里没说"
		}
	}
	return r.Dim + "：" + strings.Join(parts, "；") + "。"
}

// leanTo marks the option the user leans to, or takes the mark off the one
// that has it. It is the user's own, kept with the set and not in what was
// drawn.
func (a *app) leanTo(f *diaFile, option string) {
	if f.Lean == option {
		option = ""
	}
	f.Lean = option
	if err := a.writeDia(f); err != nil {
		a.saveErr = "保存图解失败：" + err.Error()
	}
}

// compareMove takes the cell chosen to the next by an arrow, among the
// rows that show.
func (a *app) compareMove(f *diaFile, key ui.Key) {
	shown := a.compareShown(f)
	if len(shown) == 0 {
		return
	}
	row, col, ok := compareCell(f.Compare, a.dia.pick)
	at := slices.Index(shown, row)
	if !ok || at < 0 { // none chosen: the first
		a.dia.pick, a.diaReveal = comparePick(shown[0], 0), "row:"+strconv.Itoa(shown[0]+1)
		return
	}
	switch key {
	case ui.KeyDown:
		at++
	case ui.KeyUp:
		at--
	case ui.KeyRight:
		col++
	default:
		col--
	}
	if at >= 0 && at < len(shown) && col >= 0 && col < len(f.Compare.Options) {
		a.dia.pick, a.diaReveal = comparePick(shown[at], col), "row:"+strconv.Itoa(shown[at]+1)
	}
}

// diaCompare is the comparison: the options as its head, a row what they
// are compared by, and last the user's own row, 我的倾向. Bare, as a
// figure, that row shows only while an option is marked.
func (a *app) diaCompare(c *ui.Context, f *diaFile) {
	t := c.Theme()
	tb := f.Compare
	lit := a.diaLight(f)
	changed := a.diaChanges(f).changed
	shown := a.compareShown(f)
	reveal := a.diaReveal
	a.diaReveal = ""
	if len(tb.Options) > compareWide {
		a.compareCards(c, f, lit, changed, shown)
		return
	}
	a.diaRoom(c, "dia-compare", true).Children(func() {
		ui.Column(c.Key("compare")).MaxWidth(900).FillWidth().Padding(4, 20, 20).AlignSelf(ui.Center).Children(func() {
			ui.Row(c.Key("options")).FillWidth().BorderWidth(0, 0, 1, 0).BorderColor(faint(t)).Children(func() {
				ui.Box(c).Width(compareDim).Shrink(0)
				for i, o := range tb.Options {
					ui.Box(c.Key(i)).Grow(1).Basis(0).MinWidth(0).Padding(11, 14).Children(func() {
						ui.Text(c, o).FontSize(textBody).FontWeight(700).Wrap().MaxLines(2).LineHeight(1.35)
					})
				}
			})
			for _, i := range shown {
				r := tb.Rows[i]
				row := ui.Row(c.Key(fmt.Sprint("row:", i+1))).FillWidth().AlignItems(ui.Stretch).BorderWidth(0, 0, 1, 0).BorderColor(hairline(t)).
					Transition(rowMotion)
				if reveal == "row:"+strconv.Itoa(i+1) {
					row.ScrollIntoView()
				}
				row.Children(func() {
					a.compareDimCell(c, r.Dim, lit != nil && lit.rows != nil && !lit.rows[i+1])
					for k := range r.Cells {
						a.compareCellView(c, f, lit, changed, i, k)
					}
				})
			}
			if a.diaBare && f.Lean == "" {
				return
			}
			ui.Row(c.Key("mine")).FillWidth().AlignItems(ui.Stretch).Margin(-1, 0, 0).BorderWidth(1, 0, 0, 0).BorderColor(faint(t)).Children(func() {
				ui.Box(c).Width(compareDim).Shrink(0).Padding(11, 14).Children(func() {
					ui.Text(c, "我的倾向").FontSize(textSmall + 0.5).FontWeight(700).NoWrap()
				})
				for _, o := range tb.Options {
					a.leanKey(c, f, o, false)
				}
			})
		})
	})
}

// compareDimCell is the first cell of a row: what the options are
// compared by, in muted type.
func (a *app) compareDimCell(c *ui.Context, dim string, dimmed bool) {
	t := c.Theme()
	b := ui.Box(c.Key("dim")).Width(compareDim).Shrink(0).Padding(11, 14).Transition(easeLook)
	if dimmed {
		b.Opacity(0.25)
	}
	b.Children(func() {
		ui.Text(c, dim).FontSize(textSmall + 0.5).TextColor(t.TextMuted).Wrap().MaxLines(2).LineHeight(1.5)
	})
}

// compareCellView is a cell of the comparison: what the map says of an
// option, to choose; a dashed 导图里没说 where it says nothing; a pale
// block while it is covered, asked back, which a click opens.
func (a *app) compareCellView(c *ui.Context, f *diaFile, lit *diaLit, changed map[string]bool, row, col int) ui.Element {
	t := c.Theme()
	tb := f.Compare
	r := tb.Rows[row]
	cell := r.Cells[col]
	pick := comparePick(row, col)
	quiz := a.dia.way == diaQuiz && a.dia.cover > 0
	asked := quiz && col == a.dia.cover-1 && !cell.Gap && !r.Same
	covered := asked && !a.dia.open[row+1]
	el := ui.Box(c.Key(pick)).Grow(1).Basis(0).MinWidth(0).Padding(11, 14).Radius(radiusSmall).Label(r.Dim+"："+tb.Options[col]).
		Background(t.Text.Alpha(0)).InsetShadow(0, 0, 0, 1.5, t.Accent.Alpha(0)).Transition(easeLook)
	if !a.diaBare && (!quiz || covered) {
		if el.Cursor(ui.CursorPointer); el.Hovered() {
			el.Background(blockFill(t))
		}
	}
	if a.dia.pick == pick && !quiz {
		el.InsetShadow(0, 0, 0, 1.5, t.Accent)
	}
	if lit != nil && (lit.rows != nil && !lit.rows[row+1] || lit.cells != nil && !lit.cells[pick]) {
		el.Opacity(0.25)
	}
	if el.Clicked() {
		switch {
		case covered:
			if a.dia.open == nil {
				a.dia.open, a.dia.got = map[int]bool{}, map[int]bool{}
			}
			a.dia.open[row+1], a.dia.cur = true, row+1
		case quiz:
		case a.dia.pick == pick:
			a.dia.pick = ""
		default:
			a.dia.pick = pick
		}
	}
	el.Children(func() {
		switch {
		case covered:
			ui.Box(c.Key("cover")).FillWidth().Height(20).Radius(radiusSmall).Background(t.Text.Alpha(0.09)).PassThrough()
		case cell.Gap:
			ui.Row(c).PassThrough().Children(func() { // as wide as its words
				ui.Box(c.Key("unsaid")).Padding(2, 8).Radius(radiusSmall).Border(1, faint(t)).BorderStyle(ui.BorderDashed).PassThrough().Children(func() {
					ui.Text(c, "导图里没说").FontSize(textSmall).TextColor(faint(t)).NoWrap().PassThrough()
				})
			})
		default:
			txt := ui.Text(c, cell.Text).FontSize(textSmall + 0.5).Wrap().LineHeight(1.5).PassThrough()
			if r.Same {
				txt.TextColor(t.TextMuted)
			}
			if slices.ContainsFunc(cell.Src, func(id string) bool { return changed[id] }) { // what it was drawn from has changed
				ui.Box(c.Key("stale")).Absolute().Left(3).Top(17).Size(7, 7).Radius(4).Background(t.Warning).PassThrough()
			}
		}
	})
	return el
}

// leanKey is an option's cell of 我的倾向: a click marks the option as the
// one leant to, a click again takes the mark off. On a card it is a key
// at the card's head.
func (a *app) leanKey(c *ui.Context, f *diaFile, option string, card bool) {
	t := c.Theme()
	on := f.Lean == option
	el := ui.Row(c.Key("lean:" + option)).Gap(7).AlignItems(ui.Center).Radius(radiusSmall).Label("倾向 " + option).
		Background(t.Text.Alpha(0)).Transition(easeLook)
	if card {
		el.Height(28).Padding(0, 10).Shrink(0)
	} else {
		el.Grow(1).Basis(0).MinWidth(0).Padding(11, 14)
	}
	if !a.diaBare {
		el.Cursor(ui.CursorPointer).Tooltip("标出你倾向这个方案；再点一下去掉")
		if on {
			el.Tooltip("不再倾向这个方案")
		}
		if el.Hovered() {
			el.Background(blockFill(t))
		}
	}
	if el.Clicked() {
		a.later(func() { a.leanTo(f, option) })
	}
	el.Children(func() {
		switch {
		case on:
			ui.Box(c).Size(8, 8).Radius(4).Background(t.Accent).Shrink(0).PassThrough()
			ui.Text(c, "倾向这个").FontSize(textSmall + 0.5).FontWeight(700).TextColor(t.Accent).NoWrap().PassThrough()
		case a.diaBare:
		case card:
			ui.Text(c, "标为倾向").FontSize(textSmall).TextColor(faint(t)).NoWrap().PassThrough()
		case f.Lean == "":
			ui.Text(c, "点一下标出").FontSize(textSmall + 0.5).TextColor(faint(t)).NoWrap().PassThrough()
		}
	})
}

// compareCards is the comparison of more options than stand side by side:
// each a card down the page, what it is compared by as its lines.
func (a *app) compareCards(c *ui.Context, f *diaFile, lit *diaLit, changed map[string]bool, shown []int) {
	t := c.Theme()
	tb := f.Compare
	edge := float32(0.1) // wallCard's line, on a card that is not itself pressed
	if t.Dark {
		edge = 0.065
	}
	a.diaRoom(c, "dia-compare", true).Children(func() {
		ui.Column(c.Key("compare-cards")).MaxWidth(700).FillWidth().Padding(4, 20, 20).Gap(12).AlignSelf(ui.Center).Children(func() {
			for k, o := range tb.Options {
				ui.Column(c.Key(fmt.Sprint("option:", k+1))).FillWidth().Padding(4, 4, 4).Radius(radiusCard).Background(cardColor(t)).
					InsetShadow(0, 0, 0, 1, t.Text.Alpha(edge)).Children(func() {
					ui.Row(c.Key("head")).FillWidth().MinHeight(40).Padding(4, 4, 4, 14).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Text(c, o).FontSize(textBody).FontWeight(700).Shrink(1).MinWidth(0).SingleLine()
						ui.Spacer(c)
						if !a.diaBare || f.Lean == o {
							a.leanKey(c, f, o, true)
						}
					})
					for _, i := range shown {
						ui.Row(c.Key(fmt.Sprint("row:", i+1))).FillWidth().AlignItems(ui.Stretch).BorderWidth(1, 0, 0, 0).BorderColor(hairline(t)).Children(func() {
							a.compareDimCell(c, tb.Rows[i].Dim, lit != nil && lit.rows != nil && !lit.rows[i+1])
							a.compareCellView(c, f, lit, changed, i, k)
						})
					}
				})
			}
		})
	})
}

// compareWays is the row under the comparison: told a row a sentence,
// asked back with a column covered, and at its right the key that puts
// away the rows in which every option says the same.
func (a *app) compareWays(c *ui.Context, f *diaFile) {
	t := c.Theme()
	tb := f.Compare
	shown := a.compareShown(f)
	off := func() {
		if iconButton(c, iconClose, "不这样看了", "不这样看了 Esc", 30, true).Clicked() {
			a.dia.way = diaRead
		}
	}
	chip := func(k, text string) ui.Element {
		b := ui.Box(c.Key(k)).Height(30).Padding(0, 12).Radius(radiusPill).Center().Shrink(0).Cursor(ui.CursorPointer).Label(text).
			Background(t.Text.Alpha(0.05)).Transition(easeLook)
		if b.Hovered() {
			b.Background(t.Text.Alpha(0.09))
		}
		b.Children(func() { ui.Text(c, text).FontSize(textSmall).TextColor(ink2(t)).NoWrap().PassThrough() })
		return press(b, 0.97)
	}
	if a.dia.way == diaCase || a.dia.way == diaTell && len(shown) == 0 ||
		a.dia.way == diaQuiz && len(compareQuizRows(tb, a.dia.cover)) == 0 {
		a.dia.way = diaRead
	}
	row := ui.Row(c.Key(fmt.Sprint("dia-ways", a.dia.way))).MinHeight(48).Margin(0, 16, 6).Padding(4, 12).Gap(8).Radius(radiusCard).AlignItems(ui.Center).Shrink(0)
	if a.dia.way != diaRead {
		row.Background(blockFill(t)).Justify(ui.Center)
	}
	row.Children(func() {
		switch a.dia.way {
		case diaTell:
			step := min(a.dia.step, len(shown)-1)
			if iconButton(c, iconBack, "上一行", "上一行 ←", 30, step > 0).Clicked() && step > 0 {
				a.tellStep(f, -1)
			}
			ui.Text(c, fmt.Sprintf("%d / %d", step+1, len(shown))).FontSize(textCaption).TextColor(t.TextMuted).FontFeatures("tnum").NoWrap()
			ui.Text(c, compareSay(tb, shown[step])).FontSize(textSmall + 0.5).Shrink(1).MinWidth(0).Wrap().MaxLines(3).LineHeight(1.4)
			next := "下一行"
			if step == len(shown)-1 {
				next = "讲完了"
			}
			if accentButton(c, next, "→").Clicked() {
				a.tellStep(f, 1)
			}
			off()
		case diaQuiz:
			rows := compareQuizRows(tb, a.dia.cover)
			right := 0
			for _, r := range rows {
				if a.dia.got[r] {
					right++
				}
			}
			_, answered := a.dia.got[a.dia.cur]
			switch {
			case a.dia.cur > 0 && !answered:
				ui.Text(c, "说对了吗？").FontSize(textSmall + 0.5)
				if chip("quiz-right", "想对了").Clicked() {
					a.answerQuiz(true)
				}
				if chip("quiz-missed", "没想起来").Clicked() {
					a.answerQuiz(false)
				}
			case len(a.dia.got) >= len(rows):
				say := fmt.Sprintf("%d 格里想起 %d 格。", len(rows), right)
				if right < len(rows) {
					say += fmt.Sprintf("没想起来的，%d 天后「今天」会提醒你再考。", quizAgainMissed)
				} else {
					say += fmt.Sprintf("%d 天后再考一次。", quizAgainRight)
				}
				ui.Text(c, say).FontSize(textSmall + 0.5)
			default:
				ui.Text(c, "「"+tb.Options[a.dia.cover-1]+"」这一列盖住了。先自己说，再点格子看导图里是怎么写的。").FontSize(textSmall + 0.5).
					Shrink(1).MinWidth(0).Wrap().MaxLines(2).LineHeight(1.4)
				ui.Text(c, fmt.Sprintf("%d / %d", len(a.dia.got), len(rows))).FontSize(textCaption).TextColor(t.TextMuted).FontFeatures("tnum").NoWrap()
				if other := compareCoverFrom(tb, a.dia.cover+1); other != a.dia.cover && len(a.dia.got) == 0 {
					if chip("quiz-other", "换一列").Clicked() {
						a.dia.cover, a.dia.open, a.dia.got, a.dia.cur = other, map[int]bool{}, map[int]bool{}, 0
					}
				}
			}
			off()
		default:
			way := func(text, tip string, ic icon, w int, mark bool) {
				b := key(ui.Row(c.Key("way:"+text)).Height(30).Padding(0, 12).Gap(6).Radius(radiusPill).AlignItems(ui.Center).
					Cursor(ui.CursorPointer).Label(text).Tooltip(tip).Shrink(0), t)
				if b.Clicked() {
					a.setDiaWay(w)
				}
				b.Children(func() {
					if ic >= 0 {
						glyph(c, ic, 11, ink2(t))
					}
					ui.Text(c, text).FontSize(textSmall).TextColor(ink2(t)).NoWrap().PassThrough()
					if mark {
						ui.Box(c).Size(7, 7).Radius(4).Background(t.Warning).PassThrough()
					}
				})
			}
			way("讲一遍", "一行一行讲这张表，每行一句话", iconPlay, diaTell, false)
			if compareCover(f) > 0 {
				way("考我", "盖住一个方案的那一列，先自己说，再点开对一对", -1, diaQuiz, a.dueKind(f) == "compare")
			}
			ui.Spacer(c)
			if same := tb.Sames(); same > 0 {
				b := ui.Box(c.Key("diff")).Height(30).Padding(0, 12).Radius(radiusPill).Center().Shrink(0).Cursor(ui.CursorPointer).
					Label("只看有差别的").Tooltip(fmt.Sprintf("收起每个方案都一样的 %d 行", same)).Transition(easeLook)
				ink := ink2(t)
				if a.dia.diff {
					b.Tooltip("每一行都看")
					b.Background(t.Accent.Alpha(0.12))
					ink = t.Accent
				} else {
					key(b, t)
				}
				if b.Clicked() {
					a.dia.diff = !a.dia.diff
					if row, _, ok := compareCell(tb, a.dia.pick); ok && a.dia.diff && tb.Rows[row].Same {
						a.dia.pick = "" // put away with its row
					}
				}
				b.Children(func() {
					txt := ui.Text(c, "只看有差别的").FontSize(textSmall).TextColor(ink).NoWrap().PassThrough()
					if a.dia.diff {
						txt.FontWeight(700)
					}
				})
			}
		}
	})
}

// compareNext says, in the right column with nothing chosen, what of the
// comparison to look at next: how many cells the map does not speak of,
// each a line that goes to it, and in how many rows the options say the
// same.
func (a *app) compareNext(c *ui.Context, f *diaFile) {
	t := c.Theme()
	tb := f.Compare
	gaps, sames := tb.Gaps(), tb.Sames()
	ui.Column(c.Key("compare-next")).FillWidth().Children(func() {
		trayHead(c, "next-head", "接下来看什么", "", nil)
		fig := func(n int, what string) {
			ui.Row(c).Gap(5).AlignItems(ui.Baseline).Children(func() {
				ui.Text(c, strconv.Itoa(n)).FontSize(textHeadline).FontWeight(700).FontFeatures("tnum")
				ui.Text(c, what).FontSize(textCaption).TextColor(t.TextMuted)
			})
		}
		ui.Row(c.Key("figures")).Gap(18).Padding(0, 4, 4).Children(func() {
			fig(gaps, "格没说")
			fig(sames, "行都一样")
		})
		if gaps == 0 {
			return
		}
		ui.Text(c, "导图里没说到的").FontSize(textCaption).FontWeight(700).TextColor(t.TextMuted).Padding(8, 4, 2)
		for i, r := range tb.Rows {
			for k, cell := range r.Cells {
				if !cell.Gap {
					continue
				}
				name := tb.Options[k] + "的" + r.Dim
				row := ui.Row(c.Key(comparePick(i, k))).FillWidth().Padding(9, 4).Gap(10).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label("没说 "+name).
					BorderWidth(0, 0, 1, 0).BorderColor(hairline(t)).Background(t.Text.Alpha(0)).Transition(rowMotion)
				if row.Hovered() {
					row.Background(t.Text.Alpha(0.035))
				}
				if row.Clicked() {
					a.dia.pick, a.dia.way, a.diaReveal = comparePick(i, k), diaRead, "row:"+strconv.Itoa(i+1)
				}
				row.Children(func() {
					ui.Text(c, name).FontSize(textSmall + 0.5).Grow(1).Shrink(1).MinWidth(0).Wrap().LineHeight(1.5).PassThrough()
					glyph(c, iconForward, 10, faint(t))
				})
			}
		}
	})
}
