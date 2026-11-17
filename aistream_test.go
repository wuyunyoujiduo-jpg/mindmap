package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// shortWaits has a reply asked for whole wait deadline, and a stream
// idle with nothing coming, for the test.
func shortWaits(t *testing.T, deadline, idle time.Duration) {
	t.Helper()
	wasDeadline, wasIdle := aiDeadline, aiIdle
	aiDeadline, aiIdle = deadline, idle
	t.Cleanup(func() { aiDeadline, aiIdle = wasDeadline, wasIdle })
}

// claudeEvent sends an event of a streamed reply, at once.
func claudeEvent(w http.ResponseWriter, name, data string) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data)
	w.(http.Flusher).Flush()
}

func claudeStreamHead(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	claudeEvent(w, "message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5-5","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}}`)
	claudeEvent(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
}

func claudeStreamText(w http.ResponseWriter, text string) {
	claudeEvent(w, "content_block_delta", fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":%q}}`, text))
}

func claudeStreamEnd(w http.ResponseWriter) {
	claudeEvent(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
	claudeEvent(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":3}}`)
	claudeEvent(w, "message_stop", `{"type":"message_stop"}`)
}

// wait is a pause of the server's, cut short when the request is given up.
func wait(r *http.Request, d time.Duration) bool {
	select {
	case <-r.Context().Done():
		return false
	case <-time.After(d):
		return true
	}
}

func claudeParams() anthropic.BetaMessageNewParams {
	return anthropic.BetaMessageNewParams{Model: "claude-opus-5-5", MaxTokens: 1000,
		Messages: []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("你好"))}}
}

// A reply streamed takes as long as it takes: longer than a reply asked
// for whole may, with stretches of the server's pings alone, as while the
// model thinks. It is given up when nothing at all comes for a while,
// and said so; 停止 stops it as before.
func TestClaudeStreamHasNoDeadline(t *testing.T) {
	shortWaits(t, 150*time.Millisecond, 400*time.Millisecond)
	mode := "long"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = jsonDecode(r, &body)
		if body["stream"] != true { // asked for whole: too slow for it
			wait(r, 600*time.Millisecond)
			fmt.Fprint(w, claudeText("晚了"))
			return
		}
		claudeStreamHead(w)
		claudeStreamText(w, "你好，")
		switch mode {
		case "long":
			// Pings alone for longer than a stream may idle, each within it.
			for i := 0; i < 8; i++ {
				if !wait(r, 80*time.Millisecond) {
					return
				}
				claudeEvent(w, "ping", `{"type":"ping"}`)
			}
			claudeStreamText(w, "我在。")
			claudeStreamEnd(w)
		case "stalls":
			<-r.Context().Done()
		}
	}))
	defer srv.Close()
	cfg := config{AIKey: "sk-ant-test", AIBaseURL: srv.URL}

	var pieces []string
	began := time.Now()
	msg, err := sendToClaude(context.Background(), cfg, claudeParams(), func(s string) { pieces = append(pieces, s) })
	if err != nil {
		t.Fatalf("a long stream cut: %v", err)
	}
	if took := time.Since(began); took < 600*time.Millisecond {
		t.Fatalf("the stream took %v: no longer than the waits it is to outlast", took)
	}
	if strings.Join(pieces, "") != "你好，我在。" || len(msg.Content) != 1 || msg.Content[0].Text != "你好，我在。" || msg.StopReason != anthropic.BetaStopReasonEndTurn {
		t.Fatalf("message %+v pieces %q", msg, pieces)
	}

	// Nothing comes: given up, and said as what it is.
	mode = "stalls"
	began = time.Now()
	_, err = sendToClaude(context.Background(), cfg, claudeParams(), func(string) {})
	if !errors.Is(err, errIdle) || aiError(err) != string(errIdle) {
		t.Fatalf("a stream that stalled: %v", err)
	}
	if took := time.Since(began); took < 400*time.Millisecond || took > 5*time.Second {
		t.Fatalf("given up after %v", took)
	}
	// Stopped by the user while it comes: that, and not a stream that
	// stalled.
	ctx, stop := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, stop)
	_, err = sendToClaude(ctx, cfg, claudeParams(), func(string) {})
	if !errors.Is(err, context.Canceled) || aiError(err) != "已停止。" {
		t.Fatalf("stopped: %v", err)
	}

	// A reply asked for whole still has its time to come in.
	if _, err := sendToClaude(context.Background(), cfg, claudeParams(), nil); err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("a reply asked for whole, too slow: %v", err)
	}
}

// DeepSeek's the same.
func TestDeepSeekStreamHasNoDeadline(t *testing.T) {
	shortWaits(t, 150*time.Millisecond, 400*time.Millisecond)
	mode := "long"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = jsonDecode(r, &body)
		if body["stream"] != true {
			wait(r, 600*time.Millisecond)
			fmt.Fprint(w, `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"晚了"}}]}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		send := func(s string) {
			fmt.Fprintf(w, "%s\n\n", s)
			w.(http.Flusher).Flush()
		}
		send(`data: {"choices":[{"delta":{"content":"你好，"}}]}`)
		if mode == "stalls" {
			<-r.Context().Done()
			return
		}
		for i := 0; i < 8; i++ {
			if !wait(r, 80*time.Millisecond) {
				return
			}
			send(": keep-alive") // while it thinks
		}
		send(`data: {"choices":[{"delta":{"content":"我在。"},"finish_reason":"stop"}]}`)
		send("data: [DONE]")
	}))
	defer srv.Close()
	cfg := config{AIProvider: providerDeepSeek, DSKey: "sk", DSBaseURL: srv.URL}
	req := dsRequest{Model: "deepseek-flash", Messages: []dsMessage{{Role: "user", Content: "你好"}}}

	began := time.Now()
	resp, err := sendToDeepSeek(context.Background(), cfg, req, func(string) {})
	if err != nil || resp.Choices[0].Message.Content != "你好，我在。" {
		t.Fatalf("a long stream cut: %v %+v", err, resp)
	}
	if took := time.Since(began); took < 600*time.Millisecond {
		t.Fatalf("the stream took %v", took)
	}
	mode = "stalls"
	if _, err := sendToDeepSeek(context.Background(), cfg, req, func(string) {}); !errors.Is(err, errIdle) || aiError(err) != string(errIdle) {
		t.Fatalf("a stream that stalled: %v", err)
	}
	ctx, stop := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, stop)
	if _, err := sendToDeepSeek(ctx, cfg, req, func(string) {}); !errors.Is(err, context.Canceled) || aiError(err) != "已停止。" {
		t.Fatalf("stopped: %v", err)
	}
	if _, err := sendToDeepSeek(context.Background(), cfg, req, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("a reply asked for whole, too slow: %v", err)
	}
}
