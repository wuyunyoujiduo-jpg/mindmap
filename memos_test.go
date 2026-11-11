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

// 灵感 are cards of 收集: written in its box, kept as texts of the library
// with their #tags, filtered by a tag; quick capture writes them too, and
// 回顾 looks back at the old ones.
func TestMemos(t *testing.T) {
	a, tt := newTestApp(t)
	a.showProjects()
	tt.Frame()
	if err := tt.Click("收集"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, text := range []string{"产品要做减法 #产品", "周报模板改成三段式 #工作"} {
		tt.Click("记一条灵感或任务")
		tt.Frame()
		tt.Type(text)
		tt.Key(ui.Cmd, ui.KeyEnter)
		tt.Frame()
	}
	var memos []string
	for _, c := range a.lib.Clips {
		if isMemo(c) {
			memos = append(memos, c.Text+"|"+strings.Join(c.Tags, ","))
		}
	}
	if len(memos) != 2 || !slices.Contains(memos, "产品要做减法 #产品|产品") {
		t.Fatalf("memos %q", memos)
	}
	a.libSel = ""
	tt.Frame()
	if !tt.HasText("#产品 1") || !tt.HasText("#工作 1") || tt.HasText("全部分类") {
		t.Fatalf("tags: %q", tt.Texts())
	}
	snapshot(t, tt, "memos")
	// A tag shows its notes only; again, all of them.
	tt.Click("#工作 1")
	tt.Frame()
	if hits := a.libHits(); len(hits) != 1 || !strings.HasPrefix(hits[0].Clip.Text, "周报模板") {
		t.Fatalf("filtered %d", len(hits))
	}
	tt.Click("#工作 1")
	tt.Frame()
	if a.libTag != "" || len(a.libHits()) != 2 {
		t.Fatalf("tag %q, %d", a.libTag, len(a.libHits()))
	}
	// A note's text changed, its tags follow.
	m := a.lib.Clips[0]
	for _, c := range a.lib.Clips {
		if strings.HasPrefix(c.Text, "产品") {
			m = c
		}
	}
	m.Tags = append(m.Tags, "AI给的")
	a.setMemoText(m, "产品要做减法 #取舍")
	if !slices.Equal(m.Tags, []string{"AI给的", "取舍"}) {
		t.Fatalf("tags after the edit: %q", m.Tags)
	}
	// Quick capture writes 灵感 by default.
	ct := ui.NewTester(a.captureView, 560, 170)
	a.plat.(*fakePlatform).capture = true
	ct.Type("随手一记 #想法")
	ct.Key(0, ui.KeyEnter)
	if hits := a.libHits(); len(hits) != 3 {
		t.Fatalf("captured: %d", len(hits))
	}

	// 回顾: nothing older than a week yet; then one of the old ones.
	a.libReview = reviewDaily
	tt.Frame()
	if len(a.libHits()) != 0 || !tt.HasText("一周以前的灵感会在这里，每天换几条。") {
		t.Fatalf("回顾 with nothing old: %d, %q", len(a.libHits()), tt.Texts())
	}
	m.Created = testNow.AddDate(0, 0, -30)
	a.lib.Version++
	a.libReview = reviewOff
	tt.Frame()
	if err := tt.Click("随机漫步"); err != nil { // in the left column
		t.Fatal(err)
	}
	tt.Frame()
	if hits := a.libHits(); a.libReview != reviewWalk || len(hits) != 1 || hits[0].Clip != m {
		t.Fatalf("随机漫步: %d", len(hits))
	}
	if !tt.HasText("随机漫步 ✕") || !tt.HasText("换一条") {
		t.Fatalf("回顾's chip: %q", tt.Texts())
	}
	snapshot(t, tt, "collect-review")
	tt.Click("随机漫步 ✕")
	tt.Frame()
	if a.libReview != reviewOff || len(a.libHits()) != 3 {
		t.Fatal("still looking back")
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
	a.openList(kindAll, "")
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
