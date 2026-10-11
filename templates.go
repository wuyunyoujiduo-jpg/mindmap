package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/formats"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// Templates to start maps from: built-in ones for planning, technical
// designs, iterations, retrospectives and AI-driven development, and the
// user's own, maps saved as templates in the app's folder.

// template is a map to start from.
type template struct {
	name, desc string
	outline    string // the map as a Markdown outline, for built-in ones
	path       string // the file, for the user's
}

var builtinTemplates = []template{
	{name: "AI 开发管理", desc: "跟上 AI 的节奏：架构、功能、实现细节、决策、待优化和下一步", outline: `# 项目名称
## 架构
- 模块 A
- 模块 B
- 数据与存储
## 功能清单
- [ ] 功能 1
- [ ] 功能 2
## 实现细节
- 关键文件与约定
- 踩过的坑
## 决策记录
- 为什么选这个方案
## 待优化
- [ ] 性能
- [ ] 体验
## 下一步
- [ ] 本周要做的`},
	{name: "项目规划", desc: "目标、范围、里程碑、风险和分工", outline: `# 项目名称
## 目标
- 要解决的问题
- 成功的标准
## 范围
- 必须有
- 可以有
- 不做
## 里程碑
- [ ] 原型
- [ ] 内测
- [ ] 上线
## 风险
- 技术风险
- 进度风险
## 分工
- 负责人`},
	{name: "技术方案", desc: "背景、目标、设计、备选方案、风险与上线计划", outline: `# 技术方案：名称
## 背景
## 目标与非目标
- 目标
- 非目标
## 方案设计
- 架构
- 数据模型
- 接口
## 备选方案
- 方案 B：为什么不选
## 风险与回滚
## 上线计划
- [ ] 灰度
- [ ] 全量`},
	{name: "迭代计划", desc: "本迭代的目标、任务和回顾", outline: `# 第 N 周迭代
## 目标
## 任务
- [ ] 任务 1
- [ ] 任务 2
- [ ] 任务 3
## 风险与阻塞
## 回顾
- 做得好的
- 要改进的`},
	{name: "复盘", desc: "回顾目标与结果，提炼经验和行动项", outline: `# 复盘：主题
## 目标回顾
## 结果
## 做得好的
## 待改进的
## 根因分析
## 行动项
- [ ] 行动 1
- [ ] 行动 2`},
}

func templatesDir() string {
	p := configPath()
	if p == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(p), "templates")
}

// templates are the built-in templates, then the user's.
func (a *app) templates() []template {
	out := slices.Clone(builtinTemplates)
	if dir := templatesDir(); dir != "" {
		if files, err := model.ListFiles(dir); err == nil {
			for _, f := range files {
				out = append(out, template{name: f.Name, desc: "我的模板", path: f.Path})
			}
		}
	}
	return out
}

// newFromTemplate makes a map from a template in the current folder and
// opens it.
func (a *app) newFromTemplate(tp template) {
	var f *model.File
	if tp.path != "" {
		g, err := model.Load(tp.path)
		if err != nil {
			a.saveErr = err.Error()
			return
		}
		f = g.Clone()
		for _, d := range f.Sheets { // new IDs, as for a copy
			d.ID = model.NewID()
			d.Root = d.Root.Copy()
			for i := range d.Floating {
				d.Floating[i] = d.Floating[i].Copy()
			}
			d.Relations = nil
		}
	} else {
		f = formats.ParseMarkdown(tp.outline, tp.name)
	}
	path := freeMapPath(a.currentFolder(), tp.name)
	if tp.path != "" {
		for _, d := range f.Sheets {
			copyAssets(d.Root, model.AssetsDir(tp.path), model.AssetsDir(path))
		}
	}
	if err := f.Save(path); err != nil {
		a.saveErr = err.Error()
		return
	}
	a.refreshFiles()
	if a.open(path) == nil {
		a.startEdit(a.doc.Root.ID)
	}
}

// saveAsTemplate keeps the open map as a template.
func (a *app) saveAsTemplate() {
	dir := templatesDir()
	if dir == "" || a.file == nil {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		a.saveErr = err.Error()
		return
	}
	path := freeMapPath(dir, mapName(a.path))
	if err := a.file.Save(path); err != nil {
		a.saveErr = err.Error()
		return
	}
	for _, d := range a.file.Sheets {
		copyAssets(d.Root, model.AssetsDir(a.path), model.AssetsDir(path))
	}
	a.tell("已存为模板「" + mapName(path) + "」")
}

func (a *app) deleteTemplate(tp template) {
	if tp.path == "" {
		return
	}
	_ = os.Remove(tp.path)
	_ = os.RemoveAll(model.AssetsDir(tp.path))
}

// templatesView is the window of templates to start a map from.
func (a *app) templatesView(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c.Key("templates"), &a.templatesOpen, func() {
		w, _ := c.Size()
		ui.Column(c).Gap(14).Width(min(720, w-80)).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, "从模板新建").FontSize(17).Bold().Grow(1)
				if a.file != nil && ui.Button(c, "把当前导图存为模板").Clicked() {
					a.saveAsTemplate()
				}
				if iconButton(c, iconClose, "关闭", "关闭（Esc）", 28, true).Clicked() {
					a.templatesOpen = false
				}
			})
			ui.Grid(c).Columns(2).Gap(10).Children(func() {
				for _, tp := range a.templates() {
					card := ui.Column(c.Key(tp.name+tp.path)).Padding(12).Gap(6).Radius(10).Border(1, t.Border).
						Cursor(ui.CursorPointer).Label("模板 " + tp.name)
					if card.Hovered() {
						card.Background(t.SurfaceHover)
					}
					if card.Clicked() {
						tp := tp
						a.templatesOpen = false
						a.later(func() { a.newFromTemplate(tp) })
					}
					if tp.path != "" {
						card.ContextMenu(func(m *ui.Menu) {
							if m.Item("删除模板").Chosen() {
								a.deleteTemplate(tp)
							}
						})
					}
					card.Children(func() {
						ui.Text(c, tp.name).FontSize(14).Bold()
						ui.Text(c, tp.desc).FontSize(12).TextColor(t.TextMuted).Wrap()
						if tp.outline != "" {
							var heads []string
							for _, l := range strings.Split(tp.outline, "\n") {
								if strings.HasPrefix(l, "## ") {
									heads = append(heads, strings.TrimPrefix(l, "## "))
								}
							}
							ui.Text(c, strings.Join(heads, " · ")).FontSize(11).TextColor(t.TextMuted).MaxLines(2)
						}
					})
				}
			})
		})
	})
}
