package automate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Trigger is when a program runs by itself.
type Trigger struct {
	Every   time.Duration // every so long, or
	Daily   bool          // each day at Hour:Min,
	Weekday int           // or that day of the week (1 Monday … 7 Sunday),
	Quarter bool          // or the last day of each quarter,
	Watch   string        // or when what is in a folder changes
	Hour    int
	Min     int
}

// Manual tells whether it runs only when asked.
func (t Trigger) Manual() bool {
	return t.Every == 0 && !t.Daily && t.Weekday == 0 && !t.Quarter && t.Watch == ""
}

var (
	everyRe   = regexp.MustCompile(`^每\s*(\d*)\s*(分钟|分|小时|时)$`)
	dailyRe   = regexp.MustCompile(`^每天\s*(\d{1,2})[:：](\d{2})$`)
	weeklyRe  = regexp.MustCompile(`^每周([一二三四五六日天1-7])\s*(\d{1,2})[:：](\d{2})$`)
	quarterRe = regexp.MustCompile(`^每季度末\s*(?:(\d{1,2})[:：](\d{2}))?$`)
	weekdays  = map[string]int{"一": 1, "二": 2, "三": 3, "四": 4, "五": 5, "六": 6, "日": 7, "天": 7}
)

// TriggerHelp is how triggers are written.
const TriggerHelp = "手动、每 30 分钟、每 2 小时、每天 09:00、每周一 09:00、每季度末 18:00、监视 ~/Downloads（文件夹里的文件有变化时）"

// ParseTrigger reads a trigger as written in a program's settings.
func ParseTrigger(s string) (Trigger, error) {
	s = strings.TrimSpace(s)
	var t Trigger
	if s == "" || s == "手动" || strings.EqualFold(s, "manual") {
		return t, nil
	}
	if dir, ok := strings.CutPrefix(s, "监视"); ok {
		t.Watch = strings.TrimSpace(dir)
		if t.Watch == "" {
			return t, fmt.Errorf("监视后面要写文件夹：%s", s)
		}
		return t, nil
	}
	if m := everyRe.FindStringSubmatch(s); m != nil {
		n := 1
		if m[1] != "" {
			n, _ = strconv.Atoi(m[1])
		}
		unit := time.Minute
		if m[2] == "小时" || m[2] == "时" {
			unit = time.Hour
		}
		t.Every = time.Duration(n) * unit
		if t.Every < time.Minute {
			return t, fmt.Errorf("触发间隔至少 1 分钟：%s", s)
		}
		return t, nil
	}
	clock := func(h, m string) error {
		t.Hour, _ = strconv.Atoi(h)
		t.Min, _ = strconv.Atoi(m)
		if t.Hour > 23 || t.Min > 59 {
			return fmt.Errorf("时间不对：%s", s)
		}
		return nil
	}
	if m := dailyRe.FindStringSubmatch(s); m != nil {
		t.Daily = true
		return t, clock(m[1], m[2])
	}
	if m := weeklyRe.FindStringSubmatch(s); m != nil {
		if d, ok := weekdays[m[1]]; ok {
			t.Weekday = d
		} else {
			t.Weekday, _ = strconv.Atoi(m[1])
		}
		return t, clock(m[2], m[3])
	}
	if m := quarterRe.FindStringSubmatch(s); m != nil {
		t.Quarter = true
		if m[1] == "" {
			t.Hour = 18
			return t, nil
		}
		return t, clock(m[1], m[2])
	}
	return t, fmt.Errorf("看不懂的触发：%s（可以写%s）", s, TriggerHelp)
}

// Next is the first time after from it runs; zero when only by hand.
func (t Trigger) Next(from time.Time) time.Time {
	switch {
	case t.Every > 0:
		return from.Add(t.Every)
	case t.Daily:
		at := time.Date(from.Year(), from.Month(), from.Day(), t.Hour, t.Min, 0, 0, from.Location())
		if !at.After(from) {
			at = at.AddDate(0, 0, 1)
		}
		return at
	case t.Weekday > 0:
		at := time.Date(from.Year(), from.Month(), from.Day(), t.Hour, t.Min, 0, 0, from.Location())
		wd := int(at.Weekday())
		if wd == 0 {
			wd = 7
		}
		at = at.AddDate(0, 0, (t.Weekday-wd+7)%7)
		if !at.After(from) {
			at = at.AddDate(0, 0, 7)
		}
		return at
	case t.Quarter:
		for m := from.Month(); ; m++ {
			y, mm := from.Year(), m
			for mm > 12 {
				y, mm = y+1, mm-12
			}
			if mm%3 != 0 {
				continue
			}
			// The last day of the month: the day before the next's first.
			at := time.Date(y, mm+1, 1, t.Hour, t.Min, 0, 0, from.Location()).AddDate(0, 0, -1)
			if at.After(from) {
				return at
			}
		}
	}
	return time.Time{}
}

// Due tells whether a program on the trigger should run now: its next
// time after the last run (or since it was on) has come. Runs missed while
// the app was closed or the Mac asleep make one run, not one each.
func (t Trigger) Due(last, now time.Time) bool {
	if t.Manual() || last.IsZero() {
		return false
	}
	next := t.Next(last)
	return !next.IsZero() && !next.After(now)
}

// WatchDir is the folder watched, with ~ for the home folder and a
// relative one in dir, the program's.
func (t Trigger) WatchDir(dir string) string {
	w := t.Watch
	if rest, ok := strings.CutPrefix(w, "~"); ok {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, rest)
	}
	if !filepath.IsAbs(w) {
		return filepath.Join(dir, w)
	}
	return w
}

// Signature tells what is in a folder, in and below it: the files' names,
// sizes and times, but hidden ones, at most a few thousand. It changes as
// they do.
func Signature(dir string) string {
	h := sha256.New()
	n := 0
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path != dir && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if n++; n > 5000 {
			return filepath.SkipAll
		}
		if info, err := d.Info(); err == nil && !d.IsDir() {
			fmt.Fprintf(h, "%s\x00%d\x00%d\n", path, info.Size(), info.ModTime().UnixNano())
		}
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))
}
