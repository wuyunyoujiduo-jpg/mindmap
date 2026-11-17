package main

import (
	"math"
	"sync"
	"time"
)

// The 24 solar terms: a term is the day, in China's time, on which the
// sun's apparent longitude reaches its fifteen degrees (小寒 at 285°, then
// every 15°). Of 2001 to 2100, by the almanacs' formula (寿星公式: the
// day is ⌊Y·0.2422 + C⌋ − ⌊Y/4⌋, Y the year's last two figures, C the
// term's), with the days it is known to miss; of other years, by the sun:
// its place by Meeus's low-precision formulas (Astronomical Algorithms,
// ch. 25), good to some fifteen minutes, which may give the day before or
// after for a term falling at midnight. The tests check both against the
// days the terms fell on.

// termC are the formula's constants of the 21st century, 小寒 first.
var termC = [24]float64{5.4055, 20.12, 3.87, 18.73, 5.63, 20.646, 4.81, 20.1, 5.52, 21.04, 5.678, 21.37,
	7.108, 22.83, 7.5, 23.13, 7.646, 23.042, 8.318, 23.438, 7.438, 22.36, 7.18, 21.94}

// termMiss are the days the formula misses, of 2001 to 2100: year and
// term to the days to add.
var termMiss = map[[2]int]int{
	{2019, 0}: -1, {2082, 1}: 1, {2026, 3}: -1, {2084, 5}: 1, {2008, 9}: 1, {2016, 12}: 1,
	{2002, 14}: 1, {2089, 19}: 1, {2089, 20}: 1, {2021, 23}: -1,
}

// termDay is the day of its month the term i of the year y falls on.
func termDay(y, i int) int {
	if y < 2001 || y > 2100 {
		return solarTermAt(y, i).In(chinaTime).Day()
	}
	Y := float64(y % 100)
	if y == 2100 {
		Y = 100
	}
	leaps := math.Floor(Y / 4)
	if i < 4 { // 小寒 to 雨水: before the year's leap day
		leaps = math.Floor((Y - 1) / 4)
	}
	return int(math.Floor(Y*0.2422+termC[i])) - int(leaps) + termMiss[[2]int{y, i}]
}

var solarTermNames = [24]string{"小寒", "大寒", "立春", "雨水", "惊蛰", "春分", "清明", "谷雨", "立夏", "小满", "芒种", "夏至",
	"小暑", "大暑", "立秋", "处暑", "白露", "秋分", "寒露", "霜降", "立冬", "小雪", "大雪", "冬至"}

// chinaTime is the time the terms' days are counted in, UTC+8.
var chinaTime = time.FixedZone("CST", 8*3600)

// sunLongitude is the sun's apparent longitude, in degrees, at the
// moment t.
func sunLongitude(t time.Time) float64 {
	// Julian day in terrestrial time: ΔT is some 69 seconds these years.
	jd := float64(t.Add(69*time.Second).Unix())/86400 + 2440587.5
	T := (jd - 2451545.0) / 36525
	rad := math.Pi / 180
	L0 := 280.46646 + 36000.76983*T + 0.0003032*T*T
	M := (357.52911 + 35999.05029*T - 0.0001537*T*T) * rad
	C := (1.914602-0.004817*T-0.000014*T*T)*math.Sin(M) + (0.019993-0.000101*T)*math.Sin(2*M) + 0.000289*math.Sin(3*M)
	omega := (125.04 - 1934.136*T) * rad
	lon := L0 + C - 0.00569 - 0.00478*math.Sin(omega)
	return math.Mod(math.Mod(lon, 360)+360, 360)
}

// solarTermAt is the moment of the term i (0 小寒 … 23 冬至) of the year y.
func solarTermAt(y, i int) time.Time {
	target := math.Mod(285+15*float64(i), 360)
	// It falls near the 5th or the 20th of its month: a few days each way.
	guess := time.Date(y, time.January, 6, 0, 0, 0, 0, time.UTC).Add(time.Duration(float64(i)*15.218*24) * time.Hour)
	lo, hi := guess.Add(-6*24*time.Hour), guess.Add(6*24*time.Hour)
	ahead := func(t time.Time) bool { // past the target, within half a turn
		return math.Mod(sunLongitude(t)-target+540, 360)-180 >= 0
	}
	for hi.Sub(lo) > time.Minute {
		mid := lo.Add(hi.Sub(lo) / 2)
		if ahead(mid) {
			hi = mid
		} else {
			lo = mid
		}
	}
	return hi
}

var (
	termsMu    sync.Mutex
	termsByDay = map[int]map[string]string{} // year → "01-05" → 小寒
)

// solarTerm names the term that falls on the day d, if one does.
func solarTerm(d time.Time) (string, bool) {
	y := d.Year()
	termsMu.Lock()
	days, ok := termsByDay[y]
	if !ok {
		days = map[string]string{}
		for i := range 24 {
			month := time.Month(i/2 + 1) // two terms a month, 小寒 and 大寒 January's
			days[time.Date(y, month, termDay(y, i), 0, 0, 0, 0, time.UTC).Format("01-02")] = solarTermNames[i]
		}
		termsByDay[y] = days
	}
	termsMu.Unlock()
	name, ok := days[d.Format("01-02")]
	return name, ok
}
