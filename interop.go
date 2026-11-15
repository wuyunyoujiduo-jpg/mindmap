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

// askImportFolder asks for a folder of documents and makes one map of
// them, a canvas a document: a project's rules or designs, kept as files,
// are then one map to turn the pages of, and a noun is looked for on all
// of them.
func (a *app) askImportFolder() {
	a.plat.PickFolder(func(dir string) {
		if dir != "" {
			a.importFolder(dir)
		}
	})
}

// maxImportDocs is how many documents of a folder become canvases.
const maxImportDocs = 24

func (a *app) importFolder(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		a.saveErr = "导入失败: " + err.Error()
		return
	}
	f := &model.File{Version: model.FormatVersion}
	more := 0
	for _, e := range entries {
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if e.IsDir() || ext != ".md" && ext != ".markdown" {
			continue
		}
		if len(f.Sheets) == maxImportDocs {
			more++
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil || strings.TrimSpace(string(data)) == "" {
			continue
		}
		name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		d := formats.ParseMarkdown(string(data), name).Sheets[0]
		d.Title = name
		if t := strings.TrimSpace(d.Root.Title); t != "" && len([]rune(t)) <= 16 {
			d.Title = t // named as the document names itself, when shortly
		}
		f.Sheets = append(f.Sheets, d)
	}
	if len(f.Sheets) == 0 {
		a.tell("这个文件夹里没有 Markdown 文档")
		return
	}
	dest := freeMapPath(a.currentFolder(), filepath.Base(dir))
	if err := f.Save(dest); err != nil {
		a.saveErr = "导入失败: " + err.Error()
		return
	}
	a.refreshFiles()
	_ = a.open(dest)
	msg := fmt.Sprintf("导入了 %d 篇文档，每篇一个画布", len(f.Sheets))
	if more > 0 {
		msg += fmt.Sprintf("；还有 %d 篇没导入", more)
	}
	a.tell(msg)
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
