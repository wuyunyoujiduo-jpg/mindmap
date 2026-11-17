package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// A task written in a line: 明天 9点到10点半 开会 !高, 每周五 写周报. One
// function reads it (readTaskLine), for the line to add a task, which says
// what it reads as it is written, for the bar's tag and for the task made.
// They read it each its own way, the first knowing no time and the second
// no time either, and what a line showed was not what was kept of it.

// taskLine is a task as its line says it.
type taskLine struct {
	title       string
	due         *time.Time // its day: the line's, or the one of where it was written; nil for none
	from, until string     // its time of day, as 15:04
	remind      string     // when it reminds: a time written is when it is, and when it reminds
	repeat      *model.Repeat
	prio        model.Priority
	links       []string // what the line links to ([[): a skill to do it with, a clip, a topic
}

// readTaskLine reads a task written in a line. day is the day it goes on
// where the line says none (nil for none); at and till the time of day it
// is at, as 15:04, unless the line says another. A time with no day is
// today's; what repeats has a day, today where none is said (repeatProp's
// rule: 每2周 … made a task that repeated and had no day).
func readTaskLine(line string, now time.Time, day *time.Time, at, till string) taskLine {
	var tl taskLine
	// What the line links to is the task's links, not words of its title.
	tl.links = tokenLinks(line)
	line = strings.TrimSpace(refToken.ReplaceAllString(line, ""))
	// How it repeats stands at the line's head, and its time is read of
	// what follows it (每天8点跑步: the time at the head of 8点跑步); where
	// the line begins with its time, how it repeats is read after that.
	said, rep, first := parseRepeat(line, now)
	rest, from, until, nextDay, clock := cutClock(said)
	after := rep == nil
	if after {
		rest, rep, first = parseRepeat(rest, now)
	}
	if nextDay && first != nil {
		// Midnight is the day after's, and a repeat on days of its own
		// (每周五, 每天) says the day: the time is left as words, not read
		// wrong, and how it repeats stands (每天晚上12点 睡觉 read nothing:
		// the whole line was read again, as one with no time).
		from, until, nextDay = "", "", false
		if after {
			rest = strings.TrimSpace(clock + " " + rest)
		} else {
			rest = said
		}
	}
	if from == "" {
		from, until = at, till
	}
	title, due, prio := parseTaskLine(rest, now)
	switch {
	case due != nil:
	case first != nil:
		due = first // the first day what repeats falls on
	case day != nil:
		d := *day
		due = &d
	case from != "" || rep != nil:
		d := today(now)
		due = &d
	}
	if nextDay && due != nil {
		d := due.AddDate(0, 0, 1)
		due = &d
	}
	tl.title, tl.due, tl.prio, tl.repeat = title, due, prio, rep
	tl.from, tl.until, tl.remind = from, until, from
	return tl
}

// span is when in its day the task is: 9:00–10:30, 9:00, or nothing.
func (tl taskLine) span() string {
	if tl.due == nil {
		return ""
	}
	return (&model.Task{Time: tl.from, EndTime: tl.until}).Span()
}

// day is the task's day and its time, as a few words: 明天 9:00–10:30;
// "" where it has no day.
func (tl taskLine) day(now time.Time) string {
	if tl.due == nil {
		return ""
	}
	s := dayLabel(*tl.due, now)
	if span := tl.span(); span != "" {
		s += " " + span
	}
	return s
}

// when says when the task is, whole: 明天 9:00，每周五; 没定日期.
func (tl taskLine) when(now time.Time) string {
	s := tl.day(now)
	if s == "" {
		s = "没定日期"
	}
	if tl.repeat != nil {
		s += "，" + tl.repeat.Label()
	}
	return s
}

// checklist are the tasks of a text written as a checklist, a line each
// (- [ ] 买菜), or nil where it is not one: every line is to begin so.
func checklist(text string) []string {
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(text), "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(l), "- [ ]")
		if !ok {
			return nil
		}
		if rest = strings.TrimSpace(rest); rest != "" {
			out = append(out, rest)
		}
	}
	return out
}

// addTask adds a task written in a line, with its day in words, to the
// inbox; day, when not nil, is the day it goes on when the line says none.
func (a *app) addTask(line string, day *time.Time) { a.addTaskAt(line, day, "", "") }

// addTaskAt is addTask with the time of the day the task is at, from and
// until as 15:04 (until may be ""), unless its line says another.
func (a *app) addTaskAt(line string, day *time.Time, at, till string) {
	if tl, ok := a.putTask(line, day, at, till); ok {
		a.tell("已加到收件箱：" + tl.title + "（" + tl.when(a.now()) + "）")
	}
}

// addTasks adds tasks written a line each, as a checklist's, and says so
// once: each said by itself, only the last was read.
func (a *app) addTasks(lines []string, day *time.Time) {
	var made []taskLine
	for _, l := range lines {
		if tl, ok := a.putTask(l, day, "", ""); ok {
			made = append(made, tl)
		}
	}
	switch len(made) {
	case 0:
	case 1:
		a.tell("已加到收件箱：" + made[0].title + "（" + made[0].when(a.now()) + "）")
	default:
		a.tell(fmt.Sprintf("已加到收件箱：%d 件任务", len(made)))
	}
}

// putTask makes the task a line says, in the inbox, and tells what it
// read and whether it was made.
func (a *app) putTask(line string, day *time.Time, at, till string) (taskLine, bool) {
	tl := readTaskLine(line, a.now(), day, at, till)
	if tl.title == "" {
		return tl, false
	}
	n := model.NewNode(tl.title)
	n.SetStatus(model.StatusTodo, a.now())
	n.Task.Due, n.Task.Priority, n.Task.Repeat, n.Task.Remind = tl.due, tl.prio, tl.repeat, tl.remind
	n.Task.Time, n.Task.EndTime = tl.from, tl.until
	n.Links = tl.links
	path := a.inboxPath()
	if err := a.ensureInbox(); err != nil {
		a.tell("没有加上：" + err.Error())
		return tl, false
	}
	if err := a.editMapAt(path, "加任务", func(f *model.File) error {
		f.Sheets[0].Root.Children = append(f.Sheets[0].Root.Children, n)
		return nil
	}); err != nil {
		a.tell("没有加上：" + err.Error())
		return tl, false
	}
	a.tasks.read = time.Time{}
	a.remindOwn(path + "#" + n.ID) // made here today: the day is not said for it alone
	return tl, true
}

// remindWord is a time of day written in a task's line: 9点, 9点半, 下午3点,
// 晚上8点15, 15:30.
var remindWord = regexp.MustCompile(`(上午|早上|中午|下午|晚上)?\s*(\d{1,2})(?:[:：](\d{2})|[点时](半|\d{1,2})?分?)`)

// spanTo is what joins the two ends of a time of day: 9点到10点, 9:00-10:30.
var spanTo = regexp.MustCompile(`^\s*(?:到|至|-|~|～|–|—)\s*`)

// dayWordEnd is a day's word at the end of what stands before a time,
// which then needs no space to be one: 明天9点, 周五9点, 20号9点.
var dayWordEnd = regexp.MustCompile(`(?:今天|明天|后天|今晚|(?:周|星期|礼拜)[一二三四五六日天]|\d{1,2}\s*[日号])$`)

// clockWord is a time of day as written: its part of the day (下午), its
// hour and minute as said, whether as a clock's (15:30), and whether to
// the minute (半, 15分).
type clockWord struct {
	period string
	h, min int
	clock  bool
	exact  bool
}

// clockAt reads the time of day that m, a match of remindWord, found in s.
func clockAt(s string, m []int) clockWord {
	part := func(i int) string {
		if m[2*i] < 0 {
			return ""
		}
		return s[m[2*i]:m[2*i+1]]
	}
	w := clockWord{period: part(1), clock: part(3) != "", exact: part(4) != ""}
	w.h, _ = strconv.Atoi(part(2))
	switch {
	case w.clock:
		w.min, _ = strconv.Atoi(part(3))
	case part(4) == "半":
		w.min = 30
	case part(4) != "":
		w.min, _ = strconv.Atoi(part(4))
	}
	return w
}

// parseRemind takes the time to be reminded at out of a task's line: the
// line without it, and the time as 15:04, or none.
func parseRemind(line string) (rest, remind string) {
	rest, remind, _ = parseClock(line)
	return rest, remind
}

// parseClock is readClock for a time written of a task that has its day
// (its details' fields): midnight said as 晚上12点 is the day after's,
// which a day's time cannot say, and is not read.
func parseClock(line string) (rest, from, until string) {
	rest, from, until, next := readClock(line)
	if next {
		return line, "", ""
	}
	return rest, from, until
}

// readClock takes the time of day out of a task's line: the line without
// it, and when it begins and, of a span (9点到10点半, 14:00-15:30), ends,
// as 15:04, or none. A time is one that stands by itself, at the line's
// head (9点开会) or after a day's word (明天9点开会), not a number of
// things (写3点总结, 准备 3点建议).
//
// 晚上12点 is midnight: 0:00 of the day after, which nextDay tells (it was
// read as noon). A span has an end only where it ends after it begins, in
// its day: 9点到9点 has none, nor has one that goes over midnight, which a
// day cannot hold; its beginning is kept and nothing wrong is shown.
func readClock(line string) (rest, from, until string, nextDay bool) {
	rest, from, until, nextDay, _ = cutClock(line)
	return rest, from, until, nextDay
}

// notClock is what, right after a time's words, makes them no time of the
// task: a time to be done by or about (2点前交报告, 3点左右, 10点多), and
// so many points of a thing (3点建议). At the line's head they were read
// as its time, and 前交报告 was a task at two.
var notClock = regexp.MustCompile(`^(?:前|后|内|钟|左右|多|建议|意见|总结|要求|看法|想法|体会|心得|感想|问题|原因|理由|说明|结论|注意|思考|收获|要点|事项)`)

// cutClock is readClock, with the time's words as the line had them.
func cutClock(line string) (rest, from, until string, nextDay bool, words string) {
	for _, m := range remindWord.FindAllStringSubmatchIndex(line, -1) {
		w := clockAt(line, m)
		before := line[:m[0]]
		// Of a span, its end: written after it, joined by 到 or a dash.
		var end *clockWord
		endAt := m[1]
		if j := spanTo.FindStringIndex(line[m[1]:]); j != nil {
			tail := line[m[1]+j[1]:]
			if e := remindWord.FindStringSubmatchIndex(tail); e != nil && e[0] == 0 {
				ew := clockAt(tail, e)
				end, endAt = &ew, m[1]+j[1]+e[1]
			}
		}
		if notClock.MatchString(line[endAt:]) {
			continue
		}
		after := []rune(line[endAt:])
		lead := strings.TrimRightFunc(before, unicode.IsSpace)
		afterDay := dayWordEnd.MatchString(lead)
		spaced := unicode.IsSpace([]rune(line[m[0]:])[0]) // the space before it is in what was found
		if r := []rune(before); len(r) > 0 && w.period == "" && !spaced && lead == before && !afterDay && !strings.ContainsRune("天晚早午上", r[len(r)-1]) {
			continue // 写3点总结: in the middle of words
		}
		if end == nil && !w.clock && w.period == "" && !w.exact && len(after) > 0 && !unicode.IsSpace(after[0]) && lead != "" && !afterDay {
			continue // 3点建议: three things, not three o'clock
		}
		h, next := w.h, false
		night := w.period == "晚上" || w.period == "" && strings.Contains(before, "今晚")
		switch {
		case night && h == 12:
			h, next = 0, true
		case (night || w.period == "下午") && h < 12:
			h += 12
		case w.period == "中午" && h < 11:
			h += 12
		}
		if h > 23 || w.min > 59 {
			continue
		}
		from = clockText(h*60 + w.min)
		if end != nil && end.min <= 59 {
			eh := end.h
			if (end.period == "下午" || end.period == "晚上" || end.period == "中午" && eh < 11) && eh < 12 {
				eh += 12
			}
			start, stop := h*60+w.min, eh*60+end.min
			switch {
			case stop < start && eh < 12: // 下午3点到5点: five in the afternoon too
				stop += 12 * 60
			case stop <= start && eh == 12 && end.min == 0: // 晚上10点到12点: to midnight
				stop = 24 * 60
			}
			if stop > start && stop <= 24*60 {
				until = clockText(min(stop, 24*60-1))
			}
		}
		return strings.TrimSpace(line[:m[0]] + " " + line[endAt:]), from, until, next, strings.TrimSpace(line[m[0]:endAt])
	}
	return line, "", "", false, ""
}

var (
	// A day of a month is written with 月 or a slash: 11月20日, 11/20. A
	// dot or a dash between two numbers is a version or a range (升级到 1.2
	// 版本, 买 2-3 个苹果, 看 3.5 寸屏), which were read as days.
	taskMonthDay = regexp.MustCompile(`(\d{1,2})\s*月\s*(\d{1,2})\s*([日号])?`)
	taskSlashDay = regexp.MustCompile(`\s(\d{1,2})/(\d{1,2})\s`)
	taskDayOnly  = regexp.MustCompile(`(^|\s)(\d{1,2})\s*[日号](\s|$)`)
	taskWeekday  = regexp.MustCompile(`(下)?(?:周|星期|礼拜)([一二三四五六日天])`)
	taskPrio     = regexp.MustCompile(`[!！](高|中|低|1|2|3)`)
)

// monthDayFrom is the day d of the month mo as a day from day on: this
// year's, or next year's where this year's is long past; ok is false
// where the calendar has no such day (2月30日 was read as the 2nd of
// March).
func monthDayFrom(day time.Time, mo, d int) (time.Time, bool) {
	if mo < 1 || mo > 12 || d < 1 || d > 31 {
		return time.Time{}, false
	}
	for _, y := range []int{day.Year(), day.Year() + 1} {
		t := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, day.Location())
		if t.Month() != time.Month(mo) || t.Day() != d {
			continue
		}
		if y == day.Year() && t.Before(day.AddDate(0, -1, 0)) {
			continue // long past: next year's
		}
		return t, true
	}
	return time.Time{}, false
}

// parseTaskLine reads a task written in a line: its title, and the day
// and priority it says, in words: 今天, 明天, 后天, 大后天, 周五, 下周一,
// 11月20日, 11/20, 20号; !高 !中 !低. A day the calendar has not is no day,
// and stays words of the title: 31号 in a month of thirty days was read
// as the first of the next.
func parseTaskLine(line string, now time.Time) (string, *time.Time, model.Priority) {
	s := " " + strings.TrimSpace(line) + " "
	day := today(now)
	var due *time.Time
	set := func(t time.Time) { due = &t }
	digit := func(b byte) bool { return b >= '0' && b <= '9' }
	for _, w := range []struct {
		word string
		add  int
	}{{"大后天", 3}, {"后天", 2}, {"明天", 1}, {"今天", 0}, {"今晚", 0}} {
		if strings.Contains(s, w.word) {
			s = strings.Replace(s, w.word, " ", 1)
			set(day.AddDate(0, 0, w.add))
			break
		}
	}
	if due == nil {
		for _, m := range taskWeekday.FindAllStringSubmatchIndex(s, -1) {
			// 每周一次, 上周五的纪要: every week, and a week gone by, not
			// the Monday or the Friday to come.
			if before := s[:m[0]]; strings.HasSuffix(before, "每") || strings.HasSuffix(before, "上") {
				continue
			}
			want := map[string]int{"一": 1, "二": 2, "三": 3, "四": 4, "五": 5, "六": 6, "日": 7, "天": 7}[s[m[4]:m[5]]]
			wd := int(day.Weekday())
			if wd == 0 {
				wd = 7
			}
			ahead := (want - wd + 7) % 7
			if m[2] >= 0 { // 下周: in the week after this one
				ahead = want - wd + 7
			}
			set(day.AddDate(0, 0, ahead))
			s = s[:m[0]] + " " + s[m[1]:]
			break
		}
	}
	if due == nil {
		for _, m := range taskMonthDay.FindAllStringSubmatchIndex(s, -1) {
			// A number of its own: not the end of a longer one, and, with
			// no 日 after it, not the beginning of one.
			if b := s[m[0]-1]; digit(b) || b == '.' || b == '/' {
				continue
			}
			// What follows the day's own digits: what was found ends after
			// the spaces behind them, at the line's end past the line (发布
			// 11月2, as 11月20日 is being typed, took the frame down).
			if r, _ := utf8.DecodeRuneInString(s[m[5]:]); m[6] < 0 && !unicode.IsSpace(r) {
				continue
			}
			mo, _ := strconv.Atoi(s[m[2]:m[3]])
			d, _ := strconv.Atoi(s[m[4]:m[5]])
			if t, ok := monthDayFrom(day, mo, d); ok {
				set(t)
				s = s[:m[0]] + " " + s[m[1]:]
				break
			}
		}
	}
	if due == nil {
		if m := taskSlashDay.FindStringSubmatchIndex(s); m != nil {
			mo, _ := strconv.Atoi(s[m[2]:m[3]])
			d, _ := strconv.Atoi(s[m[4]:m[5]])
			if t, ok := monthDayFrom(day, mo, d); ok {
				set(t)
				s = s[:m[0]] + " " + s[m[1]:]
			}
		}
	}
	if due == nil {
		if m := taskDayOnly.FindStringSubmatchIndex(s); m != nil {
			d, _ := strconv.Atoi(s[m[4]:m[5]])
			// This month's, or the next's once it is past: where the month
			// has that day.
			t := time.Date(day.Year(), day.Month(), d, 0, 0, 0, 0, day.Location())
			ok := d >= 1 && t.Day() == d
			if ok && t.Before(day) {
				t = time.Date(day.Year(), day.Month()+1, d, 0, 0, 0, 0, day.Location())
				ok = t.Day() == d
			}
			if ok {
				set(t)
				s = s[:m[0]] + " " + s[m[1]:]
			}
		}
	}
	prio := model.PriorityNone
	if m := taskPrio.FindStringSubmatchIndex(s); m != nil {
		prio = map[string]model.Priority{"高": model.PriorityHigh, "1": model.PriorityHigh, "中": model.PriorityMedium,
			"2": model.PriorityMedium, "低": model.PriorityLow, "3": model.PriorityLow}[s[m[2]:m[3]]]
		s = s[:m[0]] + " " + s[m[1]:]
	}
	return strings.Join(strings.Fields(s), " "), due, prio
}

var (
	repeatWeekly  = regexp.MustCompile(`^每(?:周|星期|礼拜)([一二三四五六日天])`)
	repeatMonthly = regexp.MustCompile(`^每月\s*(\d{1,2})\s*[日号]`)
	repeatEvery   = regexp.MustCompile(`^每(?:隔)?\s*(\d{1,2}|两|二|三|四)\s*(天|周|个月|月|年)`)
	repeatByWeek  = regexp.MustCompile(`^每(?:周|星期|礼拜)`)
	repeatByMonth = regexp.MustCompile(`^每(?:个)?月`)
)

// repeatWord cuts a way to repeat off the head of a line. It needs no
// space after it, Chinese being written without (每天喝水, 每周五写周报,
// 工作日9点 站会): it is no way to repeat only where the next character
// goes on with it into other words, 工作日报 已提交 and 每天都很累, which
// were made tasks of every working day and of every day (and then, a space
// asked for, 每天喝水 read nothing).
func repeatWord(s, word string) (rest string, ok bool) {
	rest, ok = strings.CutPrefix(s, word)
	if !ok {
		return s, false
	}
	goesOn := repeatGoesOn
	if strings.HasSuffix(word, "工作日") {
		goesOn = workdayGoesOn
	}
	if r, _ := utf8.DecodeRuneInString(rest); rest != "" && strings.ContainsRune(goesOn, r) {
		return s, false
	}
	return strings.TrimSpace(rest), true
}

const (
	// repeatGoesOn is what, after 每天, 每周五, 每月, 每年 and their like,
	// makes a sentence of it: 每天都很累, 每年的今天.
	repeatGoesOn = "都也还就在的会要是很没不又总只才"
	// workdayGoesOn is what makes another word of 工作日: 工作日报, 工作日志.
	workdayGoesOn = "报志记程常历里的"
)

// repeatCount is so many times a week, a month or a year (每周一次, 每月
// 两次, 每年 3 次), and a part of one (每周末, 每月初, 每月底): no way to
// repeat, though it begins as one does. 回 is a time only at a word's
// end: 每周五回家 is every Friday.
var repeatCount = regexp.MustCompile(`^每(?:周|星期|礼拜|个?月|年)\s*(?:(?:[一两二三四五六七八九十几]|\d+\s*)(?:次|回(?:\s|的|$))|[末初底])`)

// parseRepeat reads how a task repeats where its line begins with it:
// 每天, 工作日, 法定工作日, 每周, 每周五, 每月, 每月15号, 每月最后一天, 每年,
// 每2周, 每三天. It gives the rest of the line, the repeat, and the first
// day it falls on, nil where that is the line's own day or today; rep is
// nil where the line says none.
func parseRepeat(line string, now time.Time) (rest string, rep *model.Repeat, first *time.Time) {
	s := strings.TrimSpace(line)
	if repeatCount.MatchString(s) {
		return line, nil, nil
	}
	day := today(now)
	wd := int(day.Weekday())
	if wd == 0 {
		wd = 7
	}
	// The line begins with one way to repeat at most, the longest that
	// fits: where what follows that one goes on into other words, it says
	// none, and no shorter one is read of it (每周五都要开会 is not 每周).
	none := false
	word := func(w string) (string, bool) {
		if none || !strings.HasPrefix(s, w) {
			return s, false
		}
		rest, ok := repeatWord(s, w)
		none = !ok
		return rest, ok
	}
	// found cuts what a pattern found off the line's head, as word.
	found := func(re *regexp.Regexp) ([]string, string, bool) {
		m := re.FindStringSubmatch(s)
		if m == nil {
			return nil, s, false
		}
		rest, ok := word(m[0])
		return m, rest, ok
	}
	if rest, ok := word("每天"); ok {
		return rest, &model.Repeat{Unit: model.RepeatDay}, &day
	}
	for _, w := range []string{"每个法定工作日", "法定工作日"} {
		if rest, ok := word(w); ok {
			r := &model.Repeat{Unit: model.RepeatWorkday}
			first := r.Next(nil, day.AddDate(0, 0, -1)) // today, if worked
			return rest, r, &first
		}
	}
	for _, w := range []string{"每个工作日", "工作日"} {
		if rest, ok := word(w); ok {
			if wd > 5 { // on a weekend: from Monday
				day = day.AddDate(0, 0, 8-wd)
			}
			return rest, &model.Repeat{Unit: model.RepeatWeek, Days: []int{1, 2, 3, 4, 5}}, &day
		}
	}
	if m, rest, ok := found(repeatWeekly); ok {
		want := map[string]int{"一": 1, "二": 2, "三": 3, "四": 4, "五": 5, "六": 6, "日": 7, "天": 7}[m[1]]
		day = day.AddDate(0, 0, (want-wd+7)%7)
		return rest, &model.Repeat{Unit: model.RepeatWeek, Days: []int{want}}, &day
	}
	if m, rest, ok := found(repeatMonthly); ok {
		if d, _ := strconv.Atoi(m[1]); d >= 1 && d <= 31 {
			r := &model.Repeat{Unit: model.RepeatMonth, MonthDays: []int{d}}
			t := r.Next(nil, day.AddDate(0, 0, -1)) // its first: today, or after
			return rest, r, &t
		}
	}
	if rest, ok := word("每月最后一天"); ok {
		r := &model.Repeat{Unit: model.RepeatMonth, MonthDays: []int{-1}}
		t := r.Next(nil, day.AddDate(0, 0, -1))
		return rest, r, &t
	}
	if rest, ok := word("每年"); ok {
		return rest, &model.Repeat{Unit: model.RepeatYear}, nil // its day is the line's, or today
	}
	if m, rest, ok := found(repeatEvery); ok {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			n = map[string]int{"两": 2, "二": 2, "三": 3, "四": 4}[m[1]]
		}
		unit := map[string]string{"天": model.RepeatDay, "周": model.RepeatWeek, "个月": model.RepeatMonth, "月": model.RepeatMonth, "年": model.RepeatYear}[m[2]]
		if n >= 1 {
			return rest, &model.Repeat{Every: n, Unit: unit}, nil
		}
	}
	// By the week or the month and no more said: from its own day, the
	// line's or today (每周 开例会, 每月 交房租 were not understood).
	if _, rest, ok := found(repeatByWeek); ok {
		return rest, &model.Repeat{Unit: model.RepeatWeek}, nil
	}
	if _, rest, ok := found(repeatByMonth); ok {
		return rest, &model.Repeat{Unit: model.RepeatMonth}, nil
	}
	return line, nil, nil
}
