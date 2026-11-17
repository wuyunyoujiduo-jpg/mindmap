package main

import (
	"bytes"
	"encoding/binary"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"slices"
	"strings"
	"time"

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
//
// Three kinds are kept, each under its key in app.bitmaps: a card's
// (cardPicture, "thumb:" and the file), cut to the card's shape; a small
// one of the whole picture (filePicture, small: "small:" and the file),
// for a topic and a note; and the whole (the file). The whole ones are
// kept while there is room (wholeRoom), those shown last: every one
// looked at was kept till the app was left, twenty megabytes a
// screenshot.

// thumbWide and thumbTall are what a card's picture is read down to: its
// room on the wall, twice over for a dense screen.
const (
	thumbWide = 640
	thumbTall = 240
)

// smallMost is the most a side of a small picture is: of a screenshot of
// a long page, 1170 by 20000, the small one was 640 by 10940, 28 MB.
const smallMost = 3000

// picturesAtOnce is how many pictures are read at a time.
const picturesAtOnce = 3

// wholeRoom is how much the whole pictures kept may take, in bytes, and
// wholeLeast how many are kept whatever they take: a clip's in its
// details and one shown large. Those the frame shown asks for are kept
// too, however many they are (keepPicture).
var wholeRoom = 128 << 20

const wholeLeast = 3

// pictureRetry is how long a picture that could not be read is taken for
// missing before it is read again: a file the sync has not brought down
// yet was missing till the app was left.
const pictureRetry = 5 * time.Second

// pictures is what is kept of the pictures read, beside the pictures
// (app.bitmaps): when each whole one was last shown, and which files
// could not be read, and when.
type pictures struct {
	tick   int
	shown  map[string]int       // a whole picture's key → the tick it was last asked for at
	frame  map[string]int       // and the frame (app.vidFrame) it was last asked for in
	failed map[string]time.Time // a key → when its file could not be read
}

// The kinds of a picture read.
const (
	pictureWhole = iota
	pictureSmall
	pictureCard
)

func pictureKey(path string, kind int) string {
	switch kind {
	case pictureSmall:
		return "small:" + path
	case pictureCard:
		return "thumb:" + path
	}
	return path
}

// wholeKey tells whether a key of app.bitmaps is a whole picture's.
func wholeKey(key string) bool {
	return !strings.HasPrefix(key, "small:") && !strings.HasPrefix(key, "thumb:")
}

// filePicture is the picture of a file: small, for a topic or a note, or
// whole. It is nil while being read (loading), for a file that is no
// picture, and for one that is not there, which is looked for again
// after a while.
func (a *app) filePicture(path string, small bool) (b *ui.Bitmap, loading bool) {
	if small {
		return a.picture(path, pictureSmall)
	}
	return a.picture(path, pictureWhole)
}

// cardPicture is the picture of a file as a card of the wall shows it:
// cut to the card's shape and small.
func (a *app) cardPicture(path string) (b *ui.Bitmap, loading bool) {
	return a.picture(path, pictureCard)
}

func (a *app) picture(path string, kind int) (b *ui.Bitmap, loading bool) {
	if path == "" {
		return nil, false
	}
	key := pictureKey(path, kind)
	p := &a.collect.pics
	if kind == pictureWhole { // asked for in this frame: not to make room for another of it
		if p.frame == nil {
			p.frame = map[string]int{}
		}
		p.frame[key] = a.vidFrame
	}
	if b, ok := a.bitmaps[key]; ok {
		if kind == pictureWhole && b != nil {
			p.touch(key)
		}
		return b, false
	}
	if a.pictureBusy[key] {
		return nil, true
	}
	if at, ok := p.failed[key]; ok {
		if a.now().Sub(at) < pictureRetry {
			return nil, false // not there: looked for again in a while
		}
		delete(p.failed, key)
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
		// Upright, as its camera says it is to show: a photo taken with
		// the phone held up showed on its side, MyGo's own reading
		// turning it and this one not.
		return upright(img, exifOrientation(data), kind), nil
	}, func(v any, err error) {
		delete(a.pictureBusy, key)
		if err != nil {
			// Not read: not there yet, perhaps. Not kept as missing for
			// good, as a picture that is broken is.
			if p.failed == nil {
				p.failed = map[string]time.Time{}
			}
			p.failed[key] = a.now()
			return
		}
		var b *ui.Bitmap
		switch v := v.(type) {
		case image.Image:
			b = ui.NewBitmap(v)
		case []byte:
			b, _ = ui.DecodeBitmap(v)
		}
		a.keepPicture(key, b) // nil, for a file that is no picture, is kept too
	})
	b, ok := a.bitmaps[key]
	if _, failed := p.failed[key]; failed {
		return nil, false
	}
	return b, !ok
}

func (p *pictures) touch(key string) {
	if p.shown == nil {
		p.shown = map[string]int{}
	}
	p.tick++
	p.shown[key] = p.tick
}

// keepPicture keeps a picture read under its key. A whole one takes the
// room of those shown longest ago, once the whole ones take more than
// wholeRoom; the few shown last stay whatever they take, and so does
// every one the frame shown asked for: the room is at least what one
// frame shows. Four photos of twelve megapixels on a slide took more than
// it, and one of them, whichever was read last, put out another that the
// same frame showed, which was read again and put out the next, at every
// frame for as long as the slide showed.
func (a *app) keepPicture(key string, b *ui.Bitmap) {
	a.bitmaps[key] = b
	if b == nil || !wholeKey(key) {
		return
	}
	p := &a.collect.pics
	p.touch(key)
	type whole struct {
		key         string
		shown, size int
	}
	var all []whole
	total, inFrame := 0, 0
	for k, b := range a.bitmaps {
		if b == nil || !wholeKey(k) {
			continue
		}
		w, h := b.Size()
		total += 4 * w * h
		if at, asked := p.frame[k]; asked && at == a.vidFrame {
			inFrame++ // of the frame shown
			continue
		}
		all = append(all, whole{k, p.shown[k], 4 * w * h})
	}
	slices.SortFunc(all, func(x, y whole) int { return x.shown - y.shown })
	for i := 0; i < len(all)-max(0, wholeLeast-inFrame) && total > wholeRoom; i++ {
		delete(a.bitmaps, all[i].key)
		delete(p.shown, all[i].key)
		delete(p.frame, all[i].key)
		total -= all[i].size
	}
}

// forgetPicture drops what was read of a file, and that it could not be:
// it is read again when next shown.
func (a *app) forgetPicture(path string) {
	p := &a.collect.pics
	for kind := pictureWhole; kind <= pictureCard; kind++ {
		key := pictureKey(path, kind)
		delete(a.bitmaps, key)
		delete(p.shown, key)
		delete(p.failed, key)
	}
}

// upright is img as a kind of picture shows it: turned as the EXIF
// orientation o says, and for the small kinds read down to their size, a
// card's cut to the card's shape first. A small one is made small first
// and turned after, which turns fewer points.
func upright(img image.Image, o, kind int) image.Image {
	r := img.Bounds()
	if r.Dx() <= 0 || r.Dy() <= 0 {
		return img
	}
	if o < 2 || o > 8 {
		o = 1
	}
	// What shows is w by h; the picture as kept is that turned (sw by sh).
	w, h := r.Dx(), r.Dy()
	if o >= 5 {
		w, h = h, w
	}
	// The part that shows, about the middle, and how large it is to be.
	cw, ch, k := w, h, 1.0
	switch kind {
	case pictureCard:
		// The card's shape, cut about the middle as the card would cut
		// it, and no larger than the card: a long page is its middle,
		// not 28 MB of it.
		if w*thumbTall > h*thumbWide {
			cw = max(h*thumbWide/thumbTall, 1)
		} else {
			ch = max(w*thumbTall/thumbWide, 1)
		}
		k = min(float64(thumbWide)/float64(cw), 1)
	case pictureSmall:
		k = min(max(float64(thumbWide)/float64(w), float64(thumbTall)/float64(h)), float64(smallMost)/float64(max(w, h)), 1)
	}
	if o == 1 && k >= 1 && cw == w && ch == h {
		return img
	}
	// The same part of the picture as kept: about the middle too.
	sw, sh, scw, sch := w, h, cw, ch
	if o >= 5 {
		sw, sh, scw, sch = h, w, ch, cw
	}
	src := image.Rect(0, 0, scw, sch).Add(r.Min).Add(image.Pt((sw-scw)/2, (sh-sch)/2))
	out := image.NewRGBA(image.Rect(0, 0, max(int(float64(scw)*k+0.5), 1), max(int(float64(sch)*k+0.5), 1)))
	if k < 1 {
		draw.ApproxBiLinear.Scale(out, out.Bounds(), img, src, draw.Src, nil)
	} else {
		draw.Draw(out, out.Bounds(), img, src.Min, draw.Src)
	}
	return oriented(out, o)
}

// oriented is img turned and flipped as the EXIF orientation o, 2 to 8,
// says it is to show; img itself for 1.
func oriented(img *image.RGBA, o int) *image.RGBA {
	if o < 2 || o > 8 {
		return img
	}
	w, h := img.Rect.Dx(), img.Rect.Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	out := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := range dh {
		for x := range dw {
			var sx, sy int // the point of img that shows at x, y
			switch o {
			case 2: // mirrored
				sx, sy = w-1-x, y
			case 3: // upside down
				sx, sy = w-1-x, h-1-y
			case 4:
				sx, sy = x, h-1-y
			case 5:
				sx, sy = y, x
			case 6: // to be turned a quarter clockwise
				sx, sy = y, h-1-x
			case 7:
				sx, sy = w-1-y, h-1-x
			case 8: // a quarter the other way
				sx, sy = w-1-y, x
			}
			copy(out.Pix[out.PixOffset(x, y):][:4], img.Pix[img.PixOffset(img.Rect.Min.X+sx, img.Rect.Min.Y+sy):][:4])
		}
	}
	return out
}

// exifOrientation is the orientation a JPEG's EXIF data says it is to
// show in, 1 to 8; 1, upright as kept, with none said or for another kind
// of picture.
func exifOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xff || data[1] != 0xd8 {
		return 1
	}
	// The segments before the picture's data: APP1 holds the EXIF.
	for i := 2; i+4 <= len(data) && data[i] == 0xff; {
		marker, size := data[i+1], int(binary.BigEndian.Uint16(data[i+2:]))
		if marker == 0xda || size < 2 || i+2+size > len(data) {
			break
		}
		if seg := data[i+4 : i+2+size]; marker == 0xe1 && len(seg) >= 14 && string(seg[:6]) == "Exif\x00\x00" {
			return tiffOrientation(seg[6:])
		}
		i += 2 + size
	}
	return 1
}

// tiffOrientation reads the Orientation tag (0x0112, a short) of the
// first directory of TIFF data, which is what EXIF is.
func tiffOrientation(t []byte) int {
	if len(t) < 8 {
		return 1
	}
	var bo binary.ByteOrder = binary.LittleEndian
	switch string(t[:2]) {
	case "II":
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}
	dir := int(bo.Uint32(t[4:]))
	if dir < 8 || dir+2 > len(t) {
		return 1
	}
	n := int(bo.Uint16(t[dir:]))
	for e := dir + 2; n > 0 && e+12 <= len(t); e, n = e+12, n-1 {
		if bo.Uint16(t[e:]) == 0x0112 && bo.Uint16(t[e+2:]) == 3 {
			if o := int(bo.Uint16(t[e+8:])); o >= 1 && o <= 8 {
				return o
			}
			return 1
		}
	}
	return 1
}
