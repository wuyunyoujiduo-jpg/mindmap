package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// A node can show a picture: pasted, dropped from Finder or picked in a
// dialog. Its file goes into the map's assets folder; the node shows it
// small above its title, and a click shows it large.

// Sizes of a node's picture at zoom 1: it fits in thumbMaxW×thumbMaxH.
const (
	thumbMaxW = 220
	thumbMaxH = 160
	thumbGap  = 6
)

// imageExts are the extensions of the pictures a node takes.
var imageExts = []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp"}

func isImageFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, e := range imageExts {
		if ext == e {
			return true
		}
	}
	return false
}

// thumbSize is the size a picture shows at in its node, at zoom 1.
func thumbSize(img *model.Image) (float32, float32) {
	if img == nil || img.W <= 0 || img.H <= 0 {
		return 0, 0
	}
	w, h := float32(img.W), float32(img.H)
	k := min(thumbMaxW/w, thumbMaxH/h, 1)
	return max(w*k, 24), max(h*k, 24)
}

// imagePath is where the file of a node's picture is.
func (a *app) imagePath(img *model.Image) string {
	return filepath.Join(model.AssetsDir(a.path), img.File)
}

// bitmap returns a node's picture, decoded once.
func (a *app) bitmap(img *model.Image) *ui.Bitmap {
	path := a.imagePath(img)
	if b, ok := a.bitmaps[path]; ok {
		return b
	}
	var b *ui.Bitmap
	if data, err := os.ReadFile(path); err == nil {
		b, _ = ui.DecodeBitmap(data)
	}
	a.bitmaps[path] = b // nil, for a file missing or broken, is kept too
	return b
}

// attachImage gives a node a picture of data, whose extension is ext,
// writing it into the map's assets folder.
func (a *app) attachImage(n *model.Node, data []byte, ext string) error {
	if n == nil || a.path == "" {
		return errors.New("没有选中的节点")
	}
	b, err := ui.DecodeBitmap(data)
	if err != nil {
		return errors.New("无法识别的图片格式")
	}
	dir := model.AssetsDir(a.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := model.NewID() + strings.ToLower(ext)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	a.bitmaps[path] = b
	w, h := b.Size()
	a.mutate("", func() { n.Image = &model.Image{File: name, W: w, H: h} })
	a.reveal = n.ID == a.selected // the node grew: keep it in view
	return nil
}

// attachImageFile gives a node the picture in a file.
func (a *app) attachImageFile(n *model.Node, path string) error {
	if !isImageFile(path) {
		return errors.New("只支持 PNG、JPEG、GIF、WebP、BMP 图片")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return a.attachImage(n, data, filepath.Ext(path))
}

// pasteImage gives the selected node the picture on the clipboard, or of
// the first picture file copied in Finder. It reports whether there was
// one.
func (a *app) pasteImage() bool {
	n, _ := a.selectedNode()
	if n == nil {
		return false
	}
	var err error
	if data := a.plat.ClipboardImage(); len(data) > 0 {
		err = a.attachImage(n, data, ".png")
	} else {
		found := false
		for _, f := range a.plat.ClipboardFiles() {
			if isImageFile(f) {
				err, found = a.attachImageFile(n, f), true
				break
			}
		}
		if !found {
			return false
		}
	}
	if err != nil {
		a.saveErr = err.Error()
	}
	return true
}

func (a *app) removeImage(n *model.Node) {
	a.mutate("", func() { n.Image = nil })
}

// thumbnail shows a node's picture at w×h; a click shows it large.
func (a *app) thumbnail(c *ui.Context, n *model.Node, w, h float32) ui.Element {
	t := c.Theme()
	b := a.bitmap(n.Image)
	if b == nil {
		return ui.Box(c.Key("image")).Size(w, h).Radius(6).Background(t.Border).Center().Children(func() {
			ui.Text(c, "图片缺失").FontSize(min(12, h/3)).TextColor(t.TextMuted)
		})
	}
	img := ui.Image(c.Key("image"), b).Size(w, h).Fit(ui.Contain).Radius(6).Cursor(ui.CursorPointer).
		Label("图片：" + n.Title).Tooltip("点击查看大图")
	if img.Clicked() {
		a.selected = n.ID
		a.lightbox = n.ID
	}
	return img
}

// lightboxView shows the picture of the node a.lightbox names over the
// window, as large as fits; a click or Escape closes it.
func (a *app) lightboxView(c *ui.Context) {
	n, _ := a.doc.Find(a.lightbox)
	if n == nil || n.Image == nil {
		a.lightbox = ""
		return
	}
	w, h := c.Size()
	iw, ih := float32(n.Image.W), float32(n.Image.H)
	k := min(w*0.9/iw, (h*0.9-40)/ih, 2)
	overlay := ui.Box(c.Key("lightbox")).Absolute().Top(0).Left(0).Right(0).Bottom(0).
		Background(ui.RGBA(0, 0, 0, 0.85)).Center().Gap(12).Focusable().FocusRing(false).AutoFocus().
		Label("大图")
	if overlay.Clicked() || overlay.Shortcut(0, ui.KeyEscape) || overlay.OverlayShortcut(0, ui.KeyEscape) {
		a.lightbox = ""
	}
	overlay.Children(func() {
		if b := a.bitmap(n.Image); b != nil {
			ui.Image(c, b).Size(iw*k, ih*k).Fit(ui.Contain).Radius(8).PassThrough()
		}
		ui.Textf(c, "%s · %d×%d · 点击或按 Esc 关闭", n.Title, n.Image.W, n.Image.H).FontSize(12).
			TextColor(ui.RGBA(255, 255, 255, 0.8)).PassThrough()
	})
}

// imageSection is the picture part of the node panel.
func (a *app) imageSection(c *ui.Context, n *model.Node) {
	t := c.Theme()
	ui.Field(c, "图片", func() {
		ui.Column(c).Gap(8).Children(func() {
			if n.Image != nil {
				w, h := thumbSize(n.Image)
				k := min(1, (a.panelWidth-32)/w)
				a.thumbnail(c.Key("panel"), n, w*k, h*k)
			}
			ui.Row(c).Gap(8).Children(func() {
				label := "添加图片…"
				if n.Image != nil {
					label = "替换…"
				}
				if ui.Button(c, label).Clicked() {
					id := n.ID
					a.plat.PickImage(func(path string) {
						if path == "" {
							return
						}
						if node, _ := a.doc.Find(id); node != nil {
							if err := a.attachImageFile(node, path); err != nil {
								a.saveErr = err.Error()
							}
						}
					})
				}
				if n.Image != nil && ui.Button(c, "删除图片").TextColor(t.Danger).Clicked() {
					a.removeImage(n)
				}
			})
			if n.Image == nil {
				ui.Text(c, "也可以选中节点后按 ⌘V 粘贴图片，或把图片文件拖到节点上。").FontSize(12).TextColor(t.TextMuted).Wrap()
			}
		})
	})
}
