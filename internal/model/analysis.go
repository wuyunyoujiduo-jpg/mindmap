package model

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Analysis is what the AI made of a map, as of the library's clips: what
// it is about, and words to find it by, which the search looks in.
type Analysis struct {
	Desc     string    `json:"desc,omitempty"`
	Summary  string    `json:"summary,omitempty"`
	Category string    `json:"category,omitempty"`
	Tags     []string  `json:"tags,omitempty"`
	Keywords []string  `json:"keywords,omitempty"`
	Synonyms []string  `json:"synonyms,omitempty"`
	Analysis string    `json:"analysis,omitempty"`
	Hash     string    `json:"hash"` // of the content analyzed (File.ContentHash)
	Updated  time.Time `json:"updated"`
}

// Matches tells whether the query is in what the analysis says.
func (a *Analysis) Matches(query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if a == nil || q == "" {
		return false
	}
	for _, s := range append([]string{a.Desc, a.Summary, a.Category}, slices.Concat(a.Tags, a.Keywords, a.Synonyms)...) {
		if strings.Contains(strings.ToLower(s), q) {
			return true
		}
	}
	return false
}

// ContentHash is a hash of what a map says: its canvases' titles, and the
// titles, notes and tags of their nodes.
func (f *File) ContentHash() string {
	h := sha1.New()
	for _, d := range f.Sheets {
		fmt.Fprintf(h, "# %s\n", d.Title)
		d.Walk(func(n, _ *Node, depth int) bool {
			fmt.Fprintf(h, "%d %s\x00%s\x00%s\n", depth, n.Title, n.Note, strings.Join(n.Tags, ","))
			return true
		})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Outline is the map as indented text, to give the AI: canvases, nodes
// with their tags, tasks' status and notes, at most limit runes.
func (f *File) Outline(limit int) string {
	var b strings.Builder
	full := false
	for _, d := range f.Sheets {
		if full {
			break
		}
		fmt.Fprintf(&b, "## 画布：%s\n", d.Title)
		d.Walk(func(n, _ *Node, depth int) bool {
			if b.Len() > limit*3 { // runes are up to 3 bytes here
				full = true
				return false
			}
			b.WriteString(strings.Repeat("  ", depth) + "- " + n.Title)
			if n.Task != nil {
				b.WriteString("［" + n.Task.Status.Label() + "］")
			}
			if len(n.Tags) > 0 {
				b.WriteString(" #" + strings.Join(n.Tags, " #"))
			}
			b.WriteString("\n")
			if note := strings.TrimSpace(n.Note); note != "" {
				if r := []rune(note); len(r) > 300 {
					note = string(r[:300]) + "…"
				}
				b.WriteString(strings.Repeat("  ", depth+1) + "备注：" + strings.ReplaceAll(note, "\n", " ") + "\n")
			}
			return true
		})
	}
	s := b.String()
	if r := []rune(s); len(r) > limit {
		s = string(r[:limit]) + "\n…（后略）"
	}
	return s
}
