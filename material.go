package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// Material is what is kept, clips and maps alike, and maps string it
// together: the AI arranges a list of material into a map whose nodes link
// it, or merges new material into the open map.

// maxMaterial is the most material the AI is given at once.
const maxMaterial = 150

// materialRef is the link to a piece of material: a clip, or a whole map.
func (a *app) materialRef(c *library.Clip) string {
	if c.Kind == kindMap {
		return a.mapLink(strings.TrimPrefix(c.ID, "map:"))
	}
	return clipScheme + c.ID
}

// materialLines are the material, a line each, for the AI: its title,
// what it is, its tags, and its link at the end.
func (a *app) materialLines(clips []*library.Clip) (string, []string) {
	var b strings.Builder
	var refs []string
	for _, c := range clips {
		ref := a.materialRef(c)
		if ref == "" {
			continue
		}
		refs = append(refs, ref)
		desc := c.Desc
		if desc == "" {
			desc = c.Summary
		}
		if r := []rune(desc); len(r) > 80 {
			desc = string(r[:80]) + "…"
		}
		line := "- " + strings.ReplaceAll(c.Label(), "\n", " ")
		if c.Kind == kindMap {
			line += "（导图）"
		}
		if desc != "" {
			line += "｜" + strings.ReplaceAll(desc, "\n", " ")
		}
		for _, t := range c.Tags {
			line += " #" + t
		}
		b.WriteString(line + " <" + ref + ">\n")
	}
	return b.String(), refs
}

// listedMaterial is the material the library lists, as filtered.
func (a *app) listedMaterial() []*library.Clip {
	var clips []*library.Clip
	for _, h := range a.libHits() {
		clips = append(clips, h.Clip)
	}
	return clips
}

// aiMaterialToMap has the AI arrange material into a new map, linking it.
func (a *app) aiMaterialToMap(name string, clips []*library.Clip) {
	if len(clips) == 0 {
		a.libErr = "没有资料可以整理。"
		return
	}
	list, refs := a.materialLines(clips)
	prompt := fmt.Sprintf("把下面这 %d 条资料整理成一张结构化的导图：按主题和脉络分组（不超过 4 层），相关的放在一起，分组节点用简短的标题概括主题；每条资料作为叶子节点挂在最合适的位置，标题用资料的标题或更精炼的说法，行尾空一格原样写上它的链接（如 <clip:…>）。不要把摘要和标签抄进节点。用 propose_new_map 提出，导图名字建议用「%s」。", len(refs), name)
	a.ai.nextMaterial = refs
	a.aiAction(prompt, scopeNone, "资料：\n"+list)
}

// aiMergeMaterial has the AI put material into the open map, where each
// belongs.
func (a *app) aiMergeMaterial(clips []*library.Clip) {
	if a.doc == nil {
		a.libErr = "先打开要并入的导图。"
		return
	}
	list, refs := a.materialLines(clips)
	if len(refs) == 0 {
		a.libErr = "没有资料可以并入。"
		return
	}
	a.libOpen = false
	prompt := fmt.Sprintf("把下面这 %d 条资料并入这张导图：每条挂到最相关的已有节点上（update 的 links 加上它的链接），或者在合适的位置新增节点（add，links 写它的链接）；和导图无关的就不要硬塞，最后用一句话说明没并入的有哪些。用 propose_edits 一次提出。", len(refs))
	a.aiAction(prompt, scopeFile, "资料：\n"+list)
}

// keepMaterial checks the links of a map the AI made of material: links to
// clips that are not kept are taken off, and the material it left out goes
// under 「其他」.
func (a *app) keepMaterial(f *model.File, given []string) {
	used := map[string]bool{}
	f.Walk(func(n *model.Node) {
		var keep []string
		for _, l := range n.Links {
			if id, ok := strings.CutPrefix(l, clipScheme); ok && a.lib.Find(id) == nil {
				continue // made up
			}
			used[l] = true
			keep = append(keep, l)
		}
		n.Links = keep
	})
	var left []*model.Node
	for _, ref := range given {
		if used[ref] {
			continue
		}
		_, label := a.linkLabel(ref)
		n := model.NewNode(label)
		n.Links = []string{ref}
		left = append(left, n)
	}
	if len(left) > 0 {
		other := model.NewNode("其他")
		other.Children = left
		root := f.Sheets[0].Root
		root.Children = append(root.Children, other)
	}
}

// materialBar is the line over the library's list: how much it lists, and
// the AI's ways of stringing it into maps.
func (a *app) materialBar(c *ui.Context) {
	t := c.Theme()
	n := len(a.libHits())
	if n == 0 {
		return
	}
	noKey := providerKey(a.cfg) == ""
	ui.Row(c.Key("material-bar")).Padding(0, 12, 8).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, fmt.Sprintf("%d 条", n)).FontSize(12).TextColor(t.TextMuted).NoWrap()
		ui.Spacer(c)
		tip := "AI 读这些资料，按主题整理成一张新导图，每个节点挂着对应的资料"
		switch {
		case noKey:
			tip = "先在 AI 助手的设置里填写 API Key"
		case n > maxMaterial:
			tip = fmt.Sprintf("一次最多 %d 条：先用分类、标签或搜索筛选", maxMaterial)
		}
		if iconTextButton(c, iconSparkle, fmt.Sprintf("用 AI 整理成导图（%d 条）", n), "用 AI 整理成导图", tip, false).
			Disabled(noKey || n > maxMaterial || n < 2).Clicked() {
			name := "资料整理 " + a.now().Format("2006-01-02")
			if a.libCat != "" && a.libCat != "-" && a.libCat != "★" {
				name = "资料 · " + a.libCat
			} else if a.libTag != "" {
				name = "资料 · " + a.libTag
			}
			clips := a.listedMaterial()
			a.later(func() { a.aiMaterialToMap(name, clips) })
		}
		if a.doc != nil && a.path != "" {
			mergeTip := "AI 把这些资料挂到「" + mapName(a.path) + "」里最相关的节点上，或新增节点；你确认后才应用"
			if noKey {
				mergeTip = "先在 AI 助手的设置里填写 API Key"
			}
			if ui.Button(c, "并入「"+mapName(a.path)+"」").FontSize(12).Tooltip(mergeTip).
				Disabled(noKey || n > maxMaterial).Clicked() {
				clips := a.listedMaterial()
				a.later(func() { a.aiMergeMaterial(clips) })
			}
		}
	})
}
