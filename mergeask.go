package main

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/ghsync"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// A map changed on two computers is merged topic by topic by the sync
// (mergeMapVersions); what both changed, each its own way, is this
// computer's until the user chooses, as git asks: a window lists each
// place, where its topic is and what it was when the two last agreed,
// then the two versions side by side, each with when and on which
// computer it was made, one chosen by a click. Two values with nothing
// round them were two to guess between, the user found.

// mergeAsk is a merged map's clashes, waiting for the user's choice.
type mergeAsk struct {
	path    string
	clashes []model.Clash
	where   []string // what each clash is of, in words
	under   []string // where its topic is: "收件箱 › 待整理"
	theirs  []bool   // the other computer's chosen
	when    ghsync.MergeInfo
}

// askMerge queues the clashes of a map merged, for mergeView to ask.
func (a *app) askMerge(path string, clashes []model.Clash, when ghsync.MergeInfo) {
	if len(clashes) == 0 {
		return
	}
	f, _ := model.Load(path)
	m := &mergeAsk{path: path, clashes: clashes, theirs: make([]bool, len(clashes)), when: when}
	for _, c := range clashes {
		m.where = append(m.where, clashWhere(f, c))
		m.under = append(m.under, clashUnder(f, c))
	}
	a.merges = slices.DeleteFunc(a.merges, func(x *mergeAsk) bool { return x.path == path })
	a.merges = append(a.merges, m)
	a.mergeOpen = true
}

// mergeDone applies the choices of the first map asked, and asks the next.
func (a *app) mergeDone(apply bool) {
	if len(a.merges) == 0 {
		return
	}
	m := a.merges[0]
	a.merges = a.merges[1:]
	a.mergeOpen = len(a.merges) > 0
	n := 0
	for _, t := range m.theirs {
		if t {
			n++
		}
	}
	if !apply || n == 0 {
		a.tell(fmt.Sprintf("「%s」两边都改的 %d 处，留了这台电脑的", mapName(m.path), len(m.clashes)))
		return
	}
	if m.path == a.path && !a.dirty && !modTime(a.path).Equal(a.modTime) {
		a.reload() // the merge, not yet read by tick
	}
	err := a.editMapAt(m.path, "合并时选了另一台电脑的", func(f *model.File) error {
		for i, c := range m.clashes {
			if m.theirs[i] {
				f.TakeTheirs(c)
			}
		}
		return nil
	})
	if err != nil {
		a.tell("没能改好「" + mapName(m.path) + "」：" + err.Error())
		return
	}
	a.tell(fmt.Sprintf("「%s」已合并：%d 处用了另一台电脑的", mapName(m.path), n))
}

// mergeView is the window that asks.
func (a *app) mergeView(c *ui.Context) {
	if len(a.merges) == 0 {
		a.mergeOpen = false
		return
	}
	t := c.Theme()
	m := a.merges[0]
	if modal(c, "merge", &a.mergeOpen, func() {
		w, h := c.Size()
		ui.Column(c).Gap(12).Width(min(600, w-80)).Children(func() {
			ui.Text(c, "合并「"+mapName(m.path)+"」").FontSize(17).Bold()
			ui.Text(c, fmt.Sprintf("两台电脑都改了这张导图，各自改的已合在一起。下面 %d 处两边改成了不一样的：对照原来是什么、哪边较新，每处选一个留下。", len(m.clashes))).
				FontSize(12).TextColor(t.TextMuted).Wrap()
			ui.Scroll(c.Key("clashes")).FillWidth().MaxHeight(max(160, h-280)).Gap(10).Children(func() {
				for i := range m.clashes {
					a.clashRow(c, m, i)
				}
			})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if textKey(c, "全用这台的", "merge-all-ours", "").Clicked() {
					clear(m.theirs)
				}
				if textKey(c, "全用另一台的", "merge-all-theirs", "").Clicked() {
					for i := range m.theirs {
						m.theirs[i] = true
					}
				}
				ui.Spacer(c)
				if len(a.merges) > 1 {
					ui.Text(c, fmt.Sprintf("还有 %d 张", len(a.merges)-1)).FontSize(textCaption).TextColor(t.TextMuted)
				}
				if inkButton(c, "完成", true).Clicked() {
					a.later(func() { a.mergeDone(true) })
				}
			})
		})
	}).Dismissed() {
		a.mergeOpen = true // closed: this computer's stays, said so
		a.later(func() { a.mergeDone(false) })
	}
}

// clashRow is one place both changed: what it is, and the two versions.
func (a *app) clashRow(c *ui.Context, m *mergeAsk, i int) {
	t := c.Theme()
	cl := m.clashes[i]
	now := a.now()
	ours, theirs := m.when.LocalAt, m.when.RemoteAt
	later := 0 // 1 ours, 2 theirs
	if !ours.IsZero() && !theirs.IsZero() {
		if ours.After(theirs) {
			later = 1
		} else if theirs.After(ours) {
			later = 2
		}
	}
	ui.Column(c.Key(fmt.Sprint("clash", i))).Gap(6).FillWidth().Children(func() {
		ui.Column(c).Gap(2).Children(func() {
			ui.Text(c, m.where[i]).FontSize(textSmall).FontWeight(600).Wrap()
			if m.under[i] != "" {
				ui.Text(c, "在 "+m.under[i]).FontSize(textCaption).TextColor(t.TextMuted).Wrap()
			}
		})
		// What it was when the two last agreed: what each side did to it.
		was := "原来：" + clashValue(cl.Field, cl.Base)
		switch {
		case cl.NoBase:
			was = "这是旧版本同步留下的冲突副本，不知道两边原来一致时是什么"
		case !m.when.BaseAt.IsZero():
			was += "（上次两边一致，" + whenLabel(m.when.BaseAt.Local(), now) + " 同步时）"
		}
		ui.Text(c, was).FontSize(textCaption).TextColor(t.Text.MixOklab(t.TextMuted, 0.5)).Wrap()
		ui.Row(c).Gap(8).FillWidth().Children(func() {
			side := func(key, name string, at time.Time, newer bool, v json.RawMessage, theirs bool) {
				chosen := m.theirs[i] == theirs
				b := ui.Column(c.Key(key)).Grow(1).Basis(0).Gap(3).Padding(9, 12).Radius(radiusCard).
					Cursor(ui.CursorPointer).Label(name)
				if chosen {
					b.Background(t.Accent.Alpha(0.08)).InsetShadow(0, 0, 0, 1.5, t.Accent).Transition(easeLook)
				} else {
					wallCard(b, t)
				}
				if b.Clicked() {
					m.theirs[i] = theirs
				}
				b.Children(func() {
					col := t.TextMuted
					if chosen {
						col = t.Accent
					}
					ui.Row(c).Gap(6).AlignItems(ui.Center).PassThrough().Children(func() {
						ui.Text(c, name).FontSize(textCaption).TextColor(col).NoWrap().PassThrough()
						if newer {
							ui.Text(c, "较新").FontSize(11).TextColor(t.Accent).Padding(1, 6).Radius(5).
								Background(t.Accent.Alpha(0.1)).NoWrap().PassThrough()
						}
					})
					if !at.IsZero() {
						ui.Text(c, "导图最后改于 "+whenLabel(at.Local(), now)).FontSize(11).TextColor(t.TextMuted).PassThrough()
					}
					ui.Text(c, clashValue(cl.Field, v)).FontSize(textSmall).Wrap().MaxLines(6).Margin(3, 0, 0).PassThrough()
				})
			}
			other := "另一台电脑"
			if m.when.RemoteFrom != "" {
				other += " · " + m.when.RemoteFrom
			}
			side("ours", "这台电脑", ours, later == 1, cl.Ours, false)
			side("theirs", other, theirs, later == 2, cl.Theirs, true)
		})
	})
}

// clashWhere says what a clash is of: 「主题」的标题.
func clashWhere(f *model.File, c model.Clash) string {
	field := fieldName(c.Field)
	switch {
	case c.Node != "":
		title := "一个主题"
		var was string
		if slices.Equal(c.Field, []string{"title"}) && json.Unmarshal(c.Base, &was) == nil && strings.TrimSpace(was) != "" {
			title = "「" + strings.TrimSpace(was) + "」" // its title the two agreed on, not this one's
		} else if f != nil {
			if n := f.Node(c.Node); n != nil {
				title = "「" + cmpOr(strings.TrimSpace(n.Title), "无标题") + "」"
			}
		}
		return title + "的" + field
	case c.Sheet != "":
		title := "一张画布"
		if f != nil {
			if d, _ := f.Sheet(c.Sheet); d != nil {
				title = "画布「" + d.Title + "」"
			}
		}
		return title + "的" + field
	}
	return "导图的" + field
}

// clashUnder says where a clash's topic is: the titles above it, the
// canvas's too when the map has several.
func clashUnder(f *model.File, c model.Clash) string {
	if f == nil || c.Node == "" {
		return ""
	}
	d := f.SheetOf(c.Node)
	if d == nil {
		return ""
	}
	var parts []string
	if len(f.Sheets) > 1 {
		parts = append(parts, "画布「"+d.Title+"」")
	}
	path := d.Path(c.Node)
	for _, n := range path[:max(0, len(path)-1)] {
		parts = append(parts, cmpOr(strings.TrimSpace(n.Title), "无标题"))
	}
	return strings.Join(parts, " › ")
}

// fieldName names a field of a topic, a canvas or a map in words.
func fieldName(path []string) string {
	names := map[string]string{
		"title": "标题", "note": "备注", "tags": "标签", "markers": "标记", "task": "任务",
		"collapsed": "折叠", "style": "样式", "image": "图片", "links": "链接", "files": "附件",
		"summary": "概要", "boundary": "外框", "pos": "位置", "values": "数值",
		"structure": "结构", "present": "演示", "color": "颜色", "project": "项目文件夹",
		"repo": "仓库", "baseline": "代码基线",
	}
	tasks := map[string]string{
		"status": "任务状态", "priority": "优先级", "start": "开始日期", "due": "截止日期",
		"doneAt": "完成时间", "repeat": "重复", "remind": "提醒", "estimate": "估时", "after": "前置任务",
	}
	if len(path) >= 2 && path[0] == "task" {
		if s, ok := tasks[path[1]]; ok {
			return s
		}
	}
	if len(path) > 0 {
		if s, ok := names[path[0]]; ok {
			return s
		}
		return path[0]
	}
	return "内容"
}

// clashValue says a version of a field in words.
func clashValue(path []string, v json.RawMessage) string {
	if len(v) == 0 || string(v) == "null" {
		return "（无）"
	}
	var s string
	if json.Unmarshal(v, &s) == nil {
		if len(path) >= 2 && path[0] == "task" && path[1] == "status" {
			return model.Status(s).Label()
		}
		if at, err := time.Parse(time.RFC3339, s); err == nil {
			if at.Hour() == 0 && at.Minute() == 0 {
				return at.Format("2006-01-02")
			}
			return at.Local().Format("2006-01-02 15:04")
		}
		return cmpOr(s, "（空）")
	}
	var n float64
	if json.Unmarshal(v, &n) == nil {
		if len(path) >= 2 && path[0] == "task" && path[1] == "priority" {
			return model.Priority(n).Label()
		}
		return fmt.Sprint(n)
	}
	var b bool
	if json.Unmarshal(v, &b) == nil {
		if b {
			return "是"
		}
		return "否"
	}
	var list []string
	if json.Unmarshal(v, &list) == nil {
		return strings.Join(list, "、")
	}
	if len(path) == 1 && path[0] == "task" {
		var task model.Task
		if json.Unmarshal(v, &task) == nil {
			out := task.Status.Label()
			if task.Due != nil {
				out += "，" + task.Due.Format("01-02") + " 截止"
			}
			return out
		}
	}
	return "有改动"
}
