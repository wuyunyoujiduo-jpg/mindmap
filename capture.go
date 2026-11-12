package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

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
	captureLibrary // a note of 灵感, a text of the library: the default
	captureHere    // under the node selected in the open map
	captureAuto    // a line that says a day or a priority is a task, else a note
)

// taskLike tells whether a text reads as a task rather than a note: one
// line, with a day or a priority said as a word of its own (明天 写周报,
// 写周报 !高), or lines of "- [ ]". 今天很好 is a note.
func taskLike(text string, now time.Time) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	lines := strings.Split(text, "\n")
	boxes := 0
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "- [ ]") {
			boxes++
		}
	}
	if boxes == len(lines) {
		return true
	}
	if len(lines) > 1 {
		return false
	}
	if rest, rep, _ := parseRepeat(text, now); rep != nil && rest != "" {
		return true // 每周五 写周报: a task that repeats
	}
	words := strings.Fields(text)
	if len(words) < 2 {
		return false
	}
	for _, w := range words {
		if title, due, prio := parseTaskLine(w, now); title == "" && (due != nil || prio != model.PriorityNone) {
			return true
		}
	}
	return false
}

// capture adds text to the inbox, or under the node selected: a node a
// line, nested by indentation, as a pasted outline is.
func (a *app) capture(text string, where int) error {
	if where == captureAuto {
		where = captureLibrary
		if taskLike(text, a.now()) {
			where = captureInbox
		}
	}
	if where == captureLibrary { // a note of 灵感
		if _, err := a.addMemo(text, "快速记录"); err != nil {
			return err
		}
		a.tell("已记到灵感")
		return nil
	}
	if where == captureInbox { // tasks, a line each, to the inbox
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- [ ]"))
			if line != "" {
				a.addTask(line, nil)
			}
		}
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

// captureView is the capture window: where it goes on top, a field, and
// at the bottom the clipboard, the keys and the one button that saves.
func (a *app) captureView(c *ui.Context) {
	t := quietTheme(c.Theme())
	c.SetTheme(t)
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
	ui.Column(c).Fill().Padding(16, 18, 14).Gap(12).Background(t.Background).Children(func() {
		// Nothing to choose: what is written says what it is. A topic
		// selected in the open map may take it instead.
		here, hereTitle := false, ""
		if n, _ := a.selectedNode(); n != nil && a.path != a.inboxPath() {
			here, hereTitle = true, n.Title
		}
		if a.captureWhere == captureHere && !here {
			a.captureWhere = captureAuto
		}
		placeholder := "写下想法；写「明天 交周报」这样带日期的一句，就是任务"
		in := ui.TextAreaBase(c.Key("capture"), &a.captureText).Placeholder(placeholder).FontSize(15).LineHeight(1.6).
			Lines(3, 8).AutoFocus().FillWidth().Grow(1).Label("快速记录")
		in.HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyEnter && (ev.Mods == 0 || ev.Mods == ui.Cmd) {
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
			if ghostButton(c, "收藏剪贴板", "把剪贴板里的文字、链接、图片或文件存到收集（"+keyLabel(a.clipKey())+"）").Clicked() {
				if _, err := a.clipClipboard("剪贴板"); err != nil {
					a.captureErr = err.Error()
				} else {
					a.captureErr = ""
					a.plat.HideCapture()
				}
			}
			if here {
				on := a.captureWhere == captureHere
				b := ui.Box(c.Key("capture-here")).Padding(4, 8).Radius(6).Cursor(ui.CursorPointer).Label("记到选中的主题下").
					Tooltip("记成导图里「" + hereTitle + "」下的子主题")
				switch {
				case on:
					b.Background(t.Accent.Alpha(0.14))
				case b.Hovered():
					b.Background(t.Text.Alpha(0.06))
				}
				if b.Clicked() {
					if on {
						a.captureWhere = captureAuto
					} else {
						a.captureWhere = captureHere
					}
				}
				b.Children(func() {
					col := t.TextMuted
					if on {
						col = t.Accent
					}
					ui.Text(c, "记到「"+hereTitle+"」下").FontSize(textSmall).TextColor(col).SingleLine().MaxWidth(160).Ellipsis("…").PassThrough()
				})
			}
			ui.Spacer(c)
			if a.captureErr != "" {
				ui.Text(c, a.captureErr).FontSize(textSmall).TextColor(t.Danger).Shrink(1).SingleLine()
			} else if strings.TrimSpace(a.captureText) != "" && a.captureWhere != captureHere {
				// Where it goes, said as it is written.
				if a.captureWhere == captureInbox || a.captureWhere == captureAuto && taskLike(a.captureText, a.now()) {
					pill(c, "任务 → 收件箱", t.Text.Alpha(0.06), t.Accent)
				} else {
					pill(c, "灵感", t.Text.Alpha(0.06), t.TextMuted)
				}
			} else {
				ui.Text(c, "⇧↩ 换行 · Esc 关闭").FontSize(textCaption).TextColor(t.TextMuted)
			}
			if accentButton(c, "记录 ↩", "保存").Clicked() {
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
	// Three things to do from anywhere, then today's tasks, then the app:
	// nothing that only goes to a page of it, which opening it does.
	items := []trayEntry{{label: "记一条…", run: a.plat.ShowCapture},
		{label: "截图", run: func() { a.plat.Screenshot(false) }},
		{label: "收藏剪贴板", run: a.clipFromShortcut}}
	tasks := a.trayTasks()
	slices.SortStableFunc(tasks, func(x, y trayTask) int { return b2i(y.overdue) - b2i(x.overdue) }) // those late first
	items = append(items, trayEntry{sep: true})
	if len(tasks) == 0 {
		items = append(items, trayEntry{label: "今天没有要做的"})
	} else {
		items = append(items, trayEntry{label: fmt.Sprintf("今天要做 %d 件", len(tasks))})
		for i, tt := range tasks {
			if i == 8 {
				items = append(items, trayEntry{label: fmt.Sprintf("还有 %d 件，在 MyMind 里看", len(tasks)-i), run: a.plat.ShowMain})
				break
			}
			// Its words, without the link written in them; late, it says so.
			label, _ := titleAndSite(tt.node.Title)
			if r := []rune(label); len(r) > 28 {
				label = string(r[:28]) + "…"
			}
			if tt.overdue {
				label = "逾期 · " + label
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
	a.captureKeyDraft, a.clipKeyDraft = a.cfg.CaptureKey, a.cfg.ClipKey
	a.shotKeyDraft = a.cfg.ShotKey
	a.embedDrafts = [3]string{a.cfg.EmbedURL, a.cfg.EmbedKey, a.cfg.EmbedModel}
	a.syncDrafts = [2]string{a.cfg.SyncRepo, a.cfg.SyncToken}
}

// settingsView is the window of preferences: the capture shortcut, the
// menu bar icon, reminders and the editor of code links.
func (a *app) settingsView(c *ui.Context) {
	t := c.Theme()
	// On the ground, as a page: its parts are white cards.
	modal(c, "settings", &a.settingsOpen, func() {
		w, h := c.Size()
		tabs := []string{"通用", "资料整理", "备份", "同步", "连接"}
		if a.file != nil {
			tabs = append(tabs, "当前导图")
		}
		a.settingsTab = min(a.settingsTab, len(tabs)-1)
		// The parts on the left, as a list; the one chosen on the right.
		ui.Row(c).Gap(22).Width(min(820, w-80)).AlignItems(ui.Start).Children(func() {
			ui.Column(c.Key("settings-tabs")).Width(160).Shrink(0).Gap(2).Children(func() {
				ui.Text(c, "设置").FontSize(26).FontWeight(700).LetterSpacing(-0.7).Margin(0, 12, 14)
				for i, name := range tabs {
					on := i == a.settingsTab
					row := ui.Row(c.Key("settings-tab:"+name)).Height(36).Padding(0, 10).Gap(10).Radius(radiusRow).AlignItems(ui.Center).
						FillWidth().Cursor(ui.CursorPointer).Label(name).Background(t.Text.Alpha(0)).Transition(easeLook)
					hot := on || row.Hovered()
					if !on && hot {
						row.Background(t.Text.Alpha(0.05))
					}
					if row.Clicked() {
						a.settingsTab = i
					}
					press(row, 0.975)
					row.Children(func() {
						if on {
							slidePill(c, "settings-tabs", radiusRow)
							row.ZIndex(-1) // under the others, which the pill slides behind
						}
						// Its icon, as a row of the left column has.
						kubaiIcon(c, map[string]string{"通用": "sliders", "资料整理": "sparkles", "备份": "save", "同步": "sync", "连接": "plug", "当前导图": "sitemap"}[name], sideIconSize, hot)
						txt := ui.Text(c, name).FontSize(textBody + 0.5).PassThrough()
						if on {
							txt.FontWeight(700)
						} else if !hot {
							txt.TextColor(ink2(t))
						}
					})
				}
			})
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(10).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Children(func() {
					ui.Text(c, tabs[a.settingsTab]).FontSize(textTitle).FontWeight(700).Grow(1)
					if iconButton(c, iconClose, "关闭", "关闭（Esc）", 28, true).Clicked() {
						a.settingsOpen = false
					}
				})
				// The same height for every tab, so that the window does not
				// jump when switching; a long tab scrolls.
				ui.Scroll(c.Key(fmt.Sprint("settings", a.settingsTab))).Gap(20).Height(max(min(580, h-190), 200)).Padding(2, 12, 24, 2).Margin(0, -2).Label("设置内容").Children(func() {
					switch tabs[a.settingsTab] {
					case "通用":
						a.generalSettings(c)
					case "资料整理":
						rowsField(c, "AI 整理", "用的是 AI 助手的 API Key，按用量计费。", func() {
							auto := !a.cfg.LibraryManual
							switchRow(c, &auto, "自动整理资料", "新收藏的资料由 AI 起标题、写摘要、分类、打标签，并生成同义词", func() {
								a.cfg.LibraryManual = !auto
								a.cfg.save()
							})
						})
						a.embedSettings(c)
					case "备份":
						a.backupSettings(c)
					case "同步":
						a.syncSettings(c)
					case "连接":
						rowsField(c, "AI", "", func() {
							setRow(c, "AI 助手", "服务商、API Key 和模型。当前："+providerModelName(a.cfg), func() {
								if button(c, "打开 AI 设置").Clicked() {
									a.settingsOpen, a.aiOpen, a.ai.settings = false, true, true
									a.loadKeyDrafts()
								}
							})
							setLine(c)
							setRow(c, "AI 编程工具（MCP）", "让 Claude Code、Cursor 等读写导图、检索收集的资料", func() {
								if button(c, "连接说明…").Clicked() {
									a.settingsOpen, a.mcpOpen = false, true
								}
							})
						})
					default:
						a.projectSection(c)
					}
				})
			})
		})
	})
}

// switchRow is a switch with what it does beside it.
func switchRow(c *ui.Context, on *bool, label, text string, changed func()) {
	t := c.Theme()
	// The whole row sets it: a click anywhere on it.
	row := ui.Row(c.Key("row:"+label)).MinHeight(52).Padding(10, 0).Gap(16).AlignItems(ui.Center).FillWidth().
		Cursor(ui.CursorPointer).Label(label)
	if row.Clicked() {
		*on = !*on
		changed()
	}
	row.Children(func() {
		ui.Column(c).Gap(2).Grow(1).Shrink(1).Basis(0).MinWidth(0).PassThrough().Children(func() {
			ui.Text(c, label).FontSize(textBody)
			if text != "" {
				ui.Text(c, text).FontSize(textSmall).TextColor(t.TextMuted).LineHeight(1.45).Wrap()
			}
		})
		ui.Switch(c, on).Shrink(0).PassThrough()
	})
}

// generalSettings are the shortcuts, the menu bar icon and the reminders.
func (a *app) generalSettings(c *ui.Context) {
	rowsField(c, "全局快捷键", "在任何应用里都能用。点一个快捷键，再按下新的组合键。", func() {
		rows := []struct {
			name, title, what, value, def string
			save                          func(string)
			reg                           func(string) error
			effective                     func() string
		}{
			{"快速记录快捷键", "快速记录", "弹出小窗口记下想法", a.cfg.CaptureKey, defaultCaptureKey,
				func(v string) { a.cfg.CaptureKey = v }, a.plat.SetCaptureKey, a.captureKey},
			{"收藏剪贴板快捷键", "收藏剪贴板", "复制后按下，直接存到收集", a.cfg.ClipKey, defaultClipKey,
				func(v string) { a.cfg.ClipKey = v }, a.plat.SetClipKey, a.clipKey},
			{"截图快捷键", "截图", "框选屏幕、标注，存到收集或复制", a.cfg.ShotKey, defaultShotKey,
				func(v string) { a.cfg.ShotKey = v }, a.plat.SetShotKey, a.shotKey},
		}
		for i, r := range rows {
			if i > 0 {
				setLine(c)
			}
			setRow(c, r.title, r.what, func() {
				a.shortcutField(c, r.name, r.value, r.def, func(acc string) error {
					r.save(acc)
					a.cfg.save()
					return r.reg(r.effective())
				}, func() { _ = r.reg("") })
			})
		}
	})
	rowsField(c, "菜单栏和提醒", "", func() {
		tray := !a.cfg.NoTray
		switchRow(c, &tray, "菜单栏图标", "显示今天到期和已逾期的任务数，点开列出任务、快速记录、收藏剪贴板", func() {
			a.cfg.NoTray = !tray
			a.cfg.save()
			a.plat.SetTray(tray)
		})
		setLine(c)
		on := !a.cfg.NoReminders
		switchRow(c, &on, "到期提醒", "任务今天到期或已逾期时，每天发一次系统通知", func() {
			a.cfg.NoReminders = !on
			a.cfg.save()
		})
		setLine(c)
		setRow(c, "新手引导", "再看一遍怎么用", func() {
			if button(c, "重新查看").Label("重新查看新手引导").Clicked() {
				a.settingsOpen, a.tourStep = false, 1
			}
		})
	})
	a.shotSettings(c)
	a.updateSettings(c)
	a.groundSettings(c)
	a.iconSettings(c)
}
