package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// growFixture is a folder with the map 产品调研, which has a topic 待整理,
// and 收集 with six clips tagged 定价 (one holding a key), one more of the
// tag sealed whole, and a lone clip tagged 日历.
func growFixture(t *testing.T, a *app) (path string, clips []*library.Clip, locked *library.Clip) {
	t.Helper()
	path = filepath.Join(a.dir, model.FileName("产品调研"))
	f := model.NewFile("产品调研")
	for _, s := range []string{"竞品", "用户访谈", "待整理"} {
		f.Sheets[0].Root.Children = append(f.Sheets[0].Root.Children, model.NewNode(s))
	}
	if err := f.Save(path); err != nil {
		t.Fatal(err)
	}
	a.refreshFiles()
	if err := a.open(path); err != nil {
		t.Fatal(err)
	}
	add := func(cl *library.Clip, age int) *library.Clip {
		t.Helper()
		if err := a.lib.Add(cl, a.now()); err != nil {
			t.Fatal(err)
		}
		cl.Created = testNow.Add(-time.Duration(age) * 24 * time.Hour)
		return cl
	}
	tag := []string{"定价"}
	clips = []*library.Clip{
		add(&library.Clip{Kind: library.KindLink, URL: "https://example.com/notion-pricing", Title: "Notion 的套餐页", Desc: "按人收费，免费版限 block 数；团队版起步 8 美元一人一月", Tags: tag}, 1),
		add(&library.Clip{Kind: library.KindText, Text: "免费版给多少才不亏：同步是成本大头，免费版只给本机可能是条线", Tags: tag}, 2),
		add(&library.Clip{Kind: library.KindLink, URL: "https://example.com/ticktick", Title: "滴答清单会员对比", Desc: "日历视图和提醒次数放在会员里", Tags: tag}, 3),
		add(&library.Clip{Kind: library.KindText, Title: "访谈：价格那一段", Text: "“一次买断我会直接付，订阅要想想”。后台的 key：sk-abcdefghijklmnopqrstuvwxyz0123456789abcd", Tags: tag}, 4),
		add(&library.Clip{Kind: library.KindText, Text: "买断加可选订阅：软件买断，同步和 AI 按年订", Tags: tag}, 5),
		add(&library.Clip{Kind: library.KindLink, URL: "https://example.com/obsidian-sync", Title: "Obsidian Sync 定价", Desc: "软件免费，同步单独收费", Tags: tag}, 6),
	}
	locked = add(&library.Clip{Kind: library.KindText, Title: "内部报价单", Text: "渠道底价九十九", Tags: tag, Locked: true}, 7)
	add(&library.Clip{Kind: library.KindLink, URL: "https://example.com/week", Title: "滴答清单的周视图", Tags: []string{"日历"}}, 8)
	return path, clips, locked
}

// growReply is the outline the AI answers with, of the clips given: as
// the draft's, with an ID it was never given among its sources.
func growReply(clips []*library.Clip) string {
	id := func(is ...int) []string {
		var out []string
		for _, i := range is {
			out = append(out, clips[i].ID)
		}
		return out
	}
	b, _ := json.Marshal(map[string]any{
		"name": "定价怎么定",
		"topics": []map[string]any{
			{"title": "别家怎么收", "note": "按人、按功能、软件免费而同步收费，三种都有。", "src": append(id(0, 2, 5), "nosuchclip")},
			{"title": "用户对订阅的反应", "src": id(3)},
			{"title": "可以考虑：买断 + 同步订阅", "src": id(1, 4, 5), "children": []map[string]any{
				{"title": "免费版只给本机", "src": id(1)},
			}},
			{"title": "市场规模有一百亿", "src": []string{"madeup"}},
		},
		"gap": "老用户已经付过的钱怎么算",
	})
	return string(b)
}

func TestGrowBranch(t *testing.T) {
	a, tt := newTestApp(t)
	path, clips, locked := growFixture(t, a)
	a.showLibrary()

	// A tag with one clip has no key; nor has 收集 with no tag chosen.
	tt.Frame()
	if _, ok := tt.Find("长成分支"); ok {
		t.Fatal("the key with no tag chosen")
	}
	a.libTag = "日历"
	tt.Frame()
	if _, ok := tt.Find("长成分支"); ok {
		t.Fatal("the key for one clip")
	}
	a.libTag = "定价"
	tt.Frame()
	if _, ok := tt.Find("长成分支"); !ok {
		t.Fatalf("no key: %q", tt.Texts())
	}
	snapshot(t, tt, "grow-key")

	fake := &fakeClaude{replies: []string{claudeText(growReply(clips))}}
	a.sendAI = fake.send
	a.cfg.AIKey = "sk-ant-test"
	tt.Frame()
	if err := tt.Click("长成分支"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	g := &a.collect.grow
	if g.out == nil || g.err != "" {
		t.Fatalf("no outline: %q", g.err)
	}
	// The AI was given each clip as an AI reads one, its key in dots, and
	// not the one sealed whole.
	prompt := fake.requests[0].Messages[0].Content[0].OfText.Text
	for _, cl := range clips {
		if !strings.Contains(prompt, "["+cl.ID+"] "+cl.Label()) {
			t.Fatalf("clip %q not in the prompt:\n%s", cl.Label(), prompt)
		}
	}
	if strings.Contains(prompt, "sk-abcdef") || strings.Contains(prompt, locked.ID) || strings.Contains(prompt, "渠道底价") || strings.Contains(prompt, "周视图") {
		t.Fatalf("the prompt holds what it must not:\n%s", prompt)
	}
	if !strings.Contains(prompt, "一次买断我会直接付") {
		t.Fatalf("prompt:\n%s", prompt)
	}
	// Cleaned: the ID never given is gone, and the topic of no clip.
	if len(g.out.Topics) != 5 || len(g.out.Topics[1].Src) != 3 || g.out.Topics[4].Deep != 2 {
		t.Fatalf("outline %+v", g.out.Topics)
	}
	// The sheet: from what, to where, the outline, what is unsaid.
	for _, s := range []string{"从 #定价 的 6 条资料", "另有 1 条整条加密的，没有给 AI。", "产品调研 › 待整理", "定价怎么定", "别家怎么收",
		"3 条来源", "资料里没有说到：老用户已经付过的钱怎么算", "放到导图", "先不"} {
		if !tt.HasText(s) {
			t.Fatalf("the sheet lacks %q: %q", s, tt.Texts())
		}
	}
	if tt.HasText("市场规模有一百亿") {
		t.Fatal("a topic of no clip is shown")
	}
	before, _ := a.file.Marshal()

	// A line's sources open under it.
	if tt.HasText("Obsidian Sync 定价") && g.open >= 0 {
		t.Fatal("sources open before asked")
	}
	if err := tt.Click("来源 别家怎么收"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if g.open != 1 {
		t.Fatalf("open %d", g.open)
	}
	if _, ok := tt.Find("资料 Obsidian Sync 定价"); !ok {
		t.Fatalf("no sources: %q", tt.Texts())
	}
	snapshot(t, tt, "grow-sheet")
	tt.SetDark(true)
	tt.Frame()
	snapshot(t, tt, "grow-sheet-dark")
	tt.SetDark(false)
	tt.Frame()
	if now, _ := a.file.Marshal(); string(now) != string(before) {
		t.Fatal("the map changed before 放到导图")
	}

	// Another place is chosen through the palette, and the first again.
	if err := tt.Click("换个地方放"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.paletteOpen || a.paletteMode != paletteGrow || len(a.paletteItems()) == 0 {
		t.Fatalf("palette %v %d", a.paletteOpen, a.paletteMode)
	}
	a.paletteOpen = false
	a.growUnder(path, "", a.file.Sheets[0].Root.ID)
	tt.Frame()
	if label, _ := a.growPlaceLabel(g.path, g.node); label != "产品调研" {
		t.Fatalf("place %q", label)
	}
	wait := find(t, a, "待整理")
	a.growUnder(path, "", wait.ID)
	tt.Frame()

	// 放到导图: one step, each topic linking to its clips.
	if err := tt.Click("放到导图"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	wait = find(t, a, "待整理")
	if len(wait.Children) != 1 {
		t.Fatalf("under 待整理: %d", len(wait.Children))
	}
	top := wait.Children[0]
	if top.Title != "定价怎么定" || !slices.Equal(top.Links, []string{"tag:定价"}) || len(top.Children) != 3 || !strings.Contains(top.Note, "老用户") {
		t.Fatalf("top %+v", top)
	}
	first := top.Children[0]
	if !slices.Equal(first.Links, []string{"clip:" + clips[0].ID, "clip:" + clips[2].ID, "clip:" + clips[5].ID}) || !strings.Contains(first.Note, "三种都有") {
		t.Fatalf("first %+v", first)
	}
	if len(top.Children[2].Children) != 1 || top.Children[2].Children[0].Links[0] != "clip:"+clips[1].ID {
		t.Fatalf("third %+v", top.Children[2])
	}
	if a.notice != "已把 #定价 长成分支" || a.noticeUndo == nil {
		t.Fatalf("notice %q", a.notice)
	}
	// The head says where it is, in the key's place; the sheet, the way there.
	if !tt.HasText("已在「产品调研」") || !tt.HasText("去看看") {
		t.Fatalf("after: %q", tt.Texts())
	}
	if _, ok := tt.Find("再长一次"); ok {
		t.Fatal("再长一次 with nothing new")
	}
	if _, ok := tt.Find("长成分支"); !ok { // the sheet's name
		t.Fatal("the sheet went")
	}
	// A clip's details say the topic.
	a.libSel = clips[3].ID
	tt.Frame()
	if back := a.clipBacklinks(clips[3].ID); len(back) != 1 || back[0].title != "用户对订阅的反应" || !tt.HasText("用在 1 处") {
		t.Fatalf("backlinks %+v: %q", back, tt.Texts())
	}
	snapshot(t, tt, "grow-clip")
	a.libSel = ""
	tt.Frame()
	snapshot(t, tt, "grow-done")

	// 去看看 opens the map at the branch.
	if err := tt.Click("去看看"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.libOpen || a.path != path || a.selected != top.ID || a.collect.grow.tag != "" {
		t.Fatalf("not gone there: lib %v selected %q", a.libOpen, a.selected)
	}
	snapshot(t, tt, "grow-map")
	a.selectNode(first.ID) // a topic of it shows the clips it came from
	tt.Frame()
	if !tt.HasText("Notion 的套餐页") {
		t.Fatalf("the topic's clips: %q", tt.Texts())
	}
	snapshot(t, tt, "grow-map-topic")
	// Its link goes back to the tag in 收集.
	a.openLink(top.Links[0])
	tt.Frame()
	if !a.collectShown() || a.libTag != "定价" || !tt.HasText("已在「产品调研」") {
		t.Fatalf("the tag's link: %q", tt.Texts())
	}
	snapshot(t, tt, "grow-chip")

	// A clip of the tag the branch does not link to: the key again.
	late := &library.Clip{Kind: library.KindText, Text: "按年订比按月订便宜两成是常见的做法", Tags: []string{"定价"}}
	if err := a.lib.Add(late, a.now()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, ok := tt.Find("再长一次"); !ok {
		t.Fatalf("no 再长一次: %q", tt.Texts())
	}

	// 撤销 takes that branch out, whatever was done since; the key is back.
	a.mutate("", func() { find(t, a, "竞品").Title = "竞品们" })
	a.noticeUndo()
	tt.Frame()
	if n := len(find(t, a, "待整理").Children); n != 0 {
		t.Fatalf("still %d under 待整理", n)
	}
	find(t, a, "竞品们") // the edit made since stays
	if tt.HasText("已在「产品调研」") || len(a.clipBacklinks(clips[3].ID)) != 0 {
		t.Fatalf("the chip stays: %q", tt.Texts())
	}
	if _, ok := tt.Find("长成分支"); !ok {
		t.Fatal("the key is not back")
	}
}

// A branch grown on a map that is not open is written to its file, and
// taken out of it.
func TestGrowOnAnotherMap(t *testing.T) {
	a, tt := newTestApp(t)
	path, clips, _ := growFixture(t, a)
	other := filepath.Join(a.dir, model.FileName("其他"))
	if err := model.NewFile("其他").Save(other); err != nil {
		t.Fatal(err)
	}
	a.refreshFiles()
	if err := a.open(other); err != nil {
		t.Fatal(err)
	}
	a.sendAI = (&fakeClaude{replies: []string{claudeText(growReply(clips))}}).send
	a.cfg.AIKey = "sk-ant-test"
	a.showLibrary()
	a.libTag = "定价"
	tt.Frame()
	a.openGrow()
	tt.Frame()
	g := &a.collect.grow
	if label, _ := a.growPlaceLabel(g.path, g.node); label != "其他" { // the map opened last, its centre
		t.Fatalf("place %q", label)
	}
	f, err := model.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	a.growUnder(path, "", f.Sheets[0].Root.Children[2].ID)
	a.plantGrow()
	tt.Frame()
	grownIn := func() int {
		f, err := model.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		return len(f.Sheets[0].Root.Children[2].Children)
	}
	if grownIn() != 1 || !tt.HasText("已在「产品调研」") || len(a.clipBacklinks(clips[0].ID)) != 1 {
		t.Fatalf("grown %d: %q", grownIn(), tt.Texts())
	}
	a.noticeUndo()
	tt.Frame()
	if grownIn() != 0 || tt.HasText("已在「产品调研」") {
		t.Fatalf("after 撤销 %d", grownIn())
	}
}

func TestParseGrow(t *testing.T) {
	known := map[string]bool{"a": true, "b": true, "c": true}
	var topics []string
	for i := range 20 {
		topics = append(topics, fmt.Sprintf(`{"title":"主题 %d","src":["a","zz","a"]}`, i))
	}
	long := strings.Repeat("很长的标题", 8)
	text := "好的：\n```json\n" + `{"name":"  分支  名 ","topics":[
		{"title":"` + long + `","note":"细节","src":["[b]"]},
		{"title":"没有来源","src":["zz"]},
		{"title":"一组","children":[{"title":"下面的","src":["c"],"children":[{"title":"第三层","src":["a"]}]},{"title":"编的","src":[]}]},
		` + strings.Join(topics, ",") + `],"gap":"没说到的"}` + "\n```"
	out, err := parseGrow(text, known)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Topics) != growTopics+1 || out.Topics[0].Title != "分支 名" || out.Topics[0].Deep != 0 || len(out.Topics[0].Src) != 0 || out.Gap != "没说到的" {
		t.Fatalf("%d topics, name %q", len(out.Topics), out.Topics[0].Title)
	}
	first := out.Topics[1]
	if len([]rune(first.Title)) != growTitle+1 || !strings.HasPrefix(first.Note, long) || !strings.HasSuffix(first.Note, "细节") || !slices.Equal(first.Src, []string{"b"}) {
		t.Fatalf("first %+v", first)
	}
	// A topic with none of its own comes from what is under it; a third
	// level is its parent's sources; one of no clip is not kept.
	group, under := out.Topics[2], out.Topics[3]
	if group.Title != "一组" || !slices.Equal(group.Src, []string{"c", "a"}) || under.Deep != 2 || !slices.Equal(under.Src, []string{"c", "a"}) {
		t.Fatalf("group %+v under %+v", group, under)
	}
	for _, tp := range out.Topics[1:] {
		if len(tp.Src) == 0 || slices.Contains(tp.Src, "zz") || tp.Title == "没有来源" || tp.Title == "编的" || tp.Title == "第三层" {
			t.Fatalf("kept %+v", tp)
		}
	}
	if !slices.Equal(out.Topics[4].Src, []string{"a"}) {
		t.Fatalf("sources twice: %+v", out.Topics[4])
	}
	for _, bad := range []string{"画不了", `{"name":"x","topics":[{"title":"编的","src":["zz"]}]}`, `{"topics":"none"}`} {
		if _, err := parseGrow(bad, known); err == nil {
			t.Fatalf("%q read as an outline", bad)
		}
	}
}

// With no AI set up the key leads to its settings, and nothing opens.
func TestGrowNeedsAI(t *testing.T) {
	a, tt := newTestApp(t)
	growFixture(t, a)
	a.showLibrary()
	a.libTag = "定价"
	tt.Frame()
	if err := tt.Click("长成分支"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.collect.grow.tag != "" || !a.aiOpen || !strings.Contains(a.notice, "AI") {
		t.Fatalf("with no key: %+v, %q", a.collect.grow, a.notice)
	}
}
