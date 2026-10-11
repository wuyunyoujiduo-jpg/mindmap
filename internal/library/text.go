package library

import (
	"fmt"
	"strings"
)

// SearchText is what a search of the library finds, as the AI
// reads it, and how many clips it found.
// qv, when not nil, is the query's vector of the model, to find clips by
// meaning too.
func SearchText(l *Library, query, category string, limit int, qv []float32, model string) (string, int) {
	category = strings.TrimSpace(category)
	var keep func(*Clip) bool
	if category != "" {
		keep = func(c *Clip) bool { return c.Category == category }
	}
	hits := l.SearchBoth(query, qv, model, keep)
	if len(hits) == 0 {
		return "没有找到匹配的资料。可以换个说法或更少的关键词再试。", 0
	}
	var b strings.Builder
	fmt.Fprintf(&b, "找到 %d 条，最相关的 %d 条：\n", len(hits), min(limit, len(hits)))
	for i, h := range hits {
		if i == limit {
			break
		}
		b.WriteString("\n" + Brief(h.Clip, query))
	}
	return b.String(), len(hits)
}

// Brief describes a clip in a few lines, with the part of its text
// that matches the query.
func Brief(c *Clip, query string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[%s] %s（%s", c.ID, c.Label(), KindLabel(c.Kind))
	if c.Category != "" {
		b.WriteString("，分类：" + c.Category)
	}
	if len(c.Tags) > 0 {
		b.WriteString("，#" + strings.Join(c.Tags, " #"))
	}
	fmt.Fprintf(&b, "，%s）\n", c.Created.Format("2006-01-02"))
	if c.URL != "" {
		b.WriteString("  网址：" + c.URL + "\n")
	}
	if c.Desc != "" {
		b.WriteString("  说明：" + c.Desc + "\n")
	}
	if c.Summary != "" {
		b.WriteString("  摘要：" + c.Summary + "\n")
	}
	if c.Note != "" {
		b.WriteString("  备注：" + Excerpt(c.Note, query, 200) + "\n")
	}
	if c.Text != "" {
		b.WriteString("  片段：" + Excerpt(c.Text, query, 300) + "\n")
	}
	return b.String()
}

// Full is a clip with up to n characters of its text.
func Full(c *Clip, n int) string {
	var b strings.Builder
	b.WriteString(Brief(c, ""))
	if c.Name != "" {
		b.WriteString("  文件：" + c.Name + "\n")
	}
	if len(c.Keywords) > 0 {
		b.WriteString("  关键词：" + strings.Join(c.Keywords, "、") + "\n")
	}
	if len(c.Synonyms) > 0 {
		b.WriteString("  同义词：" + strings.Join(c.Synonyms, "、") + "\n")
	}
	if c.Analysis != "" {
		b.WriteString("\n分析：\n" + c.Analysis + "\n")
	}
	if c.Note != "" {
		b.WriteString("\n备注：\n" + c.Note + "\n")
	}
	if c.Text != "" {
		text := []rune(c.Text)
		if len(text) > n {
			text = append(text[:n], []rune("…（后略）")...)
		}
		b.WriteString("\n全文：\n" + string(text) + "\n")
	}
	return b.String()
}
