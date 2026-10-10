package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
}

// Views of the main area.
const (
	viewMap = iota
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
	files []model.FileInfo

	// The open map.
	path     string
	doc      *model.Document
	modTime  time.Time // of the file as last read or written
	dirty    bool
	changed  time.Time // of the last unsaved change
	checked  time.Time // when the file was last checked for outside changes
	saveErr  string
	hist     *model.History
	lastEdit string // the key of the last coalesced edit (see mutate)

	// Selection and in-place editing.
	selected  string
	editing   string // the node whose title is edited on the canvas
	editText  string
	editFocus bool // the title input has had the focus
	reveal    bool // scroll the selected node into view in the next frame

	// The canvas.
	zoom   float32
	scroll ui.ScrollState
	canvas ui.Handle
	boxes  layout.Result // the layout of the last frame, in map coordinates

	// Chrome.
	mode          int
	search        string
	tagFilter     string
	sidebarWidth  float32
	panelWidth    float32
	filesOpen     bool
	tagsOpen      bool
	confirmDelete bool
	deletePath    string
}

func newApp(plat platform, cfg config) *app {
	a := &app{
		plat:         plat,
		cfg:          cfg,
		now:          time.Now,
		hist:         model.NewHistory(200),
		zoom:         1,
		sidebarWidth: 220,
		panelWidth:   300,
		filesOpen:    true,
		tagsOpen:     true,
	}
	a.openDir(cfg.Dir)
	if cfg.File != "" && filepath.Dir(cfg.File) == a.dir {
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
		if err := welcomeDoc(a.now()).Save(path); err == nil {
			a.refreshFiles()
		}
	}
	a.cfg.Dir = dir
}

func (a *app) refreshFiles() {
	files, err := model.ListFiles(a.dir)
	if err != nil {
		a.files = nil
		return
	}
	a.files = files
}

// open shows the map in path, saving the one open first.
func (a *app) open(path string) error {
	a.flush()
	d, err := model.Load(path)
	if err != nil {
		a.saveErr = fmt.Sprintf("无法打开 %s: %v", filepath.Base(path), err)
		return err
	}
	a.path, a.doc = path, d
	a.modTime = modTime(path)
	a.dirty, a.saveErr, a.lastEdit = false, "", ""
	a.hist.Clear()
	a.selected, a.editing = d.Root.ID, ""
	a.scroll = ui.ScrollState{}
	a.cfg.File = path
	a.cfg.save()
	return nil
}

// newMap creates a map in the folder and opens it, editing its root.
func (a *app) newMap() {
	name := "新导图"
	for i := 2; ; i++ {
		if _, err := os.Stat(filepath.Join(a.dir, model.FileName(name))); errors.Is(err, os.ErrNotExist) {
			break
		}
		name = fmt.Sprintf("新导图 %d", i)
	}
	path := filepath.Join(a.dir, model.FileName(name))
	if err := model.New(name).Save(path); err != nil {
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
		a.path, a.doc = "", nil
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
	if err := a.doc.Save(a.path); err != nil {
		a.saveErr = "保存失败: " + err.Error()
		return
	}
	a.dirty, a.saveErr = false, ""
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
	d, err := model.Load(a.path)
	if err != nil {
		a.saveErr = err.Error()
		return
	}
	a.hist.Record(a.doc, a.selected)
	a.doc = d
	a.modTime = modTime(a.path)
	if n, _ := d.Find(a.selected); n == nil {
		a.selected = d.Root.ID
	}
	a.editing = ""
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
		a.hist.Record(a.doc, a.selected)
	}
	a.lastEdit = key
	fn()
	a.dirty = true
	a.changed = a.now()
}

func (a *app) undo() {
	if d, sel, ok := a.hist.Undo(a.doc, a.selected); ok {
		a.doc, a.selected, a.editing, a.lastEdit = d, sel, "", ""
		a.dirty, a.changed = true, a.now()
	}
}

func (a *app) redo() {
	if d, sel, ok := a.hist.Redo(a.doc, a.selected); ok {
		a.doc, a.selected, a.editing, a.lastEdit = d, sel, "", ""
		a.dirty, a.changed = true, a.now()
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
	walk = func(n *model.Node, depth int) {
		if depth == depthOf {
			row = append(row, n.ID)
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

func (a *app) setZoom(z float32) {
	a.zoom = min(max(z, 0.4), 2.5)
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
