package library

import (
	"fmt"
	"math/rand"
	"slices"
	"strings"
	"testing"
)

// wholeEach is rule.each as it was before a rule looked only where its
// letters are: the shape tried on the whole of the text.
func wholeEach(r rule, s string, yield func(from, to int) bool) {
	if r.not == nil {
		for _, m := range r.re.FindAllStringSubmatchIndex(s, -1) {
			if from, to, _ := r.secret(s, 0, m); !yield(from, to) {
				return
			}
		}
		return
	}
	for at := 0; at < len(s); {
		m := r.re.FindStringSubmatchIndex(s[at:])
		if m == nil {
			return
		}
		from, to, ok := r.secret(s, at, m)
		switch {
		case !ok:
			at = max(from, at+1)
		case !yield(from, to):
			return
		default:
			at += max(m[1], 1)
		}
	}
}

// What texts are made of, to try the rules on: names of secrets and names
// that are none, what stands between a name and its value, values, tokens
// and what begins as one, the same in Chinese, and what cuts a text.
var nearPieces = []string{
	"token", "api_key", "password", "PASSWD", "Secret", "access-key", "private_key", "monkey", "x.key", "a.b", "apiKey", "TOKEN",
	"=", ":", " = ", ": ", "：", "\n=\n", `"`, `'`, `": "`, " :", "\t=",
	"abcdefgh", "abc12345", "short", "zyxwvutsrq", "strings.Split(line", "maxTokensDefault", "p@ssw0rd!x", "a=b", "x:y=z", "12345678", "getToken(",
	"sk-0123456789abcdefghij", "task-0123456789abcdef0123", "sk-short", "ghp_0123456789abcdefghijklmnopqrstuvwxyz", "ghx_0123456789abcdefghijklmnopqrstuvwxyz",
	"AKIA0123456789ABCDEF", "AKIA0123456789ABCDEFX", "eyJhbGciOiJI.eyJzdWIiOiIx.SflKxwRJSMeK", "eyJhbGciOiJI.short.SflKxwRJSMeK",
	"xoxb-1234567890-abc", "xoxz-1234567890-abc", "glpat-abcdefghij0123456789", "github_pat_0123456789abcdefghijkl", "AIza0123456789abcdefghijklmnopqrstuvwxyz",
	"密码", "口令", "密钥", "是", "这是一个句子", "，", "。", "helloworld", "abc123",
	" ", "\n", ",", ";", "(", ")", "_", "-", ".", "K", "ſ", "⟦密:", "⟧", "x", "1", "https://example.com/a?b=c", "-----BEGIN RSA PRIVATE KEY-----", "-----END RSA PRIVATE KEY-----",
}

// TestRulesNearAsWhole tries each rule on texts made at random of what
// secrets are made of: looking where its letters are, a rule finds what
// it found looking over the whole text, the same secrets at the same
// places.
func TestRulesNearAsWhole(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	texts := []string{"", "token", "token=", "=", ":", "密码", "密码：", "sk-", "AKIA"}
	for range 40000 {
		var b strings.Builder
		for range 1 + rng.Intn(14) {
			b.WriteString(nearPieces[rng.Intn(len(nearPieces))])
		}
		texts = append(texts, b.String())
	}
	found := func(each func(func(from, to int) bool)) (out [][2]int) {
		each(func(from, to int) bool {
			out = append(out, [2]int{from, to})
			return true
		})
		return out
	}
	some := 0
	for _, s := range texts {
		for i, r := range secretRules {
			want := found(func(y func(int, int) bool) { wholeEach(r, s, y) })
			got := found(func(y func(int, int) bool) { r.each(s, y) })
			if !slices.Equal(got, want) {
				t.Fatalf("rule %d on %q: found %v looking near, %v over the whole", i, s, got, want)
			}
			if r.finds(s) != (len(want) > 0) {
				t.Fatalf("rule %d on %q: finds says %v, with %d to find", i, s, r.finds(s), len(want))
			}
			some += len(want)
		}
	}
	if some < 10000 {
		t.Fatalf("the texts tried hold too few secrets to tell: %d", some)
	}
}

// pageOf is a page of about n bytes that names keys and tokens in every
// line and holds no secret: what the rules cost most on.
func pageOf(n int) string {
	var b strings.Builder
	for line := 0; b.Len() < n; line++ {
		fmt.Fprintf(&b, "第 %d 行：缓存的 key 怎么取，token 过期了怎么换，task-list 里的 desk-top 和 https://example.com/a?b=%d。\n", line, line)
	}
	return b.String()
}

// BenchmarkMaskPage is Mask on a page of sixty thousand bytes with no
// secret in it, which says key and token throughout.
func BenchmarkMaskPage(b *testing.B) {
	s := pageOf(60000)
	b.SetBytes(int64(len(s)))
	for range b.N {
		if Mask(s) != s {
			b.Fatal("a secret found in a page that has none")
		}
	}
}

// BenchmarkMaskPageWhole is the same with each shape tried on the whole
// of the page, as it was.
func BenchmarkMaskPageWhole(b *testing.B) {
	s := pageOf(60000)
	b.SetBytes(int64(len(s)))
	for range b.N {
		for _, r := range secretRules {
			wholeEach(r, s, func(int, int) bool { b.Fatal("a secret found in a page that has none"); return false })
		}
	}
}

// BenchmarkMaskConfig is Mask on a file of settings, a secret in one line
// of ten: every line a name, an = and a value.
func BenchmarkMaskConfig(b *testing.B) {
	var sb strings.Builder
	for i := 0; sb.Len() < 60000; i++ {
		if i%10 == 0 {
			fmt.Fprintf(&sb, "API_KEY_%d=zx81kq0p%dmm\n", i, i)
		} else {
			fmt.Fprintf(&sb, "cache.size.%d = %d\n", i, i*7)
		}
	}
	s := sb.String()
	b.SetBytes(int64(len(s)))
	for range b.N {
		if Mask(s) == s {
			b.Fatal("no secret found")
		}
	}
}
