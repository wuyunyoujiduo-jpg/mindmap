// Package ghsync keeps a folder of maps and its library in step with a
// GitHub repository, through GitHub's REST API, without git.
//
// A sync is a three-way merge. The state file remembers, for each file,
// the version both sides had at the last sync (its git blob SHA). A file
// changed on one side only takes that side's version; changed on both, the
// two are merged with the version both came from where Config.Merge can
// (the maps, topic by topic), a clip's newer edit wins, and else the local
// one stays and the repository's is kept beside it as a copy. A file deleted on one side and changed
// on the other is kept. What the merge takes from the repository is
// written into the folder; what it takes from the folder becomes one
// commit on the branch.
package ghsync

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// MaxFile is the size of the largest file synced: GitHub refuses files
// over 100 MB and warns over 50 MB.
const MaxFile = 50 << 20

// stateFile is where the folder's sync state is, in a folder not synced.
const stateFile = ".sync/github.json"

// marker is the file that starts an empty repository; not synced.
const marker = ".mindmap-sync"

// Config is the repository to sync with, and the token to reach it.
type Config struct {
	Repo   string // owner/name
	Branch string // "" for main
	Token  string
	API    string // "" for https://api.github.com
	// Client is the HTTP client; nil for http.DefaultClient.
	Client *http.Client
	// Now is the clock, for the names of conflict copies; nil for time.Now.
	Now func() time.Time
	// Merge, when not nil, merges a file changed on both sides: the
	// version both came from (nil for none), the local one and the
	// repository's, and when each was; ok false leaves them to be kept
	// apart.
	Merge func(path string, base, local, remote []byte, when MergeInfo) (merged []byte, ok bool)
	// Mounts are folders outside the synced one kept in the repository
	// under a path of their own: by that path (".skills"), the folder on
	// this computer (~/.agents/skills, which the AI agents read, and so
	// cannot be in the maps folder). The repository is the same whichever
	// computer's folder it is.
	Mounts map[string]string
	// meter is what GitHub says of the calls left, through one Sync.
	meter *meter
}

// Reserve is how many of the hour's calls a sync leaves the token's user:
// GitHub counts them for the user, all tokens together (5000 an hour),
// and a sync of many files took them all, so that nothing else of the
// account went through, a release among them.
const Reserve = 500

// meter keeps what GitHub's answers say of the calls left to the token's
// user, and when they are counted anew.
type meter struct {
	mu    sync.Mutex
	known bool
	left  int
	reset time.Time
}

func (m *meter) note(h http.Header) {
	if m == nil {
		return
	}
	left, err := strconv.Atoi(h.Get("X-RateLimit-Remaining"))
	if err != nil {
		return
	}
	at, _ := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64)
	m.mu.Lock()
	m.known, m.left, m.reset = true, left, time.Unix(at, 0)
	m.mu.Unlock()
}

// spare tells whether what is left is to be spared, and until when.
func (m *meter) spare() (time.Time, bool) {
	if m == nil {
		return time.Time{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.reset, m.known && m.left < Reserve
}

// LimitError says the calls GitHub allows the token's user in an hour are
// used up, or (Spared) that the sync stopped short of using them up. The
// sync goes on after Reset, from where it was.
type LimitError struct {
	Reset  time.Time
	Spared bool
}

func (e *LimitError) Error() string {
	when := "稍后"
	if !e.Reset.IsZero() {
		when = e.Reset.Local().Format("15:04") + " 后"
	}
	if e.Spared {
		return "这个 GitHub 账号这一小时的调用次数快用完了，剩下的留给别的用途，" + when + "自动接着同步"
	}
	return "这个 GitHub 账号这一小时的调用次数用完了（每小时 5000 次，整个账号共用），" + when + "自动接着同步"
}

// RetryAfter is when a sync that failed for GitHub's limit is worth
// trying again, zero for another failure.
func RetryAfter(err error) time.Time {
	var e *LimitError
	if errors.As(err, &e) {
		if e.Reset.IsZero() {
			return time.Now().Add(10 * time.Minute)
		}
		return e.Reset.Add(5 * time.Second)
	}
	return time.Time{}
}

// local is where a file of the repository, by its path, is on this
// computer: in the folder synced, or in the folder mounted at its path.
func (c Config) local(dir, p string) string {
	for prefix, d := range c.Mounts {
		if strings.HasPrefix(p, prefix+"/") {
			return filepath.Join(d, filepath.FromSlash(strings.TrimPrefix(p, prefix+"/")))
		}
	}
	return filepath.Join(dir, filepath.FromSlash(p))
}

// MergeInfo is when the versions merged were, to tell the user which is
// which: the last sync, which both had; the local file's last change; the
// repository's last commit of it, and the computer that made it, as its
// message names it ("" when not found).
type MergeInfo struct {
	BaseAt, LocalAt, RemoteAt time.Time
	RemoteFrom                string
}

func (c Config) branch() string {
	if c.Branch == "" {
		return "main"
	}
	return c.Branch
}

func (c Config) api() string {
	if c.API == "" {
		return "https://api.github.com"
	}
	return strings.TrimRight(c.API, "/")
}

// Result tells what a sync did.
type Result struct {
	Pulled, Pushed, Deleted int
	// Written are the local files the sync wrote or removed, slash
	// separated and relative to the folder.
	Written []string
	// Merged are the files changed on both sides and merged; Conflicts
	// those kept apart, the repository's as a copy; Skipped those too
	// large to sync.
	Merged, Conflicts, Skipped []string
	Commit                     string
}

// Synced reports whether anything moved.
func (r Result) Synced() bool { return r.Pulled+r.Pushed+r.Deleted > 0 }

// Repo is what Check finds of the repository.
type Repo struct {
	FullName      string `json:"full_name"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
	Permissions   struct {
		Push bool `json:"push"`
	} `json:"permissions"`
}

// Check reads the repository, to tell whether the token reaches it and
// may write to it.
func Check(ctx context.Context, cfg Config) (*Repo, error) {
	var r Repo
	if err := cfg.call(ctx, "GET", "", nil, &r); err != nil {
		return nil, cfg.explain(ctx, err)
	}
	if !r.Permissions.Push {
		return &r, errors.New("令牌能看到这个仓库，但不能写入：在 GitHub 编辑这个令牌，Repository permissions → Contents 选 Read and write")
	}
	return &r, nil
}

// explain says why the repository was not found: GitHub answers 404, not
// 403, for a private repository the token may not reach, so that whether
// it exists is not told. The token's account tells which it is.
func (cfg Config) explain(ctx context.Context, err error) error {
	if status(err) != 404 {
		return err
	}
	var me struct{ Login string }
	if cfg.request(ctx, "GET", "/user", nil, &me) != nil || me.Login == "" {
		return err
	}
	owner, _, _ := strings.Cut(cfg.Repo, "/")
	if !strings.EqualFold(owner, me.Login) {
		return fmt.Errorf("令牌属于 %s，找不到 %s：检查仓库名是否写对", me.Login, cfg.Repo)
	}
	// The repositories the token reaches, which a fine-grained token's
	// list is limited to.
	var repos []struct {
		FullName string `json:"full_name"`
	}
	seen := "看不到任何仓库"
	if cfg.request(ctx, "GET", "/user/repos?per_page=100&affiliation=owner", nil, &repos) == nil && len(repos) > 0 {
		var names []string
		for _, r := range repos {
			if strings.EqualFold(r.FullName, cfg.Repo) {
				// It is listed, yet not found: the name differs in a way
				// not seen, as a character that does not show.
				return fmt.Errorf("令牌能访问 %s，但 GitHub 按填写的名字找不到它：把仓库名删掉重新输入一次", r.FullName)
			}
			names = append(names, r.FullName)
		}
		seen = "只能看到：" + strings.Join(names, "、")
	}
	return fmt.Errorf("令牌（%s）有效，但没有授权访问 %s，%s。在 GitHub 编辑这个令牌（Settings → Developer settings → Fine-grained tokens）："+
		"Repository access 选「Only select repositories」并勾上这个仓库（仓库在令牌之后新建的，要再勾一次），"+
		"Repository permissions → Contents 选 Read and write", me.Login, cfg.Repo, seen)
}

// Clean takes out of a repository's name or a token what pasting brings
// along and does not show: spaces, line breaks, zero-width characters.
func Clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.Is(unicode.Cf, r) || !unicode.IsPrint(r) {
			return -1
		}
		return r
	}, s)
}

// Included tells whether a file of the folder, by its slash-separated
// relative path, is synced: maps, their assets, the library and the
// skills of the AI agents (.skills, internal/skills), but not what can be
// made again, as the library's vectors, nor hidden files (the skills'
// trash among them).
func Included(rel string) bool {
	// On Windows a name of the repository with a backslash or a colon is
	// no file's name: joined to the folder, a\..\..\x leaves it.
	if runtime.GOOS == "windows" && strings.ContainsAny(rel, `\:`) {
		return false
	}
	parts := strings.Split(rel, "/")
	for i, p := range parts {
		if strings.HasPrefix(p, ".") && !(i == 0 && (p == ".library" || p == ".skills") && len(parts) > 1) {
			return false
		}
	}
	base := parts[len(parts)-1]
	for _, p := range parts { // what a skill's program installs, made again
		if p == "node_modules" || p == "__pycache__" {
			return false
		}
	}
	switch {
	case rel == ".library/vectors.json", strings.HasSuffix(base, ".tmp"), base == "Thumbs.db", base == "desktop.ini":
		return false
	}
	return true
}

// state is what the last sync left.
type state struct {
	Repo   string            `json:"repo"`
	Branch string            `json:"branch"`
	Commit string            `json:"commit"`
	Tree   string            `json:"tree,omitempty"` // the commit's tree, not asked for again
	Base   map[string]string `json:"base"`           // path → blob SHA both sides had
	// Hashes are the local files' SHAs, by their size and time, so that
	// unchanged files are not read again.
	Hashes map[string]cached `json:"hashes"`
	At     time.Time         `json:"at"`
	// Sent are the blobs a sync uploaded and did not commit, for it did
	// not end: the next does not upload them again. A sync of many files
	// that failed at the last began again at the first, each a call.
	Sent map[string]bool `json:"sent,omitempty"`
}

type cached struct {
	Size int64     `json:"size"`
	Mod  time.Time `json:"mod"`
	SHA  string    `json:"sha"`
}

func loadState(dir string, cfg Config) *state {
	s := &state{}
	if data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(stateFile))); err == nil {
		_ = json.Unmarshal(data, s)
	}
	if s.Repo != cfg.Repo || s.Branch != cfg.branch() {
		// Another repository: nothing is known in common.
		s = &state{Repo: cfg.Repo, Branch: cfg.branch(), Hashes: s.Hashes}
	}
	if s.Base == nil {
		s.Base = map[string]string{}
	}
	if s.Hashes == nil {
		s.Hashes = map[string]cached{}
	}
	return s
}

func (s *state) save(dir string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, filepath.FromSlash(stateFile)), data)
}

// blobSHA is git's SHA of a file's content.
func blobSHA(data []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(data))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// scan finds the folder's files and their SHAs, and the mounted
// folders', under their paths.
func scan(dir string, s *state, mounts map[string]string) (files map[string]string, skipped []string, held map[string]bool, err error) {
	files, held = map[string]string{}, map[string]bool{}
	hashes := map[string]cached{}
	// What cannot be read is not gone: it is as it was when both sides
	// last agreed, so that the sync neither deletes it from the repository
	// (and then from the other computer) nor sends it. A file locked by
	// another program, a folder whose permission was lost, a file the
	// cloud has not brought down were all read as deleted. Such a path is
	// held: this round leaves it be on both sides (syncOnce).
	asBefore := func(rel string, dir bool) {
		if !dir {
			held[rel] = true
			if sha, ok := s.Base[rel]; ok {
				files[rel] = sha
			}
			return
		}
		for path, sha := range s.Base {
			if strings.HasPrefix(path, rel+"/") {
				files[path], held[path] = sha, true
			}
		}
	}
	walk := func(root, prefix string, top bool) error {
		// A folder that is a link is walked where it leads: WalkDir does
		// not follow its root, and found such a folder empty.
		if r, err := filepath.EvalSymlinks(root); err == nil {
			root = r
		}
		return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			rel, _ := filepath.Rel(root, p)
			rel = filepath.ToSlash(rel)
			if rel == "." {
				if err != nil && prefix != "" { // a folder mounted, not read: all of it as before
					asBefore(prefix, true)
					return nil
				}
				return err // the folder itself not read: no sync, rather than one of nothing
			}
			if prefix != "" {
				rel = prefix + "/" + rel
			}
			if err != nil {
				if !errors.Is(err, fs.ErrNotExist) {
					asBefore(rel, d == nil || d.IsDir())
				}
				return nil
			}
			if d.IsDir() {
				if _, mounted := mounts[rel]; top && mounted { // its own folder is elsewhere
					return filepath.SkipDir
				}
				if !Included(rel + "/x") {
					return filepath.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() || !Included(rel) {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				if !errors.Is(err, fs.ErrNotExist) {
					asBefore(rel, false)
				}
				return nil
			}
			if info.Size() > MaxFile {
				skipped = append(skipped, rel)
				return nil
			}
			if c, ok := s.Hashes[rel]; ok && c.Size == info.Size() && c.Mod.Equal(info.ModTime()) {
				files[rel], hashes[rel] = c.SHA, c
				return nil
			}
			data, err := os.ReadFile(p)
			if err != nil {
				if !errors.Is(err, fs.ErrNotExist) {
					asBefore(rel, false)
				}
				return nil
			}
			sha := blobSHA(data)
			files[rel], hashes[rel] = sha, cached{info.Size(), info.ModTime(), sha}
			return nil
		})
	}
	err = walk(dir, "", true)
	for prefix, d := range mounts {
		switch _, e := os.Stat(d); {
		case e == nil && err == nil:
			err = walk(d, prefix, false)
		case e != nil && !errors.Is(e, fs.ErrNotExist): // there, and not to be looked at
			asBefore(prefix, true)
		}
	}
	s.Hashes = hashes
	return files, skipped, held, err
}

// Sync merges the folder dir and the repository, as the package says.
// progress, when not nil, is told what is being done.
func Sync(ctx context.Context, dir string, cfg Config, progress func(string)) (Result, error) {
	if progress == nil {
		progress = func(string) {}
	}
	if cfg.meter == nil {
		cfg.meter = &meter{}
	}
	var res Result
	for attempt := 0; ; attempt++ {
		r, err := syncOnce(ctx, dir, cfg, progress)
		res.Pulled += r.Pulled
		res.Pushed += r.Pushed
		res.Deleted += r.Deleted
		res.Written = append(res.Written, r.Written...)
		res.Merged = append(res.Merged, r.Merged...)
		res.Conflicts = append(res.Conflicts, r.Conflicts...)
		res.Skipped, res.Commit = r.Skipped, r.Commit
		// Another device pushed in between: merge again with its commit.
		if errors.Is(err, errMoved) && attempt < 3 {
			continue
		}
		return res, cfg.explain(ctx, err)
	}
}

var errMoved = errors.New("the branch moved")

func syncOnce(ctx context.Context, dir string, cfg Config, progress func(string)) (Result, error) {
	var res Result
	now := time.Now
	if cfg.Now != nil {
		now = cfg.Now
	}
	s := loadState(dir, cfg)
	progress("正在检查本地文件…")
	local, skipped, held, err := scan(dir, s, cfg.Mounts)
	if err != nil {
		return res, err
	}
	res.Skipped = skipped

	progress("正在读取 GitHub 仓库…")
	head, err := cfg.head(ctx)
	if err != nil {
		return res, err
	}
	if head == "" { // an empty repository, or no such branch yet
		if head, err = cfg.start(ctx); err != nil {
			return res, err
		}
	}
	var remote map[string]string
	var tree string
	if head == s.Commit {
		remote = s.Base // unchanged since the last sync
	} else {
		if remote, tree, err = cfg.tree(ctx, head); err != nil {
			return res, err
		}
	}

	// The merge.
	paths := map[string]bool{}
	for _, m := range []map[string]string{s.Base, local, remote} {
		for p := range m {
			paths[p] = true
		}
	}
	var pull, push, unlink, drop []string // drop: delete in the repository
	merged := map[string]string{}
	keep := func(p, sha string) {
		if sha != "" {
			merged[p] = sha
		}
	}
	type conflict struct{ path, base, local, remote string }
	var conflicts []conflict
	// again is set when a path was left as it was though the repository
	// has another version of it: the commit is then not remembered as
	// read, so that the next sync reads the repository anew and sees it.
	// (Remembered, with the path's base moved to the repository's, the
	// file here read as changed here alone once it could be read again,
	// and was sent over the other computer's work.)
	again := false
	var againMu sync.Mutex
	leave := func(p string) {
		againMu.Lock()
		defer againMu.Unlock()
		if b := s.Base[p]; b != "" {
			merged[p] = b
		} else {
			delete(merged, p)
		}
		again = true
	}
	for p := range paths {
		if slices.Contains(skipped, p) {
			keep(p, remote[p])
			continue
		}
		b, l, r := s.Base[p], local[p], remote[p]
		if held[p] { // not read this round: neither sent, pulled nor deleted
			keep(p, b)
			again = again || r != b
			continue
		}
		switch {
		case l == r:
			keep(p, l)
		case l == b: // changed in the repository only
			if r == "" {
				unlink = append(unlink, p)
			} else {
				pull = append(pull, p)
				keep(p, r)
			}
		case r == b: // changed here only
			if l == "" {
				drop = append(drop, p)
			} else {
				push = append(push, p)
				keep(p, l)
			}
		case l == "": // deleted here, changed there: kept
			pull = append(pull, p)
			keep(p, r)
		case r == "": // deleted there, changed here: kept
			push = append(push, p)
			keep(p, l)
		default:
			conflicts = append(conflicts, conflict{p, b, l, r})
			keep(p, l)
		}
	}
	slices.Sort(pull)
	slices.Sort(push)

	// From the repository.
	if len(pull) > 0 {
		progress(fmt.Sprintf("正在下载 %d 个文件…", len(pull)))
	}
	if err := cfg.each(ctx, pull, func(p string) error {
		// What was written stays, and is not asked for again: a download
		// stopped goes on from where it was by itself.
		if reset, low := cfg.meter.spare(); low {
			return &LimitError{Reset: reset, Spared: true}
		}
		data, err := cfg.blob(ctx, remote[p])
		if err != nil {
			return err
		}
		if !unchanged(cfg.local(dir, p), local[p]) {
			leave(p) // changed while syncing: the next sync takes it, against what both had
			return nil
		}
		return writeAtomic(cfg.local(dir, p), data)
	}); err != nil {
		return res, err
	}
	res.Pulled = len(pull)
	res.Written = append(res.Written, pull...)
	for _, p := range unlink {
		if unchanged(cfg.local(dir, p), local[p]) {
			_ = os.Remove(cfg.local(dir, p))
		} else {
			leave(p)
		}
	}
	res.Deleted += len(unlink)
	res.Written = append(res.Written, unlink...)

	for _, c := range conflicts {
		data, err := cfg.blob(ctx, c.remote)
		if err != nil {
			return res, err
		}
		if newer, ok := newerClip(cfg.local(dir, c.path), c.path, data); ok {
			if newer { // the repository's edit of the clip is the newer
				if err := writeAtomic(cfg.local(dir, c.path), data); err != nil {
					return res, err
				}
				merged[c.path] = c.remote
				res.Pulled++
				res.Written = append(res.Written, c.path)
			} else {
				push = append(push, c.path)
			}
			continue
		}
		if ok, err := cfg.merge(ctx, dir, c.path, c.base, c.local, data, head, s.At); err != nil {
			return res, err
		} else if ok {
			res.Merged = append(res.Merged, c.path)
			res.Written = append(res.Written, c.path)
			push = append(push, c.path)
			continue
		}
		cp := conflictCopy(c.path, now())
		if err := writeAtomic(cfg.local(dir, cp), data); err != nil {
			return res, err
		}
		res.Conflicts = append(res.Conflicts, c.path)
		res.Written = append(res.Written, cp)
		merged[cp] = c.remote
		push = append(push, c.path, cp)
	}

	// To the repository.
	if len(push)+len(drop) > 0 {
		progress(fmt.Sprintf("正在上传 %d 个文件…", len(push)))
		var mu sync.Mutex
		entries := []treeEntry{}
		// What was uploaded is kept though the sync fails, so that the
		// next goes on from there.
		fail := func(err error) (Result, error) {
			for _, p := range res.Written {
				delete(s.Hashes, p)
			}
			_ = s.save(dir)
			return res, err
		}
		if s.Sent == nil {
			s.Sent = map[string]bool{}
		}
		if err := cfg.each(ctx, push, func(p string) error {
			data, err := os.ReadFile(cfg.local(dir, p))
			if err != nil {
				return err
			}
			sha := blobSHA(data)
			if inline(data) { // with the tree, no call of its own
				text := string(data)
				mu.Lock()
				entries = append(entries, treeEntry{Path: p, Content: &text})
				merged[p] = sha
				mu.Unlock()
				return nil
			}
			mu.Lock()
			sent := s.Sent[sha]
			mu.Unlock()
			if !sent {
				if reset, low := cfg.meter.spare(); low {
					return &LimitError{Reset: reset, Spared: true}
				}
				if sha, err = cfg.putBlob(ctx, data); err != nil {
					return err
				}
			}
			mu.Lock()
			s.Sent[sha] = true
			entries = append(entries, treeEntry{Path: p, SHA: &sha})
			merged[p] = sha
			mu.Unlock()
			return nil
		}); err != nil {
			return fail(err)
		}
		for _, p := range drop {
			entries = append(entries, treeEntry{Path: p})
		}
		if tree == "" {
			tree = s.Tree
		}
		if tree == "" {
			if tree, err = cfg.treeOf(ctx, head); err != nil {
				return res, err
			}
		}
		commit, made, err := cfg.commit(ctx, head, tree, entries, message(len(push), len(drop)))
		if err != nil {
			if c := status(err); c == 404 || c == 422 {
				s.Sent = nil // a blob kept from before is gone: all are sent again
			}
			return fail(err)
		}
		if err := cfg.move(ctx, commit); err != nil {
			return fail(err)
		}
		head, tree = commit, made
		res.Pushed, res.Deleted = len(push), res.Deleted+len(drop)
	}
	res.Commit = head

	if tree != "" || head != s.Commit {
		s.Tree = tree
	}
	s.Commit, s.Base, s.At, s.Sent = head, merged, now(), nil
	if again {
		s.Commit = "" // the repository is read anew next time
	}
	// The files written are hashed again next time.
	for _, p := range res.Written {
		delete(s.Hashes, p)
	}
	return res, s.save(dir)
}

func message(pushed, dropped int) string {
	host, _ := os.Hostname()
	m := fmt.Sprintf("MyMind 同步：%d 个文件", pushed)
	if dropped > 0 {
		m += fmt.Sprintf("，删除 %d 个", dropped)
	}
	if host != "" {
		m += "（" + host + "）"
	}
	return m
}

// unchanged tells whether a local file still has the SHA it was scanned
// with ("" for none).
func unchanged(full, sha string) bool {
	data, err := os.ReadFile(full)
	if err != nil {
		return sha == ""
	}
	return blobSHA(data) == sha
}

// newerClip tells, for a clip of the library, whether the repository's
// version, data, was edited after the local one; ok is false for other
// files.
func newerClip(full, p string, data []byte) (newer, ok bool) {
	// The maps' analyses and what was drawn of their topics, of the same
	// form, are taken as clips.
	if !strings.HasPrefix(p, ".library/clips/") && !strings.HasPrefix(p, ".library/maps/") &&
		!strings.HasPrefix(p, ".library/diagrams/") || path.Ext(p) != ".json" {
		return false, false
	}
	// A clip and a map's analysis say when they were updated, a set of
	// diagrams when it was drawn.
	type stamp struct {
		Updated time.Time `json:"updated"`
		Drawn   time.Time `json:"drawn"`
	}
	when := func(s stamp) time.Time {
		if s.Updated.IsZero() {
			return s.Drawn
		}
		return s.Updated
	}
	var theirs, ours stamp
	mine, err := os.ReadFile(full)
	if err != nil || json.Unmarshal(mine, &ours) != nil || json.Unmarshal(data, &theirs) != nil {
		return false, false
	}
	// With no time on either, neither is known to be the newer: a copy
	// is kept, as of any other file, and not this side's taken unsaid.
	if when(ours).IsZero() && when(theirs).IsZero() {
		return false, false
	}
	return when(theirs).After(when(ours)), true
}

// merge merges a file changed on both sides into the folder, by
// Config.Merge, and tells whether it did: not when the file changed again
// while syncing, which the next sync takes up.
func (cfg Config) merge(ctx context.Context, dir, p, base, local string, remote []byte, head string, baseAt time.Time) (bool, error) {
	if cfg.Merge == nil {
		return false, nil
	}
	full := cfg.local(dir, p)
	mine, err := os.ReadFile(full)
	if err != nil || blobSHA(mine) != local {
		return false, nil
	}
	var old []byte
	if base != "" {
		if old, err = cfg.blob(ctx, base); err != nil {
			return false, err
		}
	}
	when := MergeInfo{BaseAt: baseAt}
	if info, err := os.Stat(full); err == nil {
		when.LocalAt = info.ModTime()
	}
	when.RemoteAt, when.RemoteFrom = cfg.lastCommit(ctx, head, p)
	merged, ok := cfg.Merge(p, old, mine, remote, when)
	if !ok || !unchanged(full, local) {
		return false, nil
	}
	return true, writeAtomic(full, merged)
}

// lastCommit is when the repository's file was last committed, and from
// which computer, as the sync's message names it ("…（host）"); zero
// when not found, which is no failure of the sync.
func (cfg Config) lastCommit(ctx context.Context, head, p string) (time.Time, string) {
	var commits []struct {
		Commit struct {
			Message   string `json:"message"`
			Committer struct {
				Date time.Time `json:"date"`
			} `json:"committer"`
		} `json:"commit"`
	}
	q := "/commits?per_page=1&sha=" + url.QueryEscape(head) + "&path=" + url.QueryEscape(p)
	if cfg.call(ctx, "GET", q, nil, &commits) != nil || len(commits) == 0 {
		return time.Time{}, ""
	}
	c := commits[0].Commit
	return c.Committer.Date, hostOf(c.Message)
}

// hostOf is the computer a sync's commit message names: "MyMind 同步：3 个
// 文件（MacBook-Pro）" is "MacBook-Pro".
func hostOf(msg string) string {
	msg = strings.TrimSpace(strings.SplitN(msg, "\n", 2)[0])
	if !strings.HasPrefix(msg, "MyMind 同步") || !strings.HasSuffix(msg, "）") {
		return ""
	}
	i := strings.LastIndex(msg, "（")
	if i < 0 {
		return ""
	}
	host := strings.TrimSuffix(msg[i+len("（"):], "）")
	return strings.TrimSuffix(host, ".local")
}

// conflictCopy names the copy of the repository's version of a file
// changed on both sides: "计划 (GitHub 冲突 2026-10-11 1504).mindmap.json".
func conflictCopy(p string, now time.Time) string {
	dir, base := path.Split(p)
	ext := path.Ext(base)
	if strings.HasSuffix(base, ".mindmap.json") {
		ext = ".mindmap.json"
	}
	return dir + strings.TrimSuffix(base, ext) + " (GitHub 冲突 " + now.Format("2006-01-02 1504") + ")" + ext
}

// CopyOf is the file a conflict copy was kept beside, by its name.
func CopyOf(p string) (original string, ok bool) {
	dir, base := filepath.Split(p)
	i := strings.LastIndex(base, " (GitHub 冲突 ")
	if i < 0 {
		return "", false
	}
	rest := base[i+len(" (GitHub 冲突 "):]
	j := strings.Index(rest, ")")
	if j != len("2006-01-02 1504") {
		return "", false
	}
	return dir + base[:i] + rest[j+1:], true
}

func writeAtomic(p string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp := p + ".sync.tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// each runs fn for the paths, a few at a time.
func (cfg Config) each(ctx context.Context, paths []string, fn func(string) error) error {
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var first error
	for _, p := range paths {
		if ctx.Err() != nil {
			break
		}
		mu.Lock()
		failed := first != nil
		mu.Unlock()
		if failed { // the rest would fail as it did, each a call
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer func() { <-sem; wg.Done() }()
			if err := fn(p); err != nil {
				mu.Lock()
				if first == nil {
					first = fmt.Errorf("%s: %w", p, err)
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if first == nil {
		first = ctx.Err()
	}
	return first
}

// The REST API.

type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string {
	switch e.Status {
	case 401:
		return "GitHub 授权无效或已过期，请在设置里重新填写令牌"
	case 403:
		if strings.Contains(strings.ToLower(e.Message), "rate limit") {
			return "GitHub 请求太频繁，稍后会自动再试"
		}
		return "令牌没有这个仓库的权限（需要 Contents 读写）：" + e.Message
	case 404:
		return "找不到仓库，或令牌没有访问它的权限"
	}
	return fmt.Sprintf("GitHub 返回 %d：%s", e.Status, e.Message)
}

// call sends a request to the repository's API, path relative to it, and
// reads the JSON answer into out.
func (cfg Config) call(ctx context.Context, method, p string, in, out any) error {
	if cfg.Repo == "" || !strings.Contains(cfg.Repo, "/") {
		return errors.New("仓库应写成 owner/name")
	}
	return cfg.request(ctx, method, "/repos/"+cfg.Repo+p, in, out)
}

// request sends a request to the API, path from its root.
func (cfg Config) request(ctx context.Context, method, p string, in, out any) error {
	if cfg.Token == "" {
		return errors.New("还没有填写 GitHub 令牌")
	}
	var body io.Reader
	// A large file takes its time on a slow line: a file that never went
	// up in two minutes failed the sync each time it was tried.
	timeout := 2 * time.Minute
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
		timeout += time.Duration(len(data)/(64<<10)) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, cfg.api()+p, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := cfg.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("连不上 GitHub：%w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 200<<20))
	if err != nil {
		return err
	}
	cfg.meter.note(resp.Header)
	if resp.StatusCode >= 300 {
		var e struct{ Message string }
		_ = json.Unmarshal(data, &e)
		if (resp.StatusCode == 403 || resp.StatusCode == 429) &&
			(resp.Header.Get("X-RateLimit-Remaining") == "0" || resp.Header.Get("Retry-After") != "" || strings.Contains(strings.ToLower(e.Message), "rate limit")) {
			var reset time.Time
			if at, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil && resp.Header.Get("X-RateLimit-Remaining") == "0" {
				reset = time.Unix(at, 0)
			} else if after, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil {
				reset = time.Now().Add(time.Duration(after) * time.Second)
			}
			return &LimitError{Reset: reset}
		}
		return &apiError{resp.StatusCode, e.Message}
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

func status(err error) int {
	var e *apiError
	if errors.As(err, &e) {
		return e.Status
	}
	return 0
}

// head is the branch's commit, "" when the repository is empty or has no
// such branch.
func (cfg Config) head(ctx context.Context) (string, error) {
	var ref struct {
		Object struct{ SHA string } `json:"object"`
	}
	err := cfg.call(ctx, "GET", "/git/ref/heads/"+cfg.branch(), nil, &ref)
	if s := status(err); s == 404 || s == 409 {
		return "", nil
	}
	return ref.Object.SHA, err
}

// start makes the branch, with a first commit holding the marker file:
// the Git database API does not work in an empty repository.
func (cfg Config) start(ctx context.Context) (string, error) {
	var out struct {
		Commit struct{ SHA string } `json:"commit"`
	}
	err := cfg.call(ctx, "PUT", "/contents/"+marker, map[string]string{
		"message": "MyMind 同步",
		"content": base64.StdEncoding.EncodeToString([]byte("MyMind 的导图和收集的资料，由 MyMind 自动同步。\n")),
		"branch":  cfg.branch(),
	}, &out)
	return out.Commit.SHA, err
}

func (cfg Config) treeOf(ctx context.Context, commit string) (string, error) {
	var c struct {
		Tree struct{ SHA string } `json:"tree"`
	}
	err := cfg.call(ctx, "GET", "/git/commits/"+commit, nil, &c)
	return c.Tree.SHA, err
}

// tree lists the files of a commit that are synced.
func (cfg Config) tree(ctx context.Context, commit string) (map[string]string, string, error) {
	sha, err := cfg.treeOf(ctx, commit)
	if err != nil {
		return nil, "", err
	}
	var t struct {
		Tree []struct {
			Path, Type, SHA string
		} `json:"tree"`
		Truncated bool `json:"truncated"`
	}
	if err := cfg.call(ctx, "GET", "/git/trees/"+sha+"?recursive=1", nil, &t); err != nil {
		return nil, "", err
	}
	if t.Truncated {
		return nil, "", errors.New("仓库里的文件太多，GitHub 列不全")
	}
	files := map[string]string{}
	for _, e := range t.Tree {
		if e.Type == "blob" && Included(e.Path) {
			files[e.Path] = e.SHA
		}
	}
	return files, sha, nil
}

func (cfg Config) blob(ctx context.Context, sha string) ([]byte, error) {
	var b struct {
		Content, Encoding string
	}
	if err := cfg.call(ctx, "GET", "/git/blobs/"+sha, nil, &b); err != nil {
		return nil, err
	}
	if b.Encoding != "base64" {
		return []byte(b.Content), nil
	}
	return base64.StdEncoding.DecodeString(strings.ReplaceAll(b.Content, "\n", ""))
}

func (cfg Config) putBlob(ctx context.Context, data []byte) (string, error) {
	var out struct{ SHA string }
	err := cfg.call(ctx, "POST", "/git/blobs", map[string]string{
		"content":  base64.StdEncoding.EncodeToString(data),
		"encoding": "base64",
	}, &out)
	return out.SHA, err
}

// A treeEntry is a file of a commit: the blob uploaded for it, or its
// text itself, which GitHub makes the blob of, or neither for a file
// deleted.
type treeEntry struct {
	Path    string
	SHA     *string
	Content *string
}

func (e treeEntry) MarshalJSON() ([]byte, error) {
	m := map[string]any{"path": e.Path, "mode": "100644", "type": "blob"}
	if e.Content != nil {
		m["content"] = *e.Content
	} else {
		m["sha"] = e.SHA // nil deletes
	}
	return json.Marshal(m)
}

// A file's text goes up in the call that makes the commit's tree, with
// the others': a call a file, as a blob is, was what used up the hour's
// calls of the account (the user asked for one call for the lot). Not a
// file that is no text, nor a long one; and a tree's call holds so much
// of them, the rest in a tree made of it.
const (
	inlineMax   = 512 << 10
	inlineBatch = 4 << 20
)

// inline tells whether a file's content goes up as text in the tree.
func inline(data []byte) bool {
	return len(data) <= inlineMax && utf8.Valid(data) && bytes.IndexByte(data, 0) < 0
}

// commit makes a commit of the tree base with the entries changed, on
// parent.
func (cfg Config) commit(ctx context.Context, parent, base string, entries []treeEntry, msg string) (commit, tree string, err error) {
	var t struct{ SHA string }
	t.SHA = base
	// One tree for all, or one upon another when the texts are many.
	for len(entries) > 0 {
		n, size := 0, 0
		for n < len(entries) && (n == 0 || size < inlineBatch) {
			if c := entries[n].Content; c != nil {
				size += len(*c)
			}
			n++
		}
		if err := cfg.call(ctx, "POST", "/git/trees", map[string]any{"base_tree": t.SHA, "tree": entries[:n]}, &t); err != nil {
			return "", "", err
		}
		entries = entries[n:]
	}
	var c struct{ SHA string }
	err = cfg.call(ctx, "POST", "/git/commits", map[string]any{
		"message": msg, "tree": t.SHA, "parents": []string{parent},
	}, &c)
	return c.SHA, t.SHA, err
}

// move points the branch at commit, unless another device moved it since.
func (cfg Config) move(ctx context.Context, commit string) error {
	err := cfg.call(ctx, "PATCH", "/git/refs/heads/"+cfg.branch(), map[string]any{"sha": commit, "force": false}, nil)
	if status(err) == 422 {
		return errMoved
	}
	return err
}
