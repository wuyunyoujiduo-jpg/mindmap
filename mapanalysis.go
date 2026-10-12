package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// Maps are analyzed by the AI as clips are organized: a line on what the
// map is, a summary, a category, tags, keywords and synonyms, which the
// search finds it by. An analysis is kept beside the library, in
// .library/maps/, one file a map, not in the map: writing it touches no
// map, its history or what is being edited. It is keyed by the map's path
// relative to the maps folder, and follows the map when it is renamed in
// the app; one renamed elsewhere takes it back by its content's hash.

// mapMetaDir is where the maps' analyses are kept.
func (a *app) mapMetaDir() string { return filepath.Join(a.dir, ".library", "maps") }

// mapMeta is the file of a map's analysis.
type mapMeta struct {
	Path string `json:"path"` // of the map, relative to the maps folder, with slashes
	model.Analysis
}

// mapKey is a map's path relative to the maps folder, with slashes.
func (a *app) mapKey(path string) string {
	rel, err := filepath.Rel(a.dir, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func mapMetaName(key string) string {
	h := sha1.Sum([]byte(key))
	return hex.EncodeToString(h[:8]) + ".json"
}

// loadMapMetas reads the analyses again when their folder changed: when
// one was written, or the sync brought some.
func (a *app) loadMapMetas() {
	dir := a.mapMetaDir()
	mt := modTime(dir)
	if a.mapMetas != nil && mt.Equal(a.mapMetaMod) && a.mapMetaFor == a.dir {
		return
	}
	a.mapMetaMod, a.mapMetaFor = mt, a.dir
	a.mapMetas = map[string]*model.Analysis{}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var m mapMeta
		if json.Unmarshal(data, &m) == nil && m.Path != "" {
			an := m.Analysis
			a.mapMetas[m.Path] = &an
		}
	}
	a.mapMetaVersion++
}

// analysisOf is the analysis of a map, nil when there is none.
func (a *app) analysisOf(path string) *model.Analysis {
	a.loadMapMetas()
	return a.mapMetas[a.mapKey(path)]
}

// writeMapMeta keeps the analysis of a map, by its key.
func (a *app) writeMapMeta(key string, an *model.Analysis) error {
	dir := a.mapMetaDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(mapMeta{Path: key, Analysis: *an}, "", "  ")
	if err != nil {
		return err
	}
	file := filepath.Join(dir, mapMetaName(key))
	if err := os.WriteFile(file+".tmp", data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(file+".tmp", file); err != nil {
		return err
	}
	a.loadMapMetas()
	a.mapMetas[key] = an
	a.mapMetaVersion++
	a.syncChange()
	return nil
}

// removeMapMeta drops the analysis of a map deleted.
func (a *app) removeMapMeta(path string) {
	key := a.mapKey(path)
	if err := os.Remove(filepath.Join(a.mapMetaDir(), mapMetaName(key))); err == nil {
		a.loadMapMetas()
		delete(a.mapMetas, key)
		a.mapMetaVersion++
		a.syncChange()
	}
}

// moveMapMetas has the analyses follow the maps moved: follow gives a
// path's new place.
func (a *app) moveMapMetas(follow func(string) string) {
	a.loadMapMetas()
	type move struct {
		from, to string
		an       *model.Analysis
	}
	var moves []move
	for key, an := range a.mapMetas {
		from := filepath.Join(a.dir, filepath.FromSlash(key))
		if to := follow(from); to != from {
			moves = append(moves, move{key, a.mapKey(to), an})
		}
	}
	for _, m := range moves {
		if a.writeMapMeta(m.to, m.an) == nil {
			_ = os.Remove(filepath.Join(a.mapMetaDir(), mapMetaName(m.from)))
			delete(a.mapMetas, m.from)
		}
	}
}

// adoptMapMeta gives a map without an analysis the one left by a map that
// is no more, whose content was the same: the map renamed or moved in
// Finder.
func (a *app) adoptMapMeta(path string, f *model.File) *model.Analysis {
	a.loadMapMetas()
	key := a.mapKey(path)
	if an := a.mapMetas[key]; an != nil {
		return an
	}
	hash := f.ContentHash()
	for k, an := range a.mapMetas {
		if an.Hash != hash {
			continue
		}
		if _, err := os.Stat(filepath.Join(a.dir, filepath.FromSlash(k))); err == nil {
			continue // still there: a copy, which keeps its own
		}
		if a.writeMapMeta(key, an) == nil {
			_ = os.Remove(filepath.Join(a.mapMetaDir(), mapMetaName(k)))
			delete(a.mapMetas, k)
		}
		return an
	}
	return nil
}

// mapFile is a map's content: the open map's as edited, or the file's.
func (a *app) mapFile(path string) *model.File {
	if path == a.path && a.file != nil {
		return a.file
	}
	return a.indexed(path)
}

// mapAnalysisPrompt is what the AI is given of a map.
func (a *app) mapAnalysisPrompt(path string, f *model.File) string {
	done, total := 0, 0
	for _, d := range f.Sheets {
		dd, tt := d.Root.Progress()
		done, total = done+dd, total+tt
	}
	text := f.Outline(6000)
	if total > 0 {
		text = fmt.Sprintf("任务：%d 个，完成 %d 个\n\n", total, done) + text
	}
	clip := library.Clip{Kind: kindMap, Title: mapName(path), Text: text, Created: modTime(path)}
	return organizePromptFor(&clip, a.organizePrompt())
}

// kindMap marks a map among the clips the library's search finds.
const kindMap = library.KindMap

// analyzeMap asks the AI to analyze a map.
func (a *app) analyzeMap(path string) {
	if a.mapWorking != "" || providerKey(a.cfg) == "" {
		return
	}
	f := a.mapFile(path)
	if f == nil {
		return
	}
	key, hash := a.mapKey(path), f.ContentHash()
	prompt := a.mapAnalysisPrompt(path, f)
	a.mapWorking, a.mapErr = path, ""
	cfg := a.cfg
	send, sendDS := a.sendAI, a.sendDeepSeek
	a.async(func() (any, error) {
		return askOrganize(context.Background(), cfg, send, sendDS, prompt, nil, "")
	}, func(v any, err error) {
		auto := a.mapAuto
		a.mapWorking, a.mapAuto = "", false
		o, _ := v.(*clipOrganized)
		if err != nil || o == nil {
			if err == nil {
				err = plainError("AI 没有回答")
			}
			a.mapErr = "AI 分析失败：" + aiError(err)
			if auto { // no more by itself until the settings change
				a.mapAutoFailed = cfg.AIProvider + providerKey(cfg)
				a.mapErr = "自动分析失败，已暂停：" + aiError(err)
			}
			return
		}
		an := &model.Analysis{Desc: o.Desc, Summary: o.Summary, Category: o.Category, Tags: o.Tags,
			Keywords: o.Keywords, Synonyms: o.Synonyms, Analysis: o.Analysis, Hash: hash, Updated: a.now()}
		if err := a.writeMapMeta(key, an); err != nil {
			a.mapErr = "保存分析失败：" + err.Error()
		}
	})
}

// mapAnalysisSection shows the open map's analysis in the canvas panel,
// and analyzes it.
func (a *app) mapAnalysisSection(c *ui.Context) {
	t := c.Theme()
	an := a.adoptMapMeta(a.path, a.file)
	ui.Column(c.Key("map-analysis")).Gap(8).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "AI 分析").FontSize(12).Bold().TextColor(t.TextMuted).Grow(1)
			switch {
			case a.mapWorking == a.path:
				ui.Spinner(c)
				ui.Text(c, "分析中…").FontSize(12).TextColor(t.TextMuted)
			case a.mapWorking != "":
				ui.Spinner(c)
				ui.Text(c, "正在分析「"+mapName(a.mapWorking)+"」").FontSize(12).TextColor(t.TextMuted).NoWrap().MaxWidth(150).Ellipsis("…")
			case providerKey(a.cfg) != "":
				label := "分析导图"
				if an != nil {
					label = "重新分析"
				}
				if ui.Button(c, label).FontSize(12).Padding(2, 10).Disabled(a.mapWorking != "").
					Tooltip("让 AI 读这张导图，写一句话说明、摘要、分类、标签和检索用的关键词、同义词，方便以后搜索").Clicked() {
					a.analyzeMap(a.path)
				}
			}
		})
		if a.mapErr != "" {
			ui.Text(c, a.mapErr).FontSize(12).TextColor(t.Danger).Wrap()
		}
		if providerKey(a.cfg) != "" {
			auto := !a.cfg.MapAutoOff
			switchRow(c, &auto, "自动分析导图", "导图停止编辑 2 分钟后，AI 自动分析或更新；其他改过的导图也会依次分析", func() {
				a.cfg.MapAutoOff = !auto
				a.mapAutoFailed, a.mapErr = "", ""
				a.cfg.save()
			})
		}
		if an == nil {
			msg := "AI 分析这张导图后，搜索时也能按它讲的内容、同义词找到它；资料库和 AI 助手也能检索到。"
			if providerKey(a.cfg) == "" {
				msg += "先在 AI 助手的设置里填写 API Key。"
			}
			ui.Text(c, msg).FontSize(12).TextColor(t.TextMuted).Wrap()
			return
		}
		if an.Hash != a.file.ContentHash() {
			ui.Text(c, "导图改过了，分析是 "+an.Updated.Format("01-02 15:04")+" 的").FontSize(11).TextColor(t.Warning)
		}
		if an.Desc != "" {
			ui.Text(c, an.Desc).FontSize(13).Wrap().Selectable()
		}
		if an.Summary != "" {
			ui.Text(c, an.Summary).FontSize(12).TextColor(t.TextMuted).Wrap().Selectable()
		}
		if an.Category != "" || len(an.Tags) > 0 {
			ui.Row(c).Wrap().Gap(4).GapY(4).Children(func() {
				if an.Category != "" {
					ui.Box(c).Padding(1, 8).Radius(999).Background(t.Accent.Alpha(0.14)).Children(func() {
						ui.Text(c, an.Category).FontSize(11).TextColor(t.Accent)
					})
				}
				for _, tag := range an.Tags {
					ui.Text(c, "#"+tag).FontSize(11).TextColor(t.TextMuted)
				}
			})
		}
		if words := slices.Concat(an.Keywords, an.Synonyms); len(words) > 0 {
			ui.Text(c, "检索词："+strings.Join(words, "、")).FontSize(11).TextColor(t.TextMuted).Wrap()
		}
		if an.Analysis != "" {
			markdownView(c.Key("map-analysis-md"), an.Analysis, 12)
		}
	})
}

// mapClips are the maps as clips, for the library's search: their
// analysis, and their nodes' titles and notes as text. They are made
// again when a map or an analysis changes, and never kept as clips.
func (a *app) mapClips() []*library.Clip {
	a.loadMapMetas()
	sig := fmt.Sprint(a.dir, a.mapMetaVersion, len(a.files))
	for _, f := range a.files {
		sig += f.Path + f.Modified.String()
	}
	if a.path != "" && a.file != nil {
		sig += a.changed.String() // the open map, as edited
	}
	if sig == a.mapClipsSig {
		return a.mapClipsList
	}
	var clips []*library.Clip
	for _, fi := range a.files {
		f := a.mapFile(fi.Path)
		if f == nil {
			continue
		}
		c := &library.Clip{ID: "map:" + fi.Path, Kind: kindMap, Title: mapName(fi.Path), Created: fi.Modified, Updated: fi.Modified}
		var text strings.Builder
		for _, d := range f.Sheets {
			d.Walk(func(n, _ *model.Node, _ int) bool {
				if text.Len() < 60000 {
					text.WriteString(n.Title + "\n")
					if n.Note != "" {
						text.WriteString(n.Note + "\n")
					}
				}
				return true
			})
		}
		c.Text = text.String()
		if an := a.adoptMapMeta(fi.Path, f); an != nil {
			c.Desc, c.Summary, c.Category, c.Tags = an.Desc, an.Summary, an.Category, an.Tags
			c.Keywords, c.Synonyms, c.Analysis = an.Keywords, an.Synonyms, an.Analysis
		}
		clips = append(clips, c)
	}
	a.mapClipsSig, a.mapClipsList, a.mapIndex = sig, clips, nil
	return clips
}

// searchMaps finds maps by the words of their analysis and content.
func (a *app) searchMaps(query string, keep func(*library.Clip) bool) []library.Hit {
	clips := a.mapClips()
	if a.mapIndex == nil {
		a.mapIndex = library.NewIndex(clips)
	}
	return a.mapIndex.Search(query, keep)
}

// mapPathOf is the path of a map found as a clip.
func mapPathOf(c *library.Clip) string { return strings.TrimPrefix(c.ID, "map:") }

// mapRow is a map in the library's list: a click opens it.
func (a *app) mapRow(c *ui.Context, clip *library.Clip) {
	t := c.Theme()
	path := mapPathOf(clip)
	row := ui.Row(c).Padding(8, 10).Gap(10).Radius(8).Cursor(ui.CursorPointer).Label("导图 " + clip.Title).
		Tooltip("打开导图")
	if row.Hovered() {
		row.Background(t.SurfaceHover)
	}
	if row.Clicked() {
		a.later(func() {
			a.libOpen = false
			a.openMap(path)
		})
	}
	row.Children(func() {
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				glyph(c, iconMap, 14, t.Accent)
				ui.Text(c, clip.Title).FontSize(13).Bold().SingleLine().Shrink(1).MinWidth(0)
				ui.Text(c, "导图").FontSize(11).TextColor(t.TextMuted).NoWrap()
			})
			sub := cmpOr(clip.Desc, clip.Summary)
			if sub == "" {
				sub = "还没有 AI 分析 · " + a.relDir(path)
			}
			ui.Text(c, sub).FontSize(12).TextColor(t.TextMuted).SingleLine()
		})
		if clip.Category != "" {
			ui.Text(c, clip.Category).FontSize(11).TextColor(t.TextMuted).NoWrap().Shrink(0)
		}
	})
}

// searchMapsText answers search_maps: the n maps that match best, with
// their nodes that match.
func (a *app) searchMapsText(query string, n int) (string, int) {
	hits := a.searchMaps(query, nil)
	if strings.TrimSpace(query) == "" {
		hits = nil
	}
	if len(hits) > n {
		hits = hits[:n]
	}
	if len(hits) == 0 {
		return "没有找到相关的导图。", 0
	}
	words := strings.Fields(query)
	var b strings.Builder
	for _, h := range hits {
		c := h.Clip
		path := mapPathOf(c)
		fmt.Fprintf(&b, "- 导图「%s」（路径：%s）", c.Title, a.mapKey(path))
		if c.Category != "" {
			b.WriteString(" 分类：" + c.Category)
		}
		if len(c.Tags) > 0 {
			b.WriteString(" 标签：" + strings.Join(c.Tags, "、"))
		}
		b.WriteString("\n")
		if s := cmpOr(c.Desc, c.Summary); s != "" {
			b.WriteString("  " + s + "\n")
		}
		if f := a.mapFile(path); f != nil {
			var lines []string
			for _, d := range f.Sheets {
				d.Walk(func(nd *model.Node, _ *model.Node, _ int) bool {
					if len(lines) >= 5 {
						return false
					}
					for _, w := range words {
						if nd.Matches(w) {
							lines = append(lines, nd.Title)
							break
						}
					}
					return true
				})
			}
			if len(lines) > 0 {
				b.WriteString("  匹配的节点：" + strings.Join(lines, "；") + "\n")
			}
		}
	}
	return b.String(), len(hits)
}

// readMapText answers read_map: a map's outline, by its path relative to
// the maps folder.
func (a *app) readMapText(key string) (result, shown string) {
	key = strings.TrimSpace(filepath.ToSlash(key))
	for _, fi := range a.files {
		if a.mapKey(fi.Path) != key && fi.Path != key {
			continue
		}
		f := a.mapFile(fi.Path)
		if f == nil {
			break
		}
		text := "导图「" + mapName(fi.Path) + "」\n"
		if an := a.analysisOf(fi.Path); an != nil && an.Summary != "" {
			text += "AI 摘要：" + an.Summary + "\n"
		}
		return text + "\n" + f.Outline(12000), "📖 读取导图「" + mapName(fi.Path) + "」"
	}
	return "找不到这张导图：" + key, "没有找到导图 " + key
}

// Maps are analyzed by themselves, one at a time: the open map when it has
// been left unedited a while after a change, then the others whose
// content changed since their analysis. One that fails stops them until
// the provider or key changes.
const (
	mapAutoIdle  = 2 * time.Minute  // the open map unedited this long
	mapAutoEvery = 20 * time.Second // between two analyses
	mapAutoNodes = 3                // maps with fewer nodes are left
)

// autoAnalyzeMaps starts the analysis of a map due, when one is; tick
// calls it.
func (a *app) autoAnalyzeMaps() {
	key := a.cfg.AIProvider + providerKey(a.cfg)
	now := a.now()
	if a.cfg.MapAutoOff || providerKey(a.cfg) == "" || a.mapAutoFailed == key ||
		a.mapWorking != "" || a.libWorking != "" || now.Sub(a.mapAutoAt) < mapAutoEvery {
		return
	}
	if a.mapAutoSkip == nil {
		a.mapAutoSkip = map[string]time.Time{}
	}
	due := func(path string, f *model.File) bool {
		if f == nil || countNodes(f) < mapAutoNodes {
			return false
		}
		an := a.adoptMapMeta(path, f)
		return an == nil || an.Hash != f.ContentHash()
	}
	// The open map, as edited, once it is left a while.
	if a.file != nil && a.path != "" && !a.dirty && !a.conflict && now.Sub(a.changed) >= mapAutoIdle &&
		!a.mapAutoSkip[a.path].Equal(a.changed) {
		a.mapAutoSkip[a.path] = a.changed
		if due(a.path, a.file) {
			a.startAutoAnalysis(a.path)
			return
		}
	}
	// The others, each looked at again when its file changes.
	for _, fi := range a.files {
		if fi.Path == a.path || a.mapAutoSkip[fi.Path].Equal(fi.Modified) {
			continue
		}
		a.mapAutoSkip[fi.Path] = fi.Modified
		if an := a.analysisOf(fi.Path); an != nil && an.Updated.After(fi.Modified) {
			continue // analyzed since it was last written
		}
		if due(fi.Path, a.indexed(fi.Path)) {
			a.startAutoAnalysis(fi.Path)
			return
		}
	}
}

func (a *app) startAutoAnalysis(path string) {
	a.mapAutoAt, a.mapAuto = a.now(), true
	a.analyzeMap(path)
	if a.mapWorking == "" { // not started
		a.mapAuto = false
	}
}

// countNodes counts the nodes of a map, at most up to 100.
func countNodes(f *model.File) int {
	n := 0
	for _, d := range f.Sheets {
		d.Walk(func(*model.Node, *model.Node, int) bool {
			n++
			return n < 100
		})
	}
	return n
}
