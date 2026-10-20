// MyMind is a desktop mind map for managing projects: nodes carry notes,
// tags and tasks with dates, shown as a map, a board and an agenda.
package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	updaterui "github.com/egoist/mygo/plugins/updater/native"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/screenshot"
)

// desktop implements platform with MyGo's dialogs and shell.
type desktop struct {
	win     *mygo.Window
	app     *app
	capture *mygo.Window // the capture window, made the first time
	tray    *mygo.Tray
	key     string // the capture shortcut registered
	clipKey string // the shortcut registered to keep the clipboard
	shotKey string // the shortcut registered to take a screenshot
}

func (d *desktop) ShowCapture() {
	if d.capture == nil {
		d.capture = mygo.NewWindow(mygo.WindowOptions{
			Title:       "快速记录",
			Width:       640,
			Height:      170,
			MinWidth:    420,
			MinHeight:   150,
			AlwaysOnTop: true,
			Hidden:      true,
			Content:     ui.View(d.app.captureView),
		})
		d.capture.Center()
	}
	d.capture.Show()
	d.capture.Focus()
}

func (d *desktop) HideCapture() {
	if d.capture != nil {
		d.capture.Hide()
	}
	d.win.Update(func() {}) // the map may have changed
	d.UpdateTray()
}

func (d *desktop) ShowMain() {
	d.win.Show()
	d.win.Focus()
}

func (d *desktop) SetCaptureKey(key string) error {
	if d.key != "" {
		mygo.GlobalShortcut.Unregister(d.key)
	}
	d.key = ""
	if key == "" { // let go, while it is being recorded
		return nil
	}
	if err := mygo.GlobalShortcut.Register(key, func() { logShot("按下快速记录快捷键 %s", key); d.ShowCapture() }); err != nil {
		return err
	}
	d.key = key
	return nil
}

func (d *desktop) SetClipKey(key string) error {
	if d.clipKey != "" {
		mygo.GlobalShortcut.Unregister(d.clipKey)
	}
	d.clipKey = ""
	if key == "" { // let go, while it is being recorded
		return nil
	}
	if err := mygo.GlobalShortcut.Register(key, func() {
		logShot("按下收藏剪贴板快捷键 %s", key)
		d.win.Update(d.app.clipFromShortcut)
	}); err != nil {
		return err
	}
	d.clipKey = key
	return nil
}

func (d *desktop) SetShotKey(key string) error {
	if d.shotKey != "" {
		mygo.GlobalShortcut.Unregister(d.shotKey)
	}
	d.shotKey = ""
	if key == "" { // let go, while it is being recorded
		return nil
	}
	if err := mygo.GlobalShortcut.Register(key, func() {
		logShot("按下截图快捷键 %s", key)
		d.Screenshot(false)
	}); err != nil {
		return err
	}
	d.shotKey = key
	return nil
}

// Screenshot frames a part of the screen. Taken from the app, the main
// window steps aside and comes back after; taken from another app, the
// focus goes back to it. Each step is logged to screenshot.log, next to
// the settings, and what fails is said.
func (d *desktop) Screenshot(fromApp bool) {
	inApp := fromApp || d.win.IsFocused()
	logShot("截图开始：从应用内 %v，应用在前台 %v", fromApp, inApp)
	go d.overlayShot(fromApp)
}

// overlayShot takes a screenshot: every display frozen under a window of
// its own, in which a region is framed and marked up. Off the main thread.
func (d *desktop) overlayShot(fromApp bool) {
	if !screenshot.HasPermission() {
		logShot("没有屏幕录制权限")
		screenshot.RequestPermission()
		// Said in the app's window, brought to the front: a dialog of its
		// own could open behind the app the shortcut was pressed in.
		d.win.Update(func() {
			d.ShowMain()
			d.app.showShotAlert("截图需要屏幕录制权限", screenshot.PermissionHelp, true)
		})
		return
	}
	if fromApp {
		d.win.Hide()
		time.Sleep(250 * time.Millisecond) // till it is off the screen
	}
	// The windows of the screenshot are panels: they show over the app the
	// shortcut was pressed in, full-screen ones too, without activating
	// this app, so the other keeps the keyboard when they close.
	screenshot.Captured = nil
	ok, err := screenshot.Start(func(png []byte, act screenshot.Action) {
		logShot("截图完成：%d 字节，操作 %d", len(png), act)
		if act == screenshot.Save {
			d.saveShot(png)
			return
		}
		d.win.Update(func() { d.app.keepShot(png, act, fromApp) })
	}, func() {
		if fromApp {
			d.win.Update(d.ShowMain)
		}
	})
	if err != nil {
		logShot("截图失败：%v", err)
		d.win.Update(func() {
			d.ShowMain()
			d.app.showShotAlert("截图失败", err.Error()+"\n\n截图记录在「帮助 → 在访达中显示截图记录」里，可以发给开发者。", false)
		})
	}
	if !ok && fromApp {
		d.win.Update(d.ShowMain)
	}
}

// shotLogPath is the log of the screenshots, next to the settings.
func shotLogPath() string {
	return filepath.Join(filepath.Dir(configPath()), "screenshot.log")
}

var shotLogMu sync.Mutex

// logShot adds a line to the screenshots' log, which keeps the last
// 200 KB.
func logShot(format string, args ...any) {
	shotLogMu.Lock()
	defer shotLogMu.Unlock()
	p := shotLogPath()
	if info, err := os.Stat(p); err == nil && info.Size() > 200<<10 {
		_ = os.Rename(p, p+".old")
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, args...))
}

// ShotLog is the end of the screenshots' log.
func (d *desktop) ShotLog() string {
	data, err := os.ReadFile(shotLogPath())
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	return strings.Join(lines[max(0, len(lines)-15):], "\n")
}

// shotUnfinished tells whether the last screenshot of the log never ended,
// as when the app quit in the middle of it.
func shotUnfinished(log string) bool {
	lines := strings.Split(strings.TrimSpace(log), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := lines[i]
		switch {
		case strings.Contains(l, "截图开始"):
			return true
		case strings.Contains(l, "截图结束"), strings.Contains(l, "截图失败"), strings.Contains(l, "没有屏幕录制权限"),
			strings.Contains(l, "系统截图"), strings.Contains(l, "上次截图没有完成"):
			return false
		}
	}
	return false
}

// saveShot asks where to save a screenshot and saves it there. Off the
// main thread.
func (d *desktop) saveShot(png []byte) {
	dir, _ := mygo.App.Path(mygo.PathDesktop)
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
		Title:             "保存截图",
		DefaultPath:       filepath.Join(dir, "截图 "+time.Now().Format("2006-01-02 15.04.05")+".png"),
		Filters:           []mygo.FileFilter{{Name: "PNG 图片", Extensions: []string{"png"}}},
		CreateDirectories: true,
	})
	if err != nil || path == "" {
		return
	}
	if err := os.WriteFile(path, png, 0o644); err != nil {
		mygo.Dialog.Error("截图没有保存", err.Error())
		return
	}
	d.Notify("截图已保存", path)
}

func (d *desktop) OpenShotSettings() { screenshot.OpenPermissionSettings() }

// CheckUpdates checks as the user asked: the update window opens at once,
// saying it is checking, and goes on to what it finds. (The updates are
// read from a directory always published, desktop-latest.)
func (d *desktop) CheckUpdates() { updater.CheckForUpdates() }

func (d *desktop) AutoUpdates() bool      { return updater.AutomaticChecks() }
func (d *desktop) SetAutoUpdates(on bool) { updater.SetAutomaticChecks(on) }

func (d *desktop) SetAppIcon(png, tray []byte) {
	_ = mygo.App.Dock.SetIcon(png)
	_ = d.win.SetIcon(png)
	if d.tray != nil {
		_ = d.tray.SetIcon(tray, true)
	}
}

func (d *desktop) SetTray(on bool) {
	if !on {
		if d.tray != nil {
			d.tray.Destroy()
			d.tray = nil
		}
		return
	}
	if d.tray == nil {
		t, err := mygo.NewTray(mygo.TrayOptions{Icon: trayIcon(d.app.cfg.Icon), IconIsTemplate: true, ToolTip: "MyMind"})
		if err != nil {
			return
		}
		d.tray = t
	}
	d.UpdateTray()
}

// UpdateTray brings the menu bar icon's count and menu up to date.
func (d *desktop) UpdateTray() {
	if d.tray == nil {
		return
	}
	var items []*mygo.MenuItem
	for _, e := range d.app.trayMenu() {
		if e.sep {
			items = append(items, mygo.Separator())
			continue
		}
		run := e.run
		item := &mygo.MenuItem{Label: e.label, Disabled: run == nil}
		if run != nil {
			item.Click = func(*mygo.MenuItem, *mygo.Window) { d.win.Update(run) }
		}
		items = append(items, item)
	}
	d.tray.SetMenu(mygo.NewMenu(items))
	d.tray.SetTitle(d.app.trayTitle())
}

func (d *desktop) PickFolder(done func(string)) {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:    d.win,
			Title:     "选择存放导图的文件夹",
			Directory: true,
		})
		dir := ""
		if err == nil && len(paths) > 0 {
			dir = paths[0]
		}
		d.win.Update(func() { done(dir) })
	}()
}

func (d *desktop) SaveFile(name, ext string, done func(string)) {
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent:            d.win,
			Title:             "导出",
			DefaultPath:       name,
			Filters:           []mygo.FileFilter{{Name: strings.ToUpper(ext), Extensions: []string{ext}}},
			CreateDirectories: true,
		})
		if err != nil {
			path = ""
		}
		d.win.Update(func() { done(path) })
	}()
}

func (d *desktop) OpenFile(title string, exts []string, done func(string)) {
	go func() {
		opts := mygo.OpenDialogOptions{Parent: d.win, Title: title}
		if len(exts) > 0 {
			opts.Filters = []mygo.FileFilter{{Name: strings.ToUpper(exts[0]), Extensions: exts}}
		}
		paths, err := mygo.Dialog.Open(opts)
		path := ""
		if err == nil && len(paths) > 0 {
			path = paths[0]
		}
		d.win.Update(func() { done(path) })
	}()
}

func (d *desktop) CopyText(s string) { _ = mygo.Clipboard.Write(transfer.TextData(s)) }

func (d *desktop) CopyImage(png []byte) {
	_ = mygo.Clipboard.Write(transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, png))))
}

func (d *desktop) ClipboardText() string {
	b, _ := mygo.Clipboard.ReadFormat(transfer.Text)
	return string(b)
}

func (d *desktop) OpenURL(url string) { _ = mygo.Shell.OpenExternal(url) }

func (d *desktop) Notify(title, body string) {
	n := mygo.NewNotification(mygo.NotificationOptions{Title: title, Body: body, Group: "reminders"})
	go func() { _ = n.Show() }() // the first asks for permission and waits
}
func (d *desktop) OpenPath(path string) { _ = mygo.Shell.OpenPath(path) }

func (d *desktop) ShowInFolder(path string) { mygo.Shell.ShowItemInFolder(path) }

func (d *desktop) Trash(path string) error { return mygo.Shell.TrashItem(path) }

func (d *desktop) ClipboardImage() []byte { return mygo.Clipboard.ReadImage() }

func (d *desktop) ClipboardFiles() []string {
	files, _ := mygo.Clipboard.ReadFiles()
	return files
}

func (d *desktop) PickImage(done func(string)) {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:  d.win,
			Title:   "选择图片",
			Filters: []mygo.FileFilter{{Name: "图片", Extensions: []string{"png", "jpg", "jpeg", "gif", "webp", "bmp"}}},
		})
		path := ""
		if err == nil && len(paths) > 0 {
			path = paths[0]
		}
		d.win.Update(func() { done(path) })
	}()
}

func (d *desktop) SetFullScreen(on bool) {
	if d.win != nil && d.win.IsFullScreen() != on {
		d.win.SetFullScreen(on)
	}
}

// resourcesDir is where the app's resources are.
func resourcesDir() string {
	dir, err := mygo.App.Path(mygo.PathResources)
	if err != nil {
		return ""
	}
	return dir
}

//go:embed mygo.json
var mygoJSON []byte

// version is the app's, from mygo.json, and appName its name.
var version, appName = func() (string, string) {
	var c struct{ Version, Name string }
	_ = json.Unmarshal(mygoJSON, &c)
	return c.Version, c.Name
}()

func main() {
	// "MindMap mcp" serves the maps to AI coding agents on stdin and
	// stdout, without a window (see internal/mcp).
	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		hideFromDock()
		if err := serveMCP(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	// Updates: a window, as Sparkle's, offers a new version once a day and
	// installs it, then restarts the app (mygo.json's updates).
	mygo.Use(updaterui.Plugin)
	screenshot.Log = logShot
	plat := &desktop{}
	cfg := loadConfig()
	if moveOldDir(&cfg) {
		log.Printf("导图文件夹已改名为 %s", cfg.Dir)
	}
	a := newApp(plat, cfg)
	plat.app = a
	if !a.cfg.TourDone {
		a.tourStep = 1 // the first run
	}
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:     "MyMind",
			Width:     1280,
			Height:    800,
			MinWidth:  900,
			MinHeight: 560,
			StateKey:  "main",
			// A sidebar of glass, and the toolbar as the title bar, the
			// window's controls over the sidebar's top.
			TitleBarStyle: mygo.TitleBarHiddenInset,
			Vibrancy:      mygo.VibrancySidebar,
			Content:       ui.View(a.view),
		})
		plat.win = win
		if err := plat.SetCaptureKey(a.captureKey()); err != nil {
			log.Printf("快速记录快捷键 %s: %v", a.captureKey(), err)
			logShot("注册快速记录快捷键 %s 失败：%v", a.captureKey(), err)
		} else {
			logShot("已注册快速记录快捷键 %s", a.captureKey())
		}
		if err := plat.SetClipKey(a.clipKey()); err != nil {
			log.Printf("收藏剪贴板快捷键 %s: %v", a.clipKey(), err)
			logShot("注册收藏剪贴板快捷键 %s 失败：%v", a.clipKey(), err)
		} else {
			logShot("已注册收藏剪贴板快捷键 %s", a.clipKey())
		}
		// The last screenshot never ended: the app quit in it, most
		// likely. The system's tool is offered.
		if log := plat.ShotLog(); shotUnfinished(log) {
			logShot("上次截图没有完成")
			a.showShotAlert("上次截图没有完成",
				"截图时 MyMind 可能意外退出了。截图记录在「帮助 → 在访达中显示截图记录」里，可以发给开发者。", false)
		}
		if err := plat.SetShotKey(a.shotKey()); err != nil {
			log.Printf("截图快捷键 %s: %v", a.shotKey(), err)
			logShot("注册截图快捷键 %s 失败：%v", a.shotKey(), err)
		} else {
			logShot("已注册截图快捷键 %s", a.shotKey())
		}
		plat.SetTray(!a.cfg.NoTray)
		if a.cfg.Icon != "" {
			plat.SetAppIcon(appIconPNG(a.cfg.Icon), trayIcon(a.cfg.Icon))
		}
		a.post = func(fn func()) { win.Update(fn) }
		watchPinch(func(m float64) { a.post(func() { a.pinch(float32(m)) }) })
		a.async = func(work func() (any, error), done func(any, error)) {
			go func() {
				v, err := work()
				win.Update(func() { done(v, err) })
			}()
		}
		item := func(label, accel string, fn func()) *mygo.MenuItem {
			return &mygo.MenuItem{Label: label, Accelerator: accel, Click: func(*mygo.MenuItem, *mygo.Window) {
				win.Update(fn)
			}}
		}
		mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
			{Label: "MyMind", Submenu: []*mygo.MenuItem{
				{Role: mygo.RoleAbout},
				item("检查更新…", "", plat.CheckUpdates),
				mygo.Separator(),
				item("设置…", "CmdOrCtrl+,", a.openSettings),
				mygo.Separator(),
				{Role: mygo.RoleHide},
				{Role: mygo.RoleHideOthers},
				{Role: mygo.RoleUnhide},
				mygo.Separator(),
				{Role: mygo.RoleQuit},
			}},
			{Label: "文件", Submenu: []*mygo.MenuItem{
				item("新建导图", "CmdOrCtrl+N", a.newMap),
				item("从模板新建…", "CmdOrCtrl+Shift+N", func() { a.templatesOpen = true }),
				item("打开文件夹…", "CmdOrCtrl+O", a.pickWorkspace),
				item("截图…", "", a.screenshot),
				item("保存", "CmdOrCtrl+S", a.save),
				item("历史版本…", "CmdOrCtrl+Shift+H", a.openHistory),
				mygo.Separator(),
				item("快速记录…", "", plat.ShowCapture),
				item("打开收件箱", "", func() { _ = a.open(a.inboxPath()) }),
				item("打开资料库", "CmdOrCtrl+Shift+L", a.showLibrary),
				item("收藏剪贴板到资料库", "", a.clipFromShortcut),
				mygo.Separator(),
				item("导入 XMind…", "CmdOrCtrl+Shift+I", a.askImportXMind),
				item("导入 Markdown / OPML…", "", a.askImportOutline),
				mygo.Separator(),
				item("导出为图片…", "CmdOrCtrl+Shift+E", a.askExportImage),
				item("导出为 Markdown…", "", a.askExportMarkdown),
				item("导出为 XMind…", "", a.askExportXMind),
				item("导出为 OPML…", "", a.askExportOPML),
				item("导出任务到日历（.ics）…", "", a.askExportICS),
				item("导出演说为 PDF…", "", a.askExportSlides),
				item("复制当前画布为 Markdown", "CmdOrCtrl+Shift+C", a.copyMarkdown),
				mygo.Separator(),
				{Role: mygo.RoleClose},
			}},
			{Role: mygo.RoleEditMenu},
			{Label: "前往", Submenu: []*mygo.MenuItem{
				item("命令面板…", "CmdOrCtrl+K", func() { a.openPalette(paletteAll) }),
				item("快速切换导图…", "CmdOrCtrl+P", func() { a.openPalette(paletteMaps) }),
				item("搜索所有导图…", "CmdOrCtrl+Shift+F", func() { a.openPalette(paletteSearch) }),
				mygo.Separator(),
				item("聚焦所选分支", "Alt+Enter", func() { a.focusOn(a.selected) }),
				item("退出聚焦", "", func() { a.focusOn("") }),
				mygo.Separator(),
				item("全部折叠", "", func() { a.foldAll() }),
				item("全部展开", "Alt+0", func() { a.expandTo(0) }),
				item("展开到第 1 层", "Alt+1", func() { a.expandTo(1) }),
				item("展开到第 2 层", "Alt+2", func() { a.expandTo(2) }),
				item("展开到第 3 层", "Alt+3", func() { a.expandTo(3) }),
			}},
			{Label: "视图", Submenu: []*mygo.MenuItem{
				item("思维导图", "CmdOrCtrl+1", func() { a.mode = viewMap }),
				item("大纲", "CmdOrCtrl+2", func() { a.mode = viewOutline }),
				item("看板", "CmdOrCtrl+3", func() { a.mode = viewBoard }),
				item("日程", "CmdOrCtrl+4", func() { a.mode = viewAgenda }),
				item("时间线", "CmdOrCtrl+5", func() { a.mode = viewTimeline }),
				item("总览", "CmdOrCtrl+6", func() { a.mode = viewOverview }),
				item("表格", "CmdOrCtrl+7", func() { a.mode = viewTable }),
				mygo.Separator(),
				item("显示/隐藏右侧面板", "CmdOrCtrl+Alt+I", func() { a.panelOff = !a.panelOff }),
				mygo.Separator(),
				item("放大", "CmdOrCtrl+=", func() { a.zoomStep(true) }),
				item("缩小", "CmdOrCtrl+-", func() { a.zoomStep(false) }),
				item("实际大小", "CmdOrCtrl+0", func() { a.setZoom(1) }),
				item("回到中心", "CmdOrCtrl+Shift+0", a.recenter),
				mygo.Separator(),
				item("开始演说", "CmdOrCtrl+Enter", func() { a.startPresenting("") }),
				item("新建画布", "CmdOrCtrl+T", func() {
					if a.file != nil {
						a.mode = viewMap
						a.addSheet()
					}
				}),
				mygo.Separator(),
				{Role: mygo.RoleToggleFullScreen},
			}},
			{Role: mygo.RoleWindowMenu},
			{Label: "帮助", Submenu: []*mygo.MenuItem{
				item("快捷键", "CmdOrCtrl+Shift+/", func() { a.cheatsOpen = true }),
				item("连接 AI 编程工具（MCP）…", "", func() { a.mcpOpen = true }),
				mygo.Separator(),
				item("在访达中显示截图记录", "", func() { plat.ShowInFolder(shotLogPath()) }),
			}},
		}))
	})
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { a.flush(); a.autoStopAll() })
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
	a.flush()
}
