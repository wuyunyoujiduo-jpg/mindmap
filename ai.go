package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
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
	result    string // what applying did
}

// aiState is the conversation of the AI panel.
type aiState struct {
	provider string                       // of the conversation, which stays with it
	claude   []anthropic.BetaMessageParam // the conversation, as sent to Claude
	ds       []dsMessage                  // or to DeepSeek
	msgs     []chatMsg
	draft    string
	busy     bool
	scope    int
	settings bool   // the settings form is shown
	extra    string // context an action adds to the next message, after the map
	// answers answer the proposals of the last reply; the next message
	// sends them first, as the APIs require.
	answers   []toolAnswer
	cancel    context.CancelFunc
	scroll    ui.ScrollState
	keyDraft  string
	baseDraft string
}

// aiSender sends a request to Claude; tests replace it.
type aiSender func(ctx context.Context, cfg config, params anthropic.BetaMessageNewParams) (*anthropic.BetaMessage, error)

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

// sendToClaude calls the Messages API with the official SDK.
func sendToClaude(ctx context.Context, cfg config, params anthropic.BetaMessageNewParams) (*anthropic.BetaMessage, error) {
	opts := []option.RequestOption{option.WithRequestTimeout(5 * time.Minute)}
	if key := aiKey(cfg); key != "" {
		opts = append(opts, option.WithAPIKey(key))
	}
	if cfg.AIBaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.AIBaseURL))
	}
	client := anthropic.NewClient(opts...)
	return client.Beta.Messages.New(ctx, params)
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

const aiSystemPrompt = `你是「MindMap」思维导图项目管理工具里的助手。用户用思维导图管理项目：模块、功能、实现细节、待优化点和任务。

每条用户消息都附带导图的当前快照：一份缩进大纲，每个节点前面的 [id] 是它的 ID，后面可能有标签（#标签）、任务（状态、截止日期）和备注。快照总是最新的，以它为准。

回答用户的问题时，简洁地用中文回答，可以引用节点标题。

当用户要求修改导图（添加、整理、拆分任务、补充备注、改状态、移动、删除等）时，调用 propose_edits 工具，把这一次的所有修改放在一次调用里。用户会先看到修改清单，确认后才会应用，所以不要在文字里重复列出修改内容，只用一两句话说明思路。
- 引用已有节点时，必须使用快照里的 ID，不要编造 ID。
- 新增的节点给一个 ref（如 "r1"），后面的修改可以用这个 ref 作为父节点。
- 标题保持简短，细节写进备注。
- 不需要修改的字段留空字符串（tags 留空数组）。
- 不要删除用户没有要求删除的内容。`

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

// aiSend sends the draft, with the map, and waits for the reply in the
// background.
func (a *app) aiSend() {
	ai := &a.ai
	text := strings.TrimSpace(ai.draft)
	if text == "" || ai.busy || a.doc == nil {
		return
	}
	if providerKey(a.cfg) == "" {
		ai.settings = true
		ai.msgs = append(ai.msgs, chatMsg{role: "error", text: "请先在设置中填写 API Key。"})
		return
	}
	if ai.provider != a.cfg.AIProvider && len(ai.claude)+len(ai.ds) > 0 {
		*ai = aiState{scope: ai.scope, draft: ai.draft} // another provider: a new conversation
	}
	ai.provider = a.cfg.AIProvider
	// Proposals left unanswered are discarded by sending on.
	for i := range ai.msgs {
		if ai.msgs[i].proposal != nil && ai.msgs[i].state == proposalPending {
			a.answerProposal(i, false)
		}
	}
	answers := ai.answers
	ai.answers = nil
	snapshot := "导图快照：\n" + a.snapshot(ai.scope)
	if ai.extra != "" { // what an action gives besides the map
		snapshot += "\n\n" + ai.extra
		ai.extra = ""
	}
	ai.msgs = append(ai.msgs, chatMsg{role: "user", text: text})
	ai.draft = ""
	ai.busy = true
	ai.scroll.Y = 1e9

	ctx, cancel := context.WithCancel(context.Background())
	ai.cancel = cancel
	cfg := a.cfg
	claudeLen, dsLen := len(ai.claude), len(ai.ds)
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
		ai.ds = append(ai.ds, dsMessage{Role: "user", Content: snapshot + "\n\n" + text})
		req := a.deepseekRequest()
		send := a.sendDeepSeek
		a.async(func() (any, error) {
			resp, err := send(ctx, cfg, req)
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
	blocks = append(blocks, anthropic.NewBetaTextBlock(snapshot), anthropic.NewBetaTextBlock(text))
	ai.claude = append(ai.claude, anthropic.NewBetaUserMessage(blocks...))
	params := a.aiParams()
	send := a.sendAI
	a.async(func() (any, error) {
		resp, err := send(ctx, cfg, params)
		if err != nil {
			return nil, err
		}
		return claudeReply(resp), nil
	}, done)
}

func (a *app) aiParams() anthropic.BetaMessageNewParams {
	m := aiModel(a.cfg)
	p := anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(m),
		MaxTokens: 16000,
		System:    []anthropic.BetaTextBlockParam{{Text: aiSystemPrompt}},
		Messages:  slices.Clone(a.ai.claude),
		Tools:     []anthropic.BetaToolUnionParam{proposeEditsTool()},
		OutputConfig: anthropic.BetaOutputConfigParam{
			Effort: anthropic.BetaOutputConfigEffortMedium,
		},
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
			r.calls = append(r.calls, aiCall{id: b.ID, name: b.Name, input: b.JSON.Input.Raw()})
		}
	}
	param := resp.ToParam()
	r.commit = func(ai *aiState) { ai.claude = append(ai.claude, param) }
	return r
}

// aiReceive shows a reply: its text, and the edits it proposes.
func (a *app) aiReceive(r aiReply, err error) {
	ai := &a.ai
	ai.busy, ai.cancel = false, nil
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
	shown := false
	for _, call := range r.calls {
		var p aiProposal
		if call.name != "propose_edits" || json.Unmarshal([]byte(call.input), &p) != nil {
			ai.msgs = append(ai.msgs, chatMsg{role: "error", text: "无法读取 AI 提出的修改"})
			ai.answers = append(ai.answers, toolAnswer{id: call.id, text: "无法解析修改，未应用。", isError: true})
			continue
		}
		msg := chatMsg{role: "assistant", proposal: &p, toolUseID: call.id}
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
	return "网络错误：" + err.Error()
}

// answerProposal applies or discards the proposal of message i, and
// prepares the tool result the next request starts with.
func (a *app) answerProposal(i int, apply bool) {
	ai := &a.ai
	m := &ai.msgs[i]
	if m.state != proposalPending {
		return
	}
	result := "用户没有应用这些修改。"
	if apply {
		n, err := a.applyEdits(m.proposal.Operations)
		if err != nil {
			m.state, m.result = proposalFailed, err.Error()
			result = "应用失败：" + err.Error()
		} else {
			m.state, m.result = proposalApplied, fmt.Sprintf("已应用 %d 项修改（可撤销）", n)
			result = fmt.Sprintf("用户已应用这 %d 项修改。", n)
		}
	} else {
		m.state = proposalDiscarded
	}
	ai.answers = append(ai.answers, toolAnswer{id: m.toolUseID, text: result})
}

// applyEdits makes the proposed edits, all or none, as one step of the
// history, and returns how many it made.
func (a *app) applyEdits(ops []aiEdit) (int, error) {
	if len(ops) == 0 {
		return 0, errors.New("没有修改")
	}
	if _, err := editFile(a.file.Clone(), ops); err != nil { // all or none
		return 0, err
	}
	var done []aiEdit
	a.mutate("", func() { done, _ = editFile(a.file, ops) })
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
	t := c.Theme()
	ai := &a.ai
	ui.Column(c.Key("ai")).Fill().Children(func() {
		ui.Row(c).Padding(12, 16, 6).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "✦ AI 助手").Bold().Grow(1)
			if len(ai.msgs) > 0 && ui.Button(c, "新对话").Disabled(ai.busy).Clicked() {
				a.ai = aiState{scope: ai.scope}
			}
			ui.MenuButton(c, "⋯", func(m *ui.Menu) {
				a.aiMenu(m)
				m.Separator()
				if m.Item("AI 设置").Chosen() {
					ai.settings = !ai.settings
					a.loadKeyDrafts()
				}
			}).Label("AI 菜单").Tooltip("周报、风险扫描、拆解任务、从文本或代码仓库生成，以及设置").Disabled(ai.busy)
			if ui.Button(c, "✕").Label("关闭").Tooltip("关闭 AI 助手").Clicked() {
				a.aiOpen = false
			}
		})
		if ai.settings || providerKey(a.cfg) == "" {
			a.aiSettings(c)
			return
		}
		ui.Scroll(c.Key("chat")).Grow(1).TrackScroll(&ai.scroll).Padding(4, 16, 12).Gap(10).Children(func() {
			if len(ai.msgs) == 0 {
				ui.Text(c, "问问导图里的内容，或让 AI 帮你整理、拆分任务、补充备注。AI 提出的修改要你确认后才会应用。").
					FontSize(12).TextColor(t.TextMuted).Wrap()
				actions := []struct {
					label string
					run   func()
				}{
					{"📋 生成本周周报", func() { a.aiReport(7) }},
					{"⚠️ 风险扫描", a.aiRiskScan},
					{"🔨 把选中的节点拆解成任务", a.aiBreakDown},
					{"📝 从文本生成导图…", func() { a.fromTextOpen = true }},
					{"📁 从代码仓库生成架构…", a.askRepo},
				}
				for _, act := range actions {
					if ui.Button(c.Key(act.label), act.label).FillWidth().Clicked() {
						a.later(act.run)
					}
				}
				for _, s := range []string{"总结这个画布的进展和风险", "给没有备注的功能补上实现要点"} {
					if ui.Button(c.Key(s), s).FillWidth().Clicked() {
						ai.draft = s
						a.aiSend()
					}
				}
			}
			for i := range ai.msgs {
				a.chatBubble(c.Key(i), i)
			}
			if ai.busy {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Spinner(c)
					ui.Text(c, "思考中…").FontSize(12).TextColor(t.TextMuted)
					if ui.Link(c, "停止", "").FontSize(12).Clicked() && ai.cancel != nil {
						ai.cancel()
					}
				})
			}
		})
		ui.Divider(c)
		ui.Column(c).Padding(10, 16, 14).Gap(8).Children(func() {
			ui.Segmented(c, &ai.scope, "当前画布", "选中节点", "整个文件").FillWidth()
			in := ui.TextAreaBase(c.Key("draft"), &ai.draft).Lines(2, 6).Placeholder("输入消息，⌘↩ 发送").
				Padding(8).Radius(8).Border(1, t.Border).Background(t.Background).FontSize(13)
			send := in.Shortcut(ui.Cmd, ui.KeyEnter)
			ui.Row(c).Children(func() {
				ui.Text(c, providerModelName(a.cfg)).FontSize(11).TextColor(t.TextMuted).Grow(1)
				if ui.PrimaryButton(c, "发送").Disabled(ai.busy || strings.TrimSpace(ai.draft) == "").Clicked() {
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
			ui.Box(c).Padding(8, 12).Radius(12).Background(t.Accent).MaxWidth(a.panelWidth * 0.8).Children(func() {
				ui.Text(c, m.text).FontSize(13).TextColor(t.AccentText).Wrap().Selectable()
			})
		})
	case "error":
		ui.Text(c, m.text).FontSize(12).TextColor(t.Danger).Wrap()
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
		added := map[string]string{}
		for j, op := range p.Operations {
			ui.Text(c.Key(j), a.describeEdit(op, added)).FontSize(12).Wrap()
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
