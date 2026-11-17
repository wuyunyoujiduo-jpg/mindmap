package library

import "sync"

// What is worked out of a clip's words is worked out once and kept with
// the clip: its label, whether it holds a secret, each of its parts with
// its secrets in dots. A wall draws its cards at every frame, and the
// rules of a secret read the whole of a text: worked out as they were
// asked for, they were most of a frame of 收集 (171 ms a frame for 120
// notes of 1,500 characters, a second for 120 pages of 20,000).
//
// It is kept with the words it was made of, and is good only while the
// clip's words are those. A clip is changed in place by whoever holds it
// (its details as they are typed in, the AI's answer, the vault sealing
// it, a file read again), and nothing says when: so each use compares the
// words, which costs nothing while they are the same strings, and what
// was worked out of other words is dropped with them. Nothing here is of
// the vault being locked or not: a secret sealed is dots whether or not
// it could be opened, and one shown in a clip's details is opened there,
// at each frame, and kept nowhere.

// A Part is a part of a clip a secret may be in.
type Part int

const (
	PartTitle Part = iota
	PartText
	PartNote
	PartSummary
	PartDesc
	PartAnalysis
	parts
)

// of is the part's words.
func (c *Clip) of(p Part) string {
	switch p {
	case PartTitle:
		return c.Title
	case PartText:
		return c.Text
	case PartNote:
		return c.Note
	case PartSummary:
		return c.Summary
	case PartDesc:
		return c.Desc
	case PartAnalysis:
		return c.Analysis
	}
	return ""
}

// worked is what was worked out of a clip's words, with the words.
type worked struct {
	// What it was made of: the parts, and what else a label or a card is
	// made of.
	part            [parts]string
	url, name, file string
	kind            string
	locked          bool

	// One clip is drawn by the window while a copy of it is read away
	// from it (the AI's work), and the two hold this together.
	mu       sync.Mutex
	label    *string
	secret   *bool
	sealed   *bool
	unsealed *bool
	masked   [parts]*string
	holds    [parts]*bool
	more     map[any]any
}

// memo is what was worked out of the clip's words as they are now, begun
// anew when they changed.
func (c *Clip) memo() *worked {
	w := c.worked
	if w != nil && w.kind == c.Kind && w.locked == c.Locked && w.url == c.URL && w.name == c.Name && w.file == c.File {
		same := true
		for p := range parts {
			if w.part[p] != c.of(p) {
				same = false
				break
			}
		}
		if same {
			return w
		}
	}
	w = &worked{url: c.URL, name: c.Name, file: c.File, kind: c.Kind, locked: c.Locked}
	for p := range parts {
		w.part[p] = c.of(p)
	}
	c.worked = w
	return w
}

// Label is the clip's title, else the start of its text, URL or name.
func (c *Clip) Label() string {
	w := c.memo()
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.label == nil {
		s := c.label()
		w.label = &s
	}
	return *w.label
}

// Masked is a part of the clip with what looks like a secret in it, or is
// one sealed, replaced by dots (Mask).
func (c *Clip) Masked(p Part) string {
	w := c.memo()
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.masked[p] == nil {
		s := Mask(w.part[p])
		w.masked[p] = &s
	}
	return *w.masked[p]
}

// Holds reports whether a part of the clip holds what looks like a
// secret, or one sealed (HasSecret).
func (c *Clip) Holds(p Part) bool {
	w := c.memo()
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.hold(p)
}

// hold is Holds, for who holds the lock.
func (w *worked) hold(p Part) bool {
	if w.holds[p] == nil {
		b := HasSecret(w.part[p])
		w.holds[p] = &b
	}
	return *w.holds[p]
}

// Secret reports whether the clip holds what looks like a secret, in its
// words or in what was said of it, or was sealed whole.
func (c *Clip) Secret() bool {
	if c.Locked {
		return true
	}
	w := c.memo()
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.secret == nil {
		// Every part that is sealed, the analysis too: sealed there and
		// not counted here, a secret could be neither shown nor found
		// under 含密钥.
		b := false
		for p := range parts {
			b = b || w.hold(p)
		}
		w.secret = &b
	}
	return *w.secret
}

// Sealed reports whether the clip holds a secret sealed.
func (c *Clip) Sealed() bool {
	w := c.memo()
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.sealed == nil {
		b := c.sealed()
		w.sealed = &b
	}
	return *w.sealed
}

// Unsealed reports whether the clip holds what a vault turned on is to
// seal and has not.
func (c *Clip) Unsealed() bool {
	w := c.memo()
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.unsealed == nil {
		b := c.unsealed()
		w.unsealed = &b
	}
	return *w.unsealed
}

// Once is what work makes of the clip's words, worked out once for the
// words as they are and kept under key: for what the app shows of a clip
// at every frame (the lines of its card, how tall it is). work may read
// of the clip only what it is kept by: its title, text, note, summary,
// description and analysis, its URL, its file and name, its kind and
// whether it is sealed whole; anything else it depends on is of the key.
func (c *Clip) Once(key any, work func() any) any {
	w := c.memo()
	w.mu.Lock()
	v, ok := w.more[key]
	w.mu.Unlock()
	if ok {
		return v
	}
	v = work() // not under the lock: it may ask for the label or a part
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.more == nil || len(w.more) >= 16 { // a search typed makes a key a letter
		w.more = map[any]any{}
	}
	w.more[key] = v
	return v
}
