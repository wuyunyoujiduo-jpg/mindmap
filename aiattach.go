package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	_ "golang.org/x/image/webp"
)

// A message to the AI can carry pictures and documents: picked with the
// paperclip, pasted (a screenshot on the clipboard, files copied in
// Finder) or dropped on the panel. Pictures go to Claude as images; the
// text of documents goes with the message.

// aiAttachMax is how many attachments a message takes.
const aiAttachMax = 10

// aiImageMax is the most a picture sends: larger ones are made JPEG.
const aiImageMax = 3 << 20

// aiAttach is a picture or a document attached to a message.
type aiAttach struct {
	name  string
	mime  string // a picture's type, "" for a document
	data  []byte // a picture's
	text  string // a document's
	thumb *ui.Bitmap
}

func (f aiAttach) isImage() bool { return f.mime != "" }

// aiAttachExts are the files the paperclip offers.
var aiAttachExts = []string{"png", "jpg", "jpeg", "gif", "webp",
	"pdf", "docx", "pptx", "xlsx", "txt", "md", "csv", "json", "yaml", "yml", "html", "xml", "log",
	"go", "py", "js", "ts", "tsx", "jsx", "css", "sql", "sh", "rs", "java", "kt", "swift", "c", "h", "cpp", "toml"}

// readAttach reads a file to attach.
func readAttach(path string) (aiAttach, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return aiAttach{}, err
	}
	name := filepath.Base(path)
	if isImageFile(path) {
		return imageAttach(name, data)
	}
	text := library.ExtractText(path, data)
	if text == "" {
		return aiAttach{}, fmt.Errorf("读不出「%s」的文字", name)
	}
	return aiAttach{name: name, text: text}, nil
}

// imageAttach makes an attachment of a picture, in a type Claude takes.
func imageAttach(name string, data []byte) (aiAttach, error) {
	mime := http.DetectContentType(data)
	switch mime {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
	default:
		mime = ""
	}
	if mime == "" || len(data) > aiImageMax {
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return aiAttach{}, fmt.Errorf("无法识别图片「%s」", name)
		}
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
			return aiAttach{}, err
		}
		if buf.Len() > 5<<20 {
			return aiAttach{}, fmt.Errorf("图片「%s」太大", name)
		}
		data, mime = buf.Bytes(), "image/jpeg"
	}
	f := aiAttach{name: name, mime: mime, data: data}
	f.thumb, _ = ui.DecodeBitmap(data)
	return f, nil
}

// addAttach attaches files to the draft, telling of those it cannot.
func (a *app) addAttach(paths []string) {
	var errs []string
	for _, p := range paths {
		if len(a.ai.attach) >= aiAttachMax {
			errs = append(errs, fmt.Sprintf("一条消息最多 %d 个附件", aiAttachMax))
			break
		}
		f, err := readAttach(p)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		a.ai.attach = append(a.ai.attach, f)
	}
	if len(errs) > 0 {
		a.tell(strings.Join(errs, "；"))
	}
}

// pasteAttach attaches the picture on the clipboard, or the files copied,
// and reports whether there was any.
func (a *app) pasteAttach() bool {
	if data := a.plat.ClipboardImage(); len(data) > 0 {
		if len(a.ai.attach) >= aiAttachMax {
			a.tell(fmt.Sprintf("一条消息最多 %d 个附件", aiAttachMax))
			return true
		}
		f, err := imageAttach("截图.png", data)
		if err != nil {
			a.tell(err.Error())
		} else {
			a.ai.attach = append(a.ai.attach, f)
		}
		return true
	}
	if files := a.plat.ClipboardFiles(); len(files) > 0 {
		a.addAttach(files)
		return true
	}
	return false
}

// attachText is the documents' text, sent with the message.
func attachText(files []aiAttach) string {
	var b strings.Builder
	for _, f := range files {
		if !f.isImage() {
			fmt.Fprintf(&b, "附件「%s」：\n%s\n\n", f.name, f.text)
		}
	}
	return strings.TrimSpace(b.String())
}

// attachNames lists the attachments, for the conversation kept.
func attachNames(files []aiAttach) []string {
	var names []string
	for _, f := range files {
		names = append(names, f.name)
	}
	return names
}

// claudeImages are the pictures as Claude's blocks.
func claudeImages(files []aiAttach) []anthropic.BetaContentBlockParamUnion {
	var blocks []anthropic.BetaContentBlockParamUnion
	for _, f := range files {
		if f.isImage() {
			blocks = append(blocks, anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{
				Data:      base64.StdEncoding.EncodeToString(f.data),
				MediaType: anthropic.BetaBase64ImageSourceMediaType(f.mime),
			}))
		}
	}
	return blocks
}

var errNoImages = errors.New("DeepSeek 看不了图片：请在 AI 设置中换成 Claude，或去掉图片")

// attachRow shows the attachments: pictures small, documents by name. Each
// is taken off with its ×, when remove is not nil.
func attachRow(c *ui.Context, files []aiAttach, names []string, remove func(i int)) {
	t := c.Theme()
	if len(files) == 0 && len(names) == 0 {
		return
	}
	fg := t.Text
	if remove == nil { // in a message sent, on the accent
		fg = t.AccentText
	}
	ui.Row(c.Key("attach")).Wrap().Gap(6).Children(func() {
		for i, f := range files {
			ui.Row(c.Key(i)).Height(44).Padding(0, 6).Gap(6).Radius(8).AlignItems(ui.Center).
				Background(fg.Alpha(0.1)).Tooltip(f.name).Children(func() {
				if b := f.thumb; b != nil {
					bw, bh := b.Size()
					w, h := float32(bw), float32(bh)
					k := min(36/w, 36/h, 1)
					ui.Image(c, b).Size(max(w*k, 8), max(h*k, 8)).Radius(4)
				} else {
					ui.Box(c).Size(16, 16).Draw(func(p *ui.Painter, r ui.Rect) { drawIcon(p, r, 14, iconFile, fg) })
					ui.Text(c, f.name).FontSize(12).TextColor(fg).NoWrap().MaxWidth(120).Ellipsis("…")
				}
				if remove != nil && iconButton(c, iconClose, "移除附件 "+f.name, "移除", 20, true).Clicked() {
					remove(i)
				}
			})
		}
		for i, name := range names { // of a conversation kept: their names
			ui.Row(c.Key(fmt.Sprint("name", i))).Padding(3, 8).Gap(4).Radius(6).AlignItems(ui.Center).
				Background(t.AccentText.Alpha(0.18)).Children(func() {
				ic := iconFile
				if isImageFile(name) {
					ic = iconImage
				}
				ui.Box(c).Size(12, 12).Draw(func(p *ui.Painter, r ui.Rect) { drawIcon(p, r, 11, ic, t.AccentText) })
				ui.Text(c, name).FontSize(11).TextColor(t.AccentText).NoWrap().MaxWidth(140).Ellipsis("…")
			})
		}
	})
}
