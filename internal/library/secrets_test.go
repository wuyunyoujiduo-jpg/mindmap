package library

import (
	"strings"
	"testing"
)

func TestMask(t *testing.T) {
	for _, c := range []struct{ in, gone, stays string }{
		{"用这个 sk-abcdefghijklmnopqrstuvwx 调接口", "abcdefghijkl", "调接口"},
		{"RELEASES_TOKEN=github_pat_11ABCDEFG0abcdefghijklmn_opqrstuvwxyz", "github_pat_", "RELEASES_TOKEN="},
		{"MYGO_UPDATER_PRIVATE_KEY=dW50cnVzdGVkIGNvbW1lbnQ6IG1pbmlzaWdu", "dW50cnVzdGVk", "MYGO_UPDATER_PRIVATE_KEY="},
		{"-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXk\nAAAA\n-----END OPENSSH PRIVATE KEY-----\n后面的话", "b3BlbnNzaC1rZXk", "后面的话"},
		{`{"api_key": "9f8e7d6c5b4a3210ffee"}`, "9f8e7d6c5b4a", `"api_key": "`},
		{"数据库密码：Hunter2Hunter2，别外传", "Hunter2", "别外传"},
		{"ghp_abcdefghijklmnopqrstuvwxyz0123456789", "abcdefghij", ""},
	} {
		got := Mask(c.in)
		if strings.Contains(got, c.gone) || !strings.Contains(got, c.stays) || !strings.Contains(got, masked) || !HasSecret(c.in) {
			t.Errorf("%q → %q", c.in, got)
		}
	}
	for _, plain := range []string{"写周报", "keyboard 的快捷键很好用", "token 这个词本身没什么", "密码要定期换", "sk-short"} {
		if got := Mask(plain); got != plain || HasSecret(plain) {
			t.Errorf("%q → %q", plain, got)
		}
	}
}

// What the user marks is a secret where no rule tells one.
func TestMarked(t *testing.T) {
	const line = "https://ttc.ttcttc.net/#/login wuy@gmail.com  ⟦密:ttc13!⟧"
	if got := Mask(line); got != "https://ttc.ttcttc.net/#/login wuy@gmail.com  "+masked || !HasSecret(line) || !Marked(line) {
		t.Errorf("marked: %q", got)
	}
	if got := Secrets(line); len(got) != 1 || got[0] != "ttc13!" {
		t.Errorf("the secret marked: %q", got)
	}
	seal := func(p string) (string, bool) { return "⟦mm1.aaaaaa." + strings.Repeat("A", len(p)) + "⟧", true }
	sealed := SealIn(line, seal)
	if strings.Contains(sealed, "ttc13") || !strings.HasSuffix(sealed, "⟦密:⟦mm1.aaaaaa.AAAAAA⟧⟧") {
		t.Fatalf("sealed: %q", sealed)
	}
	if SealIn(sealed, seal) != sealed || Unsealed(sealed) || Mask(sealed) != Mask(line) {
		t.Errorf("sealed twice, or shown otherwise: %q, %q", SealIn(sealed, seal), Mask(sealed))
	}
	// What a rule would find in it is the mark's, whole; and a mark holds
	// lines.
	in := "⟦密:password: abcdefgh ok\n第二行⟧"
	if got := Secrets(in); len(got) != 1 || got[0] != "password: abcdefgh ok\n第二行" {
		t.Errorf("a rule inside a mark: %q", got)
	}
	// No words a text has by itself are a mark: not the || of a spoiler or
	// a program, which the first try took for one.
	for _, plain := range []string{"看这个 ||剧透|| 吧", "if a||b||c { }", "x || y || z", "⟦⟧", "⟦密:⟧", "[[链接]] 和 {{模板}}"} {
		if Marked(plain) || HasSecret(plain) || Mask(plain) != plain {
			t.Errorf("%q → %q", plain, Mask(plain))
		}
	}
	if !CanMark("ttc13!") || !CanMark("a|b\nc") || CanMark("  ") || CanMark("a⟦b") || CanMark(sealed) {
		t.Error("CanMark")
	}
}

// A token put for one secret is not searched for another: its letters
// may look like a key.
func TestSealedOnce(t *testing.T) {
	seal := func(p string) (string, bool) { return "⟦mm1.aaaaaa.x-sk-" + strings.Repeat("B", 20) + "⟧", true }
	got := SealIn("OPENAI_API_KEY=sk-abcdefghijklmnopqrstuv", seal)
	if got != "OPENAI_API_KEY=⟦mm1.aaaaaa.x-sk-"+strings.Repeat("B", 20)+"⟧" {
		t.Errorf("sealed: %q", got)
	}
	// One secret two rules find is sealed whole, once.
	if got := Mask("API_KEY=sk-abcdefghijklmnopqrstuv.suffix1 后面"); got != "API_KEY="+masked+" 后面" {
		t.Errorf("two rules, one secret: %q", got)
	}
}
