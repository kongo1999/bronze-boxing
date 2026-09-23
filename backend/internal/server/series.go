package server

import (
	"fmt"
	"regexp"
	"strconv"
	"time"

	"bronzeboxing/internal/models"
)

// Limits that keep a typo ("2036" for "2026") from generating years of classes.
const (
	maxSeriesSpanDays    = 366
	maxSeriesOccurrences = 200
)

var hhmmRe = regexp.MustCompile(`^([01]\d|2[0-3]):([0-5]\d)$`)

// seriesSpec describes a weekly recurrence in studio terms: these weekdays,
// at this wall-clock time, from one studio day through another (inclusive).
type seriesSpec struct {
	Weekdays []int  // 0 = Sunday … 6 = Saturday
	Time     string // "HH:MM", studio wall clock
	FromDay  string // YYYY-MM-DD
	ToDay    string // YYYY-MM-DD, inclusive
}

// occurrences lists every start instant the spec produces, in order. Each is
// the studio's wall-clock Time on its day, so a class stays at 18:00 across a
// daylight-saving change (its UTC instant moves, the studio time doesn't).
func (s seriesSpec) occurrences() ([]time.Time, error) {
	m := hhmmRe.FindStringSubmatch(s.Time)
	if m == nil {
		return nil, badField("time", "time must be HH:MM (24-hour)")
	}
	hh, _ := strconv.Atoi(m[1])
	mm, _ := strconv.Atoi(m[2])
	if len(s.Weekdays) == 0 {
		return nil, badField("weekdays", "pick at least one weekday")
	}
	want := map[time.Weekday]bool{}
	for _, d := range s.Weekdays {
		if d < 0 || d > 6 {
			return nil, badField("weekdays", "weekdays must be 0 (Sunday) to 6 (Saturday)")
		}
		want[time.Weekday(d)] = true
	}
	from, err := models.ParseDay(s.FromDay)
	if err != nil {
		return nil, badField("fromDay", "start date must be YYYY-MM-DD")
	}
	to, err := models.ParseDay(s.ToDay)
	if err != nil {
		return nil, badField("toDay", "end date must be YYYY-MM-DD")
	}
	if to.Before(from) {
		return nil, badField("toDay", "the end date is before the start date")
	}
	if to.Sub(from) > maxSeriesSpanDays*24*time.Hour+2*time.Hour { // +2h absorbs DST
		return nil, badField("toDay", fmt.Sprintf("a series can span at most %d days", maxSeriesSpanDays))
	}
	var out []time.Time
	for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
		if want[day.Weekday()] {
			out = append(out, time.Date(day.Year(), day.Month(), day.Day(), hh, mm, 0, 0, time.Local))
		}
	}
	if len(out) > maxSeriesOccurrences {
		return nil, badField("toDay", fmt.Sprintf("that would create %d sessions — a series is limited to %d", len(out), maxSeriesOccurrences))
	}
	return out, nil
}
