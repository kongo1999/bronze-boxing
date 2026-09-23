package server

import (
	"testing"
	"time"

	"bronzeboxing/internal/models"
)

// The badge counts open reminders by their effective studio day: a snoozed
// reminder isn't overdue, a done one isn't counted at all.
func TestReminderCountsForTheBadge(t *testing.T) {
	e := newEnv(t)
	day := func(d int) string { return models.DateKey(time.Now().AddDate(0, 0, d)) }
	add := func(title, due string) string {
		var r struct {
			ID string `json:"id"`
		}
		e.ok("POST", "/reminders", map[string]any{"title": title, "dueDay": due, "priority": "normal"}, &r)
		return r.ID
	}
	add("Call the landlord", day(-2))
	add("Order tape", day(0))
	add("Plan sparring", day(3))
	snoozed := add("Fix the bag", day(-1))
	e.ok("POST", "/reminders/"+snoozed+"/snooze", map[string]any{"days": 1}, nil)
	done := add("Pay electricity", day(-5))
	e.ok("PUT", "/reminders/"+done, map[string]any{"done": true}, nil)

	var got struct{ Overdue, Today, Open int }
	e.ok("GET", "/reminders/counts", nil, &got)
	if got.Overdue != 1 || got.Today != 1 || got.Open != 4 {
		t.Fatalf("counts = %+v, want overdue 1, today 1, open 4", got)
	}
}
