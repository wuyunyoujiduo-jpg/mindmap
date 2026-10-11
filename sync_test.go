package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/ghsync"
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
	tt.Frame()
	if !tt.HasText("已同步 " + clock.Format("15:04")) {
		t.Fatalf("no status: %q", tt.Texts())
	}

	// A change here goes up a little later, by itself.
	clock = clock.Add(time.Second)
	a.syncChange()
	clock = clock.Add(5 * time.Second)
	a.syncDue()
	if len(runs) != 1 {
		t.Fatal("synced too soon after a change")
	}
	clock = clock.Add(syncAfterChange)
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
	a.shotModes = true
	a.openSettings()
	tt.Frame()
	snapshot(t, tt, "settings-shot")
	if err := tt.Click("系统截图"); err != nil {
		t.Fatal(err)
	}
	if !a.cfg.ShotSystem {
		t.Fatal("not the system's tool")
	}
	if !tt.HasText("2026-10-11 15:04:05 截图开始") {
		t.Fatalf("no log: %q", tt.Texts())
	}
	if err := tt.Click("复制截图记录"); err != nil {
		t.Fatal(err)
	}
	if a.plat.ClipboardText() != "2026-10-11 15:04:05 截图开始" {
		t.Fatalf("copied %q", a.plat.ClipboardText())
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
