// MindMap is a desktop mind map for managing projects: nodes carry notes,
// tags and tasks with dates, shown as a map, a board and an agenda.
package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

// desktop implements platform with MyGo's dialogs and shell.
type desktop struct{ win *mygo.Window }

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

func (d *desktop) ClipboardText() string {
	b, _ := mygo.Clipboard.ReadFormat(transfer.Text)
	return string(b)
}

func (d *desktop) OpenURL(url string)   { _ = mygo.Shell.OpenExternal(url) }
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

//go:embed mygo.json
var mygoJSON []byte

// version is the app's, from mygo.json.
var version = func() string {
	var c struct{ Version string }
	_ = json.Unmarshal(mygoJSON, &c)
	return c.Version
}()

func main() {
	// "MindMap mcp" serves the maps to AI coding agents on stdin and
	// stdout, without a window (see internal/mcp).
	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		if err := serveMCP(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	plat := &desktop{}
	a := newApp(plat, loadConfig())
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:     "MindMap",
			Width:     1280,
			Height:    800,
			MinWidth:  900,
			MinHeight: 560,
			StateKey:  "main",
			Content:   ui.View(a.view),
		})
		plat.win = win
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
			{Role: mygo.RoleAppMenu},
			{Label: "文件", Submenu: []*mygo.MenuItem{
				item("新建导图", "CmdOrCtrl+N", a.newMap),
				item("打开文件夹…", "CmdOrCtrl+O", a.pickWorkspace),
				item("保存", "CmdOrCtrl+S", a.save),
				item("历史版本…", "CmdOrCtrl+Shift+H", a.openHistory),
				mygo.Separator(),
				item("导入 XMind…", "CmdOrCtrl+Shift+I", a.askImportXMind),
				mygo.Separator(),
				item("导出为图片…", "CmdOrCtrl+Shift+E", a.askExportImage),
				item("导出为 Markdown…", "", a.askExportMarkdown),
				item("导出为 XMind…", "", a.askExportXMind),
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
				mygo.Separator(),
				item("放大", "CmdOrCtrl+=", func() { a.setZoom(a.cam().Zoom * 1.2) }),
				item("缩小", "CmdOrCtrl+-", func() { a.setZoom(a.cam().Zoom / 1.2) }),
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
			}},
		}))
	})
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { a.flush() })
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
	a.flush()
}
