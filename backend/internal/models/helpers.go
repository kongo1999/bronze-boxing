package models

import (
	"fmt"
	"time"
)

// MonthKey returns the YYYY-MM key used for subscription periods.
func MonthKey(t time.Time) string {
	return fmt.Sprintf("%04d-%02d", t.Year(), int(t.Month()))
}

// MonthRange returns [start, end) for a YYYY-MM key (local time).
func MonthRange(key string) (time.Time, time.Time, error) {
	t, err := time.ParseInLocation("2006-01", key, time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.Local)
	return start, start.AddDate(0, 1, 0), nil
}
