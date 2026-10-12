package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func pngBytes(t *testing.T) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 40, 30))
	for x := range 40 {
		img.Set(x, 10, color.RGBA{200, 50, 50, 255})
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAIAttachments(t *testing.T) {
	a, tt := newTestApp(t)
	plat := a.plat.(*fakePlatform)
	fake := &fakeClaude{replies: []string{`{
		"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-opus-5-5",
		"stop_reason": "end_turn",
		"content": [{"type": "text", "text": "图里是一条红线。"}],
		"usage": {"input_tokens": 10, "output_tokens": 5}
	}`}}
	a.sendAI = fake.send
	a.cfg.AIKey = "sk-ant-test"
	if err := tt.Click("AI 助手"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	// A document picked with the paperclip, and a screenshot pasted.
	dir := t.TempDir()
	notes := filepath.Join(dir, "需求.md")
	if err := os.WriteFile(notes, []byte("# 需求\n登录要支持扫码"), 0o644); err != nil {
		t.Fatal(err)
	}
	plat.openPath = notes
	if err := tt.Click("添加附件"); err != nil {
		t.Fatal(err)
	}
	plat.clipImage = pngBytes(t)
	if !a.pasteAttach() {
		t.Fatal("paste took nothing")
	}
	tt.Frame()
	if len(a.ai.attach) != 2 || !tt.HasText("需求.md") {
		t.Fatalf("attached %d: %q", len(a.ai.attach), tt.Texts())
	}
	snapshot(t, tt, "ai-attach")

	// Sent with no text: the picture as an image, the document's text.
	a.aiSend()
	tt.Frame()
	if len(fake.requests) != 1 {
		t.Fatal("not sent")
	}
	msgs := fake.requests[0].Messages
	body, _ := json.Marshal(msgs[len(msgs)-1])
	for _, want := range []string{`"type":"image"`, `"media_type":"image/png"`, "附件「需求.md」", "登录要支持扫码", "请看附件"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("no %q in %s", want, body)
		}
	}
	if len(a.ai.attach) != 0 || !tt.HasText("图里是一条红线。") {
		t.Fatalf("after sending: %d attached, %q", len(a.ai.attach), tt.Texts())
	}

	// Kept: the names of the files, taken up again.
	a.saveChat()
	chats := loadChats(a.ai.key)
	if len(chats) == 0 || len(chats[0].Msgs[0].Files) != 2 {
		t.Fatalf("kept %+v", chats)
	}
	a.restoreChat(a.ai.key, chats[0])
	tt.Frame()
	if !tt.HasText("截图.png") {
		t.Fatalf("names not shown: %q", tt.Texts())
	}

	// DeepSeek does not see pictures.
	a.cfg.AIProvider = providerDeepSeek
	a.cfg.DSKey = "sk-test"
	plat.clipImage = pngBytes(t)
	a.pasteAttach()
	a.aiSend()
	tt.Frame()
	if !tt.HasText(errNoImages.Error()) || len(a.ai.attach) != 1 {
		t.Fatalf("deepseek with a picture: %q", tt.Texts())
	}
}

// Return sends the message; Shift+Return is a new line in it.
func TestAIReturnSends(t *testing.T) {
	a, tt := newTestApp(t)
	fake := &fakeClaude{replies: []string{claudeText("好的")}}
	a.sendAI = fake.send
	a.cfg.AIKey = "sk-ant-test"
	if err := tt.Click("AI 助手"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("AI 消息"); err != nil {
		t.Fatal(err)
	}
	tt.Type("第一行")
	tt.Key(ui.Shift, ui.KeyEnter)
	tt.Type("第二行")
	tt.Frame()
	if a.ai.draft != "第一行\n第二行" || len(fake.requests) != 0 {
		t.Fatalf("draft %q, sent %d", a.ai.draft, len(fake.requests))
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if len(fake.requests) != 1 || a.ai.draft != "" {
		t.Fatalf("Return did not send: %d, draft %q", len(fake.requests), a.ai.draft)
	}
}
