package automate

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTriggers(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	from := time.Date(2026, 10, 19, 10, 30, 0, 0, loc) // a Monday
	for _, c := range []struct {
		in   string
		next time.Time
	}{
		{"手动", time.Time{}},
		{"", time.Time{}},
		{"每 30 分钟", from.Add(30 * time.Minute)},
		{"每小时", from.Add(time.Hour)},
		{"每2小时", from.Add(2 * time.Hour)},
		{"每天 09:00", time.Date(2026, 10, 20, 9, 0, 0, 0, loc)},
		{"每天 11：15", time.Date(2026, 10, 19, 11, 15, 0, 0, loc)},
		{"每周一 09:00", time.Date(2026, 10, 26, 9, 0, 0, 0, loc)},
		{"每周三 09:00", time.Date(2026, 10, 21, 9, 0, 0, 0, loc)},
		{"每周日 20:00", time.Date(2026, 10, 25, 20, 0, 0, 0, loc)},
		{"每季度末", time.Date(2026, 12, 31, 18, 0, 0, 0, loc)},
		{"每季度末 09:00", time.Date(2026, 12, 31, 9, 0, 0, 0, loc)},
	} {
		tr, err := ParseTrigger(c.in)
		if err != nil {
			t.Fatalf("%q: %v", c.in, err)
		}
		if got := tr.Next(from); !got.Equal(c.next) {
			t.Errorf("%q: next %v, want %v", c.in, got, c.next)
		}
	}
	q, _ := ParseTrigger("每季度末 18:00")
	if got := q.Next(time.Date(2026, 9, 30, 18, 0, 0, 0, loc)); !got.Equal(time.Date(2026, 12, 31, 18, 0, 0, 0, loc)) {
		t.Errorf("after a quarter's end: %v", got)
	}
	if got := q.Next(time.Date(2026, 6, 1, 0, 0, 0, 0, loc)); !got.Equal(time.Date(2026, 6, 30, 18, 0, 0, 0, loc)) {
		t.Errorf("in June: %v", got)
	}
	for _, bad := range []string{"每天 25:00", "明天", "每 0 分钟"} {
		if _, err := ParseTrigger(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
	// Runs missed make one run.
	d, _ := ParseTrigger("每天 09:00")
	last := time.Date(2026, 10, 15, 9, 0, 0, 0, loc)
	if !d.Due(last, from) {
		t.Error("missed days are not due")
	}
	if d.Due(time.Date(2026, 10, 19, 9, 0, 0, 0, loc), from) {
		t.Error("today's run is due again")
	}
	if d.Due(time.Time{}, from) {
		t.Error("due with nothing to count from")
	}
}

func TestList(t *testing.T) {
	root := t.TempDir()
	write := func(rel, s string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("日报.py", "#!/usr/bin/env python3\n# 名称: 每日日报\n# 触发: 每天 09:00\n# 模型: deepseek-chat\nprint(1)\n")
	write("网页监控/automation.toml", "name = \"网页监控\"\ntrigger = \"每 30 分钟\" # 半小时\ntimeout = \"5m\"\nmodel = \"cli:claude -p\"\n")
	write("网页监控/main.py", "import fetch\n")
	write("网页监控/fetch.py", "")
	write("网页监控/requirements.txt", "requests\n")
	write("网页监控/data/seen.json", "{}")
	write("网页监控/__pycache__/x.pyc", "")
	write("坏的/automation.toml", "trigger = \"明天\"\n")
	write("坏的/main.py", "")
	write("_共享/util.py", "")
	write(".runs/x/y.json", "{}")
	list, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("%d automations", len(list))
	}
	byID := map[string]*Automation{}
	for _, a := range list {
		byID[a.ID] = a
	}
	d := byID["日报.py"]
	if d.Name != "每日日报" || d.Trigger != "每天 09:00" || d.Model != "deepseek-chat" || d.Folder || d.Err != "" {
		t.Fatalf("file: %+v", d)
	}
	w := byID["网页监控"]
	if w.Name != "网页监控" || w.Trigger != "每 30 分钟" || w.Timeout != 5*time.Minute || w.Model != "cli:claude -p" || !w.Folder || w.Err != "" {
		t.Fatalf("folder: %+v", w)
	}
	if got := strings.Join(w.Files(), ","); got != "automation.toml,data/seen.json,fetch.py,main.py,requirements.txt" {
		t.Fatalf("files %s", got)
	}
	code := w.Code()
	if len(code) != 4 || code["data/seen.json"] != "" {
		t.Fatalf("code %v", code)
	}
	if byID["坏的"].Err == "" {
		t.Fatal("a bad trigger is not told")
	}

	// Approval: the code approved, then changed.
	m := Metas{}
	if !m.Of(w.ID).NeedsApproval(code) {
		t.Fatal("new code approved")
	}
	m.Of(w.ID).Approve(code)
	if m.Of(w.ID).NeedsApproval(w.Code()) {
		t.Fatal("approved code needs approval")
	}
	write("网页监控/data/seen.json", `{"a":1}`)
	if m.Of(w.ID).NeedsApproval(w.Code()) {
		t.Fatal("data needs approval")
	}
	write("网页监控/fetch.py", "import os\n")
	ch := Changes(m[w.ID].Code, w.Code())
	if len(ch) != 1 || ch[0].File != "fetch.py" || ch[0].New != "import os\n" {
		t.Fatalf("changes %+v", ch)
	}
	if err := m.Save(root); err != nil {
		t.Fatal(err)
	}
	if LoadMetas(root)[w.ID].Approved != m[w.ID].Approved {
		t.Fatal("metas not kept")
	}
}

func TestWrite(t *testing.T) {
	root := t.TempDir()
	id, err := Write(root, FreeID(root, "日/报", ".py"), map[string]string{"main.py": "print(1)\n"})
	if err != nil || id != "日-报.py" {
		t.Fatalf("%q %v", id, err)
	}
	if FreeID(root, "日/报", ".py") != "日-报 2.py" {
		t.Fatal("ID taken")
	}
	if _, err := Write(root, "x", map[string]string{"../evil.py": ""}); err == nil {
		t.Fatal("wrote outside")
	}
	id, err = Write(root, "多文件", map[string]string{"main.py": "import a\n", "lib/a.py": ""})
	if err != nil || id != "多文件" {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "多文件", "lib", "a.py")); err != nil {
		t.Fatal(err)
	}
}

func TestRun(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("no python3")
	}
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, Shared), 0o755)
	os.WriteFile(filepath.Join(root, Shared, "util.py"), []byte("def twice(s):\n    return s + s\n"), 0o644)
	os.MkdirAll(filepath.Join(root, "示例"), 0o755)
	os.WriteFile(filepath.Join(root, "示例", "main.py"), []byte(`import mymind, util, helper
print("开始", mymind.name)
answer = mymind.llm("说你好", input={"a": 1})
print(util.twice(answer), helper.X)
n = mymind.state.get("n", 0) + 1
mymind.state["n"] = n
mymind.output("第 %d 次：%s" % (n, answer))
try:
    mymind.llm("坏的")
except mymind.MyMindError as e:
    print("错误", e)
`), 0o644)
	os.WriteFile(filepath.Join(root, "示例", "helper.py"), []byte("X = 42\n"), 0o644)
	os.WriteFile(filepath.Join(root, "示例", Settings), []byte("model = \"测试模型\"\n"), 0o644)
	a, err := Load(root, "示例")
	if err != nil || a.Err != "" {
		t.Fatal(err, a.Err)
	}
	var asked []string
	env := Env{Root: root, Python: "python3", Handler: func(ctx context.Context, a *Automation, method string, params json.RawMessage) (any, error) {
		var p struct{ Prompt, Input, Model string }
		json.Unmarshal(params, &p)
		if p.Prompt == "坏的" {
			return nil, os.ErrInvalid
		}
		asked = append(asked, method+":"+p.Prompt+":"+p.Input+":"+p.Model)
		return "你好", nil
	}}
	for n := 1; n <= 2; n++ {
		p, err := Start(env, a, "手动", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		<-p.Done()
		r := p.Snapshot()
		if r.Status != OK {
			t.Fatalf("status %s exit %d: %s %s", r.Status, r.Exit, r.Log, r.Error)
		}
		if !strings.Contains(r.Log, "开始 示例") || !strings.Contains(r.Log, "你好你好 42") || !strings.Contains(r.Log, "错误 invalid argument") {
			t.Fatalf("log %q", r.Log)
		}
		if want := "第 " + string(rune('0'+n)) + " 次：你好"; r.Output != want {
			t.Fatalf("output %q", r.Output)
		}
	}
	if len(asked) != 2 || asked[0] != "llm:说你好:{\n  \"a\": 1\n}:" {
		t.Fatalf("asked %q", asked)
	}
	runs := Runs(root, "示例")
	if len(runs) != 2 || runs[0].Output != "第 2 次：你好" {
		t.Fatalf("runs %+v", runs)
	}

	// A failing run, and one stopped.
	os.WriteFile(filepath.Join(root, "坏.py"), []byte("raise SystemExit(3)\n"), 0o644)
	b, _ := Load(root, "坏.py")
	p, err := Start(env, b, "定时", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	<-p.Done()
	if r := p.Snapshot(); r.Status != Failed || r.Exit != 3 {
		t.Fatalf("failed run %+v", r)
	}
	os.WriteFile(filepath.Join(root, "慢.py"), []byte("import time\nprint('等', flush=True)\ntime.sleep(30)\n"), 0o644)
	s, _ := Load(root, "慢.py")
	p, err = Start(env, s, "手动", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	p.Stop()
	select {
	case <-p.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("not stopped")
	}
	if r := p.Snapshot(); r.Status != Stopped {
		t.Fatalf("stopped run %+v", r)
	}
	s.Timeout = 300 * time.Millisecond
	p, _ = Start(env, s, "手动", time.Now())
	<-p.Done()
	if r := p.Snapshot(); r.Status != TimeOut {
		t.Fatalf("timed out run %+v", r)
	}
}
