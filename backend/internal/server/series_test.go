package server

import (
	"testing"
	"time"

	"bronzeboxing/internal/models"
)

func TestSeriesOccurrences(t *testing.T) {
	// Mon/Wed/Fri for two weeks from Monday 2026-09-21.
	occ, err := seriesSpec{Weekdays: []int{1, 3, 5}, Time: "18:00", FromDay: "2026-09-21", ToDay: "2026-10-04"}.occurrences()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"2026-09-21", "2026-09-23", "2026-09-25", "2026-09-28", "2026-09-30", "2026-10-02"}
	if len(occ) != len(want) {
		t.Fatalf("got %d occurrences, want %d", len(occ), len(want))
	}
	for i, o := range occ {
		if models.DateKey(o) != want[i] || o.In(time.Local).Hour() != 18 {
			t.Errorf("occ[%d] = %s, want %s 18:00", i, o.In(time.Local), want[i])
		}
	}
}

// A class at 18:00 stays at 18:00 studio time across the autumn clock change
// (Beirut leaves summer time at the end of October), even though its UTC
// instant shifts by an hour.
func TestSeriesOccurrencesAcrossDST(t *testing.T) {
	occ, err := seriesSpec{Weekdays: []int{0, 1, 2, 3, 4, 5, 6}, Time: "18:00", FromDay: "2026-10-20", ToDay: "2026-11-05"}.occurrences()
	if err != nil {
		t.Fatal(err)
	}
	offsets := map[int]bool{}
	for _, o := range occ {
		l := o.In(time.Local)
		if l.Hour() != 18 || l.Minute() != 0 {
			t.Fatalf("%s is not 18:00 studio time", l)
		}
		_, off := l.Zone()
		offsets[off] = true
	}
	if len(offsets) != 2 {
		t.Fatalf("expected the range to cross a UTC-offset change, saw offsets %v", offsets)
	}
	if len(occ) != 17 {
		t.Fatalf("got %d daily occurrences, want 17", len(occ))
	}
}

func TestSeriesOccurrencesWeekRolloverAndYearEnd(t *testing.T) {
	occ, err := seriesSpec{Weekdays: []int{0}, Time: "09:30", FromDay: "2026-12-26", ToDay: "2027-01-10"}.occurrences()
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, o := range occ {
		got = append(got, models.DateKey(o))
	}
	want := []string{"2026-12-27", "2027-01-03", "2027-01-10"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("Sundays = %v, want %v", got, want)
	}
}

func TestSeriesOccurrencesRejectsBadInput(t *testing.T) {
	cases := map[string]seriesSpec{
		"time":     {Weekdays: []int{1}, Time: "25:00", FromDay: "2026-09-21", ToDay: "2026-09-28"},
		"weekday":  {Weekdays: []int{7}, Time: "18:00", FromDay: "2026-09-21", ToDay: "2026-09-28"},
		"none":     {Time: "18:00", FromDay: "2026-09-21", ToDay: "2026-09-28"},
		"reversed": {Weekdays: []int{1}, Time: "18:00", FromDay: "2026-09-28", ToDay: "2026-09-21"},
		"too long": {Weekdays: []int{1}, Time: "18:00", FromDay: "2026-01-01", ToDay: "2028-01-01"},
		"too many": {Weekdays: []int{0, 1, 2, 3, 4, 5, 6}, Time: "18:00", FromDay: "2026-01-01", ToDay: "2026-12-31"},
		"bad day":  {Weekdays: []int{1}, Time: "18:00", FromDay: "2026-02-30", ToDay: "2026-03-10"},
	}
	for name, spec := range cases {
		if _, err := spec.occurrences(); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
