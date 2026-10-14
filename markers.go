package main

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// Markers are small icons before a node's title, as XMind's: "group:value",
// at most one of each group on a node.

// markerColors are the colors of the groups that come in colors.
var markerColors = []struct{ id, hex, name string }{
	{"red", "#F2484B", "红"},
	{"orange", "#FF9A3C", "橙"},
	{"yellow", "#F2C200", "黄"},
	{"green", "#20B26B", "绿"},
	{"blue", "#4F7EF7", "蓝"},
	{"purple", "#6B4FD8", "紫"},
	{"gray", "#8E8E93", "灰"},
}

// markerGroup is a group of markers, of which a node takes one.
type markerGroup struct {
	id, name string
	values   []string
}

var markerGroups = func() []markerGroup {
	colors := make([]string, len(markerColors))
	for i, c := range markerColors {
		colors[i] = c.id
	}
	var symbols []string
	for _, s := range symbolGlyphs {
		symbols = append(symbols, s.id)
	}
	return []markerGroup{
		{"dot", "标签", colors},
		{"priority", "优先级", []string{"1", "2", "3", "4", "5", "6", "7"}},
		{"task", "任务进度", []string{"0", "1", "2", "3", "4", "5", "6"}},
		{"flag", "旗帜", colors},
		{"star", "星星", colors},
		{"person", "人像", colors},
		{"symbol", "符号", symbols},
	}
}()

// symbolGlyphs are the symbols' pictures, white on their color: SVG paths
// in a 24×24 box, or a text.
var symbolGlyphs = []struct {
	id, name, hex string
	svg           string
	text          string
}{
	{"heart", "喜欢", "#F2484B", `<path d="M12 20.5s-7.5-4.8-7.5-10.2A4.3 4.3 0 0 1 12 7.6a4.3 4.3 0 0 1 7.5 2.7c0 5.4-7.5 10.2-7.5 10.2z"/>`, ""},
	{"like", "赞同", "#FF7A45", `<path d="M3.5 10.5h3.5v10H3.5z"/><path d="M8.5 10.5l3.6-6.3c1.3 0 2.3 1 2.3 2.3v2.9h4.6c1 0 1.7.9 1.5 1.8l-1.5 7.6c-.2.9-.9 1.5-1.8 1.5H8.5z"/>`, ""},
	{"dislike", "反对", "#4F7EF7", `<g transform="rotate(180 12 12)"><path d="M3.5 10.5h3.5v10H3.5z"/><path d="M8.5 10.5l3.6-6.3c1.3 0 2.3 1 2.3 2.3v2.9h4.6c1 0 1.7.9 1.5 1.8l-1.5 7.6c-.2.9-.9 1.5-1.8 1.5H8.5z"/></g>`, ""},
	{"pin", "置顶", "#F2484B", `<path d="M9 3.5h6l-1 5.5 3.5 3.5h-11L10 9z"/><path d="M11.2 12h1.6v8.5h-1.6z"/>`, ""},
	{"idea", "想法", "#F2C200", `<path d="M12 3a6.2 6.2 0 0 0-3.6 11.3V16.5h7.2v-2.2A6.2 6.2 0 0 0 12 3z"/><path d="M9.2 18h5.6v2.5H9.2z"/>`, ""},
	{"bolt", "紧急", "#4F7EF7", `<path d="M13.5 2.5L4.5 14h6.5l-1 7.5L19.5 10H13z"/>`, ""},
	{"wait", "等待", "#FF9A3C", `<path d="M6.5 3h11v2.2l-4.2 6.8 4.2 6.8V21h-11v-2.2l4.2-6.8-4.2-6.8z"/>`, ""},
	{"phone", "电话", "#20B26B", `<path d="M6 3.5h3.5l1.8 4.7-2.3 1.4a10.5 10.5 0 0 0 5.4 5.4l1.4-2.3 4.7 1.8V18a2.5 2.5 0 0 1-2.5 2.5A15 15 0 0 1 3.5 6 2.5 2.5 0 0 1 6 3.5z"/>`, ""},
	{"edit", "编辑", "#FF9A3C", `<path d="M15 4.5l4.5 4.5L9 19.5H4.5V15z"/>`, ""},
	{"music", "音乐", "#6B4FD8", `<path d="M8.5 5.5l11-2.5v12.5h-1.8V6.2l-7.4 1.7v10.1H8.5z"/><circle cx="6.5" cy="18" r="2.8"/><circle cx="16" cy="15.5" r="2.8"/>`, ""},
	{"game", "游戏", "#F2C200", `<path d="M7 7.5h10a4.5 4.5 0 0 1 4.5 4.5v1.2a3.8 3.8 0 0 1-6.6 2.6l-.9-1H10l-.9 1a3.8 3.8 0 0 1-6.6-2.6V12A4.5 4.5 0 0 1 7 7.5z"/>`, ""},
	{"plane", "出行", "#4F7EF7", `<path d="M21 15.5v-2l-7.5-4.7V4a1.5 1.5 0 0 0-3 0v4.8L3 13.5v2l7.5-2.3v4.6l-2 1.5v1.7l3.5-1 3.5 1v-1.7l-2-1.5v-4.6z"/>`, ""},
	{"hundred", "满分", "#F2484B", "", "100"},
	{"warn", "注意", "#F2484B", "", "!"},
	{"question", "疑问", "#4F7EF7", "", "?"},
}

// Pictures of the groups drawn from SVG, parsed once.
var (
	flagSVG   = parseGlyph(`<path d="M6 3h1.8v18H6z"/><path d="M8.5 4h10l-2.5 4.5 2.5 4.5h-10z"/>`)
	starSVG   = parseGlyph(`<path d="M12 2.8l2.8 5.9 6.4.8-4.7 4.4 1.2 6.3L12 17.1l-5.7 3.1 1.2-6.3-4.7-4.4 6.4-.8z"/>`)
	personSVG = parseGlyph(`<circle cx="12" cy="8" r="4.2"/><path d="M3.8 21c0-4.6 3.7-7.6 8.2-7.6s8.2 3 8.2 7.6z"/>`)
	checkSVG  = parseGlyph(`<path d="M9.6 17.3L4.4 12l1.7-1.7 3.5 3.5 8.3-8.3 1.7 1.7z"/>`)
	playSVG   = parseGlyph(`<path d="M9 6.5l9 5.5-9 5.5z"/>`)
	symbolSVG = func() map[string]*ui.SVG {
		m := map[string]*ui.SVG{}
		for _, s := range symbolGlyphs {
			if s.svg != "" {
				m[s.id] = parseGlyph(s.svg)
			}
		}
		return m
	}()
)

func parseGlyph(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor">` + body + `</svg>`))
}

func splitMarker(m string) (group, value string) {
	group, value, _ = strings.Cut(m, ":")
	return group, value
}

func colorHex(id string) string {
	for _, c := range markerColors {
		if c.id == id {
			return c.hex
		}
	}
	return "#8E8E93"
}

// markerName is a marker's name, as assistive technology reads it.
func markerName(m string) string {
	group, value := splitMarker(m)
	colorName := func() string {
		for _, c := range markerColors {
			if c.id == value {
				return c.name
			}
		}
		return ""
	}
	switch group {
	case "dot":
		return colorName() + "色标签"
	case "priority":
		return "优先级 " + value
	case "task":
		if value == "0" {
			return "任务开始"
		}
		if value == "6" {
			return "任务完成"
		}
		return "任务进度 " + value + "/6"
	case "flag":
		return colorName() + "旗"
	case "star":
		return colorName() + "星"
	case "person":
		return colorName() + "色人像"
	case "symbol":
		for _, s := range symbolGlyphs {
			if s.id == value {
				return s.name
			}
		}
	}
	return m
}

// drawMarker paints a marker in the circle of r.
func drawMarker(p *ui.Painter, r ui.Rect, m string) {
	group, value := splitMarker(m)
	white := ui.Hex("#FFFFFF")
	glyph := ui.Rect{X: r.X + r.W*0.2, Y: r.Y + r.H*0.2, W: r.W * 0.6, H: r.H * 0.6}
	disc := func(hex string) { p.Fill(r, ui.Hex(hex), r.W/2) }
	text := func(s string, hex string) {
		disc(hex)
		span := ui.Span{Text: s, Size: r.H * 0.62, Weight: 800, Color: white}
		if len(s) > 1 {
			span.Size = r.H * 0.42
		}
		w, h := p.MeasureText(0, span)
		p.RichText(r.X+(r.W-w)/2, r.Y+(r.H-h)/2, 0, span)
	}
	switch group {
	case "dot":
		disc(colorHex(value))
	case "priority":
		i, _ := strconv.Atoi(value)
		text(value, markerColors[min(max(i-1, 0), len(markerColors)-1)].hex)
	case "task":
		const green = "#20B26B"
		i, _ := strconv.Atoi(value)
		switch i {
		case 0:
			p.Stroke(r, ui.Hex(green), r.W/2, max(r.W*0.09, 1))
			p.Icon(playSVG, glyph, ui.Hex(green))
		case 6:
			disc(green)
			p.Icon(checkSVG, glyph, white)
		default:
			// A pie of i sixths, from the top, clockwise.
			p.Stroke(r, ui.Hex(green), r.W/2, max(r.W*0.09, 1))
			cx, cy, rad := r.X+r.W/2, r.Y+r.H/2, r.W/2
			var pie ui.Path
			pie.MoveTo(cx, cy)
			pie.LineTo(cx, cy-rad)
			end := float64(i) / 6 * 2 * math.Pi
			for a := 0.0; a <= end+1e-6; a += math.Pi / 24 {
				pie.LineTo(cx+rad*float32(math.Sin(a)), cy-rad*float32(math.Cos(a)))
			}
			pie.LineTo(cx+rad*float32(math.Sin(end)), cy-rad*float32(math.Cos(end)))
			pie.Close()
			p.FillPath(&pie, ui.Hex(green))
		}
	case "flag", "star", "person":
		disc(colorHex(value))
		svg := map[string]*ui.SVG{"flag": flagSVG, "star": starSVG, "person": personSVG}[group]
		p.Icon(svg, glyph, white)
	case "symbol":
		for _, s := range symbolGlyphs {
			if s.id != value {
				continue
			}
			if s.text != "" {
				text(s.text, s.hex)
			} else {
				disc(s.hex)
				p.Icon(symbolSVG[s.id], glyph, white)
			}
		}
	}
}

// markerView is a marker the size of size, drawn in a box.
func markerView(c *ui.Context, m string, size float32) ui.Element {
	return ui.Box(c.Key("marker:"+m)).Size(size, size).Shrink(0).Label(markerName(m)).
		Draw(func(p *ui.Painter, r ui.Rect) { drawMarker(p, r, m) })
}

// toggleMarker puts a marker on the node, in place of another of its group,
// or takes it off.
func (a *app) toggleMarker(n *model.Node, m string) {
	a.mutate("", func() {
		group, _ := splitMarker(m)
		had := slices.Contains(n.Markers, m)
		n.Markers = slices.DeleteFunc(n.Markers, func(x string) bool {
			g, _ := splitMarker(x)
			return g == group
		})
		if !had {
			n.Markers = append(n.Markers, m)
			n.KeepStatusMarker(m) // the bulb or the progress, not both
			slices.SortStableFunc(n.Markers, func(x, y string) int {
				return groupIndex(x) - groupIndex(y)
			})
		}
		if len(n.Markers) == 0 {
			n.Markers = nil
		}
		n.FollowProgress(a.now()) // a progress marker makes it a task
	})
}

func groupIndex(m string) int {
	g, _ := splitMarker(m)
	for i, mg := range markerGroups {
		if mg.id == g {
			return i
		}
	}
	return len(markerGroups)
}

// nextMarker is the marker after m in its group, for a click on a node's
// marker; the last goes back to the first.
func nextMarker(m string) string {
	g, v := splitMarker(m)
	for _, mg := range markerGroups {
		if mg.id == g {
			i := slices.Index(mg.values, v)
			return g + ":" + mg.values[(i+1)%len(mg.values)]
		}
	}
	return m
}

// markerPanel is the panel's 标记 tab: the markers of each group, which a
// click puts on the selected node or takes off, and the legend of those the
// canvas uses.
func (a *app) markerPanel(c *ui.Context) {
	t := c.Theme()
	n, _ := a.selectedNode()
	ui.Column(c.Key("markers")).Padding(16).Gap(12).Children(func() {
		if n == nil {
			ui.Text(c, "选择一个节点添加标记").TextColor(t.TextMuted)
		}
		a.markerLegend(c)
		for _, g := range markerGroups {
			ui.Text(c, g.name).FontSize(12).Bold().TextColor(t.TextMuted)
			ui.Row(c.Key(g.id)).Wrap().Gap(6).Children(func() {
				for _, v := range g.values {
					m := g.id + ":" + v
					on := n != nil && slices.Contains(n.Markers, m)
					b := ui.Box(c.Key(m)).Padding(3).Radius(999).Cursor(ui.CursorPointer).
						Label(markerName(m)).Tooltip(markerName(m))
					if on {
						b.Border(2, t.Accent)
					} else if b.Hovered() {
						b.Background(t.SurfaceHover)
					}
					if b.Clicked() && n != nil {
						a.toggleMarker(n, m)
					}
					b.Children(func() { markerView(c, m, 26).PassThrough() })
				}
			})
		}
	})
}

// markerLegend lists the markers the canvas uses, with how many nodes
// carry each.
func (a *app) markerLegend(c *ui.Context) {
	if a.doc == nil {
		return
	}
	t := c.Theme()
	counts := map[string]int{}
	var used []string
	a.doc.Walk(func(n, _ *model.Node, _ int) bool {
		for _, m := range n.Markers {
			if counts[m] == 0 {
				used = append(used, m)
			}
			counts[m]++
		}
		return true
	})
	if len(used) == 0 {
		return
	}
	slices.SortStableFunc(used, func(x, y string) int { return groupIndex(x) - groupIndex(y) })
	ui.Column(c.Key("legend")).Gap(6).Padding(10).Radius(8).Background(t.Background).Border(1, t.Border).Children(func() {
		ui.Text(c, "图例").FontSize(12).Bold().TextColor(t.TextMuted)
		for _, m := range used {
			ui.Row(c.Key(m)).Gap(8).AlignItems(ui.Center).Children(func() {
				markerView(c, m, 16)
				ui.Text(c, markerName(m)).FontSize(12).Grow(1)
				ui.Text(c, fmt.Sprint(counts[m])).FontSize(12).TextColor(t.TextMuted)
			})
		}
	})
}
