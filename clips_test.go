package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/screenshot"
)

// claudeText is a reply of Claude with text only.
func claudeText(text string) string {
	b, _ := json.Marshal(text)
	return fmt.Sprintf(`{"id": "msg", "type": "message", "role": "assistant", "model": "claude-opus-5-5",
		"stop_reason": "end_turn", "content": [{"type": "text", "text": %s}],
		"usage": {"input_tokens": 1, "output_tokens": 1}}`, b)
}

func TestLibraryKeepsAndOrganizes(t *testing.T) {
	a, tt := newTestApp(t)
	var fetched []string
	a.fetchPage = func(_ context.Context, url string) (library.Page, error) {
		fetched = append(fetched, url)
		return library.Page{Title: "缓存设计的五个原则", Description: "讲缓存", Text: "先读缓存再读库；过期时间加随机，避免雪崩。"}, nil
	}
	fake := &fakeClaude{replies: []string{
		claudeText("```json\n" + `{"title": "缓存设计原则", "desc": "一篇讲缓存读写顺序和防雪崩的实践文章。", "summary": "介绍缓存读写顺序和防雪崩的做法。", "category": "技术", "tags": ["后端", "缓存"], "keywords": ["cache", "Redis", "雪崩"], "synonyms": ["高速缓存", "缓存穿透", "cache avalanche"], "analysis": "- 先读缓存再读库\n- 过期时间加随机值防雪崩", "text": ""}` + "\n```"),
		claudeText(`{"title": "周会改到周四", "summary": "团队周会时间调整。", "category": "工作", "tags": ["会议"], "keywords": ["例会"], "text": ""}`),
	}}
	a.sendAI = fake.send
	a.cfg.AIKey = "sk-ant-test"

	if err := tt.Click("资料库"); err != nil {
		t.Fatal(err)
	}
	if !a.libOpen || !tt.HasText("0 条") {
		t.Fatalf("library not shown: %q", tt.Texts())
	}
	a.libDraft = "https://example.com/cache"
	tt.Frame()
	if err := tt.Click("收藏"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.lib.Clips) != 1 {
		t.Fatalf("%d clips", len(a.lib.Clips))
	}
	c := a.lib.Clips[0]
	if c.Kind != library.KindLink || len(fetched) != 1 || c.Title != "缓存设计的五个原则" || !strings.Contains(c.Text, "雪崩") {
		t.Fatalf("link clip %+v, fetched %v", c, fetched)
	}
	if c.Desc != "一篇讲缓存读写顺序和防雪崩的实践文章。" {
		t.Fatalf("desc %q", c.Desc)
	}
	if c.Category != "技术" || !strings.Contains(strings.Join(c.Tags, ","), "缓存") || c.Organized == nil || c.Summary == "" {
		t.Fatalf("not organized: %+v", c)
	}
	// The AI was given the page's text.
	req, _ := json.Marshal(fake.requests[0].Messages)
	if !strings.Contains(string(req), "过期时间加随机") {
		t.Fatalf("prompt: %s", req)
	}

	// Text typed is a text clip, and the AI names it; it is given the
	// categories already in use.
	a.libDraft = "下周起周会改到周四下午三点"
	tt.Frame()
	tt.Click("收藏")
	tt.Frame()
	if len(a.lib.Clips) != 2 || a.lib.Clips[0].Title != "周会改到周四" || a.lib.Clips[0].Category != "工作" {
		t.Fatalf("text clip %+v", a.lib.Clips[0])
	}
	if a.lib.Clips[0].Desc != "团队周会时间调整。" { // none given: the summary's first sentence
		t.Fatalf("fallback desc %q", a.lib.Clips[0].Desc)
	}
	req, _ = json.Marshal(fake.requests[1].Messages)
	if !strings.Contains(string(req), "已有分类：技术") {
		t.Fatalf("categories not given: %s", req)
	}

	if len(c.Synonyms) != 3 || !strings.Contains(c.Analysis, "防雪崩") {
		t.Fatalf("synonyms %v analysis %q", c.Synonyms, c.Analysis)
	}
	if hits := a.lib.Search("高速缓存", nil); len(hits) == 0 || hits[0].Clip != c {
		t.Fatal("not found by a synonym")
	}
	// Found by the search, by a keyword the AI gave.
	a.libQuery = "redis"
	tt.Frame()
	if !tt.HasText("缓存设计的五个原则") || tt.HasText("周会改到周四") {
		t.Fatalf("search: %q", tt.Texts())
	}
	a.libQuery = ""
	tt.Frame()
	if err := tt.Click("工作 1"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("缓存设计的五个原则") {
		t.Fatal("category filter")
	}
	tt.Click("全部分类")
	if err := tt.Click("资料 缓存设计的五个原则"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("一篇讲缓存读写顺序和防雪崩的实践文章。") || !tt.HasText("AI 整理") || !tt.HasText("介绍缓存读写顺序和防雪崩的做法。") {
		t.Fatalf("detail: %q", tt.Texts())
	}
	snapshot(t, tt, "library")

	// Kept on disk, where the app finds it again.
	l, _ := library.Open(library.In(a.dir))
	if len(l.Clips) != 2 {
		t.Fatalf("on disk %d", len(l.Clips))
	}
}

func TestLibraryClipboardAndCapture(t *testing.T) {
	a, tt := newTestApp(t)
	plat := a.plat.(*fakePlatform)
	a.fetchPage = func(context.Context, string) (library.Page, error) {
		return library.Page{}, fmt.Errorf("offline")
	}
	plat.copied = "一段从别处复制的话"
	if _, err := a.clipClipboard("剪贴板"); err != nil {
		t.Fatal(err)
	}
	plat.clipImage = testPNG(t, 40, 30)
	clips, err := a.clipClipboard("剪贴板")
	if err != nil || clips[0].Kind != library.KindImage {
		t.Fatalf("image: %v %+v", err, clips)
	}
	plat.clipImage = nil
	plat.copied = "https://example.com/a"
	a.clipFromShortcut()
	if len(plat.notes) == 0 || !strings.Contains(plat.notes[len(plat.notes)-1], "已收进资料库") {
		t.Fatalf("notes %v", plat.notes)
	}
	link := a.lib.Clips[0]
	if link.Kind != library.KindLink || !strings.Contains(a.libStatus[link.ID], "网页读取失败") {
		t.Fatalf("link %+v status %q", link, a.libStatus[link.ID])
	}
	if err := a.capture("突然想到：资料库可以按周回顾", captureLibrary); err != nil {
		t.Fatal(err)
	}
	if len(a.lib.Clips) != 4 || a.lib.Clips[0].Source != "快速记录" {
		t.Fatalf("%d clips", len(a.lib.Clips))
	}
	// No key: nothing is sorted out, and nothing fails.
	if len(a.lib.Unorganized()) != 4 {
		t.Fatal("organized without a key")
	}

	// Into the map: the picture comes along.
	img := clips[0]
	root := a.doc.Root
	if err := a.clipToMap(img); err != nil {
		t.Fatal(err)
	}
	n := root.Children[len(root.Children)-1]
	if n.Image == nil || n.Image.W != 40 {
		t.Fatalf("node %+v", n)
	}
	a.showLibrary()
	tt.Frame()
	snapshot(t, tt, "library-kinds")
	if !tt.HasText("未整理 4") {
		t.Fatalf("texts %q", tt.Texts())
	}
}

func TestAISearchesLibrary(t *testing.T) {
	a, tt := newTestApp(t)
	a.cfg.LibraryManual = true
	for _, s := range []string{"Postgres 索引：联合索引遵循最左前缀。", "周末买菜清单：西红柿、鸡蛋"} {
		if _, err := a.addClipText(s, "手动"); err != nil {
			t.Fatal(err)
		}
	}
	idx := a.lib.Clips[1]
	fake := &fakeClaude{replies: []string{`{
		"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-opus-5-5",
		"stop_reason": "tool_use",
		"content": [
			{"type": "text", "text": "我查一下资料库。"},
			{"type": "tool_use", "id": "toolu_1", "name": "search_library", "input": {"query": "索引 最左前缀", "category": ""}}
		],
		"usage": {"input_tokens": 1, "output_tokens": 1}}`, fmt.Sprintf(`{
		"id": "msg_2", "type": "message", "role": "assistant", "model": "claude-opus-5-5",
		"stop_reason": "tool_use",
		"content": [{"type": "tool_use", "id": "toolu_2", "name": "read_clip", "input": {"id": %q}}],
		"usage": {"input_tokens": 1, "output_tokens": 1}}`, idx.ID),
		claudeText("根据你收藏的「Postgres 索引」：联合索引要遵循**最左前缀**。"),
	}}
	a.sendAI = fake.send
	a.cfg.AIKey = "sk-ant-test"
	a.showLibrary()
	a.aiOpen = true
	tt.Frame()
	if tt.HasText("当前画布") {
		t.Fatal("map scopes shown in the library")
	}
	a.ai.draft = "我之前存的数据库索引资料说了什么？"
	a.aiSend()
	tt.Frame()
	if len(fake.requests) != 3 {
		t.Fatalf("%d requests", len(fake.requests))
	}
	for _, tool := range fake.requests[0].Tools {
		if tool.OfTool != nil && tool.OfTool.Name == "propose_edits" {
			t.Fatal("edits offered in the library")
		}
	}
	second, _ := json.Marshal(fake.requests[1].Messages)
	if !strings.Contains(string(second), "tool_result") || !strings.Contains(string(second), "最左前缀") || strings.Contains(string(second), "西红柿") {
		t.Fatalf("search result: %s", second)
	}
	third, _ := json.Marshal(fake.requests[2].Messages)
	if !strings.Contains(string(third), "全文") {
		t.Fatalf("read result: %s", third)
	}
	if !tt.HasText("🔎 在资料库中检索「索引 最左前缀」：1 条结果") || !tt.HasText("我查一下资料库。") {
		t.Fatalf("chat: %q", tt.Texts())
	}
	snapshot(t, tt, "library-chat")
	last := a.ai.msgs[len(a.ai.msgs)-1]
	if last.role != "assistant" || !strings.Contains(last.text, "最左前缀") || a.ai.busy {
		t.Fatalf("last %+v", last)
	}

	// On the map, the library is searched too, beside the edits.
	a.libOpen = false
	a.ai = aiState{}
	fake.replies = []string{claudeText("好的")}
	a.ai.draft = "你好"
	a.aiSend()
	names := []string{}
	for _, tool := range fake.requests[3].Tools {
		names = append(names, tool.OfTool.Name)
	}
	if strings.Join(names, ",") != "propose_edits,search_library,read_clip" {
		t.Fatalf("tools %v", names)
	}
	_ = ui.KeyEnter
}

// Organizing a clip without an API key says so in the clip's AI card,
// with the way to fill it in.
func TestOrganizeAsksForKey(t *testing.T) {
	a, tt := newTestApp(t)
	if err := a.capture("今天很好", captureLibrary); err != nil {
		t.Fatal(err)
	}
	a.showLibrary()
	a.libSel = a.lib.Clips[0].ID
	tt.Frame()
	if err := tt.Click("整理"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("AI 整理要先填写 API Key。") || a.libErr != "" {
		t.Fatalf("not asked beside the card: %q", tt.Texts())
	}
	snapshot(t, tt, "library-need-key")
	if err := tt.Click("填写 API Key"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.aiOpen || !tt.HasText("API Key 保存在本机的应用配置文件中（仅当前用户可读）。") {
		t.Fatalf("settings not shown: %v %q", a.aiOpen, tt.Texts())
	}
}

// A screenshot kept from the app lands in the library, shown; one kept
// from another app says so in a notification.
func TestScreenshotIntoLibrary(t *testing.T) {
	a, tt := newTestApp(t)
	plat := a.plat.(*fakePlatform)
	a.showLibrary()
	tt.Frame()
	if err := tt.Click("截图"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if plat.shots != 1 {
		t.Fatalf("shots %d", plat.shots)
	}
	a.libOpen = false
	a.keepShot(testPNG(t, 40, 30), screenshot.Library, true)
	if len(a.lib.Clips) != 1 || !a.libOpen || a.libSel != a.lib.Clips[0].ID {
		t.Fatalf("clips %d open %v sel %q", len(a.lib.Clips), a.libOpen, a.libSel)
	}
	if c := a.lib.Clips[0]; c.Kind != library.KindImage || c.Source != "截图" {
		t.Fatalf("clip %+v", c)
	}
	a.keepShot(testPNG(t, 20, 20), screenshot.Library, false)
	if len(a.lib.Clips) != 2 || len(plat.notes) == 0 || !strings.Contains(plat.notes[len(plat.notes)-1], "截图已收进资料库") {
		t.Fatalf("clips %d notes %v", len(a.lib.Clips), plat.notes)
	}
}
