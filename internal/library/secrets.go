package library

import (
	"regexp"
	"strings"

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

// secretPatterns are the shapes of a secret; of each match the part named
// keep, if any, stays (a line's name before its value).
var secretPatterns = []*regexp.Regexp{
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
	return eachSecret(s, func(string) string { return masked })
}

// eachSecret is s with each thing that looks like a secret replaced by
// what with makes of it.
func eachSecret(s string, with func(secret string) string) string {
	for _, re := range secretPatterns {
		keep := re.SubexpIndex("keep")
		s = re.ReplaceAllStringFunc(s, func(m string) string {
			if keep < 0 {
				return with(m)
			}
			k := re.FindStringSubmatch(m)[keep]
			return k + with(m[len(k):])
		})
	}
	return s
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
	var all []string
	for _, t := range vault.Token.FindAllString(s, -1) {
		all = append(all, t)
	}
	SealIn(s, func(plain string) (string, bool) {
		all = append(all, plain)
		return "", false
	})
	return all
}

// mayHoldSecret is a quick no for most texts: the patterns are tried only
// on one that has a word they all need.
func mayHoldSecret(s string) bool {
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
	for _, s := range []string{c.Text, c.Note, c.Title, c.Summary, c.Desc} {
		if HasSecret(s) {
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
