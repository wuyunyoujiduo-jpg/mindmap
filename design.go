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

// accentButton is the one button of a part that acts: filled with the
// accent, its text white.
func accentButton(c *ui.Context, text, tip string) ui.Element {
	t := c.Theme()
	b := ui.Box(c.Key("accent:"+text)).Height(30).Padding(0, 14).Radius(8).Center().
		Cursor(ui.CursorPointer).Label(text).Tooltip(tip).Shrink(0).Background(t.Accent)
	switch {
	case b.Pressed():
		b.Background(t.Accent.MixOklab(t.Text, 0.2))
	case b.Hovered():
		b.Background(t.Accent.MixOklab(t.Text, 0.1))
	}
	b.Children(func() {
		ui.Text(c, text).FontSize(textSmall).FontWeight(600).TextColor(ui.Hex("#FFFFFF")).NoWrap().PassThrough()
	})
	return b
}

// softButton is a button on a light gray, for what is beside the accent's.
func softButton(c *ui.Context, text, tip string) ui.Element {
	t := c.Theme()
	b := ui.Box(c.Key("soft:"+text)).Height(30).Padding(0, 12).Radius(8).Center().
		Cursor(ui.CursorPointer).Label(text).Tooltip(tip).Shrink(0).Background(t.Text.Alpha(0.06))
	switch {
	case b.Pressed():
		b.Background(t.Text.Alpha(0.14))
	case b.Hovered():
		b.Background(t.Text.Alpha(0.1))
	}
	b.Children(func() { ui.Text(c, text).FontSize(textSmall).NoWrap().PassThrough() })
	return b
}

// pageHead is the heading of a part of a page: its title, how many, and a
// link at the end, over a hairline. It tells whether the link was clicked.
func pageHead(c *ui.Context, title, count, link string) bool {
	t := c.Theme()
	clicked := false
	ui.Row(c.Key("head:"+title)).Padding(0, 0, 8).Gap(8).AlignItems(ui.Center).FillWidth().
		BorderWidth(0, 0, 1, 0).BorderColor(hairline(t)).Children(func() {
		ui.Text(c, title).FontSize(textBody).FontWeight(600).TextColor(t.Text.MixOklab(t.TextMuted, 0.5))
		if count != "" {
			ui.Text(c, count).FontSize(textSmall).TextColor(t.TextMuted)
		}
		ui.Spacer(c)
		if link != "" {
			l := ui.Box(c.Key("link:" + title)).Cursor(ui.CursorPointer).Label(link + " " + title)
			if l.Clicked() {
				clicked = true
			}
			col := t.Accent
			if l.Hovered() {
				col = t.Accent.MixOklab(t.Text, 0.25)
			}
			l.Children(func() { ui.Text(c, link).FontSize(textSmall).TextColor(col).PassThrough() })
		}
	})
	return clicked
}

// quietTheme is the app's palette over the system's theme: warm grays for
// the text and the lines, and coral, the icon's color, for what acts.
func quietTheme(base *ui.Theme) *ui.Theme {
	if base.Accent == ui.Hex("#D4705F") || base.Accent == ui.Hex("#E0846F") {
		return base // already ours
	}
	t := *base
	if t.Dark {
		t.Background, t.Surface = ui.Hex("#1C1C1E"), ui.Hex("#252527")
		t.SurfaceHover, t.SurfacePressed = ui.Hex("#2F2F32"), ui.Hex("#38383C")
		t.Border = ui.Hex("#3A3A3D")
		t.Text, t.TextMuted = ui.Hex("#EDEDEF"), ui.Hex("#9C9CA3")
		t.Accent, t.AccentHover, t.AccentPressed = ui.Hex("#E0846F"), ui.Hex("#E89786"), ui.Hex("#C9715D")
	} else {
		t.Surface, t.SurfaceHover, t.SurfacePressed = ui.Hex("#F5F4F1"), ui.Hex("#ECEAE6"), ui.Hex("#E3E1DC")
		t.Border = ui.Hex("#E3E1DC")
		t.Text, t.TextMuted = ui.Hex("#1D1D1F"), ui.Hex("#76767C")
		t.Accent, t.AccentHover, t.AccentPressed = ui.Hex("#D4705F"), ui.Hex("#C4604F"), ui.Hex("#B4574A")
	}
	t.AccentText = ui.Hex("#FFFFFF")
	t.Selection = t.Accent.Alpha(0.25)
	t.Focus = t.Accent.Alpha(0.5)
	return &t
}
