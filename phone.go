package main

import (
	"fmt"
	"runtime"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/mobile"
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

// phonePlaces are the places at the window's foot, with their icons.
var phonePlaces = []struct {
	place      int
	name, icon string
}{{placeToday, "今天", "calendar"}, {placeMaps, "项目", "folder"}, {placeCollect, "收集", "layers"}, {placeSkills, "技能", "puzzle"}}

// phoneTabs is the row of the places at the window's foot: on the
// window's gray, as the top row, the place shown in the accent.
func (a *app) phoneTabs(c *ui.Context) {
	t := c.Theme()
	now := a.place()
	ui.Row(c.Key("phone-tabs")).Height(56).Padding(0, 6, 4).Shrink(0).FillWidth().AlignItems(ui.Stretch).Label("位置").Children(func() {
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
	case placeSkills:
		a.showSkills()
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
		ui.Column(c.Key("drawer")).Fill().Padding(top, 0, bottom).Background(cardColor(c.Theme())).Children(func() { a.sideColumn(c) })
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
func sheetBack(c *ui.Context, close func()) {
	t := c.Theme()
	ui.Row(c.Key("sheet-back")).Height(44).Padding(0, 8).Shrink(0).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(hairline(t)).Children(func() {
		b := ui.Row(c.Key("back")).Height(36).Padding(0, 10, 0, 4).Gap(2).Radius(radiusRow).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label("返回")
		if b.Clicked() {
			close()
		}
		press(b, 0.96)
		b.Children(func() {
			glyph(c, iconBack, 18, t.Accent)
			ui.Text(c, "返回").FontSize(textBody).TextColor(t.Accent).PassThrough()
		})
	})
}
