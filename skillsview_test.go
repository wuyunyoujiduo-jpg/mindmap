package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// writeSkill writes a skill's folder under dir.
func writeSkill(t *testing.T, dir, name, desc string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	head := "---\nname: " + name + "\n"
	if desc != "" {
		head += "description: " + desc + "\n"
	}
	if err := os.WriteFile(filepath.Join(p, "SKILL.md"), []byte(head+"---\n\n## 步骤\n1. 先读一遍要用到的文件\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func isLink(p string) bool {
	fi, err := os.Lstat(p)
	return err == nil && fi.Mode()&os.ModeSymlink != 0
}

// The agents' skills on 今天's left column: found, told where and how
// each stands; taken into MyMind with a link in its place, linked and
// unlinked by a switch, with their ways back; a tool newly installed
// offered them; a copy that took a link's place settled.
func TestSkillsPage(t *testing.T) {
	a, tt := newTestApp(t)
	home := os.Getenv("HOME")
	repo := filepath.Join(t.TempDir(), "ergora")
	cc, cx := filepath.Join(home, ".claude", "skills"), filepath.Join(home, ".codex", "skills")
	writeSkill(t, cc, "code-review", "按团队的清单审一遍改动")
	writeSkill(t, cx, "code-review", "按团队的清单审一遍改动")
	writeSkill(t, cc, "write-weekly", "")
	writeSkill(t, cc, "deploy", "发布到预发环境")
	writeSkill(t, filepath.Join(repo, ".claude", "skills"), "deploy", "ergora 自己的发布流程")
	writeSkill(t, filepath.Join(repo, ".claude", "skills"), "ergora-api-rules", "ergora 接口的约定")
	f := model.NewFile("ergora")
	f.Project = repo
	if err := f.Save(filepath.Join(a.dir, model.FileName("ergora"))); err != nil {
		t.Fatal(err)
	}
	a.refreshFiles()
	tt.Frame()
	if err := tt.Click("技能"); err != nil {
		t.Fatalf("no place: %v", err)
	}
	tt.Frame()
	tt.Frame()
	if a.place() != placeSkills {
		t.Fatal("not on 技能")
	}
	for _, want := range []string{"AI 技能", "2 个编程工具", "Claude Code", "Codex", "code-review", "write-weekly", "项目 · ergora", "有问题", "被全局的盖住", "未收进", "全部收进", "还有 2 个要你看一眼"} {
		if !tt.HasText(want) {
			t.Fatalf("no %q: %q", want, tt.Texts())
		}
	}
	snapshot(t, tt, "skills")

	// Taken in: the folder into MyMind, links in both tools.
	if err := tt.Click("code-review"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("收进 MyMind"); err != nil {
		t.Fatalf("no 收进: %v %q", err, tt.Texts())
	}
	tt.Frame()
	tt.Frame()
	store := filepath.Join(a.dir, ".skills", "code-review")
	if _, err := os.Stat(filepath.Join(store, "SKILL.md")); err != nil {
		t.Fatal("not in the store")
	}
	if !isLink(filepath.Join(cc, "code-review")) || !isLink(filepath.Join(cx, "code-review")) {
		t.Fatal("not links in the tools")
	}
	if !tt.HasText("已链接") || !tt.HasText("在哪些工具里") || !tt.HasText("放在哪一层") {
		t.Fatalf("details: %q", tt.Texts())
	}
	snapshot(t, tt, "skill-detail")
	// Out of Codex by its switch, and back by 撤销.
	if err := tt.Click("链接到 Codex"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if _, err := os.Lstat(filepath.Join(cx, "code-review")); err == nil {
		t.Fatal("still linked in Codex")
	}
	a.noticeUndo()
	tt.Frame()
	if !isLink(filepath.Join(cx, "code-review")) {
		t.Fatal("not back in Codex")
	}

	// A tool installed since: offered the skills.
	writeSkill(t, filepath.Join(home, ".newagent", "skills"), "its-own", "它自己的")
	a.skillsRescan()
	tt.Frame()
	if !tt.HasText("发现新装的 newagent") {
		t.Fatalf("no new tool: %q", tt.Texts())
	}
	snapshot(t, tt, "skills-fresh")
	if err := tt.Click("链接过去"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !isLink(filepath.Join(home, ".newagent", "skills", "code-review")) {
		t.Fatal("not linked to the new tool")
	}
	if tt.HasText("发现新装的 newagent") {
		t.Fatal("still new")
	}

	// Codex writes the skill anew: a copy of its own; taken as MyMind's.
	_ = os.Remove(filepath.Join(cx, "code-review"))
	writeSkill(t, cx, "code-review", "按团队的清单审一遍改动，先列会出错的地方")
	a.skillsRescan()
	tt.Frame()
	if !tt.HasText("有一份不一样的") {
		t.Fatalf("not told: %q", tt.Texts())
	}
	if err := tt.Click("看 Codex 的差异"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("用这份，换回链接") {
		t.Fatalf("no choice: %q", tt.Texts())
	}
	snapshot(t, tt, "skill-diff")
	if err := tt.Click("用这份，换回链接"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	data, _ := os.ReadFile(filepath.Join(store, "SKILL.md"))
	if !strings.Contains(string(data), "先列会出错的地方") || !isLink(filepath.Join(cx, "code-review")) {
		t.Fatalf("not settled: %s", data)
	}

	// Moved into the project and back.
	e := a.skills.report.Entry("g/code-review")
	a.skillToRepo(e, a.skills.env.Repos[0])
	tt.Frame()
	if !isLink(filepath.Join(repo, ".claude", "skills", "code-review")) || isLink(filepath.Join(cc, "code-review")) {
		t.Fatal("not moved to the project")
	}
	a.noticeUndo()
	tt.Frame()
	if !isLink(filepath.Join(cc, "code-review")) {
		t.Fatal("not back")
	}

	// Deleted, and back.
	a.skillDelete(a.skills.report.Entry("g/code-review"))
	tt.Frame()
	if _, err := os.Stat(store); err == nil {
		t.Fatal("not deleted")
	}
	a.noticeUndo()
	tt.Frame()
	if _, err := os.Stat(filepath.Join(store, "SKILL.md")); err != nil || !isLink(filepath.Join(cc, "code-review")) {
		t.Fatal("not back from the trash")
	}
}

// With no agent at all, the page says so.
func TestSkillsNone(t *testing.T) {
	_, tt := newTestApp(t)
	tt.Frame()
	tt.Key(ui.Cmd|ui.Alt, ui.Key4)
	tt.Frame()
	tt.Frame()
	if !tt.HasText("没有找到 AI 编程工具") {
		t.Fatalf("no empty state: %q", tt.Texts())
	}
}

// The table of the agents, newer from the network once a day, adds an
// agent the app was built without.
func TestSkillsTableFetched(t *testing.T) {
	a, _ := newTestApp(t)
	got := ""
	a.pluginGet = func(_ context.Context, url string) ([]byte, error) {
		got = url
		return []byte(`[{"id":"later","name":"Later Agent","home":".later","global":[".later/skills"],"project":[".later/skills"]}]`), nil
	}
	a.skillsFetchTable()
	if got != skillsTableURL {
		t.Fatalf("fetched %q", got)
	}
	table := skillsTable()
	if table[len(table)-1].ID != "later" || table[0].ID != "claude" {
		t.Fatalf("table %v", table)
	}
	got = ""
	a.skillsFetchTable()
	if got != "" {
		t.Fatal("fetched again the same day")
	}
	writeSkill(t, filepath.Join(os.Getenv("HOME"), ".later", "skills"), "x", "does x")
	a.skillsRescan()
	if _, ok := a.skills.report.Tool("later"); !ok {
		t.Fatal("the newer table's agent not found")
	}
}

// What every tool keeps a copy of is taken in by one click: one copy in
// MyMind, links in its place, all of it back by one 撤销; the left column
// narrows by a tool and a part together.
func TestSkillsTakeAll(t *testing.T) {
	a, tt := newTestApp(t)
	home := os.Getenv("HOME")
	var tools []string
	for _, tl := range []string{".claude", ".codex", ".kiro", ".trae"} {
		dir := filepath.Join(home, tl, "skills")
		tools = append(tools, dir)
		for _, name := range []string{"pdf", "docx", "frontend-design"} {
			writeSkill(t, dir, name, "同一份："+name)
		}
	}
	writeSkill(t, filepath.Join(home, ".claude", "skills"), "keys", "TOKEN=abcdefghijklmnop1234")
	a.showSkills()
	tt.Frame()
	tt.Frame()
	if !tt.HasText("3 个技能还没收进 MyMind") || !tt.HasText("其中 3 个在好几个工具里各存了一份。收进来只留一份，各工具里换成指向它的链接，用起来和现在一样；以后改一处，处处都改了。另有 1 个像含密钥，先不收。") {
		t.Fatalf("no suggestion: %q", tt.Texts())
	}
	snapshot(t, tt, "skills-take-all")
	if err := tt.Click("全部收进"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	for _, dir := range tools {
		for _, name := range []string{"pdf", "docx", "frontend-design"} {
			if !isLink(filepath.Join(dir, name)) {
				t.Fatalf("%s/%s not a link", dir, name)
			}
		}
	}
	if isLink(filepath.Join(tools[0], "keys")) {
		t.Fatal("the one with a key taken in")
	}
	if !tt.HasText("收进了 3 个技能，各工具里换成了链接") || tt.HasText("全部收进") {
		t.Fatalf("after: %q", tt.Texts())
	}
	// The left column: a tool and a part, together, each with its ✕.
	if err := tt.Click("kiro"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("要处理"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("都好好的") {
		t.Fatalf("kiro's to look at: %q", tt.Texts())
	}
	if err := tt.Click("去掉 要处理"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("frontend-design") || tt.HasText("keys") {
		t.Fatalf("kiro's: %q", tt.Texts())
	}
	a.noticeUndo()
	tt.Frame()
	for _, dir := range tools {
		if isLink(filepath.Join(dir, "pdf")) {
			t.Fatalf("%s not put back", dir)
		}
	}
}

// A tool's own skills, which it installs and updates itself, are shown
// and left alone: not taken in, not to be mended, not 要处理; and one
// the user says came with a tool is so too, with the way back.
func TestSkillsBundled(t *testing.T) {
	a, tt := newTestApp(t)
	home := os.Getenv("HOME")
	sys := filepath.Join(home, ".codex", "skills", ".system")
	writeSkill(t, sys, "skill-creator", "")                             // no description: not the user's to mend
	writeSkill(t, sys, "skill-installer", "TOKEN=abcdefghijklmnop1234") // what looks like a key, the tool's
	writeSkill(t, filepath.Join(home, ".codex", "skills"), "release", "")
	writeSkill(t, filepath.Join(home, ".claude", "skills"), "deploy", "发布")
	a.showSkills()
	tt.Frame()
	tt.Frame()
	if !tt.HasText("工具自带的") || !tt.HasText("skill-creator") || !tt.HasText("工具自带，只读") {
		t.Fatalf("not shown: %q", tt.Texts())
	}
	if n := a.skillsNeedCount(); n != 1 { // release's missing description alone
		t.Fatalf("要处理 %d", n)
	}
	if !tt.HasText("2 个技能还没收进 MyMind") {
		t.Fatalf("a tool's own offered to take in: %q", tt.Texts())
	}
	if err := tt.Click("skill-creator"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	snapshot(t, tt, "skills-bundled")
	all := strings.Join(tt.Texts(), "\n")
	if !strings.Contains(all, "Codex 自带 ·") || strings.Contains(all, "没有 description") || slices.Contains(tt.Texts(), "收进 MyMind") {
		t.Fatalf("its details: %q", tt.Texts())
	}

	// The user says release came with Codex: left alone too, and back.
	a.skills.sel = "g/release"
	tt.Frame()
	if err := tt.Click("标为工具自带 release"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if e := a.skills.report.Entry("g/release"); e == nil || e.Scope != 3 || a.skillsNeedCount() != 0 {
		t.Fatalf("not marked: %+v", e)
	}
	a.noticeUndo()
	tt.Frame()
	tt.Frame()
	if a.skillsNeedCount() != 1 || len(a.cfg.SkillsMarked) != 0 {
		t.Fatal("not taken back")
	}
}
