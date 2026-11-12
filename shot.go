package main

import (
	"runtime"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/screenshot"
)

// Screenshots, from OneShot: frame a part of the screen, mark it up, and
// keep it in the library, copy it or save it.

// defaultShotKey takes a screenshot from any app: ⌃⌘A on macOS, as
// OneShot's, Ctrl+Alt+A elsewhere, as QQ's and WeChat's on Windows.
var defaultShotKey = func() string {
	if runtime.GOOS == "darwin" {
		return "Control+Command+A"
	}
	return "Control+Alt+A"
}()

func (a *app) shotKey() string {
	if a.cfg.ShotKey != "" {
		return a.cfg.ShotKey
	}
	return defaultShotKey
}

// screenshot takes one from inside the app, the window stepping aside so
// that what is behind it can be framed.
func (a *app) screenshot() { a.plat.Screenshot(true) }

// showShotAlert says what kept a screenshot from being taken; settings
// offers the system's settings of screen recording.
func (a *app) showShotAlert(title, msg string, settings bool) {
	a.shotAlert, a.shotAlertTitle, a.shotAlertMsg, a.shotAlertSettings = true, title, msg, settings
}

// shotAlertView is that message.
func (a *app) shotAlertView(c *ui.Context) {
	if !a.shotAlert {
		return
	}
	if !a.shotAlertSettings {
		ui.AlertDialog(c, &a.shotAlert, a.shotAlertTitle, a.shotAlertMsg, "好")
		return
	}
	if ui.AlertDialog(c, &a.shotAlert, a.shotAlertTitle, a.shotAlertMsg, "以后再说", "打开系统设置") == 1 {
		a.plat.OpenShotSettings()
	}
}

// keepShot does what was chosen with a screenshot: into the library, where
// it is shown when the shot was taken from the app, or onto the clipboard.
func (a *app) keepShot(png []byte, act screenshot.Action, fromApp bool) {
	switch act {
	case screenshot.Library:
		clip, err := a.addClipImage(png, "截图")
		if err != nil {
			a.plat.Notify("截图没有存到收集", err.Error())
			return
		}
		if fromApp {
			a.showLibrary()
			a.libSel = clip.ID
		} else {
			a.plat.Notify("截图已存到收集", clip.Label())
		}
		a.tell("截图已存到收集")
	case screenshot.Copy:
		a.tell("截图已复制")
	}
}

// shotSettings says how screenshots are taken, and tries one. Their log
// is a menu item away (帮助 → 在访达中显示截图记录), not shown.
func (a *app) shotSettings(c *ui.Context) {
	t := c.Theme()
	rowsField(c, "截图", "屏幕定格后直接框选，可拖动、拉边调整；框选后加框线、箭头、画笔、文字，可选颜色。✓ 或双击复制到剪贴板，书签按钮存到收集。", func() {
		setRow(c, "截一张", "从这里开始，或者用上面的截图快捷键", func() {
			if button(c, "试一下").Clicked() {
				a.settingsOpen = false
				a.later(a.screenshot)
			}
		})
		setLine(c)
		setRow(c, "菜单展开时截图", "别的应用的菜单展开时，macOS 不把快捷键交给应用：给 MyMind「输入监控」权限（授权后重启）", func() {
			if screenshot.KeysAllowed() {
				ui.Text(c, "✓ 菜单展开时也能用快捷键截图").FontSize(textSmall).TextColor(t.Success)
				return
			}
			if button(c, "让菜单展开时也能截图…").Tooltip("需要「输入监控」权限：只用来在菜单展开时认出截图快捷键").Clicked() {
				screenshot.RequestKeys()
				a.later(func() { a.plat.SetShotKey(a.shotKey()) })
			}
		})
	})
}
