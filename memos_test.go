package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
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
	if err := tt.Click("灵感"); err != nil {
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
