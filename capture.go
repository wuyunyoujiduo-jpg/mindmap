package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// Capturing thoughts as they come: a global shortcut opens a small window
// from any app; what is typed goes to the inbox, a map of its own, or
// under the node selected in the open map. The inbox is sorted out later,
// moving its nodes to the maps they belong in. An icon in the menu bar
// lists the tasks due today.

// inboxName is the inbox map's name, at the top of the maps folder.
const inboxName = "收件箱"

// defaultCaptureKey opens the capture window from anywhere.
const defaultCaptureKey = "Control+Alt+Space"

func (a *app) inboxPath() string { return filepath.Join(a.dir, model.FileName(inboxName)) }

// captureKey is the shortcut of the capture window.
func (a *app) captureKey() string {
	if a.cfg.CaptureKey != "" {
		return a.cfg.CaptureKey
	}
	return defaultCaptureKey
}

// Where captured text goes.
const (
	captureInbox   = iota
	captureLibrary // a clip of the library
	captureHere    // under the node selected in the open map
)

// capture adds text to the inbox, or under the node selected: a node a
// line, nested by indentation, as a pasted outline is.
func (a *app) capture(text string, where int) error {
	if where == captureLibrary {
		if _, err := a.addClipText(text, "快速记录"); err != nil {
			return err
		}
		a.tell("已收进资料库")
		return nil
	}
	nodes := model.ParseOutline(text)
	if len(nodes) == 0 {
		return nil
	}
	if where == captureHere && a.doc != nil {
		parent, _ := a.selectedNode()
		if parent == nil {
			parent = a.doc.Root
		}
		a.mutate("", func() {
			parent.Children = append(parent.Children, nodes...)
			parent.Collapsed = false
		})
		a.tell(fmt.Sprintf("已记到「%s」下", parent.Title))
		return nil
	}
	path := a.inboxPath()
	if path == a.path { // the inbox is open: as an edit, undoable
		a.mutate("", func() { a.doc.Root.Children = append(a.doc.Root.Children, nodes...) })
		a.tell("已记到收件箱")
		return nil
	}
	f, err := model.Load(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		f = model.NewFile(inboxName)
		f.Sheets[0].Root.Note = "随手记下的想法和待办，之后整理到对应的导图里（右键节点「移到其他导图…」）。"
	}
	f.Sheets[0].Root.Children = append(f.Sheets[0].Root.Children, nodes...)
	if err := f.Save(path); err != nil {
		return err
	}
	a.refreshFiles()
	a.tell("已记到收件箱")
	return nil
}

// inboxCount is how many items wait in the inbox.
func (a *app) inboxCount() int {
	path := a.inboxPath()
	f := a.file
	if path != a.path {
		f = a.indexed(path)
	}
	if f == nil {
		return 0
	}
	return len(f.Sheets[0].Root.Children)
}

// captureView is the capture window: a field, where it goes, and keys.
func (a *app) captureView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Padding(14).Gap(10).Background(t.Background).Children(func() {
		in := ui.TextArea(c.Key("capture"), &a.captureText).Placeholder("记下想法或待办…（↩ 保存，⇧↩ 换行，Esc 关闭；“- [ ] ” 开头记为任务）").
			Lines(2, 6).AutoFocus().FillWidth().Label("快速记录")
		save := func() {
			if strings.TrimSpace(a.captureText) == "" {
				return
			}
			if err := a.capture(a.captureText, a.captureWhere); err != nil {
				a.captureErr = err.Error()
				return
			}
			a.captureText, a.captureErr = "", ""
			a.plat.HideCapture()
		}
		in.HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyEnter && ev.Mods == 0 {
				save()
				return true
			}
			if ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyEscape {
				a.plat.HideCapture()
				return true
			}
			return false
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if ui.Button(c, "收藏剪贴板").FontSize(12).Tooltip("把剪贴板里的文字、链接、图片或文件收进资料库（" + keyLabel(a.clipKey()) + "）").Clicked() {
				if _, err := a.clipClipboard("剪贴板"); err != nil {
					a.captureErr = err.Error()
				} else {
					a.captureErr = ""
					a.plat.HideCapture()
				}
			}
			ui.Text(c, "记到").FontSize(12).TextColor(t.TextMuted)
			labels := []string{"收件箱", "资料库"}
			if n, _ := a.selectedNode(); n != nil && a.path != a.inboxPath() {
				labels = append(labels, "「"+mapName(a.path)+"」的「"+n.Title+"」下")
			}
			if a.captureWhere >= len(labels) {
				a.captureWhere = 0
			}
			ui.Segmented(c, &a.captureWhere, labels...).FontSize(12)
			ui.Spacer(c)
			if a.captureErr != "" {
				ui.Text(c, a.captureErr).FontSize(12).TextColor(t.Danger)
			}
			if ui.Button(c, "保存").Clicked() {
				save()
			}
		})
	})
}

// moveToMap moves the nodes selected under a node of another map (or this
// one), as a step of each map: the other is saved at once.
func (a *app) moveToMap(path, sheet, id string) {
	nodes := a.topSelection()
	if len(nodes) == 0 {
		return
	}
	if path == a.path {
		a.mutate("", func() {
			for _, n := range nodes {
				if n.Contains(id) {
					continue // not under itself
				}
				_ = a.doc.Move(n.ID, id, -1)
			}
		})
		if d := a.file.SheetOf(nodes[0].ID); d != nil && d.ID != a.doc.ID {
			a.showSheet(d.ID)
		}
		a.tell("已移动")
		return
	}
	f, err := model.Load(path)
	if err != nil {
		a.saveErr = err.Error()
		return
	}
	d := f.SheetOf(id)
	if d == nil {
		a.saveErr = "目标节点已不存在"
		return
	}
	target, _ := d.Find(id)
	from := model.AssetsDir(a.path)
	for _, n := range nodes {
		c := n.Copy()
		copyAssets(c, from, model.AssetsDir(path))
		target.Children = append(target.Children, c)
	}
	target.Collapsed = false
	if err := f.Save(path); err != nil {
		a.saveErr = err.Error()
		return
	}
	a.deleteSelection()
	a.tell(fmt.Sprintf("已移到「%s」的「%s」下", mapName(path), target.Title))
}

// trayEntry is a line of the menu bar icon's menu.
type trayEntry struct {
	label string
	run   func()
	sep   bool
}

// trayTasks are the tasks of every map due today or overdue, not done.
func (a *app) trayTasks() []trayTask {
	day := today(a.now())
	var out []trayTask
	for _, f := range a.files {
		file := a.file
		if f.Path != a.path {
			file = a.indexed(f.Path)
		}
		if file == nil {
			continue
		}
		for _, tn := range file.Tasks() {
			task := tn.Node.Task
			if task.Status == model.StatusDone || task.Due == nil || !task.Due.Before(day.AddDate(0, 0, 1)) {
				continue
			}
			out = append(out, trayTask{path: f.Path, sheet: tn.Sheet.ID, node: tn.Node, overdue: task.Due.Before(day)})
		}
	}
	return out
}

type trayTask struct {
	path, sheet string
	node        *model.Node
	overdue     bool
}

// trayMenu is the menu of the menu bar icon.
func (a *app) trayMenu() []trayEntry {
	items := []trayEntry{{label: "快速记录…", run: a.plat.ShowCapture},
		{label: "收藏剪贴板到资料库", run: a.clipFromShortcut},
		{label: fmt.Sprintf("资料库（%d）", len(a.lib.Clips)), run: func() {
			a.plat.ShowMain()
			a.showLibrary()
		}}}
	if k := a.inboxCount(); k > 0 {
		items = append(items, trayEntry{label: fmt.Sprintf("收件箱（%d）", k), run: func() {
			a.plat.ShowMain()
			_ = a.open(a.inboxPath())
		}})
	}
	tasks := a.trayTasks()
	items = append(items, trayEntry{sep: true})
	if len(tasks) == 0 {
		items = append(items, trayEntry{label: "今天没有到期的任务"})
	} else {
		items = append(items, trayEntry{label: fmt.Sprintf("今天到期和已逾期 %d", len(tasks))})
		for i, tt := range tasks {
			if i == 12 {
				items = append(items, trayEntry{label: fmt.Sprintf("还有 %d 个…", len(tasks)-i)})
				break
			}
			mark := "○ "
			if tt.overdue {
				mark = "⚠ "
			}
			label := mark + tt.node.Title
			if tt.path != a.path {
				label += "　· " + mapName(tt.path)
			}
			tt := tt
			items = append(items, trayEntry{label: label, run: func() {
				a.plat.ShowMain()
				a.goToNode(tt.path, tt.sheet, tt.node.ID)
			}})
		}
	}
	items = append(items, trayEntry{sep: true}, trayEntry{label: "打开 MindMap", run: a.plat.ShowMain})
	return items
}

// trayTitle is shown beside the icon: how many tasks are due, if any.
func (a *app) trayTitle() string {
	if n := len(a.trayTasks()); n > 0 {
		return fmt.Sprint(n)
	}
	return ""
}

// trayIcon draws the menu bar's icon: a small mind map, black on clear,
// which macOS tints.
func trayIcon() []byte {
	const s = 36
	img := image.NewNRGBA(image.Rect(0, 0, s, s))
	disc := func(cx, cy, r float64) {
		for y := 0; y < s; y++ {
			for x := 0; x < s; x++ {
				d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
				if a := r + 0.5 - d; a > 0 {
					img.Set(x, y, color.NRGBA{0, 0, 0, uint8(255 * min(a, 1))})
				}
			}
		}
	}
	line := func(x0, y0, x1, y1, w float64) {
		for i := 0; i <= 60; i++ {
			f := float64(i) / 60
			disc(x0+(x1-x0)*f, y0+(y1-y0)*f, w)
		}
	}
	line(18, 18, 6, 8, 1.4)
	line(18, 18, 6, 28, 1.4)
	line(18, 18, 30, 8, 1.4)
	line(18, 18, 30, 28, 1.4)
	disc(18, 18, 6)
	for _, p := range [][2]float64{{5, 8}, {5, 28}, {31, 8}, {31, 28}} {
		disc(p[0], p[1], 3.4)
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

// openSettings opens the settings, with the fields as saved.
func (a *app) openSettings() {
	a.settingsOpen = true
	a.captureKeyDraft, a.ghTokenDraft, a.clipKeyDraft = a.cfg.CaptureKey, a.cfg.GitHubToken, a.cfg.ClipKey
}

// settingsView is the window of preferences: the capture shortcut, the
// menu bar icon, reminders and the editor of code links.
func (a *app) settingsView(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c.Key("settings"), &a.settingsOpen, func() {
		w, _ := c.Size()
		ui.Column(c).Gap(16).Width(min(560, w-80)).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "设置").FontSize(17).Bold().Grow(1)
				if ui.Button(c, "关闭").Clicked() {
					a.settingsOpen = false
				}
			})
			ui.Field(c, "快速记录的全局快捷键", func() {
				ui.Column(c).Gap(6).Children(func() {
					in := ui.TextInput(c.Key("capture-key"), &a.captureKeyDraft).Placeholder(defaultCaptureKey).Label("快速记录快捷键")
					if in.Submitted() || in.PressedOutside() && a.captureKeyDraft != a.cfg.CaptureKey {
						a.cfg.CaptureKey = strings.TrimSpace(a.captureKeyDraft)
						a.cfg.save()
						if err := a.plat.SetCaptureKey(a.captureKey()); err != nil {
							a.saveErr = "快捷键无效：" + err.Error()
						}
					}
					ui.Text(c, "在任何应用里按下它，弹出一个小窗口记下想法。格式如 Control+Alt+Space、CmdOrCtrl+Shift+M；留空用默认（⌃⌥Space）。").
						FontSize(12).TextColor(t.TextMuted).Wrap()
				})
			})
			ui.Field(c, "收藏剪贴板的全局快捷键", func() {
				ui.Column(c).Gap(6).Children(func() {
					in := ui.TextInput(c.Key("clip-key"), &a.clipKeyDraft).Placeholder(defaultClipKey).Label("收藏剪贴板快捷键")
					if in.Submitted() || in.PressedOutside() && a.clipKeyDraft != a.cfg.ClipKey {
						a.cfg.ClipKey = strings.TrimSpace(a.clipKeyDraft)
						a.cfg.save()
						if err := a.plat.SetClipKey(a.clipKey()); err != nil {
							a.saveErr = "快捷键无效：" + err.Error()
						}
					}
					ui.Text(c, "在任何应用里复制文字、链接、图片或文件后按下它，直接收进资料库。留空用默认（⌃⌥V）。").
						FontSize(12).TextColor(t.TextMuted).Wrap()
				})
			})
			auto := !a.cfg.LibraryManual
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				if ui.Switch(c, &auto).Label("自动整理资料").Changed() {
					a.cfg.LibraryManual = !auto
					a.cfg.save()
				}
				ui.Text(c, "资料库：新收藏的资料由 AI 自动起标题、写摘要、分类和打标签（用 AI 助手的 API Key，按用量计费）").FontSize(13).Wrap().Grow(1)
			})
			tray := !a.cfg.NoTray
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				if ui.Switch(c, &tray).Label("菜单栏图标").Changed() {
					a.cfg.NoTray = !tray
					a.cfg.save()
					a.plat.SetTray(tray)
				}
				ui.Text(c, "菜单栏图标：显示今天到期和已逾期的任务数，点开列出任务、快速记录").FontSize(13).Wrap().Grow(1)
			})
			on := !a.cfg.NoReminders
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				if ui.Switch(c, &on).Label("到期提醒").Changed() {
					a.cfg.NoReminders = !on
					a.cfg.save()
				}
				ui.Text(c, "到期提醒：任务今天到期或已逾期时，每天发一次系统通知").FontSize(13).Wrap().Grow(1)
			})
			ui.Field(c, "GitHub Token（同步私有仓库的 issues 需要）", func() {
				in := ui.TextInput(c.Key("gh-token"), &a.ghTokenDraft).Password().Placeholder("ghp_… 或 github_pat_…").Label("GitHub Token")
				if in.Submitted() || in.PressedOutside() && a.ghTokenDraft != a.cfg.GitHubToken {
					a.cfg.GitHubToken = strings.TrimSpace(a.ghTokenDraft)
					a.cfg.save()
				}
			})
			ui.Divider(c)
			if a.file != nil {
				a.projectSection(c)
			}
		})
	})
}
