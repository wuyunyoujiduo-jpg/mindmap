package library

import (
	"math"
	"slices"
	"strings"
	"unicode"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/vault"
)

// The index finds clips by words, ranked by BM25. Latin letters and
// digits make words; Chinese, Japanese and Korean text, which has no
// spaces, is cut into single characters and pairs of characters, so that
// any two-character word of a query matches.

// Weights of the parts of a clip.
var fieldWeights = []struct {
	of     func(c *Clip) string
	weight float64
}{
	{func(c *Clip) string { return c.Title }, 3},
	{func(c *Clip) string { return strings.Join(c.Tags, " ") }, 3},
	{func(c *Clip) string { return c.Category }, 2},
	{func(c *Clip) string { return strings.Join(c.Keywords, " ") }, 2},
	{func(c *Clip) string { return strings.Join(c.Synonyms, " ") }, 2},
	{func(c *Clip) string { return c.Analysis }, 1.5},
	{func(c *Clip) string { return c.Desc }, 2.5},
	{func(c *Clip) string { return c.Summary }, 2},
	{func(c *Clip) string { return c.Note }, 1.5},
	{func(c *Clip) string { return c.Text }, 1},
	{func(c *Clip) string { return c.URL + " " + c.Name }, 1},
}

// Index is the words of the clips of a library.
type Index struct {
	clips []*Clip
	terms []map[string]float64 // of each clip, weighted counts
	lens  []float64
	total float64 // of lens
	avg   float64
	df    map[string]int // how many clips have each term
	lower []string       // each clip's text in lower case, for whole phrases
	at    map[*Clip]int  // where each clip is
}

// idx is the index of the clips as they are: made when first asked for,
// and brought up to date from the one before when the clips changed
// since (Index.renew).
func (l *Library) idx() *Index {
	switch {
	case l.index == nil:
		l.index = NewIndex(l.Clips)
	case l.stale:
		l.index = l.index.renew(l.Clips, l.dirty)
	}
	l.stale, l.dirty = false, nil
	return l.index
}

// touch notes that a clip's words changed, for the index.
func (l *Library) touch(c *Clip) {
	if l.index == nil {
		return
	}
	if l.dirty == nil {
		l.dirty = map[*Clip]bool{}
	}
	l.dirty[c], l.stale = true, true
}

// NewIndex indexes clips.
func NewIndex(clips []*Clip) *Index {
	ix := &Index{clips: slices.Clone(clips), df: map[string]int{}, at: make(map[*Clip]int, len(clips))}
	for i, c := range clips {
		tf, n, lower := termsOf(c)
		for t := range tf {
			ix.df[t]++
		}
		ix.terms = append(ix.terms, tf)
		ix.lower = append(ix.lower, lower)
		ix.lens = append(ix.lens, n)
		ix.total += n
		ix.at[c] = i
	}
	if len(clips) > 0 {
		ix.avg = ix.total / float64(len(clips))
	}
	return ix
}

// termsOf are the words of a clip as the index keeps them: its terms,
// weighted, how many they are, and its text in lower case.
func termsOf(c *Clip) (tf map[string]float64, n float64, lower string) {
	tf = map[string]float64{}
	for _, f := range fieldWeights {
		for _, t := range Tokens(noSealed(f.of(c))) {
			tf[t] += f.weight
			n += f.weight
		}
	}
	lower = strings.ToLower(noSealed(c.Title+"\n"+c.Desc+"\n"+c.Summary+"\n"+c.Note+"\n"+c.Text) + "\n" + strings.Join(c.Tags, " ") + "\n" + strings.Join(c.Synonyms, " "))
	return tf, n, lower
}

// renew is the index of clips made from this one, which was of most of
// them: the words of a clip it has, and that did not change since, are
// taken as they are, and only the others cut up. The index made anew at
// each change of the library was most of the second a letter typed in a
// clip's details took (the words of 800 notes cut up for one of them),
// and of the wait when one clip came from outside. This one is not to be
// used after.
func (ix *Index) renew(clips []*Clip, changed map[*Clip]bool) *Index {
	nx := &Index{clips: slices.Clone(clips), df: ix.df, at: make(map[*Clip]int, len(clips)),
		terms: make([]map[string]float64, len(clips)), lens: make([]float64, len(clips)), lower: make([]string, len(clips))}
	kept := make([]bool, len(ix.clips))
	for i, c := range clips {
		if j, ok := ix.at[c]; ok && !changed[c] && !kept[j] {
			nx.terms[i], nx.lens[i], nx.lower[i] = ix.terms[j], ix.lens[j], ix.lower[j]
			kept[j] = true
		} else {
			nx.terms[i], nx.lens[i], nx.lower[i] = termsOf(c)
			for t := range nx.terms[i] {
				nx.df[t]++
			}
		}
		nx.total += nx.lens[i]
		nx.at[c] = i
	}
	for j, k := range kept {
		if k {
			continue
		}
		for t := range ix.terms[j] { // gone, or changed: its words as they were
			if nx.df[t]--; nx.df[t] <= 0 {
				delete(nx.df, t)
			}
		}
	}
	if len(clips) > 0 {
		nx.avg = nx.total / float64(len(clips))
	}
	return nx
}

// noSealed is s without the secrets sealed in it: a search is not to find
// a clip by the letters of a ciphertext, nor a vector to be made of them.
func noSealed(s string) string {
	if !vault.Has(s) {
		return s
	}
	return vault.Token.ReplaceAllString(s, " ")
}

// Hit is a clip found, with how well it matches.
type Hit struct {
	Clip  *Clip
	Score float64
	// Coverage is the share of the query's words the clip has (pairs of
	// characters, for Chinese), from 0 to 1.
	Coverage float64
}

// Search finds the clips matching a query, the best first; keep, when not
// nil, says which clips may be found. An empty query finds every clip
// kept, newest first.
func (l *Library) Search(query string, keep func(*Clip) bool) []Hit {
	// With nothing asked for, the clips as they are: no index is made to
	// list them, which is what 收集 does whenever it is not searched.
	if len(Tokens(query)) == 0 {
		var hits []Hit
		for _, c := range l.Clips {
			if keep == nil || keep(c) {
				hits = append(hits, Hit{Clip: c})
			}
		}
		return hits
	}
	return l.idx().Search(query, keep)
}

// Search finds the clips matching a query, the best first.
func (ix *Index) Search(query string, keep func(*Clip) bool) []Hit {
	var hits []Hit
	q := Tokens(query)
	phrase := strings.ToLower(strings.TrimSpace(query))
	if len(q) == 0 {
		for _, c := range ix.clips {
			if keep == nil || keep(c) {
				hits = append(hits, Hit{Clip: c})
			}
		}
		return hits
	}
	// Each term counts once, however often the query has it.
	slices.Sort(q)
	q = slices.Compact(q)
	const k1, b = 1.2, 0.75
	n := float64(len(ix.clips))
	for i, c := range ix.clips {
		if keep != nil && !keep(c) {
			continue
		}
		score, matched, words, wordsMatched := 0.0, 0, 0, 0
		for _, t := range q {
			tf := ix.terms[i][t]
			word := !isUnigram(t)
			if word {
				words++
			}
			if tf == 0 {
				continue
			}
			matched++
			if word {
				wordsMatched++
			}
			df := float64(ix.df[t])
			idf := math.Log(1 + (n-df+0.5)/(df+0.5))
			score += idf * tf * (k1 + 1) / (tf + k1*(1-b+b*ix.lens[i]/max(ix.avg, 1)))
		}
		if matched == 0 {
			continue
		}
		// Clips with more of the query's terms first, and the whole query
		// as it was written above all.
		score *= float64(matched) / float64(len(q))
		if len(phrase) > 1 && strings.Contains(ix.lower[i], phrase) {
			score *= 2
		}
		coverage := 1.0
		if words > 0 {
			coverage = float64(wordsMatched) / float64(words)
		}
		hits = append(hits, Hit{c, score, coverage})
	}
	slices.SortStableFunc(hits, func(a, b Hit) int {
		switch {
		case a.Score > b.Score:
			return -1
		case a.Score < b.Score:
			return 1
		}
		return b.Clip.Created.Compare(a.Clip.Created)
	})
	return hits
}

// isUnigram tells whether a term is a single Chinese, Japanese or Korean
// character, which says little alone.
func isUnigram(t string) bool {
	r := []rune(t)
	return len(r) == 1 && isCJK(r[0])
}

func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r)
}

// Tokens cuts text into the terms the index keeps: lower-case words of
// letters and digits, and the characters and pairs of characters of
// Chinese, Japanese and Korean runs.
func Tokens(text string) []string {
	var out []string
	var word []rune
	var cjk []rune
	flushWord := func() {
		if len(word) > 0 {
			out = append(out, string(word))
			word = word[:0]
		}
	}
	flushCJK := func() {
		for i, r := range cjk {
			out = append(out, string(r))
			if i+1 < len(cjk) {
				out = append(out, string(cjk[i:i+2]))
			}
		}
		cjk = cjk[:0]
	}
	for _, r := range text {
		switch {
		case isCJK(r):
			flushWord()
			cjk = append(cjk, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			flushCJK()
			word = append(word, unicode.ToLower(r))
		default:
			flushWord()
			flushCJK()
		}
	}
	flushWord()
	flushCJK()
	return out
}

// Excerpt is up to n characters of text around the first place a term of
// the query is, else its start, on one line.
func Excerpt(text, query string, n int) string {
	text = strings.Join(strings.Fields(text), " ")
	r := []rune(text)
	if len(r) <= n {
		return text
	}
	start := 0
	lower := strings.ToLower(text)
	best := -1
	for _, t := range Tokens(query) {
		if len([]rune(t)) < 2 && isCJK([]rune(t)[0]) {
			continue // a single character is everywhere
		}
		if i := strings.Index(lower, t); i >= 0 && (best < 0 || i < best) {
			best = i
		}
	}
	if best > 0 {
		at := len([]rune(lower[:best]))
		start = max(0, min(at-n/4, len(r)-n))
	}
	s := string(r[start : start+n])
	if start > 0 {
		s = "…" + s
	}
	if start+n < len(r) {
		s += "…"
	}
	return s
}
