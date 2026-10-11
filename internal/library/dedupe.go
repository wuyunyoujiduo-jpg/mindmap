package library

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strings"
)

// A clip kept twice is found again rather than kept twice: the same page,
// the same text, the same file.

// DuplicateError says a clip is already in the library, as Clip.
type DuplicateError struct{ Clip *Clip }

func (e *DuplicateError) Error() string { return "已经收藏过：" + e.Clip.Label() }

func hashOf(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// NormalizeURL is a link as it is compared: without its fragment, the
// trailing slash, "www." and the parameters that track where it came from.
func NormalizeURL(s string) string {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || u.Host == "" {
		return strings.TrimSpace(s)
	}
	u.Fragment = ""
	u.Scheme = "https"
	u.Host = strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	q := u.Query()
	for k := range q {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "utm_") || lk == "spm" || lk == "from" || lk == "share_source" || lk == "share_from" || lk == "fbclid" || lk == "gclid" {
			q.Del(k)
		}
	}
	u.RawQuery = q.Encode()
	u.Path = strings.TrimSuffix(u.Path, "/")
	return u.String()
}

func sameText(a, b string) bool {
	return strings.Join(strings.Fields(a), " ") == strings.Join(strings.Fields(b), " ")
}

// Duplicate is the clip already kept that c would repeat, nil when none.
func (l *Library) Duplicate(c *Clip) *Clip {
	for _, o := range l.Clips {
		if o == c || o.ID == c.ID {
			continue
		}
		switch {
		case c.Hash != "" && o.Hash == c.Hash:
			return o
		case c.Kind == KindLink && o.Kind == KindLink && NormalizeURL(o.URL) == NormalizeURL(c.URL):
			return o
		case c.Kind == KindText && o.Kind == KindText && strings.TrimSpace(c.Text) != "" && sameText(o.Text, c.Text):
			return o
		}
	}
	return nil
}
