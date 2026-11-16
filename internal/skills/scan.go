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

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/gitbase"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
)

// Scope is where a skill is loaded: every session's, a project's, a
// plugin's.
type Scope int

const (
	Global Scope = iota
	Project
	Plugin
)

// A Repo is a project's repository on this computer, whose skills are
// looked for: Key names its folder in the store.
type Repo struct {
	Root, Name, Key string
	Remote          string // its origin, which names it on another computer
}

// Env is what a scan looks at.
type Env struct {
	Home    string // the user's home
	Store   string // MyMind's folder of skills
	Machine string // this computer's name
	Repos   []Repo
	Table   []Tool
	// Since is how far back the skills used are counted; Used, if not
	// nil, is the count of a scan before, taken again rather than read
	// (the records of the sessions are large).
	Since time.Time
	Used  map[string]int
}

// CopyState is how a tool's folder holds a skill.
type CopyState int

const (
	Linked    CopyState = iota // a link to MyMind's copy
	Own                        // a folder of its own, not in MyMind
	Same                       // a folder of its own while MyMind has the skill, alike
	Diverged                   // a folder of its own while MyMind has the skill, not alike
	Elsewhere                  // a link to a folder that is not MyMind's (in the repository, another tool's)
	Dangling                   // a link to what is not there
)

// A Copy is a skill as a tool's folder has it.
type Copy struct {
	Tool   string
	Path   string // in the tool's folder
	Target string // what the link points to, if it is one
	State  CopyState
	// Tracked is that the path is in its repository's git index: a link
	// committed, which the repository's other clones cannot follow.
	Tracked bool
}

// An Entry is a skill, wherever it is: in MyMind, in the tools' folders.
type Entry struct {
	Key    string // "g/name", "p/repo-key/name", "x/tool/plugin/name"
	Name   string // its folder's name
	Scope  Scope
	Repo   *Repo  // a project's
	Plugin string // a plugin's
	// Stored is that MyMind has it, at StorePath.
	Stored    bool
	StorePath string
	Copies    map[string]*Copy // by tool
	// What its SKILL.md says, of MyMind's copy if there is one.
	Title, Description, Body string
	Raw                      string // its SKILL.md, as written (the first 64 KB)
	Dir                      string // the folder read
	Files                    []string
	Script, Secret           bool
	Issues                   []string
	// Shadowed is that a skill of every session has its name: for the
	// tools that have both, that one is used.
	Shadowed bool
	Used     int
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
		if e.Scope == Project || e.Copies[tool] == nil || e.Copies[tool].State == Dangling {
			continue
		}
		n += utf8.RuneCountInString(e.Name) + utf8.RuneCountInString(e.Description)
	}
	return n
}

// Effective is the skills a session of tool in repo uses: every
// session's, its plugins', the project's, not those shadowed.
func (r *Report) Effective(tool, repo string) (on, off []*Entry) {
	for _, e := range r.Entries {
		c := e.Copies[tool]
		if c == nil || c.State == Dangling || (e.Scope == Project && (e.Repo == nil || e.Repo.Root != repo)) {
			continue
		}
		if e.Scope == Project && e.Shadowed {
			off = append(off, e)
			continue
		}
		on = append(on, e)
	}
	return on, off
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
	for _, rp := range env.Repos {
		for _, d := range skillDirs(filepath.Join(env.Store, projectsDir, rp.Key)) {
			name := filepath.Base(d)
			e := entry("p/"+rp.Key+"/"+name, name, Project)
			e.Stored, e.StorePath = true, d
		}
	}
	// The tools'.
	for _, t := range r.Tools {
		for _, g := range t.Global {
			for _, d := range skillDirs(filepath.Join(env.Home, g)) {
				name := filepath.Base(d)
				addCopy(entry("g/"+name, name, Global), t.ID, d, "")
			}
		}
		for _, rp := range env.Repos {
			for _, p := range t.Project {
				for _, d := range skillDirs(filepath.Join(rp.Root, p)) {
					name := filepath.Base(d)
					addCopy(entry("p/"+rp.Key+"/"+name, name, Project), t.ID, d, rp.Root)
				}
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
	repos := map[string]*Repo{}
	for i := range env.Repos {
		repos[env.Repos[i].Key] = &env.Repos[i]
	}
	used := env.Used
	if used == nil {
		used = countUsed(env, r.Tools)
	}
	r.Used = used
	others := readMachines(env.Store, env.Machine)
	globalNames := map[string]*Entry{}
	for _, e := range r.Entries {
		if e.Scope == Global {
			globalNames[e.Name] = e
		}
	}
	for _, e := range r.Entries {
		if e.Scope == Project {
			e.Repo = repos[strings.Split(e.Key, "/")[1]]
		}
		// What a copy is to MyMind's.
		var sum string
		if e.Stored {
			sum = treeSum(e.StorePath)
		}
		for _, c := range e.Copies {
			switch {
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
		if dir != "" {
			e.read(dir)
		}
		if e.Scope == Project {
			if g := globalNames[e.Name]; g != nil {
				for tool := range e.Copies {
					if g.Copies[tool] != nil {
						e.Shadowed = true
					}
				}
			}
		}
		e.Used = used[e.Name]
		if e.Stored && len(e.Copies) == 0 {
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

// projectsDir is the store's folder of the projects' skills: no skill's
// name has an underscore, so it is never a skill's.
const projectsDir = "_projects"

// machinesDir is where each computer says what it linked.
const machinesDir = "_machines"

// addCopy adds a tool's copy at path, a link or a folder; root is the
// repository's it is in, for a project's.
func addCopy(e *Entry, tool, path, root string) {
	if e.Copies[tool] != nil {
		return // its first folder's
	}
	c := &Copy{Tool: tool, Path: path, State: Own}
	if t, ok := linkTarget(path); ok {
		c.Target = t
	}
	if root != "" && c.Target != "" {
		if rel, err := filepath.Rel(root, path); err == nil && gitbase.IsRepo(root) {
			c.Tracked = gitbase.Tracked(root, filepath.ToSlash(rel))
		}
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
