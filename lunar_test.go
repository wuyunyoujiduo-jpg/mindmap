package main

import (
	"testing"
	"time"
)

func TestLunar(t *testing.T) {
	d := func(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 0, 0, 0, 0, time.Local) }
	for _, c := range []struct {
		d    time.Time
		want string
	}{
		{d(2026, 11, 15), "初七"}, // the user's screenshot
		{d(2026, 11, 17), "初九"},
		{d(2026, 11, 21), "十三"},
		{d(2026, 2, 17), "春节"},
		{d(2025, 1, 29), "春节"},
		{d(2024, 2, 10), "春节"},
		{d(2026, 2, 16), "除夕"},
		{d(2025, 10, 6), "中秋"},
		{d(2025, 7, 25), "闰六月"}, // 2025 has a leap sixth month
		{d(2026, 10, 1), "国庆节"},
		{d(2000, 2, 5), "春节"},
	} {
		if got, _ := lunarLabel(c.d); got != c.want {
			t.Errorf("%s: %q, want %q", c.d.Format("2006-01-02"), got, c.want)
		}
	}
}
