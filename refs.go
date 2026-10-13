package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"path/filepath"
	"strings"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/versions"
)

// Material and maps are related one way only: a node's links, to a clip
// ("clip:id"), a node of any map ("mindmap:map#id"), or a whole map (its
// first canvas's root). Where a clip is used, and what links to a node or
// a map, are worked out from the maps, in one index of every link.

// refs is the index of the links of every map.
type refs struct {
	sig string
	// to lists the nodes linking to a target: "clip:id", "node:path#id",
	// and "map:path" for any node of a map, from other maps.
	to map[string][]backlink
	// nodeAt is the map of each node, by its ID, to find a node linked to
	// after its map was renamed or moved.
	nodeAt map[string]string
}

// refIndex is the index of the links, made again as the maps change.
func (a *app) refIndex() *refs {
	a.mapClips() // brings mapClipsSig up to date
	if a.refs != nil && a.refs.sig == a.mapClipsSig {
		return a.refs
	}
	r := &refs{sig: a.mapClipsSig, to: map[string][]backlink{}, nodeAt: map[string]string{}}
	for _, fi := range a.files {
		f := a.mapFile(fi.Path)
		if f == nil {
			continue
		}
		for _, d := range f.Sheets {
			walk := func(n *model.Node) {
				var walk func(n *model.Node)
				walk = func(n *model.Node) {
					r.nodeAt[n.ID] = fi.Path
					for _, l := range n.Links {
						a.addRef(r, fi.Path, d.ID, n, l)
					}
					for _, c := range n.Children {
						walk(c)
					}
				}
				walk(n)
			}
			walk(d.Root)
			for _, fl := range d.Floating {
				walk(fl)
			}
		}
	}
	a.refs = r
	return r
}

func (a *app) addRef(r *refs, path, sheet string, n *model.Node, link string) {
	b := backlink{path, sheet, n.ID, n.Title}
	if id, ok := strings.CutPrefix(link, clipScheme); ok {
		r.to[clipScheme+id] = append(r.to[clipScheme+id], b)
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
	a.syncChange()
	a.refreshFiles()
	return nil
}

// attachTo links a node of any map to something: a clip, a map or a node.
func (a *app) attachTo(path, nodeID, link, what string) error {
	var title string
	err := a.editMapAt(path, "挂上"+what, func(f *model.File) error {
		n := f.Node(nodeID)
		if n == nil {
			return fmt.Errorf("节点已不存在")
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
// or a document or a map from the sidebar, linked to it.
type canvasDrop interface{ droppedOnNode() }

func (nodeDrag) droppedOnNode() {}
func (fileDrag) droppedOnNode() {}

// dropMaterial links a document or a map dropped on a node of the open map.
func (a *app) dropMaterial(id string, d fileDrag) {
	link, what := clipScheme+d.Clip, "资料"
	if d.Clip == "" {
		if d.Path == a.path {
			return // the map itself
		}
		link, what = a.mapLink(d.Path), "导图"
	}
	if link == "" {
		return
	}
	if err := a.attachTo(a.path, id, link, what); err != nil {
		a.saveErr = err.Error()
	}
}

// linksMark is the mark of a node's links: how many, when more than one.
func linksMark(n *model.Node) string {
	if len(n.Links) > 1 {
		return fmt.Sprintf("↗%d", len(n.Links))
	}
	return "↗"
}

// dropOnMap links a clip, or a map, dropped on a map of the sidebar to the
// map's center.
func (a *app) dropOnMap(path string, d fileDrag) {
	f := a.mapFile(path)
	if f == nil || len(f.Sheets) == 0 {
		return
	}
	link, what := clipScheme+d.Clip, "资料"
	if d.Clip == "" {
		link, what = a.mapLink(d.Path), "导图"
	}
	if link == "" {
		return
	}
	if err := a.attachTo(path, f.Sheets[0].Root.ID, link, what); err != nil {
		a.libErr = err.Error()
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
	for _, b := range a.refIndex().to["map:"+a.path] {
		from[b.path] = true
	}
	if len(clips)+len(maps)+len(from)+len(gone) == 0 {
		return
	}
	sectionCard(c, iconLibrary, tileOrange, "本图资料", nil, func() {
		ui.Column(c).Gap(6).Children(func() {
			line := func(text string) { ui.Text(c, text).FontSize(12).Wrap() }
			if len(clips) > 0 {
				line(fmt.Sprintf("挂着 %d 条资料", len(clips)))
			}
			if len(maps) > 0 {
				line(fmt.Sprintf("链接了 %d 张别的导图", len(maps)))
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
					ui.Text(c, fmt.Sprintf("%d 个链接指向已不存在的资料或节点", len(gone))).FontSize(12).TextColor(t.Danger).Grow(1).Wrap()
					if ui.Button(c, "清理").FontSize(11).Tooltip("从节点上去掉这些失效的链接（可撤销）").Clicked() {
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
