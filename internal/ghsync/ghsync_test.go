package ghsync

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeGitHub is a repository in memory, behind the part of GitHub's API
// that Sync uses.
type fakeGitHub struct {
	mu      sync.Mutex
	blobs   map[string][]byte
	trees   map[string]map[string]string // tree → path → blob
	execs   map[string]map[string]bool   // tree → the paths of mode 100755
	commits map[string][2]string         // commit → tree, parent
	head    string                       // "" for an empty repository
	pushes  int
	// The limit of the calls: left counts down with each when it is set
	// (limited), told in the answers' headers, and at none every call is
	// refused. uploads counts the blobs sent.
	limited bool
	left    int
	uploads int
	drain   int // at this many uploads something else takes the calls left
}

func newFake() *fakeGitHub {
	return &fakeGitHub{blobs: map[string][]byte{}, trees: map[string]map[string]string{}, execs: map[string]map[string]bool{}, commits: map[string][2]string{}}
}

func hashOf(s string) string { h := sha1.Sum([]byte(s)); return hex.EncodeToString(h[:]) }

func (f *fakeGitHub) putTree(files map[string]string) string { return f.putTreeExec(files, nil) }

func (f *fakeGitHub) putTreeExec(files map[string]string, exec map[string]bool) string {
	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k + "=" + files[k] + ";")
		if exec[k] {
			b.WriteString("x;")
		}
	}
	sha := hashOf("tree " + b.String())
	f.trees[sha], f.execs[sha] = files, exec
	return sha
}

// modeOf is the mode the branch has a file with.
func (f *fakeGitHub) modeOf(p string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.execs[f.commits[f.head][0]][p] {
		return "100755"
	}
	return "100644"
}

func (f *fakeGitHub) putCommit(tree, parent string) string {
	sha := hashOf(fmt.Sprintf("commit %s %s %d", tree, parent, len(f.commits)))
	f.commits[sha] = [2]string{tree, parent}
	return sha
}

// files is what the branch holds now.
func (f *fakeGitHub) files() map[string]string {
	out := map[string]string{}
	if f.head == "" {
		return out
	}
	for p, b := range f.trees[f.commits[f.head][0]] {
		out[p] = string(f.blobs[b])
	}
	return out
}

// write commits a change made on GitHub itself, as by another device.
func (f *fakeGitHub) write(p, content string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	files, exec := map[string]string{}, map[string]bool{}
	if f.head != "" {
		for k, v := range f.trees[f.commits[f.head][0]] {
			files[k] = v
		}
		for k := range f.execs[f.commits[f.head][0]] {
			exec[k] = true
		}
	}
	if content == "" {
		delete(files, p)
		delete(exec, p)
	} else {
		sha := blobSHA([]byte(content))
		f.blobs[sha] = []byte(content)
		files[p] = sha
	}
	f.head = f.putCommit(f.putTreeExec(files, exec), f.head)
}

func (f *fakeGitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := strings.TrimPrefix(r.URL.Path, "/repos/me/notes")
	reply := func(code int, v any) {
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	var in map[string]any
	_ = json.NewDecoder(r.Body).Decode(&in)
	if f.limited {
		w.Header().Set("X-RateLimit-Reset", fmt.Sprint(limitReset.Unix()))
		if f.left <= 0 {
			w.Header().Set("X-RateLimit-Remaining", "0")
			reply(403, map[string]string{"message": "API rate limit exceeded for user ID 1."})
			return
		}
		f.left--
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprint(f.left))
	}
	switch {
	case r.Header.Get("Authorization") != "Bearer good":
		reply(401, map[string]string{"message": "Bad credentials"})
	case r.URL.Path == "/user":
		reply(200, map[string]string{"login": "me"})
	case r.URL.Path == "/user/repos":
		reply(200, []map[string]string{{"full_name": "me/notes"}, {"full_name": "me/blog"}})
	case !strings.HasPrefix(r.URL.Path, "/repos/me/notes"):
		reply(404, map[string]string{"message": "Not Found"})
	case p == "" && r.Method == "GET":
		reply(200, map[string]any{"full_name": "me/notes", "private": true, "permissions": map[string]bool{"push": true}})
	case p == "/git/ref/heads/main":
		if f.head == "" {
			reply(409, map[string]string{"message": "Git Repository is empty."})
			return
		}
		reply(200, map[string]any{"object": map[string]string{"sha": f.head}})
	case strings.HasPrefix(p, "/contents/") && r.Method == "PUT":
		data, _ := base64.StdEncoding.DecodeString(in["content"].(string))
		sha := blobSHA(data)
		f.blobs[sha] = data
		f.head = f.putCommit(f.putTree(map[string]string{strings.TrimPrefix(p, "/contents/"): sha}), "")
		reply(201, map[string]any{"commit": map[string]string{"sha": f.head}})
	case p == "/commits" && r.URL.Query().Get("path") != "" && r.URL.Query().Get("sha") == f.head:
		reply(200, []any{map[string]any{"commit": map[string]any{
			"message":   "MyMind 同步：1 个文件（Office-Mac.local）",
			"committer": map[string]string{"date": "2026-11-15T14:20:00Z"},
		}}})
	case strings.HasPrefix(p, "/git/commits/"):
		reply(200, map[string]any{"tree": map[string]string{"sha": f.commits[strings.TrimPrefix(p, "/git/commits/")][0]}})
	case strings.HasPrefix(p, "/git/trees/") && r.Method == "GET":
		var entries []map[string]string
		tree := strings.TrimPrefix(p, "/git/trees/")
		for path, sha := range f.trees[tree] {
			mode := "100644"
			if f.execs[tree][path] {
				mode = "100755"
			}
			entries = append(entries, map[string]string{"path": path, "type": "blob", "sha": sha, "mode": mode})
		}
		reply(200, map[string]any{"tree": entries})
	case strings.HasPrefix(p, "/git/blobs/") && r.Method == "GET":
		if _, ok := f.blobs[strings.TrimPrefix(p, "/git/blobs/")]; !ok {
			reply(404, map[string]string{"message": "Not Found"})
			return
		}
		reply(200, map[string]string{"content": base64.StdEncoding.EncodeToString(f.blobs[strings.TrimPrefix(p, "/git/blobs/")]), "encoding": "base64"})
	case p == "/git/blobs":
		if f.uploads++; f.uploads == f.drain {
			f.left = 0
		}
		data, _ := base64.StdEncoding.DecodeString(in["content"].(string))
		sha := blobSHA(data)
		f.blobs[sha] = data
		reply(201, map[string]string{"sha": sha})
	case p == "/git/trees":
		files, exec := map[string]string{}, map[string]bool{}
		for k, v := range f.trees[in["base_tree"].(string)] {
			files[k] = v
		}
		for k := range f.execs[in["base_tree"].(string)] {
			exec[k] = true
		}
		for _, e := range in["tree"].([]any) {
			e := e.(map[string]any)
			if exec[e["path"].(string)] = e["mode"] == "100755"; !exec[e["path"].(string)] {
				delete(exec, e["path"].(string))
			}
			if text, ok := e["content"].(string); ok { // the text itself: its blob is made here
				if _, both := e["sha"]; both {
					reply(422, map[string]string{"message": "tree.sha and tree.content given"})
					return
				}
				sha := blobSHA([]byte(text))
				f.blobs[sha] = []byte(text)
				files[e["path"].(string)] = sha
			} else if sha, ok := e["sha"].(string); ok {
				if f.blobs[sha] == nil {
					reply(422, map[string]string{"message": "tree.sha " + sha + " is not a valid blob"})
					return
				}
				files[e["path"].(string)] = sha
			} else {
				delete(files, e["path"].(string))
				delete(exec, e["path"].(string))
			}
		}
		reply(201, map[string]string{"sha": f.putTreeExec(files, exec)})
	case p == "/git/commits":
		parent := in["parents"].([]any)[0].(string)
		reply(201, map[string]string{"sha": f.putCommit(in["tree"].(string), parent)})
	case p == "/git/refs/heads/main" && r.Method == "PATCH":
		sha := in["sha"].(string)
		if f.commits[sha][1] != f.head {
			reply(422, map[string]string{"message": "Update is not a fast forward"})
			return
		}
		f.head = sha
		f.pushes++
		reply(200, map[string]any{})
	default:
		reply(404, map[string]string{"message": "Not Found " + r.Method + " " + p})
	}
}

func setup(t *testing.T) (*fakeGitHub, Config) {
	f := newFake()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, Config{Repo: "me/notes", Token: "good", API: srv.URL,
		Now: func() time.Time { return time.Date(2026, 10, 11, 15, 4, 0, 0, time.UTC) }}
}

func put(t *testing.T, dir, p, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(p))
	os.MkdirAll(filepath.Dir(full), 0o755)
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	// Later than what was there, as an edit is.
	later := time.Now().Add(time.Duration(len(content)) * time.Second)
	os.Chtimes(full, later, later)
}

func read(dir, p string) string {
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(p)))
	if err != nil {
		return ""
	}
	return string(data)
}

func sync1(t *testing.T, dir string, cfg Config) Result {
	t.Helper()
	res, err := Sync(context.Background(), dir, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestTwoDevices(t *testing.T) {
	f, cfg := setup(t)
	a, b := t.TempDir(), t.TempDir()
	put(t, a, "项目.mindmap.json", `{"v":1}`)
	put(t, a, ".library/clips/c1.json", `{"id":"c1","updated":"2026-10-01T00:00:00Z"}`)
	put(t, a, ".library/files/pic.png", "PNG")
	put(t, a, ".library/vectors.json", "[]")
	put(t, a, ".DS_Store", "x")

	// An empty repository is started, and takes the folder.
	res := sync1(t, a, cfg)
	if res.Pushed != 3 {
		t.Fatalf("pushed %d", res.Pushed)
	}
	got := f.files()
	if got["项目.mindmap.json"] != `{"v":1}` || got[".library/files/pic.png"] != "PNG" || got[".library/vectors.json"] != "" || got[".DS_Store"] != "" {
		t.Fatalf("repository %v", got)
	}

	// The other device gets it all.
	res = sync1(t, b, cfg)
	if res.Pulled != 3 || read(b, "项目.mindmap.json") != `{"v":1}` || read(b, ".library/clips/c1.json") == "" {
		t.Fatalf("pulled %d: %q", res.Pulled, read(b, "项目.mindmap.json"))
	}
	if read(b, marker) != "" {
		t.Fatal("the marker came down")
	}

	// Nothing changed: nothing moves.
	if res := sync1(t, a, cfg); res.Synced() {
		t.Fatalf("idle sync moved %+v", res)
	}

	// An edit on b, a new file and a deletion on a.
	put(t, b, "项目.mindmap.json", `{"v":2,"from":"b"}`)
	sync1(t, b, cfg)
	put(t, a, "新导图.mindmap.json", `{"new":true}`)
	os.Remove(filepath.Join(a, ".library/files/pic.png"))
	res = sync1(t, a, cfg)
	if read(a, "项目.mindmap.json") != `{"v":2,"from":"b"}` || res.Pulled != 1 || res.Pushed != 1 || res.Deleted != 1 {
		t.Fatalf("a: %+v %q", res, read(a, "项目.mindmap.json"))
	}
	sync1(t, b, cfg)
	if read(b, "新导图.mindmap.json") != `{"new":true}` || read(b, ".library/files/pic.png") != "" {
		t.Fatal("b did not follow")
	}
}

func TestConflicts(t *testing.T) {
	f, cfg := setup(t)
	a, b := t.TempDir(), t.TempDir()
	put(t, a, "计划.mindmap.json", "base")
	put(t, a, ".library/clips/c1.json", `{"id":"c1","updated":"2026-10-01T00:00:00Z"}`)
	sync1(t, a, cfg)
	sync1(t, b, cfg)

	// Both change the map, and the clip; b's clip edit is the later.
	put(t, a, "计划.mindmap.json", "from a")
	put(t, a, ".library/clips/c1.json", `{"id":"c1","updated":"2026-10-02T00:00:00Z","note":"a"}`)
	put(t, b, "计划.mindmap.json", "from b!")
	put(t, b, ".library/clips/c1.json", `{"id":"c1","updated":"2026-10-03T00:00:00Z","note":"b"}`)
	sync1(t, b, cfg)
	res := sync1(t, a, cfg)

	copyName := "计划 (GitHub 冲突 2026-10-11 1504).mindmap.json"
	if read(a, "计划.mindmap.json") != "from a" || read(a, copyName) != "from b!" || len(res.Conflicts) != 1 {
		t.Fatalf("map: %q copy %q, %+v", read(a, "计划.mindmap.json"), read(a, copyName), res)
	}
	if !strings.Contains(read(a, ".library/clips/c1.json"), `"note":"b"`) {
		t.Fatalf("the later clip edit lost: %s", read(a, ".library/clips/c1.json"))
	}
	if got := f.files(); got["计划.mindmap.json"] != "from a" || got[copyName] != "from b!" {
		t.Fatalf("repository %v", got)
	}

	// Deleted on one side, changed on the other: kept.
	sync1(t, b, cfg)
	os.Remove(filepath.Join(a, "计划.mindmap.json"))
	put(t, b, "计划.mindmap.json", "b again, longer")
	sync1(t, b, cfg)
	sync1(t, a, cfg)
	if read(a, "计划.mindmap.json") != "b again, longer" {
		t.Fatal("a deletion beat an edit")
	}
}

func TestBranchMovedWhileSyncing(t *testing.T) {
	f, cfg := setup(t)
	a := t.TempDir()
	put(t, a, "a.mindmap.json", "1")
	sync1(t, a, cfg)
	put(t, a, "a.mindmap.json", "22")
	// Another device pushes between this sync's read and its push.
	once := false
	cfg.Client = &http.Client{Transport: hook{func(r *http.Request) {
		if !once && r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/git/commits") {
			once = true
			f.write("other.mindmap.json", "from elsewhere")
		}
	}, f}}
	res := sync1(t, a, cfg)
	if !once || read(a, "other.mindmap.json") != "from elsewhere" || f.files()["a.mindmap.json"] != "22" || f.files()["other.mindmap.json"] == "" {
		t.Fatalf("%+v %v", res, f.files())
	}
}

type hook struct {
	fn func(*http.Request)
	f  *fakeGitHub
}

func (h hook) RoundTrip(r *http.Request) (*http.Response, error) {
	h.fn(r)
	return http.DefaultTransport.RoundTrip(r)
}

func TestErrors(t *testing.T) {
	_, cfg := setup(t)
	cfg.Token = "bad"
	if _, err := Sync(context.Background(), t.TempDir(), cfg, nil); err == nil || !strings.Contains(err.Error(), "授权") {
		t.Fatalf("err %v", err)
	}
	cfg.Token = "good"
	if r, err := Check(context.Background(), cfg); err != nil || !r.Private {
		t.Fatalf("check %v %v", r, err)
	}
	cfg.Repo = "nonsense"
	if _, err := Check(context.Background(), cfg); err == nil {
		t.Fatal("no error for a bad repository name")
	}
	// Not found: a repository of the token's account it may not reach, or
	// a name of another account mistyped.
	cfg.Repo = "me/secret"
	if _, err := Check(context.Background(), cfg); err == nil || !strings.Contains(err.Error(), "没有授权访问 me/secret，只能看到：me/notes、me/blog") {
		t.Fatalf("err %v", err)
	}
	cfg.Repo = "someone/notes"
	if _, err := Check(context.Background(), cfg); err == nil || !strings.Contains(err.Error(), "令牌属于 me") {
		t.Fatalf("err %v", err)
	}
	if _, err := Sync(context.Background(), t.TempDir(), cfg, nil); err == nil || !strings.Contains(err.Error(), "令牌属于 me") {
		t.Fatalf("sync err %v", err)
	}
}

func TestIncluded(t *testing.T) {
	for p, want := range map[string]bool{
		"a.mindmap.json": true, "dir/b.mindmap.json": true, "a.assets/x.png": true,
		".library/clips/1.json": true, ".library/files/x.pdf": true,
		".library/vectors.json": false, ".sync/github.json": false, ".DS_Store": false,
		"dir/.hidden": false, "a.mindmap.json.tmp": false, ".library": false,
		".skills/review/SKILL.md": true, ".skills/_machines/mac.json": true, ".skills/.trash/x-1/SKILL.md": false,
	} {
		if Included(p) != want {
			t.Errorf("Included(%q) = %v", p, !want)
		}
	}
}

func TestLargeFilesSkipped(t *testing.T) {
	f, cfg := setup(t)
	a := t.TempDir()
	put(t, a, "small.mindmap.json", "s")
	big := filepath.Join(a, ".library/files/big.mov")
	os.MkdirAll(filepath.Dir(big), 0o755)
	fh, _ := os.Create(big)
	fh.Truncate(MaxFile + 1)
	fh.Close()
	res := sync1(t, a, cfg)
	if len(res.Skipped) != 1 || f.files()[".library/files/big.mov"] != "" {
		t.Fatalf("%+v", res)
	}
}

func TestClean(t *testing.T) {
	if got := Clean(" github_pat_ab\u200bc\n"); got != "github_pat_abc" {
		t.Fatalf("%q", got)
	}
	if got := Clean("me/my\u00a0notes\ufeff"); got != "me/mynotes" {
		t.Fatalf("%q", got)
	}
}

// The maps' analyses, kept as clips are, take the newer of two edits
// rather than a conflict.
func TestNewerMapAnalysis(t *testing.T) {
	dir := t.TempDir()
	p := ".library/maps/abc.json"
	if err := os.MkdirAll(filepath.Join(dir, ".library", "maps"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(p)), []byte(`{"path":"a.mindmap.json","updated":"2026-10-01T00:00:00Z"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	newer, ok := newerClip(filepath.Join(dir, filepath.FromSlash(p)), p, []byte(`{"path":"a.mindmap.json","updated":"2026-10-02T00:00:00Z"}`))
	if !ok || !newer {
		t.Fatalf("newer %v ok %v", newer, ok)
	}
	// What was drawn of a map's topics, the same.
	p = ".library/diagrams/abc123.json"
	if err := os.MkdirAll(filepath.Join(dir, ".library", "diagrams"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(p)), []byte(`{"path":"a.mindmap.json","updated":"2026-10-03T00:00:00Z","node":"abc123"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if newer, ok := newerClip(filepath.Join(dir, filepath.FromSlash(p)), p, []byte(`{"path":"a.mindmap.json","updated":"2026-10-02T00:00:00Z","node":"abc123"}`)); !ok || newer {
		t.Fatalf("diagram: newer %v ok %v", newer, ok)
	}
	// A set says when it was drawn, not updated: the one drawn later is
	// the newer, and with no time on either none is.
	os.WriteFile(filepath.Join(dir, filepath.FromSlash(p)), []byte(`{"map":"a.mindmap.json","drawn":"2026-10-02T00:00:00Z","node":"abc123"}`), 0o644)
	if newer, ok := newerClip(filepath.Join(dir, filepath.FromSlash(p)), p, []byte(`{"map":"a.mindmap.json","drawn":"2026-10-09T00:00:00Z","node":"abc123"}`)); !ok || !newer {
		t.Fatalf("diagram drawn later: newer %v ok %v", newer, ok)
	}
	os.WriteFile(filepath.Join(dir, filepath.FromSlash(p)), []byte(`{"node":"abc123"}`), 0o644)
	if _, ok := newerClip(filepath.Join(dir, filepath.FromSlash(p)), p, []byte(`{"node":"abc123","x":1}`)); ok {
		t.Fatal("no time on either: one was taken as the newer")
	}
}

func TestMerged(t *testing.T) {
	f, cfg := setup(t)
	var gotBase string
	var when MergeInfo
	cfg.Merge = func(p string, base, local, remote []byte, w MergeInfo) ([]byte, bool) {
		if !strings.HasSuffix(p, ".mindmap.json") {
			return nil, false
		}
		gotBase, when = string(base), w
		return []byte(string(local) + "+" + string(remote)), true
	}
	a, b := t.TempDir(), t.TempDir()
	put(t, a, "计划.mindmap.json", "base")
	put(t, a, "笔记.txt", "base")
	sync1(t, a, cfg)
	sync1(t, b, cfg)

	put(t, a, "计划.mindmap.json", "from a")
	put(t, b, "计划.mindmap.json", "from b!")
	put(t, a, "笔记.txt", "from a")
	put(t, b, "笔记.txt", "from b!")
	sync1(t, b, cfg)
	res := sync1(t, a, cfg)

	if gotBase != "base" {
		t.Errorf("merged against %q, want the version both came from", gotBase)
	}
	if when.RemoteFrom != "Office-Mac" || when.RemoteAt.IsZero() || when.LocalAt.IsZero() || when.BaseAt.IsZero() {
		t.Errorf("when %+v", when)
	}
	if read(a, "计划.mindmap.json") != "from a+from b!" || len(res.Merged) != 1 {
		t.Fatalf("map: %q, %+v", read(a, "计划.mindmap.json"), res)
	}
	if read(a, "计划 (GitHub 冲突 2026-10-11 1504).mindmap.json") != "" {
		t.Error("a merged map was kept apart too")
	}
	// What Merge leaves is kept apart, as before.
	if len(res.Conflicts) != 1 || read(a, "笔记 (GitHub 冲突 2026-10-11 1504).txt") != "from b!" {
		t.Errorf("conflicts %v", res.Conflicts)
	}
	if got := f.files(); got["计划.mindmap.json"] != "from a+from b!" {
		t.Fatalf("repository %v", got)
	}
	sync1(t, b, cfg)
	if read(b, "计划.mindmap.json") != "from a+from b!" {
		t.Fatal("b did not take the merge")
	}
}

func TestHostOf(t *testing.T) {
	for msg, want := range map[string]string{
		"MyMind 同步：3 个文件（MacBook-Pro.local）": "MacBook-Pro",
		"MyMind 同步：1 个文件，删除 2 个（家里的 Mac）":    "家里的 Mac",
		"MyMind 同步：1 个文件":                    "",
		"Fix things (by hand)":               "",
	} {
		if got := hostOf(msg); got != want {
			t.Errorf("hostOf(%q) = %q, want %q", msg, got, want)
		}
	}
}

// A folder outside the maps folder, mounted at a path of the repository
// (~/.agents/skills at .skills), is synced as if it were in it, and lands
// in the other computer's own; what is in the maps folder at that path is
// not synced. A device of before, whose skills were in the maps folder's
// .skills, sees the same repository.
func TestMounts(t *testing.T) {
	f, cfg := setup(t)
	a, b := t.TempDir(), t.TempDir()
	skillsA, skillsB := t.TempDir(), t.TempDir()
	put(t, a, "项目.mindmap.json", `{"v":1}`)
	put(t, skillsA, "review/SKILL.md", "---\nname: review\n---\n")
	put(t, skillsA, "_machines/a.json", `{"name":"a"}`)
	put(t, skillsA, ".trash/old/SKILL.md", "gone")
	put(t, a, ".skills/stale/SKILL.md", "left over")
	ca := cfg
	ca.Mounts = map[string]string{".skills": skillsA}
	if res := sync1(t, a, ca); res.Pushed != 3 {
		t.Fatalf("pushed %d", res.Pushed)
	}
	got := f.files()
	if got[".skills/review/SKILL.md"] == "" || got[".skills/_machines/a.json"] == "" || got[".skills/.trash/old/SKILL.md"] != "" || got[".skills/stale/SKILL.md"] != "" {
		t.Fatalf("repository %v", got)
	}
	cb := cfg
	cb.Mounts = map[string]string{".skills": skillsB}
	sync1(t, b, cb)
	if read(skillsB, "review/SKILL.md") == "" || read(b, ".skills/review/SKILL.md") != "" {
		t.Fatal("not in b's mounted folder")
	}
	// Changed and deleted there, followed here.
	put(t, skillsB, "review/SKILL.md", "---\nname: review\ndescription: better\n---\n")
	os.Remove(filepath.Join(skillsB, "_machines", "a.json"))
	sync1(t, b, cb)
	sync1(t, a, ca)
	if !strings.Contains(read(skillsA, "review/SKILL.md"), "better") || read(skillsA, "_machines/a.json") != "" {
		t.Fatal("a did not follow")
	}
}

var limitReset = time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)

// A sync of many files that GitHub's limit stops goes on, the next time,
// from where it stopped: it began again at the first file, each a call,
// and so used up every hour's calls of the account without ever ending.
func TestLimitStopsAndGoesOn(t *testing.T) {
	f, cfg := setup(t)
	dir := t.TempDir()
	const files = 40
	for i := range files {
		put(t, dir, fmt.Sprintf("m%02d.mindmap.json", i), fmt.Sprint("\xff picture ", i))
	}
	// The calls run out on the way, taken by something else of the account.
	f.limited, f.left, f.drain = true, 5000, 12
	_, err := Sync(context.Background(), dir, cfg, nil)
	var limit *LimitError
	if !errors.As(err, &limit) || limit.Spared || !limit.Reset.Equal(limitReset) {
		t.Fatalf("the limit met: %v", err)
	}
	if at := RetryAfter(err); !at.After(limitReset) || at.After(limitReset.Add(time.Minute)) {
		t.Fatalf("tried again at %v", at)
	}
	sent := f.uploads
	if sent == 0 || sent >= files || f.files()["m00.mindmap.json"] != "" {
		t.Fatalf("%d uploaded before the limit, files in the repository: %v", sent, f.files())
	}
	// Counted anew, the rest goes up, and what went up before is not sent again.
	f.left, f.drain = 5000, 0
	res := sync1(t, dir, cfg)
	if res.Pushed != files || f.files()["m39.mindmap.json"] == "" {
		t.Fatalf("pushed %d, in the repository: %v", res.Pushed, f.files())
	}
	if f.uploads != files {
		t.Fatalf("%d uploads for %d files: %d were sent twice", f.uploads, files, f.uploads-files)
	}
	// Nothing is kept of it once it ended.
	if s := loadState(dir, cfg); len(s.Sent) != 0 {
		t.Fatalf("%d blobs still noted as sent", len(s.Sent))
	}
}

// A sync leaves the account some of its hour's calls: it stops short of
// the last, saying so, and goes on when they are counted anew.
func TestLimitSpared(t *testing.T) {
	f, cfg := setup(t)
	dir := t.TempDir()
	for i := range 30 {
		put(t, dir, fmt.Sprintf("m%02d.mindmap.json", i), fmt.Sprint("\xff picture ", i))
	}
	f.limited, f.left = true, Reserve+12
	_, err := Sync(context.Background(), dir, cfg, nil)
	var limit *LimitError
	if !errors.As(err, &limit) || !limit.Spared {
		t.Fatalf("not spared: %v", err)
	}
	if f.left < Reserve-8 { // those under way as it was seen went through
		t.Fatalf("%d calls left of the %d to spare", f.left, Reserve)
	}
	if f.uploads == 0 || f.files()["m00.mindmap.json"] != "" {
		t.Fatalf("%d uploaded", f.uploads)
	}
	sent := f.uploads
	f.left = 5000
	if res := sync1(t, dir, cfg); res.Pushed != 30 || f.uploads != 30 {
		t.Fatalf("pushed %d with %d uploads, %d before", res.Pushed, f.uploads, sent)
	}
	// A sync with nothing to send is no call but the one that looks.
	before := f.left
	sync1(t, dir, cfg)
	if used := before - f.left; used != 1 {
		t.Fatalf("an idle sync took %d calls", used)
	}
}

// The texts of a sync go up in one call, however many; a file that is no
// text is a call. What the other computer gets is what was written, to
// the byte.
func TestTextsGoUpTogether(t *testing.T) {
	f, cfg := setup(t)
	a, b := t.TempDir(), t.TempDir()
	sync1(t, a, cfg) // the repository begun
	texts := map[string]string{
		"计划.mindmap.json":           "{\n  \"title\": \"计划 <a> & b\"\n}\n",
		".library/clips/1.json":     "line one\r\nline two\r\n",
		".library/clips/2.json":     "\ufeff密码：⟦mm1.4c7a1e.pX0v-8Jw_⟧ \t tab",
		".skills/review/SKILL.md":   "# Review\n\n- a\n- b",
		".skills/review/run.py":     "",
		".library/clips/quotes.txt": `"quoted" \back\slash \u00e9`,
	}
	for i := range 60 {
		texts[fmt.Sprintf(".library/clips/n%02d.json", i)] = fmt.Sprint("clip ", i)
	}
	for p, c := range texts {
		put(t, a, p, c)
	}
	put(t, a, ".library/files/pic.png", "\x89PNG\x00\x01\xff")
	f.limited, f.left = true, 5000
	res := sync1(t, a, cfg)
	if res.Pushed != len(texts)+1 {
		t.Fatalf("pushed %d of %d", res.Pushed, len(texts)+1)
	}
	if f.uploads != 1 {
		t.Fatalf("%d blobs uploaded: the picture alone is one", f.uploads)
	}
	if used := 5000 - f.left; used != 5 { // the head, the picture, the tree, the commit, the branch
		t.Fatalf("%d calls for %d files", used, res.Pushed)
	}
	f.limited = false
	sync1(t, b, cfg)
	for p, c := range texts {
		if got := read(b, p); got != c {
			t.Errorf("%s: %q, written %q", p, got, c)
		}
	}
	if read(b, ".library/files/pic.png") != "\x89PNG\x00\x01\xff" {
		t.Error("the picture")
	}
	// Neither has anything more to send: what GitHub has is what each has.
	f.uploads = 0
	if r := sync1(t, a, cfg); r.Synced() {
		t.Fatalf("a synced again: %+v", r)
	}
	if r := sync1(t, b, cfg); r.Synced() || f.uploads != 0 {
		t.Fatalf("b synced again: %+v", r)
	}
}

// Texts beyond what a call holds go up in a tree upon a tree.
func TestManyTexts(t *testing.T) {
	f, cfg := setup(t)
	a := t.TempDir()
	sync1(t, a, cfg)
	long := strings.Repeat("字", inlineMax/3-10) // near the most a text may be
	for i := range 12 {
		put(t, a, fmt.Sprintf("m%02d.mindmap.json", i), fmt.Sprint(i, long))
	}
	put(t, a, "long.mindmap.json", long+long) // too long a text: a blob
	f.limited, f.left = true, 5000
	if res := sync1(t, a, cfg); res.Pushed != 13 || len(f.files()) < 13 {
		t.Fatalf("pushed %d, %d in the repository", res.Pushed, len(f.files()))
	}
	if used := 5000 - f.left; f.uploads != 1 || used > 7 {
		t.Fatalf("%d uploads, %d calls", f.uploads, used)
	}
}

// What cannot be read is not deleted from the repository, and so not
// from the other computer; a folder that is a link is read where it
// leads.
func TestUnreadableIsNotDeleted(t *testing.T) {
	f, cfg := setup(t)
	a := t.TempDir()
	put(t, a, "项目.mindmap.json", `{"v":1}`)
	put(t, a, "客户/甲.mindmap.json", `{"v":1}`)
	put(t, a, "客户/乙.mindmap.json", `{"v":1}`)
	sync1(t, a, cfg)

	// Through a link to the folder: the same files, nothing to do.
	link := filepath.Join(t.TempDir(), "maps")
	if err := os.Symlink(a, link); err == nil {
		if res := sync1(t, link, cfg); res.Deleted != 0 || f.files()["客户/甲.mindmap.json"] == "" {
			t.Fatalf("through a link: %+v, repository %v", res, f.files())
		}
	}

	if os.Geteuid() == 0 {
		t.Skip("root reads everything")
	}
	os.Chmod(filepath.Join(a, "项目.mindmap.json"), 0)
	os.Chmod(filepath.Join(a, "客户"), 0)
	t.Cleanup(func() { os.Chmod(filepath.Join(a, "客户"), 0o755) })
	if res := sync1(t, a, cfg); res.Deleted != 0 || f.files()["项目.mindmap.json"] == "" || f.files()["客户/乙.mindmap.json"] == "" {
		t.Fatalf("unreadable: %+v, repository %v", res, f.files())
	}

	// The other computer changes one and deletes another meanwhile: not
	// taken while they cannot be read here, and taken once they can, not
	// sent back as they were.
	b := t.TempDir()
	sync1(t, b, cfg)
	put(t, b, "项目.mindmap.json", `{"v":2,"from":"b"}`)
	put(t, b, "客户/甲.mindmap.json", `{"v":2,"from":"b"}`)
	os.Remove(filepath.Join(b, "客户", "乙.mindmap.json"))
	sync1(t, b, cfg)
	sync1(t, a, cfg) // still unreadable
	os.Chmod(filepath.Join(a, "项目.mindmap.json"), 0o644)
	os.Chmod(filepath.Join(a, "客户"), 0o755)
	if read(a, "项目.mindmap.json") != `{"v":1}` {
		t.Fatalf("written while unreadable: %q", read(a, "项目.mindmap.json"))
	}
	res := sync1(t, a, cfg)
	if res.Pushed != 0 || read(a, "项目.mindmap.json") != `{"v":2,"from":"b"}` || read(a, "客户/甲.mindmap.json") != `{"v":2,"from":"b"}` || read(a, "客户/乙.mindmap.json") != "" {
		t.Fatalf("readable again: %+v, 项目 %q, 甲 %q, 乙 %q", res, read(a, "项目.mindmap.json"), read(a, "客户/甲.mindmap.json"), read(a, "客户/乙.mindmap.json"))
	}
	if got := f.files(); got["项目.mindmap.json"] != `{"v":2,"from":"b"}` || got["客户/乙.mindmap.json"] != "" {
		t.Fatalf("the repository was put back: %v", got)
	}
}

// What the sync remembers is kept out of the folder. In it, a backup put
// back brought the memory of when it was made: the files, as they were
// then, read as changed in the repository alone, and the next sync pulled
// over what had been restored.
func TestStateOutOfTheFolder(t *testing.T) {
	f, cfg := setup(t)
	a, b := t.TempDir(), t.TempDir()
	ca, cb := cfg, cfg
	ca.State = filepath.Join(t.TempDir(), "sync", "a.json")
	cb.State = filepath.Join(t.TempDir(), "sync", "b.json")
	put(t, a, "计划.mindmap.json", "v1")
	sync1(t, a, ca)
	if _, err := os.Stat(filepath.Join(a, ".sync")); err == nil {
		t.Fatal("the state is in the folder still")
	}
	if _, err := os.Stat(ca.State); err != nil {
		t.Fatal("no state where it is to be kept")
	}
	// A backup of then, as one of before 0.105.2 holds it: the files and
	// the folder's own state.
	old, _ := os.ReadFile(ca.State)
	sync1(t, b, cb)
	put(t, b, "计划.mindmap.json", "v2, which went wrong")
	sync1(t, b, cb)
	sync1(t, a, ca)
	if read(a, "计划.mindmap.json") != "v2, which went wrong" {
		t.Fatal("a did not follow")
	}
	// The backup is put back.
	put(t, a, "计划.mindmap.json", "v1")
	os.MkdirAll(filepath.Join(a, ".sync"), 0o755)
	os.WriteFile(filepath.Join(a, ".sync", "github.json"), old, 0o644)
	res := sync1(t, a, ca)
	if read(a, "计划.mindmap.json") != "v1" || res.Pulled != 0 || f.files()["计划.mindmap.json"] != "v1" {
		t.Fatalf("what was restored was pulled over: %q, %+v, repository %q", read(a, "计划.mindmap.json"), res, f.files()["计划.mindmap.json"])
	}
}

// The state a sync of before kept in the folder is moved out of it, once,
// and the sync goes on from it: nothing is merged anew.
func TestAdopt(t *testing.T) {
	f, cfg := setup(t)
	a := t.TempDir()
	put(t, a, "计划.mindmap.json", "v1")
	put(t, a, "笔记.txt", "n1")
	sync1(t, a, cfg) // as before: the state in the folder
	f.write("计划.mindmap.json", "v2")
	os.Remove(filepath.Join(a, "笔记.txt"))

	state := filepath.Join(t.TempDir(), "sync", "a.json")
	if !Adopt(a, state) {
		t.Fatal("not adopted")
	}
	if _, err := os.Stat(filepath.Join(a, ".sync")); err == nil {
		t.Fatal("the folder's state is there still")
	}
	if Adopt(a, state) {
		t.Fatal("adopted twice")
	}
	cfg.State = state
	// With what was remembered, the change there is pulled and the file
	// deleted here is deleted there; with nothing remembered, the two
	// versions would be a conflict and the file deleted would come back.
	res := sync1(t, a, cfg)
	if read(a, "计划.mindmap.json") != "v2" || len(res.Conflicts) != 0 || f.files()["笔记.txt"] != "" || read(a, "笔记.txt") != "" {
		t.Fatalf("%+v: %q, repository %v", res, read(a, "计划.mindmap.json"), f.files())
	}
	// One there already is not replaced.
	os.MkdirAll(filepath.Join(a, ".sync"), 0o755)
	os.WriteFile(filepath.Join(a, ".sync", "github.json"), []byte(`{"repo":"other/one"}`), 0o644)
	if Adopt(a, state) {
		t.Fatal("the state kept was replaced by one found in the folder")
	}
}

// things are files of the form "id=N;words", one thing by its id, merged
// by joining their words: maps, for the sync's tests.
func things(cfg Config) Config {
	id := func(data []byte) string { s, _, _ := strings.Cut(string(data), ";"); return s }
	words := func(data []byte) string { _, s, _ := strings.Cut(string(data), ";"); return s }
	cfg.Same = func(from, to string, old, renamed []byte) bool {
		return strings.HasSuffix(from, ".mindmap.json") && strings.HasSuffix(to, ".mindmap.json") && id(old) == id(renamed)
	}
	cfg.Merge = func(p string, base, local, remote []byte, _ MergeInfo) ([]byte, bool) {
		if !strings.HasSuffix(p, ".mindmap.json") {
			return nil, false
		}
		return []byte(id(local) + ";" + words(local) + "+" + words(remote) + "(" + words(base) + ")"), true
	}
	return cfg
}

// A map renamed on one computer and changed on the other is one map, of
// the new name, with both changes: the old name came back with the other
// computer's changes, and the map was two.
func TestRenamedHereChangedThere(t *testing.T) {
	f, cfg := setup(t)
	cfg = things(cfg)
	a, b := t.TempDir(), t.TempDir()
	put(t, a, "计划.mindmap.json", "id=1;base")
	put(t, a, "别的.mindmap.json", "id=2;other")
	sync1(t, a, cfg)
	sync1(t, b, cfg)

	// b changes it; a renames it, its central topic changed.
	put(t, b, "计划.mindmap.json", "id=1;from b")
	sync1(t, b, cfg)
	os.Remove(filepath.Join(a, "计划.mindmap.json"))
	put(t, a, "规划.mindmap.json", "id=1;renamed on a")
	put(t, a, "新的.mindmap.json", "id=3;new")
	res := sync1(t, a, cfg)

	const want = "id=1;renamed on a+from b(base)"
	if read(a, "计划.mindmap.json") != "" || read(a, "规划.mindmap.json") != want {
		t.Fatalf("on a: 计划 %q, 规划 %q", read(a, "计划.mindmap.json"), read(a, "规划.mindmap.json"))
	}
	if got := f.files(); got["计划.mindmap.json"] != "" || got["规划.mindmap.json"] != want || got["新的.mindmap.json"] != "id=3;new" {
		t.Fatalf("repository %v", got)
	}
	if res.Renamed["计划.mindmap.json"] != "规划.mindmap.json" || !slices.Contains(res.Merged, "规划.mindmap.json") {
		t.Fatalf("%+v", res)
	}
	sync1(t, b, cfg)
	if read(b, "计划.mindmap.json") != "" || read(b, "规划.mindmap.json") != want {
		t.Fatalf("on b: 计划 %q, 规划 %q", read(b, "计划.mindmap.json"), read(b, "规划.mindmap.json"))
	}
	if r := sync1(t, a, cfg); r.Synced() {
		t.Fatalf("a synced again: %+v", r)
	}

	// A file deleted here and changed there, with a new file that is
	// another thing, is kept as before.
	put(t, b, "别的.mindmap.json", "id=2;changed on b")
	sync1(t, b, cfg)
	os.Remove(filepath.Join(a, "别的.mindmap.json"))
	put(t, a, "无关.mindmap.json", "id=9;nothing of it")
	sync1(t, a, cfg)
	if read(a, "别的.mindmap.json") != "id=2;changed on b" || read(a, "无关.mindmap.json") != "id=9;nothing of it" {
		t.Fatalf("a deletion beat an edit: %q", read(a, "别的.mindmap.json"))
	}
}

// The same from the other side: renamed there, changed here.
func TestRenamedThereChangedHere(t *testing.T) {
	f, cfg := setup(t)
	cfg = things(cfg)
	a, b := t.TempDir(), t.TempDir()
	put(t, a, "客户/计划.mindmap.json", "id=1;base")
	sync1(t, a, cfg)
	sync1(t, b, cfg)

	os.Remove(filepath.Join(b, "客户", "计划.mindmap.json"))
	put(t, b, "客户/规划.mindmap.json", "id=1;renamed on b")
	sync1(t, b, cfg)
	put(t, a, "客户/计划.mindmap.json", "id=1;changed on a")
	res := sync1(t, a, cfg)

	const want = "id=1;changed on a+renamed on b(base)"
	if read(a, "客户/计划.mindmap.json") != "" || read(a, "客户/规划.mindmap.json") != want {
		t.Fatalf("on a: 计划 %q, 规划 %q", read(a, "客户/计划.mindmap.json"), read(a, "客户/规划.mindmap.json"))
	}
	if got := f.files(); got["客户/计划.mindmap.json"] != "" || got["客户/规划.mindmap.json"] != want {
		t.Fatalf("repository %v", got)
	}
	if res.Renamed["客户/计划.mindmap.json"] != "客户/规划.mindmap.json" {
		t.Fatalf("%+v", res)
	}
	sync1(t, b, cfg)
	if read(b, "客户/规划.mindmap.json") != want || read(b, "客户/计划.mindmap.json") != "" {
		t.Fatalf("on b: %q", read(b, "客户/规划.mindmap.json"))
	}
	if r := sync1(t, a, cfg); r.Synced() {
		t.Fatalf("a synced again: %+v", r)
	}
}

// Renamed there alone, unchanged here: the files are the repository's,
// and the rename is said, so that the app follows the map it has open.
func TestRenamedThereOnly(t *testing.T) {
	f, cfg := setup(t)
	cfg = things(cfg)
	a, b := t.TempDir(), t.TempDir()
	put(t, a, "客户/计划.mindmap.json", "id=1;base")
	put(t, a, "别的.mindmap.json", "id=2;other")
	sync1(t, a, cfg)
	sync1(t, b, cfg)

	os.Remove(filepath.Join(b, "客户", "计划.mindmap.json"))
	put(t, b, "客户/规划.mindmap.json", "id=1;renamed on b")
	os.Remove(filepath.Join(b, "别的.mindmap.json")) // deleted, and nothing new of it
	put(t, b, "新的.mindmap.json", "id=3;new")
	sync1(t, b, cfg)
	res := sync1(t, a, cfg)

	if read(a, "客户/计划.mindmap.json") != "" || read(a, "客户/规划.mindmap.json") != "id=1;renamed on b" || read(a, "别的.mindmap.json") != "" {
		t.Fatalf("on a: %q, %q", read(a, "客户/计划.mindmap.json"), read(a, "客户/规划.mindmap.json"))
	}
	if len(res.Renamed) != 1 || res.Renamed["客户/计划.mindmap.json"] != "客户/规划.mindmap.json" || len(res.Merged) != 0 {
		t.Fatalf("%+v", res)
	}
	if got := f.files(); got["客户/规划.mindmap.json"] != "id=1;renamed on b" || got["客户/计划.mindmap.json"] != "" {
		t.Fatalf("repository %v", got)
	}
	if r := sync1(t, a, cfg); r.Synced() || len(r.Renamed) != 0 {
		t.Fatalf("a synced again: %+v", r)
	}
}

// The version both had, gone from the repository (made anew, its history
// rewritten), is none: the two are merged without it. It failed every
// sync from then on, saying the repository's name was mistyped.
func TestBaseGone(t *testing.T) {
	f, cfg := setup(t)
	cfg = things(cfg)
	a, b := t.TempDir(), t.TempDir()
	put(t, a, "计划.mindmap.json", "id=1;base")
	sync1(t, a, cfg)
	sync1(t, b, cfg)
	put(t, a, "计划.mindmap.json", "id=1;from a")
	put(t, b, "计划.mindmap.json", "id=1;from b")
	sync1(t, b, cfg)
	f.mu.Lock()
	delete(f.blobs, blobSHA([]byte("id=1;base")))
	f.mu.Unlock()
	res, err := Sync(context.Background(), a, cfg, nil)
	if err != nil {
		t.Fatalf("the sync failed: %v", err)
	}
	if read(a, "计划.mindmap.json") != "id=1;from a+from b()" || len(res.Merged) != 1 {
		t.Fatalf("%q, %+v", read(a, "计划.mindmap.json"), res)
	}
}

// Not found is explained, as a repository mistyped or a token that may
// not reach it, of the repository alone: not of a file of it.
func TestNotFoundOfAFileIsNotExplained(t *testing.T) {
	f, cfg := setup(t)
	a := t.TempDir()
	sync1(t, a, cfg)
	f.write("计划.mindmap.json", "v1")
	f.mu.Lock()
	delete(f.blobs, blobSHA([]byte("v1")))
	f.mu.Unlock()
	_, err := Sync(context.Background(), a, cfg, nil)
	if err == nil || strings.Contains(err.Error(), "令牌") || strings.Contains(err.Error(), "重新输入") {
		t.Fatalf("err %v", err)
	}
}

// A map edited while it is merged is left as it is for the next sync,
// which merges it against what both had. It was kept as a conflict copy,
// and that merged with no version both came from.
func TestEditedWhileMerging(t *testing.T) {
	f, cfg := setup(t)
	cfg = things(cfg)
	a, b := t.TempDir(), t.TempDir()
	put(t, a, "计划.mindmap.json", "id=1;base")
	sync1(t, a, cfg)
	sync1(t, b, cfg)
	put(t, a, "计划.mindmap.json", "id=1;from a")
	put(t, b, "计划.mindmap.json", "id=1;from b")
	sync1(t, b, cfg)

	merge := cfg.Merge
	edited := cfg
	edited.Merge = func(p string, base, local, remote []byte, w MergeInfo) ([]byte, bool) {
		put(t, a, "计划.mindmap.json", "id=1;from a, and more") // the user goes on
		return merge(p, base, local, remote, w)
	}
	res := sync1(t, a, edited)
	if len(res.Conflicts) != 0 || len(res.Merged) != 0 || read(a, "计划 (GitHub 冲突 2026-10-11 1504).mindmap.json") != "" {
		t.Fatalf("kept apart: %+v", res)
	}
	if read(a, "计划.mindmap.json") != "id=1;from a, and more" || f.files()["计划.mindmap.json"] != "id=1;from b" {
		t.Fatalf("here %q, there %q", read(a, "计划.mindmap.json"), f.files()["计划.mindmap.json"])
	}
	// The next sync merges it, against what both had.
	res = sync1(t, a, cfg)
	if want := "id=1;from a, and more+from b(base)"; read(a, "计划.mindmap.json") != want || f.files()["计划.mindmap.json"] != want || len(res.Merged) != 1 {
		t.Fatalf("%q, %+v", read(a, "计划.mindmap.json"), res)
	}
}

// A file that may be run is one on the other computer too, and stays one
// when it comes back changed; one the repository has as a text, from a
// sync of before, is mended.
func TestExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no such bit")
	}
	mode := func(dir, p string) os.FileMode {
		info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(p)))
		if err != nil {
			t.Fatal(err)
		}
		return info.Mode().Perm()
	}
	f, cfg := setup(t)
	a, b := t.TempDir(), t.TempDir()
	put(t, a, ".skills/review/run.sh", "#!/bin/sh\necho 1\n")
	put(t, a, ".skills/review/SKILL.md", "# Review")
	put(t, a, ".skills/review/tool.bin", "\x7fELF\x00\x01")
	os.Chmod(filepath.Join(a, ".skills/review/run.sh"), 0o755)
	os.Chmod(filepath.Join(a, ".skills/review/tool.bin"), 0o755)
	sync1(t, a, cfg)
	if f.modeOf(".skills/review/run.sh") != "100755" || f.modeOf(".skills/review/tool.bin") != "100755" || f.modeOf(".skills/review/SKILL.md") != "100644" {
		t.Fatalf("modes there: %s %s %s", f.modeOf(".skills/review/run.sh"), f.modeOf(".skills/review/tool.bin"), f.modeOf(".skills/review/SKILL.md"))
	}
	sync1(t, b, cfg)
	if mode(b, ".skills/review/run.sh") != 0o755 || mode(b, ".skills/review/tool.bin") != 0o755 || mode(b, ".skills/review/SKILL.md") != 0o644 {
		t.Fatalf("modes on b: %v %v %v", mode(b, ".skills/review/run.sh"), mode(b, ".skills/review/tool.bin"), mode(b, ".skills/review/SKILL.md"))
	}
	// Changed there and back: run still, here and there.
	put(t, b, ".skills/review/run.sh", "#!/bin/sh\necho 2\n")
	os.Chmod(filepath.Join(b, ".skills/review/run.sh"), 0o755)
	sync1(t, b, cfg)
	sync1(t, a, cfg)
	if read(a, ".skills/review/run.sh") != "#!/bin/sh\necho 2\n" || mode(a, ".skills/review/run.sh") != 0o755 || f.modeOf(".skills/review/run.sh") != "100755" {
		t.Fatalf("after a change: %v, %s", mode(a, ".skills/review/run.sh"), f.modeOf(".skills/review/run.sh"))
	}
	// A file's own mode is kept when it is written over.
	os.Chmod(filepath.Join(a, ".skills/review/SKILL.md"), 0o600)
	put(t, b, ".skills/review/SKILL.md", "# Review, better")
	sync1(t, b, cfg)
	sync1(t, a, cfg)
	if read(a, ".skills/review/SKILL.md") != "# Review, better" || mode(a, ".skills/review/SKILL.md") != 0o600 {
		t.Fatalf("written over: %v", mode(a, ".skills/review/SKILL.md"))
	}

	// One that went up as a text before: its mode goes up alone, no blob
	// sent, and the other computer's file may be run.
	f.write(".skills/old/go.sh", "#!/bin/sh\n")
	sync1(t, a, cfg)
	sync1(t, b, cfg)
	if mode(b, ".skills/old/go.sh") != 0o644 {
		t.Fatalf("a text there: %v", mode(b, ".skills/old/go.sh"))
	}
	os.Chmod(filepath.Join(a, ".skills/old/go.sh"), 0o755)
	f.uploads = 0
	res := sync1(t, a, cfg)
	if f.modeOf(".skills/old/go.sh") != "100755" || f.uploads != 0 || res.Pushed != 0 {
		t.Fatalf("the mode alone: %s, %d uploads, %+v", f.modeOf(".skills/old/go.sh"), f.uploads, res)
	}
	sync1(t, b, cfg)
	if mode(b, ".skills/old/go.sh") != 0o755 {
		t.Fatalf("on b: %v", mode(b, ".skills/old/go.sh"))
	}
	for _, d := range []string{a, b} {
		if r := sync1(t, d, cfg); r.Synced() || r.Commit != f.head {
			t.Fatalf("synced again: %+v", r)
		}
	}
	pushes := f.pushes
	sync1(t, a, cfg)
	sync1(t, b, cfg)
	if f.pushes != pushes {
		t.Fatal("the modes go up again and again")
	}
}

// One path that cannot be written here does not stop the rest: it is
// left on both sides, said, and written once it can be.
func TestUnwritableDoesNotBlock(t *testing.T) {
	f, cfg := setup(t)
	a := t.TempDir()
	put(t, a, "计划.mindmap.json", "v1")
	sync1(t, a, cfg)
	// A name longer than a file's may be, whoever runs this.
	long := strings.Repeat("长", 100) + ".mindmap.json"
	f.write(long, "from elsewhere")
	f.write("别的.mindmap.json", "also from elsewhere")
	put(t, a, "计划.mindmap.json", "v2")

	res, err := Sync(context.Background(), a, cfg, nil)
	if err != nil {
		t.Fatalf("one path stopped the sync: %v", err)
	}
	if !slices.Contains(res.Unwritten, long) || read(a, "别的.mindmap.json") != "also from elsewhere" || res.Pulled != 1 {
		t.Fatalf("%+v", res)
	}
	if got := f.files(); got["计划.mindmap.json"] != "v2" || got[long] != "from elsewhere" {
		t.Fatalf("repository %v", got)
	}
	// The next time: still there, still said, nothing lost, and deleted
	// there it is no more to do.
	res = sync1(t, a, cfg)
	if !slices.Contains(res.Unwritten, long) || f.files()[long] != "from elsewhere" {
		t.Fatalf("the second time: %+v", res)
	}
	f.write(long, "")
	if res = sync1(t, a, cfg); len(res.Unwritten) != 0 {
		t.Fatalf("gone from the repository: %+v", res)
	}

	// A folder that may not be written in: its file comes once it may,
	// and one to be removed from it is not taken as new here meanwhile.
	if os.Geteuid() == 0 {
		t.Skip("root writes everywhere")
	}
	put(t, a, "客户/乙.mindmap.json", "v1")
	sync1(t, a, cfg)
	f.write("客户/甲.mindmap.json", "from elsewhere")
	f.write("客户/乙.mindmap.json", "")
	os.Chmod(filepath.Join(a, "客户"), 0o555)
	t.Cleanup(func() { os.Chmod(filepath.Join(a, "客户"), 0o755) })
	res = sync1(t, a, cfg)
	if len(res.Unwritten) != 2 || read(a, "客户/乙.mindmap.json") != "v1" {
		t.Fatalf("a folder not to be written in: %+v", res)
	}
	res = sync1(t, a, cfg)
	if got := f.files(); got["客户/乙.mindmap.json"] != "" || got["客户/甲.mindmap.json"] != "from elsewhere" || res.Pushed != 0 {
		t.Fatalf("what could not be removed was sent back: %+v, %v", res, got)
	}
	os.Chmod(filepath.Join(a, "客户"), 0o755)
	res = sync1(t, a, cfg)
	if len(res.Unwritten) != 0 || read(a, "客户/甲.mindmap.json") != "from elsewhere" || read(a, "客户/乙.mindmap.json") != "" {
		t.Fatalf("once it may be written in: %+v", res)
	}
}

func TestHoldable(t *testing.T) {
	for p, want := range map[string]bool{
		"计划.mindmap.json": true, "客户/甲.mindmap.json": true, ".library/files/a b.png": true,
		"什么?.mindmap.json": false, "说明./x.json": false, "a/结尾空格 /x": false, "CON.mindmap.json": false,
		"x/nul": false, "a<b>.txt": false, "com1.assets/p.png": false, "console.json": true, "a|b": false, `q"uote`: false,
	} {
		if got := holdableOn("windows", p); got != want {
			t.Errorf("holdableOn(windows, %q) = %v", p, got)
		}
		if !holdableOn("darwin", p) {
			t.Errorf("holdableOn(darwin, %q) = false", p)
		}
	}
}

// A file that takes long to come down comes down, while something of it
// comes: no time is set for the whole. A call with nothing coming is
// given up, and said in words.
func TestSlowDownload(t *testing.T) {
	was := idle
	idle = 150 * time.Millisecond
	t.Cleanup(func() { idle = was })
	stall := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := `{"encoding":"utf-8","content":"` + strings.Repeat("x", 6000) + `"}`
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		w.WriteHeader(200)
		for i := 0; i < len(body); i += 1000 { // 1.5 times as long as may pass with nothing coming
			if stall && i == 2000 {
				select {
				case <-r.Context().Done():
				case <-time.After(2 * time.Second):
				}
				return
			}
			w.Write([]byte(body[i:min(i+1000, len(body))]))
			w.(http.Flusher).Flush()
			time.Sleep(40 * time.Millisecond)
		}
	}))
	t.Cleanup(srv.Close)
	cfg := Config{Repo: "me/notes", Token: "good", API: srv.URL}
	data, err := cfg.blob(context.Background(), "abc")
	if err != nil || len(data) != 6000 {
		t.Fatalf("a slow download: %d bytes, %v", len(data), err)
	}
	stall = true
	start := time.Now()
	_, err = cfg.blob(context.Background(), "abc")
	if err == nil || !strings.Contains(err.Error(), "没有动静") || time.Since(start) > time.Second {
		t.Fatalf("a stalled download: %v after %v", err, time.Since(start))
	}
}

// A folder the sync emptied goes: renamed or deleted on one computer, it
// stayed on the other, empty, among the projects. One empty before stays.
func TestEmptiedFoldersGo(t *testing.T) {
	_, cfg := setup(t)
	a, b := t.TempDir(), t.TempDir()
	put(t, a, "客户/甲/计划.mindmap.json", "v1")
	put(t, a, "客户/甲/计划.assets/p.png", "\x89PNG\x00")
	put(t, a, "留着/别的.mindmap.json", "v1")
	sync1(t, a, cfg)
	sync1(t, b, cfg)
	os.Mkdir(filepath.Join(b, "空的"), 0o755)
	os.WriteFile(filepath.Join(b, "客户", "甲", ".DS_Store"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(b, "留着", "自己的.txt.tmp"), []byte("x"), 0o644) // not synced, and the user's

	os.Rename(filepath.Join(a, "客户"), filepath.Join(a, "老客户"))
	os.Remove(filepath.Join(a, "留着", "别的.mindmap.json"))
	sync1(t, a, cfg)
	sync1(t, b, cfg)
	if read(b, "老客户/甲/计划.mindmap.json") != "v1" {
		t.Fatal("b did not follow")
	}
	if _, err := os.Stat(filepath.Join(b, "客户")); err == nil {
		t.Error("the folder renamed is there still, empty")
	}
	if _, err := os.Stat(filepath.Join(b, "空的")); err != nil {
		t.Error("a folder that was empty before is gone")
	}
	if _, err := os.Stat(filepath.Join(b, "留着", "自己的.txt.tmp")); err != nil {
		t.Error("a folder with a file of the user's in it is gone")
	}
}

// A file renamed is not sent again: the repository has it.
func TestRenameUploadsNothing(t *testing.T) {
	f, cfg := setup(t)
	a, b := t.TempDir(), t.TempDir()
	put(t, a, "计划.mindmap.json", "v1")
	for i := range 5 {
		put(t, a, fmt.Sprintf("计划.assets/p%d.png", i), fmt.Sprint("\x89PNG\x00", i))
	}
	sync1(t, a, cfg)
	if f.uploads != 5 {
		t.Fatalf("%d uploads", f.uploads)
	}
	os.Rename(filepath.Join(a, "计划.assets"), filepath.Join(a, "规划.assets"))
	os.Rename(filepath.Join(a, "计划.mindmap.json"), filepath.Join(a, "规划.mindmap.json"))
	res := sync1(t, a, cfg)
	if f.uploads != 5 || res.Pushed != 6 {
		t.Fatalf("%d pictures sent again, %+v", f.uploads-5, res)
	}
	sync1(t, b, cfg)
	if read(b, "规划.assets/p3.png") != "\x89PNG\x003" || read(b, "规划.mindmap.json") != "v1" {
		t.Fatal("b did not get them")
	}
}

// A sync that fails keeps what it worked out and tells what it wrote: the
// hashes of a large folder were made anew two minutes later, and the app
// did not read the files that had come.
func TestFailedSyncKeepsItsWork(t *testing.T) {
	f, cfg := setup(t)
	a := t.TempDir()
	sync1(t, a, cfg)
	put(t, a, "这里的.mindmap.json", "here")
	f.write("那边的.mindmap.json", "there")
	f.write("坏的.mindmap.json", "broken")
	f.mu.Lock()
	delete(f.blobs, blobSHA([]byte("broken")))
	f.mu.Unlock()
	res, err := Sync(context.Background(), a, cfg, nil)
	if err == nil {
		t.Fatal("no error")
	}
	if !slices.Contains(res.Written, "那边的.mindmap.json") || read(a, "那边的.mindmap.json") != "there" {
		t.Fatalf("what was written is not told: %+v", res)
	}
	if s := loadState(a, cfg); s.Hashes["这里的.mindmap.json"].SHA != blobSHA([]byte("here")) {
		t.Fatalf("the hashes are lost: %v", s.Hashes)
	}
}

// A map merged, then merged again as the branch moved, is told once.
func TestMergedToldOnce(t *testing.T) {
	f, cfg := setup(t)
	cfg = things(cfg)
	a, b := t.TempDir(), t.TempDir()
	put(t, a, "计划.mindmap.json", "id=1;base")
	sync1(t, a, cfg)
	sync1(t, b, cfg)
	put(t, a, "计划.mindmap.json", "id=1;from a")
	put(t, b, "计划.mindmap.json", "id=1;from b")
	sync1(t, b, cfg)
	once := false
	cfg.Client = &http.Client{Transport: hook{func(r *http.Request) {
		if !once && r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/git/commits") {
			once = true
			f.write("计划.mindmap.json", "id=1;from b again")
		}
	}, f}}
	res := sync1(t, a, cfg)
	if !once || len(res.Merged) != 1 {
		t.Fatalf("merged %v", res.Merged)
	}
	if strings.ContainsAny(errMoved.Error(), "abcdefghijklmnopqrstuvwxyz") {
		t.Errorf("not in the user's language: %v", errMoved)
	}
}

// A folder with none of the files it had at the last sync, by name or by
// content, is not that folder emptied: nothing is deleted from the
// repository for it. A folder renamed inside it is the folder still.
func TestStrangeFolderDeletesNothing(t *testing.T) {
	f, cfg := setup(t)
	a := t.TempDir()
	cfg.State = filepath.Join(t.TempDir(), "state.json")
	put(t, a, "项目.mindmap.json", `{"v":1}`)
	put(t, a, "客户/甲.mindmap.json", `{"v":2}`)
	put(t, a, "客户/乙.mindmap.json", `{"v":3}`)
	sync1(t, a, cfg)

	// Another folder at its path, with the guide alone.
	b := t.TempDir()
	put(t, b, "使用指南.mindmap.json", `{"guide":true}`)
	res := sync1(t, b, cfg)
	if got := f.files(); res.Deleted != 0 || got["项目.mindmap.json"] == "" || got["客户/乙.mindmap.json"] == "" || read(b, "项目.mindmap.json") == "" {
		t.Fatalf("a strange folder: %+v, repository %v", res, got)
	}
}
