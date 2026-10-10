package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/layout"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// platform is what the app asks of the desktop. main.go implements it with
// MyGo's dialogs; tests use a fake.
type platform interface {
	// PickFolder asks for a folder and calls done with it, "" when
	// canceled, on the main thread.
	PickFolder(done func(dir string))
	ShowInFolder(path string)
	Trash(path string) error
	SetFullScreen(on bool)
}

// Views of the main area.
const (
	viewMap = iota
	viewOutline
	viewBoard
	viewAgenda
)

// autosaveDelay is how long after the last change the map is saved.
const autosaveDelay = 800 * time.Millisecond

// app is the state of the window.
type app struct {
	plat platform
	cfg  config
	now  func() time.Time

	// The folder and its maps.
	dir   string
	tree  model.Folder     // the folder and the folders in it
	files []model.FileInfo // every map of the tree

	// The open map: a file of canvases, one of them shown.
	path     string
	file     *model.File
	doc      *model.Document // the canvas shown, one of file.Sheets
	modTime  time.Time       // of the file as last read or written
	dirty    bool
	changed  time.Time // of the last unsaved change
	checked  time.Time // when the file was last checked for outside changes
	saveErr  string
	savedAt  time.Time // when this run last saved the map
	hist     *model.History
	lastEdit string // the key of the last coalesced edit (see mutate)

	// Selection and in-place editing.
	selected  string
	editing   string // the node whose title is edited on the canvas
	editText  string
	editFocus bool // the title input has had the focus
	reveal    bool // scroll the selected node into view in the next frame

	// The canvas: where each canvas of the file is looked at from.
	cams    map[string]*camera
	canvas  ui.Handle
	boxes   layout.Result // the layout of the last frame, in map coordinates
	vp      ui.Rect       // the canvas's box in the last frame
	pointer [2]float32    // where the pointer is over the canvas
	mods    ui.Modifiers  // the modifier keys held, as of the last frame

	// Actions that change the map, taken while it is built, wait for the
	// end of the frame (see later).
	pending []func()

	// Canvas tabs.
	renamingSheet string
	sheetTitle    string
	confirmSheet  bool
	structureOpen bool
	panelTab      int
	outlineFocus  string // the node whose line the outline focuses

	// The presentation.
	presenting bool
	slides     []slide
	slideIndex int
	slideStep  int // items of the slide shown

	// Chrome.
	mode          int
	search        string
	tagFilter     string
	sidebarWidth  float32
	panelWidth    float32
	filesOpen     bool
	tagsOpen      bool
	favoritesOpen bool
	folderClosed  map[string]bool // folders of the sidebar shown closed
	renaming      string          // the map or folder renamed in the sidebar
	renameText    string
	tagQuery      string
	confirmDelete bool
	deletePath    string
}

func newApp(plat platform, cfg config) *app {
	a := &app{
		plat:          plat,
		cfg:           cfg,
		now:           time.Now,
		hist:          model.NewHistory(200),
		cams:          map[string]*camera{},
		sidebarWidth:  220,
		panelWidth:    300,
		filesOpen:     true,
		tagsOpen:      true,
		favoritesOpen: true,
		folderClosed:  map[string]bool{},
	}
	a.openDir(cfg.Dir)
	if cfg.File != "" && inside(cfg.File, a.dir) {
		if err := a.open(cfg.File); err == nil {
			return a
		}
	}
	if len(a.files) > 0 {
		_ = a.open(a.files[0].Path)
	}
	return a
}

// openDir shows the maps of dir, creating it, with a guide map, when it
// has none.
func (a *app) openDir(dir string) {
	a.dir = dir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		a.saveErr = err.Error()
	}
	a.refreshFiles()
	if len(a.files) == 0 {
		path := filepath.Join(dir, model.FileName("使用指南"))
		if err := welcomeFile(a.now()).Save(path); err == nil {
			a.refreshFiles()
		}
	}
	a.cfg.Dir = dir
}

func (a *app) refreshFiles() {
	tree, err := model.ScanFolder(a.dir)
	if err != nil {
		a.tree, a.files = model.Folder{Path: a.dir, Name: filepath.Base(a.dir)}, nil
		return
	}
	a.tree, a.files = tree, tree.AllFiles()
}

// inside reports whether path is dir or in it, at any depth.
func inside(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// open shows the map in path, saving the one open first.
func (a *app) open(path string) error {
	a.flush()
	f, err := model.Load(path)
	if err != nil {
		a.saveErr = fmt.Sprintf("无法打开 %s: %v", filepath.Base(path), err)
		return err
	}
	a.path, a.file = path, f
	a.modTime = modTime(path)
	a.dirty, a.saveErr, a.lastEdit = false, "", ""
	a.savedAt = time.Time{}
	a.hist.Clear()
	a.cams = map[string]*camera{}
	a.doc = nil
	a.showSheet(f.Sheets[0].ID)
	a.cfg.File = path
	a.cfg.save()
	return nil
}

// newMap creates a map in the folder of the open map and opens it, editing
// its root.
func (a *app) newMap() { a.newMapIn(a.currentFolder()) }

// currentFolder is the folder of the open map, else the workspace.
func (a *app) currentFolder() string {
	if a.path != "" && inside(a.path, a.dir) {
		return filepath.Dir(a.path)
	}
	return a.dir
}

// newMapIn creates a map in dir and opens it, editing its root.
func (a *app) newMapIn(dir string) {
	name := "新导图"
	for i := 2; ; i++ {
		if _, err := os.Stat(filepath.Join(dir, model.FileName(name))); errors.Is(err, os.ErrNotExist) {
			break
		}
		name = fmt.Sprintf("新导图 %d", i)
	}
	path := filepath.Join(dir, model.FileName(name))
	if err := model.NewFile(name).Save(path); err != nil {
		a.saveErr = err.Error()
		return
	}
	a.refreshFiles()
	if a.open(path) == nil {
		a.startEdit(a.doc.Root.ID)
	}
}

// deleteMap moves a map file to the trash.
func (a *app) deleteMap(path string) {
	if path == a.path {
		a.dirty = false // nothing to save of a map being deleted
	}
	if err := a.plat.Trash(path); err != nil {
		a.saveErr = err.Error()
		return
	}
	a.refreshFiles()
	if path == a.path {
		a.path, a.file, a.doc = "", nil, nil
		if len(a.files) > 0 {
			_ = a.open(a.files[0].Path)
		}
	}
}

// save writes the open map now.
func (a *app) save() {
	if a.doc == nil || a.path == "" {
		return
	}
	if err := a.file.Save(a.path); err != nil {
		a.saveErr = "保存失败: " + err.Error()
		return
	}
	a.dirty, a.saveErr = false, ""
	a.savedAt = a.now()
	a.modTime = modTime(a.path)
	a.refreshFiles()
}

// flush saves unsaved changes.
func (a *app) flush() {
	if a.dirty {
		a.save()
	}
}

// tick saves after a pause in editing and reloads the file when another
// program, as an AI agent, changed it. It runs every frame.
func (a *app) tick(c *ui.Context) {
	now := a.now()
	if a.dirty {
		if wait := autosaveDelay - now.Sub(a.changed); wait > 0 {
			c.After(wait)
		} else {
			a.save()
		}
	}
	if a.path != "" && now.Sub(a.checked) >= 2*time.Second {
		a.checked = now
		if mt := modTime(a.path); !mt.IsZero() && !mt.Equal(a.modTime) {
			a.reloadFromDisk(c)
		}
	}
	c.After(2 * time.Second)
}

// reloadFromDisk takes the file as changed outside the app.
func (a *app) reloadFromDisk(c *ui.Context) {
	if a.dirty {
		a.modTime = modTime(a.path) // ask once
		c.ToastAction("文件已被其他程序修改", "载入磁盘版本", func() {
			a.dirty = false
			a.reload()
		})
		return
	}
	a.reload()
	c.Toast("已载入外部修改")
}

func (a *app) reload() {
	f, err := model.Load(a.path)
	if err != nil {
		a.saveErr = err.Error()
		return
	}
	a.hist.Record(a.state())
	a.restore(model.State{File: f, Sheet: a.doc.ID, Selected: a.selected})
	a.modTime = modTime(a.path)
}

func modTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// mutate changes the map, recording the state before for undo. Edits with
// the same non-empty key one after another, as the keys typed into one
// field, make one step.
func (a *app) mutate(key string, fn func()) {
	if a.doc == nil {
		return
	}
	if key == "" || key != a.lastEdit {
		a.hist.Record(a.state())
	}
	a.lastEdit = key
	fn()
	a.dirty = true
	a.changed = a.now()
}

func (a *app) undo() {
	if s, ok := a.hist.Undo(a.state()); ok {
		a.restore(s)
		a.dirty, a.changed = true, a.now()
	}
}

func (a *app) redo() {
	if s, ok := a.hist.Redo(a.state()); ok {
		a.restore(s)
		a.dirty, a.changed = true, a.now()
	}
}

func (a *app) state() model.State {
	return model.State{File: a.file, Sheet: a.doc.ID, Selected: a.selected}
}

// restore shows a state of the history, or a file read again.
func (a *app) restore(s model.State) {
	a.file, a.doc = s.File, nil
	a.editing, a.lastEdit = "", ""
	if d, _ := a.file.Sheet(s.Sheet); d == nil {
		s.Sheet = a.file.Sheets[0].ID
	}
	a.showSheet(s.Sheet)
	if n, _ := a.doc.Find(s.Selected); n != nil {
		a.selected = s.Selected
	}
}

// selectedNode returns the selected node and its parent.
func (a *app) selectedNode() (n, parent *model.Node) {
	if a.doc == nil {
		return nil, nil
	}
	return a.doc.Find(a.selected)
}

// selectNode selects a node, opening the nodes above it, and scrolls it
// into view.
func (a *app) selectNode(id string) {
	if a.doc == nil {
		return
	}
	path := a.doc.Path(id)
	if path == nil {
		return
	}
	for _, n := range path[:len(path)-1] {
		n.Collapsed = false
	}
	a.selected, a.reveal = id, true
}

func (a *app) startEdit(id string) {
	n, _ := a.doc.Find(id)
	if n == nil {
		return
	}
	a.selected, a.editing, a.editText, a.editFocus = id, id, n.Title, false
	a.reveal = true
}

// commitEdit keeps the title typed on the canvas.
func (a *app) commitEdit() {
	id := a.editing
	a.editing = ""
	n, _ := a.doc.Find(id)
	if n == nil {
		return
	}
	title := a.editText
	if title == "" {
		title = "新主题"
	}
	if title != n.Title {
		a.mutate("", func() { n.Title = title })
	}
	a.canvas.Focus()
}

func (a *app) cancelEdit() {
	a.editing = ""
	a.canvas.Focus()
}

func (a *app) addChild() {
	if a.doc == nil {
		return
	}
	var n *model.Node
	a.mutate("", func() { n, _ = a.doc.AddChild(a.selected, "") })
	if n != nil {
		a.startEdit(n.ID)
	}
}

func (a *app) addSibling() {
	if a.doc == nil {
		return
	}
	var n *model.Node
	a.mutate("", func() { n, _ = a.doc.AddSibling(a.selected, "") })
	if n != nil {
		a.startEdit(n.ID)
	}
}

func (a *app) deleteSelected() {
	if n, p := a.selectedNode(); n == nil || p == nil {
		return
	}
	a.mutate("", func() {
		if next, err := a.doc.Delete(a.selected); err == nil {
			a.selected = next
		}
	})
	a.reveal = true
}

func (a *app) toggleCollapse(id string) {
	n, _ := a.doc.Find(id)
	if n == nil || len(n.Children) == 0 {
		return
	}
	a.mutate("", func() { n.Collapsed = !n.Collapsed })
}

func (a *app) toggleTask(n *model.Node) {
	a.mutate("", func() {
		if n.Task == nil {
			n.Task = &model.Task{Status: model.StatusTodo}
		} else {
			n.Task = nil
		}
	})
}

func (a *app) setStatus(n *model.Node, s model.Status) {
	a.mutate("", func() {
		if n.Task == nil {
			n.Task = &model.Task{}
		}
		n.Task.Status = s
	})
}

// moveNode puts a dragged node under another, as its last child.
func (a *app) moveNode(id, parent string) {
	if id == parent || a.doc == nil {
		return
	}
	n, _ := a.doc.Find(id)
	if n == nil || n.Contains(parent) {
		return
	}
	a.mutate("", func() { _ = a.doc.Move(id, parent, -1) })
	a.selectNode(id)
}

// Keyboard navigation on the canvas.

func (a *app) navParent() {
	if _, p := a.selectedNode(); p != nil {
		a.selectNode(p.ID)
	}
}

func (a *app) navChild() {
	n, _ := a.selectedNode()
	if n == nil || len(n.Children) == 0 {
		return
	}
	if n.Collapsed {
		a.mutate("", func() { n.Collapsed = false })
	}
	a.selectNode(n.Children[0].ID)
}

// navSide handles Left (left true) and Right: toward the root goes to the
// parent, away from it to the first child. From the root, it goes to the
// first branch on that side.
func (a *app) navSide(left bool) {
	n, p := a.selectedNode()
	if n == nil {
		return
	}
	if p == nil {
		for _, c := range n.Children {
			if a.boxes.Left[c.ID] == left {
				a.selectNode(c.ID)
				return
			}
		}
		if n.Collapsed && !left {
			a.navChild()
		}
		return
	}
	if a.boxes.Left[n.ID] == left {
		a.navChild()
	} else {
		a.navParent()
	}
}

// navSibling moves the selection by delta among the nodes shown at the
// same depth, from top to bottom, so it goes on into cousins.
func (a *app) navSibling(delta int) {
	if a.doc == nil {
		return
	}
	var depthOf int
	var row []string
	a.doc.Walk(func(n, _ *model.Node, depth int) bool {
		if n.ID == a.selected {
			depthOf = depth
		}
		return true
	})
	var walk func(n *model.Node, depth int)
	left := a.boxes.Left[a.selected]
	walk = func(n *model.Node, depth int) {
		if depth == depthOf {
			if a.boxes.Left[n.ID] == left { // stay on the selected node's side
				row = append(row, n.ID)
			}
			return
		}
		if n.Collapsed {
			return
		}
		for _, c := range n.Children {
			walk(c, depth+1)
		}
	}
	walk(a.doc.Root, 0)
	i := slices.Index(row, a.selected)
	if j := i + delta; i >= 0 && j >= 0 && j < len(row) {
		a.selectNode(row[j])
	}
}

func (a *app) shiftSelected(delta int) {
	if _, p := a.selectedNode(); p == nil {
		return
	}
	a.mutate("", func() { _ = a.doc.Shift(a.selected, delta) })
	a.reveal = true
}

// later runs fn once the view is built, for actions taken while building
// a collection that they change.
func (a *app) later(fn func()) { a.pending = append(a.pending, fn) }

func (a *app) runPending() {
	for len(a.pending) > 0 {
		fns := a.pending
		a.pending = nil
		for _, fn := range fns {
			fn()
		}
	}
}

// camera is where a canvas is looked at from: the screen position of the
// map's origin and the zoom, and the node selected on it.
type camera struct {
	X, Y     float32
	Zoom     float32
	placed   bool // X and Y were set for the canvas's size
	selected string
}

// cam returns the camera of the canvas shown.
func (a *app) cam() *camera {
	if a.doc == nil {
		return &camera{Zoom: 1}
	}
	c := a.cams[a.doc.ID]
	if c == nil {
		c = &camera{Zoom: 1}
		a.cams[a.doc.ID] = c
	}
	return c
}

// setZoom zooms around the point (x, y) of the canvas, its center when
// negative.
func (a *app) setZoomAt(z, x, y float32) {
	c := a.cam()
	z = min(max(z, 0.3), 3)
	if x < 0 {
		x, y = a.vp.W/2, a.vp.H/2
	}
	c.X = x - (x-c.X)*z/c.Zoom
	c.Y = y - (y-c.Y)*z/c.Zoom
	c.Zoom = z
}

func (a *app) setZoom(z float32) { a.setZoomAt(z, -1, -1) }

// recenter puts the map back at the left middle of the canvas, at zoom 1.
func (a *app) recenter() {
	c := a.cam()
	c.Zoom, c.placed = 1, false
	a.reveal = false
}

// showSheet shows a canvas of the file, keeping the selection of the one
// shown before.
func (a *app) showSheet(id string) {
	d, _ := a.file.Sheet(id)
	if d == nil {
		return
	}
	if a.doc != nil {
		a.cam().selected = a.selected
	}
	if a.editing != "" && a.doc != nil && a.doc.ID != id {
		a.commitEdit()
	}
	a.doc = d
	a.selected = a.cam().selected
	if n, _ := d.Find(a.selected); n == nil {
		a.selected = d.Root.ID
	}
}

func (a *app) addSheet() {
	_, i := a.file.Sheet(a.doc.ID)
	var d *model.Document
	a.mutate("", func() { d = a.file.AddSheet(i) })
	a.showSheet(d.ID)
	a.startEdit(d.Root.ID)
}

func (a *app) duplicateSheet(id string) {
	_, i := a.file.Sheet(id)
	if i < 0 {
		return
	}
	var d *model.Document
	a.mutate("", func() { d = a.file.DuplicateSheet(i) })
	a.showSheet(d.ID)
}

func (a *app) deleteSheet(id string) {
	_, i := a.file.Sheet(id)
	if i < 0 || len(a.file.Sheets) <= 1 {
		return
	}
	a.mutate("", func() { _ = a.file.DeleteSheet(i) })
	delete(a.cams, id)
	if a.doc.ID == id {
		a.doc = nil
		a.showSheet(a.file.Sheets[max(i-1, 0)].ID)
	}
}

// setStructure lays the canvas shown out another way, placing it anew.
func (a *app) setStructure(st model.Structure) {
	if a.doc == nil || a.doc.Structure == st {
		return
	}
	d := a.doc
	a.mutate("", func() { d.Structure = st })
	c := a.cam()
	c.placed = false
}

func (a *app) moveSheet(id string, delta int) {
	_, i := a.file.Sheet(id)
	if i < 0 {
		return
	}
	a.mutate("", func() { a.file.MoveSheet(i, i+delta) })
}

func (a *app) renameSheet(id, title string) {
	d, _ := a.file.Sheet(id)
	title = strings.TrimSpace(title)
	if d == nil || title == "" || title == d.Title {
		return
	}
	a.mutate("", func() { d.Title = title })
}

// selectTask shows a task's canvas and selects it, from the board or the
// agenda.
func (a *app) selectTask(tn model.TaskNode) {
	if tn.Sheet != nil && tn.Sheet != a.doc {
		a.showSheet(tn.Sheet.ID)
	}
	a.selected = tn.Node.ID
}

// highlighted reports whether a node matches the search and tag filter;
// those that do not are dimmed.
func (a *app) highlighted(n *model.Node) bool {
	if a.tagFilter != "" && !n.HasTag(a.tagFilter) {
		return false
	}
	return n.Matches(a.search)
}

func (a *app) filtering() bool {
	return a.tagFilter != "" || a.search != ""
}

// nextMatch selects the next node, after the selected one, that matches the
// search and tag filter.
func (a *app) nextMatch() {
	if a.doc == nil || !a.filtering() {
		return
	}
	var ids []string
	a.doc.Walk(func(n, _ *model.Node, _ int) bool {
		if a.highlighted(n) {
			ids = append(ids, n.ID)
		}
		return true
	})
	if len(ids) == 0 {
		return
	}
	i := slices.Index(ids, a.selected)
	a.selectNode(ids[(i+1)%len(ids)])
}
