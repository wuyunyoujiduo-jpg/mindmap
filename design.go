package main

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
)

// The app's look, in a few pieces used everywhere. It is flat and exact,
// as TickTick's is (the user's reference for the style, not for the
// layout, which is the app's own): the window is a quiet gray, and what is
// read and worked on is on one white leaf with round corners (leaf); on
// the leaf a card is a pale block (raise, blockFill), a list is rows with a
// thin line between them, a field is a pale block that turns white with
// the accent's line when written in. Nothing has a shadow but what floats
// over the page (lifted). Corners are small and alike, not pills. Type
// does the ranking: a page's name large and bold, a part's name smaller
// and bold, words on things small and gray. One color, the accent, flat:
// the key that acts, what is chosen, links, the AI.

// Corners: a card's, a row's in a card, and a small thing's.
const (
	radiusCard  = 12
	radiusRow   = 8
	radiusSmall = 7
	radiusPill  = 9 // a key, a tab, a field: a small round corner, not a pill
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
	// It starts at the left column's line, on every page: a page's name
	// stays where it is from a place to another, and the room left is on
	// the right, where what is chosen opens.
	col := ui.Column(c.Key(key)).FillWidth().MaxWidth(pageWidth+56).Padding(0, 28)
	if grow {
		return col.Grow(1).Basis(0).MinHeight(0)
	}
	return col.Shrink(0)
}

// titleTrack draws the letters of a page's name a little together.
const titleTrack = -0.5

// The inks, four and no more (DESIGN.md): the theme's Text for what is
// read first, ink2 for what is read second (a row not chosen), the
// theme's TextMuted for words on things, faint for counts and small names.
func ink2(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.Hex("#C7C7CC")
	}
	return ui.Hex("#48484D")
}

func faint(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.Hex("#6E6E73")
	}
	return ui.Hex("#A9A9AF")
}

// pageHeadHeight is the height of a page's heading: more than the
// toolbar's, for its name stands under the window's edge with room over it.
const pageHeadHeight = 56

// A figure is a number of a page's heading and what it counts.
type figure struct {
	n    int
	name string
}

// figures shows a page in a few figures under its name: the figure in the
// ink, what it counts in the muted ink.
func figures(c *ui.Context, fs ...figure) ui.Element {
	t := c.Theme()
	return ui.Row(c.Key("figures")).Gap(18).Shrink(0).Children(func() {
		for _, f := range fs {
			ui.Row(c.Key(f.name)).Gap(5).AlignItems(ui.Center).Label(fmt.Sprintf("%d %s", f.n, f.name)).Children(func() {
				ui.Text(c, fmt.Sprint(f.n)).FontSize(textSmall).FontWeight(700).FontFeatures("tnum")
				ui.Text(c, f.name).FontSize(textSmall).TextColor(t.TextMuted).NoWrap()
			})
		}
	})
}

// pageHead is a page's heading where it is told in figures: its name
// large, a word beside it, the figures under them, and at the end what
// right builds. 今天, 项目 and 收集 are headed so.
func pageHead(c *ui.Context, key, title, sub string, fs []figure, right func()) {
	t := c.Theme()
	ui.Row(c.Key(key)).Padding(14, 0, 10).Gap(12).AlignItems(ui.End).Shrink(0).FillWidth().DragWindow().Children(func() {
		ui.Column(c).Gap(6).Grow(1).Shrink(1).Basis(0).MinWidth(0).Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.End).Children(func() {
				ui.Text(c, title).FontSize(26).FontWeight(700).LetterSpacing(-0.7).LineHeight(1.2).NoWrap()
				if sub != "" {
					ui.Text(c, sub).FontSize(textBody).TextColor(ink2(t)).Margin(0, 0, 4).SingleLine().Shrink(1).MinWidth(0)
				}
			})
			if len(fs) > 0 {
				figures(c, fs...)
			}
		})
		if right != nil {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Shrink(0).Children(right)
		}
	})
}

// pageHeader is a page's heading, the same on every page: its name, a
// word on it, and at the end what right builds. It is the window's title
// bar there, and drags the window.
func pageHeader(c *ui.Context, key, title, sub string, right func()) {
	t := c.Theme()
	ui.Row(c.Key(key)).Height(pageHeadHeight).Gap(12).AlignItems(ui.Center).Shrink(0).FillWidth().DragWindow().Children(func() {
		ui.Text(c, title).FontSize(textPage).FontWeight(700).LetterSpacing(titleTrack).NoWrap()
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
	return ui.Row(c.Key(key)).MinHeight(44).Padding(4, 8, 4, 14).Gap(10).Radius(radiusRow+2).AlignItems(ui.Center).FillWidth().Shrink(0).
		Background(blockFill(t)).Border(1.5, t.Text.Alpha(0)).Transition(easeLook)
}

// captureFocus outlines a captureBox whose field has the focus.
func captureFocus(c *ui.Context, box, field ui.Element) {
	if field.Focused() {
		focusHalo(box.Background(cardColor(c.Theme())), c.Theme()) // white, in the accent's line
	}
}

// focusHalo outlines what is written in: a line in the accent, on white.
func focusHalo(el ui.Element, t *ui.Theme) ui.Element {
	return el.Border(1.5, t.Accent).NoShadow()
}

// soft is what a card has under it: nothing. A card on the leaf is told
// by its fill alone.
func soft(el ui.Element, t *ui.Theme) ui.Element { return el }

// lifted is the shadow of what floats over the page, the one thing that
// has one: a bar on the canvas, a popup, a sheet, a dialog. A hairline at
// its edge, and one soft shadow.
func lifted(el ui.Element, t *ui.Theme) ui.Element {
	if t.Dark {
		return el.Shadow(0, 0, 0, 1, ui.RGBA(255, 255, 255, 0.1)).Shadow(0, 12, 32, -8, ui.RGBA(0, 0, 0, 0.6))
	}
	return el.Shadow(0, 0, 0, 0.5, ui.RGBA(0, 0, 0, 0.12)).Shadow(0, 8, 28, -6, ui.RGBA(0, 0, 0, 0.16))
}

// floated is the shadow of what comes forward of the page while it is
// used, the bar open: deeper than lifted's, as a dialog's.
func floated(el ui.Element, t *ui.Theme) ui.Element {
	if t.Dark {
		return el.Shadow(0, 0, 0, 1, ui.RGBA(255, 255, 255, 0.12)).Shadow(0, 24, 60, -16, ui.RGBA(0, 0, 0, 0.7))
	}
	return el.Shadow(0, 0, 0, 0.5, ui.RGBA(0, 0, 0, 0.14)).Shadow(0, 3, 10, 0, ui.RGBA(0, 0, 0, 0.06)).Shadow(0, 22, 56, -12, ui.RGBA(0, 0, 0, 0.24))
}

// hoverLift has a card that opens answer the pointer: a shade deeper
// under it, and it gives when pressed. It does not rise: nothing does.
func hoverLift(el ui.Element, t *ui.Theme) ui.Element {
	if el.Hovered() {
		el.Background(t.Text.Alpha(0.075))
	}
	return press(el, 0.985)
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
		Transition(ui.ElementTransition{Duration: motionFast, Ease: easeDetent, Match: pillMatch{group}})
	if t.Dark {
		pill.Shadow(0, 0, 0, 1, ui.RGBA(255, 255, 255, 0.08))
	} else {
		pill.Shadow(0, 0, 0, 0.5, ui.RGBA(0, 0, 0, 0.1)).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.07))
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

// raise gives an element a card's look on the leaf: a pale block, no
// line and no shadow; its color goes smoothly where it changes.
func raise(el ui.Element, t *ui.Theme) ui.Element {
	return el.Background(blockFill(t)).Transition(easeLook)
}

// blockFill is the fill of a block on the leaf: a card, a field, a track.
func blockFill(t *ui.Theme) ui.Color { return t.Text.Alpha(0.045) }

// paper gives an element the look of a card on a block (a task in a
// column of the board, of the week): white, with a hairline at its edge.
func paper(el ui.Element, t *ui.Theme) ui.Element {
	return el.Background(cardColor(t)).Shadow(0, 0, 0, 0.5, t.Text.Alpha(0.12)).Transition(easeLook)
}

// leaf is the white sheet the pages are on, in the gray window: round
// corners, and what is in it cut to them.
func leaf(c *ui.Context, key string) ui.Element {
	return ui.Column(c.Key(key)).Background(c.Theme().Background).Radius(radiusCard).Clip()
}

// easeLook has an element's colors and shadows change smoothly, and no
// more: what is under the pointer lights up rather than switches.
var easeLook = ui.ElementTransition{Duration: motionQuick, Ease: ui.CubicBezier(0, 0, 0.58, 1), Colors: true, Shadows: true}

// How long things take, and no other lengths (DESIGN.md's motion): the
// scale of transitions.dev (https://transitions.dev), each for what it
// names there.
const (
	motionMicro  = 80 * time.Millisecond  // a delay, a step of a shake
	motionQuick  = 150 * time.Millisecond // a color under the pointer, a press, what closes or goes
	motionFast   = 250 * time.Millisecond // what opens from a key, tabs' pill, an icon's swap, a row to its place
	motionMedium = 350 * time.Millisecond // a panel closing, a toast going
	motionSlow   = 400 * time.Millisecond // a panel opening, what was loading shown
)

// easeDetent is the curve things move along: fast at first, settling long.
var easeDetent = ui.CubicBezier(0.22, 1, 0.36, 1)

// rowMotion is how a row of a list comes, goes and makes room: it fades
// in from a little above, the others move to their places, and one gone
// fades out where it was.
var rowMotion = ui.ElementTransition{
	Duration: motionFast, Ease: easeDetent, Position: true, Colors: true,
	Enter: &ui.Motion{Y: -8, Scale: 0.98}, Exit: &ui.Motion{Scale: 0.97}, ExitDuration: motionQuick,
}

// press has an element give a little under the pointer, as a key does.
func press(el ui.Element, by float32) ui.Element {
	p := ui.Pose{Scale: 1}
	if el.Pressed() {
		p.Scale = by
	}
	return el.AnimatePose(p, ui.Animation{Duration: motionQuick, Ease: easeDetent})
}

// chosenCard marks the card chosen: the accent's line around it.
func chosenCard(el ui.Element, t *ui.Theme) ui.Element {
	return el.NoShadow().Shadow(0, 0, 0, 1.5, t.Accent)
}

// whiteCard is a thing among others of its kind, a note, a clip, what
// waits: white, on the one pale ground they share (blockFill), with
// nothing round it until the pointer is over it, when a thin line comes.
// Many pale blocks on the white leaf were dull, each its own island; one
// ground and white cards read as a stack of paper.
// wallCard is a card of 收集's wall, which stands by itself on the page:
// white, told by a hairline inside its edge, darker under the pointer.
func wallCard(el ui.Element, t *ui.Theme) ui.Element {
	// On the dark leaf a white line reads louder than a black one on white.
	rest, hot := float32(0.1), float32(0.24)
	if t.Dark {
		rest, hot = 0.065, 0.16
	}
	el.Background(cardColor(t)).InsetShadow(0, 0, 0, 1, t.Text.Alpha(rest))
	if el.Hovered() {
		el.InsetShadow(0, 0, 0, 1, t.Text.Alpha(hot))
	}
	return press(el.Transition(easeLook), 0.992)
}

func whiteCard(el ui.Element, t *ui.Theme) ui.Element {
	el.Background(cardColor(t)).Shadow(0, 0, 0, 0.5, t.Text.Alpha(0))
	if el.Hovered() {
		el.Shadow(0, 0, 0, 1, t.Text.Alpha(0.12))
	}
	return press(el.Transition(easeLook), 0.992)
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
		ui.Text(c, title).FontSize(textSmall).FontWeight(700).TextColor(t.Text.MixOklab(t.TextMuted, 0.45)).NoWrap()
		if sub != "" {
			ui.Text(c, sub).FontSize(textSmall).TextColor(t.TextMuted).Shrink(1).MinWidth(0).SingleLine()
		}
		ui.Spacer(c)
		if right != nil {
			right()
		}
	})
}

// listCard is a list of rows (listCardRow) on the leaf: no block around
// them, a thin line under each.
func listCard(c *ui.Context, key string) ui.Element {
	return ui.Column(c.Key(key)).FillWidth()
}

// listCardRow is a row of a list on the leaf: a thin line under it, a
// shade under the pointer.
func listCardRow(c *ui.Context, key string) ui.Element {
	t := c.Theme()
	row := ui.Row(c.Key(key)).MinHeight(50).Padding(8, 6).Gap(12).AlignItems(ui.Center).FillWidth().
		BorderWidth(0, 0, 1, 0).BorderColor(hairline(t)).Background(t.Text.Alpha(0)).Transition(rowMotion)
	switch {
	case row.Pressed():
		row.Background(t.Text.Alpha(0.06))
	case row.Hovered():
		row.Background(t.Text.Alpha(0.035))
	}
	return row
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
		chevron(c, t.TextMuted)
	})
	return b
}

// chevron is the mark that something opens a menu or a choice: a small
// angle drawn as a thin line, the same wherever it is. Never a letter
// (⌄, ▾), which sits low and is of another weight on each font.
func chevron(c *ui.Context, col ui.Color) ui.Element {
	return ui.Box(c).Size(10, 10).Shrink(0).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) {
		cx, cy := r.X+r.W/2, r.Y+r.H/2+0.5
		p.StrokePath(new(ui.Path).MoveTo(cx-3.5, cy-1.75).LineTo(cx, cy+1.75).LineTo(cx+3.5, cy-1.75), 1.3, col)
	})
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
	if bold { // a name: heavy, and at a heading's size a little together
		in.FontWeight(700)
		if size >= textHeadline {
			in.LetterSpacing(-0.4)
		}
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
	well(ui.Row(c.Key(key)).Padding(2).Gap(2).Radius(radiusRow).Grow(1).Basis(0).MinWidth(0), t).Children(func() {
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
					txt.FontWeight(700)
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
		if ui.DateInput(c.Key("date:"+label), &d).Label(label).FontSize(textSmall).Background(blockFill(c.Theme())).BorderColor(ui.RGBA(0, 0, 0, 0)).Changed() {
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
			ui.Text(c, title).FontSize(textSmall).FontWeight(700).TextColor(t.TextMuted).Grow(1)
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
		Cursor(ui.CursorPointer).Label(text).Tooltip(tip).Shrink(0)
	jewel(b, t, true)
	b.Children(func() {
		ui.Text(c, text).FontSize(textSmall).FontWeight(700).TextColor(t.AccentText).NoWrap().PassThrough()
	})
	return press(b.Transition(easeLook), 0.97)
}

// inkButton is the key that acts, where the system's would be a button
// in the accent: in the ink, taking the keyboard as a button does, dimmed
// and deaf when not enabled.
func inkButton(c *ui.Context, text string, enabled bool) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c.Key("ink:"+text)).Height(keyHeight).Padding(0, 16).Radius(radiusPill).Label(text).Shrink(0).Disabled(!enabled)
	jewel(b, t, enabled)
	if enabled {
		b.Cursor(ui.CursorPointer)
	}
	b.Children(func() {
		ui.Text(c, text).FontSize(textSmall).FontWeight(700).TextColor(t.AccentText).NoWrap().PassThrough()
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
	// Scroll bars are thin and faint until the pointer is on them.
	t.ScrollbarStyle = ui.ScrollbarStyle{ThumbWidth: 4, TrackWidth: 10, Thumb: t.Text.Alpha(0.13), Hover: t.Text.Alpha(0.3), Active: t.Text.Alpha(0.4)}
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
	well(ui.Row(c.Key(key)).Padding(2).Gap(2).Radius(radiusPill).Shrink(0), t).Children(func() {
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
					txt.FontWeight(700)
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
			txt.FontWeight(700)
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

// emptyState says that a page or a list has nothing yet: an icon, a line,
// a word on what to do, and, when act builds one, the key that does it.
func emptyState(c *ui.Context, key, icon, title, sub string, act func()) {
	t := c.Theme()
	ui.Column(c.Key("empty:"+key)).Padding(36, 20).Gap(6).AlignItems(ui.Center).FillWidth().
		Transition(ui.ElementTransition{Duration: motionSlow, Ease: easeDetent, Enter: &ui.Motion{Y: 12}}).Children(func() {
		kubaiIcon(c, icon, 64, false).Margin(0, 0, 6)
		ui.Text(c, title).FontSize(textTitle).FontWeight(700).TextColor(ink2(t))
		if sub != "" {
			ui.Text(c, sub).FontSize(textSmall).TextColor(t.TextMuted).LineHeight(1.5).MaxWidth(420).Wrap().TextAlign(ui.Center)
		}
		if act != nil {
			ui.Box(c).Margin(10, 0, 0).Children(act)
		}
	})
}

// motionOff is set while nothing animates (the tests): what pulses is still.
var motionOff bool

// skeleton stands for lines being read or written by the AI: pale bars
// of the widths given (a part of the room each), which breathe.
func skeleton(c *ui.Context, key string, widths ...float32) {
	skeletonShapes(c, key, func(shade ui.Color) {
		for i, w := range widths {
			ui.Box(c.Key(i)).Height(11).WidthPercent(w * 100).Radius(6).Background(shade)
		}
	})
}

// skeletonShapes stands for what is being read or written, in its own
// shape: body builds pale shapes of the shade given, where and as large
// as what will come, and they breathe together. What comes then takes
// their places, and nothing moves.
func skeletonShapes(c *ui.Context, key string, body func(shade ui.Color)) ui.Element {
	t := c.Theme()
	col := ui.Column(c.Key("skeleton:"+key)).Gap(9).Padding(4, 0).FillWidth().Label("正在载入")
	lit := float32(1)
	if !motionOff {
		const beat = 900 * time.Millisecond
		now := time.Now()
		if (now.UnixMilli()/beat.Milliseconds())%2 == 1 {
			lit = 0.45
		}
		c.After(beat - time.Duration(now.UnixNano())%beat)
		lit = col.AnimateTo("breath", lit, ui.Animation{Duration: beat, NonMotion: true})
	}
	return col.Children(func() { body(t.Text.Alpha(0.08 * lit)) })
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
	if t.Dark {
		return m.Shadow(0, 0, 0, 1, ui.RGBA(255, 255, 255, 0.08)).Shadow(0, 30, 70, -20, ui.RGBA(0, 0, 0, 0.7))
	}
	return m.Shadow(0, 4, 12, 0, ui.RGBA(0, 0, 0, 0.05)).Shadow(0, 30, 80, -10, ui.RGBA(0, 0, 0, 0.22))
}

// sheet is what is chosen, in a column at the leaf's right, a thin line
// between it and the page: the page beside it stays as it is, so that
// another row chosen shows in the same column.
func sheet(c *ui.Context, key string, width float32, body func()) {
	t := c.Theme()
	w, _ := c.Size()
	ui.Column(c.Key(key)).Width(min(width, w*0.4)).Shrink(0).FillHeight().BorderWidth(0, 0, 0, 1).BorderColor(hairline(t)).
		Transition(ui.ElementTransition{Duration: motionSlow, Ease: easeDetent, Enter: &ui.Motion{X: 24}}).Children(body)
}

// field is a part of the settings: its name, small, over a white card
// that holds what body builds.
func field(c *ui.Context, title string, body func()) ui.Element {
	t := c.Theme()
	return ui.Column(c.Key("field:" + title)).Gap(8).FillWidth().Children(func() {
		ui.Text(c, title).FontSize(textCaption).FontWeight(700).LetterSpacing(0.3).TextColor(t.TextMuted).Margin(0, 6)
		raise(ui.Column(c.Key("card")).Padding(16, 18).Gap(12).Radius(radiusCard).FillWidth(), t).Children(body)
	})
}

// button is the system's button in the app's look: a pill.
func button(c *ui.Context, text string) ui.Element {
	return ui.Button(c, text).Radius(radiusPill).Padding(0, 14).MinHeight(30)
}

// rowsField is a part of the settings whose card holds rows (setRow),
// one under another with a thin line between (setLine); note, if any, is
// said small under the card.
func rowsField(c *ui.Context, title, note string, body func()) ui.Element {
	t := c.Theme()
	return ui.Column(c.Key("field:" + title)).Gap(8).FillWidth().Children(func() {
		if title != "" {
			// A group's name, small and quiet, as a list's groups are named.
			ui.Text(c, title).FontSize(textCaption).FontWeight(700).LetterSpacing(0.3).TextColor(t.TextMuted).Margin(0, 6)
		}
		raise(ui.Column(c.Key("card")).Padding(4, 18).Radius(radiusCard).FillWidth(), t).Children(body)
		if note != "" {
			ui.Text(c, note).FontSize(textCaption).TextColor(t.TextMuted).LineHeight(1.5).Wrap().Margin(0, 8)
		}
	})
}

// setRow is a row of the settings: what it is, a word on it under its
// name, and at its end what sets it.
func setRow(c *ui.Context, title, sub string, control func()) {
	setRowSays(c, title, sub, "", control)
}

// setRowSays is setRow with what is wrong with what was written in it,
// said under its name as it is written (wrong == "" for nothing).
func setRowSays(c *ui.Context, title, sub, wrong string, control func()) {
	t := c.Theme()
	ui.Row(c.Key("row:"+title)).MinHeight(52).Padding(10, 0).Gap(16).AlignItems(ui.Center).FillWidth().Children(func() {
		ui.Column(c).Gap(2).Grow(1).Shrink(1).Basis(0).MinWidth(0).Children(func() {
			ui.Text(c, title).FontSize(textBody) // a row's name is read, not shouted: regular, its word under it muted
			switch {
			case wrong != "":
				ui.Text(c.Key("wrong"), wrong).FontSize(textSmall).TextColor(t.Danger).LineHeight(1.45).Wrap().
					Transition(ui.ElementTransition{Duration: motionQuick, Ease: easeDetent, Enter: &ui.Motion{Y: -4}})
			case sub != "":
				ui.Text(c, sub).FontSize(textSmall).TextColor(t.TextMuted).LineHeight(1.45).Wrap()
			}
		})
		if control != nil {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Shrink(0).Children(control)
		}
	})
}

// setLine is the thin line between two rows of a card of the settings.
func setLine(c *ui.Context) {
	ui.Box(c).Height(1).FillWidth().Shrink(0).Background(hairline(c.Theme()))
}

// area is a box to write several lines in, in the app's look: a pale
// shade of the page with round corners, white with the accent's line and
// halo while written in, as the one bar is.
func area(c *ui.Context, value *string) ui.Element {
	t := c.Theme()
	in := ui.TextAreaBase(c, value).Padding(10, 12).Radius(12).FillWidth().LineHeight(1.5).FontSize(textBody).
		Background(t.Text.Alpha(0.04)).Border(1.5, t.Text.Alpha(0)).Transition(easeLook)
	switch {
	case in.Focused():
		focusHalo(in.Background(cardColor(t)), t)
	case in.Hovered():
		in.Background(t.Text.Alpha(0.06))
	}
	return in
}

// well is a track (of tabs, of a segmented control): a pale block, which
// the white pill chosen stands on.
func well(el ui.Element, t *ui.Theme) ui.Element {
	return el.Background(t.Text.Alpha(0.055))
}

// jewel is the key that acts: the accent, flat; a shade deeper under the
// pointer and when pressed.
func jewel(b ui.Element, t *ui.Theme, enabled bool) ui.Element {
	switch {
	case !enabled:
		return b.Background(t.Text.Alpha(0.14))
	case b.Pressed():
		return b.Background(t.AccentPressed)
	case b.Hovered():
		return b.Background(t.AccentHover)
	}
	return b.Background(t.Accent)
}
