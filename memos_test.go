package main

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
)

func TestMemoTags(t *testing.T) {
	got := memoTags("读《黑客与画家》#读书 #想法，周末再整理 #读书")
	if !slices.Equal(got, []string{"读书", "想法"}) {
		t.Fatalf("tags %q", got)
	}
}

// 灵感: written in its box, kept as a text of the library with its #tags,
// filtered by a tag, edited and found again; quick capture writes there.
func TestMemos(t *testing.T) {
	a, tt := newTestApp(t)
	// 收集 has the notes with the clips; 回顾 is their page, with the days
	// written on and the ways to look back.
	if err := tt.Click("收集"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("回顾"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.memos.open || !tt.HasText("还没有灵感。在上面写下现在的想法，#标签 归类。") {
		t.Fatalf("page: %q", tt.Texts())
	}
	tt.Click("写下灵感")
	tt.Type("产品要做减法 #产品")
	tt.Key(ui.Cmd, ui.KeyEnter)
	tt.Frame()
	tt.Click("写下灵感")
	tt.Type("周报模板改成三段式 #工作")
	tt.Click("记录")
	tt.Frame()
	var memos []string
	for _, c := range a.lib.Clips {
		if isMemo(c) {
			memos = append(memos, c.Text+"|"+strings.Join(c.Tags, ","))
		}
	}
	if len(memos) != 2 || !slices.Contains(memos, "产品要做减法 #产品|产品") {
		t.Fatalf("memos %q", memos)
	}
	if !tt.HasText("#产品 1") || !tt.HasText("#工作 1") {
		t.Fatalf("tags: %q", tt.Texts())
	}
	snapshot(t, tt, "memos")
	// A tag shows its notes only.
	tt.Click("#工作 1")
	tt.Frame()
	if list := a.memoList(); len(list) != 1 || !strings.HasPrefix(list[0].Text, "周报模板") {
		t.Fatalf("filtered %d", len(list))
	}
	a.memos.tag = ""
	// Quick capture writes 灵感 by default.
	ct := ui.NewTester(a.captureView, 560, 170)
	a.plat.(*fakePlatform).capture = true
	ct.Type("随手一记 #想法")
	ct.Key(0, ui.KeyEnter)
	if list := a.memoList(); len(list) != 3 || list[0].Text != "随手一记 #想法" {
		t.Fatalf("captured: %d %q", len(list), list[0].Text)
	}
}

// The AI says where notes belong; taken, a topic linked to the note is
// added there, and the note stays.
func TestMemoPlaces(t *testing.T) {
	a, tt := newTestApp(t)
	m, err := a.addMemo("列表可以用虚拟滚动 #性能", "灵感")
	if err != nil {
		t.Fatal(err)
	}
	a.addMemo("周末去爬山", "灵感")
	a.openMemos()
	tt.Frame()
	fake := &fakeClaude{}
	a.sendAI = func(ctx context.Context, cfg config, p anthropic.BetaMessageNewParams, on func(string)) (*anthropic.BetaMessage, error) {
		prompt := p.Messages[0].Content[0].OfText.Text
		ref, item := "", 0
		for _, line := range strings.Split(prompt, "\n") {
			if strings.HasSuffix(line, "› 待优化") {
				ref = strings.Fields(line)[0]
			}
			if strings.Contains(line, "虚拟滚动") {
				fmt.Sscanf(line, "%d.", &item)
			}
		}
		fake.replies = []string{reply(fmt.Sprintf(`{"places":[{"task":%d,"topic":"%s"}]}`, item, ref))}
		return fake.send(ctx, cfg, p, on)
	}
	a.cfg.AIKey = "sk-ant-test"
	if err := tt.Click("AI 归位"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.clipPlaces) != 1 || a.clipPlaces[m.ID].label == "" {
		t.Fatalf("places %v, %q", a.clipPlaces, a.clipPlaceErr)
	}
	snapshot(t, tt, "memos-places")
	if err := tt.Click("放过去 " + m.Label()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	opt := find(t, a, "待优化")
	last := opt.Children[len(opt.Children)-1]
	if len(last.Links) == 0 || last.Links[0] != clipScheme+m.ID {
		t.Fatalf("not placed: %q %v", last.Title, last.Links)
	}
	if a.lib.Find(m.ID) == nil || len(a.clipPlaces) != 0 {
		t.Fatal("the note is gone, or its suggestion stays")
	}
	// Placed, it is not asked about again.
	a.suggestClipPlaces([]*library.Clip{m})
	if a.clipPlaceErr == "" {
		t.Fatal("asked again of a note already on a map")
	}
}
