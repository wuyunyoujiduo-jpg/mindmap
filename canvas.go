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
	text, font string
	size, wrap float32
	weight     int
	italic     bool
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

// pictureOnly tells whether a node shows its picture alone: one with a
// picture and nothing else, its title the one given for now (新主题) or
// none, shows no title under it.
func (a *app) pictureOnly(n *model.Node) bool {
	if n.Image == nil || a.editing == n.ID {
		return false
	}
	if t := strings.TrimSpace(n.Title); t != "" && t != "新主题" {
		return false
	}
	return n.Task == nil && len(n.Markers) == 0 && len(n.Tags) == 0 && n.Note == "" && len(n.Links) == 0 &&
		filesMark(n) == "" && a.progress[n.ID][1] == 0
}

func (a *app) measure(c *ui.Context) layout.Measure {
	return func(n *model.Node, depth int) (float32, float32) {
		if a.pictureOnly(n) {
			iw, ih := thumbSize(n.Image)
			return iw + 2*nodePadX, ih + 2*nodePadY
		}
		p := a.parts(n)
		tw, th := a.titleBox(c, n, depth)
		w := 2*nodePadX + tw
		if n.Task != nil {
			if !n.HasStatusMarker() {
				w += checkSize + nodeGap
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
		if links := nodeLinks(n); len(links) > 0 {
			mw, _ := a.measureText(c, 0, ui.Span{Text: linksMark(links), Size: markSize + 1})
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
		if a.editComposing {
			// An input method's text, not in the title until chosen, has
			// room too: a few characters more, rather than a new line.
			w += 6 * span.Size
		}
		if w > wrap {
			_, h = a.measureText(c, wrap, span)
			w = wrap
		}
		// It grows by steps, and never narrows while edited: the map does
		// not shift with every character typed.
		const step = 32
		switch {
		case a.editW == 0: // as it was
			a.editW = float32(math.Ceil(float64(w))) + 1
		case w > a.editW: // room for the next characters too
			a.editW = min(w+step, max(wrap, titleMin))
		}
		return max(a.editW, titleMin) + 4, h
	}
	w, h = a.measureText(c, 0, span)
	if st.Width > 0 { // a width of its own, at which the title wraps
		_, h = a.measureText(c, st.Width, span)
		return st.Width, h
	}
	if w > titleMax {
		_, h = a.measureText(c, titleMax, span)
		w = titleMax
	}
	// A point to spare: laid out at its very width, a title of glyphs of
	// fractional widths (a "+" among Chinese) wraps its last character.
	return max(float32(math.Ceil(float64(w)))+1, titleMin), h
}

// titleOneLine tells whether the node's title fits on one line, as it is
// not to wrap.
func (a *app) titleOneLine(c *ui.Context, n *model.Node, depth int) bool {
	st := styleOf(n)
	if st.Width > 0 {
		return false
	}
	w, _ := a.measureText(c, 0, ui.Span{Text: a.parts(n).title, Size: fontSize(n, depth), Weight: fontWeight(n, depth),
		Italic: st.Italic, Font: a.doc.Style.Font})
	return w <= titleMax
}

// dropSide is where the node dragged, id, dropped on n at the pointer
// goes: before n (-1) or after it (1) among its siblings, when near the
// edge they are on, or under it (0). Among its own siblings, it is mostly
// moved: only n's middle third takes it under it. The root and floating
// topics have no siblings.
func (a *app) dropSide(el ui.Element, n *model.Node, box layout.Rect, id string) int {
	_, p := a.doc.Find(n.ID)
	if p == nil {
		return 0
	}
	_, from := a.doc.Find(id)
	x, y, _ := el.PointerPosition()
	pos, size := y, box.H*a.cam().Zoom
	if a.siblingsAcross(n) {
		pos, size = x, box.W*a.cam().Zoom
	}
	edge := min(size*0.3, 14*a.cam().Zoom)
	if from == p {
		edge = size / 3
	}
	switch {
	case pos < edge:
		return -1
	case pos > size-edge:
		return 1
	}
	return 0
}

// siblingsAcross tells whether n's siblings sit side by side (as in the
// org chart) rather than one above another.
func (a *app) siblingsAcross(n *model.Node) bool {
	_, p := a.doc.Find(n.ID)
	if p == nil {
		return false
	}
	b, ok := a.boxes.Boxes[n.ID]
	if !ok {
		return false
	}
	for _, s := range p.Children {
		if o, ok := a.boxes.Boxes[s.ID]; ok && s != n && a.boxes.Left[s.ID] == a.boxes.Left[n.ID] {
			dx := (o.X + o.W/2) - (b.X + b.W/2)
			dy := (o.Y + o.H/2) - (b.Y + b.H/2)
			return dx*dx > dy*dy
		}
	}
	return a.doc.Structure == model.StructureOrg
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
	ui.Column(c).Grow(1).ObserveInput(a.stepEnds).Children(func() {
		a.canvasArea(c)
		a.sheetTabs(c)
	})
}

// stepEnds ends the step that edits of one key make together (mutate), as
// the canvas, the outline and the topic's column see their input: at a
// press of the pointer, which begins another drag or leaves a field (or
// comes back to it), and at the keys that leave one. Without an end, a
// floating topic dragged twice was one step to undo, and a title written,
// left and written again.
func (a *app) stepEnds(ev ui.InputEvent) {
	if ev.Kind == ui.InputPointerDown || ev.Kind == ui.InputKeyDown && (ev.Key == ui.KeyTab || ev.Key == ui.KeyEscape) {
		a.lastEdit = ""
	}
}

func (a *app) canvasArea(c *ui.Context) {
	t := c.Theme()
	cam := a.cam()
	z := cam.Zoom
	lk := lookOf(a.doc.Style)
	shown := a.shownDoc()
	// Not while a picture is made of it, with nothing chosen: its central
	// topic was drawn chosen in every picture saved.
	if a.exportCam == nil && shown.Path(a.selected) == nil && a.editing == "" { // left outside the branch focused on
		a.selected, a.multi = shown.Root.ID, nil
	}
	a.progress = progressOf(shown.Root)
	lens := a.lensLooks() // nil by the branches, and in a picture
	a.lensShown, a.lensQuiet = lens, lens != nil && a.lensOver()
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
	a.summaryTopics = map[string]bool{}
	shown.Walk(func(n, p *model.Node, _ int) bool {
		if p != nil && p.SummaryTopic() == n {
			a.summaryTopics[n.ID] = true // styled as summaries (nodeView)
		}
		switch {
		case p == nil && n != shown.Root: // a floating topic: a color after the branches'
			branch[n.ID] = len(shown.Root.Children) + slices.Index(shown.Floating, n)
		case p == nil:
		case p == shown.Root:
			// The topic of the middle's own summary is no child of it: a
			// color after the branches', as a floating topic's. (It had the
			// index -1, and a logic chart with one would not draw at all.)
			if branch[n.ID] = slices.Index(p.Children, n); branch[n.ID] < 0 {
				branch[n.ID] = len(p.Children)
			}
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
	if !exporting {
		a.overCanvas = cv.Hovered()
	}
	if a.relFrom != "" && !exporting { // the line being drawn follows the pointer
		a.relTo = nil
		if x, y, over := cv.PointerPosition(); over {
			b := a.vp
			a.relTo = &[2]float32{b.X + x, b.Y + y}
		}
	}
	if cv.Pressed() && a.editing == "" {
		a.canvas.Focus()
	}
	if cv.Pressed() {
		a.selImage = "" // its click, inside, selects it again
	}
	if cv.Pressed() && !exporting && a.relFrom == "" { // a relation line near the pointer is selected
		if x, y, over := cv.PointerPosition(); over {
			a.selRel = a.relationAt(x, y)
		}
	}
	if cv.Clicked() {
		if !a.justFloated { // not the end of the double click that made one
			a.commitEditIfAny()
		}
		a.justFloated = false
		a.multi = nil
		if lens != nil && !a.overNode() && !a.overLensRow(c) && a.editing == "" && a.relFrom == "" {
			// Under a lens the bare canvas is nothing chosen: the right
			// column says the lens again.
			a.selected, a.lensRoot = shown.Root.ID, false
		}
	}
	if cv.DoubleClicked() && !exporting && shown == a.doc && !a.overNode() && !a.overLensRow(c) {
		x, y := a.pointer[0], a.pointer[1]
		a.later(func() { // a floating topic where it was double-clicked
			a.addFloatingAt(x, y)
			a.justFloated = true
		})
	}
	if !exporting && shown == a.doc {
		cv.ContextMenu(a.canvasMenu)
	}
	cv.Draw(func(p *ui.Painter, rect ui.Rect) {
		color := func(id string) ui.Color {
			if lens != nil {
				return hairline(t) // a lens has one hue, and it is the topics'
			}
			col := lk.branch(branch[id])
			if a.filtering() {
				col = col.Alpha(0.3)
			}
			return col
		}
		relations := ui.Hex("#6366f1")
		if lens != nil {
			relations = faint(t)
		}
		view := rect
		if exporting {
			view = ui.Rect{} // all of it
		}
		soft := func(id string) ui.Color { return color(id) } // the lines in their branch's color
		lines := func() {
			a.drawBoundaries(p, r, shown, rect.X+cam.X, rect.Y+cam.Y, z, color)
			drawEdges(p, r, a.doc.Structure, rect.X+cam.X, rect.Y+cam.Y, z, lk.line, view, soft)
			a.drawSummaries(p, r, shown, rect.X+cam.X, rect.Y+cam.Y, z, color)
			a.drawRelations(p, r, rect.X+cam.X, rect.Y+cam.Y, z, relations)
		}
		if exporting {
			lines()
		} else {
			// Within the canvas: a line running off it would cross the
			// toolbar.
			p.Clip(rect, 0, lines)
		}
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
			col := lk.branch(branch[n.ID])
			if lens != nil {
				col = t.TextMuted // what a topic draws in its branch's color: its ring, its key to fold
			}
			a.nodeView(c.Key(n.ID), n, depth, box, col)
		})
		if exporting {
			return
		}
		if a.selRel != "" && shown == a.doc {
			a.relationHandle(c)
		}
		a.focusBar(c)
		if shown == a.doc {
			// By the depth its topic is drawn at, a floating topic's as a
			// main topic's: the bar's ⋯ is the menu under the pointer's
			// other key, and by the depth of the tree alone a floating
			// topic was its middle there, not cut, deleted nor moved.
			depth := max(len(shown.Path(a.selected))-1, 0)
			if floating[a.selected] != nil {
				depth++
			}
			if !a.lensQuiet { // nothing chosen: no topic's bar
				a.nodeBar(c, r, depth)
			}
		}
		// Zoom and recenter, over the canvas's corner.
		lifted(ui.Row(c).Absolute().Right(14).Bottom(14).Gap(0).Padding(3, 5).Radius(radiusPill).AlignItems(ui.Center).
			Background(cardColor(t)), t).Bind(&a.canvasBar).Children(func() {
			if iconButton(c, iconMinus, "缩小", "缩小 ⌘-", 28, true).Clicked() {
				a.zoomStep(false)
			}
			zoom := ui.Box(c.Key("zoom")).Height(28).MinWidth(48).Padding(0, 8).Radius(6).Center().Cursor(ui.CursorPointer).
				Label("缩放比例").Tooltip("实际大小 ⌘0")
			if zoom.Hovered() {
				zoom.Background(t.SurfaceHover)
			}
			if zoom.Clicked() {
				a.setZoom(1)
			}
			zoom.Children(func() {
				ui.Text(c, fmt.Sprintf("%d%%", int(cam.Zoom*100+0.5))).FontSize(11).TextColor(t.TextMuted).PassThrough()
			})
			if iconButton(c, iconPlus, "放大", "放大 ⌘=", 28, true).Clicked() {
				a.zoomStep(true)
			}
			ui.Box(c).Width(1).Height(14).Background(hairline(t)).Margin(0, 2)
			if iconButton(c, iconTarget, "回到中心", "回到中心 ⌘⇧0", 28, true).Clicked() {
				a.recenter()
			}
			iconButton(c, iconLevels, "层级", "折叠 / 展开到第几层", 28, true).Menu(a.levelMenu)
			a.taskFilterKey(c)
			// While a picture is being written its key waits, in its place
			// and of its size (wait.go: one action, on its key).
			if waitShown(c, "key:导出图片", a.imageBusy) {
				ui.Box(c.Key("导出中")).Size(28, 28).Center().Shrink(0).Label("正在导出图片").Tooltip("正在导出图片…").Children(func() {
					ui.Spinner(c).Size(13, 13)
				})
			} else {
				iconButton(c, iconDownload, "导出", "导出图片、Markdown、XMind…", 28, true).Menu(a.exportMenu)
			}
		})
		a.lensBar(c)
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

// canvasInput takes the wheel, the touchpad and the keys on the canvas.
func (a *app) canvasInput(ev ui.InputEvent) bool {
	cam := a.cam()
	switch ev.Kind {
	case ui.InputPointerMove:
		a.pointer = [2]float32{ev.X, ev.Y}
		return false
	case ui.InputPointerDown:
		a.pressAt = [2]float32{ev.X, ev.Y} // where the canvas's menu opens (canvasMenu)
		return false
	case ui.InputScroll:
		mods := ev.Mods | a.mods
		if mods&(ui.Cmd|ui.Ctrl) != 0 {
			f := float32(math.Exp(float64(-ev.DY) * 0.01))
			if !ev.Precise {
				f = float32(math.Pow(1.1, float64(-ev.DY)/40))
			}
			a.zoomBy(f, a.pointer[0], a.pointer[1], ev.Precise)
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
	ui.ScrollHorizontal(c.Key("sheets")).Background(t.Background).BorderWidth(1, 0, 0, 0).BorderColor(hairline(t)).Children(func() {
		ui.Row(c).Padding(5, 10).Gap(2).AlignItems(ui.Center).Children(func() {
			for _, d := range a.file.Sheets {
				a.sheetTab(c.Key(d.ID), d)
			}
			if iconButton(c, iconPlus, "新建画布", "新建画布", 26, true).Clicked() {
				a.addSheet()
			}
		})
	})
}

// removeSheet deletes a canvas at once and says so, with the way back
// (DESIGN.md's States: what is deleted goes at once and the toast offers
// 撤销; it was asked about first). 撤销 puts that canvas back where it was
// among the others, by its ID, whatever was done meanwhile.
func (a *app) removeSheet(id string) {
	d, at := a.file.Sheet(id)
	if d == nil || len(a.file.Sheets) <= 1 {
		return
	}
	path := a.path
	a.deleteSheet(id)
	a.tellUndo("已删除画布「"+d.Title+"」", func() {
		f := a.mapFile(path)
		if f == nil {
			return
		}
		if back, _ := f.Sheet(id); back != nil {
			return // there again, by an undo
		}
		_ = a.editMapAt(path, "撤销删除画布", func(f *model.File) error {
			f.Sheets = slices.Insert(f.Sheets, min(at, len(f.Sheets)), d.Clone())
			return nil
		})
		if a.path == path {
			a.showSheet(id)
		}
	})
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
	tab := ui.Row(c).Padding(5, 14).Radius(radiusPill).Cursor(ui.CursorPointer).Label(d.Title).
		Background(t.Text.Alpha(0)).Transition(easeLook)
	if active {
		tab.Background(chosen(t))
	} else if tab.Hovered() {
		tab.Background(t.Text.Alpha(0.045))
	}
	if a.renamingSheet == d.ID {
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
		if m.Item("删除画布").Disabled(len(a.file.Sheets) == 1).Chosen() {
			a.later(func() { a.removeSheet(id) })
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
	// Under a lens with nothing chosen (lensOver) the middle, which is
	// what is chosen then, shows no ring: nothing is.
	selected := a.isSelected(n.ID) && !(depth == 0 && a.lensQuiet)
	editing := n.ID == a.editing
	side := 0 // where a node dragged over it would go: -1 before it, 1 after

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
	// Quietly: the central topic solid, the main topics tinted with their
	// branch's color and written in a deep shade of it, and the topics
	// below them all alike, on a faint gray.
	// The central topic in the ink; the main topics on a pastel of their
	// branch's color, written in the ink; those below on a faint tint.
	branchText := t.Text
	switch depth {
	case 0:
		fill, filled = lookOf(a.doc.Style).root(t), true
		el.Radius(12 * z)
	case 1:
		tint := float32(0.26)
		if t.Dark {
			tint = 0.34
		}
		fill = accent.Alpha(tint)
		el.Radius(10 * z)
	default:
		tint := float32(0.09)
		if t.Dark {
			tint = 0.16
		}
		fill = accent.Alpha(tint)
		el.Radius(8 * z)
	}
	// A summary's topic: a pill outlined in its branch's color, its text
	// in it too, lightly tinted, unlike the topics it sums up.
	summary := a.summaryTopics[n.ID] && st.Fill == "" && st.Shape == "" && st.Border == ""
	if summary {
		fill, filled = accent.Alpha(0.07), false
		el.Radius(999).Border(1.5*z, accent)
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
	if depth == 1 {
		onFill = branchText
	}
	if filled {
		onFill = textOn(fill)
	}
	if summary {
		onFill = accent.MixOklab(t.Text, 0.4) // toward the text, to read on the canvas
	}
	if st.Color != "" {
		onFill = ui.Hex(st.Color)
	}
	el.TextColor(onFill)
	if st.Border != "" {
		el.Border(max(st.BorderWidth, 1)*z, ui.Hex(st.Border))
	}
	// Through a lens (lens.go) a topic is a step of the lens's one hue,
	// whatever its branch's color or its own: its fill, its words in the
	// ink; the step that has none is white with a hairline, its words
	// muted. Its shape stays, and the middle stays in the ink.
	lt, lensed := lensTopic{}, false
	if a.lensShown != nil && depth > 0 {
		lt, lensed = a.lensShown.topics[n.ID]
	}
	if lensed {
		var plain bool
		fill, plain = lensFill(t, a.lensShown.lens, lt.step)
		filled, onFill = false, t.Text
		el.Background(fill).BorderWidth(0, 0, 0, 0)
		if plain {
			onFill = t.TextMuted
			el.Border(1, hairline(t))
		}
		el.TextColor(onFill)
		if el.Hovered() {
			el.Tooltip(a.lensFact(n.ID))
		}
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
		} else if el.Pressed() && a.selRel != "" {
			a.selRel = "" // a node pressed: the line is no longer selected
			if !adding {
				a.commitEditIfAny()
				a.selected, a.multi = n.ID, nil
			}
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
		if depth == 0 && el.Clicked() {
			a.lensRoot = true // chosen by hand: under a lens its page shows, not the lens's
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
			// A floating topic goes where it is dragged, and let go over
			// a topic it goes under it: that topic shows it would take it.
			// The pointer is told from the box as it was laid out last,
			// before what the last frame moved it by.
			if dx, dy, ok := el.Dragged(); ok {
				if a.floatDrag != n.ID && (dx != 0 || dy != 0) {
					a.floatDrag, a.floatMoved = n.ID, [2]float32{}
				}
				if a.floatDrag == n.ID { // moved: a press alone shows nothing
					x, y, _ := el.PointerPosition()
					a.floatOver = a.nodeAt(box.X-a.floatMoved[0]+x/z, box.Y-a.floatMoved[1]+y/z, n)
					a.floatMoved = [2]float32{dx / z, dy / z}
				}
				if dx != 0 || dy != 0 {
					a.mutate("float:"+n.ID, func() { n.Pos.X += dx / z; n.Pos.Y += dy / z })
				}
			}
			if _, _, ok := el.DragEnded(); ok && a.floatDrag == n.ID && a.floatOver != "" {
				id, to := n.ID, a.floatOver
				a.later(func() { a.joinFloating(id, to) })
			}
			if _, held := el.PressedButton(); !held && a.floatDrag == n.ID {
				a.floatDrag, a.floatOver = "", ""
			}
			el.Cursor(ui.CursorGrab)
		} else {
			// Other nodes are dropped on a node to move under it, and on
			// the bare canvas to stand there, out of the tree.
			el.Drag(nodeDrag{n.ID}).Cursor(ui.CursorPointer)
			if at, ok := el.DragMissed(); ok && !summary {
				id := n.ID
				a.later(func() { a.floatDropped(id, at.X, at.Y) })
			}
		}
		// A node dropped on it moves under it, or, near the edge its
		// siblings are on, before or after it; a document or a map from
		// the sidebar is linked to it.
		if d, over := ui.DragOver[canvasDrop](el); over {
			if nd, ok := d.(nodeDrag); !ok || nd.ID != n.ID {
				if !ok {
					el.Border(2*z, t.Accent).BorderStyle(ui.BorderDashed)
				} else if side = a.dropSide(el, n, box, nd.ID); side == 0 {
					el.Border(2*z, t.Accent).BorderStyle(ui.BorderDashed)
				}
			}
		}
		if a.floatOver == n.ID {
			el.Border(2*z, t.Accent).BorderStyle(ui.BorderDashed)
		}
		if d, ok := ui.Drop[canvasDrop](el); ok {
			id := n.ID
			switch d := d.(type) {
			case nodeDrag:
				if side := a.dropSide(el, n, box, d.ID); side != 0 {
					a.later(func() { a.placeNode(d.ID, id, side > 0) })
					break
				}
				a.later(func() { a.moveNode(d.ID, id) })
			case fileDrag:
				a.later(func() { a.dropMaterial(id, d) })
			}
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
	linkCol := accentOn(filled, onFill, t.Accent)
	if lensed {
		linkCol = muted // a lens has one hue: the links' accent would be a second
	}
	el.Children(func() {
		if side != 0 { // a bar on the edge the node dragged would go by
			bar := ui.Box(c.Key("drop-bar")).Absolute().Background(t.Accent).Radius(2 * z).PassThrough()
			const thick, out = 4, 7
			switch across := a.siblingsAcross(n); {
			case across && side < 0:
				bar.Left(-out*z).Top(0).Size(thick*z, box.H*z)
			case across:
				bar.Left((box.W+out-thick)*z).Top(0).Size(thick*z, box.H*z)
			case side < 0:
				bar.Left(0).Top(-out*z).Size(box.W*z, thick*z)
			default:
				bar.Left(0).Top((box.H+out-thick)*z).Size(box.W*z, thick*z)
			}
		}
		if iw > 0 {
			a.thumbnail(c, n, iw*z, ih*z).Absolute().Left((box.W - iw) / 2 * z).Top(nodePadY * z)
			if a.pictureOnly(n) {
				return // the picture alone, no title under it
			}
		}
		if n.Task != nil {
			if !n.HasStatusMarker() { // its progress marker shows the status
				a.taskCheck(c, n, checkSize*z)
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
							n.FollowProgress(a.now())
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
		tw, th := a.titleBox(c, n, depth)
		if editing {
			// A text area, wrapping as the title will, rather than a line
			// that scrolls what was typed first out of view. Enter ends
			// the edit; a title has no line breaks.
			// On a light ground of its own, whatever the node's fill: the
			// caret, drawn in the accent color, and the selection show.
			// As high as the title is laid out: should the field wrap a
			// line the title does not, it scrolls rather than spill out
			// of the node.
			// The ground reaches past the title by its padding (the margins
			// pull it back), so the text keeps the title's whole width.
			in := ui.TextAreaBase(c.Key("edit"), &a.editText).FontSize(size).FontWeight(fontWeight(n, depth)).
				Font(a.doc.Style.Font).Placeholder("新主题").Width((tw+6)*z).Height((th+4)*z).AutoFocus().
				Background(t.Background).TextColor(t.Text).Radius(4*z).Margin(-2*z, -3*z).Padding(2*z, 3*z)
			if c := in.Composing(); c != a.editComposing {
				a.editComposing = c // the node makes room, from the next frame
			}
			// A picture pasted, not text, goes on the node, as on the canvas.
			in.OnPaste(func(text string) bool { return text == "" && a.pasteImage() })
			in.HandleInput(func(ev ui.InputEvent) bool {
				if ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyTab && ev.Mods == 0 && a.editing == n.ID {
					// A #word begun: Tab makes it the first tag offered.
					if part, tag, ok := a.titleTagTab("canvas:"+n.ID, a.editText); ok {
						a.takeTitleTag(n, part, tag)
						return true
					}
					return false
				}
				if ev.Kind != ui.InputKeyDown || ev.Key != ui.KeyEnter || ev.Mods&^ui.Shift != 0 || a.editing != n.ID {
					return false
				}
				a.commitEdit()
				return true
			})
			// A #word being written: under the title, whether it is to be a
			// tag, a click or Tab making it one; not chosen, it stays words of
			// the title. Every #word a tag was a rule the user could not see,
			// nor say no to (0.98.1).
			offering := a.tagOfferShown("canvas:"+n.ID, a.editText) // as the frame began: Escape below puts it away
			a.titleTagOffer(c, in, "canvas:"+n.ID, a.editText, func(part, tag string) { a.takeTitleTag(n, part, tag) })
			if strings.ContainsAny(a.editText, "\r\n\t") { // pasted lines join
				a.editText = strings.Join(strings.Fields(strings.ReplaceAll(a.editText, "\r", "\n")), " ")
			}
			if in.Changed() {
				a.reveal = true // the node grows as it is typed in: keep it in view
			}
			if in.Shortcut(0, ui.KeyEscape) {
				if offering {
					// The offer of a tag put away, the edit going on: its hint
					// says Esc 收起, and Esc ended the edit, the words lost.
					a.dismissTagOffer("canvas:"+n.ID, a.editText)
				} else {
					a.cancelEdit()
				}
			} else if in.Focused() {
				if !a.editFocus {
					// The caret at the end: a selection replaced by an input
					// method's text puts its candidates at the screen's corner.
					n := utf8.RuneCountInString(a.editText)
					in.SetTextSelection(n, n)
				}
				a.editFocus = true
			} else if a.editFocus {
				a.commitEdit() // the focus went elsewhere
			}
			// Its tags show as it is edited, where they stand, each taken
			// off by a click: they went with the title, and could not be seen
			// nor changed while it was written.
			for _, tag := range n.Tags {
				chip := tagChip(c.Key("edit-tag:"+tag), tag, tag+" ✕", false, markSize*z).KeepFocus().
					Label("去掉标签 " + tag).Tooltip("点一下去掉这个标签")
				if chip.Clicked() {
					tg := tag
					a.later(func() { a.toggleTag(n, tg) })
				}
			}
			return
		}
		// The web addresses written in it show as links: the node's mark
		// opens them.
		var spans []ui.Span
		last := 0
		for _, r := range titleLinks(n.Title) {
			spans = append(spans, ui.Span{Text: n.Title[last:r[0]]},
				ui.Span{Text: n.Title[r[0]:r[1]], Color: linkCol, Underline: true})
			last = r[1]
		}
		spans = append(spans, ui.Span{Text: n.Title[last:]})
		title := ui.RichText(c, spans...).FontSize(size).FontWeight(fontWeight(n, depth)).Width(tw * z).Shrink(0)
		if a.titleOneLine(c, n, depth) {
			title.NoWrap() // as measured, whatever the zoom does to the glyphs
		}
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
		if f := a.diaOf(n.ID); f != nil && !exportingNow(a) {
			a.diaMark(c, f, z)
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
		if links := nodeLinks(n); len(links) > 0 {
			// One link opens; several are listed. Their names are worked
			// out under the pointer only, as each looks up a clip or a map.
			mark := ui.Box(c.Key("links")).Cursor(ui.CursorPointer).Label("打开链接")
			if mark.Hovered() {
				var tips []string
				for _, l := range links {
					_, label := a.linkLabel(l)
					tips = append(tips, label)
				}
				if len(links) == 1 {
					mark.Tooltip("点击打开：" + tips[0])
				} else {
					mark.Tooltip(fmt.Sprintf("%d 个链接：%s", len(links), strings.Join(tips, "、")))
				}
			}
			if len(links) == 1 {
				if mark.Clicked() {
					link := links[0]
					a.later(func() { a.openLink(link) })
				}
			} else {
				mark.Menu(func(m *ui.Menu) {
					for _, l := range links {
						icon, label := a.linkLabel(l)
						if strings.HasPrefix(label, "（") { // gone
							m.Item("⚠︎ " + label).Disabled(true)
							continue
						}
						if m.Item(strings.TrimSpace(icon + " " + label)).Chosen() {
							link := l
							a.later(func() { a.openLink(link) })
						}
					}
				})
			}
			mark.Children(func() {
				ui.Text(c, linksMark(links)).FontSize((markSize + 1) * z).TextColor(linkCol).NoWrap()
			})
		}
	})

	if lensed && a.lensShown.lens == lensSrc && lt.n > 0 {
		a.lensCount(c, n, [4]float32{box.X, box.Y, box.W, box.H}, a.boxes.Left[n.ID], lt.n)
	}
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
// nodeMenu is what can be done with a topic, under the pointer's other
// key. The things done every day stand at its head and alone; the rest is
// in a submenu of its kind (插入, 任务, AI, 展开/折叠, 图片), and what a key
// does as well and nobody looks for here (编辑标题, F2 and a double click)
// is not in it: the user found it too long, with some thirty rows.
func (a *app) nodeMenu(m *ui.Menu, n *model.Node, depth int) {
	if !a.isSelected(n.ID) {
		a.selected, a.multi = n.ID, nil
	}
	if m.Item("添加子主题").Shortcut(0, ui.KeyTab).Chosen() {
		a.later(func() { a.addChild() })
	}
	if m.Item("添加同级主题").Shortcut(0, ui.KeyEnter).Chosen() {
		a.later(func() { a.addSibling() })
	}
	a.annotationMenu(m, n, depth)
	m.Separator()
	if m.Item("编辑备注…").Chosen() {
		a.noteEditing = n.ID
	}
	a.tagMenu(m, n)
	if m.Item("作为任务").Checked(n.Task != nil).Chosen() {
		a.later(func() { a.toggleTask(n) })
	}
	if n.Task != nil {
		m.Submenu("任务状态", func(m *ui.Menu) {
			for _, s := range model.Statuses {
				if m.Item(s.Label()).Checked(n.Task.Status == s).Chosen() {
					a.later(func() { a.setStatus(n, s) })
				}
			}
			m.Separator()
			if m.Item("在看板中显示").Chosen() {
				a.later(func() { a.showOnBoard(n.ID) })
			}
		})
	}
	m.Separator()
	if m.Item("复制").Shortcut(ui.Cmd, ui.KeyC).Chosen() {
		a.later(func() { a.copyNodes(false) })
	}
	if m.Item("剪切").Shortcut(ui.Cmd, ui.KeyX).Disabled(depth == 0).Chosen() {
		a.later(func() { a.copyNodes(true) })
	}
	if m.Item("粘贴为子主题").Chosen() {
		a.later(func() { a.paste() })
	}
	m.Separator()
	m.Submenu("AI", func(m *ui.Menu) {
		if m.Item("拆解成任务").Chosen() {
			a.later(a.aiBreakDown)
		}
		if a.diaOf(n.ID) != nil {
			if m.Item("看图解").Chosen() {
				a.later(func() { a.openDia(n.ID, "") })
			}
		} else if m.Item("画成图解").Disabled(!diaWorth(n)).Chosen() {
			a.later(func() { a.drawDia(n.ID, "") })
		}
	})
	if depth > 0 && m.Item("移到其他导图…").Chosen() {
		a.later(func() { a.openPalette(paletteMove) })
	}
	if a.shownRoot() == n && a.focusNode() != nil {
		if m.Item("退出聚焦").Shortcut(0, ui.KeyEscape).Chosen() {
			a.later(func() { a.focusOn("") })
		}
	} else if depth > 0 && m.Item("聚焦此分支").Shortcut(ui.Alt, ui.KeyEnter).Chosen() {
		id := n.ID
		a.later(func() { a.focusOn(id) })
	}
	if len(n.Children) > 0 {
		m.Submenu("展开/折叠", func(m *ui.Menu) {
			label := "折叠"
			if n.Collapsed {
				label = "展开"
			}
			if m.Item(label).Chosen() {
				a.later(func() { a.toggleCollapse(n.ID) })
			}
			if m.Item("展开全部下级").Chosen() {
				a.later(func() { a.setCollapsedBelow(n, false) })
			}
			if m.Item("折叠全部下级").Chosen() {
				a.later(func() { a.setCollapsedBelow(n, true) })
			}
		})
	}
	if n.Image != nil {
		m.Submenu("图片", func(m *ui.Menu) {
			if m.Item("查看大图").Chosen() {
				a.lightbox = n.ID
			}
			if img := n.Image; m.Item("复制图片").Chosen() {
				a.copyImage(img)
			}
			if m.Item("删除图片").Chosen() {
				a.later(func() { a.removeImage(n) })
			}
		})
	}
	m.Separator()
	label := "删除"
	if len(a.multi) > 0 {
		label = fmt.Sprintf("删除所选 %d 个主题", len(a.selection()))
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
	case ev.Mods == ui.Shift && ev.Key == ui.KeyEnter:
		a.addSiblingAt(false)
	case ev.Mods == 0 && (ev.Key == ui.KeyDelete || ev.Key == ui.KeyBackspace):
		if a.selRel != "" { // the line selected, rather than the node
			id := a.selRel
			a.selRel = ""
			a.removeRelation(id)
			break
		}
		if n, _ := a.selectedNode(); n != nil && a.selImage == n.ID && n.Image != nil { // its picture
			a.selImage = ""
			a.removeImage(n)
			break
		}
		a.deleteSelection()
	case ev.Mods == 0 && ev.Key == ui.KeyEscape:
		switch {
		case a.relFrom != "":
			a.relFrom = ""
		case a.selRel != "":
			a.selRel = ""
		case a.lightbox != "":
			a.lightbox = ""
		case a.selImage != "":
			a.selImage = ""
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
	case ev.Mods == ui.Cmd|ui.Shift && ev.Key == ui.KeyL:
		if n, _ := a.selectedNode(); n != nil {
			a.startRelation(n)
		}
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
		drawOrgEdges(p, r, ox, oy, z, width, view, color)
		return
	case model.StructureTimeline, model.StructureFishbone:
		drawLineEdges(p, r, s == model.StructureFishbone, ox, oy, z, width, view, color)
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
			mid := (x1 - x0) / 2
			if max(x1-x0, x0-x1, y1-y0, y0-y1) > strokeChunk {
				strokeCurve(p, view, curve{{x0, y0}, {x0 + mid, y0}, {x1 - mid, y1}, {x1, y1}}, width, color(e.To))
				continue
			}
			var path ui.Path
			path.MoveTo(x0, y0)
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
				strokeLong(p, view, polyline{{tx, min(top, py)}, {tx, max(bottom, py)}}, width, col)
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
		if bottom-top > strokeChunk { // too tall to draw at once
			l := polyline{{bx + rad, top}}
			l.quad(bx, top, bx, top+rad)
			l = append(l, [2]float32{bx, mid - rad})
			l.quad(bx, mid, bx-rad, mid)
			l.quad(bx, mid, bx, mid+rad)
			l = append(l, [2]float32{bx, bottom - rad})
			l.quad(bx, bottom, bx+rad, bottom)
			strokeLong(p, view, l, width, col)
		} else {
			var brace ui.Path
			brace.MoveTo(bx+rad, top)
			brace.QuadTo(bx, top, bx, top+rad)
			brace.LineTo(bx, mid-rad)
			brace.QuadTo(bx, mid, bx-rad, mid)
			brace.QuadTo(bx, mid, bx, mid+rad)
			brace.LineTo(bx, bottom-rad)
			brace.QuadTo(bx, bottom, bx+rad, bottom)
			p.StrokePath(&brace, width, col)
		}
		strokeLong(p, view, polyline{{px, py}, {bx - rad, mid}}, width, col)
	}
}

// strokeChunk is the most, in points, a line is drawn in at once: MyGo
// leaves out a path over 4096 pixels across, which a bracket around a
// big branch, a few thousand points tall, is on a Retina screen.
const strokeChunk = 600

// polyline is a line through points, for strokeLong.
type polyline [][2]float32

// quad adds a quadratic curve from the last point.
func (l *polyline) quad(cx, cy, x, y float32) {
	x0, y0 := (*l)[len(*l)-1][0], (*l)[len(*l)-1][1]
	const n = 6
	for i := 1; i <= n; i++ {
		t := float32(i) / n
		u := 1 - t
		*l = append(*l, [2]float32{u*u*x0 + 2*u*t*cx + t*t*x, u*u*y0 + 2*u*t*cy + t*t*y})
	}
}

// cube adds a cubic curve from the last point, in steps of a few points.
func (l *polyline) cube(c1x, c1y, c2x, c2y, x, y float32) {
	x0, y0 := (*l)[len(*l)-1][0], (*l)[len(*l)-1][1]
	d := float32(math.Hypot(float64(x-x0), float64(y-y0)))
	n := min(max(int(d/4), 8), 2000)
	for i := 1; i <= n; i++ {
		t := float32(i) / float32(n)
		u := 1 - t
		a, b, c, e := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
		*l = append(*l, [2]float32{a*x0 + b*c1x + c*c2x + e*x, a*y0 + b*c1y + c*c2y + e*y})
	}
}

// curve is a cubic curve: its ends and, between them, its two controls.
type curve [4][2]float32

// halves cuts the curve in two at its middle.
func (c curve) halves() (curve, curve) {
	mid := func(a, b [2]float32) [2]float32 { return [2]float32{(a[0] + b[0]) / 2, (a[1] + b[1]) / 2} }
	ab, bc, cd := mid(c[0], c[1]), mid(c[1], c[2]), mid(c[2], c[3])
	abc, bcd := mid(ab, bc), mid(bc, cd)
	m := mid(abc, bcd)
	return curve{c[0], ab, abc, m}, curve{m, bcd, cd, c[3]}
}

// strokeCurve draws a curve that may be longer than a path can be, as a
// mind map's branch to a child far above or below: halved until a part is
// within strokeChunk across, and of the parts only those in view drawn,
// each a curve of its own. Sampled whole into points, as it was, each of
// the five hundred branches of a topic with as many children was up to
// two thousand points a frame, nearly all of them out of view.
func strokeCurve(p *ui.Painter, view ui.Rect, c curve, width float32, col ui.Color) {
	// A curve is within the box of its ends and controls.
	x0, y0, x1, y1 := c[0][0], c[0][1], c[0][0], c[0][1]
	for _, pt := range c[1:] {
		x0, y0, x1, y1 = min(x0, pt[0]), min(y0, pt[1]), max(x1, pt[0]), max(y1, pt[1])
	}
	if outside(view, x0, y0, x1, y1) {
		return
	}
	if x1-x0 > strokeChunk || y1-y0 > strokeChunk {
		l, r := c.halves()
		strokeCurve(p, view, l, width, col)
		strokeCurve(p, view, r, width, col)
		return
	}
	var path ui.Path
	path.MoveTo(c[0][0], c[0][1])
	path.CubeTo(c[1][0], c[1][1], c[2][0], c[2][1], c[3][0], c[3][1])
	p.StrokePath(&path, width, col)
	curveParts++
}

// curveParts counts the parts strokeCurve drew, for the tests.
var curveParts int

// strokeLong draws a line that may be longer than a path can be: in
// pieces of at most strokeChunk across, leaving out those outside view.
func strokeLong(p *ui.Painter, view ui.Rect, l polyline, width float32, col ui.Color) {
	if len(l) < 2 {
		return
	}
	// Long segments are cut, so that each fits in a piece.
	pts := polyline{l[0]}
	for _, pt := range l[1:] {
		a := pts[len(pts)-1]
		dx, dy := pt[0]-a[0], pt[1]-a[1]
		n := int(max(dx, -dx, dy, -dy)/(strokeChunk/2)) + 1
		for i := 1; i <= n; i++ {
			t := float32(i) / float32(n)
			pts = append(pts, [2]float32{a[0] + dx*t, a[1] + dy*t})
		}
	}
	draw := func(piece polyline) {
		x0, y0, x1, y1 := piece[0][0], piece[0][1], piece[0][0], piece[0][1]
		for _, pt := range piece {
			x0, y0, x1, y1 = min(x0, pt[0]), min(y0, pt[1]), max(x1, pt[0]), max(y1, pt[1])
		}
		if outside(view, x0, y0, x1, y1) {
			return
		}
		var path ui.Path
		path.MoveTo(piece[0][0], piece[0][1])
		for _, pt := range piece[1:] {
			path.LineTo(pt[0], pt[1])
		}
		p.StrokePath(&path, width, col)
	}
	start := 0
	x0, y0, x1, y1 := pts[0][0], pts[0][1], pts[0][0], pts[0][1]
	for i := 1; i < len(pts); i++ {
		pt := pts[i]
		nx0, ny0, nx1, ny1 := min(x0, pt[0]), min(y0, pt[1]), max(x1, pt[0]), max(y1, pt[1])
		if nx1-nx0 > strokeChunk || ny1-ny0 > strokeChunk { // a new piece, from the last point
			draw(pts[start:i])
			start = i - 1
			a := pts[start]
			nx0, ny0 = min(a[0], pt[0]), min(a[1], pt[1])
			nx1, ny1 = max(a[0], pt[0]), max(a[1], pt[1])
		}
		x0, y0, x1, y1 = nx0, ny0, nx1, ny1
	}
	draw(pts[start:])
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
func drawOrgEdges(p *ui.Painter, r layout.Result, ox, oy, z, width float32, view ui.Rect, color func(string) ui.Color) {
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
		strokeLong(p, view, polyline{{left, bar}, {right, bar}}, width, col)
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
func drawLineEdges(p *ui.Painter, r layout.Result, fishbone bool, ox, oy, z, width float32, view ui.Rect, color func(string) ui.Color) {
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
		strokeLong(p, view, polyline{{start, spineY}, {end, spineY}}, width*2, color(root))
		p.StrokePath(new(ui.Path).MoveTo(end, spineY).LineTo(end-10*z, spineY-8*z), width*2, color(root))
		p.StrokePath(new(ui.Path).MoveTo(end, spineY).LineTo(end-10*z, spineY+8*z), width*2, color(root))
	} else if !fishbone && end > start {
		strokeLong(p, view, polyline{{start, spineY}, {end, spineY}}, width*2, color(root))
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
		strokeLong(p, view, polyline{{x, y0}, {x, y1}, {ox + cb.X*z, y1}}, width, color(e.To))
	}
}

// structureLabel is what the structure's key says: the structure, and
// with room its colors.
func (a *app) structureLabel(short bool) string {
	if short {
		return a.doc.Structure.Label()
	}
	return a.doc.Structure.Label() + " · " + schemeOf(a.doc.Style.Scheme).name + "配色"
}

// structureBare tells whether the structure's key is its drawing alone:
// in a bar with no room for its name beside the views' tabs.
func structureBare(room float32) bool { return room < 700 }

// structureRoom keeps the structure's key its room in the views that have
// none: unseen, and as wide. Without it the bar's other keys, and at a
// narrower window the views' tabs, moved as the view was changed.
func (a *app) structureRoom(c *ui.Context, room float32) {
	ui.Row(c.Key("structure-room")).Height(30).Padding(0, 10, 0, 9).Gap(7).AlignItems(ui.Center).Shrink(0).Opacity(0).PassThrough().Children(func() {
		ui.Box(c).Size(22, 14).Shrink(0).PassThrough()
		if !structureBare(room) {
			ui.Text(c, a.structureLabel(room < 900)).FontSize(textSmall).NoWrap().PassThrough()
		}
		chevron(c, c.Theme().TextMuted)
	})
}

// structurePicker is the toolbar's button that chooses the canvas's
// structure, from pictures of each, and its colors; in less room it says
// the structure alone, and in less yet only draws it.
func (a *app) structurePicker(c *ui.Context, room float32) {
	t := c.Theme()
	// A key as the others of the bar: the structure drawn small, its name,
	// and the chevron of what opens.
	btn := key(ui.Row(c.Key("structure")).Height(30).Padding(0, 10, 0, 9).Gap(7).Radius(radiusPill).AlignItems(ui.Center).Shrink(0).
		Cursor(ui.CursorPointer).Label("结构").Tooltip("换这个画布的结构（思维导图、逻辑图…）和配色"), t)
	if a.structureOpen {
		btn.Background(t.Text.Alpha(0.1))
	}
	if btn.Clicked() {
		a.structureOpen = !a.structureOpen
	}
	btn.Children(func() {
		s := a.doc.Structure
		ui.Box(c).Size(22, 14).Shrink(0).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) { drawStructureIcon(p, r, s, t.TextMuted) })
		if !structureBare(room) {
			ui.Text(c, a.structureLabel(room < 900)).FontSize(textSmall).NoWrap().PassThrough()
		}
		chevron(c, t.TextMuted)
	})
	ui.Popover(c, btn, &a.structureOpen, func() {
		sectionHeader(c, "结构").Margin(4, 6, 2)
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
		// The colors of the branches, a scheme a chip: its first colors and
		// its name.
		sectionHeader(c, "配色").Margin(8, 6, 2)
		ui.Row(c).Gap(6).Wrap().GapY(6).Padding(4).MaxWidth(430).Children(func() {
			for _, sc := range schemes {
				chip := ui.Row(c.Key("scheme:"+sc.id)).Height(30).Padding(0, 10, 0, 8).Gap(6).Radius(8).AlignItems(ui.Center).
					Border(1, t.Border).Cursor(ui.CursorPointer).Label("配色 " + sc.name)
				if sc.id == schemeOf(a.doc.Style.Scheme).id {
					chip.Border(2, t.Accent)
				} else if chip.Hovered() {
					chip.Background(t.SurfaceHover)
				}
				if chip.Clicked() {
					id := sc.id
					a.later(func() { a.setScheme(id) })
				}
				chip.Children(func() {
					ui.Row(c).Gap(2).PassThrough().Children(func() {
						for i := range 4 {
							ui.Box(c).Size(9, 9).Radius(5).Background(ui.Hex(sc.colors[i]))
						}
					})
					ui.Text(c, sc.name).FontSize(textSmall).PassThrough()
				})
			}
		})
		ui.Row(c).Gap(4).Padding(6, 4, 2).Children(func() {
			if ghostButton(c, "画布", "分支线、字体、背景等更多画布设置").Clicked() {
				a.structureOpen = false
				a.panelTab, a.panelOff = tabCanvas, false
			}
			if ghostButton(c, "演说", "演说模式的幻灯片和讲稿").Clicked() {
				a.structureOpen = false
				a.panelTab, a.panelOff = tabPresent, false
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

// taskFilterKey is the key of the canvas's bar, by its levels, that shows
// some tasks alone, bright, the rest of the map dimmed as a tag's filter
// does: those not done, or those of this week (and the late). On the
// accent while it narrows, so that a dimmed map is never a riddle. (The
// map's bar at the top has no room for one more key in a narrow window.)
func (a *app) taskFilterKey(c *ui.Context) {
	t := c.Theme()
	names := []string{"全部主题", "没做完的任务", "本周的任务"}
	tip := "只让一部分任务亮着，其余变淡"
	if a.taskFilter != taskFilterNone {
		tip = "现在只亮着" + names[a.taskFilter] + "：点这里换回全部"
	}
	b := iconButton(c, iconFilter, "筛选任务", tip, 28, true)
	if a.taskFilter != taskFilterNone {
		b.Background(t.Accent.Alpha(0.18))
	}
	b.Menu(func(m *ui.Menu) {
		for i, name := range names {
			if m.Item(name).Checked(a.taskFilter == i).Chosen() {
				a.taskFilter = i
			}
		}
	})
}
