package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
)

// The AI assistant looks things up in the library while it answers: it
// calls search_library and read_clip, which the app answers at once from
// the local index, and goes on.

const libraryPrompt = `用户还有一个本地「资料库」，收藏了文字、链接（网页正文）、图片和文件，每条有标题、摘要、分类和标签。当问题可能和用户收藏过的资料有关（“我之前存过…”、“资料里…”、查找某个主题的资料、引用资料来写东西等），先调用 search_library 检索（可以换不同关键词多搜几次），需要全文时用 read_clip 读取。回答时说明依据的是哪几条资料（写出标题），不要编造资料库里没有的内容；没找到就直说。
用户的导图（项目、计划、知识整理）也是资料：问题可能和其他导图有关时，用 search_maps 按关键词找导图，用 read_map 读取某张导图的内容（当前打开的导图已在快照里，不用再读）。`

// Tools of the library.
const (
	toolSearchLibrary = "search_library"
	toolReadClip      = "read_clip"
	toolSearchMaps    = "search_maps"
	toolReadMap       = "read_map"
)

func isLibraryTool(name string) bool {
	switch name {
	case toolSearchLibrary, toolReadClip, toolSearchMaps, toolReadMap:
		return true
	}
	return false
}

func librarySchemas() []struct {
	name, desc string
	schema     map[string]any
} {
	str := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
	return []struct {
		name, desc string
		schema     map[string]any
	}{
		{toolSearchLibrary, "在用户的本地资料库中按关键词检索，返回最相关的资料：ID、标题、分类、标签、摘要和匹配的片段。",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":    str("检索词：几个关键词，空格分隔；中英文都可以"),
					"category": str("只在这个分类里找；不限留空字符串"),
				},
				"required":             []string{"query", "category"},
				"additionalProperties": false,
			}},
		{toolReadClip, "读取资料库中一条资料的全文。",
			map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"id": str("资料的 ID，来自 search_library 的结果")},
				"required":             []string{"id"},
				"additionalProperties": false,
			}},
		{toolSearchMaps, "在用户的所有导图中按关键词检索，返回最相关的导图：路径、名称、AI 分析的说明和分类，以及匹配的节点。",
			map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"query": str("检索词：几个关键词，空格分隔；中英文都可以")},
				"required":             []string{"query"},
				"additionalProperties": false,
			}},
		{toolReadMap, "读取一张导图的内容：各画布的节点大纲、任务状态和备注。",
			map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"path": str("导图的路径，来自 search_maps 的结果")},
				"required":             []string{"path"},
				"additionalProperties": false,
			}},
	}
}

func libraryToolsClaude() []anthropic.BetaToolUnionParam {
	var out []anthropic.BetaToolUnionParam
	for _, s := range librarySchemas() {
		tool := anthropic.BetaToolParam{
			Name:        s.name,
			Description: anthropic.String(s.desc),
			Strict:      anthropic.Bool(true),
			InputSchema: anthropic.BetaToolInputSchemaParam{
				Properties:  s.schema["properties"],
				Required:    s.schema["required"].([]string),
				ExtraFields: map[string]any{"additionalProperties": false},
			},
		}
		out = append(out, anthropic.BetaToolUnionParam{OfTool: &tool})
	}
	return out
}

func libraryToolsDeepSeek() []dsTool {
	var out []dsTool
	for _, s := range librarySchemas() {
		t := dsTool{Type: "function"}
		t.Function.Name, t.Function.Description, t.Function.Parameters = s.name, s.desc, s.schema
		out = append(out, t)
	}
	return out
}

// runLibraryTool answers a call of a library tool, and says what it did
// for the conversation shown.
// toolQuery is the query of a call of search_library, "" for others.
func toolQuery(call aiCall) string {
	var in struct {
		Query string `json:"query"`
	}
	if call.name != toolSearchLibrary && call.name != toolSearchMaps || json.Unmarshal([]byte(call.input), &in) != nil {
		return ""
	}
	return strings.TrimSpace(in.Query)
}

func (a *app) runLibraryTool(call aiCall) (result, shown string) {
	var in struct {
		Query    string `json:"query"`
		Category string `json:"category"`
		ID       string `json:"id"`
		Path     string `json:"path"`
	}
	if err := json.Unmarshal([]byte(call.input), &in); err != nil {
		return "参数无法解析。", "资料库检索失败"
	}
	switch call.name {
	case toolSearchMaps:
		text, n := a.searchMapsText(in.Query, 8)
		return text, fmt.Sprintf("🔎 在导图中检索「%s」：%d 张导图", in.Query, n)
	case toolReadMap:
		return a.readMapText(in.Path)
	}
	if a.lib == nil || len(a.lib.Clips) == 0 {
		return "资料库是空的。", "资料库是空的"
	}
	if call.name == toolReadClip {
		c := a.lib.Find(strings.TrimSpace(in.ID))
		if c == nil {
			return "找不到这条资料：" + in.ID, "没有找到资料 " + in.ID
		}
		return library.Full(c, 20000), "📖 读取资料「" + c.Label() + "」"
	}
	text, n := library.SearchText(a.lib, in.Query, in.Category, 8, a.qvecs[strings.TrimSpace(in.Query)], a.embedder().Name())
	return text, fmt.Sprintf("🔎 在资料库中检索「%s」：%d 条结果", in.Query, n)
}

// librarySnapshot is what the AI is given of the library, when it is what
// the conversation is about: what it holds, and the clip shown.
func (a *app) librarySnapshot() string {
	var b strings.Builder
	fmt.Fprintf(&b, "今天是 %s。\n用户正在看资料库：共 %d 条资料。", a.now().Format("2006-01-02"), len(a.lib.Clips))
	if cats := a.lib.Categories(); len(cats) > 0 {
		var s []string
		for _, k := range cats {
			s = append(s, fmt.Sprintf("%s（%d）", k.Name, k.N))
		}
		b.WriteString("\n分类：" + strings.Join(s, "、"))
	}
	if tags := a.lib.Tags(); len(tags) > 0 {
		var s []string
		for i, k := range tags {
			if i == 40 {
				break
			}
			s = append(s, k.Name)
		}
		b.WriteString("\n常用标签：" + strings.Join(s, "、"))
	}
	if c := a.lib.Find(a.libSel); c != nil {
		b.WriteString("\n\n用户当前选中的资料：\n" + library.Full(c, 6000))
	}
	return b.String()
}
