package main

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// Links both ways, wherever something is written. [[ begun in a thought
// or in a node's note offers what there is to link to: thoughts and clips,
// maps, the topics of every map; one chosen is a link there, and what it
// links to says so in turn (refIndex). A thought keeps a link as a token
// in its words, <clip:id> or <mindmap:map#id> (refToken); a node's note as
// a link of its Markdown, [name](mindmap:map#id) (noteLink). Before this a
// thought could name only another thought or clip, and a note nothing.

// refToken is a link in a thought's words: to a clip, a topic or a map.
var refToken = regexp.MustCompile(`<((?:clip|mindmap):[^>\n]+)>`)

// noteLink is a link of this kind in a node's note, as its Markdown has it.
var noteLink = regexp.MustCompile(`\]\(<?((?:clip|mindmap):[^)>\n]+)>?\)`)

// A linkOffer is something [[ offers to link to.
type linkOffer struct {
	link  string // "clip:id" or "mindmap:map#id", whole: never relative to a map
	name  string // as it is written and shown
	where string // a word on it: its kind, or the map it is of
}

// linkOffers are what [[part offers: the thoughts and clips named so, the
// maps, and, once something is written, the topics of every map; n of
// each at most.
func (a *app) linkOffers(part string, n int) []linkOffer {
	var out []linkOffer
	low := strings.ToLower(strings.TrimSpace(part))
	for _, cl := range a.clipsNamed(part, n) {
		kind := "资料"
		if isMemo(cl) {
			kind = "灵感"
		}
		out = append(out, linkOffer{clipScheme + cl.ID, refName(cl), kind})
	}
	maps := 0
	for _, f := range a.recentFiles() {
		if maps < n && f.Path != a.inboxPath() && strings.Contains(strings.ToLower(mapName(f.Path)), low) {
			if l := a.mapLink(f.Path); l != "" {
				out = append(out, linkOffer{l, mapName(f.Path), "导图"})
				maps++
			}
		}
	}
	if low == "" {
		return out
	}
	topics := 0
	for _, fi := range a.recentFiles() {
		f := a.mapFile(fi.Path)
		if f == nil || topics >= n {
			continue
		}
		for _, d := range f.Sheets {
			d.Walk(func(nd, _ *model.Node, _ int) bool {
				if topics >= n {
					return false
				}
				if nd != d.Root && strings.Contains(strings.ToLower(nd.Title), low) {
					out = append(out, linkOffer{a.wholeNodeLink(fi.Path, nd.ID), nd.Title, mapName(fi.Path)})
					topics++
				}
				return true
			})
		}
	}
	return out
}

// wholeNodeLink is the link to a node that says its map whatever map is
// open: what a thought keeps, which is of no map.
func (a *app) wholeNodeLink(path, id string) string {
	rel, err := filepath.Rel(a.dir, path)
	if err != nil {
		rel = path
	}
	return nodeScheme + filepath.ToSlash(strings.TrimSuffix(rel, model.Ext)) + "#" + id
}

// offerChips shows what [[part offers as chips, and reports the one
// pressed.
func (a *app) offerChips(c *ui.Context, part string) (linkOffer, bool) {
	t := c.Theme()
	var chosen linkOffer
	got := false
	offers := a.linkOffers(part, 4)
	if len(offers) == 0 {
		ui.Text(c, "没有找到可以链接的：换个词，灵感、资料、导图和主题都能搜").FontSize(textCaption).TextColor(faint(t))
		return chosen, false
	}
	for i, o := range offers {
		chip := ui.Row(c.Key(i)).Height(26).Padding(0, 9).Gap(5).Radius(radiusSmall).AlignItems(ui.Center).Shrink(0).MaxWidth(260).
			Cursor(ui.CursorPointer).Label("链接到 " + o.name).Tooltip(o.where + "：" + o.name).Background(t.Text.Alpha(0.05)).Transition(easeLook)
		if chip.Hovered() {
			chip.Background(t.Accent.Alpha(0.14))
		}
		press(chip, 0.96)
		chip.Children(func() {
			ui.Text(c, o.name).FontSize(textSmall).SingleLine().Shrink(1).MinWidth(0).MaxWidth(170).PassThrough()
			ui.Text(c, o.where).FontSize(textCaption).TextColor(faint(t)).NoWrap().MaxWidth(70).SingleLine().PassThrough()
		})
		if chip.Clicked() {
			chosen, got = o, true
		}
	}
	return chosen, got
}

// linkBegun is what follows a [[ not yet closed at the end of text, and
// where the [[ is, in bytes; ok when there is one.
func linkBegun(text string) (part string, at int, ok bool) {
	at = strings.LastIndex(text, "[[")
	if at < 0 || strings.ContainsAny(text[at:], "]\n") {
		return "", 0, false
	}
	return text[at+2:], at, true
}

// noteLinkOffers shows, under a node's note being written, what the [[
// begun before its caret offers; one chosen takes the [['s place as a link.
func (a *app) noteLinkOffers(c *ui.Context, place string, d *noteDoc, n *model.Node) {
	_, caret := d.doc.Selection()
	text := d.doc.Text()
	if caret < 0 || caret > text.Len() {
		return
	}
	part, at, ok := linkBegun(text.Slice(0, caret).PlainText())
	if !ok {
		return
	}
	start := caret - utf8.RuneCountInString(part) - 2
	_ = at
	ui.Row(c.Key(place+"-link-offers")).Gap(6).GapY(4).Padding(4, 0, 0).Wrap().AlignItems(ui.Center).FillWidth().Children(func() {
		if o, got := a.offerChips(c, part); got && start >= 0 {
			link := o.link
			if p, id, isNode := a.nodeOfLink(link); isNode && p == a.path { // of this map: said shortly, and moved with it
				link = nodeScheme + "#" + id
			}
			a.later(func() {
				d.doc.SetSelection(start, caret)
				d.doc.Insert(richtext.NewText("").Append(o.name, richtext.Attributes{Link: link}).Append(" ", richtext.Attributes{}))
				a.saveNoteDoc(d, n)
			})
		}
	})
}

// nodeOfLink is the map and the node a link to a node names, as written
// (not resolved after a move), and whether it is one.
func (a *app) nodeOfLink(link string) (path, id string, ok bool) {
	rest, ok := strings.CutPrefix(link, nodeScheme)
	if !ok {
		return "", "", false
	}
	rel, id, _ := strings.Cut(rest, "#")
	if rel == "" {
		return a.path, id, true
	}
	return filepath.Join(a.dir, filepath.FromSlash(rel)+model.Ext), id, true
}

// tokenLinks are the links a thought's words hold, in order, each once.
func tokenLinks(text string) []string {
	var out []string
	for _, m := range refToken.FindAllStringSubmatch(text, -1) {
		if !slices.Contains(out, m[1]) {
			out = append(out, m[1])
		}
	}
	return out
}
