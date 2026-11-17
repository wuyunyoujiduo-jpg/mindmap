package main

import (
	"sort"
	"sync"
	"time"
)

// The Chinese calendar's days, under the dates of the week and the month,
// as TickTick's and Feishu's (the user's reference): 初七, the month's name
// on its first day (十月, 闰六月), and the feasts (春节, 中秋), the solar
// terms (立冬, 冬至) and the few holidays of the sun's calendar (元旦,
// 国庆节) instead. From the table that
// calendars commonly carry, of 1900 to 2100: a year's months of thirty
// days (bits 15 to 4, the first month highest), its leap month (bits 3 to
// 0, none at 0), and whether that one has thirty (bit 16). Its first day
// is the first of 1900, 31 January 1900.
var lunarInfo = [...]uint32{
	0x04bd8, 0x04ae0, 0x0a570, 0x054d5, 0x0d260, 0x0d950, 0x16554, 0x056a0, 0x09ad0, 0x055d2, // 1900
	0x04ae0, 0x0a5b6, 0x0a4d0, 0x0d250, 0x1d255, 0x0b540, 0x0d6a0, 0x0ada2, 0x095b0, 0x14977, // 1910
	0x04970, 0x0a4b0, 0x0b4b5, 0x06a50, 0x06d40, 0x1ab54, 0x02b60, 0x09570, 0x052f2, 0x04970, // 1920
	0x06566, 0x0d4a0, 0x0ea50, 0x16a95, 0x05ad0, 0x02b60, 0x186e3, 0x092e0, 0x1c8d7, 0x0c950, // 1930
	0x0d4a0, 0x1d8a6, 0x0b550, 0x056a0, 0x1a5b4, 0x025d0, 0x092d0, 0x0d2b2, 0x0a950, 0x0b557, // 1940
	0x06ca0, 0x0b550, 0x15355, 0x04da0, 0x0a5b0, 0x14573, 0x052b0, 0x0a9a8, 0x0e950, 0x06aa0, // 1950
	0x0aea6, 0x0ab50, 0x04b60, 0x0aae4, 0x0a570, 0x05260, 0x0f263, 0x0d950, 0x05b57, 0x056a0, // 1960
	0x096d0, 0x04dd5, 0x04ad0, 0x0a4d0, 0x0d4d4, 0x0d250, 0x0d558, 0x0b540, 0x0b6a0, 0x195a6, // 1970
	0x095b0, 0x049b0, 0x0a974, 0x0a4b0, 0x0b27a, 0x06a50, 0x06d40, 0x0af46, 0x0ab60, 0x09570, // 1980
	0x04af5, 0x04970, 0x064b0, 0x074a3, 0x0ea50, 0x06b58, 0x05ac0, 0x0ab60, 0x096d5, 0x092e0, // 1990
	0x0c960, 0x0d954, 0x0d4a0, 0x0da50, 0x07552, 0x056a0, 0x0abb7, 0x025d0, 0x092d0, 0x0cab5, // 2000
	0x0a950, 0x0b4a0, 0x0baa4, 0x0ad50, 0x055d9, 0x04ba0, 0x0a5b0, 0x15176, 0x052b0, 0x0a930, // 2010
	0x07954, 0x06aa0, 0x0ad50, 0x05b52, 0x04b60, 0x0a6e6, 0x0a4e0, 0x0d260, 0x0ea65, 0x0d530, // 2020
	0x05aa0, 0x076a3, 0x096d0, 0x04afb, 0x04ad0, 0x0a4d0, 0x1d0b6, 0x0d250, 0x0d520, 0x0dd45, // 2030
	0x0b5a0, 0x056d0, 0x055b2, 0x049b0, 0x0a577, 0x0a4b0, 0x0aa50, 0x1b255, 0x06d20, 0x0ada0, // 2040
	0x14b63, 0x09370, 0x049f8, 0x04970, 0x064b0, 0x168a6, 0x0ea50, 0x06b20, 0x1a6c4, 0x0aae0, // 2050
	0x092e0, 0x0d2e3, 0x0c960, 0x0d557, 0x0d4a0, 0x0da50, 0x05d55, 0x056a0, 0x0a6d0, 0x055d4, // 2060
	0x052d0, 0x0a9b8, 0x0a950, 0x0b4a0, 0x0b6a6, 0x0ad50, 0x055a0, 0x0aba4, 0x0a5b0, 0x052b0, // 2070
	0x0b273, 0x06930, 0x07337, 0x06aa0, 0x0ad50, 0x14b55, 0x04b60, 0x0a570, 0x054e4, 0x0d160, // 2080
	0x0e968, 0x0d520, 0x0daa0, 0x16aa6, 0x056d0, 0x04ae0, 0x0a9d4, 0x0a2d0, 0x0d150, 0x0f252, // 2090
	0x0d520, // 2100
}

// lunarDate is a day of the Chinese calendar: its month (1 to 12), whether
// that is the leap month of its number, its day, and how many days the
// month has.
type lunarDate struct {
	month, day, days int
	leap             bool
}

func lunarLeap(y int) int { return int(lunarInfo[y-1900] & 0xf) }

func lunarLeapDays(y int) int {
	switch {
	case lunarLeap(y) == 0:
		return 0
	case lunarInfo[y-1900]&0x10000 != 0:
		return 30
	}
	return 29
}

func lunarMonthDays(y, m int) int {
	if lunarInfo[y-1900]&(0x10000>>m) != 0 {
		return 30
	}
	return 29
}

func lunarYearDays(y int) int {
	n := 0
	for m := 1; m <= 12; m++ {
		n += lunarMonthDays(y, m)
	}
	return n + lunarLeapDays(y)
}

// lunarYearAt is the day each year of the table begins on, counted from
// the table's first, and after the last the day the table ends.
var lunarYearAt = func() (at [len(lunarInfo) + 1]int) {
	for i := range lunarInfo {
		at[i+1] = at[i] + lunarYearDays(1900+i)
	}
	return at
}()

// lunarOf is the day d of the Chinese calendar, and false out of the
// table's years.
func lunarOf(d time.Time) (lunarDate, bool) {
	day := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
	off := int(day.Sub(time.Date(1900, 1, 31, 0, 0, 0, 0, time.UTC)).Hours() / 24)
	if off < 0 {
		return lunarDate{}, false
	}
	// The year it falls in, by where each begins (lunarYearAt): counted
	// from 1900 a year at a time, a month's forty-two cells walked some
	// five thousand years at every frame.
	i := sort.Search(len(lunarInfo), func(i int) bool { return lunarYearAt[i+1] > off })
	if i == len(lunarInfo) {
		return lunarDate{}, false
	}
	y := 1900 + i
	off -= lunarYearAt[i]
	leap := lunarLeap(y)
	for m := 1; m <= 12; m++ {
		n := lunarMonthDays(y, m)
		if off < n {
			return lunarDate{month: m, day: off + 1, days: n}, true
		}
		off -= n
		if m == leap {
			n = lunarLeapDays(y)
			if off < n {
				return lunarDate{month: m, day: off + 1, days: n, leap: true}, true
			}
			off -= n
		}
	}
	return lunarDate{}, false
}

var (
	lunarMonthNames = []string{"", "正月", "二月", "三月", "四月", "五月", "六月", "七月", "八月", "九月", "十月", "冬月", "腊月"}
	lunarDayNames   = []string{"", "初一", "初二", "初三", "初四", "初五", "初六", "初七", "初八", "初九", "初十",
		"十一", "十二", "十三", "十四", "十五", "十六", "十七", "十八", "十九", "二十",
		"廿一", "廿二", "廿三", "廿四", "廿五", "廿六", "廿七", "廿八", "廿九", "三十"}
	lunarFeasts = map[[2]int]string{{1, 1}: "春节", {1, 15}: "元宵", {5, 5}: "端午", {7, 7}: "七夕", {7, 15}: "中元",
		{8, 15}: "中秋", {9, 9}: "重阳", {12, 8}: "腊八"}
	solarHolidays = map[[2]int]string{{1, 1}: "元旦", {5, 1}: "劳动节", {6, 1}: "儿童节", {10, 1}: "国庆节"}
)

// lunarLabel names the day d by the Chinese calendar, in a word: a feast
// or a holiday, the month's name on its first day, else its day; and
// whether it is a feast or a holiday. Nothing out of the table's years.
//
// A day's word is of the day alone and never changes: it is worked out
// once and kept (lunarWords), a calendar asking for each cell's at every
// frame.
func lunarLabel(d time.Time) (string, bool) {
	y, m, day := d.Date()
	key := [3]int{y, int(m), day}
	lunarWords.Lock()
	defer lunarWords.Unlock()
	if w, ok := lunarWords.of[key]; ok {
		return w.word, w.feast
	}
	if lunarWords.of == nil || len(lunarWords.of) > 4000 {
		lunarWords.of = map[[3]int]lunarWord{}
	}
	var w lunarWord
	w.word, w.feast = lunarWordOf(d)
	lunarWords.of[key] = w
	lunarWords.made++
	return w.word, w.feast
}

type lunarWord struct {
	word  string
	feast bool
}

// lunarWords are the days' words as worked out, by the day; made counts
// them, for the tests.
var lunarWords struct {
	sync.Mutex
	of   map[[3]int]lunarWord
	made int
}

// lunarWordOf works out lunarLabel's word.
func lunarWordOf(d time.Time) (string, bool) {
	if h, ok := solarHolidays[[2]int{int(d.Month()), d.Day()}]; ok {
		return h, true
	}
	l, ok := lunarOf(d)
	if !ok {
		return "", false
	}
	if !l.leap {
		if l.month == 12 && l.day == l.days {
			return "除夕", true
		}
		if f, ok := lunarFeasts[[2]int{l.month, l.day}]; ok {
			return f, true
		}
	}
	if term, ok := solarTerm(d); ok { // solarterm.go
		return term, true
	}
	if l.day == 1 {
		name := lunarMonthNames[l.month]
		if l.leap {
			name = "闰" + name
		}
		return name, false
	}
	return lunarDayNames[l.day], false
}
