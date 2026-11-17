package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// Focus, multiple selection, copying branches and opening levels.

// focusNode is the branch the canvas shown is focused on, or nil when it
// shows the whole map.
func (a *app) focusNode() *model.Node {
	if a.doc == nil {
		return nil
	}
	n, _ := a.doc.Find(a.focus[a.doc.ID])
	if n == a.doc.Root {
		return nil
	}
	return n
}

// shownDoc is the canvas as the canvas shows it: the branch focused on as
// its root, or the whole canvas.
func (a *app) shownDoc() *model.Document {
	f := a.focusNode()
	if f == nil {
		return a.doc
	}
	d := *a.doc
	d.Root, d.Floating = f, nil
	return &d
}

// shownRoot is the node the canvas shows in the middle.
func (a *app) shownRoot() *model.Node { return a.shownDoc().Root }

// focusOn shows only the branch of the node, as the middle of the canvas;
// "" or the root shows the whole map again.
func (a *app) focusOn(id string) {
	if a.doc == nil {
		return
	}
	a.commitEditIfAny()
	if id == a.doc.Root.ID {
		id = ""
	}
	if a.focus == nil {
		a.focus = map[string]string{}
	}
	if id == "" {
		delete(a.focus, a.doc.ID)
	} else {
		a.focus[a.doc.ID] = id
		if n, _ := a.doc.Find(id); n != nil && n.Collapsed {
			a.mutate("", func() { n.Collapsed = false })
		}
		a.selected = id
	}
	a.multi = nil
	c := a.cam()
	c.placed = false // centered anew on the new middle
	a.reveal = true
}

// The menus' items that change the open map, or how it shows, were done
// behind whatever page showed: on 今天, 收集 or 技能, ⌘T gave the map a
// canvas, ⌘1–5 turned its view, ⌥0–3 folded it and ⌥↩ showed a branch of
// it alone, none of it seen. They are done to the map seen.

// toMap shows the open map where another place's page is over it, and
// tells whether it was showing already.
func (a *app) toMap() bool {
	if a.doc == nil {
		return false
	}
	if a.libOpen {
		a.libOpen = false
		return false
	}
	return true
}

// menuOnMap is an item that rearranges the map: folds it, opens it to a
// level, shows a branch alone. With another page over the map it shows
// the map, as it is, and does no more: what it would rearrange was not
// being looked at. Chosen again, on the map, it acts.
func (a *app) menuOnMap(fn func()) {
	if a.toMap() {
		fn()
	}
}

// menuView is a view of the map chosen in the menu: a way to the map, so
// the map is shown, in that view.
func (a *app) menuView(mode int) {
	a.toMap()
	a.showView(mode)
}

// menuNewSheet adds a canvas to the open map, which is shown for it: what
// is made is seen being made.
func (a *app) menuNewSheet() {
	if a.file == nil || a.doc == nil {
		return
	}
	a.toMap()
	a.mode = viewMap
	a.addSheet()
}

// focusBar shows where the branch focused on is in the map, each step a
// link back to it, over the canvas.
func (a *app) focusBar(c *ui.Context) {
	f := a.focusNode()
	if f == nil {
		return
	}
	t := c.Theme()
	path := a.doc.Path(f.ID)
	ui.Row(c.Key("focus-bar")).Absolute().Top(54).Left(12).MaxWidth(a.vp.W-24).
		Padding(4, 6, 4, 12).Gap(4).AlignItems(ui.Center).Radius(8).Background(t.Surface).Border(1, t.Border).
		Children(func() {
			ui.Text(c, "聚焦").FontSize(12).Bold().TextColor(t.Accent)
			for i, n := range path {
				title := n.Title
				if title == "" {
					title = "（无标题）"
				}
				if i == len(path)-1 {
					ui.Text(c, title).FontSize(12).Bold().NoWrap().MaxWidth(200).Ellipsis("…")
					break
				}
				id := n.ID
				crumb := ui.Box(c.Key("crumb:" + id)).Cursor(ui.CursorPointer).Label(title)
				crumb.Children(func() {
					ui.Text(c, title).FontSize(12).TextColor(t.Accent).NoWrap().MaxWidth(140).Ellipsis("…")
				})
				if crumb.Clicked() {
					a.later(func() { a.focusOn(id) })
				}
				ui.Text(c, "›").FontSize(12).TextColor(t.TextMuted)
			}
			if button(c, "退出").Tooltip("显示整张导图 Esc").FontSize(12).Clicked() {
				a.later(func() { a.focusOn("") })
			}
		})
}

// selection is the nodes selected, in the canvas's order: the one
// selected and those added with Shift or Cmd.
func (a *app) selection() []*model.Node {
	if a.doc == nil {
		return nil
	}
	var out []*model.Node
	a.doc.Walk(func(n, _ *model.Node, _ int) bool {
		if n.ID == a.selected || a.multi[n.ID] {
			out = append(out, n)
		}
		return true
	})
	return out
}

// isSelected reports whether the node is one of those selected.
func (a *app) isSelected(id string) bool { return id == a.selected || a.multi[id] }

// toggleSelect adds a node to the selection, or takes it out, as a click
// with Shift or Cmd does.
func (a *app) toggleSelect(id string) {
	if a.multi == nil {
		a.multi = map[string]bool{}
	}
	switch {
	case id == a.selected:
		for other := range a.multi { // another becomes the one selected
			delete(a.multi, other)
			a.selected = other
			return
		}
	case a.multi[id]:
		delete(a.multi, id)
	default:
		if a.selected == "" {
			a.selected = id
		} else {
			a.multi[id] = true
		}
	}
}

// topSelection is the selection without the nodes under others of it,
// and without the root: what moves, copies or goes as one.
func (a *app) topSelection() []*model.Node {
	sel := a.selection()
	var out []*model.Node
	for _, n := range sel {
		if n == a.doc.Root {
			continue
		}
		inside := false
		for _, m := range sel {
			// The root holds every topic and is not of what moves or
			// goes: chosen with others, it is not what they are inside.
			if m != n && m != a.doc.Root && m.Contains(n.ID) {
				inside = true
				break
			}
		}
		if !inside {
			out = append(out, n)
		}
	}
	return out
}

// deleteSelection deletes the nodes selected.
func (a *app) deleteSelection() {
	nodes := a.topSelection()
	// One topic chosen: as ever. Several of which one holds the others
	// are that one too, and not the one chosen last, a child of it.
	if len(nodes) == 0 || len(nodes) == 1 && nodes[0].ID == a.selected {
		a.deleteSelected()
		return
	}
	a.mutate("", func() {
		next := ""
		for _, n := range nodes {
			if id, err := a.doc.Delete(n.ID); err == nil {
				next = id
			}
		}
		if n, _ := a.doc.Find(next); n != nil {
			a.selected = next
		} else {
			a.selected = a.shownRoot().ID
		}
	})
	a.multi = nil
}

// setStatusAll gives the nodes selected a status, making them tasks.
func (a *app) setStatusAll(s model.Status) {
	a.mutate("", func() {
		for _, n := range a.selection() {
			n.SetStatus(s, a.now())
		}
	})
}

// toggleTagAll puts a tag on all the nodes selected, or, when all have it,
// takes it off them.
func (a *app) toggleTagAll(tag string) {
	sel := a.selection()
	all := true
	for _, n := range sel {
		all = all && n.HasTag(tag)
	}
	a.mutate("", func() {
		for _, n := range sel {
			i := slices.Index(n.Tags, tag)
			switch {
			case all && i >= 0:
				n.Tags = slices.Delete(slices.Clone(n.Tags), i, i+1)
			case !all && i < 0:
				n.Tags = append(slices.Clone(n.Tags), tag)
			}
			if len(n.Tags) == 0 {
				n.Tags = nil
			}
		}
	})
	a.addTag(tag)
}

// multiPanel is the panel while several nodes are selected: what applies
// to them all.
func (a *app) multiPanel(c *ui.Context) {
	t := c.Theme()
	sel := a.selection()
	ui.Column(c).Padding(12, 16).Gap(14).Children(func() {
		ui.Textf(c, "已选 %d 个节点", len(sel)).FontSize(15).Bold()
		ui.Text(c, "按住 ⇧ 或 ⌘ 点击节点加入或移出选择；Esc 取消多选。").FontSize(12).TextColor(t.TextMuted).Wrap()
		ui.Field(c, "任务状态", func() {
			ui.Row(c).Wrap().Gap(6).Children(func() {
				for _, s := range model.Statuses {
					if button(c.Key("status:"+string(s)), s.Label()).Clicked() {
						a.setStatusAll(s)
					}
				}
			})
		})
		ui.Field(c, "标签（全部有则取消）", func() {
			ui.Row(c).Wrap().Gap(6).Children(func() {
				for _, tag := range a.allTags() {
					all := true
					for _, n := range sel {
						all = all && n.HasTag(tag)
					}
					if tagChip(c.Key("tag:"+tag), tag, tag, all, 12).Label("标签 " + tag).Clicked() {
						a.toggleTagAll(tag)
					}
				}
			})
		})
		ui.Row(c).Gap(8).Wrap().Children(func() {
			if button(c, "复制 ⌘C").Clicked() {
				a.copyNodes(false)
			}
			if button(c, "剪切 ⌘X").Clicked() {
				a.copyNodes(true)
			}
			if button(c, "删除").TextColor(t.Danger).Clicked() {
				a.later(a.deleteSelection)
			}
		})
	})
}

// copyNodes puts the nodes selected on the clipboard: this app's, to
// paste as nodes here or in another map, and the system's, as a Markdown
// outline for other apps. cut deletes them then.
func (a *app) copyNodes(cut bool) {
	nodes := a.topSelection()
	if len(nodes) == 0 {
		if n, _ := a.selectedNode(); n != nil {
			nodes = []*model.Node{n} // the root copies; it does not cut
			cut = false
		} else {
			return
		}
	}
	a.clip, a.clipCut = nil, nil
	for _, n := range nodes {
		c := n.Copy()
		a.clip = append(a.clip, c)
		if cut {
			if a.clipCut == nil {
				a.clipCut = map[string]string{}
			}
			pairIDs(a.clipCut, c, n) // the clipboard's ID to the one it had
		}
	}
	a.clipAssets = model.AssetsDir(a.path)
	a.clipText = model.MarkdownNodes(nodes)
	a.plat.CopyText(a.clipText)
	if cut {
		a.deleteSelection()
		a.tell("已剪切 " + itoa(len(nodes)) + " 个节点")
	} else {
		a.tell("已复制 " + itoa(len(nodes)) + " 个节点")
	}
}

// paste puts what is on the clipboard under the node selected: nodes
// copied in this app, a picture, or text, a node a line, nested by its
// indentation as an outline is.
func (a *app) paste() bool {
	parent, _ := a.selectedNode()
	if parent == nil {
		return false
	}
	text := a.plat.ClipboardText()
	var nodes []*model.Node
	switch {
	case a.clip != nil && text == a.clipText:
		var carry func()
		nodes, carry = a.clipNodes()
		defer carry()
	case a.pasteImage():
		return true
	case strings.TrimSpace(text) != "":
		nodes = model.ParseOutline(text)
	}
	if len(nodes) == 0 {
		return false
	}
	a.mutate("", func() {
		parent.Children = append(parent.Children, nodes...)
		parent.Collapsed = false
	})
	a.multi = nil
	a.selected = nodes[0].ID
	for _, n := range nodes[1:] {
		a.toggleSelect(n.ID)
	}
	a.reveal = true
	return true
}

// clipNodes are the clipboard's topics, to put into the open map: copies,
// with IDs of their own. Topics cut come back the first time as
// themselves, by the IDs they had, as a branch moved to another map does:
// what links to them (mindmap:…#id on a topic, <mindmap:…> in a thought)
// still finds them, where a cut and a paste, which is how a branch is
// moved by the keys, left every such link pointing at nothing. Pasted
// again they are copies; and copies the first time too when one of those
// IDs is in the map, the cut having been undone. carry, run once they are
// in the map, has what was drawn of them follow them (carryDias).
func (a *app) clipNodes() (nodes []*model.Node, carry func()) {
	cut := a.clipCut
	a.clipCut = nil // moved once
	same := cut != nil
	for _, had := range cut {
		if a.file.Node(had) != nil {
			same = false
			break
		}
	}
	ids := map[string]string{} // of topics cut: the ID each had to the one it has now
	for _, n := range a.clip {
		c := n.Copy()
		if same {
			c = n.Clone()
			var back func(m *model.Node)
			back = func(m *model.Node) {
				if had, ok := cut[m.ID]; ok {
					m.ID = had
					ids[had] = had
				}
				for _, ch := range m.Children {
					back(ch)
				}
				if t := m.SummaryTopic(); t != nil {
					back(t)
				}
			}
			back(c)
		} else if cut != nil {
			now := map[string]string{}
			pairIDs(now, n, c)
			for clip, to := range now {
				if had, ok := cut[clip]; ok {
					ids[had] = to
				}
			}
		}
		copyAssets(c, a.clipAssets, model.AssetsDir(a.path))
		nodes = append(nodes, c)
	}
	return nodes, func() {
		if len(ids) > 0 {
			a.carryDias(ids)
		}
	}
}

// copyAssets copies the pictures of nodes from one map's assets folder to
// another's, as they are pasted or moved there.
func copyAssets(n *model.Node, from, to string) {
	var walk func(n *model.Node)
	walk = func(n *model.Node) {
		if from != to {
			for _, f := range n.Files {
				copyAsset(from, to, f.File)
			}
		}
		if n.Image != nil && from != to {
			src := filepath.Join(from, n.Image.File)
			dst := filepath.Join(to, n.Image.File)
			if data, err := os.ReadFile(src); err == nil {
				if _, err := os.Stat(dst); err != nil {
					_ = os.MkdirAll(to, 0o755)
					_ = os.WriteFile(dst, data, 0o644)
				}
			}
		}
		// The pictures in its note too: left behind, the note of a branch
		// moved to another map showed a picture missing.
		if from != to {
			for _, m := range notePictureFile.FindAllStringSubmatch(n.Note, -1) {
				copyAsset(from, to, m[1])
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
		if t := n.SummaryTopic(); t != nil {
			walk(t)
		}
	}
	walk(n)
}

// notePictureFile finds the file of each picture in a note's Markdown.
var notePictureFile = regexp.MustCompile(`!\[[^\]]*\]\(` + assetScheme + `([^)\s]+)[^)]*\)`)

// expandTo opens the canvas down to the depth (0: all) below the middle.
func (a *app) expandTo(depth int) {
	if a.doc == nil {
		return
	}
	d := a.shownDoc()
	a.mutate("", func() { d.ExpandTo(depth) })
	a.cam().placed = false
	a.multi = nil
	// The node selected may now be folded away: the nearest node shown
	// above it is selected instead.
	path := a.doc.Path(a.selected)
	for i, n := range path {
		if n.Collapsed && i < len(path)-1 && d.Root.Contains(n.ID) {
			a.selected = n.ID
			break
		}
	}
}

// levelMenu folds the map to a level, or opens it all (⌥1–9, ⌥0).
func (a *app) levelMenu(m *ui.Menu) {
	if m.Item("全部折叠（只看中心主题）").Chosen() {
		a.later(a.foldAll)
	}
	for i := 1; i <= 4; i++ {
		if m.Item(fmt.Sprintf("展开到第 %d 层", i)).Shortcut(ui.Alt, ui.Key0+ui.Key(i)).Chosen() {
			a.later(func() { a.expandTo(i) })
		}
	}
	if m.Item("全部展开").Shortcut(ui.Alt, ui.Key0).Chosen() {
		a.later(func() { a.expandTo(0) })
	}
}

// foldAll folds the map to its center topic.
func (a *app) foldAll() {
	if a.doc == nil {
		return
	}
	d := a.shownDoc()
	a.mutate("", func() {
		d.ExpandTo(1)
		d.Root.Collapsed = len(d.Root.Children) > 0
	})
	a.cam().placed = false
	a.multi = nil
	a.selected = d.Root.ID
}

// setCollapsedBelow opens or folds every node under one.
func (a *app) setCollapsedBelow(n *model.Node, collapsed bool) {
	a.mutate("", func() {
		var walk func(m *model.Node)
		walk = func(m *model.Node) {
			for _, c := range m.Children {
				c.Collapsed = collapsed && len(c.Children) > 0
				walk(c)
			}
		}
		walk(n)
		n.Collapsed = false
	})
}

func itoa(n int) string { return strconv.Itoa(n) }

// copyAsset copies a file of one assets folder to another, unless there.
func copyAsset(from, to, name string) {
	dst := filepath.Join(to, name)
	if _, err := os.Stat(dst); err == nil {
		return
	}
	if data, err := os.ReadFile(filepath.Join(from, name)); err == nil {
		_ = os.MkdirAll(to, 0o755)
		_ = os.WriteFile(dst, data, 0o644)
	}
}
