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
		"节点可以设为任务（状态、优先级、日期，可以重复，可以定时提醒）；所有导图的任务都在「今天」里，按列表、周、月来看，左边还有已逾期和本周回顾。",
		"「项目」里每张导图是一张卡片，左边的目录可以直接在导图之间切换；⌘K 什么都能搜到，搜到的任务可以直接勾掉。⌥⌘1 / 2 / 3 在今天、项目、收集之间切换。",
	}, "从模板新建一张导图", func(a *app) { a.templatesOpen = true }},
	{"📚", "收集：随手记下，需要时再挂到导图上", []string{
		"顶上的输入栏在每一页都有：直接写是灵感，以「周五」开头是任务，贴链接是收藏，以 ? 开头是问 AI。",
		"在任何应用里复制后按 ⌃⌥V，直接收藏；⌃⌥Space 快速记一句话；在应用里按 ⌘J 开始记一条。「明天 9点 写周报」这样写，会带上日期和提醒时间。",
		"记下的东西就留在收集里，用 #标签 和搜索找。和某个项目有关的，可以挂到那张导图上；想晚点再决定的，标为「待归位」，它会在「今天」里等着。",
		"内容里像密钥或密码的部分会自动遮住，也不会发给 AI；点开详情才看得到。",
		"写灵感或节点备注时打 [[，可以链接到别的灵感、资料、导图或导图里的主题；被链接的那一边也会显示谁链接了它。",
	}, "打开收集", func(a *app) { a.showLibrary() }},
	{"✦", "让 AI 跟上你的项目", []string{
		"工具栏「✦ AI」：问导图里的内容，让 AI 拆任务、补备注、写周报；修改要你确认才生效。",
		"AI 回答时会检索你收集的资料，并说明依据的是哪几条。",
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
	modal(c, "tour", &open, func() {
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
				if button(c, card.action).Clicked() {
					run := card.run
					a.endTour()
					a.later(func() { run(a) })
				}
				if a.tourStep > 1 && button(c, "上一步").Clicked() {
					a.tourStep--
				}
				next := "下一步"
				if a.tourStep == len(tourCards) {
					next = "开始使用"
				}
				if inkButton(c, next, true).Clicked() {
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
