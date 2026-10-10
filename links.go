package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// Links of nodes: to web pages, to the project's code, opened in the
// editor at their line, and to other nodes, of this map or another, which
// the node linked to lists back.

// Kinds of links.
const (
	linkWeb = iota
	linkFile
	linkNode
)

// nodeScheme starts links to nodes: "mindmap:folder/map#id".
const nodeScheme = "mindmap:"

// editors open the project's files, by the setting's value.
var editors = []struct{ id, name string }{
	{"vscode", "VS Code"}, {"cursor", "Cursor"}, {"default", "默认程序"}, {"finder", "在 Finder 中显示"},
}

// parseLink tells what a link is: for a file, its path made absolute and
// its line; for a node, its map's path and its id.
func (a *app) parseLink(link string) (kind int, target string, line int) {
	link = strings.TrimSpace(link)
	switch {
	case strings.HasPrefix(link, nodeScheme):
		rel, id, _ := strings.Cut(strings.TrimPrefix(link, nodeScheme), "#")
		path := a.path
		if rel != "" {
			path = filepath.Join(a.dir, filepath.FromSlash(rel)+model.Ext)
		}
		return linkNode, path + "#" + id, 0
	case strings.HasPrefix(link, "http://"), strings.HasPrefix(link, "https://"), strings.HasPrefix(link, "mailto:"):
		return linkWeb, link, 0
	case strings.HasPrefix(link, "file://"):
		if u, err := url.Parse(link); err == nil {
			link = u.Path
		}
	}
	// A path, perhaps with ":line" (or ":line:column") after it.
	path := link
	if i := strings.LastIndex(path, ":"); i > 0 {
		rest := path[i+1:]
		if j := strings.LastIndex(path[:i], ":"); j > 0 { // :line:column
			if n, err := strconv.Atoi(path[j+1 : i]); err == nil {
				if _, err := strconv.Atoi(rest); err == nil {
					path, line = path[:j], n
				}
			}
		}
		if line == 0 {
			if n, err := strconv.Atoi(rest); err == nil {
				path, line = path[:i], n
			}
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	if !filepath.IsAbs(path) {
		base := a.projectDir()
		path = filepath.Join(base, path)
	}
	return linkFile, path, line
}

// projectDir is the folder relative links of the open map are in: its
// project folder, or the map's own.
func (a *app) projectDir() string {
	if a.file != nil && a.file.Project != "" {
		return a.file.Project
	}
	return filepath.Dir(a.path)
}

// nodeLinkTo is the link to a node of a map.
func (a *app) nodeLinkTo(path, id string) string {
	if path == a.path {
		return nodeScheme + "#" + id
	}
	rel, err := filepath.Rel(a.dir, path)
	if err != nil {
		rel = path
	}
	return nodeScheme + filepath.ToSlash(strings.TrimSuffix(rel, model.Ext)) + "#" + id
}

// openLink opens what a link points to.
func (a *app) openLink(link string) {
	kind, target, line := a.parseLink(link)
	switch kind {
	case linkWeb:
		a.plat.OpenURL(target)
	case linkNode:
		path, id, _ := strings.Cut(target, "#")
		if path != a.path {
			if err := a.open(path); err != nil {
				return
			}
		}
		if d := a.file.SheetOf(id); d != nil {
			a.goToNode(a.path, d.ID, id)
		} else {
			a.saveErr = "链接的节点已不存在"
		}
	case linkFile:
		if _, err := os.Stat(target); err != nil {
			a.saveErr = "文件不存在：" + target
			return
		}
		abs := filepath.ToSlash(target)
		at := ""
		if line > 0 {
			at = ":" + strconv.Itoa(line)
		}
		switch a.cfg.Editor {
		case "cursor":
			a.plat.OpenURL("cursor://file" + abs + at)
		case "default":
			a.plat.OpenPath(target)
		case "finder":
			a.plat.ShowInFolder(target)
		default:
			a.plat.OpenURL("vscode://file" + abs + at)
		}
	}
}

// linkLabel is how a link shows: a page's address, a file's path from the
// project folder, a node's title.
func (a *app) linkLabel(link string) (icon, label string) {
	kind, target, line := a.parseLink(link)
	switch kind {
	case linkWeb:
		u, err := url.Parse(target)
		if err == nil && u.Host != "" {
			label = u.Host + strings.TrimSuffix(u.Path, "/")
			if strings.Contains(u.Host, "github.com") {
				return "", githubLabel(u.Path)
			}
			return "🌐", label
		}
		return "🌐", target
	case linkNode:
		path, id, _ := strings.Cut(target, "#")
		f := a.file
		if path != a.path {
			f = a.indexed(path)
		}
		if f != nil {
			if d := f.SheetOf(id); d != nil {
				n, _ := d.Find(id)
				if path != a.path {
					return "🔗", mapName(path) + " › " + n.Title
				}
				return "🔗", n.Title
			}
		}
		return "🔗", "（节点已不存在）"
	}
	rel, err := filepath.Rel(a.projectDir(), target)
	if err != nil || strings.HasPrefix(rel, "..") {
		rel = target
	}
	if line > 0 {
		rel += ":" + strconv.Itoa(line)
	}
	return "📄", rel
}

// githubLabel names a GitHub page by what it is: "PR #12", "issue #3",
// "commit 1a2b3c4", or its path.
func githubLabel(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 4 {
		repo := parts[1]
		switch parts[2] {
		case "pull":
			return repo + " PR #" + parts[3]
		case "issues":
			return repo + " issue #" + parts[3]
		case "commit":
			return repo + " commit " + parts[3][:min(7, len(parts[3]))]
		}
	}
	return "github.com" + path
}

// addLink adds a link to a node.
func (a *app) addLink(n *model.Node, link string) {
	link = strings.TrimSpace(link)
	if link == "" || slices.Contains(n.Links, link) {
		return
	}
	a.mutate("", func() { n.Links = append(slices.Clone(n.Links), link) })
}

func (a *app) removeLink(n *model.Node, link string) {
	a.mutate("", func() {
		n.Links = slices.DeleteFunc(slices.Clone(n.Links), func(l string) bool { return l == link })
		if len(n.Links) == 0 {
			n.Links = nil
		}
	})
}

// backlink is a node that links to another.
type backlink struct {
	path, sheet, id, title string
}

// backlinks are the nodes of every map linking to a node of the open map.
// They are found again when the node selected changes.
func (a *app) backlinks(id string) []backlink {
	if a.backlinkFor == id && a.backlinkMod == a.modTime {
		return a.backlinkList
	}
	var out []backlink
	for _, f := range a.files {
		file := a.file
		if f.Path != a.path {
			file = a.indexed(f.Path)
		}
		if file == nil {
			continue
		}
		mine := nodeScheme + "#" + id
		other := a.nodeLinkTo(a.path, id)
		if f.Path != a.path {
			rel, _ := filepath.Rel(a.dir, a.path)
			other = nodeScheme + filepath.ToSlash(strings.TrimSuffix(rel, model.Ext)) + "#" + id
		}
		for _, d := range file.Sheets {
			d.Walk(func(n, _ *model.Node, _ int) bool {
				for _, l := range n.Links {
					if f.Path == a.path && (l == mine || l == other) || f.Path != a.path && l == other {
						title := n.Title
						if f.Path != a.path {
							title = mapName(f.Path) + " › " + title
						}
						out = append(out, backlink{f.Path, d.ID, n.ID, title})
						break
					}
				}
				return true
			})
		}
	}
	a.backlinkFor, a.backlinkMod, a.backlinkList = id, a.modTime, out
	return out
}

// linksSection is the links part of the node panel.
func (a *app) linksSection(c *ui.Context, n *model.Node) {
	t := c.Theme()
	ui.Field(c, "链接", func() {
		ui.Column(c).Gap(6).Children(func() {
			for i, l := range n.Links {
				icon, label := a.linkLabel(l)
				row := ui.Row(c.Key(fmt.Sprint("link", i))).Gap(6).AlignItems(ui.Center).Padding(3, 6).Radius(6).
					Background(t.Surface).Border(1, t.Border)
				row.Children(func() {
					open := ui.Row(c).Gap(6).Grow(1).Basis(0).MinWidth(0).Cursor(ui.CursorPointer).Tooltip(l).Label("打开 " + label)
					if open.Clicked() {
						link := l
						a.later(func() { a.openLink(link) })
					}
					open.Children(func() {
						if icon != "" {
							ui.Text(c, icon).FontSize(12).Shrink(0)
						} else {
							ui.Text(c, "GH").FontSize(10).Bold().TextColor(t.TextMuted).Shrink(0)
						}
						ui.Text(c, label).FontSize(12).TextColor(t.Accent).MaxLines(1)
					})
					if ui.Button(c, "✕").Label("删除链接").Tooltip("删除链接").FontSize(11).Padding(0, 6).Clicked() {
						link := l
						a.later(func() { a.removeLink(n, link) })
					}
				})
			}
			in := ui.TextInput(c.Key("new-link"), &a.linkDraft).Placeholder("粘贴网址、文件路径（可带 :行号），回车添加").FontSize(12).Label("添加链接")
			if in.Submitted() && strings.TrimSpace(a.linkDraft) != "" {
				a.addLink(n, a.linkDraft)
				a.linkDraft = ""
			}
			ui.Row(c).Gap(6).Wrap().Children(func() {
				if ui.Button(c, "选择文件…").FontSize(12).Clicked() {
					id := n.ID
					a.plat.OpenFile("选择要链接的文件", nil, func(path string) {
						if node, _ := a.doc.Find(id); node != nil && path != "" {
							if rel, err := filepath.Rel(a.projectDir(), path); err == nil && !strings.HasPrefix(rel, "..") && a.file.Project != "" {
								path = filepath.ToSlash(rel)
							}
							a.addLink(node, path)
						}
					})
				}
				if ui.Button(c, "链接到节点…").FontSize(12).Clicked() {
					a.linkFrom = n.ID
					a.openPalette(paletteLink)
				}
			})
			if back := a.backlinks(n.ID); len(back) > 0 {
				ui.Text(c, fmt.Sprintf("被 %d 个节点链接", len(back))).FontSize(11).TextColor(t.TextMuted).Margin(4, 0, 0)
				for i, b := range back {
					row := ui.Row(c.Key(fmt.Sprint("back", i))).Gap(6).Cursor(ui.CursorPointer).Label("前往 " + b.title)
					if row.Clicked() {
						b := b
						a.later(func() { a.goToNode(b.path, b.sheet, b.id) })
					}
					row.Children(func() {
						ui.Text(c, "↩").FontSize(12).TextColor(t.TextMuted)
						ui.Text(c, b.title).FontSize(12).TextColor(t.Accent).MaxLines(1)
					})
				}
			}
		})
	})
}

// projectSection sets the folder of the project's code and the editor
// that opens its files.
func (a *app) projectSection(c *ui.Context) {
	t := c.Theme()
	ui.Field(c, "项目代码目录", func() {
		ui.Column(c).Gap(6).Children(func() {
			dir := a.file.Project
			if dir == "" {
				dir = "未设置（相对路径从导图所在文件夹算起）"
			}
			ui.Text(c, dir).FontSize(12).TextColor(t.TextMuted).Wrap()
			ui.Row(c).Gap(6).Children(func() {
				if ui.Button(c, "选择…").FontSize(12).Clicked() {
					a.plat.PickFolder(func(dir string) {
						if dir != "" && a.file != nil {
							a.mutate("", func() { a.file.Project = dir })
						}
					})
				}
				if a.file.Project != "" && ui.Button(c, "清除").FontSize(12).Clicked() {
					a.mutate("", func() { a.file.Project = "" })
				}
			})
		})
	})
	ui.Field(c, "代码文件用…打开", func() {
		cur := a.cfg.Editor
		if cur == "" {
			cur = "vscode"
		}
		ui.Row(c).Gap(6).Wrap().Children(func() {
			for _, e := range editors {
				b := ui.Button(c.Key("editor:"+e.id), e.name).FontSize(12)
				if e.id == cur {
					b.Background(t.Accent).TextColor(t.AccentText)
				}
				if b.Clicked() {
					a.cfg.Editor = e.id
					a.cfg.save()
				}
			}
		})
	})
}
