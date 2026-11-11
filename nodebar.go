package main

import (
	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/layout"
)

// nodeBarWidth is the width of the bar by the node selected.
const nodeBarWidth = 5*30 + 2*4 + 9

// nodeBar is the small bar of the node selected, over the canvas's top:
// a child, its style and markers, a task, and the rest of its
// menu. It is not there while the node is edited, or with many
// selected.
func (a *app) nodeBar(c *ui.Context, r layout.Result, depth int) {
	n, _ := a.selectedNode()
	if n == nil || a.editing != "" || len(a.multi) > 0 {
		return
	}
	if _, ok := r.Boxes[n.ID]; !ok || a.vp.W < nodeBarWidth+2*190 {
		return // not shown, or no room beside the corners' buttons
	}
	t := c.Theme()
	// At the top of the canvas, in the middle: by the node it would hide
	// the nodes around it, which are clicked as often.
	x, y := (a.vp.W-nodeBarWidth)/2, float32(12)
	bar := lifted(ui.Row(c.Key("node-bar")).Absolute().Left(x).Top(y).Height(38).Padding(0, 5).Gap(0).AlignItems(ui.Center).
		Radius(radiusPill).Background(cardColor(t)), t).Label("主题操作")
	bar.Children(func() {
		// One button to add: a topic under this one, the usual; a sibling is
		// Return's, and in the menu at the bar's end.
		if iconButton(c, iconPlus, "加子主题", "加子主题（Tab）；加同级主题按回车", 30, true).Clicked() {
			a.later(a.addChild)
		}
		ui.Box(c).Size(1, 16).Margin(0, 3).Background(hairline(t))
		col := a.branchColor(n.ID)
		style := ui.Box(c.Key("样式")).Size(30, 30).Radius(6).Center().Cursor(ui.CursorPointer).Label("样式").Tooltip("颜色、形状和文字")
		if style.Hovered() {
			style.Background(t.SurfaceHover)
		}
		if style.Clicked() {
			a.later(func() { a.panelTab, a.panelOff = tabStyle, false })
		}
		style.Children(func() { ui.Box(c).Size(14, 14).Radius(7).Background(col).PassThrough() })
		if iconButton(c, iconTag, "标记", "旗标、优先级、进度等标记", 30, true).Clicked() {
			a.later(func() { a.panelTab, a.panelOff = tabMarkers, false })
		}
		task := "设为任务"
		if n.Task != nil {
			task = "取消任务"
		}
		tb := iconButton(c, iconCheck, task, task, 30, true)
		if n.Task != nil {
			tb.Background(t.Accent.Alpha(0.12))
		}
		if tb.Clicked() {
			a.later(func() { a.toggleTask(n) })
		}
		iconButton(c, iconMore, "更多操作", "概要、关联线、复制…", 30, true).Menu(func(m *ui.Menu) {
			a.nodeMenu(m, n, depth)
		})
	})
}
