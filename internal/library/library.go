// Package library keeps what the user collects besides maps: text typed
// or copied, links, pictures and files, each a clip. The AI sorts clips
// into categories, tags them and sums them up; a local index finds them
// again, for the user and for the AI when it answers.
//
// The library is a folder: a JSON file per clip in clips/, and the
// pictures and files clips carry in files/.
package library

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Kinds of clips.
const (
	KindText  = "text"
	KindLink  = "link"
	KindImage = "image"
	KindFile  = "file"
)

// Clip is a thing collected.
type Clip struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title,omitempty"`
	// Text is what was typed or copied, a page's text, or what a picture
	// shows as the AI read it.
	Text string `json:"text,omitempty"`
	// Note is the user's own words on it.
	Note string `json:"note,omitempty"`
	URL  string `json:"url,omitempty"`
	// File is the picture's or file's name in the files folder, Name the
	// name it had.
	File string `json:"file,omitempty"`
	Name string `json:"name,omitempty"`
	// Source says where it came from: 手动, 剪贴板, 快速记录, 拖入, AI 工具.
	Source string `json:"source,omitempty"`
	// What the AI made of it: a summary, a category, tags and words to
	// find it by. Organized is when.
	Summary   string     `json:"summary,omitempty"`
	Category  string     `json:"category,omitempty"`
	Tags      []string   `json:"tags,omitempty"`
	Keywords  []string   `json:"keywords,omitempty"`
	Organized *time.Time `json:"organized,omitempty"`
	Starred   bool       `json:"starred,omitempty"`
	Created   time.Time  `json:"created"`
	Updated   time.Time  `json:"updated"`
}

// Label is the clip's title, else the start of its text, URL or name.
func (c *Clip) Label() string {
	for _, s := range []string{c.Title, c.Text, c.Note, c.URL, c.Name} {
		if s = strings.TrimSpace(s); s != "" {
			s, _, _ = strings.Cut(s, "\n")
			if r := []rune(s); len(r) > 60 {
				s = string(r[:60]) + "…"
			}
			return s
		}
	}
	return "（空）"
}

// KindLabel names a kind.
func KindLabel(kind string) string {
	switch kind {
	case KindLink:
		return "链接"
	case KindImage:
		return "图片"
	case KindFile:
		return "文件"
	}
	return "文字"
}

// Library is the clips of a folder, held in memory, newest first.
type Library struct {
	Dir   string
	Clips []*Clip
	index *Index
}

// Open reads the library of a folder, which need not exist yet.
func Open(dir string) (*Library, error) {
	l := &Library{Dir: dir}
	entries, err := os.ReadDir(filepath.Join(dir, "clips"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, "clips", e.Name()))
		if err != nil {
			continue
		}
		var c Clip
		if json.Unmarshal(data, &c) != nil || c.ID == "" {
			continue
		}
		l.Clips = append(l.Clips, &c)
	}
	l.sort()
	return l, nil
}

func (l *Library) sort() {
	slices.SortStableFunc(l.Clips, func(a, b *Clip) int { return b.Created.Compare(a.Created) })
	l.index = nil
}

// NewID returns a new clip ID.
func NewID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// FilesDir is where the clips' pictures and files are.
func (l *Library) FilesDir() string { return filepath.Join(l.Dir, "files") }

// FilePath is where a clip's picture or file is, "" for none.
func (l *Library) FilePath(c *Clip) string {
	if c.File == "" {
		return ""
	}
	return filepath.Join(l.FilesDir(), c.File)
}

// Find returns the clip of an ID.
func (l *Library) Find(id string) *Clip {
	for _, c := range l.Clips {
		if c.ID == id {
			return c
		}
	}
	return nil
}

// Add keeps a new clip, giving it an ID and times when it has none.
func (l *Library) Add(c *Clip, now time.Time) error {
	if c.ID == "" {
		c.ID = NewID()
	}
	if c.Created.IsZero() {
		c.Created = now
	}
	c.Updated = now
	if err := l.write(c); err != nil {
		return err
	}
	l.Clips = append([]*Clip{c}, l.Clips...) // first of those kept at the same time
	l.sort()
	return nil
}

// AddFile copies a file into the library and keeps a clip of it: a
// picture or a file.
func (l *Library) AddFile(path, source string, now time.Time) (*Clip, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	kind := KindFile
	if IsImage(path) {
		kind = KindImage
	}
	return l.AddData(data, filepath.Base(path), kind, source, now)
}

// AddData keeps data, a picture or file named name, as a clip.
func (l *Library) AddData(data []byte, name, kind, source string, now time.Time) (*Clip, error) {
	if err := os.MkdirAll(l.FilesDir(), 0o755); err != nil {
		return nil, err
	}
	c := &Clip{ID: NewID(), Kind: kind, Name: name, Source: source}
	c.File = c.ID + strings.ToLower(filepath.Ext(name))
	if err := os.WriteFile(filepath.Join(l.FilesDir(), c.File), data, 0o644); err != nil {
		return nil, err
	}
	if kind == KindFile {
		c.Title = strings.TrimSuffix(name, filepath.Ext(name))
		if isTextFile(name) && len(data) <= 1<<20 {
			c.Text = string(data)
		}
	}
	return c, l.Add(c, now)
}

// Save writes a changed clip.
func (l *Library) Save(c *Clip, now time.Time) error {
	c.Updated = now
	l.index = nil
	return l.write(c)
}

func (l *Library) write(c *Clip) error {
	dir := filepath.Join(l.Dir, "clips")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, c.ID+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Delete removes a clip and its file.
func (l *Library) Delete(id string) error {
	i := slices.IndexFunc(l.Clips, func(c *Clip) bool { return c.ID == id })
	if i < 0 {
		return errors.New("找不到这条记录")
	}
	c := l.Clips[i]
	if err := os.Remove(filepath.Join(l.Dir, "clips", c.ID+".json")); err != nil && !os.IsNotExist(err) {
		return err
	}
	if p := l.FilePath(c); p != "" {
		_ = os.Remove(p)
	}
	l.Clips = slices.Delete(l.Clips, i, i+1)
	l.index = nil
	return nil
}

// Categories are the categories in use, the most used first, with counts.
func (l *Library) Categories() []Count {
	return counts(l.Clips, func(c *Clip) []string {
		if c.Category == "" {
			return nil
		}
		return []string{c.Category}
	})
}

// Tags are the tags in use, the most used first, with counts.
func (l *Library) Tags() []Count {
	return counts(l.Clips, func(c *Clip) []string { return c.Tags })
}

// Count is how many clips have a category or tag.
type Count struct {
	Name string
	N    int
}

func counts(clips []*Clip, of func(*Clip) []string) []Count {
	n := map[string]int{}
	var order []string
	for _, c := range clips {
		for _, s := range of(c) {
			if n[s] == 0 {
				order = append(order, s)
			}
			n[s]++
		}
	}
	out := make([]Count, len(order))
	for i, s := range order {
		out[i] = Count{s, n[s]}
	}
	slices.SortStableFunc(out, func(a, b Count) int { return b.N - a.N })
	return out
}

// Unorganized are the clips the AI has not sorted yet, oldest first.
func (l *Library) Unorganized() []*Clip {
	var out []*Clip
	for i := len(l.Clips) - 1; i >= 0; i-- {
		if l.Clips[i].Organized == nil {
			out = append(out, l.Clips[i])
		}
	}
	return out
}

// imageExts are the extensions of pictures.
var imageExts = []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp"}

// IsImage tells whether a file is a picture, by its extension.
func IsImage(name string) bool {
	return slices.Contains(imageExts, strings.ToLower(filepath.Ext(name)))
}

var textExts = []string{".txt", ".md", ".markdown", ".csv", ".json", ".yaml", ".yml", ".go", ".py", ".js", ".ts", ".tsx", ".jsx", ".html", ".css", ".sql", ".sh", ".rs", ".java", ".kt", ".swift", ".c", ".h", ".cpp", ".toml", ".xml", ".log"}

func isTextFile(name string) bool {
	return slices.Contains(textExts, strings.ToLower(filepath.Ext(name)))
}

// IsURL tells whether text is a lone web address.
func IsURL(text string) bool {
	s := strings.TrimSpace(text)
	if strings.ContainsAny(s, " \t\n") {
		return false
	}
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// In is the library of a maps folder: a hidden folder in it, so that the
// library goes wherever the maps go.
func In(mapsDir string) string { return filepath.Join(mapsDir, ".library") }
