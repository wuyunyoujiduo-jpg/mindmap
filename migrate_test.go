package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/versions"
)

// What the app keeps by the maps' paths follows the maps folder where it
// moved: the versions, how the maps were left, the map open last, the
// favorites, the recent ones, the folders shown open, the folders of the
// maps' code, and what the sync remembers of the folder.
func TestFollowPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	from, to := filepath.Join(home, "Documents", "旧"), filepath.Join(home, "Documents", "新")
	m := filepath.Join(from, "项目", "计划"+model.Ext)
	os.MkdirAll(filepath.Dir(m), 0o755)
	f := model.NewFile("计划")
	if err := f.Save(m); err != nil {
		t.Fatal(err)
	}
	store := versions.Store{Dir: versionsDir()}
	if _, err := store.Save(m, f, versions.Auto, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Dir(syncStatePath(from)), 0o755)
	os.WriteFile(syncStatePath(from), []byte(`{"repo":"me/notes"}`), 0o644)
	views, _ := json.Marshal(map[string]*fileView{m: {Sheet: "s1"}})
	os.WriteFile(viewsPath(), views, 0o644)

	c := config{Dir: from, File: m, Favorites: []string{m}, Recent: []string{m},
		OpenFolders: []string{filepath.Dir(m)}, Projects: map[string]string{m: "/code/x"}}
	if err := os.Rename(from, to); err != nil {
		t.Fatal(err)
	}
	followPaths(&c, from, to)
	now := filepath.Join(to, "项目", "计划"+model.Ext)
	if c.Dir != to || c.File != now || c.Favorites[0] != now || c.Recent[0] != now {
		t.Fatalf("config %+v", c)
	}
	if c.OpenFolders[0] != filepath.Dir(now) || c.Projects[now] != "/code/x" || len(c.Projects) != 1 {
		t.Fatalf("the open folders and the code's folders stayed behind: %v %v", c.OpenFolders, c.Projects)
	}
	if len(store.List(now)) != 1 {
		t.Fatal("the versions stayed behind")
	}
	if got := loadConfig(); got.Dir != c.Dir {
		t.Fatalf("not saved: %s", got.Dir)
	}
	if data, _ := os.ReadFile(syncStatePath(to)); string(data) != `{"repo":"me/notes"}` {
		t.Fatalf("what the sync remembers stayed behind: %q", data)
	}
	if v := loadViews()[now]; v == nil || v.Sheet != "s1" {
		t.Fatalf("the views stayed behind: %v", loadViews())
	}
	if _, err := os.Stat(viewsPath() + ".merge.tmp"); err == nil {
		t.Fatal("the views' file was not written through another")
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
