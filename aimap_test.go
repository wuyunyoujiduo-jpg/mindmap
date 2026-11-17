package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// The AI is told the word of the UI, 主题, and its prompts and tools say
// it: an answer that said 节点 named what the user sees nowhere.
func TestAISaysTopic(t *testing.T) {
	sys := aiSystem()
	if !strings.Contains(sys, "不要说「节点」") || strings.Count(sys, "节点") != 1 {
		t.Fatalf("the system prompt does not tell the AI to say 主题, or says 节点 itself:\n%s", sys)
	}
	tools, _ := json.Marshal([]any{editsSchema(), newMapSchema(), librarySchemas()})
	if strings.Contains(string(tools), "节点") {
		t.Fatalf("a tool says 节点: %s", tools)
	}
}

// Without the map, the AI proposes a new map, which applying creates and
// opens.
func TestAINewMap(t *testing.T) {
	a, tt := newTestApp(t)
	fake := &fakeClaude{replies: []string{`{
		"id": "m1", "type": "message", "role": "assistant", "model": "claude-opus-5-5",
		"stop_reason": "tool_use",
		"content": [{"type": "tool_use", "id": "toolu_1", "name": "propose_new_map", "input": {
			"name": "旅行计划", "outline": "- 行程\n  - [ ] 订机票\n- 预算"}}],
		"usage": {"input_tokens": 10, "output_tokens": 10}
	}`}}
	a.sendAI = fake.send
	a.cfg.AIKey = "sk-ant-test"
	before := len(a.files)
	if err := tt.Click("AI 助手"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("AI 范围"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("不带导图  ·  不发送导图，问别的或新建导图"); err != nil {
		t.Fatal(err)
	}
	a.ai.draft = "帮我建一个旅行计划导图"
	a.aiSend()
	tt.Frame()
	body, _ := json.Marshal(fake.requests[0])
	if strings.Contains(string(body), "列表加载慢") || !strings.Contains(string(body), "没有附带导图内容") {
		t.Fatal("the map was sent")
	}
	if !tt.HasText("新建导图「旅行计划」") || !tt.HasText("应用") {
		t.Fatalf("texts %q", tt.Texts())
	}
	snapshot(t, tt, "ai-new-map")
	if err := tt.Click("应用"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if mapName(a.path) != "旅行计划" || len(a.files) != before+1 {
		t.Fatalf("open %q, %d maps", a.path, len(a.files))
	}
	n := find(t, a, "订机票")
	if n.Task == nil || a.doc.Root.Title != "旅行计划" || len(a.doc.Root.Children) != 2 {
		t.Fatalf("map %+v", a.doc.Root)
	}
	if !tt.HasText("已新建导图「旅行计划」") {
		t.Fatalf("texts %q", tt.Texts())
	}
}
