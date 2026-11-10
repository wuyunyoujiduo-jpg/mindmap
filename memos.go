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

// memoTags are the #tags written in a text, without their #.
func memoTags(text string) []string {
	var out []string
	for _, m := range memoTag.FindAllString(text, -1) {
		if t := strings.TrimPrefix(m, "#"); !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	return out
}

// isMemo tells whether a clip is one of 灵感: a text of the library.
func isMemo(c *library.Clip) bool { return c.Kind == library.KindText }

// addMemo keeps a note of 灵感: text, its #tags its tags.
func (a *app) addMemo(text, source string) (*library.Clip, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("没有内容")
	}
	c := &library.Clip{Kind: library.KindText, Text: text, Source: source, Tags: memoTags(text)}
	return a.kept(c, a.lib.Add(c, a.now()))
}

// setMemoText changes a note's text: the tags written in it follow, the
// others (the AI's, the user's) stay.
func (a *app) setMemoText(m *library.Clip, text string) {
	old := memoTags(m.Text)
	m.Text = text
	tags := slices.DeleteFunc(slices.Clone(m.Tags), func(s string) bool { return slices.Contains(old, s) })
	for _, tag := range memoTags(text) {
		if !slices.Contains(tags, tag) {
			tags = append(tags, tag)
		}
	}
	m.Tags = tags
	a.saveClip(m)
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
