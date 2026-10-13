// Package versions keeps past versions of maps, to look back at, compare
// and restore: a snapshot of the whole file as the app saves it now and
// then, before and after another program (an AI agent) changes it, and
// before a version is restored.
//
// Snapshots live apart from the maps, in the app's own folder, a folder
// per map named after a hash of its path, so that the maps' folders (and
// their git repositories) stay clean.
package versions

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// Why a snapshot was taken.
const (
	Auto           = "auto"           // while editing, every few minutes
	Open           = "open"           // as the map was opened
	BeforeExternal = "before-outside" // the app's version, before another program's
	External       = "outside"        // another program's change, as read
	BeforeAI       = "before-ai"      // before the MCP server changed the map
	AI             = "ai"             // after it did
	BeforeRestore  = "before-restore" // before a version was restored
	Conflict       = "conflict"       // the version given up in a conflict
)

// Label is how the UI names a reason.
func Label(reason string) string {
	switch reason {
	case Auto:
		return "自动保存"
	case Open:
		return "打开时"
	case BeforeExternal:
		return "外部修改前"
	case External:
		return "外部修改"
	case BeforeAI:
		return "AI 修改前"
	case AI:
		return "AI 修改"
	case BeforeRestore:
		return "恢复前"
	case Conflict:
		return "冲突时放弃的版本"
	}
	return reason
}

// Store keeps the versions of maps under a folder.
type Store struct{ Dir string }

// Version is a snapshot of a map.
type Version struct {
	Time   time.Time
	Reason string
	Note   string // what the change was, as an AI agent said
	path   string
}

// meta is what a snapshot's file holds besides the map.
type meta struct {
	Map  string      `json:"map"`
	Note string      `json:"note,omitempty"`
	File *model.File `json:"file"`
}

func (s Store) dirFor(mapPath string) string {
	abs, err := filepath.Abs(mapPath)
	if err != nil {
		abs = mapPath
	}
	h := sha1.Sum([]byte(abs))
	return filepath.Join(s.Dir, hex.EncodeToString(h[:8]))
}

// Moved takes the versions of a map along when its file moved.
func (s Store) Moved(from, to string) error {
	if s.Dir == "" {
		return nil
	}
	src, dst := s.dirFor(from), s.dirFor(to)
	if _, err := os.Stat(src); err != nil {
		return nil // none kept
	}
	if _, err := os.Stat(dst); err == nil {
		return nil // the new path has its own
	}
	return os.Rename(src, dst)
}

// Save keeps a snapshot of a map, unless it is the same as the last one.
// It reports whether it wrote one.
func (s Store) Save(mapPath string, f *model.File, reason, note string, now time.Time) (bool, error) {
	if s.Dir == "" || f == nil {
		return false, nil
	}
	dir := s.dirFor(mapPath)
	// Nothing changed but folding: not a version of its own.
	if last := s.List(mapPath); len(last) > 0 && reason != BeforeRestore {
		if prev, err := s.Load(last[0]); err == nil && model.SameContent(prev, f) {
			return false, nil
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, err
	}
	data, err := json.Marshal(meta{Map: mapPath, Note: note, File: f})
	if err != nil {
		return false, err
	}
	name := fmt.Sprintf("%d-%s.json", now.UnixNano(), reason)
	tmp := filepath.Join(dir, "."+name)
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return false, err
	}
	if err := os.Rename(tmp, filepath.Join(dir, name)); err != nil {
		return false, err
	}
	s.Prune(mapPath, now)
	return true, nil
}

// IsLatest tells whether f is what the newest version of a map holds, as
// when the AI that changed the file kept it already.
func (s Store) IsLatest(mapPath string, f *model.File) bool {
	last := s.List(mapPath)
	if len(last) == 0 || f == nil {
		return false
	}
	prev, err := s.Load(last[0])
	return err == nil && model.SameContent(prev, f)
}

// List returns a map's versions, the newest first.
func (s Store) List(mapPath string) []Version {
	if s.Dir == "" {
		return nil
	}
	dir := s.dirFor(mapPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Version
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".json") {
			continue
		}
		stamp, reason, ok := strings.Cut(strings.TrimSuffix(name, ".json"), "-")
		ns, err := strconv.ParseInt(stamp, 10, 64)
		if !ok || err != nil {
			continue
		}
		out = append(out, Version{Time: time.Unix(0, ns), Reason: reason, path: filepath.Join(dir, name)})
	}
	slices.SortFunc(out, func(a, b Version) int { return b.Time.Compare(a.Time) })
	return out
}

// Load reads a version's map.
func (s Store) Load(v Version) (*model.File, error) {
	m, err := s.read(v)
	if err != nil {
		return nil, err
	}
	return m.File, nil
}

// WithNotes fills in the notes of versions, as an agent gave them.
func (s Store) WithNotes(vs []Version) []Version {
	for i := range vs {
		if vs[i].Reason == AI {
			if m, err := s.read(vs[i]); err == nil {
				vs[i].Note = m.Note
			}
		}
	}
	return vs
}

func (s Store) read(v Version) (*meta, error) {
	data, err := os.ReadFile(v.path)
	if err != nil {
		return nil, err
	}
	var m meta
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m.File == nil || len(m.File.Sheets) == 0 {
		return nil, fmt.Errorf("版本文件损坏")
	}
	return &m, nil
}

// Prune thins out old versions: all of the last day stay, then one an
// hour for a week, one a day for 90 days, and none older; at most 500.
func (s Store) Prune(mapPath string, now time.Time) {
	vs := s.List(mapPath)
	kept := map[string]bool{}
	n := 0
	for _, v := range vs {
		age := now.Sub(v.Time)
		var bucket string
		switch {
		case age < 24*time.Hour:
			bucket = v.path // every one
		case age < 7*24*time.Hour:
			bucket = v.Time.Format("2006010215")
		case age < 90*24*time.Hour:
			bucket = v.Time.Format("20060102")
		default:
			bucket = ""
		}
		if bucket == "" || kept[bucket] || n >= 500 {
			_ = os.Remove(v.path)
			continue
		}
		kept[bucket] = true
		n++
	}
}

// Rename follows a map to its new path.
func (s Store) Rename(from, to string) {
	if s.Dir == "" {
		return
	}
	src, dst := s.dirFor(from), s.dirFor(to)
	if _, err := os.Stat(src); err == nil {
		_ = os.RemoveAll(dst)
		_ = os.Rename(src, dst)
	}
}
