// Package ghsync keeps a folder of maps and its library in step with a
// GitHub repository, through GitHub's REST API, without git.
//
// A sync is a three-way merge. The state file remembers, for each file,
// the version both sides had at the last sync (its git blob SHA). A file
// changed on one side only takes that side's version; changed on both, the
// local one stays and the repository's is kept beside it as a copy, but
// for clips, whose newer edit wins. A file deleted on one side and changed
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
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
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
	// Conflicts are the files changed on both sides; Skipped those too
	// large to sync.
	Conflicts, Skipped []string
	Commit             string
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
// relative path, is synced: maps, their assets and the library, but not
// what can be made again, as the library's vectors, nor hidden files.
func Included(rel string) bool {
	parts := strings.Split(rel, "/")
	for i, p := range parts {
		if strings.HasPrefix(p, ".") && !(i == 0 && p == ".library" && len(parts) > 1) {
			return false
		}
	}
	base := parts[len(parts)-1]
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
	Base   map[string]string `json:"base"` // path → blob SHA both sides had
	// Hashes are the local files' SHAs, by their size and time, so that
	// unchanged files are not read again.
	Hashes map[string]cached `json:"hashes"`
	At     time.Time         `json:"at"`
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

// LastSync is when the folder last synced with the repository, zero for
// never.
func LastSync(dir string, cfg Config) time.Time {
	s := loadState(dir, cfg)
	if s.Commit == "" {
		return time.Time{}
	}
	return s.At
}

// blobSHA is git's SHA of a file's content.
func blobSHA(data []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(data))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// scan finds the folder's files and their SHAs.
func scan(dir string, s *state) (files map[string]string, skipped []string, err error) {
	files = map[string]string{}
	hashes := map[string]cached{}
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if d.IsDir() {
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
			return nil
		}
		sha := blobSHA(data)
		files[rel], hashes[rel] = sha, cached{info.Size(), info.ModTime(), sha}
		return nil
	})
	s.Hashes = hashes
	return files, skipped, err
}

// Sync merges the folder dir and the repository, as the package says.
// progress, when not nil, is told what is being done.
func Sync(ctx context.Context, dir string, cfg Config, progress func(string)) (Result, error) {
	if progress == nil {
		progress = func(string) {}
	}
	var res Result
	for attempt := 0; ; attempt++ {
		r, err := syncOnce(ctx, dir, cfg, progress)
		res.Pulled += r.Pulled
		res.Pushed += r.Pushed
		res.Deleted += r.Deleted
		res.Written = append(res.Written, r.Written...)
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
	local, skipped, err := scan(dir, s)
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
	type conflict struct{ path, remote string }
	var conflicts []conflict
	for p := range paths {
		if slices.Contains(skipped, p) {
			keep(p, remote[p])
			continue
		}
		b, l, r := s.Base[p], local[p], remote[p]
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
			conflicts = append(conflicts, conflict{p, r})
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
		data, err := cfg.blob(ctx, remote[p])
		if err != nil {
			return err
		}
		if !unchanged(dir, p, local[p]) {
			return nil // changed while syncing: the next sync takes it
		}
		return writeAtomic(filepath.Join(dir, filepath.FromSlash(p)), data)
	}); err != nil {
		return res, err
	}
	res.Pulled = len(pull)
	res.Written = append(res.Written, pull...)
	for _, p := range unlink {
		if unchanged(dir, p, local[p]) {
			_ = os.Remove(filepath.Join(dir, filepath.FromSlash(p)))
		}
	}
	res.Deleted += len(unlink)
	res.Written = append(res.Written, unlink...)

	for _, c := range conflicts {
		data, err := cfg.blob(ctx, c.remote)
		if err != nil {
			return res, err
		}
		if newer, ok := newerClip(dir, c.path, data); ok {
			if newer { // the repository's edit of the clip is the newer
				if err := writeAtomic(filepath.Join(dir, filepath.FromSlash(c.path)), data); err != nil {
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
		cp := conflictCopy(c.path, now())
		if err := writeAtomic(filepath.Join(dir, filepath.FromSlash(cp)), data); err != nil {
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
		if err := cfg.each(ctx, push, func(p string) error {
			data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(p)))
			if err != nil {
				return err
			}
			sha, err := cfg.putBlob(ctx, data)
			if err != nil {
				return err
			}
			mu.Lock()
			entries = append(entries, treeEntry{Path: p, Mode: "100644", Type: "blob", SHA: &sha})
			merged[p] = sha
			mu.Unlock()
			return nil
		}); err != nil {
			return res, err
		}
		for _, p := range drop {
			entries = append(entries, treeEntry{Path: p, Mode: "100644", Type: "blob"})
		}
		if tree == "" {
			if tree, err = cfg.treeOf(ctx, head); err != nil {
				return res, err
			}
		}
		commit, err := cfg.commit(ctx, head, tree, entries, message(len(push), len(drop)))
		if err != nil {
			return res, err
		}
		if err := cfg.move(ctx, commit); err != nil {
			return res, err
		}
		head = commit
		res.Pushed, res.Deleted = len(push), res.Deleted+len(drop)
	}
	res.Commit = head

	s.Commit, s.Base, s.At = head, merged, now()
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
func unchanged(dir, p, sha string) bool {
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(p)))
	if err != nil {
		return sha == ""
	}
	return blobSHA(data) == sha
}

// newerClip tells, for a clip of the library, whether the repository's
// version, data, was edited after the local one; ok is false for other
// files.
func newerClip(dir, p string, data []byte) (newer, ok bool) {
	// The maps' analyses, of the same form, are taken as clips.
	if !strings.HasPrefix(p, ".library/clips/") && !strings.HasPrefix(p, ".library/maps/") || path.Ext(p) != ".json" {
		return false, false
	}
	var theirs, ours struct {
		Updated time.Time `json:"updated"`
	}
	mine, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(p)))
	if err != nil || json.Unmarshal(mine, &ours) != nil || json.Unmarshal(data, &theirs) != nil {
		return false, false
	}
	return theirs.Updated.After(ours.Updated), true
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
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
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
	if resp.StatusCode >= 300 {
		var e struct{ Message string }
		_ = json.Unmarshal(data, &e)
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
		"content": base64.StdEncoding.EncodeToString([]byte("MyMind 的导图和资料库，由 MyMind 自动同步。\n")),
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

type treeEntry struct {
	Path string  `json:"path"`
	Mode string  `json:"mode"`
	Type string  `json:"type"`
	SHA  *string `json:"sha"` // nil deletes
}

// commit makes a commit of the tree base with the entries changed, on
// parent.
func (cfg Config) commit(ctx context.Context, parent, base string, entries []treeEntry, msg string) (string, error) {
	var t struct{ SHA string }
	if err := cfg.call(ctx, "POST", "/git/trees", map[string]any{"base_tree": base, "tree": entries}, &t); err != nil {
		return "", err
	}
	var c struct{ SHA string }
	err := cfg.call(ctx, "POST", "/git/commits", map[string]any{
		"message": msg, "tree": t.SHA, "parents": []string{parent},
	}, &c)
	return c.SHA, err
}

// move points the branch at commit, unless another device moved it since.
func (cfg Config) move(ctx context.Context, commit string) error {
	err := cfg.call(ctx, "PATCH", "/git/refs/heads/"+cfg.branch(), map[string]any{"sha": commit, "force": false}, nil)
	if status(err) == 422 {
		return errMoved
	}
	return err
}
