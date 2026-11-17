package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// A topic links to a skill of the AI agents as it does to a clip: by
// 链接到技能… of its links and by [[ in its note; a thought by [[ too.
// The link opens the skill, the skill says what names it and goes there,
// and is linked from its own details as well; one that is no more says so.
func TestSkillLinks(t *testing.T) {
	a, tt := newTestApp(t)
	home := os.Getenv("HOME")
	writeSkill(t, filepath.Join(home, ".claude", "skills"), "release", "发版本的步骤")
	writeSkill(t, filepath.Join(home, ".claude", "skills"), "code-review", "按团队的清单审一遍改动")
	a.skillsRescan()
	tt.Frame()
	tt.Frame()
	if a.skills.report == nil || a.skillNamed("release") == nil {
		t.Fatal("no scan")
	}

	// From the topic's links: the palette lists the skills, by name and
	// by what they say.
	login := find(t, a, "登录")
	a.selectNode(login.ID)
	a.panelTab = tabNode
	tt.Frame()
	if err := tt.Click("＋ 链接"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("链接到技能…"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Frame()
	if !a.paletteOpen || a.paletteMode != paletteSkill || len(a.paletteItems()) != 2 {
		t.Fatalf("palette %v %d: %d items", a.paletteOpen, a.paletteMode, len(a.paletteItems()))
	}
	a.paletteQuery = "发版"
	items := a.paletteItems()
	if len(items) != 1 || items[0].title != "release" {
		t.Fatalf("items %+v", items)
	}
	snapshot(t, tt, "skill-link-palette")
	items[0].run()
	a.paletteOpen = false
	login = find(t, a, "登录")
	if len(login.Links) != 1 || login.Links[0] != "skill:release" {
		t.Fatalf("links %v", login.Links)
	}
	if icon, label := a.linkLabel("skill:release"); icon != "🧩" || label != "release" {
		t.Fatalf("label %s %s", icon, label)
	}
	tt.Frame()
	snapshot(t, tt, "skill-link-node")

	// [[ in another topic's note offers it, and the note's link counts.
	slow := find(t, a, "列表加载慢")
	a.selectNode(slow.ID)
	tt.Frame()
	tt.Frame()
	d := a.noteDocs["panel"]
	if d == nil || d.id != slow.ID {
		t.Fatalf("no note editor for the node: %+v", d)
	}
	d.doc.SetSelection(d.doc.Text().Len(), d.doc.Text().Len())
	d.doc.InsertText("改完按 [[review")
	tt.Frame()
	tt.Frame()
	if err := tt.Click("链接到 code-review"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Frame()
	tt.Frame()
	slow = find(t, a, "列表加载慢")
	if !strings.Contains(slow.Note, "](skill:code-review)") {
		t.Fatalf("note %q", slow.Note)
	}
	if back := a.skillBacklinks("code-review"); len(back) != 1 || back[0].id != slow.ID {
		t.Fatalf("backlinks %+v", back)
	}

	// A thought names one by [[ as well.
	a.showLibrary()
	a.tasks.open, a.barOpen = true, true
	a.homeDraft = "这套步骤写在 [[rele"
	tt.Frame()
	tt.Frame()
	if err := tt.Click("链接到 release"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Frame()
	a.saveHomeLine()
	tt.Frame()
	kept := a.clipsNamed("这套步骤", 1)
	if len(kept) != 1 || !strings.Contains(kept[0].Text, "<skill:release>") {
		t.Fatalf("kept %+v of %d, draft %q", kept, len(a.lib.Clips), a.homeDraft)
	}
	// And a task written so is to be done with it: the link is the
	// task's, not words of its title.
	a.barOpen, a.barKind = true, barATask
	a.homeDraft = "发 0.3 版 [[rele"
	tt.Frame()
	tt.Frame()
	if err := tt.Click("链接到 release"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Frame()
	a.saveHomeLine()
	tt.Frame()
	var task *model.Node
	a.mapFile(a.inboxPath()).Walk(func(n *model.Node) {
		if n.Title == "发 0.3 版" {
			task = n
		}
	})
	if task == nil || task.Task == nil || len(task.Links) != 1 || task.Links[0] != "skill:release" {
		t.Fatalf("the task: %+v", task)
	}
	a.barKind = barAuto
	back := a.skillBacklinks("release")
	if len(back) != 3 {
		t.Fatalf("backlinks %+v", back)
	}

	// The link opens the skill, which says what names it.
	a.skills.side, a.skills.find = "needs", "nothing"
	a.openLink("skill:release")
	tt.Frame()
	tt.Frame()
	if a.place() != placeSkills || a.skills.sel != "g/release" || a.skills.side != "" || a.skills.find != "" {
		t.Fatalf("place %d sel %q side %q", a.place(), a.skills.sel, a.skills.side)
	}
	if !tt.HasText("被 3 处引用") || !tt.HasText("登录") {
		t.Fatalf("its uses: %q", tt.Texts())
	}
	snapshot(t, tt, "skill-uses")
	// And goes to the topic.
	if err := tt.Click("前往 登录"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.place() != placeMaps || a.selected != login.ID {
		t.Fatalf("place %d selected %q", a.place(), a.selected)
	}

	// From the skill: a topic of any map is chosen to name it.
	a.skillPlace(a.skillNamed("code-review"))
	a.paletteQuery = "待优化"
	items = a.paletteItems()
	if a.paletteMode != paletteAttach || len(items) == 0 {
		t.Fatalf("mode %d items %+v", a.paletteMode, items)
	}
	items[0].run()
	a.paletteOpen = false
	if other := find(t, a, "待优化"); len(other.Links) != 1 || other.Links[0] != "skill:code-review" {
		t.Fatalf("links %v", other.Links)
	}
	if a.notice == "" || !strings.Contains(a.notice, "待优化") {
		t.Fatalf("notice %q", a.notice)
	}

	// A skill deleted: its link says so, and opens nothing.
	if err := os.RemoveAll(filepath.Join(home, ".claude", "skills", "release")); err != nil {
		t.Fatal(err)
	}
	a.skillsRescan()
	tt.Frame()
	tt.Frame()
	if _, label := a.linkLabel("skill:release"); label != "（技能已不存在）" {
		t.Fatalf("label %q", label)
	}
	a.openLink("skill:release")
	if a.place() == placeSkills || !strings.Contains(a.saveErr, "release") {
		t.Fatalf("place %d err %q", a.place(), a.saveErr)
	}
}

// An outline's line ends with the skill it is to be done with, as an
// agent writes it.
func TestOutlineLinksToASkill(t *testing.T) {
	nodes := model.ParseOutline("- [ ] 发 0.3 <skill:release>")
	if len(nodes) != 1 || nodes[0].Title != "发 0.3" || len(nodes[0].Links) != 1 || nodes[0].Links[0] != "skill:release" {
		t.Fatalf("nodes %+v", nodes[0])
	}
}
