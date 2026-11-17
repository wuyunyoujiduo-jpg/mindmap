package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Coming back to the app scans the skills only when a folder of them
// changed, or while 技能 is the page shown: a scan reads every file of
// every skill.
func TestSkillsScanWhenDue(t *testing.T) {
	a, tt := newTestApp(t)
	a.cfg.NoHolidays = true
	home := os.Getenv("HOME")
	cc := filepath.Join(home, ".claude", "skills")
	writeSkill(t, cc, "deploy", "发布到预发环境")
	at := testNow
	a.now = func() time.Time { return at }
	later := func() { at = at.Add(time.Minute) }
	a.skills.tidy = true
	a.skillsRescan() // as the app opens
	first := a.skills.report
	if first == nil || first.Entry("g/deploy") == nil || a.skills.stamp == "" || a.skills.tidy {
		t.Fatalf("no first scan: %+v", a.skills)
	}
	tt.Frame()

	// Come back to with nothing changed, a map shown: no scan.
	later()
	a.skillsActive()
	if a.skills.report != first {
		t.Fatal("scanned again with nothing changed")
	}
	// A skill written meanwhile: scanned.
	writeSkill(t, cc, "write-weekly", "写周报")
	later()
	a.skillsActive()
	second := a.skills.report
	if second == first || second.Entry("g/write-weekly") == nil {
		t.Fatal("a new skill not seen")
	}
	later()
	a.skillsActive()
	if a.skills.report != second {
		t.Fatal("scanned again after the scan that saw it")
	}
	// One changed where it is: scanned.
	md := filepath.Join(cc, "deploy", "SKILL.md")
	if err := os.WriteFile(md, []byte("---\nname: deploy\ndescription: 发布到正式环境\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(md, at, at.Add(time.Hour))
	later()
	a.skillsActive()
	third := a.skills.report
	if third == second || third.Entry("g/deploy").Description != "发布到正式环境" {
		t.Fatal("a skill changed not seen")
	}
	// A tool installed: scanned.
	writeSkill(t, filepath.Join(home, ".newagent", "skills"), "its-own", "它自己的")
	later()
	a.skillsActive()
	fourth := a.skills.report
	if _, ok := fourth.Tool("newagent"); !ok {
		t.Fatal("a new tool not seen")
	}

	// With 技能 shown, every return scans: what is looked at is as it is.
	later()
	if err := tt.Click("技能"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	shown := a.skills.report
	if shown == fourth {
		t.Fatal("not scanned as the page was opened")
	}
	later()
	a.skillsActive()
	if a.skills.report == shown {
		t.Fatal("not scanned with the page shown")
	}
}

// 重新扫描 says the folders were looked at when they were, not as the
// scan begins.
func TestSkillsScanSaidWhenIn(t *testing.T) {
	a, tt := newTestApp(t)
	a.cfg.NoHolidays = true
	writeSkill(t, filepath.Join(os.Getenv("HOME"), ".claude", "skills"), "deploy", "发布到预发环境")
	tt.Frame()
	if err := tt.Click("技能"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	// The scans wait, as away from the window they take their time.
	var waiting []func()
	a.async = func(work func() (any, error), done func(any, error)) {
		waiting = append(waiting, func() { done(work()) })
	}
	a.notice = ""
	if err := tt.Click("重新扫描"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.skills.busy || len(waiting) != 1 {
		t.Fatalf("no scan begun: busy %v, %d waiting", a.skills.busy, len(waiting))
	}
	if a.notice != "" {
		t.Fatalf("said before the scan was in: %q", a.notice)
	}
	waiting[0]()
	tt.Frame()
	if a.skills.busy || a.notice != "扫过了各个工具的技能目录" {
		t.Fatalf("not said once in: busy %v, %q", a.skills.busy, a.notice)
	}
}

// A skill's description and its SKILL.md show with what looks like a
// secret in dots, put there by the scan: a frame shows what the scan
// made and looks for no secret itself.
func TestSkillShownMasked(t *testing.T) {
	a, tt := newTestApp(t)
	const key = "sk-abcdefghijklmnopqrstuvwx"
	dir := writeSkill(t, filepath.Join(os.Getenv("HOME"), ".claude", "skills"), "call-api", "用 "+key+" 调接口")
	md := "---\nname: call-api\ndescription: 用 " + key + " 调接口\n---\n\n## 步骤\n1. 带上 " + key + " 去调\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("技能"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	e := a.skills.report.Entry("g/call-api")
	if e == nil || !e.Secret || strings.Contains(e.About+e.Shown, key) || !strings.Contains(e.About, "••••") || !strings.Contains(e.Shown, "带上 ••••") {
		t.Fatalf("not masked by the scan: %+v", e)
	}
	if !strings.Contains(e.Raw, key) || !strings.Contains(e.Description, key) {
		t.Fatal("what is copied and counted is no more as written")
	}
	if err := tt.Click("call-api"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	texts := strings.Join(tt.Texts(), "\n")
	if strings.Contains(texts, key) || !strings.Contains(texts, "带上 ••••") || !tt.HasText("含密钥") {
		t.Fatalf("shown: %q", texts)
	}
	// What a frame shows is the scan's: the entry's text changed since,
	// the page shows what was made of it then.
	e.Raw, e.Description = "别的 "+key, "别的 "+key
	tt.Frame()
	texts = strings.Join(tt.Texts(), "\n")
	if strings.Contains(texts, key) || !strings.Contains(texts, "带上 ••••") {
		t.Fatalf("a frame looked at the text again: %q", texts)
	}
}
