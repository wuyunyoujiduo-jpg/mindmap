package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// Attachments: any file on a node, copied into the map's assets folder,
// so that the map carries it. A file dropped on a node attaches; a
// picture becomes the node's picture, unless it has one.

// attachFile copies a file into the assets folder and attaches it.
func (a *app) attachFile(n *model.Node, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dir := model.AssetsDir(a.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := filepath.Base(path)
	file := model.NewID() + strings.ToLower(filepath.Ext(name))
	if err := os.WriteFile(filepath.Join(dir, file), data, 0o644); err != nil {
		return err
	}
	a.mutate("", func() { n.Files = append(slices.Clone(n.Files), model.Attachment{File: file, Name: name}) })
	return nil
}

// dropFiles attaches files dropped on a node.
func (a *app) dropFiles(n *model.Node, files []string) {
	a.selected = n.ID
	for _, f := range files {
		if info, err := os.Stat(f); err != nil || info.IsDir() {
			continue
		}
		var err error
		if isImageFile(f) && n.Image == nil {
			err = a.attachImageFile(n, f)
		} else {
			err = a.attachFile(n, f)
		}
		if err != nil {
			a.saveErr = err.Error()
			return
		}
	}
}

func (a *app) removeFile(n *model.Node, file string) {
	a.mutate("", func() {
		n.Files = slices.DeleteFunc(slices.Clone(n.Files), func(f model.Attachment) bool { return f.File == file })
		if len(n.Files) == 0 {
			n.Files = nil
		}
	})
}

func (a *app) openAttachment(f model.Attachment) {
	path := filepath.Join(model.AssetsDir(a.path), f.File)
	if _, err := os.Stat(path); err != nil {
		a.saveErr = "附件文件不见了：" + f.Name
		return
	}
	a.plat.OpenPath(path)
}

// filesSection is the attachments part of the node panel.
func (a *app) filesSection(c *ui.Context, n *model.Node) {
	t := c.Theme()
	ui.Field(c, "附件", func() {
		ui.Column(c).Gap(6).Children(func() {
			for _, f := range n.Files {
				ui.Row(c.Key("file:"+f.File)).Gap(6).AlignItems(ui.Center).Padding(3, 6).Radius(6).
					Background(t.Surface).Border(1, t.Border).Children(func() {
					open := ui.Row(c).Gap(6).Grow(1).Basis(0).MinWidth(0).Cursor(ui.CursorPointer).Label("打开 " + f.Name).Tooltip("用默认程序打开")
					if open.Clicked() {
						f := f
						a.later(func() { a.openAttachment(f) })
					}
					open.Children(func() {
						ui.Text(c, "📎").FontSize(12)
						ui.Text(c, f.Name).FontSize(12).TextColor(t.Accent).MaxLines(1)
					})
					if iconButton(c, iconClose, "删除附件", "删除附件", 22, true).Clicked() {
						file := f.File
						a.later(func() { a.removeFile(n, file) })
					}
				})
			}
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				if ui.Button(c, "添加附件…").FontSize(12).Clicked() {
					id := n.ID
					a.plat.OpenFile("选择要附加的文件", nil, func(path string) {
						if node, _ := a.doc.Find(id); node != nil && path != "" {
							if err := a.attachFile(node, path); err != nil {
								a.saveErr = err.Error()
							}
						}
					})
				}
				if len(n.Files) == 0 {
					ui.Text(c, "或把文件拖到节点上").FontSize(11).TextColor(t.TextMuted)
				}
			})
		})
	})
}

// filesMark is the attachments' mark on a node: a clip and how many.
func filesMark(n *model.Node) string {
	if len(n.Files) == 0 {
		return ""
	}
	return fmt.Sprintf("📎%d", len(n.Files))
}
