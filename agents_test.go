package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/mcp"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// What the agents report through the MCP server shows on 今天: counted
// as new, listed by day and project, opened, and taken back.
func TestAgentsPage(t *testing.T) {
	a, tt := newTestApp(t)
	at := a.now().Add(-26 * time.Hour)
	s := &mcp.Server{Dir: a.dir, Now: func() time.Time { at = at.Add(time.Minute); return at }, Agent: "claude-code"}
	call := func(tool string, args map[string]any) string {
		t.Helper()
		raw, _ := json.Marshal(args)
		out, err := s.Call(tool, raw)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	call("create_map", map[string]any{"name": "店铺", "outline": "- 结算\n- 商品"})
	call("create_map", map[string]any{"name": "博客"})
	call("log_session", map[string]any{"map": "博客", "summary": "搭了骨架，首页能打开了"})
	at = a.now().Add(-3 * time.Hour) // today
	call("capture", map[string]any{"map": "店铺", "text": "给结算加幂等"})
	call("log_decision", map[string]any{"map": "店铺", "title": "幂等键放在订单表", "why": "少一张表"})
	call("log_session", map[string]any{"map": "店铺", "summary": "给结算接口加了幂等键，补了测试；压测还没做", "todos": []any{"压测结算接口"}})
	a.refreshFiles()
	a.checkLibrary()
	a.openTasks()
	tt.Frame()
	// 今天 says so, and the left column counts it.
	if !tt.HasText("AI 新记了 4 条，在 2 个项目里") {
		t.Fatalf("no line on 今天: %q", tt.Texts())
	}
	snapshot(t, tt, "agents-today")
	if err := tt.Click("AI 动态"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	for _, want := range []string{"给结算接口加了幂等键，补了测试；压测还没做", "幂等键放在订单表", "给结算加幂等", "Claude Code · 留下 1 条待办", "店铺"} {
		if !tt.HasText(want) {
			t.Fatalf("no %q: %q", want, tt.Texts())
		}
	}
	if tt.HasText("搭了骨架，首页能打开了") {
		t.Fatal("yesterday's shows under 今天")
	}
	snapshot(t, tt, "agents")
	if n, _ := a.agentsUnseen(); n != 0 {
		t.Fatalf("%d still new after looking", n)
	}
	if err := tt.Click("7 天"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("搭了骨架，首页能打开了") {
		t.Fatalf("no yesterday in 7 days: %q", tt.Texts())
	}
	snapshot(t, tt, "agents-week")
	// A task taken back leaves the map and the record, and comes back.
	shop := filepath.Join(a.dir, model.FileName("店铺"))
	var task agentItem
	for _, it := range a.agentItems() {
		if it.e.Kind == model.LogTask {
			task = it
		}
	}
	a.takeBack(task)
	f, _ := model.Load(shop)
	if f.Node(task.e.Nodes[0]) != nil || len(f.Log) != 2 {
		t.Fatalf("not taken back: %+v", f.Log)
	}
	a.noticeUndo()
	f, _ = model.Load(shop)
	if n := f.Node(task.e.Nodes[0]); n == nil || len(f.Log) != 3 || f.Log[0].Kind != model.LogTask {
		t.Fatalf("not put back: %+v", f.Log)
	}
	// A decision taken back leaves the library too.
	var dec agentItem
	for _, it := range a.agentItems() {
		if it.e.Kind == model.LogDecision {
			dec = it
		}
	}
	a.takeBack(dec)
	if a.lib.Find(dec.e.Clip) != nil {
		t.Fatal("the decision's clip stays")
	}
	a.noticeUndo()
	if a.lib.Find(dec.e.Clip) == nil {
		t.Fatal("the decision's clip did not come back")
	}
	// One project's record, the whole of it.
	a.showAgents(shop)
	tt.Frame()
	tt.Frame()
	if !tt.HasText("项目记录") || tt.HasText("搭了骨架，首页能打开了") || !tt.HasText("幂等键放在订单表") {
		t.Fatalf("project's record: %q", tt.Texts())
	}
	snapshot(t, tt, "agents-project")
	if got := agentsReport(a.agentItems(), today(a.now())); !strings.Contains(got, "### 店铺") || !strings.Contains(got, "**决策** 幂等键放在订单表") {
		t.Fatalf("report: %s", got)
	}
}
