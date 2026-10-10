package main

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// view builds the window: the files and tags on the left, the map or the
// tasks in the middle, and the selected node's details on the right.
func (a *app) view(c *ui.Context) {
	a.tick(c)
	t := c.Theme()
	if a.presenting && a.doc != nil {
		a.presentView(c)
		a.runPending()
		return
	}
	ui.Row(c).Fill().AlignItems(ui.Stretch).Background(t.Background).Children(func() {
		ui.Split(c, &a.sidebarWidth, func() { a.sidebar(c) }, func() {
			ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
				ui.Column(c).Grow(1).Children(func() {
					a.toolbar(c)
					ui.Divider(c)
					switch {
					case a.doc == nil:
						a.empty(c)
					case a.mode == viewMap:
						a.canvasView(c)
					case a.mode == viewOutline:
						ui.Column(c).Grow(1).Children(func() {
							a.outlineView(c)
							ui.Divider(c)
							a.sheetTabs(c)
						})
					case a.mode == viewBoard:
						a.boardView(c)
					default:
						a.agendaView(c)
					}
				})
				ui.Box(c).Width(1).FillHeight().Background(t.Border)
				a.panel(c)
			})
		}).Fill()
	})
	if len(a.pending) > 0 {
		a.runPending()
		c.Invalidate()
	}
}

func mapName(path string) string {
	return strings.TrimSuffix(filepath.Base(path), model.Ext)
}

func (a *app) empty(c *ui.Context) {
	ui.Column(c).Grow(1).Center().Gap(12).Children(func() {
		ui.Text(c, "没有打开的导图").FontSize(16).TextColor(c.Theme().TextMuted)
		if ui.PrimaryButton(c, "新建导图").Clicked() {
			a.newMap()
		}
	})
}

// tagChip draws a tag as a small pill, colored by its name.
func tagChip(c *ui.Context, tag, label string, active bool, size float32) ui.Element {
	col := tagColor(tag)
	chip := ui.Box(c).Padding(size*0.15, size*0.55).Radius(999).Cursor(ui.CursorPointer)
	if active {
		chip.Background(col).Children(func() {
			ui.Text(c, label).FontSize(size).TextColor(ui.Hex("#ffffff")).NoWrap()
		})
	} else {
		chip.Background(col.Alpha(0.14)).Children(func() {
			ui.Text(c, label).FontSize(size).TextColor(col).NoWrap()
		})
	}
	return chip
}

var tagPalette = []string{"#2563eb", "#16a34a", "#d97706", "#db2777", "#7c3aed", "#0891b2", "#dc2626", "#4d7c0f"}

func tagColor(tag string) ui.Color {
	var h uint32 = 2166136261
	for _, b := range []byte(tag) {
		h = (h ^ uint32(b)) * 16777619
	}
	return ui.Hex(tagPalette[h%uint32(len(tagPalette))])
}

func (a *app) toolbar(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Padding(8, 12).Gap(10).Children(func() {
		if a.doc != nil {
			ui.Text(c, mapName(a.path)).Bold().FontSize(15).NoWrap().MaxWidth(220).Ellipsis("…")
			status, col := "已自动保存", t.TextMuted
			switch {
			case a.saveErr != "":
				status, col = a.saveErr, t.Danger
			case a.dirty:
				status = "编辑中，稍后自动保存…"
			case !a.savedAt.IsZero():
				status = "已自动保存 " + a.savedAt.Format("15:04")
			}
			ui.Text(c, status).FontSize(12).TextColor(col).NoWrap().MaxWidth(240).Ellipsis("…").
				Tooltip("编辑停顿后自动保存到文件，无需手动保存")
		}
		ui.Spacer(c)
		if a.doc == nil {
			return
		}
		ui.Segmented(c, &a.mode, "思维导图", "大纲", "看板", "日程")
		if ui.Button(c, "▶").Label("开始演说").Tooltip("开始演说 ⌘↩").Width(32).Clicked() {
			a.later(func() { a.startPresenting("") })
		}
		search := ui.TextInput(c.Key("search"), &a.search).Placeholder("搜索…").Tooltip("搜索标题、备注、标签，回车跳到下一个").Label("搜索").Width(150)
		if a.search != "" && search.Shortcut(0, ui.KeyEscape) {
			a.search = ""
		}
		if search.Submitted() {
			a.mode = viewMap
			a.nextMatch()
		}
		ai := ui.Button(c, "✦ AI").Label("AI 助手").Tooltip("AI 助手")
		if a.aiOpen {
			ai.Background(t.Accent).TextColor(t.AccentText)
		}
		if ai.Clicked() {
			a.aiOpen = !a.aiOpen
		}
		ui.Row(c).Gap(2).Children(func() {
			if ui.Button(c, "↶").Label("撤销").Tooltip("撤销 ⌘Z").Width(32).Disabled(!a.hist.CanUndo()).Clicked() {
				a.undo()
			}
			if ui.Button(c, "↷").Label("重做").Tooltip("重做 ⌘⇧Z").Width(32).Disabled(!a.hist.CanRedo()).Clicked() {
				a.redo()
			}
		})
	})
}
