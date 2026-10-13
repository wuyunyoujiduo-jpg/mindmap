package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// Where material belongs, and what material a node is about, are worked
// out on the device, never by an extra call to the AI, and only for what
// is shown; nothing is linked without a click.

// place is a node material may go under.
type place struct {
	path, sheet, id string
	title           string // "map › parent › node"
	score           float64
}

// nodeIndex is a map's nodes as clips, to search them by their words:
// each by its title, its note and the titles above it.
type nodeIndex struct {
	sig   string
	index *library.Index
	where map[string]place
}

// nodeIndexOf indexes a map's nodes, again as the map changes.
func (a *app) nodeIndexOf(path string) *nodeIndex {
	f := a.mapFile(path)
	if f == nil {
		return nil
	}
	sig := modTime(path).String()
	if path == a.path {
		sig += a.changed.String() + fmt.Sprint(a.edits)
	}
	if ix := a.nodeIdx[path]; ix != nil && ix.sig == sig {
		return ix
	}
	ix := &nodeIndex{sig: sig, where: map[string]place{}}
	var clips []*library.Clip
	for _, d := range f.Sheets {
		var walk func(n *model.Node, above []string)
		walk = func(n *model.Node, above []string) {
			// The map's name stands for its center: "map › branch › node".
			parts := []string{mapName(path)}
			if len(above) > 0 {
				parts = append(append(parts, above[1:]...), n.Title)
			}
			title := strings.Join(parts, " › ")
			ix.where[n.ID] = place{path: path, sheet: d.ID, id: n.ID, title: title}
			note := n.Note
			if r := []rune(note); len(r) > 400 {
				note = string(r[:400])
			}
			clips = append(clips, &library.Clip{ID: n.ID, Title: n.Title, Text: strings.Join(above, " ") + "\n" + note,
				Tags: n.Tags})
			for _, c := range n.Children {
				walk(c, append(slices.Clip(above), n.Title))
			}
		}
		walk(d.Root, nil)
	}
	ix.index = library.NewIndex(clips)
	if a.nodeIdx == nil {
		a.nodeIdx = map[string]*nodeIndex{}
	}
	a.nodeIdx[path] = ix
	return ix
}

// clipQuery is what a clip is about, in words.
func clipQuery(c *library.Clip) string {
	return strings.Join([]string{c.Label(), c.Desc, c.Category, strings.Join(c.Tags, " "), strings.Join(c.Keywords, " ")}, " ")
}

// placesFor are the nodes a clip may go under, the likeliest first: where
// clips like it are linked, else the nodes of the maps most about it.
func (a *app) placesFor(c *library.Clip, n int) []place {
	seen := map[string]bool{}
	for _, b := range a.clipBacklinks(c.ID) {
		seen[b.path+"#"+b.id] = true // where it is already
	}
	var out []place
	add := func(p place) {
		if seen[p.path+"#"+p.id] || len(out) >= n {
			return
		}
		seen[p.path+"#"+p.id] = true
		out = append(out, p)
	}
	// Anchors: where the clips most like it are linked.
	for _, h := range a.lib.Related(c, a.embedder().Name(), 6) {
		for _, b := range a.clipBacklinks(h.Clip.ID) {
			if ix := a.nodeIndexOf(b.path); ix != nil {
				if p, ok := ix.where[b.id]; ok {
					p.score = h.Score
					add(p)
				}
			}
		}
	}
	// Else: the maps most about it, and their nodes most about it.
	q := clipQuery(c)
	for i, mh := range a.searchMaps(q, nil) {
		if i == 3 || mh.Coverage < 0.2 {
			break
		}
		ix := a.nodeIndexOf(strings.TrimPrefix(mh.Clip.ID, "map:"))
		if ix == nil {
			continue
		}
		for j, h := range ix.index.Search(q, nil) {
			if j == 2 || h.Coverage < 0.25 {
				break
			}
			p := ix.where[h.Clip.ID]
			p.score = h.Score
			add(p)
		}
	}
	return out
}

// relatedOf is the material a node may be about, not linked to it yet:
// clips, by words and meaning, and nodes of other maps.
func (a *app) relatedOf(n *model.Node) (clips []library.Hit, nodes []place) {
	if strings.TrimSpace(n.Title) == "" {
		return nil, nil
	}
	linked := map[string]bool{}
	for _, l := range n.Links {
		linked[l] = true
	}
	q := strings.TrimSpace(n.Title + " " + strings.Join(n.Tags, " "))
	keep := func(c *library.Clip) bool { return !linked[clipScheme+c.ID] }
	for _, h := range a.lib.SearchBoth(q, a.queryVector(q), a.embedder().Name(), keep) {
		if h.Coverage >= 0.5 || h.Coverage == 0 && h.Score > 0 { // by meaning
			clips = append(clips, h)
		}
		if len(clips) == 4 {
			break
		}
	}
	for i, mh := range a.searchMaps(q, nil) {
		path := strings.TrimPrefix(mh.Clip.ID, "map:")
		if i == 3 || mh.Coverage < 0.3 {
			break
		}
		if path == a.path {
			continue
		}
		ix := a.nodeIndexOf(path)
		if ix == nil {
			continue
		}
		for _, h := range ix.index.Search(q, nil) {
			p := ix.where[h.Clip.ID]
			if h.Coverage < 0.5 || linked[a.nodeLinkTo(p.path, p.id)] || len(nodes) == 2 {
				break
			}
			p.score = h.Score
			nodes = append(nodes, p)
		}
	}
	return clips, nodes
}

// queryVector is a query's vector, when worked out; else it is asked for,
// once, and the panel shows the words' results meanwhile.
func (a *app) queryVector(q string) []float32 {
	q = strings.TrimSpace(q)
	if v, ok := a.qvecs[q]; ok || q == "" || !a.embedder().On() {
		return v
	}
	if a.qpending == nil {
		a.qpending = map[string]bool{}
	}
	if !a.qpending[q] {
		a.qpending[q] = true
		a.embedQueries([]string{q}, func() { delete(a.qpending, q) })
	}
	return nil
}
