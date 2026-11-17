package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/mcp"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// 长成分支: the clips of a tag laid out by the AI as a branch of a map,
// each topic keeping the clips it came from as links. 收集 is as it was:
// no new view of it and no grouping but its tags; with a tag chosen in the
// left column a key stands at the page's head, and what the AI proposes is
// read in the right column before anything is written. The branch is a
// map's, made of topics as any other; its first topic links to the tag
// (tagScheme), which is how the page knows there is one, from the maps
// and from no store of its own.

const (
	growMost   = 60 // the clips given to the AI at most, the newest
	growTopics = 12 // the topics of a branch at most, under its name
	growTitle  = 24 // the characters of a title at most: the rest is its note's
)

// A growTopic is a line of the outline the AI proposes.
type growTopic struct {
	Title, Note string
	Deep        int      // 0 the branch's name, 1 and 2 under it
	Src         []string // the clips it came from, by their IDs
}

// A growOutline is what the AI proposes: the branch's name first, the
// topics under it in their order, and what the clips do not speak of.
type growOutline struct {
	Topics []growTopic
	Gap    string
}

// grown is a branch put on a map by the sheet that shows.
type grown struct {
	path, id, label string
}

// growState is the sheet of 长成分支, and what the page's head was worked
// out of.
type growState struct {
	tag        string   // the tag it grows from; none: the sheet is put away
	ids        []string // the clips the AI was given
	left       int      // those sealed whole, not given
	more       int      // those beyond growMost, not given
	path, node string   // where the branch goes
	busy       bool
	since      time.Time
	err        string
	out        *growOutline
	open       int // the line whose sources show, -1 for none
	seq        int // the asking going on, which an answer must be of
	done       *grown
	seen       growSeen
}

// growSeen is what the head says of the tag chosen: how many clips a
// branch would grow from, the branch there is, and how many of the clips
// it does not link to. Worked out once for what is found and the maps'
// links as they stand.
type growSeen struct {
	hits  libHitsKey
	built int
	n     int
	at    *backlink
	fresh int
}

// growClips are the clips a branch of the tag chosen would grow from:
// what the page shows, but the maps and the clips sealed whole, which are
// counted in left and never given to an AI.
func (a *app) growClips() (clips []*library.Clip, left int) {
	for _, h := range a.libHits() {
		switch {
		case h.Clip.Kind == kindMap:
		case h.Clip.Locked:
			left++
		default:
			clips = append(clips, h.Clip)
		}
	}
	return clips, left
}

// growLook is what the head says of the tag chosen.
func (a *app) growLook() *growSeen {
	s := &a.collect.grow.seen
	a.libHits() // which brings the key of what is found up to date
	built := a.refIndex().built
	if s.hits == a.libHitsFor && s.built == built {
		return s
	}
	clips, _ := a.growClips()
	*s = growSeen{hits: a.libHitsFor, built: built, n: len(clips)}
	linked := map[string]bool{}
	for _, b := range a.refIndex().to[tagScheme+a.libTag] {
		f := a.mapFile(b.path)
		if f == nil || b.clip != "" {
			continue
		}
		top := f.Node(b.id)
		if top == nil {
			continue
		}
		if s.at == nil {
			s.at = &b
		}
		var walk func(n *model.Node)
		walk = func(n *model.Node) {
			for _, l := range n.Links {
				if id, ok := strings.CutPrefix(l, clipScheme); ok {
					linked[id] = true
				}
			}
			for _, k := range n.Children {
				walk(k)
			}
		}
		walk(top)
	}
	for _, cl := range clips {
		if !linked[cl.ID] {
			s.fresh++
		}
	}
	return s
}

// growKey is the way in, at the right of 收集's head while a tag is
// chosen: the key 长成分支, with two clips or more to grow from; once a
// branch of the tag is on a map, a quiet chip that goes to it in the
// key's place, and the key again, 再长一次, only when the tag has clips
// that branch does not link to.
func (a *app) growKey(c *ui.Context) {
	if a.libTag == "" || a.libReview != reviewOff {
		return
	}
	t := c.Theme()
	g := &a.collect.grow
	s := a.growLook()
	label := "长成分支"
	if at := s.at; at != nil {
		label = "再长一次"
		name := mapName(at.path)
		chip := ui.Row(c.Key("grown-chip")).Height(24).Padding(0, 9).Gap(5).Radius(radiusSmall).AlignItems(ui.Center).Shrink(0).
			Cursor(ui.CursorPointer).Background(t.Accent.Alpha(0.1)).Transition(easeLook).
			Label("已在「" + name + "」").Tooltip("去看导图里的这一支：" + at.title)
		if chip.Hovered() {
			chip.Background(t.Accent.Alpha(0.18))
		}
		chip.Children(func() {
			ui.Text(c, "已在「"+name+"」").FontSize(textCaption).TextColor(t.Accent).MaxWidth(200).SingleLine().PassThrough()
		})
		if chip.Clicked() {
			path, sheet, id := at.path, at.sheet, at.id
			a.later(func() { a.goToGrown(path, sheet, id) })
		}
		if s.fresh == 0 {
			return
		}
	}
	if s.n < 2 {
		return
	}
	tip := fmt.Sprintf("让 AI 把这 %d 条资料理成导图的一个分支：先看它打算怎么理，再决定放不放", s.n)
	if s.at != nil {
		tip = fmt.Sprintf("这个标签下有 %d 条资料不在那一支里：再理一次，长成新的一支", s.fresh)
	}
	b := ui.Box(c.Key("grow-key")).Height(keyHeight).Padding(0, 14).Radius(radiusPill).Center().Cursor(ui.CursorPointer).
		Label(label).Tooltip(tip).Shrink(0)
	on := g.tag != "" && g.done == nil
	if on { // its sheet shows: on the accent's tint, as what is chosen
		press(b.Background(t.Accent.Alpha(0.12)).Transition(easeLook), 0.97)
	} else {
		key(b, t)
	}
	b.Children(func() {
		txt := ui.Text(c, label).FontSize(textSmall).NoWrap().PassThrough()
		if on {
			txt.FontWeight(700).TextColor(t.Accent)
		}
	})
	if b.Clicked() {
		a.later(a.openGrow)
	}
}

// growShown tells whether the sheet of 长成分支 shows: a clip chosen has
// the column while it is, and the sheet after it, as it was.
func (a *app) growShown() bool {
	g := &a.collect.grow
	if g.tag != "" && (g.tag != a.libTag || a.libReview != reviewOff) {
		g.seq++ // an answer on its way is of a sheet put away
		*g = growState{seq: g.seq, seen: g.seen}
		stageOver("grow")
	}
	return g.tag != "" && a.lib.Find(a.libSel) == nil
}

// closeGrow puts the sheet away.
func (a *app) closeGrow() {
	g := &a.collect.grow
	*g = growState{seq: g.seq + 1, seen: g.seen}
	stageOver("grow")
}

// openGrow opens the sheet on the tag chosen and asks the AI. With no AI
// set up it leads to the settings, as the AI's other keys.
func (a *app) openGrow() {
	g := &a.collect.grow
	if a.libTag == "" {
		return
	}
	a.libSel = ""
	if g.tag == a.libTag && g.done == nil { // open already: shown again
		return
	}
	if providerKey(a.cfg) == "" {
		a.tell("长成分支要用 AI：先在 AI 设置里选好模型")
		a.openKeySettings()
		return
	}
	*g = growState{tag: a.libTag, open: -1, seq: g.seq + 1, seen: g.seen}
	g.path, g.node = a.growPlace()
	a.askGrow()
}

// growPlace is where a branch goes unless another place is chosen: the
// map opened last that is not the inbox, under its topic named 待整理 if
// it has one, else its centre.
func (a *app) growPlace() (path, node string) {
	files := a.recentFiles()
	inbox := a.inboxPath()
	slices.SortStableFunc(files, func(x, y model.FileInfo) int { // the inbox last
		return btoi(x.Path == inbox) - btoi(y.Path == inbox)
	})
	for _, fi := range files {
		f := a.mapFile(fi.Path)
		if f == nil || len(f.Sheets) == 0 {
			continue
		}
		for _, d := range f.Sheets {
			for _, k := range d.Root.Children {
				if k.Title == mcp.SortBranch {
					return fi.Path, k.ID
				}
			}
		}
		return fi.Path, f.Sheets[0].Root.ID
	}
	return "", ""
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// growPlaceLabel says a place as the field does: 「导图 › 主题」, the map
// alone for its centre. Not ok when the topic is no longer there.
func (a *app) growPlaceLabel(path, node string) (string, bool) {
	f := a.mapFile(path)
	if f == nil {
		return "", false
	}
	d := f.SheetOf(node)
	if d == nil {
		return "", false
	}
	if d.Root.ID == node {
		if len(f.Sheets) > 1 && d != f.Sheets[0] {
			return mapName(path) + " › " + cmpOr(d.Title, d.Root.Title), true
		}
		return mapName(path), true
	}
	n, _ := d.Find(node)
	return mapName(path) + " › " + cmpOr(n.Title, "（无标题）"), true
}

// growUnder is the place chosen in the palette (paletteGrow).
func (a *app) growUnder(path, _, id string) {
	if g := &a.collect.grow; g.tag != "" {
		g.path, g.node = path, id
	}
}

const growSystem = "你是 MyMind 里的整理助手：把用户收集的资料理成思维导图的一个分支。只依据给你的资料，不添加资料里没有的事实。只输出 JSON，不要解释。"

const growAsk = `请把这些资料理成思维导图的一个分支，只输出一个 JSON 对象：
{"name":"分支的名字","topics":[{"title":"主题","note":"细节","src":["资料编号"],"children":[{"title":"主题","note":"细节","src":["资料编号"]}]}],"gap":"资料里没有说到的一点"}

- name：这个分支的名字，几个字，说的是这些资料合起来在讲什么。
- topics：分支下的主题，最多两层（children 下面不要再有 children），合起来不超过 12 个。标题要短，不超过 16 个字；细节写进 note，一两句，没有就留空。
- src：这个主题是从哪几条资料来的，写资料开头方括号里的编号，只能用上面出现过的编号。每个主题至少一条；一条资料可以在几个主题下。
- 只用资料里有的内容：不要补充资料没有说的事实、数字和结论，也不要替资料下它没有下的判断。成串的圆点是遮住的密钥，不要猜，也不要写进主题。
- gap：这些资料合起来没有说到、而这个分支明显需要的一点，一句话；只标出来，不要替它回答。没有就写空字符串。`

// growPrompt is what the AI is given of the clips: each as an AI reads a
// clip (library.Brief, its secrets in dots), led by its ID.
func growPrompt(tag string, clips []*library.Clip) string {
	var b strings.Builder
	fmt.Fprintf(&b, "下面是用户收集里标签 #%s 下的 %d 条资料。每条开头方括号里是它的编号。\n\n", tag, len(clips))
	for _, cl := range clips {
		b.WriteString(library.Brief(cl, "") + "\n")
	}
	b.WriteString(growAsk)
	return b.String()
}

// askGrow asks the AI for the outline of the sheet's tag.
func (a *app) askGrow() {
	g := &a.collect.grow
	clips, left := a.growClips()
	slices.SortStableFunc(clips, func(x, y *library.Clip) int { return y.Created.Compare(x.Created) })
	g.left, g.more = left, max(len(clips)-growMost, 0)
	clips = clips[:min(len(clips), growMost)]
	g.ids = g.ids[:0]
	known := map[string]bool{}
	for _, cl := range clips {
		g.ids = append(g.ids, cl.ID)
		known[cl.ID] = true
	}
	g.out, g.err, g.open, g.done = nil, "", -1, nil
	if len(clips) < 2 {
		g.err = "要两条以上的资料才长得成一个分支"
		return
	}
	g.seq++
	seq, dir := g.seq, a.dir
	g.busy, g.since = true, a.now()
	prompt := growPrompt(g.tag, clips)
	cfg := a.cfg
	send, sendDS := a.sendAI, a.sendDeepSeek
	a.async(func() (any, error) {
		text, err := askJSON(context.Background(), cfg, send, sendDS, growSystem, prompt)
		if err != nil {
			return nil, err
		}
		return parseGrow(text, known)
	}, func(v any, err error) {
		g := &a.collect.grow
		if g.seq != seq || a.dir != dir { // put away meanwhile, or another folder's
			return
		}
		g.busy = false
		stageOver("grow")
		out, _ := v.(*growOutline)
		if err != nil || out == nil {
			if err == nil {
				err = plainError("AI 没有回答")
			}
			g.err = "没理成：" + aiError(err)
			return
		}
		g.out = out
	})
}

// parseGrow reads the AI's outline and cleans it: the IDs it was not
// given are dropped, a topic left with no source goes (the branch's name
// alone has none), the topics are two levels and growTopics at most, and
// a title too long keeps its beginning, the whole of it going to the note.
// Nothing is added: what the AI did not say is not said.
func parseGrow(text string, known map[string]bool) (*growOutline, error) {
	bad := plainError("AI 没有按格式回答，请重试")
	i, j := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if i < 0 || j < i {
		return nil, bad
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(text[i:j+1]), &raw); err != nil {
		return nil, bad
	}
	line := func(v any) string {
		s, _ := v.(string)
		return strings.Join(strings.Fields(s), " ")
	}
	sources := func(v any) []string {
		var out []string
		list, _ := v.([]any)
		for _, x := range list {
			id := strings.Trim(strings.TrimSpace(fmt.Sprint(x)), "[]")
			if known[id] && !slices.Contains(out, id) {
				out = append(out, id)
			}
		}
		return out
	}
	out := &growOutline{Gap: firstRunes(line(raw["gap"]), 80)}
	out.Topics = append(out.Topics, growTopic{})
	add := func(all []string, ids ...string) []string {
		for _, id := range ids {
			if !slices.Contains(all, id) {
				all = append(all, id)
			}
		}
		return all
	}
	// deeper are the sources of what is under a topic of the second level:
	// a third level is not made, and what it came from is that topic's.
	var deeper func(v any) []string
	deeper = func(v any) (all []string) {
		kids, _ := v.([]any)
		for _, k := range kids {
			if m, _ := k.(map[string]any); m != nil {
				all = add(add(all, sources(m["src"])...), deeper(m["children"])...)
			}
		}
		return all
	}
	// read adds the topics of a level, and gives the sources of those kept.
	var read func(v any, deep int) []string
	read = func(v any, deep int) (all []string) {
		list, _ := v.([]any)
		for _, x := range list {
			m, _ := x.(map[string]any)
			title := line(m["title"])
			if m == nil || title == "" {
				continue
			}
			if len(out.Topics)-1 >= growTopics {
				break
			}
			note, _ := m["note"].(string)
			tp := growTopic{Title: title, Note: strings.TrimSpace(note), Deep: deep, Src: sources(m["src"])}
			if r := []rune(title); len(r) > growTitle {
				tp.Title = string(r[:growTitle]) + "…"
				tp.Note = strings.TrimSpace(title + "\n" + tp.Note)
			}
			at := len(out.Topics)
			out.Topics = append(out.Topics, tp)
			if deep < 2 {
				// With none of its own, a topic comes from what those under
				// it come from.
				if under := read(m["children"], deep+1); len(tp.Src) == 0 {
					tp.Src = under
				}
			} else {
				tp.Src = add(tp.Src, deeper(m["children"])...)
			}
			if len(tp.Src) == 0 { // of no clip given: not kept
				out.Topics = out.Topics[:at]
				continue
			}
			out.Topics[at].Src = tp.Src
			all = add(all, tp.Src...)
		}
		return all
	}
	read(raw["topics"], 1)
	if len(out.Topics) < 2 {
		return nil, plainError("这几条资料里没理出可以成分支的主题")
	}
	name := line(raw["name"])
	if r := []rune(name); len(r) > growTitle {
		name = string(r[:growTitle]) + "…"
	}
	out.Topics[0].Title = name
	return out, nil
}

// plantGrow writes the branch under the place chosen, as one step: its
// name a topic that links to the tag, the others each linking to the
// clips they came from, so that a topic shows its clips and a clip's
// details say the topic. The clips stay in 收集.
func (a *app) plantGrow() {
	g := &a.collect.grow
	if g.out == nil || g.done != nil {
		return
	}
	tag, path, node := g.tag, g.path, g.node
	var top, last *model.Node
	for _, tp := range g.out.Topics {
		n := model.NewNode(tp.Title)
		n.Note = sealedOut(tp.Note)
		for _, id := range tp.Src {
			if a.lib.Find(id) != nil {
				n.Links = append(n.Links, clipScheme+id)
			}
		}
		switch {
		case top == nil:
			top = n
			if n.Title == "" {
				n.Title = "#" + tag
			}
			n.Links = []string{tagScheme + tag} // what it was grown from
			if g.out.Gap != "" {                // marked, not filled
				n.Note = "资料里没有说到：" + g.out.Gap
			}
		case tp.Deep >= 2 && last != nil:
			last.Children = append(last.Children, n)
		default:
			top.Children = append(top.Children, n)
			last = n
		}
	}
	if top == nil {
		return
	}
	err := a.editMapAt(path, "长成分支", func(f *model.File) error {
		d := f.SheetOf(node)
		if d == nil {
			return errors.New("要放到的主题已不存在，换一个地方")
		}
		under, _ := d.Find(node)
		under.Children = append(under.Children, top)
		under.Collapsed = false
		return nil
	})
	if err != nil {
		g.err = "没有放过去：" + err.Error()
		return
	}
	label, _ := a.growPlaceLabel(path, node)
	g.done = &grown{path: path, id: top.ID, label: label}
	id := top.ID
	// The way back takes that branch out, by its first topic: not the last
	// step of whatever map shows by then.
	a.tellUndo("已把 #"+tag+" 长成分支", func() {
		err := a.editMapAt(path, "撤销长成的分支", func(f *model.File) error {
			d := f.SheetOf(id)
			if d == nil {
				return errors.New("那个分支已不存在")
			}
			_, err := d.Delete(id)
			return err
		})
		if err != nil {
			a.tell("没有撤销：" + err.Error())
			return
		}
		if a.path == path && a.doc != nil {
			if n, _ := a.doc.Find(a.selected); n == nil { // what was chosen went with it
				a.selected, a.multi = a.shownRoot().ID, nil
			}
		}
		if g := &a.collect.grow; g.done != nil && g.done.id == id {
			a.closeGrow()
		}
	})
}

// goToGrown opens the map at a branch grown.
func (a *app) goToGrown(path, sheet, id string) {
	if f := a.mapFile(path); f != nil {
		if d := f.SheetOf(id); d != nil {
			sheet = d.ID
		}
	}
	a.closeGrow()
	a.goToNode(path, sheet, id)
}

// growSheet is the sheet of 长成分支, in the right column where a clip's
// details are: from what, to where, and the outline the AI proposes, read
// before anything is written.
func (a *app) growSheet(c *ui.Context) {
	t := c.Theme()
	g := &a.collect.grow
	part := func(name string) { ui.Text(c, name).FontSize(textSmall).FontWeight(700).Margin(4, 0, 0) }
	esc := !a.barOpen && !a.paletteOpen && !c.FocusTakesText() && c.Shortcut(0, ui.KeyEscape)
	ui.Row(c.Key("grow-head")).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "长成分支").FontSize(textHeadline).FontWeight(700).LetterSpacing(-0.3).Grow(1)
		if iconButton(c, iconClose, "关闭长成分支", "关闭", 28, true).Clicked() || esc {
			a.later(a.closeGrow)
		}
	})
	from := fmt.Sprintf("从 #%s 的 %d 条资料", g.tag, len(g.ids))
	if g.more > 0 {
		from += fmt.Sprintf("（最近的 %d 条，另有 %d 条没取）", len(g.ids), g.more)
	}
	ui.Text(c, from).FontSize(textCaption).TextColor(t.TextMuted).Wrap().Margin(-6, 0, 0)
	if g.left > 0 {
		ui.Text(c, fmt.Sprintf("另有 %d 条整条加密的，没有给 AI。", g.left)).FontSize(textCaption).TextColor(t.TextMuted).Wrap().Margin(-8, 0, 0)
	}
	if d := g.done; d != nil {
		ui.Text(c, "已经是「"+d.label+"」下的一个分支。每个主题记得它的来源：在导图里点一个主题，右栏列出这几条资料。").
			FontSize(textSmall).LineHeight(1.5).TextColor(ink2(t)).Wrap()
		a.growLines(c, false)
		ui.Row(c.Key("grow-done")).Gap(8).Margin(4, 0, 0).Children(func() {
			if inkButton(c, "去看看", true).Clicked() {
				path, id := d.path, d.id
				a.later(func() { a.goToGrown(path, "", id) })
			}
			if softButton(c, "好了", "").Clicked() {
				a.later(a.closeGrow)
			}
		})
		return
	}

	part("放到")
	label, ok := a.growPlaceLabel(g.path, g.node)
	if !ok {
		label = "选一张导图和它的一个主题"
	}
	field := ui.Row(c.Key("grow-place")).MinHeight(36).Padding(8, 12).Gap(8).Radius(radiusPill).AlignItems(ui.Center).FillWidth().
		Background(blockFill(t)).Cursor(ui.CursorPointer).Transition(easeLook).Label("换个地方放").Tooltip("换一张导图，或它的另一个主题")
	if field.Hovered() {
		field.Background(t.Text.Alpha(0.07))
	}
	field.Children(func() {
		if ok {
			mapDot(c, a.mapColor(g.path), 8).PassThrough()
		}
		ui.Text(c, label).FontSize(textSmall).SingleLine().Grow(1).Shrink(1).MinWidth(0).PassThrough()
		ui.Text(c, "改").FontSize(textCaption).TextColor(t.TextMuted).NoWrap().PassThrough()
	})
	if field.Clicked() {
		a.later(func() { a.openPalette(paletteGrow) })
	}

	part("AI 打算这样理")
	switch {
	case g.busy:
		stageLines(c, "grow", []string{fmt.Sprintf("读了 #%s 的 %d 条资料", g.tag, len(g.ids))}, "AI 正在把它们理成一个分支…")
	case g.out != nil:
		a.growLines(c, true)
		if g.out.Gap != "" {
			ui.Box(c.Key("grow-gap")).Padding(8, 10).Radius(radiusRow).Border(1, faint(t)).BorderStyle(ui.BorderDashed).FillWidth().Children(func() {
				ui.Text(c, "资料里没有说到："+g.out.Gap).FontSize(textSmall).LineHeight(1.5).TextColor(ink2(t)).Wrap()
			})
		}
	}
	// What failed stays said here, with the way to try again.
	if g.err != "" {
		ui.Text(c.Key("grow-err"), g.err).FontSize(textSmall).LineHeight(1.5).TextColor(t.Danger).Wrap().Selectable()
	}
	ui.Row(c.Key("grow-keys")).Gap(8).Margin(4, 0, 0).Children(func() {
		switch {
		case g.busy:
		case g.out != nil:
			if inkButton(c, "放到导图", ok).Clicked() {
				a.later(a.plantGrow)
			}
		default:
			if softButton(c, "重试", "再让 AI 理一次").Clicked() {
				a.later(a.askGrow)
			}
		}
		if softButton(c, "先不", "什么都不写，收集和导图都不变").Clicked() {
			a.later(a.closeGrow)
		}
	})
}

// growLines are the outline's lines: the branch's name heavy, those under
// it set in, each with how many clips it came from at its end; a click on
// one (with live) opens the names of its sources under it, each going to
// its clip.
func (a *app) growLines(c *ui.Context, live bool) {
	t := c.Theme()
	g := &a.collect.grow
	if g.out == nil {
		return
	}
	ui.Column(c.Key("grow-out")).FillWidth().Children(func() {
		for i, tp := range g.out.Topics {
			in := float32(tp.Deep) * 16
			row := ui.Row(c.Key(fmt.Sprint("o", i))).MinHeight(34).Padding(6, 0, 6, in).Gap(8).AlignItems(ui.Center).FillWidth().
				BorderWidth(0, 0, 1, 0).BorderColor(hairline(t))
			can := live && len(tp.Src) > 0
			if can {
				row.Cursor(ui.CursorPointer).Label("来源 " + tp.Title)
				if row.Clicked() {
					if g.open == i {
						g.open = -1
					} else {
						g.open = i
					}
				}
			}
			row.Children(func() {
				txt := ui.Text(c, cmpOr(tp.Title, "#"+g.tag)).FontSize(textBody).LineHeight(1.4).Wrap().MaxLines(2).Grow(1).Shrink(1).Basis(0).MinWidth(0).PassThrough()
				if tp.Deep == 0 {
					txt.FontWeight(700)
				}
				if len(tp.Src) > 0 {
					col := t.TextMuted
					if can && (row.Hovered() || g.open == i) {
						col = t.Accent
					}
					ui.Text(c, fmt.Sprintf("%d 条来源", len(tp.Src))).FontSize(textCaption).TextColor(col).FontFeatures("tnum").NoWrap().PassThrough()
				}
			})
			if !can || g.open != i {
				continue
			}
			ui.Column(c.Key(fmt.Sprint("from", i))).Padding(4, 0, 8, in+16).Gap(2).FillWidth().BorderWidth(0, 0, 1, 0).BorderColor(hairline(t)).Children(func() {
				for _, id := range tp.Src {
					cl := a.lib.Find(id)
					if cl == nil {
						continue
					}
					name := firstSentence(cl.Label())
					src := ui.Row(c.Key(id)).Gap(6).AlignItems(ui.Center).Padding(2, 0).FillWidth().Cursor(ui.CursorPointer).
						Label("资料 " + name).Tooltip("看这条资料")
					ink := t.TextMuted
					if src.Hovered() {
						ink = t.Text
					}
					src.Children(func() {
						mapDot(c, clipColor(cl.Kind), 5).PassThrough()
						ui.Text(c, name).FontSize(textCaption).TextColor(ink).SingleLine().Grow(1).Shrink(1).MinWidth(0).PassThrough()
					})
					if src.Clicked() {
						a.libSel, a.libTagText = id, ""
					}
				}
			})
		}
	})
}
