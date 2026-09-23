package models

import (
	"fmt"
	"math"
	"regexp"
	"time"
)

// All calendar helpers work in the studio's timezone. main() pins time.Local
// to STUDIO_TZ, so "the studio's day/month" is always time.Local — never the
// server's UTC clock and never the browser's zone. Mongo hands times back in
// UTC, which is why every helper converts with .In(time.Local) first.

var (
	monthRe = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)
	dateRe  = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])-(0[1-9]|[12]\d|3[01])$`)
)

// MonthKey returns the studio-local YYYY-MM for an instant.
func MonthKey(t time.Time) string {
	t = t.In(time.Local)
	return fmt.Sprintf("%04d-%02d", t.Year(), int(t.Month()))
}

// DateKey returns the studio-local YYYY-MM-DD for an instant.
func DateKey(t time.Time) string {
	return t.In(time.Local).Format("2006-01-02")
}

// ValidMonth reports whether s is a well-formed YYYY-MM in a sane range.
func ValidMonth(s string) bool {
	if !monthRe.MatchString(s) {
		return false
	}
	var y int
	fmt.Sscanf(s[:4], "%d", &y)
	return y >= 2000 && y <= 2100
}

// MonthRange returns [start, end) for a YYYY-MM key, as studio-local instants.
func MonthRange(key string) (time.Time, time.Time, error) {
	if !ValidMonth(key) {
		return time.Time{}, time.Time{}, fmt.Errorf("month must be YYYY-MM, got %q", key)
	}
	t, err := time.ParseInLocation("2006-01", key, time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.Local)
	return start, start.AddDate(0, 1, 0), nil
}

// ParseDay parses a studio-local YYYY-MM-DD into the instant that day starts.
func ParseDay(s string) (time.Time, error) {
	if !dateRe.MatchString(s) {
		return time.Time{}, fmt.Errorf("date must be YYYY-MM-DD, got %q", s)
	}
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return time.Time{}, err
	}
	if t.Year() < 2000 || t.Year() > 2100 {
		return time.Time{}, fmt.Errorf("date %q is out of range", s)
	}
	return t, nil
}

// ShiftMonth moves a YYYY-MM key by n months.
func ShiftMonth(key string, n int) string {
	start, _, err := MonthRange(key)
	if err != nil {
		return key
	}
	return MonthKey(start.AddDate(0, n, 0))
}

// MonthOfDay returns the YYYY-MM a YYYY-MM-DD belongs to.
func MonthOfDay(day string) string {
	if len(day) >= 7 {
		return day[:7]
	}
	return day
}

// StartOfDay is the studio-local midnight that begins t's day.
func StartOfDay(t time.Time) time.Time {
	t = t.In(time.Local)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
}

// Cents converts a money amount to integer cents (rounded half away from zero).
func Cents(v float64) int64 {
	return int64(math.Round(v * 100))
}

// Amount converts integer cents back to a money amount.
func Amount(c int64) float64 {
	return float64(c) / 100
}
