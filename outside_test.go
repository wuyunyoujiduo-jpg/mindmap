package main

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/ghsync"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/versions"
)

// The open map, with changes not saved, meets its file changed outside
// (another computer's edits, brought by the sync): the two are merged,
// topic by topic, as one step undo takes back, and both are kept as
// versions. Before, a bar asked which of the two to keep whole, and
// 保留我的版本 threw the other computer's work away.
func TestOutsideChangeMerged(t *testing.T) {
	a, tt := newTestApp(t)
	tt.Frame()
	front := find(t, a, "前端").ID
	a.mutate("", func() { a.file.Node(front).Title = "界面层" })
	var added string
	changeOnDisk(t, a, func(f *model.File) {
		f.Sheets[0].Root.Title = "外部改名"
		n, _ := f.Sheets[0].AddChild(f.Sheets[0].Root.ID, "另一台电脑加的")
		added = n.ID
	}, 2*time.Minute)
	tt.Frame()
	if a.conflict || tt.HasText("保留我的版本") {
		t.Fatal("asked which to keep whole")
	}
	if a.file.Node(front).Title != "界面层" || a.file.Node(added) == nil || a.doc.Root.Title != "外部改名" {
		t.Fatalf("not merged: %q, %v, %q", a.file.Node(front).Title, a.file.Node(added), a.doc.Root.Title)
	}
	// Saved, the file has both.
	a.now = func() time.Time { return testNow.Add(3 * time.Minute) }
	tt.Frame()
	f, err := model.Load(a.path)
	if err != nil || f.Node(front).Title != "界面层" || f.Node(added) == nil || a.dirty {
		t.Fatalf("the file: %v, dirty %v", err, a.dirty)
	}
	// Both sides are versions to go back to.
	var reasons []string
	for _, v := range a.versions.List(a.path) {
		reasons = append(reasons, v.Reason)
	}
	if !slices.Contains(reasons, versions.BeforeMerge) || !slices.Contains(reasons, versions.External) {
		t.Fatalf("versions %v", reasons)
	}
	// One step: undone, the map is as it was here before.
	a.undo()
	if a.file.Node(added) != nil || a.file.Node(front).Title != "界面层" || a.doc.Root.Title == "外部改名" {
		t.Fatal("undo did not take the merge back")
	}
	a.redo()

	// What both changed, each its own way, stays as it is here and is
	// asked, in words that do not say the other side is a computer.
	a.mutate("", func() { a.file.Node(front).Title = "我改的名字" })
	changeOnDisk(t, a, func(f *model.File) { f.Node(front).Title = "别处改的名字" }, 5*time.Minute)
	tt.Frame()
	tt.Frame()
	if a.file.Node(front).Title != "我改的名字" || !a.mergeOpen || !tt.HasText("你正在改的") || !tt.HasText("别处改的名字") {
		t.Fatalf("the clash is not asked: open %v, %q", a.mergeOpen, tt.Texts())
	}
	if err := tt.Click("别处改的"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("完成"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.file.Node(front).Title != "别处改的名字" {
		t.Fatalf("theirs not taken: %q", a.file.Node(front).Title)
	}

	// A file that does not read cannot be merged: the bar asks.
	a.mutate("", func() { a.file.Node(front).Title = "又改了" })
	os.WriteFile(a.path, []byte("{ not a map"), 0o644)
	when := testNow.Add(9 * time.Minute)
	os.Chtimes(a.path, when, when)
	a.now = func() time.Time { return when }
	a.checked = time.Time{}
	tt.Frame()
	if !a.conflict {
		t.Fatal("a file that does not read was merged")
	}
}

// Leaving a map whose file changed outside, with changes not saved: they
// are merged and saved, not left behind.
func TestOutsideChangeMergedOnLeaving(t *testing.T) {
	a, tt := newTestApp(t)
	tt.Frame()
	path := a.path
	front := find(t, a, "前端").ID
	a.mutate("", func() { a.file.Node(front).Title = "界面层" })
	changeOnDisk(t, a, func(f *model.File) { f.Sheets[0].Root.Title = "外部改名" }, 2*time.Minute)
	a.checked = a.now() // not looked at yet
	a.newMap()
	f, err := model.Load(path)
	if err != nil || f.Node(front).Title != "界面层" || f.Sheets[0].Root.Title != "外部改名" {
		t.Fatalf("left behind: %v", err)
	}
}

// A title being typed on the canvas is not taken away when the file
// changes outside: the change is read when the title is done.
func TestTypedTitleSurvivesOutsideChange(t *testing.T) {
	a, tt := newTestApp(t)
	tt.Frame()
	a.selectNode(find(t, a, "前端").ID)
	a.addChild() // a new topic, its title being typed
	id := a.editing
	if id == "" {
		t.Fatal("not editing")
	}
	a.now = func() time.Time { return testNow.Add(time.Minute) }
	tt.Frame() // saved, as a pause in typing saves
	if a.dirty {
		t.Fatal("not saved")
	}
	a.editText = "正在打的标题"
	var added string
	changeOnDisk(t, a, func(f *model.File) {
		n, _ := f.Sheets[0].AddChild(f.Sheets[0].Root.ID, "钩子写的")
		added = n.ID
	}, 2*time.Minute)
	tt.Frame()
	tt.Frame()
	if a.editing != id || a.editText != "正在打的标题" {
		t.Fatalf("the title being typed is gone: editing %q, %q", a.editing, a.editText)
	}
	a.commitEdit()
	a.now = func() time.Time { return testNow.Add(3 * time.Minute) }
	a.checked = time.Time{}
	tt.Frame()
	tt.Frame()
	if n := a.file.Node(id); n == nil || n.Title != "正在打的标题" || a.file.Node(added) == nil {
		t.Fatalf("after typing: %v, the outside change %v", a.file.Node(id), a.file.Node(added))
	}
	f, _ := model.Load(a.path)
	if n := f.Node(id); n == nil || n.Title != "正在打的标题" || f.Node(added) == nil {
		t.Fatal("the file does not have both")
	}
}

// A save reads the maps' folders again only when something else wrote in
// them: not at each pause in typing.
func TestSaveReadsFoldersWhenTouched(t *testing.T) {
	a, tt := newTestApp(t)
	tt.Frame()
	a.mutate("", func() { find(t, a, "前端").Note = "一" })
	a.save() // the first looks, having not looked before
	// A mark in the list as it is in memory: gone if the folder is read again.
	a.tree.Files = append(a.tree.Files, model.FileInfo{Path: filepath.Join(a.dir, "记号"+model.Ext), Name: "记号"})
	a.mutate("", func() { find(t, a, "前端").Note = "二" })
	a.save()
	if !slices.ContainsFunc(a.tree.Files, func(f model.FileInfo) bool { return f.Name == "记号" }) {
		t.Fatal("the folder was read again though nothing else wrote in it")
	}
	if a.files[0].Path != a.path || !a.files[0].Modified.Equal(a.modTime) {
		t.Fatalf("the map saved is not first, as of now: %+v", a.files[0])
	}
	// Another map made outside, as by an agent: seen at the next save.
	time.Sleep(20 * time.Millisecond)
	if err := model.NewFile("外面建的").Save(filepath.Join(a.dir, model.FileName("外面建的"))); err != nil {
		t.Fatal(err)
	}
	a.mutate("", func() { find(t, a, "前端").Note = "三" })
	a.save()
	if !slicesHasName(a, "外面建的") || slicesHasName(a, "记号") {
		t.Fatal("a map made outside is not seen")
	}
}

// A sync's merge of a map keeps the map as it was here, to go back to;
// what it asks waits through a restart; and a map merged into another
// name is followed there.
func TestSyncMergeKeptAndAskedAgain(t *testing.T) {
	a, tt := newTestApp(t)
	a.cfg.SyncRepo, a.cfg.SyncToken = "me/notes", "github_pat_x"
	path := filepath.Join(a.dir, "AI.mindmap.json")
	base := model.NewFile("AI")
	base.Sheets[0].ID, base.Sheets[0].Root.ID = "s", "r"
	n, _ := base.Sheets[0].AddChild("r", "模型")
	gone, _ := base.Sheets[0].AddChild("r", "那边删掉的")
	ours, theirs := base.Clone(), base.Clone()
	ours.Node(n.ID).Title = "这台的名字"
	theirs.Node(n.ID).Title = "另一台的名字"
	theirs.Sheets[0].Delete(gone.ID)
	if err := ours.Save(path); err != nil {
		t.Fatal(err)
	}
	a.refreshFiles()
	b, _ := base.Marshal()
	o, _ := ours.Marshal()
	r, _ := theirs.Marshal()
	var state string
	a.syncRun = func(_ context.Context, dir string, cfg ghsync.Config, _ func(string)) (ghsync.Result, error) {
		state = cfg.State
		data, ok := cfg.Merge("AI.mindmap.json", b, o, r, ghsync.MergeInfo{RemoteFrom: "Office-Mac"})
		if !ok {
			t.Fatal("not merged")
		}
		os.WriteFile(filepath.Join(dir, "AI.mindmap.json"), data, 0o644)
		return ghsync.Result{Pulled: 1, Written: []string{"AI.mindmap.json"}, Merged: []string{"AI.mindmap.json"}}, nil
	}
	a.syncNow()
	tt.Frame()
	if state == "" || inside(state, a.dir) {
		t.Fatalf("the sync's state is kept in the folder: %q", state)
	}
	// The map as it was here is a version: the topic the other computer
	// deleted is to be found there.
	vs := a.versions.List(path)
	if len(vs) == 0 || vs[0].Reason != versions.BeforeMerge {
		t.Fatalf("no version of before the merge: %+v", vs)
	}
	if f, err := a.versions.Load(vs[0]); err != nil || f.Node(gone.ID) == nil {
		t.Fatal("the version of before does not have what was deleted")
	}
	if len(a.merges) != 1 || !a.mergeOpen {
		t.Fatal("not asked")
	}

	// The app closes and opens: the question is asked again.
	a.flush()
	b2 := newApp(&fakePlatform{}, a.cfg)
	b2.noLocalModel, b2.noMotion = true, true
	b2.cfg.SyncOff = true // asked again as the app opens, with no sync run
	tb := ui.NewTester(b2.view, 1280, 800)
	tb.Frame()
	if len(b2.merges) != 1 || !b2.mergeOpen || !tb.HasText("另一台的名字") || !tb.HasText("另一台电脑 · Office-Mac") {
		t.Fatalf("the question did not wait through a restart: %d, %q", len(b2.merges), tb.Texts())
	}
	if err := tb.Click("另一台电脑 · Office-Mac"); err != nil {
		t.Fatal(err)
	}
	tb.Frame()
	if err := tb.Click("完成"); err != nil {
		t.Fatal(err)
	}
	tb.Frame()
	if f, _ := model.Load(path); f.Node(n.ID).Title != "另一台的名字" {
		t.Fatalf("title %q", f.Node(n.ID).Title)
	}
	if _, err := os.Stat(mergesPath(b2.dir)); err == nil {
		t.Fatal("answered, and kept still")
	}
	// The note of a change the app made to a map not open shows among its versions.
	notes := b2.versions.WithNotes(b2.versions.List(path))
	if !slices.ContainsFunc(notes, func(v versions.Version) bool { return v.Note == "合并时选了另一台电脑的" }) {
		t.Fatalf("the note is not shown: %+v", notes)
	}
}

// A sync that fails after files came still has the app read them; and
// the paths it could not write are said in the settings.
func TestFailedSyncReadsWhatCame(t *testing.T) {
	a, tt := newTestApp(t)
	a.cfg.SyncRepo, a.cfg.SyncToken = "me/notes", "github_pat_x"
	fail := true
	a.syncRun = func(_ context.Context, dir string, cfg ghsync.Config, _ func(string)) (ghsync.Result, error) {
		if !fail {
			return ghsync.Result{Unwritten: []string{"什么?.mindmap.json"}}, nil
		}
		model.NewFile("另一台电脑").Save(filepath.Join(dir, "另一台电脑.mindmap.json"))
		return ghsync.Result{Written: []string{"另一台电脑.mindmap.json"}}, errors.New("连不上 GitHub")
	}
	a.syncNow()
	tt.Frame()
	if a.syncErr == "" || !slicesHasName(a, "另一台电脑") {
		t.Fatalf("err %q, the map that came is listed: %v", a.syncErr, slicesHasName(a, "另一台电脑"))
	}
	fail = false
	a.syncNow()
	a.openSyncSettings()
	tt.Frame()
	if !tt.HasText("1 个文件在这台电脑上写不了（文件名这个系统不接受，或文件夹不让写），先留在 GitHub 上，其余的照常同步：什么?.mindmap.json") {
		t.Fatalf("not said: %q", tt.Texts())
	}
}

// The state a sync of before kept in the maps folder is moved out of it
// the first time this version runs, and never after: one found later is a
// backup's or another computer's.
func TestSyncStateAdoptedOnce(t *testing.T) {
	a, _ := newTestApp(t)
	a.cfg.SyncRepo, a.cfg.SyncToken = "me/notes", "github_pat_x"
	a.cfg.SyncStateOut = false
	old := filepath.Join(a.dir, ".sync", "github.json")
	os.MkdirAll(filepath.Dir(old), 0o755)
	os.WriteFile(old, []byte(`{"repo":"me/notes","commit":"abc"}`), 0o644)
	a.syncRun = func(context.Context, string, ghsync.Config, func(string)) (ghsync.Result, error) {
		return ghsync.Result{}, nil
	}
	a.syncDue()
	if data, _ := os.ReadFile(syncStatePath(a.dir)); !strings.Contains(string(data), `"abc"`) {
		t.Fatalf("not adopted: %q", data)
	}
	if _, err := os.Stat(old); err == nil || !a.cfg.SyncStateOut {
		t.Fatal("left in the folder")
	}
	// A backup put back brings its own: left alone.
	os.Remove(syncStatePath(a.dir))
	os.MkdirAll(filepath.Dir(old), 0o755)
	os.WriteFile(old, []byte(`{"repo":"me/notes","commit":"old"}`), 0o644)
	a.syncNow()
	if _, err := os.Stat(syncStatePath(a.dir)); err == nil {
		t.Fatal("a state found later was adopted")
	}
}

// A map the sync merged into another name: the app follows it there, the
// open map, its versions and the pictures added under the old name.
func TestSyncRenameFollowed(t *testing.T) {
	a, tt := newTestApp(t)
	a.cfg.SyncRepo, a.cfg.SyncToken = "me/notes", "github_pat_x"
	from := filepath.Join(a.dir, "计划.mindmap.json")
	to := filepath.Join(a.dir, "规划.mindmap.json")
	f := model.NewFile("计划")
	if err := f.Save(from); err != nil {
		t.Fatal(err)
	}
	a.refreshFiles()
	if err := a.open(from); err != nil {
		t.Fatal(err)
	}
	a.cfg.Favorites = []string{from}
	a.keepVersion(versions.Auto)
	os.MkdirAll(model.AssetsDir(from), 0o755)
	os.WriteFile(filepath.Join(model.AssetsDir(from), "p.png"), []byte("png"), 0o644)
	a.syncRun = func(_ context.Context, dir string, cfg ghsync.Config, _ func(string)) (ghsync.Result, error) {
		g := f.Clone()
		g.Sheets[0].Root.Title = "规划"
		data, _ := g.Marshal()
		old, _ := f.Marshal()
		if !cfg.Same("计划.mindmap.json", "规划.mindmap.json", old, data) {
			t.Fatal("one map under two names is not known as one")
		}
		other, _ := model.NewFile("别的").Marshal()
		if cfg.Same("计划.mindmap.json", "规划.mindmap.json", old, other) || cfg.Same("计划.mindmap.json", "规划.txt", old, data) {
			t.Fatal("another map is taken as the same")
		}
		os.WriteFile(filepath.Join(dir, "规划.mindmap.json"), data, 0o644)
		os.Remove(filepath.Join(dir, "计划.mindmap.json"))
		return ghsync.Result{Pulled: 1, Written: []string{"计划.mindmap.json", "规划.mindmap.json"},
			Merged: []string{"规划.mindmap.json"}, Renamed: map[string]string{"计划.mindmap.json": "规划.mindmap.json"}}, nil
	}
	a.syncNow()
	tt.Frame()
	if a.path != to || a.cfg.Favorites[0] != to || a.cfg.File != to {
		t.Fatalf("not followed: %q, %v", a.path, a.cfg.Favorites)
	}
	if a.doc.Root.Title != "规划" {
		t.Fatalf("the map shown is not the one merged: %q", a.doc.Root.Title)
	}
	if len(a.versions.List(to)) == 0 {
		t.Fatal("the versions stayed behind")
	}
	if _, err := os.Stat(filepath.Join(model.AssetsDir(to), "p.png")); err != nil {
		t.Fatal("the picture added under the old name stayed behind")
	}
	// An edit now is saved under the new name, and the old does not come back.
	a.now = func() time.Time { return testNow.Add(time.Hour) }
	a.checked = time.Time{}
	tt.Frame()
	a.mutate("", func() { a.doc.Root.Note = "之后改的" })
	a.save()
	if _, err := os.Stat(from); err == nil {
		t.Fatal("the old name came back")
	}
}

// The state of the code since the baseline is looked at away from the
// frame that asks.
func TestCodeStatusNotInTheFrame(t *testing.T) {
	a, tt := newTestApp(t)
	tt.Frame()
	a.file.Baseline = &model.Baseline{Commit: "abc"}
	var work func() (any, error)
	var done func(any, error)
	a.async = func(w func() (any, error), d func(any, error)) { work, done = w, d }
	dir := t.TempDir()
	if _, known := a.codeStatusOf(dir); known || work == nil {
		t.Fatalf("git was run in the frame: known %v", known)
	}
	asked := work
	work = nil
	if _, known := a.codeStatusOf(dir); known || work != nil {
		t.Fatal("asked twice while it is looked at")
	}
	done(asked()) // no repository there: said, not hidden
	if st, known := a.codeStatusOf(dir); !known || st.err == "" || work != nil {
		t.Fatalf("once looked at: known %v, %+v", known, st)
	}
}

// The folder of a map's code is this computer's: chosen here, it is kept
// here, and another computer's, in the file, is left as it is.
func TestProjectFolderPerComputer(t *testing.T) {
	a, tt := newTestApp(t)
	tt.Frame()
	here := t.TempDir()
	// A file of before, written on this computer: read still.
	a.file.Project = here
	if a.projectDir() != here {
		t.Fatalf("a file's own folder is not read: %q", a.projectDir())
	}
	// The other computer's folder came with the sync: it is not here.
	a.file.Project, a.file.Repo = "/Users/someone-else/code/app", "github.com/me/app"
	if a.projectDir() != filepath.Dir(a.path) {
		t.Fatalf("another computer's folder is used: %q", a.projectDir())
	}
	a.setProject(here)
	if a.projectDir() != here || a.cfg.Projects[a.path] != here {
		t.Fatalf("not set here: %q %v", a.projectDir(), a.cfg.Projects)
	}
	if a.file.Project != "/Users/someone-else/code/app" || a.file.Repo != "github.com/me/app" {
		t.Fatalf("the other computer's folder was overwritten: %q %q", a.file.Project, a.file.Repo)
	}
	// Renamed, the map keeps it.
	a.rename(a.path, "改了名")
	if a.projectDir() != here {
		t.Fatalf("lost with the rename: %v", a.cfg.Projects)
	}
	// Cleared here, the file's is left still.
	a.setProject("")
	if a.cfg.Projects[a.path] != "" || a.file.Project != "/Users/someone-else/code/app" {
		t.Fatal("cleared there too")
	}
	// A file with none, or this computer's own, takes what is chosen.
	a.file.Project = ""
	a.setProject(here)
	if a.file.Project != here {
		t.Fatalf("not written where there was none: %q", a.file.Project)
	}
	other := t.TempDir()
	a.setProject(other)
	if a.file.Project != other || a.projectDir() != other {
		t.Fatalf("this computer's own is not changed: %q", a.file.Project)
	}
}

// A rename is followed by all that keeps a map's or a folder's path.
func TestRenameFollowedEverywhere(t *testing.T) {
	a, tt := newTestApp(t)
	tt.Frame()
	dir := filepath.Join(a.dir, "客户")
	os.Mkdir(dir, 0o755)
	a.moveMap(a.path, dir)
	path := a.path
	a.remember(path)
	a.setFolderOpen(dir, true)
	var told []string
	was := pathFollowers
	pathFollowers = append(pathFollowers, func(_ *app, follow func(string) string) { told = append(told, follow(path)) })
	t.Cleanup(func() { pathFollowers = was })
	a.rename(dir, "老客户")
	now := filepath.Join(a.dir, "老客户", filepath.Base(path))
	if a.path != now || a.cfg.Recent[0] != now || !a.folderOpen(filepath.Join(a.dir, "老客户")) || a.folderOpen(dir) {
		t.Fatalf("path %q, recent %v, open %v", a.path, a.cfg.Recent, a.cfg.OpenFolders)
	}
	if len(told) != 1 || told[0] != now {
		t.Fatalf("the followers were told %v", told)
	}
}

// A backup holds the AI agents' skills, which are kept out of the maps
// folder.
func TestBackupHoldsSkills(t *testing.T) {
	a, _ := newTestApp(t)
	home, _ := os.UserHomeDir()
	skill := filepath.Join(skillsStore(home), "review", "SKILL.md")
	os.MkdirAll(filepath.Dir(skill), 0o755)
	os.WriteFile(skill, []byte("# Review"), 0o644)
	dst := filepath.Join(t.TempDir(), "b.zip")
	var failed error
	a.backupTo(dst, func(err error) { failed = err })
	if failed != nil {
		t.Fatal(failed)
	}
	zr, err := zip.OpenReader(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	if !slices.Contains(names, skillsInBackup+"/review/SKILL.md") {
		t.Fatalf("no skills in the backup: %v", names)
	}
	if !slices.ContainsFunc(names, func(n string) bool { return strings.HasPrefix(n, filepath.Base(a.dir)+"/") }) {
		t.Fatalf("no maps in the backup: %v", names)
	}
}

// A file is attached by a copy made away from the window's work.
func TestAttachCopiesAway(t *testing.T) {
	a, tt := newTestApp(t)
	tt.Frame()
	src := filepath.Join(t.TempDir(), "报告.PDF")
	os.WriteFile(src, []byte("pdf"), 0o644)
	n := find(t, a, "前端")
	var work func() (any, error)
	var done func(any, error)
	a.async = func(w func() (any, error), d func(any, error)) { work, done = w, d }
	if err := a.attachFile(n, src); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(model.AssetsDir(a.path)); len(entries) != 0 || len(n.Files) != 0 {
		t.Fatal("copied in the call, on the window's thread")
	}
	done(work())
	n = find(t, a, "前端")
	if len(n.Files) != 1 || n.Files[0].Name != "报告.PDF" || !strings.HasSuffix(n.Files[0].File, ".pdf") {
		t.Fatalf("not attached: %+v", n.Files)
	}
	if data, _ := os.ReadFile(filepath.Join(model.AssetsDir(a.path), n.Files[0].File)); string(data) != "pdf" {
		t.Fatalf("the copy: %q", data)
	}
	if err := a.attachFile(n, filepath.Join(t.TempDir(), "没有.txt")); err == nil {
		t.Fatal("a file that is not there")
	}
}
