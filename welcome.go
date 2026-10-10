package main

import (
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// welcomeDoc is the map a new folder starts with: a short guide that is
// also an example of a project map.
func welcomeDoc(now time.Time) *model.Document {
	d := model.New("我的项目")
	add := func(parent *model.Node, title, note string, tags ...string) *model.Node {
		n, _ := d.AddChild(parent.ID, title)
		n.Note, n.Tags = note, tags
		return n
	}
	day := func(offset int) *time.Time {
		y, m, dd := now.Date()
		t := time.Date(y, m, dd+offset, 0, 0, 0, 0, now.Location())
		return &t
	}

	guide := add(d.Root, "使用指南", "选中节点后：\nTab 添加子节点，Enter 添加同级节点\n双击或 F2 / 空格 编辑标题\nDelete 删除，方向键移动选择\n⌘Z 撤销，⌘⇧Z 重做，⌘= / ⌘- 缩放\n拖动节点到另一个节点上，可以改变层级")
	add(guide, "右侧面板写备注、打标签", "备注支持多行文字。标签用于分类和筛选。", "示例")
	add(guide, "把节点设为任务", "在右侧面板勾选「作为任务」，可设置状态、优先级、开始和截止日期。\n任务会出现在「看板」和「日程」视图中。")

	arch := add(d.Root, "架构", "项目的模块划分、分层和数据流。", "架构")
	add(arch, "前端", "界面层。")
	add(arch, "后端", "服务与数据层。")

	feat := add(d.Root, "功能", "已有功能和它们的实现要点。")
	login := add(feat, "登录", "实现要点、关键文件、坑。", "核心")
	login.Task = &model.Task{Status: model.StatusDone, Priority: model.PriorityHigh}
	search := add(feat, "搜索", "", "核心")
	search.Task = &model.Task{Status: model.StatusDoing, Priority: model.PriorityMedium, Start: day(-2), Due: day(3)}

	opt := add(d.Root, "待优化", "下一步要优化的点，先放在这里，定期挑选。", "优化")
	slow := add(opt, "列表加载慢", "考虑分页或缓存。", "性能")
	slow.Task = &model.Task{Status: model.StatusTodo, Priority: model.PriorityHigh, Due: day(1)}
	idea := add(opt, "支持导出图片", "")
	idea.Task = &model.Task{Status: model.StatusIdea}
	return d
}
