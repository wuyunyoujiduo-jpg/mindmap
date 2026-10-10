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
	// Favorites are the maps shown first in the sidebar, in the order they
	// were starred.
	Favorites []string `json:"favorites,omitempty"`
	// The AI assistant: an Anthropic API key, the model, and the API's
	// address when it goes through a proxy.
	AIKey     string `json:"aiKey,omitempty"`
	AIModel   string `json:"aiModel,omitempty"`
	AIBaseURL string `json:"aiBaseURL,omitempty"`
	// AIProvider is "" for Claude or "deepseek", whose key, model and
	// address are these.
	AIProvider string `json:"aiProvider,omitempty"`
	DSKey      string `json:"deepseekKey,omitempty"`
	DSModel    string `json:"deepseekModel,omitempty"`
	DSBaseURL  string `json:"deepseekBaseURL,omitempty"`
	// Tags is the library of tags shared by all maps, with their colors.
	Tags []tagDef `json:"tags,omitempty"`
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
	// Only the user may read it: it holds the API key.
	_ = os.WriteFile(p, b, 0o600)
	_ = os.Chmod(p, 0o600)
}

// defaultDir is where maps go until the user picks a folder.
func defaultDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, "Documents", "MindMap")
}
