package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// Capturing thoughts as they come: a global shortcut opens a small window
// from any app; what is typed goes to the inbox, a map of its own, or
// under the node selected in the open map. The inbox is sorted out later,
// moving its nodes to the maps they belong in. An icon in the menu bar
// lists the tasks due today.

// inboxName is the inbox map's name, at the top of the maps folder.
const inboxName = "收件箱"

// defaultCaptureKey opens the capture window from anywhere.
const defaultCaptureKey = "Control+Alt+Space"

func (a *app) inboxPath() string { return filepath.Join(a.dir, model.FileName(inboxName)) }

// captureKey is the shortcut of the capture window.
func (a *app) captureKey() string {
	if a.cfg.CaptureKey != "" {
		return a.cfg.CaptureKey
	}
	return defaultCaptureKey
}

// Where captured text goes.
const (
	captureInbox   = iota
	captureLibrary // a note of 灵感, a text of the library: the default
	captureHere    // under the node selected in the open map
	captureAuto    // a line that says a day or a priority is a task, else a note
)

// taskLike tells whether a text reads as a task rather than a note: one
// line, with a day or a priority said as a word of its own (明天 写周报,
// 写周报 !高), or lines of "- [ ]". 今天很好 is a note.
func taskLike(text string, now time.Time) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	if checklist(text) != nil {
		return true
	}
	if strings.Contains(text, "\n") {
		return false
	}
	if rest, rep, _ := parseRepeat(text, now); rep != nil && rest != "" {
		return true // 每周五 写周报: a task that repeats
	}
	words := strings.Fields(text)
	if len(words) < 2 {
		return false
	}
	for _, w := range words {
		if title, due, prio := parseTaskLine(w, now); title == "" && (due != nil || prio != model.PriorityNone) {
			return true
		}
	}
	return false
}

// capture adds text to the inbox, or under the node selected: a node a
// line, nested by indentation, as a pasted outline is.
func (a *app) capture(text string, where int) error {
	if where == captureAuto {
		where = captureLibrary
		if taskLike(text, a.now()) {
			where = captureInbox
		}
	}
	if where == captureLibrary { // a note of 灵感
		if _, err := a.addMemo(text, "快速记录"); err != nil {
			return err
		}
		a.tell("已记到灵感")
		return nil
	}
	if where == captureInbox { // tasks, a line each, to the inbox
		var lines []string
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- [ ]"))
			if line != "" {
				lines = append(lines, line)
			}
		}
		a.addTasks(lines, nil) // said once: a toast a task, only the last was read
		return nil
	}
	nodes := model.ParseOutline(text)
	if len(nodes) == 0 {
		return nil
	}
	if where == captureHere && a.doc != nil {
		parent, _ := a.selectedNode()
		if parent == nil {
			parent = a.doc.Root
		}
		a.mutate("", func() {
			parent.Children = append(parent.Children, nodes...)
			parent.Collapsed = false
		})
		a.tell(fmt.Sprintf("已记到「%s」下", parent.Title))
		return nil
	}
	path := a.inboxPath()
	if path == a.path { // the inbox is open: as an edit, undoable
		a.mutate("", func() { a.doc.Root.Children = append(a.doc.Root.Children, nodes...) })
		a.tell("已记到收件箱")
		return nil
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		f := model.NewFile(inboxName)
		f.Sheets[0].Root.Note = "随手记下的想法和待办，之后整理到对应的导图里（在主题的菜单里选「移到其他导图…」）。"
		if err := f.Save(path); err != nil {
			return err
		}
	}
	if err := a.editMapAt(path, "随手记", func(f *model.File) error {
		f.Sheets[0].Root.Children = append(f.Sheets[0].Root.Children, nodes...)
		return nil
	}); err != nil {
		return err
	}
	a.tell("已记到收件箱")
	return nil
}

// captureView is the capture window: where it goes on top, a field, and
// at the bottom the clipboard, the keys and the one button that saves.
func (a *app) captureView(c *ui.Context) {
	t := quietTheme(c.Theme())
	c.SetTheme(t)
	save := func() {
		if strings.TrimSpace(a.captureText) == "" {
			return
		}
		if err := a.capture(a.captureText, a.captureWhere); err != nil {
			a.captureErr = err.Error()
			return
		}
		a.captureText, a.captureErr = "", ""
		a.plat.HideCapture()
	}
	ui.Column(c).Fill().Padding(16, 18, 14).Gap(12).Background(t.Background).Children(func() {
		// Nothing to choose: what is written says what it is. A topic
		// selected in the open map may take it instead.
		here, hereTitle := false, ""
		if n, _ := a.selectedNode(); n != nil && a.path != a.inboxPath() {
			here, hereTitle = true, n.Title
		}
		if a.captureWhere == captureHere && !here {
			a.captureWhere = captureAuto
		}
		placeholder := "写下想法；写「明天 交周报」这样带日期的一句，就是任务"
		in := ui.TextAreaBase(c.Key("capture"), &a.captureText).Placeholder(placeholder).FontSize(15).LineHeight(1.6).
			Lines(3, 8).AutoFocus().FillWidth().Grow(1).Label("快速记录")
		in.HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyEnter && (ev.Mods == 0 || ev.Mods == ui.Cmd) {
				save()
				return true
			}
			if ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyEscape {
				a.plat.HideCapture()
				return true
			}
			return false
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if ghostButton(c, "收藏剪贴板", "把剪贴板里的文字、链接、图片或文件存到收集（"+keyLabel(a.clipKey())+"）").Clicked() {
				if _, err := a.clipClipboard("剪贴板"); err != nil {
					a.captureErr = err.Error()
				} else {
					a.captureErr = ""
					a.plat.HideCapture()
				}
			}
			if here {
				on := a.captureWhere == captureHere
				b := ui.Box(c.Key("capture-here")).Padding(4, 8).Radius(6).Cursor(ui.CursorPointer).Label("记到选中的主题下").
					Tooltip("记成导图里「" + hereTitle + "」下的子主题")
				switch {
				case on:
					b.Background(t.Accent.Alpha(0.14))
				case b.Hovered():
					b.Background(t.Text.Alpha(0.06))
				}
				if b.Clicked() {
					if on {
						a.captureWhere = captureAuto
					} else {
						a.captureWhere = captureHere
					}
				}
				b.Children(func() {
					col := t.TextMuted
					if on {
						col = t.Accent
					}
					ui.Text(c, "记到「"+hereTitle+"」下").FontSize(textSmall).TextColor(col).SingleLine().MaxWidth(160).Ellipsis("…").PassThrough()
				})
			}
			ui.Spacer(c)
			if a.captureErr != "" {
				ui.Text(c, a.captureErr).FontSize(textSmall).TextColor(t.Danger).Shrink(1).SingleLine()
			} else if strings.TrimSpace(a.captureText) != "" && a.captureWhere != captureHere {
				// Where it goes, said as it is written.
				if taskLike(a.captureText, a.now()) { // captureAuto: the window sets no other but captureHere
					pill(c, "任务 → 收件箱", t.Text.Alpha(0.06), t.Accent)
				} else {
					pill(c, "灵感", t.Text.Alpha(0.06), t.TextMuted)
				}
			}
			if accentButton(c, "记下", "记下并收起这个窗口").Clicked() {
				save()
			}
		})
	})
}

// moveToMap moves the nodes selected under a node of another map (or this
// one), as a step of each map: the other is saved at once.
func (a *app) moveToMap(path, sheet, id string) {
	nodes := a.topSelection()
	if len(nodes) == 0 {
		return
	}
	if path == a.path {
		// Under a topic of the canvas shown, or of another canvas of the
		// map: there the branch is taken off this one and put under it.
		// (Moved as on one canvas, nothing moved, and it said 已移动.)
		to := a.file.SheetOf(id)
		if to == nil {
			a.tell("没有移动：那个主题已经不在了")
			return
		}
		moved := 0
		a.mutate("", func() {
			for _, n := range nodes {
				if n.Contains(id) {
					continue // not under itself
				}
				if to == a.doc {
					if a.doc.Move(n.ID, id, -1) == nil {
						moved++
					}
					continue
				}
				target, _ := to.Find(id)
				if _, err := a.doc.Delete(n.ID); err != nil || target == nil {
					continue
				}
				target.Children = append(target.Children, n)
				target.Collapsed = false
				moved++
			}
		})
		if moved == 0 {
			a.tell("没有移动")
			return
		}
		if d := a.file.SheetOf(nodes[0].ID); d != nil && d.ID != a.doc.ID {
			a.showSheet(d.ID)
		}
		a.tell("已移动")
		return
	}
	// The nodes keep their IDs, as links to them and tasks waiting on
	// them follow them to the other map.
	m := &movedOut{from: a.path, to: path, under: id}
	for _, n := range nodes {
		h := movedHome{sheet: a.doc.ID}
		if _, p := a.doc.Find(n.ID); p != nil { // else a floating topic
			h.parent, h.index = p.ID, slices.Index(p.Children, n)
			if t := p.SummaryTopic(); t != nil && t != n {
				h.sum = t.ID
			}
		}
		m.nodes, m.homes = append(m.nodes, n.Clone()), append(m.homes, h)
	}
	var title string
	from := model.AssetsDir(a.path)
	err := a.editMapAt(path, "从「"+mapName(a.path)+"」移来", func(f *model.File) error {
		d := f.SheetOf(id)
		if d == nil {
			return errors.New("目标主题已不存在")
		}
		target, _ := d.Find(id)
		title = target.Title
		for _, n := range m.nodes {
			c := n.Clone()
			c.Pos = nil // of a floating topic, which is one of the tree there
			copyAssets(c, from, model.AssetsDir(path))
			target.Children = append(target.Children, c)
		}
		target.Collapsed = false
		return nil
	})
	if err != nil {
		a.saveErr = err.Error()
		return
	}
	m.there = true
	a.moveDiasTo(m.nodes, path) // what was drawn of them goes with them
	a.deleteSelection()
	a.followMove(m)
	a.tellUndo(fmt.Sprintf("已移到「%s」的「%s」下", mapName(path), title), func() { a.moveHome(m) })
}

// movedOut is a branch, or several, moved between the open map and
// another while the app runs. The two maps are two files, and the history
// takes back the steps of one: an undo that brought the branch back here
// left it there too, the same topics by the same IDs in both maps. So a
// move is followed (movesFollow), by its own step of the map's history
// (model.History.Step): that step undone, the branch is taken out of the
// other map; made again, it is put there again. (Forgetting the map's
// steps at a move, as the other map's are, would have cost every undo of
// what was done before it.)
//
// By its step, and not by whether the branch is here: followed so, any
// undo or redo while the branch was away for another reason, deleted
// here, or moved on to a third map, put it into the other map again. A
// move whose step is gone, too old or with another recorded after its
// undo, is followed no more.
type movedOut struct {
	from, to string        // the map whose steps are followed, and the other
	into     bool          // the move was from the other map into this one: a task put here, a branch brought back by 撤销
	under    string        // the topic of the other map it went under, or was under
	nodes    []*model.Node // the topics with their IDs: as moved, then as the other map had them when they last left it
	homes    []movedHome   // where each was here
	there    bool          // they are in the other map
	// The move's step, and the history it is of: the map's steps are
	// begun anew when its file changed while it was not open.
	hist *model.History
	step uint64
}

// movedHome is where a topic moved away was: on which canvas, under which
// topic (none: it floated), at which place among its children (-1: it was
// the topic of its summary), and that topic's summary when it had one.
type movedHome struct {
	sheet, parent, sum string
	index              int
}

// movedKept is how many moves are followed.
const movedKept = 32

// A map renamed or moved, as one is by itself when its central topic is
// retitled, is followed by the moves kept of it and by where topics were
// cut (sidebar.go's pathFollowers): left at the old path, ⌘Z and the
// toast's 撤销 found no map to take the branch out of, and it stayed in
// both.
func init() {
	pathFollowers = append(pathFollowers, func(a *app, follow func(string) string) {
		for _, m := range a.movedOut {
			m.from, m.to = follow(m.from), follow(m.to)
		}
		a.clipFrom = follow(a.clipFrom)
	})
}

// branch names what a move moved, between which maps: the moves of the
// same topics between the same two are of one branch, which is in one of
// them.
func (m *movedOut) branch() string {
	ids := []string{m.from, m.to}
	for _, n := range m.nodes {
		ids = append(ids, n.ID)
	}
	return strings.Join(ids, "\n")
}

// followMove keeps a move just made, by the open map's last step, to
// follow that map's undo and redo. The moves kept of the same branch say
// where it is as this one does.
func (a *app) followMove(m *movedOut) {
	m.hist, m.step = a.hist, a.hist.LastStep()
	for _, o := range a.movedOut {
		if o.branch() == m.branch() {
			o.there, o.nodes = m.there, m.nodes
		}
	}
	if a.movedOut = append(a.movedOut, m); len(a.movedOut) > movedKept {
		a.movedOut = a.movedOut[len(a.movedOut)-movedKept:]
	}
}

// movesFollow has the branches moved between the open map and others
// follow an undo or a redo of it: each where the map's steps, as they
// stand now, have it.
func (a *app) movesFollow() {
	if a.file == nil {
		return
	}
	// This map's moves whose step is still among its steps, in the order
	// they were made.
	var mine []*movedOut
	a.movedOut = slices.DeleteFunc(a.movedOut, func(m *movedOut) bool {
		if m.from != a.path {
			return false
		}
		if m.hist != a.hist || a.hist.Step(m.step) == 0 {
			return true
		}
		mine = append(mine, m)
		return false
	})
	// Where each branch is to be: as the last of its moves still made put
	// it or, with none made, as it was before the first of them.
	by := map[string]*movedOut{}
	var order []string
	for _, m := range mine {
		k := m.branch()
		made := a.hist.Step(m.step) > 0
		switch o := by[k]; {
		case o == nil:
			by[k], order = m, append(order, k)
		case made:
			by[k] = m
		}
	}
	for _, k := range order {
		m := by[k]
		made := a.hist.Step(m.step) > 0
		want := made != m.into // in the other map
		if want == m.there {
			continue
		}
		var said string
		if want {
			if !a.joinOther(m) {
				// The other map is gone, or does not read: the step is
				// taken back as it came, and the branch stays here, where
				// it was in neither.
				a.stepAgain(made)
				a.tell(fmt.Sprintf("没能放回「%s」（那张导图不在了或读不了），这一支留在这张导图里", mapName(m.to)))
				return
			}
			said = fmt.Sprintf("这一支又移到了「%s」", mapName(m.to))
		} else {
			switch a.leaveOther(m, true) {
			case leftAsMoved:
				said = fmt.Sprintf("这一支也从「%s」里拿回来了", mapName(m.to))
			case leftChanged:
				said = fmt.Sprintf("这一支也从「%s」里拿回来了，连同在那边改过的", mapName(m.to))
			}
		}
		for _, o := range mine {
			if o.branch() == k {
				o.there, o.nodes = m.there, m.nodes
			}
		}
		if said != "" {
			a.tell(said)
		}
	}
}

// stepAgain takes back the step just undone or made again, with nothing
// following it: undone when it was made, made again when it was undone.
func (a *app) stepAgain(made bool) {
	var s model.State
	var ok bool
	if made {
		s, ok = a.hist.Undo(a.state())
	} else {
		s, ok = a.hist.Redo(a.state())
	}
	if ok {
		a.restore(s)
	}
}

// What leaveOther found of a branch in the other map.
const (
	leftNothing = iota // it was not there: nothing done
	leftAsMoved        // taken out, the same as it was moved
	leftChanged        // taken out, changed there since: it is here as it was there
)

// leaveOther takes a branch out of the other map, as it comes back to
// this one. The branch is here as the other map had it: what was changed
// of it there since the move, a title, a topic added under it, comes back
// with it, where the branch as it was moved came back and the rest was in
// neither map. With here, the open map has the branch already, as an undo
// left it, and it is made so there; else what m keeps of it is, for the
// one who puts it back. One that is no longer there, deleted or taken
// elsewhere since, is this move's no more.
func (a *app) leaveOther(m *movedOut, here bool) int {
	f := a.mapFile(m.to)
	if f == nil {
		if _, err := os.Stat(m.to); errors.Is(err, os.ErrNotExist) {
			m.there = false // the map itself is gone
		}
		return leftNothing // else not read: not known to be gone
	}
	found, changed := false, false
	for i, n := range m.nodes {
		there := f.Node(n.ID)
		if there == nil {
			continue
		}
		found = true
		c := there.Clone()
		c.Pos = n.Pos // where it floated here, if it did
		if sameBranch(c, n) {
			continue
		}
		changed = true
		copyAssets(c, model.AssetsDir(m.to), model.AssetsDir(m.from))
		m.nodes = slices.Clone(m.nodes)
		m.nodes[i] = c
		if here && m.from == a.path {
			a.replaceTopic(c)
		}
	}
	if !found {
		m.there = false
		return leftNothing
	}
	err := a.editMapAt(m.to, "移回「"+mapName(m.from)+"」", func(f *model.File) error {
		for _, n := range m.nodes {
			if d := f.SheetOf(n.ID); d != nil {
				_, _ = d.Delete(n.ID)
			}
		}
		return nil
	})
	if err != nil {
		return leftNothing
	}
	m.there = false
	a.moveDiasTo(m.nodes, m.from)
	if changed {
		return leftChanged
	}
	return leftAsMoved
}

// sameBranch tells whether two branches are the same, topic by topic.
func sameBranch(x, y *model.Node) bool {
	bx, _ := json.Marshal(x)
	by, _ := json.Marshal(y)
	return string(bx) == string(by)
}

// replaceTopic puts a topic in the place of the open map's topic of its
// ID, with all under it: outside the map's steps, as part of the one just
// undone or made again.
func (a *app) replaceTopic(c *model.Node) {
	d := a.file.SheetOf(c.ID)
	if d == nil {
		return
	}
	n, p := d.Find(c.ID)
	switch {
	case p == nil:
		if i := slices.Index(d.Floating, n); i >= 0 {
			d.Floating[i] = c
		}
	case p.SummaryTopic() == n:
		p.Summary.Topic = c
	default:
		if i := slices.Index(p.Children, n); i >= 0 {
			p.Children[i] = c
		}
	}
	a.edits++
}

// joinOther puts a branch moved to another map, and taken out of it by an
// undo, there again: under the topic it went under, or the map's middle
// when that is gone. It reports whether it could: not into a map that is
// gone.
func (a *app) joinOther(m *movedOut) bool {
	err := a.editMapAt(m.to, "从「"+mapName(m.from)+"」移来", func(f *model.File) error {
		target := f.Node(m.under)
		if target == nil {
			target = f.Sheets[0].Root
		}
		for _, n := range m.nodes {
			if f.Node(n.ID) != nil {
				continue
			}
			c := n.Clone()
			c.Pos = nil
			copyAssets(c, model.AssetsDir(m.from), model.AssetsDir(m.to))
			target.Children = append(target.Children, c)
		}
		target.Collapsed = false
		return nil
	})
	if err != nil {
		return false
	}
	m.there = true
	a.moveDiasTo(m.nodes, m.to)
	return true
}

// moveHome is the 撤销 of a move to another map, by the topics' IDs: they
// are put back where they were here, a step of this map, and taken out of
// the other, as the other map has them now. The step is a move of its
// own, into this map, followed as the move out is: undone, the branch is
// in the other map again.
func (a *app) moveHome(m *movedOut) {
	if a.mapFile(m.from) == nil {
		return
	}
	a.leaveOther(m, false) // gone from there since, it is put back as it was moved
	open := m.from == a.path
	err := a.editMapAt(m.from, "从「"+mapName(m.to)+"」移回", func(f *model.File) error {
		for i, n := range m.nodes {
			if f.Node(n.ID) != nil {
				continue // back already
			}
			h, c := m.homes[i], n.Clone()
			d, _ := f.Sheet(h.sheet)
			if d == nil {
				d = f.Sheets[0]
			}
			p, _ := d.Find(h.parent)
			switch {
			case h.parent == "": // it floated
				d.Floating = append(d.Floating, c)
			case p == nil: // where it was is gone: under the middle
				d.Root.Children = append(d.Root.Children, c)
			case h.index < 0 && p.Summary == nil && len(p.Children) > 0: // it was the summary's topic
				p.Summary = &model.Mark{Topic: c}
			default:
				p.Children = slices.Insert(p.Children, min(max(h.index, 0), len(p.Children)), c)
				if h.sum != "" {
					p.Summarize(h.sum)
				}
			}
		}
		return nil
	})
	if err != nil {
		// Not put back here: it is not left in neither map.
		a.joinOther(m)
		a.tell("没有撤销：" + err.Error())
		return
	}
	if open && m.from == a.path {
		a.followMove(&movedOut{from: m.from, to: m.to, into: true, under: m.under, nodes: m.nodes, homes: m.homes})
	}
}

// trayEntry is a line of the menu bar icon's menu.
type trayEntry struct {
	label string
	run   func()
	sep   bool
}

// trayTasks are the tasks of every map due today or overdue, not done.
func (a *app) trayTasks() []trayTask {
	day := today(a.now())
	var out []trayTask
	for _, f := range a.files {
		file := a.file
		if f.Path != a.path {
			file = a.indexed(f.Path)
		}
		if file == nil {
			continue
		}
		for _, tn := range file.Tasks() {
			task := tn.Node.Task
			if task.Status == model.StatusDone || task.Due == nil || !task.Due.Before(day.AddDate(0, 0, 1)) {
				continue
			}
			out = append(out, trayTask{path: f.Path, sheet: tn.Sheet.ID, node: tn.Node, overdue: task.Due.Before(day)})
		}
	}
	return out
}

type trayTask struct {
	path, sheet string
	node        *model.Node
	overdue     bool
}

// trayMenu is the menu of the menu bar icon.
func (a *app) trayMenu() []trayEntry {
	// Three things to do from anywhere, then today's tasks, then the app:
	// nothing that only goes to a page of it, which opening it does.
	items := []trayEntry{{label: "记一条…", run: a.plat.ShowCapture},
		{label: "截图", run: func() { a.plat.Screenshot(false) }},
		{label: "收藏剪贴板", run: a.clipFromShortcut}}
	tasks := a.trayTasks()
	slices.SortStableFunc(tasks, func(x, y trayTask) int { return b2i(y.overdue) - b2i(x.overdue) }) // those late first
	items = append(items, trayEntry{sep: true})
	if len(tasks) == 0 {
		items = append(items, trayEntry{label: "今天没有要做的"})
	} else {
		items = append(items, trayEntry{label: fmt.Sprintf("今天要做 %d 件", len(tasks))})
		for i, tt := range tasks {
			if i == 8 {
				items = append(items, trayEntry{label: fmt.Sprintf("还有 %d 件，在 MyMind 里看", len(tasks)-i), run: a.plat.ShowMain})
				break
			}
			// Its words, without the link written in them; late, it says so.
			label, _ := titleAndSite(tt.node.Title)
			if r := []rune(label); len(r) > 28 {
				label = string(r[:28]) + "…"
			}
			if tt.overdue {
				label = "逾期 · " + label
			}
			tt := tt
			items = append(items, trayEntry{label: label, run: func() {
				a.plat.ShowMain()
				a.goToNode(tt.path, tt.sheet, tt.node.ID)
			}})
		}
	}
	items = append(items, trayEntry{sep: true}, trayEntry{label: "打开 MyMind", run: a.plat.ShowMain})
	return items
}

// trayTitle is shown beside the icon: how many tasks are due, if any.
func (a *app) trayTitle() string {
	if n := len(a.trayTasks()); n > 0 {
		return fmt.Sprint(n)
	}
	return ""
}

// openSettings opens the settings, with the fields as saved.
func (a *app) openSettings() {
	a.settingsOpen = true
	a.embedDrafts = [3]string{a.cfg.EmbedURL, a.cfg.EmbedKey, a.cfg.EmbedModel}
	a.syncDrafts = [2]string{a.cfg.SyncRepo, a.cfg.SyncToken}
}

// settingsView is the window of preferences: the capture shortcut, the
// menu bar icon, reminders and the editor of code links.
func (a *app) settingsView(c *ui.Context) {
	t := c.Theme()
	// On the ground, as a page: its parts are white cards.
	modal(c, "settings", &a.settingsOpen, func() {
		w, h := c.Size()
		tabs := slices.Clone(settingsTabs)
		if a.file != nil {
			tabs = append(tabs, "当前导图")
		}
		a.settingsTab = min(a.settingsTab, len(tabs)-1)
		// The parts on the left, as a list; the one chosen on the right.
		ui.Row(c).Gap(22).Width(min(820, w-80)).AlignItems(ui.Start).Children(func() {
			ui.Column(c.Key("settings-tabs")).Width(160).Shrink(0).Gap(2).Children(func() {
				ui.Text(c, "设置").FontSize(26).FontWeight(700).LetterSpacing(-0.7).Margin(0, 12, 14)
				for i, name := range tabs {
					on := i == a.settingsTab
					row := ui.Row(c.Key("settings-tab:"+name)).Height(36).Padding(0, 10).Gap(10).Radius(radiusRow).AlignItems(ui.Center).
						FillWidth().Cursor(ui.CursorPointer).Label(name).Background(t.Text.Alpha(0)).Transition(easeLook)
					hot := on || row.Hovered()
					if !on && hot {
						row.Background(t.Text.Alpha(0.05))
					}
					if row.Clicked() {
						a.settingsTab = i
					}
					press(row, 0.975)
					row.Children(func() {
						if on {
							slidePill(c, "settings-tabs", radiusRow)
							row.ZIndex(-1) // under the others, which the pill slides behind
						}
						// Its icon, as a row of the left column has.
						kubaiIcon(c, map[string]string{"通用": "sliders", "资料整理": "sparkles", "备份": "save", "同步": "sync", "加密": "key-round", "连接": "plug", "当前导图": "sitemap"}[name], sideIconSize, hot)
						txt := ui.Text(c, name).FontSize(textBody + 0.5).PassThrough()
						if on {
							txt.FontWeight(700)
						} else if !hot {
							txt.TextColor(ink2(t))
						}
					})
				}
			})
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(10).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Children(func() {
					ui.Text(c, tabs[a.settingsTab]).FontSize(textTitle).FontWeight(700).Grow(1)
					if iconButton(c, iconClose, "关闭", "关闭", 28, true).Clicked() {
						a.settingsOpen = false
					}
				})
				// The same height for every tab, so that the window does not
				// jump when switching; a long tab scrolls.
				ui.Scroll(c.Key(fmt.Sprint("settings", a.settingsTab))).Gap(20).Height(max(min(580, h-190), 200)).Padding(2, 12, 24, 2).Margin(0, -2).Label("设置内容").Children(func() {
					switch tabs[a.settingsTab] {
					case "通用":
						a.generalSettings(c)
					case "资料整理":
						rowsField(c, "AI 整理", "用的是 AI 助手的 API Key，按用量计费。", func() {
							auto := !a.cfg.LibraryManual
							switchRow(c, &auto, "自动整理资料", "新收藏的资料由 AI 起标题、写摘要、分类、打标签，并生成同义词", func() {
								a.cfg.LibraryManual = !auto
								a.cfg.save()
							})
						})
						a.embedSettings(c)
					case "备份":
						a.backupSettings(c)
					case "同步":
						a.syncSettings(c)
					case "加密":
						a.vaultSettings(c)
					case "连接":
						rowsField(c, "AI", "", func() {
							setRow(c, "AI 助手", "服务商、API Key 和模型。当前："+providerModelName(a.cfg), func() {
								if button(c, "打开 AI 设置").Clicked() {
									a.settingsOpen, a.aiOpen, a.ai.settings = false, true, true
									a.loadKeyDrafts()
								}
							})
							setLine(c)
							setRow(c, "AI 编程工具（MCP）", "让 Claude Code、Cursor 等读写导图、检索收集的资料", func() {
								if button(c, "连接说明…").Clicked() {
									a.settingsOpen, a.mcpOpen = false, true
								}
							})
						})
					default:
						a.projectSection(c)
					}
				})
			})
		})
	})
}

// switchRow is a switch with what it does beside it.
func switchRow(c *ui.Context, on *bool, label, text string, changed func()) {
	t := c.Theme()
	// The whole row sets it: a click anywhere on it.
	row := ui.Row(c.Key("row:"+label)).MinHeight(52).Padding(10, 0).Gap(16).AlignItems(ui.Center).FillWidth().
		Cursor(ui.CursorPointer).Label(label)
	if row.Clicked() {
		*on = !*on
		changed()
	}
	row.Children(func() {
		ui.Column(c).Gap(2).Grow(1).Shrink(1).Basis(0).MinWidth(0).PassThrough().Children(func() {
			ui.Text(c, label).FontSize(textBody)
			if text != "" {
				ui.Text(c, text).FontSize(textSmall).TextColor(t.TextMuted).LineHeight(1.45).Wrap()
			}
		})
		ui.Switch(c, on).Shrink(0).PassThrough()
	})
}

// generalSettings are the shortcuts, the menu bar icon and the reminders.
func (a *app) generalSettings(c *ui.Context) {
	a.groundSettings(c) // how it looks, first: it is what is looked for here
	rowsField(c, "全局快捷键", "在任何应用里都能用。点一个快捷键，再按下新的组合键。", func() {
		rows := []struct {
			name, title, what, value, def string
			save                          func(string)
			reg                           func(string) error
			effective                     func() string
		}{
			{"快速记录快捷键", "快速记录", "弹出小窗口记下想法", a.cfg.CaptureKey, defaultCaptureKey,
				func(v string) { a.cfg.CaptureKey = v }, a.plat.SetCaptureKey, a.captureKey},
			{"收藏剪贴板快捷键", "收藏剪贴板", "复制后按下，直接存到收集", a.cfg.ClipKey, defaultClipKey,
				func(v string) { a.cfg.ClipKey = v }, a.plat.SetClipKey, a.clipKey},
			{"截图快捷键", "截图", "框选屏幕、标注，存到收集或复制", a.cfg.ShotKey, defaultShotKey,
				func(v string) { a.cfg.ShotKey = v }, a.plat.SetShotKey, a.shotKey},
		}
		for i, r := range rows {
			if i > 0 {
				setLine(c)
			}
			setRow(c, r.title, r.what, func() {
				a.shortcutField(c, r.name, r.value, r.def, func(acc string) error {
					r.save(acc)
					a.cfg.save()
					return r.reg(r.effective())
				}, func() { _ = r.reg("") })
			})
		}
	})
	rowsField(c, "日历", "", func() {
		setRow(c, "每周从哪天开始", "日程的周视图、月视图和本周回顾", func() {
			chosen := 0
			if a.cfg.WeekFromSunday {
				chosen = 1
			}
			if n := tabGroup(c, "week-from", []string{"周一", "周日"}, chosen); n != chosen {
				a.cfg.WeekFromSunday = n == 1
				a.cfg.save()
			}
		})
		setLine(c)
		on := !a.cfg.NoHolidays
		switchRow(c, &on, "节假日和调休", "日期旁标出放假（休）和调休上班（班）的日子，重复里可选「法定工作日」。按国务院办公厅每年的放假通知，由开源项目 holiday-cn 整理，每天取一次；已有 "+holidayYearsSaid(), func() {
			a.cfg.NoHolidays = !on
			a.cfg.save()
			if on {
				a.holidaysAsked = nil
				a.holidaysFetch()
			} else {
				clearHolidays()
			}
		})
	})
	rowsField(c, "菜单栏和提醒", "", func() {
		tray := !a.cfg.NoTray
		switchRow(c, &tray, "菜单栏图标", "显示今天到期和已逾期的任务数，点开列出任务、快速记录、收藏剪贴板", func() {
			a.cfg.NoTray = !tray
			a.cfg.save()
			a.plat.SetTray(tray)
		})
		setLine(c)
		on := !a.cfg.NoReminders
		switchRow(c, &on, "到期提醒", "任务今天到期或已逾期时，每天发一次系统通知", func() {
			a.cfg.NoReminders = !on
			a.cfg.save()
		})
		setLine(c)
		setRow(c, "新手引导", "再看一遍怎么用", func() {
			if button(c, "重新查看").Label("重新查看新手引导").Clicked() {
				a.settingsOpen, a.tourStep = false, 1
			}
		})
	})
	a.shotSettings(c)
	a.updateSettings(c)
	a.iconSettings(c)
}
