package library

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/vault"
)

// What is kept may hold a key or a password, copied with the rest: a
// token of an API, a private key, a line such as TOKEN=…. Such words are
// shown masked wherever a clip is shown small (a card, a line, a title, a
// card to share) and never sent to an AI: Mask puts dots in their place.
// The clip keeps them as written; its details show them when asked.
//
// With a second password set (internal/vault), they are not kept as
// written: each is sealed as the clip is written, a token in its place,
// and the clip in memory is as the one in its file. Mask puts dots for a
// token too, and the details open it when the vault is unlocked.

// MarkOpen and MarkClose stand round a part of a text the user said is
// a secret, where no rule tells one: ⟦密:ttc13!⟧. They are put by the
// app alone, when the user chooses a part and asks (the details' key),
// and are of signs no text has by itself: a mark that can be written,
// || as a spoiler's, would have made a secret of words that were written
// so already, a rule the user could neither see nor refuse (the user
// asked just that, and it was changed before it shipped). The marks stay
// in the text, round the secret or its token, so that it is one still
// when shown and kept again; Mask takes them off with it.
const MarkOpen, MarkClose = "⟦密:", "⟧"

// marked is the first of the patterns: what the user marked is not cut
// by what another finds in it.
var marked = regexp.MustCompile(`(?P<keep>⟦密:)[^⟦⟧]+(?P<tail>⟧)`)

// Marked reports whether s holds a secret marked by hand.
func Marked(s string) bool { return strings.Contains(s, MarkOpen) && marked.MatchString(s) }

// CanMark reports whether a part of a text can be marked a secret: not
// empty, and with none marked or sealed in it already.
func CanMark(part string) bool {
	return strings.TrimSpace(part) != "" && !strings.ContainsAny(part, "⟦⟧")
}

// A rule is a shape of a secret. Of each match the part named keep, if
// any, stays (a line's name before its value), and the part named tail (a
// mark's end). not, when set, tells a value the shape takes that is no
// secret after all, by what is kept before it and what comes after.
//
// near, when set, tells where in a text the shape may be, by the letters
// every match of it has: the shape is tried there alone (rule.each). Tried
// on the whole of every text, the shapes were the largest cost of 收集:
// most begin at the edge of a word, which leaves the matcher nothing to
// look for but every letter in turn, twelve times over, on a page of
// twenty thousand characters that only says "token" somewhere.
type rule struct {
	re   *regexp.Regexp
	not  func(keep, value, after string) bool
	near func(s string) nearby
}

// A nearby gives, for a text, the next part of it a shape may be in, from
// at on: s[lo:hi], and where to look from when the shape is not there;
// !ok when no part is left.
type nearby func(at int) (lo, hi, next int, ok bool)

// value is what a line that names a secret holds: letters, figures and
// signs of ASCII, up to a space, a quote, a comma or a semicolon. A
// sentence in Chinese after the name is none (Token: 这是一个很长的句子):
// a value begins in ASCII. It may go on through letters and figures of
// other alphabets to more of ASCII (pässword1234, Abc中文12345), which
// ended it, and left it short or shown; never through their signs or
// spaces, which end it (abc12345（记得改）). How long it is at least is
// told with what it is (notNamed, notChinese).
const (
	value  = `[!#-&(-+\--:<-~]`
	more   = `[^\x00-\x7f\pP\pS\pZ\pC]`
	valued = value + `+(?:` + more + `+` + value + `+)*`
)

// secretRules are the shapes of a secret.
var secretRules = []rule{
	{re: marked}, // begins with its sign, which the matcher looks for by itself
	// A private key, the whole block; it begins with its words too.
	{re: regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?(?:-----END [A-Z ]*PRIVATE KEY-----|\z)`)},
	// Tokens told by how they begin.
	{re: regexp.MustCompile(`\bsk-[A-Za-z0-9_\-]{16,}`), near: tokenNear("sk-")},
	{re: regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}`), near: tokenNear("github_pat_")},
	{re: regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{30,}`), near: tokenNear("ghp_", "gho_", "ghu_", "ghs_", "ghr_")},
	{re: regexp.MustCompile(`\bglpat-[A-Za-z0-9_\-]{16,}`), near: tokenNear("glpat-")},
	{re: regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9\-]{10,}`), near: tokenNear("xoxa-", "xoxb-", "xoxp-", "xoxr-", "xoxs-")},
	{re: regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`), near: tokenNear("AKIA")},
	{re: regexp.MustCompile(`\bAIza[0-9A-Za-z_\-]{30,}`), near: tokenNear("AIza")},
	{re: regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}`), near: tokenNear("eyJ")},
	// A line that names what it holds: KEY=value, "token": "value".
	{regexp.MustCompile(`(?i)(?P<keep>\b[A-Za-z0-9_.\-]*(?:secret|token|passw(?:or)?d|api[_\-]?key|private[_\-]?key|access[_\-]?key)[A-Za-z0-9_.\-]*["']?\s*[=:]\s*["']?)` + valued), notNamed, namedNear},
	// The same in Chinese, where a shorter value is one: 密码：abc123. After
	// 是 too when the value comes right after it (密码是abc12345, as the
	// password of a wifi is written), which a sentence does not (密码是什么
	// 时候改的).
	{regexp.MustCompile(`(?P<keep>(?:密码|口令|密钥)(?:\s*[:：=]\s*|是))` + valued), notChinese, chineseNear},
}

// A seek finds the words of a shape in a text: the next of any of them
// from a place on, each word looked for once from where it was last found.
type seek struct {
	s     string
	words []string
	at    []int // where each word is next; -1 nowhere any more, -2 not looked for yet
}

func newSeek(s string, words ...string) *seek {
	k := &seek{s: s, words: words, at: make([]int, len(words))}
	for i := range k.at {
		k.at[i] = -2
	}
	return k
}

// next is where the first of the words is at or after from, -1 for
// nowhere.
func (k *seek) next(from int) int {
	best := -1
	for n, w := range k.words {
		if k.at[n] == -1 {
			continue
		}
		if k.at[n] < from {
			i := strings.Index(k.s[from:], w)
			if i < 0 {
				k.at[n] = -1
				continue
			}
			k.at[n] = from + i
		}
		if best < 0 || k.at[n] < best {
			best = k.at[n]
		}
	}
	return best
}

func wordByte(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

// tokenNear is where a token told by how it begins may be: at each of its
// beginnings that a word does not run into (task-…, desk-… are none), up
// to the end of the letters, figures and signs a token is made of, and the
// one character after, which tells whether it ends at the edge of a word.
func tokenNear(begins ...string) func(s string) nearby {
	return func(s string) nearby {
		k := newSeek(s, begins...)
		return func(at int) (lo, hi, next int, ok bool) {
			for at <= len(s) {
				i := k.next(at)
				if i < 0 {
					break
				}
				if i > 0 && wordByte(s[i-1]) {
					at = i + 1
					continue
				}
				e := i
				for e < len(s) && (wordByte(s[e]) || s[e] == '-' || s[e] == '.') {
					e++
				}
				hi = e
				if hi < len(s) {
					_, n := utf8.DecodeRuneInString(s[hi:])
					hi += n
				}
				// Not there, it is nowhere up to where its letters end.
				return i, hi, max(e, i+1), true
			}
			return 0, 0, 0, false
		}
	}
}

func spaceByte(b byte) bool { return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f' }

func quoteByte(b byte) bool { return b == '"' || b == '\'' }

// valueByte tells a character of a value (the const value).
func valueByte(b byte) bool {
	return b >= '!' && b <= '~' && b != '"' && b != '\'' && b != ',' && b != ';'
}

// valueEnd is where the value written after s[at] ends: past the spaces,
// a quote and the value's own characters.
func valueEnd(s string, at int) int {
	for at < len(s) && spaceByte(s[at]) {
		at++
	}
	if at < len(s) && quoteByte(s[at]) {
		at++
	}
	for at < len(s) && (valueByte(s[at]) || s[at] >= 0x80) { // and what of other alphabets may be in it
		at++
	}
	return at
}

// nameWords are what every name of a secret has in it (secret, token,
// password and passwd, api_key and its like), in small letters.
var nameWords = []string{"secret", "token", "passw", "key"}

// namesSecret tells whether a name may be one of a secret: whether it has
// one of nameWords in it, in letters small or large.
func namesSecret(name string) bool {
	for _, w := range nameWords {
		for i := 0; i+len(w) <= len(name); i++ {
			j := 0
			for j < len(w) && name[i+j]|0x20 == w[j] {
				j++
			}
			if j == len(w) {
				return true
			}
		}
	}
	return false
}

// namedNear is where a line that names a secret may be: at each = and :
// whose name, the letters right before it, has the word of a secret in
// it, from where the name begins to where the value ends. Most = and : of
// a text follow another name (https:, a=b), and cost a look back of a few
// letters.
func namedNear(s string) nearby {
	// The matcher takes two letters of other alphabets for a k and an s
	// (the sign of the kelvin and the long s), which the look back by
	// bytes does not: a text with either is searched whole, as before.
	if strings.Contains(s, "K") || strings.Contains(s, "ſ") {
		return whole(s)
	}
	k := newSeek(s, "=", ":")
	return func(at int) (lo, hi, next int, ok bool) {
		for at <= len(s) {
			j := k.next(at)
			if j < 0 {
				break
			}
			end := j
			for end > at && spaceByte(s[end-1]) {
				end--
			}
			if end > at && quoteByte(s[end-1]) {
				end--
			}
			lo = end
			for lo > at && (wordByte(s[lo-1]) || s[lo-1] == '.' || s[lo-1] == '-') {
				lo--
			}
			if !namesSecret(s[lo:end]) {
				at = j + 1
				continue
			}
			return lo, valueEnd(s, j+1), j + 1, true
		}
		return 0, 0, 0, false
	}
}

// chineseNear is where a secret named in Chinese may be: at each of the
// names, to where the value after it ends.
func chineseNear(s string) nearby {
	k := newSeek(s, "密码", "口令", "密钥")
	return func(at int) (lo, hi, next int, ok bool) {
		if at > len(s) {
			return 0, 0, 0, false
		}
		i := k.next(at)
		if i < 0 {
			return 0, 0, 0, false
		}
		e := i + len("密码")
		for e < len(s) && spaceByte(s[e]) {
			e++
		}
		switch {
		case strings.HasPrefix(s[e:], "："), e == i+len("密码") && strings.HasPrefix(s[e:], "是"):
			e += len("：")
		case e < len(s) && (s[e] == ':' || s[e] == '='):
			e++
		default:
			return i, i, i + 1, true // a name with no value after it
		}
		return i, valueEnd(s, e), i + 1, true
	}
}

// whole is the whole of a text from at on: for a shape that is to be
// looked for everywhere.
func whole(s string) nearby {
	return func(at int) (lo, hi, next int, ok bool) { return at, len(s), len(s) + 1, at <= len(s) }
}

// The name of a secret is a word of sentences and of programs too, and
// what follows it there is no secret: a word (The secret: understanding…),
// a name of a program (max_tokens: maxTokensDefault, and password=password
// in a call), a thing of one or a call (token = strings.Split(line).
// Taken for secrets, they were sealed, gone from the search and from what
// an AI reads, and counted under 含密钥: a rule the user could not refuse.
//
// The rules that left those alone left real secrets too, which is worse:
// a secret not found is kept in the open, synced, and read to an AI. So a
// value is taken for what a program or a sentence has only in the shape
// of one, and for a secret otherwise:
//   - written in quotes, or with a figure or a sign in it: a secret;
//   - a call (strings.Split(line, os.environ[name]): of a program, when it
//     goes on as a call does; hunter(2)x is none, and neither is abc(123)
//     after a colon or a name in Chinese, where nothing is called;
//   - names joined by dots: of a program after an = with spaces round it,
//     or when they name the secret or where a program keeps one
//     (config.apiKey, process.env.SECRET_KEY); 密码：zhang.san and
//     password=first.last are passwords;
//   - letters alone after an =: a secret (password = correcthorse,
//     Password=hunterhunter; of a connection), but for a name of a program
//     where one is written (password = userInput, api_key=some_name,);
//   - letters alone after a colon: a secret when long or of mixed case
//     (API_KEY: and thirty-two letters, X-Api-Key: abcdefghijklmnop); a
//     short word there is a word (password: undefined), and such a
//     password is marked by hand;
//   - what is named as the secret is (password=password, maxTokensDefault)
//     or says there is none (undefined, null): never one.
var (
	// aWord is a value of letters alone.
	aWord = regexp.MustCompile(`^[A-Za-z_]+$`)
	// selector is names joined by dots (config.apiKey), of letters
	// alone: a key may have dots in it too, and has figures (SG.x1….y2…);
	// call a name called or looked into (strings.Split(line, os.environ[).
	selector = regexp.MustCompile(`^[A-Za-z_]+(?:\.[A-Za-z_]+)+$`)
	call     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*[(\[]`)
	// lowerCamel and capital are a name as programs write one (userInput)
	// and a word that begins a sentence (Understanding).
	lowerCamel = regexp.MustCompile(`^[a-z]+(?:[A-Z][a-z]+)+$`)
	pascal     = regexp.MustCompile(`^(?:[A-Z][a-z]+){2,}$`)
	capital    = regexp.MustCompile(`^[A-Z][a-z]+$`)
)

// noValues are words that say there is no value.
var noValues = []string{"undefined", "null", "none", "nil", "true", "false", "string", "required", "optional"}

// roots are where a program keeps what it is given: names joined by dots
// that begin with one are read from it, and are no value.
var roots = []string{"os", "sys", "process", "env", "config", "cfg", "conf", "settings", "self", "this", "req", "request", "res", "resp", "response",
	"ctx", "context", "args", "opts", "options", "params", "props", "state", "data", "form", "input", "body", "json", "result", "client", "session", "app"}

func hasFigure(s string) bool { return strings.ContainsAny(s, "0123456789") }

// hasASCII tells whether a value has a letter or a figure of ASCII: one
// of signs alone is none.
func hasASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if b := s[i]; b != '_' && wordByte(b) {
			return true
		}
	}
	return false
}

// calls tells whether a value is a call or a look into a thing, as a
// program writes it. prose says it follows a colon or a name in Chinese,
// where a line of a program is the less likely.
func calls(v string, prose bool) bool {
	at := call.FindStringIndex(v)
	if at == nil {
		return false
	}
	shut := byte(')')
	if v[at[1]-1] == '[' {
		shut = ']'
	}
	in := v[at[1]:]
	// What comes after it is shut goes on as a call does, or it is none:
	// f(x).y, f(x)[0], g(f(x)); not hunter(2)x.
	if i := strings.IndexByte(in, shut); i >= 0 && i+1 < len(in) && !strings.ContainsRune(".([)]}", rune(in[i+1])) {
		return false
	}
	// abc(123), Abcd[12]: a password with a bracket in it, where no
	// program is written; one is called by names joined, or given names.
	if prose && hasFigure(v) && !strings.Contains(in, "=") && !strings.Contains(v[:at[1]], ".") {
		return false
	}
	return true
}

// reads tells whether names joined by dots are read from where a program
// keeps things, or name the secret itself: config.apiKey, not zhang.san.
func reads(sel string) bool {
	first, _, _ := strings.Cut(sel, ".")
	return namesSecret(sel) || slices.Contains(roots, strings.ToLower(first))
}

// named tells whether a word of letters is a name for the secret, as a
// program has one (password, apiKeyValue, SECRET_KEY, maxTokensDefault),
// and not letters that happen to spell one, as a key's may, or a word
// with one in it (monkeybusiness).
func named(w string) bool {
	if !namesSecret(w) {
		return false
	}
	if strings.Contains(w, "_") || lowerCamel.MatchString(w) || pascal.MatchString(w) {
		return true
	}
	low := strings.ToLower(w)
	if w != low && w != strings.ToUpper(w) && !capital.MatchString(w) {
		return false
	}
	for _, n := range []string{"secret", "token", "password", "passwd", "key"} {
		if strings.HasPrefix(low, n) || strings.HasSuffix(low, n) {
			return true
		}
	}
	return false
}

// separator is the sign between a name and its value, = or :, and whether
// spaces are round it, as a program writes an =.
func separator(keep string) (sep byte, spaced bool) {
	k := strings.TrimRight(keep, " \t\n\r\f")
	if k == "" {
		return 0, false
	}
	return k[len(k)-1], len(k) < len(keep) || len(k) > 1 && spaceByte(k[len(k)-2])
}

// notNamed tells whether what a line that names a secret holds is no
// secret: too short, a thing of a program, or a word of a sentence.
func notNamed(keep, v, after string) bool {
	if utf8.RuneCountInString(v) < 8 {
		return true
	}
	if strings.HasSuffix(keep, `"`) || strings.HasSuffix(keep, "'") {
		return false // in quotes: written out, whatever it looks like
	}
	sep, spaced := separator(keep)
	if !hasASCII(v) || calls(v, sep == ':') {
		return true
	}
	bare := strings.TrimRight(v, ")]}") // the end of a call it is in
	if selector.MatchString(bare) {
		return sep == '=' && spaced || reads(bare)
	}
	if !aWord.MatchString(bare) {
		return false
	}
	if named(bare) || slices.Contains(noValues, strings.ToLower(bare)) {
		return true
	}
	name := lowerCamel.MatchString(bare) || strings.Contains(bare, "_")
	if sep == '=' {
		inCall := bare != v || strings.HasPrefix(after, ",")
		return name && (spaced || inCall)
	}
	if name {
		return true
	}
	mixed := bare != strings.ToLower(bare) && bare != strings.ToUpper(bare) && !capital.MatchString(bare)
	return len(bare) < 16 && !mixed
}

// notChinese is notNamed for a secret named in Chinese, where a value is
// shorter and a word is a password (密码：helloworld): no sentence goes on
// in letters there. After 是 a word is of a sentence (密码是iPhone的…),
// and a value has a figure or a sign.
func notChinese(keep, v, _ string) bool {
	if utf8.RuneCountInString(v) < 6 || !hasASCII(v) || calls(v, true) {
		return true
	}
	if strings.HasSuffix(keep, "是") && !strings.ContainsFunc(v, func(r rune) bool { return !unicode.IsLetter(r) }) {
		return true
	}
	bare := strings.TrimRight(v, ")]}")
	return selector.MatchString(bare) && reads(bare)
}

// each gives yield the secrets a rule finds in s, in bytes and in their
// order, till yield says to stop.
func (r rule) each(s string, yield func(from, to int) bool) {
	if r.not == nil && r.near == nil {
		for _, m := range r.re.FindAllStringSubmatchIndex(s, -1) {
			if from, to, _ := r.secret(s, 0, m); !yield(from, to) {
				return
			}
		}
		return
	}
	// Part by part of where it may be (near), or the whole of the text
	// from where the last was found.
	near := whole(s)
	if r.near != nil {
		near = r.near(s)
	}
	for at := 0; at <= len(s); {
		lo, hi, next, ok := near(at)
		if !ok {
			return
		}
		lo = max(lo, at)
		var m []int
		if lo < hi {
			m = r.re.FindStringSubmatchIndex(s[lo:hi])
		}
		if m == nil {
			at = max(next, at+1)
			continue
		}
		from, to, ok := r.secret(s, lo, m)
		switch {
		case !ok:
			// No value: what it is may hold the name of one, as a call
			// does (getToken(password=…): looked at from where it begins.
			at = max(from, at+1)
		case !yield(from, to):
			return
		default:
			at = max(lo+m[1], at+1)
		}
	}
}

// found are the secrets in s a rule finds.
func (r rule) found(s string) (out [][2]int) {
	r.each(s, func(from, to int) bool {
		out = append(out, [2]int{from, to})
		return true
	})
	return out
}

// finds tells whether a rule finds a secret in s, with no more of s read
// than up to the first.
func (r rule) finds(s string) (found bool) {
	if r.not == nil {
		return r.re.MatchString(s)
	}
	r.each(s, func(int, int) bool {
		found = true
		return false
	})
	return found
}

// secret is the secret of a match m of the rule's shape in s[lo:], in
// bytes of s, and whether it is one.
func (r rule) secret(s string, lo int, m []int) (from, to int, ok bool) {
	from, to = lo+m[0], lo+m[1]
	if keep := r.re.SubexpIndex("keep"); keep >= 0 && m[2*keep] >= 0 {
		from = lo + m[2*keep+1]
	}
	if tail := r.re.SubexpIndex("tail"); tail >= 0 && m[2*tail] >= 0 {
		to = lo + m[2*tail]
	}
	return from, to, r.not == nil || !r.not(s[lo+m[0]:from], s[from:to], s[to:])
}

// masked stands for a secret.
const masked = "••••••••"

// Mask is s with what looks like a secret, or is one sealed, replaced by
// dots.
func Mask(s string) string {
	if !mayHoldSecret(s) {
		return s
	}
	if vault.Has(s) {
		s = vault.Token.ReplaceAllString(s, masked)
	}
	s = eachSecret(s, func(string) string { return masked })
	if strings.Contains(s, MarkOpen) {
		s = strings.ReplaceAll(s, MarkOpen+masked+MarkClose, masked)
	}
	return s
}

// ReadLen is how long a text is to read, in runes: a secret sealed in it
// counts as the dots it shows as, not as its token, which is some hundred
// and thirty letters for a key of forty (a short note with a key was no
// short note any more once it was sealed, and had a summary written of
// it).
func ReadLen(s string) int {
	s = strings.TrimSpace(s)
	if vault.Has(s) {
		s = vault.Token.ReplaceAllString(s, masked)
	}
	return utf8.RuneCountInString(s)
}

// A span is a secret's place in a text, in bytes.
type span struct {
	from, to int
	fixed    bool // marked by hand: taken as it is
}

// secretSpans are the secrets of s, in their order. They are found in s
// as it is, all of them, before any is replaced: what is put for one is
// not searched by the next pattern, which would find a key in the
// letters of a token.
func secretSpans(s string) []span {
	var spans []span
	for _, r := range secretRules {
		for _, m := range r.found(s) {
			sp := span{m[0], m[1], r.re == marked}
			// One that meets another found before is one secret with it;
			// one that meets a marked one is left to the mark.
			joined := false
			for i := range spans {
				o := &spans[i]
				if sp.from >= o.to || sp.to <= o.from {
					continue
				}
				if !o.fixed {
					o.from, o.to = min(o.from, sp.from), max(o.to, sp.to)
				}
				joined = true
				break
			}
			if !joined {
				spans = append(spans, sp)
			}
		}
	}
	slices.SortFunc(spans, func(a, b span) int { return a.from - b.from })
	out := spans[:0]
	at := 0
	for _, sp := range spans {
		if sp.from < at { // two grown into each other
			continue
		}
		out, at = append(out, sp), sp.to
	}
	return out
}

// eachSecret is s with each thing that looks like a secret replaced by
// what with makes of it.
func eachSecret(s string, with func(secret string) string) string {
	spans := secretSpans(s)
	if len(spans) == 0 {
		return s
	}
	var b strings.Builder
	at := 0
	for _, sp := range spans {
		b.WriteString(s[at:sp.from])
		b.WriteString(with(s[sp.from:sp.to]))
		at = sp.to
	}
	b.WriteString(s[at:])
	return b.String()
}

// Marks takes the marks out of a text: what shows of it, and the parts
// that were marked secrets, as ranges of its runes. The marks are the
// file's, and are not shown: a field shows the parts in a look of their
// own (the user found the signs round a password strange).
func Marks(full string) (shown string, parts [][2]int) {
	if !strings.Contains(full, MarkOpen) {
		return full, nil
	}
	var b strings.Builder
	at, runes := 0, 0
	for _, m := range marked.FindAllStringSubmatchIndex(full, -1) {
		keep, tail := 2*marked.SubexpIndex("keep"), 2*marked.SubexpIndex("tail")
		before, part := full[at:m[keep]], full[m[keep+1]:m[tail]]
		b.WriteString(before)
		b.WriteString(part)
		runes += utf8.RuneCountInString(before)
		parts = append(parts, [2]int{runes, runes + utf8.RuneCountInString(part)})
		runes += utf8.RuneCountInString(part)
		at = m[tail+1]
	}
	b.WriteString(full[at:])
	return b.String(), parts
}

// WithMarks is a text with its parts marked secrets again, as Marks took
// them out. A part that cannot be marked is left unmarked.
func WithMarks(shown string, parts [][2]int) string {
	if len(parts) == 0 {
		return shown
	}
	r := []rune(shown)
	parts = slices.Clone(parts)
	slices.SortFunc(parts, func(a, b [2]int) int { return a[0] - b[0] })
	var b strings.Builder
	at := 0
	for _, p := range parts {
		from, to := max(p[0], at), min(p[1], len(r))
		if from >= to || !CanMark(string(r[from:to])) {
			continue
		}
		b.WriteString(string(r[at:from]))
		b.WriteString(MarkOpen + string(r[from:to]) + MarkClose)
		at = to
	}
	b.WriteString(string(r[at:]))
	return b.String()
}

// Found are the parts of a text the rules take for secrets, as ranges of
// its runes: shown in a field as the marked ones are.
func Found(s string) [][2]int {
	if !mayHoldSecret(s) {
		return nil
	}
	var parts [][2]int
	for _, sp := range secretSpans(s) {
		if !dotted(s[sp.from:sp.to]) {
			from := utf8.RuneCountInString(s[:sp.from])
			parts = append(parts, [2]int{from, from + utf8.RuneCountInString(s[sp.from:sp.to])})
		}
	}
	return parts
}

// HasSecret reports whether s holds what looks like a secret, or one
// sealed.
func HasSecret(s string) bool {
	if !mayHoldSecret(s) {
		return false
	}
	return vault.Has(s) || hasPlain(s)
}

func hasPlain(s string) bool {
	for _, r := range secretRules {
		if r.finds(s) {
			return true
		}
	}
	return false
}

// dotted tells the dots a secret was shown as, which an AI may have
// written after what it was shown: no secret, and not sealed as one.
func dotted(secret string) bool { return strings.Contains(secret, "••••") }

// Unsealed reports whether s holds what looks like a secret and is not
// sealed: what a vault turned on is to seal.
func Unsealed(s string) bool {
	found := false
	SealIn(s, func(string) (string, bool) {
		found = true
		return "", false
	})
	return found
}

// SealIn is s with what looks like a secret sealed by seal, a token in
// its place. What is sealed already is left.
func SealIn(s string, seal func(plain string) (token string, ok bool)) string {
	if !mayHoldSecret(s) {
		return s
	}
	part := func(p string) string {
		if !hasPlain(p) {
			return p
		}
		return eachSecret(p, func(secret string) string {
			if dotted(secret) {
				return secret
			}
			if t, ok := seal(secret); ok {
				return t
			}
			return secret
		})
	}
	if !vault.Has(s) {
		return part(s)
	}
	// Between the tokens, each part by itself: a token is no value of a
	// line that names a key.
	var b strings.Builder
	at := 0
	for _, m := range vault.Token.FindAllStringIndex(s, -1) {
		b.WriteString(part(s[at:m[0]]))
		b.WriteString(s[m[0]:m[1]])
		at = m[1]
	}
	b.WriteString(part(s[at:]))
	return b.String()
}

// Secrets are the secrets of s, those sealed as their tokens, in the
// order they were found.
func Secrets(s string) []string {
	all := vault.Token.FindAllString(s, -1)
	SealIn(s, func(plain string) (string, bool) {
		all = append(all, plain)
		return "", false
	})
	return all
}

// mayHoldSecret is a quick no for a text too short to hold a secret. A
// longer one is for the rules, each of which looks only where its own
// letters are (rule.near): a list of the words they all need, looked for
// here in the text made small letters, was a pass of its own over every
// text, and let through any that said "key".
func mayHoldSecret(s string) bool {
	if len(s) < 12 {
		return strings.Contains(s, MarkOpen) && len(s) > len(MarkOpen+MarkClose)
	}
	return true
}

// secretFields are the parts of a clip a secret may be in.
func (c *Clip) secretFields() []*string {
	return []*string{&c.Title, &c.Text, &c.Note, &c.Summary, &c.Desc, &c.Analysis}
}

// sealed reports whether the clip holds a secret sealed (Sealed, which
// works it out once, as Secret and Unsealed do: memo.go).
func (c *Clip) sealed() bool {
	for _, f := range c.secretFields() {
		if vault.Has(*f) {
			return true
		}
	}
	return false
}

// unsealed reports whether the clip holds what a vault turned on is to
// seal and has not (Unsealed).
func (c *Clip) unsealed() bool {
	for _, f := range c.secretFields() {
		if c.Locked && (f == &c.Text || f == &c.Note) {
			if strings.TrimSpace(*f) != "" && vault.Token.FindString(*f) != *f {
				return true
			}
		} else if Unsealed(*f) {
			return true
		}
	}
	return false
}

// seal seals what of the clip looks like a secret, and the whole of its
// text and note when it is Locked. It tells whether anything changed.
func (c *Clip) seal(seal func(string) (string, bool)) bool {
	changed := false
	for _, f := range c.secretFields() {
		was := *f
		if c.Locked && (f == &c.Text || f == &c.Note) {
			if strings.TrimSpace(was) != "" && vault.Token.FindString(was) != was {
				if t, ok := seal(was); ok {
					*f = t
				}
			}
		} else {
			*f = SealIn(was, seal)
		}
		changed = changed || *f != was
	}
	return changed
}

// SealAll seals what the clips hold unsealed, as when a second password
// is set and when clips came from a computer that has none yet, and
// returns how many clips it sealed. A clip's time is left as it is:
// sealing is no edit of the user's.
func (l *Library) SealAll() (n int, err error) {
	if !l.Vault.On() {
		return 0, nil
	}
	for _, c := range l.Clips {
		if c.Kind == KindMap || !c.Unsealed() {
			continue
		}
		before := *c
		if !c.seal(l.Vault.Seal) {
			continue
		}
		if e := l.write(c); e != nil {
			*c, err = before, e
			continue
		}
		delete(l.hashes, c.ID)
		n++
	}
	if n > 0 {
		l.changed()
	}
	return n, err
}

// Unseal opens what of the clip is sealed, in place, and tells whether
// all of it was opened: none of it is while the vault is locked. The
// clip is not written.
func (l *Library) Unseal(c *Clip) bool {
	all := true
	for _, f := range c.secretFields() {
		p, ok := l.Vault.Reveal(*f)
		*f, all = p, all && ok
	}
	return all
}

// ToEdit is a part of a clip with what is sealed in it opened, to be
// written in and kept again: as Vault.Reveal has it, but that each secret
// opened which the rules would not find again where it stands is put
// between marks (MarkOpen), and so is a secret still when the part is
// kept. What was sealed is sealed again, whatever the rules say now: they
// change (a rule that took sentences for secrets was made narrower), and
// a part opened to change one word of it was then kept with a password
// the old rule had sealed in the open, synced so, and read to an AI.
//
// whole says the part is one sealed whole (Clip.Locked), which is kept
// whole again and needs no mark round all of it; what was sealed in it
// before it was sealed whole is marked as elsewhere. ok is false when a
// secret could not be opened, which is left as it is.
func (l *Library) ToEdit(s string, whole bool) (plain string, ok bool) {
	if !vault.Has(s) {
		return s, true
	}
	type part struct{ from, to int }
	var opened []part
	var b strings.Builder
	at, ok := 0, true
	for _, m := range vault.Token.FindAllStringIndex(s, -1) {
		p, did := l.Vault.Opened(s[m[0]:m[1]])
		if !did {
			ok = false
			continue // left as it is, with what is round it
		}
		b.WriteString(s[at:m[0]])
		switch {
		case vault.Has(p):
			// A text sealed whole, with what was sealed in it before.
			in, all := l.ToEdit(p, false)
			b.WriteString(in)
			ok = ok && all
		case whole && m[0] == 0 && m[1] == len(s):
			b.WriteString(p)
		default:
			opened = append(opened, part{b.Len(), b.Len() + len(p)})
			b.WriteString(p)
		}
		at = m[1]
	}
	b.WriteString(s[at:])
	plain = b.String()
	if len(opened) == 0 {
		return plain, ok
	}
	spans := secretSpans(plain)
	var out strings.Builder
	at = 0
	for _, o := range opened {
		found := slices.ContainsFunc(spans, func(sp span) bool { return sp.from <= o.from && sp.to >= o.to })
		if found || !CanMark(plain[o.from:o.to]) {
			continue
		}
		out.WriteString(plain[at:o.from])
		out.WriteString(MarkOpen + plain[o.from:o.to] + MarkClose)
		at = o.to
	}
	out.WriteString(plain[at:])
	return out.String(), ok
}

// UnsealAll opens what the clips hold sealed and keeps them as written,
// as when the vault is turned off: n are the clips opened, left those a
// key not open sealed. As SealAll, it leaves the clips' times.
func (l *Library) UnsealAll() (n, left int, err error) {
	for _, c := range l.Clips {
		if !c.Sealed() && !c.Locked {
			continue
		}
		before := *c
		if !l.Unseal(c) {
			*c = before
			left++
			continue
		}
		c.Locked = false
		if e := l.write(c); e != nil {
			*c, err = before, e
			continue
		}
		delete(l.hashes, c.ID)
		n++
	}
	if n > 0 {
		l.changed()
	}
	return n, left, err
}
