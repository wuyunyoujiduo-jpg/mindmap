package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/automate"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// postQueue keeps what goroutines post for the main thread, which the
// test runs between frames.
type postQueue struct {
	mu  sync.Mutex
	fns []func()
}

func (q *postQueue) post(fn func()) {
	q.mu.Lock()
	q.fns = append(q.fns, fn)
	q.mu.Unlock()
}

func (q *postQueue) drain() {
	q.mu.Lock()
	fns := q.fns
	q.fns = nil
	q.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
}

// waitRuns runs what is posted until no run goes on.
func waitRuns(t *testing.T, a *app, q *postQueue) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for len(a.auto.procs) > 0 || len(q.fns) > 0 {
		if time.Now().After(deadline) {
			t.Fatal("runs did not end")
		}
		time.Sleep(20 * time.Millisecond)
		q.drain()
	}
}

func TestAutomations(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("no python3")
	}
	a, tt := newTestApp(t)
	q := &postQueue{}
	a.post = q.post
	plat := a.plat.(*fakePlatform)

	if err := tt.Click("自动化"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.libOpen || !a.auto.open || !tt.HasText("把日常任务写成自动化程序") {
		t.Fatalf("page: %v %v %q", a.libOpen, a.auto.open, tt.Texts())
	}
	snapshot(t, tt, "auto-empty")

	// From a template: shown, waiting for its code to be seen.
	if err := tt.Click("网页监控"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	x := a.autoByID("网页监控")
	if x == nil || a.auto.sel != x.ID || !tt.HasText("新程序：第一次运行前，请先看一遍代码。") {
		t.Fatalf("template: %v %q", x, tt.Texts())
	}
	if _, err := os.Stat(filepath.Join(a.autoRoot(), "网页监控", "pages.txt")); err != nil {
		t.Fatal(err)
	}
	// Its folder is not one of maps.
	a.refreshFiles()
	for _, f := range a.tree.Folders {
		if f.Name == autoFolder {
			t.Fatal("the automations' folder shows in the sidebar")
		}
	}
	snapshot(t, tt, "auto-template")

	// A program of one file, run: approved first.
	a.later(func() { a.autoFromTemplate(autoBlankFile) })
	tt.Frame()
	if a.auto.sel != "新程序.py" {
		t.Fatalf("selected %q", a.auto.sel)
	}
	if err := tt.Click("运行"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.auto.approve != "新程序.py" || !tt.HasText("确认「新程序」的代码") || !tt.HasText("新 新程序.py") {
		t.Fatalf("approval: %q %q", a.auto.approve, tt.Texts())
	}
	snapshot(t, tt, "auto-approve")
	if err := tt.Click("确认并运行"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.auto.procs["新程序.py"] == nil {
		t.Fatal("not running")
	}
	waitRuns(t, a, q)
	tt.Frame()
	runs := automate.Runs(a.autoRoot(), "新程序.py")
	if len(runs) != 1 || runs[0].Status != automate.OK || !strings.Contains(runs[0].Log, "你好， 新程序") || runs[0].Output != "运行完成" {
		t.Fatalf("runs %+v", runs)
	}
	if a.auto.tab != 1 || !tt.HasText("完成") || !tt.HasText("运行完成") {
		t.Fatalf("runs shown: %d %q", a.auto.tab, tt.Texts())
	}
	snapshot(t, tt, "auto-run")

	// Changed by another program: it waits again.
	path := filepath.Join(a.autoRoot(), "新程序.py")
	os.WriteFile(path, []byte("raise SystemExit(2)\n"), 0o644)
	a.autoRefresh(true)
	tt.Frame()
	if !tt.HasText("代码被改过（AI 或其他程序），确认后才会运行。") {
		t.Fatalf("change not told: %q", tt.Texts())
	}
	// Edited here, it stays approved.
	a.auto.metas.Of("新程序.py").Approve(a.autoByID("新程序.py").Code())
	a.auto.tab = 0
	a.auto.fileOf = ""
	tt.Frame()
	if a.auto.code == nil {
		t.Fatal("no code shown")
	}
	a.auto.code.Buffer.Set("print('改过')\n")
	tt.Frame()
	a.auto.edited = a.now().Add(-2 * time.Second) // typing stopped
	tt.Frame()
	if data, _ := os.ReadFile(path); string(data) != "print('改过')\n" {
		t.Fatalf("not saved: %q", data)
	}
	if a.auto.metas.Of("新程序.py").NeedsApproval(a.autoByID("新程序.py").Code()) {
		t.Fatal("edited here, it waits")
	}

	// On a schedule: due, it runs; changed, it waits and tells.
	os.WriteFile(path, []byte("# 触发: 每 30 分钟\nprint('定时')\n"), 0o644)
	a.autoRefresh(true)
	a.auto.metas.Of("新程序.py").Approve(a.autoByID("新程序.py").Code())
	a.autoSetEnabled("新程序.py", true)
	a.auto.metas.Of("新程序.py").Since = a.now().Add(-time.Hour)
	a.auto.metas.Of("新程序.py").LastRun = a.now().Add(-time.Hour)
	a.auto.listed = time.Time{}
	a.autoTick()
	if a.auto.procs["新程序.py"] == nil {
		t.Fatal("not run on its schedule")
	}
	waitRuns(t, a, q)
	if r := automate.Runs(a.autoRoot(), "新程序.py")[0]; r.By != "定时" || !strings.Contains(r.Log, "定时") {
		t.Fatalf("scheduled run %+v", r)
	}
	a.autoTick()
	if a.auto.procs["新程序.py"] != nil {
		t.Fatal("run again at once")
	}
	os.WriteFile(path, []byte("# 触发: 每 30 分钟\nprint('又改了')\n"), 0o644)
	a.auto.metas.Of("新程序.py").LastRun = a.now().Add(-time.Hour)
	a.auto.listed = time.Time{}
	a.autoTick()
	if a.auto.procs["新程序.py"] != nil || !strings.Contains(strings.Join(plat.notes, "\n"), "自动化等待确认") {
		t.Fatalf("changed code ran: %q", plat.notes)
	}

	// mymind.confirm asks in the app.
	answer := make(chan bool)
	go func() {
		v, _ := a.autoHandle(context.Background(), x, io.Discard, "confirm", json.RawMessage(`{"message":"要提交吗？"}`))
		answer <- v.(bool)
	}()
	for len(a.auto.confirms) == 0 {
		time.Sleep(10 * time.Millisecond)
		q.drain()
	}
	tt.Frame()
	if !tt.HasText("要提交吗？") {
		t.Fatalf("confirm: %q", tt.Texts())
	}
	snapshot(t, tt, "auto-confirm")
	if err := tt.Click("允许"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !<-answer || len(a.auto.confirms) != 0 {
		t.Fatal("not allowed")
	}

	// Going to a map leaves the page.
	_ = a.open(a.files[0].Path)
	tt.Frame()
	if a.libOpen || tt.HasText("运行记录 0") {
		t.Fatal("still on the automations")
	}
}

func TestAutoLLM(t *testing.T) {
	a, _ := newTestApp(t)
	ctx := context.Background()
	if _, err := a.autoLLM(ctx, a.cfg, "", "", "你好", "", false, 0); err == nil || err.Error() != autoNoModel {
		t.Fatalf("no model: %v", err)
	}
	if _, err := a.autoLLM(ctx, a.cfg, "gpt-9", "", "你好", "", false, 0); err == nil || !strings.Contains(err.Error(), "不认识的模型") {
		t.Fatalf("unknown model: %v", err)
	}
	if _, err := a.autoLLM(ctx, a.cfg, "deepseek-chat", "", "你好", "", false, 0); err == nil || !strings.Contains(err.Error(), "API Key") {
		t.Fatalf("no key: %v", err)
	}
	got, err := a.autoLLM(ctx, a.cfg, "cli:cat", "", "说你好", "内容", false, 0)
	if err != nil || got != "说你好\n\n内容" {
		t.Fatalf("cli: %q %v", got, err)
	}
	got, err = a.autoLLM(ctx, a.cfg, "cli:echo '好的：{\"a\": 1}'", "", "x", "", true, 0)
	if err != nil || got != `{"a": 1}` {
		t.Fatalf("cli json: %q %v", got, err)
	}
	var asked dsRequest
	a.cfg.DSKey = "k"
	a.sendDeepSeek = func(ctx context.Context, cfg config, req dsRequest, onText func(string)) (*dsResponse, error) {
		asked = req
		var r dsResponse
		r.Choices = append(r.Choices, struct {
			Message      dsMessage `json:"message"`
			FinishReason string    `json:"finish_reason"`
		}{Message: dsMessage{Content: "你好！"}})
		return &r, nil
	}
	got, err = a.autoLLM(ctx, a.cfg, "deepseek-reasoner", "系统", "说你好", "", false, 0)
	if err != nil || got != "你好！" || asked.Model != "deepseek-reasoner" || len(asked.Messages) != 2 {
		t.Fatalf("deepseek: %q %v %+v", got, err, asked)
	}
}

func TestAutoGenerate(t *testing.T) {
	a, tt := newTestApp(t)
	a.cfg.AIProvider, a.cfg.DSKey = providerDeepSeek, "k"
	var prompt string
	a.sendDeepSeek = func(ctx context.Context, cfg config, req dsRequest, onText func(string)) (*dsResponse, error) {
		prompt = req.Messages[1].Content
		answer, _ := json.Marshal(autoWritten{Name: "价格提醒", Summary: "写好了，记得填模型", Files: map[string]string{
			automate.Settings: "name = \"价格提醒\"\ntrigger = \"每天 09:00\"\nmodel = \"\"\n",
			"main.py":         "import mymind\nmymind.output('ok')\n",
		}})
		var r dsResponse
		r.Choices = append(r.Choices, struct {
			Message      dsMessage `json:"message"`
			FinishReason string    `json:"finish_reason"`
		}{Message: dsMessage{Content: string(answer)}})
		return &r, nil
	}
	a.openAutomations()
	tt.Frame()
	if err := tt.Click("一句话让 AI 写"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Type("每天 9 点看价格")
	tt.Frame()
	snapshot(t, tt, "auto-new")
	if err := tt.Click("让 AI 写"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	x := a.autoByID("价格提醒")
	if !strings.Contains(prompt, "每天 9 点看价格") || x == nil || a.auto.sel != x.ID || x.Trigger != "每天 09:00" {
		t.Fatalf("generated: %q %+v", prompt, x)
	}
	if !a.auto.metas.Of(x.ID).NeedsApproval(x.Code()) || !tt.HasText("模型：未配置") {
		t.Fatalf("generated code runs unseen, or the model: %q", tt.Texts())
	}

	// Changed by the AI: the changes show to approve.
	a.sendDeepSeek = func(ctx context.Context, cfg config, req dsRequest, onText func(string)) (*dsResponse, error) {
		prompt = req.Messages[1].Content
		answer, _ := json.Marshal(autoWritten{Files: map[string]string{"main.py": "import mymind\nmymind.output('改好了')\n"}})
		var r dsResponse
		r.Choices = append(r.Choices, struct {
			Message      dsMessage `json:"message"`
			FinishReason string    `json:"finish_reason"`
		}{Message: dsMessage{Content: string(answer)}})
		return &r, nil
	}
	a.auto.metas.Of(x.ID).Approve(x.Code())
	_ = a.auto.metas.Save(a.autoRoot())
	a.autoModify(x, "改输出", "Traceback: boom")
	tt.Frame()
	if !strings.Contains(prompt, "=== main.py ===") || !strings.Contains(prompt, "boom") {
		t.Fatalf("modify prompt %q", prompt)
	}
	if a.auto.approve != x.ID || len(a.auto.diffs) != 1 || a.auto.diffs[0].change.File != "main.py" {
		t.Fatalf("approval %q %+v", a.auto.approve, a.auto.diffs)
	}
	snapshot(t, tt, "auto-diff")
}

func TestAutoMapsAndAgent(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("no python3")
	}
	a, tt := newTestApp(t)
	q := &postQueue{}
	a.post = q.post
	root := a.autoRoot()
	os.MkdirAll(filepath.Join(root, "整理"), 0o755)
	repo := t.TempDir()
	os.WriteFile(filepath.Join(root, "整理", automate.Settings), []byte("agent = \"cat > 需求.txt; echo 已实现\"\nrepo = \""+repo+"\"\n"), 0o644)
	os.WriteFile(filepath.Join(root, "整理", "main.py"), []byte(`import mymind
maps = mymind.maps.list()
name = maps[0]["name"]
m = mymind.maps.read(name)
root = m["sheets"][0]["root"]
print("导图", name, root["title"])
mymind.maps.add(name, root["id"], "- 自动加的任务")
m = mymind.maps.read(name)
node = [c for c in m["sheets"][0]["root"]["children"] if c["title"] == "自动加的任务"][0]
mymind.maps.update(name, node["id"], status="done", add_tags=["编码"])
done = mymind.maps.tasks(status="done", done_since="2000-01-01")
print("完成", [t["title"] for t in done])
print("设置", mymind.settings["repo"] == mymind.settings.get("repo"))
print("回答", mymind.agent("做个登录页", repo=mymind.settings["repo"]))
`), 0o644)
	a.openAutomations()
	tt.Frame()
	x := a.autoByID("整理")
	if x == nil || x.Agent == "" {
		t.Fatalf("automation %+v", x)
	}
	a.auto.metas.Of(x.ID).Approve(x.Code())
	a.autoStart(x, "手动")
	waitRuns(t, a, q)
	r := automate.Runs(root, "整理")[0]
	if r.Status != automate.OK || !strings.Contains(r.Log, "完成 ['自动加的任务']") || !strings.Contains(r.Log, "设置 True") ||
		!strings.Contains(r.Log, "回答 已实现") {
		t.Fatalf("run %s: %s %s", r.Status, r.Log, r.Error)
	}
	if data, _ := os.ReadFile(filepath.Join(repo, "需求.txt")); string(data) != "做个登录页" {
		t.Fatalf("agent got %q", data)
	}
	// The map open shows the node added, once read again.
	tt.Frame()
	a.reloadFromDisk(nil)
	found := false
	a.doc.Walk(func(n, _ *model.Node, _ int) bool {
		found = found || n.Title == "自动加的任务" && n.Task != nil && n.Task.Status == model.StatusDone
		return true
	})
	if !found {
		t.Fatal("the node added is not in the map")
	}

	// No coding tool named: told so.
	if _, err := runAgent(context.Background(), "", repo, "x", io.Discard); err == nil || err.Error() != autoNoAgent {
		t.Fatalf("no agent: %v", err)
	}
	got, err := runAgent(context.Background(), "printf %s {prompt}", repo, "它's 好", io.Discard)
	if err != nil || got != "它's 好" {
		t.Fatalf("{prompt}: %q %v", got, err)
	}

	// The home page lists the automations.
	a.later(func() { a.showLibrary(); a.libHome = true })
	tt.Frame()
	tt.Frame()
	if !tt.HasText("整理") || !tt.HasText("全部 ›") {
		t.Fatalf("home: %q", tt.Texts())
	}
	snapshot(t, tt, "auto-home")
}

func TestAutoWatch(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("no python3")
	}
	a, _ := newTestApp(t)
	q := &postQueue{}
	a.post = q.post
	inbox := t.TempDir()
	path := filepath.Join(a.autoRoot(), "收件.py")
	os.MkdirAll(a.autoRoot(), 0o755)
	os.WriteFile(path, []byte("# 触发: 监视 "+inbox+"\nimport os\nprint(sorted(os.listdir('"+inbox+"')))\n"), 0o644)
	a.autoRefresh(true)
	x := a.autoByID("收件.py")
	a.auto.metas.Of(x.ID).Approve(x.Code())
	a.autoSetEnabled(x.ID, true)
	tick := func() { a.auto.listed = time.Time{}; a.autoTick() }
	tick()
	tick()
	if a.auto.procs[x.ID] != nil {
		t.Fatal("ran with nothing new")
	}
	os.WriteFile(filepath.Join(inbox, "a.txt"), []byte("1"), 0o644)
	tick()
	if a.auto.procs[x.ID] == nil {
		t.Fatal("did not run on a new file")
	}
	waitRuns(t, a, q)
	if r := automate.Runs(a.autoRoot(), x.ID)[0]; r.By != "文件变化" || !strings.Contains(r.Log, "a.txt") {
		t.Fatalf("run %+v", r)
	}
	tick()
	if a.auto.procs[x.ID] != nil {
		t.Fatal("ran again with nothing new")
	}
}

// The templates' Python compiles, and their settings read.
func TestAutoTemplates(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("no python3")
	}
	a, _ := newTestApp(t)
	for _, tpl := range append(slices.Clone(autoTemplates), autoBlankFile, autoBlankFolder) {
		a.autoFromTemplate(tpl)
	}
	if len(a.auto.list) != len(autoTemplates)+2 {
		t.Fatalf("%d automations", len(a.auto.list))
	}
	for _, x := range a.auto.list {
		if x.Err != "" {
			t.Errorf("%s: %s", x.ID, x.Err)
		}
		for f, src := range x.Code() {
			if !strings.HasSuffix(f, ".py") {
				continue
			}
			cmd := exec.Command("python3", "-c", "import ast,sys; ast.parse(sys.stdin.read())")
			cmd.Stdin = strings.NewReader(src)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("%s/%s: %s", x.ID, f, out)
			}
		}
	}
	if x := a.autoByID("需求编码"); x.Agent != "" || x.Settings["repo"] != "~/code/项目" {
		t.Fatalf("coding settings %+v", x.Settings)
	}
}
