// MindMap is a desktop mind map for managing projects: nodes carry notes,
// tags and tasks with dates, shown as a map, a board and an agenda.
package main

import (
	"log"
	"strings"

	"github.com/egoist/mygo"
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

func main() {
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
				mygo.Separator(),
				item("导出为图片…", "CmdOrCtrl+Shift+E", a.askExportImage),
				mygo.Separator(),
				{Role: mygo.RoleClose},
			}},
			{Role: mygo.RoleEditMenu},
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
		}))
	})
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { a.flush() })
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
	a.flush()
}
