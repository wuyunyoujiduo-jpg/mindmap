package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

// railWidth is the width of the rail of the app's parts.
const railWidth = 76

// rail is the narrow bar on the left: the parts of the app, each an icon
// over its name, the one shown tinted with the accent; at its foot the
// search of everything, the settings and how the sync goes. The maps'
// folders open beside it from 导图.
func (a *app) rail(c *ui.Context) {
	t := c.Theme()
	bar := c.TitleBar()
	col := ui.Column(c.Key("rail")).Width(railWidth).Shrink(0).FillHeight().AlignItems(ui.Center).Padding(0, 0, 10).Gap(4)
	if !c.Vibrancy() {
		col.Background(t.Surface)
	}
	page := a.libOpen
	item := func(key string, ic icon, label, count, tip string, on bool, click func()) {
		b := ui.Column(c.Key("rail:"+key)).Width(62).Padding(8, 0, 6).Gap(3).Radius(12).AlignItems(ui.Center).
			Cursor(ui.CursorPointer).Label(label).Tooltip(tip)
		ink := t.TextMuted
		switch {
		case on:
			bg, deep := tinted(t, t.Accent)
			b.Background(bg)
			ink = deep
		case b.Hovered():
			b.Background(t.Text.Alpha(0.05))
		}
		if b.Clicked() {
			a.later(click)
		}
		b.Children(func() {
			ui.Box(c).Size(24, 22).PassThrough().Children(func() {
				ui.Box(c).Absolute().Left(2).Top(1).Children(func() { solidGlyph(c, ic, 20, ink, t.Surface) })
				if count != "" {
					ui.Box(c).Absolute().Left(14).Top(-4).MinWidth(15).Height(15).Padding(0, 4).Radius(8).Center().Background(t.Accent).Children(func() {
						ui.Text(c, count).FontSize(10).FontWeight(600).TextColor(ui.Hex("#FFFFFF")).PassThrough()
					})
				}
			})
			txt := ui.Text(c, label).FontSize(textCaption).TextColor(ink).PassThrough()
			if on {
				txt.FontWeight(600)
			}
		})
	}
	col.Children(func() {
		ui.Box(c).Height(max(bar.Height, 38)).FillWidth().DragWindow()
		item("home", iconCategory, "工作台", "", "记下来、归位、今天要做的", page && a.libHome && !a.auto.open && !a.tasks.open && !a.memos.open, func() {
			a.showLibrary()
			a.libHome = true
		})
		item("map", iconMap, "导图", "", "打开的导图；再点一下，显示或收起导图列表", !page, func() {
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
		item("tasks", iconCheck, "任务", due, "所有导图里的任务", page && a.tasks.open, a.openTasks)
		item("library", iconLibrary, "收集", "", "灵感、链接、文档和图片", page && !a.libHome && !a.auto.open && !a.tasks.open, func() { a.openList(kindAll, "") })
		item("auto", iconBolt, "自动化", "", "把日常任务写成程序，手动或定时运行", page && a.auto.open, a.openAutomations)
		ui.Spacer(c)
		if iconButton(c, iconSearch, "搜索全部", "搜索导图、任务、灵感和资料，或执行命令（⌘K）", 36, true).Clicked() {
			a.later(func() { a.openPalette(paletteAll) })
		}
		if iconButton(c, iconGear, "设置", "设置（⌘,）", 36, true).Clicked() {
			a.later(a.openSettings)
		}
	})
}

// sideWidth is the room the rail and, when shown, the maps' list take.
func (a *app) sideWidth() float32 {
	if a.cfg.ShowTree {
		return railWidth + a.sidebarWidth
	}
	return railWidth
}
