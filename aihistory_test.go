package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// thought is a reply of Claude's with its thinking before its text.
func thought(sig, text string) string {
	return fmt.Sprintf(`{"id": "msg", "type": "message", "role": "assistant", "model": "claude-opus-5-5", "stop_reason": "end_turn",
		"content": [{"type": "thinking", "thinking": "", "signature": %q}, {"type": "text", "text": %q}],
		"usage": {"input_tokens": 1, "output_tokens": 1}}`, sig, text)
}

// Each question brings the map as it is, and the one before keeps a line
// in its place: after three questions the conversation sent holds one
// snapshot, not three. What an action gave besides the map stays. The
// thinking of the replies before goes with the snapshot they were signed
// over, and stays while a question is still being answered.
func TestAIKeepsOneSnapshot(t *testing.T) {
	a, tt := newTestApp(t)
	slow := find(t, a, "列表加载慢")
	fake := &fakeClaude{replies: []string{
		thought("sig-1", "第一个回答。"),
		`{"id": "msg", "type": "message", "role": "assistant", "model": "claude-opus-5-5", "stop_reason": "tool_use",
			"content": [{"type": "thinking", "thinking": "", "signature": "sig-2"},
				{"type": "tool_use", "id": "toolu_s", "name": "search_maps", "input": {"query": "性能"}}],
			"usage": {"input_tokens": 1, "output_tokens": 1}}`,
		thought("sig-3", "第二个回答。"),
		thought("sig-4", "第三个回答。"),
	}}
	a.sendAI = fake.send
	a.cfg.AIKey = "sk-ant-test"
	a.aiOpen = true
	tt.Frame()
	sent := func(i int) string {
		t.Helper()
		if i >= len(fake.requests) {
			t.Fatalf("%d requests, no %d", len(fake.requests), i)
		}
		body, _ := json.Marshal(fake.requests[i])
		return string(body)
	}
	count := func(body, part string) int { return strings.Count(body, part) }
	const head = "导图快照："

	a.ai.extra = "最近的变化：登录改了三处"
	a.ai.draft = "第一个问题"
	a.aiSend()
	tt.Frame()
	first := sent(0)
	if count(first, head) != 1 || count(first, "] 列表加载慢") != 1 || !strings.Contains(first, "最近的变化：登录改了三处") {
		t.Fatalf("the first request: %s", first)
	}
	// The tools and the system prompt are kept by the API to be read again.
	if sys := fake.requests[0].System; len(sys) != 1 || sys[0].CacheControl.Type != "ephemeral" || count(first, `"cache_control":{"type":"ephemeral"}`) != 1 {
		t.Fatalf("the system prompt's cache mark: %+v", sys)
	}

	// The second question, answered after a look at the maps: two requests.
	a.mutate("", func() { slow.Title = "列表加载很慢" })
	a.ai.draft = "第二个问题"
	a.aiSend()
	tt.Frame()
	tt.Frame()
	second, lookup := sent(1), sent(2)
	for _, body := range []string{second, lookup} {
		if count(body, head) != 1 || count(body, snapshotGone) != 1 {
			t.Fatalf("not one snapshot and a line for the one before: %s", body)
		}
		if strings.Contains(body, "] 列表加载慢") || count(body, "] 列表加载很慢") != 1 {
			t.Fatalf("not the map as it is now: %s", body)
		}
		// What the action gave with the first question is there still.
		if !strings.Contains(body, snapshotGone+`\n\n最近的变化：登录改了三处`) {
			t.Fatalf("what went with the first snapshot is gone: %s", body)
		}
		// The first reply's thinking was signed over the snapshot now gone.
		if strings.Contains(body, "sig-1") {
			t.Fatalf("thinking sent after what it was signed over changed: %s", body)
		}
	}
	// While the question is being answered nothing before is changed, and
	// the thinking of its own turns is sent back as it came.
	if !strings.Contains(lookup, "sig-2") || !strings.Contains(lookup, "toolu_s") {
		t.Fatalf("the thinking of the turn going on is gone: %s", lookup)
	}

	a.ai.draft = "第三个问题"
	a.aiSend()
	tt.Frame()
	third := sent(3)
	if count(third, head) != 1 || count(third, snapshotGone) != 2 || count(third, "] 列表加载很慢") != 1 {
		t.Fatalf("after three questions: %s", third)
	}
	if strings.Contains(third, `"thinking"`) || !strings.Contains(third, "toolu_s") || !strings.Contains(third, "第二个回答。") {
		t.Fatalf("the turns before, less their thinking: %s", third)
	}
	if n := len(fake.requests[3].Messages); n != 7 {
		t.Fatalf("%d messages", n)
	}
	// A request made before is as it was sent.
	if again := sent(0); again != first {
		t.Fatal("the first request changed after it was sent")
	}
	for _, m := range a.ai.msgs {
		if m.role == "error" {
			t.Fatalf("error %q", m.text)
		}
	}

	// A request that fails is taken back, and the question asked again
	// brings its snapshot: still one.
	fake.replies = nil
	a.ai.draft = "第四个问题"
	a.aiSend()
	tt.Frame()
	fake.replies = []string{thought("sig-5", "第四个回答。")}
	a.ai.draft = "第四个问题"
	a.aiSend()
	tt.Frame()
	if fourth := sent(5); count(fourth, head) != 1 || count(fourth, snapshotGone) != 3 || strings.Contains(fourth, "sig-4") {
		t.Fatalf("after one that failed: %s", fourth)
	}
	// With no map this time, the one before is not sent either.
	a.ai.scope = scopeNone
	fake.replies = []string{reply("好。")}
	a.ai.draft = "第五个问题"
	a.aiSend()
	tt.Frame()
	if fifth := sent(6); count(fifth, head) != 0 || count(fifth, snapshotGone) != 4 || !strings.Contains(fifth, "这次没有附带导图内容") {
		t.Fatalf("with no map: %s", fifth)
	}
}

// DeepSeek's conversation the same.
func TestDeepSeekKeepsOneSnapshot(t *testing.T) {
	a, tt := newTestApp(t)
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []dsMessage `json:"messages"`
		}
		_ = jsonDecode(r, &body)
		var b strings.Builder
		for _, m := range body.Messages[1:] { // less the system prompt
			b.WriteString(m.Role + ": " + m.Content + "\n")
		}
		bodies = append(bodies, b.String())
		fmt.Fprintf(w, `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"回答 %d"}}]}`, len(bodies))
	}))
	defer srv.Close()
	a.cfg.AIProvider, a.cfg.DSKey, a.cfg.DSBaseURL = providerDeepSeek, "sk-test", srv.URL
	a.aiOpen = true
	tt.Frame()
	for i, q := range []string{"第一个问题", "第二个问题", "第三个问题"} {
		if i == 0 {
			a.ai.extra = "最近的变化：登录改了三处"
		}
		a.ai.draft = q
		a.aiSend()
		tt.Frame()
	}
	if len(bodies) != 3 {
		t.Fatalf("%d requests", len(bodies))
	}
	last := bodies[2]
	if strings.Count(last, "导图快照：") != 1 || strings.Count(last, snapshotGone) != 2 || strings.Count(last, "] 列表加载慢") != 1 {
		t.Fatalf("after three questions: %s", last)
	}
	if !strings.Contains(last, "user: "+snapshotGone+"\n\n最近的变化：登录改了三处\n\n第一个问题") || !strings.Contains(last, "user: "+snapshotGone+"\n\n第二个问题") ||
		!strings.Contains(last, "assistant: 回答 2") || !strings.HasSuffix(last, "\n\n第三个问题\n") {
		t.Fatalf("the turns before: %s", last)
	}
}

// The strongest model offered is the one of now; one chosen before and
// offered no more stays the user's: sent, and shown as chosen.
func TestAIModelKept(t *testing.T) {
	a, tt := newTestApp(t)
	if aiModel(a.cfg) != "claude-opus-5-5" || aiModels[len(aiModels)-1].id != "claude-fable-5-2" {
		t.Fatalf("models %v", aiModels)
	}
	fake := &fakeClaude{replies: []string{reply("好。"), reply("好。")}}
	a.sendAI = fake.send
	a.cfg.AIKey = "sk-ant-test"
	a.cfg.AIModel = "claude-fable-5-1"
	a.aiOpen = true
	tt.Frame()
	a.ai.draft = "你好"
	a.aiSend()
	tt.Frame()
	if m := fake.requests[0].Model; m != "claude-fable-5-1" {
		t.Fatalf("sent to %s", m)
	}
	if got := providerModelName(a.cfg); got != "Claude Fable 5.1（上一版）" {
		t.Fatalf("named %q", got)
	}
	a.ai.settings = true
	tt.Frame()
	if !tt.HasText("Claude Fable 5.1（上一版）") {
		t.Fatalf("not shown as chosen: %q", tt.Texts())
	}
	snapshot(t, tt, "ai-model-kept")
	// One the app never offered, written into the settings: by its ID.
	a.cfg.AIModel = "claude-of-a-proxy"
	tt.Frame()
	if !tt.HasText("claude-of-a-proxy") {
		t.Fatalf("not shown as chosen: %q", tt.Texts())
	}
	a.ai.settings = false
	a.ai.draft = "再来"
	a.aiSend()
	if m := fake.requests[1].Model; m != "claude-of-a-proxy" {
		t.Fatalf("sent to %s", m)
	}
}
