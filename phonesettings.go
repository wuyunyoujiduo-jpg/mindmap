package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

// The settings on a phone are its own, as its draft has them (the user's,
// 0.108.6: the parts as six pills over the page, its name said twice, and
// a block cut at the screen's side): a list of the parts, each a row that
// opens a page of its own, ‹ 设置 to come back, and the version at the
// list's foot, where it is looked for. A part's page scrolls as one; none
// holds what a computer's alone has (the ground, the menu bar, the
// shortcuts, the screenshot, the icons, the agents).

// settingsIcons are the parts' icons, of Kubai's.
var settingsIcons = map[string]string{"通用": "sliders", "资料整理": "sparkles", "备份": "save", "同步": "sync",
	"加密": "key-round", "连接": "plug", "AI 助手": "sparkles", "当前导图": "sitemap"}

// phoneSettings is the settings as a page of a phone's: the list of the
// parts while a.settingsHome, else the part a.settingsTab.
func (a *app) phoneSettings(c *ui.Context, tabs []string) {
	t := c.Theme()
	if !a.settingsOpen {
		ui.Box(c.Key("settings")).Size(0, 0)
		return
	}
	home := a.settingsHome
	back := func() {
		if home {
			a.settingsOpen = false
		} else {
			a.settingsHome = true
		}
	}
	page := ui.Column(c.Key("settings")).Absolute().Left(0).Top(0).Right(0).Bottom(0).ZIndex(40).Background(cardColor(t)).
		Transition(ui.ElementTransition{Duration: motionSlow, Ease: easeDetent, Enter: &ui.Motion{X: 24}})
	if c.Shortcut(0, ui.KeyEscape) { // a phone's back: a part to the list, the list away
		back()
	}
	page.Children(func() {
		to := "返回"
		if !home {
			to = "设置"
		}
		sheetBackTo(c, to, back)
		name := "设置"
		if !home {
			name = tabs[a.settingsTab]
		}
		ui.Scroll(c.Key("settings-page:"+name)).Grow(1).Basis(0).MinHeight(0).FillWidth().Gap(22).Padding(14, 16, 30).Label(name).Children(func() {
			ui.Text(c, name).FontSize(30).FontWeight(700).LetterSpacing(-0.7).Margin(0, 4)
			if home {
				a.settingsList(c, tabs)
				a.updateSettings(c)
				return
			}
			a.settingsPart(c, tabs[a.settingsTab])
		})
	})
}

// settingsList is the list of the parts: what is set once, what keeps
// the maps, what reaches out; each with how it stands, said short.
func (a *app) settingsList(c *ui.Context, tabs []string) {
	at := func(name string) int {
		for i, n := range tabs {
			if n == name {
				return i
			}
		}
		return -1
	}
	open := func(name string) func() {
		return func() { a.settingsTab, a.settingsHome = at(name), false }
	}
	type part struct {
		name, says string
		tap        func()
	}
	libSays := "自动"
	if a.cfg.LibraryManual {
		libSays = "手动"
	}
	syncSays := "未设置"
	if a.syncReady() {
		syncSays = a.syncLabel()
	}
	backupSays := "未设置"
	if a.cfg.BackupDir != "" {
		backupSays = "每天"
	}
	vaultSays := "未开启"
	if a.vault.On() {
		vaultSays = "已开启"
	}
	groups := [][]part{
		{{"通用", "", open("通用")}, {"资料整理", libSays, open("资料整理")}},
		{{"同步", syncSays, open("同步")}, {"备份", backupSays, open("备份")}, {"加密", vaultSays, open("加密")}},
	}
	if onPhone { // 连接 holds the AI alone on a phone: its row opens it
		groups = append(groups, []part{{"AI 助手", providerModelName(a.cfg), func() {
			a.settingsOpen, a.aiOpen, a.ai.settings = false, true, true
			a.loadKeyDrafts()
		}}})
	} else {
		last := []part{{"连接", "", open("连接")}}
		if at("当前导图") >= 0 {
			last = append(last, part{"当前导图", "", open("当前导图")})
		}
		groups = append(groups, last)
	}
	for gi, g := range groups {
		raise(ui.Column(c.Key(fmt.Sprint("parts", gi))).Radius(radiusCard).FillWidth(), c.Theme()).Children(func() {
			for i, p := range g {
				if navRow(c, settingsIcons[p.name], p.name, p.says, i == len(g)-1).Clicked() {
					p.tap()
				}
			}
		})
	}
}

// navRow is a row of a phone's list that opens a page: its icon, its
// name, how it stands, and a chevron; the line under it starts at its name.
func navRow(c *ui.Context, icon, name, says string, last bool) ui.Element {
	t := c.Theme()
	row := ui.Row(c.Key("nav:" + name)).Gap(12).AlignItems(ui.Center).FillWidth().Cursor(ui.CursorPointer).Label(name)
	in := float32(14)
	if icon == "" { // in a block of rows, which has its own sides (rowsField)
		in = 0
	}
	row.Padding(0, 0, 0, in)
	press(row, 0.985)
	row.Children(func() {
		if icon != "" {
			ui.Box(c).Size(30, 30).Radius(8).Background(t.Accent.Alpha(0.1)).Center().Shrink(0).PassThrough().Children(func() {
				kubaiIcon(c, icon, 20, false)
			})
		}
		line := ui.Row(c).MinHeight(52).Padding(0, in, 0, 0).Gap(8).AlignItems(ui.Center).Grow(1).MinWidth(0).PassThrough()
		if !last {
			line.BorderWidth(0, 0, 1, 0).BorderColor(hairline(t))
		}
		line.Children(func() {
			ui.Text(c, name).FontSize(textBody + 1).Grow(1).Shrink(1).MinWidth(0)
			if says != "" {
				ui.Text(c, says).FontSize(textBody).TextColor(t.TextMuted).MaxLines(1).Shrink(1).MinWidth(0)
			}
			glyph(c, iconForward, 14, faint(t))
		})
	})
	return row
}
