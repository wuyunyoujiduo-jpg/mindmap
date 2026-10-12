package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// Conversations with the AI are kept, by map and for the library, in the
// app's folder: a map opened again shows its last conversation, and the
// ones before are a menu away. A conversation taken up again is sent to
// the model as its text, the map's snapshot coming with the next message.

// chatsKept is how many conversations of a map are kept.
const chatsKept = 30

// savedMsg is a message of a conversation kept.
type savedMsg struct {
	Role     string      `json:"role"`
	Text     string      `json:"text,omitempty"`
	Proposal *aiProposal `json:"proposal,omitempty"`
	State    int         `json:"state,omitempty"`
	Result   string      `json:"result,omitempty"`
	Files    []string    `json:"files,omitempty"` // the names of the files attached
}

// savedChat is a conversation kept.
type savedChat struct {
	ID      string     `json:"id"`
	Title   string     `json:"title"`
	Updated time.Time  `json:"updated"`
	Msgs    []savedMsg `json:"msgs"`
}

// chatKey is what the conversation shown is about: the library, or the
// map open.
func (a *app) chatKey() string {
	if a.libOpen || a.doc == nil {
		return "library:" + a.dir
	}
	return a.path
}

func chatsPath(key string) string {
	p := configPath()
	if p == "" {
		return ""
	}
	h := sha1.Sum([]byte(key))
	return filepath.Join(filepath.Dir(p), "chats", hex.EncodeToString(h[:8])+".json")
}

// loadChats reads the conversations kept of a key, the latest first.
func loadChats(key string) []savedChat {
	path := chatsPath(key)
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var chats []savedChat
	_ = json.Unmarshal(data, &chats)
	return chats
}

// saveChat keeps the conversation shown.
func (a *app) saveChat() {
	ai := &a.ai
	if len(ai.msgs) == 0 || ai.key == "" {
		return
	}
	path := chatsPath(ai.key)
	if path == "" {
		return
	}
	if ai.chatID == "" {
		ai.chatID = model.NewID()
	}
	chat := savedChat{ID: ai.chatID, Updated: a.now()}
	for _, m := range ai.msgs {
		if chat.Title == "" && m.role == "user" {
			chat.Title = m.text
		}
		chat.Msgs = append(chat.Msgs, savedMsg{Role: m.role, Text: m.text, Proposal: m.proposal, State: m.state, Result: m.result,
			Files: append(attachNames(m.files), m.names...)})
	}
	if r := []rune(strings.Join(strings.Fields(chat.Title), " ")); len(r) > 40 {
		chat.Title = string(r[:40]) + "…"
	} else {
		chat.Title = string(r)
	}
	chats := slices.DeleteFunc(loadChats(ai.key), func(c savedChat) bool { return c.ID == chat.ID })
	chats = append([]savedChat{chat}, chats...)
	if len(chats) > chatsKept {
		chats = chats[:chatsKept]
	}
	data, err := json.Marshal(chats)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	_ = os.WriteFile(path, data, 0o600)
}

// switchChat shows the last conversation about what is shown now, when it
// is not the one shown.
func (a *app) switchChat() {
	key := a.chatKey()
	if a.ai.key == key || a.ai.busy {
		return
	}
	scope := a.ai.scope
	if chats := loadChats(key); len(chats) > 0 {
		a.restoreChat(key, chats[0])
	} else {
		a.ai = aiState{scope: scope, key: key}
	}
	a.ai.scope = scope
}

// newChat starts a conversation, the one shown being kept.
func (a *app) newChat() {
	a.ai = aiState{scope: a.ai.scope, key: a.chatKey(), attach: a.ai.attach}
}

// restoreChat shows a conversation kept, and gives it to the provider
// chosen as text, the user's turns and the assistant's.
func (a *app) restoreChat(key string, chat savedChat) {
	ai := aiState{scope: a.ai.scope, key: key, chatID: chat.ID, provider: a.cfg.AIProvider}
	type turn struct{ role, text string }
	var turns []turn
	for _, m := range chat.Msgs {
		msg := chatMsg{role: m.Role, text: m.Text, proposal: m.Proposal, state: m.State, result: m.Result, names: m.Files}
		if msg.proposal != nil && msg.state == proposalPending {
			msg.state = proposalDiscarded // its tool call is gone
		}
		ai.msgs = append(ai.msgs, msg)
		text := m.Text
		if len(m.Files) > 0 { // the files themselves are not kept
			text = "（附件：" + strings.Join(m.Files, "、") + "）\n" + text
		}
		if m.Proposal != nil {
			done := map[int]string{proposalApplied: "用户已应用", proposalDiscarded: "用户没有应用", proposalPending: "用户没有应用", proposalFailed: "应用失败"}[m.State]
			text = strings.TrimSpace(text + "\n\n（提出了修改：" + m.Proposal.Summary + "，" + done + "）")
		}
		if text == "" || m.Role != "user" && m.Role != "assistant" {
			continue
		}
		if n := len(turns); n > 0 && turns[n-1].role == m.Role {
			turns[n-1].text += "\n\n" + text
		} else {
			turns = append(turns, turn{m.Role, text})
		}
	}
	if n := len(turns); n > 0 && turns[n-1].role == "user" {
		turns = turns[:n-1] // unanswered: the next message goes after the last answer
	}
	for _, t := range turns {
		if ai.provider == providerDeepSeek {
			ai.ds = append(ai.ds, dsMessage{Role: t.role, Content: t.text})
		} else if t.role == "user" {
			ai.claude = append(ai.claude, anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(t.text)))
		} else {
			ai.claude = append(ai.claude, anthropic.BetaMessageParam{Role: anthropic.BetaMessageParamRoleAssistant,
				Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock(t.text)}})
		}
	}
	ai.scroll.Y = 1e9
	a.ai = ai
}

// chatsMenu lists the conversations kept about what is shown.
func (a *app) chatsMenu(m *ui.Menu) {
	key := a.chatKey()
	chats := loadChats(key)
	m.Submenu("历史对话", func(m *ui.Menu) {
		if len(chats) == 0 {
			m.Item("还没有保存的对话").Disabled(true)
			return
		}
		for _, chat := range chats {
			label := chat.Updated.Format("01-02 15:04") + "　" + cmpOr(chat.Title, "（无标题）")
			if chat.ID == a.ai.chatID {
				label = "✓ " + label
			}
			if m.Item(label).Chosen() {
				a.restoreChat(key, chat)
			}
		}
	}).Disabled(a.ai.busy)
}
