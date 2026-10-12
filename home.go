package main

import (
	"fmt"
	"hash/fnv"
	"math"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
)

// The look of the sidebar and of 概览 follows Pulse's: rows of the
// sidebar with an icon on a tile of its own color and a count on the
// right, the current one filled with the accent; pages of soft cards with
// large numbers.

// Colors of the tiles.
var (
	tileBlue   = ui.Hex("#3B82F6")
	tileOrange = ui.Hex("#F08A3C")
	tileGreen  = ui.Hex("#22C55E")
	tilePurple = ui.Hex("#8B5CF6")
	tilePink   = ui.Hex("#EC4899")
	tileYellow = ui.Hex("#EAB308")
	tileTeal   = ui.Hex("#14B8A6")
	tileRed    = ui.Hex("#EF4444")
)

var tilePalette = []ui.Color{tileBlue, tileOrange, tileGreen, tilePurple, tilePink, tileTeal, tileYellow, tileRed}

// catColor is a category's color, the same each time.
func catColor(name string) ui.Color {
	h := fnv.New32a()
	h.Write([]byte(name))
	return tilePalette[h.Sum32()%uint32(len(tilePalette))]
}

// tile is an icon, white, on a rounded square of a color.
func tile(c *ui.Context, ic icon, col ui.Color, size float32) {
	ui.Box(c).Size(size, size).Shrink(0).Radius(size * 0.28).Background(col).PassThrough().
		Draw(func(p *ui.Painter, r ui.Rect) { drawIcon(p, r, size*0.62, ic, ui.Hex("#FFFFFF")) })
}

// navRow is a row of the sidebar: a tile, a label and a count. The
// current one is filled with the accent.
func navRow(c *ui.Context, ic icon, col ui.Color, label, count string, current bool) ui.Element {
	t := c.Theme()
	r := ui.Row(c).Padding(5, 8).Gap(10).Radius(8).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(label).FillWidth()
	fg, muted := t.Text, t.TextMuted
	switch {
	case current:
		r.Background(t.Accent)
		fg, muted = t.AccentText, t.AccentText.Alpha(0.85)
	case r.Hovered():
		r.Background(t.Text.Alpha(0.06))
	}
	r.Children(func() {
		tc := col
		if current {
			tc = t.AccentText.Alpha(0.25)
		}
		tile(c, ic, tc, 22)
		ui.Text(c, label).FontSize(13).TextColor(fg).Grow(1).Shrink(1).MinWidth(0).SingleLine()
		if count != "" {
			ui.Text(c, count).FontSize(12).TextColor(muted).Shrink(0)
		}
	})
	return r
}

// pageCard is a card of a page: soft, much rounded.
func pageCard(c *ui.Context) ui.Element {
	t := c.Theme()
	return ui.Column(c).Padding(16, 18).Gap(12).Radius(16).Background(t.Text.Alpha(0.035)).FillWidth()
}

// cardHead is the head of a card: a tile, a title in its color, and a link
// on the right when more is not "".
func cardHead(c *ui.Context, ic icon, col ui.Color, title, more string) bool {
	t := c.Theme()
	clicked := false
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		tile(c, ic, col, 26)
		ui.Text(c, title).FontSize(15).Bold().TextColor(col).Grow(1)
		if more != "" {
			l := ui.Row(c.Key("more:" + title)).Gap(2).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(more + " " + title)
			if l.Clicked() {
				clicked = true
			}
			l.Children(func() {
				ui.Text(c, more+" ›").FontSize(12).TextColor(t.Accent)
			})
		}
	})
	return clicked
}

// relTime says how long ago a time was.
func relTime(now, at time.Time) string {
	d := now.Sub(at)
	switch {
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟前", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d 小时前", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%d 天前", int(d.Hours()/24))
	}
	return at.Format("01-02")
}

// kindColor and kindTile are a clip's, or a map's, in the cards.
func kindColor(kind string) ui.Color {
	switch kind {
	case kindMap:
		return tileBlue
	case library.KindLink:
		return tileGreen
	case library.KindImage:
		return tilePink
	}
	return tileOrange
}

func kindTile(kind string) icon {
	if kind == kindMap {
		return iconMap
	}
	return kindIcon(kind)
}

// homeView is 概览: what the material holds, how it grows, and what
// changed last.
func (a *app) homeView(c *ui.Context) {
	t := c.Theme()
	now := a.now()
	maps := a.mapClips()
	clips := a.lib.Clips
	ui.Column(c.Key("home")).Grow(1).Label("概览").Children(func() {
		ui.Row(c.Key("home-head")).Height(toolbarHeight).Padding(0, 10, 0, 20).Gap(10).AlignItems(ui.Center).Shrink(0).
			DragWindow().Children(func() {
			ui.Column(c).Gap(1).Children(func() {
				ui.Text(c, "概览").Bold().FontSize(textTitle)
				ui.Text(c, fmt.Sprintf("%d 张导图 · %d 条文档和链接", len(maps), len(clips))).FontSize(11).TextColor(t.TextMuted)
			})
			ui.Spacer(c)
			if iconTextButton(c, iconSparkle, "AI", "AI 助手", "问问导图和资料里的内容", a.aiOpen).Clicked() {
				a.aiOpen = !a.aiOpen
			}
			if iconButton(c, iconClose, "关闭概览", "回到导图", 30, true).Clicked() {
				a.libOpen = false
			}
		})
		ui.Scroll(c.Key("home-scroll")).Grow(1).Padding(8, 24, 28).Gap(16).Children(func() {
			a.homeStats(c, now, maps, clips)
			a.homeActivity(c, now, maps, clips)
			ui.Row(c).Gap(16).AlignItems(ui.Start).Children(func() {
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() { a.homeCategories(c, maps, clips) })
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() { a.homeRecent(c, now, maps, clips) })
			})
		})
	})
}

// openList shows the list of 资料, of a kind.
func (a *app) openList(kind int, cat string) {
	a.showLibrary()
	a.libKind, a.libCat, a.libTag, a.libQuery = kind, cat, "", ""
}

// homeStats are the cards of large numbers.
func (a *app) homeStats(c *ui.Context, now time.Time, maps, clips []*library.Clip) {
	t := c.Theme()
	docs, links, recent, waiting := 0, 0, 0, len(a.lib.Unorganized())
	for _, cl := range clips {
		if cl.Kind == library.KindLink {
			links++
		} else {
			docs++
		}
		if now.Sub(cl.Updated) < 7*24*time.Hour {
			recent++
		}
	}
	analyzed := 0
	for _, m := range maps {
		if m.Desc != "" || m.Summary != "" {
			analyzed++
		} else {
			waiting++
		}
		if now.Sub(m.Updated) < 7*24*time.Hour {
			recent++
		}
	}
	stat := func(ic icon, col ui.Color, label string, n int, unit, sub string, open func()) {
		box := pageCard(c).Grow(1).Basis(0).MinWidth(0).Gap(6).Cursor(ui.CursorPointer).Label(label)
		if box.Hovered() {
			box.Background(t.Text.Alpha(0.06))
		}
		if box.Clicked() {
			a.later(open)
		}
		box.Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				tile(c, ic, col, 22)
				ui.Text(c, label).FontSize(13).TextColor(t.TextMuted)
			})
			ui.Row(c).Gap(4).AlignItems(ui.End).Children(func() {
				ui.Text(c, fmt.Sprint(n)).FontSize(34).Bold()
				ui.Text(c, unit).FontSize(14).TextColor(t.TextMuted).Margin(0, 0, 6)
			})
			ui.Text(c, sub).FontSize(12).TextColor(t.TextMuted).SingleLine()
		})
	}
	ui.Row(c.Key("home-stats")).Gap(16).Children(func() {
		stat(iconMap, tileBlue, "导图", len(maps), "张", fmt.Sprintf("AI 已分析 %d 张", analyzed), func() { a.openList(kindMaps, "") })
		stat(iconFile, tileOrange, "文档", docs, "份", fmt.Sprintf("另有链接 %d 条", links), func() { a.openList(kindDocs, "") })
		stat(iconPencil, tileGreen, "近 7 天更新", recent, "条", "导图和资料", func() { a.openList(kindAll, "") })
		stat(iconSparkle, tilePurple, "待 AI 整理", waiting, "条", "资料未整理、导图未分析", func() {
			if k := len(a.lib.Unorganized()); k > 0 && providerKey(a.cfg) != "" {
				a.openList(kindAll, "-")
				a.organizeAll()
				return
			}
			a.openList(kindAll, "")
		})
	})
}

// homeDays is how many days the activity shows.
const homeDays = 30

// homeActivity is the dots of the last days: per day, the maps changed
// and the clips kept, stacked, as Pulse's charts.
func (a *app) homeActivity(c *ui.Context, now time.Time, maps, clips []*library.Clip) {
	t := c.Theme()
	day := func(at time.Time) int {
		y1, m1, d1 := now.Date()
		y2, m2, d2 := at.In(now.Location()).Date()
		a := time.Date(y1, m1, d1, 0, 0, 0, 0, time.UTC)
		b := time.Date(y2, m2, d2, 0, 0, 0, 0, time.UTC)
		return int(a.Sub(b).Hours() / 24)
	}
	var mapsOn, clipsOn [homeDays]int
	for _, m := range maps {
		if d := day(m.Updated); d >= 0 && d < homeDays {
			mapsOn[homeDays-1-d]++
		}
	}
	for _, cl := range clips {
		if d := day(cl.Created); d >= 0 && d < homeDays {
			clipsOn[homeDays-1-d]++
		}
	}
	most := 1
	for i := range homeDays {
		most = max(most, mapsOn[i]+clipsOn[i])
	}
	const rows = 8
	per := int(math.Ceil(float64(most) / rows)) // what a dot counts
	pageCard(c).Key("home-activity").Children(func() {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Grow(1).Children(func() { cardHead(c, iconTarget, tileOrange, "最近 30 天", "") })
			legend := func(col ui.Color, text string) {
				ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(8, 8).Radius(4).Background(col)
					ui.Text(c, text).FontSize(12).TextColor(t.TextMuted)
				})
			}
			legend(tileBlue, "导图编辑")
			legend(tileOrange, "收藏资料")
			if per > 1 {
				ui.Text(c, fmt.Sprintf("每点 %d 条", per)).FontSize(12).TextColor(t.TextMuted)
			}
		})
		ui.Box(c).Height(130).FillWidth().Radius(10).Background(tileOrange.Alpha(0.06)).
			Draw(func(p *ui.Painter, r ui.Rect) {
				padX, padY := float32(14), float32(12)
				w, h := r.W-2*padX, r.H-2*padY
				colW, rowH := w/homeDays, h/rows
				rad := min(colW, rowH) * 0.28
				for i := range homeDays {
					nm := int(math.Ceil(float64(mapsOn[i]) / float64(per)))
					nc := int(math.Ceil(float64(clipsOn[i]) / float64(per)))
					for j := range rows {
						x := r.X + padX + colW*(float32(i)+0.5)
						y := r.Y + padY + h - rowH*(float32(j)+0.5)
						col := tileOrange.Alpha(0.16)
						switch {
						case j < nm:
							col = tileBlue
						case j < nm+nc:
							col = tileOrange
						}
						p.FillPath(new(ui.Path).Circle(x, y, rad), col)
					}
				}
			})
		ui.Row(c).Children(func() {
			ui.Text(c, "30 天前").FontSize(11).TextColor(t.TextMuted)
			ui.Spacer(c)
			ui.Text(c, "15 天前").FontSize(11).TextColor(t.TextMuted)
			ui.Spacer(c)
			ui.Text(c, "今天").FontSize(11).TextColor(t.TextMuted)
		})
	})
}

// homeCategories is how the material is spread over the categories.
func (a *app) homeCategories(c *ui.Context, maps, clips []*library.Clip) {
	t := c.Theme()
	cats := library.CategoriesOf(slices.Concat(clips, maps))
	total := len(clips) + len(maps)
	pageCard(c).Key("home-cats").Children(func() {
		cardHead(c, iconCategory, tilePurple, "分类分布", "")
		if len(cats) == 0 {
			ui.Text(c, "AI 整理资料、分析导图后，会按分类统计在这里。").FontSize(12).TextColor(t.TextMuted).Wrap()
			return
		}
		for i, k := range cats {
			if i == 8 {
				break
			}
			name, col := k.Name, catColor(k.Name)
			frac := float32(k.N) / float32(max(total, 1))
			row := ui.Row(c.Key("cat:"+name)).Gap(10).Padding(4, 6).Radius(8).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label("分类 " + name)
			if row.Hovered() {
				row.Background(t.Text.Alpha(0.05))
			}
			if row.Clicked() {
				a.later(func() { a.openList(kindAll, name) })
			}
			row.Children(func() {
				ui.Box(c).Size(8, 8).Radius(4).Background(col)
				ui.Text(c, name).FontSize(13).Width(64).SingleLine()
				ui.Box(c).Height(8).Grow(1).Draw(func(p *ui.Painter, r ui.Rect) {
					p.FillPath(roundedRect(r.X, r.Y, r.W, r.H, 4), col.Alpha(0.15))
					p.FillPath(roundedRect(r.X, r.Y, max(r.W*frac, 8), r.H, 4), col)
				})
				ui.Text(c, fmt.Sprint(k.N)).FontSize(13).Bold().Width(28).TextAlign(ui.End)
			})
		}
	})
}

// homeRecent is what changed last, maps and clips together.
func (a *app) homeRecent(c *ui.Context, now time.Time, maps, clips []*library.Clip) {
	t := c.Theme()
	all := slices.Concat(maps, clips)
	slices.SortStableFunc(all, func(x, y *library.Clip) int { return y.Updated.Compare(x.Updated) })
	pageCard(c).Key("home-recent").Children(func() {
		if cardHead(c, iconInbox, tileTeal, "最近更新", "显示全部") {
			a.later(func() { a.openList(kindAll, "") })
		}
		for i, it := range all {
			if i == 8 {
				break
			}
			it := it
			row := ui.Row(c.Key("recent:"+it.ID)).Gap(10).Padding(5, 6).Radius(8).AlignItems(ui.Center).Cursor(ui.CursorPointer).
				Label("最近 " + it.Label())
			if row.Hovered() {
				row.Background(t.Text.Alpha(0.05))
			}
			if row.Clicked() {
				a.later(func() {
					if it.Kind == kindMap {
						a.libOpen = false
						a.openMap(mapPathOf(it))
						return
					}
					a.openList(kindAll, "")
					a.libSel = it.ID
				})
			}
			row.Children(func() {
				tile(c, kindTile(it.Kind), kindColor(it.Kind), 22)
				ui.Text(c, cmpOr(it.Title, it.Label())).FontSize(13).Grow(1).Shrink(1).MinWidth(0).SingleLine()
				ui.Text(c, relTime(now, it.Updated)).FontSize(12).TextColor(t.TextMuted).Shrink(0)
			})
		}
		if len(all) == 0 {
			ui.Text(c, "还没有导图和资料。").FontSize(12).TextColor(t.TextMuted)
		}
	})
}
