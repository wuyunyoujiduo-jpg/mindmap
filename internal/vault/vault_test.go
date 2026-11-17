package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func init() { cost = kdf{Time: 1, Memory: 64, Threads: 1} }

var day = time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)

func setup(t *testing.T) (*Vault, string) {
	t.Helper()
	v := Open(filepath.Join(t.TempDir(), "vault"))
	code, err := v.Setup("correct horse", day)
	if err != nil {
		t.Fatal(err)
	}
	return v, code
}

func TestSealAndReveal(t *testing.T) {
	v, _ := setup(t)
	tok, ok := v.Seal("sk-live-123456")
	if !ok || !Has(tok) || strings.Contains(tok, "sk-live") {
		t.Fatalf("token %q", tok)
	}
	text := "key: " + tok + " and more"
	if got, ok := v.Reveal(text); !ok || got != "key: sk-live-123456 and more" {
		t.Fatalf("revealed %q, %v", got, ok)
	}
	// Kept again as it was shown, it is the same token: a file is not
	// changed by being looked at.
	if again, _ := v.Seal("sk-live-123456"); again != tok {
		t.Error("a secret shown and sealed again got another token")
	}

	// Locked, it seals still, and opens nothing.
	v.Lock()
	if v.Unlocked() {
		t.Fatal("still unlocked")
	}
	other, ok := v.Seal("hunter2hunter2")
	if !ok {
		t.Fatal("a locked vault did not seal")
	}
	if got, ok := v.Reveal(text + other); ok || got != text+other {
		t.Fatalf("a locked vault revealed %q", got)
	}
	if err := v.Unlock("wrong horse"); err != ErrWrong {
		t.Fatalf("wrong password: %v", err)
	}
	if err := v.Unlock("correct horse"); err != nil {
		t.Fatal(err)
	}
	if got, ok := v.Reveal(other); !ok || got != "hunter2hunter2" {
		t.Fatalf("revealed %q", got)
	}
}

func TestAnotherComputer(t *testing.T) {
	v, code := setup(t)
	tok, _ := v.Seal("the secret")
	// Another computer has the folder alone.
	w := Open(v.Dir)
	if !w.On() || w.Unlocked() {
		t.Fatal("the folder read: on", w.On(), "unlocked", w.Unlocked())
	}
	mine, _ := w.Seal("sealed there")
	if err := w.UnlockCode(strings.ToLower(strings.ReplaceAll(code, "-", " "))); err != nil {
		t.Fatal("the code as written down:", err)
	}
	if got, ok := w.Reveal(tok + "|" + mine); !ok || got != "the secret|sealed there" {
		t.Fatalf("revealed %q", got)
	}
}

func TestChange(t *testing.T) {
	v, code := setup(t)
	tok, _ := v.Seal("the secret")
	if _, err := v.Change("short", day); err != ErrShort {
		t.Fatalf("a short password: %v", err)
	}
	fresh, err := v.Change("battery staple", day.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if fresh == code {
		t.Fatal("the recovery code was not changed with the password")
	}
	w := Open(v.Dir)
	if w.Unlock("correct horse") == nil || w.UnlockCode(code) == nil {
		t.Fatal("the old password or code still opens")
	}
	if err := w.Unlock("battery staple"); err != nil {
		t.Fatal(err)
	}
	if got, ok := w.Reveal(tok); !ok || got != "the secret" {
		t.Fatalf("what was sealed before the change: %q", got)
	}
	w.Lock()
	if err := w.UnlockCode(fresh); err != nil {
		t.Fatal("the new code:", err)
	}
	// Locked, the password is not changed: it takes the old one or the code.
	w.Lock()
	if _, err := w.Change("another one", day); err != ErrLocked {
		t.Fatalf("changed while locked: %v", err)
	}
}

func TestSealedWhole(t *testing.T) {
	v, _ := setup(t)
	inner, _ := v.Seal("inner")
	outer, _ := v.Seal("a note with " + inner)
	if got, ok := v.Reveal(outer); !ok || got != "a note with inner" {
		t.Fatalf("revealed %q", got)
	}
}

// Two computers that each set a password before they synced leave two
// keys: each opens its own, and both seal by the older.
func TestTwoKeys(t *testing.T) {
	a, _ := setup(t)
	b := Open(filepath.Join(t.TempDir(), "vault"))
	if _, err := b.Setup("another pass", day.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	ta, _ := a.Seal("of a")
	tb, _ := b.Seal("of b")
	files, _ := os.ReadDir(b.Dir)
	for _, f := range files {
		data, _ := os.ReadFile(filepath.Join(b.Dir, f.Name()))
		_ = os.WriteFile(filepath.Join(a.Dir, f.Name()), data, 0o600)
	}
	m := Open(a.Dir)
	if err := m.Unlock("another pass"); err != nil {
		t.Fatal(err)
	}
	if got, ok := m.Reveal(ta + tb); ok || got != ta+"of b" {
		t.Fatalf("with b's password: %q, %v", got, ok)
	}
	if err := m.Unlock("correct horse"); err != nil {
		t.Fatal(err)
	}
	if got, ok := m.Reveal(ta + tb); !ok || got != "of aof b" {
		t.Fatalf("with both: %q", got)
	}
	tok, _ := m.Seal("new")
	if !strings.HasPrefix(tok, strings.SplitN(ta, ".", 3)[0]+"."+strings.SplitN(ta, ".", 3)[1]+".") {
		t.Error("sealed by the newer key")
	}
}

func TestTurnOffAndSpare(t *testing.T) {
	v, _ := setup(t)
	spare := t.TempDir()
	v.Spare(spare)
	tok, _ := v.Seal("the secret")
	if err := v.TurnOff(day); err != nil {
		t.Fatal(err)
	}
	if v.On() || !v.Known() {
		t.Fatal("turned off: on", v.On(), "known", v.Known())
	}
	if _, ok := v.Seal("more"); ok {
		t.Fatal("a vault turned off sealed")
	}
	// The folder's file lost, the computer's own copy puts it back, and
	// what was sealed opens.
	if err := os.RemoveAll(v.Dir); err != nil {
		t.Fatal(err)
	}
	w := Open(v.Dir)
	if w.Known() {
		t.Fatal("the folder was not emptied")
	}
	w.Spare(spare)
	if err := w.Unlock("correct horse"); err != nil {
		t.Fatal(err)
	}
	if got, ok := w.Reveal(tok); !ok || got != "the secret" {
		t.Fatalf("revealed %q", got)
	}
	// Set up again, it seals by a new key.
	if _, err := w.Setup("a new password", day.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if !w.On() {
		t.Fatal("not on again")
	}
}

func TestToken(t *testing.T) {
	v, _ := setup(t)
	tok, _ := v.Seal("x")
	for _, s := range []string{tok, tok[:len(tok)-len(tokenEnd)], tok[:20], "⟦mm1."} {
		if got := Token.ReplaceAllString("a "+s+" b", "•"); got != "a • b" {
			t.Errorf("%q: %q", s, got)
		}
	}
	if !IsCode("7kq2 m9xd 4tpa hw3c r8nv 2jye") || IsCode("7KQ2-M9XD") {
		t.Error("IsCode")
	}
}

// While a password is tried, which takes Argon2 its time, the vault still
// answers whether it is unlocked: the window asks at every frame.
func TestUnlockHoldsNothing(t *testing.T) {
	was := cost
	cost = kdf{Time: 8, Memory: 64 * 1024, Threads: 1}
	defer func() { cost = was }()
	v := Open(t.TempDir())
	if _, err := v.Setup("correct horse", time.Now()); err != nil {
		t.Fatal(err)
	}
	v.Lock()
	done := make(chan error, 1)
	go func() { done <- v.Unlock("correct horse") }()
	asked := 0
	for waiting := true; waiting; {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
			waiting = false
		default:
			v.Unlocked()
			asked++
			time.Sleep(time.Millisecond)
		}
	}
	if asked < 10 || !v.Unlocked() {
		t.Fatalf("asked %d times while it was unlocked: held through it", asked)
	}
	// Locked while a password is tried, it stays locked.
	v.Lock()
	go func() { done <- v.Unlock("correct horse") }()
	time.Sleep(50 * time.Millisecond)
	v.Lock()
	if err := <-done; err != ErrLocked || v.Unlocked() {
		t.Fatalf("locked meanwhile, and opened: %v", err)
	}
}
