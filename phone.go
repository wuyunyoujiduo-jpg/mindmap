package main

import (
	"fmt"
	"math"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/mobile"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/layout"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// The app on a phone (Android, by MyGo 0.10; the same holds for an iPhone).
// The window is the screen, a phone's some 400 wide: the leaf has room for
// one column, the page's. So, in a window narrower than compactWidth:
//   - the places are a row at the window's foot (phoneTabs), each its
//     icon over its name, where the thumb is; the top row keeps the left
//     column's key, the one bar, the AI's spark, the search and the
//     settings (topBar);
//   - the left column slides in over the page from the left
//     (mobile.Drawer), from its key, and goes as what it narrows to is
//     chosen (drawerSubject) or by a finger, a tap beside it or back;
//   - what is chosen (sheet) and a topic's panel cover the page, under a
//     row with the way back (sheetBack), which back also takes.
// What a phone cannot do is not offered there (onPhone): the screenshot,
// the shortcuts of the keyboard, the updates (the store's or the
// release's, by hand), the agents' hooks.

// apkURL is where the newest Android package is: beside the desktops'
// updates, under one name whatever the version (the workflow build's
// android-publish).
const apkURL = "https://github.com/wuyunyoujiduo-jpg/mindmap-models/releases/download/desktop-latest/MyMind.apk"

// compactWidth is the width under which the window is laid out as a
// phone's.
const compactWidth = 640

// onPhone tells whether the app runs on a phone or a tablet.
var onPhone = runtime.GOOS == "android" || runtime.GOOS == "ios"

// narrowLayout is whether the frame being built is laid out as a phone's
// (view sets it), for what has no app at hand (sheet).
var narrowLayout bool

// narrow tells whether the window is laid out as a phone's.
func (a *app) narrow(c *ui.Context) bool {
	w, _ := c.Size()
	return w < compactWidth
}

// phonePlaces are the places at the window's foot, with their icons: not
// 技能, the AI agents' skills, which are of the computers the agents run
// on (the user's, 0.107.1); a phone neither looks for them nor shows them
// (onPhone).
var phonePlaces = []struct {
	place      int
	name, icon string
}{{placeToday, "今天", "calendar"}, {placeMaps, "项目", "folder"}, {placeCollect, "收集", "layers"}}

// phoneTabs is the row of the places at the window's foot: on the
// window's gray under a hairline, the place shown in the accent.
func (a *app) phoneTabs(c *ui.Context) {
	t := c.Theme()
	now := a.place()
	ui.Row(c.Key("phone-tabs")).Height(56).Padding(2, 6, 2).Shrink(0).FillWidth().AlignItems(ui.Stretch).Label("位置").
		Background(ground(t)).BorderWidth(1, 0, 0, 0).BorderColor(hairline(t)).Children(func() {
		for _, p := range phonePlaces {
			on := p.place == now
			b := ui.Column(c.Key("tab:" + p.name)).Grow(1).Basis(0).Gap(2).Center().Cursor(ui.CursorPointer).Label(p.name).Radius(radiusRow)
			if b.Clicked() {
				a.later(func() { a.goPlace(p.place) })
			}
			press(b, 0.94)
			b.Children(func() {
				kubaiIconOf(c, p.icon, 26, on, true)
				txt := ui.Text(c, p.name).FontSize(textCaption).PassThrough()
				if on {
					txt.FontWeight(700).TextColor(t.Accent)
				} else {
					txt.TextColor(t.TextMuted)
				}
			})
		}
	})
}

// goPlace shows a place, as its tab of the top row does.
func (a *app) goPlace(p int) {
	switch p {
	case placeToday:
		a.goToday()
	case placeMaps:
		a.showProjects()
	case placeCollect:
		a.goCollect()
	}
}

// phoneDrawer is the left column of the place shown, slid in over the
// page while a.drawer: it goes once what it shows has changed, a list, a
// map or a kind chosen in it, so that the page is seen; a folder opened
// in the directory keeps it.
func (a *app) phoneDrawer(c *ui.Context) {
	if a.presenting {
		a.drawer = false
	}
	now := a.drawerSubject()
	if a.drawer && a.drawerWas != "" && now != a.drawerWas {
		a.drawer = false
	}
	a.drawerWas = ""
	if a.drawer {
		a.drawerWas = now
	}
	mobile.Drawer(c, &a.drawer, mobile.DrawerOptions{Width: drawerWidth}, func() {
		// It is drawn over the system's bars: what it holds is under them.
		top, _, bottom, _ := c.SafeArea()
		ui.Column(c.Key("drawer")).Fill().Padding(top, 0, bottom).Background(cardColor(c.Theme())).Children(func() {
			ui.Column(c.Key("side")).Grow(1).Basis(0).MinHeight(0).FillWidth().Children(func() { a.sideColumn(c) })
			a.drawerFoot(c)
		})
	})
}

// drawerWidth is the left column's width on a phone, wider than beside a
// page: it is over the page, and a finger is broader than a pointer.
const drawerWidth = 288

// sideColumn is the left column of the place shown.
func (a *app) sideColumn(c *ui.Context) {
	switch a.place() {
	case placeMaps:
		a.mapTree(c)
	case placeToday:
		a.todaySide(c)
	case placeSkills:
		a.skillsSide(c)
	default:
		a.libSide(c)
	}
}

// sideWidthNow is the left column's width: in the drawer on a phone, else
// beside the page.
func sideWidthNow() float32 {
	if narrowLayout {
		return drawerWidth
	}
	return sideWidthOf
}

// drawerSubject is what the left column narrows the page to: when it
// changes, the drawer has done its work.
func (a *app) drawerSubject() string {
	return fmt.Sprint(a.place(), a.tasks.view, a.tasks.onMap, a.path, a.projFolder,
		a.libKind, a.libCat, a.libTag, a.libReview, a.skills.side, a.skills.tool)
}

// sheetBack is the row over what is chosen when it covers the page on a
// phone: the way back to the page. Escape, and so back, puts it away as
// each page has it do beside the page.
func sheetBack(c *ui.Context, close func()) { sheetBackTo(c, "返回", close) }

// sheetBackTo is sheetBack saying where it goes back to.
func sheetBackTo(c *ui.Context, to string, close func()) {
	t := c.Theme()
	ui.Row(c.Key("sheet-back")).Height(44).Padding(0, 8).Shrink(0).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(hairline(t)).Children(func() {
		b := ui.Row(c.Key("back")).Height(36).Padding(0, 10, 0, 4).Gap(2).Radius(radiusRow).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(to)
		if b.Clicked() {
			close()
		}
		press(b, 0.96)
		b.Children(func() {
			glyph(c, iconBack, 18, t.Accent)
			ui.Text(c, to).FontSize(textTitle).TextColor(t.Accent).PassThrough()
		})
	})
}

// drawerFoot is the drawer's foot: how the sync stands, which opens its
// settings, and the settings, which a phone's top row has no room for.
func (a *app) drawerFoot(c *ui.Context) {
	t := c.Theme()
	ui.Column(c.Key("drawer-foot")).Padding(6, 10, 8).Gap(1).Shrink(0).FillWidth().BorderWidth(1, 0, 0, 0).BorderColor(hairline(t)).Children(func() {
		if a.syncReady() {
			ink := t.TextMuted
			if a.syncErr != "" {
				ink = t.Danger
			}
			row := ui.Row(c.Key("sync")).Height(44).Padding(0, 12).Gap(12).Radius(radiusRow).AlignItems(ui.Center).FillWidth().
				Cursor(ui.CursorPointer).Label("同步：" + a.syncLabel())
			if row.Clicked() {
				a.later(a.openSyncSettings)
			}
			row.Children(func() {
				glyph(c, iconCloud, 18, ink)
				ui.Text(c, "GitHub "+a.syncLabel()).FontSize(textSmall).TextColor(ink).PassThrough()
			})
		}
		if sideRow(c, "settings", "设置", "", false, sideIcon(c, "settings")).Clicked() {
			a.later(a.openSettings)
		}
	})
}

// phoneTop is a place's row of keys on a phone, over its page: the left
// column's, and at the right the search of everything and the AI's spark
// (the settings are at the drawer's foot, the one bar at the window's,
// phoneBar).
func (a *app) phoneTop(c *ui.Context) {
	ui.Row(c.Key("phone-top")).Height(44).Padding(0, 8).Gap(4).AlignItems(ui.Center).Shrink(0).FillWidth().Children(func() {
		if kubaiKey(c, "sidebar", "侧栏", "侧栏", a.drawer, true).Clicked() {
			a.drawer = !a.drawer
		}
		ui.Spacer(c)
		if a.place() == placeMaps {
			iconButton(c, iconPlus, "新建", "新建导图或文件夹", 34, true).Menu(func(m *ui.Menu) {
				if m.Item("新建导图").Chosen() {
					a.later(func() { a.newMapIn(cmpOr(a.projFolder, a.dir)) })
				}
				if m.Item("从模板新建…").Chosen() {
					a.templatesOpen = true
				}
				if m.Item("新建文件夹").Chosen() {
					a.later(func() { a.newFolder(cmpOr(a.projFolder, a.dir)) })
				}
			})
		}
		if kubaiKey(c, "search", "搜索全部", "搜索导图、主题、随记和资料", false, true).Clicked() {
			a.later(func() { a.openPalette(paletteAll) })
		}
		if kubaiKey(c, "sparkles", "AI 助手", "问 AI：问问导图和资料里的内容", a.aiOpen, false).Clicked() {
			a.aiOpen = !a.aiOpen
		}
	})
}

// phoneBar is the one bar on a phone: a line at the window's foot, over
// the places, where the thumb is, saying what a line is kept as there (a
// task on 今天, else a note of 随记; barTask). A tap grows it upward where
// it is into the card to write in, its tools and what it is kept as, the
// keyboard under it and the places put away meanwhile (view): not a
// sheet over a dimmed page, which the user found not good to look at.
// A tap beside it with nothing written puts it back.
func (a *app) phoneBar(c *ui.Context) {
	t := c.Theme()
	text := strings.TrimSpace(a.homeDraft)
	ui.Column(c.Key("phone-bar-room")).Padding(4, 12, 8).Gap(8).Shrink(0).FillWidth().Children(func() {
		if a.barOpen {
			card := ui.Column(c.Key("phone-bar-open")).Gap(8).FillWidth().
				Transition(ui.ElementTransition{Duration: motionFast, Ease: easeDetent, Enter: &ui.Motion{Y: 12}})
			if card.PressedOutside() && text == "" {
				a.barOpen = false
			}
			card.Children(func() { a.barCard(c, text) })
			return
		}
		hint, kind := "记一条随记、加任务…", "随记"
		if a.barTask(text) {
			hint, kind = "加任务，如「明天 9点 写周报」", "任务"
		}
		if text != "" {
			hint = text // what was written and not kept, waiting
		}
		line := captureBox(c, "phone-bar").MinHeight(46).Radius(radiusCard).Cursor(ui.CursorText).Label("记一条随记或任务")
		if line.Clicked() {
			a.barOpen, a.writeRefocus = true, "home-line"
		}
		line.Children(func() {
			glyph(c, iconPlus, 15, t.Accent)
			txt := ui.Text(c, hint).FontSize(textTitle).SingleLine().Grow(1).Shrink(1).MinWidth(0).PassThrough()
			if text == "" {
				txt.TextColor(t.TextMuted)
			}
			pill(c, kind, t.Accent.Alpha(0.1), t.Accent)
		})
	})
}

// phoneWeek is 日程's strip on a phone, over the hours of the day shown:
// the seven days of its week, each its weekday, its date and its word of
// the Chinese calendar, the day shown on the accent's tint, today in the
// accent, a day of rest on the rest's ground (restFill); the weeks before
// and after at its ends.
func (a *app) phoneWeek(c *ui.Context) {
	t := c.Theme()
	now := today(a.now())
	start := a.weekOf(a.tasks.day)
	ui.Row(c.Key("phone-week")).Padding(2, 4, 8).Gap(2).AlignItems(ui.Center).Shrink(0).FillWidth().
		BorderWidth(0, 0, 1, 0).BorderColor(hairline(t)).Children(func() {
		if iconButton(c, iconBack, "上一周", "上一周", 28, true).Clicked() {
			a.tasks.day = a.tasks.day.AddDate(0, 0, -7)
		}
		for i := range 7 {
			d := start.AddDate(0, 0, i)
			on, isToday := sameDay(d, a.tasks.day), sameDay(d, now)
			cell := ui.Column(c.Key(i)).Grow(1).Basis(0).MinWidth(0).Padding(6, 0).Gap(1).Radius(10).AlignItems(ui.Center).
				Cursor(ui.CursorPointer).Label(fmt.Sprintf("%d月%d日", d.Month(), d.Day()))
			switch {
			case on:
				cell.Background(t.Accent.Alpha(0.1))
			case restDay(d):
				cell.Background(restFill(t))
			}
			if cell.Clicked() {
				a.tasks.day = d
			}
			press(cell, 0.95)
			cell.Children(func() {
				ui.Text(c, weekdayName(d)[len("周"):]).FontSize(textCaption - 1).TextColor(t.TextMuted).PassThrough()
				day := ui.Text(c, fmt.Sprint(d.Day())).FontSize(17).FontFeatures("tnum").PassThrough()
				if on || isToday {
					day.FontWeight(700).TextColor(t.Accent)
				}
				word, feast := lunarLabel(d)
				ink := faint(t)
				if feast {
					ink = t.Accent.MixOklab(t.Text, 0.2)
				}
				ui.Text(c, word).FontSize(10).TextColor(ink).SingleLine().PassThrough()
			})
		}
		if iconButton(c, iconForward, "下一周", "下一周", 28, true).Clicked() {
			a.tasks.day = a.tasks.day.AddDate(0, 0, 7)
		}
	})
}

// phoneKinds are 收集's kinds as a row of chips under its head on a
// phone, where the left column is a drawer away: 全部, the kinds, and 星标;
// the one chosen on the accent's tint. The tags and the rest stay in the
// drawer.
func (a *app) phoneKinds(c *ui.Context) {
	t := c.Theme()
	counts := a.kindCounts()
	ui.ScrollHorizontal(c.Key("phone-kinds")).Shrink(0).FillWidth().Children(func() {
		ui.Row(c.Key("chips")).Gap(8).Padding(2, 0, 10).Children(func() {
			chip := func(key, name string, on bool, click func()) {
				b := ui.Box(c.Key(key)).Height(32).Padding(0, 13).Radius(radiusPill).Center().Cursor(ui.CursorPointer).Label(name).
					Background(t.Text.Alpha(0.045)).Transition(easeLook)
				if on {
					b.Background(t.Accent.Alpha(0.1))
				}
				if b.Clicked() {
					a.later(click)
				}
				press(b, 0.96)
				b.Children(func() {
					txt := ui.Text(c, name).FontSize(textBody).NoWrap().PassThrough()
					if on {
						txt.FontWeight(700).TextColor(t.Accent)
					} else {
						txt.TextColor(ink2(t))
					}
				})
			}
			for _, k := range []struct {
				kind int
				name string
			}{{kindAll, "全部"}, {kindMemos, "随记"}, {kindLinks, "链接"}, {kindDocs, "文档"}, {kindImages, "图片"}, {kindVideos, "视频"}} {
				if k.kind == kindVideos && counts[k.kind] == 0 {
					continue
				}
				chip("kind:"+k.name, k.name, a.libKind == k.kind && a.libReview == reviewOff, func() {
					a.libKind, a.libReview, a.libSel = k.kind, reviewOff, ""
				})
			}
			chip("star", "★ 星标", a.libCat == "★", func() { a.libCat, a.libReview = toggled(a.libCat, "★"), reviewOff })
		})
	})
}

// phoneCanvasFoot is the foot of a map's canvas on a phone: with a topic
// chosen, its keys in a bar floating over the canvas, a finger's size
// each (a topic under it, one beside it, its words, a task, its details
// in the sheet from the foot); and a round key back to the middle. The
// zoom is the fingers'; the lenses and the rest are the desktop's.
func (a *app) phoneCanvasFoot(c *ui.Context) {
	t := c.Theme()
	if a.carry != "" { // a topic taken up: its ghost under the finger
		if n, _ := a.doc.Find(a.carry); n != nil {
			lifted(ui.Box(c.Key("carried")).Absolute().Left(a.carryAt[0]-60).Top(a.carryAt[1]-44).Width(120).Height(36).
				Padding(0, 12).Radius(10).Center().Background(cardColor(t)).PassThrough().Label("拿起的主题"), t).Children(func() {
				ui.Text(c, n.Title).FontSize(textBody).FontWeight(700).SingleLine().PassThrough()
			})
		}
		return
	}
	n, _ := a.selectedNode()
	bar := n != nil && a.editing == "" && len(a.multi) == 0 && !a.lensQuiet
	foot := float32(16)
	if bar {
		foot = 88
	}
	home := lifted(ui.Box(c.Key("recenter")).Absolute().Right(14).Bottom(foot).Size(44, 44).Radius(22).Center().
		Background(cardColor(t)).Cursor(ui.CursorPointer).Label("回到中心"), t)
	if home.Clicked() {
		a.recenter()
	}
	press(home, 0.94)
	home.Children(func() { glyph(c, iconTarget, 18, ink2(t)) })
	if !bar {
		return
	}
	task := "任务"
	if n.Task != nil {
		task = "取消任务"
	}
	keys := []struct {
		name string
		ic   icon
		do   func()
	}{
		{"子主题", iconPlus, a.addChild},
		{"同级", iconLevels, a.addSibling},
		{"改字", iconPencil, func() { a.startEdit(n.ID) }},
		{task, iconCheck, func() { a.toggleTask(n) }},
		{"详情", iconPanel, func() { a.panelSheet = true }},
	}
	lifted(ui.Row(c.Key("phone-node-bar")).Absolute().Left(12).Right(12).Bottom(16).Height(62).Padding(0, 4).Radius(16).
		AlignItems(ui.Center).Background(cardColor(t)).Label("主题操作"), t).
		Transition(ui.ElementTransition{Duration: motionFast, Ease: easeDetent, Enter: &ui.Motion{Y: 16}}).Children(func() {
		for _, k := range keys {
			b := ui.Column(c.Key(k.name)).Grow(1).Basis(0).Height(54).Gap(3).Radius(12).Center().Cursor(ui.CursorPointer).Label(k.name)
			if b.Clicked() {
				a.later(k.do)
			}
			press(b, 0.94)
			b.Children(func() {
				ink := t.Text
				if k.name == "详情" {
					ink = t.Accent
				}
				glyph(c, k.ic, 18, ink)
				ui.Text(c, k.name).FontSize(textCaption - 1).TextColor(ink).PassThrough()
			})
		}
		// The rest of what is done to a topic, in a short menu over the key
		// (topicMoreMenu).
		look := phoneMenuLook()
		look.At, look.Self = ui.AnchorTopEnd, ui.AnchorBottomEnd // over the key, at the bar's end
		more := ui.Column(c.Key("更多")).Grow(1).Basis(0).Height(54).Gap(3).Radius(12).Center().Cursor(ui.CursorPointer).Label("更多").
			PopoverMenuButton(a.topicMoreMenu(n), look)
		press(more, 0.94)
		more.Children(func() {
			glyph(c, iconMore, 18, t.Text)
			ui.Text(c, "更多").FontSize(textCaption - 1).PassThrough()
		})
	})
}

// fingerPan moves the canvas with a finger dragging its blank: the map
// follows the finger, as a list follows it, and a finger landing stops a
// glide. (A topic does the same, carryTopic.)
func (a *app) fingerPan(c *ui.Context, el ui.Element, cam *camera) {
	if dx, dy, pressed := el.Dragged(); pressed {
		a.panBy(cam, dx, dy, el)
	}
}

// panBy moves the canvas by what a finger pressing el moved since the
// last frame, keeping how fast for the glide after it lets go.
func (a *app) panBy(cam *camera, dx, dy float32, el ui.Element) {
	if !a.panning || a.panSeen < a.frameN-1 { // a finger landing
		a.glide, a.panV = [2]float32{}, [2]float32{}
	}
	cam.X += dx
	cam.Y += dy
	if vx, vy := el.DragVelocity(); vx != 0 || vy != 0 {
		a.panV = [2]float32{vx, vy}
	}
	a.panning, a.panSeen = true, a.frameN
}

// panGlide carries the canvas on after a flick, slowing as a list does,
// and stops it when slow. A pan let go is one no frame since moved.
func (a *app) panGlide(c *ui.Context, cam *camera) {
	if a.panning && a.panSeen < a.frameN-1 {
		a.panning = false
		if math.Hypot(float64(a.panV[0]), float64(a.panV[1])) > 250 {
			a.glide = a.panV
		}
		a.panV = [2]float32{}
	}
	if a.glide == [2]float32{} || a.noMotion {
		a.glide = [2]float32{}
		return
	}
	const step = 1.0 / 60
	cam.X += a.glide[0] * step
	cam.Y += a.glide[1] * step
	a.glide[0] *= 0.93
	a.glide[1] *= 0.93
	if math.Hypot(float64(a.glide[0]), float64(a.glide[1])) < 30 {
		a.glide = [2]float32{}
	}
	c.Invalidate()
}

// carryTopic is a topic under a finger on a phone: held still for half a
// second (a long press) it is taken up, its ghost following the finger
// and the topic it would go under outlined, and let go there it goes
// under it, said with 撤销; a finger that moves at once pans the map.
// The desktop's drag of a topic (Drag) would hold every finger on a topic
// till a long press, so that the map could not be panned from one.
func (a *app) carryTopic(c *ui.Context, el ui.Element, n *model.Node, box layout.Rect, cam *camera) {
	z := cam.Zoom
	// Asked every frame, so that MyGo hands the topic a finger's moves,
	// and, held still, its moves after (rather than scrolling what is
	// under it, or taking the press for a long press of its own, which
	// would leave no move to carry the topic by).
	dx, dy, pressed := el.Dragged()
	if !pressed {
		if a.hold.id == n.ID {
			a.hold.id = ""
		}
		if a.carry == n.ID { // let go
			id, to := n.ID, a.carryOver
			a.carry, a.carryOver = "", ""
			if to != "" {
				a.later(func() { a.carryDrop(id, to) })
			}
		}
		return
	}
	if a.hold.id != n.ID { // a finger landing on it
		a.hold.id, a.hold.at, a.hold.moved = n.ID, c.Now(), false
	}
	switch {
	case a.carry == n.ID:
		a.carryAt[0] += dx
		a.carryAt[1] += dy
		a.carryOver = a.nodeAt((a.carryAt[0]-cam.X)/z, (a.carryAt[1]-cam.Y)/z, n)
	case dx != 0 || dy != 0:
		a.hold.moved = true
		a.panBy(cam, dx, dy, el)
	case !a.hold.moved && a.carry == "" && c.Now().Sub(a.hold.at) >= carryAfter:
		a.carry, a.carryOver = n.ID, ""
		a.carryAt = [2]float32{cam.X + (box.X+box.W/2)*z, cam.Y + (box.Y+box.H/2)*z}
		a.selected, a.multi = n.ID, nil
		a.glide = [2]float32{}
	case !a.hold.moved:
		c.Invalidate() // till the half second is up
	}
}

// carryAfter is how long a finger holds a topic still to take it up: a
// long press's, as the systems have it.
const carryAfter = 500 * time.Millisecond

// carryDrop puts a topic a finger carried under the topic it was let go
// over, and says so with the way back.
func (a *app) carryDrop(id, to string) {
	n, _ := a.doc.Find(id)
	t, _ := a.doc.Find(to)
	if n == nil || t == nil {
		return
	}
	edits := a.edits
	a.moveNode(id, to)
	if a.edits != edits {
		a.tellUndo("已移到「"+t.Title+"」下", a.stepBack())
	}
}

// phonePage is on a phone what a window over the page is on a desktop
// (modal): a page of its own over the place's, under a row with the way
// back, which back and Escape take too. A window there ran past the
// screen's edge with what it held, the user saw (0.108.1).
func phonePage(c *ui.Context, key string, open *bool, fn func()) ui.Element {
	if !*open {
		return ui.Box(c.Key(key)).Size(0, 0)
	}
	t := c.Theme()
	page := ui.Column(c.Key(key)).Absolute().Left(0).Top(0).Right(0).Bottom(0).ZIndex(40).Background(cardColor(t)).
		Transition(ui.ElementTransition{Duration: motionSlow, Ease: easeDetent, Enter: &ui.Motion{X: 24}})
	if c.Shortcut(0, ui.KeyEscape) {
		*open = false
	}
	page.Children(func() {
		sheetBack(c, func() { *open = false })
		ui.Column(c.Key("body")).Grow(1).Basis(0).MinHeight(0).Padding(12, pagePad(), 0).FillWidth().Children(fn)
	})
	return page
}

// phoneCovered tells whether what is shown on a phone is a page of its
// own over the place's, with no row of keys, bar nor places round it: a
// clip, read at length, as the draft has it.
func (a *app) phoneCovered() bool {
	return narrowLayout && a.libOpen && a.place() == placeCollect && !a.growShown() && a.lib.Find(a.libSel) != nil
}

// topicMoreMenu is a topic's 更多 on a phone: what its bar has not, as a
// short menu above the key, drawn in the app's look (phoneMenuLook). The
// desktop's menu, laid out by Android, put each submenu's every row in one
// list past the screen's foot (0.108.5); then a sheet from the foot rose
// under the finger that had tapped and went with it, showing a moment and
// nothing (the user's, 0.108.11).
func (a *app) topicMoreMenu(n *model.Node) func(m *ui.Menu) {
	return func(m *ui.Menu) {
		depth := max(len(a.shownDoc().Path(n.ID))-1, 0)
		if a.doc.IsFloating(n.ID) {
			depth++
		}
		// Six, the user's (0.108.5): what a phone does to a topic now and
		// then, with neither the bar's (子主题, 同级, 改字, 任务, 详情, where
		// its note, tags and links are) nor a desktop's (剪切, 聚焦此分支;
		// 图解 is among the views of 导图 ▼).
		if depth > 0 && m.Item("移到其他导图…").Chosen() {
			a.later(func() { a.openPalette(paletteMove) })
		}
		if len(n.Children) > 0 {
			label := "折叠下级"
			if n.Collapsed {
				label = "展开下级"
			}
			id := n.ID
			if m.Item(label).Chosen() {
				a.later(func() { a.toggleCollapse(id) })
			}
		}
		if m.Item("复制").Chosen() {
			a.later(func() { a.copyNodes(false) })
		}
		if m.Item("粘贴为子主题").Chosen() {
			a.later(func() { a.paste() })
		}
		if m.Item("AI 拆解成任务").Chosen() {
			a.later(a.aiBreakDown)
		}
		m.Separator()
		if m.Item("删除").Danger().Disabled(depth == 0).Chosen() {
			a.later(a.deleteSelection)
		}
	}
}

// phoneBack is Android's back on a phone where nothing open takes it
// (MyGo hands it to Escape, then to the back key, and leaves the app when
// neither is taken): a map goes back to the place it was opened from, a
// clip's page to 收集. It left the app from a map (the user's, 0.108.5).
func (a *app) phoneBack(c *ui.Context) {
	if a.libOpen {
		a.backTo = a.place() // where a map opened now is gone back to
	}
	switch {
	case a.phoneCovered():
		if c.Shortcut(0, ui.KeyBack) {
			a.libSel = ""
		}
	case a.libOpen && a.barOpen: // the bar open goes back to its line, what is written kept
		if c.Shortcut(0, ui.KeyEscape) {
			a.barOpen = false
		}
	case a.libOpen && (a.place() != placeToday || a.tasks.view != tasksToday || a.tasks.onMap != ""):
		// A place or a list other than today's goes back to it, and from
		// there out of the app, as Android's apps with places at their
		// foot do: back on 项目 left the app (0.108.8).
		if c.Shortcut(0, ui.KeyBack) {
			a.later(func() {
				a.goPlace(placeToday)
				a.tasks.view, a.tasks.onMap = tasksToday, ""
			})
		}
	case !a.libOpen && a.doc != nil && !a.presenting && a.editing == "":
		if c.Shortcut(0, ui.KeyBack) {
			to := a.backTo
			a.later(func() {
				if to == placeToday || to == placeCollect {
					a.goPlace(to)
					return
				}
				a.showProjects()
			})
		}
	}
}

// phoneMenuLook is a menu's look on a phone, drawn in the window (view
// sets every menu so): a card of the app's corners, rows a finger's height.
func phoneMenuLook() ui.PopoverMenuButtonOptions {
	return ui.PopoverMenuButtonOptions{
		Panel: func(panel ui.Element) { panel.Radius(14).Padding(6).MinWidth(176) },
		Item:  func(item ui.Element) { item.Padding(11, 14).Radius(10) },
	}
}
