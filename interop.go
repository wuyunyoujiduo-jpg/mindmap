package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/formats"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// Other formats in and out: Markdown and OPML outlines, a calendar of the
// tasks, the slides as a PDF.

// askImportOutline asks for a Markdown or OPML file and makes a map of it.
func (a *app) askImportOutline() {
	a.plat.OpenFile("导入 Markdown 或 OPML", []string{"md", "markdown", "txt", "opml"}, func(path string) {
		if path != "" {
			a.importOutline(path)
		}
	})
}

func (a *app) importOutline(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		a.saveErr = "导入失败: " + err.Error()
		return
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	var f *model.File
	if strings.EqualFold(filepath.Ext(path), ".opml") {
		f, err = formats.ParseOPML(data, name)
		if err != nil {
			a.saveErr = "导入失败: " + err.Error()
			return
		}
	} else {
		f = formats.ParseMarkdown(string(data), name)
	}
	dest := freeMapPath(a.currentFolder(), name)
	if err := f.Save(dest); err != nil {
		a.saveErr = "导入失败: " + err.Error()
		return
	}
	a.refreshFiles()
	_ = a.open(dest)
}

func (a *app) askExportOPML() {
	a.exportTo("opml", func(path string) error {
		data, err := formats.OPML(a.file, mapName(a.path))
		if err != nil {
			return err
		}
		return os.WriteFile(path, data, 0o644)
	})
}

// askExportICS saves the tasks with dates as a calendar, which Calendar
// and other apps import.
func (a *app) askExportICS() {
	a.exportTo("ics", func(path string) error {
		return os.WriteFile(path, formats.ICS(a.file, mapName(a.path), a.now()), 0o644)
	})
}

// askExportSlides saves the slides of the canvas shown as a PDF, each
// slide with all it shows.
func (a *app) askExportSlides() {
	a.exportTo("pdf", func(path string) error {
		var buf bytes.Buffer
		if err := a.slidesPDF(&buf); err != nil {
			return err
		}
		return os.WriteFile(path, buf.Bytes(), 0o644)
	})
}

func (a *app) slidesPDF(buf *bytes.Buffer) (err error) {
	slides := slidesOf(a.doc)
	if len(slides) == 0 {
		return errors.New("这个画布没有可生成的幻灯片")
	}
	w, h := float32(1280), float32(720)
	switch a.doc.Present.Aspect {
	case "4:3":
		w, h = 1024, 768
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	var pages []image.Image
	for i, s := range slides {
		s := s
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Box(c).Fill().Children(func() {
				a.slideView(c, a.doc, s, i, len(slides), w, h, -1) // all of it shown
			})
		}, int(w), int(h))
		tt.SetScale(2)
		tt.Frame()
		pages = append(pages, tt.Image())
	}
	return formats.PDF(buf, pages, float64(w)*0.75) // points: 3/4 of a pixel
}
