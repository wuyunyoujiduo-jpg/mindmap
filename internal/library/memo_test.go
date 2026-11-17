package library

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// says is what is worked out of a clip, all of it, as its words are now:
// by the clip, which keeps it, and afresh by a copy that has kept nothing.
func says(c *Clip) (kept, fresh string) {
	of := func(c *Clip) string {
		var b strings.Builder
		b.WriteString(c.Label())
		for _, v := range []bool{c.Secret(), c.Sealed(), c.Unsealed()} {
			if v {
				b.WriteString("|yes")
			} else {
				b.WriteString("|no")
			}
		}
		for p := range parts {
			b.WriteString("|" + c.Masked(p))
			if c.Holds(p) {
				b.WriteString("|holds")
			}
		}
		return b.String()
	}
	anew := *c
	anew.worked = nil
	return of(c), of(&anew)
}

// What is worked out of a clip is of its words as they are, whatever was
// worked out of them before and however they were changed: typed in, a
// part of them marked a secret and not, sealed by the vault and opened,
// sealed whole and back, read again from a file, put back as they were.
// A secret that showed because it was no secret a moment ago is a leak.
func TestWorkedOutFollowsTheWords(t *testing.T) {
	now := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	l, _ := Open(dir)
	c := &Clip{Kind: KindText, Title: "路由器", Text: "家里的 wifi 在楼下", Note: "备用"}
	if err := l.Add(c, now); err != nil {
		t.Fatal(err)
	}
	check := func(when string, secret bool, shows, hides string) {
		t.Helper()
		kept, fresh := says(c)
		if kept != fresh {
			t.Fatalf("%s: kept from before\n%q\nthe words say\n%q", when, kept, fresh)
		}
		if c.Secret() != secret {
			t.Fatalf("%s: a secret %v", when, c.Secret())
		}
		card := c.Label() + "\n" + c.Masked(PartTitle) + "\n" + c.Masked(PartText) + "\n" + c.Masked(PartNote)
		if shows != "" && !strings.Contains(card, shows) {
			t.Fatalf("%s: %q does not show in %q", when, shows, card)
		}
		if hides != "" && strings.Contains(card, hides) {
			t.Fatalf("%s: %q shows in %q", when, hides, card)
		}
	}
	check("as kept", false, "家里的 wifi", "")

	// Typed in: a secret comes letter by letter, and is one from the
	// letter that makes it one.
	c.Text = "家里的 wifi 在楼下\npassword: hunt2"
	check("five letters of a password", false, "hunt2", "")
	c.Text += "r"
	check("the sixth", true, "password: "+masked, "hunt2r")
	c.Text = "家里的 wifi 在楼下"
	check("taken out again", false, "在楼下", masked)

	// A part marked a secret, and not any more.
	c.Text = "家里的 wifi " + MarkOpen + "plumtree" + MarkClose + " 在楼下"
	check("marked", true, "wifi "+masked, "plumtree")
	c.Text = "家里的 wifi plumtree 在楼下"
	check("no secret any more", false, "plumtree", masked)

	// In another part: the title is the label, the note counts.
	c.Title = "token=zyxwvutsrq"
	check("in the title", true, "token="+masked, "zyxwvutsrq")
	c.Title = "路由器"
	c.Note = "sk-abcdefghijklmnopqrstuvwx"
	check("in the note", true, masked, "sk-abc")
	c.Note = "备用"
	check("gone from the note", false, "路由器", masked)

	// Put back as it was, as when a change could not be kept.
	c.Text = "password: hunter2hunter2"
	check("a secret", true, masked, "hunter2")
	before := *c
	c.Text = "没有了"
	check("changed", false, "没有了", masked)
	*c = before
	check("put back", true, masked, "hunter2")

	// The vault set: sealed in place, by SealAll and as it is kept.
	if _, err := l.Vault.Setup("correct horse", now); err != nil {
		t.Fatal(err)
	}
	if !c.Unsealed() {
		t.Fatal("nothing to seal")
	}
	if n, err := l.SealAll(); n != 1 || err != nil {
		t.Fatalf("sealed %d, %v", n, err)
	}
	check("sealed", true, "password: "+masked, "mm1")
	if c.Unsealed() || !c.Sealed() {
		t.Fatal("sealed, and not said so")
	}
	// Locked and unlocked: what a card shows is dots either way.
	l.Vault.Lock()
	check("locked", true, "password: "+masked, "hunter2")
	if err := l.Vault.Unlock("correct horse"); err != nil {
		t.Fatal(err)
	}
	check("unlocked", true, "password: "+masked, "hunter2")

	// Sealed whole and back.
	c.Locked = true
	if err := l.Save(c, now); err != nil {
		t.Fatal(err)
	}
	check("sealed whole", true, masked, "password")
	open := *c
	if !l.Unseal(&open) {
		t.Fatal("not opened")
	}
	open.Locked = false
	*c = open
	check("opened, not kept yet", true, "password: "+masked, "hunter2")
	if err := l.Save(c, now); err != nil {
		t.Fatal(err)
	}
	check("whole no more", true, "password: "+masked, "mm1")

	// The vault turned off: opened and kept as written.
	if err := l.Vault.TurnOff(now); err != nil {
		t.Fatal(err)
	}
	if n, _, err := l.UnsealAll(); n != 1 || err != nil {
		t.Fatalf("opened %d, %v", n, err)
	}
	check("the vault off", true, "password: "+masked, "hunter2")
	if c.Sealed() || !strings.Contains(c.Text, "hunter2hunter2") {
		t.Fatalf("not kept as written: %q", c.Text)
	}
}

// Once works a thing out once for the words as they are, and again when
// they change; a copy of the clip read elsewhere shares it safely.
func TestOnce(t *testing.T) {
	c := &Clip{Kind: KindText, Text: "第一行"}
	n := 0
	line := func() string {
		return c.Once("line", func() any { n++; return "「" + c.Text + "」" }).(string)
	}
	if line() != "「第一行」" || line() != "「第一行」" || n != 1 {
		t.Fatalf("worked out %d times", n)
	}
	c.Text = "第二行"
	if line() != "「第二行」" || n != 2 {
		t.Fatalf("kept from the words of before: %q", line())
	}
	var wg sync.WaitGroup
	for range 8 {
		copied := *c
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 200 {
				_ = copied.Label() + copied.Masked(PartText)
				_ = copied.Secret() || copied.Unsealed() || copied.Holds(PartNote)
				_ = copied.Once("line", func() any { return copied.Text })
			}
		}()
	}
	for range 200 {
		_ = c.Label() + c.Masked(PartText)
		_ = line()
	}
	wg.Wait()
}

// A wall asks for a clip's label and whether it holds a secret at every
// frame: the rules are tried once for the words as they are.
func BenchmarkLabelAndSecret(b *testing.B) {
	c := &Clip{Kind: KindLink, Title: "文章", Text: pageOf(60000)}
	for range b.N {
		if c.Secret() || c.Label() != "文章" || c.Masked(PartText) != c.Text {
			b.Fatal("a secret found in a page that has none")
		}
	}
}
