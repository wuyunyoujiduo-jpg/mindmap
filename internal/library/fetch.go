package library

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/encoding/simplifiedchinese"
)

// Page is what a web page says of itself, and its text.
type Page struct {
	Title, Description, Text string
}

// MaxPageText is how much of a page's text a clip keeps.
const MaxPageText = 30000

var (
	reTitle   = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	reMeta    = regexp.MustCompile(`(?is)<meta\s[^>]*>`)
	reAttr    = regexp.MustCompile(`(?is)([a-z:_-]+)\s*=\s*("[^"]*"|'[^']*')`)
	reDrop    = regexp.MustCompile(`(?is)<(script|style|noscript|svg|template|iframe|head)\b.*?</(script|style|noscript|svg|template|iframe|head)>|<!--.*?-->`)
	reChrome  = regexp.MustCompile(`(?is)<(nav|footer|aside)\b.*?</(nav|footer|aside)>`)
	reBlock   = regexp.MustCompile(`(?i)<(br|/p|/div|/li|/h[1-6]|/tr|/section|/article|/blockquote|/pre)\b[^>]*>`)
	reTag     = regexp.MustCompile(`(?s)<[^>]*>`)
	reSpaces  = regexp.MustCompile(`[ \t\x{a0}]+`)
	reNewline = regexp.MustCompile(`\n\s*\n+`)
)

// FetchPage reads a web page: its title, description and text.
func FetchPage(ctx context.Context, url string) (Page, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Page{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.9,*/*;q=0.5")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Page{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Page{}, fmt.Errorf("网页返回 %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return Page{}, err
	}
	ct := resp.Header.Get("Content-Type")
	text, err := Decode(data, ct)
	if err != nil {
		return Page{}, err
	}
	if strings.HasPrefix(ct, "text/plain") || strings.HasPrefix(ct, "text/markdown") {
		return Page{Text: clip(text, MaxPageText)}, nil
	}
	return ParseHTML(text), nil
}

var reCharset = regexp.MustCompile(`(?i)charset\s*=\s*["']?([\w.:-]+)`)

// Decode reads a page's bytes as text, in the encoding its Content-Type
// or its <meta> names, else UTF-8, else GB18030, as Chinese pages often
// are.
func Decode(data []byte, contentType string) (string, error) {
	name := ""
	if m := reCharset.FindStringSubmatch(contentType); m != nil {
		name = m[1]
	} else if m := reCharset.FindSubmatch(data[:min(len(data), 4096)]); m != nil {
		name = string(m[1])
	}
	name = strings.ToLower(name)
	if name != "" && name != "utf-8" && name != "utf8" {
		if enc, err := htmlindex.Get(name); err == nil {
			if out, err := enc.NewDecoder().Bytes(data); err == nil {
				return string(out), nil
			}
		}
	}
	if utf8.Valid(data) {
		return string(data), nil
	}
	if out, err := simplifiedchinese.GB18030.NewDecoder().Bytes(data); err == nil {
		return string(out), nil
	}
	return "", fmt.Errorf("无法识别网页的文字编码")
}

// ParseHTML reads a page's title, description and text from its HTML.
func ParseHTML(src string) Page {
	var p Page
	if m := reTitle.FindStringSubmatch(src); m != nil {
		p.Title = clean(m[1])
	}
	for _, tag := range reMeta.FindAllString(src, -1) {
		attrs := map[string]string{}
		for _, a := range reAttr.FindAllStringSubmatch(tag, -1) {
			attrs[strings.ToLower(a[1])] = html.UnescapeString(a[2][1 : len(a[2])-1])
		}
		name := strings.ToLower(attrs["name"] + attrs["property"])
		content := strings.TrimSpace(attrs["content"])
		switch {
		case content == "":
		case name == "og:title" && p.Title == "":
			p.Title = content
		case (name == "description" || name == "og:description") && p.Description == "":
			p.Description = content
		}
	}
	body := src
	if i := strings.Index(strings.ToLower(body), "<body"); i >= 0 {
		body = body[i:]
	}
	body = reDrop.ReplaceAllString(body, " ")
	if main := mainPart(body); main != "" {
		body = main
	} else {
		body = reChrome.ReplaceAllString(body, " ")
	}
	body = reBlock.ReplaceAllString(body, "\n")
	body = reTag.ReplaceAllString(body, " ")
	p.Text = clip(clean(body), MaxPageText)
	return p
}

// mainPart is the page's <article> or <main>, where its content is.
func mainPart(body string) string {
	lower := strings.ToLower(body)
	for _, tag := range []string{"article", "main"} {
		i := strings.Index(lower, "<"+tag)
		j := strings.LastIndex(lower, "</"+tag+">")
		if i >= 0 && j > i {
			return body[i:j]
		}
	}
	return ""
}

// clean unescapes HTML text and tidies its spaces and blank lines.
func clean(s string) string {
	s = html.UnescapeString(s)
	s = reSpaces.ReplaceAllString(s, " ")
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	s = strings.Join(lines, "\n")
	s = reNewline.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
