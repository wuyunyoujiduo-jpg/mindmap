// Package automate keeps the automations: Python programs, each a file or
// a folder of files in the automations' folder, run by hand or on a
// schedule, that call back into the app (the mymind module) to ask a model,
// notify, ask for a confirmation or keep a clip.
//
// The folder holds:
//
//	日报.py              a program of one file, its settings in its header
//	网页监控/             a program of several files:
//	  automation.toml    its settings (name, trigger, entry, timeout, model)
//	  main.py            the entry, by default
//	  requirements.txt   what it needs from pip
//	_共享/               modules every program may import
//	.runs/ .state/ .sdk/ .venv/ .meta.json   kept by the app
package automate

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Shared is the folder of modules shared by all the programs.
const Shared = "_共享"

// Settings is the file of a folder's settings.
const Settings = "automation.toml"

// DefaultTimeout is how long a run may take when nothing says otherwise.
const DefaultTimeout = 30 * time.Minute

// Automation is one program.
type Automation struct {
	ID      string // its file's or folder's name in the folder
	Path    string // the file, or the folder
	Folder  bool
	Name    string
	Trigger string // as written: "手动", "每天 09:00", …
	Entry   string // the file run, relative to Dir
	Timeout time.Duration
	// Model is what the program's mymind.llm asks when it names none: a
	// model ("deepseek-chat", "claude-…") or a command ("cli:claude -p").
	// There is no default: a program that asks a model says which.
	Model string
	// Agent is the command of the coding tool mymind.agent runs, as
	// "claude -p" or "codex exec": none by default either.
	Agent string
	// Settings are all its settings, its own keys included, which the
	// program reads as mymind.settings.
	Settings map[string]string
	Err      string // what is wrong with its settings
}

// Dir is the folder the program runs in.
func (a *Automation) Dir() string {
	if a.Folder {
		return a.Path
	}
	return filepath.Dir(a.Path)
}

// EntryPath is the file run.
func (a *Automation) EntryPath() string { return filepath.Join(a.Dir(), a.Entry) }

// List reads the programs of the folder, by name.
func List(root string) ([]*Automation, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []*Automation
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "__pycache__" {
			continue
		}
		path := filepath.Join(root, name)
		switch {
		case e.IsDir():
			out = append(out, readFolder(path))
		case strings.HasSuffix(name, ".py"):
			out = append(out, readFile(path))
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Load reads the program of the folder by its ID.
func Load(root, id string) (*Automation, error) {
	path := filepath.Join(root, id)
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		return readFolder(path), nil
	}
	return readFile(path), nil
}

func readFile(path string) *Automation {
	a := &Automation{ID: filepath.Base(path), Path: path, Entry: filepath.Base(path), Timeout: DefaultTimeout}
	a.Name = strings.TrimSuffix(a.ID, ".py")
	data, err := os.ReadFile(path)
	if err != nil {
		a.Err = err.Error()
		return a
	}
	a.apply(Header(string(data)))
	return a
}

func readFolder(path string) *Automation {
	a := &Automation{ID: filepath.Base(path), Path: path, Folder: true, Name: filepath.Base(path), Entry: "main.py", Timeout: DefaultTimeout}
	data, err := os.ReadFile(filepath.Join(path, Settings))
	if err == nil {
		kv, err := ParseSettings(string(data))
		if err != nil {
			a.Err = err.Error()
		}
		a.apply(kv)
	} else if !errors.Is(err, fs.ErrNotExist) {
		a.Err = err.Error()
	}
	if _, err := os.Stat(a.EntryPath()); err != nil && a.Err == "" {
		a.Err = "找不到入口文件 " + a.Entry
	}
	return a
}

func (a *Automation) apply(kv map[string]string) {
	a.Settings = kv
	for k, v := range kv {
		switch k {
		case "name", "名称":
			if v != "" {
				a.Name = v
			}
		case "trigger", "触发":
			a.Trigger = v
		case "entry", "入口":
			if a.Folder && v != "" {
				a.Entry = v
			}
		case "model", "模型":
			a.Model = v
		case "agent", "编码工具":
			a.Agent = v
		case "timeout", "超时":
			if d, err := ParseDuration(v); err == nil {
				a.Timeout = d
			} else if a.Err == "" {
				a.Err = "超时写法不对：" + v
			}
		}
	}
	if _, err := ParseTrigger(a.Trigger); err != nil && a.Err == "" {
		a.Err = err.Error()
	}
}

// Header reads the settings in the comments that start a file of one
// program: "# 名称: 日报", "# 触发: 每天 09:00".
func Header(src string) map[string]string {
	kv := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(src))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#!") || strings.HasPrefix(line, "# -*-") {
			continue
		}
		if !strings.HasPrefix(line, "#") {
			break
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
		k, v, ok := cutKey(line)
		if ok {
			kv[k] = v
		}
	}
	return kv
}

func cutKey(line string) (string, string, bool) {
	i := strings.IndexAny(line, ":：")
	if i <= 0 {
		return "", "", false
	}
	k := strings.ToLower(strings.TrimSpace(line[:i]))
	v := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line[i:], ":"), "："))
	switch k {
	case "", "http", "https":
		return "", "", false
	}
	if strings.ContainsAny(k, " \t") || utf8.RuneCountInString(k) > 16 {
		return "", "", false
	}
	return k, v, true
}

// ParseSettings reads automation.toml, the few keys of it there are:
// key = "value" lines, with # comments.
func ParseSettings(src string) (map[string]string, error) {
	kv := map[string]string{}
	for n, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return kv, fmt.Errorf("%s 第 %d 行看不懂：%s", Settings, n+1, line)
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if strings.HasPrefix(v, `"`) {
			s, err := strconv.Unquote(v[:strings.LastIndex(v, `"`)+1])
			if err != nil {
				return kv, fmt.Errorf("%s 第 %d 行的引号不对：%s", Settings, n+1, line)
			}
			v = s
		} else if i := strings.Index(v, "#"); i >= 0 {
			v = strings.TrimSpace(v[:i])
		}
		kv[strings.ToLower(k)] = v
	}
	return kv, nil
}

// ParseDuration reads "90s", "10m", "2h", "10 分钟", "2 小时" or a number of
// minutes.
func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if d, err := time.ParseDuration(s); err == nil && d > 0 {
		return d, nil
	}
	for suffix, unit := range map[string]time.Duration{"秒": time.Second, "分钟": time.Minute, "分": time.Minute, "小时": time.Hour} {
		if n, ok := strings.CutSuffix(s, suffix); ok {
			if v, err := strconv.Atoi(strings.TrimSpace(n)); err == nil && v > 0 {
				return time.Duration(v) * unit, nil
			}
		}
	}
	if v, err := strconv.Atoi(s); err == nil && v > 0 {
		return time.Duration(v) * time.Minute, nil
	}
	return 0, fmt.Errorf("看不懂的时长：%s", s)
}

// Files are the program's files, relative to its folder: all of a
// folder's, but what the app or Python leave there.
func (a *Automation) Files() []string {
	if !a.Folder {
		return []string{a.ID}
	}
	var out []string
	_ = filepath.WalkDir(a.Path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if path != a.Path && (strings.HasPrefix(name, ".") || name == "__pycache__" || name == "node_modules") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(a.Path, path)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(out)
	return out
}

// Code are the files that make the program, as approved: its Python, its
// settings and its requirements, not the data it keeps.
func (a *Automation) Code() map[string]string {
	out := map[string]string{}
	for _, f := range a.Files() {
		if !IsCode(f) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(a.Dir(), filepath.FromSlash(f)))
		if err == nil {
			out[f] = string(data)
		}
	}
	return out
}

// IsCode tells whether a file of a program is part of its code.
func IsCode(rel string) bool {
	base := filepath.Base(rel)
	return strings.HasSuffix(base, ".py") || base == Settings || base == "requirements.txt"
}

// Hash is the fingerprint of the code, which an approval is for.
func Hash(code map[string]string) string {
	keys := make([]string, 0, len(code))
	for k := range code {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	h := sha256.New()
	for _, k := range keys {
		fmt.Fprintf(h, "%s\x00%d\x00%s", k, len(code[k]), code[k])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Meta is what the app keeps of a program: whether its schedule is on, the
// code last approved, and its last run.
type Meta struct {
	Enabled  bool              `json:"enabled,omitempty"`
	Approved string            `json:"approved,omitempty"` // the hash
	Code     map[string]string `json:"code,omitempty"`     // the code approved
	LastRun  time.Time         `json:"lastRun,omitzero"`
	Status   string            `json:"status,omitempty"` // of the last run
	// Since is when its schedule was turned on, which the first run
	// counts from.
	Since time.Time `json:"since,omitzero"`
}

// From is what the next scheduled run counts from.
func (m *Meta) From() time.Time {
	if m.LastRun.After(m.Since) {
		return m.LastRun
	}
	return m.Since
}

// Metas are the programs', by ID, in .meta.json.
type Metas map[string]*Meta

func metaPath(root string) string { return filepath.Join(root, ".meta.json") }

// LoadMetas reads .meta.json; a missing one is empty.
func LoadMetas(root string) Metas {
	m := Metas{}
	data, err := os.ReadFile(metaPath(root))
	if err == nil {
		_ = json.Unmarshal(data, &m)
	}
	for k, v := range m {
		if v == nil {
			delete(m, k)
		}
	}
	return m
}

// Save writes .meta.json, whole, by a file renamed over it.
func (m Metas) Save(root string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(metaPath(root), data)
}

// Of is the program's, made when missing.
func (m Metas) Of(id string) *Meta {
	if m[id] == nil {
		m[id] = &Meta{}
	}
	return m[id]
}

// Approve keeps the code as approved.
func (m *Meta) Approve(code map[string]string) {
	m.Approved, m.Code = Hash(code), code
}

// NeedsApproval tells whether the code changed since it was approved.
func (m *Meta) NeedsApproval(code map[string]string) bool {
	return m == nil || m.Approved != Hash(code)
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Change is a file of a program as it was approved and as it is.
type Change struct {
	File     string
	Old, New string
	Removed  bool
	Added    bool
}

// Changes are the files that changed since the code was approved.
func Changes(old, cur map[string]string) []Change {
	var out []Change
	for f, s := range cur {
		o, ok := old[f]
		if !ok {
			out = append(out, Change{File: f, New: s, Added: true})
		} else if o != s {
			out = append(out, Change{File: f, Old: o, New: s})
		}
	}
	for f, o := range old {
		if _, ok := cur[f]; !ok {
			out = append(out, Change{File: f, Old: o, Removed: true})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].File < out[j].File })
	return out
}

// Write puts files in the program's place: in a folder for several (or
// when it is one), else as one file. It returns the program's ID.
func Write(root, id string, files map[string]string) (string, error) {
	if len(files) == 0 {
		return "", errors.New("没有文件")
	}
	folder := len(files) > 1 || !strings.HasSuffix(id, ".py")
	if st, err := os.Stat(filepath.Join(root, id)); err == nil {
		folder = st.IsDir()
	}
	if !folder {
		for _, s := range files {
			return id, writeFile(filepath.Join(root, id), []byte(s))
		}
	}
	dir := filepath.Join(root, id)
	for f, s := range files {
		clean := filepath.Clean(filepath.FromSlash(f))
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("文件不在程序的文件夹里：%s", f)
		}
		if err := writeFile(filepath.Join(dir, clean), []byte(s)); err != nil {
			return "", err
		}
	}
	return id, nil
}

// SafeName makes a name fit for a file or folder.
func SafeName(name string) string {
	name = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', '\n', '\r', '\t':
			return '-'
		}
		return r
	}, strings.TrimSpace(name))
	name = strings.TrimLeft(name, "._ ")
	if name == "" {
		name = "自动化"
	}
	return name
}

// FreeID is an ID not taken in the folder, from a name.
func FreeID(root, name, ext string) string {
	base := SafeName(name)
	id := base + ext
	for n := 2; ; n++ {
		if _, err := os.Stat(filepath.Join(root, id)); errors.Is(err, fs.ErrNotExist) {
			return id
		}
		id = fmt.Sprintf("%s %d%s", base, n, ext)
	}
}
