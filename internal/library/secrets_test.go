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

// The name of a secret is a word of sentences and of programs too: what
// follows it there is left as it is, and a value still is one.
func TestSecretRules(t *testing.T) {
	for _, plain := range []string{
		"密码是什么时候改的我忘了",
		"Token: 这是一个很长的中文句子，后面还有很多字要写下去",
		"max_tokens: maxTokensDefault,",
		"token = strings.Split(line",
		"The secret: understanding…",
		"The secret: understanding your users before you build",
		`password = os.Getenv("DB_PASSWORD")`,
		"apiKey: config.apiKey,",
		"secret: process.env.SECRET_KEY",
		"conn = connect(user=user, password=password)",
		"client := NewClient(api_key=apiKeyValue, timeout=30)",
		"token := parser.nextToken()",
		"headers[token] = os.environ[name]",
		"密钥: config.secretKey 这样取",
		"口令是 abc123456 那个",
		"password: undefined;",
	} {
		if got := Mask(plain); got != plain || HasSecret(plain) || len(Found(plain)) != 0 {
			t.Errorf("no secret, and taken for one: %q → %q", plain, got)
		}
		if got := SealIn(plain, func(string) (string, bool) { return "⟦mm1.aaaaaa.AAAA⟧", true }); got != plain {
			t.Errorf("no secret, and sealed: %q → %q", plain, got)
		}
	}
	for _, c := range []struct{ in, secret string }{
		{"密码: Abc12345!", "Abc12345!"},
		{"password=hunter2hunter2", "hunter2hunter2"},
		{"OPENAI_API_KEY=sk-abcdefghijklmnopqrstuvwx", "sk-abcdefghijklmnopqrstuvwx"},
		{"token: ghp_abcdefghijklmnopqrstuvwxyz0123456789", "ghp_abcdefghijklmnopqrstuvwxyz0123456789"},
		{"password: hunter2hunter2", "hunter2hunter2"},
		{"账号 bob, password: hunter2hunter2, 服务器在机房", "hunter2hunter2"},
		{"token=zyxwvutsrq", "zyxwvutsrq"},
		{`{"password": "correcthorse"}`, "correcthorse"},
		{"password = 'john.smith'", "john.smith"},
		{"数据库密码：Hunter2Hunter2，别外传", "Hunter2Hunter2"},
		{"密码：helloworld", "helloworld"},
		{"密码：abc12345（记得改）", "abc12345"},
		{"口令=abc123", "abc123"},
		{"wifi 密码: abc123 记得换", "abc123"},
		{"SECRET_KEY=dW50cnVzdGVkIGNvbW1lbnQ6IG1pbmlzaWdu)", "dW50cnVzdGVkIGNvbW1lbnQ6IG1pbmlzaWdu)"},
		{"token: getToken(password=hunter2hunter2)", "hunter2hunter2)"}, // in what is no value itself
		{"SENDGRID_API_KEY=SG.abcDEF123ghiJKL456mnoPQ.xyz789ABCdef012GHIjkl345MNOpqr678STUvwx901", "SG.abcDEF123ghiJKL456mnoPQ.xyz789ABCdef012GHIjkl345MNOpqr678STUvwx901"}, // dots, as names of a program have
		{"client.login(user, password=hunter2hunter2)", "hunter2hunter2)"},
	} {
		if got := Secrets(c.in); len(got) != 1 || got[0] != c.secret || !HasSecret(c.in) {
			t.Errorf("a secret, and not found: %q → %q", c.in, got)
		}
		if got := Mask(c.in); strings.Contains(got, c.secret) || !strings.Contains(got, masked) {
			t.Errorf("a secret, and shown: %q → %q", c.in, got)
		}
	}
}

// The rules that left the words of sentences and programs alone left real
// secrets too (a reading of them over some hundred and thirty lines told
// which): each is found, beside what must stay as it is written.
func TestSecretsNotMissed(t *testing.T) {
	for _, c := range []struct{ in, secret string }{
		// Letters alone after an = with spaces round it.
		{"password = correcthorse", "correcthorse"},
		{"aws_secret_access_key = wJalrXUtnFEMIKEXAMPLEKEYbPxRfiCY", "wJalrXUtnFEMIKEXAMPLEKEYbPxRfiCY"},
		// Letters alone after an =, and a sign that ends a value after them.
		{"Server=db;User=sa;Password=hunterhunter;", "hunterhunter"},
		{"mysql --password=hunterhunter; ls", "hunterhunter"},
		{"export API_KEY=abcdefghijkl; echo done", "abcdefghijkl"},
		{"login(user, password=hunterhunter)", "hunterhunter)"},
		// A bracket after its first letters is no call by itself.
		{"password: hunter(2)x", "hunter(2)x"},
		{"密码：abc(123)", "abc(123)"},
		{"密码：Abcd[12]", "Abcd[12]"},
		// Letters joined by dots are a password where no program is written.
		{"密码：zhang.san", "zhang.san"},
		{"password=first.last", "first.last"},
		{"refresh_token: Abcdef.Ghijkl", "Abcdef.Ghijkl"},
		// Letters alone after a colon, long or of mixed case.
		{"API_KEY: abcdefghijklmnopqrstuvwxyzabcdef", "abcdefghijklmnopqrstuvwxyzabcdef"},
		{"client_secret: qwertyuiopasdfghjklz", "qwertyuiopasdfghjklz"},
		{"X-Api-Key: abcdefghijklmnop", "abcdefghijklmnop"},
		{"wifi password: correcthorsebattery", "correcthorsebattery"},
		{"password: CorrectHorse", "CorrectHorse"},
		{"token: aBcDeFgHiJ", "aBcDeFgHiJ"},
		// Letters of other alphabets in it.
		{"密码：Abc中文12345", "Abc中文12345"},
		{"password: pässword1234", "pässword1234"},
		// 是, and the value right after it.
		{"wifi 密码是abc12345", "abc12345"},
		{"家里的密码是Hunter2Hunter2，别外传", "Hunter2Hunter2"},
		// A word with the name of a secret in it is no name for one.
		{"password=monkeybusiness", "monkeybusiness"},
	} {
		if got := Secrets(c.in); len(got) != 1 || got[0] != c.secret || !HasSecret(c.in) {
			t.Errorf("a secret, and not found: %q → %q", c.in, got)
		}
		if got := Mask(c.in); strings.Contains(got, c.secret) || !strings.Contains(got, masked) {
			t.Errorf("a secret, and shown: %q → %q", c.in, got)
		}
		sealed := SealIn(c.in, func(p string) (string, bool) { return "⟦mm1.aaaaaa." + strings.Repeat("A", 8) + "⟧", true })
		if strings.Contains(sealed, c.secret) {
			t.Errorf("a secret, and kept as written: %q → %q", c.in, sealed)
		}
	}
	for _, plain := range []string{
		// What the reading of the whole found taken for secrets.
		"密码是什么时候改的我忘了",
		"Token: 这是一个很长的中文句子，后面还有很多字要写下去",
		"max_tokens: maxTokensDefault,",
		"token = strings.Split(line",
		"tokenizer = AutoTokenizer.from_pretrained(model)",
		"The secret: understanding…",
		// And what is like them.
		"The Secret: Understanding your users",
		"token = client.get(url).json()",
		"api_key = sys.argv[1]",
		"token = tokens[0]",
		"password = userInput",
		"secret = some_other_name",
		"callback({ token: accessToken, user })",
		"password: undefined;",
		"password = null",
		"密码：我的生日加名字",
		"密码是iPhone的锁屏密码",
		"密码是 abc12345 那个", // a space after 是: a sentence goes on
		"密钥: getKey(name)",
		"token: response.data.token",
		"password = form.value",
		"Token: 用GPT4生成的那个",
	} {
		if got := Mask(plain); got != plain || HasSecret(plain) || len(Found(plain)) != 0 {
			t.Errorf("no secret, and taken for one: %q → %q", plain, got)
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

// The marks are the file's: a field shows the text without them, and the
// parts they held, and puts them back as it keeps it.
func TestMarksInAndOut(t *testing.T) {
	const full = "账号 ⟦密:wuy@gmail.com  ttc13!⟧ 备用 ⟦密:第二个⟧。"
	shown, parts := Marks(full)
	if shown != "账号 wuy@gmail.com  ttc13! 备用 第二个。" || len(parts) != 2 {
		t.Fatalf("shown %q, parts %v", shown, parts)
	}
	r := []rune(shown)
	if string(r[parts[0][0]:parts[0][1]]) != "wuy@gmail.com  ttc13!" || string(r[parts[1][0]:parts[1][1]]) != "第二个" {
		t.Fatalf("parts %v", parts)
	}
	if got := WithMarks(shown, parts); got != full {
		t.Fatalf("put back: %q", got)
	}
	if s, p := Marks("没有记号的 || 一行"); s != "没有记号的 || 一行" || p != nil {
		t.Error("a text with no marks")
	}
	// What the rules find shows in the same look.
	if f := Found("用 sk-abcdefghijklmnopqrstuv 调，密码：Hunter2Hunter2"); len(f) != 2 || f[0] != [2]int{2, 27} {
		t.Errorf("found %v", f)
	}
}

// What an AI reads of a clip, the app's or an agent's, has its secrets
// in dots: in the lines of a search and in the clip whole.
func TestAIReadsNoSecret(t *testing.T) {
	const key = "sk-abcdefghijklmnopqrstuvwx"
	c := &Clip{ID: "c1", Kind: KindText, Title: "接口", Note: "备用 " + key, Text: strings.Repeat("前言。", 200) + "调接口用 " + key + " 就行"}
	for name, got := range map[string]string{"brief": Brief(c, "调接口"), "full": Full(c, 50000)} {
		if strings.Contains(got, "abcdefghijkl") || !strings.Contains(got, masked) {
			t.Errorf("%s shows the secret: %q", name, got)
		}
	}
}

// A # in a password is no tag's; what is sealed in a clip's analysis is
// a secret of the clip; a page of letters whose small form is of another
// length is cut where it should be.
func TestSecretsOddPlaces(t *testing.T) {
	c := &Clip{ID: "c1", Kind: KindLink, Analysis: "接口用 sk-abcdefghijklmnopqrstuvwx 调"}
	if !c.Secret() {
		t.Error("a secret in the analysis is not the clip's")
	}
	p := ParseHTML("<html><head><title>T</title></head>" + strings.Repeat("Ⱥ", 200) + "<BODY><article>正文在这里</article></BODY></html>")
	if !strings.Contains(p.Text, "正文在这里") {
		t.Errorf("page text %q", p.Text)
	}
}
