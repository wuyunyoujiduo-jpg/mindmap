package main

import (
	"testing"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
)

// TestWaitCards: long work that can be counted is a card at the window's
// top right, on whatever page, and turns into its result, which stays
// until put away.
func TestWaitCards(t *testing.T) {
	a, tt := newTestApp(t)
	a.embedTotal, a.embedLeft, a.embedSince = 86, 40, a.now().Add(-8*time.Second)
	a.orgResult = "整理了 9 条资料"
	a.showLibrary()
	a.libHome = true
	tt.Frame()
	for _, want := range []string{"建立语义索引", "整理了 9 条资料"} {
		if !tt.HasText(want) {
			t.Fatalf("no card %q: %q", want, tt.Texts())
		}
	}
	if js := a.jobs(); len(js) != 2 || js[0].frac < 0.5 || js[0].frac > 0.55 || !js[1].done {
		t.Fatalf("jobs %+v", js)
	}
	snapshot(t, tt, "wait-cards")
	if err := tt.Click("关闭 整理了 9 条资料"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("整理了 9 条资料") {
		t.Fatal("the card put away still shows")
	}
	// A run of clips counts itself: each begun and each done.
	a.orgTotal, a.orgDone = 0, 0
	a.cfg.LibraryManual = true
	for _, s := range []string{"一", "二", "三"} {
		cl := &library.Clip{Kind: library.KindLink, URL: "https://example.com/" + s}
		if err := a.lib.Add(cl, a.now()); err != nil {
			t.Fatal(err)
		}
	}
	if left := timeLeft(a.now(), a.now().Add(-10*time.Second), 0.5); left != "约剩 15 秒" {
		t.Fatalf("time left %q", left)
	}
	if left := timeLeft(a.now(), a.now(), 0.5); left != "正在估算…" {
		t.Fatalf("time left at once %q", left)
	}
}

// TestWaitStagesAndShapes: a wait that cannot be counted says what it is
// doing; what the AI writes of a clip stands as shapes where it will be;
// what failed stays said, with the way to try again.
func TestWaitStagesAndShapes(t *testing.T) {
	a, tt := newTestApp(t)
	if _, err := a.addMemo("换一家云服务商", "手动"); err != nil {
		t.Fatal(err)
	}
	a.openTasks()
	a.tasks.view = tasksWaiting
	a.clipPlacing = true
	tt.Frame()
	if !tt.HasText("AI 正在给每条找去处…") {
		t.Fatalf("no stage: %q", tt.Texts())
	}
	snapshot(t, tt, "wait-stages")
	a.clipPlacing, a.clipPlaceErr = false, "AI 没有给出建议：超时"
	tt.Frame()
	if !tt.HasText("AI 没有给出建议：超时") || !tt.HasText("重试") {
		t.Fatalf("the failure is not said with a way back: %q", tt.Texts())
	}
	// A clip being organized: shapes in its own part, nothing over its title.
	clip := a.lib.Clips[0]
	a.openList(kindAll, "")
	a.libSel = clip.ID
	a.libStatus[clip.ID] = "AI 整理中…"
	tt.Frame()
	if !tt.HasText("正在载入") || !tt.HasText("AI 整理中…") {
		t.Fatalf("no shapes: %q", tt.Texts())
	}
	snapshot(t, tt, "wait-shapes")
	// The AI's answer can be asked for again when it is the last said.
	a.ai.msgs = []chatMsg{{role: "user", text: "问"}, {role: "error", text: "网络错误"}}
	if !a.canRetryAI(1) || a.canRetryAI(0) {
		t.Fatal("which message can be retried")
	}
}
