package main

import (
	"bytes"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
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
