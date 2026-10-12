package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// DeepSeek is called through its OpenAI-compatible Chat Completions API,
// with plain HTTP: POST {base}/chat/completions.

const deepseekBaseURL = "https://api.deepseek.com"

// deepseekModels are the DeepSeek models offered, the first being the
// default: deepseek-flash serves DeepSeek V4.1 Flash.
var deepseekModels = []struct{ id, name string }{
	{"deepseek-flash", "DeepSeek V4.1 Flash（推荐）"},
	{"deepseek-v4-pro", "DeepSeek V4 Pro"},
}

type dsMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	// ReasoningContent is the thinking of an assistant message. DeepSeek
	// wants it back on assistant messages that call tools.
	ReasoningContent string       `json:"reasoning_content,omitempty"`
	ToolCalls        []dsToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string       `json:"tool_call_id,omitempty"`
}

type dsToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type dsTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

type dsRequest struct {
	Model     string      `json:"model"`
	Messages  []dsMessage `json:"messages"`
	Tools     []dsTool    `json:"tools,omitempty"`
	MaxTokens int         `json:"max_tokens,omitempty"`
	Stream    bool        `json:"stream"`
	// ResponseFormat asks for a JSON object: {"type": "json_object"}.
	ResponseFormat *dsFormat `json:"response_format,omitempty"`
}

type dsFormat struct {
	Type string `json:"type"`
}

type dsResponse struct {
	Choices []struct {
		Message      dsMessage `json:"message"`
		FinishReason string    `json:"finish_reason"`
	} `json:"choices"`
}

// dsError is an error of the DeepSeek API, with its HTTP status.
type dsError struct {
	Status  int
	Message string
}

func (e *dsError) Error() string { return fmt.Sprintf("%d %s", e.Status, e.Message) }

// deepseekSender sends a request to DeepSeek; tests point it at a server
// of their own through the base URL.
type deepseekSender func(ctx context.Context, cfg config, req dsRequest, onText func(string)) (*dsResponse, error)

// sendToDeepSeek calls the API, streaming the reply when onText takes its
// text.
func sendToDeepSeek(ctx context.Context, cfg config, req dsRequest, onText func(string)) (*dsResponse, error) {
	req.Stream = onText != nil
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	base := cfg.DSBaseURL
	if base == "" {
		base = deepseekBaseURL
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	hr.Header.Set("Content-Type", "application/json")
	hr.Header.Set("Authorization", "Bearer "+deepseekKey(cfg))
	resp, err := http.DefaultClient.Do(hr)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK && strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		return readDeepSeekStream(resp.Body, onText)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		msg := strings.TrimSpace(string(data))
		if json.Unmarshal(data, &e) == nil && e.Error.Message != "" {
			msg = e.Error.Message
		}
		return nil, &dsError{Status: resp.StatusCode, Message: msg}
	}
	var out dsResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("无法解析 DeepSeek 的回复：%w", err)
	}
	if len(out.Choices) == 0 {
		return nil, fmt.Errorf("DeepSeek 没有返回内容")
	}
	return &out, nil
}

func deepseekKey(cfg config) string {
	if cfg.DSKey != "" {
		return cfg.DSKey
	}
	return os.Getenv("DEEPSEEK_API_KEY")
}

func deepseekModel(cfg config) string {
	if cfg.DSModel != "" {
		return cfg.DSModel
	}
	return deepseekModels[0].id
}

// deepseekRequest is the request of the conversation so far.
func (a *app) deepseekRequest() dsRequest {
	tool := dsTool{Type: "function"}
	tool.Function.Name = "propose_edits"
	tool.Function.Description = proposeEditsDescription
	tool.Function.Parameters = editsSchema()
	msgs := append([]dsMessage{{Role: "system", Content: aiSystem()}}, a.ai.ds...)
	tools := libraryToolsDeepSeek()
	if a.aiEdits() {
		tools = append([]dsTool{tool}, tools...)
	}
	return dsRequest{Model: deepseekModel(a.cfg), Messages: msgs, Tools: tools, MaxTokens: 16000}
}

// deepseekReply turns DeepSeek's answer into what the panel shows.
func deepseekReply(resp *dsResponse) aiReply {
	choice := resp.Choices[0]
	m := choice.Message
	r := aiReply{truncated: choice.FinishReason == "length"}
	if s := strings.TrimSpace(m.Content); s != "" {
		r.texts = append(r.texts, s)
	}
	for _, tc := range m.ToolCalls {
		r.calls = append(r.calls, aiCall{id: tc.ID, name: tc.Function.Name, input: tc.Function.Arguments})
	}
	if len(m.ToolCalls) == 0 {
		m.ReasoningContent = "" // only turns that call tools send it back
	}
	m.Role = "assistant"
	r.commit = func(ai *aiState) { ai.ds = append(ai.ds, m) }
	return r
}

func deepseekError(err error) string {
	if e, ok := err.(*dsError); ok {
		switch e.Status {
		case 401:
			return "DeepSeek API Key 无效，请在设置中检查。"
		case 402:
			return "DeepSeek 账户余额不足。"
		case 429:
			return "请求太频繁，请稍后再试。"
		case 500, 502, 503:
			return "DeepSeek 服务繁忙，请稍后再试。"
		}
		return "请求失败（" + e.Error() + "）"
	}
	return ""
}

// readDeepSeekStream reads a streamed reply, server-sent events of pieces
// of the message, into the whole message.
func readDeepSeekStream(r io.Reader, onText func(string)) (*dsResponse, error) {
	var msg dsMessage
	msg.Role = "assistant"
	finish := ""
	var calls []dsToolCall
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 32<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		data = strings.TrimSpace(data)
		if data == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
					ToolCalls        []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil {
			return nil, &dsError{Status: 500, Message: chunk.Error.Message}
		}
		for _, ch := range chunk.Choices {
			d := ch.Delta
			if d.Content != "" {
				msg.Content += d.Content
				if onText != nil {
					onText(d.Content)
				}
			}
			msg.ReasoningContent += d.ReasoningContent
			for _, tc := range d.ToolCalls {
				for len(calls) <= tc.Index {
					calls = append(calls, dsToolCall{Type: "function"})
				}
				c := &calls[tc.Index]
				if tc.ID != "" {
					c.ID = tc.ID
				}
				c.Function.Name += tc.Function.Name
				c.Function.Arguments += tc.Function.Arguments
			}
			if ch.FinishReason != nil {
				finish = *ch.FinishReason
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	msg.ToolCalls = calls
	out := &dsResponse{}
	out.Choices = append(out.Choices, struct {
		Message      dsMessage `json:"message"`
		FinishReason string    `json:"finish_reason"`
	}{msg, finish})
	return out, nil
}
