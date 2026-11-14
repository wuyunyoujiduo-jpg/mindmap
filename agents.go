package main

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/gitbase"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// AI 动态 is what the AI agents reported on the projects, a list of
// 今天's left column: the user runs many agents and cannot follow each,
// and this is where they are caught up with. It reads the maps' logs
// (model.File.Log, written by internal/mcp) by day and by project: what
// a session did, a decision, a task or a thought written down, a change
// to a map. A click opens the thing; its menu takes it back. With a
// project chosen in the left column it is that project's record alone,
// the whole of it: how the project came to be as it is. What was
// reported since the list was last looked at is counted on its row and
// said on 今天, and marked with a dot here.

// agentItem is an entry of a map's log.
type agentItem struct {
	path string
	e    model.LogEntry
}

// agentRanges are how far back the list looks.
var agentRanges = []struct {
	name string
	days int
}{{"今天", 1}, {"7 天", 7}, {"30 天", 30}}

// agentItems are the entries of every map's log, the newest first.
func (a *app) agentItems() []agentItem {
	now := a.now()
	if now.Sub(a.tasks.agentsRead) < time.Second && a.tasks.agentsEdits == a.edits && !a.tasks.agentsRead.IsZero() {
		return a.tasks.agents
	}
	a.tasks.agentsRead, a.tasks.agentsEdits = now, a.edits
	var items []agentItem
	for _, fi := range a.files {
		f := a.file
		if fi.Path != a.path {
			f = a.indexed(fi.Path)
		}
		if f == nil {
			continue
		}
		for _, e := range f.Log {
			items = append(items, agentItem{fi.Path, e})
		}
	}
	slices.SortStableFunc(items, func(x, y agentItem) int { return y.e.At.Compare(x.e.At) })
	a.tasks.agents = items
	return items
}

// agentsUnseen counts what was reported since the list was last looked
// at, and the projects it was reported on.
func (a *app) agentsUnseen() (entries, projects int) {
	seen := map[string]bool{}
	for _, it := range a.agentItems() {
		if !it.e.At.After(a.cfg.AgentsSeen) {
			break
		}
		entries++
		if !seen[it.path] {
			seen[it.path] = true
			projects++
		}
	}
	return entries, projects
}

// showAgents shows the list, of all projects or of one, and takes what
// is there as seen: what was new stays marked while the list shows.
func (a *app) showAgents(onMap string) {
	if a.tasks.view != tasksAgents {
		a.tasks.agentsSeenWas = a.cfg.AgentsSeen
	}
	a.tasks.view, a.tasks.onMap = tasksAgents, onMap
	a.cfg.AgentsSeen = a.now()
	a.cfg.save()
}

// agentKind is the word for an entry's kind.
func agentKind(kind string) string {
	switch kind {
	case model.LogSession:
		return "会话"
	case model.LogDecision:
		return "决策"
	case model.LogTask:
		return "待办"
	case model.LogIdea:
		return "想法"
	}
	return "修改"
}

// agentDay heads a day of the list.
func agentDay(day, today time.Time) string {
	s := fmt.Sprintf("%d月%d日 %s", day.Month(), day.Day(), weekdays[day.Weekday()])
	switch daysBetween(day, today) {
	case 0:
		return "今天 · " + s
	case 1:
		return "昨天 · " + s
	}
	return s
}

// agentsView is the list's page.
func (a *app) agentsView(c *ui.Context) {
	now := a.now()
	day := today(now)
	one := a.tasks.onMap
	var items []agentItem
	for _, it := range a.agentItems() {
		if one != "" && it.path != one {
			continue
		}
		if one == "" && !it.e.At.After(day.AddDate(0, 0, 1-agentRanges[a.tasks.agentRange].days)) {
			break
		}
		items = append(items, it)
	}
	count := map[string]int{}
	projects := map[string]bool{}
	for _, it := range items {
		count[it.e.Kind]++
		projects[it.path] = true
	}
	ui.Row(c.Key("agents-top")).Gap(12).AlignItems(ui.Center).FillWidth().Children(func() {
		if one == "" {
			var names []string
			for _, r := range agentRanges {
				names = append(names, r.name)
			}
			if r := tabGroup(c, "agents-range", names, a.tasks.agentRange); r != a.tasks.agentRange {
				a.tasks.agentRange = r
			}
			figures(c, figure{len(projects), "个项目"}, figure{count[model.LogSession], "次会话"}, figure{count[model.LogDecision], "条决策"}, figure{count[model.LogTask], "条待办"})
		} else {
			figures(c, figure{count[model.LogSession], "次会话"}, figure{count[model.LogDecision], "条决策"}, figure{count[model.LogTask], "条待办"})
		}
		ui.Spacer(c)
		if len(items) > 0 && textLink(c, "复制成记录", "把这些复制成一段 Markdown").Clicked() {
			a.plat.CopyText(agentsReport(items, day))
			a.tell("已复制，可以粘贴到别处")
		}
	})
	if len(items) == 0 {
		title, sub := "这段时间 AI 没有记下什么", "接上 Claude Code 等 AI 编程工具后，它们做了什么、定了什么、留下什么待办，会按天汇总在这里。"
		if one != "" {
			title = "这个项目还没有 AI 的记录"
		}
		emptyState(c, "agents", "sparkles", title, sub, func() {
			if textKey(c, "连接 AI 编程工具…", "连接 AI 编程工具", "怎样让 AI 把做了什么记到这里").Clicked() {
				a.mcpOpen = true
			}
		})
		return
	}
	// By day, and in a day by project: a project's name once over what
	// was reported on it.
	for i := 0; i < len(items); {
		d := today(items[i].e.At)
		j := i
		for j < len(items) && today(items[j].e.At).Equal(d) {
			j++
		}
		dayItems := items[i:j]
		i = j
		ui.Column(c.Key("day:"+d.Format("2006-01-02"))).Margin(14, 0, 0).FillWidth().Children(func() {
			trayHead(c, "head", agentDay(d, day), fmt.Sprint(len(dayItems)), nil)
			var order []string
			by := map[string][]agentItem{}
			for _, it := range dayItems {
				if _, ok := by[it.path]; !ok {
					order = append(order, it.path)
				}
				by[it.path] = append(by[it.path], it)
			}
			for _, path := range order {
				ui.Column(c.Key("of:"+path)).FillWidth().Margin(0, 0, 8).Children(func() {
					if one == "" {
						ui.Row(c.Key("project")).Height(30).Padding(0, 6).Gap(8).AlignItems(ui.Center).FillWidth().Children(func() {
							mapDot(c, a.mapColor(path), 8)
							ui.Text(c, mapName(path)).FontSize(textBody).FontWeight(700).SingleLine().Shrink(1).MinWidth(0)
							ui.Spacer(c)
							if textLink(c, "只看它 ›", "只看 "+mapName(path)+" 的记录").Clicked() {
								a.later(func() { a.showAgents(path) })
							}
						})
					}
					listCard(c, "rows").Children(func() {
						for _, it := range by[path] {
							a.agentRow(c, it, now)
						}
					})
				})
			}
		})
	}
}

// agentRow is an entry: its kind, what it says, and when.
func (a *app) agentRow(c *ui.Context, it agentItem, now time.Time) {
	t := c.Theme()
	e := it.e
	row := listCardRow(c, "log:"+e.ID).MinHeight(42).AlignItems(ui.Start).Cursor(ui.CursorPointer).Label(agentKind(e.Kind) + " " + e.Text)
	if row.Clicked() {
		a.later(func() { a.openAgentItem(it) })
	}
	row.ContextMenu(func(m *ui.Menu) { a.agentMenu(m, it) })
	row.Children(func() {
		// Its kind, a small tablet of one width: a decision's in the accent,
		// being what is looked for here.
		bg, ink := blockFill(t), t.TextMuted
		if e.Kind == model.LogDecision {
			bg, ink = t.Accent.Alpha(0.1), t.Accent.MixOklab(t.Text, 0.25)
		}
		ui.Box(c.Key("kind")).Size(40, 22).Radius(6).Center().Background(bg).Shrink(0).Margin(2, 0, 0).PassThrough().Children(func() {
			ui.Text(c, agentKind(e.Kind)).FontSize(textCaption).TextColor(ink).NoWrap()
		})
		ui.Column(c).Gap(3).Grow(1).Shrink(1).Basis(0).MinWidth(0).Padding(3, 0, 0).PassThrough().Children(func() {
			text := ui.Text(c, library.Mask(e.Text)).FontSize(textBody).LineHeight(1.45)
			if e.Kind == model.LogSession {
				text.Wrap().MaxLines(4)
			} else {
				text.SingleLine()
			}
			if under := a.agentUnder(it); under != "" {
				ui.Text(c, under).FontSize(textCaption).TextColor(faint(t)).SingleLine()
			}
		})
		if e.At.After(a.tasks.agentsSeenWas) && !a.tasks.agentsSeenWas.IsZero() {
			ui.Box(c.Key("new")).Size(6, 6).Radius(3).Background(t.Accent).Shrink(0).Margin(9, 0, 0).Label("新的")
		}
		ui.Text(c, e.At.Local().Format("15:04")).FontSize(textCaption).TextColor(t.TextMuted).FontFeatures("tnum").NoWrap().Shrink(0).Margin(5, 0, 0).PassThrough()
		// One key for all that can be done, as the pointer's other key.
		more := ui.Box(c.Key("more")).Size(26, 26).Radius(radiusPill).Label("操作 " + e.Text).Tooltip("打开、撤回…").Shrink(0).
			Cursor(ui.CursorPointer).Background(t.Text.Alpha(0)).Transition(easeLook)
		dots := t.TextMuted.Alpha(0.7)
		if more.Hovered() {
			more.Background(t.Text.Alpha(0.06))
			dots = t.Text
		}
		more.Draw(func(p *ui.Painter, r ui.Rect) { drawIcon(p, r, 15, iconMore, dots) })
		more.Menu(func(m *ui.Menu) { a.agentMenu(m, it) })
	})
}

// agentUnder is the small line under an entry: of a session, who did it
// and where the code was; of a task, that it is done or gone.
func (a *app) agentUnder(it agentItem) string {
	e := it.e
	switch e.Kind {
	case model.LogSession:
		var parts []string
		if e.Agent != "" {
			parts = append(parts, agentName(e.Agent))
		}
		if e.Branch != "" {
			parts = append(parts, e.Branch)
		}
		if e.Commit != "" {
			parts = append(parts, gitbase.Short(e.Commit))
		}
		if n := len(e.Nodes); n > 0 {
			parts = append(parts, fmt.Sprintf("留下 %d 条待办", n))
		}
		return strings.Join(parts, " · ")
	case model.LogTask:
		if len(e.Nodes) == 0 {
			return ""
		}
		f := a.mapFile(it.path)
		if f == nil {
			return ""
		}
		switch n := f.Node(e.Nodes[0]); {
		case n == nil:
			return "已不在导图里"
		case n.Task != nil && n.Task.Status == model.StatusDone:
			return "已完成"
		case n.Task != nil && n.Task.Status == model.StatusDoing:
			return "进行中"
		}
	}
	return ""
}

// agentName is how an agent is named, of what it called itself.
func agentName(agent string) string {
	switch strings.ToLower(agent) {
	case "claude-code":
		return "Claude Code"
	case "claude-ai", "claude-desktop":
		return "Claude"
	case "cursor", "cursor-vscode":
		return "Cursor"
	}
	return agent
}

// openAgentItem goes to what an entry is of: the clip, the node, the map.
func (a *app) openAgentItem(it agentItem) {
	e := it.e
	if e.Clip != "" && a.lib.Find(e.Clip) != nil {
		a.showClip(e.Clip)
		return
	}
	if f := a.mapFile(it.path); f != nil && len(e.Nodes) > 0 {
		if d := f.SheetOf(e.Nodes[0]); d != nil {
			a.goToNode(it.path, d.ID, e.Nodes[0])
			return
		}
	}
	_ = a.open(it.path)
}

// agentMenu is the menu of an entry.
func (a *app) agentMenu(m *ui.Menu, it agentItem) {
	e := it.e
	if m.Item("打开").Chosen() {
		a.later(func() { a.openAgentItem(it) })
	}
	if m.Item("打开导图").Chosen() {
		a.later(func() { _ = a.open(it.path) })
	}
	if e.Kind == model.LogEdit && m.Item("看历史版本…").Chosen() {
		a.later(func() {
			if a.open(it.path) == nil {
				a.openHistory()
			}
		})
	}
	m.Separator()
	word := "从记录里删去"
	switch e.Kind {
	case model.LogTask:
		word = "撤回这条待办"
	case model.LogDecision:
		word = "删除这条决策"
	case model.LogIdea:
		word = "删除这条想法"
	}
	if m.Item(word).Chosen() {
		a.later(func() { a.takeBack(it) })
	}
}

// takeBack undoes what an entry reported: a task written down is taken
// off the map, a decision or a thought out of the library and off the
// node linked to it; a session or a change only leaves the record (the
// map's versions take a change back). All of it can be put back at once.
func (a *app) takeBack(it agentItem) {
	e := it.e
	type removed struct {
		node   *model.Node
		parent string
		index  int
	}
	var nodes []removed
	var unlinked []string
	link := "clip:" + e.Clip
	err := a.editMapAt(it.path, "撤回 AI 记下的："+e.Text, func(f *model.File) error {
		f.DropLog(e.ID)
		switch e.Kind {
		case model.LogTask:
			for _, id := range e.Nodes {
				d := f.SheetOf(id)
				if d == nil {
					continue
				}
				n, p := d.Find(id)
				if p == nil {
					continue
				}
				i := slices.Index(p.Children, n)
				p.Children = slices.Delete(p.Children, i, i+1)
				nodes = append(nodes, removed{n, p.ID, i})
			}
		case model.LogDecision, model.LogIdea:
			f.Walk(func(n *model.Node) {
				if i := slices.Index(n.Links, link); i >= 0 {
					n.Links = slices.Delete(n.Links, i, i+1)
					if len(n.Links) == 0 {
						n.Links = nil
					}
					unlinked = append(unlinked, n.ID)
				}
			})
		}
		return nil
	})
	if err != nil {
		a.tell("没有撤回：" + err.Error())
		return
	}
	var restoreClip func() error
	if e.Clip != "" && a.lib.Find(e.Clip) != nil {
		if restoreClip, err = a.lib.Take(e.Clip); err == nil {
			a.noteLibraryWrite()
			if a.libSel == e.Clip {
				a.libSel = ""
			}
		}
	}
	a.tasks.agentsRead = time.Time{}
	said := "已从记录里删去"
	switch e.Kind {
	case model.LogTask:
		said = "已撤回这条待办"
	case model.LogDecision, model.LogIdea:
		said = "已删除"
	}
	a.tellUndo(said, func() {
		if restoreClip != nil && restoreClip() == nil {
			a.noteLibraryWrite()
		}
		_ = a.editMapAt(it.path, "放回 AI 记下的："+e.Text, func(f *model.File) error {
			f.Log = append(f.Log, e)
			slices.SortStableFunc(f.Log, func(x, y model.LogEntry) int { return x.At.Compare(y.At) })
			for _, r := range slices.Backward(nodes) {
				if p := f.Node(r.parent); p != nil {
					p.Children = slices.Insert(p.Children, min(r.index, len(p.Children)), r.node)
				}
			}
			for _, id := range unlinked {
				if n := f.Node(id); n != nil && !slices.Contains(n.Links, link) {
					n.Links = append(n.Links, link)
				}
			}
			return nil
		})
		a.tasks.agentsRead = time.Time{}
	})
}

// agentsReport is the entries as Markdown, to paste elsewhere.
func agentsReport(items []agentItem, today time.Time) string {
	var b strings.Builder
	day, path := time.Time{}, ""
	for _, it := range items {
		if d := today.AddDate(0, 0, -daysBetween(it.e.At, today)); !d.Equal(day) {
			day, path = d, ""
			fmt.Fprintf(&b, "\n## %d月%d日 %s\n", d.Month(), d.Day(), weekdays[d.Weekday()])
		}
		if it.path != path {
			path = it.path
			fmt.Fprintf(&b, "\n### %s\n\n", mapName(path))
		}
		fmt.Fprintf(&b, "- %s **%s** %s\n", it.e.At.Local().Format("15:04"), agentKind(it.e.Kind), strings.ReplaceAll(library.Mask(it.e.Text), "\n", " "))
	}
	return strings.TrimSpace(b.String()) + "\n"
}

// agentsLine says, on 今天, that the agents reported something since the
// list was last looked at, with the way to it.
func (a *app) agentsLine(c *ui.Context) {
	n, projects := a.agentsUnseen()
	if n == 0 {
		return
	}
	t := c.Theme()
	row := ui.Row(c.Key("agents-line")).MinHeight(36).Padding(6, 10).Gap(8).Radius(radiusRow).AlignItems(ui.Center).FillWidth().
		Cursor(ui.CursorPointer).Label("AI 动态").Background(t.Accent.Alpha(0.07)).Transition(rowMotion)
	if row.Hovered() {
		row.Background(t.Accent.Alpha(0.11))
	}
	if row.Clicked() {
		a.later(func() { a.showAgents("") })
	}
	row.Children(func() {
		glyph(c, iconSparkle, 14, t.Accent).PassThrough()
		ui.Text(c, fmt.Sprintf("AI 新记了 %d 条，在 %d 个项目里", n, projects)).FontSize(textBody).Grow(1).Shrink(1).MinWidth(0).SingleLine().PassThrough()
		ui.Text(c, "去看 ›").FontSize(textSmall).TextColor(t.Accent).NoWrap().PassThrough()
	})
}
