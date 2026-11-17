package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/vault"
)

func TestSealedAsWritten(t *testing.T) {
	now := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	l, _ := Open(dir)
	old := &Clip{Kind: KindText, Text: "OpenAI 的 key\nOPENAI_API_KEY=sk-proj-9fK2mB7tLx0QaZr4Hn8Qx7b"}
	plain := &Clip{Kind: KindText, Text: "买牛奶", Note: "路上"}
	for _, c := range []*Clip{old, plain} {
		if err := l.Add(c, now); err != nil {
			t.Fatal(err)
		}
	}
	file := func(c *Clip) string {
		data, _ := os.ReadFile(filepath.Join(dir, "clips", c.ID+".json"))
		return string(data)
	}
	if !strings.Contains(file(old), "sk-proj-9fK2") {
		t.Fatal("with no password set, a clip is kept as written")
	}

	// A password set: what is there is sealed, the rest left to the byte.
	if _, err := l.Vault.Setup("correct horse", now); err != nil {
		t.Fatal(err)
	}
	was := file(plain)
	if n, err := l.SealAll(); n != 1 || err != nil {
		t.Fatalf("sealed %d, %v", n, err)
	}
	if strings.Contains(file(old), "sk-proj") || !strings.Contains(file(old), "OPENAI_API_KEY=⟦mm1.") {
		t.Fatalf("the file: %s", file(old))
	}
	if !old.Updated.Equal(now) || file(plain) != was {
		t.Error("sealing changed a time, or a clip with no secret")
	}
	if !old.Secret() || !old.Sealed() || old.Unsealed() || strings.Contains(Mask(old.Text), "mm1") || !strings.Contains(Mask(old.Text), "OpenAI 的 key\nOPENAI_API_KEY=••••••••") {
		t.Errorf("the clip in memory: %q, masked %q", old.Text, Mask(old.Text))
	}
	if n, _ := l.SealAll(); n != 0 {
		t.Error("sealed twice")
	}

	// A new clip is sealed as it is kept, with the vault locked.
	l.Vault.Lock()
	fresh := &Clip{Kind: KindText, Text: "服务器 密码：Hunter2-Tr0ub4dor 别外传", Note: "token: abcdefgh12345678"}
	if err := l.Add(fresh, now); err != nil {
		t.Fatal(err)
	}
	if f := file(fresh); strings.Contains(f, "Hunter2") || strings.Contains(f, "abcdefgh1234") || !strings.Contains(f, "别外传") {
		t.Fatalf("a new clip's file: %s", f)
	}
	// It is not found by its ciphertext, and is by its words.
	tok := vault.Token.FindString(fresh.Text)
	if hits := l.Search(tok[8:20], nil); len(hits) != 0 {
		t.Error("found by the letters of a token")
	}
	if hits := l.Search("服务器", nil); len(hits) != 1 {
		t.Error("not found by its words")
	}
	if strings.Contains(EmbedText(fresh), "mm1") {
		t.Error("a token in what the vector is made of")
	}

	// Read again and unlocked, it opens; kept again, its file is the same.
	l2, _ := Open(dir)
	c := l2.Find(fresh.ID)
	if l2.Unseal(&Clip{Text: c.Text}) {
		t.Fatal("opened while locked")
	}
	if err := l2.Vault.Unlock("correct horse"); err != nil {
		t.Fatal(err)
	}
	was = file(fresh)
	if !l2.Unseal(c) || c.Text != "服务器 密码：Hunter2-Tr0ub4dor 别外传" || c.Note != "token: abcdefgh12345678" {
		t.Fatalf("opened: %q, %q", c.Text, c.Note)
	}
	c.Text += "，下月换"
	if err := l2.Save(c, now); err != nil {
		t.Fatal(err)
	}
	if f := file(fresh); strings.Contains(f, "Hunter2") || !strings.Contains(f, "下月换") || !strings.Contains(f, tok) {
		t.Fatalf("kept again: %s\nwas: %s", f, was)
	}

	// Sealed whole: a password nothing tells from a word.
	whole := &Clip{Kind: KindText, Title: "路由器", Text: "admin / plumtree", Locked: true}
	if err := l2.Add(whole, now); err != nil {
		t.Fatal(err)
	}
	if f := file(whole); strings.Contains(f, "plumtree") || !strings.Contains(f, "路由器") {
		t.Fatalf("sealed whole: %s", f)
	}
	if !whole.Secret() || whole.Label() != "路由器" || Mask(whole.Text) != "••••••••" {
		t.Errorf("sealed whole: label %q, masked %q", whole.Label(), Mask(whole.Text))
	}
	if !l2.Unseal(whole) || whole.Text != "admin / plumtree" {
		t.Fatalf("opened whole: %q", whole.Text)
	}
}

func TestSealIn(t *testing.T) {
	seal := func(p string) (string, bool) { return "⟦mm1.aaaaaa." + strings.Repeat("A", len(p)) + "⟧", true }
	once := SealIn("password: hunter2hunter2\nsee sk-abcdefghijklmnopqrstuv", seal)
	if strings.Contains(once, "hunter2") || strings.Contains(once, "sk-abc") || !strings.HasPrefix(once, "password: ⟦mm1.") {
		t.Fatalf("sealed: %q", once)
	}
	// Sealed already, it is left: a token is no value to seal again, nor
	// do the letters in one look like a key.
	if again := SealIn(once, seal); again != once {
		t.Errorf("sealed twice: %q", again)
	}
	if Unsealed(once) || !Unsealed(once+"\ntoken=zyxwvutsrq") {
		t.Error("Unsealed")
	}
	// The dots a secret was shown as are no secret.
	if s := "API_KEY=••••••••"; SealIn(s, seal) != s || Unsealed(s) {
		t.Error("the dots were sealed")
	}
	if got := Secrets(once + "\ntoken=zyxwvutsrq"); len(got) != 3 || got[2] != "zyxwvutsrq" {
		t.Errorf("Secrets: %q", got)
	}
}

// What was sealed is sealed again when the part it is in was opened,
// written in and kept, whatever the rules say of it now: sealed by a rule
// that is no more, or by one that was made narrower since, a secret was
// kept in the open on the first edit of its field.
func TestSealedStaysSealed(t *testing.T) {
	now := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	l, _ := Open(dir)
	if _, err := l.Vault.Setup("correct horse", now); err != nil {
		t.Fatal(err)
	}
	file := func(c *Clip) string {
		data, _ := os.ReadFile(filepath.Join(dir, "clips", c.ID+".json"))
		return string(data)
	}
	for i, c := range []struct{ before, secret, after string }{
		// What no rule finds now, nor is to: sealed by a rule of before.
		{"wifi ", "plumtree", " 在楼下"},
		{"密码是", "什么都行的那个", ""},
		{"The secret: ", "understanding", " of it"},
		{"password: ", "undefined", ""},
		{"max_tokens: ", "maxTokensDefault", ","},
		{"token = ", "strings.Split(line", ""},
		{"", "Abcdefgh", ""}, // the whole of the part
		// And each kind the rules were found to miss, and find again.
		{"wifi password: ", "correcthorsebattery", ""},
		{"password = ", "correcthorse", ""},
		{"Server=db;Password=", "hunterhunter", ";"},
		{"password: ", "hunter(2)x", ""},
		{"密码：", "abc(123)", ""},
		{"密码：", "zhang.san", ""},
		{"password=", "first.last", ""},
		{"API_KEY: ", "abcdefghijklmnopqrstuvwxyzabcdef", ""},
		{"client_secret: ", "qwertyuiopasdfghjklz", ""},
		{"密码：", "Abc中文12345", ""},
		{"password: ", "pässword1234", ""},
		{"密码是", "abc12345", ""},
		{"OPENAI_API_KEY=", "sk-proj-9fK2mB7tLx0QaZr4Hn8Qx7b", "\n第二行"},
		// And the forms found since.
		{"密码 ", "abc12345", ""},
		{"pwd=", "abc123", ""},
		{"DB_PASS=", "s3cr3tpass", ""},
		{"Authorization: Bearer ", "abcdefghij0123456789", ""},
		{"postgres://admin:", "s3cret", "@db.example.com/app"},
		{"redis://:", "hunter2", "@cache:6379"},
		{"password: ", "correcthorse", ""},
	} {
		tok, ok := l.Vault.Seal(c.secret)
		if !ok {
			t.Fatal("not sealed")
		}
		for _, field := range []string{"text", "note", "title", "desc"} {
			clip := &Clip{ID: NewID(), Kind: KindText, Text: "一条", Created: now, Updated: now}
			at := map[string]*string{"text": &clip.Text, "note": &clip.Note, "title": &clip.Title, "desc": &clip.Desc}[field]
			*at = c.before + tok + c.after
			if err := l.write(clip); err != nil {
				t.Fatal(err)
			}
			l.Clips = append(l.Clips, clip)
			// Opened to be written in, a word added at each end, and kept.
			open, ok := l.ToEdit(*at, false)
			if shown, _ := Marks(open); !ok || shown != c.before+c.secret+c.after {
				t.Fatalf("%d %s: opened as %q", i, field, open)
			}
			*at = "改 " + open + " 完"
			if err := l.Save(clip, now); err != nil {
				t.Fatal(err)
			}
			if f := file(clip); strings.Contains(f, c.secret) || strings.Contains(*at, c.secret) || !vault.Has(*at) {
				t.Errorf("%d %s: %q was sealed, and is kept as written after an edit: %q", i, field, c.secret, *at)
			}
			if !clip.Secret() || !clip.Sealed() || clip.Unsealed() {
				t.Errorf("%d %s: not a clip with a secret any more: %q", i, field, *at)
			}
			if strings.Contains(Brief(clip, "")+Full(clip, 5000)+clip.Label()+EmbedText(clip), c.secret) {
				t.Errorf("%d %s: %q is read to an AI", i, field, c.secret)
			}
			// It is the same secret still, where it was.
			again, ok := l.ToEdit(*at, false)
			if shown, _ := Marks(again); !ok || shown != "改 "+c.before+c.secret+c.after+" 完" {
				t.Errorf("%d %s: opened again as %q", i, field, again)
			}
		}
	}

	// Sealed whole, with a secret sealed in it before: the whole is the
	// text again, and the secret in it one still when it is whole no more.
	inner, _ := l.Vault.Seal("plumtree")
	text := "路由器 admin / " + inner + " 就这个"
	whole, _ := l.Vault.Seal(text)
	open, ok := l.ToEdit(whole, true)
	if shown, parts := Marks(open); !ok || shown != "路由器 admin / plumtree 就这个" || len(parts) != 1 {
		t.Fatalf("a text sealed whole, opened: %q", open)
	}
	clip := &Clip{ID: NewID(), Kind: KindText, Text: open, Created: now}
	l.Clips = append(l.Clips, clip)
	if err := l.Save(clip, now); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(file(clip), "plumtree") || !strings.Contains(clip.Text, "路由器 admin / ") {
		t.Errorf("sealed whole no more, the secret in it is kept as written: %q", clip.Text)
	}

	// Locked, nothing is opened, and what is there is left to the letter.
	l.Vault.Lock()
	if got, ok := l.ToEdit(clip.Text, false); ok || got != clip.Text {
		t.Errorf("opened while locked: %q", got)
	}
}
