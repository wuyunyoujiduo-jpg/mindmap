package main

import (
	"github.com/egoist/mygo/ui"
)

// Each map has a color of its own, which marks what is its: its card on
// the desk, its tasks in the lists, what was put on it. It is the map's
// own (File.Color) or, until one is chosen, one of the palette by its
// name, the same each time.

// mapColor is the color of the map at path.
func (a *app) mapColor(path string) ui.Color {
	f := a.file
	if path != a.path {
		f = a.indexed(path)
	}
	if f != nil && f.Color != "" {
		return ui.Hex(f.Color)
	}
	var h uint32 = 2166136261
	for _, b := range []byte(mapName(path)) {
		h = (h ^ uint32(b)) * 16777619
	}
	return ui.Hex(mapPalette[h%uint32(len(mapPalette))])
}

// setMapColor gives the open map a color.
func (a *app) setMapColor(hex string) {
	if a.file == nil {
		return
	}
	a.mutate("", func() { a.file.Color = hex })
}

// mapColorMenu offers the colors for the open map.
func (a *app) mapColorMenu(m *ui.Menu) {
	names := []string{"珊瑚", "琥珀", "青绿", "天蓝", "紫", "玫红"}
	for i, hex := range mapPalette {
		if m.Item(names[i]).Checked(a.file != nil && a.file.Color == hex).Chosen() {
			a.later(func() { a.setMapColor(hex) })
		}
	}
}

// mapDot is a small dot of a map's color.
func mapDot(c *ui.Context, col ui.Color, size float32) ui.Element {
	return ui.Box(c).Size(size, size).Radius(size / 2).Background(col).Shrink(0).PassThrough()
}
