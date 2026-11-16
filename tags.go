package main

import (
	"regexp"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// The tags are global: a library of every tag, with its color, shared by
// all maps. Tags put on nodes join it; the sidebar creates, renames,
// recolors and deletes them, in every map of the folder.

// tagDef is a tag of the library.
type tagDef struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty"` // #rrggbb; "" picks one from the name
}

// tagColors are the colors chosen for tags, which tagColor reads.
var tagColors = map[string]string{}

// tagColorChoices are the colors offered for a tag.
var tagColorChoices = []struct{ hex, name string }{
	{"#4F74B8", "蓝"}, {"#4F9668", "绿"}, {"#B7843F", "橙"}, {"#B3618A", "粉"},
	{"#7A68BD", "紫"}, {"#3F8C9C", "青"}, {"#B95A54", "红"}, {"#6E8A45", "橄榄"}, {"#6b7280", "灰"},
}

// syncTagColors makes tagColor follow the library.
func (a *app) syncTagColors() {
	tagColors = map[string]string{}
	for _, t := range a.cfg.Tags {
		if t.Color != "" {
			tagColors[t.Name] = t.Color
		}
	}
}

// libraryIndex is the index of the tag in the library, or -1.
func (a *app) libraryIndex(name string) int {
	return slices.IndexFunc(a.cfg.Tags, func(t tagDef) bool { return t.Name == name })
}

// learnTags adds the tags of a map to the library.
func (a *app) learnTags(f *model.File) {
	changed := false
	for _, t := range f.AllTags() {
		if a.libraryIndex(t) < 0 {
			a.cfg.Tags = append(a.cfg.Tags, tagDef{Name: t})
			changed = true
		}
	}
	if changed {
		a.cfg.save()
	}
}

// allTags is every tag: the library's, in its order, then any of the open
// map not in it.
func (a *app) allTags() []string {
	var tags []string
	for _, t := range a.cfg.Tags {
		tags = append(tags, t.Name)
	}
	if a.file != nil {
		for _, t := range a.file.AllTags() {
			if !slices.Contains(tags, t) {
				tags = append(tags, t)
			}
		}
	}
	return tags
}

// addTag adds a tag to the library, and returns its cleaned name.
func (a *app) addTag(name string) string {
	name = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(name), "#"))
	if name == "" {
		return ""
	}
	if a.libraryIndex(name) < 0 {
		a.cfg.Tags = append(a.cfg.Tags, tagDef{Name: name})
		a.cfg.save()
	}
	return name
}

// toggleTag puts the tag on the node, or takes it off.
func (a *app) toggleTag(n *model.Node, tag string) {
	a.mutate("", func() {
		if i := slices.Index(n.Tags, tag); i >= 0 {
			n.Tags = slices.Delete(slices.Clone(n.Tags), i, i+1)
		} else {
			n.Tags = append(slices.Clone(n.Tags), tag)
		}
		if len(n.Tags) == 0 {
			n.Tags = nil
		}
	})
	a.addTag(tag)
}

// editTagEverywhere changes a tag on the nodes of every map of the folder:
// to another name, or, for "", off. The open map's change is a step of its
// history; the others are saved at once.
func (a *app) editTagEverywhere(tag, to string) {
	edit := func(f *model.File) bool {
		changed := false
		f.Walk(func(n *model.Node) {
			i := slices.Index(n.Tags, tag)
			if i < 0 {
				return
			}
			changed = true
			tags := slices.Delete(slices.Clone(n.Tags), i, i+1)
			if to != "" && !slices.Contains(tags, to) {
				tags = slices.Insert(tags, i, to)
			}
			n.Tags = tags
			if len(n.Tags) == 0 {
				n.Tags = nil
			}
		})
		return changed
	}
	if a.file != nil {
		probe := a.file.Clone()
		if edit(probe) {
			a.mutate("", func() { edit(a.file) })
		}
	}
	for _, info := range a.files {
		if info.Path == a.path {
			continue
		}
		f, err := model.Load(info.Path)
		if err != nil || !edit(f) {
			continue
		}
		if err := f.Save(info.Path); err != nil {
			a.saveErr = err.Error()
		}
		delete(a.hists, info.Path) // its undo steps hold the tag as it was

	}
	a.refreshFiles()
	if a.tagFilter == tag {
		a.tagFilter = to
	}
}

// renameTag renames a tag in the library and in every map.
func (a *app) renameTag(tag, to string) {
	to = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(to), "#"))
	if to == "" || to == tag {
		return
	}
	i := a.libraryIndex(tag)
	if j := a.libraryIndex(to); j >= 0 { // merged into a tag that is there
		if i >= 0 {
			a.cfg.Tags = slices.Delete(a.cfg.Tags, i, i+1)
		}
	} else if i >= 0 {
		a.cfg.Tags[i].Name = to
	} else {
		a.cfg.Tags = append(a.cfg.Tags, tagDef{Name: to})
	}
	a.cfg.save()
	a.syncTagColors()
	a.editTagEverywhere(tag, to)
}

// deleteTag takes a tag out of the library and off every node.
func (a *app) deleteTag(tag string) {
	if i := a.libraryIndex(tag); i >= 0 {
		a.cfg.Tags = slices.Delete(a.cfg.Tags, i, i+1)
		a.cfg.save()
		a.syncTagColors()
	}
	a.editTagEverywhere(tag, "")
}

func (a *app) setTagColor(tag, hex string) {
	i := a.libraryIndex(tag)
	if i < 0 {
		a.cfg.Tags = append(a.cfg.Tags, tagDef{Name: tag})
		i = len(a.cfg.Tags) - 1
	}
	a.cfg.Tags[i].Color = hex
	a.cfg.save()
	a.syncTagColors()
}

// tagPicker shows every tag as a chip, those on the node filled: a click
// puts one on or takes it off. A field below makes a new one.
func (a *app) tagPicker(c *ui.Context, n *model.Node) {
	t := c.Theme()
	ui.Column(c).Gap(8).Children(func() {
		ui.Row(c).Wrap().Gap(6).Children(func() {
			for _, tag := range a.allTags() {
				on := slices.Contains(n.Tags, tag)
				chip := tagChip(c.Key("pick:"+tag), tag, tag, on, 12).Label("标签 " + tag)
				if chip.Clicked() {
					a.toggleTag(n, tag)
				}
			}
		})
		// A pale block, as the other fields: no darker fill, no line of its own.
		in := ui.TextInputBase(c.Key("new-tag"), &a.tagDraft).Placeholder("新标签").Label("新标签").FontSize(textSmall).
			Height(30).Padding(0, 10).Radius(radiusPill).FillWidth().Background(blockFill(t))
		if in.Focused() {
			in.Background(t.Background).InsetShadow(0, 0, 0, 1.5, t.Accent)
		}
		if a.tagFocus { // asked for by the 标签 menu's 新建标签…
			a.tagFocus = false
			in.Focus()
		}
		if in.Submitted() {
			if tag := a.addTag(a.tagDraft); tag != "" && !slices.Contains(n.Tags, tag) {
				a.toggleTag(n, tag)
			}
			a.tagDraft = ""
		}
		if len(a.allTags()) == 0 {
			ui.Text(c, "还没有标签，输入名称来创建。").FontSize(12).TextColor(t.TextMuted)
		}
	})
}

// tagMenu is the 标签 submenu of a node's context menu.
func (a *app) tagMenu(m *ui.Menu, n *model.Node) {
	m.Submenu("标签", func(m *ui.Menu) {
		tags := a.allTags()
		for _, tag := range tags {
			if m.Item(tag).Checked(slices.Contains(n.Tags, tag)).Chosen() {
				t := tag
				a.later(func() { a.toggleTag(n, t) })
			}
		}
		if len(tags) == 0 {
			m.Item("还没有标签").Disabled(true)
		}
		m.Separator()
		// A new tag is written in the right column's field, which shows
		// though it was put away: before, 管理标签… only chose that column's
		// page, and with the column put away nothing came of it.
		if m.Item("新建标签…").Chosen() {
			a.later(func() {
				a.selected, a.multi, a.panelTab, a.aiOpen = n.ID, nil, tabNode, false
				if a.panelOff {
					a.togglePanel()
				}
				if a.panelShow == nil {
					a.panelShow = map[string]bool{}
				}
				a.panelShow[n.ID+":tags"] = true // the picker, with its field
				a.tagFocus = true
			})
		}
		m.Item("在标题里写 #标签 也能加").Disabled(true)
	})
}

// titleTag is a #tag in a topic's title: # at its start or after a space,
// then what is up to a space or a mark. A # in a word (C#, a link's
// #part) is no tag, nor # and a number alone (#12, an issue's).
var titleTag = regexp.MustCompile(`(^|\s)#([^\s#，。,.!！?？;；:：、（）()\[\]{}"'“”]+)`)

// titleTags takes the #tags out of a title: what is left of it, its words
// closed up, and the tags.
func titleTags(title string) (string, []string) {
	var tags []string
	out := titleTag.ReplaceAllStringFunc(title, func(m string) string {
		sub := titleTag.FindStringSubmatch(m)
		tag := sub[2]
		if strings.Trim(tag, "0123456789") == "" {
			return m
		}
		if !slices.Contains(tags, tag) {
			tags = append(tags, tag)
		}
		return sub[1]
	})
	if len(tags) == 0 {
		return title, nil
	}
	return strings.Join(strings.Fields(out), " "), tags
}

// titleTagBegun is the tag being written at a title's end: after a # at
// its start or after a space, with no space since.
func titleTagBegun(title string) (string, bool) {
	i := strings.LastIndex(title, "#")
	if i < 0 || i > 0 && !strings.ContainsAny(title[i-1:i], " \t") || strings.ContainsAny(title[i:], " \t\n　") {
		return "", false
	}
	return title[i+1:], true
}

// tagOffers are the tags, of the maps and of 收集, that begin with part
// and are not it: n at most, those of the map first.
func (a *app) tagOffers(part string, n int) []string {
	var out []string
	for _, tag := range append(a.allTags(), a.recentTags(part, n)...) {
		if len(out) < n && strings.HasPrefix(tag, part) && tag != part && !slices.Contains(out, tag) {
			out = append(out, tag)
		}
	}
	return out
}
