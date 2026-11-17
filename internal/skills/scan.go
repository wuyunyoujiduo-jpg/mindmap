package skills

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
)

// Scope is where a skill is loaded: every session's, a plugin's; or a
// tool's own, which came with it (Bundled), loaded in every session too.
// A project's skills, in its repository, are not looked at: the user
// leaves them to the repository (0.100.7).
type Scope int

const (
	Global Scope = iota
	Plugin
	Bundled
)

// ReadOnly tells whether a skill is another's to keep: a plugin's or a
// tool's own, installed and updated by them. MyMind shows it, and takes
// it in, changes it or says what is wrong with it never: a change would
// be put back at the next update, and a link put in its place undone.
func (e *Entry) ReadOnly() bool { return e.Scope == Plugin || e.Scope == Bundled }

// Env is what a scan looks at.
type Env struct {
	Home  string // the user's home
	Store string // MyMind's folder of skills: ~/.agents/skills, the tools' common place
	// Keep is MyMind's own beside it, which no tool reads: what is put
	// away (.trash) and what is turned off (disabled). Empty, the store.
	Keep    string
	Machine string // this computer's name
	Table   []Tool
	// Since is how far back the skills used are counted; Used, if not
	// nil, is the count of a scan before, taken again rather than read
	// (the records of the sessions are large).
	Since time.Time
	Used  map[string]int
	// Marked is what the user said of a skill by its key: true that it
	// came with a tool, false that it did not, whatever its place says.
	Marked map[string]bool
}

// CopyState is how a tool's folder holds a skill.
type CopyState int

const (
	Linked    CopyState = iota // a link to MyMind's copy
	Own                        // a folder of its own, not in MyMind
	Same                       // a folder of its own while MyMind has the skill, alike
	Diverged                   // a folder of its own while MyMind has the skill, not alike
	Elsewhere                  // a link to a folder that is not MyMind's (another tool's)
	Dangling                   // a link to what is not there
	Native                     // the tool reads MyMind's folder itself (~/.agents/skills): no link
)

// Reads tells whether a tool reads MyMind's folder of a scope itself,
// needing no link there: one of its folders of skills is it.
func (env Env) Reads(t Tool, scope Scope) bool {
	if scope != Global {
		return false
	}
	for _, g := range t.Global {
		if env.Store != "" && filepath.Clean(filepath.Join(env.Home, g)) == filepath.Clean(env.Store) {
			return true
		}
	}
	return false
}

// ReadsID is Reads of the tool of the table by its ID.
func (env Env) ReadsID(id string, scope Scope) bool {
	for _, t := range env.Table {
		if t.ID == id {
			return env.Reads(t, scope)
		}
	}
	return false
}

// A Copy is a skill as a tool's folder has it.
type Copy struct {
	Tool   string
	Path   string // in the tool's folder
	Target string // what the link points to, if it is one
	State  CopyState
}

// An Entry is a skill, wherever it is: in MyMind, in the tools' folders.
type Entry struct {
	Key    string // "g/name", "b/tool/name", "x/tool/plugin/name"
	Name   string // its folder's name
	Scope  Scope
	Plugin string // a plugin's
	// By is the tool a bundled skill came with; Marked that the user said
	// so, or that it is not (Env.Marked), and not the table or its link.
	By     string
	Marked bool
	// Stored is that MyMind has it, at StorePath; Disabled that it is
	// turned off, kept apart where no tool reads it (Env.Keep).
	Stored    bool
	StorePath string
	Disabled  bool
	Copies    map[string]*Copy // by tool
	// What its SKILL.md says, of MyMind's copy if there is one.
	Title, Description, Body string
	Raw                      string // its SKILL.md, as written (the first 64 KB)
	// Shown is the head of Raw and About the description as a page shows
	// them, what looks like a secret in dots (library.Mask): found once,
	// as the skill is read, for the patterns of a secret are slow on a
	// long text and a page is drawn at every frame.
	Shown, About   string
	Dir            string // the folder read
	Files          []string
	Script, Secret bool
	Issues         []string
	Used           int
	// Others is what the user's other computers linked it to, by their
	// names, when this one has it in no tool.
	Others map[string][]string
	Mod    time.Time
}

// Report is what a scan found.
type Report struct {
	Tools   []Tool
	Entries []*Entry
	Used    map[string]int // how often each was used, by its name
	At      time.Time
}

// Tool is the tool of id, if found.
func (r *Report) Tool(id string) (Tool, bool) {
	for _, t := range r.Tools {
		if t.ID == id {
			return t, true
		}
	}
	return Tool{}, false
}

// Entry is the entry of key.
func (r *Report) Entry(key string) *Entry {
	for _, e := range r.Entries {
		if e.Key == key {
			return e
		}
	}
	return nil
}

// Load is how many characters the tool reads first in each session: the
// names and descriptions of the skills it would use there, every
// session's and its plugins'.
func (r *Report) Load(tool string) int {
	n := 0
	for _, e := range r.Entries {
		if e.Copies[tool] == nil || e.Copies[tool].State == Dangling {
			continue
		}
		n += utf8.RuneCountInString(e.Name) + utf8.RuneCountInString(e.Description)
	}
	return n
}

// Scan finds every skill of env.
func Scan(env Env) *Report {
	r := &Report{Tools: Detect(env.Home, env.Table), At: time.Now()}
	byKey := map[string]*Entry{}
	entry := func(key, name string, scope Scope) *Entry {
		e := byKey[key]
		if e == nil {
			e = &Entry{Key: key, Name: name, Scope: scope, Copies: map[string]*Copy{}}
			byKey[key] = e
			r.Entries = append(r.Entries, e)
		}
		return e
	}
	// MyMind's.
	for _, d := range skillDirs(env.Store) {
		name := filepath.Base(d)
		e := entry("g/"+name, name, Global)
		e.Stored, e.StorePath = true, d
	}
	// What is turned off, unless a skill of its name came back meanwhile.
	if env.Keep != "" {
		off := filepath.Join(env.Keep, disabledDir)
		for _, d := range skillDirs(off) {
			name := filepath.Base(d)
			if byKey["g/"+name] == nil {
				e := entry("g/"+name, name, Global)
				e.Stored, e.StorePath, e.Disabled = true, d, true
			}
		}
	}
	// The tools'.
	for _, t := range r.Tools {
		for _, g := range t.Global {
			for _, d := range skillDirs(filepath.Join(env.Home, g)) {
				name := filepath.Base(d)
				addCopy(entry("g/"+name, name, Global), t.ID, d)
			}
		}
		for _, b := range t.Bundled {
			for _, d := range skillDirs(filepath.Join(env.Home, b)) {
				name := filepath.Base(d)
				e := entry("b/"+t.ID+"/"+name, name, Bundled)
				e.By = t.ID
				addCopy(e, t.ID, d)
			}
		}
		if t.Plugins != "" {
			for _, ps := range pluginSkills(filepath.Join(env.Home, t.Plugins)) {
				e := entry("x/"+t.ID+"/"+ps.plugin+"/"+ps.name, ps.name, Plugin)
				e.Plugin = ps.plugin
				e.Copies[t.ID] = &Copy{Tool: t.ID, Path: ps.dir, State: Own}
			}
		}
	}
	used := env.Used
	if used == nil {
		used = countUsed(env, r.Tools)
	}
	r.Used = used
	others := readMachines(env.Store, env.Machine)
	for _, e := range r.Entries {
		// What a copy is to MyMind's.
		var sum string
		if e.Stored {
			sum = treeSum(e.StorePath)
		}
		for _, c := range e.Copies {
			switch {
			// By where it leads, not by how it is written: a tool's folder
			// that is a link to the store whole is the store, and settled
			// as a copy alike it was put away from under itself.
			case c.Target == "" && e.StorePath != "" && samePath(c.Path, e.StorePath):
				c.State = Native
			case c.Target != "" && !exists(c.Target):
				c.State = Dangling
			case c.Target != "" && e.Stored && samePath(c.Target, e.StorePath):
				c.State = Linked
			case c.Target != "":
				c.State = Elsewhere
			case e.Stored && treeSum(c.Path) == sum:
				c.State = Same
			case e.Stored:
				c.State = Diverged
			}
		}
		dir := e.StorePath
		if dir == "" {
			for _, t := range r.Tools { // in the table's order: Claude Code's first
				if c := e.Copies[t.ID]; c != nil && c.State != Dangling {
					dir = c.Path
					break
				}
			}
		}
		// A tool's own, by where its folder is (a link into the tool's
		// install), or as the user said.
		if by := bundledBy(env, r.Tools, e); by != "" {
			e.Scope, e.By = Bundled, by
		}
		if m, ok := env.Marked[e.Key]; ok && !e.Stored && e.Scope != Plugin {
			e.Marked = true
			if m && e.Scope != Bundled {
				e.Scope, e.By = Bundled, firstCopy(r.Tools, e)
			} else if !m && e.Scope == Bundled && !strings.HasPrefix(e.Key, "b/") {
				e.Scope, e.By = Global, ""
			}
		}
		if dir != "" {
			e.read(dir)
		}
		e.Used = used[e.Name]
		if e.Stored && !e.Disabled && len(e.Copies) == 0 {
			e.Others = others[e.Key]
		}
	}
	sort.SliceStable(r.Entries, func(i, j int) bool {
		a, b := r.Entries[i], r.Entries[j]
		if a.Scope != b.Scope {
			return a.Scope < b.Scope
		}
		return a.Key < b.Key
	})
	return r
}

// Stamp is a word that changes when the folders of skills did: which
// tools there are, and of each folder a scan lists (MyMind's, what is
// turned off, the tools' and their own, their plugins' at its head) its
// time, and of each folder or link in it its time and its SKILL.md's. A
// scan reads every file of every skill, to tell a copy from another, and
// walks the plugins' folders; this reads no file, and tells, while the
// page is not looked at, whether a scan is due.
func Stamp(env Env) string {
	h := sha256.New()
	dir := func(p string) {
		fi, err := os.Stat(p)
		if err != nil {
			return
		}
		fmt.Fprintf(h, "%s %d\n", p, fi.ModTime().UnixNano())
		entries, _ := os.ReadDir(p)
		for _, d := range entries {
			name := d.Name()
			if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
				continue // not a skill's: what MyMind writes there itself (_machines), a tool's own
			}
			at := filepath.Join(p, name)
			if li, err := os.Lstat(at); err == nil {
				fmt.Fprintf(h, " %s %d %o", name, li.ModTime().UnixNano(), li.Mode().Type())
			}
			if t, err := os.Readlink(at); err == nil {
				fmt.Fprintf(h, " -> %s", t)
			}
			if fi, err := os.Stat(filepath.Join(at, "SKILL.md")); err == nil {
				fmt.Fprintf(h, " %d %d", fi.ModTime().UnixNano(), fi.Size())
			}
			h.Write([]byte{'\n'})
		}
	}
	dir(env.Store)
	if env.Keep != "" {
		dir(filepath.Join(env.Keep, disabledDir))
	}
	for _, t := range Detect(env.Home, env.Table) {
		fmt.Fprintf(h, "tool %s\n", t.ID)
		for _, g := range t.Global {
			dir(filepath.Join(env.Home, g))
		}
		for _, b := range t.Bundled {
			dir(filepath.Join(env.Home, b))
		}
		if t.Plugins != "" {
			dir(filepath.Join(env.Home, t.Plugins))
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// bundledBy is the tool a skill of every session came with, by where it
// is: each of its folders a link into where a program is installed (a
// package's node_modules, an app, Homebrew's), or into the tool's own
// folder outside its folders of skills. "" for none.
func bundledBy(env Env, tools []Tool, e *Entry) string {
	if e.Scope != Global || e.Stored || len(e.Copies) == 0 {
		return ""
	}
	by := ""
	for _, t := range tools {
		c := e.Copies[t.ID]
		if c == nil {
			continue
		}
		if c.Target == "" || !installed(env.Home, t, c.Target) {
			return ""
		}
		if by == "" {
			by = t.ID
		}
	}
	return by
}

// installed tells whether a link's target is where a program keeps what
// it installs, and not a folder of the user's.
func installed(home string, t Tool, target string) bool {
	p := filepath.ToSlash(target)
	for _, part := range []string{"/node_modules/", "/Cellar/", "/.app/", ".app/Contents/", "/Program Files", "/AppData/Local/Programs/", "/site-packages/"} {
		if strings.Contains(p, part) {
			return true
		}
	}
	for _, root := range []string{"/Applications/", "/opt/", "/usr/", "/Library/"} {
		if strings.HasPrefix(p, root) {
			return true
		}
	}
	// In the tool's own folder, and not in a folder of its skills.
	own := filepath.Join(home, t.Home)
	if rel, err := filepath.Rel(own, target); err != nil || strings.HasPrefix(rel, "..") {
		return false
	}
	for _, g := range t.Global {
		if rel, err := filepath.Rel(filepath.Join(home, g), target); err == nil && !strings.HasPrefix(rel, "..") {
			return false
		}
	}
	return true
}

// firstCopy is the first tool of the table's that has the skill.
func firstCopy(tools []Tool, e *Entry) string {
	for _, t := range tools {
		if e.Copies[t.ID] != nil {
			return t.ID
		}
	}
	return ""
}

// disabledDir is the folder of Env.Keep where what is turned off waits.
const disabledDir = "disabled"

// machinesDir is where each computer says what it linked.
const machinesDir = "_machines"

// addCopy adds a tool's copy at path, a link or a folder.
func addCopy(e *Entry, tool, path string) {
	if e.Copies[tool] != nil {
		return // its first folder's
	}
	c := &Copy{Tool: tool, Path: path, State: Own}
	if t, ok := linkTarget(path); ok {
		c.Target = t
	}
	e.Copies[tool] = c
}

// skillDirs are the folders of dir that hold a SKILL.md, or links to
// what held one.
func skillDirs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, d := range entries {
		name := d.Name()
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			continue
		}
		p := filepath.Join(dir, name)
		if fileExists(filepath.Join(p, "SKILL.md")) {
			out = append(out, p)
			continue
		}
		if _, ok := linkTarget(p); ok { // a link whose folder went
			if !exists(p) {
				out = append(out, p)
			}
		}
	}
	return out
}

type pluginSkill struct{ plugin, name, dir string }

// pluginSkills are the skills of the plugins under dir: in a folder
// named skills, the plugin's name the folder above it or, when that is
// its version, the one above that. Of a plugin's versions, the newest.
func pluginSkills(dir string) []pluginSkill {
	var out []pluginSkill
	mod := map[string]time.Time{}
	idx := map[string]int{}
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		if strings.Count(rel, string(filepath.Separator)) > 6 || strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules" {
			return filepath.SkipDir
		}
		if d.Name() != "skills" || p == dir {
			return nil
		}
		parts := strings.Split(rel, string(filepath.Separator))
		plugin := "plugin"
		if len(parts) >= 2 {
			plugin = parts[len(parts)-2]
			if looksVersion(plugin) && len(parts) >= 3 {
				plugin = parts[len(parts)-3]
			}
		}
		for _, s := range skillDirs(p) {
			name := filepath.Base(s)
			key := plugin + "/" + name
			fi, err := os.Stat(s)
			if err != nil {
				continue
			}
			if i, ok := idx[key]; ok {
				if fi.ModTime().After(mod[key]) {
					out[i].dir, mod[key] = s, fi.ModTime()
				}
				continue
			}
			idx[key], mod[key] = len(out), fi.ModTime()
			out = append(out, pluginSkill{plugin, name, s})
		}
		return filepath.SkipDir
	})
	return out
}

var versionLike = regexp.MustCompile(`^v?[0-9][0-9a-z.\-]*$|^[0-9a-f]{7,40}$`)

func looksVersion(s string) bool { return versionLike.MatchString(s) }

// skillName is what a skill's name may be, by the form.
var skillName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// read reads what the skill at dir says of itself, and what is wrong.
func (e *Entry) read(dir string) {
	e.Dir = dir
	data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		e.Issues = append(e.Issues, "没有 SKILL.md：工具不会把它当成技能。")
		return
	}
	if fi, err := os.Stat(filepath.Join(dir, "SKILL.md")); err == nil {
		e.Mod = fi.ModTime()
	}
	text := string(data)
	e.Raw = text
	if len(e.Raw) > 64<<10 {
		e.Raw = e.Raw[:64<<10]
	}
	meta, body, ok := Frontmatter(text)
	e.Title, e.Description = meta["name"], meta["description"]
	e.Body = body
	if len(e.Body) > 6000 {
		e.Body = e.Body[:6000]
	}
	e.Secret = library.HasSecret(text)
	e.Shown, e.About = shown(e.Raw), library.Mask(e.Description)
	e.Files, e.Script = listFiles(dir)
	for _, f := range e.Files {
		if !e.Secret && !strings.HasSuffix(f, "SKILL.md") && smallText(filepath.Join(dir, f)) {
			if b, err := os.ReadFile(filepath.Join(dir, f)); err == nil && library.HasSecret(string(b)) {
				e.Secret = true
			}
		}
	}
	switch {
	case !ok:
		e.Issues = append(e.Issues, "开头没有 --- 包起来的 name 和 description：工具认不出它。")
	case strings.TrimSpace(e.Description) == "":
		e.Issues = append(e.Issues, "没有 description：工具不知道什么时候该用它，不会被自动用上。")
	}
	if n := utf8.RuneCountInString(e.Description); n > 1024 {
		e.Issues = append(e.Issues, fmt.Sprintf("description 有 %d 个字，超过 1024，多出的会被截掉。", n))
	}
	if ok && e.Title == "" {
		e.Issues = append(e.Issues, "没有 name。")
	} else if e.Title != "" && (!skillName.MatchString(e.Title) || len(e.Title) > 64) {
		e.Issues = append(e.Issues, fmt.Sprintf("name「%s」只能用小写字母、数字和连字符，64 个字以内。", e.Title))
	} else if e.Title != "" && e.Title != e.Name && e.Scope != Plugin {
		e.Issues = append(e.Issues, fmt.Sprintf("name「%s」和文件夹名「%s」不一样，有的工具会认不出。", e.Title, e.Name))
	}
}

// ShownLen is how much of a SKILL.md a page shows, in characters.
const ShownLen = 3000

// shown is a SKILL.md as a page shows it: its secrets in dots, and of a
// long one its head.
func shown(raw string) string {
	text := library.Mask(raw)
	if utf8.RuneCountInString(text) > ShownLen {
		text = string([]rune(text)[:ShownLen]) + "\n…"
	}
	return strings.TrimRight(text, "\n")
}

// Frontmatter reads the head of a SKILL.md: its keys (one line each, or
// a block of lines after | or >) and what is after it.
func Frontmatter(text string) (meta map[string]string, body string, ok bool) {
	meta = map[string]string{}
	text = strings.TrimPrefix(text, "\ufeff")
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return meta, text, false
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return meta, text, false
	}
	for i := 1; i < end; i++ {
		line := lines[i]
		if line == "" || line[0] == ' ' || line[0] == '\t' || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if v == "|" || v == ">" || v == "|-" || v == ">-" || v == "" {
			var block []string
			for i+1 < end && (strings.HasPrefix(lines[i+1], " ") || strings.HasPrefix(lines[i+1], "\t") || lines[i+1] == "") {
				i++
				block = append(block, strings.TrimSpace(lines[i]))
			}
			sep := " "
			if strings.HasPrefix(v, "|") {
				sep = "\n"
			}
			v = strings.TrimSpace(strings.Join(block, sep))
		}
		meta[k] = unquote(v)
	}
	return meta, strings.TrimLeft(strings.Join(lines[end+1:], "\n"), "\n"), true
}

func unquote(v string) string {
	if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
		if v[0] == '"' {
			var s string
			if json.Unmarshal([]byte(v), &s) == nil {
				return s
			}
		}
		return v[1 : len(v)-1]
	}
	return v
}

// listFiles is the files of a skill, from its folder, and whether any is
// a program to run: a script, or a file that may be executed.
func listFiles(dir string) (files []string, script bool) {
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") && p != dir {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if d.Name() == "node_modules" || d.Name() == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if len(files) < 40 {
			files = append(files, rel)
		}
		if isScript(rel, d) {
			script = true
		}
		return nil
	})
	return files, script
}

func isScript(rel string, d fs.DirEntry) bool {
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".sh", ".bash", ".zsh", ".py", ".js", ".mjs", ".ts", ".rb", ".pl", ".ps1", ".bat", ".cmd":
		return true
	}
	if strings.HasPrefix(rel, "scripts/") || strings.HasPrefix(rel, "bin/") {
		return true
	}
	if fi, err := d.Info(); err == nil && fi.Mode()&0o111 != 0 {
		return true
	}
	return false
}

// IsScript tells whether a skill's file, by its path in it, may be run.
func IsScript(rel string) bool {
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".sh", ".bash", ".zsh", ".py", ".js", ".mjs", ".ts", ".rb", ".pl", ".ps1", ".bat", ".cmd":
		return true
	}
	return strings.HasPrefix(rel, "scripts/") || strings.HasPrefix(rel, "bin/")
}

func smallText(p string) bool {
	fi, err := os.Stat(p)
	if err != nil || fi.Size() > 256<<10 {
		return false
	}
	switch strings.ToLower(filepath.Ext(p)) {
	case ".md", ".txt", ".sh", ".py", ".js", ".ts", ".json", ".yaml", ".yml", ".toml", ".env", ".ini", ".cfg", "":
		return true
	}
	return false
}

// treeSum is a sum of a folder's files, by their paths and contents; ""
// for one not there.
func treeSum(dir string) string {
	if !isDir(dir) {
		return ""
	}
	h := sha256.New()
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") && p != dir {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		defer f.Close()
		fmt.Fprintf(h, "%s\x00", filepath.ToSlash(rel))
		_, _ = io.Copy(h, f)
		h.Write([]byte{0})
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))
}

// linkTarget is where p points, if it is a link (a junction too), made
// absolute.
func linkTarget(p string) (string, bool) {
	fi, err := os.Lstat(p)
	if err != nil || fi.Mode()&(os.ModeSymlink|os.ModeIrregular) == 0 {
		return "", false
	}
	t, err := os.Readlink(p)
	if err != nil {
		return "", false
	}
	if !filepath.IsAbs(t) {
		t = filepath.Join(filepath.Dir(p), t)
	}
	return filepath.Clean(t), true
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// samePath tells whether two paths are the one folder.
func samePath(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	return err1 == nil && err2 == nil && ra == rb
}

// usedPattern finds the skill a session's line used: Claude Code's
// Skill tool, or a skill run as a command.
var usedPattern = regexp.MustCompile(`"name":"Skill","input":\{"(?:skill|command)":"([^"]+)"|<command-name>/?([a-z0-9:\-]+)</command-name>`)

// countUsed is how many times each skill was used since env.Since, by
// its name, in the records of the tools' sessions that are read: Claude
// Code's. The newest records first, and not more than enough of them.
func countUsed(env Env, tools []Tool) map[string]int {
	used := map[string]int{}
	if !slices.ContainsFunc(tools, func(t Tool) bool { return t.Usage == "claude" }) {
		return used
	}
	type rec struct {
		path string
		mod  time.Time
	}
	var recs []rec
	files, _ := filepath.Glob(filepath.Join(env.Home, ".claude", "projects", "*", "*.jsonl"))
	for _, f := range files {
		if fi, err := os.Stat(f); err == nil && fi.ModTime().After(env.Since) {
			recs = append(recs, rec{f, fi.ModTime()})
		}
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].mod.After(recs[j].mod) })
	budget := int64(300 << 20)
	needle := []byte(`"Skill"`)
	cmd := []byte("<command-name>")
	for _, r := range recs {
		f, err := os.Open(r.path)
		if err != nil {
			continue
		}
		br := bufio.NewReaderSize(f, 256<<10)
		for {
			line, err := br.ReadSlice('\n')
			budget -= int64(len(line))
			if bytes.Contains(line, needle) || bytes.Contains(line, cmd) {
				for _, m := range usedPattern.FindAllSubmatch(line, -1) {
					name := string(m[1])
					if name == "" {
						name = string(m[2])
					}
					if i := strings.LastIndex(name, ":"); i >= 0 {
						name = name[i+1:]
					}
					used[name]++
				}
			}
			if err == bufio.ErrBufferFull {
				for err == bufio.ErrBufferFull { // a long line: the rest of it
					line, err = br.ReadSlice('\n')
					budget -= int64(len(line))
				}
			}
			if err != nil || budget < 0 {
				break
			}
		}
		f.Close()
		if budget < 0 {
			break
		}
	}
	return used
}

// readMachines is what the other computers linked, by the key of the
// skill and their names.
func readMachines(store, me string) map[string]map[string][]string {
	out := map[string]map[string][]string{}
	files, _ := filepath.Glob(filepath.Join(store, machinesDir, "*.json"))
	for _, f := range files {
		var m machine
		data, err := os.ReadFile(f)
		if err != nil || json.Unmarshal(data, &m) != nil || m.Name == "" || m.Name == me {
			continue
		}
		for key, tools := range m.Links {
			if out[key] == nil {
				out[key] = map[string][]string{}
			}
			out[key][m.Name] = tools
		}
	}
	return out
}

// machine is what a computer says it linked, in the store.
type machine struct {
	Name  string              `json:"name"`
	Links map[string][]string `json:"links"` // by the key of the skill, its tools
}
