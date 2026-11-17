package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// China's holidays and the weekend days worked for them (调休), as the
// State Council's notice of each year says: the app has no such data of
// its own and is not to guess it (the user, 0.105). They are read from
// holiday-cn (github.com/NateScarlet/holiday-cn), which keeps each year's
// notice as a file and gives its paper's address, fetched once a day for
// the year before, this one and the next, and kept; a year not yet out is
// as weeks are, Monday to Friday, and the page that leans on it says so.
// The calendars mark a day off 休 and a weekend day worked 班, and the
// repeat 法定工作日 (model.RepeatWorkday) comes on the days worked.

// holidayURLs are where a year's file is, tried in turn: GitHub's, then a
// mirror of it reached where GitHub is not.
var holidayURLs = []string{
	"https://raw.githubusercontent.com/NateScarlet/holiday-cn/master/%d.json",
	"https://cdn.jsdelivr.net/gh/NateScarlet/holiday-cn@master/%d.json",
}

// holidayFile is a year's file of holiday-cn.
type holidayFile struct {
	Year   int      `json:"year"`
	Papers []string `json:"papers"`
	Days   []struct {
		Name string `json:"name"`
		Date string `json:"date"`
		Off  bool   `json:"isOffDay"`
	} `json:"days"`
}

// holidayDay is a day the notice names: off, or a weekend day worked.
type holidayDay struct {
	Name string
	Off  bool
}

var holidays struct {
	sync.RWMutex
	days  map[string]holidayDay // 2006-01-02 →
	years map[int]bool          // the years whose notice is known
	// loads counts the times they were read or forgotten: what is worked
	// out of the days worked is of them as they stood.
	loads int
}

// holidaysLoads tells which reading of the notices stands (holidays.loads),
// 0 before any.
func holidaysLoads() int {
	holidays.RLock()
	defer holidays.RUnlock()
	if holidays.days == nil {
		return -holidays.loads
	}
	return holidays.loads
}

func holidaysDir() string {
	if p := configPath(); p != "" {
		return filepath.Join(filepath.Dir(p), "holidays")
	}
	return ""
}

// readHolidays parses a year's file, nil when it is not one.
func readHolidays(data []byte) *holidayFile {
	var f holidayFile
	if json.Unmarshal(data, &f) != nil || f.Year < 2000 || len(f.Days) == 0 {
		return nil
	}
	return &f
}

// loadHolidays reads what was kept of the notices.
func loadHolidays() {
	days, years := map[string]holidayDay{}, map[int]bool{}
	if dir := holidaysDir(); dir != "" {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			f := readHolidays(data)
			if err != nil || f == nil {
				continue
			}
			years[f.Year] = true
			for _, d := range f.Days {
				days[d.Date] = holidayDay{Name: d.Name, Off: d.Off}
			}
		}
	}
	holidays.Lock()
	holidays.days, holidays.years = days, years
	holidays.loads++
	holidays.Unlock()
}

// holidayOn is what the notice says of the day d, if it names it.
func holidayOn(d time.Time) (holidayDay, bool) {
	holidays.RLock()
	defer holidays.RUnlock()
	h, ok := holidays.days[d.Format("2006-01-02")]
	return h, ok
}

// holidayYearKnown reports whether the notice of the year y is known.
func holidayYearKnown(y int) bool {
	holidays.RLock()
	defer holidays.RUnlock()
	return holidays.years[y]
}

// holidayYears are the years whose notice is known, in order.
func holidayYears() []int {
	holidays.RLock()
	defer holidays.RUnlock()
	var ys []int
	for y := range holidays.years {
		ys = append(ys, y)
	}
	slices.Sort(ys)
	return ys
}

// workday reports whether the day d is one worked: by the notice where it
// names it, else Monday to Friday.
func workday(d time.Time) bool {
	if h, ok := holidayOn(d); ok {
		return !h.Off
	}
	wd := d.Weekday()
	return wd != time.Saturday && wd != time.Sunday
}

func init() { model.Workday = workday }

// holidaysFetch fetches the notices of the year before, this one and the
// next, each once a day; a year not out yet is asked again the next day.
func (a *app) holidaysFetch() {
	dir := holidaysDir()
	if dir == "" || a.cfg.NoHolidays {
		return
	}
	// It is asked for whenever the app is come back to. What was kept is
	// read once, and again once something was fetched or the settings
	// forgot it; and whether a year is due is looked for once an hour: every
	// year's file was read and parsed each time, before any was found due.
	if holidaysLoads() <= 0 {
		loadHolidays()
	}
	if at := a.memo.holidaysAt; !at.IsZero() && !a.now().Before(at) && a.now().Sub(at) < time.Hour {
		return
	}
	a.memo.holidaysAt = a.now()
	y := a.now().Year()
	var want []int
	for _, year := range []int{y - 1, y, y + 1} {
		p := filepath.Join(dir, strconv.Itoa(year)+".json")
		if fi, err := os.Stat(p); err == nil && a.now().Sub(fi.ModTime()) < 24*time.Hour {
			continue
		}
		if a.holidaysAsked[year].After(a.now().Add(-24 * time.Hour)) {
			continue
		}
		if a.holidaysAsked == nil {
			a.holidaysAsked = map[int]time.Time{}
		}
		a.holidaysAsked[year] = a.now()
		want = append(want, year)
	}
	if len(want) == 0 {
		return
	}
	get := a.pluginGet
	a.async(func() (any, error) {
		for _, year := range want {
			for _, u := range holidayURLs {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				data, err := get(ctx, fmt.Sprintf(u, year))
				cancel()
				if f := readHolidays(data); err == nil && f != nil && f.Year == year {
					_ = os.MkdirAll(dir, 0o755)
					_ = os.WriteFile(filepath.Join(dir, strconv.Itoa(year)+".json"), data, 0o644)
					break
				}
			}
		}
		return nil, nil
	}, func(any, error) { loadHolidays() })
}

// holidayYearsSaid says which years' notices are known, for the settings.
func holidayYearsSaid() string {
	ys := holidayYears()
	if len(ys) == 0 {
		return "还没取到"
	}
	s := ""
	for i, y := range ys {
		if i > 0 {
			s += "、"
		}
		s += strconv.Itoa(y)
	}
	return s + " 年"
}

// clearHolidays forgets the notices, the settings having turned them off.
func clearHolidays() {
	holidays.Lock()
	holidays.days, holidays.years = nil, nil
	holidays.loads++
	holidays.Unlock()
}

// holidayMark is the small mark of a day the notice names, by its date:
// 休 for a day off, 班 for a weekend day worked; its name under the
// pointer.
func holidayMark(c *ui.Context, d time.Time) {
	h, ok := holidayOn(d)
	if !ok {
		return
	}
	t := c.Theme()
	word, col, says := "休", t.Success, h.Name+"假期"
	if !h.Off {
		word, col, says = "班", t.Danger, h.Name+"调休，上班"
	}
	ui.Box(c.Key("holiday-mark")).Padding(0, 3).Radius(4).Background(col.Alpha(0.12)).Shrink(0).Tooltip(says).Label(word + " " + says).Children(func() {
		ui.Text(c, word).FontSize(textCaption - 1).FontWeight(700).TextColor(col).NoWrap()
	})
}
