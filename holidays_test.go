package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

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
