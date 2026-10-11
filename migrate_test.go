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
