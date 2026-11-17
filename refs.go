package main

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
	"path/filepath"
	"strings"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/versions"
)

// Material and maps are related one way only: a node's links, to a clip
// ("clip:id"), a node of any map ("mindmap:map#id"), a whole map (its
// first canvas's root), or a skill of the AI agents ("skill:name"). Where
// a clip or a skill is used, and what links to a node or a map, are
// worked out from the maps, in one index of every link.

// refs is the index of the links of every map.
type refs struct {
	// sig is another whenever a map or the library changed, in its links
	// or not: what else is worked out from the maps is worked out again
	// then (the places a clip may go to).
	sig string
	// to lists the nodes linking to a target: "clip:id", "skill:name",
	// "node:path#id", and "map:path" for any node of a map, from other maps.
	to map[string][]backlink
	// nodeAt is the map of each node, by its ID, to find a node linked to
	// after its map was renamed or moved.
	nodeAt map[string]string

	// What each map says, by its path, and what the index was made of:
	// a map that changed is read again alone, and the index made again
	// only when what it links to did.
	maps    map[string]*mapRefs
	dir     string
	path    string // the open map, which a link with no map named is to
	lib     *library.Library
	libVer  int
	version int
}

// mapRefs is what a map says to the index: its links, and its topics.
type mapRefs struct {
	// As of which state of the map: its file's time or, for the open map,
	// its count of changes.
	open  bool
	mod   time.Time
	edits int
	file  *model.File
	links []refLink
	ids   []string
}

// refLink is a link of a topic: on which canvas, of which topic, to what.
type refLink struct {
	sheet, id, title, link string
}

// refScans counts the maps read for the index, for the tests.
var refScans int

// refIndex is the index of the links, brought up to date as the maps
// change: only the maps that changed are read again, the open one as it
// is edited, the others by their files' times; and the index is made
// again only when a link, a linking topic's title or the topics there are
// changed. (Made whole of every map at every change of the open one, a
// key typed with a topic's page showing read sixty maps again.)
func (a *app) refIndex() *refs {
	r := a.refs
	if r == nil || r.dir != a.dir {
		r = &refs{dir: a.dir, nodeAt: map[string]string{}, maps: map[string]*mapRefs{}}
		a.refs = r
	}
	changed, read := false, false
	for _, fi := range a.files {
		open := fi.Path == a.path && a.file != nil
		m := r.maps[fi.Path]
		if m != nil && m.open == open && (open && m.edits == a.edits && m.file == a.file || !open && m.mod.Equal(fi.Modified)) {
			continue
		}
		now := a.mapRefsOf(fi.Path)
		now.open, now.mod, now.edits = open, fi.Modified, a.edits
		if open {
			now.file = a.file
		}
		r.maps[fi.Path], read = now, true
		if m != nil && slices.Equal(m.links, now.links) && slices.Equal(m.ids, now.ids) {
			continue // changed, but not in what it links to
		}
		changed = true
		if m != nil {
			for _, id := range m.ids {
				if r.nodeAt[id] == fi.Path {
					delete(r.nodeAt, id)
				}
			}
		}
		for _, id := range now.ids {
			r.nodeAt[id] = fi.Path
		}
	}
	if len(r.maps) > len(a.files) { // maps that are gone, or were renamed
		here := map[string]bool{}
		for _, fi := range a.files {
			here[fi.Path] = true
		}
		for path, m := range r.maps {
			if here[path] {
				continue
			}
			for _, id := range m.ids {
				if r.nodeAt[id] == path {
					delete(r.nodeAt, id)
				}
			}
			delete(r.maps, path)
			changed = true
		}
	}
	if r.lib != a.lib || r.libVer != a.lib.Version || r.path != a.path { // the thoughts link too
		r.lib, r.libVer, r.path, changed = a.lib, a.lib.Version, a.path, true
	}
	if changed || read || r.sig == "" {
		r.version++
		r.sig = fmt.Sprint(r.dir, "|", r.version)
	}
	if !changed && r.to != nil {
		return r
	}
	r.to = map[string][]backlink{}
	for _, fi := range a.files {
		m := r.maps[fi.Path]
		if m == nil {
			continue
		}
		for _, l := range m.links {
			a.addRef(r, backlink{path: fi.Path, sheet: l.sheet, id: l.id, title: l.title}, l.link)
		}
	}
	// A thought's links to topics and maps: what it links to says so. Its
	// links to other thoughts are told by clipMentions, and are no use of
	// a clip on a map (clipBacklinks).
	for _, cl := range a.lib.Clips {
		if !strings.Contains(cl.Text, "<"+nodeScheme) && !strings.Contains(cl.Text, "<"+skillScheme) {
			continue
		}
		for _, link := range tokenLinks(cl.Text) {
			if strings.HasPrefix(link, skillScheme) {
				r.to[link] = append(r.to[link], backlink{title: cl.Label(), clip: cl.ID})
				continue
			}
			path, id, ok := a.nodeOfLink(link)
			if !ok {
				continue
			}
			b := backlink{title: cl.Label(), clip: cl.ID}
			r.to["node:"+path+"#"+id] = append(r.to["node:"+path+"#"+id], b)
			r.to["map:"+path] = append(r.to["map:"+path], b)
		}
	}
	return r
}

// mapRefsOf reads what a map says to the index: every link of its topics,
// those written in a note too, and its topics' IDs.
func (a *app) mapRefsOf(path string) *mapRefs {
	refScans++
	m := &mapRefs{}
	f := a.mapFile(path)
	if f == nil {
		return m
	}
	for _, d := range f.Sheets {
		var walk func(n *model.Node)
		walk = func(n *model.Node) {
			m.ids = append(m.ids, n.ID)
			for _, l := range n.Links {
				m.links = append(m.links, refLink{d.ID, n.ID, n.Title, l})
			}
			// The links written in its note ([[…) are links of it too.
			if strings.Contains(n.Note, "](") {
				for _, nl := range noteLink.FindAllStringSubmatch(n.Note, -1) {
					if !slices.Contains(n.Links, nl[1]) {
						m.links = append(m.links, refLink{d.ID, n.ID, n.Title, nl[1]})
					}
				}
			}
			for _, c := range n.Children {
				walk(c)
			}
		}
		walk(d.Root)
		for _, fl := range d.Floating {
			walk(fl)
		}
	}
	return m
}

func (a *app) addRef(r *refs, b backlink, link string) {
	path := b.path
	if id, ok := strings.CutPrefix(link, clipScheme); ok {
		r.to[clipScheme+id] = append(r.to[clipScheme+id], b)
		return
	}
	if strings.HasPrefix(link, skillScheme) {
		r.to[link] = append(r.to[link], b)
		return
	}
	rest, ok := strings.CutPrefix(link, nodeScheme)
	if !ok {
		return
	}
	rel, id, _ := strings.Cut(rest, "#")
	target := path
	if rel != "" {
		target = filepath.Join(a.dir, filepath.FromSlash(rel)+model.Ext)
	}
	r.to["node:"+target+"#"+id] = append(r.to["node:"+target+"#"+id], b)
	if target != path {
		r.to["map:"+target] = append(r.to["map:"+target], b)
	}
}

// resolveNode finds the map a node linked to is in: its own, or, when the
// map was renamed or moved, the map now holding a node of that ID.
func (a *app) resolveNode(path, id string) string {
	if id == "" {
		return path
	}
	if f := a.mapFile(path); f != nil && f.SheetOf(id) != nil {
		return path
	}
	if p, ok := a.refIndex().nodeAt[id]; ok {
		return p
	}
	return path
}

// mapLink is the link to a whole map: to its first canvas's root.
func (a *app) mapLink(path string) string {
	f := a.mapFile(path)
	if f == nil || len(f.Sheets) == 0 {
		return ""
	}
	rel, err := filepath.Rel(a.dir, path)
	if err != nil {
		rel = path
	}
	return nodeScheme + filepath.ToSlash(strings.TrimSuffix(rel, model.Ext)) + "#" + f.Sheets[0].Root.ID
}

// editMapAt changes the map at path: the open one as a step undo takes
// back; another on disk, keeping its versions before and after, and
// dropping its undo steps, which hold it as it was.
func (a *app) editMapAt(path, note string, fn func(f *model.File) error) error {
	if path == a.path && a.file != nil {
		var err error
		a.mutate("", func() { err = fn(a.file) })
		return err
	}
	f, err := model.Load(path)
	if err != nil {
		return err
	}
	before := f.Clone()
	if err := fn(f); err != nil {
		return err
	}
	now := a.now()
	_, _ = a.versions.Save(path, before, versions.Auto, "", now)
	if err := f.Save(path); err != nil {
		return err
	}
	_, _ = a.versions.Save(path, f, versions.Auto, note, now.Add(1))
	delete(a.hists, path)
	delete(a.index, path) // read again, though written twice within the clock's tick
	a.refreshFiles()
	return nil
}

// attachTo links a node of any map to something: a clip, a map or a node.
func (a *app) attachTo(path, nodeID, link, what string) error {
	var title string
	err := a.editMapAt(path, "挂上"+what, func(f *model.File) error {
		n := f.Node(nodeID)
		if n == nil {
			return fmt.Errorf("主题已不存在")
		}
		title = n.Title
		for _, l := range n.Links {
			if l == link {
				return nil
			}
		}
		n.Links = append(append([]string(nil), n.Links...), link)
		return nil
	})
	if err == nil {
		a.tell(fmt.Sprintf("已挂到「%s › %s」", mapName(path), title))
	}
	return err
}

// canvasDrop is what a node takes dropped on it: a node, moved under it,
// or a map from the list, linked to it.
type canvasDrop interface{ droppedOnNode() }

func (nodeDrag) droppedOnNode() {}
func (fileDrag) droppedOnNode() {}

// dropMaterial links a map dropped on a node of the open map.
func (a *app) dropMaterial(id string, d fileDrag) {
	if d.Path == a.path {
		return // the map itself
	}
	link := a.mapLink(d.Path)
	if link == "" {
		return
	}
	if err := a.attachTo(a.path, id, link, "导图"); err != nil {
		a.saveErr = err.Error()
	}
}

// linksMark is the mark of a node's links: how many, when more than one.
func linksMark(links []string) string {
	if len(links) > 1 {
		return fmt.Sprintf("↗%d", len(links))
	}
	return "↗"
}

// titleURL finds the web addresses written in a title: they end at a
// space or at what is not ASCII, as Chinese written right after one.
var titleURL = regexp.MustCompile(`https?://[!-~]+`)

// titleLinks are where the web addresses written in a title are, as
// byte ranges, without the punctuation that ends a sentence after one.
func titleLinks(title string) [][2]int {
	if !strings.Contains(title, "://") {
		return nil
	}
	var at [][2]int
	for _, m := range titleURL.FindAllStringIndex(title, -1) {
		end := m[0] + len(strings.TrimRight(title[m[0]:m[1]], ".,;:!?'\")]}>"))
		if u, err := url.Parse(title[m[0]:end]); err != nil || u.Host == "" {
			continue
		}
		at = append(at, [2]int{m[0], end})
	}
	return at
}

// nodeLinks are a node's links and, after them, the web addresses
// written in its title, which open as links do without being added.
func nodeLinks(n *model.Node) []string {
	at := titleLinks(n.Title)
	if len(at) == 0 {
		return n.Links
	}
	links := slices.Clone(n.Links)
	for _, r := range at {
		if u := n.Title[r[0]:r[1]]; !slices.Contains(links, u) {
			links = append(links, u)
		}
	}
	return links
}

// dropOnMap links a map dropped on a map of the list to that map's
// center.
func (a *app) dropOnMap(path string, d fileDrag) {
	f := a.mapFile(path)
	if f == nil || len(f.Sheets) == 0 {
		return
	}
	link := a.mapLink(d.Path)
	if link == "" {
		return
	}
	if err := a.attachTo(path, f.Sheets[0].Root.ID, link, "导图"); err != nil {
		// Said where it is seen: in 收集's line of errors, it showed on
		// 收集's page alone, and a map is dropped on a map elsewhere.
		a.tell("没有链接上：" + err.Error())
	}
}

// mapMaterialSection is the canvas panel's account of the map's material:
// the clips and maps it links, the maps linking to it, and links to what
// is no more, which a click takes off.
func (a *app) mapMaterialSection(c *ui.Context) {
	if a.file == nil {
		return
	}
	t := c.Theme()
	clips, maps := map[string]bool{}, map[string]bool{}
	var gone []string
	a.file.Walk(func(n *model.Node) {
		for _, l := range n.Links {
			kind, target, _ := a.parseLink(l)
			switch kind {
			case linkClip:
				if a.lib.Find(target) == nil {
					gone = append(gone, l)
				} else {
					clips[target] = true
				}
			case linkNode:
				path, id, _ := strings.Cut(target, "#")
				if f := a.mapFile(path); f == nil || id != "" && f.SheetOf(id) == nil {
					gone = append(gone, l)
				} else if path != a.path {
					maps[path] = true
				}
			}
		}
	})
	from := map[string]bool{}
	var thoughts []backlink // the thoughts and clips that link to it or to a topic of it
	for _, b := range a.refIndex().to["map:"+a.path] {
		if b.clip != "" {
			if !slices.ContainsFunc(thoughts, func(x backlink) bool { return x.clip == b.clip }) {
				thoughts = append(thoughts, b)
			}
			continue
		}
		from[b.path] = true
	}
	if len(clips)+len(maps)+len(from)+len(gone)+len(thoughts) == 0 {
		return
	}
	section(c, iconLibrary, "本图资料", nil, func() {
		ui.Column(c).Gap(6).Children(func() {
			line := func(text string) { ui.Text(c, text).FontSize(12).Wrap() }
			if len(clips) > 0 {
				line(fmt.Sprintf("挂着 %d 条资料", len(clips)))
			}
			if len(maps) > 0 {
				line(fmt.Sprintf("链接了 %d 张别的导图", len(maps)))
			}
			if len(thoughts) > 0 {
				line(fmt.Sprintf("被 %d 条灵感或资料提到", len(thoughts)))
				for _, b := range thoughts {
					row := ui.Row(c.Key("thought:" + b.clip)).Gap(6).Cursor(ui.CursorPointer).Label("前往 " + b.title)
					if row.Clicked() {
						a.later(func() { a.showClip(b.clip) })
					}
					row.Children(func() {
						ui.Text(c, "↩").FontSize(12).TextColor(c.Theme().TextMuted)
						ui.Text(c, b.title).FontSize(12).TextColor(c.Theme().Accent).MaxLines(1)
					})
				}
			}
			for path := range from {
				path := path
				row := ui.Row(c.Key("from:" + path)).Gap(6).Cursor(ui.CursorPointer).Label("打开 " + mapName(path))
				if row.Clicked() {
					a.later(func() { _ = a.open(path) })
				}
				row.Children(func() {
					ui.Text(c, "↩ 被「"+mapName(path)+"」引用").FontSize(12).TextColor(t.Accent).MaxLines(1)
				})
			}
			if len(gone) > 0 {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, fmt.Sprintf("%d 个链接指向已不存在的资料或主题", len(gone))).FontSize(12).TextColor(t.Danger).Grow(1).Wrap()
					if button(c, "清理").FontSize(11).Tooltip("从主题上去掉这些失效的链接（可撤销）").Clicked() {
						dead := map[string]bool{}
						for _, l := range gone {
							dead[l] = true
						}
						a.later(func() {
							a.mutate("", func() {
								a.file.Walk(func(n *model.Node) {
									var keep []string
									for _, l := range n.Links {
										if !dead[l] {
											keep = append(keep, l)
										}
									}
									n.Links = keep
								})
							})
						})
					}
				})
			}
		})
	})
}
