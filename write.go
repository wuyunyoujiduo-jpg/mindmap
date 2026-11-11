package main

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
)

// writeCard is the card to write a note in, on the desk and in 收集: a
// white card, the one thing outlined in the accent while written in, with
// what a note can take under the words, as flomo's: a tag (those used
// last are offered as one is written), a picture, bold, a list with
// points or with numbers, another note to refer to (offered after an @),
// and a key that keeps it.
type writeCard struct {
	key, label, placeholder string
	draft                   *string
	enterSaves              bool   // Enter keeps it and Shift+Enter breaks the line; else ⌘Enter keeps it
	sendLabel               string // the label of the key that keeps it
	fileLabel               string // the label of the tool that takes a picture or a file
	save                    func()
	paste                   func(text string) bool // what is pasted, when it is not text
	reads                   func()                 // what the card says of the words, after the tools
	lines                   int                    // the lines it shows empty: 2 when zero
	focused                 func()                 // told while it is written in
	esc                     func()                 // told when Escape is pressed in it
	over                    bool                   // it lies over the page: lifted
}

func (a *app) writeCard(c *ui.Context, w writeCard) {
	t := c.Theme()
	box := soft(ui.Column(c.Key(w.key)).Radius(radiusCard).Padding(14, 14, 12, 18).Gap(4).FillWidth().Shrink(0).
		Background(cardColor(t)).Border(1.5, t.Text.Alpha(0)), t).Transition(easeLook)
	if w.over {
		lifted(box.NoShadow(), t)
	}
	box.Children(func() {
		lines := 2
		if w.lines > 0 {
			lines = w.lines
		}
		in := ui.TextAreaBase(c.Key(w.key+"-input"), w.draft).Lines(lines, 6).
			Placeholder(w.placeholder).FontSize(textTitle).LineHeight(1.5).FillWidth().Label(w.label)
		if w.paste != nil {
			in.OnPaste(w.paste)
		}
		captureFocus(c, box, in)
		if w.focused != nil && in.Focused() {
			w.focused()
		}
		if w.esc != nil && in.Shortcut(0, ui.KeyEscape) {
			w.esc()
		}
		if a.writeRefocus == w.key {
			a.writeRefocus = ""
			in.Focus()
		}
		keep := in.Shortcut(ui.Cmd, ui.KeyEnter)
		if w.enterSaves {
			in.HandleInput(func(ev ui.InputEvent) bool {
				if ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyEnter && ev.Mods&ui.Shift == 0 {
					a.later(w.save)
					return true
				}
				return false
			})
		}
		text := strings.TrimSpace(*w.draft)
		ui.Row(c.Key("tools")).Gap(6).AlignItems(ui.Center).FillWidth().Wrap().GapY(4).Children(func() {
			tool := func(key, mark string, ic icon, tip string, click func()) {
				b := ui.Box(c.Key(key)).Size(32, 32).Radius(radiusPill).Center().Cursor(ui.CursorPointer).Label(key).Tooltip(tip).
					Background(t.Text.Alpha(0)).Transition(easeLook)
				ink := t.TextMuted
				if b.Hovered() {
					b.Background(t.Text.Alpha(0.05))
					ink = t.Text
				}
				if b.Clicked() {
					click()
					a.writeRefocus = w.key // writing goes on
				}
				b.Children(func() {
					if mark != "" {
						ui.Text(c, mark).FontSize(16).FontWeight(500).TextColor(ink).PassThrough()
					} else {
						glyph(c, ic, 17, ink)
					}
				})
			}
			line := func(lead string) { // a new line beginning so, or the first
				d := *w.draft
				if d != "" && !strings.HasSuffix(d, "\n") {
					d += "\n"
				}
				*w.draft = d + lead
			}
			add := func(s string) { *w.draft = strings.TrimRight(*w.draft, " ") + " " + s }
			tool("加标签", "#", 0, "加一个 #标签：会列出最近用过的", func() { add("#") })
			tool(w.fileLabel, "", iconImage, "收一张图片或一个文件（也可以直接粘贴或拖进来）", func() {
				a.plat.OpenFile("选择图片或文件", nil, func(path string) {
					if path != "" {
						a.dropClips([]string{path})
					}
				})
			})
			ui.Box(c).Width(1).Height(16).Margin(0, 4).Background(hairline(t))
			tool("加粗", "B", 0, "加粗：点一下开始，写完再点一下结束（**粗体**）", func() { *w.draft += "**" })
			tool("圆点列表", "", iconLevels, "列表：每行一个点", func() { line("- ") })
			tool("编号列表", "1.", 0, "编号列表：1. 2. 3.", func() {
				n := 1
				for _, l := range strings.Split(*w.draft, "\n") {
					if numberedLine.MatchString(l) {
						n++
					}
				}
				line(fmt.Sprintf("%d. ", n))
			})
			ui.Box(c).Width(1).Height(16).Margin(0, 4).Background(hairline(t))
			tool("引用", "@", 0, "引用另一条灵感或资料：会列出最近的", func() { add("@") })
			if w.reads != nil {
				w.reads()
			}
			// While a tag or a reference is being written, those used last
			// that begin so: a click writes one.
			if part, ok := beingWritten(*w.draft, "#"); ok {
				for _, tag := range a.recentTags(part, 6) {
					chip := softTag(c, "#"+tag).Cursor(ui.CursorPointer).Label("用标签 " + tag).Tooltip("用这个标签")
					if chip.Clicked() {
						*w.draft = (*w.draft)[:len(*w.draft)-len(part)] + tag + " "
						a.writeRefocus = w.key
					}
				}
			}
			if part, ok := beingWritten(*w.draft, "@"); ok {
				for _, cl := range a.clipsNamed(part, 5) {
					name := refName(cl)
					chip := softTag(c, "↗ "+name).MaxWidth(200).Cursor(ui.CursorPointer).Label("引用 " + name).Tooltip(cl.Label())
					if chip.Clicked() {
						if a.writeRefs == nil {
							a.writeRefs = map[string]string{}
						}
						a.writeRefs["@"+name] = cl.ID
						*w.draft = (*w.draft)[:len(*w.draft)-len(part)] + name + " "
						a.writeRefocus = w.key
					}
				}
			}
			ui.Spacer(c)
			if !w.enterSaves && text != "" {
				ui.Text(c, "⌘↩").FontSize(textCaption).TextColor(t.TextMuted)
			}
			tip := "记下（⌘回车）"
			if w.enterSaves {
				tip = "记下（回车；Shift+回车换行）"
			}
			send := ui.Box(c.Key("send")).Size(52, 32).Radius(radiusPill).Center().Label(w.sendLabel).Tooltip(tip).
				Background(t.Text.Alpha(0.12)).Transition(easeLook)
			press(send, 0.94)
			if text != "" {
				jewel(send, t, true).Cursor(ui.CursorPointer)
				if send.Clicked() {
					keep = true
				}
				press(send, 0.94)
			}
			send.Children(func() { glyph(c, iconSend, 16, ui.Hex("#FFFFFF")) })
		})
		if keep && text != "" {
			a.later(w.save)
		}
	})
}

var numberedLine = regexp.MustCompile(`^\d+\. `)

// beingWritten tells whether a draft ends in a word begun with mark (a #
// or an @) and not yet finished, with no space after it, and gives what
// follows the mark.
func beingWritten(draft, mark string) (string, bool) {
	i := strings.LastIndex(draft, mark)
	if i < 0 || strings.ContainsAny(draft[i:], " \n\t　") {
		return "", false
	}
	return draft[i+len(mark):], true
}

// recentTags are the tags of the notes and clips, those used last first,
// that begin with part: n at most.
func (a *app) recentTags(part string, n int) []string {
	var out []string
	for _, cl := range a.clipsByTime() {
		for _, tag := range cl.Tags {
			if len(out) < n && strings.HasPrefix(tag, part) && tag != part && !slices.Contains(out, tag) {
				out = append(out, tag)
			}
		}
	}
	return out
}

// clipsNamed are the notes and clips kept last whose name has part in
// it: n at most.
func (a *app) clipsNamed(part string, n int) []*library.Clip {
	var out []*library.Clip
	for _, cl := range a.clipsByTime() {
		if len(out) < n && strings.Contains(strings.ToLower(refName(cl)), strings.ToLower(part)) {
			out = append(out, cl)
		}
	}
	return out
}

func (a *app) clipsByTime() []*library.Clip {
	clips := slices.Clone(a.lib.Clips)
	slices.SortStableFunc(clips, func(x, y *library.Clip) int { return y.Created.Compare(x.Created) })
	return clips
}

// refName is the name a clip is referred to by in a note: its label,
// short, with no space, for the name ends where a space is.
func refName(cl *library.Clip) string {
	name := strings.Join(strings.Fields(clipRef.ReplaceAllString(cl.Label(), "")), "")
	if r := []rune(name); len(r) > 16 {
		name = string(r[:16])
	}
	return name
}

// clipRef is a reference to a clip in a note's text: <clip:id>.
var clipRef = regexp.MustCompile(`<clip:([^>\s]+)>`)

// resolveRefs writes the references chosen in a draft (@name) as they are
// kept (<clip:id>), and forgets them.
func (a *app) resolveRefs(text string) string {
	for name, id := range a.writeRefs {
		text = strings.ReplaceAll(text, name, "<"+clipScheme+id+">")
	}
	a.writeRefs = nil
	return text
}

// noteText shows a note's words: as they are, or, where they are marked
// (bold, lists), as the marks say; the notes it refers to are under the
// words, each going to its note.
func (a *app) noteText(c *ui.Context, text string, size float32, lines int) {
	t := c.Theme()
	var refs []*library.Clip
	for _, m := range clipRef.FindAllStringSubmatch(text, -1) {
		if cl := a.lib.Find(m[1]); cl != nil && !slices.Contains(refs, cl) {
			refs = append(refs, cl)
		}
	}
	body := strings.TrimSpace(clipRef.ReplaceAllString(text, ""))
	switch {
	case body == "":
	case noteMarked.MatchString(body):
		markdownView(c.Key("marked"), body, size)
	default:
		ui.Text(c, body).FontSize(size).LineHeight(1.65).Wrap().MaxLines(lines).FillWidth()
	}
	for _, cl := range refs {
		b := ui.Row(c.Key("ref:" + cl.ID)).Gap(6).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label("前往引用 " + refName(cl)).Tooltip("打开这条")
		ink := t.TextMuted
		if b.Hovered() {
			ink = t.Accent
		}
		if b.Clicked() {
			a.later(func() {
				a.openList(kindAll, "")
				a.libSel = cl.ID
			})
		}
		b.Children(func() {
			ui.Text(c, "↗").FontSize(textSmall).TextColor(ink)
			ui.Text(c, cl.Label()).FontSize(textSmall).TextColor(ink).SingleLine().Shrink(1).MinWidth(0)
		})
	}
}

// noteMarked tells a note with marks to show by: bold, or a list.
var noteMarked = regexp.MustCompile(`\*\*[^*\n]+\*\*|(?m)^(- |\d+\. )`)
