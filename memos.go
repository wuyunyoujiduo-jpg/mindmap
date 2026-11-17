package main

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
)

// 灵感 are the notes written as they come: the library's texts, cards of
// 收集 among the clips, whose #tags written in the text sort them. They
// are found by the library's search, sorted by its AI and put on topics
// as any clip; the shortcut of quick capture writes them too. 回顾 looks
// back at the old ones.

// memoTag is a #tag in a text: # then what is up to a space or another #.
var memoTag = regexp.MustCompile(`#[^\s#，。,.!！?？;；:：、（）()\[\]{}"'“”]+`)

// memoTags are the #tags written in a text, without their #. A # that
// is of something else is none: in a web address or after a letter, a
// figure or a slash (…/guide#install-2, C#, /#/login), and before a color
// (#E8A13A, #fff), which were made tags of the note.
func memoTags(text string) []string {
	var out []string
	var urls [][]int
	if strings.Contains(text, "://") {
		urls = titleURL.FindAllStringIndex(text, -1)
	}
	for _, m := range memoTag.FindAllStringIndex(text, -1) {
		if m[0] > 0 {
			if c := text[m[0]-1]; c == '/' || c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' {
				continue
			}
		}
		if slices.ContainsFunc(urls, func(u []int) bool { return m[0] > u[0] && m[0] < u[1] }) {
			continue
		}
		if t := text[m[0]+1 : m[1]]; !colorWord(t) && !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	return out
}

// colorWord tells whether what follows a # is a color (hexColor): with a
// figure of ten in it, as a topic's title takes it (tagWord), or all one
// letter (fff). Other letters alone may be a word (#decade, #CAD), and
// are a tag as they were.
func colorWord(s string) bool {
	if !hexColor.MatchString(s) {
		return false
	}
	return strings.ContainsAny(s, "0123456789") || strings.Count(strings.ToLower(s), strings.ToLower(s[:1])) == len(s)
}

// noteHasMarks tells whether a note has marks to show by, bold or a list:
// what noteMarked tells, read off the text itself. Every card of a wall
// asks at every frame, and the pattern, tried from each letter in turn,
// was two thirds of a frame of 收集 once the rest of it was light.
func noteHasMarks(s string) bool {
	// A list: a line that begins with "- ", or with figures and ". ".
	for at := 0; at < len(s); {
		line := s[at:]
		if strings.HasPrefix(line, "- ") {
			return true
		}
		d := 0
		for d < len(line) && line[d] >= '0' && line[d] <= '9' {
			d++
		}
		if d > 0 && strings.HasPrefix(line[d:], ". ") {
			return true
		}
		i := strings.IndexByte(line, '\n')
		if i < 0 {
			break
		}
		at += i + 1
	}
	// Bold: ** and, with no * nor end of line between, ** again.
	for at := 0; ; {
		i := strings.Index(s[at:], "**")
		if i < 0 {
			return false
		}
		from := at + i + 2
		to := from
		for to < len(s) && s[to] != '*' && s[to] != '\n' {
			to++
		}
		if to > from && strings.HasPrefix(s[to:], "**") {
			return true
		}
		at += i + 1
	}
}

// isMemo tells whether a clip is one of 灵感: a text of the library.
func isMemo(c *library.Clip) bool { return c.Kind == library.KindText }

// addMemo keeps a note of 灵感: text, its #tags its tags.
func (a *app) addMemo(text, source string) (*library.Clip, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("没有内容")
	}
	c := &library.Clip{Kind: library.KindText, Text: text, Source: source, Tags: memoTags(library.Mask(text))}
	return a.kept(c, a.lib.Add(c, a.now()))
}

// setMemoText changes a note's text: the tags written in it follow, the
// others (the AI's, the user's) stay.
func (a *app) setMemoText(m *library.Clip, text string) {
	memoText(m, text)
	a.saveClip(m)
}

// memoText gives a note its text, and the tags written in it; the note is
// not kept (setMemoText keeps it; its details, when the typing pauses).
func memoText(m *library.Clip, text string) {
	// The tags of the text with its secrets in dots: a # in a password
	// made the rest of it a tag, kept and shown in the open.
	old := memoTags(m.Masked(library.PartText))
	m.Text = text
	tags := slices.DeleteFunc(slices.Clone(m.Tags), func(s string) bool { return slices.Contains(old, s) })
	for _, tag := range memoTags(m.Masked(library.PartText)) {
		if !slices.Contains(tags, tag) {
			tags = append(tags, tag)
		}
	}
	m.Tags = tags
}

// The ways 收集 looks back (app.libReview): 每日回顾, a few of the old
// notes, the same all day; 随机漫步, one at a time, at random.
const (
	reviewOff = iota
	reviewDaily
	reviewWalk
)

// reviewClips are the notes a way of looking back shows: of those older
// than a week, a few by the day, or one at random.
func (a *app) reviewClips() []*library.Clip {
	week := a.now().AddDate(0, 0, -7)
	var old []*library.Clip
	for _, c := range a.lib.Clips {
		if isMemo(c) && !c.Created.After(week) {
			old = append(old, c)
		}
	}
	if len(old) == 0 {
		return nil
	}
	slices.SortStableFunc(old, func(x, y *library.Clip) int { return y.Created.Compare(x.Created) })
	day := today(a.now())
	seed := day.Year()*400 + day.YearDay()
	if a.libReview == reviewWalk {
		return []*library.Clip{old[(seed*7919+a.libWalk*104729)%len(old)]}
	}
	n := min(3, len(old))
	out := make([]*library.Clip, 0, n)
	for i := range len(old) {
		if len(out) == n {
			break
		}
		m := old[(seed*31+i*17)%len(old)]
		if !slices.Contains(out, m) {
			out = append(out, m)
		}
	}
	return out
}

// tagCount is a tag of what is collected, and how many carry it.
type tagCount struct {
	name string
	n    int
}

// clipTags are the tags of what is collected, the most used first.
func (a *app) clipTags() []tagCount {
	if a.clipTagsFor == a.lib.Version && a.clipTagsOf == a.lib {
		return a.clipTagsList
	}
	counts := map[string]int{}
	var order []string
	for _, c := range a.lib.Clips {
		for _, tag := range c.Tags {
			if counts[tag] == 0 {
				order = append(order, tag)
			}
			counts[tag]++
		}
	}
	slices.SortStableFunc(order, func(x, y string) int { return counts[y] - counts[x] })
	list := make([]tagCount, len(order))
	for i, tag := range order {
		list[i] = tagCount{tag, counts[tag]}
	}
	a.clipTagsFor, a.clipTagsOf, a.clipTagsList = a.lib.Version, a.lib, list
	return list
}
