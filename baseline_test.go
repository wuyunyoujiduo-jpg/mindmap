package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// proposeReply is a reply of Claude's proposing edits.
func proposeReply(id, summary, ops string) string {
	return fmt.Sprintf(`{
		"id": %q, "type": "message", "role": "assistant", "model": "claude-opus-5-5",
		"stop_reason": "tool_use",
		"content": [{"type": "tool_use", "id": "toolu_%s", "name": "propose_edits", "input": {"summary": %q, "operations": [%s]}}],
		"usage": {"input_tokens": 10, "output_tokens": 10}
	}`, id, id, summary, ops)
}

// The AI alone moves a map's code baseline: asked to bring the map up to
// date, it reads the changes since, and applying its edits records the
// commit it read, in one step that undo takes back.
func TestSyncWithCode(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	repo := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(name, text string) {
		p := filepath.Join(repo, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(text), 0o644)
	}
	git("init", "-q", "-b", "main")
	write("parse/lexer.go", "package parse\n")
	git("add", ".")
	git("commit", "-q", "-m", "first")
	first := git("rev-parse", "HEAD")
	write("parse/lexer.go", "package parse\n\n// handles unicode now\n")
	write("web/app.ts", "export {}\n")
	git("add", ".")
	git("commit", "-q", "-m", "加前端")
	second := git("rev-parse", "HEAD")

	a, tt := newTestApp(t)
	a.file.Project = repo
	a.file.Baseline = &model.Baseline{Commit: first, Branch: "main", Subject: "first", At: testNow}
	root := a.doc.Root
	parse, _ := a.doc.AddChild(root.ID, "解析")
	parse.Links = []string{"parse/"}
	fake := &fakeClaude{replies: []string{
		proposeReply("m1", "加上前端模块", fmt.Sprintf(`{"op": "add", "id": "", "parent": %q, "ref": "", "title": "前端模块", "note": "", "tags": [], "status": "", "due": "", "links": ["web/"]}`, root.ID)),
		proposeReply("m2", "导图已与代码一致", ""),
	}}
	a.sendAI = fake.send
	a.cfg.AIKey = "sk-ant-test"
	a.panelTab = tabCanvas
	tt.Frame()
	for i := 0; i < 30; i++ {
		tt.Scroll(1100, 500, 0, 300)
		tt.Frame()
	}
	if !tt.HasText("之后有 1 个提交、2 个文件改动还没同步到导图") {
		t.Fatalf("status: %q", tt.Texts())
	}
	if tt.HasText("记录当前版本") {
		t.Fatal("the baseline can be set by hand")
	}
	snapshot(t, tt, "code-sync")
	if err := tt.Click("按代码更新导图"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	raw, _ := json.Marshal(fake.requests[0])
	body := strings.NewReplacer(`\u003c`, "<", `\u003e`, ">").Replace(string(raw))
	for _, want := range []string{"M parse/lexer.go → 「解析」", "A web/app.ts", "+// handles unicode now", "<parse/>"} {
		if !strings.Contains(body, want) {
			t.Errorf("no %q in the request", want)
		}
	}
	if a.file.Baseline.Commit != first {
		t.Fatal("the baseline moved before the edits were applied")
	}
	if !tt.HasText("应用后，代码基线更新到 main · commit " + second[:7] + "（加前端）") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if err := tt.Click("应用"); err != nil {
		t.Fatal(err)
	}
	find(t, a, "前端模块")
	if a.file.Baseline.Commit != second {
		t.Fatalf("baseline %s", a.file.Baseline.Commit)
	}
	// Undo takes back the edits and the baseline together.
	a.undo()
	if a.file.Baseline.Commit != first {
		t.Fatal("undo left the baseline")
	}
	if _, err := findIn(a.doc, "前端模块"); err == nil {
		t.Fatal("undo left the node")
	}

	// Nothing to change: confirming it moves the baseline alone.
	a.syncWithCode()
	tt.Frame()
	if err := tt.Click("应用"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.file.Baseline.Commit != second || !tt.HasText("✓ 导图已与代码一致，代码基线已更新到 "+second[:7]) {
		t.Fatalf("baseline %s, texts %q", a.file.Baseline.Commit, tt.Texts())
	}

	// A message of the user's afterwards moves no baseline.
	a.file.Baseline.Commit = first
	a.ai.draft = "随便聊聊"
	a.aiSend()
	if a.ai.syncBase != nil {
		t.Fatal("a message of the user's carries the baseline")
	}
}
