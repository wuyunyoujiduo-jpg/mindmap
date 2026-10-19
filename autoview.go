package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/automate"
)

// The automations' page: their list, and the one chosen's code and runs.

// openAutomations shows the automations in the main area.
func (a *app) openAutomations() {
	a.showLibrary()
	a.auto.open = true
	a.autoRefresh(true)
	if a.autoByID(a.auto.sel) == nil {
		a.auto.sel = ""
		if len(a.auto.list) > 0 {
			a.autoSelect(a.auto.list[0].ID)
		}
	}
}

func (a *app) autoSelect(id string) {
	if a.auto.sel != id {
		a.autoSaveCode()
		a.auto.sel, a.auto.file, a.auto.fileOf, a.auto.code, a.auto.run = id, "", "", nil, ""
	}
}

// autoRow is the sidebar's row of the automations.
func (a *app) autoRow(c *ui.Context) {
	count := ""
	switch n := len(a.auto.procs); {
	case n > 0:
		count = fmt.Sprintf("%d 运行中", n)
	case len(a.auto.list) > 0:
		count = fmt.Sprint(len(a.auto.list))
	}
	row := navRow(c.Key("auto-row"), iconBolt, tileTeal, "自动化", count, a.libOpen && a.auto.open).
		Tooltip("把日常任务写成 Python 程序：手动或定时运行，可以用大模型")
	if row.Clicked() {
		a.later(a.openAutomations)
	}
}

// autoStatus is what a row tells of an automation: running, waiting, when
// it runs next, or how it last ended.
func (a *app) autoStatus(t *ui.Theme, x *automate.Automation) (string, ui.Color) {
	m := a.auto.metas[x.ID]
	switch {
	case x.Err != "":
		return "设置有误", t.Danger
	case a.auto.procs[x.ID] != nil:
		return "运行中…", t.Accent
	case m != nil && m.Status == automate.Running:
		return "上次没有结束", t.TextMuted
	case m.NeedsApproval(x.Code()):
		if m == nil || m.Approved == "" {
			return "新程序，待确认", tileOrange
		}
		return "代码改过，待确认", tileOrange
	}
	last := ""
	col := t.TextMuted
	if m != nil {
		switch m.Status {
		case automate.Failed, automate.TimeOut:
			last, col = "上次出错", t.Danger
		case automate.Stopped:
			last = "上次被停止"
		}
	}
	tr, _ := automate.ParseTrigger(x.Trigger)
	if m != nil && m.Enabled && tr.Watch != "" {
		if last != "" {
			return last + " · 监视中", col
		}
		return "监视 " + tr.Watch, col
	}
	if m != nil && m.Enabled && !tr.Manual() {
		next := tr.Next(m.From())
		if !next.After(a.now()) {
			next = a.now()
		}
		s := "下次 " + autoWhen(next, a.now())
		if last != "" {
			s = last + " · " + s
		}
		return s, col
	}
	if last != "" {
		return last, col
	}
	if !tr.Manual() {
		return x.Trigger + "（未开启）", col
	}
	return "手动运行", col
}

// autoWhen tells a time near now shortly: 14:30, 明天 09:00, 10月21日 09:00.
func autoWhen(at, now time.Time) string {
	y1, m1, d1 := at.Date()
	y2, m2, d2 := now.Date()
	switch {
	case y1 == y2 && m1 == m2 && d1 == d2:
		return at.Format("15:04")
	case at.Sub(now) < 48*time.Hour && at.AddDate(0, 0, -1).Day() == d2:
		return "明天 " + at.Format("15:04")
	}
	return fmt.Sprintf("%d月%d日 %s", m1, d1, at.Format("15:04"))
}

// autoView is the page of the automations.
func (a *app) autoView(c *ui.Context) {
	t := c.Theme()
	a.autoRefresh(false)
	a.autoAutosave(c)
	if a.auto.dirty.Load() || len(a.auto.procs) > 0 {
		c.After(300 * time.Millisecond)
	}
	ui.Column(c.Key("auto")).Grow(1).Label("自动化").Children(func() {
		ui.Row(c.Key("auto-head")).Height(toolbarHeight).Padding(0, 10, 0, 20).Gap(8).AlignItems(ui.Center).Shrink(0).
			DragWindow().Children(func() {
			ui.Column(c).Gap(1).Children(func() {
				ui.Text(c, "自动化").Bold().FontSize(textTitle)
				on := 0
				for _, m := range a.auto.metas {
					if m.Enabled {
						on++
					}
				}
				ui.Text(c, fmt.Sprintf("%d 个程序 · %d 个定时", len(a.auto.list), on)).FontSize(11).TextColor(t.TextMuted)
			})
			ui.Spacer(c)
			if a.auto.busy != "" {
				ui.Spinner(c)
				ui.Text(c, a.auto.busy).FontSize(textSmall).TextColor(t.TextMuted)
			}
			if ghostButton(c, "安装依赖", "为所有程序建好共用的 Python 环境，安装各自 requirements.txt 里的包").Clicked() {
				a.autoInstall()
			}
			if ghostButton(c, "打开文件夹", "在 Finder 中打开自动化文件夹").Clicked() {
				_ = os.MkdirAll(a.autoRoot(), 0o755)
				a.plat.OpenPath(a.autoRoot())
			}
			iconTextButton(c, iconPlus, "新建", "新建自动化", "新建自动化：一句话让 AI 写，或从模板开始", false).Menu(func(m *ui.Menu) {
				a.autoNewMenu(m)
			})
		})
		ui.Divider(c)
		if len(a.auto.list) == 0 {
			a.autoEmpty(c)
			return
		}
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			ui.Scroll(c.Key("auto-list")).Width(250).Shrink(0).Padding(8).Gap(2).
				BorderWidth(0, 1, 0, 0).BorderColor(hairline(t)).Children(func() {
				for _, x := range a.auto.list {
					a.autoListRow(c, x)
				}
			})
			if x := a.autoByID(a.auto.sel); x != nil {
				a.autoDetail(c, x)
			} else {
				ui.Column(c).Grow(1).Center().Children(func() {
					ui.Text(c, "选一个程序").TextColor(t.TextMuted)
				})
			}
		})
	})
}

func (a *app) autoNewMenu(m *ui.Menu) {
	if m.Item("一句话让 AI 写…").Chosen() {
		a.auto.newOpen, a.auto.aiErr = true, ""
	}
	m.Separator()
	for _, tpl := range autoTemplates {
		if m.Item("模板：" + tpl.name).Chosen() {
			a.later(func() { a.autoFromTemplate(tpl) })
		}
	}
	m.Separator()
	if m.Item("空白程序（单个文件）").Chosen() {
		a.later(func() { a.autoFromTemplate(autoBlankFile) })
	}
	if m.Item("空白程序（文件夹，多个文件）").Chosen() {
		a.later(func() { a.autoFromTemplate(autoBlankFolder) })
	}
}

func (a *app) autoEmpty(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).Center().Gap(14).Padding(24).Children(func() {
		tile(c, iconBolt, tileTeal, 44)
		ui.Text(c, "把日常任务写成自动化程序").FontSize(17).Bold()
		ui.Text(c, "每个自动化是一个 Python 程序（一个文件，或一个文件夹），手动或定时运行。"+
			"程序里用 mymind 调大模型、发通知、等你确认、存进资料库。模型由每个程序自己配置。").
			FontSize(textBody).TextColor(t.TextMuted).Wrap().MaxWidth(460).TextAlign(ui.Center)
		ui.Row(c).Gap(8).Children(func() {
			if ui.PrimaryButton(c, "一句话让 AI 写").Clicked() {
				a.auto.newOpen, a.auto.aiErr = true, ""
			}
			for _, tpl := range autoTemplates {
				if ui.Button(c, tpl.name).Clicked() {
					a.later(func() { a.autoFromTemplate(tpl) })
				}
			}
		})
	})
}

func (a *app) autoListRow(c *ui.Context, x *automate.Automation) {
	t := c.Theme()
	sel := a.auto.sel == x.ID
	r := ui.Column(c.Key("auto:"+x.ID)).Padding(7, 10).Gap(2).Radius(8).FillWidth().Cursor(ui.CursorPointer).Label(x.Name)
	switch {
	case sel:
		r.Background(t.Accent.Alpha(0.16))
	case r.Hovered():
		r.Background(t.Text.Alpha(0.06))
	}
	if r.Clicked() {
		id := x.ID
		a.later(func() { a.autoSelect(id) })
	}
	status, col := a.autoStatus(t, x)
	r.Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).PassThrough().Children(func() {
			ui.Box(c).Size(7, 7).Radius(4).Shrink(0).Background(col)
			name := ui.Text(c, x.Name).FontSize(textBody).Grow(1).Shrink(1).MinWidth(0).SingleLine()
			if sel {
				name.Bold()
			}
			if x.Folder {
				ui.Text(c, fmt.Sprintf("%d 个文件", len(x.Files()))).FontSize(textCaption).TextColor(t.TextMuted).Shrink(0)
			}
		})
		ui.Text(c, status).FontSize(textCaption).TextColor(t.TextMuted).SingleLine().PassThrough().Padding(0, 0, 0, 13)
	})
}

func (a *app) autoDetail(c *ui.Context, x *automate.Automation) {
	t := c.Theme()
	m := a.auto.metas.Of(x.ID)
	running := a.auto.procs[x.ID] != nil
	code := x.Code()
	waiting := m.NeedsApproval(code)
	tr, _ := automate.ParseTrigger(x.Trigger)
	ui.Column(c.Key("auto-detail:" + x.ID)).Grow(1).Basis(0).MinWidth(0).Children(func() {
		ui.Column(c).Padding(14, 20, 10).Gap(6).Shrink(0).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, x.Name).FontSize(textHeadline).Bold().Shrink(1).MinWidth(0).SingleLine()
				ui.Spacer(c)
				if running {
					if ui.Button(c, "停止").Clicked() {
						a.autoStop(x.ID)
					}
				} else if ui.PrimaryButton(c, "运行").Disabled(x.Err != "").Tooltip("运行一次").Clicked() {
					a.later(func() { a.autoStart(x, "手动") })
				}
				if ui.Button(c, "让 AI 修改").Tooltip("说一句要怎么改，AI 改好后你确认").Clicked() {
					a.auto.fixOpen, a.auto.fixText, a.auto.aiErr = true, "", ""
				}
				iconButton(c, iconMore, "更多", "更多", 28, true).Menu(func(mm *ui.Menu) {
					if mm.Item("在 Finder 中显示").Chosen() {
						a.plat.ShowInFolder(x.Path)
					}
					if mm.Item("用默认应用打开").Chosen() {
						a.plat.OpenPath(x.EntryPath())
					}
					mm.Separator()
					if mm.Item("移到废纸篓").Chosen() {
						id, path := x.ID, x.Path
						a.later(func() { a.autoDelete(id, path) })
					}
				})
			})
			ui.Row(c).Gap(14).AlignItems(ui.Center).Wrap().GapY(4).Children(func() {
				when := x.Trigger
				if tr.Manual() {
					when = "手动"
				}
				ui.Text(c, "触发："+when).FontSize(textSmall).TextColor(t.TextMuted)
				model := x.Model
				col := t.TextMuted
				if model == "" {
					model = "未配置"
					if strings.Contains(strings.Join(mapValues(code), "\n"), "mymind.llm") {
						col = t.Danger
					}
				}
				ui.Text(c, "模型："+model).FontSize(textSmall).TextColor(col).
					Tooltip("程序自己的模型：automation.toml 的 model，或单文件开头的 # 模型:\n可以写 deepseek-chat、claude-…，或 cli:claude -p 这样的命令")
				if x.Agent != "" || strings.Contains(strings.Join(mapValues(code), "\n"), "mymind.agent") {
					agent, col := x.Agent, t.TextMuted
					if agent == "" {
						agent, col = "未配置", t.Danger
					}
					ui.Text(c, "编码工具："+agent).FontSize(textSmall).TextColor(col).
						Tooltip("程序自己的编码工具：automation.toml 的 agent，如 claude -p、codex exec、aider --yes --message {prompt}")
				}
				if !tr.Manual() {
					on := m.Enabled
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						if ui.Switch(c.Key("auto-on"), &on).Label("自动运行").Changed() {
							id := x.ID
							a.later(func() { a.autoSetEnabled(id, on) })
						}
						s := "定时运行"
						if tr.Watch != "" {
							s = "文件变化时运行"
						} else if m.Enabled {
							next := tr.Next(m.From())
							if next.Before(a.now()) {
								next = a.now()
							}
							s += " · 下次 " + autoWhen(next, a.now())
						}
						ui.Text(c, s).FontSize(textSmall).TextColor(t.TextMuted)
					})
				}
			})
			if x.Err != "" {
				ui.Text(c, x.Err).FontSize(textSmall).TextColor(t.Danger).Wrap()
			}
			if waiting {
				ui.Row(c.Key("auto-approve-bar")).Padding(8, 12).Gap(10).Radius(8).AlignItems(ui.Center).
					Background(tileOrange.Alpha(0.12)).Children(func() {
					msg := "新程序：第一次运行前，请先看一遍代码。"
					if m.Approved != "" {
						msg = "代码被改过（AI 或其他程序），确认后才会运行。"
					}
					ui.Text(c, msg).FontSize(textSmall).Grow(1).Wrap()
					if ui.Button(c, "查看改动并确认").Clicked() {
						a.later(func() { a.openApproval(x, false) })
					}
				})
			}
		})
		ui.Row(c).Padding(0, 20, 8).Gap(10).AlignItems(ui.Center).Shrink(0).Children(func() {
			runs := len(automate.Runs(a.autoRoot(), x.ID))
			ui.Segmented(c.Key("auto-tab"), &a.auto.tab, "代码", fmt.Sprintf("运行记录 %d", runs)).Label("代码或运行记录")
		})
		ui.Divider(c)
		if a.auto.tab == 0 {
			a.autoCodeView(c, x)
		} else {
			a.autoRunsView(c, x)
		}
	})
}

func mapValues(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

// autoCodeView edits the program's files.
func (a *app) autoCodeView(c *ui.Context, x *automate.Automation) {
	t := c.Theme()
	files := x.Files()
	if a.auto.file == "" || !containsStr(files, a.auto.file) {
		a.auto.file = x.Entry
		if !x.Folder {
			a.auto.file = x.ID
		}
	}
	a.autoLoadCode(x)
	ui.Column(c).Grow(1).Children(func() {
		if x.Folder {
			ui.Row(c.Key("auto-files")).Padding(6, 16).Gap(4).Wrap().GapY(4).Shrink(0).Children(func() {
				for _, f := range files {
					sel := f == a.auto.file
					b := ui.Box(c.Key("file:"+f)).Padding(3, 9).Radius(6).Cursor(ui.CursorPointer).Label(f)
					switch {
					case sel:
						b.Background(t.Accent.Alpha(0.16))
					case b.Hovered():
						b.Background(t.Text.Alpha(0.06))
					}
					if b.Clicked() {
						f := f
						a.later(func() {
							a.autoSaveCode()
							a.auto.file, a.auto.fileOf = f, ""
						})
					}
					b.Children(func() {
						col := t.Text
						if !automate.IsCode(f) {
							col = t.TextMuted
						}
						ui.Text(c, f).FontSize(textSmall).TextColor(col).Font("monospace").PassThrough()
					})
				}
				ui.Spacer(c)
				if ghostButton(c, "新文件", "在程序的文件夹里加一个文件").Clicked() {
					a.later(func() { a.autoNewFile(x) })
				}
			})
			ui.Divider(c)
		}
		if a.auto.code == nil {
			ui.Column(c).Grow(1).Center().Children(func() {
				ui.Text(c, "不是文本文件，或太大，不在这里编辑").TextColor(t.TextMuted)
			})
			return
		}
		editor.View(c.Key("auto-code"), a.auto.code).Grow(1).FillWidth()
	})
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// autoLoadCode opens the file chosen in the editor, and takes it again
// from the disk when another program changed it and it is not being
// edited.
func (a *app) autoLoadCode(x *automate.Automation) {
	key := x.ID + "/" + a.auto.file
	path := filepath.Join(x.Dir(), filepath.FromSlash(a.auto.file))
	if a.auto.fileOf == key && a.auto.code != nil {
		if !a.auto.code.Buffer.Modified() && a.auto.loaded != a.auto.gen {
			a.auto.loaded = a.auto.gen
			if data, err := os.ReadFile(path); err == nil && string(data) != a.auto.code.Buffer.String() {
				a.auto.code.Buffer.Reset(string(data))
			}
		}
		return
	}
	a.auto.fileOf, a.auto.code, a.auto.loaded = key, nil, a.auto.gen
	st, err := os.Stat(path)
	if err != nil || st.Size() > 1<<20 {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil || !utf8.Valid(data) {
		return
	}
	buf := editor.NewBuffer(string(data))
	buf.SetLanguage(editor.LanguageFor(a.auto.file))
	a.auto.code = &editor.State{Buffer: buf, Label: a.auto.file, TabSize: 4}
	a.auto.approved = !a.auto.metas.Of(x.ID).NeedsApproval(x.Code())
}

// autoAutosave saves the code a moment after typing stops.
func (a *app) autoAutosave(c *ui.Context) {
	st := a.auto.code
	if st == nil || !st.Buffer.Modified() {
		a.auto.edited = time.Time{}
		return
	}
	if a.auto.edited.IsZero() {
		a.auto.edited = a.now()
	}
	if wait := time.Second - a.now().Sub(a.auto.edited); wait > 0 {
		c.After(wait)
		return
	}
	a.autoSaveCode()
}

// autoSaveCode writes the file edited. Code approved stays approved: what
// is typed here is the user's own.
func (a *app) autoSaveCode() {
	st := a.auto.code
	if st == nil || !st.Buffer.Modified() {
		return
	}
	id, file, _ := strings.Cut(a.auto.fileOf, "/")
	x := a.autoByID(id)
	if x == nil {
		return
	}
	path := filepath.Join(x.Dir(), filepath.FromSlash(file))
	if err := os.WriteFile(path, []byte(st.Buffer.String()), 0o644); err != nil {
		a.tell("没有保存：" + err.Error())
		return
	}
	st.Buffer.MarkSaved()
	a.auto.edited = time.Time{}
	a.autoRefresh(true)
	if x = a.autoByID(id); x != nil && a.auto.approved {
		a.auto.metas.Of(id).Approve(x.Code())
		_ = a.auto.metas.Save(a.autoRoot())
	}
}

func (a *app) autoNewFile(x *automate.Automation) {
	name := "新模块.py"
	for n := 2; ; n++ {
		if _, err := os.Stat(filepath.Join(x.Path, name)); err != nil {
			break
		}
		name = fmt.Sprintf("新模块%d.py", n)
	}
	if err := os.WriteFile(filepath.Join(x.Path, name), []byte("# "+name+"\n"), 0o644); err != nil {
		a.tell(err.Error())
		return
	}
	a.autoRefresh(true)
	a.auto.file, a.auto.fileOf = name, ""
	a.tell("已新建 " + name + "，可以在 Finder 里改名")
}

func (a *app) autoDelete(id, path string) {
	a.autoStop(id)
	if err := a.plat.Trash(path); err != nil {
		a.tell("没有删除：" + err.Error())
		return
	}
	delete(a.auto.metas, id)
	_ = a.auto.metas.Save(a.autoRoot())
	a.auto.sel, a.auto.code, a.auto.fileOf = "", nil, ""
	a.openAutomations()
}

// autoRunsView lists the runs, and shows the one chosen: its result and
// what it printed.
func (a *app) autoRunsView(c *ui.Context, x *automate.Automation) {
	t := c.Theme()
	runs := automate.Runs(a.autoRoot(), x.ID)
	if p := a.auto.procs[x.ID]; p != nil {
		runs = append([]automate.Run{p.Snapshot()}, runs...)
	}
	if len(runs) == 0 {
		ui.Column(c).Grow(1).Center().Children(func() {
			ui.Text(c, "还没有运行过").TextColor(t.TextMuted)
		})
		return
	}
	var shown *automate.Run
	for i := range runs {
		if runs[i].ID == a.auto.run {
			shown = &runs[i]
		}
	}
	if shown == nil {
		shown = &runs[0]
	}
	ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
		ui.Scroll(c.Key("auto-runs")).Width(200).Shrink(0).Padding(6).Gap(2).BorderWidth(0, 1, 0, 0).BorderColor(hairline(t)).Children(func() {
			for _, r := range runs {
				row := ui.Column(c.Key("run:"+r.ID)).Padding(6, 8).Gap(1).Radius(6).FillWidth().Cursor(ui.CursorPointer)
				switch {
				case r.ID == shown.ID:
					row.Background(t.Accent.Alpha(0.16))
				case row.Hovered():
					row.Background(t.Text.Alpha(0.06))
				}
				if row.Clicked() {
					a.auto.run = r.ID
				}
				label, col := runStatus(t, r.Status)
				row.Children(func() {
					ui.Row(c).Gap(6).AlignItems(ui.Center).PassThrough().Children(func() {
						ui.Box(c).Size(7, 7).Radius(4).Background(col)
						ui.Text(c, autoWhen(r.Start, a.now())).FontSize(textSmall)
					})
					ui.Text(c, label+" · "+r.By+" · "+runTook(r, a.now())).FontSize(textCaption).TextColor(t.TextMuted).PassThrough().Padding(0, 0, 0, 13)
				})
			}
		})
		ui.Scroll(c.Key("auto-run:"+shown.ID)).Grow(1).Basis(0).MinWidth(0).Padding(14, 20).Gap(10).Children(func() {
			r := *shown
			label, col := runStatus(t, r.Status)
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, label).Bold().TextColor(col)
				ui.Text(c, r.Start.Format("2006-01-02 15:04:05")+" · "+r.By+" · 用时 "+runTook(r, a.now())).FontSize(textSmall).TextColor(t.TextMuted)
				if r.Exit != 0 {
					ui.Text(c, fmt.Sprintf("退出码 %d", r.Exit)).FontSize(textSmall).TextColor(t.TextMuted)
				}
				ui.Spacer(c)
				if (r.Status == automate.Failed || r.Status == automate.TimeOut) && ui.Button(c, "让 AI 修").Tooltip("把出错的输出给 AI，改好后你确认").Clicked() {
					log := cmpOr(r.Error, "") + "\n" + r.Log
					a.later(func() { a.autoModify(x, "运行出错了，请找出原因并修好。", log) })
				}
			})
			if r.Error != "" {
				ui.Text(c, r.Error).FontSize(textSmall).TextColor(t.Danger).Wrap().Selectable()
			}
			if r.Output != "" {
				ui.Column(c).Padding(12, 14).Radius(10).Background(t.Surface).Gap(6).FillWidth().Children(func() {
					sectionHeader(c, "结果")
					markdownView(c, r.Output, textBody)
				})
			}
			sectionHeader(c, "输出")
			log := r.Log
			if strings.TrimSpace(log) == "" {
				log = "（没有输出）"
			}
			ui.Text(c, log).Font("monospace").FontSize(12).Wrap().Selectable().FillWidth()
		})
	})
}

func runStatus(t *ui.Theme, s string) (string, ui.Color) {
	switch s {
	case automate.Running:
		return "运行中", t.Accent
	case automate.OK:
		return "完成", tileGreen
	case automate.Stopped:
		return "已停止", t.TextMuted
	case automate.TimeOut:
		return "超时", t.Danger
	}
	return "出错", t.Danger
}

func runTook(r automate.Run, now time.Time) string {
	end := r.End
	if end.IsZero() {
		end = now
	}
	d := end.Sub(r.Start).Round(time.Second)
	if d < time.Second {
		return "不到 1 秒"
	}
	return strings.NewReplacer("h", " 小时 ", "m", " 分 ", "s", " 秒").Replace(d.String())
}

// openApproval shows the changes of the code since it was approved.
func (a *app) openApproval(x *automate.Automation, runAfter bool) {
	m := a.auto.metas.Of(x.ID)
	a.auto.approve, a.auto.runAfter, a.auto.diffSel = x.ID, runAfter, 0
	a.auto.diffs = nil
	lang := func(f string) *editor.Language { return editor.LanguageFor(f) }
	for _, ch := range automate.Changes(m.Code, x.Code()) {
		old, cur := editor.NewBuffer(ch.Old), editor.NewBuffer(ch.New)
		old.SetLanguage(lang(ch.File))
		cur.SetLanguage(lang(ch.File))
		a.auto.diffs = append(a.auto.diffs, autoDiff{change: ch, view: &editor.DiffState{Old: old, New: cur, Layout: editor.DiffUnified}})
	}
}

// approvalView is the dialog of the changes to approve.
func (a *app) approvalView(c *ui.Context) {
	if a.auto.approve == "" {
		return
	}
	t := c.Theme()
	open := true
	x := a.autoByID(a.auto.approve)
	if x == nil {
		a.auto.approve = ""
		return
	}
	ui.Modal(c.Key("auto-approve"), &open, func() {
		w, h := c.Size()
		ui.Column(c).Gap(10).Width(min(860, w-80)).Height(min(640, h-80)).Children(func() {
			ui.Text(c, "确认「"+x.Name+"」的代码").FontSize(17).Bold()
			ui.Text(c, "新写的或被改过的程序，要你看过才运行。下面是和上次确认时相比的改动。").FontSize(textSmall).TextColor(t.TextMuted).Wrap()
			if len(a.auto.diffs) == 0 {
				ui.Text(c, "代码没有变化。").TextColor(t.TextMuted)
			} else {
				ui.Row(c).Gap(4).Wrap().GapY(4).Children(func() {
					for i, d := range a.auto.diffs {
						mark := "改"
						switch {
						case d.change.Added:
							mark = "新"
						case d.change.Removed:
							mark = "删"
						}
						b := ui.Box(c.Key(fmt.Sprint("diff:", i))).Padding(3, 9).Radius(6).Cursor(ui.CursorPointer).Label(d.change.File)
						if i == a.auto.diffSel {
							b.Background(t.Accent.Alpha(0.16))
						}
						if b.Clicked() {
							a.auto.diffSel = i
						}
						b.Children(func() {
							ui.Text(c, mark+" "+d.change.File).FontSize(textSmall).Font("monospace").PassThrough()
						})
					}
				})
				if a.auto.diffSel < len(a.auto.diffs) {
					editor.DiffView(c.Key(fmt.Sprint("diff-view:", a.auto.diffSel)), a.auto.diffs[a.auto.diffSel].view).Grow(1).FillWidth().
						Radius(8).BorderWidth(1).BorderColor(hairline(t))
				}
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Spacer(c)
				if ui.Button(c, "取消").Clicked() {
					open = false
				}
				label := "确认"
				if a.auto.runAfter {
					label = "确认并运行"
				}
				if ui.PrimaryButton(c, label).Clicked() {
					open = false
					id, run := x.ID, a.auto.runAfter
					a.later(func() {
						a.autoRefresh(true)
						if x := a.autoByID(id); x != nil {
							a.auto.metas.Of(id).Approve(x.Code())
							_ = a.auto.metas.Save(a.autoRoot())
							a.auto.approved = true
							delete(a.auto.waiting, id)
							if run {
								a.autoStart(x, "手动")
							}
						}
					})
				}
			})
		})
	})
	if !open {
		a.auto.approve, a.auto.diffs = "", nil
	}
}

// confirmView asks what a running program asks, through mymind.confirm.
func (a *app) confirmView(c *ui.Context) {
	if len(a.auto.confirms) == 0 {
		return
	}
	t := c.Theme()
	q := a.auto.confirms[0]
	open := true
	answer := false
	ui.Modal(c.Key("auto-confirm"), &open, func() {
		w, _ := c.Size()
		ui.Column(c).Gap(10).Width(min(480, w-80)).Children(func() {
			ui.Text(c, "「"+q.name+"」问").FontSize(textSmall).TextColor(t.TextMuted)
			ui.Text(c, q.message).FontSize(16).Bold().Wrap()
			if q.detail != "" {
				ui.Scroll(c).MaxHeight(320).Children(func() {
					ui.Text(c, q.detail).FontSize(textSmall).Wrap().Selectable()
				})
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Spacer(c)
				if ui.Button(c, "不允许").Clicked() {
					open = false
				}
				if ui.PrimaryButton(c, "允许").Clicked() {
					open, answer = false, true
				}
			})
		})
	})
	if !open {
		q.answer <- answer
		a.dropConfirm(q)
	}
}

// autoInstall makes the shared Python environment and installs what the
// programs need, showing how it goes.
func (a *app) autoInstall() {
	if a.auto.installing {
		a.auto.logOpen = true
		return
	}
	a.auto.installing, a.auto.installLog, a.auto.logOpen = true, "", true
	root := a.autoRoot()
	_ = os.MkdirAll(root, 0o755)
	w := &postWriter{a: a}
	a.async(func() (any, error) {
		return nil, automate.Install(context.Background(), root, w)
	}, func(_ any, err error) {
		a.auto.installing = false
		if err != nil {
			a.auto.installLog += "\n" + err.Error()
		}
	})
}

// postWriter adds what is written to the install's log, on the main
// thread.
type postWriter struct{ a *app }

func (w *postWriter) Write(b []byte) (int, error) {
	s := string(b)
	w.a.post(func() { w.a.auto.installLog += s })
	return len(b), nil
}

func (a *app) installView(c *ui.Context) {
	if !a.auto.logOpen {
		return
	}
	t := c.Theme()
	ui.Modal(c.Key("auto-install"), &a.auto.logOpen, func() {
		w, h := c.Size()
		ui.Column(c).Gap(10).Width(min(640, w-80)).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "安装依赖").FontSize(17).Bold()
				if a.auto.installing {
					ui.Spinner(c)
				}
			})
			ui.Text(c, "所有程序共用一个 Python 环境（自动化/.venv），安装各自 requirements.txt 里的包。").FontSize(textSmall).TextColor(t.TextMuted).Wrap()
			ui.Scroll(c.Key("install-log")).Height(min(360, h-220)).Padding(10).Radius(8).Background(t.Surface).Children(func() {
				ui.Text(c, cmpOr(a.auto.installLog, "…")).Font("monospace").FontSize(12).Wrap().Selectable().FillWidth()
			})
			ui.Row(c).Children(func() {
				ui.Spacer(c)
				if ui.Button(c, "关闭").Clicked() {
					a.auto.logOpen = false
				}
			})
		})
	})
}

// homeAutomations is 概览's card of the automations: those running, those
// that want a look (failed, waiting for an approval), then those on a
// schedule, each with a button to run it.
func (a *app) homeAutomations(c *ui.Context) {
	a.autoRefresh(false)
	if len(a.auto.list) == 0 {
		return
	}
	t := c.Theme()
	rank := func(x *automate.Automation) int {
		m := a.auto.metas[x.ID]
		switch {
		case a.auto.procs[x.ID] != nil:
			return 0
		case x.Err != "", m != nil && (m.Status == automate.Failed || m.Status == automate.TimeOut):
			return 1
		case m.NeedsApproval(x.Code()):
			return 2
		case m != nil && m.Enabled:
			return 3
		}
		return 4
	}
	list := slices.Clone(a.auto.list)
	slices.SortStableFunc(list, func(x, y *automate.Automation) int { return rank(x) - rank(y) })
	pageCard(c).Key("home-auto").Children(func() {
		if cardHead(c, iconBolt, tileTeal, "自动化", "全部") {
			a.later(a.openAutomations)
		}
		for i, x := range list {
			if i == 6 {
				break
			}
			x := x
			row := ui.Row(c.Key("home-auto:"+x.ID)).Gap(10).Padding(5, 6).Radius(8).AlignItems(ui.Center).Cursor(ui.CursorPointer).
				Label("自动化 " + x.Name)
			if row.Hovered() {
				row.Background(t.Text.Alpha(0.05))
			}
			if row.Clicked() {
				a.later(func() {
					a.openAutomations()
					a.autoSelect(x.ID)
				})
			}
			status, col := a.autoStatus(t, x)
			running := a.auto.procs[x.ID] != nil
			row.Children(func() {
				ui.Box(c).Size(8, 8).Radius(4).Shrink(0).Background(col)
				ui.Text(c, x.Name).FontSize(13).Shrink(1).MinWidth(0).SingleLine()
				ui.Text(c, status).FontSize(12).TextColor(t.TextMuted).Grow(1).Shrink(1).MinWidth(0).SingleLine()
				if running {
					ui.Spinner(c)
				} else if iconButton(c.Key("home-run:"+x.ID), iconPlay, "运行 "+x.Name, "运行一次", 24, x.Err == "").Clicked() {
					a.later(func() {
						if a.auto.metas.Of(x.ID).NeedsApproval(x.Code()) {
							a.openAutomations()
							a.autoSelect(x.ID)
						}
						a.autoStart(x, "手动")
					})
				}
			})
		}
	})
}
