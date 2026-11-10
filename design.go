package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// The app's look, in a few pieces used everywhere. It is laid out as
// Detent's components are (detentui.com): the window is a quiet ground
// the rail stands on; what is read and worked on is a leaf on it, near
// white, with round corners; a part of a page is a tray, a shade darker
// than the leaf, with its heading on it and what it holds in a white card;
// cards are told by a thin ring rather than a shadow; the one key that
// acts is in the ink, not in the accent, which is kept for what is chosen
// and for the AI.

// Sizes of text.
const (
	textCaption  = 11
	textSmall    = 12
	textBody     = 13
	textTitle    = 15
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
const pageHeadHeight = 68

// pageHeader is a page's heading, the same on every page: its name, a
// word on it, and at the end what right builds. It is the window's title
// bar there, and drags the window.
func pageHeader(c *ui.Context, key, title, sub string, right func()) {
	t := c.Theme()
	ui.Row(c.Key(key)).Height(pageHeadHeight).Padding(16, 0, 0).Gap(10).AlignItems(ui.Center).Shrink(0).FillWidth().DragWindow().Children(func() {
		ui.Text(c, title).FontSize(textPage).FontWeight(600).NoWrap()
		if sub != "" {
			ui.Text(c, sub).FontSize(textSmall).TextColor(t.TextMuted).SingleLine().Shrink(1).MinWidth(0)
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
	return raise(ui.Row(c.Key(key)).MinHeight(48).Padding(5, 8, 5, 16).Gap(10).Radius(12).AlignItems(ui.Center).FillWidth().Shrink(0), t)
}

// captureFocus outlines a captureBox whose field has the focus.
func captureFocus(c *ui.Context, box, field ui.Element) {
	if field.Focused() {
		t := c.Theme()
		box.Border(1, t.Accent.Alpha(0.55)).Shadow(0, 0, 0, 4, t.Accent.Alpha(0.14)) // a soft halo, not a loud line
	} else {
		box.Shadow(0, 0, 0, 0, c.Theme().Accent.Alpha(0)) // the halo's place, for it to grow from
	}
}

// ground is the window's own color, which the rail and the maps' list
// stand on and the leaf lies on.
func ground(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.Hex("#131211")
	}
	return ui.Hex("#EEEDE9")
}

// cardEdge is the thin ring a card is told by.
func cardEdge(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.RGBA(255, 255, 255, 0.09)
	}
	return ui.RGBA(40, 32, 16, 0.1)
}

// shadowColor is the shadow under a card: hardly there, the ring tells it.
var shadowColor = ui.RGBA(40, 32, 16, 0.05)

// raise gives an element a card's look: white, a thin ring, a faint
// shadow under it.
func raise(el ui.Element, t *ui.Theme) ui.Element {
	// Lit from above: a line of light inside its top edge, a short shadow
	// close under it and a long faint one further down. Its colors and
	// shadows go smoothly where they change, as under the pointer.
	light := ui.RGBA(255, 255, 255, 0.9)
	if t.Dark {
		light = ui.RGBA(255, 255, 255, 0.05)
	}
	return el.Background(cardColor(t)).Border(1, cardEdge(t)).InsetShadow(0, 1, 0, 0, light).
		Shadow(0, 1, 1, 0, shadowColor).Shadow(0, 3, 8, -3, ui.RGBA(40, 32, 16, 0.08)).Transition(easeLook)
}

// easeLook has an element's colors and shadows change smoothly, and no
// more: what is under the pointer lights up rather than switches.
var easeLook = ui.ElementTransition{Duration: 140 * time.Millisecond, Colors: true, Shadows: true}

// easeDetent is the curve things move along: fast at first, settling.
var easeDetent = ui.CubicBezier(0.2, 0.9, 0.25, 1)

// rowMotion is how a row of a list comes, goes and makes room: it fades
// in from a little above, the others move to their places, and one gone
// fades out where it was.
var rowMotion = ui.ElementTransition{
	Duration: 260 * time.Millisecond, Ease: easeDetent, Position: true, Colors: true,
	Enter: &ui.Motion{Y: -6}, Exit: &ui.Motion{Scale: 0.98}, ExitDuration: 160 * time.Millisecond,
}

// press has an element give a little under the pointer, as a key does.
func press(el ui.Element, by float32) ui.Element {
	p := ui.Pose{Scale: 1}
	if el.Pressed() {
		p.Scale = by
	}
	return el.AnimatePose(p, ui.Animation{Duration: 120 * time.Millisecond, Ease: easeDetent})
}

// chosenCard marks the card chosen: its ring in the accent, a soft halo
// around it; its size is as it was.
func chosenCard(el ui.Element, t *ui.Theme) ui.Element {
	return el.Border(1, t.Accent).Shadow(0, 0, 0, 3, t.Accent.Alpha(0.18))
}

// tray is a part of a page: a soft ground with its heading on it
// (trayHead) and what it holds in a white card (listCard), as the columns
// of the board.
func tray(c *ui.Context, key string) ui.Element {
	return ui.Column(c.Key(key)).Padding(6).Radius(16).Background(c.Theme().Surface).FillWidth()
}

// trayHead is a tray's heading: its title, a word on it, and at its end
// what right builds. It is as high on every tray, so that trays side by
// side start level.
func trayHead(c *ui.Context, key, title, sub string, right func()) ui.Element {
	t := c.Theme()
	return ui.Row(c.Key(key)).Height(38).Padding(0, 4, 2, 10).Gap(8).AlignItems(ui.Center).FillWidth().Shrink(0).Children(func() {
		ui.Text(c, title).FontSize(textBody).FontWeight(600).NoWrap()
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
	return raise(ui.Column(c.Key(key)).FillWidth().Padding(4).Gap(1).Radius(10), c.Theme())
}

// listCardRow is a row of a listCard: no lines between rows, each lit
// under the pointer.
func listCardRow(c *ui.Context, key string) ui.Element {
	t := c.Theme()
	row := ui.Row(c.Key(key)).MinHeight(40).Padding(6, 10).Gap(10).Radius(7).AlignItems(ui.Center).FillWidth().
		Background(t.Text.Alpha(0)).Transition(rowMotion)
	if row.Hovered() {
		row.Background(t.Text.Alpha(0.045))
	}
	return press(row, 0.99)
}

// textLink is a few words in the accent that go somewhere: no box, so
// that it ends where the part it heads ends.
func textLink(c *ui.Context, text, label string) ui.Element {
	t := c.Theme()
	l := ui.Box(c.Key("link:"+label)).Padding(0, 6).Cursor(ui.CursorPointer).Label(label).Shrink(0)
	col := t.Accent
	if l.Hovered() {
		col = t.AccentHover
	}
	l.Children(func() { ui.Text(c, text).FontSize(textSmall).FontWeight(500).TextColor(col).PassThrough() })
	return l
}

// key gives a button the look of a key: a white face with a thin ring,
// a shade under the pointer.
func key(b ui.Element, t *ui.Theme) ui.Element {
	switch {
	case b.Pressed():
		b.Background(t.SurfacePressed)
	case b.Hovered():
		b.Background(t.SurfaceHover)
	default:
		b.Background(cardColor(t))
	}
	light := ui.RGBA(255, 255, 255, 0.9)
	if t.Dark {
		light = ui.RGBA(255, 255, 255, 0.05)
	}
	return press(b.Border(1, cardEdge(t)).InsetShadow(0, 1, 0, 0, light).Shadow(0, 1, 1, 0, shadowColor).Transition(easeLook), 0.97)
}

// dropButton is a key that opens a menu (the caller's Menu): its text
// and a mark that it opens.
func dropButton(c *ui.Context, k, text, label, tip string) ui.Element {
	t := c.Theme()
	b := key(ui.Row(c.Key(k)).Height(30).Padding(0, 9, 0, 11).Gap(5).Radius(8).AlignItems(ui.Center).Cursor(ui.CursorPointer).
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
		Padding(4, 6).Radius(6).FillWidth()
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
		Padding(6, 8).Radius(8).FillWidth().LineHeight(1.5)
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
	ui.Row(c.Key(key)).Padding(2).Gap(2).Radius(8).Background(t.Text.Alpha(0.05)).Grow(1).Basis(0).MinWidth(0).Children(func() {
		for i, name := range names {
			on := i == chosenPart
			b := ui.Box(c.Key("part:" + name)).Height(26).Radius(6).Center().Grow(1).Basis(0).MinWidth(0).Cursor(ui.CursorPointer).
				Label(label + " " + name)
			switch {
			case on:
				raise(b, t)
			case b.Hovered():
				b.Background(t.Text.Alpha(0.05))
			}
			if b.Clicked() {
				out = i
			}
			b.Children(func() {
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
	b := ui.Box(c.Key("chip:"+label)).Height(26).Padding(0, 10).Radius(999).Center().Cursor(ui.CursorPointer).
		Label(label).Tooltip(tip).Shrink(0)
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
	return ui.Column(c).Padding(12, 14).Gap(8).Radius(10).Background(tint).FillWidth()
}

// ghostButton is a button of text with no box until hovered.
func ghostButton(c *ui.Context, text, tip string) ui.Element {
	t := c.Theme()
	b := ui.Box(c.Key("ghost:"+text)).Padding(4, 8).Radius(6).Cursor(ui.CursorPointer).Label(text).Tooltip(tip).Shrink(0)
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
	box := ui.Row(c.Key(key+"-box")).Height(30).Width(width).Shrink(1).MinWidth(80).Padding(0, 8).Gap(6).Radius(8).
		AlignItems(ui.Center).Background(t.Text.Alpha(0.05))
	var in ui.Element
	box.Children(func() {
		ui.Box(c).Size(14, 14).Shrink(0).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) { drawIcon(p, r, 13, iconSearch, t.TextMuted) })
		in = ui.TextInputBase(c.Key(key), value).Placeholder(placeholder).Label(label).FontSize(textBody).Grow(1).Tooltip(tip)
		if in.Focused() {
			box.Border(1, t.Accent.Alpha(0.6))
		}
	})
	return in
}

// listRow is a row of a list that acts when clicked: text on the left, a
// soft highlight under the pointer, as the suggestions of Spotlight.
func listRow(c *ui.Context, ic icon, text string) ui.Element {
	t := c.Theme()
	b := ui.Row(c.Key("row:"+text)).Padding(7, 10).Gap(10).Radius(8).AlignItems(ui.Center).FillWidth().
		Cursor(ui.CursorPointer).Label(text)
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
	b := ui.Box(c.Key("accent:"+text)).Height(30).Padding(0, 14).Radius(8).Center().
		Cursor(ui.CursorPointer).Label(text).Tooltip(tip).Shrink(0).Background(t.Text)
	switch {
	case b.Pressed():
		b.Background(t.Text.MixOklab(t.Background, 0.3))
	case b.Hovered():
		b.Background(t.Text.MixOklab(t.Background, 0.15))
	}
	b.Children(func() {
		ui.Text(c, text).FontSize(textSmall).FontWeight(600).TextColor(t.Background).NoWrap().PassThrough()
	})
	return press(b.Transition(easeLook), 0.97)
}

// inkButton is the key that acts, where the system's would be a button
// in the accent: in the ink, taking the keyboard as a button does, dimmed
// and deaf when not enabled.
func inkButton(c *ui.Context, text string, enabled bool) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c.Key("ink:"+text)).Height(30).Padding(0, 14).Radius(8).Label(text).Shrink(0).Disabled(!enabled)
	switch {
	case !enabled:
		b.Background(t.Text.Alpha(0.22))
	case b.Pressed():
		b.Background(t.Text.MixOklab(t.Background, 0.3))
	case b.Hovered():
		b.Background(t.Text.MixOklab(t.Background, 0.15)).Cursor(ui.CursorPointer)
	default:
		b.Background(t.Text).Cursor(ui.CursorPointer)
	}
	b.Children(func() {
		ui.Text(c, text).FontSize(textSmall).FontWeight(600).TextColor(t.Background).NoWrap().PassThrough()
	})
	return press(b.Transition(easeLook), 0.97)
}

// softButton is a key of a word, for what is beside the one that acts.
func softButton(c *ui.Context, text, tip string) ui.Element {
	t := c.Theme()
	b := key(ui.Box(c.Key("soft:"+text)).Height(30).Padding(0, 12).Radius(8).Center().
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
	if base.Accent == ui.Hex("#5B5BD6") || base.Accent == ui.Hex("#9A9AF2") {
		return base // already ours
	}
	t := *base
	if t.Dark {
		t.Background, t.Surface = ui.Hex("#1B1A18"), ui.Hex("#232120")
		t.SurfaceHover, t.SurfacePressed = ui.Hex("#2E2C29"), ui.Hex("#393632")
		t.Border = ui.Hex("#37342F")
		t.Text, t.TextMuted = ui.Hex("#EDEBE7"), ui.Hex("#A39E96")
		t.Accent, t.AccentHover, t.AccentPressed = ui.Hex("#9A9AF2"), ui.Hex("#ABABF5"), ui.Hex("#8585E6")
		t.Danger = ui.Hex("#F0806A")
	} else {
		t.Background, t.Surface = ui.Hex("#FDFDFC"), ui.Hex("#F2F1EE")
		t.SurfaceHover, t.SurfacePressed = ui.Hex("#EAE8E4"), ui.Hex("#E1DFDA")
		t.Border = ui.Hex("#E4E2DD")
		t.Text, t.TextMuted = ui.Hex("#1C1B19"), ui.Hex("#6F6B65")
		t.Accent, t.AccentHover, t.AccentPressed = ui.Hex("#5B5BD6"), ui.Hex("#4D4DC4"), ui.Hex("#4040A8")
		t.Danger = ui.Hex("#D2452D")
	}
	t.AccentText = ui.Hex("#FFFFFF")
	t.Success = ui.Hex("#2E9E6B")
	t.Selection = t.Accent.Alpha(0.22)
	t.Focus = t.Accent.Alpha(0.45)
	return &t
}

// cardColor is the fill of a card: white, or, dark, lighter than the
// tray it is in.
func cardColor(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.Hex("#2A2826")
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
	ui.Row(c.Key(key)).Padding(2).Gap(2).Radius(8).Background(t.Text.Alpha(0.05)).Shrink(0).Children(func() {
		for i, name := range names {
			on := i == chosenTab
			b := ui.Box(c.Key("tab:"+name)).Height(26).Padding(0, 12).Radius(6).Center().Cursor(ui.CursorPointer).Label(name)
			switch {
			case on:
				raise(b, t)
			case b.Hovered():
				b.Background(t.Text.Alpha(0.05))
			}
			if b.Clicked() {
				out = i
			}
			b.Children(func() {
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
	b := ui.Row(c.Key(key)).Height(28).Padding(0, 10).Gap(5).Radius(7).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(name).Shrink(0)
	switch {
	case on:
		b.Background(t.Text.Alpha(0.07))
	case b.Hovered():
		b.Background(t.Text.Alpha(0.04))
	}
	b.Children(func() {
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
	b := ui.Box(c.Key("add:"+text)).Padding(4, 8).Radius(6).Cursor(ui.CursorPointer).Label(text).Tooltip(tip).Shrink(0)
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
