package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

// The tour: on the first run, three cards on what the app is for and how
// to start, the map, the library and the AI. The settings show it again.

type tourCard struct {
	icon, title string
	points      []string
	action      string // the button that tries it
	run         func(a *app)
}

var tourCards = []tourCard{
	{"🗺", "用导图管理项目", []string{
		"把项目拆成模块、功能、实现细节和待办，一目了然。",
		"选中节点：Tab 加子节点，回车加同级，空格或双击改标题，Delete 删除。",
		"节点可以设为任务（状态、优先级、日期），在看板、日程、时间线里查看。",
		"⌘K 打开命令面板，什么都能搜到；⌘? 查看全部快捷键。",
	}, "从模板新建一张导图", func(a *app) { a.templatesOpen = true }},
	{"📚", "资料库：随手收藏，AI 整理", []string{
		"文字、链接、图片、文件都能收进左侧的「资料库」。",
		"在任何应用里复制后按 ⌃⌥V，直接收藏；⌃⌥Space 快速记一句话。",
		"AI 自动起标题、加说明、分类、打标签、生成同义词，按意思就能搜到。",
	}, "打开资料库", func(a *app) { a.showLibrary() }},
	{"✦", "让 AI 跟上你的项目", []string{
		"工具栏「✦ AI」：问导图里的内容，让 AI 拆任务、补备注、写周报；修改要你确认才生效。",
		"AI 回答时会检索你的资料库，并说明依据的是哪几条资料。",
		"帮助 →「连接 AI 编程工具（MCP）」：让 Claude Code 写代码时读写导图、更新任务进度。",
	}, "设置 AI 助手", func(a *app) {
		a.aiOpen, a.ai.settings = true, true
		a.loadKeyDrafts()
	}},
}

// tourView shows the tour's card of a.tourStep.
func (a *app) tourView(c *ui.Context) {
	if a.tourStep <= 0 || a.tourStep > len(tourCards) {
		return
	}
	t := c.Theme()
	open := true
	ui.Modal(c.Key("tour"), &open, func() {
		card := tourCards[a.tourStep-1]
		w, _ := c.Size()
		ui.Column(c).Gap(14).Width(min(520, w-80)).Children(func() {
			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, card.icon).FontSize(30)
				ui.Column(c).Gap(2).Grow(1).Children(func() {
					ui.Text(c, fmt.Sprintf("欢迎使用 MyMind · %d/%d", a.tourStep, len(tourCards))).FontSize(12).TextColor(t.TextMuted)
					ui.Text(c, card.title).FontSize(18).Bold()
				})
			})
			ui.Column(c).Gap(8).Children(func() {
				for i, p := range card.points {
					ui.Row(c.Key(i)).Gap(8).Children(func() {
						ui.Text(c, "•").TextColor(t.Accent)
						ui.Text(c, p).FontSize(13).Wrap().Grow(1)
					})
				}
			})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if ui.Link(c, "跳过", "").FontSize(12).Clicked() {
					a.endTour()
				}
				ui.Spacer(c)
				if ui.Button(c, card.action).Clicked() {
					run := card.run
					a.endTour()
					a.later(func() { run(a) })
				}
				if a.tourStep > 1 && ui.Button(c, "上一步").Clicked() {
					a.tourStep--
				}
				next := "下一步"
				if a.tourStep == len(tourCards) {
					next = "开始使用"
				}
				if ui.PrimaryButton(c, next).Clicked() {
					if a.tourStep == len(tourCards) {
						a.endTour()
					} else {
						a.tourStep++
					}
				}
			})
		})
	})
	if !open { // closed with Esc or a click outside
		a.endTour()
	}
}

func (a *app) endTour() {
	a.tourStep = 0
	if !a.cfg.TourDone {
		a.cfg.TourDone = true
		a.cfg.save()
	}
}
