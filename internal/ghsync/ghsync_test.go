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
	return &fakeGitHub{blobs: map[string][]byte{}, trees: map[string]map[string]string{}, commits: map[string][2]string{}}
}

func hashOf(s string) string { h := sha1.Sum([]byte(s)); return hex.EncodeToString(h[:]) }

func (f *fakeGitHub) putTree(files map[string]string) string {
	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k + "=" + files[k] + ";")
	}
	sha := hashOf("tree " + b.String())
	f.trees[sha] = files
	return sha
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
	files := map[string]string{}
	if f.head != "" {
		for k, v := range f.trees[f.commits[f.head][0]] {
			files[k] = v
		}
	}
	if content == "" {
		delete(files, p)
	} else {
		sha := blobSHA([]byte(content))
		f.blobs[sha] = []byte(content)
		files[p] = sha
	}
	f.head = f.putCommit(f.putTree(files), f.head)
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
		for path, sha := range f.trees[strings.TrimPrefix(p, "/git/trees/")] {
			entries = append(entries, map[string]string{"path": path, "type": "blob", "sha": sha})
		}
		reply(200, map[string]any{"tree": entries})
	case strings.HasPrefix(p, "/git/blobs/") && r.Method == "GET":
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
		files := map[string]string{}
		for k, v := range f.trees[in["base_tree"].(string)] {
			files[k] = v
		}
		for _, e := range in["tree"].([]any) {
			e := e.(map[string]any)
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
			}
		}
		reply(201, map[string]string{"sha": f.putTree(files)})
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
