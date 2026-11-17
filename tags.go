package main

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

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
		if err != nil {
			continue
		}
		was := model.PrintsOf(f)
		if !edit(f) {
			continue
		}
		was.Stamp(f, a.now()) // when each topic it changed did
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
		if a.tagFocus > 0 { // asked for by the 标签 menu's 新建标签…
			a.tagFocus = 0
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
				a.tagFocus = 3                   // a few frames: the field not drawn by then, it is not asked for later
			})
		}
		m.Item("在标题里写 #标签 也能加").Disabled(true)
	})
}

// titleTag is a #tag in a topic's title: # at its start or after a space
// (a full-width one too), then what is up to a space or a mark.
var titleTag = regexp.MustCompile(`(^|[\s　])#([^\s　#，。,.!！?？;；:：、（）()\[\]{}"'“”<>]+)`)

// hexColor is a color written as #ff6600: no tag, though letters lead it.
var hexColor = regexp.MustCompile(`^(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)

// tagWord tells whether what follows a # is a tag: it begins with a
// letter, of any script, and is no color. #12, #2024Q3, #1号方案 and
// #ff6600 are words of the title.
func tagWord(w string) bool {
	r, _ := utf8.DecodeRuneInString(w)
	return w != "" && unicode.IsLetter(r) && !(hexColor.MatchString(w) && strings.ContainsAny(w, "0123456789"))
}

// titleTagBegun is the #word being written at a title's end, which may be
// made a tag: after a # at its start or after a space, with no space nor
// mark since, and a letter first (no #12, no #ff6600).
func titleTagBegun(title string) (string, bool) {
	i := strings.LastIndex(title, "#")
	if i < 0 {
		return "", false
	}
	if i > 0 {
		r, _ := utf8.DecodeLastRuneInString(title[:i])
		if !unicode.IsSpace(r) {
			return "", false
		}
	}
	part := title[i+1:]
	if part != "" && (!tagWord(part) || titleTag.FindString("#"+part) != "#"+part) {
		return "", false
	}
	return part, true
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

// dropUnusedTags takes tags out of the library that no topic of the open
// map carries: those a title made, taken back.
func (a *app) dropUnusedTags(tags []string) {
	if len(tags) == 0 || a.file == nil {
		return
	}
	used := a.file.AllTags()
	n := len(a.cfg.Tags)
	a.cfg.Tags = slices.DeleteFunc(a.cfg.Tags, func(t tagDef) bool {
		return slices.Contains(tags, t.Name) && !slices.Contains(used, t.Name)
	})
	if len(a.cfg.Tags) != n {
		a.cfg.save()
	}
}

// titleTagOffers are the tags offered for a #word being written: the tag
// of that name if there is one, those that begin so, and the word as a
// new tag.
func (a *app) titleTagOffers(part string) []string {
	var out []string
	if part != "" && slices.Contains(a.allTags(), part) {
		out = append(out, part)
	}
	out = append(out, a.tagOffers(part, 6)...)
	if part != "" && !slices.Contains(out, part) {
		out = append(out, part) // a new one, last
	}
	return out
}

// titleTagOffer offers the #word being written at the end of a title
// field as a tag, under the field: the tags of titleTagOffers, a click
// taking one (take). The canvas's title, the right column's and the
// outline's lines have it alike; the user asked for the two after the
// first. Tab takes the first offered: titleTagTab, in the field's keys.
func (a *app) titleTagOffer(c *ui.Context, in ui.Element, key, text string, take func(part, tag string)) {
	part, ok := titleTagBegun(text)
	if !ok {
		if strings.HasPrefix(a.tagDismissed, key+"|") {
			a.tagDismissed = "" // the word ended: the next one is offered
		}
		return
	}
	// Put away (Escape, a click beside it), it stays away for this word:
	// "not chosen" is a choice, and it came back a frame later before.
	if a.tagDismissed == key+"|"+part {
		return
	}
	t := c.Theme()
	offers := a.titleTagOffers(part)
	a.tagOffered = true
	ui.Popover(c, in, &a.tagOffered, func() {
		ui.Column(c.Key("tag-offers")).Gap(6).Padding(8, 10).MaxWidth(420).KeepFocus().Children(func() {
			ui.Row(c).Gap(6).Wrap().GapY(6).AlignItems(ui.Center).Children(func() {
				for i, tag := range offers {
					label, tip := "#"+tag, "设为标签"
					if !slices.Contains(a.allTags(), tag) {
						label = "设为新标签 #" + tag
					}
					if i == 0 {
						tip += "（Tab）"
					}
					if softTag(c, label).Cursor(ui.CursorPointer).KeepFocus().Label("设为标签 " + tag).Tooltip(tip).Clicked() {
						take(part, tag)
					}
				}
			})
			say := "写标签名，选一个设为标签；不选就当文字（Esc 收起）"
			if len(offers) > 0 {
				say = "点一下或按 Tab 设为标签；不选就当文字（Esc 收起）"
			}
			ui.Text(c, say).FontSize(textCaption).TextColor(t.TextMuted).NoWrap()
		})
	})
	if !a.tagOffered {
		a.tagDismissed = key + "|" + part
	}
}

// tagOfferShown tells whether the field key shows an offer for the #word
// at the end of text: one is being written, and was not put away.
func (a *app) tagOfferShown(key, text string) bool {
	part, ok := titleTagBegun(text)
	return ok && a.tagDismissed != key+"|"+part
}

// dismissTagOffer puts away the field key's offer for the #word at the end
// of text: Escape on the canvas, which would else end the edit.
func (a *app) dismissTagOffer(key, text string) {
	if part, ok := titleTagBegun(text); ok {
		a.tagDismissed, a.tagOffered = key+"|"+part, false
	}
}

// titleTagTab is what Tab takes in the title field key: the #word being
// written and the first tag offered for it, while the offer shows. Put
// away, Tab is the field's own again: the outline's indents (the review
// of 0.99.1).
func (a *app) titleTagTab(key, text string) (part, tag string, ok bool) {
	if !a.tagOfferShown(key, text) {
		return "", "", false
	}
	part, _ = titleTagBegun(text)
	offers := a.titleTagOffers(part)
	if len(offers) == 0 {
		return "", "", false
	}
	return part, offers[0], true
}

// takeFieldTag makes the #word written at the end of n's title, in a field
// that writes the title as it is typed (the right column's, the outline's),
// the tag tag: the word out of the title and the tag on, one step. The
// caret goes back to the field's end (tagCaret).
func (a *app) takeFieldTag(n *model.Node, part, tag string) {
	word := "#" + part
	if !strings.HasSuffix(n.Title, word) {
		return // the field and the topic do not say the same: nothing cut
	}
	before := n.Title
	title := strings.TrimRight(strings.TrimSuffix(before, word), " 　")
	a.tagCaret, a.tagCaretTTL = n.ID, 3
	a.later(func() { a.putTitleTag(n, tag, before, &title) })
}

// putTitleTag puts tag on n, and title as its title when there is one, in
// one step: 撤销 on its toast takes it back, and a tag it made out of the
// library too.
func (a *app) putTitleTag(n *model.Node, tag, before string, title *string) {
	if n.HasTag(tag) && title == nil {
		return
	}
	fresh := !slices.Contains(a.allTags(), tag)
	had := n.HasTag(tag)
	a.mutate("", func() {
		if title != nil {
			n.Title = *title
		}
		if !had {
			n.Tags = append(slices.Clone(n.Tags), tag)
		}
	})
	a.addTag(tag)
	id := n.ID
	// 撤销 takes back this and nothing else: the tag off, and the title as
	// it was if it is as this left it. An undo of the history took back what
	// was written after it, and, on the canvas, the edit itself (the review
	// of 0.98.3).
	a.tellUndo("已加标签 #"+tag, func() {
		// On whichever canvas of the map it is now; nothing to do, no step.
		if a.file == nil {
			return
		}
		n := a.file.Node(id)
		if n == nil {
			return
		}
		untag := !had && n.HasTag(tag)
		retitle := title != nil && n.Title == *title
		if !untag && !retitle {
			return
		}
		a.mutate("", func() {
			if untag {
				n.Tags = slices.DeleteFunc(slices.Clone(n.Tags), func(s string) bool { return s == tag })
				if len(n.Tags) == 0 {
					n.Tags = nil
				}
			}
			if retitle {
				n.Title = before
			}
		})
		if fresh {
			a.dropUnusedTags([]string{tag})
		}
	})
}

// takeTitleTag makes the #word being written in n's title on the canvas
// the tag tag: the word out of the title being written, the tag on the
// topic, the title still being written.
func (a *app) takeTitleTag(n *model.Node, part, tag string) {
	word := "#" + part
	if !strings.HasSuffix(a.editText, word) {
		return
	}
	// The word out, the space before it kept for what is written on; the
	// title kept is trimmed (commitEdit), not left with a space at its end
	// (the review of 0.98.3).
	a.editText = strings.TrimSuffix(a.editText, word)
	a.editFocus = false // the caret to the end
	a.later(func() { a.putTitleTag(n, tag, "", nil) })
}

// fieldTitleTags is what a title field that writes as it is typed does
// for its #words: after a tag is taken the caret goes back to its end,
// and, for a field with no keys of its own (keys false: the outline's line
// has, and takes Tab there, outlineTagTab), Tab takes the tag offered and
// nothing else: a shortcut of Tab took it from the outline's indent.
func (a *app) fieldTitleTags(in ui.Element, key string, n *model.Node, text string, keys bool) {
	if a.tagCaret == n.ID && in.Focused() {
		k := utf8.RuneCountInString(n.Title)
		in.SetTextSelection(k, k)
		a.tagCaret, a.tagCaretTTL = "", 0
	}
	if !keys {
		return
	}
	in.HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind != ui.InputKeyDown || ev.Key != ui.KeyTab || ev.Mods != 0 {
			return false
		}
		part, tag, ok := a.titleTagTab(key, n.Title)
		if ok {
			a.takeFieldTag(n, part, tag)
		}
		return ok
	})
}

// outlineTagTab takes, for Tab on an outline's line, the tag offered for
// the #word being written: true when it did, the line not to be indented.
func (a *app) outlineTagTab(n *model.Node) bool {
	part, tag, ok := a.titleTagTab("outline:"+n.ID, n.Title) // the topic's, as takeFieldTag cuts it
	if ok {
		a.takeFieldTag(n, part, tag)
	}
	return ok
}
