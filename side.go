package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
)

// Every place has a column at the leaf's left that says what of it shows
// (sidePane): 今天's lists and the maps to narrow by, 项目's directory,
// 收集's kinds and tags. It is the one way to narrow a page, where there
// were three (tabs, a menu, chips); it can be put away (config.HideTree).

// sideWidthOf is the width of that column.
const sideWidthOf = 228

// sideShown tells whether the left column shows.
func (a *app) sideShown() bool { return !a.cfg.HideTree && !a.presenting }

// sidePane is the left column: what body builds, which scrolls, over what
// foot builds, if anything.
func (a *app) sidePane(c *ui.Context, key string, body, foot func()) {
	t := c.Theme()
	ui.Column(c.Key(key)).Width(sideWidthOf).Shrink(0).FillHeight().BorderWidth(0, 1, 0, 0).BorderColor(hairline(t)).Children(func() {
		ui.Scroll(c.Key("scroll")).Grow(1).Children(func() {
			ui.Column(c.Key("list")).Padding(10, 8, 14).Gap(1).FillWidth().Children(body)
		})
		if foot != nil {
			ui.Column(c.Key("foot")).Padding(6, 8, 8).Gap(2).Shrink(0).Children(foot)
		}
	})
}

// sideRow is a row of the left column: what lead builds, a name, a count
// at its end; the one shown is on a shade. The caller acts on its click.
func sideRow(c *ui.Context, key, name, count string, on bool, lead func(hot bool)) ui.Element {
	t := c.Theme()
	row := ui.Row(c.Key(key)).Height(36).Padding(0, 10).Gap(10).Radius(radiusRow).AlignItems(ui.Center).FillWidth().
		Cursor(ui.CursorPointer).Label(name).Background(t.Text.Alpha(0)).Transition(easeLook)
	switch {
	case on:
		row.Background(t.Text.Alpha(0.07))
	case row.Hovered():
		row.Background(t.Text.Alpha(0.04))
	}
	row.Children(func() {
		if lead != nil {
			lead(on || row.Hovered())
		}
		txt := ui.Text(c, name).FontSize(textBody + 0.5).SingleLine().Grow(1).Shrink(1).MinWidth(0).PassThrough()
		if on {
			txt.FontWeight(600)
		} else if !row.Hovered() {
			txt.TextColor(ink2(t)) // read second, until it is chosen or pointed at
		}
		if count != "" && count != "0" {
			ui.Text(c, count).FontSize(textCaption).TextColor(faint(t)).FontFeatures("tnum").PassThrough()
		}
	})
	return row
}

// sideIconSize is the room an icon takes before a row's name, a map's dot too.
const sideIconSize = 22

// sideIcon is a row's icon, lit when the row is chosen or pointed at.
func sideIcon(c *ui.Context, name string) func(bool) {
	return func(hot bool) { kubaiIcon(c, name, sideIconSize, hot) }
}

// sideHead is the small name of a part of the left column.
func sideHead(c *ui.Context, text string) {
	ui.Text(c, text).FontSize(textCaption).FontWeight(600).LetterSpacing(0.4).TextColor(faint(c.Theme())).Margin(14, 10, 6)
}

// sideLine is the thin line between two parts of the left column.
func sideLine(c *ui.Context) {
	ui.Box(c).Height(1).FillWidth().Margin(8, 4).Shrink(0).Background(hairline(c.Theme()))
}

// todaySide is 今天's column: its lists, what waits for a place, and the
// maps to see the tasks of alone.
func (a *app) todaySide(c *ui.Context) {
	all := a.taskItems()
	items := all
	counts := a.taskListCounts(items)
	a.sidePane(c, "today-side", func() {
		for _, l := range taskLists {
			if sideRow(c, "list:"+l.name, l.name, counts[l.view], a.tasks.view == l.view, sideIcon(c, l.icon)).Clicked() {
				a.tasks.view, a.tasks.onMap = l.view, ""
			}
		}
		if n := len(a.waiting()); n > 0 || a.tasks.view == tasksWaiting {
			if sideRow(c, "list:waiting", "等着归位", fmt.Sprint(n), a.tasks.view == tasksWaiting, sideIcon(c, "import")).Clicked() {
				a.tasks.view = tasksWaiting
			}
		}
		open := map[string]int{}
		for _, it := range all {
			if !it.done() {
				open[it.path]++
			}
		}
		sideLine(c)
		sideHead(c, "按项目")
		for _, fi := range a.files {
			path := fi.Path
			if n := open[path]; n > 0 || a.tasks.onMap == path {
				col := a.mapColor(path)
				row := sideRow(c, "map:"+path, mapName(path), fmt.Sprint(n), a.tasks.onMap == path, func(bool) {
					ui.Box(c).Size(sideIconSize, 14).Center().Shrink(0).PassThrough().Children(func() { mapDot(c, col, 8) })
				})
				row.Label("只看 " + mapName(path))
				if row.Clicked() {
					if a.tasks.onMap == path {
						a.tasks.onMap = ""
					} else {
						a.tasks.onMap = path
						if a.tasks.view == tasksWaiting || a.tasks.view == tasksInbox {
							a.tasks.view = tasksAll
						}
					}
				}
			}
		}
	}, nil)
}

// libSide is 收集's column: its kinds, the ways to look back, and its
// tags, every one of them.
func (a *app) libSide(c *ui.Context) {
	l := a.lib
	a.sidePane(c, "lib-side", func() {
		for _, k := range []struct {
			kind       int
			name, icon string
		}{{kindAll, "全部", "layers"}, {kindMemos, "灵感", "lightbulb"}, {kindLinks, "链接", "link"}, {kindDocs, "文档", "file-text"}, {kindImages, "图片", "image"}} {
			n := 0
			for _, cl := range l.Clips {
				if ofKind(cl, k.kind) {
					n++
				}
			}
			on := a.libKind == k.kind && a.libReview == reviewOff && a.libCat == "" && a.libTag == ""
			row := sideRow(c, "kind:"+k.name, k.name, fmt.Sprint(n), on, sideIcon(c, k.icon))
			if row.Clicked() {
				a.libKind, a.libReview, a.libCat, a.libTag = k.kind, reviewOff, "", ""
			}
		}
		sideLine(c)
		if hasStarred(l) {
			if sideRow(c, "cat:★", "星标", "", a.libCat == "★", sideIcon(c, "star")).Clicked() {
				a.libCat, a.libReview = toggled(a.libCat, "★"), reviewOff
			}
		}
		if k := len(l.Unorganized()); k > 0 {
			if sideRow(c, "cat:-", "未整理", fmt.Sprint(k), a.libCat == "-", sideIcon(c, "archive")).Label(fmt.Sprintf("未整理 %d", k)).Clicked() {
				a.libCat, a.libReview = toggled(a.libCat, "-"), reviewOff
			}
		}
		// Looking back at the old notes, as flomo does.
		if sideRow(c, "review:daily", "每日回顾", "", a.libReview == reviewDaily, sideIcon(c, "history")).Tooltip("几条以前的灵感，每天换").Clicked() {
			a.libReview, a.libSel = reviewDaily, ""
		}
		if sideRow(c, "review:walk", "随机漫步", "", a.libReview == reviewWalk, sideIcon(c, "shuffle")).Tooltip("一次看一条以前的灵感").Clicked() {
			a.libReview, a.libSel = reviewWalk, ""
		}
		if tags := a.clipTags(); len(tags) > 0 {
			sideLine(c)
			sideHead(c, "标签")
			for i, tc := range tags {
				if i == 80 {
					break
				}
				name := tc.name
				row := sideRow(c, "tag:"+name, "#"+name, fmt.Sprint(tc.n), a.libTag == name, nil)
				row.Label(fmt.Sprintf("#%s %d", name, tc.n))
				if row.Clicked() {
					a.libTag, a.libReview = toggled(a.libTag, name), reviewOff
				}
			}
		}
	}, nil)
}

// toggled is what a choice becomes when one is clicked: it, or none when
// it was the one chosen.
func toggled(now, clicked string) string {
	if now == clicked {
		return ""
	}
	return clicked
}

// hasStarred tells whether any clip is starred.
func hasStarred(l *library.Library) bool {
	for _, cl := range l.Clips {
		if cl.Starred {
			return true
		}
	}
	return false
}
