package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// The AI arranges the material listed into a new map, its nodes linking
// the material; made-up links are dropped, what it left out goes under 其他.
func TestAIMaterialToMap(t *testing.T) {
	a, tt := newTestApp(t)
	var ids []string
	for _, s := range []string{"Go 并发模式", "channel 用法", "咖啡豆烘焙"} {
		c, err := a.addClipText(s, "手动")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, c.ID)
	}
	fake := &fakeClaude{replies: []string{fmt.Sprintf(`{
		"id": "m1", "type": "message", "role": "assistant", "model": "claude-opus-5-5",
		"stop_reason": "tool_use",
		"content": [{"type": "tool_use", "id": "toolu_1", "name": "propose_new_map", "input": {
			"name": "资料整理", "outline": "- Go\n  - 并发模式 <clip:%s>\n  - channel <clip:%s>\n  - 编的 <clip:nope>"}}],
		"usage": {"input_tokens": 10, "output_tokens": 10}
	}`, ids[0], ids[1])}}
	a.sendAI = fake.send
	a.cfg.AIKey = "sk-ant-test"
	a.showLibrary()
	tt.Frame()
	if !tt.HasText("用 AI 整理成导图（4 条）") {
		t.Fatalf("texts %q", tt.Texts())
	}
	snapshot(t, tt, "material-bar")
	if err := tt.Click("用 AI 整理成导图"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	body, _ := json.Marshal(fake.requests[0])
	for _, id := range ids {
		if !strings.Contains(string(body), "clip:"+id) {
			t.Fatalf("clip %s not listed", id)
		}
	}
	guide := a.mapLink(a.path) // maps are material too
	if !strings.Contains(string(body), strings.TrimPrefix(guide, "mindmap:")) {
		t.Fatalf("the map not listed: %s", guide)
	}
	if err := tt.Click("应用"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if mapName(a.path) != "资料整理" || a.libOpen {
		t.Fatalf("open %q, library %v", a.path, a.libOpen)
	}
	made := find(t, a, "编的")
	if len(made.Links) != 0 {
		t.Fatalf("made-up link kept: %v", made.Links)
	}
	other := find(t, a, "其他")
	left := map[string]string{}
	for _, n := range other.Children {
		left[n.Links[0]] = n.Title
	}
	if len(left) != 2 || left[clipScheme+ids[2]] != "咖啡豆烘焙" || left[guide] != "使用指南" {
		t.Fatalf("left out: %v", left)
	}
	if bl := a.clipBacklinks(ids[0]); len(bl) != 1 || bl[0].title != "并发模式" {
		t.Fatalf("backlinks %+v", bl)
	}
}

// Merging material into the open map: the AI links it to the nodes it
// belongs to.
func TestAIMergeMaterial(t *testing.T) {
	a, tt := newTestApp(t)
	c, err := a.addClipText("列表虚拟滚动方案", "手动")
	if err != nil {
		t.Fatal(err)
	}
	slow := find(t, a, "列表加载慢")
	fake := &fakeClaude{replies: []string{fmt.Sprintf(`{
		"id": "m1", "type": "message", "role": "assistant", "model": "claude-opus-5-5",
		"stop_reason": "tool_use",
		"content": [{"type": "tool_use", "id": "toolu_1", "name": "propose_edits", "input": {"summary": "挂上资料", "operations": [
			{"op": "update", "id": %q, "parent": "", "ref": "", "title": "", "note": "", "tags": [], "status": "", "due": "", "links": ["clip:%s"]}]}}],
		"usage": {"input_tokens": 10, "output_tokens": 10}
	}`, slow.ID, c.ID)}}
	a.sendAI = fake.send
	a.cfg.AIKey = "sk-ant-test"
	a.showLibrary()
	a.libKind = kindDocs
	tt.Frame()
	if err := tt.Click("并入「使用指南」"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	body, _ := json.Marshal(fake.requests[0])
	if !strings.Contains(string(body), "clip:"+c.ID) || !strings.Contains(string(body), slow.ID) || a.libOpen {
		t.Fatal("not asked with the map and the material")
	}
	if err := tt.Click("应用"); err != nil {
		t.Fatal(err)
	}
	if len(slow.Links) != 1 || slow.Links[0] != clipScheme+c.ID {
		t.Fatalf("links %v", slow.Links)
	}
}
