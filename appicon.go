package main

import (
	"bytes"
	"embed"
	"image/png"

	"github.com/egoist/mygo/ui"
)

// The app's icon, chosen in the settings: the library's cards with the
// AI's spark, the default and the one in the app bundle, or knowledge in
// orbit around the spark. The choice shows in the Dock (macOS) or the
// taskbar (Windows) while the app runs.

//go:embed appicons/*.png
var appIconFiles embed.FS

type appIcon struct{ key, name, file, tray string }

var appIcons = []appIcon{
	{"", "知识卡片", "appicons/cards.png", "appicons/tray-cards.png"},
	{"orbit", "知识星图", "appicons/orbit.png", "appicons/tray-orbit.png"},
}

// trayIcon is the menu bar's icon of the key: black on clear, which macOS
// tints.
func trayIcon(key string) []byte {
	ic := appIcons[0]
	for _, i := range appIcons {
		if i.key == key {
			ic = i
		}
	}
	data, _ := appIconFiles.ReadFile(ic.tray)
	return data
}

// appIconPNG is the icon of the key, the default for one unknown.
func appIconPNG(key string) []byte {
	for _, ic := range appIcons {
		if ic.key == key {
			data, _ := appIconFiles.ReadFile(ic.file)
			return data
		}
	}
	data, _ := appIconFiles.ReadFile(appIcons[0].file)
	return data
}

// setIcon makes the icon of the key the app's, and remembers it.
func (a *app) setIcon(key string) {
	if a.cfg.Icon == key {
		return
	}
	a.cfg.Icon = key
	a.cfg.save()
	a.plat.SetAppIcon(appIconPNG(key), trayIcon(key))
}

// iconBitmap is an icon's picture for the settings, decoded once.
func (a *app) iconBitmap(key string) *ui.Bitmap {
	if b, ok := a.iconBitmaps[key]; ok {
		return b
	}
	img, err := png.Decode(bytes.NewReader(appIconPNG(key)))
	if err != nil {
		return nil
	}
	if a.iconBitmaps == nil {
		a.iconBitmaps = map[string]*ui.Bitmap{}
	}
	a.iconBitmaps[key] = ui.NewBitmap(img)
	return a.iconBitmaps[key]
}

// iconSettings lets the app's icon be chosen.
func (a *app) iconSettings(c *ui.Context) {
	t := c.Theme()
	ui.Field(c, "应用图标", func() {
		ui.Column(c).Gap(6).Children(func() {
			ui.Row(c).Gap(12).Children(func() {
				for _, ic := range appIcons {
					on := a.cfg.Icon == ic.key
					box := ui.Column(c.Key("icon:"+ic.name)).Padding(8, 10).Gap(6).Radius(12).AlignItems(ui.Center).
						Cursor(ui.CursorPointer).Label("图标 " + ic.name)
					switch {
					case on:
						box.Background(t.Accent.Alpha(0.1)).Border(2, t.Accent)
					case box.Hovered():
						box.Background(t.Text.Alpha(0.05)).Border(2, ui.RGBA(0, 0, 0, 0))
					default:
						box.Border(2, ui.RGBA(0, 0, 0, 0))
					}
					if box.Clicked() {
						key := ic.key
						a.later(func() { a.setIcon(key) })
					}
					box.Children(func() {
						if b := a.iconBitmap(ic.key); b != nil {
							ui.Image(c, b).Size(72, 72).PassThrough()
						}
						col := t.TextMuted
						if on {
							col = t.Accent
						}
						ui.Text(c, ic.name).FontSize(textSmall).TextColor(col).PassThrough()
					})
				}
			})
			ui.Text(c, "换上的图标在程序坞（Windows 是任务栏）和菜单栏里显示；访达和启动台里仍是默认的「知识卡片」。").
				FontSize(12).TextColor(t.TextMuted).Wrap()
		})
	})
}

// updateSettings shows the version, and checks for a newer one.
func (a *app) updateSettings(c *ui.Context) {
	ui.Field(c, "版本", func() {
		ui.Column(c).Gap(6).Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, appName+" "+version).FontSize(textBody).Grow(1)
				if ui.Button(c, "检查更新…").Clicked() {
					a.plat.CheckUpdates()
				}
			})
			auto := a.plat.AutoUpdates()
			switchRow(c, &auto, "自动检查更新", "每天检查一次；有新版本时显示更新说明，点「安装更新」后自动下载并重启", func() {
				a.plat.SetAutoUpdates(auto)
			})
		})
	})
}
