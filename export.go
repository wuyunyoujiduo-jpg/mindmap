package main

import (
	"bytes"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/xmind"
)

// The canvas shown exports as a PNG picture of the whole map, drawn off
// screen as the canvas draws it, without its grid and controls, at twice
// its size for sharp text on Retina screens.

const exportScale = 2

// askExportImage asks where to save the picture of the canvas shown.
func (a *app) askExportImage() {
	if a.doc == nil {
		return
	}
	a.commitEditIfAny()
	name := mapName(a.path)
	if len(a.file.Sheets) > 1 {
		name += " - " + a.sheetName(a.doc.ID)
	}
	a.plat.SaveFile(name+".png", "png", func(path string) {
		if path == "" {
			return
		}
		if !strings.EqualFold(filepath.Ext(path), ".png") {
			path += ".png"
		}
		data, err := a.exportImage()
		if err == nil {
			err = os.WriteFile(path, data, 0o644)
		}
		if err != nil {
			a.saveErr = "导出失败: " + err.Error()
			return
		}
		a.plat.ShowInFolder(path)
	})
}

// exportImage draws the canvas shown into a PNG.
func (a *app) exportImage() (data []byte, err error) {
	if a.doc == nil {
		return nil, fmt.Errorf("没有打开的导图")
	}
	// What only the screen shows is put aside while drawing.
	selected, boxes, vp, reveal := a.selected, a.boxes, a.vp, a.reveal
	tagFilter, search := a.tagFilter, a.search
	a.selected, a.tagFilter, a.search = "", "", ""
	a.exportCam = &camera{Zoom: 1, placed: true}
	defer func() {
		a.selected, a.boxes, a.vp, a.reveal = selected, boxes, vp, reveal
		a.tagFilter, a.search = tagFilter, search
		a.exportCam = nil
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	tt := ui.NewTester(func(c *ui.Context) { a.canvasArea(c) }, 200, 200)
	w, h := a.boxes.Width, a.boxes.Height // laid out in the first frame
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("导图是空的")
	}
	tt.SetSize(int(w+0.5), int(h+0.5))
	tt.SetScale(exportScale)
	tt.Frame()
	var buf bytes.Buffer
	if err := png.Encode(&buf, tt.Image()); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// exportMenu is the menu of the canvas's export button.
func (a *app) exportMenu(m *ui.Menu) {
	if m.Item("导出为图片…").Shortcut(ui.Cmd|ui.Shift, ui.KeyE).Chosen() {
		a.later(a.askExportImage)
	}
	if m.Item("导出为 Markdown…").Chosen() {
		a.later(a.askExportMarkdown)
	}
	if m.Item("导出为 XMind…").Chosen() {
		a.later(a.askExportXMind)
	}
	m.Separator()
	if m.Item("复制当前画布为 Markdown").Shortcut(ui.Cmd|ui.Shift, ui.KeyC).Chosen() {
		a.later(a.copyMarkdown)
	}
}

// exportTo asks where to save the open map as a file of the extension,
// and writes it there with write.
func (a *app) exportTo(ext string, write func(path string) error) {
	if a.file == nil {
		return
	}
	a.commitEditIfAny()
	a.flush()
	a.plat.SaveFile(mapName(a.path)+"."+ext, ext, func(path string) {
		if path == "" {
			return
		}
		if !strings.EqualFold(filepath.Ext(path), "."+ext) {
			path += "." + ext
		}
		if err := write(path); err != nil {
			a.saveErr = "导出失败: " + err.Error()
			return
		}
		a.plat.ShowInFolder(path)
	})
}

// askExportMarkdown saves every canvas of the map as a Markdown outline.
func (a *app) askExportMarkdown() {
	a.exportTo("md", func(path string) error {
		return os.WriteFile(path, []byte(model.MarkdownFile(a.file)), 0o644)
	})
}

// copyMarkdown puts the canvas shown on the clipboard as Markdown, to
// paste into a document or a chat with an AI.
func (a *app) copyMarkdown() {
	if a.doc == nil {
		return
	}
	a.commitEditIfAny()
	a.plat.CopyText(model.Markdown(a.doc))
	a.tell("已复制为 Markdown，可以粘贴给 AI 或文档")
}

// askExportXMind saves the map as an .xmind file, which XMind opens.
func (a *app) askExportXMind() {
	a.exportTo("xmind", func(path string) error {
		var buf bytes.Buffer
		if err := xmind.Export(&buf, a.file, model.AssetsDir(a.path)); err != nil {
			return err
		}
		return os.WriteFile(path, buf.Bytes(), 0o644)
	})
}

// askImportXMind asks for an .xmind file and imports it.
func (a *app) askImportXMind() {
	a.plat.OpenFile("导入 XMind 文件", []string{"xmind"}, func(path string) {
		if path != "" {
			a.importXMind(path)
		}
	})
}

// importXMind makes a map of an .xmind file, in the current folder,
// named after it, and opens it.
func (a *app) importXMind(path string) {
	fail := func(err error) { a.saveErr = "导入失败: " + err.Error() }
	src, err := os.Open(path)
	if err != nil {
		fail(err)
		return
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		fail(err)
		return
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	dest := freeMapPath(a.currentFolder(), name)
	f, err := xmind.Import(src, info.Size(), model.AssetsDir(dest))
	if err != nil {
		fail(err)
		return
	}
	if err := f.Save(dest); err != nil {
		fail(err)
		return
	}
	a.refreshFiles()
	_ = a.open(dest)
}
