package main

import (
	"github.com/egoist/mygo/ui"
)

// The app's look, in a few pieces used everywhere, as the macOS Human
// Interface Guidelines have it: a sidebar of glass beside the content, a
// toolbar that is the title bar, an inspector edge to edge; an 8 point
// grid; the system font in a few sizes; hairlines rather than boxes; text
// edited where it is read rather than in form fields; the accent kept for
// what is chosen and what acts.

// Sizes of text.
const (
	textCaption  = 11
	textSmall    = 12
	textBody     = 13
	textTitle    = 15
	textHeadline = 20
)

// toolbarHeight is the toolbar's, which is the window's title bar.
const toolbarHeight = 52

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
	ui.Row(c.Key("prop:" + name)).Gap(8).AlignItems(ui.Center).MinHeight(30).Children(func() {
		ui.Text(c, name).FontSize(textSmall).TextColor(t.TextMuted).Width(56).Shrink(0)
		ui.Row(c).Grow(1).Shrink(1).MinWidth(0).Gap(6).Wrap().GapY(6).AlignItems(ui.Center).Children(fn)
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
