package main

import (
	"bytes"
	"fmt"
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

// refundSet is what the AI answers of the branch refundBranch builds: its
// sources are the numbers the outline gives the topics, in order.
const refundSet = `{
 "flow": {"lanes": ["门店", "业务员 · 主管", "仓库 · 司机", "财务"],
  "nodes": [
   {"id": "A", "kind": "start", "title": "申请退货", "lane": 0, "src": [1]},
   {"id": "B", "kind": "decision", "title": "符合申请条件？", "lane": 0, "src": [3, 4]},
   {"id": "X", "kind": "end", "title": "不受理", "lane": 0, "src": [3]},
   {"id": "C", "kind": "decision", "title": "质量问题？", "lane": 1, "src": [9]},
   {"id": "D", "kind": "step", "title": "审核", "sub": "≤500 元业务员，以上主管", "lane": 1, "src": [7, 8]},
   {"id": "G", "kind": "decision", "title": "审核通过？", "lane": 1, "src": [18]},
   {"id": "R", "kind": "end", "title": "驳回", "lane": 1, "src": [18]},
   {"id": "P", "kind": "step", "title": "取货", "sub": "司机上门，或门店送回", "lane": 2, "src": [11, 12]},
   {"id": "V", "kind": "step", "title": "验收", "lane": 2, "src": [20]},
   {"id": "M", "kind": "ok", "title": "退款", "sub": "现结原路退，账期冲应收", "lane": 3, "src": [14, 15]}],
  "edges": [{"from": "A", "to": "B"}, {"from": "B", "to": "X", "label": "否"}, {"from": "B", "to": "C", "label": "是"},
   {"from": "C", "to": "D", "label": "否"}, {"from": "C", "to": "P", "label": "是，免审"}, {"from": "D", "to": "G"},
   {"from": "G", "to": "R", "label": "否"}, {"from": "G", "to": "P", "label": "是"}, {"from": "P", "to": "V"}, {"from": "V", "to": "M"}],
  "tell": [{"nodes": ["A"], "say": "门店提交退货申请。"},
   {"nodes": ["B", "X"], "say": "先看能不能申请：签收 7 天内、不是临期品才行，否则不受理。"},
   {"nodes": ["C"], "say": "质量问题不用审核，直接去取货。"},
   {"nodes": ["D"], "say": "其他情况要审核：500 元以内业务员审，超过的主管审。"},
   {"nodes": ["G", "R"], "say": "审核没通过就驳回，到此结束。"},
   {"nodes": ["P"], "say": "通过了就取货：司机上门，或者门店自己送回。"},
   {"nodes": ["V"], "say": "货回到仓库，验收。"},
   {"nodes": ["M"], "say": "财务退款：现结客户原路退回，账期客户冲减应收。"}]},
 "state": {"nodes": [{"id": "s0", "kind": "start"}, {"id": "s1", "title": "待审核", "src": [17]}, {"id": "s2", "title": "待取货", "src": [19]},
   {"id": "s3", "title": "待验收", "src": [20]}, {"id": "s4", "title": "已退款", "kind": "ok", "src": [21]},
   {"id": "s5", "title": "已驳回", "kind": "bad", "src": [18]}, {"id": "s6", "title": "已关闭", "kind": "end", "src": [22]}],
  "edges": [{"from": "s0", "to": "s1", "label": "提交申请"}, {"from": "s0", "to": "s2", "label": "质量问题，免审"},
   {"from": "s1", "to": "s2", "label": "审核通过"}, {"from": "s2", "to": "s3", "label": "取到货"}, {"from": "s3", "to": "s4", "label": "验收通过"},
   {"from": "s1", "to": "s5", "label": "驳回"}, {"from": "s5", "to": "s1", "label": "改后重提"},
   {"from": "s2", "to": "s6", "label": "门店撤销"}, {"from": "s3", "to": "s6", "label": "货不对"}]},
 "table": {"conds": ["签收后", "商品", "原因", "金额"], "results": ["能不能退", "谁来审"],
  "rows": [
   {"when": ["7 天内", "正常", "不是质量问题", "≤ 500 元"], "then": ["可退", "业务员"], "src": [3, 7]},
   {"when": ["7 天内", "正常", "不是质量问题", "> 500 元"], "then": ["可退", "主管"], "src": [3, 8]},
   {"when": ["7 天内", "不限", "质量问题", "不限"], "then": ["可退", "免审，直接取货"], "src": [3, 9]},
   {"when": ["7 天内", "临期", "不是质量问题", "不限"], "then": ["不可退", "—"], "src": [4]},
   {"when": ["超过 7 天", "不限", "不是质量问题", "不限"], "then": ["不可退", "—"], "src": [3]},
   {"when": ["超过 7 天", "不限", "质量问题", "不限"], "then": [], "src": [3, 9], "gap": true,
    "why": "超过 7 天不能申请，质量问题又免审：超过 7 天才发现的质量问题，退不退？"}],
  "notes": [{"text": "赠品随主品一起退。", "src": [5]}]},
 "objects": [
  {"name": "业务员", "kind": "角色", "what": "审核 500 元以内的退货。"},
  {"name": "主管", "kind": "角色", "what": "审核超过 500 元的退货。"},
  {"name": "退货单", "kind": "单据", "what": "一次退货从申请到退款的记录。", "aliases": ["退货"]},
  {"name": "账期客户", "kind": "角色", "what": "先货后款的客户，退款冲减应收。"}],
 "cases": [
  {"title": "320 元，不想要了", "facts": ["签收后第 3 天", "正常品", "不是质量问题", "320 元", "现结客户"],
   "flow": ["A", "B", "C", "D", "G", "P", "V", "M"], "states": ["s1", "s2", "s3", "s4"], "row": 1, "result": "可退。业务员审核，原路退回。"},
  {"title": "到货破损", "facts": ["签收后第 2 天", "质量问题", "150 元"],
   "flow": ["A", "B", "C", "P", "V", "M"], "states": ["s2", "s3", "s4"], "row": 3, "result": "可退。免审，直接取货。"},
  {"title": "第 10 天发现变质", "facts": ["签收后第 10 天", "质量问题"], "flow": ["A", "B"], "states": [], "row": 6,
   "result": "走到「符合申请条件？」走不下去：导图里没写按哪条。", "gap": true}],
 "gaps": [{"say": "超过 7 天才发现的质量问题，退不退？", "src": [3, 9], "row": 6}]
}`

// refundBranch puts the draft's branch under the map's root, and on a
// second canvas a topic that speaks of one of its nouns.
func refundBranch(t *testing.T, a *app) *model.Node {
	t.Helper()
	add := func(parent *model.Node, title string) *model.Node {
		n, err := a.doc.AddChild(parent.ID, title)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	root := add(a.doc.Root, "退货退款")
	for _, part := range [][]string{
		{"申请条件", "签收后 7 天内可以申请", "临期品不退", "赠品随主品一起退"},
		{"审核", "500 元以内业务员审", "超过 500 元主管审", "质量问题免审，直接取货"},
		{"退货方式", "司机上门取货", "门店自己送回"},
		{"退款", "现结客户原路退回", "账期客户冲减应收"},
		{"状态", "待审核", "已驳回", "待取货", "待验收", "已退款", "已关闭"},
	} {
		p := add(root, part[0])
		for _, leaf := range part[1:] {
			add(p, leaf)
		}
	}
	find(t, a, "超过 500 元主管审").Note = "主管 24 小时没处理，自动转给区域经理。"
	_, i := a.file.Sheet(a.doc.ID)
	other := a.file.AddSheet(i)
	other.Title = "组织"
	if _, err := other.AddChild(other.Root.ID, "业务员按片区分"); err != nil {
		t.Fatal(err)
	}
	a.edits++
	return root
}

func TestDiagram(t *testing.T) {
	a, tt := newTestApp(t)
	fake := &fakeClaude{replies: []string{claudeText(refundSet)}}
	a.sendAI = fake.send
	a.cfg.AIKey = "sk-ant-test"
	root := refundBranch(t, a)
	mapBefore, _ := a.file.Marshal()
	a.selectNode(root.ID)
	tt.Frame()

	// Drawn from the topic's page: kept beside the library, not in the map.
	if err := tt.Click("＋ 图解"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	f := a.diaOf(root.ID)
	if f == nil || f.Flow == nil || f.State == nil || f.Table == nil || len(f.Objects) != 4 || len(f.Cases) != 3 {
		t.Fatalf("set %+v, err %q", f, a.diaErr)
	}
	if _, err := os.Stat(filepath.Join(a.diaDir(), root.ID+".json")); err != nil {
		t.Fatal(err)
	}
	if after, _ := a.file.Marshal(); string(after) != string(mapBefore) {
		t.Fatal("the map changed")
	}
	prompt := fake.requests[0].Messages[0].Content[0].OfText.Text
	if !strings.Contains(prompt, "[1] 退货退款") || !strings.Contains(prompt, "[8] 超过 500 元主管审") || !strings.Contains(prompt, "备注：主管 24 小时") {
		t.Fatalf("prompt %q", prompt)
	}
	// Its sources are the topics, by their IDs.
	if d := f.Flow.Node("D"); len(d.Src) != 2 || d.Src[1] != find(t, a, "超过 500 元主管审").ID {
		t.Fatalf("sources %+v", d)
	}
	if a.mode != viewDiagram || a.dia.kind != "flow" || !tt.HasText("符合申请条件？") || !tt.HasText("业务员 · 主管") {
		t.Fatalf("not shown: mode %d, %q", a.mode, tt.Texts())
	}

	// A box chosen says where it comes from, and a click there goes to the map.
	if err := tt.Click("审核"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.dia.pick != "D" || !tt.HasText("图上的这一步") || !tt.HasText("备注：主管 24 小时没处理，自动转给区域经理。") {
		t.Fatalf("pick %q: %q", a.dia.pick, tt.Texts())
	}
	snapshot(t, tt, "diagram-flow")
	if err := tt.Click("主题 超过 500 元主管审"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.mode != viewMap || a.selected != find(t, a, "超过 500 元主管审").ID {
		t.Fatalf("mode %d selected %q", a.mode, a.selected)
	}
	snapshot(t, tt, "diagram-mark")

	// Back by the topic's page; the states, and the table.
	a.selectNode(root.ID)
	tt.Frame()
	if err := tt.Click("图解 状态"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.mode != viewDiagram || a.dia.kind != "state" || !tt.HasText("改后重提") {
		t.Fatalf("states: %q", tt.Texts())
	}
	snapshot(t, tt, "diagram-state")
	if err := tt.Click("图解的判定表"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("第 6 条"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("这种情况没说清") || !tt.HasText("导图里没写") || !tt.HasText("去导图补上") {
		t.Fatalf("gap: %q", tt.Texts())
	}
	snapshot(t, tt, "diagram-table")

	// The nouns: where the map speaks of one, on every canvas.
	if err := tt.Click("图解的名词"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("名词 业务员"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("画布 · 组织") || !tt.HasText("业务员按片区分") || !tt.HasText("500 元以内业务员审") {
		t.Fatalf("mentions: %q", tt.Texts())
	}
	snapshot(t, tt, "diagram-objects")
	if err := tt.Click("提到 业务员按片区分"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.mode != viewMap || a.doc.Title != "组织" {
		t.Fatalf("not on the other canvas: %q", a.doc.Title)
	}

	// Told step by step.
	a.openDia(root.ID, "flow")
	tt.Frame()
	if err := tt.Click("讲一遍"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Key(0, ui.KeyRight)
	tt.Key(0, ui.KeyRight)
	tt.Key(0, ui.KeyRight)
	tt.Frame()
	if a.dia.way != diaTell || a.dia.step != 3 || !tt.HasText("4 / 8") || !tt.HasText("讲一遍 · 第 4 步") {
		t.Fatalf("telling: step %d, %q", a.dia.step, tt.Texts())
	}
	if lit := a.diaLight(f); !lit.nodes["C"] || lit.nodes["G"] || !lit.cur["D"] {
		t.Fatalf("lit %+v", lit)
	}
	snapshot(t, tt, "diagram-tell")
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.dia.way != diaRead || a.mode != viewDiagram {
		t.Fatal("Escape did not leave the telling alone")
	}

	// Walked with a case, on each kind.
	if err := tt.Click("拿例子走一遍"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("到货破损"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if lit := a.diaLight(f); lit.nodes["D"] || !lit.nodes["P"] || !lit.hot[4] {
		t.Fatalf("case lit %+v", lit)
	}
	snapshot(t, tt, "diagram-case")
	a.dia.kind = "state"
	tt.Frame()
	if lit := a.diaLight(f); lit.nodes["s1"] || !lit.nodes["s0"] || !lit.nodes["s2"] {
		t.Fatalf("case states %+v", lit)
	}
	a.dia.kind = "table"
	tt.Frame()
	if lit := a.diaLight(f); !lit.rows[3] || lit.rows[1] {
		t.Fatalf("case rows %+v", lit)
	}

	// Asked back: results covered, a row opened and answered, and when all
	// are, kept with the day to ask again.
	a.setDiaWay(diaQuiz)
	tt.Frame()
	if tt.HasText("主管") || tt.HasText("不可退") || !tt.HasText("不是质量问题") {
		t.Fatalf("results not covered: %q", tt.Texts())
	}
	if err := tt.Click("第 6 条"); err != nil { // the one the map does not decide is not asked
		t.Fatal(err)
	}
	tt.Frame()
	if a.dia.cur != 0 {
		t.Fatal("a row the map leaves unsaid was asked back")
	}
	for i, right := range []bool{true, false, true, true, true} {
		if err := tt.Click(fmt.Sprintf("第 %d 条", i+1)); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if !a.dia.open[i+1] || a.dia.cur != i+1 {
			t.Fatalf("row %d not opened by its click", i+1)
		}
		if i == 1 {
			if !tt.HasText("主管") || tt.HasText("不可退") {
				t.Fatalf("the row opened does not show alone: %q", tt.Texts())
			}
			tt.Frame()
			snapshot(t, tt, "diagram-quiz")
			if err := tt.Click("没想起来"); err != nil {
				t.Fatal(err)
			}
			continue
		}
		a.answerQuiz(right)
	}
	tt.Frame()
	f = a.diaOf(root.ID)
	if f.Quiz == nil || len(f.Quiz.Missed) != 1 || f.Quiz.Missed[0] != 2 || !tt.HasText("5 条里想起 4 条。没想起来的，3 天后「今天」会提醒你再考。") {
		t.Fatalf("quiz %+v: %q", f.Quiz, tt.Texts())
	}
	if a.quizDue(f) || len(a.diasDue()) != 0 {
		t.Fatal("due at once")
	}
	now := testNow
	clock = func() time.Time { return now.AddDate(0, 0, 3) }
	a.now = clock
	if !a.quizDue(f) || len(a.diasDue()) != 1 {
		t.Fatal("not due after three days")
	}
	a.goToday()
	tt.Frame()
	if !tt.HasText("「退货退款」的判定表该再考了") {
		t.Fatalf("no line on 今天: %q", tt.Texts())
	}
	snapshot(t, tt, "diagram-due")
	if err := tt.Click("该再考的判定表"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.mode != viewDiagram || a.dia.way != diaQuiz || a.dia.kind != "table" {
		t.Fatalf("not asked back: mode %d way %d", a.mode, a.dia.way)
	}
	clock = func() time.Time { return now }
	a.now = clock

	// The map changed since: said, with what; nothing to draw when it has not.
	a.dia.way = diaRead
	a.drawDia(root.ID, "")
	tt.Frame()
	if len(fake.requests) != 1 {
		t.Fatal("asked the AI though nothing changed")
	}
	limit := find(t, a, "超过 500 元主管审")
	a.mutate("title:"+limit.ID, func() { limit.Title = "超过 1000 元主管审" })
	a.dia.kind = "flow"
	tt.Frame()
	if ch := a.diaChanges(f); ch.n != 1 || !ch.changed[limit.ID] || !tt.HasText("画完之后导图改过 1 处：「超过 1000 元主管审」。图可能旧了。") {
		t.Fatalf("changes %+v: %q", a.diaChanges(f), tt.Texts())
	}
	snapshot(t, tt, "diagram-stale")

	// Drawn again, with the way back to what it was.
	fake.replies = []string{claudeText(strings.ReplaceAll(refundSet, "≤500 元业务员", "≤1000 元业务员"))}
	if err := tt.Click("重画"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if f = a.diaOf(root.ID); f.Flow.Node("D").Sub != "≤1000 元业务员，以上主管" || a.diaChanges(f).n != 0 || f.Quiz != nil {
		t.Fatalf("redrawn %+v", f.Flow.Node("D"))
	}
	if a.noticeUndo == nil { // the toast's 撤销
		t.Fatalf("no way back: %q", a.notice)
	}
	a.noticeUndo()
	tt.Frame()
	if f = a.diaOf(root.ID); f.Flow.Node("D").Sub != "≤500 元业务员，以上主管" || f.Quiz == nil {
		t.Fatalf("not taken back %+v", f.Flow.Node("D"))
	}

	// Another canvas turned to leaves the diagram; a map renamed keeps its sets.
	a.showSheet(a.file.Sheets[1].ID)
	tt.Frame()
	if a.dia.node != "" || !tt.HasText("这张导图画过的") {
		t.Fatalf("the diagram stayed over another canvas: %q", tt.Texts())
	}
	// 图解's first page lists the map's sets, of every canvas, and opens one.
	snapshot(t, tt, "diagram-home")
	if err := tt.Click("打开图解 退货退款"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.dia.node != root.ID || a.doc != a.file.Sheets[0] {
		t.Fatal("not opened from the first page, on its canvas")
	}
	// The map's tabs: 导图 leaves it, 图解 comes back to the topic's set.
	if err := tt.Click("导图"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.mode != viewMap {
		t.Fatal("the tab 导图 did not leave the diagram")
	}
	a.selectNode(root.ID)
	if err := tt.Click("图解"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.mode != viewDiagram || a.dia.node != root.ID {
		t.Fatal("the tab 图解 did not open the topic's set")
	}
	a.closeDia()
	a.rename(a.path, "订单规则")
	tt.Frame()
	if f = a.diaOf(root.ID); f == nil || f.Path != a.mapKey(a.path) {
		t.Fatalf("the set did not follow the map: %+v", f)
	}

	// Forgotten, with the way back.
	a.openDia(root.ID, "")
	tt.Frame()
	a.deleteDia(root.ID)
	tt.Frame()
	if a.diaOf(root.ID) != nil || a.mode != viewMap {
		t.Fatal("not deleted")
	}
	if a.noticeUndo == nil { // the toast's 撤销
		t.Fatalf("no way back: %q", a.notice)
	}
	a.noticeUndo()
	tt.Frame()
	if a.diaOf(root.ID) == nil {
		t.Fatal("not brought back")
	}
}

// What the AI cannot draw, and no key, are said; a topic with nothing
// under it is not offered.
func TestDiagramFails(t *testing.T) {
	a, tt := newTestApp(t)
	root := refundBranch(t, a)
	a.selectNode(root.ID)
	tt.Frame()
	if err := tt.Click("＋ 图解"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.diaBusy != "" || a.diaOf(root.ID) != nil {
		t.Fatal("drawn with no key")
	}
	fake := &fakeClaude{replies: []string{claudeText(`{"objects": []}`), claudeText("画不了")}}
	a.sendAI = fake.send
	a.cfg.AIKey = "sk-ant-test"
	a.aiOpen = false // where the key is set, which the telling opened
	a.drawDia(root.ID, "")
	tt.Frame()
	if a.diaOf(root.ID) != nil || !tt.HasText("没画成：这一支里没找到可以画的先后、状态、规则或并排的方案") {
		t.Fatalf("empty answer: %q, %q", a.diaErr, tt.Texts())
	}
	snapshot(t, tt, "diagram-failed")
	if err := tt.Click("再画一次图解"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !strings.Contains(a.diaErr, "AI 没有按格式回答") {
		t.Fatalf("err %q", a.diaErr)
	}
	a.selectNode(find(t, a, "司机上门取货").ID)
	tt.Frame()
	if _, ok := tt.Find("＋ 图解"); ok {
		t.Fatal("offered for a topic with nothing under it")
	}
}

// What was added to the page since: a noun lights what speaks of it on
// every kind, the arrows go from box to box, a set follows its branch cut
// and pasted, and is written out as Markdown.
func TestDiagramHands(t *testing.T) {
	a, tt := newTestApp(t)
	fake := &fakeClaude{replies: []string{claudeText(refundSet)}}
	a.sendAI = fake.send
	a.cfg.AIKey = "sk-ant-test"
	root := refundBranch(t, a)
	a.selectNode(root.ID)
	tt.Frame()
	// The bar over the canvas has its key, which draws and then opens.
	if err := tt.Click("画成图解"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	f := a.diaOf(root.ID)
	if f == nil || a.mode != viewDiagram {
		t.Fatalf("not drawn from the bar: %q", a.diaErr)
	}

	// The arrows: from nothing to where it begins, then box to box.
	tt.Key(0, ui.KeyDown)
	if a.dia.pick != "A" {
		t.Fatalf("first %q", a.dia.pick)
	}
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyRight)
	tt.Key(0, ui.KeyDown)
	if a.dia.pick != "D" {
		t.Fatalf("arrows led to %q", a.dia.pick)
	}
	tt.Key(0, ui.KeyEscape)

	// A noun: where it is on the diagrams, and the flow lit for it.
	a.dia.kind = "objects"
	tt.Frame()
	if err := tt.Click("名词 主管"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("在流程里看") || !tt.HasText("在判定表里看") {
		t.Fatalf("no places on the diagrams: %q", tt.Texts())
	}
	if err := tt.Click("在流程里看"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	lit := a.diaLight(f)
	if a.dia.kind != "flow" || a.dia.noun == 0 || !lit.nodes["D"] || lit.nodes["P"] || !tt.HasText("只亮提到「主管」的") {
		t.Fatalf("noun's light %+v: %q", lit, tt.Texts())
	}
	snapshot(t, tt, "diagram-noun-flow")
	a.dia.kind = "table"
	tt.Frame()
	if lit := a.diaLight(f); !lit.rows[2] || lit.rows[1] {
		t.Fatalf("noun's rows %+v", lit.rows)
	}
	snapshot(t, tt, "diagram-noun-table")
	if err := tt.Click("不只亮这个名词了"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.dia.noun != 0 || a.diaLight(f) != nil {
		t.Fatal("the noun's light stayed")
	}

	// From the set's own page: a case is walked by its row.
	a.dia.kind = "flow"
	tt.Frame()
	if err := tt.Click("例子 到货破损"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.dia.way != diaCase || a.dia.cas != 1 {
		t.Fatalf("case not walked: way %d case %d", a.dia.way, a.dia.cas)
	}
	a.dia.way = diaRead

	// Written out: Mermaid for the flow and the states, a table, the nouns.
	md := f.Markdown()
	for _, want := range []string{"flowchart TD", `subgraph L1["业务员 · 主管"]`, `nB{"符合申请条件？"}`, `nC -->|"是，免审"| nP`, "stateDiagram-v2", "[*] --> ns1: 提交申请",
		"| 签收后 | 商品 | 原因 | 金额 | 能不能退 | 谁来审 |", "没说清：超过 7 天不能申请", "- **退货单**（单据）"} {
		if !strings.Contains(md, want) {
			t.Errorf("Markdown lacks %q in\n%s", want, md)
		}
	}

	// Going to a source leaves the way back, as it was.
	a.dia.pick = "D"
	a.diaSource(find(t, a, "500 元以内业务员审").ID)
	tt.Frame()
	if a.mode != viewMap || a.noticeUndo == nil {
		t.Fatal("no way back from the map")
	}
	a.noticeUndo()
	tt.Frame()
	if a.mode != viewDiagram || a.dia.pick != "D" {
		t.Fatalf("not back as it was: mode %d pick %q", a.mode, a.dia.pick)
	}

	// A set drawn by 0.96.0, whose record of the branch was one hash a
	// topic, is not taken for changed; a topic changed since is told.
	was := *f
	f.Seen = branchSeen(a.doc, root, true)
	if err := a.writeDia(f); err != nil {
		t.Fatal(err)
	}
	if n := a.diaChanges(a.diaOf(root.ID)).n; n != 0 {
		t.Fatalf("an older set reads as changed in %d places", n)
	}
	leaf := find(t, a, "门店自己送回")
	a.mutate("", func() { leaf.Title = "门店自己送回仓库" })
	if ch := a.diaChanges(a.diaOf(root.ID)); ch.n != 1 || !ch.changed[leaf.ID] {
		t.Fatalf("changes of an older set %+v", ch)
	}
	a.mutate("", func() { leaf.Title = "门店自己送回" })
	if err := a.writeDia(&was); err != nil {
		t.Fatal(err)
	}

	// A line drawn between two topics of the branch is part of its record,
	// and a set drawn with it does not read as changed.
	a.doc.Relations = append(a.doc.Relations, model.Relation{ID: "r1", From: find(t, a, "审核").ID, To: find(t, a, "退款").ID, Label: "通过后"})
	a.edits++
	if n := a.diaChanges(a.diaOf(root.ID)).n; n != 1 {
		t.Fatalf("a line added reads as %d changes", n)
	}
	fake.replies = []string{claudeText(refundSet)}
	a.drawDia(root.ID, "")
	tt.Frame()
	if f = a.diaOf(root.ID); a.diaChanges(f).n != 0 || !strings.HasPrefix(f.Seen["~relations"], "r/") {
		t.Fatalf("just drawn with a line, it reads as changed: %+v", a.diaChanges(f))
	}
	a.doc.Relations = nil // a cut would take the line with it, which is a change of its own
	a.edits++
	fake.replies = []string{claudeText(refundSet)}
	a.drawDia(root.ID, "")
	tt.Frame()
	if f = a.diaOf(root.ID); a.diaChanges(f).n != 0 {
		t.Fatalf("drawn again without the line: %+v", a.diaChanges(f))
	}

	// Cut and pasted elsewhere, the branch is itself still, by the IDs it
	// had, and its set with it (before 0.105.2 a paste gave new IDs, and
	// the set followed by them: TestCutAndPasteKeepLinks has that way,
	// which a cut undone still takes).
	a.closeDia()
	a.selectNode(root.ID)
	a.copyNodes(true)
	if a.file.Node(root.ID) != nil {
		t.Fatal("not cut")
	}
	a.selectNode(find(t, a, "架构").ID)
	if !a.paste() {
		t.Fatal("not pasted")
	}
	tt.Frame()
	moved := find(t, a, "退货退款")
	g := a.diaOf(moved.ID)
	if moved.ID != root.ID || g == nil {
		t.Fatalf("the set did not follow: %+v", g)
	}
	if src := g.Flow.Node("D").Src; len(src) != 2 || src[1] != find(t, a, "超过 500 元主管审").ID || a.diaChanges(g).n != 0 {
		t.Fatalf("sources %v, changes %+v", src, a.diaChanges(g))
	}
	// The paste and the cut undone, the branch is back by the ID it had,
	// and its set with it; done again, the pasted one has its own.
	a.undo()
	a.undo()
	if a.file.Node(root.ID) == nil || a.diaOf(root.ID) == nil {
		t.Fatal("undone, the branch came back without its set")
	}
	a.redo()
	a.redo()
	if a.diaOf(moved.ID) == nil {
		t.Fatal("done again, the pasted branch lost its set")
	}
	a.selectNode(find(t, a, "功能").ID)
	a.paste() // again: a copy, with no set of its own
	if n := len(a.mapDias()); n != 1 {
		t.Fatalf("%d sets after a second paste", n)
	}
}

// A folder of documents is one map, a canvas a document.
func TestImportFolder(t *testing.T) {
	a, tt := newTestApp(t)
	dir := filepath.Join(t.TempDir(), "rules")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{
		"permission.md": "# 数据权限规则\n\n谁能看到哪些记录。\n\n## 决策表\n\n| 编号 | 规则 |\n|---|---|\n| R-1 | **全局一个开关** |\n",
		"time.md":       "# 时间\n\n- 本周\n- 上月\n",
		"notes.txt":     "不是文档",
		"empty.md":      "",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a.importFolder(dir)
	tt.Frame()
	if mapName(a.path) != "rules" || len(a.file.Sheets) != 2 || a.file.Sheets[0].Title != "数据权限规则" || a.file.Sheets[1].Title != "时间" {
		t.Fatalf("map %q sheets %d", mapName(a.path), len(a.file.Sheets))
	}
	if r := a.file.Sheets[0].Root; !strings.Contains(r.Note, "谁能看到") || r.Children[0].Children[0].Title != "R-1 全局一个开关" {
		t.Fatalf("first canvas %+v", r)
	}
	if !strings.Contains(a.notice, "导入了 2 篇文档") {
		t.Fatalf("notice %q", a.notice)
	}
	snapshot(t, tt, "import-folder")
}

// The tab 图解 is the way in: with no set yet, its first page says what it
// is and draws the topic chosen.
func TestDiagramTab(t *testing.T) {
	a, tt := newTestApp(t)
	fake := &fakeClaude{replies: []string{claudeText(refundSet)}}
	a.sendAI = fake.send
	a.cfg.AIKey = "sk-ant-test"
	root := refundBranch(t, a)
	a.selectNode(root.ID)
	tt.Frame()
	if err := tt.Click("图解"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.mode != viewDiagram || a.dia.node != "" || !tt.HasText("「退货退款」这一支") || tt.HasText("这张导图画过的") {
		t.Fatalf("first page: %q", tt.Texts())
	}
	snapshot(t, tt, "diagram-home-empty")
	if err := tt.Click("画成图解"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.diaOf(root.ID) == nil || a.dia.node != root.ID {
		t.Fatalf("not drawn from the first page: %q", a.diaErr)
	}
	// Its head: the kinds as words, and the map's sets under its name.
	if err := tt.Click("图解的判定表"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.dia.kind != "table" {
		t.Fatalf("kind %q", a.dia.kind)
	}
	snapshot(t, tt, "diagram-head")
	// A flow whose ways go back and across shows as steps; a plain one as a
	// drawing; either by its key.
	f := a.diaOf(root.ID)
	a.dia.kind = "flow"
	if a.flowSteps(f) {
		t.Fatal("a plain flow shown as steps")
	}
	if err := func() error { tt.Frame(); return tt.Click("流程显示成步骤") }(); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.flowSteps(f) || !tt.HasText("↳ 是，免审") || !tt.HasText("到此结束") {
		t.Fatalf("steps: %q", tt.Texts())
	}
	snapshot(t, tt, "diagram-steps")
	if err := tt.Click("跳到第 6 步"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.dia.pick != "P" {
		t.Fatalf("the leap led to %q", a.dia.pick)
	}
	tt.Key(0, ui.KeyDown)
	if a.dia.pick != "V" {
		t.Fatalf("the arrow led to %q", a.dia.pick)
	}
	a.setDiaWay(diaCase)
	a.dia.cas = 1
	tt.Frame()
	snapshot(t, tt, "diagram-steps-case")
}

// A kind of a set is saved as a picture, and its diagrams go among the
// slides of its canvas.
func TestDiagramPictureAndSlides(t *testing.T) {
	a, tt := newTestApp(t)
	fake := &fakeClaude{replies: []string{claudeText(refundSet)}}
	a.sendAI = fake.send
	a.cfg.AIKey = "sk-ant-test"
	root := refundBranch(t, a)
	a.selectNode(root.ID)
	a.drawDia(root.ID, "")
	tt.Frame()
	f := a.diaOf(root.ID)
	if f == nil {
		t.Fatal(a.diaErr)
	}
	// As pictures: the flow as its drawing and as its steps, the table.
	dir := os.Getenv("MINDMAP_SNAPSHOTS")
	for name, of := range map[string]struct {
		kind string
		as   int
	}{"flow": {"flow", flowAsGraph}, "steps": {"flow", flowAsSteps}, "state": {"state", 0}, "table": {"table", 0}} {
		data, err := a.diaImage(f, of.kind, of.as)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		cfg, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil || cfg.Height < 400 || cfg.Width < 1200 {
			t.Fatalf("%s: %dx%d %v", name, cfg.Width, cfg.Height, err)
		}
		if name == "flow" && cfg.Width != (700+56)*2 {
			t.Fatalf("the flow's picture is %d wide", cfg.Width)
		}
		if dir != "" {
			if err := os.WriteFile(filepath.Join(dir, "diagram-picture-"+name+".png"), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	// By its menu's key, to where it is asked.
	a.plat.(*fakePlatform).savePath = filepath.Join(t.TempDir(), "flow")
	a.askExportDia(f, "flow", flowAsSuits)
	if _, err := os.Stat(a.plat.(*fakePlatform).savePath + ".png"); err != nil {
		t.Fatal(err)
	}

	// Among the slides: after its topic's, a slide a kind drawn, not the nouns.
	before := len(a.slidesFor(a.doc))
	a.presentDia(f, true)
	slides := a.slidesFor(a.doc)
	at := slices.IndexFunc(slides, func(s slide) bool { return s.kind == slideDiagram })
	if len(slides) != before+3 || at < 1 || slides[at-1].node.ID != root.ID || slides[at].diaOf != "flow" || slides[at+2].diaOf != "table" {
		t.Fatalf("%d slides (%d before), the first diagram at %d", len(slides), before, at)
	}
	a.startPresenting(root.ID)
	if a.slides[a.slideIndex].node.ID != root.ID {
		t.Fatal("not begun at the topic")
	}
	a.nextSlide()
	for range 4 {
		tt.Frame()
	}
	if a.slides[a.slideIndex].kind != slideDiagram || !tt.HasText("符合申请条件？") || !tt.HasText("流程") {
		t.Fatalf("the flow's slide: %q", tt.Texts())
	}
	snapshot(t, tt, "diagram-slide-flow")
	// A press on the slide goes on, though the diagram is under it.
	tt.ClickAt(640, 400)
	for range 4 {
		tt.Frame()
	}
	if a.slides[a.slideIndex].diaOf != "state" {
		t.Fatalf("a press on the diagram did not go on: slide %d", a.slideIndex)
	}
	a.nextSlide()
	for range 4 {
		tt.Frame()
	}
	snapshot(t, tt, "diagram-slide-table")
	a.stopPresenting()
	// The slides' PDF has them.
	var buf bytes.Buffer
	if err := a.slidesPDF(&buf); err != nil || buf.Len() == 0 {
		t.Fatal(err)
	}
	// Taken out again.
	a.presentDia(f, false)
	if len(a.slidesFor(a.doc)) != before {
		t.Fatal("not taken out of the slides")
	}
}
