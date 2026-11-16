package skills

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// skill writes a skill's folder at dir.
func skill(t *testing.T, dir, name, desc string, extra ...string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	md := "---\nname: " + name + "\ndescription: " + desc + "\n---\n\n# " + name + "\n"
	if err := os.WriteFile(filepath.Join(p, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, f := range extra {
		fp := filepath.Join(p, f)
		_ = os.MkdirAll(filepath.Dir(fp), 0o755)
		if err := os.WriteFile(fp, []byte("echo hi\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

type world struct {
	env  Env
	home string
	repo string
}

func newWorld(t *testing.T) world {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	repo := filepath.Join(root, "code", "ergora")
	for _, d := range []string{filepath.Join(home, ".claude"), filepath.Join(home, ".codex"), repo} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	env := Env{Home: home, Store: filepath.Join(root, "maps", ".skills"), Machine: "here",
		Repos: []Repo{{Root: repo, Name: "ergora", Key: "ergora"}}, Table: Builtin(), Since: time.Now().AddDate(0, 0, -30)}
	return world{env, home, repo}
}

func (w world) tool(t *testing.T, id string) Tool {
	t.Helper()
	for _, tl := range w.env.Table {
		if tl.ID == id {
			return tl
		}
	}
	t.Fatalf("no tool %s", id)
	return Tool{}
}

func TestDetect(t *testing.T) {
	w := newWorld(t)
	skill(t, filepath.Join(w.home, ".newagent", "skills"), "a", "does a")
	_ = os.MkdirAll(filepath.Join(w.home, ".cache"), 0o755) // hidden, no skills: not a tool
	tools := Detect(w.home, Builtin())
	var ids []string
	for _, tl := range tools {
		ids = append(ids, tl.ID)
	}
	if got := strings.Join(ids, ","); got != "claude,codex,newagent" {
		t.Fatalf("tools %s", got)
	}
	if !tools[2].Found || tools[2].Global[0] != filepath.Join(".newagent", "skills") {
		t.Fatalf("found %+v", tools[2])
	}
}

func TestTableMerge(t *testing.T) {
	newer := Table([]byte(`[{"id":"codex","name":"Codex","home":".codex","global":[".agents/skills"]},{"id":"x","name":"X","home":".x","global":[".x/skills"]},{"id":"bad"}]`))
	if len(newer) != 2 {
		t.Fatalf("table %v", newer)
	}
	m := Merge(Builtin(), newer)
	for _, tl := range m {
		if tl.ID == "codex" && tl.Global[0] != ".agents/skills" {
			t.Fatal("codex not replaced")
		}
	}
	if m[len(m)-1].ID != "x" {
		t.Fatal("x not added")
	}
	if Table([]byte("nope")) != nil {
		t.Fatal("a table of nothing")
	}
}

func TestFrontmatter(t *testing.T) {
	meta, body, ok := Frontmatter("---\nname: a-b\ndescription: >\n  one\n  two\nother: \"x: y\"\n---\nbody\n")
	if !ok || meta["name"] != "a-b" || meta["description"] != "one two" || meta["other"] != "x: y" || body != "body\n" {
		t.Fatalf("%v %q %v", meta, body, ok)
	}
	if _, _, ok := Frontmatter("# no head"); ok {
		t.Fatal("no head read as one")
	}
}

func TestScan(t *testing.T) {
	w := newWorld(t)
	g := filepath.Join(w.home, ".claude", "skills")
	skill(t, g, "deploy", "ships it", "scripts/deploy.sh")
	skill(t, filepath.Join(w.home, ".codex", "skills"), "deploy", "ships it", "scripts/deploy.sh")
	skill(t, g, "Bad Name", "")
	skill(t, filepath.Join(w.repo, ".claude", "skills"), "deploy", "ergora's way")
	skill(t, filepath.Join(w.repo, ".claude", "skills"), "rules", "the rules")
	skill(t, filepath.Join(w.home, ".claude", "plugins", "cache", "mkt", "docs", "1.2.0", "skills"), "pdf", "reads pdfs")
	r := Scan(w.env)
	if len(r.Entries) != 5 {
		for _, e := range r.Entries {
			t.Log(e.Key)
		}
		t.Fatalf("%d entries", len(r.Entries))
	}
	d := r.Entry("g/deploy")
	if d == nil || len(d.Copies) != 2 || !d.Script || d.Copies["codex"].State != Own {
		t.Fatalf("deploy %+v", d)
	}
	if bad := r.Entry("g/Bad Name"); bad == nil || len(bad.Issues) < 2 {
		t.Fatalf("bad %+v", bad)
	}
	if p := r.Entry("p/ergora/deploy"); p == nil || !p.Shadowed || p.Repo == nil {
		t.Fatalf("project deploy %+v", p)
	}
	if p := r.Entry("x/claude/docs/pdf"); p == nil || p.Plugin != "docs" || p.Scope != Plugin {
		t.Fatalf("plugin %+v", p)
	}
	if n := r.Load("claude"); n != len("deploy")+len("ships it")+len("Bad Name")+len("pdf")+len("reads pdfs") {
		t.Fatalf("load %d", n)
	}
	on, off := r.Effective("claude", w.repo)
	if len(on) != 4 || len(off) != 1 {
		t.Fatalf("effective %d %d", len(on), len(off))
	}
}

func TestAdoptLinkAndBack(t *testing.T) {
	w := newWorld(t)
	cc := skill(t, filepath.Join(w.home, ".claude", "skills"), "review", "reviews")
	cx := skill(t, filepath.Join(w.home, ".codex", "skills"), "review", "reviews")
	r := Scan(w.env)
	e := r.Entry("g/review")
	undo, err := Adopt(w.env, e, "claude")
	if err != nil {
		t.Fatal(err)
	}
	r = Scan(w.env)
	e = r.Entry("g/review")
	if !e.Stored || e.Copies["claude"].State != Linked || e.Copies["codex"].State != Linked {
		t.Fatalf("after adopt %+v %+v", e.Copies["claude"], e.Copies["codex"])
	}
	// Written through a link, it is the one copy.
	if err := os.WriteFile(filepath.Join(cx, "notes.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(w.env.Store, "review", "notes.md")); err != nil {
		t.Fatal("not written to MyMind's copy")
	}
	if err := undo(); err != nil {
		t.Fatal(err)
	}
	if _, ok := linkTarget(cc); ok || !fileExists(filepath.Join(cc, "SKILL.md")) {
		t.Fatal("claude's not put back")
	}
	if exists(filepath.Join(w.env.Store, "review")) {
		t.Fatal("store kept it")
	}
	// Adopted again, linked to a third and taken out of one.
	_ = os.RemoveAll(cx)
	r = Scan(w.env)
	if _, err := Adopt(w.env, r.Entry("g/review"), "claude"); err != nil {
		t.Fatal(err)
	}
	r = Scan(w.env)
	e = r.Entry("g/review")
	un, err := Link(w.env, e, w.tool(t, "codex"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := linkTarget(cx); !ok {
		t.Fatal("no link in codex")
	}
	_ = un()
	if exists(cx) {
		t.Fatal("link not undone")
	}
	r = Scan(w.env)
	if _, err := Unlink(w.env, r.Entry("g/review"), "claude"); err != nil {
		t.Fatal(err)
	}
	r = Scan(w.env)
	if e := r.Entry("g/review"); !e.Stored || len(e.Copies) != 0 {
		t.Fatalf("after unlink %+v", e)
	}
}

func TestDivergedAndResolve(t *testing.T) {
	w := newWorld(t)
	skill(t, filepath.Join(w.home, ".claude", "skills"), "msg", "writes messages")
	r := Scan(w.env)
	if _, err := Adopt(w.env, r.Entry("g/msg"), "claude"); err != nil {
		t.Fatal(err)
	}
	r = Scan(w.env)
	if _, err := Link(w.env, r.Entry("g/msg"), w.tool(t, "codex")); err != nil {
		t.Fatal(err)
	}
	// Codex writes the skill again: a folder of its own, other words.
	cx := filepath.Join(w.home, ".codex", "skills", "msg")
	_ = os.Remove(cx)
	skill(t, filepath.Join(w.home, ".codex", "skills"), "msg", "writes better messages")
	r = Scan(w.env)
	e := r.Entry("g/msg")
	if e.Copies["codex"].State != Diverged {
		t.Fatalf("state %v", e.Copies["codex"].State)
	}
	undo, err := Resolve(w.env, e, "codex", true)
	if err != nil {
		t.Fatal(err)
	}
	r = Scan(w.env)
	e = r.Entry("g/msg")
	if e.Description != "writes better messages" || e.Copies["codex"].State != Linked || e.Copies["claude"].State != Linked {
		t.Fatalf("after taking theirs %q %v", e.Description, e.Copies["codex"].State)
	}
	if err := undo(); err != nil {
		t.Fatal(err)
	}
	r = Scan(w.env)
	if e := r.Entry("g/msg"); e.Description != "writes messages" || e.Copies["codex"].State != Diverged {
		t.Fatalf("undo %q", e.Description)
	}
}

func TestDeleteScopeRename(t *testing.T) {
	w := newWorld(t)
	skill(t, filepath.Join(w.home, ".claude", "skills"), "deploy", "ships")
	r := Scan(w.env)
	if _, err := Adopt(w.env, r.Entry("g/deploy"), "claude"); err != nil {
		t.Fatal(err)
	}
	tools := Detect(w.home, w.env.Table)
	r = Scan(w.env)
	repo := &w.env.Repos[0]
	undo, err := SetScope(w.env, r.Entry("g/deploy"), Project, repo, tools)
	if err != nil {
		t.Fatal(err)
	}
	r = Scan(w.env)
	p := r.Entry("p/ergora/deploy")
	if p == nil || p.Copies["claude"] == nil || p.Copies["claude"].State != Linked || r.Entry("g/deploy") != nil {
		t.Fatalf("to project %+v", p)
	}
	if err := undo(); err != nil {
		t.Fatal(err)
	}
	r = Scan(w.env)
	if g := r.Entry("g/deploy"); g == nil || g.Copies["claude"].State != Linked {
		t.Fatal("not back to every session's")
	}
	un, err := Rename(w.env, r.Entry("g/deploy"), "ergora-deploy", tools)
	if err != nil {
		t.Fatal(err)
	}
	r = Scan(w.env)
	if e := r.Entry("g/ergora-deploy"); e == nil || e.Title != "ergora-deploy" || e.Copies["claude"].State != Linked {
		t.Fatalf("renamed %+v", e)
	}
	_ = un()
	r = Scan(w.env)
	del, err := Delete(w.env, r.Entry("g/deploy"))
	if err != nil {
		t.Fatal(err)
	}
	if r = Scan(w.env); r.Entry("g/deploy") != nil {
		t.Fatal("still there")
	}
	if err := del(); err != nil {
		t.Fatal(err)
	}
	if r = Scan(w.env); r.Entry("g/deploy") == nil || r.Entry("g/deploy").Copies["claude"].State != Linked {
		t.Fatal("not back")
	}
}

func TestMakeRealAndTracked(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	w := newWorld(t)
	if out, err := exec.Command("git", "init", "-q", w.repo).CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	skill(t, filepath.Join(w.repo, ".claude", "skills"), "rules", "the rules")
	r := Scan(w.env)
	if _, err := Adopt(w.env, r.Entry("p/ergora/rules"), "claude"); err != nil {
		t.Fatal(err)
	}
	tools := Detect(w.home, w.env.Table)
	r = Scan(w.env)
	if _, err := Link(w.env, r.Entry("p/ergora/rules"), w.tool(t, "codex")); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", w.repo, "add", ".claude/skills/rules").CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	r = Scan(w.env)
	e := r.Entry("p/ergora/rules")
	if !e.Copies["claude"].Tracked || e.Copies["codex"].Tracked {
		t.Fatalf("tracked %v %v", e.Copies["claude"].Tracked, e.Copies["codex"].Tracked)
	}
	if _, err := MakeReal(w.env, e, tools); err != nil {
		t.Fatal(err)
	}
	cc := filepath.Join(w.repo, ".claude", "skills", "rules")
	if _, ok := linkTarget(cc); ok || !fileExists(filepath.Join(cc, "SKILL.md")) {
		t.Fatal("not real")
	}
	cx := filepath.Join(w.repo, ".codex", "skills", "rules")
	if tg, _ := os.Readlink(cx); filepath.IsAbs(tg) || !fileExists(filepath.Join(cx, "SKILL.md")) {
		t.Fatalf("codex's link %q", tg)
	}
	r = Scan(w.env)
	if e := r.Entry("p/ergora/rules"); e.Stored || e.Copies["codex"].State != Elsewhere {
		t.Fatalf("after %+v", e)
	}
}

func TestOtherComputers(t *testing.T) {
	w := newWorld(t)
	skill(t, filepath.Join(w.home, ".claude", "skills"), "sql", "writes sql")
	r := Scan(w.env)
	if _, err := Adopt(w.env, r.Entry("g/sql"), "claude"); err != nil {
		t.Fatal(err)
	}
	r = Scan(w.env)
	if err := Remember(w.env, r); err != nil {
		t.Fatal(err)
	}
	// The other computer: the store synced, no links of its own.
	other := w.env
	other.Machine = "there"
	other.Home = t.TempDir()
	_ = os.MkdirAll(filepath.Join(other.Home, ".claude"), 0o755)
	r2 := Scan(other)
	e := r2.Entry("g/sql")
	if e == nil || len(e.Copies) != 0 || len(e.Others["here"]) != 1 || e.Others["here"][0] != "claude" {
		t.Fatalf("other %+v", e)
	}
	if _, err := InstallHere(other, e, r2.Tools); err != nil {
		t.Fatal(err)
	}
	if e := Scan(other).Entry("g/sql"); e.Copies["claude"] == nil || e.Copies["claude"].State != Linked {
		t.Fatal("not linked there")
	}
}

func TestRepair(t *testing.T) {
	w := newWorld(t)
	skill(t, filepath.Join(w.home, ".claude", "skills"), "x", "does x")
	r := Scan(w.env)
	if _, err := Adopt(w.env, r.Entry("g/x"), "claude"); err != nil {
		t.Fatal(err)
	}
	// The maps folder moves.
	moved := filepath.Join(filepath.Dir(filepath.Dir(w.env.Store)), "maps2", ".skills")
	_ = os.MkdirAll(filepath.Dir(moved), 0o755)
	if err := os.Rename(w.env.Store, moved); err != nil {
		t.Fatal(err)
	}
	w.env.Store = moved
	r = Scan(w.env)
	if r.Entry("g/x").Copies["claude"].State != Dangling {
		t.Fatal("not dangling")
	}
	if n := Repair(w.env, r); n != 1 {
		t.Fatalf("repaired %d", n)
	}
	if Scan(w.env).Entry("g/x").Copies["claude"].State != Linked {
		t.Fatal("not linked again")
	}
}

func TestUsed(t *testing.T) {
	w := newWorld(t)
	skill(t, filepath.Join(w.home, ".claude", "skills"), "review", "reviews")
	dir := filepath.Join(w.home, ".claude", "projects", "p")
	_ = os.MkdirAll(dir, 0o755)
	log := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Skill","input":{"skill":"review"}}]}}` + "\n" +
		`{"type":"user","message":{"content":"<command-name>/review</command-name>"}}` + "\n" +
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Skill","input":{"skill":"docs:pdf"}}]}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "s.jsonl"), []byte(log), 0o644); err != nil {
		t.Fatal(err)
	}
	used := countUsed(w.env, Detect(w.home, w.env.Table))
	if used["review"] != 2 || used["pdf"] != 1 {
		t.Fatalf("used %v", used)
	}
}

func TestDiff(t *testing.T) {
	d := Diff("a\nb\nc", "a\nB\nc\nd")
	var s strings.Builder
	for _, l := range d {
		s.WriteByte(l.Kind)
		s.WriteString(l.Text)
		s.WriteByte('|')
	}
	if s.String() != " a|-b|+B| c|+d|" {
		t.Fatal(s.String())
	}
	if got := Changed(Diff("1\n2\n3\n4\n5\n6", "1\n2\n3\n4\n5\nX"), 1); len(got) != 4 || got[0].Kind != '~' {
		t.Fatalf("%v", got)
	}
}

// A tool's own skills, which it installs and updates, are of a kind of
// their own: by the table's folders, by a link into the tool's install,
// or as the user says.
func TestBundled(t *testing.T) {
	w := newWorld(t)
	// Codex's own, in its hidden folder.
	skill(t, filepath.Join(w.home, ".codex", "skills", ".system"), "skill-creator", "makes skills")
	// A link into the tool's install, as some tools put theirs.
	install := filepath.Join(w.home, ".codex", "vendor", "skills")
	skill(t, install, "imagegen", "draws")
	if err := os.Symlink(filepath.Join(install, "imagegen"), filepath.Join(w.home, ".codex", "skills", "imagegen")); err != nil {
		t.Fatal(err)
	}
	// A link to the user's own folder is the user's.
	mine := filepath.Join(w.home, "code", "my-skills")
	skill(t, mine, "notes", "takes notes")
	if err := os.Symlink(filepath.Join(mine, "notes"), filepath.Join(w.home, ".codex", "skills", "notes")); err != nil {
		t.Fatal(err)
	}
	// One the user says came with a tool.
	skill(t, filepath.Join(w.home, ".claude", "skills"), "builtin-ish", "")
	w.env.Marked = map[string]bool{"g/builtin-ish": true, "g/imagegen": false}

	r := Scan(w.env)
	sc := r.Entry("b/codex/skill-creator")
	if sc == nil || sc.Scope != Bundled || sc.By != "codex" || !sc.ReadOnly() {
		t.Fatalf("skill-creator %+v", sc)
	}
	if n := r.Load("codex"); n < len("skill-creator")+len("makes skills") {
		t.Errorf("a tool's own is loaded too: %d", n)
	}
	if e := r.Entry("g/notes"); e == nil || e.Scope != Global {
		t.Errorf("a link to the user's folder taken as a tool's: %+v", e)
	}
	if e := r.Entry("g/builtin-ish"); e == nil || e.Scope != Bundled || e.By != "claude" || !e.Marked {
		t.Errorf("marked by the user %+v", e)
	}
	if e := r.Entry("g/imagegen"); e == nil || e.Scope != Global || !e.Marked {
		t.Errorf("said not to be a tool's own %+v", e)
	}
	w.env.Marked = nil
	if e := Scan(w.env).Entry("g/imagegen"); e == nil || e.Scope != Bundled || e.By != "codex" {
		t.Errorf("a link into the tool's install %+v", e)
	}
}

// What was taken in is given back, found by its links once the app has
// forgotten the way back: each tool its folder again, MyMind's put away.
func TestGiveBack(t *testing.T) {
	w := newWorld(t)
	cc := filepath.Join(w.home, ".claude", "skills")
	cx := filepath.Join(w.home, ".codex", "skills")
	skill(t, cc, "review", "reviews")
	skill(t, cx, "review", "reviews")
	skill(t, cc, "older", "taken in before")
	r := Scan(w.env)
	if _, err := Adopt(w.env, r.Entry("g/older"), "claude"); err != nil {
		t.Fatal(err)
	}
	lchtimes(t, filepath.Join(cc, "older"), time.Now().Add(-time.Hour)) // the link's own time
	r = Scan(w.env)
	if _, err := Adopt(w.env, r.Entry("g/review"), "claude"); err != nil {
		t.Fatal(err)
	}
	r = Scan(w.env)
	taken, at := r.LastTaken()
	if len(taken) != 1 || taken[0].Name != "review" || at.IsZero() {
		t.Fatalf("last taken %v", taken)
	}
	undo, err := GiveBack(w.env, taken[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{cc, cx} {
		p := filepath.Join(dir, "review")
		if _, ok := linkTarget(p); ok || !fileExists(filepath.Join(p, "SKILL.md")) {
			t.Fatalf("%s not a folder of its own again", p)
		}
	}
	if exists(filepath.Join(w.env.Store, "review")) {
		t.Fatal("MyMind's copy stayed")
	}
	if e := Scan(w.env).Entry("g/review"); e == nil || e.Stored || len(e.Copies) != 2 {
		t.Fatalf("after %+v", e)
	}
	if err := undo(); err != nil {
		t.Fatal(err)
	}
	if e := Scan(w.env).Entry("g/review"); e == nil || !e.Stored || e.Copies["codex"].State != Linked {
		t.Fatalf("not taken back %+v", e)
	}
}

// ~/.agents/skills is the tools' common place, not a tool: its skills are
// Codex's, which reads it.
func TestSharedFolder(t *testing.T) {
	w := newWorld(t)
	skill(t, filepath.Join(w.home, ".agents", "skills"), "find-docs", "finds docs")
	tools := Detect(w.home, Builtin())
	for _, tl := range tools {
		if tl.ID == "agents" {
			t.Fatal("the common folder taken for a tool")
		}
	}
	e := Scan(w.env).Entry("g/find-docs")
	if e == nil || e.Copies["codex"] == nil || e.Copies["claude"] != nil {
		t.Fatalf("find-docs %+v", e)
	}
}
