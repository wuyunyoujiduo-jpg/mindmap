package ghsync

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
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
			if sha, ok := e["sha"].(string); ok {
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
	newer, ok := newerClip(dir, p, []byte(`{"path":"a.mindmap.json","updated":"2026-10-02T00:00:00Z"}`))
	if !ok || !newer {
		t.Fatalf("newer %v ok %v", newer, ok)
	}
}
