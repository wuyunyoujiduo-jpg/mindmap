package main

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/layout"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// nodeDrag is the value a node carries while it is dragged onto another.
type nodeDrag struct{ ID string }

// Sizes of a node's box at zoom 1.
const (
	nodePadX   = 14
	nodePadY   = 8
	nodeGap    = 6
	nodeHeight = 34
	rootHeight = 46
	titleMax   = 260
	titleMin   = 24
	markSize   = 11
	checkSize  = 15 // a task's check box
)

func titleSize(depth int) float32 {
	switch depth {
	case 0:
		return 18
	case 1:
		return 15
	}
	return 14
}

// nodeParts are what a node shows besides its title.
type nodeParts struct {
	title string
	tags  []string // at most two shown
	more  int      // tags not shown
	due   string
	note  bool
}

func (a *app) parts(n *model.Node) nodeParts {
	p := nodeParts{title: n.Title, note: n.Note != ""}
	if a.editing == n.ID {
		p.title = a.editText
	}
	if p.title == "" {
		p.title = " "
	}
	p.tags = n.Tags
	if len(p.tags) > 2 {
		p.tags, p.more = p.tags[:2], len(p.tags)-2
	}
	if n.Task != nil && n.Task.Due != nil {
		p.due = n.Task.Due.Format("1/2")
	}
	return p
}

// measure sizes a node's box at zoom 1, as nodeView draws it.
// measureKey is a text measured, with how.
type measureKey struct {
	text, font   string
	size, wrap   float32
	weight       int
	italic, bold bool
}

// measureText is c.MeasureText, kept: the layout measures every node of
// the map at every frame, and their titles seldom change.
func (a *app) measureText(c *ui.Context, wrap float32, span ui.Span) (float32, float32) {
	k := measureKey{text: span.Text, font: span.Font, size: span.Size, wrap: wrap, weight: int(span.Weight), italic: span.Italic}
	if v, ok := a.measured[k]; ok {
		return v[0], v[1]
	}
	w, h := c.MeasureText(wrap, span)
	if a.measured == nil || len(a.measured) > 50000 {
		a.measured = map[measureKey][2]float32{}
	}
	a.measured[k] = [2]float32{w, h}
	return w, h
}

func (a *app) measure(c *ui.Context) layout.Measure {
	return func(n *model.Node, depth int) (float32, float32) {
		p := a.parts(n)
		tw, th := a.titleBox(c, n, depth)
		w := 2*nodePadX + tw
		if n.Task != nil {
			w += checkSize + nodeGap
			if a.blocked(n) {
				mw, _ := a.measureText(c, 0, ui.Span{Text: "🔒", Size: markSize})
				w += mw + nodeGap
			}
		}
		w += float32(len(n.Markers)) * (markerSize(depth) + nodeGap)
		for _, tag := range p.tags {
			mw, _ := a.measureText(c, 0, ui.Span{Text: tag, Size: markSize})
			w += nodeGap + mw + markSize*1.1
		}
		if p.more > 0 {
			mw, _ := a.measureText(c, 0, ui.Span{Text: fmt.Sprintf("+%d", p.more), Size: markSize})
			w += nodeGap + mw
		}
		if p.due != "" {
			mw, _ := a.measureText(c, 0, ui.Span{Text: p.due, Size: markSize})
			w += nodeGap + mw
		}
		if pg := a.progress[n.ID]; pg[1] > 0 {
			mw, _ := a.measureText(c, 0, ui.Span{Text: progressText(pg), Size: markSize})
			w += nodeGap + markSize + 3 + mw
		}
		if p.note {
			w += nodeGap + markSize + 3
		}
		if len(n.Links) > 0 {
			mw, _ := a.measureText(c, 0, ui.Span{Text: "↗", Size: markSize + 1})
			w += nodeGap + mw
		}
		if m := filesMark(n); m != "" {
			mw, _ := a.measureText(c, 0, ui.Span{Text: m, Size: markSize})
			w += nodeGap + mw
		}
		h := float32(nodeHeight)
		if depth == 0 {
			h = rootHeight
		}
		h = max(h, th+2*nodePadY)
		if iw, ih := thumbSize(n.Image); iw > 0 { // the picture above the title
			w = max(w, iw+2*nodePadX)
			h += ih + thumbGap
		}
		return w, h
	}
}

// titleBox is the size of a node's title: one line up to titleMax wide,
// and longer titles wrapped onto more lines at that width. A title being
// edited wraps the same way, so what is typed first stays in view.
func (a *app) titleBox(c *ui.Context, n *model.Node, depth int) (w, h float32) {
	st := styleOf(n)
	span := ui.Span{Text: a.parts(n).title, Size: fontSize(n, depth), Weight: fontWeight(n, depth),
		Italic: st.Italic, Font: a.doc.Style.Font}
	if a.editing == n.ID {
		// As wide as what is typed (or the placeholder), with room for the
		// caret: the node keeps its size as editing starts, and grows as
		// the title does.
		if a.editText == "" {
			span.Text = "新主题"
		}
		wrap := float32(titleMax)
		if st.Width > 0 {
			wrap = st.Width
		}
		w, h = a.measureText(c, 0, span)
		if w > wrap {
			_, h = a.measureText(c, wrap, span)
			w = wrap
		}
		return max(w, titleMin) + 4, h
	}
	w, h = a.measureText(c, 0, span)
	if st.Width > 0 { // a width of its own, at which the title wraps
		_, h = a.measureText(c, st.Width, span)
		return st.Width, h
	}
	if w > titleMax {
		w, h = a.measureText(c, titleMax, span)
		w = titleMax
	}
	return max(w, titleMin), h
}

// styleOf is the node's own style, zero where it follows the canvas.
func styleOf(n *model.Node) model.NodeStyle {
	if n.Style == nil {
		return model.NodeStyle{}
	}
	return *n.Style
}

func fontSize(n *model.Node, depth int) float32 {
	if s := styleOf(n).FontSize; s > 0 {
		return s
	}
	return titleSize(depth)
}

func fontWeight(n *model.Node, depth int) int {
	if styleOf(n).Bold {
		return 700
	}
	return weight(depth)
}

// markerSize is the size of a node's markers at zoom 1.
func markerSize(depth int) float32 {
	if depth <= 1 {
		return 20
	}
	return 18
}

func weight(depth int) int {
	if depth == 0 {
		return 700
	}
	if depth == 1 {
		return 600
	}
	return 400
}

// taskSummary describes a task in a line, as the dot of its node shows it
// on hover: "任务：进行中 · 优先级 高 · 截止 10/12（已逾期）".
func (a *app) taskSummary(task *model.Task) string {
	parts := []string{"任务：" + task.Status.Label()}
	if task.Priority != model.PriorityNone {
		parts = append(parts, "优先级 "+task.Priority.Label())
	}
	if task.Start != nil {
		parts = append(parts, "开始 "+task.Start.Format("1/2"))
	}
	if task.Due != nil {
		due := "截止 " + task.Due.Format("1/2")
		if task.Overdue(a.now()) {
			due += "（已逾期）"
		}
		parts = append(parts, due)
	}
	if task.Estimate > 0 {
		parts = append(parts, "估时 "+strconv.FormatFloat(task.Estimate, 'f', -1, 64)+" 小时")
	}
	if a.file != nil {
		if it := a.file.Iteration(task.Iteration); it != nil {
			parts = append(parts, it.Name)
		}
	}
	return strings.Join(parts, " · ")
}

func statusColor(s model.Status) ui.Color {
	switch s {
	case model.StatusIdea:
		return ui.Hex("#a78bfa")
	case model.StatusTodo:
		return ui.Hex("#3b82f6")
	case model.StatusDoing:
		return ui.Hex("#f59e0b")
	case model.StatusDone:
		return ui.Hex("#22c55e")
	}
	return ui.Hex("#9ca3af")
}

// canvasView is the map on an endless canvas: dragging the central node,
// the wheel or the touchpad move it, and Cmd with the wheel zooms. Below it are
// the tabs of the file's canvases.
func (a *app) canvasView(c *ui.Context) {
	ui.Column(c).Grow(1).Children(func() {
		a.canvasArea(c)
		ui.Divider(c)
		a.sheetTabs(c)
	})
}

func (a *app) canvasArea(c *ui.Context) {
	t := c.Theme()
	cam := a.cam()
	z := cam.Zoom
	lk := lookOf(a.doc.Style)
	shown := a.shownDoc()
	if shown.Path(a.selected) == nil && a.editing == "" { // left outside the branch focused on
		a.selected, a.multi = shown.Root.ID, nil
	}
	a.progress = progressOf(shown.Root)
	r := layout.Compute(shown, a.measure(c), layoutOptions(a.doc.Structure))
	if shown == a.doc {
		r = a.layoutFloating(r, a.measure(c), layoutOptions(a.doc.Structure))
	}
	a.boxes = r
	exporting := a.exportCam != nil // drawn into an image, off screen
	if exporting {
		a.reveal = false
	} else {
		a.vp = a.canvas.Bounds(c) // the canvas's box, as of the last frame
		a.mods = c.Modifiers()    // held now; the view builds again as they change
	}

	// Put the map at the left middle the first time, and keep the node
	// selected by keys in view.
	if exporting {
	} else if a.vp.W > 0 {
		if root, ok := r.Boxes[shown.Root.ID]; ok && !cam.placed {
			// The mind map centers its root; the one-sided structures
			// center the whole map while it fits, else start at the left.
			cam.X = a.vp.W/2 - (root.X+root.W/2)*z
			cam.Y = a.vp.H/2 - (root.Y+root.H/2)*z
			switch a.doc.Structure {
			case model.StructureMindMap, model.StructureOrg:
			case model.StructureFishbone: // the head on the right
				cam.X = min((a.vp.W-r.Width*z)/2, a.vp.W-20-r.Width*z)
			default:
				cam.X = max((a.vp.W-r.Width*z)/2, 20)
			}
			if a.doc.Structure == model.StructureOrg {
				cam.Y = max((a.vp.H-r.Height*z)/2, 60) // from the top
			}
			cam.placed = true
		}
		target := a.selected
		if a.editing != "" {
			target = a.editing
		}
		if b, ok := r.Boxes[target]; ok && a.reveal {
			a.reveal = false
			cam.X += keepIn(cam.X+b.X*z, b.W*z, a.vp.W)
			cam.Y += keepIn(cam.Y+b.Y*z, b.H*z, a.vp.H)
		}
	} else {
		c.Invalidate() // lay out once more, with the canvas's size
	}

	branch := map[string]int{}
	shown.Walk(func(n, p *model.Node, _ int) bool {
		switch {
		case p == nil && n != shown.Root: // a floating topic: a color after the branches'
			branch[n.ID] = len(shown.Root.Children) + slices.Index(shown.Floating, n)
		case p == nil:
		case p == shown.Root:
			branch[n.ID] = slices.Index(p.Children, n)
		default:
			branch[n.ID] = branch[p.ID]
		}
		return true
	})

	cv := ui.Box(c.Key("canvas")).Grow(1).Clip().Background(t.Background).Label("导图画布")
	if exporting {
		cv.Fill()
	} else {
		cv.Focusable().FocusRing(false).Bind(&a.canvas)
	}
	cv.HandleInput(a.canvasInput)
	if cv.Pressed() && a.editing == "" {
		a.canvas.Focus()
	}
	if cv.Clicked() {
		if !a.justFloated { // not the end of the double click that made one
			a.commitEditIfAny()
		}
		a.justFloated = false
		a.multi = nil
	}
	if cv.DoubleClicked() && !exporting && shown == a.doc && !a.overNode() {
		x, y := a.pointer[0], a.pointer[1]
		a.later(func() { a.addFloatingAt(x, y) }) // a floating topic where it was double-clicked
	}
	cv.Draw(func(p *ui.Painter, rect ui.Rect) {
		// A grid of dots, which moves with the map.
		step := 24 * z
		for step < 12 {
			step *= 2
		}
		dot := t.Border
		x0 := rect.X + mod(cam.X, step)
		y0 := rect.Y + mod(cam.Y, step)
		for y := y0; y < rect.Y+rect.H && !exporting; y += step {
			for x := x0; x < rect.X+rect.W; x += step {
				p.Fill(ui.Rect{X: x - 1, Y: y - 1, W: 2, H: 2}, dot, 1)
			}
		}
		color := func(id string) ui.Color {
			col := lk.branch(branch[id])
			if a.filtering() {
				col = col.Alpha(0.3)
			}
			return col
		}
		a.drawBoundaries(p, r, shown, rect.X+cam.X, rect.Y+cam.Y, z, color)
		view := rect
		if exporting {
			view = ui.Rect{} // all of it
		}
		drawEdges(p, r, a.doc.Structure, rect.X+cam.X, rect.Y+cam.Y, z, lk.line, view, color)
		a.drawSummaries(p, r, shown, rect.X+cam.X, rect.Y+cam.Y, z, color)
		a.drawDependencies(p, r, rect.X+cam.X, rect.Y+cam.Y, z)
		a.drawRelations(p, r, rect.X+cam.X, rect.Y+cam.Y, z, ui.Hex("#6366f1"))
	})
	cv.Children(func() {
		floating := a.floatingTree()
		shown.WalkShown(func(n, _ *model.Node, depth int) {
			box, ok := r.Boxes[n.ID]
			if !ok {
				return
			}
			if !exporting && a.vp.W > 0 && n.ID != a.selected && n.ID != a.editing && !a.multi[n.ID] {
				// Off the screen: not built, which a large map needs.
				x, y := cam.X+box.X*z, cam.Y+box.Y*z
				if x+box.W*z < -nodeCull || x > a.vp.W+nodeCull || y+box.H*z < -nodeCull || y > a.vp.H+nodeCull {
					return
				}
			}
			if floating[n.ID] != nil {
				depth++ // floating topics look as main topics
			}
			a.nodeView(c.Key(n.ID), n, depth, box, lk.branch(branch[n.ID]))
		})
		if exporting {
			return
		}
		a.focusBar(c)
		a.structurePicker(c)
		// Zoom and recenter, over the canvas's corner.
		ui.Row(c).Absolute().Right(12).Bottom(12).Gap(2).Padding(3).Radius(8).
			Background(t.Surface).Border(1, t.Border).Children(func() {
			if iconButton(c, iconMinus, "缩小", "缩小 ⌘-", 30, true).Clicked() {
				a.setZoom(cam.Zoom / 1.2)
			}
			zoom := ui.Box(c.Key("zoom")).Height(30).MinWidth(54).Padding(0, 8).Radius(6).Center().Cursor(ui.CursorPointer).
				Label("缩放比例").Tooltip("实际大小 ⌘0")
			if zoom.Hovered() {
				zoom.Background(t.SurfaceHover)
			}
			if zoom.Clicked() {
				a.setZoom(1)
			}
			zoom.Children(func() {
				ui.Text(c, fmt.Sprintf("%d%%", int(cam.Zoom*100+0.5))).FontSize(12).PassThrough()
			})
			if iconButton(c, iconPlus, "放大", "放大 ⌘=", 30, true).Clicked() {
				a.setZoom(cam.Zoom * 1.2)
			}
			ui.Box(c).Width(1).Height(16).Background(t.Border).Margin(0, 2)
			if iconButton(c, iconTarget, "回到中心", "回到中心 ⌘⇧0", 30, true).Clicked() {
				a.recenter()
			}
			iconButton(c, iconDownload, "导出", "导出图片、Markdown、XMind…", 30, true).Menu(a.exportMenu)
		})
	})
}

func exportingNow(a *app) bool { return a.exportCam != nil }

// nodeCull is how far off the canvas nodes are still built, so that they
// are there as soon as a pan brings them in.
const nodeCull = 200

// keepIn returns how far to move a span at pos, size long, to keep it in
// [margin, length-margin], preferring its start.
func keepIn(pos, size, length float32) float32 {
	const margin = 40
	switch {
	case pos < margin:
		return margin - pos
	case pos+size > length-margin:
		return max(length-margin-pos-size, margin-pos)
	}
	return 0
}

func mod(x, m float32) float32 {
	r := float32(math.Mod(float64(x), float64(m)))
	if r < 0 {
		r += m
	}
	return r
}

// canvasInput takes the wheel, the touchpad and the keys on the canvas.
func (a *app) canvasInput(ev ui.InputEvent) bool {
	cam := a.cam()
	switch ev.Kind {
	case ui.InputPointerMove:
		a.pointer = [2]float32{ev.X, ev.Y}
		return false
	case ui.InputScroll:
		mods := ev.Mods | a.mods
		if mods&(ui.Cmd|ui.Ctrl) != 0 {
			f := float32(math.Exp(float64(-ev.DY) * 0.01))
			if !ev.Precise {
				f = float32(math.Pow(1.1, float64(-ev.DY)/40))
			}
			a.setZoomAt(cam.Zoom*f, a.pointer[0], a.pointer[1])
			return true
		}
		dx, dy := ev.DX, ev.DY
		if mods&ui.Shift != 0 && dx == 0 {
			dx, dy = dy, 0
		}
		cam.X -= dx
		cam.Y -= dy
		return true
	}
	return a.canvasKey(ev)
}

// sheetTabs shows the canvases of the file as tabs: a click shows one, a
// double click renames it, and its context menu copies, moves or deletes it.
func (a *app) sheetTabs(c *ui.Context) {
	t := c.Theme()
	ui.ScrollHorizontal(c.Key("sheets")).Background(t.Surface).Children(func() {
		ui.Row(c).Padding(4, 8).Gap(4).AlignItems(ui.Center).Children(func() {
			for _, d := range a.file.Sheets {
				a.sheetTab(c.Key(d.ID), d)
			}
			if iconButton(c, iconPlus, "新建画布", "新建画布", 26, true).Clicked() {
				a.addSheet()
			}
		})
	})
	if ui.AlertDialog(c, &a.confirmSheet, "删除画布「"+a.sheetName(a.renamingSheet)+"」？", "画布上的所有节点都会删除，可以用撤销恢复。", "取消", "删除") == 1 {
		a.deleteSheet(a.renamingSheet)
		a.renamingSheet = ""
	}
}

func (a *app) sheetName(id string) string {
	if d, _ := a.file.Sheet(id); d != nil {
		return d.Title
	}
	return ""
}

func (a *app) sheetTab(c *ui.Context, d *model.Document) {
	t := c.Theme()
	active := d == a.doc
	tab := ui.Row(c).Padding(4, 12).Radius(6).Cursor(ui.CursorPointer).Label(d.Title)
	if active {
		tab.Background(t.Background).Border(1, t.Border)
	} else if tab.Hovered() {
		tab.Background(t.SurfaceHover)
	}
	if a.renamingSheet == d.ID && !a.confirmSheet {
		tab.Children(func() {
			in := ui.TextInputBase(c.Key("rename"), &a.sheetTitle).Width(120).AutoFocus()
			switch {
			case in.Shortcut(0, ui.KeyEscape):
				a.renamingSheet = ""
			case in.Submitted():
				a.renameSheet(d.ID, a.sheetTitle)
				a.renamingSheet = ""
			case in.PressedOutside():
				a.renameSheet(d.ID, a.sheetTitle)
				a.renamingSheet = ""
			}
		})
		return
	}
	if tab.Pressed() && !active {
		a.showSheet(d.ID)
	}
	if tab.DoubleClicked() {
		a.renamingSheet, a.sheetTitle = d.ID, d.Title
	}
	_, i := a.file.Sheet(d.ID)
	id := d.ID
	tab.ContextMenu(func(m *ui.Menu) {
		if m.Item("重命名").Chosen() {
			a.renamingSheet, a.sheetTitle = id, a.sheetName(id)
		}
		if m.Item("复制画布").Chosen() {
			a.later(func() { a.duplicateSheet(id) })
		}
		m.Separator()
		if m.Item("左移").Disabled(i == 0).Chosen() {
			a.later(func() { a.moveSheet(id, -1) })
		}
		if m.Item("右移").Disabled(i == len(a.file.Sheets)-1).Chosen() {
			a.later(func() { a.moveSheet(id, 1) })
		}
		m.Separator()
		if m.Item("删除画布…").Disabled(len(a.file.Sheets) == 1).Chosen() {
			a.renamingSheet, a.confirmSheet = id, true
		}
	})
	tab.Children(func() {
		text := ui.Text(c, d.Title).FontSize(13).NoWrap()
		if !active {
			text.TextColor(t.TextMuted)
		}
	})
}

func (a *app) commitEditIfAny() {
	if a.editing != "" {
		a.commitEdit()
	}
}

// nodeView draws one node at its box.
func (a *app) nodeView(c *ui.Context, n *model.Node, depth int, box layout.Rect, accent ui.Color) {
	t := c.Theme()
	cam := a.cam()
	z := cam.Zoom
	p := a.parts(n)
	selected := a.isSelected(n.ID)
	editing := n.ID == a.editing

	el := ui.Row(c).Absolute().Left(cam.X+box.X*z).Top(cam.Y+box.Y*z).Size(box.W*z, box.H*z).
		Padding(0, nodePadX*z).Gap(nodeGap * z).AlignItems(ui.Center).Radius(8 * z)
	iw, ih := thumbSize(n.Image)
	if iw > 0 { // the title's row goes below the picture
		el.Padding((nodePadY+ih+thumbGap)*z, nodePadX*z, nodePadY*z)
	}
	// As XMind draws them: a dark central topic, main topics filled with
	// their branch's color, and subtopics tinted with it; then the node's
	// own style.
	st := styleOf(n)
	var fill ui.Color
	filled := false // with a fill whose text color differs from the canvas's
	switch depth {
	case 0:
		fill, filled = lookOf(a.doc.Style).root(t), true
		el.Radius(10 * z)
	case 1:
		fill, filled = accent, true
	default:
		tint := float32(0.12)
		if t.Dark {
			tint = 0.24
		}
		fill = accent.Alpha(tint)
		el.Radius(6 * z)
	}
	if st.Fill != "" {
		fill, filled = ui.Hex(st.Fill), true
	}
	switch st.Shape {
	case model.ShapeRect:
		el.Radius(2 * z)
	case model.ShapeRounded:
		el.Radius(8 * z)
	case model.ShapePill:
		el.Radius(999)
	case model.ShapeUnderline, model.ShapeNone:
		el.Radius(0)
		if st.Fill == "" {
			fill, filled = ui.Color{}, false
		}
		if st.Shape == model.ShapeUnderline {
			el.BorderWidth(0, 0, 2*z, 0).BorderColor(accent)
		}
	}
	el.Background(fill)
	onFill := t.Text
	if filled {
		onFill = textOn(fill)
	}
	if st.Color != "" {
		onFill = ui.Hex(st.Color)
	}
	el.TextColor(onFill)
	if st.Border != "" {
		el.Border(max(st.BorderWidth, 1)*z, ui.Hex(st.Border))
	}
	el.Font(a.doc.Style.Font)
	if selected {
		el.Border(2*z, t.Accent)
	}
	if a.filtering() && !a.highlighted(n) {
		el.Opacity(0.35)
	} else if a.filtering() {
		el.Shadow(0, 0, 0, 3*z, ui.Hex("#facc15").Alpha(0.6))
	}
	if n.Task != nil && n.Task.Status == model.StatusDone {
		el.Opacity(0.7)
	}
	label := n.Title
	if n.Task != nil {
		label += "，" + n.Task.Status.Label()
	}
	el.Label(label)

	// Selecting, editing, dragging.
	if !editing {
		adding := a.mods&(ui.Shift|ui.Cmd) != 0 // adds to the selection, or takes out
		if a.relFrom != "" {                    // drawing a relation line: this node ends it
			if el.Pressed() {
				id := n.ID
				a.later(func() { a.endRelation(id) })
			}
			el.Cursor(ui.CursorCrosshair)
		} else if el.Pressed() && !adding && (n.ID != a.selected || len(a.multi) > 0) {
			a.commitEditIfAny()
			a.selected, a.multi = n.ID, nil
		}
		if el.Clicked() && adding {
			a.commitEditIfAny()
			a.toggleSelect(n.ID)
		}
		if el.DoubleClicked() {
			a.startEdit(n.ID)
		}
		if depth == 0 {
			// The central node carries the whole map: dragging it moves
			// the canvas.
			if dx, dy, ok := el.Dragged(); ok {
				cam.X += dx
				cam.Y += dy
			}
			if el.Pressed() {
				el.Cursor(ui.CursorGrabbing)
			} else {
				el.Cursor(ui.CursorGrab)
			}
		} else if n.Pos != nil && a.doc.IsFloating(n.ID) {
			// A floating topic goes where it is dragged.
			if dx, dy, ok := el.Dragged(); ok {
				a.mutate("float:"+n.ID, func() { n.Pos.X += dx / z; n.Pos.Y += dy / z })
			}
			el.Cursor(ui.CursorGrab)
		} else {
			// Other nodes are dropped on a node to move under it.
			el.Drag(nodeDrag{n.ID}).Cursor(ui.CursorPointer)
		}
		if d, over := ui.DragOver[nodeDrag](el); over && d.ID != n.ID {
			el.Border(2*z, t.Accent).BorderStyle(ui.BorderDashed)
		}
		if d, ok := ui.Drop[nodeDrag](el); ok {
			id := n.ID
			a.later(func() { a.moveNode(d.ID, id) })
		}
		el.ContextMenu(func(m *ui.Menu) { a.nodeMenu(m, n, depth) })
		if files := el.DroppedFiles(); len(files) > 0 {
			a.later(func() { a.dropFiles(n, files) })
		}
	}

	// Marks read on the node's fill: on filled topics, in the fill's text
	// color.
	muted := t.TextMuted
	if filled {
		muted = onFill.Alpha(0.75)
	}
	el.Children(func() {
		if iw > 0 {
			a.thumbnail(c, n, iw*z, ih*z).Absolute().Left((box.W - iw) / 2 * z).Top(nodePadY * z)
		}
		if n.Task != nil {
			a.taskCheck(c, n, checkSize*z)
			if a.blocked(n) {
				var names []string
				for _, b := range a.file.Blockers(n) {
					names = append(names, "「"+b.Title+"」")
				}
				ui.Text(c.Key("blocked"), "🔒").FontSize(markSize * z).NoWrap().Tooltip("等待 " + strings.Join(names, "、") + " 完成")
			}
		}
		for _, m := range n.Markers {
			// A click goes to the next marker of its group, as in XMind;
			// its menu takes it off.
			mk := markerView(c, m, markerSize(depth)*z).Cursor(ui.CursorPointer).Tooltip(markerName(m) + "（点击切换）")
			if mk.Clicked() {
				a.later(func() {
					a.mutate("", func() {
						if i := slices.Index(n.Markers, m); i >= 0 {
							n.Markers[i] = nextMarker(m)
						}
					})
				})
			}
			mk.ContextMenu(func(menu *ui.Menu) {
				if menu.Item("移除「" + markerName(m) + "」").Chosen() {
					a.later(func() { a.toggleMarker(n, m) })
				}
			})
		}
		size := fontSize(n, depth) * z
		tw, _ := a.titleBox(c, n, depth)
		if editing {
			// A text area, wrapping as the title will, rather than a line
			// that scrolls what was typed first out of view. Enter ends
			// the edit; a title has no line breaks.
			in := ui.TextAreaBase(c.Key("edit"), &a.editText).FontSize(size).FontWeight(fontWeight(n, depth)).
				Placeholder("新主题").Width(tw*z).Lines(1, 50).AutoFocus()
			in.HandleInput(func(ev ui.InputEvent) bool {
				if ev.Kind != ui.InputKeyDown || ev.Key != ui.KeyEnter || ev.Mods&^ui.Shift != 0 || a.editing != n.ID {
					return false
				}
				a.commitEdit()
				return true
			})
			if strings.ContainsAny(a.editText, "\r\n\t") { // pasted lines join
				a.editText = strings.Join(strings.Fields(strings.ReplaceAll(a.editText, "\r", "\n")), " ")
			}
			if in.Changed() {
				a.reveal = true // the node grows as it is typed in: keep it in view
			}
			if in.Shortcut(0, ui.KeyEscape) {
				a.cancelEdit()
			} else if in.Focused() {
				if !a.editFocus {
					in.SetTextSelection(0, utf8.RuneCountInString(a.editText)) // typing replaces the title
				}
				a.editFocus = true
			} else if a.editFocus {
				a.commitEdit() // the focus went elsewhere
			}
			return
		}
		title := ui.Text(c, n.Title).FontSize(size).FontWeight(fontWeight(n, depth)).Width(tw * z).Shrink(0)
		if n.Task != nil && n.Task.Status == model.StatusDone || st.Strike {
			title.Strikethrough()
		}
		if st.Italic {
			title.Italic()
		}
		if st.Underline {
			title.Underline()
		}
		switch st.Align {
		case "center":
			title.TextAlign(ui.Center)
		case "right":
			title.TextAlign(ui.End)
		}
		for _, tag := range p.tags {
			if filled {
				size := markSize * z
				ui.Box(c.Key("tag:"+tag)).Padding(size*0.15, size*0.55).Radius(999).Background(onFill.Alpha(0.22)).
					PassThrough().Children(func() {
					ui.Text(c, tag).FontSize(size).TextColor(onFill).NoWrap()
				})
				continue
			}
			tagChip(c.Key("tag:"+tag), tag, tag, false, markSize*z).PassThrough()
		}
		if p.more > 0 {
			ui.Text(c, "+"+strconv.Itoa(p.more)).FontSize(markSize * z).TextColor(muted).NoWrap()
		}
		if pg := a.progress[n.ID]; pg[1] > 0 {
			ui.Row(c.Key("progress")).Gap(3 * z).AlignItems(ui.Center).PassThrough().
				Tooltip(fmt.Sprintf("下级任务完成 %d / %d", pg[0], pg[1])).Children(func() {
				progressRing(c, pg, markSize*z, onFill, accent)
				ui.Text(c, progressText(pg)).FontSize(markSize * z).TextColor(muted).NoWrap()
			})
		}
		if p.due != "" {
			col := muted
			if n.Task.Overdue(a.now()) && !filled {
				col = t.Danger
			}
			ui.Text(c, p.due).FontSize(markSize * z).TextColor(col).NoWrap()
		}
		if p.note {
			a.noteMark(c, n, (markSize+3)*z, muted)
		}
		if a.noteEditing == n.ID && !exportingNow(a) {
			a.notePopover(c, el, n)
		}
		if m := filesMark(n); m != "" {
			var names []string
			for _, f := range n.Files {
				names = append(names, f.Name)
			}
			clip := ui.Box(c.Key("files")).Cursor(ui.CursorPointer).Label("打开附件").
				Tooltip("附件：" + strings.Join(names, "、") + "（点击打开第一个）")
			if clip.Clicked() {
				f := n.Files[0]
				a.later(func() { a.openAttachment(f) })
			}
			clip.Children(func() { ui.Text(c, m).FontSize(markSize * z).TextColor(muted).NoWrap() })
		}
		if len(n.Links) > 0 {
			var tips []string
			for _, l := range n.Links {
				_, label := a.linkLabel(l)
				tips = append(tips, label)
			}
			mark := ui.Box(c.Key("links")).Cursor(ui.CursorPointer).Label("打开链接").
				Tooltip("点击打开：" + strings.Join(tips, "、"))
			if mark.Clicked() {
				link := n.Links[0]
				a.later(func() { a.openLink(link) })
			}
			mark.Children(func() {
				ui.Text(c, "↗").FontSize((markSize + 1) * z).TextColor(accentOn(filled, onFill, t.Accent)).NoWrap()
			})
		}
	})

	// The button opening and closing the children, just right of the box.
	if len(n.Children) > 0 && (n.Collapsed || selected) {
		s := 18 * z
		foldX := cam.X + (box.X+box.W)*z + 3*z
		if a.boxes.Left[n.ID] {
			foldX = cam.X + box.X*z - 3*z - s
		}
		btn := ui.Box(c.Key("fold:"+n.ID)).Absolute().Left(foldX).Top(cam.Y+(box.Y+box.H/2)*z-s/2).
			Size(s, s).Radius(999).Center().Background(t.Background).Border(1, accent).Cursor(ui.CursorPointer)
		text := "−"
		if n.Collapsed {
			text = strconv.Itoa(len(n.Children))
			btn.Label("展开")
		} else {
			btn.Label("折叠")
		}
		btn.Children(func() {
			ui.Text(c, text).FontSize(11 * z).TextColor(accent).Bold()
		})
		if btn.Clicked() {
			a.selected = n.ID
			a.later(func() { a.toggleCollapse(n.ID) })
		}
	}
}

// nodeMenu is a node's context menu. Its actions wait until the map is
// built, as they change the nodes being walked.
func (a *app) nodeMenu(m *ui.Menu, n *model.Node, depth int) {
	if !a.isSelected(n.ID) {
		a.selected, a.multi = n.ID, nil
	}
	if m.Item("添加子节点").Shortcut(0, ui.KeyTab).Chosen() {
		a.later(func() { a.addChild() })
	}
	if m.Item("添加同级节点").Shortcut(0, ui.KeyEnter).Chosen() {
		a.later(func() { a.addSibling() })
	}
	if m.Item("编辑标题").Shortcut(0, ui.KeyF2).Chosen() {
		a.later(func() { a.startEdit(n.ID) })
	}
	if m.Item("编辑备注…").Chosen() {
		a.noteEditing = n.ID
	}
	a.tagMenu(m, n)
	a.annotationMenu(m, n, depth)
	m.Separator()
	if m.Item("复制").Shortcut(ui.Cmd, ui.KeyC).Chosen() {
		a.later(func() { a.copyNodes(false) })
	}
	if m.Item("剪切").Shortcut(ui.Cmd, ui.KeyX).Disabled(depth == 0).Chosen() {
		a.later(func() { a.copyNodes(true) })
	}
	if m.Item("粘贴为子节点").Chosen() {
		a.later(func() { a.paste() })
	}
	if m.Item("AI 拆解成任务").Chosen() {
		a.later(a.aiBreakDown)
	}
	if depth > 0 && m.Item("移到其他导图…").Chosen() {
		a.later(func() { a.openPalette(paletteMove) })
	}
	m.Separator()
	if a.shownRoot() == n && a.focusNode() != nil {
		if m.Item("退出聚焦").Shortcut(0, ui.KeyEscape).Chosen() {
			a.later(func() { a.focusOn("") })
		}
	} else if depth > 0 && m.Item("聚焦此分支").Shortcut(ui.Alt, ui.KeyEnter).Chosen() {
		id := n.ID
		a.later(func() { a.focusOn(id) })
	}
	if len(n.Children) > 0 {
		if m.Item("展开全部下级").Chosen() {
			a.later(func() { a.setCollapsedBelow(n, false) })
		}
		if m.Item("折叠全部下级").Chosen() {
			a.later(func() { a.setCollapsedBelow(n, true) })
		}
	}
	if n.Image != nil {
		if m.Item("查看大图").Chosen() {
			a.lightbox = n.ID
		}
		if m.Item("删除图片").Chosen() {
			a.later(func() { a.removeImage(n) })
		}
	} else if m.Item("粘贴图片").Shortcut(ui.Cmd, ui.KeyV).Chosen() {
		a.later(func() { a.pasteImage() })
	}
	m.Separator()
	if m.Item("作为任务").Checked(n.Task != nil).Chosen() {
		a.later(func() { a.toggleTask(n) })
	}
	m.Submenu("任务状态", func(m *ui.Menu) {
		for _, s := range model.Statuses {
			if m.Item(s.Label()).Checked(n.Task != nil && n.Task.Status == s).Chosen() {
				a.later(func() { a.setStatus(n, s) })
			}
		}
	})
	if len(n.Children) > 0 {
		label := "折叠"
		if n.Collapsed {
			label = "展开"
		}
		if m.Item(label).Chosen() {
			a.later(func() { a.toggleCollapse(n.ID) })
		}
	}
	m.Separator()
	label := "删除"
	if len(a.multi) > 0 {
		label = fmt.Sprintf("删除所选 %d 个节点", len(a.selection()))
	}
	if m.Item(label).Shortcut(0, ui.KeyDelete).Disabled(depth == 0 && len(a.multi) == 0).Chosen() {
		a.later(a.deleteSelection)
	}
}

// accentOn is the color of a link's mark on a node: the node's text on a
// fill, else the accent.
func accentOn(filled bool, onFill, accent ui.Color) ui.Color {
	if filled {
		return onFill
	}
	return accent
}

// progressOf counts the tasks under every node below root: done and all.
func progressOf(root *model.Node) map[string][2]int {
	out := map[string][2]int{}
	var walk func(n *model.Node) [2]int
	walk = func(n *model.Node) [2]int {
		var sum [2]int
		for _, c := range n.Children {
			below := walk(c)
			sum[0] += below[0]
			sum[1] += below[1]
			if c.Task != nil {
				sum[1]++
				if c.Task.Status == model.StatusDone {
					sum[0]++
				}
			}
		}
		if sum[1] > 0 {
			out[n.ID] = sum
		}
		return sum
	}
	walk(root)
	return out
}

func progressText(pg [2]int) string { return fmt.Sprintf("%d/%d", pg[0], pg[1]) }

// progressRing draws how much of a node's tasks are done, as a pie.
func progressRing(c *ui.Context, pg [2]int, size float32, on, accent ui.Color) {
	frac := float32(pg[0]) / float32(pg[1])
	ui.Box(c).Size(size, size).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
		cx, cy, rad := r.X+r.W/2, r.Y+r.H/2, r.W/2
		p.FillPath(new(ui.Path).Circle(cx, cy, rad), on.Alpha(0.18))
		if frac <= 0 {
			return
		}
		col := ui.Hex("#22c55e")
		if frac >= 1 {
			p.FillPath(new(ui.Path).Circle(cx, cy, rad), col)
			return
		}
		path := new(ui.Path).MoveTo(cx, cy)
		const steps = 24
		for i := 0; i <= steps; i++ {
			ang := -math.Pi/2 + 2*math.Pi*float64(frac)*float64(i)/steps
			path.LineTo(cx+rad*float32(math.Cos(ang)), cy+rad*float32(math.Sin(ang)))
		}
		p.FillPath(path.Close(), col)
	})
}

// canvasKey handles the keys pressed while the canvas has the focus.
func (a *app) canvasKey(ev ui.InputEvent) bool {
	if a.editing != "" || a.doc == nil {
		return false
	}
	if ev.Kind == ui.InputCommand {
		switch ev.Text {
		case "undo":
			a.undo()
			return true
		case "redo":
			a.redo()
			return true
		case "delete":
			a.deleteSelection()
			return true
		case "copy":
			a.copyNodes(false)
			return true
		case "cut":
			a.copyNodes(true)
			return true
		case "paste":
			return a.paste()
		}
		return false
	}
	if ev.Kind != ui.InputKeyDown {
		return false
	}
	switch {
	case ev.Mods == 0 && ev.Key == ui.KeyTab:
		a.addChild()
	case ev.Mods == 0 && ev.Key == ui.KeyEnter:
		a.addSibling()
	case ev.Mods == 0 && (ev.Key == ui.KeyDelete || ev.Key == ui.KeyBackspace):
		a.deleteSelection()
	case ev.Mods == 0 && ev.Key == ui.KeyEscape:
		switch {
		case a.relFrom != "":
			a.relFrom = ""
		case len(a.multi) > 0:
			a.multi = nil
		case a.focusNode() != nil:
			a.focusOn("")
		default:
			return false
		}
	case ev.Mods == ui.Alt && ev.Key == ui.KeyEnter:
		if a.selected == a.shownRoot().ID {
			a.focusOn("") // again on the middle: out
		} else {
			a.focusOn(a.selected)
		}
	case ev.Mods == ui.Alt && ev.Key >= ui.Key0 && ev.Key <= ui.Key9:
		a.expandTo(int(ev.Key - ui.Key0))
	case ev.Mods == ui.Cmd && ev.Key == ui.KeyC:
		a.copyNodes(false)
	case ev.Mods == ui.Cmd && ev.Key == ui.KeyX:
		a.copyNodes(true)
	case ev.Mods == 0 && (ev.Key == ui.KeyF2 || ev.Key == ui.KeySpace):
		a.startEdit(a.selected)
	case ev.Mods == 0 && ev.Key == ui.KeyLeft:
		a.navSide(true)
	case ev.Mods == 0 && ev.Key == ui.KeyRight:
		a.navSide(false)
	case ev.Mods == 0 && ev.Key == ui.KeyUp:
		a.navSibling(-1)
	case ev.Mods == 0 && ev.Key == ui.KeyDown:
		a.navSibling(1)
	case ev.Mods == ui.Cmd && ev.Key == ui.KeyUp:
		a.shiftSelected(-1)
	case ev.Mods == ui.Cmd && ev.Key == ui.KeyDown:
		a.shiftSelected(1)
	case ev.Mods == ui.Cmd && ev.Key == ui.KeySlash:
		a.toggleCollapse(a.selected)
	case ev.Mods == ui.Cmd && ev.Key == ui.KeyV:
		return a.paste()
	case ev.Mods == ui.Cmd && ev.Key == ui.KeyZ:
		a.undo()
	case ev.Mods == ui.Cmd|ui.Shift && ev.Key == ui.KeyZ:
		a.redo()
	default:
		return false
	}
	return true
}

// layoutOptions are the layout's spacing for a structure: the logic chart
// and the bracket chart grow to the right only.
func layoutOptions(s model.Structure) layout.Options {
	o := layout.DefaultOptions
	switch s {
	case model.StructureLogic:
		o.OneSided = true
	case model.StructureBracket:
		o.OneSided, o.GapX, o.GapY = true, 40, 10
	case model.StructureOrg:
		o.Mode, o.GapX, o.GapY = layout.ModeDown, 44, 12
	case model.StructureTimeline:
		o.Mode, o.GapX, o.GapY = layout.ModeTimeline, 36, 10
	case model.StructureFishbone:
		o.Mode, o.GapX, o.GapY = layout.ModeFishbone, 36, 10
	}
	return o
}

// drawEdges joins each node to its children, as the structure draws them,
// with the map's origin at (ox, oy) and zoom z: curves for the mind map, a
// trunk of straight lines for the logic chart, and a curly bracket around
// the children for the bracket chart.
// Edges wholly outside view are not drawn.
func drawEdges(p *ui.Painter, r layout.Result, s model.Structure, ox, oy, z, line float32, view ui.Rect, color func(to string) ui.Color) {
	width := max(line*z, 1)
	switch s {
	case model.StructureOrg:
		drawOrgEdges(p, r, ox, oy, z, width, color)
		return
	case model.StructureTimeline, model.StructureFishbone:
		drawLineEdges(p, r, s == model.StructureFishbone, ox, oy, z, width, color)
		return
	}
	if s == model.StructureMindMap {
		for _, e := range r.Edges {
			from, to := r.Boxes[e.From], r.Boxes[e.To]
			x0, y0 := ox+(from.X+from.W)*z, oy+(from.Y+from.H/2)*z
			x1, y1 := ox+to.X*z, oy+(to.Y+to.H/2)*z
			if e.Left {
				x0, x1 = ox+from.X*z, ox+(to.X+to.W)*z
			}
			if outside(view, min(x0, x1), min(y0, y1), max(x0, x1), max(y0, y1)) {
				continue
			}
			var path ui.Path
			path.MoveTo(x0, y0)
			mid := (x1 - x0) / 2
			path.CubeTo(x0+mid, y0, x1-mid, y1, x1, y1)
			p.StrokePath(&path, width, color(e.To))
		}
		return
	}

	// The other structures draw each node's children together.
	var order []string
	children := map[string][]string{}
	for _, e := range r.Edges {
		if children[e.From] == nil {
			order = append(order, e.From)
		}
		children[e.From] = append(children[e.From], e.To)
	}
	for _, id := range order {
		from, kids := r.Boxes[id], children[id]
		px, py := ox+(from.X+from.W)*z, oy+(from.Y+from.H/2)*z
		cx := ox + r.Boxes[kids[0]].X*z // the children's column
		top, bottom := float32(1e9), float32(-1e9)
		for _, k := range kids {
			b := r.Boxes[k]
			if s == model.StructureBracket {
				top, bottom = min(top, oy+b.Y*z), max(bottom, oy+(b.Y+b.H)*z)
			} else {
				top, bottom = min(top, oy+(b.Y+b.H/2)*z), max(bottom, oy+(b.Y+b.H/2)*z)
			}
		}
		if outside(view, min(px, cx-12*z), min(top, py), max(px, cx), max(bottom, py)) {
			continue
		}
		col := color(kids[0])
		if s == model.StructureLogic {
			// Out from the parent to the trunk, along it, and into each child.
			tx := px + (cx-px)/2
			var path ui.Path
			path.MoveTo(px, py)
			path.LineTo(tx, py)
			p.StrokePath(&path, width, col)
			if len(kids) > 1 {
				var trunk ui.Path
				trunk.MoveTo(tx, min(top, py))
				trunk.LineTo(tx, max(bottom, py))
				p.StrokePath(&trunk, width, col)
			}
			for _, k := range kids {
				b := r.Boxes[k]
				y := oy + (b.Y+b.H/2)*z
				var branch ui.Path
				branch.MoveTo(tx, y)
				branch.LineTo(cx, y)
				p.StrokePath(&branch, width, color(k))
			}
			continue
		}
		// A bracket: its point toward the parent, its arms around the
		// children.
		bx := cx - 10*z // the bracket's spine
		rad := min(8*z, (bottom-top)/4)
		mid := min(max(py, top+2*rad), bottom-2*rad)
		var brace ui.Path
		brace.MoveTo(bx+rad, top)
		brace.QuadTo(bx, top, bx, top+rad)
		brace.LineTo(bx, mid-rad)
		brace.QuadTo(bx, mid, bx-rad, mid)
		brace.QuadTo(bx, mid, bx, mid+rad)
		brace.LineTo(bx, bottom-rad)
		brace.QuadTo(bx, bottom, bx+rad, bottom)
		p.StrokePath(&brace, width, col)
		var stem ui.Path
		stem.MoveTo(px, py)
		stem.LineTo(bx-rad, mid)
		p.StrokePath(&stem, width, col)
	}
}

// outside tells whether a box, by its corners, is wholly outside view, with
// a margin for the lines' width; an empty view holds everything.
func outside(view ui.Rect, x0, y0, x1, y1 float32) bool {
	if view.W <= 0 || view.H <= 0 {
		return false
	}
	const m = 8
	return x1 < view.X-m || x0 > view.X+view.W+m || y1 < view.Y-m || y0 > view.Y+view.H+m
}

// drawOrgEdges joins each node to the row of its children: down from its
// bottom, along a bar over them, and down into each.
func drawOrgEdges(p *ui.Painter, r layout.Result, ox, oy, z, width float32, color func(string) ui.Color) {
	kids := map[string][]string{}
	var order []string
	for _, e := range r.Edges {
		if kids[e.From] == nil {
			order = append(order, e.From)
		}
		kids[e.From] = append(kids[e.From], e.To)
	}
	for _, id := range order {
		from := r.Boxes[id]
		px, py := ox+(from.X+from.W/2)*z, oy+(from.Y+from.H)*z
		top := float32(1e9)
		left, right := px, px
		for _, k := range kids[id] {
			b := r.Boxes[k]
			top = min(top, oy+b.Y*z)
			left, right = min(left, ox+(b.X+b.W/2)*z), max(right, ox+(b.X+b.W/2)*z)
		}
		bar := py + (top-py)/2
		col := color(kids[id][0])
		p.StrokePath(new(ui.Path).MoveTo(px, py).LineTo(px, bar), width, col)
		p.StrokePath(new(ui.Path).MoveTo(left, bar).LineTo(right, bar), width, col)
		for _, k := range kids[id] {
			b := r.Boxes[k]
			x := ox + (b.X+b.W/2)*z
			p.StrokePath(new(ui.Path).MoveTo(x, bar).LineTo(x, oy+b.Y*z), width, color(k))
		}
	}
}

// drawLineEdges draws a timeline's or a fishbone's line from the root, the
// main branches' links to it, and within each branch's column the lines
// from a node down (or up) its side to each child.
func drawLineEdges(p *ui.Painter, r layout.Result, fishbone bool, ox, oy, z, width float32, color func(string) ui.Color) {
	to := map[string]bool{}
	for _, e := range r.Edges {
		to[e.To] = true
	}
	root := ""
	for _, e := range r.Edges {
		if !to[e.From] {
			root = e.From
			break
		}
	}
	rb := r.Boxes[root]
	spineY := oy + (rb.Y+rb.H/2)*z
	start, end := ox+(rb.X+rb.W)*z, ox+(rb.X+rb.W)*z
	if fishbone {
		start, end = ox+rb.X*z, ox+rb.X*z
	}
	for _, e := range r.Edges {
		if e.From != root {
			continue
		}
		b := r.Boxes[e.To]
		up := r.Up[e.To]
		edgeY := oy + (b.Y+b.H)*z // the box's side toward the line
		if !up {
			edgeY = oy + b.Y*z
		}
		col := color(e.To)
		if fishbone {
			// A bone, leaning back from the spine to the branch's near end.
			x := ox + (b.X+b.W)*z
			at := x + 40*z
			end = min(end, ox+b.X*z-12*z)
			p.StrokePath(new(ui.Path).MoveTo(at, spineY).LineTo(x-8*z, edgeY), width*1.4, col)
			continue
		}
		x := ox + (b.X+14)*z
		end = max(end, ox+(b.X+b.W)*z+12*z)
		p.StrokePath(new(ui.Path).MoveTo(x, spineY).LineTo(x, edgeY), width, col)
		p.FillPath(new(ui.Path).Circle(x, spineY, 3.5*z), col)
	}
	if fishbone && end < start { // the spine ends in a tail
		p.StrokePath(new(ui.Path).MoveTo(start, spineY).LineTo(end, spineY), width*2, color(root))
		p.StrokePath(new(ui.Path).MoveTo(end, spineY).LineTo(end-10*z, spineY-8*z), width*2, color(root))
		p.StrokePath(new(ui.Path).MoveTo(end, spineY).LineTo(end-10*z, spineY+8*z), width*2, color(root))
	} else if !fishbone && end > start {
		p.StrokePath(new(ui.Path).MoveTo(start, spineY).LineTo(end, spineY), width*2, color(root))
		p.StrokePath(new(ui.Path).MoveTo(end, spineY).LineTo(end-8*z, spineY-6*z), width*2, color(root))
		p.StrokePath(new(ui.Path).MoveTo(end, spineY).LineTo(end-8*z, spineY+6*z), width*2, color(root))
	}
	// Within a column: down (or up) the parent's left side, into each child.
	for _, e := range r.Edges {
		if e.From == root {
			continue
		}
		pb, cb := r.Boxes[e.From], r.Boxes[e.To]
		x := ox + (pb.X+12)*z
		y0 := oy + (pb.Y+pb.H)*z
		if cb.Y < pb.Y { // the column goes up
			y0 = oy + pb.Y*z
		}
		y1 := oy + (cb.Y+cb.H/2)*z
		p.StrokePath(new(ui.Path).MoveTo(x, y0).LineTo(x, y1).LineTo(ox+cb.X*z, y1), width, color(e.To))
	}
}

// structurePicker is the button over the canvas's top left corner that
// chooses the canvas's structure, from pictures of each.
func (a *app) structurePicker(c *ui.Context) {
	t := c.Theme()
	btn := ui.Button(c.Key("structure"), "结构："+a.doc.Structure.Label()+" ▾").
		Absolute().Left(12).Top(12).Tooltip("选择这个画布的结构")
	if btn.Clicked() {
		a.structureOpen = !a.structureOpen
	}
	ui.Popover(c, btn, &a.structureOpen, func() {
		ui.Grid(c).Columns(3).Gap(8).Padding(4).Children(func() {
			for _, st := range model.Structures {
				card := ui.Column(c.Key(string(st))).Gap(6).Padding(8).Radius(8).
					Border(1, t.Border).Cursor(ui.CursorPointer).Label(st.Label())
				if st == a.doc.Structure {
					card.Border(2, t.Accent)
				} else if card.Hovered() {
					card.Background(t.SurfaceHover)
				}
				if card.Clicked() {
					a.structureOpen = false
					a.setStructure(st)
				}
				card.Children(func() {
					ui.Box(c).Size(120, 72).Draw(func(p *ui.Painter, r ui.Rect) {
						drawStructureIcon(p, r, st, t.TextMuted)
					})
					ui.Text(c, st.Label()).FontSize(12).TextAlign(ui.Center)
				})
			}
		})
	})
}

// drawStructureIcon draws a small picture of a structure: a root and four
// branches, joined as the structure joins them.
func drawStructureIcon(p *ui.Painter, r ui.Rect, s model.Structure, col ui.Color) {
	d := model.New("")
	for range 4 {
		d.AddChild(d.Root.ID, "")
	}
	measure := func(_ *model.Node, depth int) (float32, float32) {
		if depth == 0 {
			return 30, 12
		}
		return 22, 8
	}
	o := layoutOptions(s)
	o.Margin, o.GapX, o.GapY = 0, 14, 5
	lr := layout.Compute(d, measure, o)
	z := min(r.W/lr.Width, r.H/lr.Height)
	ox := r.X + (r.W-lr.Width*z)/2
	oy := r.Y + (r.H-lr.Height*z)/2
	drawEdges(p, lr, s, ox, oy, z, 1.5, ui.Rect{}, func(string) ui.Color { return col })
	for id, b := range lr.Boxes {
		box := ui.Rect{X: ox + b.X*z, Y: oy + b.Y*z, W: b.W * z, H: b.H * z}
		if id == d.Root.ID {
			p.Fill(box, col, 3)
		} else {
			p.Stroke(box, col, 3, 1)
		}
	}
}
