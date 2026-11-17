package main

import (
	"testing"
	"time"
)

// The terms fall on the days they fell on, those the almanacs and the
// common formula's exceptions name too.
func TestSolarTerms(t *testing.T) {
	for _, c := range []struct {
		date, want string
	}{
		{"2026-02-04", "立春"}, {"2026-02-18", "雨水"}, {"2026-04-05", "清明"}, {"2026-11-07", "立冬"},
		{"2026-11-22", "小雪"}, {"2026-12-22", "冬至"}, {"2025-02-03", "立春"}, {"2024-12-21", "冬至"},
		{"2023-12-22", "冬至"}, {"2021-12-21", "冬至"}, {"2019-01-05", "小寒"}, {"2024-06-21", "夏至"},
		{"2025-09-23", "秋分"}, {"2024-04-04", "清明"}, {"2008-05-21", "小满"}, {"2016-07-07", "小暑"},
		{"2002-08-08", "立秋"}, {"2027-03-21", "春分"}, {"2025-08-07", "立秋"}, {"2025-10-08", "寒露"},
	} {
		d, _ := time.ParseInLocation("2006-01-02", c.date, time.Local)
		if got, ok := solarTerm(d); !ok || got != c.want {
			t.Errorf("%s: %q, want %q", c.date, got, c.want)
		}
		if got, _ := solarTerm(d.AddDate(0, 0, 1)); got == c.want {
			t.Errorf("%s: the next day is %s too", c.date, got)
		}
	}
	// Every year has its 24, each on a day of its own.
	for y := 1950; y <= 2100; y++ {
		termsMu.Lock()
		delete(termsByDay, y)
		termsMu.Unlock()
		solarTerm(time.Date(y, 1, 1, 0, 0, 0, 0, time.Local))
		if n := len(termsByDay[y]); n != 24 {
			t.Fatalf("%d: %d terms", y, n)
		}
	}
}
