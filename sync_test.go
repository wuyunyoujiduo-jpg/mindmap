package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/ghsync"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// The sync is set up in the settings, runs when asked, by itself after a
// change, and says how it stands in the sidebar.
func TestSyncWithGitHub(t *testing.T) {
	a, tt := newTestApp(t)
	clock := testNow
	a.now = func() time.Time { return clock }
	var runs []ghsync.Config
	a.syncCheckRepo = func(_ context.Context, cfg ghsync.Config) (*ghsync.Repo, error) {
		return &ghsync.Repo{FullName: cfg.Repo, Private: true}, nil
	}
	a.syncRun = func(_ context.Context, dir string, cfg ghsync.Config, progress func(string)) (ghsync.Result, error) {
		runs = append(runs, cfg)
		progress("正在下载 1 个文件…")
		if len(runs) == 1 { // a map made on another computer comes down
			os.WriteFile(filepath.Join(dir, "另一台电脑.mindmap.json"), []byte(`{"sheets":[{"id":"s","title":"另一台电脑","root":{"id":"r","title":"另一台电脑"}}]}`), 0o644)
			return ghsync.Result{Pulled: 1, Written: []string{"另一台电脑.mindmap.json"}}, nil
		}
		return ghsync.Result{Pushed: 1}, nil
	}

	a.openSyncSettings()
	tt.Frame()
	snapshot(t, tt, "settings-sync")
	if err := tt.Click("同步仓库"); err != nil {
		t.Fatal(err)
	}
	tt.Type("https://github.com/me/notes.git")
	if err := tt.Click("同步令牌"); err != nil {
		t.Fatal(err)
	}
	tt.Type("github_pat_x")
	if err := tt.Click("检查连接"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.cfg.SyncRepo != "me/notes" || a.cfg.SyncToken != "github_pat_x" || !tt.HasText("✓ 已连接私有仓库 me/notes") {
		t.Fatalf("repo %q token %q: %q", a.cfg.SyncRepo, a.cfg.SyncToken, tt.Texts())
	}
	if err := tt.Click("立即同步"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(runs) != 1 || runs[0].Repo != "me/notes" || runs[0].Token != "github_pat_x" {
		t.Fatalf("runs %+v", runs)
	}
	if !slicesHasName(a, "另一台电脑") {
		t.Fatal("the map that came down is not in the sidebar")
	}
	a.settingsOpen = false
	a.showProjects()
	tt.Frame()
	if !tt.HasText("已同步 " + clock.Format("15:04")) {
		t.Fatalf("no status: %q", tt.Texts())
	}

	// A change here goes up with the next round, by itself: not at once,
	// each sync being calls of the account's hour.
	clock = clock.Add(time.Second)
	a.syncChange()
	clock = clock.Add(time.Minute)
	a.syncDue()
	if len(runs) != 1 {
		t.Fatal("synced at once after a change")
	}
	clock = clock.Add(syncEvery)
	a.syncDue()
	if len(runs) != 2 {
		t.Fatalf("runs %d", len(runs))
	}
	// Turned off, it waits to be asked.
	a.cfg.SyncOff = true
	a.syncChange()
	clock = clock.Add(time.Hour)
	a.syncDue()
	if len(runs) != 2 {
		t.Fatal("synced while off")
	}
}

func slicesHasName(a *app, name string) bool {
	for _, f := range a.files {
		if strings.Contains(f.Path, name) {
			return true
		}
	}
	return false
}

// The settings choose how screenshots are taken, and show their log.
func TestShotSettings(t *testing.T) {
	a, tt := newTestApp(t)
	a.openSettings()
	tt.Frame()
	snapshot(t, tt, "settings-shot")
	// The screenshots' log is not shown (帮助 → 在访达中显示截图记录 has it).
	if tt.HasText("2026-10-11 15:04:05 截图开始") || tt.HasText("复制截图记录") {
		t.Fatal("the log is shown")
	}
	// Every tab is as high: the window does not jump.
	general, ok := tt.Find("设置内容")
	if !ok {
		t.Fatal("no settings")
	}
	for _, tab := range []string{"备份", "同步", "连接", "资料整理"} {
		if err := tt.Click(tab); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if r, _ := tt.Find("设置内容"); r.H != general.H || r.Y != general.Y {
			t.Fatalf("%s: %v, 通用 %v", tab, r, general)
		}
	}
}

func TestShotUnfinished(t *testing.T) {
	for log, want := range map[string]bool{
		"": false,
		"a 截图开始：…\nb 截图窗口已打开":               true,
		"a 截图开始：…\nb 截图结束":                  false,
		"a 截图开始\nb 没有屏幕录制权限":                false,
		"a 截图开始\nb 截图结束\nc 截图开始\nd 正在截取屏幕…": true,
		"a 截图开始\nb 上次截图没有完成":                false,
	} {
		if shotUnfinished(log) != want {
			t.Errorf("%q: %v", log, !want)
		}
	}
}

// A map changed on two computers is merged by the sync; where both
// changed one thing, a window asks which to keep, as git does.
func TestSyncMergeAsks(t *testing.T) {
	a, tt := newTestApp(t)
	a.cfg.SyncRepo, a.cfg.SyncToken = "me/notes", "github_pat_x"
	path := filepath.Join(a.dir, "AI.mindmap.json")
	base := model.NewFile("AI")
	base.Sheets[0].Root.ID = "r"
	n, _ := base.Sheets[0].AddChild("r", "模型")
	if err := base.Save(path); err != nil {
		t.Fatal(err)
	}
	ours, theirs := base.Clone(), base.Clone()
	ours.Node(n.ID).Title = "这台的名字"
	theirs.Node(n.ID).Title = "另一台的名字"
	added, _ := theirs.Sheets[0].AddChild("r", "另一台加的")
	if err := ours.Save(path); err != nil {
		t.Fatal(err)
	}
	b, _ := base.Marshal()
	o, _ := ours.Marshal()
	r, _ := theirs.Marshal()
	a.syncRun = func(_ context.Context, dir string, cfg ghsync.Config, _ func(string)) (ghsync.Result, error) {
		data, ok := cfg.Merge("AI.mindmap.json", b, o, r, ghsync.MergeInfo{
			BaseAt: testNow.Add(-26 * time.Hour), LocalAt: testNow.Add(-time.Hour), RemoteAt: testNow.Add(-2 * time.Hour), RemoteFrom: "Office-Mac"})
		if !ok {
			t.Fatal("not merged")
		}
		os.WriteFile(filepath.Join(dir, "AI.mindmap.json"), data, 0o644)
		return ghsync.Result{Pulled: 1, Written: []string{"AI.mindmap.json"}, Merged: []string{"AI.mindmap.json"}}, nil
	}
	a.syncNow()
	tt.Frame()
	tt.Frame()
	snapshot(t, tt, "sync-merge-ask")
	if !a.mergeOpen || !tt.HasText("合并「AI」") || !tt.HasText("「模型」的标题") || !tt.HasText("另一台的名字") ||
		!tt.HasText("原来：模型（上次两边一致，昨天 07:00 同步时）") || !tt.HasText("在 AI") ||
		!tt.HasText("另一台电脑 · Office-Mac") || !tt.HasText("导图最后改于 今天 08:00") || !tt.HasText("较新") {
		t.Fatalf("not asked: %q", tt.Texts())
	}
	if f, _ := model.Load(path); f.Node(added.ID) == nil {
		t.Fatal("what the other computer added is not merged")
	}
	if err := tt.Click("另一台电脑 · Office-Mac"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("完成"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	f, _ := model.Load(path)
	if a.mergeOpen || f.Node(n.ID).Title != "另一台的名字" || f.Node(added.ID) == nil {
		t.Fatalf("open %v, title %q", a.mergeOpen, f.Node(n.ID).Title)
	}
}

// A copy kept by a sync before is merged into its map, and asked about.
func TestConflictCopyMerged(t *testing.T) {
	a, tt := newTestApp(t)
	path := filepath.Join(a.dir, "AI.mindmap.json")
	cp := filepath.Join(a.dir, "AI (GitHub 冲突 2026-10-11 1011).mindmap.json")
	f := model.NewFile("AI")
	n, _ := f.Sheets[0].AddChild(f.Sheets[0].Root.ID, "两边都有")
	n.Note = "这台的备注"
	if err := f.Save(path); err != nil {
		t.Fatal(err)
	}
	g := f.Clone()
	added, _ := g.Sheets[0].AddChild(g.Sheets[0].Root.ID, "只在副本")
	g.Node(n.ID).Note = "副本的备注"
	if err := g.Save(cp); err != nil {
		t.Fatal(err)
	}
	a.refreshFiles()
	a.mergeConflictCopies()
	tt.Frame()
	if _, err := os.Stat(cp); err == nil {
		t.Fatal("the copy stayed")
	}
	m, _ := model.Load(path)
	if m.Node(added.ID) == nil {
		t.Fatal("the copy's topic is not merged")
	}
	if !a.mergeOpen || !tt.HasText("「两边都有」的备注") {
		t.Fatalf("not asked: %q", tt.Texts())
	}
	// Taken back: both files as they were.
	a.noticeUndo()
	if _, err := os.Stat(cp); err != nil || len(a.merges) != 0 {
		t.Fatal("not taken back")
	}
	if m, _ := model.Load(path); m.Node(added.ID) != nil {
		t.Fatal("the map not as it was")
	}
}
