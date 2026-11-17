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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/vault"
)

// Kinds of clips.
const (
	KindText  = "text"
	KindLink  = "link"
	KindImage = "image"
	KindFile  = "file"
	KindMap   = "map" // a map of the app, found with the clips; never kept as one
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
	// Seen says, for the AI, what was seen of a video: how long it is and
	// the pictures taken of it. It is not kept.
	Seen string `json:"-"`
	URL  string `json:"url,omitempty"`
	// File is the picture's or file's name in the files folder, Name the
	// name it had.
	File string `json:"file,omitempty"`
	Name string `json:"name,omitempty"`
	// Source says where it came from: 手动, 剪贴板, 快速记录, 拖入, AI 工具.
	Source string `json:"source,omitempty"`
	// What the AI made of it: a line saying what it is, a summary, a
	// category, tags and words to find it by. Organized is when.
	Desc     string   `json:"desc,omitempty"`
	Summary  string   `json:"summary,omitempty"`
	Category string   `json:"category,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Keywords []string `json:"keywords,omitempty"`
	// Synonyms are other words for what it is about: synonyms, the English
	// or Chinese for it, abbreviations, related terms. Analysis is the
	// AI's reading of it: its points, where it applies, what it is good for.
	Synonyms  []string   `json:"synonyms,omitempty"`
	Analysis  string     `json:"analysis,omitempty"`
	Organized *time.Time `json:"organized,omitempty"`
	Starred   bool       `json:"starred,omitempty"`
	// ToPlace says the clip waits for a place on a map: the user asked
	// that it be put on one later. What is collected is at home in the
	// collection, and waits for nothing unless this is set.
	ToPlace bool `json:"toPlace,omitempty"`
	// Locked says the whole of the text and the note is sealed, not only
	// what in them looks like a secret (internal/vault): for a password
	// that nothing tells from a word.
	Locked bool `json:"locked,omitempty"`
	// Hash is the SHA-256 of a picture's or file's data, to find it again.
	Hash    string    `json:"hash,omitempty"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
	// What was worked out of its words, kept while they are those (memo.go).
	worked *worked
}

// label is the clip's title, else the start of its text, URL or name
// (Label, which works it out once).
func (c *Clip) label() string {
	for _, s := range []string{c.Title, c.Text, c.Note, c.URL, c.Name} {
		if s = strings.TrimSpace(Mask(s)); s != "" { // a secret is never a title
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
	case KindMap:
		return "导图"
	}
	return "文档"
}

// Library is the clips of a folder, held in memory, newest first.
type Library struct {
	Dir   string
	Clips []*Clip
	index *Index
	// stale says the index is of the clips as they were, dirty which of
	// them had their words changed since: idx makes it good.
	stale  bool
	dirty  map[*Clip]bool
	vecs   *vectorFile
	hashes map[string]cachedHash // of the clips' EmbedText
	dups   *dupIndex             // the clips by what makes them the same
	// Broken are the clips' files that would not read.
	Broken []string
	// Version counts the changes, for what is worked out from the clips
	// to be kept until it changes.
	Version int
	byID    map[string]*Clip // Find's index, as of byIDVer
	byIDVer int
	// Vault seals what of a clip looks like a secret as the clip is
	// written, once a second password is set.
	Vault *vault.Vault
	// VectorsLater says the clips' vectors are read by who holds the
	// library, away from its own work (ReadVectors, UseVectors): till
	// they are given, it has none, and asks for none to be made.
	VectorsLater bool
	// seen is each clip's file as it was last read or written here, by
	// its name: what Reload tells a file changed outside by. twice says
	// some clip was in two files.
	seen  map[string]seenFile
	twice bool
}

// A seenFile is a clip's file as it was read or written: its size and
// time, and the clip it read as, nil for one that would not read.
type seenFile struct {
	size int64
	mod  time.Time
	clip *Clip
}

// Open reads the library of a folder, which need not exist yet.
func Open(dir string) (*Library, error) {
	l := &Library{Dir: dir, Vault: vault.Open(filepath.Join(dir, "vault"))}
	var err error
	if l.Clips, l.Broken, l.seen, l.twice, err = readClips(dir); err != nil {
		return nil, err
	}
	l.sort()
	return l, nil
}

// readClips reads the clips' files of a library's folder, all of them.
func readClips(dir string) (clips []*Clip, broken []string, seen map[string]seenFile, twice bool, err error) {
	entries, err := os.ReadDir(filepath.Join(dir, "clips"))
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, nil, false, err
	}
	seen = map[string]seenFile{}
	at := map[string]int{}       // where the clip of an ID is in clips
	named := map[string]string{} // and the file it was first read from
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		// Its size and time first: changed while it is read, it is read
		// again by the next Reload (as is one whose size and time could
		// not be had, kept as none).
		var f seenFile
		if info, err := e.Info(); err == nil {
			f.size, f.mod = info.Size(), info.ModTime()
		}
		data, err := os.ReadFile(filepath.Join(dir, "clips", e.Name()))
		if err != nil {
			continue
		}
		var c Clip
		if json.Unmarshal(data, &c) != nil || c.ID == "" {
			broken = append(broken, e.Name())
			seen[e.Name()] = f
			continue
		}
		f.clip = &c
		seen[e.Name()] = f
		// A second file of one clip, as a cloud's copy "abc 2.json" is:
		// the one updated later is the clip, the other is said with the
		// files that could not be read. Both loaded were two cards of one
		// key, and Find's index was made again at every call.
		if i, ok := at[c.ID]; ok {
			twice = true
			stray := e.Name()
			if stray == c.ID+".json" { // the clip's own file: the other is the stray
				stray = named[c.ID]
			}
			broken = append(broken, stray)
			if c.Updated.After(clips[i].Updated) {
				clips[i] = &c
			}
			continue
		}
		at[c.ID], named[c.ID] = len(clips), e.Name()
		clips = append(clips, &c)
	}
	return clips, broken, seen, twice, nil
}

// Reload reads again what changed in the library's folder since it was
// read or written here: the clips' files that came, that went, and that
// are of another size or time (an agent's add_clip, what a sync brought).
// It returns the clips that are new, those that changed and the IDs of
// those gone. The whole library read again for one file, on the window's
// thread, held the window for about a second in a library of some size.
//
// A clip that changed is changed in place: who holds it holds it as it is
// now. A file that cannot be read now is left as it was read before, not
// taken for gone. With a clip in two files, which is rare and is told by
// which was updated last, everything is read again as Open reads it.
func (l *Library) Reload() (fresh, changed []*Clip, gone []string, err error) {
	dir := filepath.Join(l.Dir, "clips")
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, nil, err
	}
	there := map[string]bool{}
	differ := map[string]seenFile{} // the files to read, as they are now
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		there[e.Name()] = true
		info, err := e.Info()
		if err != nil {
			continue // not to be looked at now: as it was
		}
		if was, ok := l.seen[e.Name()]; !ok || was.size != info.Size() || !was.mod.Equal(info.ModTime()) {
			differ[e.Name()] = seenFile{size: info.Size(), mod: info.ModTime()}
		}
	}
	var removed []string
	for name := range l.seen {
		if !there[name] {
			removed = append(removed, name)
		}
	}
	if len(differ)+len(removed) == 0 {
		return nil, nil, nil, nil
	}
	if l.twice {
		return l.reloadAll()
	}
	// Read those that differ. A clip's ID is in one file: found in two,
	// all is read again.
	owner := map[string]string{} // an ID's file, of the files as they will be
	for name, f := range l.seen {
		if _, d := differ[name]; f.clip != nil && there[name] && !d {
			owner[f.clip.ID] = name
		}
	}
	for name, f := range differ {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			delete(differ, name) // as it was, and read when it can be
			if was := l.seen[name]; was.clip != nil {
				owner[was.clip.ID] = name
			}
			continue
		}
		var c Clip
		if json.Unmarshal(data, &c) == nil && c.ID != "" {
			f.clip = &c
			differ[name] = f
		}
	}
	for name, f := range differ {
		if f.clip == nil {
			continue
		}
		if other, ok := owner[f.clip.ID]; ok && other != name {
			return l.reloadAll()
		}
		owner[f.clip.ID] = name
	}

	// What leaves its file: the clip of a file gone, or of one that reads
	// as another clip now, or as none.
	left := map[string]*Clip{}
	for _, name := range removed {
		if c := l.seen[name].clip; c != nil {
			left[c.ID] = c
		}
		delete(l.seen, name)
	}
	for name, f := range differ {
		if was := l.seen[name].clip; was != nil && (f.clip == nil || f.clip.ID != was.ID) {
			left[was.ID] = was
		}
	}
	have := make(map[string]*Clip, len(l.Clips))
	for _, c := range l.Clips {
		have[c.ID] = c
	}
	names := make([]string, 0, len(differ))
	for name := range differ {
		names = append(names, name)
	}
	slices.Sort(names) // as a folder is read
	for _, name := range names {
		f := differ[name]
		switch c := f.clip; {
		case c == nil: // would not read: said with the broken ones
		case have[c.ID] != nil:
			// A clip the library has, in its file or in one renamed: in
			// place.
			was := have[c.ID]
			delete(left, c.ID)
			*was = *c
			f.clip = was
			changed = append(changed, was)
			l.touch(was)
			delete(l.hashes, was.ID)
		default:
			l.Clips = append(l.Clips, c)
			have[c.ID] = c
			fresh = append(fresh, c)
		}
		l.seen[name] = f
	}
	if len(left) > 0 {
		l.Clips = slices.DeleteFunc(l.Clips, func(c *Clip) bool { return left[c.ID] == c })
		for id := range left {
			gone = append(gone, id)
			delete(l.hashes, id)
		}
		slices.Sort(gone)
	}
	l.Broken = nil
	for name, f := range l.seen {
		if f.clip == nil {
			l.Broken = append(l.Broken, name)
		}
	}
	slices.Sort(l.Broken)
	l.dups = nil
	l.sort()
	return fresh, changed, gone, nil
}

// reloadAll reads every clip's file again, as Open does, and tells what
// came, what is there still and what went.
func (l *Library) reloadAll() (fresh, changed []*Clip, gone []string, err error) {
	clips, broken, seen, twice, err := readClips(l.Dir)
	if err != nil {
		return nil, nil, nil, err
	}
	was := map[string]bool{}
	for _, c := range l.Clips {
		was[c.ID] = true
	}
	for _, c := range clips {
		if was[c.ID] {
			changed = append(changed, c)
			delete(was, c.ID)
		} else {
			fresh = append(fresh, c)
		}
	}
	for id := range was {
		gone = append(gone, id)
	}
	slices.Sort(gone)
	l.Clips, l.Broken, l.seen, l.twice = clips, broken, seen, twice
	l.index, l.dirty, l.dups, l.hashes = nil, nil, nil, nil
	l.sort()
	return fresh, changed, gone, nil
}

// wrote notes a clip's file as it was just written, so that Reload does
// not take it for changed outside.
func (l *Library) wrote(c *Clip) {
	name := c.ID + ".json"
	info, err := os.Stat(filepath.Join(l.Dir, "clips", name))
	if err != nil {
		return
	}
	if l.seen == nil {
		l.seen = map[string]seenFile{}
	}
	if _, had := l.seen[name]; !had {
		// A clip read from a file of another name has two files now.
		for other, f := range l.seen {
			if f.clip == c && other != name {
				l.twice = true
			}
		}
	}
	l.seen[name] = seenFile{info.Size(), info.ModTime(), c}
}

// forget drops what is known of a clip's files, as it is deleted.
func (l *Library) forget(c *Clip) {
	for name, f := range l.seen {
		if f.clip == c {
			delete(l.seen, name)
		}
	}
}

func (l *Library) sort() {
	slices.SortStableFunc(l.Clips, func(a, b *Clip) int { return b.Created.Compare(a.Created) })
	l.stale = true
	l.Version++
}

// changed drops what was worked out from the clips.
func (l *Library) changed() {
	l.stale, l.dups = true, nil
	l.Version++
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
	// By an index of the IDs, made again as the clips change: links to
	// clips are looked up for every node drawn.
	if l.byIDVer != l.Version || len(l.byID) != len(l.Clips) {
		l.byID, l.byIDVer = make(map[string]*Clip, len(l.Clips)), l.Version
		for _, c := range l.Clips {
			l.byID[c.ID] = c
		}
	}
	if c := l.byID[id]; c != nil && c.ID == id {
		return c
	}
	for _, c := range l.Clips { // changed in place since
		if c.ID == id {
			return c
		}
	}
	return nil
}

// Add keeps a new clip, giving it an ID and times when it has none. A clip
// already kept is not kept again: Add returns a *DuplicateError.
func (l *Library) Add(c *Clip, now time.Time) error {
	if d := l.Duplicate(c); d != nil {
		return &DuplicateError{d}
	}
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
	if l.dups != nil {
		l.dups.add(c)
	}
	l.sort()
	return nil
}

// AddFile copies a file into the library and keeps a clip of it: a
// picture or a file. It is CopyFile and AddCopied one after the other,
// for who has no window to keep waiting.
func (l *Library) AddFile(path, source string, now time.Time) (*Clip, error) {
	c, err := l.CopyFile(path, source)
	if err != nil {
		return nil, err
	}
	return l.AddCopied(c, now)
}

// A Copied is a file copied into the library's folder, yet to be kept as
// a clip (AddCopied) or dropped (Drop).
type Copied struct {
	clip     *Clip
	tmp, dst string
}

// CopyFile copies a file into the library's folder, and reads what there
// is to read of it: the long part of keeping a file, which touches no
// clip and may be done away from who holds the library (a video of a
// gigabyte was copied in the window's turn). It is copied as it is read,
// and told from those kept by the same reading: read whole into memory
// first, it took that much memory and the time to read it twice.
func (l *Library) CopyFile(path, source string) (*Copied, error) {
	src, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer src.Close()
	files := filepath.Join(l.Dir, "files")
	if err := os.MkdirAll(files, 0o755); err != nil {
		return nil, err
	}
	name := filepath.Base(path)
	c := &Clip{ID: NewID(), Kind: KindFile, Name: name, Source: source}
	if IsImage(path) {
		c.Kind = KindImage
	}
	c.File = c.ID + strings.ToLower(filepath.Ext(name))
	dst := filepath.Join(files, c.File)
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	size, err := io.Copy(io.MultiWriter(out, h), src)
	if e := out.Close(); err == nil {
		err = e
	}
	if err != nil {
		_ = os.Remove(tmp)
		return nil, err
	}
	c.Hash = hex.EncodeToString(h.Sum(nil))
	if c.Kind == KindFile {
		c.Title = strings.TrimSuffix(name, filepath.Ext(name))
		// A document's text, of one that has any to read and is no larger
		// than a document is.
		if readsText(name) && size <= maxTextFile {
			if data, err := os.ReadFile(tmp); err == nil {
				c.Text = ExtractText(name, data)
			}
		}
	}
	return &Copied{clip: c, tmp: tmp, dst: dst}, nil
}

// Drop removes a file copied that is not to be kept after all.
func (c *Copied) Drop() { _ = os.Remove(c.tmp) }

// AddCopied keeps a clip of a file CopyFile copied, unless it is kept
// already: then the copy is dropped, and a *DuplicateError says which.
func (l *Library) AddCopied(c *Copied, now time.Time) (*Clip, error) {
	if d := l.Duplicate(&Clip{Hash: c.clip.Hash}); d != nil {
		c.Drop()
		return nil, &DuplicateError{d}
	}
	if err := os.Rename(c.tmp, c.dst); err != nil {
		c.Drop()
		return nil, err
	}
	if err := l.Add(c.clip, now); err != nil {
		_ = os.Remove(c.dst)
		return nil, err
	}
	return c.clip, nil
}

// maxTextFile is the largest document whose text is read as it is kept.
const maxTextFile = 64 << 20

// AddData keeps data, a picture or file named name, as a clip: for what
// is in memory already, as a picture of the clipboard. A file is
// AddFile's.
func (l *Library) AddData(data []byte, name, kind, source string, now time.Time) (*Clip, error) {
	if d := l.Duplicate(&Clip{Hash: hashOf(data)}); d != nil {
		return nil, &DuplicateError{d}
	}
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
		c.Text = ExtractText(name, data)
	}
	c.Hash = hashOf(data)
	if err := l.Add(c, now); err != nil {
		_ = os.Remove(filepath.Join(l.FilesDir(), c.File))
		return nil, err
	}
	return c, nil
}

// Save writes a changed clip.
func (l *Library) Save(c *Clip, now time.Time) error {
	c.Updated = now
	delete(l.hashes, c.ID)
	err := l.write(c) // which seals what is to be: the index has its words after
	l.touch(c)
	l.changed()
	return err
}

func (l *Library) write(c *Clip) error {
	if l.Vault.On() {
		c.seal(l.Vault.Seal)
	}
	if err := writeClip(l.Dir, c); err != nil {
		return err
	}
	l.wrote(c)
	return nil
}

// writeClip writes a clip's file in the library of a folder.
func writeClip(lib string, c *Clip) error {
	dir := filepath.Join(lib, "clips")
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
	l.forget(c)
	l.changed()
	return nil
}

// Taken is a clip taken out of the library, which PutBack puts back.
type Taken struct {
	clip *Clip
	at   int    // where it was among the clips
	lib  string // the library's folder
	file string // where its file waits in the trash, "" when it has none
}

// ID is the ID of the clip taken.
func (t *Taken) ID() string { return t.clip.ID }

// trashDir is where the files of clips deleted wait to be taken back: a
// hidden folder of the library's, which the sync leaves alone (a name
// that begins with a dot, ghsync.Included).
func trashDir(lib string) string { return filepath.Join(lib, ".trash") }

// Trash removes a clip as Delete does, keeping what PutBack needs to put
// it back as it was, where it was: for a deletion that can be taken
// back. Its file is moved into the library's trash, not read: read whole
// to be written again, a video of a gigabyte held the window, and that
// much memory until the next thing was said. EmptyTrash clears it.
func (l *Library) Trash(id string, now time.Time) (*Taken, error) {
	i := slices.IndexFunc(l.Clips, func(c *Clip) bool { return c.ID == id })
	if i < 0 {
		return nil, errors.New("找不到这条记录")
	}
	c := l.Clips[i]
	t := &Taken{clip: c, at: i, lib: l.Dir}
	p := l.FilePath(c)
	if p != "" {
		if err := os.MkdirAll(trashDir(l.Dir), 0o755); err != nil {
			return nil, err
		}
		t.file = filepath.Join(trashDir(l.Dir), fmt.Sprintf("%d-%s", now.UnixNano(), c.File))
		switch err := os.Rename(p, t.file); {
		case os.IsNotExist(err):
			t.file = "" // not there, as one not come down yet: the clip alone goes
		case err != nil:
			return nil, err
		}
	}
	if err := os.Remove(filepath.Join(l.Dir, "clips", c.ID+".json")); err != nil && !os.IsNotExist(err) {
		if t.file != "" {
			_ = os.Rename(t.file, p)
		}
		return nil, err
	}
	l.Clips = slices.Delete(l.Clips, i, i+1)
	l.forget(c)
	l.changed()
	return t, nil
}

// PutBack puts back a clip Trash took: into this library when it is that
// of the folder the clip was in, which may have been read again since;
// into its folder alone when another is open.
func (l *Library) PutBack(t *Taken) error {
	here := l.Dir == t.lib
	if here && slices.ContainsFunc(l.Clips, func(x *Clip) bool { return x.ID == t.clip.ID }) {
		return nil
	}
	if t.file != "" {
		p := filepath.Join(t.lib, "files", t.clip.File)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		switch err := os.Rename(t.file, p); {
		case os.IsNotExist(err):
			return errors.New("删除得太久，文件已经清掉了")
		case err != nil:
			return err
		}
		t.file = ""
	}
	if !here {
		return writeClip(t.lib, t.clip)
	}
	if err := l.write(t.clip); err != nil {
		return err
	}
	l.Clips = slices.Insert(l.Clips, min(t.at, len(l.Clips)), t.clip)
	l.changed()
	return nil
}

// EmptyTrash clears the trash of what was put there before a time: of
// all of it, for the zero time. What is there since may still be taken
// back.
func (l *Library) EmptyTrash(before time.Time) {
	entries, err := os.ReadDir(trashDir(l.Dir))
	if err != nil {
		return
	}
	left := 0
	for _, e := range entries {
		at, _, _ := strings.Cut(e.Name(), "-")
		ns, err := strconv.ParseInt(at, 10, 64)
		if err == nil && !before.IsZero() && !time.Unix(0, ns).Before(before) {
			left++
			continue
		}
		_ = os.RemoveAll(filepath.Join(trashDir(l.Dir), e.Name()))
	}
	if left == 0 {
		_ = os.Remove(trashDir(l.Dir))
	}
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

// NeedsOrganizing tells whether the AI has yet to sort a clip out, or to
// say in a line what it is.
func (c *Clip) NeedsOrganizing() bool {
	if c.Locked {
		return false // sealed whole: an AI is shown dots, and has nothing to say of them
	}
	return c.Organized == nil || c.Desc == "" || len(c.Synonyms) == 0 && c.Analysis == ""
}

// Unorganized are the clips the AI has not sorted yet, oldest first.
func (l *Library) Unorganized() []*Clip {
	var out []*Clip
	for i := len(l.Clips) - 1; i >= 0; i-- {
		if l.Clips[i].NeedsOrganizing() {
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

// readsText tells whether ExtractText reads a file of this name.
func readsText(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".pdf", ".docx", ".pptx", ".xlsx":
		return true
	}
	return isTextFile(name)
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
