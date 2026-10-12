package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/versions"
)

// The maps folder of before, ~/Documents/MindMap, becomes ~/Documents/MyMind,
// its maps' versions and the map open last with it.
func TestMoveOldDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	old := filepath.Join(home, "Documents", "MindMap")
	m := filepath.Join(old, "项目", "计划"+model.Ext)
	os.MkdirAll(filepath.Dir(m), 0o755)
	f := model.NewFile("计划")
	if err := f.Save(m); err != nil {
		t.Fatal(err)
	}
	store := versions.Store{Dir: versionsDir()}
	if _, err := store.Save(m, f, versions.Auto, "", time.Now()); err != nil {
		t.Fatal(err)
	}

	c := loadConfig()
	if c.Dir != old {
		t.Fatalf("the folder of before is not used: %s", c.Dir)
	}
	c.File, c.Favorites = m, []string{m}
	if !moveOldDir(&c) {
		t.Fatal("not moved")
	}
	now := filepath.Join(home, "Documents", "MyMind", "项目", "计划"+model.Ext)
	if _, err := os.Stat(now); err != nil {
		t.Fatal(err)
	}
	if c.Dir != filepath.Join(home, "Documents", "MyMind") || c.File != now || c.Favorites[0] != now {
		t.Fatalf("config %+v", c)
	}
	if len(store.List(now)) != 1 {
		t.Fatal("the versions stayed behind")
	}
	if got := loadConfig(); got.Dir != c.Dir {
		t.Fatalf("not saved: %s", got.Dir)
	}
	// Once only.
	if moveOldDir(&c) {
		t.Fatal("moved twice")
	}
}

// The maps folder renamed outside the app, as in Finder: the app finds it
// under its new name and goes on there, with the map that was open.
func TestFollowRenamedDir(t *testing.T) {
	a, tt := newTestApp(t)
	open := a.path
	from := a.dir
	to := filepath.Join(filepath.Dir(from), "我的项目")
	if err := os.Rename(from, to); err != nil {
		t.Fatal(err)
	}
	if !a.followDir() {
		t.Fatal("not followed")
	}
	rel, _ := filepath.Rel(from, open)
	if a.dir != to || a.cfg.Dir != to || a.path != filepath.Join(to, rel) || a.doc == nil {
		t.Fatalf("dir %q, map %q", a.dir, a.path)
	}
	if _, err := os.Stat(from); err == nil {
		t.Fatal("the old folder came back")
	}
	tt.Frame()
	if !tt.HasText("我的项目") {
		t.Fatalf("the new name is not shown: %q", tt.Texts())
	}
	if a.followDir() {
		t.Fatal("followed again")
	}
}

// A map's name follows its central topic, but for a name the user gave.
func TestMapNameFollowsTopic(t *testing.T) {
	a, _ := newTestApp(t)
	// The topic typed as a user types it, in the title's editor.
	retitle := func(title string) {
		a.startEdit(a.doc.Root.ID)
		a.editText = title
		a.commitEdit()
		a.save()
	}
	a.newMap()
	if mapName(a.path) != "新导图" {
		t.Fatalf("new map %q", mapName(a.path))
	}
	retitle("发布计划")
	if mapName(a.path) != "发布计划" {
		t.Fatalf("named %q", mapName(a.path))
	}
	// Taken: numbered, and still following.
	a.newMap()
	retitle("发布计划")
	if mapName(a.path) != "发布计划 2" {
		t.Fatalf("named %q", mapName(a.path))
	}
	retitle("发布计划：二期")
	if mapName(a.path) != "发布计划：二期" {
		t.Fatalf("named %q", mapName(a.path))
	}
	// Renamed by hand: the name stays.
	a.rename(a.path, "我的名字")
	retitle("别的主题")
	if mapName(a.path) != "我的名字" {
		t.Fatalf("a name given by hand changed: %q", mapName(a.path))
	}
}

func TestNameFor(t *testing.T) {
	for in, want := range map[string]string{
		"计划 / 第二版":    "计划 第二版",
		"a:b*c?\n第二行": "a b c",
		"  .隐藏. ":     "隐藏",
	} {
		if got := nameFor(in); got != want {
			t.Errorf("nameFor(%q) = %q", in, got)
		}
	}
}
