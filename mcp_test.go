package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The hooks go into Claude Code's settings beside what is there, once,
// and follow the app when it moves.
func TestInstallHooks(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude", "settings.json")
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte(`{"model":"opus","hooks":{"Stop":[{"hooks":[{"type":"command","command":"say done"}]}]}}`), 0o644)
	exe := "/Applications/MyMind.app/Contents/MacOS/MyMind"
	if changed, err := installHooks(path, exe); err != nil || !changed {
		t.Fatal(changed, err)
	}
	if changed, _ := installHooks(path, exe); changed {
		t.Fatal("installed twice")
	}
	read := func() map[string]any {
		var s map[string]any
		data, _ := os.ReadFile(path)
		if err := json.Unmarshal(data, &s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	s := read()
	hooks := s["hooks"].(map[string]any)
	if s["model"] != "opus" || len(hooks["Stop"].([]any)) != 2 || len(hooks["SessionStart"].([]any)) != 1 {
		t.Fatalf("settings %v", s)
	}
	// Moved: the old commands give way to the new.
	if changed, _ := installHooks(path, "/Users/me/Apps/MyMind.app/Contents/MacOS/MyMind"); !changed {
		t.Fatal("not moved")
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "/Applications/") || strings.Count(string(data), "hook stop") != 1 || !strings.Contains(string(data), "say done") {
		t.Fatalf("after moving: %s", data)
	}
	// A file that cannot be read is left alone.
	os.WriteFile(path, []byte(`{not json`), 0o644)
	if _, err := installHooks(path, exe); err == nil {
		t.Fatal("wrote over a file it could not read")
	}
}
