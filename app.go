package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/ghsync"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/layout"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/versions"
)

// platform is what the app asks of the desktop. main.go implements it with
// MyGo's dialogs; tests use a fake.
type platform interface {
	// PickFolder asks for a folder and calls done with it, "" when
	// canceled, on the main thread.
	PickFolder(done func(dir string))
	// SaveFile asks where to save a file named name, of the extension,
	// and calls done with the path, "" when canceled, on the main thread.
	SaveFile(name, ext string, done func(path string))
	// OpenFile asks for a file of one of the extensions and calls done with
	// it, "" when canceled, on the main thread.
	OpenFile(title string, exts []string, done func(path string))
	// CopyText puts text on the clipboard, and ClipboardText reads it.
	CopyText(s string)
	ClipboardText() string
	// OpenURL opens a web page or an app's link (vscode://…); OpenPath
	// opens a file in its default app.
	OpenURL(url string)
	OpenPath(path string)
	// Notify shows a system notification.
	Notify(title, body string)
	// The capture window, the main window and the menu bar icon.
	ShowCapture()
	HideCapture()
	ShowMain()
	SetCaptureKey(accelerator string) error
	// SetClipKey sets the shortcut that keeps the clipboard in the
	// library, from any app.
	SetClipKey(accelerator string) error
	// SetShotKey sets the shortcut that takes a screenshot, from any app,
	// and Screenshot takes one, the main window stepping aside when it is
	// taken from the app.
	SetShotKey(accelerator string) error
	Screenshot(fromApp bool)
	// OpenShotSettings opens the system's settings of screen recording.
	OpenShotSettings()
	// ShotLog is the end of the screenshots' log.
	ShotLog() string
	// SetAppIcon shows the app's icon in the Dock or the taskbar, and the
	// menu bar's.
	SetAppIcon(app, tray []byte)
	// CheckUpdates checks for a newer version and offers to install it and
	// restart; AutoUpdates and SetAutoUpdates are whether it checks by
	// itself, once a day.
	CheckUpdates()
	AutoUpdates() bool
	SetAutoUpdates(on bool)
	SetTray(on bool)
	UpdateTray()
	ShowInFolder(path string)
	Trash(path string) error
	SetFullScreen(on bool)
	// ClipboardImage returns the picture on the clipboard as PNG, nil when
	// there is none, and ClipboardFiles the files copied, as in Finder.
	ClipboardImage() []byte
	ClipboardFiles() []string
	// PickImage asks for a picture file and calls done with it, "" when
	// canceled, on the main thread.
	PickImage(done func(path string))
}

// asyncRunner runs work off the main thread and then done on it, with what
// work returned.
type asyncRunner func(work func() (any, error), done func(any, error))

// Views of the main area.
const (
	viewMap = iota
	viewOutline
	viewBoard
	viewAgenda
	viewTimeline
	viewOverview
	viewTable
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
	path      string
	file      *model.File
	doc       *model.Document // the canvas shown, one of file.Sheets
	modTime   time.Time       // of the file as last read or written
	dirty     bool
	changed   time.Time // of the last unsaved change
	checked   time.Time // when the file was last checked for outside changes
	saveErr   string
	savedAt   time.Time // when this run last saved the map
	hist      *model.History
	hists     map[string]*model.History // of the maps opened, while the app runs
	views     map[string]*fileView      // how each map was left
	versions  versions.Store            // past versions of the maps
	lastSnap  map[string]time.Time      // when each map was last kept as a version
	conflict  bool                      // the file changed outside while the map had unsaved changes
	broken    string                    // a map that would not open, whose versions brokenBar offers
	brokenErr string
	lastEdit  string // the key of the last coalesced edit (see mutate)

	// Selection and in-place editing.
	selected  string
	editing   string // the node whose title is edited on the canvas
	editText  string
	editFocus bool              // the title input has had the focus
	reveal    bool              // scroll the selected node into view in the next frame
	multi     map[string]bool   // nodes selected besides a.selected, with Shift or Cmd
	focus     map[string]string // the branch each canvas is focused on, by canvas
	progress  map[string][2]int // done and total tasks under each node shown
	// Branches copied, to paste here or in another map: the copies, the
	// assets folder of their pictures, and the text put on the clipboard.
	clip       []*model.Node
	clipAssets string
	clipText   string

	// The canvas: where each canvas of the file is looked at from.
	cams    map[string]*camera
	canvas  ui.Handle
	boxes   layout.Result // the layout of the last frame, in map coordinates
	vp      ui.Rect       // the canvas's box in the last frame
	pointer [2]float32    // where the pointer is over the canvas
	mods    ui.Modifiers  // the modifier keys held, as of the last frame

	measured map[measureKey][2]float32 // texts measured, by how (see measureText)

	// Actions that change the map, taken while it is built, wait for the
	// end of the frame (see later).
	pending []func()

	// Canvas tabs.
	renamingSheet string
	sheetTitle    string
	confirmSheet  bool
	structureOpen bool
	panelTab      int
	notePreview   bool // the panel shows notes as Markdown
	// The command palette and the cheat sheet of shortcuts.
	paletteOpen  bool
	paletteMode  int
	paletteQuery string
	paletteIndex int
	paletteShown int // the line last scrolled into view
	cheatsOpen   bool
	// The window of past versions.
	historyOpen     bool
	mcpOpen         bool // the window on connecting AI agents
	linkDraft       string
	captureText     string
	captureWhere    int
	captureErr      string
	captureKeyDraft string
	settingsOpen    bool
	cardOpen        bool // the window of a clip's card
	cardFor         string
	cardOpts        cardOpts
	cardShowDate    bool
	cardShowSource  bool
	cardSource      string
	noteEditing     string // the node whose note the popover over the canvas edits
	settingsTab     int
	tourStep        int    // the step of the tour shown, 0 for none
	ghBusy          bool   // the issues are being synced
	ghResult        string // how the last sync went
	ghTokenDraft    string
	templatesOpen   bool // the window of templates
	fromTextOpen    bool // the window to paste text to make nodes of
	fromText        string
	trayAt          time.Time // when the menu bar icon was last brought up to date
	iterFilter      string    // the iteration the board and timeline show: "" all, "-" none
	tlWeeks         bool      // the timeline shows weeks rather than days
	tlDrag          tlDrag
	tlScroll        ui.ScrollState
	reminded        map[string]bool // tasks reminded of on remindedDay
	remindedDay     time.Time
	remindedAt      time.Time
	linkFrom        string // the node a link to a node is being chosen for
	relFrom         string // the node a relation line is being drawn from
	justFloated     bool   // a double click just made a floating topic, whose release should not end its editing
	historyList     []versions.Version
	historySel      int
	historyFile     *model.File           // the version chosen
	historyDiff     []model.Change        // what it changed, or from it to now
	historyToNow    bool                  // the changes from the version to now
	codeStat        codeStatus            // how far the code has gone since the baseline
	refs            *refs                 // the links of every map (refs.go)
	edits           int                   // counts the open map's changes, for what is worked out from it
	index           map[string]indexEntry // maps read for the search, by path
	// The maps' AI analyses (mapanalysis.go), by key; the map analyzed.
	mapMetas       map[string]*model.Analysis
	mapMetaMod     time.Time
	mapMetaFor     string
	mapMetaVersion int
	mapWorking     string
	mapAuto        bool                 // the analysis running was started by itself
	mapAutoAt      time.Time            // when the last one by itself started
	mapAutoFailed  string               // the provider and key with which one failed: no more until they change
	mapAutoSkip    map[string]time.Time // maps looked at, by path: their file's time then
	mapErr         string
	mapClipsSig    string
	mapClipsList   []*library.Clip
	mapIndex       *library.Index
	outlineFocus   string // the node whose line the outline focuses

	// The library of clips.
	lib           *library.Library
	libOpen       bool   // the main area shows the library
	libSel        string // the clip shown
	libQuery      string
	libKind       int        // kindAll, kindMaps, kindDocs or kindLinks
	libHome       bool       // 概览 is shown, not the list
	libDay        time.Time  // the day the list shows, from 概览's dots; zero for any
	unfiledOpen   bool       // 未归档 of the sidebar is open
	panelOff      bool       // the right panel is hidden
	sideItems     []sideItem // the sidebar's maps and documents, in order
	sideDrawn     []sideItem // as the frame before drew them
	sideFocus     string     // "side" while the sidebar has the keys
	libCat        string     // the category shown, "" all
	libTag        string     // the tag shown, "" all
	libDraft      string     // what is typed to keep
	libQueue      []clipJob
	libWorking    string            // the clip being read or sorted out
	libStatus     map[string]string // what is happening to clips, or went wrong
	libErr        string
	libNeedKey    string    // the clip whose organizing waits on an API key
	libMod        time.Time // of the clips folder, as last read or written
	libTagText    string    // a tag being typed for the clip shown
	libSynText    string    // the synonyms of the clip libSynFor, as edited
	libSynFor     string
	libSynEditing bool
	libConfirm    bool // deleting the clip shown is being confirmed
	fetchPage     fetchPager
	embed         embedFunc
	qvecs         map[string][]float32 // the vectors of queries, by query
	qPending      map[string]bool      // queries whose vectors are being made
	embedBusy     bool                 // clips' vectors are being made
	embedErr      string               // why making vectors failed, which stops it until the settings change
	libQueryAt    time.Time            // when the library's search was last typed in
	libHitsFor    libHitsKey           // what libHitsList was found for
	libHitsList   []library.Hit
	relatedFor    string // the clip and library version relatedList is of
	relatedList   []library.Hit
	dupe          bool // the last clip kept was kept before
	backupBusy    bool
	backupErr     string
	embedDrafts   [3]string // the settings' fields of the embedding model: address, key, model
	embedTest     string    // how testing the embedding model went
	noLocalModel  bool      // tests leave the built-in model out
	pluginBusy    bool
	pluginErr     string
	pluginNote    string
	pluginDone    int64 // bytes downloaded of pluginSize
	pluginSize    int64
	pluginCancel  context.CancelFunc
	pluginGet     pluginGetter
	clipKeyDraft  string
	shotKeyDraft  string
	nameTitle     string // the central topic the open map's name was given after
	dirID         fileID
	// The measures being edited (measures.go).
	cellKey, cellDraft            string
	renamingMeasure, measureDraft string
	newMeasure                    string // the maps folder's identity, to follow it when renamed
	recordingKey                  string // the shortcut being recorded, by its label
	// What kept a screenshot from being taken, said in the app's window,
	// which comes to the front for it.
	shotAlert                    bool
	shotAlertTitle, shotAlertMsg string
	shotAlertSettings            bool
	shotModes                    bool                  // the choice of how, shown on any system (tests)
	keyErr                       map[string]string     // why a shortcut recorded is not used
	iconBitmaps                  map[string]*ui.Bitmap // the app's icons, in the settings
	// The sync with GitHub (sync.go).
	syncBusy                       bool
	syncMsg, syncErr, syncCheck    string
	syncAt, syncTried, syncChanged time.Time
	syncLast                       ghsync.Result
	syncDrafts                     [2]string
	syncRun                        func(context.Context, string, ghsync.Config, func(string)) (ghsync.Result, error)
	syncCheckRepo                  func(context.Context, ghsync.Config) (*ghsync.Repo, error)

	// The presentation.
	presenting bool
	slides     []slide
	slideIndex int
	slideStep  int // items of the slide shown

	// The AI assistant.
	bitmaps      map[string]*ui.Bitmap // pictures of nodes, by file, decoded once
	lightbox     string                // the node whose picture shows large
	ai           aiState
	aiOpen       bool // the panel shows the assistant
	sendAI       aiSender
	sendDeepSeek deepseekSender
	async        asyncRunner
	post         func(fn func()) // runs fn on the main thread, from any

	// Chrome.
	mode          int
	search        string
	tagFilter     string
	sidebarWidth  float32
	panelWidth    float32
	resizingPanel bool
	filesOpen     bool
	catsOpen      bool
	tagsOpen      bool
	favoritesOpen bool
	folderClosed  map[string]bool // folders of the sidebar shown closed
	renaming      string          // the map or folder renamed in the sidebar
	renameText    string
	tagQuery      string
	notice        string    // a passing message in the toolbar, as "已复制"
	noticeAt      time.Time // since when it shows, for noticeTime
	exportCam     *camera   // set while the canvas is drawn into an image
	tagDraft      string    // a tag being typed in the node panel
	tagAdding     bool      // the sidebar's new-tag field is shown
	tagNew        string
	tagRenaming   string // the tag renamed in the sidebar
	tagRenameText string
	confirmDelete bool
	deletePath    string
}

func newApp(plat platform, cfg config) *app {
	a := &app{
		plat:          plat,
		cfg:           cfg,
		now:           time.Now,
		hist:          model.NewHistory(200),
		hists:         map[string]*model.History{},
		views:         loadViews(),
		versions:      versions.Store{Dir: versionsDir()},
		cams:          map[string]*camera{},
		sidebarWidth:  220,
		panelWidth:    300,
		filesOpen:     true,
		catsOpen:      true,
		tagsOpen:      true,
		favoritesOpen: true,
		folderClosed:  map[string]bool{},
		bitmaps:       map[string]*ui.Bitmap{},
		sendAI:        sendToClaude,
		sendDeepSeek:  sendToDeepSeek,
		fetchPage:     library.FetchPage,
		embed:         embedWith,
		pluginGet:     httpGet,
		keyErr:        map[string]string{},
		syncRun:       ghsync.Sync,
		syncCheckRepo: ghsync.Check,
		qvecs:         map[string][]float32{},
		qPending:      map[string]bool{},
		libStatus:     map[string]string{},
		post:          func(fn func()) { fn() },
		async: func(work func() (any, error), done func(any, error)) {
			v, err := work()
			done(v, err)
		},
	}
	a.syncTagColors()
	if cfg.PanelWidth >= panelMin {
		a.panelWidth = cfg.PanelWidth
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
	a.dirID, _ = idOf(dir)
	a.refreshFiles()
	a.openLibrary()
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

// open shows the map in path, saving the one open first, as it was left.
func (a *app) open(path string) error {
	a.commitEditIfAny()
	a.flush()
	a.lightbox, a.presenting, a.libOpen = "", false, false
	f, err := model.Load(path)
	if err != nil {
		a.saveErr = fmt.Sprintf("无法打开 %s: %v", filepath.Base(path), err)
		if !errors.Is(err, os.ErrNotExist) {
			a.broken, a.brokenErr = path, err.Error() // brokenBar offers its versions
		}
		return err
	}
	if a.broken == path {
		a.broken = ""
	}
	a.path, a.file = path, f
	a.nameTitle = rootTitle(f)
	a.conflict = false
	a.remember(path)
	a.learnTags(f)
	a.keepVersion(versions.Open)
	a.modTime = modTime(path)
	a.dirty, a.saveErr, a.lastEdit = false, "", ""
	a.savedAt = time.Time{}
	a.restoreView(f)
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

// noticeTime is how long a notice shows.
const noticeTime = 3 * time.Second

// tell shows a passing message in the toolbar.
func (a *app) tell(msg string) { a.notice, a.noticeAt = msg, a.now() }

// freeMapPath is the path of a map named name in dir, numbered past the
// maps there: "name", "name 2", "name 3"…
func freeMapPath(dir, name string) string {
	try := name
	for i := 2; ; i++ {
		path := filepath.Join(dir, model.FileName(try))
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return path
		}
		try = fmt.Sprintf("%s %d", name, i)
	}
}

// newMapIn creates a map in dir and opens it, editing its root.
func (a *app) newMapIn(dir string) {
	path := freeMapPath(dir, "新导图")
	name := mapName(path)
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
	a.removeMapMeta(path)
	if _, err := os.Stat(model.AssetsDir(path)); err == nil {
		_ = a.plat.Trash(model.AssetsDir(path)) // its pictures go with it
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
	// Another program changed the file since it was read: saving would
	// lose that, so the user chooses first (conflictBar).
	if mt := modTime(a.path); !mt.IsZero() && !a.modTime.IsZero() && !mt.Equal(a.modTime) {
		a.conflict = true
		return
	}
	if err := a.file.Save(a.path); err != nil {
		a.saveErr = "保存失败: " + err.Error()
		return
	}
	a.dirty, a.saveErr = false, ""
	a.savedAt = a.now()
	a.syncChange()
	a.modTime = modTime(a.path)
	a.followTitle()
	a.refreshFiles()
	a.keepVersionIfDue()
}

// flush saves unsaved changes, and how the map is left.
func (a *app) flush() {
	if a.conflict {
		// Leaving a map in conflict: the file's version stays, and the
		// changes made here are kept as a version to restore.
		a.keepVersion(versions.Conflict)
		a.conflict, a.dirty = false, false
	}
	if a.dirty {
		a.save()
	}
	a.rememberView()
	a.saveViews()
}

// tick saves after a pause in editing and reloads the file when another
// program, as an AI agent, changed it. It runs every frame.
func (a *app) tick(c *ui.Context) {
	now := a.now()
	if a.dirty && !a.conflict { // a conflict waits for a choice
		if wait := autosaveDelay - now.Sub(a.changed); wait > 0 {
			c.After(wait)
		} else {
			a.save()
		}
	}
	a.reminderCheck()
	if now.Sub(a.trayAt) >= time.Minute || a.trayAt.IsZero() {
		a.trayAt = now
		a.plat.UpdateTray()
	}
	if now.Sub(a.checked) >= 2*time.Second {
		a.checked = now
		a.followDir()
		a.checkLibrary()
		a.embedLibrary()
		a.autoBackup()
		a.syncDue()
		a.autoAnalyzeMaps()
		a.keysBack()
	}
	if a.path != "" && a.checked == now {
		if mt := modTime(a.path); !mt.IsZero() && !mt.Equal(a.modTime) {
			a.reloadFromDisk(c)
		}
	}
	c.After(2 * time.Second)
}

// reloadFromDisk takes the file as changed outside the app.
// Both versions are kept: the app's before, and the file's after.
func (a *app) reloadFromDisk(c *ui.Context) {
	a.modTime = modTime(a.path) // ask once
	if a.dirty {
		a.conflict = true // conflictBar asks which to keep
		return
	}
	before := a.file.Clone()
	a.reload()
	// An AI changing it through MCP kept the versions before and after
	// already: they are not kept twice.
	if !a.versions.IsLatest(a.path, a.file) {
		a.keepVersionOf(before, versions.BeforeExternal)
		a.keepVersion(versions.External)
	}
	c.ToastAction("已载入外部修改", "查看改动", func() {
		a.openHistory() // the newest version, with what it changed
	})
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
	a.edits++
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
	a.edits++
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
		n.Task.SetStatus(s, a.now())
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
	if n, p := a.selectedNode(); p != nil && n != a.shownRoot() {
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
	if p == nil || n == a.shownRoot() {
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
	shown := a.shownDoc()
	shown.Walk(func(n, _ *model.Node, depth int) bool {
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
	walk(shown.Root, 0)
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
	if a.exportCam != nil {
		return a.exportCam
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
