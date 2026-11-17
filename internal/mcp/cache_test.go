package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// A server is called many times in a session. The maps it looks at are
// read once and again only when their files changed, by another's hand
// or its own; what finds clips by meaning is set up at the first search
// of the library, not before; and the library is opened once while its
// files stay as they are.
func TestServerKeepsWhatItRead(t *testing.T) {
	dir, code := t.TempDir(), repo(t, "shop")
	for i := range 6 {
		f := model.NewFile(fmt.Sprintf("别的项目 %d", i))
		d := f.Sheets[0]
		n, _ := d.AddChild(d.Root.ID, fmt.Sprintf("任务 %d", i))
		n.Task = &model.Task{Status: model.StatusTodo}
		f.Project = filepath.Join(t.TempDir(), fmt.Sprint(i))
		if err := f.Save(filepath.Join(dir, model.FileName(fmt.Sprintf("别的项目 %d", i)))); err != nil {
			t.Fatal(err)
		}
	}
	mine := model.NewFile("这个项目")
	mine.Project = code
	d := mine.Sheets[0]
	login, _ := d.AddChild(d.Root.ID, "登录")
	login.Task = &model.Task{Status: model.StatusTodo}
	path := filepath.Join(dir, model.FileName("这个项目"))
	if err := mine.Save(path); err != nil {
		t.Fatal(err)
	}
	loads := 0
	s := &Server{Dir: dir, Now: time.Now, Cwd: code, LoadEmbedder: func() library.Embedder {
		loads++
		return library.Embedder{}
	}}
	ss := &session{t: t, s: s}
	ss.do("initialize", map[string]any{"protocolVersion": "2025-06-18"})
	ss.do("tools/list", nil)

	// The maps as kept are the ones looked at again: the same in memory.
	keptOf := func() map[string]*model.File {
		out := map[string]*model.File{}
		s.mu.Lock()
		defer s.mu.Unlock()
		for p, k := range s.maps {
			out[p] = k.f
		}
		return out
	}
	if tasks := ss.tool("list_tasks", map[string]any{"status": "open"}, false); !strings.Contains(tasks, "登录") || !strings.Contains(tasks, "任务 5") {
		t.Fatalf("tasks %s", tasks)
	}
	first := keptOf()
	if len(first) != 7 {
		t.Fatalf("%d maps kept after a call that read them all", len(first))
	}
	// Calls that name no map look for the project's among them all.
	if text := ss.tool("project_brief", nil, false); !strings.Contains(text, "登录") {
		t.Fatalf("brief %s", text)
	}
	ss.tool("read_map", nil, false)
	ss.tool("list_tasks", map[string]any{"status": "open"}, false)
	ss.tool("search", map[string]any{"query": "任务"}, false)
	ss.tool("list_maps", nil, false)
	for p, f := range keptOf() {
		if first[p] != f {
			t.Fatalf("%s read again with its file as it was", filepath.Base(p))
		}
	}

	// A map the server writes is read again, the others not; and what a
	// call changes is never the map as kept.
	ss.tool("update_node", map[string]any{"id": login.ID, "status": "done", "summary": "登录做完了"}, false)
	if first[path].Sheets[0].Root.Children[0].Task.Status != model.StatusTodo {
		t.Fatal("a call changed the map as kept")
	}
	if tasks := ss.tool("list_tasks", map[string]any{"status": "open"}, false); strings.Contains(tasks, "登录") || !strings.Contains(tasks, "任务 3") {
		t.Fatalf("the map as it was before its change: %s", tasks)
	}
	now := keptOf()
	if now[path] == first[path] {
		t.Fatal("a map written not read again")
	}
	for p, f := range now {
		if p != path && first[p] != f {
			t.Fatalf("%s read again", filepath.Base(p))
		}
	}
	// Changed by another's hand, the app's: read again.
	other, err := model.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	od := other.Sheets[0]
	od.AddChild(od.Root.ID, "应用里新加的主题")
	if err := other.Save(path); err != nil {
		t.Fatal(err)
	}
	if found := ss.tool("search", map[string]any{"query": "应用里新加的"}, false); !strings.Contains(found, "应用里新加的主题") {
		t.Fatalf("a change of another's not seen: %s", found)
	}
	// Gone: no longer kept, nor found.
	gone := filepath.Join(dir, model.FileName("别的项目 0"))
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	if maps := ss.tool("list_maps", nil, false); strings.Contains(maps, "别的项目 0") || !strings.Contains(maps, "别的项目 1") {
		t.Fatalf("maps %s", maps)
	}

	// The library: not a model read until it is searched, and then once.
	if loads != 0 {
		t.Fatalf("the embedder set up %d times before any search of the library", loads)
	}
	ss.tool("add_clip", map[string]any{"text": "发布前先跑一遍回归测试", "title": "发布清单"}, false)
	if loads != 0 {
		t.Fatal("the embedder set up to keep a clip")
	}
	if found := ss.tool("search_library", map[string]any{"query": "回归测试"}, false); !strings.Contains(found, "发布清单") {
		t.Fatalf("search %s", found)
	}
	lib := s.lib
	if lib == nil || loads != 1 {
		t.Fatalf("after a search: library kept %v, embedder set up %d times", lib != nil, loads)
	}
	ss.tool("search_library", map[string]any{"query": "发布"}, false)
	id := lib.Clips[0].ID
	if text := ss.tool("read_clip", map[string]any{"id": id}, false); !strings.Contains(text, "回归测试") {
		t.Fatalf("clip %s", text)
	}
	if s.lib != lib || loads != 1 {
		t.Fatalf("the library opened again with its files as they were (embedder set up %d times)", loads)
	}
	// A clip kept since, by the server or by the app: opened again.
	ss.tool("add_clip", map[string]any{"text": "灰度发布先放百分之五", "title": "灰度"}, false)
	if found := ss.tool("search_library", map[string]any{"query": "灰度"}, false); !strings.Contains(found, "灰度") || s.lib == lib {
		t.Fatalf("a clip kept since not found: %s", found)
	}
	lib = s.lib
	app, err := library.Open(library.In(dir))
	if err != nil {
		t.Fatal(err)
	}
	c := app.Find(id)
	c.Text = "发布前先跑一遍回归测试，再看监控"
	if err := app.Save(c, time.Now()); err != nil {
		t.Fatal(err)
	}
	if text := ss.tool("read_clip", map[string]any{"id": id}, false); !strings.Contains(text, "再看监控") || s.lib == lib {
		t.Fatalf("a clip changed in the app not read again: %s", text)
	}
}
