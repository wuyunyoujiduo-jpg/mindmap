package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// A year's notice, as holiday-cn keeps it: 国庆节 off from the 1st to the
// 7th of October, Sunday the 27th of September and Saturday the 10th of
// October worked.
const testHolidays2026 = `{"year": 2026, "papers": ["https://www.gov.cn/"], "days": [
	{"name": "国庆节", "date": "2026-09-27", "isOffDay": false},
	{"name": "国庆节", "date": "2026-10-01", "isOffDay": true},
	{"name": "国庆节", "date": "2026-10-02", "isOffDay": true},
	{"name": "国庆节", "date": "2026-10-05", "isOffDay": true},
	{"name": "国庆节", "date": "2026-10-06", "isOffDay": true},
	{"name": "国庆节", "date": "2026-10-07", "isOffDay": true},
	{"name": "国庆节", "date": "2026-10-10", "isOffDay": false}]}`

// A day of rest is a day the notice gives off, or a Saturday or Sunday it
// does not name as worked; a year with no notice, and the holidays turned
// off, go by Saturday and Sunday alone.
func TestRestDay(t *testing.T) {
	defer clearHolidays()
	f := readHolidays([]byte(testHolidays2026))
	if f == nil {
		t.Fatal("the notice is not read")
	}
	holidays.Lock()
	holidays.days, holidays.years = map[string]holidayDay{}, map[int]bool{f.Year: true}
	for _, day := range f.Days {
		holidays.days[day.Date] = holidayDay{Name: day.Name, Off: day.Off}
	}
	holidays.loads++
	holidays.Unlock()
	d := func(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 0, 0, 0, 0, time.Local) }
	for _, c := range []struct {
		day  time.Time
		rest bool
		why  string
	}{
		{d(2026, 10, 1), true, "Thursday, off"},
		{d(2026, 10, 2), true, "Friday, off"},
		{d(2026, 10, 3), true, "Saturday of the holiday, which the notice need not name"},
		{d(2026, 10, 4), true, "Sunday of the holiday"},
		{d(2026, 10, 5), true, "Monday, off"},
		{d(2026, 10, 6), true, "Tuesday, off"},
		{d(2026, 10, 7), true, "Wednesday, off"},
		{d(2026, 10, 8), false, "Thursday, worked"},
		{d(2026, 9, 27), false, "Sunday, worked for the holiday"},
		{d(2026, 10, 10), false, "Saturday, worked for the holiday"},
		{d(2026, 10, 11), true, "a Sunday as any"},
		{d(2026, 9, 26), true, "a Saturday as any"},
		{d(2027, 10, 1), false, "a Friday of a year with no notice"},
		{d(2027, 10, 2), true, "a Saturday of a year with no notice"},
		{d(2027, 10, 3), true, "a Sunday of a year with no notice"},
	} {
		if got := restDay(c.day); got != c.rest {
			t.Errorf("%s (%s): rest %v, want %v", c.day.Format("2006-01-02"), c.why, got, c.rest)
		}
		if restDay(c.day) == workday(c.day) {
			t.Errorf("%s is both of rest and worked, or neither", c.day.Format("2006-01-02"))
		}
	}
	// The holidays turned off in the settings: the weekends alone.
	clearHolidays()
	if restDay(d(2026, 10, 1)) || !restDay(d(2026, 10, 10)) || !restDay(d(2026, 10, 11)) || restDay(d(2026, 9, 28)) {
		t.Error("with no notices, rest is not Saturday and Sunday alone")
	}
}

// The notices are fetched, by the mirror where GitHub fails, and kept; the
// calendars mark 休 and 班; 法定工作日 comes on the days worked; a year with
// no notice is by the week, and said so. The week may begin on Sunday.
func TestHolidays(t *testing.T) {
	a, tt := newTestApp(t)
	defer clearHolidays()
	var asked []string
	a.pluginGet = func(_ context.Context, url string) ([]byte, error) {
		asked = append(asked, url)
		if strings.Contains(url, "jsdelivr") && strings.HasSuffix(url, "/2026.json") {
			return []byte(testHolidays2026), nil
		}
		return nil, fmt.Errorf("404")
	}
	a.holidaysFetch()
	tt.Frame()
	if !holidayYearKnown(2026) || holidayYearKnown(2027) {
		t.Fatalf("years %v, asked %q", holidayYears(), asked)
	}
	d := func(m time.Month, day int) time.Time { return time.Date(2026, m, day, 0, 0, 0, 0, time.Local) }
	if workday(d(10, 1)) || !workday(d(10, 10)) || !workday(d(9, 27)) || workday(d(10, 11)) || !workday(d(10, 9)) {
		t.Fatal("workday reads the notice wrong")
	}
	// Asked again the same day: nothing is fetched.
	n := len(asked)
	a.holidaysFetch()
	if len(asked) != n {
		t.Fatalf("fetched again: %q", asked[n:])
	}
	// 法定工作日 from Wednesday the 30th of September: Thursday the 8th.
	r := &model.Repeat{Unit: model.RepeatWorkday}
	due := d(9, 30)
	if got := r.Next(&due, due); !got.Equal(d(10, 8)) {
		t.Fatalf("after the holiday: %v", got)
	}
	_, rep, first := parseRepeat("法定工作日 打卡", a.now())
	if rep == nil || rep.Unit != model.RepeatWorkday || !first.Equal(d(10, 10)) { // testNow, a Saturday worked
		t.Fatalf("parsed %+v %v", rep, first)
	}
	// The week of the holiday marks them.
	a.openTasks()
	a.tasks.view = tasksWeek
	a.tasks.day = d(10, 1)
	tt.Frame()
	tt.Frame()
	if _, ok := tt.Find("休 国庆节假期"); !ok {
		t.Fatalf("no 休: %q", tt.Texts())
	}
	snapshot(t, tt, "holidays-week")
	// From Sunday.
	a.cfg.WeekFromSunday = true
	a.tasks.weekAt = time.Time{}
	tt.Frame()
	tt.Frame()
	if !a.weekOf(d(10, 1)).Equal(d(9, 27)) {
		t.Fatalf("week of %v", a.weekOf(d(10, 1)))
	}
	if _, ok := tt.Find("班 国庆节调休，上班"); !ok { // Sunday the 27th, now in the week
		t.Fatalf("no 班: %q", tt.Texts())
	}
	snapshot(t, tt, "holidays-week-sunday")
	a.tasks.view = tasksMonth
	tt.Frame()
	tt.Frame()
	snapshot(t, tt, "holidays-month")
	// The same in the dark: the ground of rest is of the ink there too.
	tt.SetDark(true)
	tt.Frame()
	tt.Frame()
	snapshot(t, tt, "holidays-month-dark")
	a.tasks.view = tasksWeek
	a.tasks.weekAt = time.Time{}
	tt.Frame()
	tt.Frame()
	snapshot(t, tt, "holidays-week-dark")
	tt.SetDark(false)
	a.cfg.WeekFromSunday = false
	// The list of the seven days from today, Saturday the 10th, worked: 班
	// by 今天, and nothing by Sunday the 11th, a weekend as any.
	a.tasks.view = tasksWeekAhead
	tt.Frame()
	tt.Frame()
	if _, ok := tt.Find("班 国庆节调休，上班"); !ok {
		t.Fatalf("no 班 by today's heading: %q", tt.Texts())
	}
	if _, ok := tt.Find("休 国庆节假期"); ok {
		t.Fatal("a day off among the seven days from the 10th")
	}
	snapshot(t, tt, "holidays-list")
	// The small calendar of a task's day goes by the same: the days off and
	// the weekend on the ground, 休 and 班 at their corners; a click on a
	// day chooses it, as the arrows and Enter do.
	a.addTask("交报告", nil)
	a.tasks.view = tasksAll
	tt.Frame()
	dueOf := func() *time.Time {
		for _, it := range a.taskItems() {
			if it.node.Title == "交报告" {
				return it.node.Task.Due
			}
		}
		return nil
	}
	for _, it := range a.taskItems() {
		if it.node.Title == "交报告" {
			a.mutate("", func() { day := d(10, 9); it.node.Task.Due = &day })
			a.showTask(it)
		}
	}
	tt.Frame()
	tt.Frame()
	clickInPanel(t, tt, "截止日期")
	tt.Frame()
	tt.Frame()
	for _, label := range []string{"10月1日 周四 国庆节假期", "10月10日 周六 国庆节调休，上班", "10月11日 周日"} {
		if _, ok := tt.Find(label); !ok {
			t.Fatalf("no %q in the small calendar: %q", label, tt.Texts())
		}
	}
	snapshot(t, tt, "holidays-datepick")
	tt.SetDark(true)
	tt.Frame()
	tt.Frame()
	snapshot(t, tt, "holidays-datepick-dark")
	tt.SetDark(false)
	tt.Frame()
	if err := tt.Click("10月12日 周一"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if got := dueOf(); got == nil || !got.Equal(d(10, 12)) {
		t.Fatalf("clicked the 12th: %v", got)
	}
	if _, ok := tt.Find("10月11日 周日"); ok {
		t.Fatal("the small calendar stayed open")
	}
	clickInPanel(t, tt, "截止日期")
	tt.Frame()
	tt.Frame()
	tt.Key(0, ui.KeyRight)
	tt.Frame()
	tt.Key(0, ui.KeyPageDown)
	tt.Frame()
	if !tt.HasText("2026年11月") {
		t.Fatalf("Page Down: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	tt.Frame()
	if got := dueOf(); got == nil || !got.Equal(d(11, 13)) {
		t.Fatalf("→, Page Down, Enter from the 12th: %v", got)
	}
	// A repeat by the notices whose next day's year has none says so.
	a.addTask("法定工作日 写日报", nil)
	a.tasks.view = tasksAll
	tt.Frame()
	for _, it := range a.taskItems() {
		if it.node.Title == "写日报" {
			a.mutate("", func() { d := time.Date(2027, 3, 1, 0, 0, 0, 0, time.Local); it.node.Task.Due = &d })
			a.showTask(it)
		}
	}
	tt.Frame()
	tt.Frame()
	if !tt.HasText("2027 年的放假安排还没拿到，先按周一到周五算") {
		t.Fatalf("no word of 2027: %q", tt.Texts())
	}
}
