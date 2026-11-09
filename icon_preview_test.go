package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// The map's icon at the sizes it shows, for the snapshots.
func TestIconPreview(t *testing.T) {
	tt := ui.NewTester(func(c *ui.Context) {
		th := c.Theme()
		ui.Column(c).Fill().Padding(20).Gap(16).Background(ui.Hex("#F5F5F5")).Children(func() {
			for _, size := range []float32{14, 16, 20, 32, 64} {
				ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
					glyph(c, iconFolder, size, th.TextMuted)
					glyph(c, iconMap, size, th.TextMuted)
					glyph(c, iconText, size, th.TextMuted)
					glyph(c, iconInbox, size, th.TextMuted)
					glyph(c, iconMap, size, ui.Hex("#E8833A"))
					ui.Text(c, "使用指南").FontSize(13)
				})
			}
		})
	}, 360, 300)
	tt.Frame()
	snapshot(t, tt, "icon-map")
}
