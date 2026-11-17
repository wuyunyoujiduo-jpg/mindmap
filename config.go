package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/gitbase"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// config is what the app remembers between runs.
type config struct {
	Dir  string `json:"dir"`  // the folder of mind maps shown in the sidebar
	File string `json:"file"` // the map open last
	// AgentsSeen is when what the AI agents reported was last looked at
	// (agents.go): what came since is counted as new.
	AgentsSeen time.Time `json:"agentsSeen,omitzero"`
	// SkillsSeen are the AI agents found before (skillsview.go): one not
	// among them is new, and said so; SkillsScanned is that they were
	// looked for once, when every one found is taken as seen.
	SkillsSeen    []string `json:"skillsSeen,omitempty"`
	SkillsScanned bool     `json:"skillsScanned,omitempty"`
	// SkillsMarked is what the user said of a skill, by its key: true that
	// it came with its tool, false that it did not (skills.Env.Marked).
	SkillsMarked map[string]bool `json:"skillsMarked,omitempty"`
	// PanelOpen shows the panel of the topic chosen beside the canvas: put
	// away at first, which leaves the canvas the room.
	PanelOpen bool `json:"panelOpen,omitempty"`
	// Ground is the color of the window's ground, a key of grounds: ""
	// is white, as the page.
	Ground string `json:"ground,omitempty"`
	// Appearance is the look chosen whatever the system's: "light",
	// "dark", or "" for the system's own.
	Appearance string `json:"appearance,omitempty"`
	// HideTree puts away the maps' directory of 项目 (tree.go), which
	// shows unless it is set.
	HideTree bool `json:"hideTree,omitempty"`
	// Favorites are the maps starred, by their paths: first in their
	// folder and where maps are switched.
	Favorites []string `json:"favorites,omitempty"`
	// OpenFolders are the folders of the sidebar shown open, by their
	// paths; the others show closed.
	OpenFolders []string `json:"openFolders,omitempty"`
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
	// GitHubAPI is the address of GitHub's API the sync goes to, for
	// GitHub Enterprise ("" is github.com's: https://api.github.com; an
	// Enterprise server's is https://HOST/api/v3). It has no field in the
	// settings, and is not to have one: it is written here, in
	// config.json ("githubAPI"), by the few who need it, with the app
	// closed.
	GitHubAPI string `json:"githubAPI,omitempty"`
	// NoReminders turns off the notifications of tasks due.
	NoReminders bool `json:"noReminders,omitempty"`
	// NoHolidays leaves out China's holidays and the weekend days worked
	// for them (holidays.go); WeekFromSunday begins the calendars' weeks
	// on Sunday, not Monday.
	NoHolidays     bool `json:"noHolidays,omitempty"`
	WeekFromSunday bool `json:"weekFromSunday,omitempty"`
	// ClipKey keeps what is on the clipboard in the library, from any
	// app; "" is the default.
	ClipKey string `json:"clipKey,omitempty"`
	// ShotKey takes a screenshot, from any app.
	ShotKey string `json:"shotKey,omitempty"`
	// SyncRepo is the GitHub repository, owner/name, that the maps and
	// the library sync with, with the token that may write to it;
	// SyncOff stops the sync by itself. SyncBranch is the branch synced
	// with, "" for main: as GitHubAPI, it has no field in the settings and
	// is written in config.json ("syncBranch") with the app closed. Every
	// computer of the user's is to name the same one.
	SyncRepo   string `json:"syncRepo,omitempty"`
	SyncBranch string `json:"syncBranch,omitempty"`
	SyncToken  string `json:"syncToken,omitempty"`
	SyncOff    bool   `json:"syncOff,omitempty"`
	// SyncStateOut is set once the sync's state of before 0.105.2, kept in
	// the maps folder, was looked for and moved out of it (syncState): it
	// is looked for that once, in the folder the app had then. Found
	// later, in a backup put back or a folder a cloud brought, it is
	// another computer's or another time's, and is left alone.
	SyncStateOut bool `json:"syncStateOut,omitempty"`
	// Projects are the folders of the maps' code on this computer, by the
	// maps' paths (projectOf): a map's file names its
	// repository, the same on every computer, and not where this
	// computer keeps it, which another computer's choice overwrote.
	Projects map[string]string `json:"projects,omitempty"`
	// Icon is the app's icon chosen, "" for the default.
	Icon string `json:"icon,omitempty"`
	// LibraryManual leaves new clips for the AI to sort out when asked,
	// rather than at once.
	LibraryManual bool `json:"libraryManual,omitempty"`
	// MapAutoOff stops the AI analyzing maps by itself (mapanalysis.go).
	MapAutoOff bool `json:"mapAutoOff,omitempty"`
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
	// VaultLock is after how many minutes the secrets unlocked are locked
	// again (vault.go): 0 for five, less for never while the app runs.
	// VaultKeepClipboard leaves a secret copied on the clipboard, which is
	// else cleared of it a minute on.
	VaultLock          int  `json:"vaultLock,omitempty"`
	VaultKeepClipboard bool `json:"vaultKeepClipboard,omitempty"`
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

// The folder of a map's code is this computer's to know. A map's file,
// which the sync carries to every computer, held one computer's absolute
// path (File.Project): on the other it led nowhere, and chosen anew there
// it was the first computer's that led nowhere. So the folder chosen
// here is kept here (config.Projects), and the file names the
// repository (File.Repo), by which the agents find the map on any
// computer. File.Project is still read, for the files of before and the
// computer that wrote it, and still written where it is empty or this
// computer's own, for the agents of a repository with no remote, which
// find their map by it (internal/mcp's projectMap).

// projectOf is the folder of a map's code on this computer, "" for none:
// the one chosen here, else the one its file names when that is a folder
// of this computer.
func (a *app) projectOf(path string, f *model.File) string {
	if dir := a.cfg.Projects[path]; dir != "" {
		return dir
	}
	if f != nil && isDir(f.Project) {
		return f.Project
	}
	return ""
}

func isDir(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// setProject sets the folder of the open map's code on this computer,
// "" for none. Another computer's folder, in the file, is left as it is.
func (a *app) setProject(dir string) {
	if a.file == nil || a.path == "" {
		return
	}
	if dir == "" {
		delete(a.cfg.Projects, a.path)
	} else {
		if a.cfg.Projects == nil {
			a.cfg.Projects = map[string]string{}
		}
		a.cfg.Projects[a.path] = dir
	}
	a.cfg.save()
	repo := ""
	if dir != "" {
		repo = gitbase.Remote(dir)
	}
	// The file's own folder is this computer's to change when there is
	// none, or one that is a folder here; and with it the repository
	// named, which is else set only where none is.
	project, named := a.file.Project, a.file.Repo
	if project == "" || isDir(project) {
		project = dir
		if repo != "" {
			named = repo
		}
	} else if named == "" {
		named = repo
	}
	if project == a.file.Project && named == a.file.Repo {
		return
	}
	a.mutate("", func() { a.file.Project, a.file.Repo = project, named })
}

// defaultDir is where maps go until the user picks a folder.
func defaultDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, "Documents", "MyMind")
}
