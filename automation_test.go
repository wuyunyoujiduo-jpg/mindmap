package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/automate"
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
		v, _ := a.autoHandle(context.Background(), x, "confirm", json.RawMessage(`{"message":"要提交吗？"}`))
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
