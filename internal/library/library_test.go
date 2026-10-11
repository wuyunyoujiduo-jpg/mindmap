package library

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)

func TestTokens(t *testing.T) {
	got := Tokens("Go 语言并发 API-v2")
	want := []string{"go", "语", "语言", "言", "言并", "并", "并发", "发", "api", "v2"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q", got)
	}
}

func TestLibrary(t *testing.T) {
	dir := t.TempDir()
	l, err := Open(dir)
	if err != nil || len(l.Clips) != 0 {
		t.Fatalf("open empty: %v %d", err, len(l.Clips))
	}
	add := func(c *Clip, at time.Time) {
		t.Helper()
		if err := l.Add(c, at); err != nil {
			t.Fatal(err)
		}
	}
	add(&Clip{Kind: KindText, Text: "React 的 useEffect 依赖数组要写全，否则会拿到旧的闭包值。", Tags: []string{"前端"}, Category: "技术"}, now)
	add(&Clip{Kind: KindLink, URL: "https://go.dev/blog/pipelines", Title: "Go 并发模式：管道", Summary: "用 channel 组织并发流水线。", Category: "技术"}, now.Add(time.Minute))
	add(&Clip{Kind: KindText, Text: "周末去爬山，带上水和充电宝。", Category: "生活"}, now.Add(2*time.Minute))

	if l.Clips[0].Category != "生活" {
		t.Fatalf("not newest first: %q", l.Clips[0].Label())
	}
	hits := l.Search("并发", nil)
	if len(hits) != 1 || hits[0].Clip.Title != "Go 并发模式：管道" {
		t.Fatalf("并发: %v", hits)
	}
	hits = l.Search("useeffect 闭包", nil)
	if len(hits) == 0 || !strings.Contains(hits[0].Clip.Text, "useEffect") {
		t.Fatalf("useEffect: %v", hits)
	}
	if hits := l.Search("前端", nil); len(hits) != 1 {
		t.Fatalf("tag: %v", hits)
	}
	if hits := l.Search("", func(c *Clip) bool { return c.Category == "技术" }); len(hits) != 2 {
		t.Fatalf("category: %d", len(hits))
	}
	if cats := l.Categories(); len(cats) != 2 || cats[0] != (Count{"技术", 2}) {
		t.Fatalf("categories %v", cats)
	}

	// Kept on disk, a file a clip.
	l2, err := Open(dir)
	if err != nil || len(l2.Clips) != 3 {
		t.Fatalf("reopen: %v %d", err, len(l2.Clips))
	}
	c := l2.Find(l.Clips[1].ID)
	c.Note = "很经典"
	if err := l2.Save(c, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if hits := l2.Search("经典", nil); len(hits) != 1 {
		t.Fatalf("after save: %v", hits)
	}

	// Files and pictures are copied in.
	src := filepath.Join(t.TempDir(), "笔记.md")
	_ = os.WriteFile(src, []byte("# 部署清单\n先跑迁移"), 0o644)
	fc, err := l2.AddFile(src, "拖入", now)
	if err != nil || fc.Kind != KindFile || fc.Title != "笔记" || !strings.Contains(fc.Text, "迁移") {
		t.Fatalf("file: %v %+v", err, fc)
	}
	if _, err := os.Stat(l2.FilePath(fc)); err != nil {
		t.Fatal(err)
	}
	if err := l2.Delete(fc.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(l2.FilePath(fc)); !os.IsNotExist(err) {
		t.Fatal("file left behind")
	}
	if len(l2.Clips) != 3 || len(l2.Unorganized()) != 3 {
		t.Fatalf("after delete: %d", len(l2.Clips))
	}
}

func TestExcerpt(t *testing.T) {
	text := strings.Repeat("无关的内容。", 50) + "关键的结论在这里。" + strings.Repeat("后面的话。", 50)
	got := Excerpt(text, "结论", 40)
	if !strings.Contains(got, "结论") || !strings.HasPrefix(got, "…") {
		t.Fatalf("excerpt %q", got)
	}
	if got := Excerpt("短文本", "x", 40); got != "短文本" {
		t.Fatalf("short %q", got)
	}
}

func TestParseHTML(t *testing.T) {
	p := ParseHTML(`<html><head><title>标题 &amp; 副标题</title>
<meta name="description" content="一篇讲缓存的文章">
<style>.x{}</style><script>var a = "<p>no</p>";</script></head>
<body><nav>首页 | 关于</nav><article><h1>缓存</h1><p>先读缓存，<b>再</b>读库。</p><p>第二段</p></article><footer>版权</footer></body></html>`)
	if p.Title != "标题 & 副标题" || p.Description != "一篇讲缓存的文章" {
		t.Fatalf("title %q desc %q", p.Title, p.Description)
	}
	if !strings.Contains(p.Text, "先读缓存， 再 读库。") || strings.Contains(p.Text, "首页") || strings.Contains(p.Text, "no") {
		t.Fatalf("text %q", p.Text)
	}
}

func TestIsURL(t *testing.T) {
	for s, want := range map[string]bool{"https://a.com/x?y=1": true, " http://a.com \n": true, "看 https://a.com": false, "a.com": false} {
		if IsURL(s) != want {
			t.Errorf("%q: %v", s, !want)
		}
	}
}
