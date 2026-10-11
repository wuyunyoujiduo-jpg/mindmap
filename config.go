package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
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
	// Recent are the maps opened last, the last first.
	Recent []string `json:"recent,omitempty"`
	// Editor opens the project's files linked from nodes: "vscode" (or
	// ""), "cursor", "default" or "finder".
	Editor string `json:"editor,omitempty"`
	// CaptureKey opens the capture window from any app; "" is the default.
	CaptureKey string `json:"captureKey,omitempty"`
	// NoTray hides the menu bar icon.
	NoTray bool `json:"noTray,omitempty"`
	// GitHubToken reads the issues of private repositories; GitHubAPI is
	// the API's address, for GitHub Enterprise ("" is github.com's).
	GitHubToken string `json:"githubToken,omitempty"`
	GitHubAPI   string `json:"githubAPI,omitempty"`
	// NoReminders turns off the notifications of tasks due.
	NoReminders bool `json:"noReminders,omitempty"`
	// ClipKey keeps what is on the clipboard in the library, from any
	// app; "" is the default.
	ClipKey string `json:"clipKey,omitempty"`
	// ShotKey takes a screenshot, from any app.
	ShotKey string `json:"shotKey,omitempty"`
	// SyncRepo is the GitHub repository, owner/name, that the maps and
	// the library sync with, with the token that may write to it;
	// SyncOff stops the sync by itself.
	SyncRepo   string `json:"syncRepo,omitempty"`
	SyncBranch string `json:"syncBranch,omitempty"`
	SyncToken  string `json:"syncToken,omitempty"`
	SyncOff    bool   `json:"syncOff,omitempty"`
	// ShotSystem takes screenshots with macOS's own tool rather than the
	// app's, which marks them up.
	ShotSystem bool `json:"shotSystem,omitempty"`
	// Icon is the app's icon chosen, "" for the default.
	Icon string `json:"icon,omitempty"`
	// LibraryManual leaves new clips for the AI to sort out when asked,
	// rather than at once.
	LibraryManual bool `json:"libraryManual,omitempty"`
	// The embedding model of the library's semantic search: an
	// OpenAI-compatible API's address, its key and the model.
	EmbedURL   string `json:"embedURL,omitempty"`
	EmbedKey   string `json:"embedKey,omitempty"`
	EmbedModel string `json:"embedModel,omitempty"`
	// NoEmbed turns semantic search off, the built-in model's included.
	NoEmbed bool `json:"noEmbed,omitempty"`
	// BackupDir is where the maps and the library are backed up each day,
	// "" for no backups; BackupLast is when they last were.
	BackupDir  string    `json:"backupDir,omitempty"`
	BackupLast time.Time `json:"backupLast,omitzero"`
	// PluginDeclined hides the library's offer of the semantic search
	// plugin.
	PluginDeclined bool `json:"pluginDeclined,omitempty"`
	// CardStyle is the style of the clips' cards last chosen.
	CardStyle int `json:"cardStyle,omitempty"`
	// TourDone is set once the tour of the first run was seen.
	TourDone bool `json:"tourDone,omitempty"`
	// PanelWidth is the width the right panel was dragged to.
	PanelWidth float32 `json:"panelWidth,omitempty"`
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
	// Only the user may read it: it holds the API key. Written whole or
	// not at all, through a file renamed over it.
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return
	}
	_ = os.Chmod(tmp, 0o600)
	_ = os.Rename(tmp, p)
}

// defaultDir is where maps go until the user picks a folder.
func defaultDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, "Documents", "MindMap")
}
