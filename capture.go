package main

import (
	"errors"
	"fmt"
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
	if _, err := os.Stat(path); os.IsNotExist(err) {
		f := model.NewFile(inboxName)
		f.Sheets[0].Root.Note = "随手记下的想法和待办，之后整理到对应的导图里（右键节点「移到其他导图…」）。"
		if err := f.Save(path); err != nil {
			return err
		}
	}
	if err := a.editMapAt(path, "随手记", func(f *model.File) error {
		f.Sheets[0].Root.Children = append(f.Sheets[0].Root.Children, nodes...)
		return nil
	}); err != nil {
		return err
	}
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
	// The nodes keep their IDs, as links to them and tasks waiting on
	// them follow them to the other map.
	var title string
	from := model.AssetsDir(a.path)
	err := a.editMapAt(path, "从「"+mapName(a.path)+"」移来", func(f *model.File) error {
		d := f.SheetOf(id)
		if d == nil {
			return errors.New("目标节点已不存在")
		}
		target, _ := d.Find(id)
		title = target.Title
		for _, n := range nodes {
			c := n.Clone()
			copyAssets(c, from, model.AssetsDir(path))
			target.Children = append(target.Children, c)
		}
		target.Collapsed = false
		return nil
	})
	if err != nil {
		a.saveErr = err.Error()
		return
	}
	a.deleteSelection()
	a.tell(fmt.Sprintf("已移到「%s」的「%s」下", mapName(path), title))
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
		{label: "截图…", run: func() { a.plat.Screenshot(false) }},
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
	items = append(items, trayEntry{sep: true}, trayEntry{label: "打开 MyMind", run: a.plat.ShowMain})
	return items
}

// trayTitle is shown beside the icon: how many tasks are due, if any.
func (a *app) trayTitle() string {
	if n := len(a.trayTasks()); n > 0 {
		return fmt.Sprint(n)
	}
	return ""
}

// openSettings opens the settings, with the fields as saved.
func (a *app) openSettings() {
	a.settingsOpen = true
	a.captureKeyDraft, a.ghTokenDraft, a.clipKeyDraft = a.cfg.CaptureKey, a.cfg.GitHubToken, a.cfg.ClipKey
	a.shotKeyDraft = a.cfg.ShotKey
	a.embedDrafts = [3]string{a.cfg.EmbedURL, a.cfg.EmbedKey, a.cfg.EmbedModel}
	a.syncDrafts = [2]string{a.cfg.SyncRepo, a.cfg.SyncToken}
}

// settingsView is the window of preferences: the capture shortcut, the
// menu bar icon, reminders and the editor of code links.
func (a *app) settingsView(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c.Key("settings"), &a.settingsOpen, func() {
		w, h := c.Size()
		ui.Column(c).Gap(14).Width(min(600, w-80)).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "设置").FontSize(17).Bold().Grow(1)
				if iconButton(c, iconClose, "关闭", "关闭（Esc）", 28, true).Clicked() {
					a.settingsOpen = false
				}
			})
			tabs := []string{"通用", "资料库", "备份", "同步", "连接"}
			if a.file != nil {
				tabs = append(tabs, "当前导图")
			}
			a.settingsTab = min(a.settingsTab, len(tabs)-1)
			ui.Segmented(c, &a.settingsTab, tabs...).FillWidth()
			// The same height for every tab, so that the window does not
			// jump when switching; a long tab scrolls.
			ui.Scroll(c.Key(fmt.Sprint("settings", a.settingsTab))).Gap(16).Height(max(min(560, h-200), 200)).Padding(0, 10, 0, 0).Label("设置内容").Children(func() {
				switch tabs[a.settingsTab] {
				case "通用":
					a.generalSettings(c)
				case "资料库":
					auto := !a.cfg.LibraryManual
					switchRow(c, &auto, "自动整理资料", "新收藏的资料由 AI 自动起标题、加说明、写摘要、分类、打标签，并生成同义词和分析（用 AI 助手的 API Key，按用量计费）", func() {
						a.cfg.LibraryManual = !auto
						a.cfg.save()
					})
					a.embedSettings(c)
				case "备份":
					a.backupSettings(c)
				case "同步":
					a.syncSettings(c)
				case "连接":
					ui.Field(c, "AI 助手", func() {
						ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
							ui.Text(c, "服务商、API Key 和模型在 AI 助手面板的「⋯ → AI 设置」里。当前："+providerModelName(a.cfg)).FontSize(12).TextColor(t.TextMuted).Wrap().Grow(1)
							if ui.Button(c, "打开 AI 设置").Clicked() {
								a.settingsOpen, a.aiOpen, a.ai.settings = false, true, true
								a.loadKeyDrafts()
							}
						})
					})
					ui.Field(c, "AI 编程工具（MCP）", func() {
						ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
							ui.Text(c, "让 Claude Code、Cursor 等读写导图、检索资料库").FontSize(12).TextColor(t.TextMuted).Grow(1)
							if ui.Button(c, "连接说明…").Clicked() {
								a.settingsOpen, a.mcpOpen = false, true
							}
						})
					})
					ui.Field(c, "GitHub Token（同步私有仓库的 issues 需要）", func() {
						in := ui.TextInput(c.Key("gh-token"), &a.ghTokenDraft).Password().Placeholder("ghp_… 或 github_pat_…").Label("GitHub Token")
						if in.Submitted() || in.PressedOutside() && a.ghTokenDraft != a.cfg.GitHubToken {
							a.cfg.GitHubToken = strings.TrimSpace(a.ghTokenDraft)
							a.cfg.save()
						}
					})
				default:
					a.projectSection(c)
				}
			})
		})
	})
}

// switchRow is a switch with what it does beside it.
func switchRow(c *ui.Context, on *bool, label, text string, changed func()) {
	ui.Row(c.Key(label)).Gap(10).AlignItems(ui.Center).Children(func() {
		if ui.Switch(c, on).Label(label).Changed() {
			changed()
		}
		ui.Column(c).Gap(2).Grow(1).Children(func() {
			ui.Text(c, label).FontSize(13).Bold()
			ui.Text(c, text).FontSize(12).TextColor(c.Theme().TextMuted).Wrap()
		})
	})
}

// generalSettings are the shortcuts, the menu bar icon and the reminders.
func (a *app) generalSettings(c *ui.Context) {
	t := c.Theme()
	a.iconSettings(c)
	a.updateSettings(c)
	a.shotSettings(c)
	ui.Field(c, "全局快捷键", func() {
		ui.Column(c).Gap(10).Children(func() {
			rows := []struct {
				name, what, value, def string
				save                   func(string)
				reg                    func(string) error
				effective              func() string
			}{
				{"快速记录快捷键", "快速记录：弹出小窗口记下想法", a.cfg.CaptureKey, defaultCaptureKey,
					func(v string) { a.cfg.CaptureKey = v }, a.plat.SetCaptureKey, a.captureKey},
				{"收藏剪贴板快捷键", "收藏剪贴板：复制后按下，直接收进资料库", a.cfg.ClipKey, defaultClipKey,
					func(v string) { a.cfg.ClipKey = v }, a.plat.SetClipKey, a.clipKey},
				{"截图快捷键", "截图：框选屏幕、标注，收进资料库或复制", a.cfg.ShotKey, defaultShotKey,
					func(v string) { a.cfg.ShotKey = v }, a.plat.SetShotKey, a.shotKey},
			}
			for _, r := range rows {
				ui.Row(c.Key(r.name)).Gap(10).AlignItems(ui.Center).Children(func() {
					ui.Text(c, r.what).FontSize(textBody).Grow(1).Shrink(1).Wrap()
					a.shortcutField(c, r.name, r.value, r.def, func(acc string) error {
						r.save(acc)
						a.cfg.save()
						return r.reg(r.effective())
					}, func() { _ = r.reg("") })
				})
			}
			ui.Text(c, "在任何应用里都能用。点快捷键框，再按下新的组合键；Esc 取消。").FontSize(12).TextColor(t.TextMuted).Wrap()
		})
	})
	tray := !a.cfg.NoTray
	switchRow(c, &tray, "菜单栏图标", "显示今天到期和已逾期的任务数，点开列出任务、快速记录、收藏剪贴板", func() {
		a.cfg.NoTray = !tray
		a.cfg.save()
		a.plat.SetTray(tray)
	})
	on := !a.cfg.NoReminders
	switchRow(c, &on, "到期提醒", "任务今天到期或已逾期时，每天发一次系统通知", func() {
		a.cfg.NoReminders = !on
		a.cfg.save()
	})
	if ui.Link(c, "重新查看新手引导", "").FontSize(12).Clicked() {
		a.settingsOpen, a.tourStep = false, 1
	}
}
