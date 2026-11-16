package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	a.syncChange()
}

// kept takes a clip just kept, or the one it repeats: that one is shown
// rather than a second kept, and a.dupe says so.
func (a *app) kept(c *library.Clip, err error) (*library.Clip, error) {
	var dup *library.DuplicateError
	if errors.As(err, &dup) {
		a.dupe = true
		a.libSel = dup.Clip.ID
		a.tell("已经收藏过：" + dup.Clip.Label())
		return dup.Clip, nil
	}
	if err != nil {
		return nil, err
	}
	a.noteLibraryWrite()
	a.enqueueClip(c.ID, !a.cfg.LibraryManual)
	return c, nil
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
	} else {
		c.Tags = memoTags(text) // a note's #tags are its tags
	}
	return a.kept(c, a.lib.Add(c, a.now()))
}

// addClipFile copies a file into the library: a picture or a file.
func (a *app) addClipFile(path, source string) (*library.Clip, error) {
	return a.kept(a.lib.AddFile(path, source, a.now()))
}

// addClipImage keeps a picture, as PNG data.
func (a *app) addClipImage(data []byte, source string) (*library.Clip, error) {
	name := "图片 " + a.now().Format("2006-01-02 15.04.05") + ".png"
	return a.kept(a.lib.AddData(data, name, library.KindImage, source, a.now()))
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
	a.dupe = false
	clips, err := a.clipClipboard("剪贴板")
	if err != nil {
		a.plat.Notify("没有存到收集", err.Error())
		return
	}
	if a.dupe && len(clips) == 1 {
		a.plat.Notify("已经收藏过", clips[0].Label())
		return
	}
	body := clips[0].Label()
	if len(clips) > 1 {
		body = fmt.Sprintf("%s 等 %d 项", body, len(clips))
	}
	a.plat.Notify("已存到收集", body)
	a.tell("已存到收集")
}

// clipJob is a clip waiting for its page to be read or for the AI.
type clipJob struct {
	id       string
	organize bool
}

func (a *app) enqueueClip(id string, organize bool) {
	if a.grabbingForAIOf(id) {
		return // its job is under way, waiting on its pictures: not counted twice
	}
	for i, j := range a.libQueue {
		if j.id == id {
			a.libQueue[i].organize = a.libQueue[i].organize || organize
			return
		}
	}
	a.libQueue = append(a.libQueue, clipJob{id, organize})
	if a.orgTotal == 0 {
		a.orgSince, a.orgDone, a.orgResult = a.now(), 0, ""
	}
	a.orgTotal++
	a.nextClipJob()
}

// organizeAll has the AI sort out every clip it has not.
func (a *app) organizeAll() {
	if providerKey(a.cfg) == "" {
		a.openKeySettings()
		return
	}
	for _, c := range a.lib.Unorganized() {
		a.enqueueClip(c.ID, true)
	}
}

// organizeClip has the AI sort out a clip again.
func (a *app) organizeClip(id string) {
	if providerKey(a.cfg) == "" {
		a.libNeedKey = id // said beside the button that asked
		return
	}
	a.libNeedKey = ""
	a.enqueueClip(id, true)
}

// openKeySettings opens the AI's settings, where its API key goes.
func (a *app) openKeySettings() {
	a.aiOpen, a.ai.settings = true, true
	a.loadKeyDrafts()
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
	Synonyms []string `json:"synonyms"`
	Analysis string   `json:"analysis"`
	Text     string   `json:"text"` // what a picture shows
}

// nextClipJob starts on the next clip waiting, one at a time.
func (a *app) nextClipJob() {
	for a.libWorking == "" && len(a.libQueue) > 0 {
		job := a.libQueue[0]
		a.libQueue = a.libQueue[1:]
		c := a.lib.Find(job.id)
		if c == nil {
			a.orgDone++
			continue
		}
		fetch := c.Kind == library.KindLink && c.Text == "" && c.URL != ""
		organize := job.organize && providerKey(a.cfg) != ""
		if !fetch && !organize {
			a.orgDone++
			continue
		}
		a.startClipJob(c, fetch, organize)
	}
	if a.libWorking == "" && len(a.libQueue) == 0 && a.orgTotal > 0 && !a.grabbingForAI() {
		// The run is over: said, when it was of several and long enough.
		if a.orgTotal > 1 && a.now().Sub(a.orgSince) > 2*time.Second {
			a.orgResult = fmt.Sprintf("整理了 %d 条资料", a.orgTotal)
		}
		a.orgTotal, a.orgDone = 0, 0
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
	var images []aiImage
	if organize && clip.Kind == library.KindImage && a.cfg.AIProvider == providerClaude {
		if data, err := os.ReadFile(a.lib.FilePath(c)); err == nil && len(data) <= 4<<20 {
			if mt := imageMediaType(clip.File); mt != "" {
				images = append(images, aiImage{data, mt})
			}
		}
	}
	if organize && isVideo(c) && a.cfg.AIProvider == providerClaude {
		// The AI sees a video by a few of its pictures, taken across the
		// window's frames before it is asked (video.go): the user's video
		// had a random name and nothing else, and the AI said so at length.
		shots, ok := a.vidShots[c.ID]
		if !ok {
			// Its pictures first, in their own queue: the clips after it
			// go on meanwhile, and it comes back at the queue's head.
			a.libWorking = ""
			a.libStatus[c.ID] = "正在看视频…"
			a.queueGrab(c.ID, true)
			return
		}
		if len(shots.pics) == 0 {
			delete(a.vidShots, c.ID) // not seen this time: tried again when asked again
		}
		for _, s := range shots.pics {
			images = append(images, aiImage{s, "image/jpeg"})
		}
		clip.Seen = shots.say()
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
			r.org, r.orgErr = askOrganize(ctx, cfg, send, sendDS, prompt, images)
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
	a.orgDone++
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
	// A file's title is its name till the AI gives a better: a video's
	// random name was all the user's had.
	named := c.Kind == library.KindFile && c.Name != "" && c.Title == strings.TrimSuffix(c.Name, filepath.Ext(c.Name))
	if t := strings.TrimSpace(o.Title); t != "" && (c.Title == "" || named || c.Kind == library.KindText && c.Organized == nil) {
		c.Title = t
	}
	if briefClip(c) {
		o.Summary, o.Desc, o.Analysis = "", "", "" // it says itself what it is
	}
	// As long as what it was made of allows: a summary of a few lines and
	// an analysis of a document, not of a line or of a name. The user
	// found a line kept and a page written of it.
	if n := clipContentLen(c, o); n < summaryLen {
		o.Summary = ""
	}
	if clipContentLen(c, o) < analysisLen || c.Kind == library.KindImage || isVideo(c) {
		o.Analysis = ""
	}
	// What the AI says of what it does not know goes, a sentence at a time;
	// of the new answer only, never of what the user wrote.
	o.Desc, o.Summary, o.Analysis = dropFiller(o.Desc), dropFiller(o.Summary), dropFiller(o.Analysis)
	// Organized again, it is the new answer: a summary or an analysis the
	// new one leaves out goes, and a description that said nothing.
	c.Summary = strings.TrimSpace(o.Summary)
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
	if syn := cleanTags(o.Synonyms); len(syn) > 0 {
		c.Synonyms = syn
	}
	c.Analysis = strings.TrimSpace(o.Analysis)
	if c.Kind == library.KindImage && c.Text == "" || isVideo(c) && strings.TrimSpace(o.Text) != "" {
		c.Text = strings.TrimSpace(o.Text) // a video's, seen again, is new
	}
	c.Organized = &now
}

// What a clip was made of decides what the AI writes of it: a summary of
// summaryLen runes and more, an analysis of analysisLen.
const (
	summaryLen  = 400
	analysisLen = 1500
)

// clipContentLen is how much there was to read of a clip: its text and
// note, or what its pictures showed (the new answer's, when there is one).
func clipContentLen(c *library.Clip, o *clipOrganized) int {
	text := c.Text
	if (c.Kind == library.KindImage || isVideo(c)) && o != nil && strings.TrimSpace(o.Text) != "" {
		text = o.Text // seen again: what was seen before is not added to it
	}
	return utf8.RuneCountInString(strings.TrimSpace(text)) + utf8.RuneCountInString(strings.TrimSpace(c.Note))
}

// fillerWords say what is not known of the clip itself: that it cannot be
// told from its name, that this video or file cannot be told, that it is
// to be opened, that a line is to be added. Only of the clip: what cannot
// be decided in a text (医生无法确定这是否是过敏, 难以判断其长期影响,
// 无法从文件恢复) is its content, which two reviews found dropped.
var fillerWords = regexp.MustCompile(`无法从(文件名|标题|名称)|` +
	`(无法|不能|没法|难以|看不出)(判断|确定|得知|看出|知道)?(这个|这条|该|此)(视频|文件|资料|图片|条目|笔记|链接|网页)|` +
	`(这个|这条|该|此)(视频|文件|资料|图片|条目|笔记|链接|网页)[^，。；]{0,12}(无法|不能|没法|难以|看不出|不清楚)|` +
	`(需要|还需|需)(打开|播放)(后|才|查看|确认|播放|一遍|看看|并)|没有(任何)?(实际内容|内容线索)|` +
	`建议(补充|补上)(一句|说明|描述)|(内容|主题)(未知|待补充)`)

// fillerSaid tells whether a text says what is not known rather than
// what is.
func fillerSaid(s string) bool { return fillerWords.MatchString(s) }

// dropFiller takes out of a text the clauses, or the points of a list,
// that say what is not known of the clip: a point whole, or not at all.
func dropFiller(s string) string {
	if !fillerSaid(s) {
		return s
	}
	var b strings.Builder
	for _, line := range strings.SplitAfter(s, "\n") {
		if t := strings.TrimSpace(line); strings.HasPrefix(t, "-") || strings.HasPrefix(t, "*") {
			if !fillerSaid(line) {
				b.WriteString(line)
			}
			continue
		}
		var kept strings.Builder
		start := 0
		for i, r := range line {
			if strings.ContainsRune("。！？；，", r) {
				end := i + utf8.RuneLen(r)
				if !fillerSaid(line[start:end]) {
					kept.WriteString(line[start:end])
				}
				start = end
			}
		}
		if rest := line[start:]; !fillerSaid(rest) {
			kept.WriteString(rest)
		}
		// A clause gone from the end leaves its comma: the sentence ends.
		k := strings.TrimRight(kept.String(), "\n")
		if strings.HasSuffix(k, "，") {
			k = strings.TrimSuffix(k, "，") + "。"
		}
		b.WriteString(k)
		if strings.HasSuffix(line, "\n") {
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String())
}

// summaryShown and analysisShown tell whether a clip's details show what
// the AI wrote of it: what was written before the rules of applyOrganized
// is kept, and shown by them, by the length of what it was made of.
func summaryShown(c *library.Clip) bool {
	return c.Summary != "" && !briefClip(c) && clipContentLen(c, nil) >= summaryLen
}

func analysisShown(c *library.Clip) bool {
	return c.Analysis != "" && clipContentLen(c, nil) >= analysisLen && c.Kind != library.KindImage && !isVideo(c)
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
{"title": "简短标题，不超过 30 字", "desc": "一句话说清这是什么，不超过 40 字", "summary": "只有内容有好几段时才写，2–3 句；否则空字符串", "category": "一个分类", "tags": ["1–4 个标签"], "keywords": ["3–8 个检索关键词：资料里的核心概念和术语"], "synonyms": ["3–8 个同义词和近义说法：别名、中英文对应、缩写，用户以后可能用这些词来找它"], "analysis": "只有长文章、长文档才写，2–3 条 Markdown 要点；否则空字符串", "text": "只有图片和视频才填：画面里的文字和内容，视频按时间顺序；其他留空字符串"}

写多少看内容有多少：内容只有一句话、一个文件名时，只起标题、分类、标签和检索词，其余留空。不知道的就留空，不要写“无法判断”“需要打开查看”“建议补充说明”“没有任何信息”之类的话。
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

// briefLen is how long a note may be and still say itself what it is.
const briefLen = 200

// briefClip tells whether a clip is a note short enough to need no
// summary: what the AI wrote of it would be longer than the note.
func briefClip(c *library.Clip) bool {
	return c.Kind == library.KindText && len([]rune(strings.TrimSpace(c.Text))) <= briefLen
}

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
	if isVideo(c) {
		if c.Seen != "" {
			b.WriteString("（这是一个视频：" + c.Seen + "。按画面起标题、写描述，文件名多半是随机字符，不用管它）\n")
		} else {
			b.WriteString("（这是一个视频，画面没能读到。文件名是随机字符时，title 和 desc 都留空字符串）\n")
		}
	}
	if briefClip(c) {
		b.WriteString("\n这条内容很短，它自己就说清了自己：desc、summary、analysis 都填空字符串，不要复述或展开原文；只起标题、分类、打标签、给关键词和同义词。\n")
	} else {
		b.WriteString("\n摘要和分析是为了比读原文更快：两者加起来不要超过原文长度的一半，原文没说的不要补。\n")
	}
	// A key or a password in it stays here: the AI is sent dots.
	return strings.Replace(prompt, clipPromptBody, library.Mask(b.String()), 1)
}

func cmpOr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// askOrganize asks the provider chosen to sort out a clip.
// aiImage is a picture sent to the AI with what it is asked.
type aiImage struct {
	data []byte
	mt   string // its media type
}

func askOrganize(ctx context.Context, cfg config, send aiSender, sendDS deepseekSender, prompt string, images []aiImage) (*clipOrganized, error) {
	var text string
	if cfg.AIProvider == providerDeepSeek {
		// Room for the thinking of the models that think first, as V4 Pro.
		resp, err := sendDS(ctx, cfg, dsRequest{Model: deepseekModel(cfg), MaxTokens: 16000, Messages: []dsMessage{
			{Role: "system", Content: organizeSystem}, {Role: "user", Content: prompt},
		}, ResponseFormat: &dsFormat{Type: "json_object"}}, nil)
		if err != nil {
			return nil, err
		}
		text = resp.Choices[0].Message.Content
		if strings.TrimSpace(text) == "" && resp.Choices[0].FinishReason == "length" {
			return nil, plainError("回答超出长度，没有写完，请重试或换用 Flash 模型")
		}
	} else {
		var blocks []anthropic.BetaContentBlockParamUnion
		for _, im := range images {
			blocks = append(blocks, anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{
				Data: base64.StdEncoding.EncodeToString(im.data), MediaType: anthropic.BetaBase64ImageSourceMediaType(im.mt),
			}))
		}
		blocks = append(blocks, anthropic.NewBetaTextBlock(prompt))
		resp, err := send(ctx, cfg, anthropic.BetaMessageNewParams{
			Model:     anthropic.Model(aiModel(cfg)),
			MaxTokens: 2000,
			System:    []anthropic.BetaTextBlockParam{{Text: organizeSystem}},
			Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(blocks...)},
		}, nil)
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
		return nil, plainError("AI 没有按格式回答，请重试")
	}
	var o clipOrganized
	if err := json.Unmarshal([]byte(text[i:j+1]), &o); err != nil {
		return nil, plainError("AI 的回答无法解析，请重试")
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
	n.Links = []string{clipScheme + c.ID}
	if c.URL != "" {
		n.Links = append(n.Links, c.URL)
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
