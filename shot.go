package main

import (
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/screenshot"
)

// Screenshots, from OneShot: frame a part of the screen, mark it up, and
// keep it in the library, copy it or save it.

// defaultShotKey takes a screenshot from any app.
const defaultShotKey = "Control+Alt+A"

func (a *app) shotKey() string {
	if a.cfg.ShotKey != "" {
		return a.cfg.ShotKey
	}
	return defaultShotKey
}

// screenshot takes one from inside the app, the window stepping aside so
// that what is behind it can be framed.
func (a *app) screenshot() { a.plat.Screenshot(true) }

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
