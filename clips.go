package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// The library keeps what the user collects: text typed or copied, links,
// pictures and files. A link's page is read for its title and text; the
// AI then sums each clip up, sorts it into a category and tags it, with
// words to find it by. A local index finds clips again, for the library's
// search and for the AI assistant, which looks things up in it when it
// answers.

// defaultClipKey keeps the clipboard in the library from any app.
const defaultClipKey = "Control+Alt+V"

func (a *app) clipKey() string {
	if a.cfg.ClipKey != "" {
		return a.cfg.ClipKey
	}
	return defaultClipKey
}

// openLibrary reads the library of the maps folder.
func (a *app) openLibrary() {
	dir := library.In(a.dir)
	l, err := library.Open(dir)
	if err != nil {
		l = &library.Library{Dir: dir}
	}
	a.lib, a.libSel = l, ""
	a.libMod = modTime(filepath.Join(dir, "clips"))
}

// checkLibrary reads the library again when clips were added or changed
// outside the app, as by an AI agent.
func (a *app) checkLibrary() {
	if a.lib == nil {
		return
	}
	mt := modTime(filepath.Join(a.lib.Dir, "clips"))
	if mt.Equal(a.libMod) || a.libWorking != "" {
		return
	}
	known := map[string]bool{}
	for _, c := range a.lib.Clips {
		known[c.ID] = true
	}
	sel := a.libSel
	a.openLibrary()
	a.libSel = sel
	// Clips kept by agents are read and sorted out as the app's own.
	for _, c := range a.lib.Clips {
		if !known[c.ID] && c.Organized == nil {
			a.enqueueClip(c.ID, !a.cfg.LibraryManual)
		}
	}
}

// noteLibraryWrite takes the library's folder as written by the app.
func (a *app) noteLibraryWrite() {
	a.libMod = modTime(filepath.Join(a.lib.Dir, "clips"))
}

// addClip keeps a clip and has its page read and the AI sort it out.
func (a *app) addClip(c *library.Clip) error {
	if err := a.lib.Add(c, a.now()); err != nil {
		return err
	}
	a.noteLibraryWrite()
	a.enqueueClip(c.ID, !a.cfg.LibraryManual)
	return nil
}

// addClipText keeps text: a link when it is a lone web address.
func (a *app) addClipText(text, source string) (*library.Clip, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, errors.New("没有内容")
	}
	c := &library.Clip{Kind: library.KindText, Text: text, Source: source}
	if library.IsURL(text) {
		c.Kind, c.Text, c.URL = library.KindLink, "", text
	}
	return c, a.addClip(c)
}

// addClipFile copies a file into the library: a picture or a file.
func (a *app) addClipFile(path, source string) (*library.Clip, error) {
	c, err := a.lib.AddFile(path, source, a.now())
	if err != nil {
		return nil, err
	}
	a.noteLibraryWrite()
	a.enqueueClip(c.ID, !a.cfg.LibraryManual)
	return c, nil
}

// addClipImage keeps a picture, as PNG data.
func (a *app) addClipImage(data []byte, source string) (*library.Clip, error) {
	name := "图片 " + a.now().Format("2006-01-02 15.04.05") + ".png"
	c, err := a.lib.AddData(data, name, library.KindImage, source, a.now())
	if err != nil {
		return nil, err
	}
	a.noteLibraryWrite()
	a.enqueueClip(c.ID, !a.cfg.LibraryManual)
	return c, nil
}

// clipClipboard keeps what is on the clipboard: the files copied, else a
// picture, else the text. It returns the clips kept.
func (a *app) clipClipboard(source string) ([]*library.Clip, error) {
	var out []*library.Clip
	for _, f := range a.plat.ClipboardFiles() {
		if info, err := os.Stat(f); err != nil || info.IsDir() {
			continue
		}
		c, err := a.addClipFile(f, source)
		if err != nil {
			return out, err
		}
		out = append(out, c)
	}
	if len(out) > 0 {
		return out, nil
	}
	if data := a.plat.ClipboardImage(); len(data) > 0 {
		c, err := a.addClipImage(data, source)
		if err != nil {
			return nil, err
		}
		return []*library.Clip{c}, nil
	}
	text := a.plat.ClipboardText()
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("剪贴板是空的")
	}
	c, err := a.addClipText(text, source)
	if err != nil {
		return nil, err
	}
	return []*library.Clip{c}, nil
}

// clipFromShortcut keeps the clipboard when the global shortcut is
// pressed, in any app, and says so in a notification.
func (a *app) clipFromShortcut() {
	clips, err := a.clipClipboard("剪贴板")
	if err != nil {
		a.plat.Notify("没有收进资料库", err.Error())
		return
	}
	body := clips[0].Label()
	if len(clips) > 1 {
		body = fmt.Sprintf("%s 等 %d 项", body, len(clips))
	}
	a.plat.Notify("已收进资料库", body)
	a.tell("已收进资料库")
}

// clipJob is a clip waiting for its page to be read or for the AI.
type clipJob struct {
	id       string
	organize bool
}

func (a *app) enqueueClip(id string, organize bool) {
	for i, j := range a.libQueue {
		if j.id == id {
			a.libQueue[i].organize = a.libQueue[i].organize || organize
			return
		}
	}
	a.libQueue = append(a.libQueue, clipJob{id, organize})
	a.nextClipJob()
}

// organizeAll has the AI sort out every clip it has not.
func (a *app) organizeAll() {
	if providerKey(a.cfg) == "" {
		a.libErr = "AI 整理需要 API Key：在 AI 助手的设置里填写。"
		return
	}
	for _, c := range a.lib.Unorganized() {
		a.enqueueClip(c.ID, true)
	}
}

// organizeClip has the AI sort out a clip again.
func (a *app) organizeClip(id string) {
	if providerKey(a.cfg) == "" {
		a.libErr = "AI 整理需要 API Key：在 AI 助手的设置里填写。"
		return
	}
	a.enqueueClip(id, true)
}

// clipResult is what reading a clip's page and asking the AI gave.
type clipResult struct {
	page     *library.Page
	fetchErr error
	org      *clipOrganized
	orgErr   error
}

// clipOrganized is the AI's answer on a clip.
type clipOrganized struct {
	Title    string   `json:"title"`
	Desc     string   `json:"desc"`
	Summary  string   `json:"summary"`
	Category string   `json:"category"`
	Tags     []string `json:"tags"`
	Keywords []string `json:"keywords"`
	Text     string   `json:"text"` // what a picture shows
}

// nextClipJob starts on the next clip waiting, one at a time.
func (a *app) nextClipJob() {
	for a.libWorking == "" && len(a.libQueue) > 0 {
		job := a.libQueue[0]
		a.libQueue = a.libQueue[1:]
		c := a.lib.Find(job.id)
		if c == nil {
			continue
		}
		fetch := c.Kind == library.KindLink && c.Text == "" && c.URL != ""
		organize := job.organize && providerKey(a.cfg) != ""
		if !fetch && !organize {
			continue
		}
		a.startClipJob(c, fetch, organize)
	}
}

func (a *app) startClipJob(c *library.Clip, fetch, organize bool) {
	a.libWorking = c.ID
	if fetch {
		a.libStatus[c.ID] = "正在读取网页…"
	} else {
		a.libStatus[c.ID] = "AI 整理中…"
	}
	clip := *c
	var image []byte
	if organize && clip.Kind == library.KindImage && a.cfg.AIProvider == providerClaude {
		if data, err := os.ReadFile(a.lib.FilePath(c)); err == nil && len(data) <= 4<<20 {
			image = data
		}
	}
	base := a.organizePrompt()
	cfg := a.cfg
	fetchPage, send, sendDS := a.fetchPage, a.sendAI, a.sendDeepSeek
	a.async(func() (any, error) {
		var r clipResult
		ctx := context.Background()
		if fetch {
			p, err := fetchPage(ctx, clip.URL)
			if err != nil {
				r.fetchErr = err
			} else {
				r.page = &p
				applyPage(&clip, p)
			}
		}
		if organize {
			prompt := organizePromptFor(&clip, base)
			r.org, r.orgErr = askOrganize(ctx, cfg, send, sendDS, prompt, image, clip.File)
		}
		return r, nil
	}, func(v any, _ error) {
		r, _ := v.(clipResult)
		a.finishClipJob(c.ID, r)
	})
}

// finishClipJob keeps what a clip's page and the AI gave.
func (a *app) finishClipJob(id string, r clipResult) {
	a.libWorking = ""
	delete(a.libStatus, id)
	if c := a.lib.Find(id); c != nil {
		if r.page != nil {
			applyPage(c, *r.page)
		}
		if r.fetchErr != nil {
			a.libStatus[id] = "网页读取失败：" + r.fetchErr.Error()
		}
		if r.org != nil {
			applyOrganized(c, r.org, a.now())
		}
		if r.orgErr != nil {
			a.libStatus[id] = "AI 整理失败：" + aiError(r.orgErr)
		}
		if r.page != nil || r.org != nil {
			if err := a.lib.Save(c, a.now()); err != nil {
				a.libErr = err.Error()
			}
			a.noteLibraryWrite()
		}
	}
	a.nextClipJob()
}

// applyPage takes a page's title and text into a link's clip.
func applyPage(c *library.Clip, p library.Page) {
	if c.Title == "" {
		c.Title = p.Title
	}
	text := p.Text
	if p.Description != "" && !strings.Contains(text, p.Description) {
		text = p.Description + "\n\n" + text
	}
	if c.Text == "" {
		c.Text = strings.TrimSpace(text)
	}
}

// applyOrganized takes the AI's answer into a clip: a title where it has
// none, the summary and words to find it by, a category where it has
// none, and the tags beside the user's.
func applyOrganized(c *library.Clip, o *clipOrganized, now time.Time) {
	if c.Title == "" || c.Kind == library.KindText && c.Organized == nil {
		c.Title = strings.TrimSpace(o.Title)
	}
	if s := strings.TrimSpace(o.Summary); s != "" {
		c.Summary = s
	}
	if s := strings.TrimSpace(o.Desc); s != "" {
		c.Desc = s
	} else if c.Desc == "" {
		c.Desc = firstSentence(c.Summary)
	}
	if c.Category == "" {
		c.Category = strings.TrimSpace(o.Category)
	}
	for _, t := range cleanTags(o.Tags) {
		if !slices.Contains(c.Tags, t) {
			c.Tags = append(c.Tags, t)
		}
	}
	c.Keywords = cleanTags(o.Keywords)
	if c.Kind == library.KindImage && c.Text == "" {
		c.Text = strings.TrimSpace(o.Text)
	}
	c.Organized = &now
}

const organizeSystem = `你是一个资料整理助手。用户把看到的文字、链接、图片和文件收进资料库，你为每条资料起标题、写摘要、分类、打标签，并给出检索关键词，方便以后查找和让 AI 检索。只输出一个 JSON 对象，不要输出其他内容。`

// organizePrompt asks the AI to sort out a clip, with the categories and
// tags already in use.
func (a *app) organizePrompt() string {
	var cats, tags []string
	for _, k := range a.lib.Categories() {
		cats = append(cats, k.Name)
	}
	seen := map[string]bool{}
	for _, t := range a.allTags() {
		seen[t] = true
		tags = append(tags, t)
	}
	for _, k := range a.lib.Tags() {
		if !seen[k.Name] {
			tags = append(tags, k.Name)
		}
	}
	var b strings.Builder
	b.WriteString(`请整理下面这条资料，输出 JSON：
{"title": "简短标题，不超过 30 字", "desc": "一句简单的描述说明：这是什么、有什么用，不超过 40 字", "summary": "2–4 句话的摘要，说清它讲了什么、有什么用", "category": "一个分类", "tags": ["1–5 个标签"], "keywords": ["5–12 个检索关键词：同义词、中英文说法、相关概念"], "text": "只有图片才填：图中的文字和内容描述；其他留空字符串"}
`)
	if len(cats) > 0 {
		b.WriteString("\n已有分类：" + strings.Join(cats, "、") + "。合适就沿用已有分类，都不合适才新建；分类要宽泛，比如 技术、产品、设计、学习、工作、灵感、生活、资源。\n")
	} else {
		b.WriteString("\n分类要宽泛，比如 技术、产品、设计、学习、工作、灵感、生活、资源。\n")
	}
	if len(tags) > 0 {
		b.WriteString("已有标签：" + strings.Join(tags, "、") + "。优先沿用已有标签。\n")
	}
	b.WriteString("\n" + clipPromptBody)
	return b.String()
}

// clipPromptBody marks where the clip goes in the prompt.
const clipPromptBody = "<资料/>"

// organizePromptFor puts a clip into a prompt.
func organizePromptFor(c *library.Clip, prompt string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "资料（%s，来源：%s，收藏于 %s）：\n", library.KindLabel(c.Kind), cmpOr(c.Source, "手动"), c.Created.Format("2006-01-02"))
	if c.Title != "" {
		b.WriteString("标题：" + c.Title + "\n")
	}
	if c.URL != "" {
		b.WriteString("网址：" + c.URL + "\n")
	}
	if c.Name != "" {
		b.WriteString("文件名：" + c.Name + "\n")
	}
	if c.Note != "" {
		b.WriteString("用户备注：" + c.Note + "\n")
	}
	if c.Text != "" {
		text := []rune(c.Text)
		if len(text) > 8000 {
			text = append(text[:8000], []rune("…（后略）")...)
		}
		b.WriteString("内容：\n" + string(text) + "\n")
	}
	if c.Kind == library.KindImage {
		b.WriteString("（图片见附件）\n")
	}
	return strings.Replace(prompt, clipPromptBody, b.String(), 1)
}

func cmpOr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// askOrganize asks the provider chosen to sort out a clip.
func askOrganize(ctx context.Context, cfg config, send aiSender, sendDS deepseekSender, prompt string, image []byte, file string) (*clipOrganized, error) {
	var text string
	if cfg.AIProvider == providerDeepSeek {
		resp, err := sendDS(ctx, cfg, dsRequest{Model: deepseekModel(cfg), MaxTokens: 2000, Messages: []dsMessage{
			{Role: "system", Content: organizeSystem}, {Role: "user", Content: prompt},
		}})
		if err != nil {
			return nil, err
		}
		text = resp.Choices[0].Message.Content
	} else {
		var blocks []anthropic.BetaContentBlockParamUnion
		if mt := imageMediaType(file); len(image) > 0 && mt != "" {
			blocks = append(blocks, anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{
				Data: base64.StdEncoding.EncodeToString(image), MediaType: anthropic.BetaBase64ImageSourceMediaType(mt),
			}))
		}
		blocks = append(blocks, anthropic.NewBetaTextBlock(prompt))
		resp, err := send(ctx, cfg, anthropic.BetaMessageNewParams{
			Model:     anthropic.Model(aiModel(cfg)),
			MaxTokens: 2000,
			System:    []anthropic.BetaTextBlockParam{{Text: organizeSystem}},
			Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(blocks...)},
		})
		if err != nil {
			return nil, err
		}
		if resp.StopReason == anthropic.BetaStopReasonRefusal {
			return nil, errors.New("请求被模型拒绝")
		}
		for _, block := range resp.Content {
			if b, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
				text += b.Text
			}
		}
	}
	return parseOrganized(text)
}

// parseOrganized reads the JSON object of an answer, around which the
// model may have put words or a code fence.
func parseOrganized(text string) (*clipOrganized, error) {
	i, j := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if i < 0 || j < i {
		return nil, errors.New("AI 没有按格式回答")
	}
	var o clipOrganized
	if err := json.Unmarshal([]byte(text[i:j+1]), &o); err != nil {
		return nil, errors.New("AI 的回答无法解析")
	}
	return &o, nil
}

func imageMediaType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	}
	return ""
}

// clipToMap adds a clip as a node under the node selected in the open
// map: its title, its summary and text as the note, its link, its tags
// and its picture or file.
func (a *app) clipToMap(c *library.Clip) error {
	if a.doc == nil {
		return errors.New("没有打开的导图")
	}
	parent, _ := a.selectedNode()
	if parent == nil {
		parent = a.doc.Root
	}
	n := model.NewNode(c.Label())
	var note []string
	for _, s := range []string{c.Desc, c.Summary, c.Note} {
		if s = strings.TrimSpace(s); s != "" {
			note = append(note, s)
		}
	}
	if c.Kind == library.KindText && c.Text != c.Title {
		note = append(note, c.Text)
	}
	n.Note = strings.Join(note, "\n\n")
	if c.URL != "" {
		n.Links = []string{c.URL}
	}
	n.Tags = slices.Clone(c.Tags)
	if p := a.lib.FilePath(c); p != "" {
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		dir := model.AssetsDir(a.path)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		file := model.NewID() + strings.ToLower(filepath.Ext(p))
		if err := os.WriteFile(filepath.Join(dir, file), data, 0o644); err != nil {
			return err
		}
		var b *ui.Bitmap
		if c.Kind == library.KindImage {
			b, _ = ui.DecodeBitmap(data)
		}
		if b != nil {
			a.bitmaps[filepath.Join(dir, file)] = b
			w, h := b.Size()
			n.Image = &model.Image{File: file, W: w, H: h}
		} else {
			n.Files = []model.Attachment{{File: file, Name: c.Name}}
		}
	}
	a.mutate("", func() {
		parent.Children = append(parent.Children, n)
		parent.Collapsed = false
	})
	a.selected = n.ID
	a.tell(fmt.Sprintf("已加到「%s」下", parent.Title))
	return nil
}

// fetchPager reads a web page; tests replace it.
type fetchPager func(ctx context.Context, url string) (library.Page, error)

// firstSentence is the first sentence of a text, for a description.
func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "。！？\n"); i >= 0 {
		_, size := utf8.DecodeRuneInString(s[i:])
		s = s[:i+size]
	}
	if r := []rune(s); len(r) > 60 {
		s = string(r[:60]) + "…"
	}
	return strings.TrimSpace(s)
}
