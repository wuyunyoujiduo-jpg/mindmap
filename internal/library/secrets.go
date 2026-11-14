package library

import (
	"regexp"
	"strings"
)

// What is kept may hold a key or a password, copied with the rest: a
// token of an API, a private key, a line such as TOKEN=…. Such words are
// shown masked wherever a clip is shown small (a card, a line, a title, a
// card to share) and never sent to an AI: Mask puts dots in their place.
// The clip keeps them as written; its details show them when asked.

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

// Mask is s with what looks like a secret replaced by dots.
func Mask(s string) string {
	if !mayHoldSecret(s) {
		return s
	}
	for _, re := range secretPatterns {
		keep := re.SubexpIndex("keep")
		s = re.ReplaceAllStringFunc(s, func(m string) string {
			if keep < 0 {
				return masked
			}
			return re.FindStringSubmatch(m)[keep] + masked
		})
	}
	return s
}

// HasSecret reports whether s holds what looks like a secret.
func HasSecret(s string) bool {
	if !mayHoldSecret(s) {
		return false
	}
	for _, re := range secretPatterns {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

// mayHoldSecret is a quick no for most texts: the patterns are tried only
// on one that has a word they all need.
func mayHoldSecret(s string) bool {
	if len(s) < 12 {
		return false
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

// Secret reports whether the clip holds what looks like a secret, in its
// words or in what was said of it.
func (c *Clip) Secret() bool {
	for _, s := range []string{c.Text, c.Note, c.Title, c.Summary, c.Desc} {
		if HasSecret(s) {
			return true
		}
	}
	return false
}
