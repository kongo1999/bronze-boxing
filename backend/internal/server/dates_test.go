package server

import (
	"testing"
	"time"
)

// Date-only bounds are studio days, half-open: a week query from Monday to
// the next Monday includes a class at 00:30 on the first Monday (21:30 UTC
// the day before) and excludes one at 00:30 on the next.
func TestWeekQueryUsesStudioMidnight(t *testing.T) {
	e := newEnv(t)
	at := func(day, hhmm string) time.Time {
		tt, _ := time.ParseInLocation("2006-01-02 15:04", day+" "+hhmm, time.Local)
		return tt
	}
	mk := func(title string, start time.Time) {
		e.ok("POST", "/sessions", map[string]any{"title": title, "type": "private", "start": start, "durationMin": 30}, nil)
	}
	mk("first-monday-early", at("2026-09-21", "00:30"))
	mk("sunday-late", at("2026-09-27", "23:30"))
	mk("next-monday-early", at("2026-09-28", "00:30"))
	mk("previous-sunday-late", at("2026-09-20", "23:59"))

	var got []struct {
		Title string `json:"title"`
	}
	e.ok("GET", "/sessions?from=2026-09-21&to=2026-09-28", nil, &got)
	titles := map[string]bool{}
	for _, s := range got {
		titles[s.Title] = true
	}
	if !titles["first-monday-early"] || !titles["sunday-late"] || titles["next-monday-early"] || titles["previous-sunday-late"] || len(got) != 2 {
		t.Fatalf("week query returned %v", titles)
	}
}

// A month is the studio's month: a payment at 00:30 on the 1st (Beirut) is in
// that month even though it is still the previous month in UTC.
func TestMonthBoundaryIsStudioLocal(t *testing.T) {
	e := newEnv(t)
	first := time.Date(2026, 9, 1, 0, 30, 0, 0, time.Local)
	e.ok("POST", "/payments", map[string]any{"amount": 10, "type": "dropin", "date": first.UTC()}, nil)
	var fin struct {
		Income float64 `json:"income"`
	}
	e.ok("GET", "/financials?m=2026-09", nil, &fin)
	if fin.Income != 10 {
		t.Fatalf("September income = %v, want the 00:30-on-the-1st payment", fin.Income)
	}
	e.ok("GET", "/financials?m=2026-08", nil, &fin)
	if fin.Income != 0 {
		t.Fatalf("August income = %v, want 0", fin.Income)
	}
}

// Cents survive the round trip and totals add exactly (no float drift).
func TestMoneyRoundsToTheCent(t *testing.T) {
	e := newEnv(t)
	for _, amt := range []float64{12, 12.5, 12.55} {
		var ex struct {
			Amount float64 `json:"amount"`
		}
		e.ok("POST", "/expenses", map[string]any{"amount": amt, "category": "supplies", "day": "2026-09-10"}, &ex)
		if ex.Amount != amt {
			t.Fatalf("stored %v, want %v", ex.Amount, amt)
		}
	}
	var ex struct {
		Amount float64 `json:"amount"`
	}
	e.ok("POST", "/expenses", map[string]any{"amount": 0.1 + 0.2, "category": "supplies", "day": "2026-09-10"}, &ex)
	if ex.Amount != 0.3 {
		t.Fatalf("0.1+0.2 stored as %v, want 0.3", ex.Amount)
	}
	var fin struct {
		Outgoings float64 `json:"outgoings"`
	}
	e.ok("GET", "/financials?m=2026-09", nil, &fin)
	if fin.Outgoings != 37.35 {
		t.Fatalf("outgoings = %v, want 37.35 exactly", fin.Outgoings)
	}
}
