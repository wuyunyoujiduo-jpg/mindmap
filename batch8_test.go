package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// fakeEmbed gives texts vectors by the words they have of a few, and counts
// the calls.
func fakeEmbed(calls *int) embedFunc {
	axes := [][]string{{"数据库", "SQL", "索引", "慢"}, {"菜", "鸡蛋"}}
	return func(_ context.Context, _ library.Embedder, texts []string) ([][]float32, error) {
		*calls++
		var out [][]float32
		for _, s := range texts {
			v := []float32{0, 0, 0.05}
			for i, words := range axes {
				for _, w := range words {
					if strings.Contains(s, w) {
						v[i]++
					}
				}
			}
			var n float32
			for _, x := range v {
				n += x * x
			}
			for i := range v {
				v[i] /= float32(sqrt(float64(n)))
			}
			out = append(out, v)
		}
		return out, nil
	}
}

func sqrt(x float64) float64 {
	z := x
	for range 30 {
		z = (z + x/z) / 2
	}
	return z
}

func TestLibrarySemanticAndRelated(t *testing.T) {
	a, tt := newTestApp(t)
	calls := 0
	a.embed = fakeEmbed(&calls)
	a.cfg.LibraryManual = true
	for _, s := range []string{"SQL 优化：联合索引遵循最左前缀", "番茄炒蛋：鸡蛋先炒", "SQL 慢查询日志怎么看"} {
		if _, err := a.addClipText(s, "手动"); err != nil {
			t.Fatal(err)
		}
	}
	a.cfg.EmbedURL, a.cfg.EmbedModel = "http://embed", "m"
	a.embedLibrary()
	if ids, _ := a.lib.Unembedded("m"); len(ids) != 0 {
		t.Fatalf("%d clips without vectors", len(ids))
	}
	a.showLibrary()
	a.libQuery = "数据库变慢"
	a.libQueryAt = testNow.Add(-time.Second)
	tt.Frame()
	tt.Frame()
	if _, ok := a.qvecs["数据库变慢"]; !ok {
		t.Fatal("query not embedded")
	}
	hits := a.libHits()
	if len(hits) < 2 || !strings.HasPrefix(hits[0].Clip.Text, "SQL") || !strings.HasPrefix(hits[1].Clip.Text, "SQL") {
		t.Fatalf("hits %v", hits)
	}

	// The clip shown lists those like it.
	a.libQuery = ""
	sqlNote := a.lib.Clips[2]
	a.libSel = sqlNote.ID
	tt.Frame()
	if !tt.HasText("相关资料") || !tt.HasText("SQL 慢查询日志怎么看") {
		t.Fatalf("related: %q", tt.Texts())
	}
	snapshot(t, tt, "library-related")

	// Kept twice: the first is shown.
	plat := a.plat.(*fakePlatform)
	plat.copied = "番茄炒蛋：鸡蛋先炒"
	a.clipFromShortcut()
	if len(a.lib.Clips) != 3 || !strings.Contains(plat.notes[len(plat.notes)-1], "已经收藏过") || a.libSel != a.lib.Clips[1].ID {
		t.Fatalf("duplicate kept: %d %v", len(a.lib.Clips), plat.notes)
	}
}

func TestNodeLinksToClips(t *testing.T) {
	a, tt := newTestApp(t)
	a.cfg.LibraryManual = true
	clip, _ := a.addClipText("性能优化清单：图片懒加载、接口合并", "手动")
	clip.Title = "性能优化清单"
	a.lib.Save(clip, testNow)

	// A node about it finds it, and links it.
	slow := find(t, a, "列表加载慢")
	a.mutate("", func() { slow.Title = "性能优化" })
	if hits := a.nodeClips(slow); len(hits) != 1 || hits[0].Clip != clip {
		t.Fatalf("node clips %v", hits)
	}
	a.selectNode(slow.ID)
	a.panelTab = tabNode
	tt.Frame()
	if !tt.HasText("资料库里可能相关") {
		t.Fatalf("panel: %q", tt.Texts())
	}
	a.addLink(slow, clipScheme+clip.ID)
	if icon, label := a.linkLabel(clipScheme + clip.ID); icon != "📚" || label != "性能优化清单" {
		t.Fatalf("label %s %s", icon, label)
	}
	if hits := a.nodeClips(slow); len(hits) != 0 {
		t.Fatal("linked clip still suggested")
	}
	// The clip knows the node, and the link opens the clip.
	if back := a.clipBacklinks(clip.ID); len(back) != 1 || back[0].id != slow.ID {
		t.Fatalf("backlinks %v", back)
	}
	a.openLink(clipScheme + clip.ID)
	if !a.libOpen || a.libSel != clip.ID {
		t.Fatal("link did not show the clip")
	}
	tt.Frame()
	if !tt.HasText("被 1 个节点引用") {
		t.Fatalf("detail: %q", tt.Texts())
	}

	// The palette links clips to nodes.
	a.libOpen = false
	other := find(t, a, "待优化")
	a.linkFrom = other.ID
	a.openPalette(paletteClip)
	a.paletteQuery = "性能"
	items := a.paletteItems()
	if len(items) != 1 {
		t.Fatalf("items %v", items)
	}
	items[0].run()
	if !strings.Contains(strings.Join(other.Links, " "), clipScheme+clip.ID) {
		t.Fatalf("links %v", other.Links)
	}

	// Clips become a map, a branch a category, linked back.
	clip.Category = "技术"
	a.clipsToMap("资料回顾", a.lib.Clips)
	if mapName(a.path) != "资料回顾" || a.doc.Root.Children[0].Title != "技术" {
		t.Fatalf("map %s %v", a.path, a.doc.Root.Children)
	}
	n := a.doc.Root.Children[0].Children[0]
	if n.Title != "性能优化清单" || n.Links[0] != clipScheme+clip.ID {
		t.Fatalf("node %+v", n)
	}
}

func TestStreamingAndSavedChats(t *testing.T) {
	a, tt := newTestApp(t)
	a.cfg.AIKey = "sk-ant-test"
	var seen []string
	fake := &fakeClaude{replies: []string{claudeText("第一段回答。")}}
	a.sendAI = func(ctx context.Context, cfg config, p anthropic.BetaMessageNewParams, onText func(string)) (*anthropic.BetaMessage, error) {
		if onText == nil {
			t.Fatal("not streamed")
		}
		onText("第一段")
		seen = append(seen, a.ai.streaming) // shown while it comes
		return fake.send(ctx, cfg, p, nil)
	}
	tt.Click("AI 助手")
	a.ai.draft = "总结一下"
	a.aiSend()
	tt.Frame()
	if len(seen) != 1 || seen[0] != "第一段" || a.ai.streaming != "" || !tt.HasText("第一段回答。") {
		t.Fatalf("streaming %q, now %q", seen, a.ai.streaming)
	}

	// Another map: its own conversation; back: the first again, which goes
	// on with what was said.
	first := a.path
	a.newMap()
	tt.Frame()
	if len(a.ai.msgs) != 0 || a.ai.key != a.path {
		t.Fatalf("other map shows %d messages", len(a.ai.msgs))
	}
	_ = a.open(first)
	tt.Frame()
	if len(a.ai.msgs) != 2 || a.ai.msgs[1].text != "第一段回答。" || len(a.ai.claude) != 2 {
		t.Fatalf("restored %d msgs, %d turns", len(a.ai.msgs), len(a.ai.claude))
	}
	fake.replies = []string{claudeText("好的")}
	a.ai.draft = "继续"
	a.aiSend()
	if got := len(fake.requests[len(fake.requests)-1].Messages); got != 3 {
		t.Fatalf("%d messages sent", got)
	}
	// A new conversation; the old one stays in the history.
	a.newChat()
	a.ai.draft = "新问题"
	fake.replies = []string{claudeText("新回答")}
	a.aiSend()
	if chats := loadChats(first); len(chats) != 2 || chats[1].Title != "总结一下" {
		t.Fatalf("chats %+v", chats)
	}
}

func TestDeepSeekStreams(t *testing.T) {
	a, tt := newTestApp(t)
	var streamed bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = jsonDecode(r, &body)
		streamed = body["stream"] == true
		w.Header().Set("Content-Type", "text/event-stream")
		for _, chunk := range []string{
			`{"choices":[{"delta":{"role":"assistant","reasoning_content":"想一想"}}]}`,
			`{"choices":[{"delta":{"content":"你好，"}}]}`,
			`{"choices":[{"delta":{"content":"我在。"}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"search_library","arguments":"{\"query\":"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"x\",\"category\":\"\"}"}}]},"finish_reason":"tool_calls"}]}`,
		} {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	a.cfg.AIProvider, a.cfg.DSKey, a.cfg.DSBaseURL = providerDeepSeek, "sk", srv.URL
	var pieces []string
	resp, err := sendToDeepSeek(context.Background(), a.cfg, a.deepseekRequest(), func(s string) { pieces = append(pieces, s) })
	if err != nil {
		t.Fatal(err)
	}
	m := resp.Choices[0].Message
	if !streamed || strings.Join(pieces, "") != "你好，我在。" || m.Content != "你好，我在。" || m.ReasoningContent != "想一想" {
		t.Fatalf("message %+v pieces %q", m, pieces)
	}
	if len(m.ToolCalls) != 1 || m.ToolCalls[0].ID != "call_1" || m.ToolCalls[0].Function.Arguments != `{"query":"x","category":""}` || resp.Choices[0].FinishReason != "tool_calls" {
		t.Fatalf("tool calls %+v", m.ToolCalls)
	}
	_ = tt
}

func TestBackups(t *testing.T) {
	a, _ := newTestApp(t)
	a.cfg.LibraryManual = true
	a.addClipText("备份里也要有资料", "手动")
	dir := t.TempDir()
	// Old backups past the number kept go.
	for i := 1; i <= backupsKept; i++ {
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s2026-09-%02d.zip", backupPrefix, i)), nil, 0o644)
	}
	a.cfg.BackupDir = dir
	a.autoBackup()
	zipPath := filepath.Join(dir, backupPrefix+"2026-10-10.zip")
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	all := strings.Join(names, "\n")
	base := filepath.Base(a.dir)
	if !strings.Contains(all, base+"/"+model.FileName("使用指南")) || !strings.Contains(all, base+"/.library/clips/") {
		t.Fatalf("zip has %v", names)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != backupsKept {
		t.Fatalf("%d backups kept", len(entries))
	}
	if _, err := os.Stat(filepath.Join(dir, backupPrefix+"2026-09-01.zip")); !os.IsNotExist(err) {
		t.Fatal("oldest not removed")
	}
	// Once a day.
	os.Remove(zipPath)
	a.autoBackup()
	if _, err := os.Stat(zipPath); !os.IsNotExist(err) {
		t.Fatal("backed up twice a day")
	}
}

func jsonDecode(r *http.Request, v any) error { return json.NewDecoder(r.Body).Decode(v) }

// TestBuiltinModel searches by meaning with the built-in model, when it is
// at hand (MINDMAP_EMBED_MODEL, as in CI).
func TestBuiltinModel(t *testing.T) {
	if os.Getenv("MINDMAP_EMBED_MODEL") == "" {
		t.Skip("no model")
	}
	a, tt := newTestApp(t)
	a.noLocalModel = false
	if !a.embedder().Builtin() {
		t.Fatal("built-in model not used")
	}
	a.cfg.LibraryManual = true
	for _, s := range []string{"SQL 优化：联合索引遵循最左前缀", "番茄炒蛋的做法", "下周去东京旅行，记得订机票", "每周三次力量训练"} {
		a.addClipText(s, "手动")
	}
	a.embedLibrary()
	if ids, _ := a.lib.Unembedded(a.embedder().Name()); len(ids) != 0 {
		t.Fatalf("%d clips without vectors", len(ids))
	}
	for q, want := range map[string]string{"数据库变慢": "SQL", "想吃家常菜": "番茄", "健身": "每周"} {
		a.showLibrary()
		a.libQuery, a.libQueryAt = q, testNow.Add(-time.Second)
		tt.Frame()
		tt.Frame()
		hits := a.libHits()
		if len(hits) == 0 || !strings.HasPrefix(hits[0].Clip.Text, want) {
			var got []string
			for _, h := range hits {
				got = append(got, h.Clip.Text)
			}
			t.Errorf("%s: %q", q, got)
		}
	}
}

func TestBrokenMapRestored(t *testing.T) {
	a, tt := newTestApp(t)
	path := a.path
	find(t, a, "列表加载慢")
	os.WriteFile(path, []byte(`{"version": 2, "sheets": [`), 0o644) // cut off
	if err := a.open(path); err == nil || a.broken != path {
		t.Fatalf("err %v broken %q", err, a.broken)
	}
	tt.Frame()
	if err := tt.Click("从历史版本恢复"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Frame()
	if a.broken != "" || a.path != path {
		t.Fatalf("not restored: %q %q", a.broken, a.saveErr)
	}
	find(t, a, "列表加载慢")
	matches, _ := filepath.Glob(path + ".broken-*")
	if len(matches) != 1 {
		t.Fatal("the broken file was not kept")
	}
	// A file that would not read back is never written.
	f := model.NewFile("x")
	f.Sheets[0].Root = nil
	if err := f.Save(filepath.Join(a.dir, "x"+model.Ext)); err == nil {
		t.Fatal("an unreadable file was saved")
	}
}

func TestTour(t *testing.T) {
	a, tt := newTestApp(t)
	a.tourStep = 1
	tt.Frame()
	if !tt.HasText("用导图管理项目") {
		t.Fatalf("%q", tt.Texts())
	}
	snapshot(t, tt, "tour")
	tt.Click("下一步")
	if !tt.HasText("资料库：随手收藏，AI 整理") {
		t.Fatal("step 2")
	}
	tt.Click("打开资料库")
	tt.Frame()
	if a.tourStep != 0 || !a.libOpen || !a.cfg.TourDone {
		t.Fatalf("step %d lib %v done %v", a.tourStep, a.libOpen, a.cfg.TourDone)
	}
	// The settings, a tab a part.
	a.libOpen = false
	a.openSettings()
	a.settingsTab = 1 // 资料库
	tt.Frame()
	if !tt.HasText("自动整理资料") {
		t.Fatalf("library tab: %q", tt.Texts())
	}
	snapshot(t, tt, "settings-library")
}

func TestTaskCheckAndNotePopover(t *testing.T) {
	a, tt := newTestApp(t)
	slow := find(t, a, "列表加载慢")
	tt.Frame()
	clickNode(t, tt, a, "列表加载慢") // brings it in view
	if err := tt.Click("完成任务 列表加载慢"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if slow.Task.Status != model.StatusDone {
		t.Fatalf("status %v", slow.Task.Status)
	}
	if err := tt.Click("取消完成 列表加载慢"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if slow.Task.Status != model.StatusTodo {
		t.Fatalf("status after uncheck %v", slow.Task.Status)
	}
	// A note by the node.
	a.noteEditing = slow.ID
	tt.Frame()
	tt.Frame()
	if !tt.HasText("支持 Markdown") {
		t.Fatalf("no popover: %q", tt.Texts())
	}
	snapshot(t, tt, "note-popover")
	tt.Type("。先加分页")
	tt.Frame()
	if !strings.HasSuffix(slow.Note, "先加分页") {
		t.Fatalf("note %q", slow.Note)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.noteEditing != "" {
		t.Fatal("popover still open")
	}
	snapshot(t, tt, "task-checks")
}
