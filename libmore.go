package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/embed"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// Semantic search of the library, clips related to a clip or a node,
// reviews of what was kept, and links from nodes to clips.

// embedFunc makes vectors of texts; tests replace it.
type embedFunc func(ctx context.Context, e library.Embedder, texts []string) ([][]float32, error)

func embedWith(ctx context.Context, e library.Embedder, texts []string) ([][]float32, error) {
	return e.Embed(ctx, texts)
}

func (a *app) embedder() library.Embedder {
	if a.cfg.NoEmbed {
		return library.Embedder{}
	}
	return library.Embedder{URL: a.cfg.EmbedURL, Key: a.cfg.EmbedKey, Model: a.cfg.EmbedModel, Local: a.localModel()}
}

// embedQueries makes the vectors of queries not made yet, then calls done;
// at once when semantic search is off.
func (a *app) embedQueries(queries []string, done func()) {
	e := a.embedder()
	var todo []string
	for _, q := range queries {
		if _, ok := a.qvecs[q]; !ok && q != "" && e.On() {
			todo = append(todo, q)
		}
	}
	if len(todo) == 0 {
		done()
		return
	}
	embed := a.embed
	a.async(func() (any, error) {
		return embed(context.Background(), e, todo)
	}, func(v any, err error) {
		if vecs, ok := v.([][]float32); ok && err == nil {
			for i, q := range todo {
				a.qvecs[q] = vecs[i]
			}
		} else if err != nil {
			a.embedErr = err.Error()
		}
		done()
	})
}

// embedLibrary makes the vectors of the clips that have none or changed,
// a batch at a time, in the background.
func (a *app) embedLibrary() {
	e := a.embedder()
	if !e.On() || a.embedBusy || a.lib == nil || a.embedErr != "" {
		return
	}
	ids, texts := a.lib.Unembedded(e.Name())
	if len(ids) == 0 {
		return
	}
	const batch = 16
	if len(ids) > batch {
		ids, texts = ids[:batch], texts[:batch]
	}
	a.embedBusy = true
	embed := a.embed
	lib := a.lib
	a.async(func() (any, error) {
		return embed(context.Background(), e, texts)
	}, func(v any, err error) {
		a.embedBusy = false
		if err != nil {
			a.embedErr = err.Error()
			return
		}
		if lib != a.lib {
			return // another folder since
		}
		if err := lib.SetVectors(e.Name(), ids, texts, v.([][]float32)); err != nil {
			a.embedErr = err.Error()
			return
		}
		a.noteLibraryWrite()
		a.embedLibrary()
	})
}

// searchQueryVector asks for the vector of the library's search once the
// typing has paused.
func (a *app) searchQueryVector(c *ui.Context) {
	q := strings.TrimSpace(a.libQuery)
	if q == "" || !a.embedder().On() || a.qPending[q] {
		return
	}
	if _, ok := a.qvecs[q]; ok {
		return
	}
	if wait := 400*time.Millisecond - a.now().Sub(a.libQueryAt); wait > 0 {
		c.After(wait)
		return
	}
	a.qPending[q] = true
	a.embedQueries([]string{q}, func() { delete(a.qPending, q) })
}

// relatedSection lists the clips like the one shown.
func (a *app) relatedSection(c *ui.Context, clip *library.Clip) {
	hits := a.lib.Related(clip, a.embedder().Name(), 5)
	if len(hits) == 0 {
		return
	}
	ui.Field(c, "相关资料", func() {
		ui.Column(c).Gap(4).Children(func() {
			for _, h := range hits {
				a.clipLink(c.Key("rel"+h.Clip.ID), h.Clip)
			}
		})
	})
}

// clipLink is a clip's line that shows it in the library when clicked.
func (a *app) clipLink(c *ui.Context, clip *library.Clip) {
	t := c.Theme()
	row := ui.Row(c).Gap(6).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label("打开资料 " + clip.Label())
	if row.Clicked() {
		id := clip.ID
		a.later(func() { a.showClip(id) })
	}
	row.Children(func() {
		ui.Text(c, kindIcon(clip.Kind)).FontSize(12)
		ui.Text(c, clip.Label()).FontSize(12).TextColor(t.Accent).NoWrap().Ellipsis("…").Shrink(1).MinWidth(0)
		if clip.Desc != "" {
			ui.Text(c, clip.Desc).FontSize(11).TextColor(t.TextMuted).NoWrap().Ellipsis("…").Shrink(2).MinWidth(0)
		}
	})
}

// showClip shows a clip in the library.
func (a *app) showClip(id string) {
	if a.lib.Find(id) == nil {
		a.saveErr = "链接的资料已不存在"
		return
	}
	a.showLibrary()
	a.libSel, a.libQuery, a.libCat, a.libTag, a.libKind = id, "", "", "", 0
}

// nodeClips are the clips of the library that the node selected may be
// about: by its title and tags.
func (a *app) nodeClips(n *model.Node) []library.Hit {
	if a.lib == nil || len(a.lib.Clips) == 0 || strings.TrimSpace(n.Title) == "" {
		return nil
	}
	linked := map[string]bool{}
	for _, l := range n.Links {
		if id, ok := strings.CutPrefix(l, clipScheme); ok {
			linked[id] = true
		}
	}
	q := n.Title + " " + strings.Join(n.Tags, " ")
	var out []library.Hit
	for _, h := range a.lib.Search(q, func(c *library.Clip) bool { return !linked[c.ID] }) {
		if h.Coverage >= 0.5 {
			out = append(out, h)
		}
		if len(out) == 3 {
			break
		}
	}
	return out
}

// nodeClipsSection is the part of the node panel with the clips linked
// to the node and those it may be about.
func (a *app) nodeClipsSection(c *ui.Context, n *model.Node) {
	hits := a.nodeClips(n)
	if len(hits) == 0 {
		return
	}
	t := c.Theme()
	ui.Field(c, "资料库里可能相关", func() {
		ui.Column(c).Gap(4).Children(func() {
			for _, h := range hits {
				clip := h.Clip
				ui.Row(c.Key("nc" + clip.ID)).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Grow(1).Shrink(1).MinWidth(0).Children(func() { a.clipLink(c, clip) })
					if ui.Link(c, "链接", "").FontSize(11).Tooltip("把这条资料链接到节点").Clicked() {
						id := n.ID
						a.later(func() {
							if node := a.file.Node(id); node != nil {
								a.addLink(node, clipScheme+clip.ID)
							}
						})
					}
				})
			}
			ui.Text(c, "按标题和标签在资料库里找到的").FontSize(11).TextColor(t.TextMuted)
		})
	})
}

// clipScheme starts links to clips: "clip:id".
const clipScheme = "clip:"

// clipBacklinks are the nodes of every map that link to a clip.
func (a *app) clipBacklinks(id string) []backlink {
	var out []backlink
	for _, f := range a.files {
		file := a.file
		if f.Path != a.path {
			file = a.indexed(f.Path)
		}
		if file == nil {
			continue
		}
		for _, d := range file.Sheets {
			d.Walk(func(n, _ *model.Node, _ int) bool {
				for _, l := range n.Links {
					if l == clipScheme+id {
						out = append(out, backlink{f.Path, d.ID, n.ID, n.Title})
						break
					}
				}
				return true
			})
		}
	}
	return out
}

// clipBacklinksSection lists the nodes that link to a clip.
func (a *app) clipBacklinksSection(c *ui.Context, clip *library.Clip) {
	back := a.clipBacklinks(clip.ID)
	if len(back) == 0 {
		return
	}
	t := c.Theme()
	ui.Field(c, fmt.Sprintf("被 %d 个节点引用", len(back)), func() {
		ui.Column(c).Gap(4).Children(func() {
			for i, b := range back {
				row := ui.Row(c.Key(fmt.Sprint("cb", i))).Gap(6).Cursor(ui.CursorPointer).Label("前往 " + b.title)
				if row.Clicked() {
					b := b
					a.later(func() { a.goToNode(b.path, b.sheet, b.id) })
				}
				row.Children(func() {
					ui.Text(c, "↩").FontSize(12).TextColor(t.TextMuted)
					label := b.title
					if b.path != a.path {
						label = mapName(b.path) + " › " + label
					}
					ui.Text(c, label).FontSize(12).TextColor(t.Accent).MaxLines(1)
				})
			}
		})
	})
}

// recentClips are the clips kept in the last days, newest first.
func (a *app) recentClips(days int) []*library.Clip {
	since := a.now().AddDate(0, 0, -days)
	var out []*library.Clip
	for _, c := range a.lib.Clips {
		if c.Created.After(since) {
			out = append(out, c)
		}
	}
	return out
}

// aiReview asks the AI to sum up what was kept in the last days.
func (a *app) aiReview(days int) {
	clips := a.recentClips(days)
	if len(clips) == 0 {
		a.libErr = fmt.Sprintf("最近 %d 天没有收藏资料。", days)
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "最近 %d 天收藏的 %d 条资料：\n", days, len(clips))
	for i, c := range clips {
		if i == 80 {
			fmt.Fprintf(&b, "…还有 %d 条\n", len(clips)-i)
			break
		}
		b.WriteString("\n" + library.Brief(c, ""))
	}
	prompt := fmt.Sprintf("回顾我最近 %d 天收藏的资料（见下文）：按主题归纳成几组，每组说清讲了什么、有什么值得用的结论或行动，写出依据的资料标题；最后列出最值得深入看的 3 条和可以跟进的事。用 Markdown，简洁。", days)
	a.showLibrary()
	a.aiOpen = true
	a.ai.draft = prompt
	a.ai.extra = b.String()
	a.aiSend()
}

// clipsToMap makes a map of clips, a branch a category, and opens it.
func (a *app) clipsToMap(name string, clips []*library.Clip) {
	if len(clips) == 0 {
		a.libErr = "没有资料可以生成导图。"
		return
	}
	f := model.NewFile(name)
	d := f.Sheets[0]
	d.Root.Note = fmt.Sprintf("由资料库的 %d 条资料生成，%s。", len(clips), a.now().Format("2006-01-02"))
	branches := map[string]*model.Node{}
	for _, c := range clips {
		cat := c.Category
		if cat == "" {
			cat = "未分类"
		}
		b := branches[cat]
		if b == nil {
			b, _ = d.AddChild(d.Root.ID, cat)
			branches[cat] = b
		}
		n, _ := d.AddChild(b.ID, c.Label())
		var note []string
		for _, s := range []string{c.Desc, c.Summary, c.Note} {
			if s = strings.TrimSpace(s); s != "" {
				note = append(note, s)
			}
		}
		n.Note = strings.Join(note, "\n\n")
		n.Tags = append([]string(nil), c.Tags...)
		n.Links = []string{clipScheme + c.ID}
		if c.URL != "" {
			n.Links = append(n.Links, c.URL)
		}
	}
	path := freeMapPath(a.dir, name)
	if err := f.Save(path); err != nil {
		a.libErr = err.Error()
		return
	}
	a.refreshFiles()
	_ = a.open(path)
}

// reviewMenu is the library's menu of reviews.
func (a *app) reviewMenu(m *ui.Menu) {
	noKey := providerKey(a.cfg) == ""
	if m.Item("AI 回顾最近 7 天").Disabled(noKey).Chosen() {
		a.aiReview(7)
	}
	if m.Item("AI 回顾最近 30 天").Disabled(noKey).Chosen() {
		a.aiReview(30)
	}
	m.Separator()
	if m.Item("把最近 7 天的资料生成导图").Chosen() {
		a.later(func() { a.clipsToMap("资料回顾 "+a.now().Format("2006-01-02"), a.recentClips(7)) })
	}
	if m.Item("把当前列表生成导图").Chosen() {
		var clips []*library.Clip
		for _, h := range a.libHits() {
			clips = append(clips, h.Clip)
		}
		name := "资料整理 " + a.now().Format("2006-01-02")
		if a.libCat != "" && a.libCat != "-" && a.libCat != "★" {
			name = "资料 · " + a.libCat
		}
		a.later(func() { a.clipsToMap(name, clips) })
	}
}

// embedPresets are embedding models the settings fill in.
var embedPresets = []struct{ name, url, model, note string }{
	{"Ollama（本机，免费离线）", "http://localhost:11434/v1", "bge-m3", "先安装 Ollama（ollama.com），在终端运行 ollama pull bge-m3"},
	{"硅基流动", "https://api.siliconflow.cn/v1", "BAAI/bge-m3", "在 cloud.siliconflow.cn 注册并创建 API Key，bge-m3 免费"},
	{"OpenAI", "https://api.openai.com/v1", "text-embedding-3-small", "需要 OpenAI 的 API Key"},
}

// embedSettings is the settings' part on the embedding model of the
// library's semantic search.
func (a *app) embedSettings(c *ui.Context) {
	t := c.Theme()
	d := &a.embedDrafts
	ui.Field(c, "资料库语义检索", func() {
		ui.Column(c).Gap(8).Children(func() {
			on := !a.cfg.NoEmbed
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				if ui.Switch(c, &on).Label("语义检索").Changed() {
					a.cfg.NoEmbed = !on
					a.cfg.save()
					a.embedErr, a.qvecs = "", map[string][]float32{}
					a.embedLibrary()
				}
				ui.Text(c, "按意思检索资料（如搜「数据库变慢」能找到「SQL 优化」），并推荐相关资料").FontSize(13).Wrap().Grow(1)
			})
			if a.cfg.NoEmbed {
				return
			}
			switch e := a.embedder(); {
			case e.Builtin():
				ui.Text(c, "✓ 正在使用内置模型：离线、免费、不上传任何内容。想要更准，可以在下面换成外部向量模型。").FontSize(12).TextColor(t.Success).Wrap()
			case a.cfg.EmbedURL != "":
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "正在使用外部向量模型 "+a.cfg.EmbedModel).FontSize(12).TextColor(t.TextMuted).Grow(1)
					if a.localModel() != nil && ui.Button(c, "改回内置模型").FontSize(12).Clicked() {
						a.cfg.EmbedURL, a.cfg.EmbedKey, a.cfg.EmbedModel = "", "", ""
						a.embedDrafts = [3]string{}
						a.cfg.save()
						a.embedErr, a.embedTest, a.qvecs = "", "", map[string][]float32{}
						a.embedLibrary()
					}
				})
			default:
				ui.Text(c, "这个版本没有内置模型，可以接一个外部向量模型。").FontSize(12).TextColor(t.Warning).Wrap()
			}
			ui.Text(c, "外部向量模型（可选，更准）：支持任何 OpenAI 兼容的 /embeddings 接口。").
				FontSize(12).TextColor(t.TextMuted).Wrap()
			ui.Row(c).Gap(6).Wrap().GapY(6).Children(func() {
				for _, p := range embedPresets {
					if ui.Button(c.Key(p.name), p.name).FontSize(12).Tooltip(p.note).Clicked() {
						d[0], d[2] = p.url, p.model
					}
				}
			})
			ui.TextInput(c.Key("embed-url"), &d[0]).Placeholder("接口地址，如 http://localhost:11434/v1").Label("向量接口地址")
			ui.TextInput(c.Key("embed-key"), &d[1]).Password().Placeholder("API Key（Ollama 不需要）").Label("向量 API Key")
			ui.TextInput(c.Key("embed-model"), &d[2]).Placeholder("模型，如 bge-m3").Label("向量模型")
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				changed := d[0] != a.cfg.EmbedURL || d[1] != a.cfg.EmbedKey || d[2] != a.cfg.EmbedModel
				if ui.PrimaryButton(c, "保存并测试").Disabled(!changed && a.embedErr == "").Clicked() {
					a.cfg.EmbedURL = strings.TrimRight(strings.TrimSpace(d[0]), "/")
					a.cfg.EmbedKey = strings.TrimSpace(d[1])
					a.cfg.EmbedModel = strings.TrimSpace(d[2])
					a.cfg.save()
					a.embedErr, a.embedTest = "", "测试中…"
					a.qvecs = map[string][]float32{}
					if !a.embedder().On() {
						a.embedTest = "未设置外部向量模型"
					} else {
						a.embedQueries([]string{"测试"}, func() {
							if a.embedErr != "" {
								a.embedTest = "✕ " + a.embedErr
								return
							}
							a.embedTest = "✓ 连接成功，正在为资料建立语义索引"
							a.embedLibrary()
						})
					}
				}
				if a.embedTest != "" {
					col := t.TextMuted
					if strings.HasPrefix(a.embedTest, "✕") {
						col = t.Danger
					}
					ui.Text(c, a.embedTest).FontSize(12).TextColor(col).Wrap().Grow(1)
				}
			})
		})
	})
}

// backupSettings is the settings' part on backups.
func (a *app) backupSettings(c *ui.Context) {
	t := c.Theme()
	ui.Field(c, "备份", func() {
		ui.Column(c).Gap(8).Children(func() {
			ui.Text(c, "把导图文件夹（含资料库、图片和附件）打包成 zip。建议把导图文件夹放在 iCloud 云盘里同步，再定期备份到另一个地方。").
				FontSize(12).TextColor(t.TextMuted).Wrap()
			ui.Row(c).Gap(8).Wrap().GapY(6).AlignItems(ui.Center).Children(func() {
				if ui.Button(c, "立即备份…").Disabled(a.backupBusy).Clicked() {
					a.backupNow()
				}
				label := "每天自动备份到…"
				if a.cfg.BackupDir != "" {
					label = "更换自动备份文件夹…"
				}
				if ui.Button(c, label).Clicked() {
					a.plat.PickFolder(func(dir string) {
						if dir != "" {
							a.cfg.BackupDir, a.cfg.BackupLast = dir, time.Time{}
							a.cfg.save()
							a.autoBackup()
						}
					})
				}
				if a.cfg.BackupDir != "" && ui.Button(c, "关闭自动备份").Clicked() {
					a.cfg.BackupDir = ""
					a.cfg.save()
				}
				if a.backupBusy {
					ui.Spinner(c)
				}
			})
			if a.cfg.BackupDir != "" {
				s := "每天第一次打开时备份到 " + a.cfg.BackupDir + "，保留最近 14 份"
				if !a.cfg.BackupLast.IsZero() {
					s += "；上次 " + a.cfg.BackupLast.Format("2006-01-02 15:04")
				}
				ui.Text(c, s).FontSize(12).TextColor(t.TextMuted).Wrap()
			}
			if a.backupErr != "" {
				ui.Text(c, "自动备份失败："+a.backupErr).FontSize(12).TextColor(t.Danger).Wrap()
			}
		})
	})
}

// localModelPath finds the built-in embedding model's file: among the
// app's resources, or where MINDMAP_EMBED_MODEL says. Tests replace it.
var localModelPath = func() string {
	if p := os.Getenv("MINDMAP_EMBED_MODEL"); p != "" {
		return p
	}
	if dir := resourcesDir(); dir != "" {
		return filepath.Join(dir, embed.FileName)
	}
	return ""
}

var (
	localOnce  sync.Once
	localModel *embed.Model
)

// loadLocalModel reads the built-in model once, nil when the app has none.
func loadLocalModel() *embed.Model {
	localOnce.Do(func() {
		if p := localModelPath(); p != "" {
			localModel, _ = embed.Load(p)
		}
	})
	return localModel
}

func (a *app) localModel() *embed.Model {
	if a.noLocalModel {
		return nil
	}
	return loadLocalModel()
}
