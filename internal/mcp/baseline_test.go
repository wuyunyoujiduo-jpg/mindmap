package mcp

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/versions"
)

func TestBaseline(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	repo := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	write := func(name, text string) {
		t.Helper()
		p := filepath.Join(repo, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q", "-b", "main")
	write("parse/lexer.go", "package parse\n")
	write("rules/a.md", "a\n")
	run("add", ".")
	run("commit", "-q", "-m", "first")

	dir := t.TempDir()
	f := model.NewFile("ergora")
	d := f.Sheets[0]
	parse, _ := d.AddChild(d.Root.ID, "解析管线")
	parse.Links = []string{"parse/"}
	lexer, _ := d.AddChild(parse.ID, "词法")
	lexer.Links = []string{"parse/lexer.go:12"}
	rules, _ := d.AddChild(d.Root.ID, "规则")
	rules.Links = []string{"rules"}
	path := filepath.Join(dir, model.FileName("ergora"))
	f.Save(path)
	ss := &session{t: t, s: &Server{Dir: dir, Versions: versions.Store{Dir: t.TempDir()}, Version: "1.0", Now: time.Now}}

	// Without a project folder, read_map says nothing of code.
	if text := ss.tool("read_map", map[string]any{"map": "ergora"}, false); strings.Contains(text, "代码基线") {
		t.Fatalf("baseline shown:\n%s", text)
	}
	ss.tool("set_baseline", map[string]any{"map": "ergora", "summary": "x"}, true) // no folder known
	msg := ss.tool("set_baseline", map[string]any{"map": "ergora", "project_dir": repo, "summary": "记录代码基线"}, false)
	if !strings.Contains(msg, "main") {
		t.Fatalf("set: %s", msg)
	}
	if text := ss.tool("read_map", map[string]any{"map": "ergora"}, false); !strings.Contains(text, "没有改动") || !strings.Contains(text, "first") {
		t.Fatalf("up to date:\n%s", text)
	}

	write("parse/lexer.go", "package parse\n\n// more\n")
	write("parse/parser.go", "package parse\n")
	write("rules/a.md", "b\n")
	run("add", ".")
	run("commit", "-q", "-m", "second")
	write("web/app.ts", "x\n") // not committed, and no node for it
	text := ss.tool("read_map", map[string]any{"map": "ergora"}, false)
	for _, want := range []string{
		"1 个提交", "second",
		"M parse/lexer.go → 「词法」{id:" + lexer.ID + "}",
		"A parse/parser.go → 「解析管线」{id:" + parse.ID + "}",
		"M rules/a.md → 「规则」",
		"A web/app.ts → （没有节点链接它）",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("no %q in:\n%s", want, text)
		}
	}

	ss.tool("set_baseline", map[string]any{"map": "ergora", "summary": "更新代码基线"}, false)
	g, _ := model.Load(path)
	if g.Project != repo || g.Baseline == nil || g.Baseline.Subject != "second" || g.Baseline.Branch != "main" {
		t.Fatalf("baseline %+v, project %q", g.Baseline, g.Project)
	}
	if text := ss.tool("read_map", map[string]any{"map": "ergora"}, false); !strings.Contains(text, "A web/app.ts") || strings.Contains(text, "M parse/lexer.go") {
		t.Fatalf("after update:\n%s", text)
	}
	ss.tool("set_baseline", map[string]any{"map": "ergora", "commit": "nope", "summary": "x"}, true)
}

// 76 of docs/审查清单.md, the server's half. The folder of a map's code is
// this computer's to know: a file that names another computer's is not
// looked in for the code, and a folder given here is kept here, the
// file's own left as the other computer needs it.
func TestProjectFolderIsThisComputers(t *testing.T) {
	code := repo(t, "shop")
	run(t, code, "remote", "add", "origin", "https://github.com/me/shop")
	dir := t.TempDir()
	other := filepath.Join(t.TempDir(), "the", "other", "computer", "shop") // no folder of this one
	save := func(name, project, repo string) string {
		f := model.NewFile(name)
		f.Project, f.Repo = project, repo
		path := filepath.Join(dir, model.FileName(name))
		if err := f.Save(path); err != nil {
			t.Fatal(err)
		}
		return path
	}
	foreign := save("别处的", other, "")
	ss := &session{t: t, s: &Server{Dir: dir, Versions: versions.Store{Dir: t.TempDir()}, Now: time.Now}}
	// Another computer's folder is no folder here: nothing of code is said,
	// and no baseline is set from it.
	if text := ss.tool("read_map", map[string]any{"map": "别处的"}, false); strings.Contains(text, "代码基线") || strings.Contains(text, other) {
		t.Fatalf("another computer's folder is read:\n%s", text)
	}
	ss.tool("set_baseline", map[string]any{"map": "别处的", "summary": "x"}, true)
	// Given here, it is kept here: the baseline is set, and the file still
	// names the other computer's, with the repository it had none of.
	ss.tool("set_baseline", map[string]any{"map": "别处的", "project_dir": code, "summary": "x"}, false)
	g, _ := model.Load(foreign)
	if g.Project != other || g.Baseline == nil || g.Repo != "github.com/me/shop" {
		t.Fatalf("the file: project %q, repo %q, baseline %v", g.Project, g.Repo, g.Baseline)
	}
	if !samePath(ProjectsHere()[foreign], code) || !samePath(ProjectOf(foreign, g), code) {
		t.Fatalf("not kept here: %v", ProjectsHere())
	}
	if text := ss.tool("read_map", map[string]any{"map": "别处的"}, false); !strings.Contains(text, "没有改动") {
		t.Fatalf("the code is not read where it is here:\n%s", text)
	}
	ss.tool("set_baseline", map[string]any{"map": "别处的", "summary": "x"}, false) // and again, with no folder given
	// A session in that folder finds the map by it, and makes no second one.
	in := &session{t: t, s: &Server{Dir: dir, Versions: versions.Store{Dir: t.TempDir()}, Now: time.Now, Cwd: code}}
	in.tool("capture", map[string]any{"text": "在这台记的"}, false)
	if files, _ := model.AllFilesIn(dir); len(files) != 1 {
		t.Fatalf("a second map was made: %v", files)
	}
	// A file with no folder, and one whose folder is of this computer, take
	// the one given, as the app's setProject does.
	none := save("没有的", "", "")
	ss.tool("set_baseline", map[string]any{"map": "没有的", "project_dir": code, "summary": "x"}, false)
	if g, _ := model.Load(none); !samePath(g.Project, code) || g.Repo != "github.com/me/shop" {
		t.Fatalf("a file with no folder: %q %q", g.Project, g.Repo)
	}
	here := save("这台的", t.TempDir(), "")
	ss.tool("set_baseline", map[string]any{"map": "这台的", "project_dir": code, "summary": "x"}, false)
	if g, _ := model.Load(here); !samePath(g.Project, code) {
		t.Fatalf("a folder of this computer is not replaced: %q", g.Project)
	}
	// Found by its repository alone, the map's code is where the session is.
	byRepo := save("按仓库的", other, "github.com/me/shop")
	_ = byRepo
	if text := in.tool("read_map", map[string]any{"map": "按仓库的"}, false); !strings.Contains(text, "项目代码目录") || strings.Contains(text, other) {
		t.Fatalf("the session's own folder is not the repository's here:\n%s", text)
	}
}
