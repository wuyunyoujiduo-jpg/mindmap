package main

import (
	"bytes"
	"embed"
	"image/png"

	"github.com/egoist/mygo/ui"
)

// The icons of the app's places and lists are Kubai Icons
// (https://github.com/kubai087/kubai-icons, MIT), drawn as bitmaps by
// tools/kubai/gen.mjs, MyGo's SVG having no filters: each at rest, soft,
// and as it is under the pointer or chosen, in the accent and in the pose
// the set gives it on hover (a lid lifts, a bulb lights). kubaiIcon goes
// from one to the other. The small marks on keys and tools stay drawn
// lines (icons.go): these are for what is gone to.

//go:embed kubai/*.png
var kubaiFS embed.FS

var kubaiBitmaps = map[string]*ui.Bitmap{}

func kubaiBitmap(name string) *ui.Bitmap {
	if b, ok := kubaiBitmaps[name]; ok {
		return b
	}
	var b *ui.Bitmap
	if data, err := kubaiFS.ReadFile("kubai/" + name + ".png"); err == nil {
		if img, err := png.Decode(bytes.NewReader(data)); err == nil {
			b = ui.NewBitmap(img)
		}
	}
	kubaiBitmaps[name] = b
	return b
}

// kubaiIcon shows the icon name in a square of size: soft at rest, in the
// accent and in its hover pose when on, one fading into the other.
func kubaiIcon(c *ui.Context, name string, size float32, on bool) ui.Element {
	box := ui.Box(c.Key("kubai:"+name)).Size(size, size).Shrink(0).PassThrough()
	k := box.AnimateTo("on", b2f(on), ui.Animation{Duration: motionFast, Ease: easeDetent, NonMotion: true})
	rest, lit := kubaiBitmap(name), kubaiBitmap(name+"-on")
	return box.Draw(func(p *ui.Painter, r ui.Rect) {
		for _, l := range []struct {
			b *ui.Bitmap
			a float32
		}{{rest, 1 - k}, {lit, k}} {
			if l.b == nil || l.a <= 0.01 {
				continue
			}
			w, h := l.b.Size()
			p.ImageRegion(l.b, ui.Rect{W: float32(w), H: float32(h)}, r, ui.ImageOptions{Tint: ui.RGBA(255, 255, 255, l.a)})
		}
	})
}

// kubaiKey is a key of the top row that is one of these icons: lit under
// the pointer and while what it opens is open.
func kubaiKey(c *ui.Context, name, label, tip string, open bool) ui.Element {
	t := c.Theme()
	b := ui.Box(c.Key("key:"+label)).Size(keyHeight, keyHeight).Radius(radiusPill).Center().Cursor(ui.CursorPointer).
		Label(label).Tooltip(tip).Background(t.Text.Alpha(0)).Transition(easeLook)
	hot := b.Hovered()
	switch {
	case open:
		b.Background(t.Accent.Alpha(0.12))
	case hot:
		b.Background(t.Text.Alpha(0.06))
	}
	press(b, 0.92)
	b.Children(func() { kubaiIcon(c, name, 26, hot || open) })
	return b
}
