package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
)

// A clip as a card to share: its words on a page of one of a few styles,
// with its date and where it came from, saved as a PNG at twice its size.

// cardStyle is a look of a card.
type cardStyle struct {
	name                string
	page, paper         ui.Color // around the card, and the card
	text, muted, accent ui.Color
	rule                ui.Color // the lines of the letter paper
	kind                string   // "mint", "letter" or "night"
}

var cardStyles = []cardStyle{
	{name: "清新", kind: "mint", page: ui.Hex("#d5eee4"), paper: ui.Hex("#ffffff"), text: ui.Hex("#3d3d3d"), muted: ui.Hex("#a3a3a3"), accent: ui.Hex("#3cc47c")},
	{name: "信笺", kind: "letter", page: ui.Hex("#f4f2ee"), paper: ui.Hex("#fffbf5"), text: ui.Hex("#5a3e1b"), muted: ui.Hex("#a99f8d"), accent: ui.Hex("#b08d57"), rule: ui.Hex("#dcd7ca")},
	{name: "夜读", kind: "night", page: ui.Hex("#15181d"), paper: ui.Hex("#1f242b"), text: ui.Hex("#ecebe7"), muted: ui.Hex("#8b919a"), accent: ui.Hex("#e3b95f")},
}

// cardWidth is the card's width, in points; it saves at twice that.
const cardWidth = 600

// cardOpts is what a card shows.
type cardOpts struct {
	style    int
	heading  string // the line above the words, in the mint style
	text     string
	date     string
	source   string // where it came from, at the foot; "" for none
	fontSize float32
}

// cardText is what of a clip a card shows: its words, else its title and
// what the AI said of it.
func cardText(c *library.Clip) string {
	switch {
	case c.Kind == library.KindText && strings.TrimSpace(c.Text) != "":
		return strings.TrimSpace(c.Text)
	case c.Note != "":
		return strings.TrimSpace(c.Note)
	}
	var parts []string
	for _, s := range []string{c.Title, c.Desc} {
		if s = strings.TrimSpace(s); s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		parts = append(parts, c.Label())
	}
	return strings.Join(parts, "\n")
}

// cardSource is where a clip came from, as a card's foot says it.
func cardSource(c *library.Clip) string {
	if c.URL != "" {
		if u, err := url.Parse(c.URL); err == nil && u.Host != "" {
			label := strings.TrimPrefix(u.Host, "www.")
			if c.Title != "" {
				label = c.Title + " · " + label
			}
			return label
		}
	}
	if c.Name != "" {
		return c.Name
	}
	return ""
}

// cardSize is the text's size for its length: large for a line, smaller
// for a page.
func cardSize(text string) float32 {
	switch n := utf8.RuneCountInString(text); {
	case n < 60:
		return 25
	case n < 160:
		return 22
	case n < 400:
		return 19
	}
	return 17
}

// cardView draws a card. bound, when not nil, is given the card's box.
func cardView(c *ui.Context, o cardOpts, bound *ui.Handle) {
	st := cardStyles[min(o.style, len(cardStyles)-1)]
	size := o.fontSize
	if size == 0 {
		size = cardSize(o.text)
	}
	page := ui.Column(c.Key("card")).Width(cardWidth).Padding(28).Background(st.page)
	if bound != nil {
		page.Bind(bound)
	}
	paras := strings.Split(strings.TrimSpace(o.text), "\n")
	words := func(c *ui.Context, col ui.Color) {
		ui.Column(c).Gap(size * 0.6).Children(func() {
			for i, p := range paras {
				if strings.TrimSpace(p) == "" {
					continue
				}
				ui.Text(c.Key(i), strings.TrimSpace(p)).FontSize(size).LineHeight(1.85).TextColor(col).Wrap()
			}
		})
	}
	foot := func(c *ui.Context) {
		if o.source != "" {
			ui.Text(c, o.source).FontSize(12).TextColor(st.muted).MaxLines(1).Ellipsis("…").Grow(1).Shrink(1).MinWidth(0)
		} else {
			ui.Spacer(c)
		}
	}
	page.Children(func() {
		switch st.kind {
		case "mint":
			// A sheet with a raised tab on its left, and a pencil topped by a
			// flower in the notch on its right.
			body := ui.Column(c).Padding(46, 40, 30).Gap(26).Margin(52, 0, 0)
			body.Draw(func(p *ui.Painter, r ui.Rect) {
				const rad, drop = 22, 46 // the corners; how far the right of the top is below its left
				x0, y0, x1, y1 := r.X, r.Y, r.X+r.W, r.Y+r.H
				tab := x0 + r.W*0.64 // where the top starts down
				low := x0 + r.W*0.82 // and is down
				sheet := new(ui.Path).MoveTo(x0+rad, y0).LineTo(tab, y0).
					CubeTo(tab+r.W*0.08, y0, low-r.W*0.08, y0+drop, low, y0+drop).
					LineTo(x1-rad, y0+drop).QuadTo(x1, y0+drop, x1, y0+drop+rad).
					LineTo(x1, y1-rad).QuadTo(x1, y1, x1-rad, y1).
					LineTo(x0+rad, y1).QuadTo(x0, y1, x0, y1-rad).
					LineTo(x0, y0+rad).QuadTo(x0, y0, x0+rad, y0).Close()
				p.FillPath(sheet, st.paper)
				drawPencilFlower(p, x1-58, y0+drop, st)
			})
			body.Children(func() {
				ui.Text(c, cmpOr(o.heading, "写下一个想法")).FontSize(15).TextColor(st.muted)
				words(c, st.text)
				ui.Row(c).Gap(12).AlignItems(ui.Center).Margin(18, 0, 0).Children(func() {
					foot(c)
					if o.date != "" {
						ui.Text(c, o.date).FontSize(14).TextColor(st.muted).NoWrap()
					}
				})
			})
		case "letter":
			rules := func(top bool) {
				ui.Box(c).Height(9).FillWidth().Draw(func(p *ui.Painter, r ui.Rect) {
					thick, thin := r.Y+1.5, r.Y+7
					if !top {
						thick, thin = r.Y+7.5, r.Y+2
					}
					p.Line(r.X, thick, r.X+r.W, thick, 3, st.rule)
					p.Line(r.X, thin, r.X+r.W, thin, 1, st.rule)
				})
			}
			ui.Column(c).Padding(40, 40, 36).Gap(22).Background(st.paper).Shadow(0, 2, 10, 0, ui.RGBA(0, 0, 0, 0.08)).Children(func() {
				if o.date != "" {
					ui.Text(c, o.date).FontSize(14).TextColor(st.muted)
				}
				rules(true)
				ui.Box(c).Padding(10, 0, 24).Children(func() { words(c, st.text) })
				rules(false)
				if o.source != "" {
					ui.Row(c).Children(func() { foot(c) })
				}
			})
		default: // night
			ui.Column(c).Padding(36, 40, 32).Gap(18).Background(st.paper).Radius(16).Children(func() {
				ui.Text(c, "“").FontSize(56).TextColor(st.accent).Bold().Margin(0, 0, -26)
				words(c, st.text)
				ui.Box(c).Height(1).Width(40).Background(st.accent.Alpha(0.6)).Margin(8, 0, 0)
				ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
					foot(c)
					if o.date != "" {
						ui.Text(c, o.date).FontSize(13).TextColor(st.muted).NoWrap()
					}
				})
			})
		}
	})
}

// drawPencilFlower draws the mint card's pencil, standing at x, y (its
// tip's base on the sheet's notch), with a flower on a stem above it.
func drawPencilFlower(p *ui.Painter, x, y float32, st cardStyle) {
	ink := ui.Hex("#3a3a3a")
	// The pencil: a body and a point, upright.
	body := new(ui.Path).MoveTo(x-9, y).LineTo(x-9, y-26).LineTo(x+9, y-26).LineTo(x+9, y).Close()
	p.FillPath(body, ui.Hex("#ffffff"))
	p.StrokePath(body, 2.2, ink)
	p.Line(x, y, x, y-26, 1.6, ink)
	point := new(ui.Path).MoveTo(x-9, y-26).LineTo(x, y-40).LineTo(x+9, y-26).Close()
	p.FillPath(point, ui.Hex("#ffffff"))
	p.StrokePath(point, 2.2, ink)
	p.FillPath(new(ui.Path).MoveTo(x-3, y-35).LineTo(x, y-40).LineTo(x+3, y-35).Close(), ink)
	// The stem and the flower.
	stem := new(ui.Path).MoveTo(x, y-40).CubeTo(x+2, y-52, x+10, y-58, x+20, y-60)
	p.StrokePath(stem, 2, ink)
	cx, cy := x+30, y-70
	for i := range 6 {
		a := float64(i) * 3.14159 / 3
		px, py := cx+float32(9*cosf(a)), cy+float32(9*sinf(a))
		p.FillPath(new(ui.Path).Circle(px, py, 7.5), st.accent)
		p.StrokePath(new(ui.Path).Circle(px, py, 7.5), 2, ink)
	}
	p.FillPath(new(ui.Path).Circle(cx, cy, 8), st.accent)
	p.FillPath(new(ui.Path).Circle(cx, cy, 5), ui.Hex("#ffffff"))
	p.StrokePath(new(ui.Path).Circle(cx, cy, 5), 2, ink)
}

// cardImage draws a card into a PNG, at twice its size.
func cardImage(o cardOpts) (data []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	var bound ui.Handle
	var box ui.Rect
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Children(func() { cardView(c, o, &bound) })
		box = bound.Bounds(c)
	}, cardWidth, 4000)
	tt.SetScale(exportScale)
	tt.Frame()
	tt.Frame()
	img := tt.Image()
	if box.H <= 0 {
		return nil, fmt.Errorf("卡片没有画出来")
	}
	rect := image.Rect(int(box.X*exportScale), int(box.Y*exportScale), int((box.X+box.W)*exportScale), int((box.Y+box.H)*exportScale)).Intersect(img.Bounds())
	var b bytes.Buffer
	if err := png.Encode(&b, img.SubImage(rect)); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// openCard opens the window to make a card of a clip.
func (a *app) openCard(clip *library.Clip) {
	a.cardFor = clip.ID
	a.cardOpts = cardOpts{style: a.cfg.CardStyle, heading: "", text: cardText(clip), date: clip.Created.Format("2006-01-02"), source: cardSource(clip)}
	a.cardShowDate, a.cardShowSource = true, a.cardOpts.source != ""
	a.cardSource = a.cardOpts.source
	a.cardOpen = true
}

// cardWindow is the window of a card: its style, its words, and saving it.
func (a *app) cardWindow(c *ui.Context) {
	if !a.cardOpen {
		return
	}
	t := c.Theme()
	clip := a.lib.Find(a.cardFor)
	if clip == nil {
		a.cardOpen = false
		return
	}
	o := a.cardOpts
	if !a.cardShowDate {
		o.date = ""
	}
	o.source = ""
	if a.cardShowSource {
		o.source = a.cardSource
	}
	ui.Modal(c.Key("card-window"), &a.cardOpen, func() {
		w, h := c.Size()
		ui.Row(c).Gap(20).AlignItems(ui.Start).Children(func() {
			ui.Scroll(c.Key("card-preview")).MaxHeight(h - 140).Width(cardWidth + 8).Children(func() {
				cardView(c, o, nil)
			})
			ui.Column(c).Gap(14).Width(min(280, w-cardWidth-160)).Children(func() {
				ui.Text(c, "分享卡片").FontSize(17).Bold()
				ui.Field(c, "样式", func() {
					names := make([]string, len(cardStyles))
					for i, s := range cardStyles {
						names[i] = s.name
					}
					if ui.Segmented(c, &a.cardOpts.style, names...).FillWidth().Changed() {
						a.cfg.CardStyle = a.cardOpts.style
						a.cfg.save()
					}
				})
				ui.Field(c, "内容", func() {
					ui.TextArea(c.Key("card-text"), &a.cardOpts.text).Lines(4, 12).Label("卡片内容")
				})
				if cardStyles[a.cardOpts.style].kind == "mint" {
					ui.Field(c, "题记", func() {
						ui.TextInput(c.Key("card-heading"), &a.cardOpts.heading).Placeholder("写下一个想法").Label("卡片题记")
					})
				}
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Checkbox(c, &a.cardShowDate, "日期")
					if a.cardSource != "" {
						ui.Checkbox(c, &a.cardShowSource, "来源")
					}
				})
				ui.Row(c).Gap(8).Children(func() {
					if ui.PrimaryButton(c, "保存图片…").Clicked() {
						a.saveCard(o, clip)
					}
					if iconButton(c, iconClose, "关闭", "关闭（Esc）", 28, true).Clicked() {
						a.cardOpen = false
					}
				})
				ui.Text(c, "保存为 2 倍清晰度的 PNG，可以直接发朋友圈或小红书。").FontSize(11).TextColor(t.TextMuted).Wrap()
			})
		})
	})
}

// saveCard asks where to save a card and saves it.
func (a *app) saveCard(o cardOpts, clip *library.Clip) {
	name := clip.Label()
	if r := []rune(name); len(r) > 20 {
		name = string(r[:20])
	}
	a.plat.SaveFile(name+".png", "png", func(path string) {
		if path == "" {
			return
		}
		if !strings.EqualFold(filepath.Ext(path), ".png") {
			path += ".png"
		}
		data, err := cardImage(o)
		if err == nil {
			err = os.WriteFile(path, data, 0o644)
		}
		if err != nil {
			a.libErr = "保存图片失败：" + err.Error()
			return
		}
		a.tell("已保存卡片")
		a.plat.ShowInFolder(path)
	})
}

func cosf(a float64) float64 { return math.Cos(a) }
func sinf(a float64) float64 { return math.Sin(a) }
