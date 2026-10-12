package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// The kinds of slides.
const (
	slideTitle = iota // the root, alone
	slideList         // a topic and its subtopics
	slideTopic        // a topic without subtopics and its note
)

// slide is one page of a presentation of a canvas.
type slide struct {
	kind   int
	node   *model.Node
	items  []*model.Node // a list slide's subtopics
	path   []string      // the titles above the node
	branch int           // the index of the main branch it is on
}

// steps is how many times Next stays on the slide, revealing an item.
func (s slide) steps(p model.Present) int {
	if s.kind == slideList && p.Reveal {
		return len(s.items)
	}
	return 0
}

// slidesOf makes a canvas's slides: its root, the list of its main topics,
// then, depth first, a slide for each topic with subtopics and for each
// topic without them that has a note.
func slidesOf(d *model.Document) []slide {
	root := d.Root
	slides := []slide{{kind: slideTitle, node: root}}
	if len(root.Children) > 0 {
		slides = append(slides, slide{kind: slideList, node: root, items: root.Children})
	}
	var walk func(n *model.Node, path []string, branch int)
	walk = func(n *model.Node, path []string, branch int) {
		for i, c := range n.Children {
			b := branch
			if n == root {
				b = i
			}
			p := append(slices.Clone(path), n.Title)
			switch {
			case len(c.Children) > 0:
				slides = append(slides, slide{kind: slideList, node: c, items: c.Children, path: p, branch: b})
				walk(c, p, b)
			case strings.TrimSpace(c.Note) != "":
				slides = append(slides, slide{kind: slideTopic, node: c, path: p, branch: b})
			}
		}
	}
	walk(root, nil, 0)
	return slides
}

// slideColors are a slide's background, text, secondary text and accent.
type slideColors struct{ bg, text, muted, accent ui.Color }

func colorsOf(p model.Present, l look, s slide) slideColors {
	accent := l.branch(s.branch)
	if s.kind == slideTitle || s.node != nil && s.kind == slideList && len(s.path) == 0 {
		accent = l.branch(0)
	}
	switch p.Theme {
	case "light":
		return slideColors{ui.Hex("#FFFFFF"), ui.Hex("#1F2329"), ui.Hex("#6B7280"), accent}
	case "color":
		bg := accent.Mix(ui.Hex("#000000"), 0.15)
		fg := textOn(bg)
		return slideColors{bg, fg, fg.Alpha(0.75), fg}
	}
	return slideColors{ui.Hex("#111214"), ui.Hex("#F5F5F7"), ui.Hex("#9CA3AF"), accent}
}

// slideView draws a slide in a box w×h DIPs, its text scaled to the box,
// with the first step items of a list slide shown (all for -1).
func (a *app) slideView(c *ui.Context, d *model.Document, s slide, index, count int, w, h float32, step int) ui.Element {
	p := d.Present
	col := colorsOf(p, lookOf(d.Style), s)
	k := h / 720 // a slide is designed 720 DIPs high
	font := d.Style.Font
	box := ui.Column(c).Size(w, h).Background(col.bg).Clip().Font(font).TextColor(col.text)
	box.Children(func() {
		switch s.kind {
		case slideTitle:
			ui.Column(c).Grow(1).Center().Gap(24 * k).Padding(60 * k).Children(func() {
				ui.Box(c).Size(80*k, 8*k).Radius(4 * k).Background(col.accent)
				ui.Text(c, s.node.Title).FontSize(76 * k).Bold().TextAlign(ui.Center).MaxWidth(w * 0.85)
				if n := len(s.node.Children); n > 0 {
					ui.Textf(c, "%d 个主题", n).FontSize(26 * k).TextColor(col.muted)
				}
			})
		case slideList, slideTopic:
			ui.Column(c).Grow(1).Padding(64*k, 80*k).Gap(18 * k).Children(func() {
				if len(s.path) > 0 {
					ui.Text(c, strings.Join(s.path, "  ›  ")).FontSize(20 * k).TextColor(col.muted).SingleLine()
				}
				ui.Text(c, s.node.Title).FontSize(54 * k).Bold()
				ui.Box(c).Size(64*k, 6*k).Radius(3*k).Background(col.accent).Margin(4*k, 0, 18*k)
				if s.kind == slideTopic {
					ui.Text(c, s.node.Note).FontSize(28 * k).TextColor(col.text).Wrap()
					return
				}
				shown := len(s.items)
				if step >= 0 {
					shown = step
				}
				if p.Layout == "cards" {
					a.slideCards(c, s, col, k, shown)
				} else {
					a.slideList(c, s, col, k, shown)
				}
			})
		}
		ui.Row(c).Padding(0, 40*k, 24*k).Children(func() {
			ui.Spacer(c)
			ui.Textf(c, "%d / %d", index+1, count).FontSize(16 * k).TextColor(col.muted)
		})
	})
	return box
}

func (a *app) slideList(c *ui.Context, s slide, col slideColors, k float32, shown int) {
	ui.Column(c).Gap(22 * k).Children(func() {
		for i, it := range s.items {
			row := ui.Row(c.Key(it.ID)).Gap(18 * k).AlignItems(ui.Center)
			if i >= shown {
				row.Invisible() // keeps its room, so the list does not move as it shows
			}
			row.Children(func() {
				ui.Box(c).Size(12*k, 12*k).Radius(999).Background(col.accent).Shrink(0)
				for _, m := range it.Markers {
					markerView(c, m, 30*k)
				}
				ui.Text(c, it.Title).FontSize(34 * k).Wrap()
				if n := len(it.Children); n > 0 {
					ui.Textf(c, "%d", n).FontSize(18 * k).TextColor(col.muted)
				}
			})
		}
	})
}

func (a *app) slideCards(c *ui.Context, s slide, col slideColors, k float32, shown int) {
	cols := 3
	if len(s.items) <= 4 {
		cols = 2
	}
	ui.Grid(c).Columns(cols).Gap(20 * k).Children(func() {
		for i, it := range s.items {
			card := ui.Column(c.Key(it.ID)).Padding(24 * k).Gap(10 * k).Radius(16 * k).Background(col.text.Alpha(0.08))
			if i >= shown {
				card.Invisible()
			}
			card.Children(func() {
				ui.Box(c).Size(36*k, 5*k).Radius(3 * k).Background(col.accent)
				ui.Text(c, it.Title).FontSize(30 * k).Bold().Wrap()
				switch {
				case it.Note != "":
					ui.Text(c, it.Note).FontSize(18 * k).TextColor(col.muted).MaxLines(3).Ellipsis("…")
				case len(it.Children) > 0:
					ui.Textf(c, "%d 个子主题", len(it.Children)).FontSize(18 * k).TextColor(col.muted)
				}
			})
		}
	})
}

// startPresenting shows the canvas's slides full screen, from the slide of
// the node with the ID, or from the first.
func (a *app) startPresenting(from string) {
	if a.doc == nil {
		return
	}
	a.commitEditIfAny()
	a.slides = slidesOf(a.doc)
	a.slideIndex, a.slideStep = 0, 0
	for i, s := range a.slides {
		if from != "" && s.node.ID == from && s.kind != slideTitle {
			a.slideIndex = i
			break
		}
	}
	a.presenting = true
	a.plat.SetFullScreen(true)
}

func (a *app) stopPresenting() {
	a.presenting = false
	a.plat.SetFullScreen(false)
	a.canvas.Focus()
}

func (a *app) nextSlide() {
	if a.slideIndex >= len(a.slides) {
		return
	}
	if a.slideStep < a.slides[a.slideIndex].steps(a.doc.Present) {
		a.slideStep++
		return
	}
	if a.slideIndex+1 < len(a.slides) {
		a.slideIndex++
		a.slideStep = 0
	}
}

func (a *app) prevSlide() {
	if a.slideStep > 0 {
		a.slideStep--
		return
	}
	if a.slideIndex > 0 {
		a.slideIndex--
		a.slideStep = a.slides[a.slideIndex].steps(a.doc.Present)
	}
}

// presentView fills the window with the slide shown: the arrows, Space and
// a click go through the slides, and Escape ends.
func (a *app) presentView(c *ui.Context) {
	w, h := c.Size()
	stage := ui.Box(c.Key("stage")).Fill().Background(ui.Hex("#000000")).Center().Focusable().FocusRing(false).AutoFocus().
		Label("演说").Cursor(ui.CursorPointer)
	stage.HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind != ui.InputKeyDown || ev.Mods != 0 {
			return false
		}
		switch ev.Key {
		case ui.KeyRight, ui.KeyDown, ui.KeySpace, ui.KeyEnter, ui.KeyPageDown:
			a.nextSlide()
		case ui.KeyLeft, ui.KeyUp, ui.KeyPageUp:
			a.prevSlide()
		case ui.KeyHome:
			a.slideIndex, a.slideStep = 0, 0
		case ui.KeyEnd:
			a.slideIndex, a.slideStep = len(a.slides)-1, 0
		case ui.KeyEscape:
			a.stopPresenting()
		default:
			return false
		}
		return true
	})
	if stage.Clicked() {
		a.nextSlide()
	}
	sw, sh := w, h
	switch a.doc.Present.Aspect {
	case "16:9":
		sw, sh = fit(w, h, 16.0/9)
	case "4:3":
		sw, sh = fit(w, h, 4.0/3)
	}
	stage.Children(func() {
		if len(a.slides) == 0 {
			return
		}
		i := min(a.slideIndex, len(a.slides)-1)
		step := a.slideStep
		if !a.doc.Present.Reveal {
			step = -1
		}
		a.slideView(c.Key(fmt.Sprint("slide", i)), a.doc, a.slides[i], i, len(a.slides), sw, sh, step).PassThrough()
	})
}

// fit is the largest size of the ratio inside w×h.
func fit(w, h, ratio float32) (float32, float32) {
	if w/h > ratio {
		return h * ratio, h
	}
	return w, w / ratio
}

// presentPanel is the panel's 演说 tab: a preview of the slides and how
// they look.
func (a *app) presentPanel(c *ui.Context) {
	t := c.Theme()
	d := a.doc
	p := d.Present
	slides := slidesOf(d)
	set := func(key string, fn func(p *model.Present)) {
		a.mutate("present:"+key+":"+d.ID, func() { fn(&d.Present) })
	}
	ui.Column(c.Key("present")).Padding(16).Gap(12).Children(func() {
		ui.Text(c, "演说模式预览").FontSize(12).Bold().TextColor(t.TextMuted)
		// The slide of the selected node, else the first.
		i := 0
		for j, s := range slides {
			if s.node.ID == a.selected && s.kind != slideTitle {
				i = j
				break
			}
		}
		pw := a.panelWidth - 32
		ph := pw * 9 / 16
		ui.Box(c).Radius(10).Clip().Children(func() {
			a.slideView(c.Key("preview"), d, slides[i], i, len(slides), pw, ph, -1)
		})
		ui.Textf(c, "共 %d 页", len(slides)).FontSize(12).TextColor(t.TextMuted)

		theme := map[string]int{"": 0, "light": 1, "color": 2}[p.Theme]
		ui.Field(c, "风格", func() {
			if ui.Segmented(c, &theme, "深色", "浅色", "彩色").FillWidth().Changed() {
				v := []string{"", "light", "color"}[theme]
				set("theme", func(p *model.Present) { p.Theme = v })
			}
		})
		aspect := map[string]string{"": "自动", "16:9": "16:9", "4:3": "4:3"}[p.Aspect]
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "长宽比").Grow(1)
			if ui.Select(c, &aspect, []string{"自动", "16:9", "4:3"}).Label("长宽比").Width(110).Changed() {
				v := map[string]string{"自动": "", "16:9": "16:9", "4:3": "4:3"}[aspect]
				set("aspect", func(p *model.Present) { p.Aspect = v })
			}
		})
		layout := map[string]int{"": 0, "cards": 1}[p.Layout]
		ui.Field(c, "列表幻灯片", func() {
			if ui.Segmented(c, &layout, "列表", "卡片").FillWidth().Changed() {
				v := []string{"", "cards"}[layout]
				set("layout", func(p *model.Present) { p.Layout = v })
			}
		})
		reveal := p.Reveal
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "逐条出现").Grow(1)
			if ui.Switch(c, &reveal).Label("逐条出现").Changed() {
				v := reveal
				set("reveal", func(p *model.Present) { p.Reveal = v })
			}
		})
		ui.Divider(c)
		if ui.PrimaryButton(c, "▶ 从头开始演说").FillWidth().Clicked() {
			a.later(func() { a.startPresenting("") })
		}
		if ui.Button(c, "从选中的节点开始").FillWidth().Clicked() {
			sel := a.selected
			a.later(func() { a.startPresenting(sel) })
		}
		ui.Text(c, "演说时：← → 或空格翻页，Esc 退出").FontSize(12).TextColor(t.TextMuted)
	})
}
