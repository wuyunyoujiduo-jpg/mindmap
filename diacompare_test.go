package main

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// syncSet is what the AI answers of the branch syncBranch builds, the
// draft's: three ways to sync, side by side. Two cells the map does not
// speak of, a row in which all say the same, and a row that judges, which
// it was told not to write.
const syncSet = `{
 "compare": {"options": ["GitHub 仓库", "iCloud 文件夹", "自己搭服务"],
  "rows": [
   {"dim": "两边都改了怎么办", "cells": [{"text": "按主题三方合并，改到同一处才问", "src": [3]}, {"text": "系统留两份文件，自己比", "src": [10]}, {"text": "可以做到实时合并", "src": [16]}]},
   {"dim": "另一台不是 Mac", "cells": [{"text": "能用", "src": [4]}, {"text": "不能用", "src": [11]}, {"text": "能用", "src": [17]}]},
   {"dim": "历史版本", "cells": [{"text": "每一轮同步都是一个提交，随时能找回", "src": [5]}, {"gap": true}, {"text": "要自己做", "src": [18]}]},
   {"dim": "断网时", "same": true, "cells": [{"text": "照常用，联网后补一轮", "src": [6]}, {"text": "照常用，联网后补", "src": [12]}, {"text": "照常用，联网后补", "src": [19]}]},
   {"dim": "用户要做什么", "cells": [{"text": "建一个私有仓库，填一次令牌", "src": [7]}, {"text": "把文件夹放进 iCloud", "src": [13]}, {"gap": true}]},
   {"dim": "费用", "cells": [{"text": "免费，每小时调用次数有上限", "src": [8]}, {"text": "占自己的 iCloud 空间", "src": [14]}, {"text": "服务器按月付钱", "src": [20]}]},
   {"dim": "推荐", "cells": [{"text": "首选，最省心"}, {"text": "其次"}, {"text": "不建议"}]}]},
 "objects": [{"name": "令牌", "kind": "概念", "what": "GitHub 给的一串字，让 App 能读写仓库。"},
  {"name": "提交", "kind": "概念", "what": "仓库里的一次改动记录。"}]
}`

// syncBranch puts the draft's branch under the map's root: the options,
// and under each what the map says of it.
func syncBranch(t *testing.T, a *app, more ...[]string) *model.Node {
	t.Helper()
	add := func(parent *model.Node, title string) *model.Node {
		n, err := a.doc.AddChild(parent.ID, title)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	root := add(a.doc.Root, "选哪种同步")
	for _, part := range append([][]string{
		{"GitHub 仓库", "按主题三方合并，改到同一处才问", "另一台不是 Mac 也能用", "每一轮同步都是一个提交，随时能找回", "断网照常用，联网后补一轮", "建一个私有仓库，填一次令牌", "免费，每小时调用次数有上限"},
		{"iCloud 文件夹", "冲突时系统留两份文件", "只有苹果的设备能用", "断网照常用，联网后补", "把文件夹放进 iCloud", "占自己的 iCloud 空间"},
		{"自己搭服务", "可以做到实时合并", "哪种电脑都能用", "历史版本要自己做", "断网也照常用", "服务器按月付钱"},
	}, more...) {
		p := add(root, part[0])
		for _, leaf := range part[1:] {
			add(p, leaf)
		}
	}
	a.edits++
	return root
}

func TestDiagramCompare(t *testing.T) {
	a, tt := newTestApp(t)
	fake := &fakeClaude{replies: []string{claudeText(syncSet)}}
	a.sendAI = fake.send
	a.cfg.AIKey = "sk-ant-test"
	root := syncBranch(t, a)
	a.selectNode(root.ID)
	a.drawDia(root.ID, "")
	tt.Frame()
	f := a.diaOf(root.ID)
	if f == nil || f.Compare == nil {
		t.Fatalf("set %+v, err %q", f, a.diaErr)
	}
	// The AI is asked for the table, told to leave open what the map does
	// not say, and to judge nothing.
	prompt := fake.requests[0].Messages[0].Content[0].OfText.Text
	for _, want := range []string{"[9] iCloud 文件夹", "flow、state、table、objects、cases、gaps、compare 这七个", "7. compare：对比表", `{"gap":true}`, "不要推测",
		"不说哪个方案更好", "不排名", "不打分", "不写“结论”“推荐”", "选哪个由用户自己定"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("the prompt lacks %q", want)
		}
	}
	// The row that judges is not kept; a cell names its topic.
	tb := f.Compare
	if len(tb.Rows) != 6 || tb.Gaps() != 2 || tb.Sames() != 1 || tb.Rows[2].Cells[0].Src[0] != find(t, a, "每一轮同步都是一个提交，随时能找回").ID {
		t.Fatalf("table %+v", tb)
	}
	if !slices.Equal(f.Kinds(), []string{"objects", "compare"}) {
		t.Fatalf("kinds %v", f.Kinds())
	}

	// Its tab is the last of the kinds; the table: the options, what they
	// are compared by, what is unsaid dashed, and the user's own row.
	tt.Frame()
	if err := tt.Click("图解的对比"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.dia.kind != "compare" || !tt.HasText("自己搭服务") || !tt.HasText("两边都改了怎么办") || !tt.HasText("导图里没说") ||
		!tt.HasText("我的倾向") || !tt.HasText("点一下标出") || tt.HasText("首选，最省心") || tt.HasText("推荐") {
		t.Fatalf("the table: %q", tt.Texts())
	}
	// Nothing chosen: what to look at next, and a line to each cell unsaid.
	if !tt.HasText("接下来看什么") || !tt.HasText("格没说") || !tt.HasText("行都一样") || !tt.HasText("让 AI 改图") {
		t.Fatalf("the right column: %q", tt.Texts())
	}
	snapshot(t, tt, "diagram-compare")
	if err := tt.Click("没说 自己搭服务的用户要做什么"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.dia.pick != "cell:5:3" || !tt.HasText("导图里没有说到这一格。AI 不替你填：去导图里补上，再重画。") || !tt.HasText("去导图补上") {
		t.Fatalf("pick %q: %q", a.dia.pick, tt.Texts())
	}
	if err := tt.Click("历史版本：iCloud 文件夹"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.dia.pick != "cell:3:2" {
		t.Fatalf("pick %q", a.dia.pick)
	}
	snapshot(t, tt, "diagram-compare-cell")
	// A cell says the topics it came from; Escape leaves it.
	tt.Key(0, ui.KeyLeft)
	tt.Frame()
	if a.dia.pick != "cell:3:1" || !tt.HasText("来自这些主题") || !tt.HasText("主题 每一轮同步都是一个提交，随时能找回") {
		t.Fatalf("pick %q by the arrow: %q", a.dia.pick, tt.Texts())
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.dia.pick != "" || a.mode != viewDiagram {
		t.Fatal("Escape did not leave the cell alone")
	}

	// 只看有差别的 puts away the row in which all say the same.
	if !tt.HasText("断网时") {
		t.Fatal("the row the same is not shown")
	}
	if err := tt.Click("只看有差别的"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.dia.diff || tt.HasText("断网时") || !tt.HasText("费用") {
		t.Fatalf("the rows the same stayed: %q", tt.Texts())
	}
	// Told a row a sentence, of the rows that show.
	if err := tt.Click("讲一遍"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.dia.way != diaTell || a.dia.kind != "compare" || !tt.HasText("1 / 5") ||
		!tt.HasText("两边都改了怎么办：GitHub 仓库，按主题三方合并，改到同一处才问；iCloud 文件夹，系统留两份文件，自己比；自己搭服务，可以做到实时合并。") {
		t.Fatalf("told: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyRight)
	tt.Key(0, ui.KeyRight)
	tt.Frame()
	if a.dia.step != 2 || !tt.HasText("讲一遍 · 第 3 行") || !tt.HasText("历史版本：GitHub 仓库，每一轮同步都是一个提交，随时能找回；iCloud 文件夹，导图里没说；自己搭服务，要自己做。") {
		t.Fatalf("step %d: %q", a.dia.step, tt.Texts())
	}
	if lit := a.diaLight(f); !lit.rows[3] || lit.rows[1] {
		t.Fatalf("lit %+v", lit)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if err := tt.Click("只看有差别的"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.dia.diff || a.dia.way != diaRead || !tt.HasText("断网时") {
		t.Fatal("the rows the same did not come back")
	}
	if got := compareSay(tb, 3); got != "断网时：3 个方案都一样，照常用，联网后补一轮。" {
		t.Fatalf("a row the same is told %q", got)
	}

	// Asked back: a column covered, but for what the map does not say and
	// what the others say too; a click on a cell shows it.
	if err := tt.Click("考我"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.dia.way != diaQuiz || a.dia.cover != 1 || tt.HasText("按主题三方合并，改到同一处才问") || tt.HasText("免费，每小时调用次数有上限") ||
		!tt.HasText("照常用，联网后补一轮") || !tt.HasText("系统留两份文件，自己比") || !tt.HasText("「GitHub 仓库」这一列盖住了。先自己说，再点格子看导图里是怎么写的。") {
		t.Fatalf("the column is not covered: %q", tt.Texts())
	}
	if err := tt.Click("断网时：GitHub 仓库"); err != nil { // not asked: the others say it
		t.Fatal(err)
	}
	tt.Frame()
	if a.dia.cur != 0 {
		t.Fatal("a row the same was asked back")
	}
	for i, label := range []string{"两边都改了怎么办", "另一台不是 Mac", "历史版本", "用户要做什么", "费用"} {
		if err := tt.Click(label + "：GitHub 仓库"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if a.dia.cur == 0 || !a.dia.open[a.dia.cur] {
			t.Fatalf("%s not opened by its click", label)
		}
		if i == 0 {
			if !tt.HasText("按主题三方合并，改到同一处才问") || tt.HasText("免费，每小时调用次数有上限") || !tt.HasText("说对了吗？") {
				t.Fatalf("the cell opened does not show alone: %q", tt.Texts())
			}
			snapshot(t, tt, "diagram-compare-quiz")
			if err := tt.Click("没想起来"); err != nil {
				t.Fatal(err)
			}
			continue
		}
		a.answerQuiz(true)
	}
	tt.Frame()
	f = a.diaOf(root.ID)
	if q := f.CompareQuiz; q == nil || q.Col != 1 || !slices.Equal(q.Missed, []int{1}) || f.Quiz != nil ||
		!tt.HasText("5 格里想起 4 格。没想起来的，3 天后「今天」会提醒你再考。") {
		t.Fatalf("quiz %+v: %q", f.CompareQuiz, tt.Texts())
	}
	if a.quizDue(f) {
		t.Fatal("due at once")
	}
	now := testNow
	clock = func() time.Time { return now.AddDate(0, 0, 3) }
	a.now = clock
	if a.dueKind(f) != "compare" || len(a.diasDue()) != 1 {
		t.Fatal("not due after three days")
	}
	a.goToday()
	tt.Frame()
	if !tt.HasText("「选哪种同步」的对比该再考了") {
		t.Fatalf("no line on 今天: %q", tt.Texts())
	}
	if err := tt.Click("该再考的对比"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.mode != viewDiagram || a.dia.way != diaQuiz || a.dia.kind != "compare" || a.dia.cover != 1 { // the column missed, again
		t.Fatalf("not asked back: mode %d way %d kind %q cover %d", a.mode, a.dia.way, a.dia.kind, a.dia.cover)
	}
	clock = func() time.Time { return now }
	a.now = clock
	if err := tt.Click("换一列"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.dia.cover != 2 || tt.HasText("系统留两份文件，自己比") || !tt.HasText("按主题三方合并，改到同一处才问") {
		t.Fatalf("cover %d: %q", a.dia.cover, tt.Texts())
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()

	// 我的倾向: a click marks an option, a click again takes the mark off;
	// one at most. It is kept with the set, beside what was drawn.
	if err := tt.Click("倾向 自己搭服务"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if f = a.diaOf(root.ID); f.Lean != "自己搭服务" || !tt.HasText("倾向这个") || tt.HasText("点一下标出") {
		t.Fatalf("lean %q: %q", f.Lean, tt.Texts())
	}
	if err := tt.Click("倾向 自己搭服务"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if f = a.diaOf(root.ID); f.Lean != "" || tt.HasText("倾向这个") {
		t.Fatalf("the mark stayed: %q", f.Lean)
	}
	bare, err := a.diaImage(f, "compare", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("倾向 自己搭服务"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("倾向 iCloud 文件夹"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	f = a.diaOf(root.ID)
	data, _ := os.ReadFile(filepath.Join(a.diaDir(), root.ID+".json"))
	if f.Lean != "iCloud 文件夹" || !strings.Contains(string(data), `"lean": "iCloud 文件夹"`) || strings.Contains(string(data), "倾向这个") {
		t.Fatalf("lean %q, kept as %s", f.Lean, data)
	}

	// A noun lights the cells that speak of it.
	a.dia.noun = 1
	tt.Frame()
	if lit := a.diaLight(f); len(lit.cells) != 1 || !lit.cells["cell:5:1"] || !tt.HasText("只亮提到「令牌」的") {
		t.Fatalf("noun lit %+v", lit)
	}
	a.dia.noun = 0
	if err := tt.Click("费用：iCloud 文件夹"); err != nil {
		t.Fatal(err)
	}
	tt.SetDark(true)
	tt.Frame()
	tt.Frame()
	snapshot(t, tt, "diagram-compare-dark")
	tt.SetDark(false)

	// As a picture: bare, with the user's row only while an option is marked.
	marked, err := a.diaImage(f, "compare", 0)
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := png.DecodeConfig(bytes.NewReader(bare))
	with, _ := png.DecodeConfig(bytes.NewReader(marked))
	if plain.Width != (760+56)*2 || plain.Height < 600 || with.Height <= plain.Height+40 {
		t.Fatalf("the picture is %dx%d, marked %dx%d", plain.Width, plain.Height, with.Width, with.Height)
	}
	if dir := os.Getenv("MINDMAP_SNAPSHOTS"); dir != "" {
		_ = os.WriteFile(filepath.Join(dir, "diagram-picture-compare.png"), bare, 0o644)
		_ = os.WriteFile(filepath.Join(dir, "diagram-picture-compare-lean.png"), marked, 0o644)
	}
	// Among the slides, a slide of its own.
	before := len(a.slidesFor(a.doc))
	a.presentDia(f, true)
	if slides := a.slidesFor(a.doc); len(slides) != before+1 || !slices.ContainsFunc(slides, func(s slide) bool { return s.diaOf == "compare" }) {
		t.Fatalf("%d slides, %d before", len(slides), before)
	}
	a.presentDia(f, false)

	// The map changed since: said, and the cell drawn from it is marked.
	topic := find(t, a, "服务器按月付钱")
	a.mutate("title:"+topic.ID, func() { topic.Title = "服务器按年付钱" })
	tt.Frame()
	if ch := a.diaChanges(f); ch.n != 1 || !ch.changed[topic.ID] || !tt.HasText("画完之后导图改过 1 处：「服务器按年付钱」。图可能旧了。") {
		t.Fatalf("changes %+v: %q", a.diaChanges(f), tt.Texts())
	}

	// Drawn again, on the kind looked at: the mark is the user's and stays.
	fake.replies = []string{claudeText(strings.ReplaceAll(syncSet, "服务器按月付钱", "服务器按年付钱"))}
	if err := tt.Click("重画"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if f = a.diaOf(root.ID); f.Compare.Rows[5].Cells[2].Text != "服务器按年付钱" || f.Lean != "iCloud 文件夹" || a.dia.kind != "compare" || !tt.HasText("倾向这个") {
		t.Fatalf("redrawn: %+v, lean %q, kind %q", f.Compare.Rows[5], f.Lean, a.dia.kind)
	}
	// Drawn without its option, the mark goes with it; taken back, it is there.
	fake.replies = []string{claudeText(strings.ReplaceAll(syncSet, `"iCloud 文件夹"`, `"苹果的云"`))}
	a.drawDia(root.ID, "把 iCloud 叫成苹果的云")
	tt.Frame()
	if f = a.diaOf(root.ID); f.Compare.Options[1] != "苹果的云" || f.Lean != "" || tt.HasText("倾向这个") || f.Told != "把 iCloud 叫成苹果的云" {
		t.Fatalf("redrawn without the option: %v, lean %q", f.Compare.Options, f.Lean)
	}
	a.noticeUndo()
	tt.Frame()
	if f = a.diaOf(root.ID); f.Lean != "iCloud 文件夹" {
		t.Fatalf("not taken back: lean %q", f.Lean)
	}

	// A click on a topic a cell came from goes to it in the map.
	a.openDia(root.ID, "compare")
	tt.Frame()
	if err := tt.Click("历史版本：GitHub 仓库"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("主题 每一轮同步都是一个提交，随时能找回"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.mode != viewMap || a.selected != find(t, a, "每一轮同步都是一个提交，随时能找回").ID {
		t.Fatalf("mode %d selected %q", a.mode, a.selected)
	}
}

// More options than stand side by side are a card each down the page.
func TestDiagramCompareCards(t *testing.T) {
	a, tt := newTestApp(t)
	five := strings.NewReplacer(
		`["GitHub 仓库", "iCloud 文件夹", "自己搭服务"]`, `["GitHub 仓库", "iCloud 文件夹", "自己搭服务", "坚果云", "Syncthing"]`,
		`{"text": "可以做到实时合并", "src": [16]}]`, `{"text": "可以做到实时合并", "src": [16]}, {"text": "留冲突文件", "src": [22]}, {"text": "留冲突文件", "src": [27]}]`,
		`{"text": "能用", "src": [17]}]`, `{"text": "能用", "src": [17]}, {"text": "能用", "src": [23]}, {"text": "能用", "src": [28]}]`,
		`{"text": "要自己做", "src": [18]}]`, `{"text": "要自己做", "src": [18]}, {"text": "留一个月的旧版本", "src": [24]}, null]`,
		`{"text": "照常用，联网后补", "src": [19]}]`, `{"text": "照常用，联网后补", "src": [19]}, {"text": "照常用，联网后补"}, {"text": "照常用，联网后补"}]`,
		`{"text": "服务器按月付钱", "src": [20]}]`, `{"text": "服务器按月付钱", "src": [20]}, {"text": "免费的空间很小", "src": [25]}, {"text": "免费", "src": [29]}]`,
	).Replace(syncSet)
	fake := &fakeClaude{replies: []string{claudeText(five)}}
	a.sendAI = fake.send
	a.cfg.AIKey = "sk-ant-test"
	root := syncBranch(t, a,
		[]string{"坚果云", "两边都改了留冲突文件", "哪种电脑都有客户端", "留一个月的旧版本", "免费的空间很小"},
		[]string{"Syncthing", "两边都改了留冲突文件", "电脑和安卓都能用", "不要钱"})
	a.selectNode(root.ID)
	a.drawDia(root.ID, "")
	tt.Frame()
	f := a.diaOf(root.ID)
	if f == nil || f.Compare == nil || len(f.Compare.Options) != 5 {
		t.Fatalf("set %+v, err %q", f, a.diaErr)
	}
	a.openDia(root.ID, "compare")
	tt.Frame()
	// A card an option: its name once, the rows as its lines, and its key.
	n := 0
	for _, s := range tt.Texts() {
		if s == "用户要做什么" {
			n++
		}
	}
	if n != 5 || !tt.HasText("Syncthing") || !tt.HasText("标为倾向") || tt.HasText("我的倾向") {
		t.Fatalf("%d cards' lines: %q", n, tt.Texts())
	}
	if err := tt.Click("倾向 iCloud 文件夹"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("历史版本：iCloud 文件夹"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if f = a.diaOf(root.ID); f.Lean != "iCloud 文件夹" || a.dia.pick != "cell:3:2" || !tt.HasText("倾向这个") || !tt.HasText("导图里没有说到这一格。AI 不替你填：去导图里补上，再重画。") {
		t.Fatalf("lean %q pick %q: %q", f.Lean, a.dia.pick, tt.Texts())
	}
	snapshot(t, tt, "diagram-compare-cards")
	if err := tt.Click("只看有差别的"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("断网时") || !tt.HasText("费用") {
		t.Fatalf("the rows the same stayed on the cards: %q", tt.Texts())
	}
	if data, err := a.diaImage(f, "compare", 0); err != nil || len(data) == 0 {
		t.Fatal(err)
	} else if dir := os.Getenv("MINDMAP_SNAPSHOTS"); dir != "" {
		_ = os.WriteFile(filepath.Join(dir, "diagram-picture-compare-cards.png"), data, 0o644)
	}
}
