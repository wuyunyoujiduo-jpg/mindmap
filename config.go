package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// config is what the app remembers between runs.
type config struct {
	Dir  string `json:"dir"`  // the folder of mind maps shown in the sidebar
	File string `json:"file"` // the map open last
}

func configPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "MindMap", "config.json")
}

func loadConfig() config {
	var c config
	if p := configPath(); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			_ = json.Unmarshal(b, &c)
		}
	}
	if c.Dir == "" {
		c.Dir = defaultDir()
	}
	return c
}

func (c config) save() {
	p := configPath()
	if p == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	_ = os.WriteFile(p, b, 0o644)
}

// defaultDir is where maps go until the user picks a folder.
func defaultDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, "Documents", "MindMap")
}
