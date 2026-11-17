package library

import (
	"regexp"
	"slices"
	"strings"
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

// secretPatterns are the shapes of a secret; of each match the part named
// keep, if any, stays (a line's name before its value), and the part
// named tail (a mark's end).
var secretPatterns = []*regexp.Regexp{
	marked,
	// A private key, the whole block.
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?(?:-----END [A-Z ]*PRIVATE KEY-----|\z)`),
	// Tokens told by how they begin.
	regexp.MustCompile(`\bsk-[A-Za-z0-9_\-]{16,}`),
	regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}`),
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{30,}`),
	regexp.MustCompile(`\bglpat-[A-Za-z0-9_\-]{16,}`),
	regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9\-]{10,}`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`\bAIza[0-9A-Za-z_\-]{30,}`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}`),
	// A line that names what it holds: KEY=value, "token": "value".
	regexp.MustCompile(`(?i)(?P<keep>\b[A-Za-z0-9_.\-]*(?:secret|token|passw(?:or)?d|api[_\-]?key|private[_\-]?key|access[_\-]?key)[A-Za-z0-9_.\-]*["']?\s*[=:]\s*["']?)[^\s"',;]{8,}`),
	regexp.MustCompile(`(?P<keep>(?:密码|口令|密钥)\s*[:：=是]\s*)[^\s，。；,;]{6,}`),
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
	for _, re := range secretPatterns {
		keep, tail := re.SubexpIndex("keep"), re.SubexpIndex("tail")
		for _, m := range re.FindAllStringSubmatchIndex(s, -1) {
			sp := span{m[0], m[1], re == marked}
			if keep >= 0 && m[2*keep] >= 0 {
				sp.from = m[2*keep+1]
			}
			if tail >= 0 && m[2*tail] >= 0 {
				sp.to = m[2*tail]
			}
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
	for _, re := range secretPatterns {
		if re.MatchString(s) {
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

// mayHoldSecret is a quick no for most texts: the patterns are tried only
// on one that has a word they all need.
func mayHoldSecret(s string) bool {
	if strings.Contains(s, MarkOpen) && len(s) > len(MarkOpen+MarkClose) {
		return true
	}
	if len(s) < 12 {
		return false
	}
	if vault.Has(s) {
		return true
	}
	l := strings.ToLower(s)
	for _, w := range []string{"sk-", "github_pat_", "ghp_", "gho_", "ghu_", "ghs_", "ghr_", "glpat-", "xox", "akia", "aiza", "eyj",
		"private key", "secret", "token", "passw", "key", "密码", "口令", "密钥"} {
		if strings.Contains(l, w) {
			return true
		}
	}
	return false
}

// secretFields are the parts of a clip a secret may be in.
func (c *Clip) secretFields() []*string {
	return []*string{&c.Title, &c.Text, &c.Note, &c.Summary, &c.Desc, &c.Analysis}
}

// Secret reports whether the clip holds what looks like a secret, in its
// words or in what was said of it, or was sealed whole.
func (c *Clip) Secret() bool {
	if c.Locked {
		return true
	}
	// Every part that is sealed (secretFields), the analysis too: sealed
	// there and not counted here, a secret could be neither shown nor
	// found under 含密钥.
	for _, f := range c.secretFields() {
		if HasSecret(*f) {
			return true
		}
	}
	return false
}

// Sealed reports whether the clip holds a secret sealed.
func (c *Clip) Sealed() bool {
	for _, f := range c.secretFields() {
		if vault.Has(*f) {
			return true
		}
	}
	return false
}

// Unsealed reports whether the clip holds what a vault turned on is to
// seal and has not.
func (c *Clip) Unsealed() bool {
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
