package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/gitbase"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/versions"
)

// The AI panel chats with Claude or DeepSeek about the map. Each message
// carries the map as an outline with the nodes' IDs; the model answers,
// and when asked to change the map it proposes edits through the
// propose_edits tool, which the user reviews and applies, as one step that
// undo takes back.

// Providers of the assistant.
const (
	providerClaude   = ""
	providerDeepSeek = "deepseek"
)

// aiModels are the Claude models offered, the first being the default.
var aiModels = []struct{ id, name string }{
	{"claude-opus-5-5", "Claude Opus 5.5（推荐）"},
	{"claude-sonnet-5-5", "Claude Sonnet 5.5（更快）"},
	{"claude-haiku-5-5", "Claude Haiku 5.5（最便宜）"},
	{"claude-fable-5-1", "Claude Fable 5.1（最强）"},
}

// Scopes of what the AI sees.
const (
	scopeCanvas  = iota // the canvas shown
	scopeSubtree        // the selected node and what is below it
	scopeFile           // every canvas of the file
	scopeNone           // no map: a question of its own, or a new map
)

// aiEdit is one change the AI proposes.
type aiEdit struct {
	Op     string   `json:"op"`     // "add", "update", "delete" or "move"
	ID     string   `json:"id"`     // the node changed, deleted or moved
	Parent string   `json:"parent"` // where a node is added or moved: an ID or a ref
	Ref    string   `json:"ref"`    // a name for an added node, for later edits to use
	Title  string   `json:"title"`
	Note   string   `json:"note"`
	Tags   []string `json:"tags"`
	Status string   `json:"status"` // "", "none", "idea", "todo", "doing" or "done"
	Due    string   `json:"due"`    // "", "none" or YYYY-MM-DD
	Links  []string `json:"links"`  // links to add: pages, code files (path:line)
}

type aiProposal struct {
	Summary    string   `json:"summary"`
	Operations []aiEdit `json:"operations"`
	// NewMap, instead, is a map to create (propose_new_map).
	NewMap *aiNewMap `json:"newMap,omitempty"`
}

// aiNewMap is a new map the AI proposes: its name and its nodes.
type aiNewMap struct {
	Name    string `json:"name"`
	Outline string `json:"outline"` // a Markdown list, nested by two spaces
}

// Proposal states.
const (
	proposalPending = iota
	proposalApplied
	proposalDiscarded
	proposalFailed
)

// aiCall is a tool call of a reply.
type aiCall struct{ id, name, input string }

// aiReply is a reply of either provider, as the panel shows it.
type aiReply struct {
	texts     []string
	calls     []aiCall
	truncated bool
	refusal   string
	// commit adds the reply to the conversation sent to its provider.
	commit func(ai *aiState)
}

// toolAnswer is what became of a proposal, sent with the next message.
type toolAnswer struct {
	id, text string
	isError  bool
}

// chatMsg is a message shown in the panel.
type chatMsg struct {
	role      string // "user", "assistant" or "error"
	text      string
	proposal  *aiProposal
	toolUseID string
	state     int
	result    string     // what applying did
	files     []aiAttach // attached to a message of the user's
	names     []string   // of the files, of a conversation kept
	// baseline is the commit of the code a proposal brings the map up to
	// date with: applying it records it as the map's code baseline.
	baseline *model.Baseline
}

// aiState is the conversation of the AI panel.
type aiState struct {
	provider  string                       // of the conversation, which stays with it
	claude    []anthropic.BetaMessageParam // the conversation, as sent to Claude
	ds        []dsMessage                  // or to DeepSeek
	msgs      []chatMsg
	draft     string
	attach    []aiAttach // attached to the draft
	busy      bool
	scope     int
	settings  bool   // the settings form is shown
	extra     string // context an action adds to the next message, after the map
	rounds    int    // replies in a row that only looked things up in the library
	turn      int    // counts the requests, so that a late piece of a reply is dropped
	key       string // what the conversation is about: a map's path, or the library
	chatID    string // the conversation's, as kept
	streaming string // the text of the reply coming
	// answers answer the proposals of the last reply; the next message
	// sends them first, as the APIs require.
	answers []toolAnswer
	// syncBase is the commit of the code the request is about (see
	// syncWithCode), which its proposals bring the map up to date with.
	syncBase  *model.Baseline
	nextSync  *model.Baseline // the next request's syncBase
	cancel    context.CancelFunc
	scroll    ui.ScrollState
	keyDraft  string
	baseDraft string
}

// aiSender sends a request to Claude; tests replace it. onText, when not
// nil, is given the reply's text as it comes, off the main thread.
type aiSender func(ctx context.Context, cfg config, params anthropic.BetaMessageNewParams, onText func(string)) (*anthropic.BetaMessage, error)

// providerKey is the API key of the provider chosen, from the settings or
// the environment.
func providerKey(cfg config) string {
	if cfg.AIProvider == providerDeepSeek {
		return deepseekKey(cfg)
	}
	return aiKey(cfg)
}

func providerModelName(cfg config) string {
	if cfg.AIProvider == providerDeepSeek {
		id := deepseekModel(cfg)
		for _, m := range deepseekModels {
			if m.id == id {
				return m.name
			}
		}
		return id
	}
	return aiModelName(aiModel(cfg))
}

// sendToClaude calls the Messages API with the official SDK, streaming the
// reply when onText takes its text.
func sendToClaude(ctx context.Context, cfg config, params anthropic.BetaMessageNewParams, onText func(string)) (*anthropic.BetaMessage, error) {
	opts := []option.RequestOption{option.WithRequestTimeout(5 * time.Minute)}
	if key := aiKey(cfg); key != "" {
		opts = append(opts, option.WithAPIKey(key))
	}
	if cfg.AIBaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.AIBaseURL))
	}
	client := anthropic.NewClient(opts...)
	if onText == nil {
		return client.Beta.Messages.New(ctx, params)
	}
	stream := client.Beta.Messages.NewStreaming(ctx, params)
	defer stream.Close()
	var msg anthropic.BetaMessage
	for stream.Next() {
		ev := stream.Current()
		if err := msg.Accumulate(ev); err != nil {
			return nil, err
		}
		if ev.Type == "content_block_delta" && ev.Delta.Type == "text_delta" && ev.Delta.Text != "" {
			onText(ev.Delta.Text)
		}
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	return &msg, nil
}

// aiKey is the API key of the settings, else of the environment.
func aiKey(cfg config) string {
	if cfg.AIKey != "" {
		return cfg.AIKey
	}
	return os.Getenv("ANTHROPIC_API_KEY")
}

func aiModel(cfg config) string {
	if cfg.AIModel != "" {
		return cfg.AIModel
	}
	return aiModels[0].id
}

const aiSystemPrompt = `你是「MyMind」思维导图项目管理工具里的助手。用户用思维导图管理项目：模块、功能、实现细节、待优化点和任务。

每条用户消息都附带导图的当前快照：一份缩进大纲，每个节点前面的 [id] 是它的 ID，后面可能有标签（#标签）、任务（状态、截止日期）和备注。快照总是最新的，以它为准。

回答用户的问题时，简洁地用中文回答，可以引用节点标题。

当用户要求修改导图（添加、整理、拆分任务、补充备注、改状态、移动、删除等）时，调用 propose_edits 工具，把这一次的所有修改放在一次调用里。用户会先看到修改清单，确认后才会应用，所以不要在文字里重复列出修改内容，只用一两句话说明思路。
- 引用已有节点时，必须使用快照里的 ID，不要编造 ID。
- 新增的节点给一个 ref（如 "r1"），后面的修改可以用这个 ref 作为父节点。
- 标题保持简短，细节写进备注。
- 不需要修改的字段留空字符串（tags 留空数组）。
- 不要删除用户没有要求删除的内容。

当用户要新建一张导图（而不是改当前这张）时，调用 propose_new_map：给导图起个简短的名字，节点写成 Markdown 列表（两个空格缩进一层，"- [ ] 文字" 是待做任务）。用户确认后才会创建并打开它。`

const proposeEditsDescription = "向用户提出对导图的一组修改。用户确认后才会应用。一次请求的所有修改放在一次调用里。"

// editsSchema is the JSON schema of propose_edits's input.
func editsSchema() map[string]any {
	str := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
	op := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"op":     map[string]any{"type": "string", "enum": []string{"add", "update", "delete", "move"}, "description": "add 新增节点；update 修改节点；delete 删除节点及其下级；move 把节点移到新的父节点下"},
			"id":     str("update / delete / move 的目标节点 ID；add 时留空"),
			"parent": str("add / move 时的父节点：已有节点的 ID，或本次新增节点的 ref；其他操作留空"),
			"ref":    str("add 时给新节点起的引用名，如 r1；其他操作留空"),
			"title":  str("add 时的标题；update 时的新标题，不改留空"),
			"note":   str("备注；不改留空"),
			"tags":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "update 时替换为这些标签，add 时设置的标签；不改留空数组"},
			"status": map[string]any{"type": "string", "enum": []string{"", "none", "idea", "todo", "doing", "done"}, "description": "任务状态：idea 想法，todo 待做，doing 进行中，done 完成，none 取消任务；不改留空"},
			"due":    str("截止日期 YYYY-MM-DD，none 表示清除；不改留空"),
			"links":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "要添加的链接：网址，或代码文件的相对路径（可带 :行号）；没有留空数组"},
		},
		"required":             []string{"op", "id", "parent", "ref", "title", "note", "tags", "status", "due", "links"},
		"additionalProperties": false,
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"summary":    str("一句话概括这组修改"),
			"operations": map[string]any{"type": "array", "items": op},
		},
		"required":             []string{"summary", "operations"},
		"additionalProperties": false,
	}
}

// proposeEditsTool is the tool Claude proposes edits with. Strict mode
// keeps its input valid against the schema.
func proposeEditsTool() anthropic.BetaToolUnionParam {
	schema := editsSchema()
	tool := anthropic.BetaToolParam{
		Name:        "propose_edits",
		Description: anthropic.String(proposeEditsDescription),
		Strict:      anthropic.Bool(true),
		InputSchema: anthropic.BetaToolInputSchemaParam{
			Properties:  schema["properties"],
			Required:    schema["required"].([]string),
			ExtraFields: map[string]any{"additionalProperties": false},
		},
	}
	return anthropic.BetaToolUnionParam{OfTool: &tool}
}

// snapshot writes the map the AI sees as an outline with the nodes' IDs.
func (a *app) snapshot(scope int) string {
	var b strings.Builder
	var write func(n *model.Node, depth int)
	write = func(n *model.Node, depth int) {
		b.WriteString(strings.Repeat("  ", depth))
		fmt.Fprintf(&b, "- [%s] %s", n.ID, n.Title)
		for _, t := range n.Tags {
			b.WriteString(" #" + t)
		}
		if n.Task != nil {
			fmt.Fprintf(&b, "（任务：%s", n.Task.Status.Label())
			if n.Task.Due != nil {
				fmt.Fprintf(&b, "，截止 %s", n.Task.Due.Format("2006-01-02"))
			}
			if n.Task.Priority != model.PriorityNone {
				fmt.Fprintf(&b, "，优先级%s", n.Task.Priority.Label())
			}
			b.WriteString("）")
		}
		for _, l := range n.Links {
			b.WriteString(" <" + l + ">")
		}
		if n.Image != nil {
			b.WriteString("（附图片）")
		}
		if n.Collapsed {
			b.WriteString("（已折叠）")
		}
		b.WriteString("\n")
		if note := strings.TrimSpace(n.Note); note != "" {
			for _, line := range strings.Split(note, "\n") {
				fmt.Fprintf(&b, "%s  > %s\n", strings.Repeat("  ", depth), line)
			}
		}
		for _, c := range n.Children {
			write(c, depth+1)
		}
	}
	sheet := func(d *model.Document) {
		fmt.Fprintf(&b, "画布「%s」（结构：%s）\n", d.Title, d.Structure.Label())
		write(d.Root, 0)
		if len(d.Floating) > 0 {
			b.WriteString("自由主题（不在树上）：\n")
			for _, f := range d.Floating {
				write(f, 0)
			}
		}
	}
	fmt.Fprintf(&b, "今天是 %s。\n", a.now().Format("2006-01-02"))
	switch scope {
	case scopeFile:
		for _, d := range a.file.Sheets {
			sheet(d)
			b.WriteString("\n")
		}
	case scopeSubtree:
		n, _ := a.selectedNode()
		if n == nil {
			n = a.doc.Root
		}
		fmt.Fprintf(&b, "画布「%s」中选中的节点及其下级：\n", a.doc.Title)
		write(n, 0)
	default:
		sheet(a.doc)
	}
	return b.String()
}

// aiSend sends the draft, with the map, or what the library holds when it
// is shown, and waits for the reply in the background.
func (a *app) aiSend() {
	draft, extra, files, sync := a.ai.draft, a.ai.extra, a.ai.attach, a.ai.nextSync
	a.ai.nextSync = nil
	a.switchChat() // the conversation about what is shown
	a.ai.draft, a.ai.extra, a.ai.attach = draft, extra, files
	ai := &a.ai
	text := strings.TrimSpace(ai.draft)
	if text == "" && len(files) > 0 {
		text = "请看附件。"
	}
	if text == "" || ai.busy || a.doc == nil && !a.libOpen {
		return
	}
	if providerKey(a.cfg) == "" {
		ai.settings = true
		ai.msgs = append(ai.msgs, chatMsg{role: "error", text: "请先在设置中填写 API Key。"})
		return
	}
	if ai.provider != a.cfg.AIProvider && len(ai.claude)+len(ai.ds) > 0 {
		*ai = aiState{scope: ai.scope, draft: ai.draft, attach: ai.attach, key: ai.key} // another provider: a new conversation
	}
	if a.cfg.AIProvider == providerDeepSeek && len(claudeImages(files)) > 0 {
		ai.msgs = append(ai.msgs, chatMsg{role: "error", text: errNoImages.Error()})
		return
	}
	ai.provider = a.cfg.AIProvider
	ai.rounds = 0
	ai.syncBase = sync // nil for a message of the user's, about something else
	// Proposals left unanswered are discarded by sending on.
	for i := range ai.msgs {
		if ai.msgs[i].proposal != nil && ai.msgs[i].state == proposalPending {
			a.answerProposal(i, false)
		}
	}
	var snapshot string
	if a.libOpen || a.doc == nil {
		snapshot = "资料库：\n" + a.librarySnapshot()
	} else if ai.scope == scopeNone {
		snapshot = "（这次没有附带导图内容。要新建导图时用 propose_new_map。）"
	} else {
		snapshot = "导图快照：\n" + a.snapshot(ai.scope)
	}
	if ai.extra != "" { // what an action gives besides the map
		snapshot += "\n\n" + ai.extra
		ai.extra = ""
	}
	ai.msgs = append(ai.msgs, chatMsg{role: "user", text: text, files: files})
	ai.draft, ai.attach = "", nil
	a.aiTurn(snapshot, text, files)
}

// aiTurn sends the answers to the last reply's tool calls, and the user's
// message when there is one, with its files, and waits for the reply in the background.
func (a *app) aiTurn(snapshot, text string, files []aiAttach) {
	ai := &a.ai
	answers := ai.answers
	ai.answers = nil
	ai.busy = true
	ai.scroll.Y = 1e9

	ctx, cancel := context.WithCancel(context.Background())
	ai.cancel = cancel
	cfg := a.cfg
	claudeLen, dsLen := len(ai.claude), len(ai.ds)
	// The reply's text shows as it comes.
	ai.turn++
	turn := ai.turn
	ai.streaming = ""
	post := a.post
	onText := func(s string) {
		post(func() {
			if ai.turn == turn && ai.busy {
				ai.streaming += s
				ai.scroll.Y = 1e9
			}
		})
	}
	done := func(v any, err error) {
		cancel()
		r, _ := v.(aiReply)
		if err != nil || r.refusal != "" {
			// Take the turn back, so the next message sends it again.
			ai.claude, ai.ds = ai.claude[:claudeLen], ai.ds[:dsLen]
			ai.answers = append(answers, ai.answers...)
		}
		a.aiReceive(r, err)
	}
	if ai.provider == providerDeepSeek {
		for _, an := range answers {
			ai.ds = append(ai.ds, dsMessage{Role: "tool", ToolCallID: an.id, Content: an.text})
		}
		if text != "" {
			content := snapshot + "\n\n" + text
			if t := attachText(files); t != "" {
				content = snapshot + "\n\n" + t + "\n\n" + text
			}
			ai.ds = append(ai.ds, dsMessage{Role: "user", Content: content})
		}
		req := a.deepseekRequest()
		send := a.sendDeepSeek
		a.async(func() (any, error) {
			resp, err := send(ctx, cfg, req, onText)
			if err != nil {
				return nil, err
			}
			return deepseekReply(resp), nil
		}, done)
		return
	}
	var blocks []anthropic.BetaContentBlockParamUnion
	for _, an := range answers {
		blocks = append(blocks, anthropic.NewBetaToolResultBlock(an.id, an.text, an.isError))
	}
	if text != "" {
		blocks = append(blocks, anthropic.NewBetaTextBlock(snapshot))
		if t := attachText(files); t != "" {
			blocks = append(blocks, anthropic.NewBetaTextBlock(t))
		}
		blocks = append(blocks, claudeImages(files)...)
		blocks = append(blocks, anthropic.NewBetaTextBlock(text))
	}
	ai.claude = append(ai.claude, anthropic.NewBetaUserMessage(blocks...))
	params := a.aiParams()
	send := a.sendAI
	a.async(func() (any, error) {
		resp, err := send(ctx, cfg, params, onText)
		if err != nil {
			return nil, err
		}
		return claudeReply(resp), nil
	}, done)
}

// aiSystem is the system prompt: on the map, and on the library.
func aiSystem() string { return aiSystemPrompt + "\n\n" + libraryPrompt }

// aiEdits tells whether the assistant may propose edits: when the map is
// what the conversation is about.
func (a *app) aiEdits() bool { return !a.libOpen && a.doc != nil }

func (a *app) aiParams() anthropic.BetaMessageNewParams {
	m := aiModel(a.cfg)
	p := anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(m),
		MaxTokens: 16000,
		System:    []anthropic.BetaTextBlockParam{{Text: aiSystem()}},
		Messages:  slices.Clone(a.ai.claude),
		Tools:     libraryToolsClaude(),
		OutputConfig: anthropic.BetaOutputConfigParam{
			Effort: anthropic.BetaOutputConfigEffortMedium,
		},
	}
	p.Tools = append([]anthropic.BetaToolUnionParam{newMapToolClaude()}, p.Tools...)
	if a.aiEdits() {
		p.Tools = append([]anthropic.BetaToolUnionParam{proposeEditsTool()}, p.Tools...)
	}
	// A refusal of the model's safeguards is answered again by another
	// model the API picks, where the model has such fallbacks.
	if m != "claude-haiku-5-5" {
		p.Betas = []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01}
		p.Fallbacks = anthropic.BetaFallbacksParamUnion{OfDefault: constant.ValueOf[constant.Default]()}
	}
	return p
}

// claudeReply turns Claude's answer into what the panel shows.
func claudeReply(resp *anthropic.BetaMessage) aiReply {
	if resp.StopReason == anthropic.BetaStopReasonRefusal {
		return aiReply{refusal: string(resp.StopDetails.Category)}
	}
	r := aiReply{truncated: resp.StopReason == anthropic.BetaStopReasonMaxTokens}
	for _, block := range resp.Content {
		switch b := block.AsAny().(type) {
		case anthropic.BetaTextBlock:
			if s := strings.TrimSpace(b.Text); s != "" {
				r.texts = append(r.texts, s)
			}
		case anthropic.BetaToolUseBlock:
			input := b.JSON.Input.Raw()
			if input == "" {
				raw, _ := json.Marshal(b.Input)
				input = string(raw)
			}
			r.calls = append(r.calls, aiCall{id: b.ID, name: b.Name, input: input})
		}
	}
	param := resp.ToParam()
	r.commit = func(ai *aiState) { ai.claude = append(ai.claude, param) }
	return r
}

// aiReceive shows a reply: its text, and the edits it proposes.
func (a *app) aiReceive(r aiReply, err error) {
	ai := &a.ai
	ai.busy, ai.cancel, ai.streaming = false, nil, ""
	defer a.saveChat()
	ai.scroll.Y = 1e9
	switch {
	case err != nil:
		ai.msgs = append(ai.msgs, chatMsg{role: "error", text: aiError(err)})
		return
	case r.refusal != "":
		ai.msgs = append(ai.msgs, chatMsg{role: "error", text: "请求被模型拒绝：" + r.refusal})
		return
	}
	r.commit(ai)
	text := strings.Join(r.texts, "\n\n")
	looked := false // the reply looked things up, and goes on
	for _, call := range r.calls {
		looked = looked || isLibraryTool(call.name)
	}
	if looked {
		if text != "" {
			ai.msgs = append(ai.msgs, chatMsg{role: "assistant", text: text})
		}
		var queries []string
		for _, call := range r.calls {
			if q := toolQuery(call); q != "" {
				queries = append(queries, q)
			}
		}
		// The queries' vectors first, to search by meaning, then the tools.
		ai.busy = true
		a.embedQueries(queries, func() { a.answerLibraryCalls(r.calls) })
		return
	}
	shown := false
	for _, call := range r.calls {
		var p aiProposal
		if call.name == "propose_new_map" {
			var nm aiNewMap
			if json.Unmarshal([]byte(call.input), &nm) != nil || strings.TrimSpace(nm.Name) == "" {
				ai.msgs = append(ai.msgs, chatMsg{role: "error", text: "无法读取 AI 提出的新导图"})
				ai.answers = append(ai.answers, toolAnswer{id: call.id, text: "无法解析，没有创建。", isError: true})
				continue
			}
			nm.Name = strings.TrimSpace(nm.Name)
			p = aiProposal{Summary: "新建导图「" + nm.Name + "」", NewMap: &nm}
		} else if call.name != "propose_edits" || json.Unmarshal([]byte(call.input), &p) != nil {
			ai.msgs = append(ai.msgs, chatMsg{role: "error", text: "无法读取 AI 提出的修改"})
			ai.answers = append(ai.answers, toolAnswer{id: call.id, text: "无法解析修改，未应用。", isError: true})
			continue
		}
		msg := chatMsg{role: "assistant", proposal: &p, toolUseID: call.id, baseline: ai.syncBase}
		if !shown {
			msg.text, shown = text, true
		}
		ai.msgs = append(ai.msgs, msg)
	}
	if !shown && text != "" {
		ai.msgs = append(ai.msgs, chatMsg{role: "assistant", text: text})
	}
	if r.truncated {
		ai.msgs = append(ai.msgs, chatMsg{role: "error", text: "回复太长，被截断了。"})
	}
}

// answerLibraryCalls answers the library tools a reply called, and goes on.
func (a *app) answerLibraryCalls(calls []aiCall) {
	ai := &a.ai
	ai.busy = false
	for _, call := range calls {
		if !isLibraryTool(call.name) {
			ai.answers = append(ai.answers, toolAnswer{id: call.id, text: "没有展示给用户：请先完成检索，再单独提出修改。", isError: true})
			continue
		}
		result, shown := a.runLibraryTool(call)
		ai.answers = append(ai.answers, toolAnswer{id: call.id, text: result})
		ai.msgs = append(ai.msgs, chatMsg{role: "tool", text: shown})
	}
	if ai.rounds < 8 {
		ai.rounds++
		a.aiTurn("", "", nil)
	} else {
		ai.msgs = append(ai.msgs, chatMsg{role: "error", text: "检索次数太多，已停下。可以换个问法再试。"})
	}
}

func aiError(err error) string {
	if s := deepseekError(err); s != "" {
		return s
	}
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case 401:
			return "API Key 无效，请在设置中检查。"
		case 403:
			return "没有权限使用这个模型。"
		case 404:
			return "找不到模型或接口地址，请在设置中检查。"
		case 429:
			return "请求太频繁或额度已用完，请稍后再试。"
		case 529, 500, 502, 503:
			return "Claude 服务繁忙，请稍后再试。"
		}
		return fmt.Sprintf("请求失败（%d）：%s", apiErr.StatusCode, apiErr.Error())
	}
	if errors.Is(err, context.Canceled) {
		return "已停止。"
	}
	var plain plainError
	if errors.As(err, &plain) {
		return string(plain)
	}
	return "网络错误：" + err.Error()
}

// plainError is an error shown as it is: not of the network.
type plainError string

func (e plainError) Error() string { return string(e) }

// answerProposal applies or discards the proposal of message i, and
// prepares the tool result the next request starts with.
func (a *app) answerProposal(i int, apply bool) {
	ai := &a.ai
	m := &ai.msgs[i]
	if m.state != proposalPending {
		return
	}
	result := "用户没有应用这些修改。"
	if apply && a.file == nil {
		m.state, m.result = proposalFailed, "没有打开的导图"
		result = "应用失败：没有打开的导图。"
	} else if apply && m.proposal.NewMap != nil {
		if name, err := a.createMapFromAI(m.proposal.NewMap); err != nil {
			m.state, m.result = proposalFailed, err.Error()
			result = "创建失败：" + err.Error()
		} else {
			m.state, m.result = proposalApplied, "已新建导图「"+name+"」并打开"
			result = "用户已创建导图「" + name + "」，现在打开的就是它。"
		}
	} else if apply {
		n, err := a.applyEdits(m.proposal.Operations, m.baseline)
		switch {
		case err != nil:
			m.state, m.result = proposalFailed, err.Error()
			result = "应用失败：" + err.Error()
		case n == 0:
			m.state, m.result = proposalApplied, "导图已与代码一致，代码基线已更新到 "+gitbase.Short(m.baseline.Commit)
			result = "用户已确认导图与代码一致。"
		default:
			m.state, m.result = proposalApplied, fmt.Sprintf("已应用 %d 项修改（可撤销）", n)
			if m.baseline != nil {
				m.result += "，代码基线已更新到 " + gitbase.Short(m.baseline.Commit)
			}
			result = fmt.Sprintf("用户已应用这 %d 项修改。", n)
		}
	} else {
		m.state = proposalDiscarded
	}
	ai.answers = append(ai.answers, toolAnswer{id: m.toolUseID, text: result})
	a.saveChat()
}

// applyEdits makes the proposed edits, all or none, as one step of the
// history, and returns how many it made. With a baseline, the map is up
// to date with that code: it is recorded in the same step, which undo
// takes back with the edits.
func (a *app) applyEdits(ops []aiEdit, base *model.Baseline) (int, error) {
	if len(ops) == 0 && base == nil {
		return 0, errors.New("没有修改")
	}
	if _, err := editFile(a.file.Clone(), ops); err != nil { // all or none
		return 0, err
	}
	var done []aiEdit
	a.mutate("", func() {
		done, _ = editFile(a.file, ops)
		if base != nil {
			b := *base
			a.file.Baseline = &b
		}
	})
	ops = done
	// The canvas object stays; show the one with the last node touched.
	for i := len(ops) - 1; i >= 0; i-- {
		if d := a.file.SheetOf(ops[i].ID); d != nil && ops[i].Op != "delete" {
			a.showSheet(d.ID)
			a.selectNode(ops[i].ID)
			break
		}
	}
	return len(ops), nil
}

// editFile makes the edits on a file, stopping at the first that fails.
// It returns the edits with the IDs of the nodes they made or changed.
func editFile(f *model.File, in []aiEdit) ([]aiEdit, error) {
	ops := slices.Clone(in)
	refs := map[string]string{} // ref -> the ID of the node added
	resolve := func(id string) string {
		if r, ok := refs[id]; ok {
			return r
		}
		return id
	}
	find := func(id string) (*model.Document, *model.Node, error) {
		id = resolve(id)
		d := f.SheetOf(id)
		if d == nil {
			return nil, nil, fmt.Errorf("找不到节点 %s", id)
		}
		n, _ := d.Find(id)
		return d, n, nil
	}
	for i := range ops {
		op := &ops[i]
		switch op.Op {
		case "add":
			d, parent, err := find(op.Parent)
			if err != nil {
				return nil, err
			}
			n, _ := d.AddChild(parent.ID, strings.TrimSpace(op.Title))
			if op.Ref != "" {
				refs[op.Ref] = n.ID
			}
			op.ID = n.ID
			setFields(n, *op)
		case "update":
			_, n, err := find(op.ID)
			if err != nil {
				return nil, err
			}
			op.ID = n.ID
			if t := strings.TrimSpace(op.Title); t != "" {
				n.Title = t
			}
			setFields(n, *op)
		case "delete":
			d, n, err := find(op.ID)
			if err != nil {
				return nil, err
			}
			if n == d.Root {
				return nil, errors.New("不能删除中心主题")
			}
			_, _ = d.Delete(n.ID)
		case "move":
			d, n, err := find(op.ID)
			if err != nil {
				return nil, err
			}
			_, parent, err := find(op.Parent)
			if err != nil {
				return nil, err
			}
			if f.SheetOf(parent.ID) != d {
				return nil, errors.New("不能把节点移到另一个画布")
			}
			if err := d.Move(n.ID, parent.ID, -1); err != nil {
				return nil, err
			}
			op.ID = n.ID
		default:
			return nil, fmt.Errorf("未知的修改 %q", op.Op)
		}
	}
	return ops, nil
}

// setFields sets what an add or update gives besides the title.
func setFields(n *model.Node, op aiEdit) {
	if note := strings.TrimSpace(op.Note); note != "" {
		n.Note = note
	}
	if len(op.Tags) > 0 {
		n.Tags = cleanTags(op.Tags)
	}
	for _, l := range op.Links {
		if l = strings.TrimSpace(l); l != "" && !slices.Contains(n.Links, l) {
			n.Links = append(n.Links, l)
		}
	}
	switch op.Status {
	case "":
	case "none":
		n.Task = nil
	default:
		if n.Task == nil {
			n.Task = &model.Task{}
		}
		n.Task.SetStatus(model.Status(op.Status), time.Now())
	}
	switch op.Due {
	case "":
	case "none":
		if n.Task != nil {
			n.Task.Due = nil
		}
	default:
		if t, err := time.ParseInLocation("2006-01-02", op.Due, time.Local); err == nil {
			if n.Task == nil {
				n.Task = &model.Task{Status: model.StatusTodo}
			}
			n.Task.Due = &t
		}
	}
}

// describeEdit says what an edit does, for the proposal card.
func (a *app) describeEdit(op aiEdit, added map[string]string) string {
	title := func(id string) string {
		if t, ok := added[id]; ok {
			return t
		}
		if d := a.file.SheetOf(id); d != nil {
			n, _ := d.Find(id)
			return n.Title
		}
		return id
	}
	var extra []string
	if op.Note != "" {
		extra = append(extra, "备注")
	}
	if len(op.Tags) > 0 {
		extra = append(extra, "标签 #"+strings.Join(op.Tags, " #"))
	}
	switch op.Status {
	case "":
	case "none":
		extra = append(extra, "取消任务")
	default:
		extra = append(extra, "状态："+model.Status(op.Status).Label())
	}
	if op.Due != "" && op.Due != "none" {
		extra = append(extra, "截止 "+op.Due)
	}
	tail := ""
	if len(extra) > 0 {
		tail = "（" + strings.Join(extra, "，") + "）"
	}
	switch op.Op {
	case "add":
		if op.Ref != "" {
			added[op.Ref] = op.Title
		}
		return fmt.Sprintf("＋ 在「%s」下新增「%s」%s", title(op.Parent), op.Title, tail)
	case "update":
		s := fmt.Sprintf("✎ 修改「%s」", title(op.ID))
		if op.Title != "" && op.Title != title(op.ID) {
			s += "，标题改为「" + op.Title + "」"
		}
		return s + tail
	case "delete":
		return fmt.Sprintf("✕ 删除「%s」及其下级", title(op.ID))
	case "move":
		return fmt.Sprintf("→ 把「%s」移到「%s」下", title(op.ID), title(op.Parent))
	}
	return op.Op
}

// aiPanel is the panel's AI tab: the conversation, a field to write in,
// and the settings.
func (a *app) aiPanel(c *ui.Context) {
	a.switchChat()
	t := c.Theme()
	ai := &a.ai
	panel := ui.Column(c.Key("ai")).Fill()
	if files := panel.DroppedFiles(); len(files) > 0 {
		a.later(func() { a.addAttach(files) })
	}
	panel.Children(func() {
		ui.Row(c.Key("ai-head")).Height(toolbarHeight).Padding(0, 10, 0, 16).Gap(6).AlignItems(ui.Center).Shrink(0).
			BorderWidth(0, 0, 1, 0).BorderColor(hairline(t)).DragWindow().Children(func() {
			ui.Box(c).Size(16, 16).Draw(func(p *ui.Painter, r ui.Rect) { drawIcon(p, r, 14, iconSparkle, t.Accent) })
			ui.Text(c, "AI 助手").FontSize(textTitle).Bold().Grow(1)
			if len(ai.msgs) > 0 && ui.Button(c, "新对话").Disabled(ai.busy).Clicked() {
				a.newChat()
			}
			iconButton(c, iconMore, "AI 菜单", "周报、风险扫描、拆解任务、从文本或代码仓库生成，以及设置", 28, !ai.busy).Menu(func(m *ui.Menu) {
				if !a.libOpen {
					a.aiMenu(m)
					m.Separator()
				}
				a.chatsMenu(m)
				m.Separator()
				if m.Item("AI 设置").Chosen() {
					ai.settings = !ai.settings
					a.loadKeyDrafts()
				}
			})
			if iconButton(c, iconClose, "关闭 AI 助手", "关闭 AI 助手", 28, true).Clicked() {
				a.aiOpen = false
			}
		})
		if ai.settings || providerKey(a.cfg) == "" {
			a.aiSettings(c)
			return
		}
		ui.Scroll(c.Key("chat")).Grow(1).TrackScroll(&ai.scroll).Padding(4, 16, 12).Gap(10).Children(func() {
			if len(ai.msgs) == 0 && a.libOpen {
				ui.Text(c, "问问资料库里的内容：AI 会先在本地检索相关资料，再根据它们回答，并说明依据的是哪几条。").
					FontSize(12).TextColor(t.TextMuted).Wrap()
				asks := []string{"我最近收藏了哪些资料？按主题归纳一下", "资料库里有哪些值得深入看的内容？"}
				if clip := a.lib.Find(a.libSel); clip != nil {
					asks = append([]string{"总结选中的这条资料，并找找资料库里相关的内容"}, asks...)
				}
				for _, s := range asks {
					if ui.Button(c.Key(s), s).FillWidth().Clicked() {
						ai.draft = s
						a.aiSend()
					}
				}
			} else if len(ai.msgs) == 0 {
				ui.Text(c, "问问导图里的内容，或让 AI 帮你整理、拆分任务、补充备注。AI 提出的修改要你确认后才会应用。").
					FontSize(12).TextColor(t.TextMuted).Wrap()
				actions := []struct {
					icon  icon
					label string
					run   func()
				}{
					{iconPencil, "生成本周周报", func() { a.aiReport(7) }},
					{iconTarget, "风险扫描", a.aiRiskScan},
					{iconCheck, "把选中的节点拆解成任务", a.aiBreakDown},
					{iconMap, "从文本生成导图…", func() { a.fromTextOpen = true }},
					{iconFolder, "从代码仓库生成架构…", a.askRepo},
				}
				ui.Column(c).Gap(2).Margin(4, -6, 0).Children(func() {
					sectionHeader(c, "常用").Margin(0, 10, 4)
					for _, act := range actions {
						if listRow(c, act.icon, act.label).Clicked() {
							a.later(act.run)
						}
					}
					sectionHeader(c, "试着问").Margin(10, 10, 4)
					for _, s := range []string{"总结这个画布的进展和风险", "给没有备注的功能补上实现要点"} {
						if listRow(c, iconSparkle, s).Clicked() {
							ai.draft = s
							a.aiSend()
						}
					}
				})
			}
			for i := range ai.msgs {
				a.chatBubble(c.Key(i), i)
			}
			if ai.busy && ai.streaming != "" {
				markdownView(c.Key("streaming"), ai.streaming, 13)
			}
			if ai.busy {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Spinner(c)
					status := "思考中…"
					if ai.streaming != "" {
						status = "正在回答…"
					}
					ui.Text(c, status).FontSize(12).TextColor(t.TextMuted)
					if ui.Link(c, "停止", "").FontSize(12).Clicked() && ai.cancel != nil {
						ai.cancel()
					}
				})
			}
		})
		ui.Divider(c)
		ui.Column(c).Padding(10, 16, 14).Gap(8).Children(func() {
			if a.libOpen {
				ui.Text(c, "基于资料库对话；导图里提问时也能检索资料库").FontSize(11).TextColor(t.TextMuted)
			}
			attachRow(c, ai.attach, nil, func(i int) {
				a.later(func() { ai.attach = slices.Delete(ai.attach, i, i+1) })
			})
			in := ui.TextAreaBase(c.Key("draft"), &ai.draft).Lines(2, 6).Placeholder("输入消息，↩ 发送，⇧↩ 换行").Label("AI 消息").
				Padding(8).Radius(8).Border(1, t.Border).Background(t.Background).FontSize(13).
				OnPaste(func(text string) bool { return text == "" && a.pasteAttach() })
			// Return sends, as in chats; Shift+Return, the field's own, is
			// a new line. The field takes keys before shortcuts: its key
			// handler, which comes first, takes Return. A Return that ends
			// an input method's composition never reaches it.
			send := false
			in.HandleInput(func(ev ui.InputEvent) bool {
				if ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyEnter && (ev.Mods == 0 || ev.Mods == ui.Cmd) {
					a.later(a.aiSend)
					return true
				}
				return false
			})
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				if iconButton(c, iconClip, "添加附件", "添加图片或文件（PDF、Word、文本、代码…）", 28, len(ai.attach) < aiAttachMax).Clicked() {
					a.plat.OpenFile("添加附件", aiAttachExts, func(path string) {
						if path != "" {
							a.addAttach([]string{path})
						}
					})
				}
				if !a.libOpen {
					a.scopeChip(c)
				}
				model, _, _ := strings.Cut(providerModelName(a.cfg), "（")
				ui.Text(c, model).FontSize(11).TextColor(t.TextMuted).SingleLine().Ellipsis("…").Grow(1).Shrink(1).MinWidth(0).
					Tooltip(providerModelName(a.cfg) + "（在 AI 设置里更换）")
				if ui.PrimaryButton(c, "发送").Disabled(ai.busy || strings.TrimSpace(ai.draft) == "" && len(ai.attach) == 0).Clicked() {
					send = true
				}
			})
			if send {
				a.aiSend()
			}
		})
	})
}

func aiModelName(id string) string {
	for _, m := range aiModels {
		if m.id == id {
			return m.name
		}
	}
	return id
}

func (a *app) chatBubble(c *ui.Context, i int) {
	t := c.Theme()
	m := a.ai.msgs[i]
	switch m.role {
	case "user":
		ui.Row(c).Children(func() {
			ui.Spacer(c)
			ui.Column(c).Gap(6).Padding(8, 12).Radius(12).Background(t.Accent).MaxWidth(a.panelWidth * 0.8).Children(func() {
				attachRow(c, m.files, m.names, nil)
				ui.Text(c, m.text).FontSize(13).TextColor(t.AccentText).Wrap().Selectable()
			})
		})
	case "error":
		ui.Text(c, m.text).FontSize(12).TextColor(t.Danger).Wrap()
	case "tool":
		ui.Text(c, m.text).FontSize(12).TextColor(t.TextMuted).Wrap()
	default:
		ui.Column(c).Gap(8).Children(func() {
			if m.text != "" {
				markdownView(c, m.text, 13)
				text := m.text
				if ui.Button(c, "复制").FontSize(11).Padding(1, 8).Tooltip("复制这段回复（Markdown）").AlignSelf(ui.Start).Clicked() {
					a.plat.CopyText(text)
					a.tell("已复制")
				}
			}
			if m.proposal != nil {
				a.proposalCard(c, i)
			}
		})
	}
}

func (a *app) proposalCard(c *ui.Context, i int) {
	t := c.Theme()
	m := &a.ai.msgs[i]
	p := m.proposal
	ui.Column(c).Padding(10).Gap(6).Radius(10).Background(t.Background).Border(1, t.Border).Children(func() {
		ui.Text(c, "建议的修改").FontSize(11).Bold().TextColor(t.TextMuted)
		if p.Summary != "" {
			ui.Text(c, p.Summary).FontSize(13).Bold().Wrap()
		}
		if nm := p.NewMap; nm != nil {
			lines := strings.Split(strings.TrimSpace(nm.Outline), "\n")
			if len(lines) > 12 {
				lines = append(lines[:12], fmt.Sprintf("…还有 %d 行", len(lines)-12))
			}
			ui.Text(c, strings.Join(lines, "\n")).FontSize(12).Wrap()
			ui.Text(c, "应用后在「"+filepath.Base(a.currentFolder())+"」里新建并打开").FontSize(11).TextColor(t.TextMuted).Wrap()
		}
		added := map[string]string{}
		for j, op := range p.Operations {
			ui.Text(c.Key(j), a.describeEdit(op, added)).FontSize(12).Wrap()
		}
		if b := m.baseline; b != nil && m.state == proposalPending {
			ui.Text(c, "应用后，代码基线更新到 "+baselineLabel(b)).FontSize(11).TextColor(t.TextMuted).Wrap()
		}
		switch m.state {
		case proposalPending:
			ui.Row(c).Gap(8).Children(func() {
				if ui.PrimaryButton(c, "应用").Clicked() {
					a.answerProposal(i, true)
				}
				if ui.Button(c, "放弃").Clicked() {
					a.answerProposal(i, false)
				}
			})
		case proposalApplied:
			ui.Text(c, "✓ "+m.result).FontSize(12).TextColor(t.Success)
		case proposalDiscarded:
			ui.Text(c, "已放弃").FontSize(12).TextColor(t.TextMuted)
		case proposalFailed:
			ui.Text(c, "✕ "+m.result).FontSize(12).TextColor(t.Danger).Wrap()
		}
	})
}

// loadKeyDrafts fills the settings' fields from the provider chosen.
func (a *app) loadKeyDrafts() {
	if a.cfg.AIProvider == providerDeepSeek {
		a.ai.keyDraft, a.ai.baseDraft = a.cfg.DSKey, a.cfg.DSBaseURL
	} else {
		a.ai.keyDraft, a.ai.baseDraft = a.cfg.AIKey, a.cfg.AIBaseURL
	}
}

// aiSettings is the form of the provider, its API key, its model and the
// API's address.
func (a *app) aiSettings(c *ui.Context) {
	t := c.Theme()
	ai := &a.ai
	deepseek := a.cfg.AIProvider == providerDeepSeek
	ui.Scroll(c.Key("ai-settings")).Grow(1).Padding(8, 16, 16).Gap(12).Children(func() {
		provider := 0
		if deepseek {
			provider = 1
		}
		ui.Field(c, "服务商", func() {
			if ui.Segmented(c, &provider, "Claude", "DeepSeek").FillWidth().Changed() {
				a.cfg.AIProvider = []string{providerClaude, providerDeepSeek}[provider]
				a.cfg.save()
				a.loadKeyDrafts()
			}
		})
		models := aiModels
		envKey, placeholder, base := "ANTHROPIC_API_KEY", "sk-ant-…", "https://api.anthropic.com"
		if deepseek {
			models = deepseekModels
			envKey, placeholder, base = "DEEPSEEK_API_KEY", "sk-…", deepseekBaseURL
			ui.Text(c, "使用深度求索的 DeepSeek 模型。在 platform.deepseek.com 创建 API Key，按用量付费。").
				FontSize(12).TextColor(t.TextMuted).Wrap()
		} else {
			ui.Text(c, "使用 Anthropic 的 Claude 模型。在 console.anthropic.com 创建 API Key，按用量付费。").
				FontSize(12).TextColor(t.TextMuted).Wrap()
		}
		ui.Field(c, "API Key", func() {
			ui.TextInput(c.Key("key"+a.cfg.AIProvider), &ai.keyDraft).Password().Placeholder(placeholder)
		})
		if ai.keyDraft == "" && os.Getenv(envKey) != "" {
			ui.Text(c, "未填写时使用环境变量 "+envKey+"。").FontSize(12).TextColor(t.TextMuted)
		}
		names := make([]string, len(models))
		for i, m := range models {
			names[i] = m.name
		}
		current := providerModelName(a.cfg)
		ui.Field(c, "模型", func() {
			if ui.Select(c.Key("model"+a.cfg.AIProvider), &current, names).Label("模型").Changed() {
				for _, m := range models {
					if m.name == current {
						if deepseek {
							a.cfg.DSModel = m.id
						} else {
							a.cfg.AIModel = m.id
						}
						a.cfg.save()
					}
				}
			}
		})
		ui.Field(c, "接口地址（可选）", func() {
			ui.TextInput(c.Key("base"+a.cfg.AIProvider), &ai.baseDraft).Placeholder(base)
		})
		ui.Text(c, "使用代理或中转服务时填写，留空使用官方地址。").FontSize(12).TextColor(t.TextMuted)
		ui.Text(c, "API Key 保存在本机的应用配置文件中（仅当前用户可读）。").FontSize(12).TextColor(t.TextMuted).Wrap()
		if len(ai.msgs) > 0 && ai.provider != a.cfg.AIProvider {
			ui.Text(c, "换了服务商，下一条消息会开始新的对话。").FontSize(12).TextColor(t.Warning).Wrap()
		}
		ui.Row(c).Gap(8).Children(func() {
			if ui.PrimaryButton(c, "保存").Clicked() {
				key := strings.TrimSpace(ai.keyDraft)
				url := strings.TrimRight(strings.TrimSpace(ai.baseDraft), "/")
				if deepseek {
					a.cfg.DSKey, a.cfg.DSBaseURL = key, url
				} else {
					a.cfg.AIKey, a.cfg.AIBaseURL = key, url
				}
				a.cfg.save()
				ai.settings = false
			}
			if providerKey(a.cfg) != "" && ui.Button(c, "取消").Clicked() {
				ai.settings = false
			}
		})
	})
}

const newMapDescription = "向用户提议新建一张导图（不改当前导图）。用户确认后才会创建并打开它。"

// newMapSchema is the JSON schema of propose_new_map's input.
func newMapSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":    map[string]any{"type": "string", "description": "导图的名字，简短，也是文件名"},
			"outline": map[string]any{"type": "string", "description": "中心主题下的节点，Markdown 列表：两个空格缩进一层，\"- [ ] 文字\" 是待做任务"},
		},
		"required":             []string{"name", "outline"},
		"additionalProperties": false,
	}
}

// newMapToolClaude is the tool Claude proposes a new map with.
func newMapToolClaude() anthropic.BetaToolUnionParam {
	schema := newMapSchema()
	return anthropic.BetaToolUnionParam{OfTool: &anthropic.BetaToolParam{
		Name:        "propose_new_map",
		Description: anthropic.String(newMapDescription),
		Strict:      anthropic.Bool(true),
		InputSchema: anthropic.BetaToolInputSchemaParam{
			Properties:  schema["properties"],
			Required:    schema["required"].([]string),
			ExtraFields: map[string]any{"additionalProperties": false},
		},
	}}
}

// createMapFromAI creates the map the AI proposed, in the folder of the
// open map, and opens it.
func (a *app) createMapFromAI(nm *aiNewMap) (string, error) {
	name := strings.NewReplacer("/", "-", "\\", "-").Replace(strings.TrimSpace(nm.Name))
	if name == "" {
		name = "新导图"
	}
	path := freeMapPath(a.currentFolder(), name)
	f := model.NewFile(mapName(path))
	f.Sheets[0].Root.Children = model.ParseOutline(nm.Outline)
	if err := f.Save(path); err != nil {
		return "", err
	}
	_, _ = a.versions.Save(path, f, versions.AI, "AI 新建导图", a.now())
	a.refreshFiles()
	if err := a.open(path); err != nil {
		return "", err
	}
	a.tell("已新建导图「" + mapName(path) + "」")
	return mapName(path), nil
}

// aiScopes name what the AI sees of the map, and tell of it.
var aiScopes = []struct{ name, desc string }{
	{"本画布", "当前画布的全部节点"},
	{"选中节点", "选中的节点和它下面的分支"},
	{"全部画布", "这个文件的所有画布"},
	{"不带导图", "不发送导图，问别的或新建导图"},
}

// scopeChip is the small button by the message that chooses what the AI
// sees of the map, from a menu.
func (a *app) scopeChip(c *ui.Context) {
	t := c.Theme()
	ai := &a.ai
	chip := ui.Row(c.Key("ai-scope")).Gap(4).Padding(3, 8).Radius(999).AlignItems(ui.Center).Shrink(0).Border(1, t.Border).
		Cursor(ui.CursorPointer).Label("AI 范围").Tooltip("AI 能看到的内容：" + aiScopes[ai.scope].desc)
	if chip.Hovered() {
		chip.Background(t.SurfaceHover)
	} else {
		chip.Background(t.Surface)
	}
	chip.Menu(func(m *ui.Menu) {
		for i, s := range aiScopes {
			if m.Item(s.name + "  ·  " + s.desc).Checked(ai.scope == i).Chosen() {
				ai.scope = i
			}
		}
	})
	chip.Children(func() {
		ic := iconMap
		if ai.scope == scopeNone {
			ic = iconText
		}
		glyph(c, ic, 12, t.TextMuted)
		ui.Text(c, aiScopes[ai.scope].name).FontSize(11).TextColor(t.Text).NoWrap().PassThrough()
		ui.Text(c, "▾").FontSize(9).TextColor(t.TextMuted).PassThrough()
	})
}
