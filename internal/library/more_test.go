package library

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func TestDecode(t *testing.T) {
	gbk, _ := simplifiedchinese.GBK.NewEncoder().String(`<html><head><meta charset="gbk"><title>中文网页</title></head><body>正文内容</body></html>`)
	text, err := Decode([]byte(gbk), "text/html")
	if err != nil || !strings.Contains(text, "中文网页") {
		t.Fatalf("gbk by meta: %v %q", err, text)
	}
	plain, _ := simplifiedchinese.GB18030.NewEncoder().String("没有声明编码的中文")
	if text, _ := Decode([]byte(plain), ""); text != "没有声明编码的中文" {
		t.Fatalf("guessed: %q", text)
	}
	if text, _ := Decode([]byte("utf-8 中文"), "text/html; charset=UTF-8"); text != "utf-8 中文" {
		t.Fatalf("utf-8: %q", text)
	}
}

// officeFile makes a zip of parts, as Office files are.
func officeFile(t *testing.T, parts map[string]string) []byte {
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for name, body := range parts {
		w, _ := zw.Create(name)
		w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestExtractText(t *testing.T) {
	docx := officeFile(t, map[string]string{"word/document.xml": `<w:document xmlns:w="w"><w:body>
		<w:p><w:r><w:t>第一段</w:t></w:r><w:r><w:t>接着</w:t></w:r></w:p><w:p><w:r><w:t>第二段</w:t></w:r></w:p></w:body></w:document>`})
	if got := ExtractText("a.docx", docx); got != "第一段接着\n第二段" {
		t.Fatalf("docx %q", got)
	}
	pptx := officeFile(t, map[string]string{
		"ppt/slides/slide10.xml": `<p:sld xmlns:a="a" xmlns:p="p"><a:p><a:r><a:t>第十页</a:t></a:r></a:p></p:sld>`,
		"ppt/slides/slide2.xml":  `<p:sld xmlns:a="a" xmlns:p="p"><a:p><a:r><a:t>第二页</a:t></a:r></a:p></p:sld>`,
	})
	if got := ExtractText("a.pptx", pptx); got != "第二页\n\n第十页" {
		t.Fatalf("pptx %q", got)
	}
	if got := ExtractText("broken.pdf", []byte("%PDF-1.4 not really")); got != "" {
		t.Fatalf("broken pdf %q", got)
	}
	if got := ExtractText("a.md", []byte("# 标题")); got != "# 标题" {
		t.Fatalf("md %q", got)
	}
}

func TestDuplicates(t *testing.T) {
	if NormalizeURL("https://www.Example.com/a/?utm_source=x&id=1#top") != NormalizeURL("http://example.com/a?id=1") {
		t.Fatal("normalize")
	}
	l, _ := Open(t.TempDir())
	if err := l.Add(&Clip{Kind: KindLink, URL: "https://example.com/a?utm_source=wx"}, now); err != nil {
		t.Fatal(err)
	}
	var dup *DuplicateError
	if err := l.Add(&Clip{Kind: KindLink, URL: "https://www.example.com/a/"}, now); !errors.As(err, &dup) {
		t.Fatalf("link not found again: %v", err)
	}
	l.Add(&Clip{Kind: KindText, Text: "同一段  话"}, now)
	if err := l.Add(&Clip{Kind: KindText, Text: "同一段 话\n"}, now); !errors.As(err, &dup) {
		t.Fatalf("text: %v", err)
	}
	if _, err := l.AddData([]byte("png"), "a.png", KindImage, "", now); err != nil {
		t.Fatal(err)
	}
	if _, err := l.AddData([]byte("png"), "b.png", KindImage, "", now); !errors.As(err, &dup) {
		t.Fatalf("file: %v", err)
	}
	if len(l.Clips) != 3 {
		t.Fatalf("%d clips", len(l.Clips))
	}
}

// fakeVectors gives texts vectors by the words they have of a few.
func fakeVectors(texts []string) [][]float32 {
	axes := [][]string{{"数据库", "SQL", "索引", "慢"}, {"菜", "鸡蛋", "厨房"}, {"旅行", "机票"}}
	var out [][]float32
	for _, s := range texts {
		v := make([]float32, len(axes)+1)
		v[len(axes)] = 0.1
		for i, words := range axes {
			for _, w := range words {
				if strings.Contains(s, w) {
					v[i]++
				}
			}
		}
		out = append(out, normalize(v))
	}
	return out
}

func TestSemanticSearch(t *testing.T) {
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		asked = append(asked, req.Input...)
		if r.Header.Get("Authorization") != "Bearer k" || req.Model != "m" {
			http.Error(w, `{"error":{"message":"bad key"}}`, 401)
			return
		}
		var data []map[string]any
		for i, v := range fakeVectors(req.Input) {
			data = append(data, map[string]any{"index": i, "embedding": v})
		}
		json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	defer srv.Close()
	e := Embedder{URL: srv.URL, Key: "k", Model: "m"}
	if _, err := (Embedder{URL: srv.URL, Key: "x", Model: "m"}).Embed(context.Background(), []string{"a"}); err == nil || !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("error %v", err)
	}

	dir := t.TempDir()
	l, _ := Open(dir)
	l.Add(&Clip{Kind: KindText, Title: "SQL 优化笔记", Text: "联合索引遵循最左前缀"}, now)
	l.Add(&Clip{Kind: KindText, Title: "番茄炒蛋", Text: "鸡蛋先炒"}, now)
	l.Add(&Clip{Kind: KindText, Title: "去东京", Text: "机票提前订"}, now)
	ids, texts := l.Unembedded("m")
	if len(ids) != 3 {
		t.Fatalf("%d to embed", len(ids))
	}
	vecs, err := e.Embed(context.Background(), texts)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.SetVectors("m", ids, texts, vecs); err != nil {
		t.Fatal(err)
	}
	// Kept: read again, nothing left to make.
	l2, _ := Open(dir)
	if ids, _ := l2.Unembedded("m"); len(ids) != 0 {
		t.Fatalf("%d left after reopening", len(ids))
	}
	if ids, _ := l2.Unembedded("other"); len(ids) != 3 {
		t.Fatal("another model's vectors used")
	}

	// "数据库变慢" shares no word with the SQL note, but means the same.
	q := "数据库变慢怎么办"
	if hits := l2.Search(q, nil); len(hits) != 0 && hits[0].Clip.Title == "SQL 优化笔记" {
		t.Fatal("found by words alone; the test proves nothing")
	}
	qv, _ := e.Embed(context.Background(), []string{q})
	hits := l2.SearchBoth(q, qv[0], "m", nil)
	if len(hits) == 0 || hits[0].Clip.Title != "SQL 优化笔记" {
		t.Fatalf("by meaning: %v", hits)
	}
	rel := l2.Related(l2.Clips[2], "m", 3) // the SQL note
	for _, h := range rel {
		if h.Clip.Title == "番茄炒蛋" {
			t.Fatalf("related: %v", rel)
		}
	}

	// A clip changed has its vector made again.
	c := l2.Clips[0]
	c.Note = "改了"
	l2.Save(c, now)
	if ids, _ := l2.Unembedded("m"); len(ids) != 1 || ids[0] != c.ID {
		t.Fatalf("changed: %v", ids)
	}
}
