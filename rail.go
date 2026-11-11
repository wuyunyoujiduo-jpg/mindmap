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
	a.railNow = a.sideWidth()
	col := ui.Column(c.Key("rail")).Width(a.railNow).Shrink(0).FillHeight().Padding(0, 12, 12)
	col.Background(ground(t)) // the ground chosen, not the system's glass
	page := a.libOpen
	now := a.now()
	muted := t.TextMuted
	// A row of the sidebar: an icon, a word, a count at its end; the part
	// shown is on the accent, whole.
	item := func(key string, kind int, label, count, tip string, on bool, click func()) {
		b := ui.Row(c.Key("rail:"+key)).Height(32).Padding(0, 10).Gap(10).Radius(radiusKey).AlignItems(ui.Center).FillWidth().
			Cursor(ui.CursorPointer).Label(label).Tooltip(tip).Background(t.Text.Alpha(0)).Transition(easeLook)
		ink, num := ink2(t), faint(t)
		switch {
		case on:
			// A shade, not the accent: the accent is the key's that acts.
			b.Background(chosen(t))
			ink = t.Text
		case b.Hovered():
			b.Background(t.Text.Alpha(0.045))
			ink = t.Text
		}
		if b.Clicked() {
			a.later(click)
		}
		press(b, 0.985)
		b.Children(func() {
			ui.Box(c).Size(18, 18).Shrink(0).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) { drawRailIcon(p, r, 18, kind, ink) })
			txt := ui.Text(c, label).FontSize(textBody).TextColor(ink).Grow(1).PassThrough()
			if on {
				txt.FontWeight(600)
			}
			if count != "" {
				ui.Text(c, count).FontSize(textCaption).TextColor(num).PassThrough()
			}
		})
	}
	col.Children(func() {
		ui.Box(c).Height(max(bar.Height, 40)).FillWidth().Shrink(0).DragWindow()
		// The day in numbers, and the week: a square a day, deeper the more
		// was done on it.
		due, done := a.dueToday(now), a.doneToday(now)
		ui.Row(c.Key("rail-stats")).Gap(22).Padding(4, 10, 14).Shrink(0).DragWindow().Children(func() {
			stat := func(n int, name string) {
				ui.Column(c.Key(name)).Gap(1).Children(func() {
					ui.Text(c, fmt.Sprint(n)).FontSize(22).LineHeight(1.2).FontWeight(600).LetterSpacing(-0.5).FontFeatures("tnum")
					ui.Text(c, name).FontSize(textCaption).TextColor(muted).NoWrap()
				})
			}
			stat(len(due), "要做")
			stat(len(done), "做完")
			stat(len(a.lib.Clips), "已收集")
		})
		start := weekStart(today(now))
		per := make([]int, 7)
		for _, it := range a.taskItems() {
			if d := it.node.Task.DoneAt; it.done() && d != nil {
				if i := int(today(*d).Sub(start).Hours() / 24); i >= 0 && i < 7 {
					per[i]++
				}
			}
		}
		// The week is a line of seven strokes, in the accent where something
		// was done; today's letter is in the ink.
		ui.Row(c.Key("rail-week")).Gap(5).Padding(0, 10, 18).Shrink(0).Children(func() {
			for i, n := range per {
				ui.Column(c.Key(fmt.Sprint(i))).Grow(1).Basis(0).Gap(6).AlignItems(ui.Center).
					Tooltip(fmt.Sprintf("%s 做完 %d 件", weekdays[(i+1)%7], n)).Children(func() {
					sq := ui.Box(c).Height(4).FillWidth().Radius(2).Background(t.Text.Alpha(0.09))
					switch {
					case n >= 3:
						sq.Background(t.Accent)
					case n > 0:
						sq.Background(t.Accent.Alpha(0.45))
					}
					day := ui.Text(c, string([]rune(weekdays[(i+1)%7])[1:])).FontSize(11).TextColor(faint(t))
					if sameDay(start.AddDate(0, 0, i), now) {
						day.TextColor(t.Text).FontWeight(600)
					}
				})
			}
		})
		ui.Column(c.Key("rail-nav")).Gap(2).Shrink(0).Children(func() {
			item("home", railDesk, "工作台", "", "记下来、归位、今天要做的", page && a.libHome && !a.tasks.open, func() {
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
			n := ""
			if len(due) > 0 {
				n = fmt.Sprint(len(due))
			}
			item("tasks", railTasks, "任务", n, "所有导图里的任务", page && a.tasks.open, a.openTasks)
			item("library", railCollect, "收集", fmt.Sprint(len(a.lib.Clips)), "灵感、链接、文档和图片", page && !a.libHome && !a.tasks.open, func() { a.openList(kindAll, "") })
		})
		if a.treeShown() {
			// On the maps' page the maps and their folders are here.
			ui.Column(c.Key("rail-tree")).Grow(1).Basis(0).MinHeight(0).Margin(8, -12, 0).Children(func() { a.sidebar(c) })
		} else {
			ui.Text(c, "最近的导图").FontSize(textCaption).FontWeight(600).LetterSpacing(0.3).TextColor(faint(t)).Margin(22, 10, 6).Shrink(0)
			ui.Column(c.Key("rail-maps")).Gap(2).Grow(1).Basis(0).MinHeight(0).Clip().Children(func() {
				shown := 0
				for _, fi := range a.recentFiles() {
					if fi.Path == a.inboxPath() || shown == 5 {
						continue
					}
					shown++
					b := ui.Row(c.Key(fi.Path)).Height(30).Padding(0, 10).Gap(10).Radius(radiusKey).AlignItems(ui.Center).FillWidth().
						Cursor(ui.CursorPointer).Label("最近 " + mapName(fi.Path)).Background(t.Text.Alpha(0)).Transition(easeLook)
					if b.Hovered() {
						b.Background(t.Text.Alpha(0.045))
					}
					if b.Clicked() {
						a.later(func() { _ = a.open(fi.Path) })
					}
					b.Children(func() {
						ui.Box(c).Size(18, 18).Center().Shrink(0).PassThrough().Children(func() { mapDot(c, a.mapColor(fi.Path), 8) })
						ui.Text(c, mapName(fi.Path)).FontSize(textBody).TextColor(ink2(t)).SingleLine().Grow(1).Shrink(1).MinWidth(0).PassThrough()
					})
				}
			})
		}
		ui.Column(c.Key("rail-foot")).Gap(2).Shrink(0).Padding(6, 0, 0).Children(func() {
			item("search", railSearch, "搜索全部", "⌘K", "搜索导图、主题、灵感和资料，#标签 按标签找（⌘K）", false, func() { a.openPalette(paletteAll) })
			item("settings", railGear, "设置", "", "设置（⌘,）", false, a.openSettings)
		})
	})
}

// sideWidth is the room the rail and, when shown, the maps' list take.
func (a *app) sideWidth() float32 {
	return max(a.sidebarWidth+railWidth, sideMin)
}

// sideMin is the least the sidebar is wide.
const sideMin = 220

// treeShown tells whether the maps' list is beside the rail: when asked
// for, and on the maps' page only. The desk, the tasks and 收集 are pages
// of their own, with the whole width.
func (a *app) treeShown() bool { return a.cfg.ShowTree && !a.libOpen }
