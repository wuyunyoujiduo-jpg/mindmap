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

// dupIndex finds clips by their file, page and text.
type dupIndex struct {
	hash, url map[string]*Clip
	text      map[string][]*Clip // those alike but for their secrets
}

// textKey is what texts alike have alike: the text, its spaces as one and
// its secrets as dots. A secret sealed is a token, never twice the same,
// and the text to compare with it is as it was written: by their tokens
// and letters, a note with a key in it was never found kept already once
// a second password was set. In dots the two are alike, and are then
// compared opened (sameText).
func textKey(s string) string {
	s = strings.Join(strings.Fields(Mask(s)), " ")
	if s == "" {
		return ""
	}
	return hashOf([]byte(s))
}

// sameText tells whether two texts alike but for their secrets are the
// same text: they are with no secret in either, and with the same
// secrets, which of those sealed is known only while the vault is
// unlocked. Locked, a note with a secret in it is kept a second time
// rather than taken for one that may differ by the secret (a key
// changed for a new one); so is one kept before whole sealed (Locked),
// which is alike to nothing.
func (l *Library) sameText(a, b string) bool {
	if !HasSecret(a) && !HasSecret(b) {
		return true
	}
	pa, ok := l.Vault.Reveal(a)
	if !ok {
		return false
	}
	pb, ok := l.Vault.Reveal(b)
	return ok && strings.Join(strings.Fields(pa), " ") == strings.Join(strings.Fields(pb), " ")
}

func (l *Library) dupIdx() *dupIndex {
	if l.dups == nil {
		d := &dupIndex{hash: map[string]*Clip{}, url: map[string]*Clip{}, text: map[string][]*Clip{}}
		for _, c := range l.Clips {
			d.add(c)
		}
		l.dups = d
	}
	return l.dups
}

func (d *dupIndex) add(c *Clip) {
	if c.Hash != "" {
		d.hash[c.Hash] = c
	}
	switch c.Kind {
	case KindLink:
		d.url[NormalizeURL(c.URL)] = c
	case KindText:
		if k := textKey(c.Text); k != "" {
			d.text[k] = append(d.text[k], c)
		}
	}
}

// Duplicate is the clip already kept that c would repeat, nil when none.
func (l *Library) Duplicate(c *Clip) *Clip {
	d := l.dupIdx()
	var o *Clip
	switch {
	case c.Hash != "" && d.hash[c.Hash] != nil:
		o = d.hash[c.Hash]
	case c.Kind == KindLink:
		o = d.url[NormalizeURL(c.URL)]
	case c.Kind == KindText:
		for _, x := range d.text[textKey(c.Text)] {
			if x != c && x.ID != c.ID && l.sameText(x.Text, c.Text) {
				o = x
				break
			}
		}
	}
	if o == c || o != nil && o.ID == c.ID {
		return nil
	}
	return o
}
