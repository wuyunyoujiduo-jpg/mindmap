package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/mcp"
)

// The folder of a map's code an agent gives the MCP server (another
// process, writing the settings) while the app runs is kept through the
// app's next save; and a change of the app's own to the same map, made
// after, is the one kept.
func TestProjectsOfTheServerKept(t *testing.T) {
	a, _ := newTestApp(t)
	mine, theirs, later := t.TempDir(), t.TempDir(), t.TempDir()
	m1, m2 := filepath.Join(a.dir, "一.mindmap.json"), filepath.Join(a.dir, "二.mindmap.json")
	a.cfg.Projects = map[string]string{m1: mine}
	a.cfg.save()

	// The server, as another process: what it reads is the file.
	mcp.KeepProjectHere(m2, theirs)
	if loadConfig().Projects[m2] != theirs {
		t.Fatal("the server did not write it")
	}
	// The app saves something else of its settings: both are there.
	a.cfg.TourDone = true
	a.cfg.save()
	got := loadConfig().Projects
	if got[m1] != mine || got[m2] != theirs {
		t.Fatalf("lost at the app's save: %v", got)
	}
	if a.projectOf(m2, nil) != theirs {
		t.Fatalf("the app does not know it after: %v", a.cfg.Projects)
	}

	// The server sets the app's map; the app then sets it otherwise: the
	// app's, the later, is kept.
	mcp.KeepProjectHere(m1, theirs)
	a.cfg.save()
	if got := loadConfig().Projects[m1]; got != theirs {
		t.Fatalf("the server's later change was written over: %q", got)
	}
	a.cfg.Projects[m1] = later
	a.cfg.save()
	if got := loadConfig().Projects[m1]; got != later {
		t.Fatalf("the app's later change was not kept: %q", got)
	}
	// Taken off by the app: gone, the server's other kept.
	delete(a.cfg.Projects, m1)
	a.cfg.save()
	if got := loadConfig().Projects; got[m1] != "" || got[m2] != theirs {
		t.Fatalf("%v", got)
	}

	// Settings that cannot be read are not taken as having no folders.
	os.WriteFile(configPath(), []byte("{not json"), 0o600)
	a.cfg.save()
	if got := loadConfig().Projects; got[m2] != theirs {
		t.Fatalf("lost to a file that did not read: %v", got)
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(configPath()), "*.tmp")); len(left) > 0 {
		t.Fatalf("a temporary file left: %v", left)
	}
}
