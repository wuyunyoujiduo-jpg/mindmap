package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// The app's look, in a few pieces used everywhere. It is quiet as Apple's
// pages are (the reference is library.aimran.es): a cool pale ground; what
// is read and worked on in white cards with large round corners, lifted by
// a shadow so soft that it is hardly seen; keys, tabs and the fields of
// search are pills; what is chosen among a few is a white pill that slides
// to its place (slidePill); things give under the pointer and settle along
// one curve (easeDetent). The accent is kept for what is chosen, for the
// key that acts and for the AI.

// Corners: a card's, a row's in a card, and a small thing's.
const (
	radiusCard  = 16
	radiusRow   = 10
	radiusSmall = 8
	radiusPill  = 999
)

// keyHeight is the height of a key of words.
const keyHeight = 32

// Sizes of text.
const (
	textCaption  = 12
	textSmall    = 13
	textBody     = 14
	textTitle    = 16
	textHeadline = 20
	textPage     = 22 // a page's name
)

// toolbarHeight is the toolbar's, which is the window's title bar.
const toolbarHeight = 52

// pageWidth is the most a page takes, in the middle of the room it has:
// the desk, the tasks and 收集 are columns of this width, so that their
// headings and what is under them start at the same place.
const pageWidth = 1080

// pageColumn is a page's column, in the middle of the room it has; grow
// has it take the height left.
func pageColumn(c *ui.Context, key string, grow bool) ui.Element {
	col := ui.Column(c.Key(key)).FillWidth().MaxWidth(pageWidth+56).Margin(0, ui.Auto).Padding(0, 28)
	if grow {
		return col.Grow(1).Basis(0).MinHeight(0)
	}
	return col.Shrink(0)
}

// pageHeadHeight is the height of a page's heading: more than the
// toolbar's, for its name stands under the window's edge with room over it.
const pageHeadHeight = 56

// pageHeader is a page's heading, the same on every page: its name, a
// word on it, and at the end what right builds. It is the window's title
// bar there, and drags the window.
func pageHeader(c *ui.Context, key, title, sub string, right func()) {
	t := c.Theme()
	ui.Row(c.Key(key)).Height(pageHeadHeight).Gap(12).AlignItems(ui.Center).Shrink(0).FillWidth().DragWindow().Children(func() {
		ui.Text(c, title).FontSize(17).FontWeight(600).NoWrap()
		if sub != "" {
			ui.Text(c, sub).FontSize(textBody).TextColor(t.TextMuted).SingleLine().Shrink(1).MinWidth(0)
		}
		ui.Spacer(c)
		if right != nil {
			right()
		}
	})
}

// captureBox is the box of a line to write something down in, the same
// on the desk, the tasks and 收集: a white card with a thin line, a plus
// before what the caller builds. The caller outlines it in the accent
// while its field has the focus (captureFocus).
func captureBox(c *ui.Context, key string) ui.Element {
	t := c.Theme()
	return soft(ui.Row(c.Key(key)).MinHeight(52).Padding(5, 8, 5, 18).Gap(10).Radius(radiusCard).AlignItems(ui.Center).FillWidth().Shrink(0).
		Background(cardColor(t)).Border(1.5, t.Text.Alpha(0)), t).Transition(easeLook)
}

// captureFocus outlines a captureBox whose field has the focus.
func captureFocus(c *ui.Context, box, field ui.Element) {
	if field.Focused() {
		focusHalo(box, c.Theme()) // the one thing outlined in the accent
	}
}

// focusHalo outlines what is written in: a line in the accent, and a pale
// halo of it around, as the system's fields have.
func focusHalo(el ui.Element, t *ui.Theme) ui.Element {
	return el.Border(1.5, t.Accent).NoShadow().Shadow(0, 0, 0, 4, t.Accent.Alpha(0.14))
}

// soft is the shadow a card rests on: one close under it, one wide and
// faint. It is hardly seen, and tells the card from the ground.
func soft(el ui.Element, t *ui.Theme) ui.Element {
	if t.Dark {
		return el.Shadow(0, 0, 0, 1, ui.RGBA(255, 255, 255, 0.06))
	}
	return el.Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.03)).Shadow(0, 6, 24, 0, ui.RGBA(0, 0, 0, 0.035))
}

// lifted is the shadow of what is over the page: a card under the pointer,
// a bar on the canvas, a popup.
func lifted(el ui.Element, t *ui.Theme) ui.Element {
	if t.Dark {
		return el.Shadow(0, 0, 0, 1, ui.RGBA(255, 255, 255, 0.08)).Shadow(0, 12, 32, -8, ui.RGBA(0, 0, 0, 0.6))
	}
	return el.Shadow(0, 4, 12, 0, ui.RGBA(0, 0, 0, 0.04)).Shadow(0, 24, 60, 0, ui.RGBA(0, 0, 0, 0.1))
}

// hoverLift has a card that opens rise a little under the pointer, its
// shadow growing with it, and give when pressed.
func hoverLift(el ui.Element, t *ui.Theme) ui.Element {
	p := ui.Pose{Scale: 1}
	switch {
	case el.Pressed():
		p.Scale = 0.985
	case el.Hovered():
		p.Y = -2
		lifted(el.NoShadow(), t)
	}
	return el.AnimatePose(p, ui.Animation{Duration: 260 * time.Millisecond, Ease: easeDetent})
}

// A pillMatch is the identity of the white pill of a group of tabs: the
// one of the tab chosen comes from where the last one was.
type pillMatch struct{ group string }

// slidePill is the white pill under the part chosen of a group (a
// segmented control, a row of tabs, the sidebar): built first in that
// part, it fills it, and slides there from the part chosen before.
func slidePill(c *ui.Context, group string, radius float32) {
	t := c.Theme()
	pill := ui.Box(c.Key("pill")).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(radius).PassThrough().ZIndex(-1).
		Background(pillColor(t)).
		Transition(ui.ElementTransition{Duration: 320 * time.Millisecond, Ease: easeDetent, Match: pillMatch{group}})
	if t.Dark {
		pill.Shadow(0, 0, 0, 1, ui.RGBA(255, 255, 255, 0.06))
	} else {
		pill.Shadow(0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.1)).Shadow(0, 0, 0, 0.5, ui.RGBA(0, 0, 0, 0.05))
	}
}

// pillColor is the fill of the pill chosen: white, or, dark, a gray over
// its track.
func pillColor(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.Hex("#3A3A3C")
	}
	return ui.Hex("#FFFFFF")
}

// ground is the window's own color, which the rail and the maps' list
// stand on and the leaf lies on.
func ground(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.Hex("#0E0E10")
	}
	return ui.Hex(groundNow().ground)
}

// A groundLook is a color the window's ground may take, in the light
// theme, with the leaf and the trays that go with it. The ink's has the
// rail dark, its words light.
type groundLook struct {
	key, name          string
	ground, tray, leaf string
	dark               bool // the rail's words are light on it
}

// grounds are the grounds to choose from in the settings; the first, the
// leaf's own white, is the one taken until another is chosen.
var grounds = []groundLook{
	{"", "浅灰", "#F5F5F7", "#EBEBEE", "#FFFFFF", false},
	{"violet", "淡紫", "#F4F3FB", "#E9E8F5", "#FFFFFF", false},
	{"blue", "淡蓝", "#F1F5FA", "#E6EBF2", "#FFFFFF", false},
	{"cream", "米色", "#F7F4EE", "#EFE9DF", "#FFFFFF", false},
}

// groundKey is the ground chosen (config.Ground), set as each frame begins.
var groundKey string

func groundNow() groundLook {
	for _, g := range grounds {
		if g.key == groundKey {
			return g
		}
	}
	return grounds[0]
}

// railInks are the rail's colors on the ground: its words chosen and not,
// and the shades of the part shown and of the one under the pointer.
func railInks(t *ui.Theme) (ink, muted, on, hover ui.Color) {
	return t.Text, t.TextMuted, t.Accent, t.Text.Alpha(0.045)
}

// sideGround is what the maps' list stands on: the ground, or, where the
// ground is dark under a light page, the trays' shade, its words being
// the page's.
func sideGround(t *ui.Theme) ui.Color {
	if g := groundNow(); !t.Dark && g.dark {
		return ui.Hex(g.tray)
	}
	return ground(t)
}

// cardEdge is the thin ring a card is told by.
func cardEdge(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.RGBA(255, 255, 255, 0.09)
	}
	return ui.RGBA(0, 0, 0, 0.08)
}

// shadowColor is the shadow under a card: hardly there, the ring tells it.
var shadowColor = ui.RGBA(0, 0, 0, 0.05)

// raise gives an element a card's look: white, on a soft shadow; its
// color and its shadow go smoothly where they change.
func raise(el ui.Element, t *ui.Theme) ui.Element {
	return soft(el.Background(cardColor(t)), t).Transition(easeLook)
}

// easeLook has an element's colors and shadows change smoothly, and no
// more: what is under the pointer lights up rather than switches.
var easeLook = ui.ElementTransition{Duration: 200 * time.Millisecond, Ease: ui.CubicBezier(0.25, 0.1, 0.25, 1), Colors: true, Shadows: true}

// easeDetent is the curve things move along: fast at first, settling long.
var easeDetent = ui.CubicBezier(0.22, 1, 0.36, 1)

// rowMotion is how a row of a list comes, goes and makes room: it fades
// in from a little above, the others move to their places, and one gone
// fades out where it was.
var rowMotion = ui.ElementTransition{
	Duration: 320 * time.Millisecond, Ease: easeDetent, Position: true, Colors: true,
	Enter: &ui.Motion{Y: -8, Scale: 0.98}, Exit: &ui.Motion{Scale: 0.97}, ExitDuration: 180 * time.Millisecond,
}

// press has an element give a little under the pointer, as a key does.
func press(el ui.Element, by float32) ui.Element {
	p := ui.Pose{Scale: 1}
	if el.Pressed() {
		p.Scale = by
	}
	return el.AnimatePose(p, ui.Animation{Duration: 220 * time.Millisecond, Ease: easeDetent})
}

// chosenCard marks the card chosen: its ring in the accent, a soft halo
// around it; its size is as it was.
func chosenCard(el ui.Element, t *ui.Theme) ui.Element {
	return el.NoShadow().Shadow(0, 0, 0, 1.5, t.Accent).Shadow(0, 0, 0, 5, t.Accent.Alpha(0.12))
}

// tray is a part of a page: a soft ground with its heading on it
// (trayHead) and what it holds in a white card (listCard), as the columns
// of the board.
func tray(c *ui.Context, key string) ui.Element {
	return ui.Column(c.Key(key)).FillWidth()
}

// trayHead is a tray's heading: its title, a word on it, and at its end
// what right builds. It is as high on every tray, so that trays side by
// side start level.
func trayHead(c *ui.Context, key, title, sub string, right func()) ui.Element {
	t := c.Theme()
	return ui.Row(c.Key(key)).Height(30).Padding(0, 4, 6).Gap(8).AlignItems(ui.Center).FillWidth().Shrink(0).Children(func() {
		ui.Text(c, title).FontSize(textSmall).FontWeight(600).TextColor(t.Text.MixOklab(t.TextMuted, 0.45)).NoWrap()
		if sub != "" {
			ui.Text(c, sub).FontSize(textSmall).TextColor(t.TextMuted).Shrink(1).MinWidth(0).SingleLine()
		}
		ui.Spacer(c)
		if right != nil {
			right()
		}
	})
}

// listCard is a white card of rows (listCardRow): the lists of a tray.
func listCard(c *ui.Context, key string) ui.Element {
	return raise(ui.Column(c.Key(key)).FillWidth().Padding(6).Gap(1).Radius(radiusCard), c.Theme())
}

// listCardRow is a row of a listCard: no lines between rows, each lit
// under the pointer.
func listCardRow(c *ui.Context, key string) ui.Element {
	t := c.Theme()
	row := ui.Row(c.Key(key)).MinHeight(48).Padding(8, 12).Gap(12).Radius(radiusRow).AlignItems(ui.Center).FillWidth().
		Background(t.Text.Alpha(0)).Transition(rowMotion)
	switch {
	case row.Pressed():
		row.Background(t.Text.Alpha(0.07))
	case row.Hovered():
		row.Background(t.Text.Alpha(0.04))
	}
	return press(row, 0.985)
}

// textLink is a few words in the accent that go somewhere: no box, so
// that it ends where the part it heads ends.
func textLink(c *ui.Context, text, label string) ui.Element {
	t := c.Theme()
	l := ui.Box(c.Key("link:"+label)).Padding(0, 4).Cursor(ui.CursorPointer).Label(label).Shrink(0)
	col := t.TextMuted
	if l.Hovered() {
		col = t.Accent
	}
	l.Children(func() { ui.Text(c, text).FontSize(textSmall).TextColor(col).PassThrough() })
	return l
}

// key gives a button the look of a key: a white face with a thin ring,
// a shade under the pointer.
func key(b ui.Element, t *ui.Theme) ui.Element {
	// No ring: a shade of the ground, a little more under the pointer.
	switch {
	case b.Pressed():
		b.Background(t.Text.Alpha(0.11))
	case b.Hovered():
		b.Background(t.Text.Alpha(0.08))
	default:
		b.Background(t.Text.Alpha(0.05))
	}
	return press(b.Transition(easeLook), 0.97)
}

// dropButton is a key that opens a menu (the caller's Menu): its text
// and a mark that it opens.
func dropButton(c *ui.Context, k, text, label, tip string) ui.Element {
	t := c.Theme()
	b := key(ui.Row(c.Key(k)).Height(keyHeight).Padding(0, 11, 0, 14).Gap(5).Radius(radiusPill).AlignItems(ui.Center).Cursor(ui.CursorPointer).
		Label(label).Tooltip(tip).Shrink(0), t)
	b.Children(func() {
		ui.Text(c, text).FontSize(textBody).MaxWidth(160).SingleLine().PassThrough()
		ui.Box(c).Size(10, 10).Shrink(0).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) {
			cx, cy := r.X+r.W/2, r.Y+r.H/2+0.5
			p.StrokePath(new(ui.Path).MoveTo(cx-3.5, cy-1.75).LineTo(cx, cy+1.75).LineTo(cx+3.5, cy-1.75), 1.3, t.TextMuted)
		})
	})
	return b
}

// hairline is the color of the thin lines between parts.
func hairline(t *ui.Theme) ui.Color { return t.Border.Alpha(0.75) }

// sectionHeader is the small heading of a part of a panel.
func sectionHeader(c *ui.Context, text string) ui.Element {
	return ui.Text(c, text).FontSize(textCaption).Bold().TextColor(c.Theme().TextMuted)
}

// inlineInput is a line of text edited in place: no box until hovered or
// focused, as titles in Notes and Things.
func inlineInput(c *ui.Context, value *string, size float32, bold bool, placeholder, label string) ui.Element {
	t := c.Theme()
	in := ui.TextInputBase(c, value).FontSize(size).Placeholder(placeholder).Label(label).
		Padding(4, 6).Radius(radiusSmall).FillWidth()
	if bold {
		in.Bold()
	}
	switch {
	case in.Focused():
		in.Background(t.Background).Border(1, t.Accent.Alpha(0.6))
	case in.Hovered():
		in.Background(t.Text.Alpha(0.05))
	}
	return in
}

// inlineArea is inlineInput's many-line kind.
func inlineArea(c *ui.Context, value *string, size float32, minLines, maxLines int, placeholder, label string) ui.Element {
	t := c.Theme()
	in := ui.TextAreaBase(c, value).FontSize(size).Lines(minLines, maxLines).Placeholder(placeholder).Label(label).
		Padding(8, 10).Radius(radiusRow).FillWidth().LineHeight(1.5)
	switch {
	case in.Focused():
		in.Background(t.Background).Border(1, t.Accent.Alpha(0.6))
	case in.Hovered():
		in.Background(t.Text.Alpha(0.05))
	}
	return in
}

// propRow is a property of an inspector: its name on the left, its value.
func propRow(c *ui.Context, name string, fn func()) {
	t := c.Theme()
	// The name stands by the value's first line, should the value wrap.
	ui.Row(c.Key("prop:" + name)).Gap(8).AlignItems(ui.Start).FillWidth().Children(func() {
		ui.Row(c).Width(44).Height(30).Shrink(0).AlignItems(ui.Center).Children(func() {
			ui.Text(c, name).FontSize(textSmall).TextColor(t.TextMuted)
		})
		ui.Row(c).Grow(1).Shrink(1).MinWidth(0).MinHeight(30).Gap(6).Wrap().GapY(6).AlignItems(ui.Center).Children(fn)
	})
}

// segmented is a choice among a few, all of them shown: as tabGroup, but
// as wide as the room it has, its parts equal; label names the choice
// before each part's name. It gives the part clicked, else the one chosen.
func segmented(c *ui.Context, key, label string, names []string, chosenPart int) int {
	t := c.Theme()
	out := chosenPart
	ui.Row(c.Key(key)).Padding(2).Gap(2).Radius(radiusRow).Background(t.Text.Alpha(0.05)).Grow(1).Basis(0).MinWidth(0).Children(func() {
		for i, name := range names {
			on := i == chosenPart
			b := ui.Box(c.Key("part:" + name)).Height(28).Radius(radiusSmall).Center().Grow(1).Basis(0).MinWidth(0).Cursor(ui.CursorPointer).
				Label(label + " " + name).Background(t.Text.Alpha(0)).Transition(easeLook)
			if !on && b.Hovered() {
				b.Background(t.Text.Alpha(0.05))
			}
			if b.Clicked() {
				out = i
			}
			b.Children(func() {
				if on {
					slidePill(c, key, radiusSmall)
					b.ZIndex(-1) // under the others, which the pill slides behind
				}
				txt := ui.Text(c, name).FontSize(textSmall).NoWrap().PassThrough()
				if on {
					txt.FontWeight(600)
				} else {
					txt.TextColor(t.Text.MixOklab(t.TextMuted, 0.6))
				}
			})
		}
	})
	return out
}

// chipButton is a small round button of a word, as a day to choose.
func chipButton(c *ui.Context, text, label, tip string) ui.Element {
	t := c.Theme()
	b := ui.Box(c.Key("chip:"+label)).Height(28).Padding(0, 12).Radius(radiusPill).Center().Cursor(ui.CursorPointer).
		Label(label).Tooltip(tip).Shrink(0).Transition(easeLook)
	press(b, 0.96)
	switch {
	case b.Pressed():
		b.Background(t.Text.Alpha(0.12))
	case b.Hovered():
		b.Background(t.Text.Alpha(0.09))
	default:
		b.Background(t.Text.Alpha(0.05))
	}
	b.Children(func() { ui.Text(c, text).FontSize(textSmall).NoWrap().PassThrough() })
	return b
}

// dateProp is the row of a day that may not be set: the day to change
// and to clear, or, with none, a few days near to choose from.
func dateProp(c *ui.Context, name, label string, date *time.Time, now time.Time, set func(*time.Time)) {
	propRow(c, name, func() {
		if date == nil {
			day := today(now)
			for _, q := range []struct {
				label string
				d     time.Time
			}{{"今天", day}, {"明天", day.AddDate(0, 0, 1)}, {"下周一", weekStart(day).AddDate(0, 0, 7)}} {
				if chipButton(c, q.label, label+" "+q.label, label+"定在"+q.label+"，之后可以再改").Clicked() {
					d := q.d
					set(&d)
				}
			}
			return
		}
		d := *date
		if ui.DateInput(c.Key("date:"+label), &d).Label(label).FontSize(textSmall).Changed() {
			d = today(d)
			set(&d)
		}
		if iconButton(c, iconClose, "清除"+label, "清除"+label, 24, true).Clicked() {
			set(nil)
		}
	})
}

// card is a soft block of a panel, tinted.
func card(c *ui.Context, tint ui.Color) ui.Element {
	return ui.Column(c).Padding(14, 16).Gap(8).Radius(14).Background(tint).FillWidth()
}

// ghostButton is a button of text with no box until hovered.
func ghostButton(c *ui.Context, text, tip string) ui.Element {
	t := c.Theme()
	b := ui.Box(c.Key("ghost:"+text)).Padding(5, 10).Radius(radiusPill).Cursor(ui.CursorPointer).Label(text).Tooltip(tip).Shrink(0).
		Background(t.Text.Alpha(0)).Transition(easeLook)
	press(b, 0.96)
	switch {
	case b.Pressed():
		b.Background(t.Text.Alpha(0.1))
	case b.Hovered():
		b.Background(t.Text.Alpha(0.06))
	}
	b.Children(func() { ui.Text(c, text).FontSize(textSmall).TextColor(t.Accent).PassThrough() })
	return b
}

// searchBox is a field of search as in a toolbar: a rounded, tinted box
// with a magnifier, outlined while focused.
func searchBox(c *ui.Context, key string, value *string, placeholder, label, tip string, width float32) ui.Element {
	t := c.Theme()
	box := ui.Row(c.Key(key+"-box")).Height(keyHeight).Width(width).Shrink(1).MinWidth(80).Padding(0, 12).Gap(7).Radius(radiusPill).
		AlignItems(ui.Center).Background(t.Text.Alpha(0.05)).Border(1.5, t.Text.Alpha(0)).Transition(easeLook)
	var in ui.Element
	box.Children(func() {
		ui.Box(c).Size(14, 14).Shrink(0).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) { drawIcon(p, r, 13, iconSearch, t.TextMuted) })
		in = ui.TextInputBase(c.Key(key), value).Placeholder(placeholder).Label(label).FontSize(textBody).Grow(1).Tooltip(tip)
		switch {
		case in.Focused():
			focusHalo(box.Background(cardColor(t)), t)
		case box.Hovered():
			box.Background(t.Text.Alpha(0.075))
		}
	})
	return in
}

// listRow is a row of a list that acts when clicked: text on the left, a
// soft highlight under the pointer, as the suggestions of Spotlight.
func listRow(c *ui.Context, ic icon, text string) ui.Element {
	t := c.Theme()
	b := ui.Row(c.Key("row:"+text)).Padding(8, 10).Gap(10).Radius(radiusRow).AlignItems(ui.Center).FillWidth().
		Cursor(ui.CursorPointer).Label(text).Background(t.Text.Alpha(0)).Transition(easeLook)
	switch {
	case b.Pressed():
		b.Background(t.Text.Alpha(0.1))
	case b.Hovered():
		b.Background(t.Text.Alpha(0.06))
	}
	b.Children(func() {
		ui.Box(c).Size(16, 16).Shrink(0).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) { drawIcon(p, r, 14, ic, t.Accent) })
		ui.Text(c, text).FontSize(textBody).Grow(1).PassThrough()
	})
	return b
}

// pill is a small rounded label, as a kind or a category.
func pill(c *ui.Context, text string, bg, fg ui.Color) ui.Element {
	return ui.Box(c.Key("pill:"+text)).Padding(2, 8).Radius(999).Background(bg).Shrink(0).Children(func() {
		ui.Text(c, text).FontSize(textCaption).TextColor(fg).NoWrap()
	})
}

// chosen is the fill of the row chosen in a list: a neutral gray, as the
// accent would shout across a quiet page.
func chosen(t *ui.Theme) ui.Color {
	if t.Dark {
		return t.Text.Alpha(0.12)
	}
	return t.Text.Alpha(0.075)
}

// section is a part of a panel or a detail: a quiet heading, an icon and a
// title in the muted text, with what right builds at its end, then body,
// under a hairline rather than in a card.
func section(c *ui.Context, ic icon, title string, right func(), body func()) {
	t := c.Theme()
	ui.Column(c.Key("section:"+title)).Padding(14, 0, 4).Gap(10).FillWidth().
		BorderWidth(1, 0, 0, 0).BorderColor(hairline(t)).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).MinHeight(20).Children(func() {
			glyph(c, ic, 13, t.TextMuted)
			ui.Text(c, title).FontSize(textSmall).FontWeight(600).TextColor(t.TextMuted).Grow(1)
			if right != nil {
				right()
			}
		})
		body()
	})
}

// accentButton is the one key of a part that acts: in the ink, its text
// in the page's color. The accent is kept for what is chosen.
func accentButton(c *ui.Context, text, tip string) ui.Element {
	t := c.Theme()
	b := ui.Box(c.Key("accent:"+text)).Height(keyHeight).Padding(0, 16).Radius(radiusPill).Center().
		Cursor(ui.CursorPointer).Label(text).Tooltip(tip).Shrink(0).Background(t.Accent)
	switch {
	case b.Pressed():
		b.Background(t.AccentPressed)
	case b.Hovered():
		b.Background(t.AccentHover)
	}
	b.Children(func() {
		ui.Text(c, text).FontSize(textSmall).FontWeight(600).TextColor(t.AccentText).NoWrap().PassThrough()
	})
	return press(b.Transition(easeLook), 0.97)
}

// inkButton is the key that acts, where the system's would be a button
// in the accent: in the ink, taking the keyboard as a button does, dimmed
// and deaf when not enabled.
func inkButton(c *ui.Context, text string, enabled bool) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c.Key("ink:"+text)).Height(keyHeight).Padding(0, 16).Radius(radiusPill).Label(text).Shrink(0).Disabled(!enabled)
	switch {
	case !enabled:
		b.Background(t.Text.Alpha(0.18))
	case b.Pressed():
		b.Background(t.AccentPressed)
	case b.Hovered():
		b.Background(t.AccentHover).Cursor(ui.CursorPointer)
	default:
		b.Background(t.Accent).Cursor(ui.CursorPointer)
	}
	b.Children(func() {
		ui.Text(c, text).FontSize(textSmall).FontWeight(600).TextColor(t.AccentText).NoWrap().PassThrough()
	})
	return press(b.Transition(easeLook), 0.97)
}

// softButton is a key of a word, for what is beside the one that acts.
func softButton(c *ui.Context, text, tip string) ui.Element {
	t := c.Theme()
	b := key(ui.Box(c.Key("soft:"+text)).Height(keyHeight).Padding(0, 14).Radius(radiusPill).Center().
		Cursor(ui.CursorPointer).Label(text).Tooltip(tip).Shrink(0), t)
	b.Children(func() { ui.Text(c, text).FontSize(textSmall).NoWrap().PassThrough() })
	return b
}

// quietTheme is the app's palette over the system's theme: a leaf near
// white, its trays and controls a shade of warm gray, white cards, a
// near-black ink, lines thin and warm; a violet-blue for what is chosen
// and for the AI, used sparingly. Color is kept for small things that
// mean something: a map's dot, a kind's pill.
func quietTheme(base *ui.Theme) *ui.Theme {
	t := *base
	if t.Dark {
		t.Background, t.Surface = ui.Hex("#1C1C1E"), ui.Hex("#2C2C2E")
		t.SurfaceHover, t.SurfacePressed = ui.Hex("#3A3A3C"), ui.Hex("#48484A")
		t.Border = ui.Hex("#38383A")
		t.Text, t.TextMuted = ui.Hex("#F5F5F7"), ui.Hex("#98989D")
		t.Accent, t.AccentHover, t.AccentPressed = ui.Hex("#8F8FF0"), ui.Hex("#A0A0F4"), ui.Hex("#7C7CE2")
		t.Danger = ui.Hex("#FF6961")
	} else {
		g := groundNow()
		t.Background, t.Surface = ui.Hex(g.leaf), ui.Hex(g.tray)
		t.SurfaceHover, t.SurfacePressed = ui.Hex("#E4E4E8"), ui.Hex("#DADADF")
		t.Border = ui.Hex("#E5E5EA")
		t.Text, t.TextMuted = ui.Hex("#1D1D1F"), ui.Hex("#86868B")
		t.Accent, t.AccentHover, t.AccentPressed = ui.Hex("#5B5BD6"), ui.Hex("#4D4DC4"), ui.Hex("#4040A8")
		t.Danger = ui.Hex("#E5484D")
	}
	t.AccentText = ui.Hex("#FFFFFF")
	t.Success = ui.Hex("#30A46C")
	t.Radius = 10
	t.Selection = t.Accent.Alpha(0.22)
	t.Focus = t.Accent.Alpha(0.45)
	return &t
}

// cardColor is the fill of a card: white, or, dark, lighter than the
// tray it is in.
func cardColor(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.Hex("#1C1C1E")
	}
	return ui.Hex("#FFFFFF")
}

// mapPalette are the colors maps take: coral, amber, green, blue, purple
// and rose, as bright as each other.
var mapPalette = []string{"#E8684A", "#E8A13A", "#3DAA7D", "#3B8FE0", "#8B6FE0", "#D95C8C"}

// tinted is a color as a light fill and as a deep ink to write on it: a
// chip of the color that reads.
func tinted(t *ui.Theme, col ui.Color) (bg, ink ui.Color) {
	if t.Dark {
		return col.Alpha(0.22), col.MixOklab(t.Text, 0.45)
	}
	return col.Alpha(0.14), col.MixOklab(t.Text, 0.5)
}

// tabGroup is a few tabs in a gray box, the one chosen raised on white,
// as a segmented control; it gives the one clicked, else the one chosen.
func tabGroup(c *ui.Context, key string, names []string, chosenTab int) int {
	t := c.Theme()
	out := chosenTab
	ui.Row(c.Key(key)).Padding(2).Gap(2).Radius(radiusPill).Background(t.Text.Alpha(0.05)).Shrink(0).Children(func() {
		for i, name := range names {
			on := i == chosenTab
			b := ui.Box(c.Key("tab:"+name)).Height(28).Padding(0, 14).Radius(radiusPill).Center().Cursor(ui.CursorPointer).Label(name).
				Background(t.Text.Alpha(0)).Transition(easeLook)
			if !on && b.Hovered() {
				b.Background(t.Text.Alpha(0.05))
			}
			if b.Clicked() {
				out = i
			}
			b.Children(func() {
				if on {
					slidePill(c, key, radiusPill)
					b.ZIndex(-1) // under the others, which the pill slides behind
				}
				txt := ui.Text(c, name).FontSize(textSmall).PassThrough()
				if on {
					txt.FontWeight(600)
				} else {
					txt.TextColor(t.Text.MixOklab(t.TextMuted, 0.6))
				}
			})
		}
	})
	return out
}

// pillTab is a tab of a row of them, as a page's lists: its name and
// count, the one chosen on a shade.
func pillTab(c *ui.Context, key, name, count string, on bool) ui.Element {
	t := c.Theme()
	b := ui.Row(c.Key(key)).Height(30).Padding(0, 13).Gap(6).Radius(radiusPill).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(name).Shrink(0).
		Background(t.Text.Alpha(0)).Transition(easeLook)
	if !on && b.Hovered() {
		b.Background(t.Text.Alpha(0.05))
	}
	press(b, 0.97)
	b.Children(func() {
		if on {
			slidePill(c, "tabs", radiusPill)
			b.ZIndex(-1) // under the others, which the pill slides behind
		}
		txt := ui.Text(c, name).FontSize(textBody).PassThrough()
		if on {
			txt.FontWeight(600)
		} else {
			txt.TextColor(t.TextMuted)
		}
		if count != "" {
			ui.Text(c, count).FontSize(textSmall).TextColor(t.TextMuted).PassThrough()
		}
	})
	return b
}

// addButton is a button that adds a part to a page: as ghostButton, in
// the muted text, for a row of them is not what the page is about.
func addButton(c *ui.Context, text, tip string) ui.Element {
	t := c.Theme()
	b := ui.Box(c.Key("add:"+text)).Padding(5, 10).Radius(radiusPill).Cursor(ui.CursorPointer).Label(text).Tooltip(tip).Shrink(0).
		Background(t.Text.Alpha(0)).Transition(easeLook)
	col := t.TextMuted
	switch {
	case b.Pressed():
		b.Background(t.Text.Alpha(0.1))
		col = t.Text
	case b.Hovered():
		b.Background(t.Text.Alpha(0.06))
		col = t.Text
	}
	b.Children(func() { ui.Text(c, text).FontSize(textSmall).TextColor(col).PassThrough() })
	return b
}

// emptyNote says, under a part's heading, that the part has nothing yet.
func emptyNote(c *ui.Context, text string) {
	ui.Text(c, text).FontSize(textSmall).TextColor(c.Theme().TextMuted).Margin(2, 10, 10)
}

// modal is a dialog over the dimmed window, as ui.Modal's, in the app's
// look: large round corners, lifted well over the page.
func modal(c *ui.Context, key string, open *bool, fn func()) ui.Element {
	t := c.Theme()
	m := ui.Modal(c.Key(key), open, fn).Radius(20)
	if key == "settings" {
		m.Background(ground(t)) // a page of white cards (field)
	}
	if t.Dark {
		return m.Shadow(0, 0, 0, 1, ui.RGBA(255, 255, 255, 0.08)).Shadow(0, 30, 70, -20, ui.RGBA(0, 0, 0, 0.7))
	}
	return m.Shadow(0, 4, 12, 0, ui.RGBA(0, 0, 0, 0.05)).Shadow(0, 30, 80, -10, ui.RGBA(0, 0, 0, 0.22))
}

// sheet is what is chosen, shown over the page from its right: a white
// leaf with round corners, lifted, which slides in; the page beside it
// stays as it is, so that another row chosen shows in the same sheet.
func sheet(c *ui.Context, key string, width float32, body func()) {
	t := c.Theme()
	w, _ := c.Size()
	lifted(ui.Column(c.Key(key)).Absolute().Top(4).Right(10).Bottom(10).Width(min(width, w-20)).Radius(20).Clip().ZIndex(5).
		Background(cardColor(t)), t).
		Transition(ui.ElementTransition{Duration: 360 * time.Millisecond, Ease: easeDetent, Enter: &ui.Motion{X: 56},
			Exit: &ui.Motion{X: 56}, ExitDuration: 200 * time.Millisecond}).Children(body)
}

// field is a part of the settings: its name, small, over a white card
// that holds what body builds.
func field(c *ui.Context, title string, body func()) ui.Element {
	t := c.Theme()
	return ui.Column(c.Key("field:" + title)).Gap(8).FillWidth().Children(func() {
		ui.Text(c, title).FontSize(textSmall).FontWeight(600).TextColor(t.Text.MixOklab(t.TextMuted, 0.45)).Margin(0, 6)
		raise(ui.Column(c.Key("card")).Padding(16, 18).Gap(12).Radius(radiusCard).FillWidth(), t).Children(body)
	})
}

// button is the system's button in the app's look: a pill.
func button(c *ui.Context, text string) ui.Element {
	return ui.Button(c, text).Radius(radiusPill).Padding(0, 14).MinHeight(30)
}
