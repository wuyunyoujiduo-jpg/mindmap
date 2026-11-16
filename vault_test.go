package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/vault"
)

// A second password set, what looks like a key is kept sealed, asked for
// by the password, copied unseen, and the password changed with its code.
func TestSecretsAreSealed(t *testing.T) {
	vault.Quick()
	a, tt := newTestApp(t)
	const key = "sk-abcdefghijklmnopqrstuvwxyz012345"
	cl := &library.Clip{Kind: library.KindText, Text: "OpenAI 的 key\nOPENAI_API_KEY=" + key, Note: "额度 20 刀"}
	plain := &library.Clip{Kind: library.KindText, Text: "没有密钥的一条"}
	for _, c := range []*library.Clip{cl, plain} {
		if err := a.lib.Add(c, a.now()); err != nil {
			t.Fatal(err)
		}
	}
	file := func(c *library.Clip) string {
		data, _ := os.ReadFile(filepath.Join(a.lib.Dir, "clips", c.ID+".json"))
		return string(data)
	}
	shows := func(s string) bool {
		for _, x := range tt.Texts() {
			if strings.Contains(x, s) {
				return true
			}
		}
		return false
	}
	// What the details' field of the content holds: a field's words are
	// not among the window's texts.
	field := func() string {
		c := a.lib.Find(cl.ID)
		s, _ := a.fieldText(c, "text", c.Text)
		return s
	}
	click := func(s string) {
		t.Helper()
		if err := tt.Click(s); err != nil {
			t.Fatalf("%v: %q", err, tt.Texts())
		}
		tt.Frame()
		tt.Frame()
	}

	// The settings' page, with none set: the way to set one, and how many
	// are kept as written.
	a.openVaultSettings()
	tt.Frame()
	if !shows("现在有 1 条含密钥的记录是原样保存的") {
		t.Fatalf("the page: %q", tt.Texts())
	}
	snapshot(t, tt, "vault-settings-off")
	click("设置密码…")
	if a.settingsOpen || !tt.HasText("设置二次密码") {
		t.Fatalf("the window to set one: %q", tt.Texts())
	}
	snapshot(t, tt, "vault-setup")
	// Too short, and not the same twice, are said and not taken.
	a.vaultAsk.pass, a.vaultAsk.again = "short", "short"
	tt.Frame()
	click("下一步")
	if !tt.HasText("密码至少 8 位") || a.vault.On() {
		t.Fatalf("a short password: %q", tt.Texts())
	}
	a.vaultAsk.pass, a.vaultAsk.again = "correct horse", "correct horsf"
	tt.Frame()
	click("下一步")
	if !tt.HasText("两次输入的不一样") || a.vault.On() {
		t.Fatalf("two passwords: %q", tt.Texts())
	}
	a.vaultAsk.again = "correct horse"
	tt.Frame()
	click("下一步")
	code := a.vaultAsk.code
	if !a.vault.On() || !vault.IsCode(code) || !tt.HasText("记下恢复码") || !shows(code) {
		t.Fatalf("the code: %q, %q", code, tt.Texts())
	}
	snapshot(t, tt, "vault-code")
	// The code is shown once: the window stays till it is written down.
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	tt.Frame()
	if !a.vaultAsk.open || !tt.HasText("记下恢复码") {
		t.Fatal("the code's window was put away unsaved")
	}
	click("我已经存好了")
	click("完成")
	if a.vaultAsk.open || !a.settingsOpen || !strings.Contains(a.notice, "1 条里的密钥已加密") {
		t.Fatalf("done: %q, settings %v", a.notice, a.settingsOpen)
	}
	if f := file(cl); strings.Contains(f, key) || !strings.Contains(f, "OPENAI_API_KEY=⟦mm1.") || !strings.Contains(f, "额度 20 刀") {
		t.Fatalf("the clip's file: %s", f)
	}
	if strings.Contains(file(plain), "mm1") {
		t.Fatal("a clip with no secret was sealed")
	}
	if !shows("1 条里的密钥已加密 · 已解锁") {
		t.Fatalf("the page, on: %q", tt.Texts())
	}
	snapshot(t, tt, "vault-settings-on")
	click("现在锁上")
	if a.vault.Unlocked() || !shows("锁着") {
		t.Fatal("not locked")
	}
	a.settingsOpen = false

	// In the details, dots; the password asked to show it.
	a.openList(kindAll, "")
	a.libSel = cl.ID
	tt.Frame()
	if shows(key) || shows("mm1") || !shows("OPENAI_API_KEY=••••••••") {
		t.Fatalf("the details, locked: %q", tt.Texts())
	}
	snapshot(t, tt, "vault-detail-locked")
	click("显示密钥")
	if !tt.HasText("输入二次密码") || a.secretShown != "" {
		t.Fatalf("no password asked: %q", tt.Texts())
	}
	a.vaultAsk.old = "wrong horse"
	tt.Frame()
	click("解锁")
	if !tt.HasText("密码不对") || a.vault.Unlocked() {
		t.Fatalf("a wrong password: %q", tt.Texts())
	}
	snapshot(t, tt, "vault-unlock-wrong")
	a.vaultAsk.old = "correct horse"
	tt.Frame()
	click("解锁")
	if !a.vault.Unlocked() || a.secretShown != cl.ID || a.vaultAsk.open || !strings.Contains(field(), key) || shows("mm1") {
		t.Fatalf("unlocked: shown %q, %q", a.secretShown, tt.Texts())
	}
	snapshot(t, tt, "vault-detail-open")

	// Written in while shown, it is kept sealed, and the field goes on
	// showing what was typed.
	click("资料内容")
	tt.Type("X")
	tt.Frame()
	tt.Frame()
	if f := file(cl); strings.Contains(f, key) || !strings.Contains(f, "⟦mm1.") {
		t.Fatalf("edited: %s", f)
	}
	if got, _ := a.vault.Reveal(a.lib.Find(cl.ID).Text); !strings.Contains(got, "X") || !strings.Contains(strings.ReplaceAll(got, "X", ""), key) {
		t.Fatalf("edited: %q", got)
	}
	if got := field(); strings.Contains(got, "mm1") || !strings.Contains(strings.ReplaceAll(got, "X", ""), key) {
		t.Fatalf("the field while written in: %q", got)
	}

	// Its time up, it locks by itself and the secret is in dots again.
	a.now = func() time.Time { return testNow.Add(6 * time.Minute) }
	tt.Frame()
	tt.Frame()
	if got := field(); a.vault.Unlocked() || a.secretShown != "" || strings.Contains(got, "sk-") || !tt.HasText("显示密钥") {
		t.Fatalf("not locked by itself: %q, %q", got, tt.Texts())
	}

	// Copied unseen: the password, then the clipboard, cleared a minute on.
	plat := a.plat.(*fakePlatform)
	click("复制密钥")
	a.vaultAsk.old = "correct horse"
	tt.Frame()
	click("解锁")
	if !strings.Contains(plat.copied, "sk-") || a.secretShown != "" || shows("sk-abc") {
		t.Fatalf("copied %q, shown %q", plat.copied, a.secretShown)
	}
	a.now = func() time.Time { return testNow.Add(8 * time.Minute) }
	tt.Frame()
	if plat.copied != "" {
		t.Fatalf("the clipboard a minute on: %q", plat.copied)
	}

	// A new clip is sealed as it is kept, with nothing asked.
	a.lockVault()
	fresh := &library.Clip{Kind: library.KindText, Text: "数据库密码：Hunter2Hunter2，别外传"}
	if err := a.lib.Add(fresh, a.now()); err != nil {
		t.Fatal(err)
	}
	if f := file(fresh); strings.Contains(f, "Hunter2") || !strings.Contains(f, "别外传") {
		t.Fatalf("a new clip: %s", f)
	}
	// What it becomes elsewhere has dots, not the letters of a ciphertext.
	if s := sealedOut(fresh.Text); strings.Contains(s, "mm1") || !strings.Contains(s, "••••") {
		t.Fatalf("out of the clip: %q", s)
	}
	if p := organizePromptFor(fresh, clipPromptBody); strings.Contains(p, "mm1") || strings.Contains(p, "Hunter2") {
		t.Fatalf("to the AI: %q", p)
	}

	// The password changed by the recovery code: a new code, the old ones
	// open nothing, what was sealed still opens.
	a.askVault(askChange, nil)
	a.vaultAsk.byCode = true
	a.vaultAsk.old, a.vaultAsk.pass, a.vaultAsk.again = strings.ToLower(code), "battery staple", "battery staple"
	tt.Frame()
	snapshot(t, tt, "vault-change")
	click("下一步")
	fresher := a.vaultAsk.code
	if !vault.IsCode(fresher) || fresher == code || !tt.HasText("记下恢复码") {
		t.Fatalf("the new code: %q, %q", fresher, tt.Texts())
	}
	click("我已经存好了")
	click("完成")
	a.lockVault()
	if a.vault.Unlock("correct horse") == nil || a.vault.UnlockCode(code) == nil {
		t.Fatal("the old password or code still opens")
	}
	if err := a.vault.Unlock("battery staple"); err != nil {
		t.Fatal(err)
	}
	if got, ok := a.vault.Reveal(fresh.Text); !ok || !strings.Contains(got, "Hunter2Hunter2") {
		t.Fatalf("after the change: %q", got)
	}

	// The library read again, as after a sync, stays unlocked.
	a.openLibrary()
	if !a.vault.Unlocked() || a.lib.Vault != a.vault {
		t.Fatal("read again, the vault was locked")
	}

	// Sealed whole from its menu, and back.
	whole := &library.Clip{Kind: library.KindText, Title: "路由器", Text: "admin / plumtree"}
	if err := a.lib.Add(whole, a.now()); err != nil {
		t.Fatal(err)
	}
	a.lockWhole(whole, true)
	if f := file(whole); strings.Contains(f, "plumtree") || !strings.Contains(f, "路由器") || !whole.Secret() {
		t.Fatalf("sealed whole: %s", f)
	}
	a.noticeUndo()
	if f := file(whole); !strings.Contains(f, "plumtree") || whole.Locked {
		t.Fatalf("taken back: %s", f)
	}

	// Turned off, all is opened and kept as written.
	a.vaultOff()
	if a.vault.On() || strings.Contains(file(cl), "mm1") || !strings.Contains(strings.ReplaceAll(file(cl), "X", ""), key) || !strings.Contains(file(fresh), "Hunter2Hunter2") {
		t.Fatalf("turned off: %s", file(cl))
	}
}
