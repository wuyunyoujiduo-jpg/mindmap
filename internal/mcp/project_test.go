package mcp

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/versions"
)

// repo makes a git repository with a commit, named name.
func repo(t *testing.T, name string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := filepath.Join(t.TempDir(), name)
	os.MkdirAll(dir, 0o755)
	run(t, dir, "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644)
	commit(t, dir, "first")
	return dir
}

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func commit(t *testing.T, dir, msg string) {
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-q", "-m", msg)
}

// A project has one map, found by the repository the agent works in and
// made the first time something is written down for it.
func TestProjectMap(t *testing.T) {
	code := repo(t, "shop")
	dir := t.TempDir()
	ss := &session{t: t, s: &Server{Dir: dir, Versions: versions.Store{Dir: t.TempDir()}, Now: time.Now, Cwd: code, StateDir: t.TempDir()}}
	ss.do("initialize", map[string]any{"clientInfo": map[string]any{"name": "claude-code"}})

	// Nothing yet: the brief says so, and makes no map.
	if out := ss.tool("project_brief", nil, true); !strings.Contains(out, "还没有导图") {
		t.Fatal(out)
	}
	if files, _ := model.AllFilesIn(dir); len(files) != 0 {
		t.Fatalf("a map was made by reading: %v", files)
	}
	// A task written down makes the project's map, bound to the code.
	out := ss.tool("capture", map[string]any{"text": "给结算加幂等\n重复提交会扣两次款", "priority": "high"}, false)
	if !strings.Contains(out, "「shop」的「待整理」下") {
		t.Fatal(out)
	}
	path := filepath.Join(dir, model.FileName("shop"))
	f, err := model.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !samePath(f.Project, code) {
		t.Fatalf("project %q", f.Project)
	}
	branch := f.Sheets[0].Root.Children[0]
	if branch.Title != SortBranch || len(branch.Children) != 1 || branch.Children[0].Title != "给结算加幂等" ||
		branch.Children[0].Note != "重复提交会扣两次款" || branch.Children[0].Task.Priority != model.PriorityHigh {
		t.Fatalf("branch %+v", branch)
	}
	if len(f.Log) != 1 || f.Log[0].Kind != model.LogTask || f.Log[0].Agent != "claude-code" || f.Log[0].Commit == "" || f.Log[0].Branch != "main" {
		t.Fatalf("log %+v", f.Log)
	}
	// A decision: whole in the library, in the log, linked from its node.
	task := branch.Children[0].ID
	out = ss.tool("log_decision", map[string]any{"title": "幂等键放在订单表", "why": "少一张表，查询也在一处",
		"alternatives": "单独的幂等表：要多一次写", "node_id": task}, false)
	if !strings.Contains(out, "链接到「给结算加幂等」") {
		t.Fatal(out)
	}
	l, _ := library.Open(library.In(dir))
	if len(l.Clips) != 1 || l.Clips[0].Title != "幂等键放在订单表" || !strings.Contains(l.Clips[0].Text, "没有选的方案：单独的幂等表") ||
		l.Clips[0].Tags[0] != DecisionTag || l.Clips[0].Tags[1] != "shop" {
		t.Fatalf("clip %+v", l.Clips)
	}
	// A thought, to the library; the session, with what it leaves.
	ss.tool("capture", map[string]any{"text": "结算页可以做成一步", "kind": "idea"}, false)
	out = ss.tool("log_session", map[string]any{"summary": "给结算接口加了幂等键，补了测试", "todos": []any{"- [ ] 压测结算接口", "清理旧的重试逻辑"}}, false)
	if !strings.Contains(out, "留下的 2 条待办") {
		t.Fatal(out)
	}
	// The older tools need no map named either, and say what they did.
	ss.tool("update_node", map[string]any{"id": task, "status": "doing", "summary": "开始做幂等"}, false)
	f, _ = model.Load(path)
	if n := f.Node(task); n.Task.Status != model.StatusDoing || n.Links[0] != "clip:"+l.Clips[0].ID {
		t.Fatalf("task %+v", n)
	}
	var kinds []string
	for _, e := range f.Log {
		kinds = append(kinds, e.Kind)
	}
	if got := strings.Join(kinds, " "); got != "task decision idea session edit" {
		t.Fatalf("log kinds %s", got)
	}
	if len(f.Sheets[0].Root.Children) != 1 || len(f.Sheets[0].Root.Children[0].Children) != 3 {
		t.Fatalf("one branch of three: %+v", f.Sheets[0].Root.Children)
	}
	// The brief: what is in hand and to do, decided and done.
	out = ss.tool("project_brief", nil, false)
	for _, want := range []string{"# 项目「shop」", "## 进行中（1）", "[~] 给结算加幂等", "## 待办（2）", "压测结算接口", "幂等键放在订单表（clip ", "给结算接口加了幂等键", "待整理 {id:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("brief lacks %q:\n%s", want, out)
		}
	}
	// A worktree of the repository is the same project.
	wt := filepath.Join(t.TempDir(), "wt")
	run(t, code, "worktree", "add", "-q", "-b", "feat", wt)
	other := &session{t: t, s: &Server{Dir: dir, Now: time.Now, Cwd: wt, StateDir: ss.s.StateDir}}
	other.tool("capture", map[string]any{"text": "在分支上发现的事"}, false)
	if files, _ := model.AllFilesIn(dir); len(files) != 1 {
		t.Fatalf("a second map for a worktree: %v", files)
	}
	f, _ = model.Load(path)
	if e := f.Log[len(f.Log)-1]; e.Branch != "feat" {
		t.Fatalf("branch %+v", e)
	}
	// A map the user made by the project's name, bound to nothing, is taken.
	code2 := repo(t, "blog")
	model.NewFile("博客").Save(filepath.Join(dir, model.FileName("blog")))
	third := &session{t: t, s: &Server{Dir: dir, Now: time.Now, Cwd: t.TempDir()}}
	third.tool("log_session", map[string]any{"summary": "搭了骨架", "project_dir": code2}, false)
	g, _ := model.Load(filepath.Join(dir, model.FileName("blog")))
	if !samePath(g.Project, code2) || len(g.Log) != 1 || g.Sheets[0].Root.Title != "博客" {
		t.Fatalf("blog %+v", g)
	}
	// In no project: a task to the inbox, the rest asks which.
	if out := third.tool("capture", map[string]any{"text": "买咖啡豆"}, false); !strings.Contains(out, "收件箱") {
		t.Fatal(out)
	}
	if out := third.tool("log_session", map[string]any{"summary": "x"}, true); !strings.Contains(out, "project_dir") {
		t.Fatal(out)
	}
	in, _ := model.Load(filepath.Join(dir, model.FileName(InboxName)))
	if len(in.Sheets[0].Root.Children) != 1 || len(in.Log) != 0 {
		t.Fatalf("inbox %+v", in.Sheets[0].Root.Children)
	}
}

// On another computer the code is in another folder: the map is the
// project's by the repository's origin.
func TestProjectMapByRemote(t *testing.T) {
	here, there := repo(t, "shop"), repo(t, "shop-work")
	run(t, here, "remote", "add", "origin", "git@github.com:me/shop.git")
	run(t, there, "remote", "add", "origin", "https://github.com/me/shop")
	dir := t.TempDir() // the maps folder, synced between the two
	a := &Server{Dir: dir, Now: time.Now, Cwd: here}
	if _, err := a.Call("capture", json.RawMessage(`{"text":"在这台记的"}`)); err != nil {
		t.Fatal(err)
	}
	b := &Server{Dir: dir, Now: time.Now, Cwd: there}
	if _, err := b.Call("log_session", json.RawMessage(`{"summary":"在另一台做的"}`)); err != nil {
		t.Fatal(err)
	}
	files, _ := model.AllFilesIn(dir)
	if len(files) != 1 {
		t.Fatalf("a map for each computer: %v", files)
	}
	f, _ := model.Load(files[0])
	if f.Repo != "github.com/me/shop" || !samePath(f.Project, here) || len(f.Log) != 2 || f.Log[1].Commit == "" {
		t.Fatalf("%+v %+v", f, f.Log)
	}
}

// What looks like a secret is not written into a map.
func TestCaptureMasksSecrets(t *testing.T) {
	code := repo(t, "api")
	dir := t.TempDir()
	ss := &session{t: t, s: &Server{Dir: dir, Now: time.Now, Cwd: code}}
	ss.tool("log_session", map[string]any{"summary": "换了密钥 API_KEY=sk-abcdefghijklmnop1234567890", "todos": []any{"轮换 token: ghp_abcdefghijklmnopqrstuvwxyz0123456789"}}, false)
	data, _ := os.ReadFile(filepath.Join(dir, model.FileName("api")))
	if strings.Contains(string(data), "abcdefghijklmnop") {
		t.Fatalf("a secret was written: %s", data)
	}
}

// Agents that write at the same moment lose nothing of each other's.
func TestConcurrentWrites(t *testing.T) {
	code := repo(t, "busy")
	dir, state := t.TempDir(), t.TempDir()
	first := &Server{Dir: dir, Now: time.Now, Cwd: code, StateDir: state}
	if _, err := first.Call("capture", json.RawMessage(`{"text":"第一条"}`)); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	const agents, each = 6, 5
	for i := range agents {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := &Server{Dir: dir, Now: time.Now, Cwd: code, StateDir: state}
			for j := range each {
				args, _ := json.Marshal(map[string]any{"text": "任务 " + string(rune('A'+i)) + string(rune('0'+j))})
				if _, err := s.Call("capture", args); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	f, _ := model.Load(filepath.Join(dir, model.FileName("busy")))
	if n := len(f.Sheets[0].Root.Children[0].Children); n != 1+agents*each {
		t.Fatalf("%d tasks kept of %d", n, 1+agents*each)
	}
}

// The hooks: the brief as a session begins, and one request to record the
// session when the code changed.
func TestHooks(t *testing.T) {
	code := repo(t, "site")
	dir := t.TempDir()
	now := time.Date(2026, 11, 14, 10, 0, 0, 0, time.Local)
	s := &Server{Dir: dir, Now: func() time.Time { return now }, Cwd: code, StateDir: t.TempDir()}
	if out := s.HookStart(code); out != "" {
		t.Fatalf("a project with no map has no brief: %s", out)
	}
	if out := s.HookStop(code, false); out != "" {
		t.Fatalf("nothing changed: %s", out)
	}
	// A file changed, not committed: asked once, then not again at once.
	os.WriteFile(filepath.Join(code, "b.go"), []byte("package a\n"), 0o644)
	out := s.HookStop(code, false)
	var d struct{ Decision, Reason string }
	if json.Unmarshal([]byte(out), &d) != nil || d.Decision != "block" || !strings.Contains(d.Reason, "log_session") {
		t.Fatalf("stop: %s", out)
	}
	if out := s.HookStop(code, true); out != "" {
		t.Fatalf("asked while already kept from stopping: %s", out)
	}
	os.WriteFile(filepath.Join(code, "c.go"), []byte("package a\n"), 0o644)
	if out := s.HookStop(code, false); out != "" {
		t.Fatalf("asked again within the half hour: %s", out)
	}
	now = now.Add(31 * time.Minute)
	if out := s.HookStop(code, false); out == "" {
		t.Fatal("not asked after half an hour of changes")
	}
	// A commit is asked for at once; a session recorded is not asked for.
	commit(t, code, "second")
	if out := s.HookStop(code, false); out == "" {
		t.Fatal("not asked after a commit")
	}
	os.WriteFile(filepath.Join(code, "d.go"), []byte("package a\n"), 0o644)
	commit(t, code, "third")
	if _, err := s.Call("log_session", json.RawMessage(`{"summary":"加了三个文件"}`)); err != nil {
		t.Fatal(err)
	}
	if out := s.HookStop(code, false); out != "" {
		t.Fatalf("asked though the session was recorded: %s", out)
	}
	if out := s.HookStart(code); !strings.Contains(out, "加了三个文件") || !strings.Contains(out, "# 项目「site」") {
		t.Fatalf("start: %s", out)
	}
	// Outside a repository the hooks say nothing.
	if out := s.HookStart(t.TempDir()) + s.HookStop(t.TempDir(), false); out != "" {
		t.Fatal(out)
	}
}
