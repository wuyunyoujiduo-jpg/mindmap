package main

import (
	"fmt"
	"hash/fnv"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
)

// The look of the sidebar and of 概览 follows Pulse's: rows of the
// sidebar with a line icon in a quiet shade of its color and a count on
// the right, the current one on a light gray; pages of outlined cards with
// large numbers. Colors are muted, as the rest is.

// Colors of the kinds of things, muted.
var (
	tileBlue   = ui.Hex("#5B83C9")
	tileOrange = ui.Hex("#D08A55")
	tileGreen  = ui.Hex("#5FA37E")
	tilePurple = ui.Hex("#8C79C9")
	tilePink   = ui.Hex("#C77796")
	tileYellow = ui.Hex("#C9A443")
	tileTeal   = ui.Hex("#4E9E9A")
	tileRed    = ui.Hex("#C9655F")
)

var tilePalette = []ui.Color{tileBlue, tileOrange, tileGreen, tilePurple, tilePink, tileTeal, tileYellow, tileRed}

// catColor is a category's color, the same each time.
func catColor(name string) ui.Color {
	h := fnv.New32a()
	h.Write([]byte(name))
	return tilePalette[h.Sum32()%uint32(len(tilePalette))]
}

// tile is an icon of a kind of thing, drawn as a line in a quiet shade of
// its color: a hint of the color, no block of it.
func tile(c *ui.Context, ic icon, col ui.Color, size float32) {
	t := c.Theme()
	ink := col.MixOklab(t.TextMuted, 0.4)
	ui.Box(c).Size(size, size).Shrink(0).PassThrough().
		Draw(func(p *ui.Painter, r ui.Rect) { drawIcon(p, r, size*0.72, ic, ink) })
}

// navRow is a row of the sidebar: a tile, a label and a count. The
// current one is filled with the accent.
func navRow(c *ui.Context, ic icon, _ ui.Color, label, count string, current bool) ui.Element {
	t := c.Theme()
	r := ui.Row(c).Height(30).Padding(0, 10).Gap(10).Radius(8).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(label).FillWidth()
	// Selected as the other rows of the sidebar are: a light accent.
	switch {
	case current:
		r.Background(chosen(t))
	case r.Hovered():
		r.Background(t.Text.Alpha(0.06))
	}
	r.Children(func() {
		ink := t.TextMuted // quiet: gray lines, the ink of the text on the row shown
		if current {
			ink = t.Text
		}
		glyph(c, ic, 16, ink)
		text := ui.Text(c, label).FontSize(13).Grow(1).Shrink(1).MinWidth(0).SingleLine()
		if current {
			text.Bold()
		}
		if count != "" {
			ui.Text(c, count).FontSize(12).TextColor(t.TextMuted).Shrink(0)
		}
	})
	return r
}

// pageCard is a card of a page: soft, much rounded.
func pageCard(c *ui.Context) ui.Element {
	t := c.Theme()
	return ui.Column(c).Padding(16, 18).Gap(12).Radius(12).Background(t.Background).Border(1, hairline(t)).FillWidth()
}

// cardHead is the head of a card: a tile, a title in its color, and a link
// on the right when more is not "".
func cardHead(c *ui.Context, ic icon, col ui.Color, title, more string) bool {
	t := c.Theme()
	clicked := false
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		tile(c, ic, col, 26)
		ui.Text(c, title).FontSize(15).Bold().Grow(1) // the tile says the color
		if more != "" {
			l := ui.Row(c.Key("more:" + title)).Gap(2).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(more + " " + title)
			if l.Clicked() {
				clicked = true
			}
			l.Children(func() {
				ui.Text(c, more).FontSize(12).TextColor(t.Accent)
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

// openList shows the list of 资料, of a kind.
func (a *app) openList(kind int, cat string) {
	a.showLibrary()
	a.libKind, a.libCat, a.libTag, a.libQuery, a.libDay = kind, cat, "", "", time.Time{}
}

// sectionCard is a card of a detail or a panel: a head of a tile and a
// title in its color, with what right builds at its end, then body.
func sectionCard(c *ui.Context, ic icon, col ui.Color, title string, right func(), body func()) {
	section(c, ic, title, right, body)
}

// dotRow is a row of the sidebar with a small dot of a color, as Finder's
// tags: for the categories, many, which tiles would make heavy.
func dotRow(c *ui.Context, col ui.Color, label, count string, current bool) ui.Element {
	t := c.Theme()
	r := ui.Row(c).Padding(5, 8, 5, 14).Gap(10).Radius(8).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(label).FillWidth()
	switch {
	case current:
		r.Background(chosen(t))
	case r.Hovered():
		r.Background(t.Text.Alpha(0.06))
	}
	r.Children(func() {
		ui.Box(c).Size(8, 8).Radius(4).Background(col).Shrink(0).Margin(0, 5, 0, 1)
		text := ui.Text(c, label).FontSize(13).Grow(1).Shrink(1).MinWidth(0).SingleLine()
		if current {
			text.Bold()
		}
		if count != "" {
			ui.Text(c, count).FontSize(12).TextColor(t.TextMuted).Shrink(0)
		}
	})
	return r
}

// midEllipsis cuts a name too long for width points in its middle,
// keeping its end, where names that begin alike differ: "资料 6：关于…记录".
func midEllipsis(s string, width float32) string {
	r := []rune(s)
	w := func(r rune) float32 {
		if r < 0x2E80 {
			return 0.55
		}
		return 1
	}
	total := float32(0)
	for _, x := range r {
		total += w(x)
	}
	room := width / 13 // ems of 13 pt
	if total <= room || room < 4 {
		return s
	}
	keep := room - 1 // for the ellipsis
	var head, tail []rune
	hw, tw := float32(0), float32(0)
	i, j := 0, len(r)-1
	for i <= j {
		if hw <= tw {
			if hw+w(r[i]) > keep*0.55 {
				break
			}
			hw += w(r[i])
			head = append(head, r[i])
			i++
		} else {
			if tw+w(r[j]) > keep*0.45 {
				break
			}
			tw += w(r[j])
			tail = append([]rune{r[j]}, tail...)
			j--
		}
	}
	return string(head) + "…" + string(tail)
}

// dayOf is the day a clip counts on in 概览's dots: a map's last change,
// a clip's keeping.
func dayOf(c *library.Clip) time.Time {
	if c.Kind == kindMap {
		return c.Updated
	}
	return c.Created
}
