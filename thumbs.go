package main

import (
	"bytes"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"

	"github.com/egoist/mygo/ui"
	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// The pictures of 收集 are read away from the window's own work: a wall of
// screenshots, each decoded whole as its card was first drawn, held the
// window still for as long as that took, the first time 收集 was opened
// (and kept every one of them whole in memory). A card has its picture
// small, as large as it shows; the clip chosen has it whole; until either
// is read its place is a pale block.

// thumbWide and thumbTall are what a card's picture is read down to: its
// room on the wall, twice over for a dense screen.
const (
	thumbWide = 640
	thumbTall = 240
)

// picturesAtOnce is how many pictures are read at a time.
const picturesAtOnce = 3

// filePicture is the picture of a file: small for a card of the wall, or
// whole. It is nil while being read (loading) and for a file that is no
// picture.
func (a *app) filePicture(path string, small bool) (b *ui.Bitmap, loading bool) {
	if path == "" {
		return nil, false
	}
	key := path
	if small {
		key = "thumb:" + path
	}
	if b, ok := a.bitmaps[key]; ok {
		return b, false
	}
	if a.pictureBusy[key] {
		return nil, true
	}
	if len(a.pictureBusy) >= picturesAtOnce {
		return nil, true // its turn comes when one of those is read
	}
	if a.pictureBusy == nil {
		a.pictureBusy = map[string]bool{}
	}
	a.pictureBusy[key] = true
	a.async(func() (any, error) {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return data, nil // a kind only MyGo reads: decoded in the window's turn
		}
		if small {
			img = shrunk(img, thumbWide, thumbTall)
		}
		return img, nil
	}, func(v any, err error) {
		delete(a.pictureBusy, key)
		var b *ui.Bitmap
		switch v := v.(type) {
		case image.Image:
			b = ui.NewBitmap(v)
		case []byte:
			b, _ = ui.DecodeBitmap(v)
		}
		a.bitmaps[key] = b // nil, for a file missing or broken, is kept too
	})
	b, ok := a.bitmaps[key]
	return b, !ok
}

// shrunk is img no larger than covers a box of w by h; itself when it is
// smaller already.
func shrunk(img image.Image, w, h int) image.Image {
	r := img.Bounds()
	if r.Dx() <= 0 || r.Dy() <= 0 {
		return img
	}
	k := max(float64(w)/float64(r.Dx()), float64(h)/float64(r.Dy()))
	if k >= 1 {
		return img
	}
	out := image.NewRGBA(image.Rect(0, 0, max(int(float64(r.Dx())*k), 1), max(int(float64(r.Dy())*k), 1)))
	draw.ApproxBiLinear.Scale(out, out.Bounds(), img, r, draw.Src, nil)
	return out
}
