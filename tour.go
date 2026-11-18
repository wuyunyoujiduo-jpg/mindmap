package main

import (
	"fmt"
	"strings"

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
		"选中主题：Tab 加子主题，回车加同级，空格或双击改标题，Delete 删除。",
		"不想挂在树上的想法：双击画布空白处放一个自由主题，随意摆；把主题拖到空白处就取出来，拖回主题上就挂回去。",
		"主题可以设为任务（状态、优先级、日期，可以重复，可以定时提醒）；所有导图的任务都在「今天」里，按列表、周、月来看，左边还有已逾期和本周回顾。",
		"「项目」里每张导图是一张卡片，左边的目录可以直接在导图之间切换；⌘K 什么都能搜到，搜到的任务可以直接勾掉。⌥⌘1 / 2 / 3 / 4 在今天、项目、收集、技能之间切换。",
	}, "从模板新建一张导图", func(a *app) { a.templatesOpen = true }},
	{"📚", "收集：随手记下，需要时再挂到导图上", []string{
		"顶上的输入栏在每一页都有：直接写是随记，以「周五」开头是任务，贴链接是收藏，以 ? 开头是问 AI。",
		"在任何应用里复制后按 {clip}，直接收藏；{capture} 快速记一句话；在应用里按 ⌘J 开始记一条。「明天 9点 写周报」这样写，会带上日期和提醒时间。",
		"记下的东西就留在收集里，用 #标签 和搜索找。和某个项目有关的，可以挂到那张导图上；想晚点再决定的，标为「待归位」，它会在「今天」里等着。",
		"内容里像密钥或密码的部分会自动遮住，也不会发给 AI；打开详情才看得到。",
		"写随记或主题备注时打 [[，可以链接到别的随记、资料、导图或导图里的主题；被链接的那一边也会显示谁链接了它。",
	}, "打开收集", func(a *app) { a.showLibrary() }},
	{"✦", "让 AI 跟上你的项目", []string{
		"顶上一排的 ✦：打开 AI 助手，问导图里的内容，让 AI 拆任务、补备注、写周报；修改要你确认才生效。",
		"AI 回答时会检索你收集的资料，并说明依据的是哪几条。",
		"帮助 →「连接 AI 编程工具（MCP）」：让 Claude Code 写代码时读写导图、更新任务进度。",
	}, "设置 AI 助手", func(a *app) {
		a.aiOpen, a.ai.settings = true, true
		a.loadKeyDrafts()
	}},
}

// phoneTourCards are the tour on a phone: the same three, said of
// fingers and of the bar at the foot, with no key of a keyboard, and the
// sync where a desktop has its agents.
var phoneTourCards = []tourCard{
	{"🗺", "用导图管理项目", []string{
		"点一个主题，底部浮出一排键：加子主题、加同级、改字、记成任务、详情；⋯ 里是其余的。",
		"一根手指拖动画布，两根手指缩放；按住一个主题半秒拿起来，放到另一个主题上就挂到它下面。",
		"写长一点、整理结构，用「大纲」最顺手：键盘上面有缩进、上下移、记成任务。",
		"所有导图的任务都在「今天」里，按列表或日程看；一行往右滑是完成，往左滑改到明天或删除。",
	}, "从模板新建一张导图", func(a *app) { a.templatesOpen = true }},
	{"📚", "收集：随手记下，需要时再挂到导图上", []string{
		"最下面的输入框在每一页都有：在今天记成任务，别处记成随记，右边的标签写着，点一下能换；贴链接是收藏，以 ? 开头是问 AI。",
		"「明天 9点 写周报」这样写，会带上日期和提醒时间。",
		"记下的东西就留在收集里，用 #标签 和搜索找。和某个项目有关的，可以挂到那张导图上；想晚点再决定的，标为「待归位」，它会在「今天」里等着。",
		"内容里像密钥或密码的部分会自动遮住，也不会发给 AI；打开详情才看得到。",
	}, "打开收集", func(a *app) { a.goCollect() }},
	{"✦", "让 AI 跟上你的项目", []string{
		"右上角的 ✦：打开 AI 助手，问导图里的内容，让 AI 拆任务、补备注、写周报；修改要你确认才生效。",
		"AI 回答时会检索你收集的资料，并说明依据的是哪几条。",
		"在 设置 → 同步 填上和电脑一样的 GitHub 仓库和令牌，手机和电脑上就是同一份导图和收集。",
	}, "设置 AI 助手", func(a *app) {
		a.aiOpen, a.ai.settings = true, true
		a.loadKeyDrafts()
	}},
}

// tourKeys puts into a line of the tour the keys it names that the
// settings can change, {clip} and {capture}, as they are set now: written
// in, they were the keys of before once one was changed.
func (a *app) tourKeys(line string) string {
	return strings.NewReplacer("{clip}", keyLabel(a.clipKey()), "{capture}", keyLabel(a.captureKey())).Replace(line)
}

// tourView shows the tour's card of a.tourStep.
func (a *app) tourView(c *ui.Context) {
	if a.tourStep <= 0 || a.tourStep > len(tourCards) {
		return
	}
	t := c.Theme()
	open := true
	cards := tourCards
	if narrowLayout {
		cards = phoneTourCards
	}
	modal(c, "tour", &open, func() {
		card := cards[a.tourStep-1]
		w, _ := c.Size()
		room := min(520, w-80)
		if narrowLayout {
			room = w - 72
		}
		ui.Column(c).Gap(14).Width(room).Children(func() {
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
						ui.Text(c, a.tourKeys(p)).FontSize(13).Wrap().Grow(1)
					})
				}
			})
			if narrowLayout { // its keys one over another, a finger's width each
				a.tourKeysPhone(c, card, len(cards))
				return
			}
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

// tourKeysPhone are the tour's keys on a phone: what is next the width of
// the card, under it what the card tries and the step back side by side,
// and 跳过 under them: in a row they ran past the card's edge.
func (a *app) tourKeysPhone(c *ui.Context, card tourCard, steps int) {
	ui.Column(c.Key("tour-keys")).Gap(8).FillWidth().Children(func() {
		next := "下一步"
		if a.tourStep == steps {
			next = "开始使用"
		}
		if inkButton(c, next, true).FillWidth().MinHeight(44).Clicked() {
			if a.tourStep == steps {
				a.endTour()
			} else {
				a.tourStep++
			}
		}
		ui.Row(c).Gap(8).FillWidth().Children(func() {
			if button(c, card.action).Grow(1).Basis(0).MinHeight(44).Clicked() {
				run := card.run
				a.endTour()
				a.later(func() { run(a) })
			}
			if a.tourStep > 1 && button(c, "上一步").Grow(1).Basis(0).MinHeight(44).Clicked() {
				a.tourStep--
			}
		})
		ui.Row(c).Justify(ui.Center).FillWidth().Children(func() {
			if ui.Link(c, "跳过", "").FontSize(textSmall).NoWrap().Clicked() {
				a.endTour()
			}
		})
	})
}
