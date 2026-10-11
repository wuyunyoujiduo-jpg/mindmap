package main

import (
	"runtime"
	"strings"

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
			a.plat.Notify("截图没有收进资料库", err.Error())
			return
		}
		if fromApp {
			a.showLibrary()
			a.libSel = clip.ID
		} else {
			a.plat.Notify("截图已收进资料库", clip.Label())
		}
		a.tell("截图已收进资料库")
	case screenshot.Copy:
		a.tell("截图已复制")
	}
}

// shotSettings says how screenshots are taken, tries one, and shows the
// end of their log, to be copied when one goes wrong.
func (a *app) shotSettings(c *ui.Context) {
	t := c.Theme()
	ui.Field(c, "截图", func() {
		ui.Column(c).Gap(8).Children(func() {
			ui.Text(c, "屏幕定格后直接框选，可拖动、拉边调整；框选后加框线、箭头、画笔、文字，可选颜色。✓、回车或双击复制到剪贴板，书签按钮或 ⇧↩ 收进资料库，⌘S 保存，Esc 取消。").
				FontSize(12).TextColor(t.TextMuted).Wrap()
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if ui.Button(c, "试一下").Clicked() {
					a.settingsOpen = false
					a.later(a.screenshot)
				}
				if ui.Button(c, "复制截图记录").Tooltip("截图不成功时，把这段记录发给开发者").Clicked() {
					a.plat.CopyText(a.plat.ShotLog())
					a.tell("截图记录已复制")
				}
			})
			if log := strings.TrimSpace(a.plat.ShotLog()); log != "" {
				ui.Box(c.Key("shot-log")).Padding(8, 10).Radius(8).Background(t.Text.Alpha(0.05)).FillWidth().Children(func() {
					ui.Text(c, log).FontSize(11).TextColor(t.TextMuted).Wrap().Selectable()
				})
			}
		})
	})
}
