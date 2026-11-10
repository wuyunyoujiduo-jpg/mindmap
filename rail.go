package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

// railWidth is the least width of the rail of the app's parts.
const railWidth = 76

// rail is the narrow bar on the left: the parts of the app, each an icon
// over its name, the one shown tinted with the accent; at its foot the
// search of everything, the settings and how the sync goes. The maps'
// folders open beside it from 导图.
func (a *app) rail(c *ui.Context) {
	t := c.Theme()
	bar := c.TitleBar()
	// As wide as the window's controls and a little more, so that they sit
	// over the rail and what is beside it keeps clear of them.
	a.railNow = max(railWidth, bar.Left+12)
	col := ui.Column(c.Key("rail")).Width(a.railNow).Shrink(0).FillHeight().AlignItems(ui.Center).Padding(0, 0, 10).Gap(4)
	if !c.Vibrancy() {
		col.Background(t.Surface)
	}
	page := a.libOpen
	item := func(key string, kind int, label, count, tip string, on bool, click func()) {
		b := ui.Column(c.Key("rail:"+key)).Width(62).Padding(9, 0, 7).Gap(4).Radius(12).AlignItems(ui.Center).
			Cursor(ui.CursorPointer).Label(label).Tooltip(tip)
		// Quiet lines in the muted ink; the part shown in the ink, on a
		// shade of the rail.
		ink, text := t.TextMuted, t.TextMuted
		switch {
		case on:
			b.Background(t.Text.Alpha(0.075))
			ink, text = t.Text, t.Text
		case b.Hovered():
			b.Background(t.Text.Alpha(0.04))
		}
		if b.Clicked() {
			a.later(click)
		}
		b.Children(func() {
			ui.Box(c).Size(24, 22).PassThrough().Children(func() {
				ui.Box(c).Absolute().Left(0).Top(-1).Size(24, 24).PassThrough().
					Draw(func(p *ui.Painter, r ui.Rect) { drawRailIcon(p, r, 23, kind, ink) })
				if count != "" {
					ui.Box(c).Absolute().Left(14).Top(-4).MinWidth(15).Height(15).Padding(0, 4).Radius(8).Center().Background(t.Accent).Children(func() {
						ui.Text(c, count).FontSize(10).FontWeight(600).TextColor(ui.Hex("#FFFFFF")).PassThrough()
					})
				}
			})
			txt := ui.Text(c, label).FontSize(textCaption).TextColor(text).PassThrough()
			if on {
				txt.FontWeight(600)
			}
		})
	}
	col.Children(func() {
		ui.Box(c).Height(max(bar.Height, 38)).FillWidth().DragWindow()
		item("home", railDesk, "工作台", "", "记下来、归位、今天要做的", page && a.libHome && !a.tasks.open && !a.memos.open, func() {
			a.showLibrary()
			a.libHome = true
		})
		item("map", railMap, "导图", "", "打开的导图；再点一下，显示或收起导图列表", !page, func() {
			if a.libOpen {
				a.commitEditIfAny()
				a.libOpen = false
				return
			}
			a.cfg.ShowTree = !a.cfg.ShowTree
			a.cfg.save()
		})
		due := ""
		if n := a.tasksDue(); n > 0 {
			due = fmt.Sprint(n)
		}
		item("tasks", railTasks, "任务", due, "所有导图里的任务", page && a.tasks.open, a.openTasks)
		item("library", railCollect, "收集", "", "灵感、链接、文档和图片", page && !a.libHome && !a.tasks.open, func() { a.openList(kindAll, "") })
		ui.Spacer(c)
		foot := func(kind int, label, tip string, click func()) {
			b := ui.Box(c.Key("rail-foot:"+label)).Size(40, 40).Radius(12).Cursor(ui.CursorPointer).Label(label).Tooltip(tip)
			if b.Hovered() {
				b.Background(t.Text.Alpha(0.05))
			}
			if b.Clicked() {
				a.later(click)
			}
			b.Draw(func(p *ui.Painter, r ui.Rect) { drawRailIcon(p, r, 21, kind, t.TextMuted) })
		}
		foot(railSearch, "搜索全部", "搜索导图、主题、灵感和资料，#标签 按标签找（⌘K）", func() { a.openPalette(paletteAll) })
		foot(railGear, "设置", "设置（⌘,）", a.openSettings)
	})
}

// sideWidth is the room the rail and, when shown, the maps' list take.
func (a *app) sideWidth() float32 {
	rail := max(a.railNow, railWidth)
	if a.cfg.ShowTree {
		return rail + a.sidebarWidth
	}
	return rail
}
