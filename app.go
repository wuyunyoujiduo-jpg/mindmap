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
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/vault"
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
	// CopyImage puts a picture, PNG, on the clipboard.
	CopyImage(png []byte)
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
	// SetAppearance has the app light ("light"), dark ("dark"), or as
	// the system is ("").
	SetAppearance(look string)
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
	viewAgenda // no more a view: kept for the maps left on it
	viewTimeline
	viewOverview
	viewTable // no more a view either: it was the measures' sheet
	// viewDiagram is a topic's 图解, in the canvas's place (diagram.go). It
	// is left for the map, and never kept as the view a map was left on.
	viewDiagram
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
	path    string
	file    *model.File
	doc     *model.Document // the canvas shown, one of file.Sheets
	modTime time.Time       // of the file as last read or written
	// base is the open map as its file was when last read or written:
	// what a change of the file made outside is merged against
	// (mergeOutside). dirSeen is when each folder of the maps was written
	// in, as save last saw (foldersTouched).
	base      *model.File
	dirSeen   map[string]time.Time
	dirty     bool
	changed   time.Time // of the last unsaved change
	checked   time.Time // when the file was last checked for outside changes
	saveErr   string
	savedAt   time.Time // when this run last saved the map
	hist      *model.History
	hists     map[string]*model.History // of the maps opened, while the app runs
	histAt    map[string]time.Time      // each as its file was when it was left (restoreView)
	histUsed  []string                  // the maps left with steps kept, the last left last (histMaps)
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
	editFocus bool         // the title input has had the focus
	editW     float32      // the widest the title edited has been
	selImage  string       // the node whose picture is selected, which Delete takes off
	notePic   *model.Image // the picture of a note shown large
	// summaryTopics are the nodes of the canvas drawn that are summaries'
	// topics, which show as such.
	summaryTopics map[string]bool
	// editComposing tells an input method is composing in the title.
	editComposing bool
	// The board's columns' lists, and the task to show there next.
	boardLists  map[model.Status]*ui.ListState
	boardReveal string
	reveal      bool              // scroll the selected node into view in the next frame
	multi       map[string]bool   // nodes selected besides a.selected, with Shift or Cmd
	focus       map[string]string // the branch each canvas is focused on, by canvas
	progress    map[string][2]int // done and total tasks under each node shown
	// Branches copied, to paste here or in another map: the copies, the
	// assets folder of their pictures, and the text put on the clipboard.
	clip       []*model.Node
	clipAssets string
	clipText   string
	// clipCut is, of topics cut, the ID each had by its ID on the
	// clipboard: pasted, what was drawn of them follows them (carryDias).
	// clipFrom is the map they were cut from, the one map where a paste
	// gives them back by those IDs (clipNodes).
	clipCut  map[string]string
	clipFrom string
	// movedOut are the branches moved to other maps while the app runs:
	// an undo that brings one back here takes it out there (movesFollow).
	movedOut []*movedOut
	// diaSwept is when the sets of topics that are gone were last looked
	// for, and in which folder (sweepDias).
	diaSwept    time.Time
	diaSweptFor string
	// imageBusy tells a picture of the canvas is being written.
	imageBusy bool

	// The canvas: where each canvas of the file is looked at from.
	cams    map[string]*camera
	canvas  ui.Handle
	boxes   layout.Result // the layout of the last frame, in map coordinates
	vp      ui.Rect       // the canvas's box in the last frame
	pointer [2]float32    // where the pointer is over the canvas
	// zoomHold is how far a pinch or the wheel went at 100%, which holds
	// the map there for a while.
	zoomHold float32
	// overCanvas tells the pointer is over the map, as of the last frame.
	overCanvas bool
	mods       ui.Modifiers // the modifier keys held, as of the last frame

	measured map[measureKey][2]float32 // texts measured, by how (see measureText)

	// Actions that change the map, taken while it is built, wait for the
	// end of the frame (see later).
	pending []func()

	// Canvas tabs.
	renamingSheet string
	sheetTitle    string
	structureOpen bool
	panelTab      int
	// The command palette and the cheat sheet of shortcuts.
	paletteOpen  bool
	paletteMode  int
	paletteQuery string
	paletteIndex int
	paletteShown int // the line last scrolled into view
	cheatsOpen   bool
	// The window of past versions.
	historyOpen    bool
	mcpOpen        bool // the window on connecting AI agents
	linkDraft      string
	captureText    string
	captureWhere   int
	captureErr     string
	settingsOpen   bool
	cardOpen       bool // the window of a clip's card
	cardFor        string
	cardOpts       cardOpts
	cardShowDate   bool
	cardShowSource bool
	cardSource     string
	noteEditing    string              // the node whose note the popover over the canvas edits
	noteDocs       map[string]*noteDoc // the notes edited as rich text, by place
	settingsTab    int
	tourStep       int  // the step of the tour shown, 0 for none
	templatesOpen  bool // the window of templates
	fromTextOpen   bool // the window to paste text to make nodes of
	fromText       string
	trayAt         time.Time // when the menu bar icon was last brought up to date
	tlWeeks        bool      // the timeline shows weeks rather than days
	tlDrag         tlDrag
	tlScroll       ui.ScrollState
	reminded       map[string]bool // tasks reminded of on remindedDay
	remindedDay    time.Time
	remindedAt     time.Time
	linkFrom       string       // the node a link to a node is being chosen for
	relFrom        string       // the node a relation line is being drawn from
	relTo          *[2]float32  // where the pointer draws it to, in the window
	selRel         string       // the relation line selected
	justFloated    bool         // a double click just made a floating topic, whose release should not end its editing
	floatDrag      string       // the floating topic being dragged
	floatOver      string       // the topic it would go under, let go there
	floatMoved     [2]float32   // how far the last frame moved it, on the map
	floatAt        *model.Point // where the clip chosen in the palette goes, as a floating topic
	pressAt        [2]float32   // where the canvas was last pressed, in its box
	historyList    []versions.Version
	historySel     int
	historyFile    *model.File           // the version chosen
	historyDiff    []model.Change        // what it changed, or from it to now
	codeStat       codeStatus            // how far the code has gone since the baseline
	refs           *refs                 // the links of every map (refs.go)
	edits          int                   // counts the open map's changes, for what is worked out from it
	nodeIdx        map[string]*nodeIndex // maps' nodes by their words (matcher.go)
	qpending       map[string]bool       // queries whose vectors are being made
	attachRef      string                // the material the palette places (paletteAttach)
	placesKey      string                // what placesList was worked out for
	placesList     []place
	relatedKey     string // what relatedClips and relatedNodes were worked out for
	relatedNode    string // the topic they are of, and how long until they are worked out again (relatedOfCached)
	relatedWait    time.Duration
	relatedClips   []library.Hit
	relatedNodes   []place
	index          map[string]indexEntry // maps read for the search, by path
	// The maps' AI analyses (mapanalysis.go), by key; the map analyzed.
	mapMetas       map[string]*model.Analysis
	mapMetaMod     time.Time
	mapMetaFor     string
	mapMetaVersion int

	// The 图解 of topics (diagram.go): the sets of every map, by their
	// topic's ID, and what of one shows.
	dias          map[string]*diaFile
	diaMod        time.Time
	diaFor        string
	diaVersion    int
	diaLooked     time.Time // when the sets' folder was last looked at
	diaDueKey     string
	diaDue        []*diaFile
	diaIDsKey     string
	diaIDsOf      map[string]*diaFile
	diaChangeKey  string
	diaChanged    diaChange
	dia           diaState
	diaBusy       string // the topic being drawn
	diaSince      time.Time
	diaErr        string
	diaErrFor     string             // the topic whose drawing failed
	diaHover      string             // the node of a graph under the pointer
	diaBare       bool               // a kind is drawn as a figure: no scrolling room, bar or margin (diaFigure)
	figNat        map[string]float32 // a figure's own height, once drawn, by what it is of
	figLast       string             // the figure drawn in the frame before
	figHandle     ui.Handle
	diaZoom       float32 // of the diagram shown; 0 fits the page's width
	diaReveal     string  // the node of a graph to bring into view
	diaFocus      bool    // the diagram takes the keys, just opened
	diaStage      ui.Handle
	diaBody       ui.Handle
	mentionOf     map[string][]mention
	mapWorking    string
	mapAuto       bool                 // the analysis running was started by itself
	mapAutoAt     time.Time            // when the last one by itself started
	mapAutoFailed string               // the provider and key with which one failed: no more until they change
	mapAutoSkip   map[string]time.Time // maps looked at, by path: their file's time then
	mapErr        string
	mapClipsSig   string
	mapClipsList  []*library.Clip
	mapIndex      *library.Index
	othersIndex   *library.Index // the maps but the open one, for a topic's page (searchOtherMaps)
	othersSig     uint64
	outlineFocus  string // the node whose line the outline focuses
	// The outline's lines selected whole, to copy, cut or delete: from
	// outlineAnchor to outlineEnd, by dragging across lines or Shift+arrows.
	outlineBlock              bool
	outlineAnchor, outlineEnd string
	outlineRowAt              map[string]ui.Rect       // where each line built was drawn
	outlineDragFrom           string                   // the line pressed, dragged from
	outlinePressed            bool                     // it is pressed still, in this frame
	outlineDragY              float32                  // where the pointer is, dragging
	outlineLists              map[string]*ui.ListState // the outline's list, of each canvas
	outlineFocusWas           string                   // the focus a frame before: a line it goes to is scrolled to

	// The library of clips.
	lib           *library.Library
	libOpen       bool   // the main area shows the library
	libSel        string // the clip shown
	libQuery      string
	libKind       int               // kindAll, kindMaps, kindDocs or kindLinks
	libHome       bool              // 今天 is shown, not the list
	barOpen       bool              // the one bar is open, a card to write in (bar.go)
	barKind       int               // what the bar keeps its words as, chosen by hand: barAuto, barATask, barThought
	keptLast      string            // the clip last left in 收集 from what waits: said there, with the way back
	projOpen      bool              // 项目, the maps by folder, is the page shown (places.go)
	projFolder    string            // the folder whose maps 项目 shows: all of them when empty
	writeRefocus  string            // the card to write in that takes the focus back, after a tool of its was clicked
	writeRefs     map[string]string // the notes referred to in a draft: the @name written, the clip's ID
	noMotion      bool              // nothing animates: the tests look at where things end
	homeDraft     string            // what is typed in 今天's line: a note of 灵感, or a task
	libDay        time.Time         // the day the list shows, from 概览's dots; zero for any
	panelOff      bool              // the right panel is hidden
	sideItems     []string          // the list's maps, in order
	sideDrawn     []string          // as the frame before drew them
	sideFocus     string            // "side" while the sidebar has the keys
	libCat        string            // the category shown, "" all
	libTag        string            // the tag shown, "" all
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
	nameTitle     string // the central topic the open map's name was given after
	dirID         fileID
	recordingKey  string // the shortcut being recorded, by its label
	// What kept a screenshot from being taken, said in the app's window,
	// which comes to the front for it.
	shotAlert                    bool
	shotAlertTitle, shotAlertMsg string
	shotAlertSettings            bool
	keyErr                       map[string]string     // why a shortcut recorded is not used
	iconBitmaps                  map[string]*ui.Bitmap // the app's icons, in the settings
	// The sync with GitHub (sync.go).
	syncBusy                    bool
	syncMsg, syncErr, syncCheck string
	syncAt, syncTried           time.Time
	syncHold                    time.Time // not synced before: GitHub's calls are used up till then
	syncLast                    ghsync.Result
	syncDrafts                  [2]string
	syncRun                     func(context.Context, string, ghsync.Config, func(string)) (ghsync.Result, error)
	merges                      []*mergeAsk // maps merged by the sync, whose clashes are asked
	mergeOpen                   bool
	mergesOf                    string // the folder whose questions kept were read (loadMerges)
	syncCheckRepo               func(context.Context, ghsync.Config) (*ghsync.Repo, error)

	// The presentation.
	presenting bool
	slides     []slide
	slideIndex int
	slideStep  int // items of the slide shown

	// The AI assistant.
	bitmaps      map[string]*ui.Bitmap // pictures of nodes, by file, decoded once
	pictureBusy  map[string]bool       // the pictures being read (filePicture)
	collect      collectState          // more of 收集's state (libview.go)
	lightbox     string                // the node whose picture shows large
	ai           aiState
	aiOpen       bool // the panel shows the assistant
	sendAI       aiSender
	sendDeepSeek deepseekSender
	async        asyncRunner
	post         func(fn func()) // runs fn on the main thread, from any

	tasks  tasksState  // the tasks of all the maps (taskspage.go)
	skills skillsState // the skills of the AI agents (skillsview.go)
	// holidaysAsked is when each year's notice of the holidays was last
	// asked for (holidays.go).
	holidaysAsked map[int]time.Time
	skillsOpen    bool // 技能 is the place shown
	libReview     int  // the way 收集 looks back, or reviewOff (memos.go)
	libWalk       int  // how many times 随机漫步 went on
	clipTagsFor   int  // the library's version clipTagsList is of
	clipTagsOf    *library.Library
	clipTagsList  []tagCount
	// lastTheme is the theme the window was last drawn in, for what is
	// worked out outside a frame's build.
	lastTheme *ui.Theme

	// Chrome.
	mode       int
	search     string
	searchOpen bool // the map's search shows: ⌘F, until Esc
	// panelShow are the parts of the topic's page opened to add to, by
	// the node and the part (panel.go).
	panelShow map[string]bool
	// clipPlaces are where the AI would put notes and clips, by their
	// IDs (sortout.go); clipPlacing while it is asked.
	clipPlaces    map[string]taskPlace
	nearPlaces    map[string]*taskPlace // where what waits would go by its words alone (nearPlace)
	nearEdits     int
	clipPlacing   bool
	clipPlaceErr  string
	tagFilter     string
	taskFilter    int // the tasks a map shows bright alone (taskFilterOpen…)
	panelWidth    float32
	resizingPanel bool
	// Folders of the sidebar are shown closed but for those opened, which
	// the config keeps.
	renaming    string // the map or folder renamed in the sidebar
	renameText  string
	tagQuery    string
	notice      string           // a passing message in the toolbar, as "已复制"
	noticeAt    time.Time        // since when it shows
	libReveal   string           // the clip the keyboard went to, to scroll to once
	secretsOf   *library.Library // what secretCount counted, and when
	secretsAt   int
	secrets     int
	treeReveal  string // the map just opened, whose row of the directory is scrolled to once
	treeQuick   bool   // the directory's rows being drawn are 快捷访问's
	escLeave    bool   // Escape was pressed in a field of a pane: the focus leaves it (escRest)
	secretShown string // the clip whose secrets its details show, asked for
	// The secrets sealed (vault.go): the keys of the library, until when
	// they stay unlocked, the window that asks, what is being typed of a
	// clip, and a secret copied, to clear the clipboard of.
	vault       *vault.Vault
	vaultUntil  time.Time
	vaultAsk    vaultAsk
	drafts      map[string]draft
	marking     mark // the part of a field chosen, to be made a secret
	copied      string
	copiedUntil time.Time
	wallShown   int // how many clips 收集's wall shows, when more than wallStep
	// Long work, told in cards (wait.go): how many clips the index is made
	// of and how many are left, since when, and what to say once done.
	embedTotal, embedLeft int
	embedSince            time.Time
	embedResult           string
	orgTotal, orgDone     int // the clips of a run being organized, and those done
	orgSince              time.Time
	orgResult             string
	pluginSince           time.Time
	pluginResult          string
	syncChecking          bool                     // the sync's repository is being checked
	syncByHand            bool                     // the sync going on was asked for: its result is said
	pluginChecking        bool                     // a newer plugin is being looked for
	noticeAct             string                   // the name of the notice's key, when not 撤销
	noticeToasted         time.Time                // the notice shown as a toast already
	noticeUndo            func()                   // what takes back what the notice says, if it can be
	exportCam             *camera                  // set while the canvas is drawn into an image
	tagDraft              string                   // a tag being typed in the node panel
	tagFocus              int                      // frames left for the node panel's field of a new tag to take the focus
	vids                  map[string]*vidPlayer    // the players of the videos shown (video.go)
	vidFrame              int                      // the frames, for the players not shown to be closed
	videoMuted            bool                     // the videos play without their sound
	videoBig              string                   // the clip whose video plays larger, over the window
	grabs                 []*frameGrab             // the videos whose pictures are being taken, the first being stepped
	vidLens               map[string]time.Duration // the videos' lengths, as kept with their posters
	vidPosterAsked        map[string]bool          // the videos whose poster was asked for in this run
	vidPosterHas          map[string]bool          // the videos whose poster is in the cache
	folds                 map[string]bool          // the folded parts opened (foldKey)
	vidShots              map[string]vidShots      // the pictures taken of the videos, by clip
	tagOffered            bool                     // the tags offered under a title being written show
	tagCaret              string                   // the topic whose title field has its caret put back at its end, a tag taken
	tagCaretTTL           int                      // frames left for tagCaret to find its field
	tagDismissed          string                   // the #word whose offer was put away: field|word
	tagAdding             bool                     // the sidebar's new-tag field is shown
	tagNew                string
	tagRenaming           string // the tag renamed in the sidebar
	tagRenameText         string
	confirmDelete         bool
	deletePath            string
}

// clock is the time a new app starts with as now: the tests' own day, so
// that the guide map's dates are where they expect them.
var clock = time.Now

func newApp(plat platform, cfg config) *app {
	a := &app{
		plat:          plat,
		cfg:           cfg,
		panelOff:      !cfg.PanelOpen,
		now:           clock,
		hist:          newHistory(),
		hists:         map[string]*model.History{},
		views:         loadViews(),
		versions:      versions.Store{Dir: versionsDir()},
		cams:          map[string]*camera{},
		panelWidth:    300,
		captureWhere:  captureAuto, // a note, or a task when it says a day
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
	// A folder that is not there and is made now is not the folder the
	// sync remembers at this path (one moved, renamed or put in the bin
	// while the app was closed, or a disk not yet there): with its
	// memory, every file of before read as deleted here, and was deleted
	// from the repository and then from the other computer.
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		for _, p := range []string{syncStatePath(dir), mergesPath(dir)} {
			if p != "" {
				_ = os.Remove(p)
			}
		}
	}
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
	a.skillsMigrate() // a store of before in this folder, to ~/.agents/skills, before any sync
}

func (a *app) refreshFiles() {
	tree, err := model.ScanFolder(a.dir)
	if err != nil {
		a.tree, a.files = model.Folder{Path: a.dir, Name: filepath.Base(a.dir)}, nil
		return
	}
	// The folder the automations had, which the app has no more, is not
	// shown while it holds no map.
	tree.Folders = slices.DeleteFunc(tree.Folders, func(f model.Folder) bool {
		return f.Name == "自动化" && len(f.AllFiles()) == 0
	})
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
	a.path, a.file, a.base = path, f, f.Clone()
	a.edits++                                        // what was worked out of the map before is of another content
	if a.dia = (diaState{}); a.mode == viewDiagram { // a diagram is of the map left
		a.mode = viewMap
	}
	// The directory goes with it: the folders it is in open, and its row
	// is brought into view (treeReveal), so that where it is kept is seen.
	for dir := filepath.Dir(path); dir != a.dir && inside(dir, a.dir); dir = filepath.Dir(dir) {
		a.setFolderOpen(dir, true)
	}
	a.treeReveal = path
	a.nameTitle = rootTitle(f)
	a.conflict = false
	a.remember(path)
	a.learnTags(f)
	a.keepVersion(versions.Open)
	a.modTime = modTime(path)
	a.dirty, a.saveErr, a.lastEdit = false, "", ""
	a.savedAt = time.Time{}
	a.healSummaries(false) // a summary its file had with no child, mended now and in no step of the user's
	a.restoreView(f)
	a.adoptDias()
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

// tell shows a passing message in the toolbar.
func (a *app) tell(msg string) {
	a.notice, a.noticeAt, a.noticeUndo, a.noticeAct = msg, a.now(), nil, ""
}

// tellAct says what was done and offers what is next with it, under the
// name act: to show a file made, as to take a thing back.
func (a *app) tellAct(msg, act string, do func()) {
	a.notice, a.noticeAt, a.noticeUndo, a.noticeAct = msg, a.now(), do, act
}

// tellUndo says what was done and offers to take it back.
func (a *app) tellUndo(msg string, undo func()) { a.tellAct(msg, "撤销", undo) }

// toasts shows a notice not shown yet as a toast, on whatever page: with
// 撤销 on it when what was done can be taken back.
func (a *app) toasts(c *ui.Context) {
	if a.notice == "" || a.noticeAt.Equal(a.noticeToasted) {
		return
	}
	a.noticeToasted = a.noticeAt
	if undo := a.noticeUndo; undo != nil {
		c.ToastAction(a.notice, cmpOr(a.noticeAct, "撤销"), func() { undo() })
		return
	}
	c.Toast(a.notice)
}

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
	if path == a.inboxPath() {
		a.tell("收件箱是兜底的导图，不能删：里面的任务归位后，它就空了")
		return
	}
	if path == a.path {
		a.dirty = false // nothing to save of a map being deleted
	}
	if err := a.plat.Trash(path); err != nil {
		a.saveErr = err.Error()
		return
	}
	a.removeMapMeta(path)
	a.dropDiasOf(path)
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
	// The file changed since it was read, by another computer's sync or
	// another program: saving would lose that. What changed there is
	// merged into the map first (mergeOutside); where it cannot be, the
	// user chooses which to keep (conflictBar).
	if mt := modTime(a.path); !mt.IsZero() && !a.modTime.IsZero() && !mt.Equal(a.modTime) {
		if !a.mergeOutside() {
			a.conflict = true
			return
		}
	}
	touched := a.foldersTouched() // before the write, which is one
	if err := a.file.Save(a.path); err != nil {
		a.saveErr = "保存失败: " + err.Error()
		return
	}
	a.dirty, a.saveErr = false, ""
	a.savedAt = a.now()
	a.modTime = modTime(a.path)
	a.base = a.file.Clone()
	was := a.path
	a.followTitle()
	// The maps' list is read again only when a folder was written in by
	// something else since the last save (a map made, renamed or written
	// by an agent or a sync), or the map was renamed: read at each save,
	// some eight tenths of a second after each pause in typing, it was
	// every folder listed and every map looked at on the window's thread.
	switch {
	case a.path != was: // renamed after its topic: read then (moved)
	case touched:
		a.refreshFiles()
	default:
		a.noteSaved()
	}
	a.foldersTouched() // as they are now, this write among it
	a.keepVersionIfDue()
}

// foldersTouched tells whether a folder of the maps was written in since
// it was last asked: a file made, removed, renamed, or written through
// another renamed over it, as the app, the agents and the sync write.
func (a *app) foldersTouched() bool {
	seen := map[string]time.Time{}
	for _, dir := range a.tree.FolderPaths() {
		seen[dir] = modTime(dir)
	}
	same := len(seen) == len(a.dirSeen)
	for dir, at := range seen {
		if was, ok := a.dirSeen[dir]; !ok || !was.Equal(at) {
			same = false
		}
	}
	a.dirSeen = seen
	return !same
}

// noteSaved has the maps' list say the open map was written just now,
// first of its folder, without the folders being read again.
func (a *app) noteSaved() {
	var touch func(f *model.Folder) bool
	touch = func(f *model.Folder) bool {
		for i := range f.Files {
			if f.Files[i].Path == a.path {
				f.Files[i].Modified = a.modTime
				slices.SortStableFunc(f.Files, func(x, y model.FileInfo) int { return y.Modified.Compare(x.Modified) })
				return true
			}
		}
		for i := range f.Folders {
			if touch(&f.Folders[i]) {
				return true
			}
		}
		return false
	}
	if !touch(&a.tree) { // not among them: a new one
		a.refreshFiles()
		return
	}
	a.files = a.tree.AllFiles()
}

// mergeOutside merges into the open map, which has changes not saved,
// what changed its file meanwhile: another computer's edits, brought by
// the sync, or a program's, as an AI agent's. Topic by topic
// (model.Merge), against the map as its file was when last read or
// written; what both changed, each its own way, stays as it is here and
// is asked (mergeView). It is one step, which undo takes back, and both
// versions are kept: the file's, and the map as it was here. It reports
// whether it could: not with nothing to merge against, nor a file that
// does not read.
//
// Before, the user was asked which of the two to keep whole, and
// 保留我的版本 threw away the other computer's work.
func (a *app) mergeOutside() bool {
	if a.base == nil || a.file == nil || a.doc == nil {
		return false
	}
	a.commitEditIfAny() // a title being typed is of the map as it is here
	at := modTime(a.path)
	theirs, err := model.Load(a.path)
	if err != nil {
		return false
	}
	if model.SameContent(theirs, a.base) { // written, and the same: nothing to merge
		// But for the agents' record, which is not of the content: what
		// one wrote of its session meanwhile is kept, not saved over.
		if m, _ := model.Merge(a.base, a.file, theirs); m != nil {
			a.file.Log = m.Log
		}
		a.base = theirs
		a.modTime, a.conflict = at, false
		return true
	}
	merged, clashes := model.Merge(a.base, a.file, theirs)
	if data, err := merged.Marshal(); err != nil {
		return false
	} else if _, err := model.Parse(data); err != nil {
		return false
	}
	now := a.now()
	_, _ = a.versions.Save(a.path, theirs, versions.External, "", now)
	_, _ = a.versions.Save(a.path, a.file, versions.BeforeMerge, "", now.Add(1))
	when := ghsync.MergeInfo{BaseAt: a.savedAt, LocalAt: a.changed, RemoteAt: at}
	a.hist.Record(a.state())
	a.restore(model.State{File: merged, Sheet: a.doc.ID, Selected: a.selected})
	a.base, a.modTime, a.conflict = theirs, at, false
	a.dirty, a.changed = true, now
	a.tell("这张导图在别处也改了（另一台电脑或 AI），已和你改的合在一起，可以撤销")
	a.queueMerge(&mergeAsk{path: a.path, clashes: clashes, when: when, outside: true})
	return true
}

// flush saves unsaved changes, and how the map is left.
func (a *app) flush() {
	if a.dirty && !a.conflict {
		a.save() // which merges what changed outside, or finds that it cannot
	}
	if a.conflict {
		// Leaving a map in conflict: the file's version stays, and the
		// changes made here are kept as a version to restore.
		a.keepVersion(versions.Conflict)
		a.conflict, a.dirty = false, false
		a.hist = newHistory() // its steps are of the changes left, not of the file's version
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
		} else if a.editing == "" || modTime(a.path).Equal(a.modTime) {
			// Not while a title is typed and the file changed outside: the
			// merge of the two would close the field (below).
			a.save()
		}
	}
	a.reminderCheck()
	a.vaultTick(c, now)
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
	// A file changed outside is read when the title being typed on the
	// canvas is done, not under it: read at once, the field closed, what
	// was typed was gone, and a topic just made was left with no title
	// (an agent's hook writing the map as the user typed).
	if a.path != "" && a.checked == now && a.editing == "" {
		if mt := modTime(a.path); !mt.IsZero() && !mt.Equal(a.modTime) {
			a.reloadFromDisk(c)
		}
	}
	c.After(2 * time.Second)
}

// reloadFromDisk takes the file as changed outside the app.
// Both versions are kept: the app's before, and the file's after.
func (a *app) reloadFromDisk(c *ui.Context) {
	if a.dirty {
		// Changed here too: the two are merged, and where they cannot be,
		// conflictBar asks which to keep.
		if !a.mergeOutside() {
			a.modTime = modTime(a.path) // ask once
			a.conflict = true
		}
		return
	}
	a.modTime = modTime(a.path) // ask once
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
	a.modTime, a.base = modTime(a.path), f.Clone()
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
// field or the moves of one drag, make one step: until the pointer is
// pressed again or the field is left (stepEnds), so that two drags of a
// thing, or a title written, left and written again, are two.
func (a *app) mutate(key string, fn func()) {
	if a.doc == nil {
		return
	}
	if key == "" || key != a.lastEdit {
		a.hist.Record(a.state())
	}
	a.lastEdit = key
	sums := a.summarized()
	fn()
	a.dirty = true
	a.changed = a.now()
	a.edits++
	a.settleSummaries(sums)
}

func (a *app) undo() {
	if s, ok := a.hist.Undo(a.state()); ok {
		a.restore(s)
		a.dirty, a.changed = true, a.now()
		a.movesFollow() // a branch moved to another map is taken out of it
	}
}

func (a *app) redo() {
	if s, ok := a.hist.Redo(a.state()); ok {
		a.restore(s)
		a.dirty, a.changed = true, a.now()
		a.movesFollow()
	}
}

func (a *app) state() model.State {
	return model.State{File: a.file, Sheet: a.doc.ID, Selected: a.selected}
}

// restore shows a state of the history, or a file read again.
func (a *app) restore(s model.State) {
	a.edits++
	a.file, a.doc = s.File, nil
	a.healSummaries(true) // of a file read again, or a merge's
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
	a.selected, a.editing, a.editText, a.editFocus, a.editW, a.editComposing = id, id, n.Title, false, 0, false
	if n.Title == "新主题" || n.Title == "新导图" || strings.HasPrefix(n.Title, "新导图 ") {
		a.editText = "" // a title given for now: typing starts it afresh, the old one kept if none is typed
	}
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
	// A #word is words of the title unless made a tag as it was written
	// (takeTitleTag): no rule turns it into one here.
	title := strings.TrimRight(a.editText, " 　") // a space left where a #word was taken
	if title == "" {
		title = cmpOr(n.Title, "新主题")
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
	a.addSiblingAt(true)
}

// addSiblingAt adds a node after the one selected, or before it.
func (a *app) addSiblingAt(after bool) {
	if a.doc == nil {
		return
	}
	var n *model.Node
	a.mutate("", func() {
		if after {
			n, _ = a.doc.AddSibling(a.selected, "")
		} else {
			n, _ = a.doc.AddSiblingBefore(a.selected, "")
		}
	})
	if n != nil {
		a.startEdit(n.ID)
	}
}

func (a *app) deleteSelected() {
	if n, p := a.selectedNode(); n == nil || p == nil && !a.doc.IsFloating(n.ID) {
		return // the root stays
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
			n.DropTask()
		}
	})
}

func (a *app) setStatus(n *model.Node, s model.Status) {
	a.mutate("", func() { n.SetStatus(s, a.now()) })
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

// placeNode puts a dragged node next to another, before or after it,
// among that node's siblings.
func (a *app) placeNode(id, by string, after bool) {
	if id == by || a.doc == nil {
		return
	}
	n, _ := a.doc.Find(id)
	b, p := a.doc.Find(by)
	if n == nil || b == nil || p == nil || n.Contains(by) {
		return
	}
	i := slices.Index(p.Children, b)
	if after {
		i++
	}
	a.mutate("", func() { _ = a.doc.Move(id, p.ID, i) })
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

// zoomSteps are where the buttons and the keys zoom to, 100% among them.
var zoomSteps = []float32{0.3, 0.5, 0.67, 0.8, 0.9, 1, 1.1, 1.25, 1.5, 1.75, 2, 2.5, 3}

// zoomStep zooms in (or out) to the next step: from 95%, in to 100%.
func (a *app) zoomStep(in bool) {
	z := a.cam().Zoom
	if in {
		for _, s := range zoomSteps {
			if s > z+0.001 {
				a.setZoom(s)
				return
			}
		}
		return
	}
	for i := len(zoomSteps) - 1; i >= 0; i-- {
		if zoomSteps[i] < z-0.001 {
			a.setZoom(zoomSteps[i])
			return
		}
	}
}

// zoomBy zooms by a factor bit by bit, as a pinch or the wheel do, around
// (x, y): passing 100%, it stops there, and, held (a gesture's small
// steps), goes on once the gesture has gone some way further; a mouse's
// wheel, by notches, goes on with the next.
func (a *app) zoomBy(f, x, y float32, hold bool) {
	z := a.cam().Zoom
	if z == 1 && hold {
		if a.zoomHold == 0 {
			a.zoomHold = 1
		}
		a.zoomHold *= f
		if a.zoomHold < 1.12 && a.zoomHold > 1/1.12 {
			return // held at 100%
		}
		f, a.zoomHold = a.zoomHold, 1
	}
	next := z * f
	if z < 1 && next > 1 || z > 1 && next < 1 {
		next, a.zoomHold = 1, 1
	}
	a.setZoomAt(next, x, y)
}

// pinch zooms the map by a pinch on the touchpad over it, around the
// pointer.
func (a *app) pinch(magnification float32) {
	if a.doc == nil || a.mode != viewMap || !a.overCanvas || a.presenting {
		return
	}
	a.zoomBy(1+magnification, a.pointer[0], a.pointer[1], true)
}

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
	if a.doc != nil && a.doc.ID != id {
		// What was chosen of the canvas left, and a relation begun on it,
		// are not of this one: the panel said one topic chosen of several,
		// and a relation ended here joined a topic that is not here.
		a.multi, a.relFrom, a.selRel = nil, "", ""
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
	switch a.taskFilter {
	case taskFilterOpen:
		if n.Task == nil || n.Task.Status == model.StatusDone {
			return false
		}
	case taskFilterWeek: // not done, and due before next week: this week's and the late
		if n.Task == nil || n.Task.Status == model.StatusDone || n.Task.Due == nil ||
			!n.Task.Due.Before(a.weekOf(today(a.now())).AddDate(0, 0, 7)) {
			return false
		}
	}
	return n.Matches(a.search)
}

func (a *app) filtering() bool {
	return a.tagFilter != "" || a.search != "" || a.taskFilter != taskFilterNone
}

// Which tasks alone a map shows bright, the rest of it dimmed
// (app.taskFilter): none, those not done, those of this week.
const (
	taskFilterNone = iota
	taskFilterOpen
	taskFilterWeek
)

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
