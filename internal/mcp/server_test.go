package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/versions"
)

type session struct {
	t  *testing.T
	s  *Server
	id int
}

// do sends a request and returns its result.
func (ss *session) do(method string, params any) map[string]any {
	ss.t.Helper()
	ss.id++
	req, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": ss.id, "method": method, "params": params})
	var out bytes.Buffer
	if err := ss.s.Serve(bytes.NewReader(append(req, '\n')), &out); err != nil {
		ss.t.Fatal(err)
	}
	var resp struct {
		ID     int            `json:"id"`
		Result map[string]any `json:"result"`
		Error  *rpcError      `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		ss.t.Fatalf("%v: %s", err, out.String())
	}
	if resp.Error != nil {
		ss.t.Fatalf("%s: %s", method, resp.Error.Message)
	}
	if resp.ID != ss.id {
		ss.t.Fatalf("id %d", resp.ID)
	}
	return resp.Result
}

// tool calls a tool and returns its text, failing on a tool error unless
// wantErr.
func (ss *session) tool(name string, args map[string]any, wantErr bool) string {
	ss.t.Helper()
	r := ss.do("tools/call", map[string]any{"name": name, "arguments": args})
	text := r["content"].([]any)[0].(map[string]any)["text"].(string)
	if isErr, _ := r["isError"].(bool); isErr != wantErr {
		ss.t.Fatalf("%s: error %v: %s", name, isErr, text)
	}
	return text
}

func TestServer(t *testing.T) {
	dir := t.TempDir()
	f := model.NewFile("后台系统")
	d := f.Sheets[0]
	auth, _ := d.AddChild(d.Root.ID, "权限")
	login, _ := d.AddChild(auth.ID, "登录")
	login.Task = &model.Task{Status: model.StatusTodo}
	path := filepath.Join(dir, model.FileName("后台"))
	f.Save(path)
	store := versions.Store{Dir: t.TempDir()}
	ss := &session{t: t, s: &Server{Dir: dir, Versions: store, Version: "1.0", Now: time.Now}}

	init := ss.do("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": "test", "version": "1"}})
	if init["protocolVersion"] != "2025-06-18" || init["instructions"] == "" {
		t.Fatalf("init %v", init)
	}
	tools := ss.do("tools/list", nil)["tools"].([]any)
	if len(tools) != 12 {
		t.Fatalf("%d tools", len(tools))
	}

	if out := ss.tool("list_maps", nil, false); !strings.Contains(out, "后台 — 中心主题「后台系统」") || !strings.Contains(out, "任务 1（未完成 1）") {
		t.Fatalf("list_maps %s", out)
	}
	out := ss.tool("read_map", map[string]any{"map": "后台"}, false)
	if !strings.Contains(out, "[ ] 登录 {id:"+login.ID+"}") {
		t.Fatalf("read_map %s", out)
	}
	// Changes, each kept as a version with what the agent said.
	ss.tool("update_node", map[string]any{"map": "后台", "id": login.ID, "status": "done", "due": "2026-10-20",
		"append_note": "用 JWT 实现", "add_tags": []any{"#核心"}, "summary": "登录已完成"}, false)
	out = ss.tool("add_nodes", map[string]any{"map": "后台", "parent_id": auth.ID,
		"outline": "- [ ] 刷新令牌\n  - 过期策略", "summary": "补充令牌任务"}, false)
	if !strings.Contains(out, "[ ] 刷新令牌") {
		t.Fatalf("add %s", out)
	}
	g, _ := model.Load(path)
	gl, _ := g.Sheets[0].Find(login.ID)
	if gl.Task.Status != model.StatusDone || gl.Note != "用 JWT 实现" || gl.Tags[0] != "核心" || gl.Task.Due.Format("01-02") != "10-20" {
		t.Fatalf("updated %+v %+v", gl, gl.Task)
	}
	vs := store.WithNotes(store.List(path))
	if len(vs) < 3 || vs[0].Reason != versions.AI || vs[0].Note != "补充令牌任务" {
		t.Fatalf("versions %+v", vs)
	}
	if out := ss.tool("list_tasks", map[string]any{"status": "open"}, false); !strings.Contains(out, "刷新令牌") || strings.Contains(out, "登录") {
		t.Fatalf("tasks %s", out)
	}
	if out := ss.tool("search", map[string]any{"query": "jwt"}, false); !strings.Contains(out, "登录") {
		t.Fatalf("search %s", out)
	}
	// Moving and deleting.
	ss.tool("move_node", map[string]any{"map": "后台", "id": login.ID, "parent_id": d.Root.ID, "summary": "移动"}, false)
	ss.tool("delete_node", map[string]any{"map": "后台", "id": auth.ID, "summary": "删除权限分支"}, false)
	g, _ = model.Load(path)
	if len(g.Sheets[0].Root.Children) != 1 || g.Sheets[0].Root.Children[0].ID != login.ID {
		t.Fatalf("after move/delete %+v", g.Sheets[0].Root.Children)
	}
	// Mistakes come back as tool errors the agent reads.
	if out := ss.tool("update_node", map[string]any{"map": "后台", "id": "nope", "summary": "x"}, true); !strings.Contains(out, "找不到节点") {
		t.Fatal(out)
	}
	if out := ss.tool("read_map", map[string]any{"map": "不存在"}, true); !strings.Contains(out, "找不到导图") {
		t.Fatal(out)
	}
	ss.tool("delete_node", map[string]any{"map": "后台", "id": g.Sheets[0].Root.ID, "summary": "x"}, true)
	// A new map.
	out = ss.tool("create_map", map[string]any{"name": "新项目", "outline": "- 模块 A\n- 模块 B", "folder": "工作"}, false)
	if !strings.Contains(out, "工作/新项目") {
		t.Fatalf("create %s", out)
	}
	if _, err := model.Load(filepath.Join(dir, "工作", model.FileName("新项目"))); err != nil {
		t.Fatal(err)
	}
}

func TestProtocol(t *testing.T) {
	s := &Server{Dir: t.TempDir(), Now: time.Now}
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1999-01-01"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":"a","method":"ping"}`,
		`not json`,
		`{"jsonrpc":"2.0","id":3,"method":"nope"}`,
	}, "\n")
	var out bytes.Buffer
	s.Serve(strings.NewReader(in), &out)
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 4 { // no answer to the notification
		t.Fatalf("%d lines: %s", len(lines), out.String())
	}
	if !strings.Contains(lines[0], fmt.Sprintf(`"protocolVersion":"%s"`, supported[0])) ||
		!strings.Contains(lines[1], `"id":"a"`) || !strings.Contains(lines[2], "-32700") || !strings.Contains(lines[3], "-32601") {
		t.Fatalf("%s", out.String())
	}
}

func TestLibraryTools(t *testing.T) {
	dir := t.TempDir()
	ss := &session{t: t, s: &Server{Dir: dir, Now: time.Now}}
	if got := ss.tool("search_library", map[string]any{"query": "缓存"}, false); !strings.Contains(got, "没有找到") {
		t.Fatalf("empty: %s", got)
	}
	got := ss.tool("add_clip", map[string]any{"text": "Redis 缓存要设过期时间，避免雪崩。", "title": "缓存要点", "tags": []any{"后端"}}, false)
	id := strings.TrimSuffix(strings.TrimPrefix(got, "Kept in the library as "), ".")
	ss.tool("add_clip", map[string]any{"text": "https://example.com/post"}, false)
	got = ss.tool("search_library", map[string]any{"query": "缓存 雪崩"}, false)
	if !strings.Contains(got, "["+id+"] 缓存要点") || !strings.Contains(got, "#后端") {
		t.Fatalf("search: %s", got)
	}
	if got := ss.tool("read_clip", map[string]any{"id": id}, false); !strings.Contains(got, "避免雪崩") {
		t.Fatalf("read: %s", got)
	}
	ss.tool("read_clip", map[string]any{"id": "nope"}, true)
	if got := ss.tool("search_library", map[string]any{"query": "example"}, false); !strings.Contains(got, "链接") {
		t.Fatalf("link: %s", got)
	}
}
