// Package skills finds the skills of the AI coding agents on this
// computer (Claude Code, Codex, Grok Build and the others: a folder with
// a SKILL.md, the Agent Skills form), says what each is and where, and
// keeps them in one place: the skills the user takes in are one folder
// each in MyMind's store (.skills in the maps folder), and the agents'
// folders hold links to them, so that whichever is changed, it is the
// one copy. The store goes with the maps to the user's other computers
// (ghsync), and each computer makes its own links, its paths being its
// own; what each computer linked is written in the store
// (_machines/<name>.json), so that the other one offers the same.
//
// The agents are known by a table (tools.json), which the app may have
// newer from the network (Table), and an agent the table does not know
// is found by a folder of skills in a hidden folder of the home
// (~/.name/skills/*/SKILL.md), as most follow the form.
package skills

import (
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// A Tool is an AI coding agent: where it keeps its things and its skills,
// in the home and in a project's repository.
type Tool struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Short string `json:"short,omitempty"` // two letters for its mark
	// Home is its folder from the user's home (".claude"): there, it is
	// installed.
	Home string `json:"home"`
	// Global and Project are the folders of its skills, from the home and
	// from a repository's root; a skill is linked into the first.
	Global  []string `json:"global"`
	Project []string `json:"project"`
	// Plugins is the folder under which its plugins keep their skills,
	// from the home, if it has plugins.
	Plugins string `json:"plugins,omitempty"`
	// Usage names the record of its sessions that the skills used are
	// counted from ("claude": ~/.claude/projects).
	Usage string `json:"usage,omitempty"`
	// Checked is that its folders were tried; the others are as their
	// documentation says.
	Checked bool `json:"checked,omitempty"`
	// Found is that the table does not know it: it was found by its folder
	// of skills alone.
	Found bool `json:"-"`
}

//go:embed tools.json
var builtin []byte

// Builtin is the table the app was built with.
func Builtin() []Tool {
	var ts []Tool
	_ = json.Unmarshal(builtin, &ts)
	return ts
}

// Table reads a table, as fetched; nil if it is not one.
func Table(data []byte) []Tool {
	var ts []Tool
	if json.Unmarshal(data, &ts) != nil {
		return nil
	}
	ts = slices.DeleteFunc(ts, func(t Tool) bool { return t.ID == "" || t.Name == "" || t.Home == "" || len(t.Global) == 0 })
	if len(ts) == 0 {
		return nil
	}
	return ts
}

// Merge is the built table with what newer says of each tool and the
// tools it adds.
func Merge(base, newer []Tool) []Tool {
	out := slices.Clone(base)
	for _, n := range newer {
		if i := slices.IndexFunc(out, func(t Tool) bool { return t.ID == n.ID }); i >= 0 {
			out[i] = n
		} else {
			out = append(out, n)
		}
	}
	return out
}

// Mark is the two letters that stand for the tool.
func (t Tool) Mark() string {
	if t.Short != "" {
		return t.Short
	}
	r := []rune(strings.ToUpper(strings.TrimLeft(t.Name, ".")))
	if len(r) > 2 {
		r = r[:2]
	}
	return string(r)
}

// Detect is the tools of the table that are on this computer, by their
// folders in home, and those found by a folder of skills: in a hidden
// folder of the home or of ~/.config, as the table's are.
func Detect(home string, table []Tool) []Tool {
	var out []Tool
	known := map[string]bool{}
	for _, t := range table {
		known[filepath.Clean(t.Home)] = true
		for _, g := range t.Global {
			known[filepath.Clean(filepath.Dir(g))] = true
		}
		if isDir(filepath.Join(home, t.Home)) {
			out = append(out, t)
		}
	}
	for _, parent := range []string{"", ".config"} {
		entries, err := os.ReadDir(filepath.Join(home, parent))
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if parent == "" && (!strings.HasPrefix(name, ".") || name == ".config") {
				continue
			}
			rel := filepath.Join(parent, name)
			if known[rel] || strings.HasPrefix(name, ".mymind") || !isDir(filepath.Join(home, rel)) {
				continue
			}
			skills := filepath.Join(rel, "skills")
			if !hasSkill(filepath.Join(home, skills)) {
				continue
			}
			id := strings.TrimPrefix(name, ".")
			out = append(out, Tool{ID: id, Name: id, Home: rel, Global: []string{skills},
				Project: []string{filepath.Join("."+id, "skills")}, Found: true})
		}
	}
	return out
}

// hasSkill tells whether dir holds a skill: a folder with a SKILL.md.
func hasSkill(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if fileExists(filepath.Join(dir, e.Name(), "SKILL.md")) {
			return true
		}
	}
	return false
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}
