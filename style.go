package main

import (
	"fmt"
	"strconv"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// scheme is a color scheme: a color for each main branch, in turn, and the
// central topic's.
type scheme struct {
	id, name string
	colors   []string
	root     string
}

// schemes are the color schemes a canvas can take; the first is the
// default.
var schemes = []scheme{
	{"", "晨曦", []string{"#F76D6D", "#FFA45B", "#7ECC9B", "#6FD3CB", "#68BFF2", "#D88BEA"}, "#1F1F1F"},
	{"classic", "经典", []string{"#3B82F6", "#10B981", "#F59E0B", "#EC4899", "#8B5CF6", "#06B6D4"}, "#111827"},
	{"ocean", "海洋", []string{"#0EA5E9", "#2563EB", "#06B6D4", "#0284C7", "#38BDF8", "#1D4ED8"}, "#0C4A6E"},
	{"forest", "森林", []string{"#16A34A", "#65A30D", "#059669", "#84CC16", "#0D9488", "#4D7C0F"}, "#14532D"},
	{"sunset", "暖阳", []string{"#F97316", "#EF4444", "#F59E0B", "#EC4899", "#FB7185", "#D97706"}, "#7C2D12"},
	{"candy", "糖果", []string{"#F472B6", "#A78BFA", "#60A5FA", "#34D399", "#FBBF24", "#FB7185"}, "#4C1D95"},
	{"business", "商务", []string{"#334155", "#1E40AF", "#0F766E", "#475569", "#1D4ED8", "#155E75"}, "#0F172A"},
}

func schemeOf(id string) scheme {
	for _, s := range schemes {
		if s.id == id {
			return s
		}
	}
	return schemes[0]
}

// singleColors are the colors offered for branches of one color.
var singleColors = []string{"#F76D6D", "#FFA45B", "#E9B308", "#22B573", "#4F86F7", "#7C5CE0", "#8E8E93"}

// fonts are the families offered for a canvas, by their names in the UI.
var fonts = []struct{ family, name string }{
	{"", "默认"},
	{"PingFang SC", "苹方"},
	{"Hiragino Sans GB", "冬青黑体"},
	{"Songti SC", "宋体"},
	{"Kaiti SC", "楷体"},
	{"Yuanti SC", "圆体"},
	{"Helvetica Neue", "Helvetica"},
	{"Menlo", "Menlo（等宽）"},
}

// lines are the widths offered for branches.
var lines = []struct {
	id, name string
	width    float32
}{
	{"thin", "细", 1},
	{"", "默认", 1.6},
	{"thick", "粗", 2.6},
	{"bolder", "特粗", 4},
}

// look is a canvas's style, resolved to what the canvas draws with.
type look struct {
	style  model.CanvasStyle
	scheme scheme
	font   string
	line   float32
}

func lookOf(s model.CanvasStyle) look {
	l := look{style: s, scheme: schemeOf(s.Scheme), font: s.Font, line: 1.6}
	for _, w := range lines {
		if w.id == s.Line {
			l.line = w.width
		}
	}
	return l
}

// branch is the color of the main branch at index i and everything on it.
func (l look) branch(i int) ui.Color {
	if l.style.SingleColor {
		if l.style.Color != "" {
			return ui.Hex(l.style.Color)
		}
		return ui.Hex(l.scheme.colors[0])
	}
	return ui.Hex(l.scheme.colors[i%len(l.scheme.colors)])
}

// root is the central topic's fill.
func (l look) root(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.Hex("#F2F2F2")
	}
	return ui.Hex(l.scheme.root)
}

// textOn is a text color that reads on the fill.
func textOn(fill ui.Color) ui.Color {
	c := fill.SRGB()
	if (0.299*float32(c.R)+0.587*float32(c.G)+0.114*float32(c.B))/255 > 0.62 {
		return ui.Hex("#1F2329")
	}
	return ui.Hex("#FFFFFF")
}

// setStyle changes the shown canvas's style, as one step of the history
// for each control.
func (a *app) setStyle(key string, fn func(s *model.CanvasStyle)) {
	d := a.doc
	a.mutate("style:"+key+":"+d.ID, func() { fn(&d.Style) })
}

// canvasPanel is the panel's 画布 tab: the shown canvas's structure,
// color scheme, font and branches.
func (a *app) canvasPanel(c *ui.Context) {
	t := c.Theme()
	st := a.doc.Style
	l := lookOf(st)
	section := func(title string) {
		ui.Text(c, title).FontSize(12).Bold().TextColor(t.TextMuted)
	}
	ui.Column(c.Key("canvas-style")).Padding(16).Gap(12).Children(func() {
		section("结构")
		ui.Row(c).Gap(6).Children(func() {
			for _, s := range model.Structures {
				card := ui.Column(c.Key(string(s))).Grow(1).Basis(0).Gap(4).Padding(6).Radius(8).
					Background(t.Background).Border(1, t.Border).Cursor(ui.CursorPointer).Label(s.Label())
				if s == a.doc.Structure {
					card.Border(2, t.Accent)
				}
				if card.Clicked() {
					a.setStructure(s)
				}
				card.Children(func() {
					ui.Box(c).Height(40).Draw(func(p *ui.Painter, r ui.Rect) {
						drawStructureIcon(p, r, s, t.TextMuted)
					})
					ui.Text(c, s.Label()).FontSize(11).TextAlign(ui.Center)
				})
			}
		})

		ui.Divider(c)
		section("配色方案")
		ui.Grid(c).Columns(2).Gap(8).Children(func() {
			for _, sc := range schemes {
				card := ui.Column(c.Key("scheme:"+sc.id)).Gap(6).Padding(8).Radius(8).
					Background(t.Background).Border(1, t.Border).Cursor(ui.CursorPointer).Label(sc.name)
				if sc.id == st.Scheme {
					card.Border(2, t.Accent)
				}
				if card.Clicked() {
					id := sc.id
					a.setStyle("scheme", func(s *model.CanvasStyle) { s.Scheme, s.SingleColor = id, false })
				}
				card.Children(func() {
					ui.Box(c).Height(46).Draw(func(p *ui.Painter, r ui.Rect) {
						drawSchemePreview(p, r, sc, t)
					})
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Box(c).Size(36, 8).Radius(4).Draw(func(p *ui.Painter, r ui.Rect) {
							w := r.W / float32(len(sc.colors))
							for i, col := range sc.colors {
								p.Fill(ui.Rect{X: r.X + w*float32(i), Y: r.Y, W: w + 0.5, H: r.H}, ui.Hex(col), 0)
							}
						}).Clip()
						ui.Text(c, sc.name).FontSize(12)
					})
				})
			}
		})

		ui.Divider(c)
		font := "默认"
		names := make([]string, len(fonts))
		for i, f := range fonts {
			names[i] = f.name
			if f.family == st.Font {
				font = f.name
			}
		}
		ui.Field(c, "全局字体", func() {
			if ui.Select(c, &font, names).Label("全局字体").Changed() {
				for _, f := range fonts {
					if f.name == font {
						family := f.family
						a.setStyle("font", func(s *model.CanvasStyle) { s.Font = family })
					}
				}
			}
		})

		line := "默认"
		lineNames := make([]string, len(lines))
		for i, w := range lines {
			lineNames[i] = w.name
			if w.id == st.Line {
				line = w.name
			}
		}
		ui.Field(c, "分支线粗细", func() {
			if ui.Select(c, &line, lineNames).Label("分支线粗细").Changed() {
				for _, w := range lines {
					if w.name == line {
						id := w.id
						a.setStyle("line", func(s *model.CanvasStyle) { s.Line = id })
					}
				}
			}
		})

		rainbow := !st.SingleColor
		toggle := func() {
			single := !st.SingleColor
			a.setStyle("rainbow", func(s *model.CanvasStyle) { s.SingleColor = single })
		}
		row := ui.Row(c).AlignItems(ui.Center).Cursor(ui.CursorPointer)
		if row.Clicked() { // the label toggles too
			toggle()
		}
		row.Children(func() {
			ui.Text(c, "彩虹分支").Grow(1)
			if ui.Switch(c, &rainbow).Label("彩虹分支开关").Changed() {
				toggle()
			}
		})
		if st.SingleColor {
			ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
				ui.Text(c, "颜色").Grow(1)
				for _, hex := range singleColors {
					dot := ui.Box(c.Key(hex)).Size(20, 20).Radius(999).Background(ui.Hex(hex)).
						Cursor(ui.CursorPointer).Label(hex)
					if l.branch(0) == ui.Hex(hex) {
						dot.Border(2, t.Text)
					}
					if dot.Clicked() {
						col := hex
						a.setStyle("color", func(s *model.CanvasStyle) { s.Color = col })
					}
				}
			})
		}
	})
}

// drawSchemePreview draws a small map in a color scheme: a dark central
// topic and three main topics filled with the scheme's first colors.
func drawSchemePreview(p *ui.Painter, r ui.Rect, sc scheme, t *ui.Theme) {
	l := lookOf(model.CanvasStyle{Scheme: sc.id})
	cx, cy := r.X+r.W*0.3, r.Y+r.H/2
	root := ui.Rect{X: cx - 18, Y: cy - 6, W: 36, H: 12}
	for i := range 3 {
		y := r.Y + 4 + float32(i)*(r.H-8)/2 - 4
		box := ui.Rect{X: r.X + r.W*0.62, Y: y, W: 30, H: 8}
		var path ui.Path
		path.MoveTo(root.X+root.W, cy)
		path.CubeTo(box.X-8, cy, root.X+root.W+8, y+4, box.X, y+4)
		p.StrokePath(&path, 1, l.branch(i))
		p.Fill(box, l.branch(i), 2)
	}
	p.Fill(root, l.root(t), 3)
}

// setNodeStyle changes the selected node's own style, as one step of the
// history for each control.
func (a *app) setNodeStyle(n *model.Node, key string, fn func(s *model.NodeStyle)) {
	a.mutate("node-style:"+key+":"+n.ID, func() {
		st := styleOf(n)
		fn(&st)
		if st == (model.NodeStyle{}) {
			n.Style = nil
		} else {
			n.Style = &st
		}
	})
}

func hexOf(c ui.Color) string {
	c = c.SRGB()
	return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
}

// nodeStylePanel is the panel's 样式 tab: the selected node's shape, fill,
// border, width and text.
func (a *app) nodeStylePanel(c *ui.Context) {
	t := c.Theme()
	n, _ := a.selectedNode()
	if n == nil {
		ui.Column(c).Padding(20).Children(func() {
			ui.Text(c, "选择一个节点设置样式").TextColor(t.TextMuted)
		})
		return
	}
	st := styleOf(n)
	section := func(title string) {
		ui.Text(c, title).FontSize(12).Bold().TextColor(t.TextMuted)
	}
	ui.Column(c.Key("node-style:" + n.ID)).Padding(16).Gap(12).Children(func() {
		section("形状")
		ui.Grid(c).Columns(3).Gap(6).Children(func() {
			for _, sh := range model.Shapes {
				b := ui.Box(c.Key(string(sh))).Height(44).Radius(8).Background(t.Background).
					Border(1, t.Border).Cursor(ui.CursorPointer).Label(sh.Label())
				if sh == st.Shape {
					b.Border(2, t.Accent)
				}
				if b.Clicked() {
					shape := sh
					a.setNodeStyle(n, "shape", func(s *model.NodeStyle) { s.Shape = shape })
				}
				b.Draw(func(p *ui.Painter, r ui.Rect) {
					drawShapeIcon(p, r, sh, t.TextMuted)
				})
			}
		})

		colorField := func(label, key string, value string, set func(s *model.NodeStyle, hex string)) {
			ui.Row(c.Key(key)).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, label).Grow(1)
				col := ui.Hex("#888888")
				if value != "" {
					col = ui.Hex(value)
				}
				if ui.ColorWell(c, &col).Label(label).Changed() {
					hex := hexOf(col)
					a.setNodeStyle(n, key, func(s *model.NodeStyle) { set(s, hex) })
				}
				auto := ui.Button(c, "自动").Disabled(value == "")
				if auto.Clicked() {
					a.setNodeStyle(n, key, func(s *model.NodeStyle) { set(s, "") })
				}
			})
		}
		colorField("填充", "fill", st.Fill, func(s *model.NodeStyle, hex string) { s.Fill = hex })
		colorField("边框", "border", st.Border, func(s *model.NodeStyle, hex string) { s.Border = hex })
		if st.Border != "" {
			bw := float64(max(st.BorderWidth, 1))
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "边框粗细").Grow(1)
				if ui.Stepper(c, &bw, 1, 6, 1).Label("边框粗细").Changed() {
					a.setNodeStyle(n, "border-width", func(s *model.NodeStyle) { s.BorderWidth = float32(bw) })
				}
				ui.Textf(c, "%.0f px", bw).FontSize(12).TextColor(t.TextMuted)
			})
		}

		ui.Divider(c)
		width := float64(st.Width)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "宽度").Grow(1)
			if st.Width == 0 {
				ui.Text(c, "适应文字").FontSize(12).TextColor(t.TextMuted)
				if ui.Button(c, "固定宽度").Clicked() {
					w, _ := a.titleBox(c, n, len(a.doc.Path(n.ID))-1)
					a.setNodeStyle(n, "width", func(s *model.NodeStyle) { s.Width = w })
				}
				return
			}
			if ui.NumberInput(c.Key(n.ID), &width, 40, 600, 10).Label("宽度").Width(120).Changed() {
				a.setNodeStyle(n, "width", func(s *model.NodeStyle) { s.Width = float32(width) })
			}
			if ui.Button(c, "适应").Clicked() {
				a.setNodeStyle(n, "width", func(s *model.NodeStyle) { s.Width = 0 })
			}
		})

		ui.Divider(c)
		section("文本")
		sizes := []string{"自动", "12", "14", "16", "18", "20", "24", "28", "32", "40"}
		size := "自动"
		if st.FontSize > 0 {
			size = fmt.Sprint(st.FontSize)
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "字号").Grow(1)
			if ui.Select(c, &size, sizes).Label("字号").Width(100).Changed() {
				v, _ := strconv.ParseFloat(size, 32)
				a.setNodeStyle(n, "font-size", func(s *model.NodeStyle) { s.FontSize = float32(v) })
			}
		})
		ui.Row(c).Gap(4).Children(func() {
			toggle := func(label, key string, on bool, set func(s *model.NodeStyle, on bool)) {
				v := on
				tg := ui.Toggle(c.Key(key), &v, label).Grow(1)
				if key == "bold" {
					tg.Bold()
				}
				if tg.Changed() {
					a.setNodeStyle(n, key, func(s *model.NodeStyle) { set(s, v) })
				}
			}
			toggle("B", "bold", st.Bold, func(s *model.NodeStyle, on bool) { s.Bold = on })
			toggle("I", "italic", st.Italic, func(s *model.NodeStyle, on bool) { s.Italic = on })
			toggle("S", "strike", st.Strike, func(s *model.NodeStyle, on bool) { s.Strike = on })
			toggle("U", "underline", st.Underline, func(s *model.NodeStyle, on bool) { s.Underline = on })
		})
		align := map[string]int{"": 0, "left": 0, "center": 1, "right": 2}[st.Align]
		if ui.Segmented(c, &align, "左对齐", "居中", "右对齐").FillWidth().Changed() {
			v := []string{"", "center", "right"}[align]
			a.setNodeStyle(n, "align", func(s *model.NodeStyle) { s.Align = v })
		}
		colorField("文字颜色", "color", st.Color, func(s *model.NodeStyle, hex string) { s.Color = hex })

		ui.Divider(c)
		if ui.Button(c, "清除样式").Disabled(n.Style == nil).Clicked() {
			a.mutate("", func() { n.Style = nil })
		}
	})
}

// drawShapeIcon draws a small box in a shape, with its name.
func drawShapeIcon(p *ui.Painter, r ui.Rect, sh model.Shape, col ui.Color) {
	box := ui.Rect{X: r.X + r.W/2 - 18, Y: r.Y + 7, W: 36, H: 16}
	switch sh {
	case model.ShapeAuto, model.ShapeRounded:
		p.Stroke(box, col, 5, 1.5)
	case model.ShapeRect:
		p.Stroke(box, col, 0, 1.5)
	case model.ShapePill:
		p.Stroke(box, col, 8, 1.5)
	case model.ShapeUnderline:
		p.Line(box.X, box.Y+box.H, box.X+box.W, box.Y+box.H, 1.5, col)
	case model.ShapeNone:
		p.StrokeDashed(box, col, 3, 1)
	}
	label := ui.Span{Text: sh.Label(), Size: 10, Color: col}
	w, _ := p.MeasureText(0, label)
	p.RichText(r.X+(r.W-w)/2, box.Y+box.H+3, 0, label)
}
