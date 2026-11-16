package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/skills"
)

// A skill of the AI agents is linked to as a clip is: a topic's link
// "skill:name", made by 链接到技能… of the topic's links, by [[ in its
// note and in a thought (<skill:name> there), and from the skill itself
// (引用到导图…); the skill says in turn which topics and thoughts name it
// (skillUses). The user asked for it (0.100.8): a task says so which
// skill it is to be done with, to the user and to an agent that reads the
// map, which is told what the link means (internal/mcp's instructions).
// A skill is named by its folder's name, which the agents call it by and
// which is the same on the user's other computers.

// skillScheme starts links to skills: "skill:name".
const skillScheme = "skill:"

// skillNamed is the skill of a name: every session's first, else a
// plugin's or a tool's own; nil with none, or with no scan yet.
func (a *app) skillNamed(name string) *skills.Entry {
	r := a.skills.report
	if r == nil {
		return nil
	}
	if e := r.Entry("g/" + name); e != nil {
		return e
	}
	for _, e := range r.Entries {
		if e.Name == name {
			return e
		}
	}
	return nil
}

// skillsNamed are the skills whose name or description has part (in
// lower case), those named so first, each name once, n at most.
func (a *app) skillsNamed(part string, n int) []*skills.Entry {
	r := a.skills.report
	if r == nil {
		return nil
	}
	var named, said []*skills.Entry
	seen := map[string]bool{}
	for _, e := range r.Entries {
		if seen[e.Name] {
			continue
		}
		switch {
		case strings.Contains(strings.ToLower(e.Name), part):
			named = append(named, e)
		case strings.Contains(strings.ToLower(e.Title+" "+e.Description), part):
			said = append(said, e)
		default:
			continue
		}
		seen[e.Name] = true
	}
	out := append(named, said...)
	return out[:min(n, len(out))]
}

// skillsToLink tells whether there is a skill to link to.
func (a *app) skillsToLink() bool {
	return a.skills.report != nil && len(a.skills.report.Entries) > 0
}

// showSkill shows a skill in 技能, its details open: where a link to it
// goes. Before the first scan is in, it is chosen when the scan comes.
func (a *app) showSkill(name string) {
	if a.skills.report != nil && a.skillNamed(name) == nil {
		a.saveErr = "链接的技能已不存在：" + name
		return
	}
	a.showSkills()
	a.skills.side, a.skills.tool, a.skills.find = "", "", ""
	if e := a.skillNamed(name); e != nil {
		a.skills.sel, a.skills.want = e.Key, ""
	} else {
		a.skills.want = name
	}
}

// skillBacklinks are the topics of every map, and the thoughts, that
// link to a skill.
func (a *app) skillBacklinks(name string) []backlink {
	return a.refIndex().to[skillScheme+name]
}

// skillPlace asks which topic is to link to a skill.
func (a *app) skillPlace(e *skills.Entry) {
	a.attachRef = skillScheme + e.Name
	a.openPalette(paletteAttach)
}

// skillUses is the part of a skill's details that says which topics and
// thoughts name it, each going there, and the way to name it in one more.
func (a *app) skillUses(c *ui.Context, e *skills.Entry) {
	t := c.Theme()
	back := a.skillBacklinks(e.Name)
	ui.Column(c.Key("uses")).Gap(8).FillWidth().Margin(14, 0, 0).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).FillWidth().Children(func() {
			head := "导图里的引用"
			if len(back) > 0 {
				head = fmt.Sprintf("被 %d 处引用", len(back))
			}
			ui.Text(c, head).FontSize(textSmall).FontWeight(700).Grow(1)
			if textLink(c, "引用到导图…", "引用到导图…").Tooltip("让某张导图的某个主题引用这个技能").Clicked() {
				a.later(func() { a.skillPlace(e) })
			}
		})
		if len(back) == 0 {
			ui.Text(c, "还没有主题引用它。任务引用了技能，你和读导图的 AI 工具都知道这件事该用它来做。").
				FontSize(textCaption).TextColor(t.TextMuted).LineHeight(1.5).Wrap()
			return
		}
		raise(ui.Column(c.Key("rows")).Padding(4, 12).Radius(radiusCard).FillWidth(), t).Children(func() {
			for i, b := range back {
				if i > 0 {
					setLine(c)
				}
				where := "收集"
				if b.clip == "" {
					where = mapName(b.path)
				}
				title := strings.TrimSpace(refToken.ReplaceAllString(b.title, "")) // a thought's words, without its links
				row := ui.Row(c.Key(fmt.Sprint("use", i))).MinHeight(36).Padding(6, 0).Gap(8).AlignItems(ui.Center).FillWidth().
					Cursor(ui.CursorPointer).Label("前往 " + title).Tooltip("去引用它的这一处")
				if row.Clicked() {
					a.later(func() { a.goToBacklink(b) })
				}
				row.Children(func() {
					ink := t.Text
					if row.Hovered() {
						ink = t.Accent
					}
					ui.Text(c, title).FontSize(textSmall).TextColor(ink).SingleLine().Grow(1).Shrink(1).Basis(0).MinWidth(0).PassThrough()
					ui.Text(c, where).FontSize(textCaption).TextColor(t.TextMuted).SingleLine().MaxWidth(130).Shrink(0).PassThrough()
				})
			}
		})
	})
}
